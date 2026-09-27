package podcast

import (
	"encoding/json"
	"os"
	"path/filepath"
	"pod/pkg/episode"
	"testing"
	"time"
)

func TestPublicationDatesRepairAndUnknown(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	dir := filepath.Join(root, "Show")
	first := filepath.Join(dir, "first", "podcast.mp3")
	second := filepath.Join(dir, "second", "podcast.mp3")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
		data := `{"id":"existing","status":"needs_adr","published_at":"2026-09-10T13:00:00Z","extra":{"keep":true}}`
		if err := os.WriteFile(episode.StatusPathFor(path), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if !GetEpisodePublicationTime(first).IsZero() {
		t.Fatal("trusted a legacy file timestamp")
	}
	date := time.Date(2026, 9, 9, 9, 0, 0, 0, time.UTC)
	cache := &CachedPodcastIndex{Episodes: []CachedEpisodeSummary{{EpisodeFile: EpisodeFile{Path: first, PublishedAt: 1}}, {EpisodeFile: EpisodeFile{Path: second, PublishedAt: 1}}}}
	if err := SavePodcastCache(dir, cache); err != nil {
		t.Fatal(err)
	}
	dates := map[string]time.Time{first: date}
	SetPublicationSource(root, dates)
	defer SetPublicationSource("", nil)
	if !GetEpisodePublicationTime(first).Equal(date) || !GetEpisodePublicationTime(second).IsZero() {
		t.Fatal("source dates did not override local metadata")
	}
	for _, dry := range []bool{true, false} {
		statuses, caches, err := RepairPublicationDates(root, dates, dry)
		if err != nil || statuses != 2 || caches != 1 {
			t.Fatalf("repair %v: %d %d %v", dry, statuses, caches, err)
		}
		data, err := os.ReadFile(episode.StatusPathFor(first))
		if err != nil {
			t.Fatal(err)
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		if record["extra"] == nil || record["id"] == nil {
			t.Fatal("repair lost unrelated fields")
		}
		st, err := episode.LoadEpisodeStatus(episode.StatusPathFor(first))
		if err != nil {
			t.Fatal(err)
		}
		if !dry && (st.PublicationSource != "source" || st.PublishedAt != date.Format(time.RFC3339)) {
			t.Fatalf("wrong repair: %+v", st)
		}
		if dry && st.PublicationSource != "" {
			t.Fatal("dry run wrote status")
		}
	}
	SetPublicationSource("", nil)
	if !GetEpisodePublicationTime(first).Equal(date) || !GetEpisodePublicationTime(second).IsZero() {
		t.Fatal("repaired offline dates wrong")
	}
	statuses, caches, err := RepairPublicationDates(root, dates, false)
	if err != nil || statuses != 0 || caches != 0 {
		t.Fatalf("repair not idempotent: %d %d %v", statuses, caches, err)
	}
}

func TestSourceDatePersistsInNewStatuses(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "episode.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	st := episode.GetOrCreateEpisodeStatus(path)
	if st.PublishedAt != "" {
		t.Fatal("new status invented a publication date")
	}
	date := time.Date(2026, 9, 9, 9, 0, 0, 0, time.UTC)
	SetPublicationSource(root, map[string]time.Time{path: date})
	defer SetPublicationSource("", nil)
	if err := episode.SaveEpisodeStatus(episode.StatusPathFor(path), st); err != nil {
		t.Fatal(err)
	}
	stored, err := episode.LoadEpisodeStatus(episode.StatusPathFor(path))
	if err != nil || stored.PublishedAt != date.Format(time.RFC3339) || stored.PublicationSource != "source" {
		t.Fatalf("wrong source date: %+v %v", stored, err)
	}
}
