package tui

import (
	"fmt"
	"strings"
	"time"

	"pod/pkg/config"
	"pod/pkg/kitty"
	"pod/pkg/util"
)

func (m *tuiModel) drawPodcastsList() string {
	out := &strings.Builder{}

	titleBanner := tuiHeaderBanner.Render(" PODCASTS ")
	out.WriteString(fmt.Sprintf("  %s\n", titleBanner))

	pods := m.filteredPodcasts()
	dividerWidth := max(0, m.width-4)
	renderPodcastsListHeader(pods, dividerWidth, out)

	maxVis := m.visibleLines(4)
	start := m.podScroll
	end := min(len(pods), start+maxVis)

	if m.width >= 65 && len(pods) > 0 {
		renderPodcastsSplitView(m, pods, start, end, maxVis, out)
	} else {
		renderPodcastsNarrowView(m, pods, start, end, out)
	}

	renderPodcastsListFooter(m, len(pods), maxVis, dividerWidth, out)
	return out.String()
}

func renderPodcastsListHeader(pods []tuiPodcast, dividerWidth int, out *strings.Builder) {
	totalEps := 0
	totalDone := 0
	for _, p := range pods {
		totalEps += len(p.episodes)
		for _, e := range p.episodes {
			if e.hasAdsRemoved {
				totalDone++
			}
		}
	}
	statPill := tuiStatStyle.Render(fmt.Sprintf("  %d podcasts, %d episodes, %d ad-free", len(pods), totalEps, totalDone))
	out.WriteString(statPill + "\n")
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
}

func renderPodcastsSplitView(m *tuiModel, pods []tuiPodcast, start, end, maxVis int, out *strings.Builder) {
	leftW := min(36, max(26, m.width*32/100))
	rightW := max(30, m.width-leftW-5)

	leftLines := renderPodcastSidebarLines(pods, start, end, m.podIdx, leftW)
	var rightLines []string
	if m.podIdx < len(pods) {
		rightLines = renderPodcastDetailPaneLines(m, pods[m.podIdx], rightW, maxVis)
	}

	totalLines := max(len(leftLines), len(rightLines))
	for k := 0; k < totalLines; k++ {
		leftPart := strings.Repeat(" ", leftW)
		if k < len(leftLines) {
			leftPart = leftLines[k]
		}
		rightPart := ""
		if k < len(rightLines) {
			rightPart = rightLines[k]
		}
		out.WriteString(leftPart + tuiDividerStyle.Render(" │ ") + rightPart + "\n")
	}
}

func renderPodcastSidebarLines(pods []tuiPodcast, start, end, selectedIdx, leftW int) []string {
	var leftLines []string
	for i := start; i < end; i++ {
		p := pods[i]
		doneCount := 0
		for _, e := range p.episodes {
			if e.hasAdsRemoved {
				doneCount++
			}
		}
		nameStr := util.DisplayName(p.name)
		statsStr := fmt.Sprintf("(%d/%d)", len(p.episodes), doneCount)
		line := fmt.Sprintf("  %s %s", nameStr, statsStr)
		truncLine := truncate(line, leftW-2)
		pad := strings.Repeat(" ", max(0, leftW-len([]rune(truncLine))))
		fullLine := truncLine + pad
		if i == selectedIdx {
			leftLines = append(leftLines, tuiSelectedStyle.Render(fullLine))
		} else {
			leftLines = append(leftLines, fullLine)
		}
	}
	return leftLines
}

