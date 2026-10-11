package adremoval

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pod/pkg/audio"
	"pod/pkg/episode"
	"pod/pkg/format"
	"pod/pkg/music"
	"pod/pkg/pipeline"
	"pod/pkg/types"
	"pod/pkg/util"
)

func TestMusicReviewCachedTranscriptRecutAndParallelWake(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "episode.mp3")
	silentWav(t, path, 30)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	whisperWoke := make(chan struct{})
	var whisperOnce atomic.Bool
	whisper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if whisperOnce.CompareAndSwap(false, true) {
				close(whisperWoke)
			}
			return
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		io.WriteString(w, `{"text":"if you like YouTube you would love Premium","segments":[{"start":0.75,"end":5.75,"text":"if you like YouTube you would love Premium","words":[{"start":0.75,"end":5.75,"word":"YouTube Premium advertisement"}]}]}`)
	}))
	defer whisper.Close()
	classifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			select {
			case <-whisperWoke:
			case <-time.After(time.Second):
				t.Error("Whisper and YAMNet were not woken concurrently")
			}
			io.WriteString(w, `{"status":"healthy"}`)
			return
		}
		io.Copy(io.Discard, r.Body)
		result := music.Result{Duration: 30}
		for sec := 0.0; sec < 30; sec += 0.48 {
			frame := music.Frame{Time: sec, Music: 0.99}
			if sec >= 2 && sec < 6 {
				frame.Speech = 0.9
			}
			result.Frames = append(result.Frames, frame)
		}
		if err := json.NewEncoder(w).Encode(result); err != nil {
			t.Error(err)
		}
	}))
	defer classifier.Close()
	profile := types.WhisperProfile{ID: 1, Engine: types.WhisperEngineRemote, URL: whisper.URL, Languages: []string{"*"}}
	cfg := types.Config{WhisperConfig: types.WhisperConfig{YamnetURL: classifier.URL, WhisperURL: whisper.URL, ActiveWhisperID: 1, WhisperProfiles: []types.WhisperProfile{profile}}}
	td := &types.TranscriptionData{Text: strings.Repeat("episode ", 60), Segments: []types.TranscriptionSegment{{Start: 0, End: 2, Text: strings.Repeat("intro ", 30)}, {Start: 26, End: 30, Text: strings.Repeat("episode ", 30)}}}
	transcript := util.StripExt(path) + ".transcript.json"
	if err := pipeline.SaveJSONTranscript(path, td, transcript, true, map[string]string{"artist": "preserve me"}); err != nil {
		t.Fatal(err)
	}
	if result := format.SaveDetectedCutsJSON(path, 30, []types.AdSegment{{Start: 0, End: 1, Reason: "existing ad"}}, nil, true, false); result.Err != nil {
		t.Fatal(result.Err)
	}
	job := episodeJob{sourceAudioFile: path, mainMP3File: path, jsonFile: transcript, totalDuration: 30, cfg: cfg, opts: types.ProcOptions{Quiet: true}}
	if err := reviewMusicForRecut(job); err != nil {
		t.Fatal(err)
	}
	var cuts types.CutsData
	data, err := os.ReadFile(util.StripExt(path) + ".cuts.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cuts); err != nil {
		t.Fatal(err)
	}
	if len(cuts.MergedCutIntervals) != 2 || cuts.MergedCutIntervals[0].End != 1 {
		t.Fatalf("existing cut lost: %+v", cuts)
	}
	var saved map[string]any
	data, err = os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["id3_artist"] != "preserve me" || !strings.Contains(saved["text"].(string), "YouTube Premium") {
		t.Fatal(saved)
	}
	if err := json.Unmarshal(data, td); err != nil {
		t.Fatal(err)
	}
	if td.Segments[1].Words[0].Start != 2 || td.Segments[1].Words[0].End != 7 {
		t.Fatalf("gap timestamps not offset: %+v", td.Segments[1])
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("review changed original audio")
	}
	if _, err := os.Stat(util.WorkDirFor(path)); !os.IsNotExist(err) {
		t.Fatalf("owned scratch directory left behind: %v", err)
	}
	job.precutFile, job.outputFile = path+".precut", path
	job.fileStartTime = time.Now()
	if err := recutReviewedEpisode(job); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(job.precutFile)
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatal("recut did not preserve the original")
	}
	if duration := audio.GetAudioDuration(path); math.Abs(duration-(30-cuts.TotalCutDurationSec)) > 1 {
		t.Fatalf("actual music cut duration = %.2f", duration)
	}
	if !episode.IsEpisodeClean(path) {
		t.Fatal("successful music recut was not marked clean")
	}
}

