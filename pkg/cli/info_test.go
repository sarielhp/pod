package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"strings"
	"testing"
)

func TestInfoPodcastCardAndJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "Huberman_Lab")
	_ = os.MkdirAll(podDir, 0755)

	ep1 := filepath.Join(podDir, "ep1.mp3")
	_ = os.WriteFile(ep1, []byte("audio"), 0644)

	podID := podcast.GetOrSetPodcastShortID(podDir, "Huberman Lab")
	cfg := Config{PodcastsDir: tempDir}

	// Test text card

	cli := CLIOptions{Args: []string{podID}}
	cli.Out = &buf
	cli.Err = &buf
	err := runInfoCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runInfoCommand failed: %v", err)
	}

	outBytes := buf.Bytes()
	out := string(outBytes)

	if !strings.Contains(out, "Podcast: Huberman_Lab") || !strings.Contains(out, podID) {
		t.Errorf("expected podcast info header with Huberman_Lab and %s, got: %s", podID, out)
	}
	if !strings.Contains(out, "Policy & Sync:") || !strings.Contains(out, "Library Stats:") {
		t.Errorf("expected policy and library stats in output, got: %s", out)
	}

	// Test JSON mode
	buf.Reset()

	cliJSON := CLIOptions{Args: []string{podID}, JSON: true}
	cliJSON.Out = &buf
	cliJSON.Err = &buf
	err = runInfoCommand(cfg, cliJSON)

	if err != nil {
		t.Fatalf("runInfoCommand json failed: %v", err)
	}

	jsonBytes, _ := buf.Bytes(), error(nil)
	var podInfo PodcastInfoJSON
	if err := json.Unmarshal(jsonBytes, &podInfo); err != nil {
		t.Fatalf("failed to parse podcast info json: %v", err)
	}
	if podInfo.ID != podID || podInfo.Title != "Huberman_Lab" {
		t.Errorf("mismatched podInfo: got %+v", podInfo)
	}
}

func TestInfoEpisodeCardAndJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "Daily_Cast")
	_ = os.MkdirAll(podDir, 0755)

	ep1 := filepath.Join(podDir, "ep1.mp3")
	_ = os.WriteFile(ep1, []byte("audio"), 0644)

	podID := podcast.GetOrSetPodcastShortID(podDir, "Daily Cast")
	epID := podcast.GetOrSetEpisodeShortID(podDir, podID, ep1)

	cfg := Config{PodcastsDir: tempDir}

	// Text card

	cli := CLIOptions{Args: []string{epID}}
	cli.Out = &buf
	cli.Err = &buf
	err := runInfoCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runInfoCommand failed for episode: %v", err)
	}

	outBytes := buf.Bytes()
	out := string(outBytes)

	if !strings.Contains(out, "Episode: ep1") || !strings.Contains(out, epID) {
		t.Errorf("expected episode info for %s, got: %s", epID, out)
	}

	// JSON mode
	buf.Reset()

	cliJSON := CLIOptions{Args: []string{epID}, JSON: true}
	cliJSON.Out = &buf
	cliJSON.Err = &buf
	err = runInfoCommand(cfg, cliJSON)

	if err != nil {
		t.Fatalf("runInfoCommand json failed for episode: %v", err)
	}

	jsonBytes, _ := buf.Bytes(), error(nil)
	var epInfo EpisodeInfoJSON
	if err := json.Unmarshal(jsonBytes, &epInfo); err != nil {
		t.Fatalf("failed to parse episode info json: %v", err)
	}
	if epInfo.ID != epID || epInfo.PodcastID != podID {
		t.Errorf("mismatched epInfo: got %+v", epInfo)
	}
}

