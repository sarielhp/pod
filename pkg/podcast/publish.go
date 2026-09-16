package podcast

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"pod/pkg/backend"
	"pod/pkg/podsite"
	"pod/pkg/util"
)

func siteBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("SERVER_BASE_URL")), "/")
	}
	return baseURL
}

func siteShow(sub Subscription, podDir string) podsite.Show {
	folder := sub.Folder
	if folder == "" {
		folder = filepath.Base(podDir)
	}
	coverFile := ""
	if p := findLocalCover(podDir); p != "" {
		coverFile = filepath.Base(p)
	}
	return podsite.Show{
		Title:     sub.Title,
		FeedURL:   sub.FeedURL,
		Folder:    folder,
		ImageURL:  sub.ImageURL,
		CoverFile: coverFile,
		CoverSrc:  findCoverRelativePath(podDir, sub.ImageURL),
	}
}

func siteEpisodes(podDir string, episodes []LocalEpisodeMeta) []podsite.Episode {
	out := make([]podsite.Episode, 0, len(episodes))
	for _, ep := range episodes {
		out = append(out, podsite.Episode{
			Filename:    ep.Filename,
			Title:       ep.Title,
			GUID:        ep.GUID,
			PubDate:     ep.PubDate,
			Description: ep.Description,
			DurationSec: ep.DurationSec,
			SizeBytes:   ep.SizeBytes,
			ReportHref:  episodeReportHref(podDir, ep.Filename),
		})
	}
	return out
}

func episodeReportHref(podDir, filename string) string {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	reportPath := filepath.Join(podDir, stem+".report.html")
	if _, err := os.Stat(reportPath); err != nil {
		return ""
	}
	return url.PathEscape(filepath.Base(reportPath))
}

func findCoverRelativePath(podDir, fallbackURL string) string {
	for _, name := range []string{"cover.jpg", "cover.png", "cover.jpeg"} {
		if fi, err := os.Stat(filepath.Join(podDir, name)); err == nil && !fi.IsDir() {
			return name
		}
	}
	return fallbackURL
}

func generateCatalogWebpageHTML(podcastsDir string, subs []Subscription) ([]byte, error) {
	entries := make([]podsite.CatalogEntry, 0, len(subs))
	for _, sub := range subs {
		entries = append(entries, catalogEntry(podcastsDir, sub))
	}
	_, err := os.Stat(filepath.Join(podcastsDir, "antennapod.opml"))
	return podsite.RenderCatalog(entries, podsite.CatalogOptions{HasOPML: err == nil})
}

func catalogEntry(podcastsDir string, sub Subscription) podsite.CatalogEntry {
	podDir := resolvePodcastDirForSub(sub, podcastsDir)
	folder := sub.Folder
	if folder == "" {
		folder = filepath.Base(podDir)
	}
	coverSrc := findCoverRelativePath(podDir, sub.ImageURL)
	if coverSrc != "" && !strings.HasPrefix(coverSrc, "http://") && !strings.HasPrefix(coverSrc, "https://") {
		coverSrc = url.PathEscape(folder) + "/" + url.PathEscape(coverSrc)
	}
	return podsite.CatalogEntry{
		Title:        sub.Title,
		Folder:       folder,
		CoverSrc:     coverSrc,
		EpisodeCount: len(util.FindMP3Files(podDir)),
	}
}

// PublishPodcast regenerates the static site for one podcast: feed.xml and
// index.html, written together. It is the only way these files are produced —
// previously WritePodcastFeedXML wrote the webpage as an undeclared side
// effect, so callers that wanted both wrote index.html twice.
func PublishPodcast(podDir string, sub Subscription, baseURL string, feedEpisodes []backend.FeedEpisode) error {
	if err := os.MkdirAll(podDir, 0755); err != nil {
		return err
	}
	ensurePodcastCover(podDir, &sub)

	if len(feedEpisodes) == 0 && sub.FeedURL != "" {
		if entry := defaultFeedCache().Get(sub.FeedURL); entry != nil {
			feedEpisodes = entry.FeedEpisodes()
		}
	}

	episodes := CollectLocalEpisodes(podDir, feedEpisodes)
	show := siteShow(sub, podDir)
	eps := siteEpisodes(podDir, episodes)

	feed, err := podsite.RenderFeed(show, eps, siteBaseURL(baseURL))
	if err != nil {
		return err
	}
	if err := util.WriteFileAtomic(filepath.Join(podDir, "feed.xml"), feed, 0644); err != nil {
		return err
	}

	page, err := podsite.RenderShowPage(show, eps, baseURL)
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(filepath.Join(podDir, "index.html"), page, 0644)
}

// PublishCatalog regenerates the library index page across all podcasts.
func PublishCatalog(podcastsDir string, subs []Subscription) error {
	if err := os.MkdirAll(podcastsDir, 0755); err != nil {
		return err
	}
	data, err := generateCatalogWebpageHTML(podcastsDir, subs)
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(filepath.Join(podcastsDir, "index.html"), data, 0644)
}
