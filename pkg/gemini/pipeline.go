package gemini

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/audio"
	"pod/pkg/config"
	"pod/pkg/format"
	"pod/pkg/transcribe"
	"pod/pkg/types"
	"pod/pkg/util"
)

// DefaultGeminiChunkSec is re-exported so callers in this package's
// vocabulary need not reach into pkg/types for it.
const DefaultGeminiChunkSec = types.DefaultGeminiChunkSec

func ConvertGeminiToAbsTypes(payload *types.GeminiResponsePayload) (*types.TranscriptionData, []types.AdSegment) {
	td := &types.TranscriptionData{}
	if payload == nil {
		return td, nil
	}
	for _, s := range payload.Segments {
		td.Segments = append(td.Segments, types.TranscriptionSegment{
			Start: s.Start,
			End:   s.End,
			Text:  s.Text,
		})
		td.Text += s.Text + " "
	}
	td.Text = strings.TrimSpace(td.Text)

	var ads []types.AdSegment
	for _, c := range payload.Cuts {
		reason := c.Reason
		if c.Type != "" {
			if reason != "" {
				reason = fmt.Sprintf("[%s] %s", c.Type, c.Reason)
			} else {
				reason = fmt.Sprintf("[%s]", c.Type)
			}
		}
		ads = append(ads, types.AdSegment{
			Start:  c.Start,
			End:    c.End,
			Reason: reason,
		})
	}

	return td, ads
}

func ComputeGeminiChunks(totalDuration, chunkDurSec float64) []types.GeminiChunkInfo {
	if chunkDurSec <= 0 {
		chunkDurSec = DefaultGeminiChunkSec
	}
	if totalDuration <= chunkDurSec {
		return []types.GeminiChunkInfo{{Index: 0, StartSec: 0, DurSec: totalDuration}}
	}
	numChunks := int(math.Ceil(totalDuration / chunkDurSec))
	chunks := make([]types.GeminiChunkInfo, 0, numChunks)
	for i := 0; i < numChunks; i++ {
		st := float64(i) * chunkDurSec
		dur := chunkDurSec
		if st+dur > totalDuration {
			dur = totalDuration - st
		}
		chunks = append(chunks, types.GeminiChunkInfo{
			Index:    i,
			StartSec: st,
			DurSec:   dur,
		})
	}
	return chunks
}

func SplitAudioChunk(inputPath, outputPath string, startSec, durSec float64) error {
	if err := util.VerifyTempFile(outputPath); err != nil {
		return err
	}
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.3f", startSec),
		"-t", fmt.Sprintf("%.3f", durSec),
		"-i", inputPath,
		"-c", "copy", outputPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		fbCmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
			"-ss", fmt.Sprintf("%.3f", startSec),
			"-t", fmt.Sprintf("%.3f", durSec),
			"-i", inputPath,
			"-c:a", "libmp3lame", "-b:a", "128k", outputPath)
		if fbOut, fbErr := fbCmd.CombinedOutput(); fbErr != nil {
			return fmt.Errorf("split audio chunk failed: %w (copy: %s, fallback: %s)", fbErr, string(out), string(fbOut))
		}
	}
	fi, err := os.Stat(outputPath)
	if err != nil || fi.Size() == 0 {
		return fmt.Errorf("split audio chunk produced empty or missing file: %s", outputPath)
	}
	return nil
}

func PrepareGeminiChunks(audioPath string, chunks []types.GeminiChunkInfo) ([]types.GeminiChunkInfo, func(), error) {
	if len(chunks) <= 1 {
		if len(chunks) == 1 {
			chunks[0].FilePath = audioPath
		}
		return chunks, func() {}, nil
	}

	workDir := util.WorkDirFor(audioPath)
	_ = os.MkdirAll(workDir, 0755)
	ext := filepath.Ext(audioPath)
	if ext == "" {
		ext = ".mp3"
	}

	prepared := make([]types.GeminiChunkInfo, len(chunks))
	cleanup := func() {
		for _, ch := range prepared {
			if ch.FilePath != "" && ch.FilePath != audioPath {
				_ = os.Remove(ch.FilePath)
			}
		}
	}

	for i, ch := range chunks {
		chunkFile := filepath.Join(workDir, fmt.Sprintf("%s.gemini_chunk_%d_%d%s", filepath.Base(audioPath), i, time.Now().UnixNano(), ext))
		if err := SplitAudioChunk(audioPath, chunkFile, ch.StartSec, ch.DurSec); err != nil {
			cleanup()
			return nil, nil, err
		}
		ch.FilePath = chunkFile
		prepared[i] = ch
	}

	return prepared, cleanup, nil
}

