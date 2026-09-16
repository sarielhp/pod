package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pod/pkg/types"
	"pod/pkg/util"
)

const defaultGeminiModel = "gemini-flash-latest"

type geminiStudioFileUploadResponse struct {
	File struct {
		Name     string `json:"name"`
		URI      string `json:"uri"`
		MimeType string `json:"mimeType"`
		State    string `json:"state"`
	} `json:"file"`
}

type geminiStudioGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	// ModelVersion is the model that actually answered. It matters because
	// the model we ask for may be an alias: "gemini-flash-latest" resolves to
	// a different model over time, so recording the alias on a transcript
	// says nothing about what produced it.
	ModelVersion string `json:"modelVersion"`
}

// ModelUnavailableError means this particular model cannot serve the request
// now — its quota is spent, or it has been retired — while another model
// might. The free tier meters requests per model, so trying the next one is
// usually the difference between a transcript and a fallback to whisper.
type ModelUnavailableError struct {
	Model  string
	Status int
	Body   []byte
	Daily  bool
	Err    error
}

func (e *ModelUnavailableError) Error() string { return e.Err.Error() }
func (e *ModelUnavailableError) Unwrap() error { return e.Err }

func validateKey(apiKey string) (string, error) {
	if apiKey == "" || util.IsZeroedKey(apiKey) {
		if apiKey != "" {
			_ = util.ZeroWipeKey(apiKey)
		}
		return "", fmt.Errorf("gemini API key is disabled in configuration")
	}
	return apiKey, nil
}

