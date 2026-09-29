package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"pod/pkg/format"
	"pod/pkg/types"
	"pod/pkg/util"
)

const WavBytesPerSec = WavSampleRate * 2
const maxWhisperResponseBytes int64 = 128 << 20

// The Whisper server is normally kept asleep. It sits behind a Traefik proxy
// with a Sablier middleware, which starts the container on the first request
// and suspends it again when idle. Until the container is up, Sablier answers
// every request itself with an HTML "waking up" page and HTTP status 200, so a
// transcription request that arrives at a sleeping server gets back a web page
// where the transcript should be.
//
// That is not a failure and must not be reported as one. The request is simply
// sent again a few seconds later, as many times as the wake-up takes, and it
// succeeds as soon as Whisper is running. These retries are counted apart from
// the ordinary attempts, so a slow wake-up cannot use up the attempts meant for
// a server that is really broken.
const (
	// maxWakeRetries with wakeRetryDelay bounds the wait for a wake-up to about
	// a minute and a half, long enough for a cold start that loads a model.
	maxWakeRetries = 30
)

// retryUnit is the second that the ordinary retry delay is counted in, a variable
// so tests need not wait for real seconds. wakeRetryDelay is likewise.
var (
	wakeRetryDelay = 3 * time.Second
	retryUnit      = time.Second
)

// errWhisperWaking marks a reply that came from the proxy in front of Whisper
// while the container starts, not from Whisper itself.
var errWhisperWaking = errors.New("whisper server is waking up")

// looksLikeWakingPage reports whether a reply that should have been a JSON
// transcript is a web page instead.
func looksLikeWakingPage(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '<'
}

// pageSnippet is the start of a web page reduced to one short line of text, for
// an error message that says what the server actually sent.
func pageSnippet(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if len(text) > 120 {
		text = text[:120] + "..."
	}
	return text
}

func TranscribeWhisper(audioPath, whisperURL string, quiet, verbose bool, totalDuration, speedFactor float64, dockerContainer string, prompt, language string, pcmData []byte) (*types.TranscriptionData, error) {
	return TranscribeWhisperContext(context.Background(), audioPath, whisperURL, quiet, verbose, totalDuration, speedFactor, dockerContainer, prompt, language, pcmData)
}

func TranscribeWhisperContext(ctx context.Context, audioPath, whisperURL string, quiet, verbose bool, totalDuration, speedFactor float64, dockerContainer string, prompt, language string, pcmData []byte) (*types.TranscriptionData, error) {
	return TranscribeWhisperRequest(ctx, WhisperRequest{
		AudioPath: audioPath, URL: whisperURL, Quiet: quiet, Verbose: verbose,
		TotalDuration: totalDuration, SpeedFactor: speedFactor, DockerContainer: dockerContainer,
		Prompt: prompt, Language: language, PCM: pcmData,
	})
}

// WhisperRequest is one transcription sent to a Whisper server. Fields are extra
// form fields for servers that take more than whisper.cpp does: WhisperX takes
// "diarize" and "model", which whisper.cpp ignores.
type WhisperRequest struct {
	AudioPath       string
	URL             string
	Quiet, Verbose  bool
	TotalDuration   float64
	SpeedFactor     float64
	DockerContainer string
	Prompt          string
	Language        string
	PCM             []byte
	Fields          map[string]string
}

func TranscribeWhisperRequest(ctx context.Context, req WhisperRequest) (*types.TranscriptionData, error) {
	audioPath, whisperURL, quiet, verbose := req.AudioPath, req.URL, req.Quiet, req.Verbose
	totalDuration := req.TotalDuration
	maxRetries := 5
	retryDelay := 5
	readTimeout := int(totalDuration*1.5) + 600
	if readTimeout < 1800 {
		readTimeout = 1800
	}

	client := &http.Client{
		Timeout: time.Duration(readTimeout) * time.Second,
	}

	wakeRetries := 0
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		bodyReader, contentType, err := buildWhisperBody(audioPath, req.Prompt, req.Language, req.PCM, req.Fields)
		if err != nil {
			return nil, err
		}

		data, err := ExecuteWhisperAttemptContext(ctx, client, whisperURL, contentType, bodyReader, quiet, verbose)
		if err == nil {
			return data, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if errors.Is(err, errWhisperWaking) && wakeRetries < maxWakeRetries {
			wakeRetries++
			attempt--
			if !quiet {
				fmt.Printf("\nWhisper is waking up; trying again in %d seconds (%d/%d)...\n", int(wakeRetryDelay/time.Second), wakeRetries, maxWakeRetries)
			}
			if err := pause(ctx, wakeRetryDelay); err != nil {
				return nil, err
			}
			continue
		}

		if attempt < maxRetries {
			if !quiet {
				util.Errorf("Whisper server error (attempt %d/%d): %v", attempt, maxRetries, err)
				fmt.Printf("Retrying in %d seconds...\n\n", retryDelay)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(retryDelay) * retryUnit):
			}
		} else {
			return nil, fmt.Errorf("failed to connect to Whisper GPU server at '%s' after %d attempts: %w", whisperURL, maxRetries, err)
		}
	}

	return nil, fmt.Errorf("whisper transcription failed after %d attempts", maxRetries)
}

