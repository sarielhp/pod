package podcast

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/config"
	"pod/pkg/util"
)

func TestApplyPodcastKeepPolicyAlways(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	now := time.Now()

	// Old episode from 200 days ago
	oldMP3 := filepath.Join(tempDir, "ep1.mp3")
	_ = os.WriteFile(oldMP3, []byte("fake mp3 data"), 0644)
	oldTime := now.AddDate(0, 0, -200)
	_ = os.Chtimes(oldMP3, oldTime, oldTime)

	cfg := config.PodcastConfig{}
	cfg.SetKeepPolicy(config.KeepPolicyAlways)

	res, err := ApplyPodcastKeepPolicy(tempDir, "Always Show", cfg, now, false)
	if err != nil {
		t.Fatalf("ApplyPodcastKeepPolicy failed: %v", err)
	}
	if res.DeletedEpisodes != 0 {
		t.Errorf("expected 0 deleted for KeepPolicyAlways, got %d", res.DeletedEpisodes)
	}
	if !util.FileExists(oldMP3) {
		t.Errorf("expected oldMP3 to still exist under KeepPolicyAlways")
	}
}

func TestApplyPodcastKeepPolicyMonth(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	now := time.Now()

	// ep1: 45 days old (expired for 30d policy)
	ep1MP3 := filepath.Join(tempDir, "ep1.mp3")
	ep1Tx := filepath.Join(tempDir, "ep1.transcript.json")
	ep1Status := filepath.Join(tempDir, "ep1.mp3.json")
	_ = os.WriteFile(ep1MP3, []byte("audio-ep1"), 0644)
	_ = os.WriteFile(ep1Tx, []byte(`{"text":"transcript of ep1"}`), 0644)
	_ = os.WriteFile(ep1Status, []byte(`{"version":1}`), 0644)
	t1 := now.AddDate(0, 0, -45)
	_ = os.Chtimes(ep1MP3, t1, t1)

	// ep2: 10 days old (kept)
	ep2MP3 := filepath.Join(tempDir, "ep2.mp3")
	ep2Tx := filepath.Join(tempDir, "ep2.transcript.json")
	_ = os.WriteFile(ep2MP3, []byte("audio-ep2"), 0644)
	_ = os.WriteFile(ep2Tx, []byte(`{"text":"transcript of ep2"}`), 0644)
	t2 := now.AddDate(0, 0, -10)
	_ = os.Chtimes(ep2MP3, t2, t2)

	cfg := config.PodcastConfig{}
	cfg.SetKeepPolicy(config.KeepPolicyMonth)

	res, err := ApplyPodcastKeepPolicy(tempDir, "Regular Show", cfg, now, false)
	if err != nil {
		t.Fatalf("ApplyPodcastKeepPolicy failed: %v", err)
	}
	if res.DeletedEpisodes != 1 {
		t.Errorf("expected 1 deleted episode, got %d", res.DeletedEpisodes)
	}
	if res.PreservedTranscripts != 1 {
		t.Errorf("expected 1 preserved transcript, got %d", res.PreservedTranscripts)
	}
	if util.FileExists(ep1MP3) {
		t.Errorf("expected ep1 MP3 to be deleted")
	}
	if !util.FileExists(ep1Tx) {
		t.Errorf("expected ep1 transcript to be preserved")
	}
	if !util.FileExists(ep1Status) {
		t.Errorf("expected ep1 status file to be preserved")
	}
	if !util.FileExists(ep2MP3) {
		t.Errorf("expected ep2 MP3 to be kept")
	}
	if !util.FileExists(ep2Tx) {
		t.Errorf("expected ep2 transcript to be preserved")
	}
}

func TestApplyPodcastKeepPolicyDryRun(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	now := time.Now()

	epMP3 := filepath.Join(tempDir, "ep_old.mp3")
	_ = os.WriteFile(epMP3, []byte("audio-old"), 0644)
	tOld := now.AddDate(0, 0, -50)
	_ = os.Chtimes(epMP3, tOld, tOld)

	cfg := config.PodcastConfig{}
	cfg.SetKeepPolicy(config.KeepPolicyMonth)

	res, err := ApplyPodcastKeepPolicy(tempDir, "Dry Run Show", cfg, now, true)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if res.DeletedEpisodes != 1 {
		t.Errorf("expected 1 reported deleted in dry run, got %d", res.DeletedEpisodes)
	}
	if !util.FileExists(epMP3) {
		t.Errorf("dry run should not delete file on disk")
	}
}

func TestApplyPodcastKeepPolicyHourlyAndFavorite(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	now := time.Now()

	// ep1: 2 days old (expired for hourly news 1d policy)
	ep1 := filepath.Join(tempDir, "news.mp3")
	_ = os.WriteFile(ep1, []byte("hourly news"), 0644)
	tNews := now.AddDate(0, 0, -2)
	_ = os.Chtimes(ep1, tNews, tNews)

	cfgHourly := config.PodcastConfig{}
	cfgHourly.SetKeepPolicy(config.KeepPolicyHourly)

	res, err := ApplyPodcastKeepPolicy(tempDir, "Hourly News", cfgHourly, now, false)
	if err != nil || res.DeletedEpisodes != 1 {
		t.Fatalf("hourly prune expected 1 deleted, got %d (err: %v)", res.DeletedEpisodes, err)
	}

	// Favorite show: 7 months old vs 3 months old
	favDir := filepath.Join(tempDir, "fav")
	_ = os.MkdirAll(favDir, 0755)
	favOld := filepath.Join(favDir, "fav_old.mp3")
	favNew := filepath.Join(favDir, "fav_new.mp3")
	_ = os.WriteFile(favOld, []byte("fav old audio"), 0644)
	_ = os.WriteFile(favNew, []byte("fav new audio"), 0644)
	_ = os.Chtimes(favOld, now.AddDate(0, 0, -210), now.AddDate(0, 0, -210))
	_ = os.Chtimes(favNew, now.AddDate(0, 0, -90), now.AddDate(0, 0, -90))

	cfgFav := config.PodcastConfig{Favorite: true}
	resFav, err := ApplyPodcastKeepPolicy(favDir, "Fav Show", cfgFav, now, false)
	if err != nil || resFav.DeletedEpisodes != 1 {
		t.Fatalf("favorite prune expected 1 deleted, got %d (err: %v)", resFav.DeletedEpisodes, err)
	}
	if util.FileExists(favOld) {
		t.Errorf("expected favOld to be deleted")
	}
	if !util.FileExists(favNew) {
		t.Errorf("expected favNew to be kept")
	}
}