func ProcessSingleGeminiChunk(ctx context.Context, ch types.GeminiChunkInfo, cfg types.Config) (*types.GeminiChunkResult, error) {
	return processSingleGeminiChunk(ctx, ch, cfg, GeminiModelChain(&cfg))
}

func processSingleGeminiChunk(ctx context.Context, ch types.GeminiChunkInfo, cfg types.Config, models []string) (*types.GeminiChunkResult, error) {
	apiKey := config.ResolveGeminiAPIKey(&cfg)
	if apiKey != "" {
		fileURI, fileName, err := UploadAudioToGeminiStudio(ctx, apiKey, ch.FilePath)
		if err != nil {
			return nil, fmt.Errorf("chunk %d studio upload failed:\n   %w", ch.Index, err)
		}
		defer DeleteGeminiStudioFile(ctx, apiKey, fileName)

		payload, err := callStudioAcrossModels(ctx, apiKey, fileURI, AudioMIMEType(ch.FilePath), models)
		if err != nil {
			return nil, fmt.Errorf("chunk %d studio processing failed:\n   %w", ch.Index, err)
		}
		return &types.GeminiChunkResult{
			Index:    ch.Index,
			StartSec: ch.StartSec,
			Payload:  payload,
		}, nil
	}

	bucketName := cfg.GetGeminiStagingBucket()
	projectID := cfg.GetGeminiProjectID()
	location := cfg.GetGeminiLocation()

	gcsURI, err := UploadAudioToGCS(ctx, bucketName, ch.FilePath)
	if err != nil {
		return nil, fmt.Errorf("chunk %d upload failed:\n   %w", ch.Index, err)
	}
	defer DeleteGCSObject(ctx, bucketName, gcsURI)

	payload, err := CallGeminiAudioProcessor(ctx, projectID, location, gcsURI)
	if err != nil {
		return nil, fmt.Errorf("chunk %d processing failed:\n   %w", ch.Index, err)
	}

	return &types.GeminiChunkResult{
		Index:    ch.Index,
		StartSec: ch.StartSec,
		Payload:  payload,
	}, nil
}

func ProcessGeminiChunksParallel(ctx context.Context, chunks []types.GeminiChunkInfo, cfg types.Config) ([]*types.GeminiChunkResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// One chain for the whole file. The port, shared by every chunk, is what
	// makes one chunk's discovery that a model is spent apply to the rest
	// rather than each rediscovering it and burning the quota doing so.
	models := GeminiModelChain(&cfg)

	results := make([]*types.GeminiChunkResult, len(chunks))
	var wg util.WaitGroup
	var mu util.Mutex
	var firstErr error

	const maxConcurrentChunks = 2
	sem := make(chan struct{}, maxConcurrentChunks)

	for i, ch := range chunks {
		wg.Add(1)
		go func(idx int, chunk types.GeminiChunkInfo) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				mu.Lock()
				if firstErr == nil {
					firstErr = ctx.Err()
				}
				mu.Unlock()
				return
			case sem <- struct{}{}:
				defer func() { <-sem }()
			}

			if ctx.Err() != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = ctx.Err()
				}
				mu.Unlock()
				return
			}
			res, err := processSingleGeminiChunk(ctx, chunk, cfg, models)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
				cancel()
			}
			results[idx] = res
		}(i, ch)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return results, nil
}

