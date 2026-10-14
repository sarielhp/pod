package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pod/pkg/format"
	"pod/pkg/music"
	"pod/pkg/progress"
	"pod/pkg/transcribe"
	"pod/pkg/types"
	"pod/pkg/util"
)

const musicReason = "music-only (YAMNet + Whisper gap review)"

// MusicReviewRequest resolves the original audio and settings for gap recovery.
type MusicReviewRequest struct {
	AudioPath string
	CachePath string
	Duration  float64
	Config    types.Config
	Options   types.ProcOptions
	Reporter  progress.Reporter
}

type classification struct {
	result *music.Result
	err    error
}

// MusicReview runs full-audio classification concurrently with initial transcription.
type MusicReview struct {
	req    MusicReviewRequest
	result chan classification
	retry  func(context.Context, types.KeepSegment) (*types.TranscriptionData, error)
}

// StartMusicReview starts the classifier immediately; callers cancel ctx on exit.
func StartMusicReview(ctx context.Context, req MusicReviewRequest) *MusicReview {
	r := &MusicReview{req: req, result: make(chan classification, 1)}
	r.retry = r.transcribeGap
	go func() {
		result, err := music.Classify(ctx, req.Config.YamnetURL, req.AudioPath, req.CachePath)
		r.result <- classification{result, err}
	}()
	return r
}