func TestMusicReviewFailurePreservesAudioAndRecordsStatus(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "episode.mp3")
	silentWav(t, path, 30)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			io.WriteString(w, `{"status":"healthy"}`)
			return
		}
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"duration":30,"frames":[]}`)
	}))
	defer srv.Close()
	transcript := util.StripExt(path) + ".transcript.json"
	if err := pipeline.SaveJSONTranscript(path, &types.TranscriptionData{}, transcript, true, nil); err != nil {
		t.Fatal(err)
	}
	cfg := types.Config{WhisperConfig: types.WhisperConfig{YamnetURL: srv.URL, WhisperEngine: types.WhisperEngineLocal}}
	_, _, _, failed := runLocalTranscriptionStep(path, transcript, path, 30, cfg, types.ProcOptions{Quiet: true}, types.LLMProfile{}, time.Now(), nil)
	if !failed {
		t.Fatal("unusable classifier allowed processing")
	}
	st, err := episode.LoadEpisodeStatus(episode.StatusPathFor(path))
	if err != nil || st.Status != types.StateFailed || st.CurrentStep != "gap-review" || st.LastError == "" {
		t.Fatalf("status=%+v err=%v", st, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("failed review changed original")
	}
}

func TestUnfilledSpeechGapAllowsProcessingAndRecutPreservesItsAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "episode.mp3")
	silentWav(t, path, 30)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	whisper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"text":"","segments":[]}`)
	}))
	defer whisper.Close()
	classifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			io.WriteString(w, `{"status":"healthy"}`)
			return
		}
		io.Copy(io.Discard, r.Body)
		result := music.Result{Duration: 30}
		for sec := 0.0; sec < 30; sec += 0.48 {
			result.Frames = append(result.Frames, music.Frame{Time: sec, Music: 0.99, Speech: 0.9})
		}
		if err := json.NewEncoder(w).Encode(result); err != nil {
			t.Error(err)
		}
	}))
	defer classifier.Close()
	profile := types.WhisperProfile{ID: 1, Engine: types.WhisperEngineRemote, URL: whisper.URL, Languages: []string{"*"}}
	cfg := types.Config{WhisperConfig: types.WhisperConfig{YamnetURL: classifier.URL, WhisperURL: whisper.URL, ActiveWhisperID: 1, WhisperProfiles: []types.WhisperProfile{profile}}}
	td := &types.TranscriptionData{Text: "intro end", Segments: []types.TranscriptionSegment{{Start: 0, End: 2, Text: "intro"}, {Start: 28, End: 30, Text: "end"}}}
	transcript := util.StripExt(path) + ".transcript.json"
	if err := pipeline.SaveJSONTranscript(path, td, transcript, true, nil); err != nil {
		t.Fatal(err)
	}
	if result := format.SaveCutsJSON(path, 30, []types.AdSegment{{Start: 0, End: 30, Reason: "old ad spanning gap"}}, nil, true); result.Err != nil {
		t.Fatal(result.Err)
	}
	opts := types.ProcOptions{Quiet: true}
	reviewed, _, ok, failed := runLocalTranscriptionStep(path, transcript, path, 30, cfg, opts, types.LLMProfile{}, time.Now(), nil)
	if failed || !ok || reviewed.GapReview.Complete || len(reviewed.GapReview.Unresolved) != 1 {
		t.Fatalf("unfilled gap stopped processing: ok=%v failed=%v transcript=%+v", ok, failed, reviewed)
	}
	job := episodeJob{sourceAudioFile: path, mainMP3File: path, jsonFile: transcript, totalDuration: 30, cfg: cfg, opts: opts, precutFile: path + ".precut", outputFile: path, fileStartTime: time.Now()}
	if err := recutReviewedEpisode(job); err != nil {
		t.Fatal(err)
	}
	var cuts types.CutsData
	data, err := os.ReadFile(util.StripExt(path) + ".cuts.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cuts); err != nil {
		t.Fatal(err)
	}
	if len(cuts.KeepIntervals) != 1 || cuts.KeepIntervals[0].Start != 2 || cuts.KeepIntervals[0].End != 28 {
		t.Fatalf("recut lost protected gap: %+v", cuts)
	}
	if duration := audio.GetAudioDuration(path); math.Abs(duration-26) > 0.5 {
		t.Fatalf("gap audio was removed: duration=%.2f", duration)
	}
	backup, err := os.ReadFile(job.precutFile)
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatal("recut did not preserve the original")
	}
	if !episode.IsEpisodeClean(path) {
		t.Fatal("preserved speech gap prevented successful completion")
	}
}

