package types

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type AudioFileMetadata struct {
	Filename string `json:"filename,omitempty"`
	Path     string `json:"path"`
	RelPath  string `json:"relPath,omitempty"`
	Size     int64  `json:"size"`
	CTimeMs  int64  `json:"ctimeMs,omitempty"`
	MTimeMs  int64  `json:"mtimeMs,omitempty"`
}

type PodcastAudioFile struct {
	Duration      float64            `json:"duration"`
	BitRate       int                `json:"bitRate,omitempty"`
	Codec         string             `json:"codec,omitempty"`
	Channels      int                `json:"channels,omitempty"`
	ChannelLayout string             `json:"channelLayout,omitempty"`
	Format        string             `json:"format,omitempty"`
	AddedAt       int64              `json:"addedAt,omitempty"`
	Metadata      *AudioFileMetadata `json:"metadata,omitempty"`
}

type Episode struct {
	ID           string            `json:"id"`
	Index        int               `json:"index,omitempty"`
	Season       string            `json:"season,omitempty"`
	Episode      string            `json:"episode,omitempty"`
	EpisodeType  string            `json:"episodeType,omitempty"`
	Title        string            `json:"title"`
	Subtitle     string            `json:"subtitle,omitempty"`
	Description  string            `json:"description,omitempty"`
	PubDate      string            `json:"pubDate,omitempty"`
	PublishedAt  int64             `json:"publishedAt,omitempty"`
	Duration     float64           `json:"duration,omitempty"`
	Size         int64             `json:"size,omitempty"`
	GUID         string            `json:"guid,omitempty"`
	EnclosureURL string            `json:"enclosureURL,omitempty"`
	AudioFile    *PodcastAudioFile `json:"audioFile,omitempty"`
}

type PodcastMetadata struct {
	Title       string   `json:"title"`
	Author      string   `json:"author,omitempty"`
	Description string   `json:"description,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Language    string   `json:"language,omitempty"`
	ReleaseDate string   `json:"releaseDate,omitempty"`
	FeedURL     string   `json:"feedUrl,omitempty"`
	ImageURL    string   `json:"imageUrl,omitempty"`
}

type PodcastMedia struct {
	ID        string          `json:"id,omitempty"`
	Metadata  PodcastMetadata `json:"metadata"`
	Episodes  []Episode       `json:"episodes,omitempty"`
	CoverPath string          `json:"coverPath,omitempty"`
}

type Podcast struct {
	ID        string       `json:"id"`
	Path      string       `json:"path,omitempty"`
	RelPath   string       `json:"relPath,omitempty"`
	MediaType string       `json:"mediaType,omitempty"`
	Media     PodcastMedia `json:"media"`
	RSSFeed   *struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	} `json:"rssFeed,omitempty"`
}

type FeedEnclosure struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

type FeedEpisode struct {
	Title            string         `json:"title"`
	Subtitle         string         `json:"subtitle,omitempty"`
	Description      string         `json:"description,omitempty"`
	DescriptionPlain string         `json:"descriptionPlain,omitempty"`
	PubDate          string         `json:"pubDate"`
	PublishedAt      int64          `json:"publishedAt"`
	EpisodeType      string         `json:"episodeType,omitempty"`
	Season           string         `json:"season,omitempty"`
	Episode          string         `json:"episode,omitempty"`
	DurationSeconds  float64        `json:"durationSeconds,omitempty"`
	GUID             string         `json:"guid,omitempty"`
	EnclosureURL     string         `json:"enclosureUrl,omitempty"`
	Enclosure        *FeedEnclosure `json:"enclosure,omitempty"`
}

func (f *FeedEpisode) UnmarshalJSON(data []byte) error {
	type Alias FeedEpisode
	aux := &struct {
		PublishedAt  interface{} `json:"publishedAt"`
		Season       interface{} `json:"season"`
		Episode      interface{} `json:"episode"`
		EnclosureURL string      `json:"enclosureUrl"`
		*Alias
	}{
		Alias: (*Alias)(f),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.EnclosureURL != "" && f.Enclosure == nil {
		f.Enclosure = &FeedEnclosure{URL: aux.EnclosureURL}
	}
	if f.Enclosure != nil && f.Enclosure.URL != "" && f.EnclosureURL == "" {
		f.EnclosureURL = f.Enclosure.URL
	}

	if aux.PublishedAt != nil {
		switch v := aux.PublishedAt.(type) {
		case float64:
			f.PublishedAt = int64(v)
		case int64:
			f.PublishedAt = v
		case string:
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				f.PublishedAt = n
			}
		}
	}

	if aux.Season != nil {
		if m, ok := aux.Season.(map[string]interface{}); ok {
			if n, ok := m["number"]; ok && n != nil {
				f.Season = fmt.Sprintf("%v", n)
			}
		} else {
			f.Season = fmt.Sprintf("%v", aux.Season)
		}
	}

	if aux.Episode != nil {
		f.Episode = fmt.Sprintf("%v", aux.Episode)
	}

	return nil
}