func TestInfoEpisodeWithCuts(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "ShowWithAds")
	_ = os.MkdirAll(podDir, 0755)

	ep1 := filepath.Join(podDir, "ep1.mp3")
	_ = os.WriteFile(ep1, []byte("audio"), 0644)

	cutsData := CutsData{
		OriginalDurationSec: 600,
		TotalCutDurationSec: 60,
		CutIntervals: []CutEntry{
			{
				StartSec:       10,
				EndSec:         70,
				DurationSec:    60,
				StartFormatted: "00:10",
				EndFormatted:   "01:10",
				Reason:         "Sponsor segment",
			},
		},
	}
	cutsBytes, _ := json.Marshal(cutsData)
	_ = os.WriteFile(util.StripExt(ep1)+".cuts.json", cutsBytes, 0644)

	podID := podcast.GetOrSetPodcastShortID(podDir, "ShowWithAds")
	epID := podcast.GetOrSetEpisodeShortID(podDir, podID, ep1)

	cfg := Config{PodcastsDir: tempDir}

	cli := CLIOptions{Args: []string{epID}}
	cli.Out = &buf
	cli.Err = &buf
	cli.ShowCuts = true
	err := runInfoCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runInfoCommand failed with cuts: %v", err)
	}

	outBytes := buf.Bytes()
	out := string(outBytes)

	if !strings.Contains(out, "Sponsor segment") || !strings.Contains(out, "00:10") {
		t.Errorf("expected cuts information in output, got: %s", out)
	}
}

func TestFormatPodcastInfoHebrew(t *testing.T) {
	t.Parallel()
	info := PodcastInfoJSON{
		ID:        "pod1",
		Title:     "פודקאסט חדשות",
		Directory: "/podcasts/news",
		Author:    "יוסי כהן",
		RecentEpisodes: []RecentEpisodeDTO{
			{
				ID:       "ep01",
				Title:    "פרק ראשון",
				Date:     "2026-09-06",
				Status:   "Clean",
				Duration: "10:00",
			},
		},
	}

	out := formatPodcastInfo(info)

	expectedTitle := util.DisplayName("פודקאסט חדשות")
	expectedAuthor := util.DisplayName("יוסי כהן")
	expectedEp := util.DisplayName("פרק ראשון")

	if !strings.Contains(out, expectedTitle) {
		t.Errorf("expected output to contain %q, got: %s", expectedTitle, out)
	}
	if !strings.Contains(out, expectedAuthor) {
		t.Errorf("expected output to contain %q, got: %s", expectedAuthor, out)
	}
	if !strings.Contains(out, expectedEp) {
		t.Errorf("expected output to contain %q, got: %s", expectedEp, out)
	}
}

func TestFormatEpisodeInfoHebrew(t *testing.T) {
	t.Parallel()
	info := EpisodeInfoJSON{
		ID:           "ep01",
		PodcastID:    "pod1",
		PodcastTitle: "פודקאסט היסטוריה",
		Title:        "פרק 1 - העת העתיקה",
		AudioPath:    "/podcasts/hist/ep1.mp3",
		Status:       "Clean",
	}

	out := formatEpisodeInfo(info)

	expectedPod := util.DisplayName("פודקאסט היסטוריה")
	expectedTitle := util.DisplayName("פרק 1 - העת העתיקה")

	if !strings.Contains(out, expectedPod) {
		t.Errorf("expected output to contain %q, got: %s", expectedPod, out)
	}
	if !strings.Contains(out, expectedTitle) {
		t.Errorf("expected output to contain %q, got: %s", expectedTitle, out)
	}
}

func TestInfoStatusDiscoveredPodcast(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "UnconfiguredPod")
	_ = os.MkdirAll(podDir, 0755)

	ep1 := filepath.Join(podDir, "ep1.mp3")
	_ = os.WriteFile(ep1, []byte("audio"), 0644)

	cfg := Config{PodcastsDir: tempDir}
	podcastsCount, totalEpisodes, totalNeedsAd := renderLocalSummary(&buf, cfg, false)

	if podcastsCount != 1 {
		t.Errorf("expected 1 podcast, got %d", podcastsCount)
	}
	if totalEpisodes != 1 {
		t.Errorf("expected 1 episode, got %d", totalEpisodes)
	}
	if totalNeedsAd != 1 {
		t.Errorf("expected 1 episode needing AdR under default ad removal policy, got %d", totalNeedsAd)
	}

	buf.Reset()
	renderLocalLibraryStatus(&buf, cfg, false)
	out := buf.String()
	if !strings.Contains(out, "UnconfiguredPod") || !strings.Contains(out, "1") {
		t.Errorf("expected detailed status report to list UnconfiguredPod, got:\n%s", out)
	}
}
