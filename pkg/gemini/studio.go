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
	"strings"
	"time"

	"pod/pkg/port"
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

	go PipeMultipartAudio(pw, mpw, localAudioPath, AudioMIMEType(localAudioPath))

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

// CallGeminiStudioProcessor transcribes one uploaded file with one model.
//
// It performs exactly one request. Retrying, waiting and moving to another
// model belong to pkg/port, which is the only component that can see the
// whole picture: a second retry loop here would spend quota the port was
// trying to conserve, and the two schedules would drift apart — which is the
// condition that let transcription and ad detection disagree about how to
// treat a rate limit in the first place.
// AudioMIMEType is the type to declare for an audio file.
//
// The upload and the generateContent request must agree: the API rejects a
// request whose declared type differs from the type the file was stored
// under, with "MIME type audio/mpeg does not match parent MIME type
// audio/wav". The type was hardcoded on the request side, so every chunk —
// which pod converts to WAV — was declared as MP3. Models differed in whether
// they enforced it, which is why this surfaced only on one of them.
func AudioMIMEType(path string) string {
	if strings.HasSuffix(strings.ToLower(path), ".wav") {
		return "audio/wav"
	}
	return "audio/mpeg"
}

func CallGeminiStudioProcessor(ctx context.Context, apiKey, modelName, fileURI, mimeType string) (*types.GeminiResponsePayload, error) {
	payload, attempt := callGeminiStudioOnce(ctx, apiKey, modelName, fileURI, mimeType)
	if attempt.Verdict == port.Success {
		return payload, nil
	}
	return nil, attempt.Err
}

// callGeminiStudioOnce performs one generateContent request and reports both
// the payload and what the port should make of the outcome.
func callGeminiStudioOnce(ctx context.Context, apiKey, modelName, fileURI, mimeType string) (*types.GeminiResponsePayload, port.Attempt) {
	apiKey, err := validateKey(apiKey)
	if err != nil {
		return nil, port.Fail(err)
	}
	if modelName == "" {
		modelName = defaultGeminiModel
	}
	reqBytes, err := buildStudioGeneratePayload(fileURI, mimeType)
	if err != nil {
		return nil, port.Fail(err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", modelName)
	client := &http.Client{Timeout: 5 * time.Minute}

	body, statusCode, reqErr := executeStudioRequest(ctx, client, url, apiKey, reqBytes)
	if reqErr != nil {
		if ctx.Err() != nil {
			return nil, port.Fail(fmt.Errorf("gemini studio request failed: %w", reqErr))
		}
		// A transport failure is worth another go on the same model.
		return nil, port.Attempt{Verdict: port.Overloaded, Err: fmt.Errorf("gemini studio request failed: %w", reqErr)}
	}
	if statusCode == http.StatusOK {
		payload, err := ParseGeminiStudioResponse(body)
		if err != nil {
			// The model answered with something unusable — a blocked
			// candidate, or JSON in the wrong shape. Models differ in this:
			// gemini-3.6-flash writes timestamps as "01:39" where the prompt
			// asks for seconds, which is not valid JSON at all. That is the
			// model's behaviour, not the request's, so the next model is
			// worth trying rather than giving up on the whole chain.
			return nil, port.Attempt{Verdict: port.Unusable, Err: err}
		}
		return payload, port.OK()
	}
	return nil, studioFailure(modelName, statusCode, body)
}

// studioFailure classifies a non-OK response.
func studioFailure(modelName string, statusCode int, body []byte) port.Attempt {
	err := studioHTTPError(statusCode, body)
	switch {
	case statusCode == http.StatusTooManyRequests:
		_, perModel, _ := port.QuotaDetail(body)
		return port.Attempt{
			Verdict:    port.RateLimited,
			RetryAfter: port.ParseRetryAfter(body),
			Daily:      IsGeminiDailyQuotaExhausted(body),
			PerModel:   perModel,
			Err:        err,
		}
	case statusCode == http.StatusNotFound:
		// Retired models answer 404 ("no longer available to new users"),
		// which is permanent and pinning a model makes it inevitable.
		return port.Attempt{Verdict: port.ModelGone, Err: err}
	case studioIsOverload(statusCode):
		return port.Attempt{Verdict: port.Overloaded, Err: err}
	default:
		return port.Fail(err)
	}
}

func studioHTTPError(statusCode int, body []byte) error {
	return fmt.Errorf("gemini studio generateContent HTTP %d: %s", statusCode, FormatGeminiErrorBody(body))
}

func studioIsOverload(statusCode int) bool {
	return statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout
}

func buildStudioGeneratePayload(fileURI, mimeType string) ([]byte, error) {
	if mimeType == "" {
		mimeType = "audio/mpeg"
	}
	reqPayload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{
						"file_data": map[string]string{
							"mime_type": mimeType,
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

func IsGeminiDailyQuotaExhausted(body []byte) bool {
	// The quotaId names the quota actually violated; everything below is a
	// fallback for responses that do not carry one.
	if daily, known := port.QuotaScope(body); known {
		return daily
	}
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