// Review recovers missing speech and records conservatively confirmed music cuts.
func (r *MusicReview) Review(ctx context.Context, td *types.TranscriptionData) error {
	if td == nil {
		return fmt.Errorf("gap review requires a transcript")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var classified classification
	select {
	case classified = <-r.result:
	case <-ctx.Done():
		return ctx.Err()
	}
	if classified.err != nil {
		return classified.err
	}
	// ffprobe estimates MP3 container duration from file size and nominal bitrate
	// when Xing/VBRI headers are absent or unpadded, routinely drifting by 1-2%
	// from the true decoded PCM audio duration. Allow up to 1% (or 15s) drift,
	// and adopt YAMNet's exact decoded audio duration for gap review.
	if math.Abs(classified.result.Duration-r.req.Duration) > math.Max(15, 0.01*r.req.Duration) {
		return fmt.Errorf("classifier duration does not match original audio")
	}
	r.req.Duration = classified.result.Duration
	key, err := r.sourceKey()
	if err != nil {
		return err
	}
	if saved := td.GapReview; saved != nil && saved.Version == 1 && saved.Complete && saved.SourceKey == key && saved.TranscriptKey == transcriptKey(td) {
		td.Cuts = replaceMusicCuts(td.Cuts, saved.MusicCuts)
		return nil
	}
	progress.Or(r.req.Reporter).Infof("Reviewing transcript gaps with YAMNet and short Whisper retries...")
	review := &types.GapReview{Version: 1, SourceKey: key, Duration: r.req.Duration}
	td.GapReview = review
	td.Cuts = replaceMusicCuts(td.Cuts, nil)
	gaps := transcriptGaps(td, r.req.Duration, 0)
	var retried []types.KeepSegment
	seconds, attempts := 0.0, 0
	for _, gap := range gaps {
		if gap.End-gap.Start < 2 || silentGap(classified.result, gap) {
			continue
		}
		for start := gap.Start; start < gap.End; start += 30 {
			part := types.KeepSegment{Start: start, End: math.Min(start+30, gap.End)}
			if seconds+part.End-part.Start > 900 || attempts >= 96 {
				progress.Or(r.req.Reporter).Infof("Transcript gap retry limit reached; preserving remaining gaps.")
				return r.finishReview(td, classified.result, retried, nil)
			}
			if err := ctx.Err(); err != nil {
				return r.finishReview(td, classified.result, retried, err)
			}
			seconds += part.End - part.Start
			attempts++
			recovered, retryErr := r.retry(ctx, part)
			if err := ctx.Err(); err != nil {
				return r.finishReview(td, classified.result, retried, err)
			}
			if retryErr == nil {
				retryErr = validateRecovered(recovered)
			}
			if retryErr != nil {
				progress.Or(r.req.Reporter).Infof("Could not recover transcript gap %.2f–%.2f; preserving its audio: %v", part.Start, part.End, retryErr)
				continue
			}
			review.RecoveredSegments += mergeRecovered(td, recovered, part)
			retried = append(retried, part)
		}
	}
	return r.finishReview(td, classified.result, retried, nil)
}

func (r *MusicReview) sourceKey() (string, error) {
	fi, err := os.Stat(r.req.AudioPath)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%s:%d:%d:%.3f", r.req.Config.YamnetURL, fi.Size(), fi.ModTime().UnixNano(), r.req.Duration)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key))), nil
}

func transcriptKey(td *types.TranscriptionData) string {
	data, _ := json.Marshal(td.Segments)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (r *MusicReview) finishReview(td *types.TranscriptionData, result *music.Result, retried []types.KeepSegment, failure error) error {
	review := td.GapReview
	for _, gap := range transcriptGaps(td, r.req.Duration, 0) {
		if gap.End-gap.Start < 2 || silentGap(result, gap) {
			continue
		}
		if !coveredBy(gap, retried) || hasSpeech(result, gap) {
			review.Unresolved = append(review.Unresolved, gap)
			continue
		}
		padded := gap
		if gap.Start > 0 {
			padded.Start += 0.75
		}
		if gap.End < r.req.Duration {
			padded.End -= 0.75
		}
		cuts := confirmedMusic(result, padded)
		if len(cuts) == 0 {
			review.Unresolved = append(review.Unresolved, gap)
			continue
		}
		review.MusicCuts = append(review.MusicCuts, cuts...)
	}
	review.Complete = failure == nil && len(review.Unresolved) == 0
	review.TranscriptKey = transcriptKey(td)
	td.Cuts = format.PreserveGaps(replaceMusicCuts(td.Cuts, review.MusicCuts), review.Unresolved)
	progress.Or(r.req.Reporter).Infof("Gap review: recovered %d transcript segments; confirmed %d music-only cuts; %d unresolved gaps preserved.", review.RecoveredSegments, len(review.MusicCuts), len(review.Unresolved))
	return failure
}

func replaceMusicCuts(cuts, musicCuts []types.AdSegment) []types.AdSegment {
	var kept []types.AdSegment
	for _, cut := range cuts {
		if cut.Reason != musicReason {
			kept = append(kept, cut)
		}
	}
	return append(kept, musicCuts...)
}

func (r *MusicReview) transcribeGap(ctx context.Context, gap types.KeepSegment) (*types.TranscriptionData, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	work := util.WorkDirFor(r.req.AudioPath)
	if err := os.MkdirAll(work, 0755); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(work, "gap-*.wav")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(path)
		return nil, err
	}
	defer func() { os.Remove(path); os.Remove(work); os.Remove(filepath.Dir(work)) }()
	if err := util.VerifyTempFile(path); err != nil {
		return nil, err
	}
	start := math.Max(0, gap.Start-0.75)
	end := math.Min(r.req.Duration, gap.End+0.75)
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-y", "-v", "error", "-ss", fmt.Sprintf("%.3f", start), "-i", r.req.AudioPath, "-t", fmt.Sprintf("%.3f", end-start), "-ar", "16000", "-ac", "1", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("extract transcript gap: %w: %s", err, strings.TrimSpace(string(output)))
	}
	cfg := r.req.Config
	opts := r.req.Options
	lang := cfg.WhisperLanguage
	wp := resolveWhisperRoutingProfile(&cfg, r.req.AudioPath, opts, &lang)
	options := transcribe.Options{Language: lang, Prompt: cfg.WhisperPrompt, Quiet: true, Verbose: opts.Verbose, SpeedFactor: cfg.WhisperSpeedFactor}
	var engine transcribe.Transcriber
	if wp.Engine == types.WhisperEngineLocal {
		engine = transcribe.NewWhisperCLITranscriber(wp)
	} else {
		wp.URL = cfg.WhisperURL
		engine = transcribe.NewWhisperHTTPTranscriber(wp)
	}
	td, err := engine.Transcribe(ctx, path, options)
	if err != nil {
		return nil, err
	}
	if td == nil {
		return nil, fmt.Errorf("gap retry returned no transcript")
	}
	for i := range td.Segments {
		td.Segments[i].Start += start
		td.Segments[i].End += start
		for j := range td.Segments[i].Words {
			td.Segments[i].Words[j].Start += start
			td.Segments[i].Words[j].End += start
		}
	}
	return td, nil
}

func mergeRecovered(td, recovered *types.TranscriptionData, gap types.KeepSegment) int {
	if recovered == nil {
		return 0
	}
	added := 0
	for _, seg := range recovered.Segments {
		if strings.TrimSpace(seg.Text) == "" || seg.End <= gap.Start || seg.Start >= gap.End {
			continue
		}
		words := make([]types.TranscriptionWord, 0, len(seg.Words))
		for _, word := range seg.Words {
			if word.End > gap.Start && word.Start < gap.End && strings.TrimSpace(word.Word) != "" {
				words = append(words, word)
			}
		}
		if len(seg.Words) > 0 && len(words) == 0 {
			continue
		}
		if len(words) > 0 {
			seg.Words = words
			seg.Text = ""
			for _, word := range words {
				seg.Text += word.Word + " "
			}
		}
		seg.Start = math.Max(seg.Start, gap.Start)
		seg.End = math.Min(seg.End, gap.End)
		td.Segments = append(td.Segments, seg)
		added++
	}
	sort.SliceStable(td.Segments, func(i, j int) bool { return td.Segments[i].Start < td.Segments[j].Start })
	var text strings.Builder
	for _, seg := range td.Segments {
		text.WriteString(strings.TrimSpace(seg.Text))
		text.WriteByte(' ')
	}
	td.Text = strings.TrimSpace(text.String())
	return added
}

