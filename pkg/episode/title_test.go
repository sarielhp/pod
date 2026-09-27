package episode

import (
	"testing"
	"time"

	"pod/pkg/backend"
)

func TestParseABSEpisodePublishedAt(t *testing.T) {
	t.Parallel()
	ep1 := &backend.Episode{PublishedAt: 1724000000000}
	if got := ParseABSEpisodePublishedAt(ep1); got != 1724000000000 {
		t.Errorf("expected 1724000000000, got %d", got)
	}

	ep2 := &backend.Episode{PubDate: "Sun, 20 Aug 2026 12:00:00 GMT"}
	expectedT, _ := time.Parse(time.RFC1123, "Sun, 20 Aug 2026 12:00:00 GMT")
	if got := ParseABSEpisodePublishedAt(ep2); got != expectedT.UnixMilli() {
		t.Errorf("expected %d, got %d", expectedT.UnixMilli(), got)
	}

	ep3 := &backend.Episode{PubDate: "2026-08-20T12:00:00Z"}
	expectedISO, _ := time.Parse(time.RFC3339, "2026-08-20T12:00:00Z")
	if got := ParseABSEpisodePublishedAt(ep3); got != expectedISO.UnixMilli() {
		t.Errorf("expected %d, got %d", expectedISO.UnixMilli(), got)
	}

	if got := ParseABSEpisodePublishedAt(nil); got != 0 {
		t.Errorf("expected 0 for nil, got %d", got)
	}
}

func TestNormalizeEpisodeTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"Episode 1.mp3", "episode 1"},
		{"Episode 1 (90b50030-4e0f-4e45-af9d-6).mp3", "episode 1"},
		{"My Great Show.mp3", "my great show"},
	}
	for _, c := range cases {
		if got := NormalizeEpisodeTitle(c.in); got != c.want {
			t.Errorf("NormalizeEpisodeTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
