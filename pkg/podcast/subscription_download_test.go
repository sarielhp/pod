package podcast

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
)

// Moved here from pkg/cli with the logic it covers. The policy cascade is the
// part of a download run that decides what a user actually gets, so it is
// tested against the library rather than through a command's flag struct.
func TestSelectSubscriptionEpisodesRespectsPolicy(t *testing.T) {
	t.Parallel()
	podDir := filepath.Join(t.TempDir(), "Show")
	if err := os.MkdirAll(podDir, 0755); err != nil {
		t.Fatal(err)
	}

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	feedEps := []backend.FeedEpisode{
		{Title: "Ep 3", PublishedAt: t2.UnixMilli(), EnclosureURL: "http://example.com/3.mp3"},
		{Title: "Ep 2", PublishedAt: t1.UnixMilli(), EnclosureURL: "http://example.com/2.mp3"},
		{Title: "Ep 1", PublishedAt: t0.UnixMilli(), EnclosureURL: "http://example.com/1.mp3"},
	}
	opts := SubscriptionDownloadOptions{
		Defaults: config.PolicyDefaults{DownloadPolicy: "none", DownloadK: 3},
	}
	sub := Subscription{Title: "Show", Folder: "Show"}

	if res := selectSubscriptionEpisodes(podDir, feedEps, sub, opts); len(res) != 0 {
		t.Fatalf("policy none should select nothing, got %d", len(res))
	}

	sub.DownloadPolicy = "latest"
	res := selectSubscriptionEpisodes(podDir, feedEps, sub, opts)
	if len(res) != 1 || res[0].Title != "Ep 3" {
		t.Fatalf("policy latest should select Ep 3, got %v", res)
	}

	sub.DownloadPolicy = "latest_k"
	sub.DownloadK = 2
	if res := selectSubscriptionEpisodes(podDir, feedEps, sub, opts); len(res) != 2 {
		t.Fatalf("latest_k(2) should select 2, got %d", len(res))
	}

	sub.DownloadPolicy = ""
	if err := os.WriteFile(filepath.Join(podDir, "Ep 2.mp3"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.SavePodcastConfig(podDir, config.PodcastConfig{Favorite: true}); err != nil {
		t.Fatal(err)
	}
	res = selectSubscriptionEpisodes(podDir, feedEps, sub, opts)
	if len(res) != 1 || res[0].Title != "Ep 3" {
		t.Fatalf("a favourite should take only new episodes, got %v", res)
	}
}

func TestSelectSubscriptionEpisodesExplicitCountOverridesPolicy(t *testing.T) {
	t.Parallel()
	podDir := t.TempDir()
	feedEps := []backend.FeedEpisode{
		{Title: "Ep 3", PublishedAt: 3000, EnclosureURL: "http://e.com/3.mp3"},
		{Title: "Ep 2", PublishedAt: 2000, EnclosureURL: "http://e.com/2.mp3"},
		{Title: "Ep 1", PublishedAt: 1000, EnclosureURL: "http://e.com/1.mp3"},
	}
	sub := Subscription{Title: "Show", DownloadPolicy: "none"}

	opts := SubscriptionDownloadOptions{Count: 2, CountGiven: true}
	if res := selectSubscriptionEpisodes(podDir, feedEps, sub, opts); len(res) != 2 {
		t.Fatalf("an explicit count should override policy none, got %d", len(res))
	}

	opts = SubscriptionDownloadOptions{DownloadAll: true}
	if res := selectSubscriptionEpisodes(podDir, feedEps, sub, opts); len(res) != 3 {
		t.Fatalf("DownloadAll should override policy none, got %d", len(res))
	}
}

func TestSubscriptionTargets(t *testing.T) {
	t.Parallel()
	subs := []Subscription{
		{ID: "p1", Title: "Podcast 1"},
		{ID: "p2", Title: "Podcast 2", Disabled: true},
		{ID: "p3", Title: "Science Show"},
	}

	targets := SubscriptionTargets(subs, "")
	if len(targets) != 2 || targets[0].ID != "p1" || targets[1].ID != "p3" {
		t.Fatalf("an empty target should select every enabled subscription: %+v", targets)
	}
	if targets := SubscriptionTargets(subs, "p1"); len(targets) != 1 || targets[0].ID != "p1" {
		t.Fatalf("select by id: %+v", targets)
	}
	if targets := SubscriptionTargets(subs, "p2"); len(targets) != 0 {
		t.Fatalf("a disabled subscription must never be selected: %+v", targets)
	}
	if targets := SubscriptionTargets(subs, "science"); len(targets) != 1 || targets[0].ID != "p3" {
		t.Fatalf("select by title substring: %+v", targets)
	}
}

func TestShouldQueueForAdRemoval(t *testing.T) {
	t.Parallel()
	podDir := t.TempDir()
	defaults := config.PolicyDefaults{AdRemoval: "none"}

	if shouldQueueForAdRemoval(podDir, Subscription{}, defaults) {
		t.Error("ad removal none should not queue")
	}
	if !shouldQueueForAdRemoval(podDir, Subscription{AdRemoval: "all"}, defaults) {
		t.Error("the subscription's own policy should win over the default")
	}
	if err := config.SavePodcastConfig(podDir, config.PodcastConfig{Favorite: true}); err != nil {
		t.Fatal(err)
	}
	if !shouldQueueForAdRemoval(podDir, Subscription{}, defaults) {
		t.Error("a favourite should always queue")
	}
}