func PipeMultipartAudio(pw *io.PipeWriter, mpw *multipart.Writer, localAudioPath, mimeType string) {
	var err error
	defer func() {
		if err != nil {
			_ = pw.CloseWithError(err)
		} else {
			_ = pw.Close()
		}
	}()

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="metadata"`)
	h.Set("Content-Type", "application/json; charset=UTF-8")
	part, err := mpw.CreatePart(h)
	if err != nil {
		return
	}
	meta := fmt.Sprintf(`{"file": {"display_name": %q}}`, filepath.Base(localAudioPath))
	if _, err = part.Write([]byte(meta)); err != nil {
		return
	}

	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filepath.Base(localAudioPath)))
	fileHeader.Set("Content-Type", mimeType)
	filePart, err := mpw.CreatePart(fileHeader)
	if err != nil {
		return
	}

	f, err := os.Open(localAudioPath)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err = io.Copy(filePart, f); err != nil {
		return
	}
	err = mpw.Close()
}

func UploadAudioToGeminiStudio(ctx context.Context, apiKey, localAudioPath string) (string, string, error) {
	var err error
	apiKey, err = validateKey(apiKey)
	if err != nil {
		return "", "", err
	}

	pr, pw := io.Pipe()
	mpw := multipart.NewWriter(pw)

	mimeType := "audio/mpeg"
	if strings.HasSuffix(strings.ToLower(localAudioPath), ".wav") {
		mimeType = "audio/wav"
	}

	go PipeMultipartAudio(pw, mpw, localAudioPath, mimeType)

	url := "https://generativelanguage.googleapis.com/upload/v1beta/files"
	req, err := http.NewRequestWithContext(ctx, "POST", url, pr)
	if err != nil {
		return "", "", fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("x-goog-api-key", apiKey)
	req.Header.Set("X-Goog-Upload-Protocol", "multipart")
	req.Header.Set("Content-Type", mpw.FormDataContentType())

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("gemini file upload failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("gemini file upload HTTP %d: %s", resp.StatusCode, FormatGeminiErrorBody(body))
	}

	var res geminiStudioFileUploadResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return "", "", fmt.Errorf("failed to parse upload response: %w", err)
	}
	return res.File.URI, res.File.Name, nil
}

func DeleteGeminiStudioFile(ctx context.Context, apiKey, fileName string) {
	if fileName == "" || apiKey == "" {
		return
	}
	var err error
	apiKey, err = validateKey(apiKey)
	if err != nil {
		return
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/%s", fileName)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("x-goog-api-key", apiKey)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

type geminiErrorDetail struct {
	Type     string            `json:"@type"`
	Reason   string            `json:"reason"`
	Domain   string            `json:"domain"`
	Metadata map[string]string `json:"metadata"`
}

type geminiErrorResponse struct {
	Error struct {
		Code    int                 `json:"code"`
		Message string              `json:"message"`
		Status  string              `json:"status"`
		Details []geminiErrorDetail `json:"details"`
	} `json:"error"`
}

// CallGeminiStudioProcessor transcribes one uploaded file with one model,
// waiting out a rate limit rather than giving up on it.
func CallGeminiStudioProcessor(ctx context.Context, apiKey, modelName, fileURI string) (*types.GeminiResponsePayload, error) {
	return callGeminiStudioModel(ctx, apiKey, modelName, fileURI, true)
}

// callGeminiStudioModel transcribes one uploaded file with one model.
//
// waitOutRateLimit says whether a 429 is worth sitting through. It is only
// true for the last model in the chain: while another model remains, waiting
// the minute the server asks for is a minute spent not transcribing, when a
// model with its own untouched quota would answer immediately.
func callGeminiStudioModel(ctx context.Context, apiKey, modelName, fileURI string, waitOutRateLimit bool) (*types.GeminiResponsePayload, error) {
	var err error
	apiKey, err = validateKey(apiKey)
	if err != nil {
		return nil, err
	}
	if modelName == "" {
		modelName = defaultGeminiModel
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", modelName)

	reqBytes, err := buildStudioGeneratePayload(fileURI)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	// Rate limiting and capacity are different problems and get different
	// budgets. A 429 means this key is asking for too much, so retrying
	// harder makes it worse; a 503 means the model is momentarily
	// oversubscribed, which no amount of restraint on our side fixes and
	// which usually clears in well under a minute. Three attempts over six
	// seconds is not long enough to ride out a demand spike, and the cost of
	// giving up is a whole transcript silently produced by a weaker engine.
	const maxAttempts = 3
	const maxOverloadAttempts = 6
	var lastErr error
	overloaded := false
	budget := func() int {
		if overloaded {
			return maxOverloadAttempts
		}
		return maxAttempts
	}

	for attempt := 1; attempt <= budget(); attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		body, statusCode, reqErr := executeStudioRequest(ctx, client, url, apiKey, reqBytes)
		switch {
		case reqErr != nil:
			lastErr = fmt.Errorf("gemini studio request failed: %w", reqErr)
		case statusCode == http.StatusOK:
			return ParseGeminiStudioResponse(body)
		default:
			lastErr = studioHTTPError(statusCode, body)
			giveUpOnRateLimit := !waitOutRateLimit || attempt == maxAttempts
			if unavailable := studioModelUnavailable(modelName, statusCode, body, giveUpOnRateLimit); unavailable != nil {
				return nil, unavailable
			}
			if !studioIsOverload(statusCode) {
				return nil, lastErr
			}
			overloaded = true
		}

		if attempt >= budget() {
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(studioRetryDelay(attempt, overloaded, body)):
		}
	}

	return nil, lastErr
}

func studioHTTPError(statusCode int, body []byte) error {
	return fmt.Errorf("gemini studio generateContent HTTP %d: %s", statusCode, FormatGeminiErrorBody(body))
}

func studioIsOverload(statusCode int) bool {
	return statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout
}

// studioModelUnavailable reports whether a failed response means this model
// cannot serve the request at all, as opposed to a transient failure worth
// retrying. Whether to stop calling Gemini altogether is deliberately not
// decided here: another model may still have quota, and only the caller knows
// whether one remains.
func studioModelUnavailable(modelName string, statusCode int, body []byte, giveUpOnRateLimit bool) *ModelUnavailableError {
	err := studioHTTPError(statusCode, body)
	// Retired models answer 404 ("no longer available to new users"), which is
	// permanent, and pinning a model makes it inevitable eventually.
	if statusCode == http.StatusNotFound {
		return &ModelUnavailableError{Model: modelName, Status: statusCode, Body: body, Err: err}
	}
	if statusCode != http.StatusTooManyRequests {
		return nil
	}
	if IsGeminiDailyQuotaExhausted(body) {
		return &ModelUnavailableError{Model: modelName, Status: statusCode, Body: body, Daily: true, Err: err}
	}
	if giveUpOnRateLimit {
		return &ModelUnavailableError{Model: modelName, Status: statusCode, Body: body, Err: err}
	}
	return nil
}

// studioRetryDelay is how long to wait before the next attempt. A delay the
// server asked for beats any guess of ours.
func studioRetryDelay(attempt int, overloaded bool, body []byte) time.Duration {
	if requested := ExtractGeminiRetryDelay(body); requested > 0 && requested <= 60*time.Second {
		return requested
	}
	if overloaded {
		// 2s, 4s, 8s, 16s, 32s: about a minute in total, which is the
		// timescale Google's own "spikes are usually temporary" message
		// refers to.
		return time.Duration(1<<attempt) * time.Second
	}
	return time.Duration(attempt*2) * time.Second
}

func buildStudioGeneratePayload(fileURI string) ([]byte, error) {
	reqPayload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{
						"file_data": map[string]string{
							"mime_type": "audio/mpeg",
							"file_uri":  fileURI,
						},
					},
					{
						"text": types.GeminiAdRemovalPrompt,
					},
				},
			},
		},
		"generationConfig": map[string]any{
			"response_mime_type": "application/json",
			"temperature":        0.1,
		},
	}
	data, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal studio request: %w", err)
	}
	return data, nil
}

func executeStudioRequest(ctx context.Context, client *http.Client, url, apiKey string, reqBytes []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create studio request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func FormatGeminiErrorBody(body []byte) string {
	var errResp geminiErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		msg := strings.TrimSpace(errResp.Error.Message)
		if quotaInfo := extractGeminiQuotaDetails(&errResp); quotaInfo != "" {
			return fmt.Sprintf("%s [%s]", msg, quotaInfo)
		}
		if errResp.Error.Status != "" {
			return fmt.Sprintf("%s (%s)", msg, errResp.Error.Status)
		}
		return msg
	}
	return strings.TrimSpace(string(body))
}

func extractGeminiQuotaDetails(errResp *geminiErrorResponse) string {
	for _, d := range errResp.Error.Details {
		if len(d.Metadata) == 0 {
			continue
		}
		limit := d.Metadata["quota_limit"]
		val := d.Metadata["quota_limit_value"]
		consumer := d.Metadata["consumer"]

		var parts []string
		if limit != "" {
			if strings.Contains(limit, "PerDay") || val == "1500" {
				parts = append(parts, "Free Tier: Daily quota exhausted (1,500 req/day limit)")
			} else if strings.Contains(limit, "PerMinute") || val == "15" {
				parts = append(parts, "Free Tier: Rate limit exceeded (15 req/min limit)")
			} else {
				parts = append(parts, fmt.Sprintf("Quota: %s", limit))
			}
		}
		if val != "" && !strings.Contains(limit, "PerDay") && !strings.Contains(limit, "PerMinute") {
			parts = append(parts, fmt.Sprintf("Limit: %s", val))
		}
		if consumer != "" {
			parts = append(parts, fmt.Sprintf("Project: %s", strings.TrimPrefix(consumer, "projects/")))
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	}
	return ""
}

var geminiRetryRegex = regexp.MustCompile(`(?i)(?:please retry in|retry in)\s+([0-9.]+)\s*(s(?:ec(?:ond)?)?|m(?:in(?:ute)?)?)?`)

func ExtractGeminiRetryDelay(body []byte) time.Duration {
	matches := geminiRetryRegex.FindSubmatch(body)
	if len(matches) < 2 {
		return 0
	}
	val, err := strconv.ParseFloat(string(matches[1]), 64)
	if err != nil || val <= 0 {
		return 0
	}
	unit := ""
	if len(matches) >= 3 {
		unit = strings.ToLower(string(matches[2]))
	}
	if strings.HasPrefix(unit, "m") {
		return time.Duration(val*60*float64(time.Second)) + time.Second
	}
	return time.Duration((val + 1.0) * float64(time.Second))
}

func IsGeminiDailyQuotaExhausted(body []byte) bool {
	var errResp geminiErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return false
	}
	msg := strings.ToLower(errResp.Error.Message)
	if strings.Contains(msg, "please retry in") || strings.Contains(msg, "retry after") {
		return false
	}
	for _, d := range errResp.Error.Details {
		limit := d.Metadata["quota_limit"]
		val := d.Metadata["quota_limit_value"]
		if strings.Contains(limit, "PerMinute") || val == "15" || val == "20" {
			return false
		}
		if strings.Contains(limit, "PerDay") || val == "1500" {
			return true
		}
	}
	if strings.Contains(msg, "free_tier_requests") && (strings.Contains(msg, "limit: 15") || strings.Contains(msg, "limit: 20")) {
		return false
	}
	return strings.Contains(msg, "exceeded your current quota") && !strings.Contains(msg, "per minute")
}

func ParseGeminiStudioResponse(body []byte) (*types.GeminiResponsePayload, error) {
	var res geminiStudioGenerateResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("failed to parse studio response json: %w", err)
	}
	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty candidate in studio response: %s", string(body))
	}
	var sb strings.Builder
	for _, part := range res.Candidates[0].Content.Parts {
		sb.WriteString(part.Text)
	}
	payload, err := ParseGeminiJSONString(sb.String())
	if payload != nil {
		payload.ModelVersion = res.ModelVersion
	}
	return payload, err
}