func TestTranscriptSanityAcceptsReviewedPreservedGaps(t *testing.T) {
	t.Parallel()
	td := &types.TranscriptionData{Text: strings.Repeat("spoken ", 30), Segments: []types.TranscriptionSegment{{Start: 0, End: 10, Text: strings.Repeat("spoken ", 30)}}}
	if validateTranscriptSanity(td, 600, true) {
		t.Fatal("unreviewed truncated transcript accepted")
	}
	td.GapReview = &types.GapReview{Version: 1, TranscriptKey: "reviewed", Duration: 600, Unresolved: []types.KeepSegment{{Start: 10, End: 600}}}
	if !validateTranscriptSanity(td, 600, true) {
		t.Fatal("preserved trailing gap blocked processing")
	}
	td.Text, td.Segments = "", nil
	td.GapReview.Unresolved = []types.KeepSegment{{Start: 0, End: 600}}
	if !validateTranscriptSanity(td, 600, true) {
		t.Fatal("preserving all audio required fabricated transcript words")
	}
}

func TestMusicReviewRequiresOriginalAndAllowsUnprocessedAudio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "episode.mp3")
	if err := verifyMusicSource(path, path+".precut", 30, "http://classifier"); err != nil {
		t.Fatal(err)
	}
	if err := episode.SaveEpisodeStatus(episode.StatusPathFor(path), &types.EpisodeStatusFile{Original: types.EpisodeAudioMeta{DurationSec: 60}}); err != nil {
		t.Fatal(err)
	}
	if err := verifyMusicSource(path, path+".precut", 30, "http://classifier"); err == nil {
		t.Fatal("already cut audio accepted as original")
	}
	if err := verifyMusicSource(path, path+".precut", 30, ""); err != nil {
		t.Fatal("disabled review changed recut behavior")
	}
}

func TestReviewedMusicCutsExcludeLegacyAdsAndDisabledReviews(t *testing.T) {
	td := &types.TranscriptionData{Cuts: []types.AdSegment{{Start: 0, End: 10, Reason: "old Gemini ad"}}, GapReview: &types.GapReview{Complete: true, MusicCuts: []types.AdSegment{{Start: 20, End: 30, Reason: "confirmed music"}}}}
	cfg := types.Config{WhisperConfig: types.WhisperConfig{YamnetURL: "http://classifier"}}
	if cuts := reviewedMusicCuts(cfg, td); len(cuts) != 1 || cuts[0].Start != 20 {
		t.Fatal(cuts)
	}
	if cuts := reviewedMusicCuts(types.Config{}, td); len(cuts) != 0 {
		t.Fatal("disabled review retained music cuts")
	}
	td.GapReview.Complete = false
	if cuts := reviewedMusicCuts(cfg, td); len(cuts) != 0 {
		t.Fatal("incomplete review approved cuts")
	}
}

func TestLiveMusicGapRecovery(t *testing.T) {
	audioPath, transcriptPath := os.Getenv("POD_GAP_TEST_AUDIO"), os.Getenv("POD_GAP_TEST_TRANSCRIPT")
	if audioPath == "" || transcriptPath == "" {
		t.Skip("optional live-service gap recovery test")
	}
	data, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "episode.wav")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}
	var td types.TranscriptionData
	if err := json.Unmarshal(data, &td); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(td.Text), "youtube") {
		t.Fatal("live regression fixture already contains the skipped ad")
	}
	profile := types.WhisperProfile{ID: 1, Engine: types.WhisperEngineRemote, URL: os.Getenv("POD_GAP_TEST_WHISPER"), Languages: []string{"*"}}
	cfg := types.Config{WhisperConfig: types.WhisperConfig{YamnetURL: os.Getenv("POD_GAP_TEST_YAMNET"), WhisperURL: profile.URL, ActiveWhisperID: 1, WhisperProfiles: []types.WhisperProfile{profile}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	duration := audio.GetAudioDuration(path)
	r := pipeline.StartMusicReview(ctx, pipeline.MusicReviewRequest{AudioPath: path, CachePath: path + ".yamnet.json", Duration: duration, Config: cfg, Options: types.ProcOptions{Quiet: true}})
	if err := r.Review(ctx, &td); err != nil {
		t.Fatal(err)
	}
	if td.GapReview.RecoveredSegments == 0 || !strings.Contains(strings.ToLower(td.Text), "youtube") {
		t.Fatalf("skipped ad not recovered: %+v", td.GapReview)
	}
	t.Logf("recovered %d segments; confirmed %d music cuts", td.GapReview.RecoveredSegments, len(td.GapReview.MusicCuts))
}
