package adremoval

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/pipeline"
	"pod/pkg/types"
	"pod/pkg/util"
)

func writeRealMP3(t *testing.T, path string, seconds int) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono",
		"-t", itoa(seconds), "-c:a", "libmp3lame", "-b:a", "64k", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test mp3: %v: %s", err, out)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func writeSaneTranscript(t *testing.T, path string, dur float64) {
	t.Helper()
	body := `{"text":"one two three four five six seven eight nine ten eleven twelve ` +
		`thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twenty-one",` +
		`"segments":[{"start":0.0,"end":` + "10.0" + `,"text":"one two three four five six seven eight nine ten ` +
		`eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twenty-one"}]}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// offlineProfile has no URL, so detectAdsLLM returns nil without any network call.
func offlineProfile() types.LLMProfile {
	return types.LLMProfile{ID: 1, Name: "offline", Type: "ollama", Model: "none"}
}

func TestTranscribeFailureReturnsInsteadOfPanicking(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "ep.mp3")
	if err := os.WriteFile(mp3, []byte("not an mp3"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ep.transcript.json"), []byte("{{{ bad"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := processSingleAudioFile(0, 1, 0, mp3,
		types.ProcOptions{
			Quiet: true,
		}, types.Config{}, time.Now(), offlineProfile())

	if err == nil {
		t.Errorf("expected error when the transcript cannot be parsed")
	}
}

func TestRecutDoesNotFallThroughIntoTheFullPipeline(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "ep.mp3")
	writeRealMP3(t, mp3, 10)
	cuts := `{"version":1,"generator":"test","target_file":"ep.mp3","original_duration_sec":10,
	  "cut_intervals":[{"start_sec":2,"end_sec":3,"reason":"ad"}]}`
	if err := os.WriteFile(filepath.Join(dir, "ep.cuts.json"), []byte(cuts), 0644); err != nil {
		t.Fatal(err)
	}
	// A transcript that cannot be parsed. recut must never look at it; if control
	// falls through into the transcribe pipeline this panics at the nil deref.
	if err := os.WriteFile(filepath.Join(dir, "ep.transcript.json"), []byte("{{{ bad"), 0644); err != nil {
		t.Fatal(err)
	}

	optsRecut := types.ProcOptions{
		Quiet: true,
	}
	optsRecut.Recut = true
	_, _ = processSingleAudioFile(0, 1, 0, mp3,
		optsRecut, types.Config{}, time.Now(), offlineProfile())
}

func TestTranscribeMinDoesNotWriteCutMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "ep.mp3")
	writeRealMP3(t, mp3, 10)
	writeSaneTranscript(t, filepath.Join(dir, "ep.transcript.json"), 10)

	before, err := os.ReadFile(mp3)
	if err != nil {
		t.Fatal(err)
	}

	optsTMin := types.ProcOptions{
		Quiet: true,
	}
	optsTMin.TranscribeMin = "1"
	_, _ = processSingleAudioFile(0, 1, 0, mp3,
		optsTMin, types.Config{}, time.Now(), offlineProfile())

	if util.FileExists(filepath.Join(dir, "ep.cuts.json")) {
		t.Errorf("--tminutes wrote ep.cuts.json; a preview run must not touch cut metadata")
	}
	if util.FileExists(mp3 + ".precut") {
		t.Errorf("--tminutes created a .precut; it claims the original is not modified")
	}
	after, err := os.ReadFile(mp3)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("--tminutes modified the original audio after printing that it did not")
	}
}

func TestNoAdsDetectedDoesNotReEncodeOrCreatePrecut(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "ep.mp3")
	writeRealMP3(t, mp3, 10)
	writeSaneTranscript(t, filepath.Join(dir, "ep.transcript.json"), 10)

	before, err := os.ReadFile(mp3)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = processSingleAudioFile(0, 1, 0, mp3,
		types.ProcOptions{
			Quiet: true,
		}, types.Config{}, time.Now(), offlineProfile())

	if util.FileExists(mp3 + ".precut") {
		t.Errorf("no ads were detected, but the original was moved to .precut")
	}
	after, err := os.ReadFile(mp3)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("no ads were detected, but the audio was re-encoded (%d -> %d bytes)",
			len(before), len(after))
	}
}

func TestAdDetectionFailureDoesNotMarkEpisodeClean(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "ep.mp3")
	writeRealMP3(t, mp3, 10)
	writeSaneTranscript(t, filepath.Join(dir, "ep.transcript.json"), 10)

	failingProfile := types.LLMProfile{
		ID:    2,
		Name:  "failing",
		Type:  "openrouter",
		URL:   "http://127.0.0.1:1/invalid",
		Model: "test-model",
	}

	_, err := processSingleAudioFile(0, 1, 0, mp3,
		types.ProcOptions{
			Quiet: true,
		}, types.Config{}, time.Now(), failingProfile)

	if err == nil {
		t.Errorf("expected error when LLM ad detection fails")
	}

	cutsFile := filepath.Join(dir, "ep.cuts.json")
	if util.FileExists(cutsFile) {
		t.Errorf("ep.cuts.json should not exist when LLM ad detection fails")
	}

	if pipeline.IsEpisodeCompleted(mp3) {
		t.Errorf("episode must not be considered completed when ad detection fails")
	}

	st, err := pipeline.LoadEpisodeStatus(pipeline.StatusPathFor(mp3))
	if err != nil || st == nil {
		t.Fatalf("expected status file to exist: %v", err)
	}
	if st.Status != types.StateFailed {
		t.Errorf("expected status %s, got %s", types.StateFailed, st.Status)
	}
}
