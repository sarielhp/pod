package podcast

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/backend"
	"pod/pkg/config"
)

func TestParsePodcastGroupKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		wantKind PodcastGroupKind
		wantOK   bool
	}{
		{"all", GroupKindAll, true},
		{"*", GroupKindAll, true},
		{"fav", GroupKindFavorites, true},
		{"favorite", GroupKindFavorites, true},
		{"favorites", GroupKindFavorites, true},
		{"not-fav", GroupKindNonFavorites, true},
		{"non-fav", GroupKindNonFavorites, true},
		{"non-favorites", GroupKindNonFavorites, true},
		{"unfav", GroupKindNonFavorites, true},
		{"other", GroupKindNone, false},
		{"", GroupKindNone, false},
	}

	for _, tc := range cases {
		gotKind, gotOK := parsePodcastGroupKind(tc.input)
		if gotKind != tc.wantKind || gotOK != tc.wantOK {
			t.Errorf("parsePodcastGroupKind(%q) = (%v, %v), want (%v, %v)",
				tc.input, gotKind, gotOK, tc.wantKind, tc.wantOK)
		}
	}
}

func TestResolvePodcastGroup(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	pod1 := filepath.Join(tempDir, "regular_show")
	pod2 := filepath.Join(tempDir, "favorite_show")
	_ = os.MkdirAll(pod1, 0755)
	_ = os.MkdirAll(pod2, 0755)

	c1 := config.DefaultPodcastConfig(nil)
	_ = config.SavePodcastConfig(pod1, c1)

	c2 := config.DefaultPodcastConfig(nil)
	c2.Favorite = true
	_ = config.SavePodcastConfig(pod2, c2)

	// Group: all
	gAll, err := ResolvePodcastGroup(tempDir, "all")
	if err != nil || gAll.Kind != GroupKindAll || len(gAll.Entries) != 2 {
		t.Fatalf("ResolvePodcastGroup(all) failed: %+v, err=%v", gAll, err)
	}

	// Group: fav
	gFav, err := ResolvePodcastGroup(tempDir, "fav")
	if err != nil || gFav.Kind != GroupKindFavorites || len(gFav.Entries) != 1 || gFav.Entries[0].Title != "favorite_show" {
		t.Fatalf("ResolvePodcastGroup(fav) failed: %+v, err=%v", gFav, err)
	}

	// Group: not-fav
	gNotFav, err := ResolvePodcastGroup(tempDir, "not-fav")
	if err != nil || gNotFav.Kind != GroupKindNonFavorites || len(gNotFav.Entries) != 1 || gNotFav.Entries[0].Title != "regular_show" {
		t.Fatalf("ResolvePodcastGroup(not-fav) failed: %+v, err=%v", gNotFav, err)
	}

	// Single: by phrase
	gSingle, err := ResolvePodcastGroup(tempDir, "regular")
	if err != nil || gSingle.Kind != GroupKindSingle || len(gSingle.Entries) != 1 || gSingle.Entries[0].Title != "regular_show" {
		t.Fatalf("ResolvePodcastGroup(regular) failed: %+v, err=%v", gSingle, err)
	}

	// Ambiguous match
	_, errAmb := ResolvePodcastGroup(tempDir, "show")
	if errAmb == nil || !errors.Is(errAmb, ErrAmbiguousPodcast) {
		t.Fatalf("expected ErrAmbiguousPodcast for 'show', got: %v", errAmb)
	}
}

func TestResolveBackendPodcastGroup(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	pod1 := filepath.Join(tempDir, "ShowA")
	pod2 := filepath.Join(tempDir, "ShowB")
	_ = os.MkdirAll(pod1, 0755)
	_ = os.MkdirAll(pod2, 0755)

	c1 := config.DefaultPodcastConfig(nil)
	_ = config.SavePodcastConfig(pod1, c1)

	c2 := config.DefaultPodcastConfig(nil)
	c2.Favorite = true
	_ = config.SavePodcastConfig(pod2, c2)

	podcasts := []backend.Podcast{
		{ID: "p1", Path: pod1, Media: backend.PodcastMedia{Metadata: backend.PodcastMetadata{Title: "Show A"}}},
		{ID: "p2", Path: pod2, Media: backend.PodcastMedia{Metadata: backend.PodcastMetadata{Title: "Show B"}}},
	}

	gFav, err := resolveBackendPodcastGroup(podcasts, tempDir, "favorites")
	if err != nil || len(gFav.Podcasts) != 1 || gFav.Podcasts[0].ID != "p2" {
		t.Fatalf("resolveBackendPodcastGroup(favorites) failed: %+v, err=%v", gFav, err)
	}

	gNotFav, err := resolveBackendPodcastGroup(podcasts, tempDir, "not-fav")
	if err != nil || len(gNotFav.Podcasts) != 1 || gNotFav.Podcasts[0].ID != "p1" {
		t.Fatalf("resolveBackendPodcastGroup(not-fav) failed: %+v, err=%v", gNotFav, err)
	}

	gSingle, err := resolveBackendPodcastGroup(podcasts, tempDir, "show a")
	if err != nil || len(gSingle.Podcasts) != 1 || gSingle.Podcasts[0].ID != "p1" {
		t.Fatalf("resolveBackendPodcastGroup(show a) failed: %+v, err=%v", gSingle, err)
	}
}
