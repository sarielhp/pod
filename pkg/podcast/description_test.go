package podcast

import (
	"strings"
	"testing"

	"pod/pkg/backend"
)

func TestTrimDescriptionStripsMarkupAndBudgets(t *testing.T) {
	t.Parallel()

	t.Run("markup is removed whichever field carries it", func(t *testing.T) {
		t.Parallel()
		// Feeds put HTML in both fields regardless of the name, so the plain
		// one cannot be trusted to be plain.
		got := trimDescription(backend.FeedEpisode{DescriptionPlain: "<p>Hello <b>there</b></p>"})
		if strings.ContainsAny(got, "<>") {
			t.Errorf("markup survived: %q", got)
		}
		if got != "Hello there" {
			t.Errorf("got %q, want %q", got, "Hello there")
		}
	})

	t.Run("entities are decoded", func(t *testing.T) {
		t.Parallel()
		if got := trimDescription(backend.FeedEpisode{Description: "Europe&#39;s history"}); got != "Europe's history" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("long notes are cut at a word boundary", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("word ", 400)
		got := trimDescription(backend.FeedEpisode{Description: long})
		if len(got) > FeedCacheDescLimit+8 {
			t.Errorf("exceeded the budget: %d chars", len(got))
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("truncation not marked: %q", got[len(got)-12:])
		}
		// Cutting mid-word reads as a typo rather than as an elision.
		if strings.HasSuffix(strings.TrimSuffix(got, "…"), "wor") {
			t.Error("cut mid-word")
		}
	})

	t.Run("short notes are left whole", func(t *testing.T) {
		t.Parallel()
		if got := trimDescription(backend.FeedEpisode{Description: "Brief notes."}); got != "Brief notes." {
			t.Errorf("got %q", got)
		}
	})
}

func TestPlainDescriptionCleansLegacyEntries(t *testing.T) {
	t.Parallel()
	// Entries written before descriptions were stripped keep their markup
	// until they are re-read, and enrichment will not overwrite a non-empty
	// value — so the cleaning has to happen on the way out too.
	if got := plainDescription("<p>Stored <i>with</i> markup</p>"); strings.ContainsAny(got, "<>") {
		t.Errorf("markup survived a read: %q", got)
	}
	// Text with nothing to clean is returned untouched, not rebuilt.
	plain := "Already plain text."
	if got := plainDescription(plain); got != plain {
		t.Errorf("got %q, want %q", got, plain)
	}
}

func TestRemoteDescriptionFallsBackToTheTitle(t *testing.T) {
	t.Parallel()
	// Records written before descriptions were retained have none, and an
	// item with an empty description is worse than one repeating its title.
	if got := remoteDescription(backend.FeedEpisode{Title: "Only a title"}); got != "Only a title" {
		t.Errorf("got %q", got)
	}
}
