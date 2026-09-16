package podcast

import (
	"testing"

	"pod/pkg/backend"
)

func TestMergePubDatesKeepsFetchedEpisodes(t *testing.T) {
	t.Parallel()
	// The defect this exists to prevent: a fetch carried the old history
	// across unchanged and never added what it had just read, so the
	// catalogue froze at whenever it was first written and an episode that
	// was never downloaded was known to no part of pod.
	existing := []FeedCachePubDate{{Title: "old", PublishedAt: 1000}}
	fetched := []backend.FeedEpisode{
		{Title: "new", PublishedAt: 3000},
		{Title: "middle", PublishedAt: 2000},
	}
	got := mergePubDates(existing, fetched, 0)
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(got), got)
	}
	if got[0].Title != "new" || got[2].Title != "old" {
		t.Errorf("not sorted newest first: %+v", got)
	}
}

func TestMergePubDatesDeduplicates(t *testing.T) {
	t.Parallel()
	// Feeds are re-read constantly; the history must not grow a copy per check.
	existing := []FeedCachePubDate{{Title: "ep", PublishedAt: 1000}}
	fetched := []backend.FeedEpisode{{Title: "ep", PublishedAt: 1000}}
	if got := mergePubDates(existing, fetched, 0); len(got) != 1 {
		t.Errorf("got %d entries, want 1: %+v", len(got), got)
	}
}

func TestMergePubDatesPrefersTheCopyWithATitle(t *testing.T) {
	t.Parallel()
	existing := []FeedCachePubDate{{PublishedAt: 1000}}
	fetched := []backend.FeedEpisode{{Title: "named", PublishedAt: 1000}}
	got := mergePubDates(existing, fetched, 0)
	if len(got) != 1 || got[0].Title != "named" {
		t.Errorf("got %+v, want one titled entry", got)
	}
}

func TestMergePubDatesKeepsTheNewestWhenCapped(t *testing.T) {
	t.Parallel()
	var fetched []backend.FeedEpisode
	for i := 1; i <= 10; i++ {
		fetched = append(fetched, backend.FeedEpisode{Title: "e", PublishedAt: int64(i) * 100})
	}
	got := mergePubDates(nil, fetched, 3)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	// A "what is new" listing wants the newest, so the cap must drop the tail.
	if got[0].PublishedAt != 1000 || got[2].PublishedAt != 800 {
		t.Errorf("capped the wrong end: %+v", got)
	}
}

func TestMergePubDatesIgnoresEmptyEntries(t *testing.T) {
	t.Parallel()
	got := mergePubDates(nil, []backend.FeedEpisode{{}, {Title: "real", PublishedAt: 5}}, 0)
	if len(got) != 1 || got[0].Title != "real" {
		t.Errorf("got %+v", got)
	}
}
