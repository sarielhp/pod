package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"cloud.google.com/go/vertexai/genai"
	"pod/pkg/types"
)

func CallGeminiAudioProcessor(ctx context.Context, projectID, location, gcsURI string) (*types.GeminiResponsePayload, error) {
	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		return nil, fmt.Errorf("failed to create vertex ai client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-flash")
	model.ResponseMIMEType = "application/json"
	model.SetTemperature(0.1)

	mimeType := "audio/mpeg"
	if strings.HasSuffix(strings.ToLower(gcsURI), ".wav") {
		mimeType = "audio/wav"
	}

	audioPart := genai.FileData{
		MIMEType: mimeType,
		FileURI:  gcsURI,
	}

	resp, err := model.GenerateContent(ctx, audioPart, genai.Text(types.GeminiAdRemovalPrompt))
	if err != nil {
		return nil, fmt.Errorf("gemini generate content failed: %w", err)
	}

	return ParseGeminiContentResponse(resp)
}

func ParseGeminiContentResponse(resp *genai.GenerateContentResponse) (*types.GeminiResponsePayload, error) {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("empty response received from Gemini")
	}

	var jsonText string
	for _, part := range resp.Candidates[0].Content.Parts {
		if textPart, ok := part.(genai.Text); ok {
			jsonText += string(textPart)
		}
	}

	return ParseGeminiJSONString(jsonText)
}

func ParseGeminiJSONString(jsonText string) (*types.GeminiResponsePayload, error) {
	trimmed := strings.TrimSpace(jsonText)
	if trimmed == "" {
		return nil, fmt.Errorf("empty JSON text received from Gemini")
	}
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 {
			end := len(lines)
			if strings.HasPrefix(lines[len(lines)-1], "```") {
				end = len(lines) - 1
			}
			trimmed = strings.TrimSpace(strings.Join(lines[1:end], "\n"))
		}
	}
	trimmed = normaliseClockTimestamps(trimmed)

	var payload types.GeminiResponsePayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini JSON: %w (raw response: %s)", err, jsonText)
	}
	return &payload, nil
}

// clockTimestamp matches a bare MM:SS or HH:MM:SS where a number belongs.
// Go's regexp has no lookahead, so the trailing delimiter is captured and
// written back rather than merely asserted.
var clockTimestamp = regexp.MustCompile(`("(?:start|end)"\s*:\s*)(\d{1,2}):([0-5]\d)(?::([0-5]\d))?(\s*[,}\]])`)

// normaliseClockTimestamps rewrites clock-style timestamps as seconds.
//
// The prompt asks for seconds and most models comply, but gemini-3.6-flash
// answers with "start": 01:39, which is not valid JSON — a bare 01:39 is not
// a number — so the whole response was discarded over a formatting habit.
// The conversion is unambiguous, so recovering the answer is better than
// spending another model's quota to ask again.
func normaliseClockTimestamps(s string) string {
	return clockTimestamp.ReplaceAllStringFunc(s, func(match string) string {
		parts := clockTimestamp.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		a, _ := strconv.Atoi(parts[2])
		b, _ := strconv.Atoi(parts[3])
		total := a*60 + b
		if parts[4] != "" {
			c, _ := strconv.Atoi(parts[4])
			total = a*3600 + b*60 + c
		}
		return fmt.Sprintf("%s%d%s", parts[1], total, parts[5])
	})
}
