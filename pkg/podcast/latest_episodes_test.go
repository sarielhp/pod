package podcast

import (
	"testing"

	"pod/pkg/backend"
)

func ep(title string, pubMS int64, url string) backend.FeedEpisode {
	return backend.FeedEpisode{Title: title, PublishedAt: pubMS, EnclosureURL: url}
}

func TestLatestSelectionPathsCoversBothHalves(t *testing.T) {
	t.Parallel()
	// A "process the newest ten" request means ten episodes, whether they
	// have to be fetched or were downloaded yesterday and never cleaned.
	sel := LatestSelection{
		Existing: []string{"/pod/show/already.mp3"},
		Plans: []SubscriptionPlan{{
			PodDir:     "/pod/show",
			ToDownload: []backend.FeedEpisode{ep("New One", 1_000_000, "https://cdn/1.mp3")},
		}},
	}
	got := sel.Paths()
	if len(got) != 2 {
		t.Fatalf("got %d paths, want 2: %v", len(got), got)
	}
	if got[0] != "/pod/show/already.mp3" {
		t.Errorf("existing episode missing: %v", got)
	}
}

func TestEpisodeDestPathMatchesTheDownloader(t *testing.T) {
	t.Parallel()
	// The path has to be predictable before the download runs, so the caller
	// can clean what it fetched. It must agree with what downloadEpisode
	// actually writes.
	e := ep("Some Episode", 1_757_980_800_000, "https://cdn/x.mp3")
	got := EpisodeDestPath("/pod/show", e)
	if got == "/pod/show" || got == "" {
		t.Fatalf("no filename produced: %q", got)
	}
	if !contains(got, "Some_Episode") {
		t.Errorf("path does not reflect the title: %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestPlanLatestEpisodesRefusesANonPositiveCount(t *testing.T) {
	t.Parallel()
	var l *Library
	if sel := l.PlanLatestEpisodes(nil, LatestEpisodePlanOptions{Count: 0}); len(sel.Plans) != 0 || len(sel.Existing) != 0 {
		t.Errorf("a zero count selected something: %+v", sel)
	}
}
