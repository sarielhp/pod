package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"pod/pkg/pipeline"
	"pod/pkg/podcast"
	"pod/pkg/types"
	"pod/pkg/util"
	"strings"
	"testing"
	"time"
)

func TestLsLatestCommand(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	pod1 := filepath.Join(tempDir, "Daily_Show")
	pod2 := filepath.Join(tempDir, "News_Hour")
	_ = os.MkdirAll(pod1, 0755)
	_ = os.MkdirAll(pod2, 0755)

	ep1 := filepath.Join(pod1, "ep1.mp3")
	ep2 := filepath.Join(pod2, "ep2.mp3")
	ep3 := filepath.Join(pod2, "ep3.mp3")

	_ = os.WriteFile(ep1, []byte("audio1"), 0644)
	time.Sleep(10 * time.Millisecond)
	_ = os.WriteFile(ep2, []byte("audio2"), 0644)
	time.Sleep(10 * time.Millisecond)
	_ = os.WriteFile(ep3, []byte("audio3"), 0644)

	_ = pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(ep3), &EpisodeStatusFile{
		Status: StateDone,
	})
	for i, path := range []string{ep1, ep2, ep3} {
		st := pipeline.GetOrCreateEpisodeStatus(path)
		st.PublicationSource = "source"
		st.PublishedAt = time.Date(2026, 9, 8+i, 9, 0, 0, 0, time.UTC).Format(time.RFC3339)
		if err := pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(path), st); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(strings.TrimSuffix(ep3, filepath.Ext(ep3))+".transcript.json", []byte(`{"text":"This episode contains a complete discussion with enough meaningful transcript text."}`), 0644)

	cfg := Config{
		PodcastsDir: tempDir,
	}

	cli := CLIOptions{
		ProcOptions: ProcOptions{
			Count: 2,
		},
		InfoSubcmd: "latest",
	}
	cli.Out = &buf
	cli.Err = &buf
	err := runInfoCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runInfoCommand failed: %v", err)
	}

	outBytes := buf.Bytes()
	out := string(outBytes)

	if !strings.Contains(out, "Latest 2 Episodes Across All Podcasts") {
		t.Errorf("expected Latest 2 header, got: %s", out)
	}
	if !strings.Contains(out, "ep3") {
		t.Errorf("expected newest episode ep3 to be listed, got: %s", out)
	}
	// The listing shows one glyph per fact rather than naming a state: the
	// cleaned episode carries the ad-free tick, the one still needing ad
	// removal does not.
	if !strings.Contains(out, adFreeMark) {
		t.Errorf("expected the cleaned episode to be marked ad-free, got: %s", out)
	}
	if !strings.Contains(out, downloadedMark) {
		t.Errorf("expected downloaded episodes to be marked, got: %s", out)
	}
	if strings.Contains(out, "Clean") {
		t.Errorf("status words should no longer appear, got: %s", out)
	}
}

func TestLsSinglePodcastCommand(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	pod1 := filepath.Join(tempDir, "ShowA")
	_ = os.MkdirAll(pod1, 0755)
	ep1 := filepath.Join(pod1, "ep1.mp3")
	_ = os.WriteFile(ep1, []byte("audio"), 0644)

	id := podcast.GetOrSetPodcastShortID(pod1, "ShowA")

	cfg := Config{
		PodcastsDir: tempDir,
	}

	cli := CLIOptions{
		Args: []string{id},
	}
	cli.Out = &buf
	cli.Err = &buf
	err := runInfoCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runInfoCommand failed: %v", err)
	}

	outBytes := buf.Bytes()
	out := string(outBytes)

	if !strings.Contains(out, "Podcast: ShowA") || !strings.Contains(out, "ep1") {
		t.Errorf("expected podcast episodes listing, got: %s", out)
	}
}

func TestLsAllPodcastsCommand(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	pod1 := filepath.Join(tempDir, "Alpha_Show")
	pod2 := filepath.Join(tempDir, "Beta_Cast")
	_ = os.MkdirAll(pod1, 0755)
	_ = os.MkdirAll(pod2, 0755)

	_ = os.WriteFile(filepath.Join(pod1, "ep1.mp3"), []byte("audio"), 0644)
	_ = os.WriteFile(filepath.Join(pod2, "ep2.mp3"), []byte("audio"), 0644)

	id1 := podcast.GetOrSetPodcastShortID(pod1, "Alpha Show")
	id2 := podcast.GetOrSetPodcastShortID(pod2, "Beta Cast")

	cfg := Config{PodcastsDir: tempDir}

	cli := CLIOptions{}
	cli.Out = &buf
	cli.Err = &buf
	err := runInfoCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runInfoCommand failed: %v", err)
	}

	outBytes := buf.Bytes()
	out := string(outBytes)

	if !strings.Contains(out, "Podcasts in Library") || !strings.Contains(out, id1) || !strings.Contains(out, id2) {
		t.Errorf("expected podcasts list with IDs %s and %s, got: %s", id1, id2, out)
	}
}

