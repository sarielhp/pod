package backend

import (
	"encoding/xml"

	"pod/pkg/types"
)

type LibraryFolder struct {
	ID        string `json:"id"`
	FullPath  string `json:"fullPath"`
	LibraryID string `json:"libraryId"`
}

type Library struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	MediaType string          `json:"mediaType"`
	Folders   []LibraryFolder `json:"folders"`
}

// The domain model — a podcast, its episodes, and an upstream feed's
// episodes — lives in pkg/types. These aliases keep the names this adapter
// package has always exported, so callers need not know where they moved.
type (
	AudioFileMetadata = types.AudioFileMetadata
	PodcastAudioFile  = types.PodcastAudioFile
	Episode           = types.Episode
	PodcastMetadata   = types.PodcastMetadata
	PodcastMedia      = types.PodcastMedia
	Podcast           = types.Podcast
	FeedEnclosure     = types.FeedEnclosure
	FeedEpisode       = types.FeedEpisode
)

type ActiveDownload struct {
	ID                  string `json:"id"`
	EpisodeDisplayTitle string `json:"episodeDisplayTitle"`
	DisplayTitle        string `json:"displayTitle"`
	Title               string `json:"title"`
	EpisodeID           string `json:"episodeId"`
	URL                 string `json:"url"`
	Episode             struct {
		Title        string `json:"title"`
		GUID         string `json:"guid"`
		EnclosureURL string `json:"enclosureUrl"`
	} `json:"episode"`
}

type OPMLFeed struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	ImageURL string `json:"image_url,omitempty"`
}

type OPMLDoc struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    OPMLHead `xml:"head"`
	Body    OPMLBody `xml:"body"`
}

type OPMLHead struct {
	Title string `xml:"title"`
}

type OPMLBody struct {
	Outline OPMLOutlineGroup `xml:"outline"`
}

type OPMLOutlineGroup struct {
	Text     string        `xml:"text,attr"`
	Outlines []OPMLOutline `xml:"outline"`
}

type OPMLOutline struct {
	Type   string `xml:"type,attr"`
	Text   string `xml:"text,attr"`
	XMLURL string `xml:"xmlUrl,attr"`
}

type PodcastCadence = types.PodcastCadence

const (
	CadenceHourly       = types.CadenceHourly
	CadenceDaily        = types.CadenceDaily
	CadenceWeekly       = types.CadenceWeekly
	CadenceMonthly      = types.CadenceMonthly
	CadenceIntermittent = types.CadenceIntermittent
)

type PodcastFrequencyInfo = types.PodcastFrequencyInfo

type ScanOptions struct {
	PodcastsDir  string
	Quiet        bool
	Verbose      bool
	EpisodesOnly bool
	PodcastsOnly bool
}

type ScanResult struct {
	NewPodcasts     int
	CheckedPodcasts int
	NewEpisodes     int
	Podcasts        []Podcast
}

type RescanOptions struct {
	PodcastsDir string
	PodcastID   string
	DryRun      bool
	Verbose     bool
	Quiet       bool
}

type RescanResult struct {
	RescanCount  int
	CheckedCount int
}

type OPMLExportOptions struct {
	Quiet   bool
	Verbose bool
}

type OPMLImportOptions struct {
	Quiet   bool
	Verbose bool
}

type OPMLImportResult struct {
	Subscribed       int
	AlreadyExisted   int
	SkippedSelfFeeds int
	TotalFeeds       int
}
