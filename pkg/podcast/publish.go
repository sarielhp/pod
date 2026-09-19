package podcast

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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
			PublishedAt: ep.PublishedAt,
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

// appendRemoteEpisodes adds the feed's episodes that are not held locally,
// pointing at their original audio.
//
// Without this a show with nothing downloaded publishes an empty feed, and a
// show with three downloads publishes three episodes — so subscribing to the
// local feed gave you strictly less than subscribing upstream. The local feed
// is meant to be a superset: ad-free audio served from here where it exists,
// the original everywhere else.
func appendRemoteEpisodes(local []podsite.Episode, localMeta []LocalEpisodeMeta, feedEpisodes []backend.FeedEpisode) []podsite.Episode {
	if len(feedEpisodes) == 0 {
		return local
	}

	held := make(map[string]bool, len(localMeta)*2)
	for _, ep := range localMeta {
		if ep.GUID != "" {
			held[ep.GUID] = true
		}
		held[remoteEpisodeKey(ep.Title)] = true
	}

	out := local
	for _, fe := range feedEpisodes {
		url := episodeEnclosureURL(fe)
		if url == "" || fe.Title == "" {
			continue
		}
		pubMs := GetPubMS(fe)
		if (fe.GUID != "" && held[fe.GUID]) || held[remoteEpisodeKey(fe.Title)] {
			continue
		}
		held[remoteEpisodeKey(fe.Title)] = true

		guid := fe.GUID
		if guid == "" {
			guid = "pod:remote:" + remoteEpisodeKey(fe.Title)
		}
		out = append(out, podsite.Episode{
			PublishedAt: pubMs,
			Title:       fe.Title,
			GUID:        guid,
			PubDate:     time.UnixMilli(pubMs).UTC().Format(time.RFC1123Z),
			Description: remoteDescription(fe),
			DurationSec: fe.DurationSeconds,
			RemoteURL:   url,
		})
	}

	// Newest first, by instant rather than by the formatted string: RSS dates
	// begin with a weekday and carry a textual month, so comparing them as
	// text interleaves the years and leaves a client no way to find the most
	// recent episode.
	sort.SliceStable(out, func(i, j int) bool { return out[i].PublishedAt > out[j].PublishedAt })
	return out
}

// remoteDescription is the notes to publish for an episode held upstream,
// falling back to the title when the cache has none — which is the case for
// records written before descriptions were retained.
func remoteDescription(fe backend.FeedEpisode) string {
	if d := strings.TrimSpace(fe.DescriptionPlain); d != "" {
		return d
	}
	if d := strings.TrimSpace(fe.Description); d != "" {
		return d
	}
	return fe.Title
}

// episodeDatePrefix matches the publication date pod prefixes to a filename.
var episodeDatePrefix = regexp.MustCompile(`^\d{4}[-_]\d{2}[-_]\d{2}[-_ ]*`)

// remoteEpisodeKey identifies an episode across the local and upstream views.
//
// The two share a title but not a filename, and a local episode whose title
// could not be matched to the feed carries a title derived from its filename
// instead — date-prefixed, punctuation replaced. Both sides are stripped of a
// leading date and reduced to letters and digits so those still meet, or the
// same episode is published twice, once from each side.
func remoteEpisodeKey(title string) string {
	title = episodeDatePrefix.ReplaceAllString(strings.TrimSpace(title), "")
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
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
	eps := appendRemoteEpisodes(siteEpisodes(podDir, episodes), episodes, feedEpisodes)

	feed, err := podsite.RenderFeed(show, eps, siteBaseURL(baseURL))
	if err != nil {
		return err
	}
	if _, err := util.WriteFileAtomicIfChanged(filepath.Join(podDir, "feed.xml"), feed, 0644); err != nil {
		return err
	}

	page, err := podsite.RenderShowPage(show, eps, baseURL)
	if err != nil {
		return err
	}
	_, err = util.WriteFileAtomicIfChanged(filepath.Join(podDir, "index.html"), page, 0644)
	return err
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
	_, err = util.WriteFileAtomicIfChanged(filepath.Join(podcastsDir, "index.html"), data, 0644)
	return err
}
