package tui

import (
	"fmt"
	"strings"

	"pod/pkg/config"
	"pod/pkg/kitty"
	"pod/pkg/util"
)

func (m *tuiModel) drawPodcastDetail() string {
	out := &strings.Builder{}

	if kitty.IsKittyTerminal() {
		out.WriteString(kitty.KittyClearGraphics())
	}

	if m.podIdx >= len(m.podcasts) {
		m.screen = screenPodcasts
		return m.drawPodcastsList()
	}

	pod := m.podcasts[m.podIdx]
	eps := m.filteredEpisodes()
	dividerWidth := max(0, m.width-4)

	renderPodcastDetailHeader(pod, eps, len(m.queue[pod.dir]), len(m.selectedEpisodes), dividerWidth, out)

	overhead := 8
	if globalPlayer.View().Has {
		overhead = 10
	}
	maxVis := max(3, m.height-overhead)

	adjustPodcastDetailScroll(m, len(eps), maxVis)

	start := m.epScroll
	end := min(len(eps), start+maxVis)
	queueEntries := m.queue[pod.dir]

	for i := start; i < end; i++ {
		ep := eps[i]
		epGUID := ""
		if ep.absData != nil {
			epGUID = ep.absData.ID
		}
		inDLQueue := m.lib.Queue().IsEpisodeInQueue(epGUID, "", ep.displayTitle()) || m.lib.Queue().IsEpisodeInQueue(epGUID, "", ep.filename)
		isQueued := isFileInQueue(ep.filename, queueEntries)
		out.WriteString(renderPodcastDetailEpisodeRow(m, ep, i == m.epIdx, m.isEpisodeSelected(ep.path), inDLQueue, isQueued))
		out.WriteByte('\n')
	}

	renderPodcastDetailFooter(m, len(eps), maxVis, dividerWidth, out)
	return out.String()
}

func isFileInQueue(filename string, queueEntries []string) bool {
	for _, q := range queueEntries {
		if q == filename {
			return true
		}
	}
	return false
}

func renderPodcastDetailHeader(pod tuiPodcast, eps []tuiEpisode, queuedCount, selCount, dividerWidth int, out *strings.Builder) {
	out.WriteString(tuiTitleStyle.Render("  "+util.DisplayName(pod.name)) + "\n")

	done := 0
	for _, e := range eps {
		if e.hasAdsRemoved {
			done++
		}
	}

	txCount := pod.transcribedCount()
	policyLabel := config.AdRemovalModeLabel(pod.config.AdRemoval)
	dlPolicyLabel := config.DownloadPolicyLabel(pod.config.DownloadPolicy, pod.config.DownloadK)
	keepBadge := config.KeepPolicyBadge(pod.config.EffectiveKeepPolicy(), pod.config.EffectiveCleanupDays())
	selInfo := ""
	if selCount > 0 {
		selInfo = fmt.Sprintf(" • %d selected ('a' queue AdR, 'p' play)", selCount)
	}
	statPill := tuiStatStyle.Render(fmt.Sprintf("  %d episodes, %d ad-free, %d transcribed, %d queued • AdR: %s • DL: %s • %s%s", len(eps), done, txCount, queuedCount, policyLabel, dlPolicyLabel, keepBadge, selInfo))
	out.WriteString(statPill + "\n")
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
}

func adjustPodcastDetailScroll(m *tuiModel, totalEps, maxVis int) {
	if totalEps > 0 {
		if m.epIdx >= totalEps {
			m.epIdx = totalEps - 1
		}
		if m.epIdx < 0 {
			m.epIdx = 0
		}
		if m.epIdx < m.epScroll {
			m.epScroll = m.epIdx
		}
		if m.epIdx >= m.epScroll+maxVis {
			m.epScroll = m.epIdx - maxVis + 1
		}
		if m.epScroll > max(0, totalEps-maxVis) {
			m.epScroll = max(0, totalEps-maxVis)
		}
		if m.epScroll < 0 {
			m.epScroll = 0
		}
	}
}

func renderPodcastDetailEpisodeRow(m *tuiModel, ep tuiEpisode, isSelected, isBatchSelected, inDLQueue, isQueued bool) string {
	displayNameStr := ep.displayTitle()
	d := ep.displayDate()
	dateStr := strings.Repeat(" ", 10)
	if !d.IsZero() {
		dateStr = d.Format("2006-01-02")
	}

	availWidth := m.width - 2
	selPrefix := ""
	if len(m.selectedEpisodes) > 0 {
		if isBatchSelected {
			selPrefix = "[x] "
		} else {
			selPrefix = "[ ] "
		}
	}

	if availWidth >= 40 {
		return renderWideDetailEpisodeRow(ep, displayNameStr, dateStr, selPrefix, availWidth, isSelected, isBatchSelected, inDLQueue, isQueued)
	}
	return renderNarrowDetailEpisodeRow(ep, displayNameStr, dateStr, selPrefix, m.width, isSelected, inDLQueue, isQueued)
}

