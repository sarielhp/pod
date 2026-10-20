package podsite

import (
	"fmt"
	"html"
	"net/url"
	"sort"
	"strings"

	"pod/pkg/format"
)

// RenderShowPage produces a self-contained HTML player for one show.
func RenderShowPage(show Show, episodes []Episode, baseURL string) ([]byte, error) {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n", html.EscapeString(show.Title))
	fmt.Fprintf(&b, "<style>%s</style>\n</head>\n<body>\n<div class=\"container\">\n", siteCSS)
	b.WriteString("<a href=\"../index.html\" class=\"nav-link\">&larr; All Podcasts</a>\n")

	renderShowHeader(&b, show, len(episodes))
	b.WriteString("<main>\n")
	for _, ep := range episodes {
		renderEpisodeCard(&b, ep, baseURL, show.Folder)
	}
	b.WriteString("</main>\n</div>\n")
	b.WriteString(playerBar)
	fmt.Fprintf(&b, "<script>%s</script>\n</body>\n</html>\n", playerJS)
	return []byte(b.String()), nil
}

const playerBar = `<div id="player" hidden><div class="p-inner">
<div class="p-title"></div>
<div class="p-row">
<button type="button" class="btn p-back">-15s</button>
<button type="button" class="btn p-toggle">Play</button>
<button type="button" class="btn p-fwd">+30s</button>
<input type="range" class="p-seek" min="0" max="0" value="0" step="1">
<span class="p-time"><span class="p-cur">0:00</span><span class="p-dur-wrap"> / <span class="p-dur">0:00</span></span></span>
<button type="button" class="btn p-speed">1x</button>
</div></div></div>
`

func renderShowHeader(b *strings.Builder, show Show, count int) {
	b.WriteString("<header><div class=\"header-flex\">\n")
	if show.CoverSrc != "" {
		fmt.Fprintf(b, "<img src=\"%s\" alt=\"Cover\" class=\"cover-img\">\n", html.EscapeString(show.CoverSrc))
	}
	b.WriteString("<div class=\"header-info\">\n")
	fmt.Fprintf(b, "<h1>%s</h1>\n", html.EscapeString(show.Title))
	fmt.Fprintf(b, "<p>%d downloaded episode(s)</p>\n", count)
	b.WriteString("<a href=\"feed.xml\" class=\"btn\">RSS Feed</a>\n")
	b.WriteString("<a href=\"feed.xml\" class=\"btn\" id=\"subscribe-app\">Subscribe in AntennaPod</a>\n")
	b.WriteString("<button type=\"button\" class=\"btn\" id=\"copy-feed\">Copy feed URL</button>\n")
	if show.FeedURL != "" {
		fmt.Fprintf(b, "<a href=\"%s\" target=\"_blank\" rel=\"noopener\" class=\"btn\">Original Feed</a>\n", html.EscapeString(show.FeedURL))
	}
	b.WriteString("</div></div></header>\n")
}

func renderEpisodeCard(b *strings.Builder, ep Episode, baseURL, folder string) {
	audioSrc := episodeAudioSrc(ep, baseURL, folder)
	fmt.Fprintf(b, "<div class=\"episode-card\" data-src=\"%s\" data-title=\"%s\">\n", html.EscapeString(audioSrc), html.EscapeString(ep.Title))
	titleHTML := html.EscapeString(ep.Title)
	if ep.IsArchived {
		titleHTML += " <span class=\"badge ep-archived\">Archived</span>"
	}
	fmt.Fprintf(b, "<div class=\"episode-title\">%s</div>\n", titleHTML)

	var metaParts []string
	if ep.PubDate != "" {
		metaParts = append(metaParts, ep.PubDate)
	}
	if ep.DurationSec > 0 {
		metaParts = append(metaParts, format.FormatClock(ep.DurationSec))
	}
	if ep.SizeBytes > 0 {
		metaParts = append(metaParts, formatBytes(ep.SizeBytes))
	}
	if len(metaParts) > 0 {
		fmt.Fprintf(b, "<div class=\"episode-meta\">%s</div>\n", html.EscapeString(strings.Join(metaParts, " • ")))
	}

	if ep.Description != "" {
		fmt.Fprintf(b, "<div class=\"episode-desc\">%s</div>\n", html.EscapeString(ep.Description))
	}

	b.WriteString("<div class=\"ep-actions\">\n<button type=\"button\" class=\"btn ep-play\">Play</button>\n")
	fmt.Fprintf(b, "<a class=\"btn\" href=\"%s\" download>Download</a>\n", html.EscapeString(audioSrc))
	b.WriteString("<button type=\"button\" class=\"btn ep-offline\">Save offline</button>\n</div>\n")
	fmt.Fprintf(b, "<noscript><audio controls preload=\"none\" src=\"%s\"></audio></noscript>\n", html.EscapeString(audioSrc))

	if ep.ReportHref != "" {
		fmt.Fprintf(b, "<div style=\"margin-top:6px;\"><a href=\"%s\" class=\"btn\" target=\"_blank\">Ad Removal Report</a></div>\n", ep.ReportHref)
	}

	b.WriteString("</div>\n")
}

func episodeAudioSrc(ep Episode, baseURL, folder string) string {
	if ep.RemoteURL != "" {
		return ep.RemoteURL
	}
	escapedFile := url.PathEscape(ep.Filename)
	if baseURL == "" {
		return escapedFile
	}
	base := strings.TrimRight(baseURL, "/")
	if folder == "" {
		return fmt.Sprintf("%s/%s", base, escapedFile)
	}
	return fmt.Sprintf("%s/%s/%s", base, url.PathEscape(folder), escapedFile)
}

// RenderCatalog produces the library index page listing every show.
func RenderCatalog(entries []CatalogEntry, opts CatalogOptions) ([]byte, error) {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>Podcast Library</title>\n")
	fmt.Fprintf(&b, "<style>%s</style>\n</head>\n<body>\n<div class=\"container\">\n", siteCSS)
	b.WriteString("<header><h1>Podcast Library</h1>\n<p style=\"color:var(--text-muted);\">Local Ad-Free Podcasts</p>\n")
	if opts.HasOPML {
		b.WriteString("<a href=\"antennapod.opml\" class=\"btn\">AntennaPod OPML</a>\n")
	}
	b.WriteString("</header>\n<main class=\"grid\">\n")

	sorted := make([]CatalogEntry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Title) < strings.ToLower(sorted[j].Title)
	})
	for _, e := range sorted {
		renderCatalogCard(&b, e)
	}

	b.WriteString("</main>\n</div>\n</body>\n</html>\n")
	return []byte(b.String()), nil
}

func renderCatalogCard(b *strings.Builder, e CatalogEntry) {
	escapedFolder := url.PathEscape(e.Folder)
	b.WriteString("<div class=\"show-card\">\n")
	if e.CoverSrc != "" {
		fmt.Fprintf(b, "<img src=\"%s\" alt=\"Cover\" class=\"cover-img\">\n", html.EscapeString(e.CoverSrc))
	}
	fmt.Fprintf(b, "<div class=\"show-title\">%s</div>\n", html.EscapeString(e.Title))
	fmt.Fprintf(b, "<p style=\"color:var(--text-muted);font-size:0.85rem;margin:4px 0 10px;\">%d episode(s)</p>\n", e.EpisodeCount)
	fmt.Fprintf(b, "<div><a href=\"%s/index.html\" class=\"btn\">Episodes</a>\n", escapedFolder)
	fmt.Fprintf(b, "<a href=\"%s/feed.xml\" class=\"btn\">RSS</a></div>\n", escapedFolder)
	b.WriteString("</div>\n")
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}
