package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

// catalogEpisode is one episode a feed has published, downloaded or not.
//
// `pod info latest` used to list MP3 files on disk ordered by modification
// time, which answers "what did I fetch recently" rather than "what has been
// published". With most podcasts set to download nothing automatically, the
// episodes worth seeing are exactly the ones that listing could not show.
type catalogEpisode struct {
	PodcastTitle   string
	PodcastShortID string
	Title          string
	PublishedAt    time.Time
	Downloaded     bool

	// Item is set only for episodes present on disk, and carries the local
	// detail — size, cut state, transcript — that a catalogue entry has no
	// way to know.
	Item *lsEpisodeItem
}

// collectCatalogEpisodes merges what the feeds have published with what is on
// disk, newest first.
//
// Matching is by title, because that is all the publication history retains
// and all a downloaded file's name preserves. A miss costs a duplicate row
// rather than a wrong one.
func collectCatalogEpisodes(lib *podcast.Library, onDisk []lsEpisodeItem, includeHourly bool) []catalogEpisode {
	downloaded := make(map[string]*lsEpisodeItem, len(onDisk))
	for i := range onDisk {
		downloaded[catalogKey(onDisk[i].podcastShortID, onDisk[i].episodeName)] = &onDisk[i]
	}

	var out []catalogEpisode
	seen := make(map[string]bool, len(onDisk))

	// With no library there is no catalogue to merge, and the listing falls
	// back to what is on disk rather than failing.
	podcasts := []podcast.PodcastDirEntry(nil)
	feeds := map[string]string{}
	if lib != nil {
		podcasts = lib.Podcasts()
		feeds = feedURLsByPodcast(lib)
	}

	hidden := map[string]bool{}
	for _, p := range podcasts {
		if !includeHourly && isHourlyPodcast(p.Dir) {
			hidden[p.ShortID] = true
			continue
		}
		for _, ep := range cachedFeedEpisodes(lib, feeds[p.Dir]) {
			if ep.Title == "" || ep.PublishedAt <= 0 {
				continue
			}
			key := catalogKey(p.ShortID, ep.Title)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, catalogEpisode{
				PodcastTitle:   p.Title,
				PodcastShortID: p.ShortID,
				Title:          ep.Title,
				PublishedAt:    time.UnixMilli(ep.PublishedAt),
				Downloaded:     downloaded[key] != nil,
				Item:           downloaded[key],
			})
		}
	}

	// Anything on disk the catalogue does not mention still belongs in the
	// listing: the history is capped, and older downloads fall off it.
	for i := range onDisk {
		if hidden[onDisk[i].podcastShortID] {
			continue
		}
		key := catalogKey(onDisk[i].podcastShortID, onDisk[i].episodeName)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, catalogEpisode{
			PodcastTitle:   onDisk[i].podcastTitle,
			PodcastShortID: onDisk[i].podcastShortID,
			Title:          onDisk[i].episodeName,
			PublishedAt:    onDisk[i].modTime,
			Downloaded:     true,
			Item:           &onDisk[i],
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt.After(out[j].PublishedAt) })
	return out
}

// isHourlyPodcast reports a show that publishes roughly every hour.
//
// A rolling news bulletin puts out over a hundred episodes a week — one here
// runs at 169 — so a listing of the most recent episodes across the library is
// simply a list of that one show. They are excluded by default and restored by
// --hourly, which is the same judgement `pod server disable-hourly` already
// makes about downloading them.
func isHourlyPodcast(podDir string) bool {
	cfg := config.LoadPodcastConfig(podDir, config.PodcastConfig{})
	return cfg.Frequency != nil && cfg.Frequency.Type == string(backend.CadenceHourly)
}

// cachedFeedEpisodes is the publication history retained for a podcast's feed.
//
// The feed URL lives on the subscription rather than on the directory entry,
// so the two are matched by title — the same basis the rest of the listing
// uses, and the only one both records share.
func cachedFeedEpisodes(lib *podcast.Library, feedURL string) []backend.FeedEpisode {
	if lib == nil || feedURL == "" {
		return nil
	}
	entry := lib.FeedCache().Get(feedURL)
	if entry == nil {
		return nil
	}
	return entry.FeedEpisodes()
}

