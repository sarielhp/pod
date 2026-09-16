package podcast

import (
	"testing"

	"pod/pkg/backend"
	"pod/pkg/podsite"
)

func TestAppendRemoteEpisodesPublishesTheWholeRun(t *testing.T) {
	t.Parallel()
	// A show with nothing downloaded used to publish an empty feed, so
	// subscribing locally gave strictly less than subscribing upstream.
	feed := []backend.FeedEpisode{
		{Title: "Episode One", PublishedAt: 1000, GUID: "g1", EnclosureURL: "https://cdn/1.mp3"},
		{Title: "Episode Two", PublishedAt: 2000, GUID: "g2", EnclosureURL: "https://cdn/2.mp3"},
	}
	got := appendRemoteEpisodes(nil, nil, feed)
	if len(got) != 2 {
		t.Fatalf("got %d episodes, want 2", len(got))
	}
	for _, e := range got {
		if e.Local() {
			t.Errorf("episode %q claimed to be local", e.Title)
		}
		if e.RemoteURL == "" {
			t.Errorf("episode %q has no audio URL", e.Title)
		}
	}
}

func TestAppendRemoteEpisodesPrefersTheLocalCopy(t *testing.T) {
	t.Parallel()
	// A downloaded episode is served from here, ad-free; it must not also be
	// published pointing upstream.
	local := []podsite.Episode{{Title: "Episode One", Filename: "2026-01-01_Episode_One.mp3"}}
	meta := []LocalEpisodeMeta{{
		EpisodeFile: EpisodeFile{Title: "Episode One", PublishedAt: 1000},
		GUID:        "g1",
	}}
	feed := []backend.FeedEpisode{
		{Title: "Episode One", PublishedAt: 1000, GUID: "g1", EnclosureURL: "https://cdn/1.mp3"},
		{Title: "Episode Two", PublishedAt: 2000, GUID: "g2", EnclosureURL: "https://cdn/2.mp3"},
	}
	got := appendRemoteEpisodes(local, meta, feed)
	if len(got) != 2 {
		t.Fatalf("got %d episodes, want 2 (one local, one remote)", len(got))
	}
	for _, e := range got {
		if e.Title == "Episode One" && !e.Local() {
			t.Error("the downloaded episode was published pointing upstream")
		}
	}
}

func TestAppendRemoteEpisodesMatchesAFilenameDerivedTitle(t *testing.T) {
	t.Parallel()
	// A local episode whose title could not be matched to the feed carries a
	// title derived from its filename. It must still dedupe, or the episode
	// is published twice — once from each side.
	meta := []LocalEpisodeMeta{{
		EpisodeFile: EpisodeFile{Title: "2026-01-01_Episode_One", PublishedAt: 1000},
	}}
	local := []podsite.Episode{{Title: "2026-01-01_Episode_One"}}
	feed := []backend.FeedEpisode{
		{Title: "Episode One", PublishedAt: 1000, GUID: "g1", EnclosureURL: "https://cdn/1.mp3"},
	}
	if got := appendRemoteEpisodes(local, meta, feed); len(got) != 1 {
		t.Errorf("episode published twice: %+v", got)
	}
}

func TestAppendRemoteEpisodesSkipsEntriesWithNoAudio(t *testing.T) {
	t.Parallel()
	// The retained history predates enclosure URLs being kept, so older
	// entries have none. Publishing them would produce items nothing can play.
	feed := []backend.FeedEpisode{
		{Title: "No URL", PublishedAt: 1000},
		{Title: "Has URL", PublishedAt: 2000, EnclosureURL: "https://cdn/2.mp3"},
	}
	got := appendRemoteEpisodes(nil, nil, feed)
	if len(got) != 1 || got[0].Title != "Has URL" {
		t.Errorf("got %+v, want only the playable episode", got)
	}
}
