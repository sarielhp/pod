package podcast

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"pod/pkg/backend"
)

func TestMatchLocalPodcastsUniqueName(t *testing.T) {
	t.Parallel()
	entries := []PodcastDirEntry{
		{ShortID: "p01", Title: "Hardcore History", FolderName: "hardcore_history", Dir: "/podcasts/hh"},
		{ShortID: "p02", Title: "Lex Fridman Podcast", FolderName: "lex_fridman", Dir: "/podcasts/lex"},
	}

	p, err := MatchLocalPodcasts(entries, "fridman")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ShortID != "p02" {
		t.Errorf("expected p02, got %s", p.ShortID)
	}
}

func TestMatchLocalPodcastsAmbiguous(t *testing.T) {
	t.Parallel()
	entries := []PodcastDirEntry{
		{ShortID: "p01", Title: "The Daily News", Dir: "/podcasts/p1"},
		{ShortID: "p02", Title: "The Daily Show", Dir: "/podcasts/p2"},
		{ShortID: "p03", Title: "Daily Tech Update", Dir: "/podcasts/p3"},
	}

	p, err := MatchLocalPodcasts(entries, "daily")
	if p != nil {
		t.Fatalf("expected nil podcast on ambiguous match, got %+v", p)
	}
	if !errors.Is(err, ErrAmbiguousPodcast) {
		t.Fatalf("expected ErrAmbiguousPodcast, got %v", err)
	}

	formatted := err.Error()
	expectedLines := []string{
		"p01 | The Daily News",
		"p02 | The Daily Show",
		"p03 | Daily Tech Update",
	}
	for _, l := range expectedLines {
		if !strings.Contains(formatted, l) {
			t.Errorf("expected line %q in formatted output:\n%s", l, formatted)
		}
	}
	if strings.Contains(formatted, "...") {
		t.Errorf("did not expect '...' for 3 items, got:\n%s", formatted)
	}
}

func TestMatchLocalPodcastsAmbiguousMoreThanFive(t *testing.T) {
	t.Parallel()
	var entries []PodcastDirEntry
	for i := 1; i <= 7; i++ {
		entries = append(entries, PodcastDirEntry{
			ShortID: fmt.Sprintf("p%02d", i),
			Title:   fmt.Sprintf("Podcast Show %d", i),
			Dir:     fmt.Sprintf("/podcasts/p%02d", i),
		})
	}

	p, err := MatchLocalPodcasts(entries, "podcast show")
	if p != nil {
		t.Fatalf("expected nil podcast on ambiguous match, got %+v", p)
	}
	if !errors.Is(err, ErrAmbiguousPodcast) {
		t.Fatalf("expected ErrAmbiguousPodcast, got %v", err)
	}

	formatted := err.Error()
	lines := strings.Split(strings.TrimSpace(formatted), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines (5 items + '...'), got %d lines:\n%s", len(lines), formatted)
	}
	if lines[5] != "..." {
		t.Errorf("expected 6th line to be '...', got %q", lines[5])
	}
	if lines[0] != "p01 | Podcast Show 1" {
		t.Errorf("expected first line 'p01 | Podcast Show 1', got %q", lines[0])
	}
}

func TestMatchLocalPodcastsFallbackToID(t *testing.T) {
	t.Parallel()
	entries := []PodcastDirEntry{
		{ShortID: "p01", Title: "Hardcore History", Dir: "/podcasts/hh"},
		{ShortID: "p02", Title: "Lex Fridman Podcast", Dir: "/podcasts/lex"},
	}

	p, err := MatchLocalPodcasts(entries, "p01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ShortID != "p01" {
		t.Errorf("expected p01, got %s", p.ShortID)
	}
}

func TestMatchBackendPodcastsAmbiguous(t *testing.T) {
	t.Parallel()
	podcasts := []backend.Podcast{
		{ID: "101", Media: backend.PodcastMedia{Metadata: backend.PodcastMetadata{Title: "News Daily"}}},
		{ID: "102", Media: backend.PodcastMedia{Metadata: backend.PodcastMetadata{Title: "Science Daily"}}},
	}

	p, err := MatchBackendPodcasts(podcasts, "daily")
	if p != nil {
		t.Fatalf("expected nil podcast, got %+v", p)
	}
	if !errors.Is(err, ErrAmbiguousPodcast) {
		t.Fatalf("expected ErrAmbiguousPodcast, got %v", err)
	}

	formatted := err.Error()
	expected := "101 | News Daily\n102 | Science Daily"
	if formatted != expected {
		t.Errorf("expected %q, got %q", expected, formatted)
	}
}

func TestMatchSubscription(t *testing.T) {
	t.Parallel()
	subs := []Subscription{
		{ID: "sub1", Title: "Hardcore History", Folder: "hh"},
		{ID: "sub2", Title: "Lex Fridman Podcast", Folder: "lex"},
	}

	// Match by title substring
	s, err := MatchSubscription(subs, "fridman")
	if err != nil || s == nil || s.ID != "sub2" {
		t.Fatalf("expected sub2, got %+v (err: %v)", s, err)
	}

	// Match by ID
	s, err = MatchSubscription(subs, "sub1")
	if err != nil || s == nil || s.ID != "sub1" {
		t.Fatalf("expected sub1, got %+v (err: %v)", s, err)
	}

	// Match by folder
	s, err = MatchSubscription(subs, "hh")
	if err != nil || s == nil || s.ID != "sub1" {
		t.Fatalf("expected sub1 by folder, got %+v (err: %v)", s, err)
	}
}