// pause waits for d, or returns early with the context's error if it is done.
func pause(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func BuildWhisperMultipartBody(audioPath, prompt, language string, pcmData []byte) (io.ReadCloser, string, error) {
	return buildWhisperBody(audioPath, prompt, language, pcmData, nil)
}

func buildWhisperBody(audioPath, prompt, language string, pcmData []byte, fields map[string]string) (io.ReadCloser, string, error) {
	var audioSource io.Reader
	var closeSrc func() error

	if pcmData != nil {
		header := BuildWavHeader(len(pcmData))
		audioSource = io.MultiReader(bytes.NewReader(header), bytes.NewReader(pcmData))
	} else {
		f, err := os.Open(audioPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to open audio file: %w", err)
		}
		audioSource = f
		closeSrc = f.Close
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		if closeSrc != nil {
			defer closeSrc()
		}
		err := writeWhisperFields(mw, audioSource, filepath.Base(audioPath), prompt, language, fields)
		if closeErr := mw.Close(); err == nil {
			err = closeErr
		}
		_ = pw.CloseWithError(err)
	}()

	return pr, mw.FormDataContentType(), nil
}

func WriteWhisperMultipartFields(mw *multipart.Writer, audioSource io.Reader, filename, prompt, language string) error {
	return writeWhisperFields(mw, audioSource, filename, prompt, language, nil)
}

func writeWhisperFields(mw *multipart.Writer, audioSource io.Reader, filename, prompt, language string, fields map[string]string) error {
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, audioSource); err != nil {
		return err
	}
	if err := mw.WriteField("response_format", "verbose_json"); err != nil {
		return err
	}
	if err := mw.WriteField("temperature", "0.0"); err != nil {
		return err
	}
	if language != "" && language != "auto" {
		if err := mw.WriteField("language", language); err != nil {
			return err
		}
	}
	if prompt != "" {
		if err := mw.WriteField("prompt", prompt); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := mw.WriteField(name, fields[name]); err != nil {
			return err
		}
	}
	return nil
}

func ReadLimitedBody(r io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes limit", maxBytes)
	}
	return body, nil
}

func ExecuteWhisperAttemptContext(ctx context.Context, client *http.Client, uri, contentType string, bodyReader io.ReadCloser, quiet, verbose bool) (*types.TranscriptionData, error) {
	defer bodyReader.Close()

	startTime := time.Now()
	req, err := http.NewRequestWithContext(ctx, "POST", uri, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	stopTicker := startProgressTicker(quiet, startTime)
	defer stopTicker()

	resp, err := client.Do(req)
	stopTicker()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !quiet && verbose {
		elapsed := time.Since(startTime)
		fmt.Printf("\rTranscription finished in %s!                                  \n", format.FormatClock(elapsed.Seconds()))
	}

	body, err := ReadLimitedBody(resp.Body, maxWhisperResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	if looksLikeWakingPage(resp.Header.Get("Content-Type"), body) {
		return nil, fmt.Errorf("%w (the server sent a web page: %s)", errWhisperWaking, pageSnippet(body))
	}

	var data types.TranscriptionData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse transcription JSON: %w", err)
	}
	return &data, nil
}

// progressInterval is how often the elapsed time is redrawn; a variable for tests.
var progressInterval = 2 * time.Second

// startProgressTicker shows the elapsed time on a single line that redraws in
// place. The function it returns stops the ticker and ends that line, so that
// whatever is printed next starts on a line of its own; without that, the next
// message ran on after the last "Elapsed" figure and the output was unreadable.
// It may be called more than once.
func startProgressTicker(quiet bool, start time.Time) func() {
	if quiet {
		return func() {}
	}
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		StartTranscriptionProgressTicker(start, done)
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}
}

// StartTranscriptionProgressTicker redraws the elapsed time every two seconds
// until done is closed, then ends the line if it drew one.
func StartTranscriptionProgressTicker(startTime time.Time, done chan struct{}) {
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()
	drew := false
	for {
		select {
		case <-done:
			if drew {
				fmt.Println()
			}
			return
		case <-ticker.C:
			drew = true
			elapsed := time.Since(startTime)
			fmt.Printf("\rTranscribing audio... Elapsed: %s   ", format.FormatClock(elapsed.Seconds()))
		}
	}
}

func SortSegments(segs []types.TranscriptionSegment) {
	sort.Slice(segs, func(i, j int) bool {
		return segs[i].Start < segs[j].Start
	})
}

func JoinSegmentText(segs []types.TranscriptionSegment) string {
	var b strings.Builder
	total := 0
	for _, seg := range segs {
		total += len(seg.Text) + 1
	}
	b.Grow(total)
	for i, seg := range segs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(seg.Text)
	}
	return strings.TrimSpace(b.String())
}

func MergeSegments(segs []types.TranscriptionSegment) []types.TranscriptionSegment {
	if len(segs) == 0 {
		return segs
	}

	merged := make([]types.TranscriptionSegment, 0, len(segs))
	current := segs[0]
	currentParts := []string{segs[0].Text}

	for i := 1; i < len(segs); i++ {
		seg := segs[i]
		if seg.Start <= current.End+0.5 {
			if seg.End > current.End {
				current.End = seg.End
				currentParts = append(currentParts, seg.Text)
			}
		} else {
			current.Text = strings.Join(currentParts, " ")
			merged = append(merged, current)
			current = seg
			currentParts = []string{seg.Text}
		}
	}
	current.Text = strings.Join(currentParts, " ")
	merged = append(merged, current)
	return merged
}

func TrimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
