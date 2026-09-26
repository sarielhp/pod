package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

func TestServerPruneKeepPolicy(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	now := time.Now()

	// 1. Regular podcast (default keep policy month / 30d)
	regDir := filepath.Join(tempDir, "Regular_Podcast")
	_ = os.MkdirAll(regDir, 0755)
	_ = podcast.GetOrSetPodcastShortID(regDir, "Regular Podcast")
	regMP3 := filepath.Join(regDir, "old.mp3")
	regTx := filepath.Join(regDir, "old.transcript.json")
	_ = os.WriteFile(regMP3, []byte("reg audio"), 0644)
	_ = os.WriteFile(regTx, []byte(`{"text":"regular transcript"}`), 0644)
	tOld := now.AddDate(0, 0, -45)
	_ = os.Chtimes(regMP3, tOld, tOld)

	// 2. Favorite podcast (keep policy favorite / 180d)
	favDir := filepath.Join(tempDir, "Fav_Podcast")
	_ = os.MkdirAll(favDir, 0755)
	_ = podcast.GetOrSetPodcastShortID(favDir, "Fav Podcast")
	favCfg := config.PodcastConfig{Favorite: true}
	_ = config.SavePodcastConfig(favDir, favCfg)
	favMP3 := filepath.Join(favDir, "recent.mp3")
	favTx := filepath.Join(favDir, "recent.transcript.json")
	_ = os.WriteFile(favMP3, []byte("fav audio"), 0644)
	_ = os.WriteFile(favTx, []byte(`{"text":"fav transcript"}`), 0644)
	_ = os.Chtimes(favMP3, tOld, tOld) // 45 days old is NOT expired for 180d favorite policy!

	cfg := Config{PodcastsDir: tempDir}
	cli := CLIOptions{}
	cli.Out = &buf
	cli.Err = &buf

	err := handleServerKeep(cfg, cli)
	if err != nil {
		t.Fatalf("handleServerKeep failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "pruned 1 episode(s)") {
		t.Errorf("expected prune report in output, got: %s", out)
	}
	if util.FileExists(regMP3) {
		t.Errorf("expected regular old MP3 to be pruned")
	}
	if !util.FileExists(regTx) {
		t.Errorf("expected regular transcript to be preserved")
	}
	if !util.FileExists(favMP3) {
		t.Errorf("expected favorite MP3 to be kept under 180d policy")
	}
	if !util.FileExists(favTx) {
		t.Errorf("expected favorite transcript to be preserved")
	}
}

func TestServerPruneDryRun(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	now := time.Now()

	regDir := filepath.Join(tempDir, "News_Podcast")
	_ = os.MkdirAll(regDir, 0755)
	_ = podcast.GetOrSetPodcastShortID(regDir, "News Podcast")
	cfgPod := config.PodcastConfig{}
	cfgPod.SetKeepPolicy("hourly") // 1 day retention
	_ = config.SavePodcastConfig(regDir, cfgPod)

	mp3File := filepath.Join(regDir, "news.mp3")
	_ = os.WriteFile(mp3File, []byte("audio"), 0644)
	tOld := now.AddDate(0, 0, -3)
	_ = os.Chtimes(mp3File, tOld, tOld)

	cfg := Config{PodcastsDir: tempDir}
	cli := CLIOptions{}
	cli.DryRun = true
	cli.Out = &buf
	cli.Err = &buf

	err := handleServerKeep(cfg, cli)
	if err != nil {
		t.Fatalf("handleServerKeep failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[dry-run]") || !strings.Contains(out, "would prune 1 episode") {
		t.Errorf("expected dry-run output, got: %s", out)
	}
	if !util.FileExists(mp3File) {
		t.Errorf("dry-run should not delete audio file on disk")
	}
}