func renderWideDetailEpisodeRow(ep tuiEpisode, displayNameStr, dateStr, selPrefix string, availWidth int, isSelected, isBatchSelected, inDLQueue, isQueued bool) string {
	qBadgeWidth, dlBadgeWidth, txBadgeWidth := 0, 0, 0
	if isQueued {
		qBadgeWidth = 4
	}
	if inDLQueue {
		dlBadgeWidth = 11
	}
	if ep.hasTranscript {
		txBadgeWidth = 5
	}
	titleWidth := max(10, availWidth-16-qBadgeWidth-dlBadgeWidth-txBadgeWidth-len([]rune(selPrefix)))
	truncTitle := truncate(util.DisplayName(displayNameStr), titleWidth)
	padding := strings.Repeat(" ", max(0, titleWidth-len([]rune(truncTitle))))

	chk, txBadge, qBadge, dlBadge := "  ", "", "", ""
	if isSelected {
		if ep.isFeedOnly {
			chk = "☁ "
		} else if ep.hasAdsRemoved {
			chk = "✓ "
		}
		if ep.hasTranscript {
			txBadge = "[TX] "
		}
		if isQueued {
			qBadge = " [Q]"
		}
		if inDLQueue {
			dlBadge = " [⏳ Queued]"
		}
		plainLine := "  " + selPrefix + dateStr + "  " + chk + txBadge + truncTitle + padding + qBadge + dlBadge
		fullPad := max(0, availWidth-len([]rune(plainLine)))
		return tuiSelectedStyle.Render(plainLine + strings.Repeat(" ", fullPad))
	}

	if ep.isFeedOnly {
		chk = tuiYellowStyle.Render("☁") + " "
	} else if ep.hasAdsRemoved {
		chk = tuiGreenStyle.Render("✓") + " "
	}
	if ep.hasTranscript {
		txBadge = tuiCyanStyle.Render("[TX]") + " "
	}
	if isQueued {
		qBadge = " " + tuiBadgeQueued.Render("[Q]")
	}
	if inDLQueue {
		dlBadge = " " + tuiBadgeQueued.Render("[⏳ Queued]")
	}
	selRender := selPrefix
	if isBatchSelected {
		selRender = tuiYellowStyle.Render("[x] ")
	}
	return "  " + selRender + tuiSubtextStyle.Render(dateStr) + "  " + chk + txBadge + truncTitle + padding + qBadge + dlBadge
}

func renderNarrowDetailEpisodeRow(ep tuiEpisode, displayNameStr, dateStr, selPrefix string, width int, isSelected, inDLQueue, isQueued bool) string {
	chk, txBadge, qBadge := " ", "", ""
	if isSelected {
		if ep.isFeedOnly {
			chk = "☁"
		} else if ep.hasAdsRemoved {
			chk = "✓"
		}
		if ep.hasTranscript {
			txBadge = "[TX]"
		}
		if isQueued {
			qBadge = " [Q]"
		}
		line := selPrefix + dateStr + " " + chk + " " + txBadge + " " + util.DisplayName(displayNameStr) + qBadge
		truncLine := truncate(line, max(1, width-1))
		return tuiSelectedStyle.Render(truncLine + strings.Repeat(" ", max(0, (width-1)-len([]rune(truncLine)))))
	}

	if ep.isFeedOnly {
		chk = tuiYellowStyle.Render("☁")
	} else if ep.hasAdsRemoved {
		chk = tuiGreenStyle.Render("✓")
	}
	if ep.hasTranscript {
		txBadge = tuiCyanStyle.Render("[TX]")
	}
	if isQueued {
		qBadge = " " + tuiBadgeQueued.Render("[Q]")
	}
	line := selPrefix + tuiSubtextStyle.Render(dateStr) + " " + chk + " " + txBadge + " " + util.DisplayName(displayNameStr) + qBadge
	return truncate(line, max(1, width-1))
}

func renderPodcastDetailFooter(m *tuiModel, totalEps, maxVis, dividerWidth int, out *strings.Builder) {
	helpText := "↑↓ navigate │ Enter details │ p play │ F fetch-feed │ D download │ K policy │ x prune │ v select │ a batch-queue │ t transcript │ ? help"
	if m.searchMode {
		helpText = fmt.Sprintf("Search: %s█  (Enter: Apply, Esc: Cancel)", m.searchQuery)
	} else if totalEps > maxVis {
		helpText += fmt.Sprintf(" │ [%d/%d]", m.epIdx+1, totalEps)
	}
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
	if m.searchMode {
		out.WriteString(tuiSearchStyle.Render("  "+helpText) + "\n")
	} else {
		out.WriteString(tuiDimStyle.Render("  "+helpText) + "\n")
	}
}
