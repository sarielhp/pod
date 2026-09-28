package podcast

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/config"
	"pod/pkg/util"
)

func TestKeepPolicyDoesNotDeleteAFreshDownloadOfAnOldEpisode(t *testing.T) {
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "backcatalogue.mp3")
	if err := os.WriteFile(mp3, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	status := `{"version":1,"published_at":"2020-01-01T00:00:00Z","publication_source":"feed","status":"downloaded"}`
	if err := os.WriteFile(mp3+".json", []byte(status), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.PodcastConfig{}
	cfg.SetKeepPolicy(config.KeepPolicyMonth)
	res, err := ApplyPodcastKeepPolicy(dir, "Back Catalogue", cfg, time.Now(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !util.FileExists(mp3) {
		t.Fatal("an episode downloaded just now was deleted because its feed pubDate is old")
	}
	if res.DeletedEpisodes != 0 || res.ExpiredEpisodes != 0 {
		t.Fatalf("deleted %d expired %d, want 0 and 0", res.DeletedEpisodes, res.ExpiredEpisodes)
	}
}

func TestKeepPolicyStillExpiresAnEpisodeThatIsOldOnDisk(t *testing.T) {
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "stale.mp3")
	if err := os.WriteFile(mp3, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -45)
	if err := os.Chtimes(mp3, old, old); err != nil {
		t.Fatal(err)
	}
	status := `{"version":1,"published_at":"2020-01-01T00:00:00Z","publication_source":"feed","status":"downloaded"}`
	if err := os.WriteFile(mp3+".json", []byte(status), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.PodcastConfig{}
	cfg.SetKeepPolicy(config.KeepPolicyMonth)
	res, err := ApplyPodcastKeepPolicy(dir, "Stale", cfg, time.Now(), false)
	if err != nil {
		t.Fatal(err)
	}
	if util.FileExists(mp3) || res.DeletedEpisodes != 1 {
		t.Fatalf("an episode 45 days old on disk with an old pubDate survived (deleted=%d)", res.DeletedEpisodes)
	}
}