func TestLsAllPodcastsJSONAndQuiet(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	pod1 := filepath.Join(tempDir, "Gamma_Show")
	_ = os.MkdirAll(pod1, 0755)
	_ = os.WriteFile(filepath.Join(pod1, "ep1.mp3"), []byte("audio"), 0644)
	id1 := podcast.GetOrSetPodcastShortID(pod1, "Gamma Show")

	cfg := Config{PodcastsDir: tempDir}

	// JSON test

	cliJSON := CLIOptions{Args: []string{"podcasts"}, JSON: true}
	cliJSON.Out = &buf
	cliJSON.Err = &buf
	err := runInfoCommand(cfg, cliJSON)

	if err != nil {
		t.Fatalf("runInfoCommand json failed: %v", err)
	}
	outBytes := buf.Bytes()
	if !strings.Contains(string(outBytes), id1) || !strings.Contains(string(outBytes), "episode_count") {
		t.Errorf("expected json output with id and episode_count, got: %s", string(outBytes))
	}

	// Quiet test
	buf.Reset()

	cliQuiet := CLIOptions{
		ProcOptions: ProcOptions{
			Quiet: true,
		},
	}
	cliQuiet.Out = &buf
	cliQuiet.Err = &buf
	err = runInfoCommand(cfg, cliQuiet)

	if err != nil {
		t.Fatalf("runInfoCommand quiet failed: %v", err)
	}
	qBytes, _ := buf.Bytes(), error(nil)
	if strings.TrimSpace(string(qBytes)) != id1 {
		t.Errorf("expected quiet output %q, got: %s", id1, string(qBytes))
	}
}

func TestLsSinglePodcastJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	pod1 := filepath.Join(tempDir, "Delta_Show")
	_ = os.MkdirAll(pod1, 0755)
	_ = os.WriteFile(filepath.Join(pod1, "ep1.mp3"), []byte("audio"), 0644)
	id1 := podcast.GetOrSetPodcastShortID(pod1, "Delta Show")

	cfg := Config{PodcastsDir: tempDir}

	cli := CLIOptions{Args: []string{id1}, JSON: true}
	cli.Out = &buf
	cli.Err = &buf
	err := runInfoCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runInfoCommand failed: %v", err)
	}

	outBytes := buf.Bytes()
	out := string(outBytes)
	if !strings.Contains(out, "recent_episodes") || !strings.Contains(out, id1) {
		t.Errorf("expected json episodes list, got: %s", out)
	}
}

func TestLsLatestHebrewEpisodeTitle(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	pod := filepath.Join(tempDir, "Hebrew_Podcast")
	_ = os.MkdirAll(pod, 0755)

	epHebrew := filepath.Join(pod, "פרק 1 - שלום עולם.mp3")
	_ = os.WriteFile(epHebrew, []byte("audio"), 0644)

	cfg := Config{PodcastsDir: tempDir}

	cli := CLIOptions{
		ProcOptions: ProcOptions{
			Count: 1,
		},
		InfoSubcmd: "latest",
	}
	cli.Out = &buf
	cli.Err = &buf
	err := runInfoCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runInfoCommand failed: %v", err)
	}

	outBytes := buf.Bytes()
	out := string(outBytes)

	rawTitle := "פרק 1 - שלום עולם"
	expectedTitle := util.DisplayName(rawTitle)
	if !strings.Contains(out, expectedTitle) {
		t.Errorf("expected latest episodes table to contain displayName reordered %q, got: %s", expectedTitle, out)
	}
	if expectedTitle != rawTitle && strings.Contains(out, rawTitle) {
		t.Errorf("expected latest episodes table not to contain raw Hebrew %q, got: %s", rawTitle, out)
	}
}

