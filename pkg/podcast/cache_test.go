package podcast

import (
	"os"
	"path/filepath"
	"pod/pkg/episode"
	"pod/pkg/types"
	"testing"
	"time"
)

func TestParseAnyPublicationTime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  time.Time
	}{
		{
			input: "2026-09-01T12:35:39Z",
			want:  time.Date(2026, 9, 1, 12, 35, 39, 0, time.UTC),
		},
		{
			input: "Tue, 01 Sep 2026 12:35:39 +0000",
			want:  time.Date(2026, 9, 1, 12, 35, 39, 0, time.UTC),
		},
		{
			input: "Tue, 01 Sep 2026 12:35:39 GMT",
			want:  time.Date(2026, 9, 1, 12, 35, 39, 0, time.UTC),
		},
		{
			input: "2026-09-01 12:35:39",
			want:  time.Date(2026, 9, 1, 12, 35, 39, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		got, err := ParseAnyPublicationTime(tc.input)
		if err != nil {
			t.Errorf("ParseAnyPublicationTime(%q) failed: %v", tc.input, err)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("ParseAnyPublicationTime(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestGetEpisodePublicationTimeFromFeedXML(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "ShowA")
	if err := os.MkdirAll(podDir, 0755); err != nil {
		t.Fatal(err)
	}

	mp3Path := filepath.Join(podDir, "Episode 1.mp3")
	if err := os.WriteFile(mp3Path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	feedXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>ShowA</title>
    <item>
      <title>Episode 1</title>
      <pubDate>Tue, 01 Sep 2026 12:35:39 +0000</pubDate>
      <guid>ep1</guid>
    </item>
  </channel>
</rss>`
	if err := os.WriteFile(filepath.Join(podDir, "feed.xml"), []byte(feedXML), 0644); err != nil {
		t.Fatal(err)
	}

	pubTime := GetEpisodePublicationTime(mp3Path)
	want := time.Date(2026, 9, 1, 12, 35, 39, 0, time.UTC)
	if !pubTime.Equal(want) {
		t.Fatalf("GetEpisodePublicationTime(%s) = %v, want %v", mp3Path, pubTime, want)
	}
}

func TestGetEpisodePublicationTimeFromFeedSourceStatus(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "ShowB")
	if err := os.MkdirAll(podDir, 0755); err != nil {
		t.Fatal(err)
	}

	mp3Path := filepath.Join(podDir, "Episode 2.mp3")
	if err := os.WriteFile(mp3Path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	st := &types.EpisodeStatusFile{
		Status:            types.StateDownloaded,
		PublishedAt:       "2026-09-02T15:04:05Z",
		PublicationSource: "feed",
	}
	if err := episode.SaveEpisodeStatus(episode.StatusPathFor(mp3Path), st); err != nil {
		t.Fatal(err)
	}

	pubTime := GetEpisodePublicationTime(mp3Path)
	want := time.Date(2026, 9, 2, 15, 4, 5, 0, time.UTC)
	if !pubTime.Equal(want) {
		t.Fatalf("GetEpisodePublicationTime(%s) = %v, want %v", mp3Path, pubTime, want)
	}
}
