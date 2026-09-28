package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/kitty"
	"pod/pkg/transcribe"
	"pod/pkg/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderImageToHalfBlocks(t *testing.T) {
	result, err := kitty.EncodeKittyGraphicsFile("/nonexistent/image.jpg", 10, 10)
	if err == nil {
		t.Error("encodeKittyGraphicsFile with non-existent file should return error")
	}
	if result != "" {
		t.Error("result should be empty on error")
	}
}

func TestRenderImageFile(t *testing.T) {
	_, err := kitty.EncodeKittyGraphicsFile("/nonexistent/image.jpg", 10, 10)
	if err == nil {
		t.Error("encodeKittyGraphicsFile with non-existent file should return error")
	}
}

func TestTUIApplyColorConfig(t *testing.T) {
	origCyan, origYellow, origGreen := colorCyan, colorYellow, colorGreen
	t.Cleanup(func() { colorCyan, colorYellow, colorGreen = origCyan, origYellow, origGreen })

	applyTUIColorConfig(&types.TUIColorConfig{Cyan: "#ff0000", Yellow: "#00ff00"})
	if colorCyan != lipgloss.Color("#ff0000") || colorYellow != lipgloss.Color("#00ff00") {
		t.Fatalf("configured colours not applied: cyan %v yellow %v", colorCyan, colorYellow)
	}
	if colorGreen != origGreen {
		t.Fatalf("an unset colour must keep its default, got %v", colorGreen)
	}
	applyTUIColorConfig(nil)
	if colorCyan != lipgloss.Color("#ff0000") {
		t.Fatalf("a nil config must be a no-op, cyan became %v", colorCyan)
	}
}

func TestTUINewModelWithConfig(t *testing.T) {
	bk := &TuiBackend{
		LoadPodcasts: func(dir string) ([]tuiPodcast, error) {
			return nil, nil
		},
		LoadQueues: func(pods []tuiPodcast) map[string][]string {
			return map[string][]string{}
		},
	}
	cfg := &types.Config{
		TUIColor: &types.TUIColorConfig{
			Cyan: "#ff0000",
		},
	}
	m := newTuiModel(bk, "/tmp/test", cfg, testLibrary())
	if m == nil {
		t.Error("newTuiModel should return non-nil model")
	}
	if m.podcastsDir != "/tmp/test" {
		t.Errorf("expected podcastsDir /tmp/test, got %s", m.podcastsDir)
	}
}

func TestTUISetTerminalTitle(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	makeTestModel().setTerminalTitle("test title")
	os.Stdout = origStdout
	_ = w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "\033]0;test title\007" {
		t.Fatalf("terminal title escape = %q", got)
	}
}