func renderPodcastDetailPaneLines(m *tuiModel, selPod tuiPodcast, rightW, maxVis int) []string {
	var rightLines []string
	if kitty.IsKittySupported() && m.showCover {
		rightLines = append(rightLines, renderCoverImageLines(selPod)...)
	}

	rightLines = append(rightLines, tuiTitleStyle.Render(truncate(util.DisplayName(selPod.name), rightW-2)))
	if author := selPod.displayAuthor(); author != "" {
		rightLines = append(rightLines, tuiSubtitleStyle.Render(truncate("by "+util.DisplayName(author), rightW-2)))
	}

	selDone, totalDurSec, newestDate, oldestDate := summarizePodcastEpisodes(selPod.episodes)
	statParts := []string{
		fmt.Sprintf("%d episodes", len(selPod.episodes)),
		fmt.Sprintf("%d ad-free", selDone),
		fmt.Sprintf("%d transcribed", selPod.transcribedCount()),
	}
	if queued := len(m.queue[selPod.dir]); queued > 0 {
		statParts = append(statParts, fmt.Sprintf("%d queued", queued))
	}
	if totalDurSec > 0 {
		hrs := int(totalDurSec) / 3600
		mins := (int(totalDurSec) % 3600) / 60
		if hrs > 0 {
			statParts = append(statParts, fmt.Sprintf("~%dh %dm audio", hrs, mins))
		} else {
			statParts = append(statParts, fmt.Sprintf("~%dm audio", mins))
		}
	}
	rightLines = append(rightLines, tuiStatStyle.Render(truncate(strings.Join(statParts, " • "), rightW-2)))

	policyLine := fmt.Sprintf("Ad Policy: %s ('c' change, 'e' timeline)", config.AdRemovalModeLabel(selPod.config.AdRemoval))
	rightLines = append(rightLines, tuiBadgePolicy.Render(truncate(policyLine, rightW-2)))
	dlPolicyLine := fmt.Sprintf("Download: %s ('d' change)", config.DownloadPolicyLabel(selPod.config.DownloadPolicy, selPod.config.DownloadK))
	rightLines = append(rightLines, tuiBadgePolicy.Render(truncate(dlPolicyLine, rightW-2)))
	keepPolicyLine := fmt.Sprintf("Keep Policy: %s ('d' change)", config.KeepPolicyLabel(selPod.config.EffectiveKeepPolicy(), selPod.config.EffectiveCleanupDays()))
	rightLines = append(rightLines, tuiBadgePolicy.Render(truncate(keepPolicyLine, rightW-2)))

	if !newestDate.IsZero() {
		dateInfo := fmt.Sprintf("Timeline: %s", newestDate.Format("2006-01-02"))
		if !oldestDate.IsZero() && !oldestDate.Equal(newestDate) {
			dateInfo += fmt.Sprintf(" (oldest: %s)", oldestDate.Format("2006-01-02"))
		}
		rightLines = append(rightLines, tuiSubtextStyle.Render(truncate(dateInfo, rightW-2)))
	}

	rightLines = appendPodcastDescriptionAndRecentEpisodes(rightLines, selPod, rightW, maxVis)
	return rightLines
}

func summarizePodcastEpisodes(episodes []tuiEpisode) (int, float64, time.Time, time.Time) {
	selDone := 0
	var totalDurSec float64
	var newestDate, oldestDate time.Time
	for _, e := range episodes {
		if e.hasAdsRemoved {
			selDone++
		}
		totalDurSec += e.duration
		d := e.displayDate()
		if !d.IsZero() {
			if newestDate.IsZero() || d.After(newestDate) {
				newestDate = d
			}
			if oldestDate.IsZero() || d.Before(oldestDate) {
				oldestDate = d
			}
		}
	}
	return selDone, totalDurSec, newestDate, oldestDate
}

func renderCoverImageLines(selPod tuiPodcast) []string {
	coverPath := selPod.coverPath
	if coverPath == "" {
		coverPath = kitty.FindCoverImage(selPod.dir)
	}
	if coverPath == "" {
		return nil
	}
	imgW, imgH := 24, 7
	imgEsc, err := kitty.EncodeKittyGraphicsFile(coverPath, imgW, imgH)
	if err != nil || imgEsc == "" {
		return nil
	}
	var lines []string
	if kitty.IsKittyTerminal() {
		lines = append(lines, imgEsc)
		for h := 1; h < imgH; h++ {
			lines = append(lines, strings.Repeat(" ", imgW))
		}
	} else {
		split := strings.Split(imgEsc, "\n")
		for h := 0; h < imgH; h++ {
			if h < len(split) && split[h] != "" {
				lines = append(lines, split[h])
			} else {
				lines = append(lines, strings.Repeat(" ", imgW))
			}
		}
	}
	return lines
}

