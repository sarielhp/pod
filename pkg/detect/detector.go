package detect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"pod/pkg/types"
)

const SystemPrompt = `You are an expert podcast editor assistant.
Your job is to analyze the timestamped transcript of a podcast episode and identify all advertisement segments, host-read sponsor plugs, promotional breaks, midroll/preroll ads, and sponsor call-outs.

Return ONLY a raw JSON array of objects with the exact start and end seconds of each ad segment, like this:
[
  {"start": 15.0, "end": 65.5, "reason": "Host read sponsor plug for VPN"},
  {"start": 1200.0, "end": 1290.0, "reason": "Midroll ad break"}
]

If NO ads or sponsor plugs are found, return an empty JSON array: []
Do not include markdown formatting or commentary outside the JSON array.`

const KeywordExtractionPrompt = `You are a transcription assistant. Your job is to extract key topics, names, technical terms,
brand names, and unusual words from a podcast transcript segment.

Return ONLY a comma-separated list of 10-20 keywords/phrases (each 1-3 words).
Focus on: guest names, topic-specific jargon, product names, locations, and any words
that are unusual or easily misheard.

Keep each keyword short. Do not include markdown or commentary.`

type LLMRequest struct {
	Model       string       `json:"model"`
	Messages    []LLMMessage `json:"messages"`
	Temperature float64      `json:"temperature"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
}

type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMResponse struct {
	Choices []LLMChoice `json:"choices"`
	Usage   LLMUsage    `json:"usage"`
}

// LLMUsage is what a call consumed. Providers report it on every response and
// pod discarded it, which left no way to see what detection costs, nor to
// tell a cached call from a full-price one — the difference that decides
// whether two identical answers mean the model is reproducible or merely that
// nothing was recomputed.
type LLMUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`

	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// CachedTokens is how much of the prompt the provider served from its cache.
func (u LLMUsage) CachedTokens() int { return u.PromptTokensDetails.CachedTokens }

// Add accumulates another call's usage, so a detection that asked several
// times reports what the whole detection cost rather than the last request.
func (u *LLMUsage) Add(other LLMUsage) {
	u.PromptTokens += other.PromptTokens
	u.CompletionTokens += other.CompletionTokens
	u.TotalTokens += other.TotalTokens
	u.PromptTokensDetails.CachedTokens += other.PromptTokensDetails.CachedTokens
}

type LLMChoice struct {
	Message LLMMessage `json:"message"`
}

var sharedLLMClient = &http.Client{
	Timeout: 120 * time.Second,
}

var DefaultLLMTimeout = 120 * time.Second

// CallLLMChat sends one chat completion request and returns its content.
func CallLLMChat(profile types.LLMProfile, sysPrompt, userPrompt string, maxTokens int, timeout time.Duration, apiKey string) (string, error) {
	content, _, err := CallLLMChatUsage(profile, sysPrompt, userPrompt, maxTokens, timeout, apiKey)
	return content, err
}

// CallLLMChatUsage is CallLLMChat, additionally reporting what the call cost.
func CallLLMChatUsage(profile types.LLMProfile, sysPrompt, userPrompt string, maxTokens int, timeout time.Duration, apiKey string) (string, LLMUsage, error) {
	payload := LLMRequest{
		Model: profile.Model,
		Messages: []LLMMessage{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.1,
		MaxTokens:   maxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", LLMUsage{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	client := sharedLLMClient
	if timeout > 0 && timeout != sharedLLMClient.Timeout {
		client = &http.Client{Timeout: timeout}
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if d := retryBackoff(attempt); d > 0 {
				time.Sleep(d)
			}
		}
		req, err := http.NewRequest("POST", profile.URL, bytes.NewReader(body))
		if err != nil {
			return "", LLMUsage{}, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("server returned status code %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return "", LLMUsage{}, fmt.Errorf("server returned status code %d: %s", resp.StatusCode, string(respBody))
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", LLMUsage{}, fmt.Errorf("failed to read response: %w", err)
		}

		var llmResp LLMResponse
		if err := json.Unmarshal(respBody, &llmResp); err != nil {
			return "", LLMUsage{}, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if len(llmResp.Choices) == 0 {
			return "", LLMUsage{}, fmt.Errorf("no choices in response")
		}

		return llmResp.Choices[0].Message.Content, llmResp.Usage, nil
	}
	return "", LLMUsage{}, lastErr
}

func DetectAdsLLM(transcriptText string, profile types.LLMProfile, apiKey string) ([]types.AdSegment, error) {
	return DetectAdsLLMTimeout(transcriptText, profile, apiKey, DefaultLLMTimeout)
}

// EmptyResultConfirmations is how many extra agreeing answers are required
// before "no ads" is believed. An empty answer is the one result that is
// cheap for the model to produce and expensive to get wrong: it is written
// to the cuts file, marks the episode clean, and makes every later run skip
// it. Sampling the same model repeatedly shows it returns an empty array on
// roughly one call in eight for an episode that plainly contains an ad, so a
// single empty answer is not evidence of an ad-free episode.
var EmptyResultConfirmations = 2

// DetectAdsLLMOnce asks exactly once and returns whatever comes back.
// Callers that only need to know the endpoint answers correctly — the
// profile probe, for one — use this, so a legitimately empty answer does not
// cost three requests.
func DetectAdsLLMOnce(transcriptText string, profile types.LLMProfile, apiKey string, timeout time.Duration) ([]types.AdSegment, error) {
	if profile.URL == "" {
		return nil, nil
	}
	return askForAdSegments(profile, adUserPrompt(transcriptText), timeout, apiKey)
}

func adUserPrompt(transcriptText string) string {
	return fmt.Sprintf("Here is the podcast transcript with timestamps in seconds:\n\n%s", transcriptText)
}

func DetectAdsLLMTimeout(transcriptText string, profile types.LLMProfile, apiKey string, timeout time.Duration) ([]types.AdSegment, error) {
	segs, _, err := DetectAdsLLMTimeoutUsage(transcriptText, profile, apiKey, timeout)
	return segs, err
}

// DetectAdsLLMTimeoutUsage is DetectAdsLLMTimeout, additionally reporting what
// the detection cost. The total covers the confirmation re-asks as well as the
// first request, because an empty first answer silently triples the cost of a
// detection and that is worth being able to see.
func DetectAdsLLMTimeoutUsage(transcriptText string, profile types.LLMProfile, apiKey string, timeout time.Duration) ([]types.AdSegment, LLMUsage, error) {
	var total LLMUsage
	if profile.URL == "" {
		return nil, total, nil
	}
	userPrompt := adUserPrompt(transcriptText)
	segs, usage, err := askForAdSegmentsUsage(profile, userPrompt, timeout, apiKey)
	total.Add(usage)
	if err != nil || len(segs) > 0 {
		return segs, total, err
	}
	// Empty answer: re-ask before accepting it. Any confirmation run that
	// does find ads wins, because a miss is what an empty answer looks like.
	for i := 0; i < EmptyResultConfirmations; i++ {
		retry, retryUsage, retryErr := askForAdSegmentsUsage(profile, userPrompt, timeout, apiKey)
		total.Add(retryUsage)
		if retryErr != nil {
			return nil, total, fmt.Errorf("ad detection confirmation failed: %w", retryErr)
		}
		if len(retry) > 0 {
			return retry, total, nil
		}
	}
	return segs, total, nil
}

func askForAdSegments(profile types.LLMProfile, userPrompt string, timeout time.Duration, apiKey string) ([]types.AdSegment, error) {
	segs, _, err := askForAdSegmentsUsage(profile, userPrompt, timeout, apiKey)
	return segs, err
}

func askForAdSegmentsUsage(profile types.LLMProfile, userPrompt string, timeout time.Duration, apiKey string) ([]types.AdSegment, LLMUsage, error) {
	content, usage, err := CallLLMChatUsage(profile, SystemPrompt, userPrompt, 0, timeout, apiKey)
	if err != nil {
		return nil, usage, fmt.Errorf("LLM ad detection failed: %w", err)
	}
	segs, err := ExtractJSONArray(content)
	return segs, usage, err
}

func ExtractJSONArray(content string) ([]types.AdSegment, error) {
	start := strings.IndexByte(content, '[')
	if start < 0 {
		return nil, fmt.Errorf("no JSON array start found in response")
	}

	end := -1
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(content); i++ {
		c := content[i]
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				end = i
				i = len(content)
			}
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("no matching JSON array end found in response")
	}

	var ads []types.AdSegment
	if err := json.Unmarshal([]byte(content[start:end+1]), &ads); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ads JSON: %w", err)
	}
	return ads, nil
}

func ExtractKeywordsLLM(transcriptText string, profile types.LLMProfile, apiKey string, quiet bool) string {
	userPrompt := fmt.Sprintf("Extract keywords from this podcast transcript segment:\n\n%s", transcriptText)
	content, err := CallLLMChat(profile, KeywordExtractionPrompt, userPrompt, 200, 60*time.Second, apiKey)
	if err != nil {
		if !quiet {
			fmt.Fprintf(os.Stderr, "\nError during keyword extraction: %v\n\n", err)
		}
		return ""
	}

	var keywords []string
	var current strings.Builder
	for _, ch := range content {
		if ch == ',' || ch == '[' || ch == ']' || ch == '"' {
			if current.Len() > 0 {
				keywords = append(keywords, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		keywords = append(keywords, current.String())
	}

	var cleaned []string
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			cleaned = append(cleaned, kw)
		}
	}
	if len(cleaned) > 30 {
		cleaned = cleaned[:30]
	}

	return strings.Join(cleaned, ", ")
}