func TestFindCoverImage(t *testing.T) {
	bare := t.TempDir()
	generated := kitty.FindCoverImage(bare)
	if !strings.HasPrefix(generated, config.CacheDirForPodcast(bare)) || filepath.Base(generated) != "cover.png" {
		t.Fatalf("a directory without art must get a generated cover.png in its cache dir, got %q", generated)
	}
	if fi, err := os.Stat(generated); err != nil || fi.Size() == 0 {
		t.Fatalf("generated cover is not a real file: %v", err)
	}
	dir := t.TempDir()
	cover := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(cover, []byte("jpeg bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := kitty.FindCoverImage(dir); got != cover {
		t.Fatalf("FindCoverImage = %q, want %q", got, cover)
	}
}

func TestStripHTMLEntities(t *testing.T) {
	cases := []struct {
		in  string
		out string
	}{
		{"Hello &amp; World", "Hello & World"},
		{"a &lt; b &gt; c", "a < b > c"},
		{"&quot;quoted&quot;", "\"quoted\""},
		{"&apos;single&apos;", "'single'"},
		{"hello&nbsp;world", "hello world"},
		{"<b>bold</b>", "bold"},
		{"<p>para</p>", "para"},
	}
	for _, c := range cases {
		got := backend.StripHTML(c.in)
		if got != c.out {
			t.Errorf("stripHTML(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestMergeSegmentsText(t *testing.T) {
	segs := []types.TranscriptionSegment{
		{Start: 0, End: 10, Text: "first part"},
		{Start: 9, End: 20, Text: "second part"},
	}
	merged := transcribe.MergeSegments(segs)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged segment, got %d", len(merged))
	}
	if merged[0].Text != "first part second part" {
		t.Errorf("merged text should combine both parts, got %q", merged[0].Text)
	}
	if merged[0].End != 20 {
		t.Errorf("merged end should be 20, got %f", merged[0].End)
	}
}

func TestMergeSegmentsNoOverlap(t *testing.T) {
	segs := []types.TranscriptionSegment{
		{Start: 0, End: 10, Text: "first"},
		{Start: 20, End: 30, Text: "second"},
	}
	merged := transcribe.MergeSegments(segs)
	if len(merged) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(merged))
	}
}

func TestTranscribeRetryBuffer(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("test data")
	reader := bytes.NewReader(buf.Bytes())
	first := make([]byte, 4)
	n1, _ := reader.Read(first)
	reader.Seek(0, 0)
	second := make([]byte, 4)
	n2, _ := reader.Read(second)
	if n1 != n2 || string(first) != string(second) {
		t.Error("bytes.NewReader should allow re-reading")
	}
}

func TestTUIFKeyScreens(t *testing.T) {
	m := makeTestModel()

	m.handleKey(tea.KeyMsg{Type: tea.KeyF1})
	if m.screen != screenPlayer {
		t.Errorf("expected screenPlayer on F1, got %v", m.screen)
	}
	viewF1 := m.View()
	if !strings.Contains(viewF1, "AUDIO PLAYER (F1)") {
		t.Errorf("expected AUDIO PLAYER (F1) in view, got %q", viewF1)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m.screen != screenPodcasts {
		t.Errorf("expected screenPodcasts after Esc, got %v", m.screen)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	if m.screen != screenPlayQueue {
		t.Errorf("expected screenPlayQueue on F2, got %v", m.screen)
	}
	viewF2 := m.View()
	if !strings.Contains(viewF2, "PLAYING QUEUE (F2)") {
		t.Errorf("expected PLAYING QUEUE (F2) in view, got %q", viewF2)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.pqGrabbed {
		t.Errorf("expected pqGrabbed to be true after space")
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m.screen != screenPodcasts {
		t.Errorf("expected screenPodcasts after Esc, got %v", m.screen)
	}

	m.screen = screenPodcastDetail
	m.handleKey(tea.KeyMsg{Type: tea.KeyF1})
	m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	m.handleKey(tea.KeyMsg{Type: tea.KeyF3})
	m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m.screen != screenPodcastDetail {
		t.Errorf("expected screenPodcastDetail after Esc from F-key hopping, got %v", m.screen)
	}
}

func TestAutoAdQueueWhenPlayQueueAdded(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcastDetail
	m.podIdx = 0
	m.epIdx = 1

	podDir := m.podcasts[0].dir
	m.queue[podDir] = nil

	m.playSelectedEpisode()

	found := false
	for _, fn := range m.queue[podDir] {
		if fn == "ep102.mp3" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ep102.mp3 to be auto-added to ad removal queue: %v", m.queue[podDir])
	}
}

func TestQueueToggleAdFreeEpisode(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcastDetail
	m.podIdx = 0
	m.epIdx = 0

	podDir := m.podcasts[0].dir
	m.queue[podDir] = nil

	m.handleQueueToggle()

	if len(m.queue[podDir]) != 0 {
		t.Errorf("expected ad-free episode not to be added to queue, got: %v", m.queue[podDir])
	}
	if m.popupMsg != "Episode already has ads removed" {
		t.Errorf("expected popup 'Episode already has ads removed', got %q", m.popupMsg)
	}
}

func TestTUICyclePodcastConfig(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podcasts[0].config = config.PodcastConfig{AdRemoval: config.AdRemovalNone}
	m.screen = screenPodcasts
	m.podIdx = 0

	m.handlePodcastConfigToggle()
	if m.podcasts[0].config.AdRemoval != config.AdRemovalLatest {
		t.Errorf("expected AdRemovalLatest, got %s", m.podcasts[0].config.AdRemoval)
	}
	loaded := config.LoadPodcastConfig(tempDir, config.PodcastConfig{})
	if loaded.AdRemoval != config.AdRemovalLatest {
		t.Errorf("expected saved file to have latest, got %s", loaded.AdRemoval)
	}

	m.handlePodcastConfigToggle()
	if m.podcasts[0].config.AdRemoval != config.AdRemovalAll {
		t.Errorf("expected AdRemovalAll, got %s", m.podcasts[0].config.AdRemoval)
	}

	m.handlePodcastConfigToggle()
	if m.podcasts[0].config.AdRemoval != config.AdRemovalNone {
		t.Errorf("expected AdRemovalNone, got %s", m.podcasts[0].config.AdRemoval)
	}
}

func TestTUITranscriptViewer(t *testing.T) {
	tempDir := t.TempDir()
	mp3Path := tempDir + "/ep1.mp3"
	txtPath := tempDir + "/ep1.transcript.txt"
	cutsPath := tempDir + "/ep1.cuts.json"
	os.WriteFile(mp3Path, []byte("audio"), 0644)
	os.WriteFile(txtPath, []byte("[00:00.0 -> 00:10.0] Hello world\n[00:10.0 -> 00:20.0] Buy our sponsor product now"), 0644)
	os.WriteFile(cutsPath, []byte(`{"cut_intervals":[{"start_sec":10.0,"end_sec":20.0,"reason":"sponsor"}]}`), 0644)

	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podcasts[0].episodes[0].path = mp3Path
	m.podcasts[0].episodes[0].hasTranscript = true
	m.screen = screenPodcastDetail
	m.podIdx = 0
	m.epIdx = 0

	m.openTranscriptViewer()
	if m.screen != screenTranscript {
		t.Fatalf("expected screenTranscript, got %v", m.screen)
	}
	if len(m.transcriptLines) != 2 {
		t.Errorf("expected 2 transcript lines, got %d", len(m.transcriptLines))
	}
	if len(m.transcriptItems) != 2 {
		t.Fatalf("expected 2 transcript items, got %d", len(m.transcriptItems))
	}
	if m.transcriptItems[0].isAd {
		t.Errorf("expected item 0 not to be ad")
	}
	if !m.transcriptItems[1].isAd {
		t.Errorf("expected item 1 to be identified as ad")
	}

	view := m.View()
	if !strings.Contains(view, "TRANSCRIPT") || !strings.Contains(view, "Hello world") {
		t.Errorf("expected transcript view to contain text, got: %s", view)
	}
	if !strings.Contains(view, "Tab Short Time") {
		t.Errorf("expected view to contain Tab Short Time help, got: %s", view)
	}

	m.handleTranscriptKey("tab")
	if m.transcriptViewMode != 1 {
		t.Errorf("expected transcriptViewMode to be 1 after 1st tab, got %d", m.transcriptViewMode)
	}
	viewShort := m.View()
	if !strings.Contains(viewShort, "Tab Line Nums") {
		t.Errorf("expected view to contain Tab Line Nums help, got: %s", viewShort)
	}

	m.handleTranscriptKey("tab")
	if m.transcriptViewMode != 2 {
		t.Errorf("expected transcriptViewMode to be 2 after 2nd tab, got %d", m.transcriptViewMode)
	}
	viewLines := m.View()
	if !strings.Contains(viewLines, "Tab Time Arrows") || !strings.Contains(viewLines, "│") {
		t.Errorf("expected view to contain Tab Time Arrows help and line separator, got: %s", viewLines)
	}

	m.handleTranscriptKey("tab")
	if m.transcriptViewMode != 0 {
		t.Errorf("expected transcriptViewMode to be 0 after 3rd tab, got %d", m.transcriptViewMode)
	}

	m.handleEscape()
	if m.screen != screenPodcastDetail {
		t.Errorf("expected escape to return to screenPodcastDetail, got %v", m.screen)
	}
}
