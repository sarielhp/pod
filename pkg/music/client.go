// Package music classifies audio with a caller-configured YAMNet HTTP service.
package music

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/util"
)

// Frame contains raw scores for a 0.96-second window beginning at Time.
type Frame struct {
	Time    float64 `json:"time"`
	Speech  float64 `json:"speech_score"`
	Music   float64 `json:"music_score"`
	Silence float64 `json:"silence_score"`
}

// Result retains frame scores rather than the service's overlapping summaries.
type Result struct {
	Duration float64 `json:"duration"`
	Frames   []Frame `json:"frames"`
}

type cache struct {
	Version  int    `json:"version"`
	Endpoint string `json:"endpoint"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Result   Result `json:"result"`
}

// Classify caches a successful full-source classification at cachePath.
func Classify(ctx context.Context, endpoint, audioPath, cachePath string) (*Result, error) {
	fi, err := os.Stat(audioPath)
	if err != nil {
		return nil, fmt.Errorf("stat classifier input: %w", err)
	}
	endpoint = strings.TrimRight(endpoint, "/")
	key := cache{Version: 1, Endpoint: fmt.Sprintf("%x", sha256.Sum256([]byte(endpoint))), Size: fi.Size(), Modified: fi.ModTime().UnixNano()}
	if data, err := os.ReadFile(cachePath); err == nil {
		var saved cache
		if json.Unmarshal(data, &saved) == nil && saved.Version == key.Version && saved.Endpoint == key.Endpoint && saved.Size == key.Size && saved.Modified == key.Modified && validateResult(&saved.Result) == nil {
			return &saved.Result, nil
		}
	}
	result, err := infer(ctx, endpoint, audioPath)
	if err != nil {
		return nil, err
	}
	key.Result = *result
	data, err := json.Marshal(key)
	if err != nil {
		return nil, fmt.Errorf("encode music cache: %w", err)
	}
	if err := util.WriteFileAtomic(cachePath, data, 0644); err != nil {
		return nil, fmt.Errorf("save music cache: %w", err)
	}
	return result, nil
}

func infer(ctx context.Context, endpoint, audioPath string) (*Result, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid yamnet_url %q", endpoint)
	}
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/inference"), "/predict")
	base := strings.TrimRight(u.String(), "/")
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 5 * time.Minute}
	if err := awaitReady(ctx, client, base); err != nil {
		return nil, err
	}
	req, err := uploadRequest(ctx, base+"/inference", audioPath)
	if err != nil {
		return nil, err
	}
	defer req.Body.Close()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("classify music: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("read music classification: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("music classifier returned HTTP %d", resp.StatusCode)
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode music classification: %w", err)
	}
	if err := validateResult(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func awaitReady(ctx context.Context, client *http.Client, base string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		if healthy(ctx, client, base+"/health") {
			return nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("wait for music classifier: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func healthy(ctx context.Context, client *http.Client, endpoint string) bool {
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probe, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var status struct {
		Status string `json:"status"`
	}
	return resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&status) == nil && status.Status == "healthy"
}

func uploadRequest(ctx context.Context, endpoint, audioPath string) (*http.Request, error) {
	f, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("open classifier input: %w", err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	var header bytes.Buffer
	w := multipart.NewWriter(&header)
	if err := w.WriteField("threshold", "0.1"); err != nil {
		f.Close()
		return nil, err
	}
	if err := w.WriteField("include_frames", "true"); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := w.CreateFormFile("file", filepath.Base(audioPath)); err != nil {
		f.Close()
		return nil, err
	}
	prefix := append([]byte(nil), header.Bytes()...)
	header.Reset()
	if err := w.Close(); err != nil {
		f.Close()
		return nil, err
	}
	suffix := append([]byte(nil), header.Bytes()...)
	body := &uploadBody{Reader: io.MultiReader(bytes.NewReader(prefix), f, bytes.NewReader(suffix)), file: f}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		f.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.ContentLength = int64(len(prefix)+len(suffix)) + fi.Size()
	return req, nil
}

type uploadBody struct {
	io.Reader
	file *os.File
}

func (b *uploadBody) Close() error { return b.file.Close() }

func validateResult(result *Result) error {
	if result.Duration <= 0 || math.IsNaN(result.Duration) || math.IsInf(result.Duration, 0) || len(result.Frames) == 0 {
		return fmt.Errorf("music classifier returned no usable frame coverage")
	}
	last := -1.0
	for _, f := range result.Frames {
		if !validScore(f.Speech) || !validScore(f.Music) || !validScore(f.Silence) || math.IsNaN(f.Time) || math.IsInf(f.Time, 0) || f.Time < 0 || f.Time <= last || f.Time-last > 1 {
			return fmt.Errorf("music classifier returned invalid or missing frames")
		}
		last = f.Time
	}
	if last+0.96 < result.Duration-0.5 {
		return fmt.Errorf("music classifier frame coverage ended prematurely")
	}
	return nil
}

func validScore(score float64) bool { return !math.IsNaN(score) && score >= 0 && score <= 1 }