// feedURLsByPodcast maps each podcast directory to its feed.
//
// The feed URL lives on the subscription and the directory entry has none, so
// the two are matched by folder name and then by title — the only fields both
// records share. Built once rather than per podcast, since the store is read
// from disk.
func feedURLsByPodcast(lib *podcast.Library) map[string]string {
	out := map[string]string{}
	store, err := lib.Subscriptions()
	if err != nil || store == nil {
		return out
	}
	subs := store.List()
	for _, p := range lib.Podcasts() {
		for _, sub := range subs {
			if sub.Folder == p.FolderName || strings.EqualFold(sub.Title, p.Title) {
				out[p.Dir] = sub.FeedURL
				break
			}
		}
	}
	return out
}

// datePrefix matches the publication date pod prefixes to a downloaded file.
var datePrefix = regexp.MustCompile(`^\d{4}[-_]\d{2}[-_]\d{2}[-_ ]*`)

// catalogKey identifies an episode within a podcast.
//
// Titles reach us from a feed and from a filename derived from one, so both
// sides are reduced to letters and digits. Digits are kept, because episode
// numbers distinguish otherwise identical titles — but the date pod prefixes
// to a downloaded filename is stripped first, or every downloaded episode
// fails to match its own catalogue entry and is listed twice.
func catalogKey(podcastID, title string) string {
	title = datePrefix.ReplaceAllString(strings.TrimSpace(title), "")
	var b strings.Builder
	b.WriteString(podcastID)
	b.WriteByte(0)
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// listCatalogEpisodes prints the newest episodes the feeds have published,
// whether or not they were downloaded.
func listCatalogEpisodes(podcastsDir string, onDisk []lsEpisodeItem, limit int, cli CLIOptions) error {
	lib := library(Config{PodcastsDir: podcastsDir}, cli, nil)
	episodes := collectCatalogEpisodes(lib, onDisk, cli.IncludeHourly)
	if len(episodes) == 0 {
		fmt.Fprintln(progressFor(cli), "No episodes known. Run `pod server feeds` to read the feeds.")
		return nil
	}
	if limit > len(episodes) {
		limit = len(episodes)
	}
	episodes = episodes[:limit]

	if cli.JSON {
		return outputCatalogJSON(outFor(cli), episodes)
	}
	if cli.Quiet {
		for _, e := range episodes {
			fmt.Fprintln(outFor(cli), e.Title)
		}
		return nil
	}
	printCatalogTable(outFor(cli), episodes)
	return nil
}

// catalogRow is the JSON shape of one listed episode.
type catalogRow struct {
	Podcast    string `json:"podcast"`
	PodcastID  string `json:"podcast_id"`
	Episode    string `json:"episode"`
	EpisodeID  string `json:"episode_id,omitempty"`
	Published  string `json:"published"`
	Downloaded bool   `json:"downloaded"`
	Status     string `json:"status,omitempty"`
}

func outputCatalogJSON(w io.Writer, episodes []catalogEpisode) error {
	rows := make([]catalogRow, 0, len(episodes))
	for _, e := range episodes {
		row := catalogRow{
			Podcast:    e.PodcastTitle,
			PodcastID:  e.PodcastShortID,
			Episode:    e.Title,
			Published:  e.PublishedAt.Format("2006-01-02"),
			Downloaded: e.Downloaded,
		}
		if e.Item != nil {
			row.EpisodeID = e.Item.episodeShortID
			row.Status = e.Item.statusStr
		}
		rows = append(rows, row)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func printCatalogTable(w io.Writer, episodes []catalogEpisode) {
	fmt.Fprintf(w, "\nLatest %d Episodes Across All Podcasts:\n\n", len(episodes))
	for _, e := range episodes {
		mark := "  ·"
		if e.Downloaded {
			mark = util.BoldGreen("  ✓")
		}
		id := "     "
		status := ""
		if e.Item != nil {
			id = e.Item.episodeShortID
			status = e.Item.statusStr
		}
		fmt.Fprintf(w, "%s %s  %-6s %-22s %s\n",
			mark,
			e.PublishedAt.Format("2006-01-02"),
			id,
			util.TruncateDisplayName(e.PodcastTitle, 22),
			util.TruncateDisplayName(e.Title, 58),
		)
		if status != "" {
			fmt.Fprintf(w, "        %s\n", status)
		}
	}
	fmt.Fprintf(w, "\n  ✓ downloaded   · not downloaded\n")
}