func TestLsSinglePodcastHebrewEpisodeTitle(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	pod := filepath.Join(tempDir, "פודקאסט_בעברית")
	_ = os.MkdirAll(pod, 0755)

	epHebrew := filepath.Join(pod, "פרק ראשון של הפודקאסט.mp3")
	_ = os.WriteFile(epHebrew, []byte("audio"), 0644)

	id := podcast.GetOrSetPodcastShortID(pod, "פודקאסט_בעברית")
	cfg := Config{PodcastsDir: tempDir}

	cli := CLIOptions{
		Args: []string{id},
	}
	cli.Out = &buf
	cli.Err = &buf
	err := runInfoCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runInfoCommand failed: %v", err)
	}

	outBytes := buf.Bytes()
	out := string(outBytes)

	rawTitle := "פרק ראשון של הפודקאסט"
	expectedTitle := util.TruncateDisplayName(rawTitle, 35)
	if !strings.Contains(out, expectedTitle) {
		t.Errorf("expected single podcast table to contain displayName reordered %q, got: %s", expectedTitle, out)
	}
	expectedPodTitle := util.DisplayName("פודקאסט_בעברית")
	if !strings.Contains(out, expectedPodTitle) {
		t.Errorf("expected header to contain displayName reordered podcast title %q, got: %s", expectedPodTitle, out)
	}
}

func TestPrintLatestEpisodesTableHebrewDirect(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	items := []lsEpisodeItem{
		{
			podcastTitle:   "חדשות הבוקר",
			podcastShortID: "hds1",
			episodeShortID: "ep01",
			episodeName:    "פרק בדיקה עם עברית",
			modTime:        time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC),
			origDuration:   120.0,
			statusStr:      "Needs Ad Removal",
			statusColor:    "yellow",
		},
	}

	printLatestEpisodesTable(&buf, items, 1)

	outBytes := buf.Bytes()
	out := string(outBytes)

	expectedEp := util.DisplayName("פרק בדיקה עם עברית")
	expectedPod := util.DisplayName("חדשות הבוקר")

	if !strings.Contains(out, expectedEp) {
		t.Errorf("expected table to contain %q, got: %s", expectedEp, out)
	}
	if !strings.Contains(out, expectedPod) {
		t.Errorf("expected table to contain %q, got: %s", expectedPod, out)
	}
}

func TestFormatShortStatusAdR(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"NeedAdR", "✂ NeedAdR"},
		{"NeedsAd", "✂ NeedAdR"},
		{"NeedAd", "✂ NeedAdR"},
		{"Needs Ad Removal", "✂ NeedAdR"},
		{"Clean", "✓ Clean"},
		{"Queued", "⏳ Queued"},
		{"Active", "⚡ Active"},
	}
	for _, tc := range cases {
		got := formatShortStatus(tc.input)
		if got != tc.want {
			t.Errorf("formatShortStatus(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestGetEpisodeStatusLabelStaleActive(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	mp3Path := filepath.Join(tempDir, "stale_ep.mp3")
	if err := os.WriteFile(mp3Path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	st := pipeline.GetOrCreateEpisodeStatus(mp3Path)
	st.Status = StateTranscribingLocally
	if err := pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(mp3Path), st); err != nil {
		t.Fatal(err)
	}

	label, color := getEpisodeStatusLabel(mp3Path)
	if label != "NeedAdR" || color != "yellow" {
		t.Fatalf("expected NeedAdR/yellow for dead local process, got %s/%s", label, color)
	}

	reloaded, err := pipeline.LoadEpisodeStatus(pipeline.StatusPathFor(mp3Path))
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != types.StateNeedsAdR {
		t.Fatalf("expected healed status %s, got %s", types.StateNeedsAdR, reloaded.Status)
	}

	lock, err := util.AcquireFileLock(mp3Path)
	if err != nil || lock == nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer lock.Release()

	st.Status = StateTranscribingLocally
	_ = pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(mp3Path), st)

	label, color = getEpisodeStatusLabel(mp3Path)
	if label != "In Progress" || color != "yellow" {
		t.Fatalf("expected In Progress/yellow for actively locked process, got %s/%s", label, color)
	}
}

func TestResolveEpisodePublicationTimeFallback(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	mp3Path := filepath.Join(tempDir, "fallback_ep.mp3")
	if err := os.WriteFile(mp3Path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(mp3Path)
	if err != nil {
		t.Fatal(err)
	}

	got := resolveEpisodePublicationTime(mp3Path, nil, fi)
	if !got.Equal(fi.ModTime()) {
		t.Fatalf("expected modTime %v, got %v", fi.ModTime(), got)
	}

	st := &types.EpisodeStatusFile{
		PublishedAt: "2026-09-05T10:00:00Z",
	}
	got = resolveEpisodePublicationTime(mp3Path, st, fi)
	want := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected published_at %v, got %v", want, got)
	}
}