func transcriptGaps(td *types.TranscriptionData, duration, padding float64) []types.KeepSegment {
	var speech []types.KeepSegment
	for _, seg := range td.Segments {
		if strings.TrimSpace(seg.Text) == "" {
			continue
		}
		if !usableWordTimes(seg) {
			speech = append(speech, types.KeepSegment{Start: seg.Start, End: seg.End})
			continue
		}
		for _, word := range seg.Words {
			if word.End > word.Start && strings.TrimSpace(word.Word) != "" {
				speech = append(speech, types.KeepSegment{Start: word.Start, End: word.End})
			}
		}
	}
	sort.Slice(speech, func(i, j int) bool { return speech[i].Start < speech[j].Start })
	var gaps []types.KeepSegment
	end := 0.0
	for _, span := range speech {
		start := math.Max(0, span.Start-padding)
		if start > end {
			gaps = append(gaps, types.KeepSegment{Start: end, End: math.Min(start, duration)})
		}
		end = math.Max(end, math.Min(duration, span.End+padding))
	}
	if end < duration {
		gaps = append(gaps, types.KeepSegment{Start: end, End: duration})
	}
	return gaps
}

func usableWordTimes(seg types.TranscriptionSegment) bool {
	if len(seg.Words) == 0 {
		return false
	}
	hasWord := false
	for _, word := range seg.Words {
		if strings.TrimSpace(word.Word) == "" {
			continue
		}
		if word.End <= word.Start || word.Start < seg.Start-0.5 || word.End > seg.End+0.5 {
			return false
		}
		hasWord = true
	}
	return hasWord
}

func validateRecovered(td *types.TranscriptionData) error {
	if td == nil {
		return fmt.Errorf("gap retry returned no transcript")
	}
	if strings.TrimSpace(td.Text) != "" && len(td.Segments) == 0 {
		return fmt.Errorf("gap retry returned speech without timestamps")
	}
	for _, seg := range td.Segments {
		if strings.TrimSpace(seg.Text) != "" && seg.End <= seg.Start {
			return fmt.Errorf("gap retry returned invalid speech timestamps")
		}
	}
	return nil
}

func coveredBy(gap types.KeepSegment, spans []types.KeepSegment) bool {
	end := gap.Start
	for _, span := range spans {
		if span.Start > end+0.01 {
			continue
		}
		if span.End > end {
			end = span.End
		}
	}
	return end >= gap.End-0.01
}

func silentGap(result *music.Result, gap types.KeepSegment) bool {
	count := 0
	for _, frame := range result.Frames {
		if frame.Time >= gap.End || frame.Time+0.96 <= gap.Start {
			continue
		}
		count++
		if frame.Silence < 0.8 || frame.Speech > 0.05 || frame.Music > 0.1 {
			return false
		}
	}
	return count > 0
}

func hasSpeech(result *music.Result, gap types.KeepSegment) bool {
	for _, frame := range result.Frames {
		if frame.Time >= gap.Start && frame.Time+0.96 <= gap.End && frame.Speech >= 0.1 {
			return true
		}
	}
	return false
}

func confirmedMusic(result *music.Result, gap types.KeepSegment) []types.AdSegment {
	var cuts []types.AdSegment
	start, end := -1.0, 0.0
	for i, frame := range result.Frames {
		cellStart := frame.Time
		cellEnd := math.Min(frame.Time+0.48, result.Duration)
		strong := frame.Music >= 0.8 && frame.Speech <= 0.05
		if i > 0 {
			strong = strong && result.Frames[i-1].Music >= 0.8 && result.Frames[i-1].Speech <= 0.05
		}
		if cellStart < gap.Start || cellEnd > gap.End {
			strong = false
		}
		if strong {
			if start < 0 {
				start = cellStart
			}
			end = cellEnd
			continue
		}
		if start >= 0 && end-start >= 2 {
			cuts = append(cuts, types.AdSegment{Start: start, End: end, Reason: musicReason})
		}
		start = -1
	}
	if start >= 0 && end-start >= 2 {
		cuts = append(cuts, types.AdSegment{Start: start, End: end, Reason: musicReason})
	}
	return cuts
}

// MusicCachePath places classification metadata beside the episode's transcript.
func MusicCachePath(mainFile string) string {
	return util.StripExt(filepath.Clean(mainFile)) + ".yamnet.json"
}
