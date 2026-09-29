package podcast

import (
	"errors"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/episode"
)

func identifyFixture(t *testing.T) (PodcastDirEntry, string, string) {
	t.Helper()
	dir := t.TempDir()
	inFeed := writeAudio(t, dir, FormatEpisodeFilename(pub, "", "In The Feed"))
	gone := writeAudio(t, dir, FormatEpisodeFilename(pub.AddDate(0, 0, -400), "", "Long Gone From The Feed"))
	return PodcastDirEntry{Dir: dir, Title: "Show", ShortID: "shw01"}, inFeed, gone
}

func fakeFeed(eps ...backend.FeedEpisode) FeedFetcher {
	return func(string) ([]backend.FeedEpisode, error) { return eps, nil }
}

func TestIdentifyRecordsFeedMatchesAndGivesTheRestALocalIdentity(t *testing.T) {
	t.Parallel()
	entry, inFeed, gone := identifyFixture(t)
	fe := feedEpisode("guid-feed", "In The Feed", pub)

	res := IdentifyEpisodes(entry, "https://example.test/feed.xml", fakeFeed(fe), false)
	if res.Total != 2 || res.FromFeed != 1 || res.Synthetic != 1 || res.Already != 0 {
		t.Fatalf("unexpected result %+v", res)
	}
	if id := episode.LoadIdentity(inFeed); id == nil || id.GUID != "guid-feed" || id.Synthetic {
		t.Errorf("the feed episode should carry the feed's GUID, got %+v", id)
	}
	id := episode.LoadIdentity(gone)
	if id == nil || !id.Synthetic || id.Title == "" || id.PublishedAt == 0 {
		t.Errorf("an episode the feed lacks needs a local identity with title and date, got %+v", id)
	}

	again := IdentifyEpisodes(entry, "https://example.test/feed.xml", fakeFeed(fe), false)
	if again.Already != 2 || again.FromFeed+again.Synthetic != 0 {
		t.Errorf("a second run must find nothing to do, got %+v", again)
	}
	if id2 := episode.LoadIdentity(gone); id2.GUID != id.GUID {
		t.Error("a local identity must be stable across runs")
	}
}

func TestIdentifyDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	entry, inFeed, _ := identifyFixture(t)
	res := IdentifyEpisodes(entry, "u", fakeFeed(feedEpisode("g", "In The Feed", pub)), true)
	if res.FromFeed != 1 || res.Synthetic != 1 {
		t.Fatalf("a dry run still reports the counts: %+v", res)
	}
	if episode.LoadIdentity(inFeed) != nil {
		t.Error("a dry run must not record anything")
	}
}

func TestIdentifyWithAnUnreachableFeedStillIdentifiesEverything(t *testing.T) {
	t.Parallel()
	entry, inFeed, gone := identifyFixture(t)
	res := IdentifyEpisodes(entry, "u", func(string) ([]backend.FeedEpisode, error) { return nil, errors.New("offline") }, false)
	if res.FeedErr == nil || res.Synthetic != 2 {
		t.Fatalf("all episodes should get local identities and the error should be reported: %+v", res)
	}
	for _, f := range []string{inFeed, gone} {
		if episode.LoadIdentity(f) == nil {
			t.Errorf("%s has no identity", f)
		}
	}
	_ = time.Now
}