func MergeGeminiChunkResults(results []*types.GeminiChunkResult) *types.GeminiResponsePayload {
	merged := &types.GeminiResponsePayload{}
	for _, res := range results {
		if res == nil || res.Payload == nil {
			continue
		}
		offset := res.StartSec
		for _, c := range res.Payload.Cuts {
			merged.Cuts = append(merged.Cuts, types.GeminiCutItem{
				Start:  c.Start + offset,
				End:    c.End + offset,
				Type:   c.Type,
				Reason: c.Reason,
			})
		}
		for _, s := range res.Payload.Segments {
			merged.Segments = append(merged.Segments, types.GeminiSegmentItem{
				Start: s.Start + offset,
				End:   s.End + offset,
				Text:  s.Text,
			})
		}
	}
	return merged
}

func ProcessWithGeminiFlash(ctx context.Context, audioPath, projectID, bucketName string) (*types.TranscriptionData, []types.AdSegment, error) {
	var cfg types.Config
	cfg.GeminiProjectID = projectID
	cfg.GeminiStagingBucket = bucketName
	return ProcessWithGeminiConfig(ctx, audioPath, cfg, DefaultGeminiChunkSec)
}

func ProcessWithGeminiFlashChunks(ctx context.Context, audioPath, projectID, bucketName string, chunkDurSec float64) (*types.TranscriptionData, []types.AdSegment, error) {
	var cfg types.Config
	cfg.GeminiProjectID = projectID
	cfg.GeminiStagingBucket = bucketName
	return ProcessWithGeminiConfig(ctx, audioPath, cfg, chunkDurSec)
}

func ProcessWithGeminiConfig(ctx context.Context, audioPath string, cfg types.Config, chunkDurSec float64) (*types.TranscriptionData, []types.AdSegment, error) {
	if isOpen, until, reason := IsCircuitBreakerOpen(); isOpen {
		return nil, nil, fmt.Errorf("gemini in cooldown until %s: %s", until.Format("15:04:05"), reason)
	}

	backendLabel, model := "Vertex AI", "gemini-1.5-flash"
	if config.ResolveGeminiAPIKey(&cfg) != "" {
		backendLabel, model = "Google AI Studio", cfg.GetGeminiModel()
	}
	transcribe.AnnounceUsing(types.WhisperEngineGemini, fmt.Sprintf("(%s, model: %s)", backendLabel, model), false)
	fmt.Println(util.BoldCyan(fmt.Sprintf("Ad detection: Gemini via %s (model: %s; combined with transcription)", backendLabel, model)))
	totDur := audio.GetAudioDuration(audioPath)
	if totDur <= 0 {
		totDur = DefaultGeminiChunkSec
	}
	chunks := ComputeGeminiChunks(totDur, chunkDurSec)
	prepared, cleanup, err := PrepareGeminiChunks(audioPath, chunks)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()

	if len(prepared) > 1 {
		fmt.Printf("Splitting '%s' (%s) into %d parallel chunks of %s for Gemini [%s]...\n",
			filepath.Base(audioPath), format.FormatTime(totDur), len(prepared),
			format.FormatMinutes(chunkDurSec), backendLabel)
	} else {
		fmt.Printf("Processing '%s' with Gemini [%s]...\n", filepath.Base(audioPath), backendLabel)
	}

	t0 := time.Now()
	results, err := ProcessGeminiChunksParallel(ctx, prepared, cfg)
	if err != nil {
		return nil, nil, err
	}
	fmt.Printf("Gemini processing finished in %s across %d chunk(s)!\n",
		format.FormatClock(time.Since(t0).Seconds()), len(prepared))

	merged := MergeGeminiChunkResults(results)
	td, ads := ConvertGeminiToAbsTypes(merged)
	// Record the model the API says answered, rather than the one that was
	// asked for. With a chain the two routinely differ, and an alias never
	// named a real model in the first place.
	if v := resolvedModelVersion(results); v != "" && td != nil {
		td.Model = v
	}
	return td, ads, nil
}

// resolvedModelVersion is the model that produced these chunks. The chunks of
// one file share a selector, so they agree except across a mid-run switch, in
// which case the last one is the model that finished the work.
func resolvedModelVersion(results []*types.GeminiChunkResult) string {
	version := ""
	for _, r := range results {
		if r != nil && r.Payload != nil && r.Payload.ModelVersion != "" {
			version = r.Payload.ModelVersion
		}
	}
	return version
}
