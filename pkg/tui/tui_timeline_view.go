package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"pod/pkg/episode"
	"pod/pkg/util"

	tea "github.com/charmbracelet/bubbletea"
)

type EpisodeOnlineRelease struct {
	Index           int
	Filename        string
	Title           string
	ExactTimeLocal  string
	Age             string
	Duration        string
	HasAdsRemoved   bool
	HasTranscript   bool
	Source          string
	ReleaseDateTime time.Time
}

func getPodcastLastEpisodesOnlineTimeline(pod tuiPodcast, maxEpisodes int) []EpisodeOnlineRelease {
	if maxEpisodes <= 0 {
		maxEpisodes = 20
	}

	eps := make([]tuiEpisode, len(pod.episodes))
	copy(eps, pod.episodes)

	sort.Slice(eps, func(i, j int) bool {
		return eps[i].displayDate().After(eps[j].displayDate())
	})

	limit := min(maxEpisodes, len(eps))
	var results []EpisodeOnlineRelease

	for i := 0; i < limit; i++ {
		ep := eps[i]
		d := ep.displayDate()

		localStr := "--:--"
		source := "Unknown"

		if !d.IsZero() {
			source = "Feed"
			localStr = d.Local().Format("2006-01-02 15:04:05 MST")
			if ep.publishedAt > 0 || (ep.absData != nil && episode.ParseABSEpisodePublishedAt(ep.absData) > 0) {
				source = "Feed"
			}
		}

		durStr := "--:--"
		if ep.duration > 0 {
			durStr = formatDurationShort(ep.duration)
		}

		results = append(results, EpisodeOnlineRelease{
			Index:           i + 1,
			Filename:        ep.filename,
			Title:           ep.displayTitle(),
			ExactTimeLocal:  localStr,
			Age:             formatRelativeAge(d),
			Duration:        durStr,
			HasAdsRemoved:   ep.hasAdsRemoved,
			HasTranscript:   ep.hasTranscript,
			Source:          source,
			ReleaseDateTime: d,
		})
	}

	return results
}

func formatEpisodesTimelineTable(releases []EpisodeOnlineRelease, podName string, termWidth int) string {
	out := &strings.Builder{}

	if termWidth <= 0 {
		termWidth = 100
	}

	out.WriteString(fmt.Sprintf("\n  Exact Online Release Timestamps for '%s' (Last %d Episodes):\n\n", podName, len(releases)))

	hdr := fmt.Sprintf("  %-3s │ %-24s │ %-7s │ %-8s │ %-8s │ %-4s │ %s",
		"#", "Release Time", "Age", "Duration", "Ads", "TX", "Title")
	divider := strings.Repeat("─", max(20, min(termWidth-4, len(hdr)+30)))

	out.WriteString("  " + divider + "\n")
	out.WriteString(hdr + "\n")
	out.WriteString("  " + divider + "\n")

	for _, r := range releases {
		adsStr := "Has Ads"
		if r.HasAdsRemoved {
			adsStr = "✓ AdFree"
		}
		txStr := "No"
		if r.HasTranscript {
			txStr = "✓ Yes"
		}
		titleWidth := max(10, termWidth-66)
		truncTitle := truncate(util.DisplayName(r.Title), titleWidth)

		row := fmt.Sprintf("  %2d. │ %-24s │ %-7s │ %-8s │ %-8s │ %-4s │ %s\n",
			r.Index, r.ExactTimeLocal, r.Age, r.Duration, adsStr, txStr, truncTitle)
		out.WriteString(row)
	}

	out.WriteString("  " + divider + "\n")
	return out.String()
}

func (m *tuiModel) openTimelineViewer() {
	if m.podIdx >= len(m.podcasts) {
		return
	}
	if m.screen != screenPlayer && m.screen != screenPlayQueue && m.screen != screenAdQueue && m.screen != screenTranscript && m.screen != screenTimeline {
		m.prevScreen = m.screen
	}
	m.timelineScroll = 0
	m.screen = screenTimeline
}

func (m *tuiModel) handleTimelineKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "up", "k":
		if m.timelineScroll > 0 {
			m.timelineScroll--
		}
	case "down", "j":
		m.timelineScroll++
	case "esc", "q", "e", "E", "o", "O":
		m.screen = m.prevScreen
	}
	return m, nil
}

func (m *tuiModel) drawTimelineScreen() string {
	out := &strings.Builder{}

	if m.podIdx >= len(m.podcasts) {
		m.screen = screenPodcasts
		return m.drawPodcastsList()
	}

	pod := m.podcasts[m.podIdx]
	releases := getPodcastLastEpisodesOnlineTimeline(pod, 20)

	banner := tuiHeaderBanner.Render(" ONLINE AVAILABILITY TIMELINE ")
	out.WriteString("  " + banner + "  " + tuiTitleStyle.Render(util.DisplayName(pod.name)) + "\n")
	out.WriteString(fmt.Sprintf("    %s\n", tuiSubtitleStyle.Render(fmt.Sprintf("Exact availability timestamps for the last %d episode(s)", len(releases)))))

	dividerWidth := max(20, m.width-4)
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")

	if len(releases) == 0 {
		out.WriteString("\n  " + tuiDimStyle.Render("No episodes found for this podcast.") + "\n\n")
		out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
		out.WriteString(tuiDimStyle.Render("  Esc/q/e Back") + "\n")
		return out.String()
	}

	hdr := fmt.Sprintf("  %-3s │ %-24s │ %-7s │ %-8s │ %-8s │ %-4s │ %s",
		"#", "Release Time", "Age", "Duration", "Ads", "TX", "Title")
	out.WriteString(tuiLabelStyle.Render(hdr) + "\n")
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")

	maxVis := max(5, m.height-8)
	if m.timelineScroll > max(0, len(releases)-maxVis) {
		m.timelineScroll = max(0, len(releases)-maxVis)
	}
	if m.timelineScroll < 0 {
		m.timelineScroll = 0
	}

	end := min(len(releases), m.timelineScroll+maxVis)
	for i := m.timelineScroll; i < end; i++ {
		r := releases[i]

		adsBadge := tuiBadgeHasAds.Render("Has Ads")
		if r.HasAdsRemoved {
			adsBadge = tuiBadgeAdFree.Render("✓ AdFree")
		}

		txBadge := tuiDimStyle.Render("No")
		if r.HasTranscript {
			txBadge = tuiCyanStyle.Render("✓ Yes")
		}

		titleWidth := max(10, m.width-62)
		truncTitle := truncate(util.DisplayName(r.Title), titleWidth)

		row := fmt.Sprintf("  %2d. │ %s │ %-7s │ %-8s │ %s │ %s │ %s\n",
			r.Index,
			tuiYellowStyle.Render(r.ExactTimeLocal),
			r.Age,
			r.Duration,
			adsBadge,
			txBadge,
			truncTitle,
		)
		out.WriteString(row)
	}

	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
	out.WriteString(tuiDimStyle.Render("  ↑/↓ Scroll │ Esc/q/e Back") + "\n")

	return out.String()
}