func appendPodcastDescriptionAndRecentEpisodes(rightLines []string, selPod tuiPodcast, rightW, maxVis int) []string {
	if desc := selPod.displayDescription(); desc != "" {
		clean := strings.TrimSpace(renderHTML(desc))
		if len(clean) > 0 {
			rightLines = append(rightLines, "")
			prefix := "About: "
			lines := wrapText(prefix+clean, rightW-2)
			if len(lines) > 0 {
				firstLine := lines[0]
				if strings.HasPrefix(firstLine, prefix) {
					rightLines = append(rightLines, tuiSectionTitle.Render(prefix)+tuiDimStyle.Render(strings.TrimPrefix(firstLine, prefix)))
				} else {
					rightLines = append(rightLines, tuiDimStyle.Render(firstLine))
				}
				for _, l := range lines[1:] {
					if (len(rightLines) >= maxVis-5 && len(selPod.episodes) > 0) || len(rightLines) >= maxVis {
						break
					}
					rightLines = append(rightLines, tuiDimStyle.Render(l))
				}
			}
		}
	}

	if len(selPod.episodes) > 0 && len(rightLines) < maxVis-2 {
		rightLines = append(rightLines, "", tuiSectionTitle.Render("Recent Episodes:"))
		for epIdx, ep := range selPod.episodes {
			if len(rightLines) >= maxVis || epIdx >= 5 {
				break
			}
			d := ep.displayDate()
			dStr := ""
			if !d.IsZero() {
				dStr = d.Format("2006-01-02") + " "
			}
			chk := " "
			if ep.hasAdsRemoved {
				chk = "✓ "
			}
			txTag := ""
			if ep.hasTranscript {
				txTag = "[TX] "
			}
			epLine := fmt.Sprintf("  %s%s%s%s", chk, txTag, dStr, util.DisplayName(ep.displayTitle()))
			rightLines = append(rightLines, tuiDimStyle.Render(truncate(epLine, rightW-2)))
		}
	}
	return rightLines
}

func renderPodcastsNarrowView(m *tuiModel, pods []tuiPodcast, start, end int, out *strings.Builder) {
	for i := start; i < end; i++ {
		p := pods[i]
		doneCount := 0
		for _, e := range p.episodes {
			if e.hasAdsRemoved {
				doneCount++
			}
		}
		nameStr := util.DisplayName(p.name)
		statsStr := fmt.Sprintf("(%d/%d)", len(p.episodes), doneCount)
		authorStr := ""
		if author := p.displayAuthor(); author != "" {
			authorStr = fmt.Sprintf(" [%s]", util.DisplayName(author))
		}
		line := truncate(fmt.Sprintf("  %s  %s%s", nameStr, statsStr, authorStr), max(1, m.width-1))
		if i == m.podIdx {
			out.WriteString(tuiSelectedStyle.Render(line))
		} else {
			out.WriteString(line)
		}
		out.WriteByte('\n')
	}
}

func renderPodcastsListFooter(m *tuiModel, totalPods, maxVis, dividerWidth int, out *strings.Builder) {
	helpText := "↑↓ navigate │ Enter select │ F fetch-feed │ D dl-all │ L latest │ c ad-policy │ d policy │ x prune │ ? help"
	if m.searchMode {
		helpText = fmt.Sprintf("Search: %s█  (Enter: Apply, Esc: Cancel)", m.searchQuery)
	} else if totalPods > maxVis {
		pct := int(float64(m.podIdx+1) / float64(totalPods) * 100)
		helpText += fmt.Sprintf(" │ [%d/%d (%d%%)]", m.podIdx+1, totalPods, pct)
	}
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
	if m.searchMode {
		out.WriteString(tuiSearchStyle.Render("  "+helpText) + "\n")
	} else {
		out.WriteString(tuiDimStyle.Render("  "+helpText) + "\n")
	}
}
