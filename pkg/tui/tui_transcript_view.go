package tui

import (
	"fmt"
	"regexp"
	"strings"

	"pod/pkg/player"
	"pod/pkg/util"

	tea "github.com/charmbracelet/bubbletea"
)

var timestampLineRegex = regexp.MustCompile(`^(\[\d{1,2}:\d{2}(?:\.\d+)?\s*->\s*\d{1,2}:\d{2}(?:\.\d+)?\])\s*(.*)$`)

type transcriptItem struct {
	startSec  float64
	endSec    float64
	timeFull  string
	timeShort string
	text      string
	isAd      bool
}

func parseTimestampSec(s string) float64 {
	s = strings.Trim(s, "[] ")
	parts := strings.Split(s, ":")
	if len(parts) == 2 {
		var m, sec float64
		_, _ = fmt.Sscanf(parts[0], "%f", &m)
		_, _ = fmt.Sscanf(parts[1], "%f", &sec)
		return m*60 + sec
	}
	if len(parts) == 3 {
		var h, m, sec float64
		_, _ = fmt.Sscanf(parts[0], "%f", &h)
		_, _ = fmt.Sscanf(parts[1], "%f", &m)
		_, _ = fmt.Sscanf(parts[2], "%f", &sec)
		return h*3600 + m*60 + sec
	}
	return 0
}

func (m *tuiModel) openTranscriptViewer() {
	if m.podIdx >= len(m.podcasts) {
		return
	}
	eps := m.filteredEpisodes()
	if m.epIdx >= len(eps) {
		return
	}
	ep := eps[m.epIdx]

	items, lines, err := loadEpisodeTranscriptData(ep.path)
	if err != nil || len(items) == 0 {
		m.showPopup("No transcript found for: " + truncate(util.DisplayName(ep.displayTitle()), 25))
		return
	}

	if m.screen != screenPlayer && m.screen != screenPlayQueue && m.screen != screenAdQueue && m.screen != screenTranscript && m.screen != screenTimeline {
		m.prevScreen = m.screen
	}
	m.transcriptItems = items
	m.transcriptLines = lines
	m.transcriptScroll = 0
	m.transcriptMatchIdx = 0
	m.transcriptLoadedFor = ep.path
	m.screen = screenTranscript
}

func (m *tuiModel) handleTranscriptKey(s string) (tea.Model, tea.Cmd) {
	maxVis := max(5, m.height-7)
	total := len(m.transcriptItems)

	switch s {
	case "tab":
		m.transcriptViewMode = (m.transcriptViewMode + 1) % 3
	case "/":
		m.searchMode = true
		m.searchQuery = ""
		m.transcriptMatchIdx = 0
	case "n", "enter":
		if m.searchQuery != "" {
			m.nextTranscriptMatch()
		}
	case "N":
		if m.searchQuery != "" {
			m.prevTranscriptMatch()
		}
	case "s", "S", "e", "E":
		m.exportTranscript()
	case "up", "k":
		if m.transcriptScroll > 0 {
			m.transcriptScroll--
		}
	case "down", "j":
		if m.transcriptScroll < max(0, total-maxVis) {
			m.transcriptScroll++
		}
	case "pgup", "b", "ctrl+u":
		m.transcriptScroll = max(0, m.transcriptScroll-maxVis)
	case "pgdown", "f", "space", "ctrl+d":
		m.transcriptScroll = min(max(0, total-maxVis), m.transcriptScroll+maxVis)
	case "g", "home":
		m.transcriptScroll = 0
	case "G", "end":
		m.transcriptScroll = max(0, total-maxVis)
	case "esc":
		if m.searchQuery != "" {
			m.searchQuery = ""
			m.transcriptMatchIdx = 0
		} else {
			m.screen = m.prevScreen
		}
	case "q", "t", "T":
		m.screen = m.prevScreen
	}
	return m, nil
}

func parseTimestampSeconds(ts string) float64 {
	ts = strings.TrimSpace(ts)
	parts := strings.Split(ts, ":")
	if len(parts) == 2 {
		var mins, secs float64
		fmt.Sscanf(parts[0], "%f", &mins)
		fmt.Sscanf(parts[1], "%f", &secs)
		return mins*60 + secs
	}
	if len(parts) == 3 {
		var hrs, mins, secs float64
		fmt.Sscanf(parts[0], "%f", &hrs)
		fmt.Sscanf(parts[1], "%f", &mins)
		fmt.Sscanf(parts[2], "%f", &secs)
		return hrs*3600 + mins*60 + secs
	}
	return 0
}

func highlightTranscriptText(text, query string, isCurrent bool) string {
	if query == "" {
		return text
	}
	idx := strings.Index(strings.ToLower(text), strings.ToLower(query))
	if idx == -1 {
		return text
	}
	before := text[:idx]
	match := text[idx : idx+len(query)]
	after := text[idx+len(query):]

	style := tuiYellowStyle.Bold(true)
	if isCurrent {
		style = tuiSelectedStyle.Bold(true)
	}
	return before + style.Render(match) + highlightTranscriptText(after, query, isCurrent)
}

func (m *tuiModel) drawTranscriptScreen() string {
	out := &strings.Builder{}

	titleStr := "Episode Transcript"
	podName := ""
	durStr := ""
	if m.podIdx < len(m.podcasts) {
		pod := m.podcasts[m.podIdx]
		podName = pod.name
		eps := m.filteredEpisodes()
		if m.epIdx < len(eps) {
			ep := eps[m.epIdx]
			titleStr = ep.displayTitle()
			if ep.duration > 0 {
				durStr = player.FormatPlayerTime(ep.duration)
			}
		}
	}

	banner := tuiHeaderBanner.Render(" TRANSCRIPT ")
	out.WriteString("  " + banner + "  " + tuiTitleStyle.Render(truncate(util.DisplayName(titleStr), max(10, m.width-20))) + "\n")

	subInfo := util.DisplayName(podName)
	if durStr != "" {
		subInfo += " • " + durStr
	}
	ensureTranscriptItemsLoaded(m)
	subInfo += fmt.Sprintf(" • %d lines", len(m.transcriptItems))
	out.WriteString("    " + tuiSubtitleStyle.Render(subInfo) + "\n")

	dividerWidth := max(20, m.width-4)
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")

	maxVis := max(5, m.height-7)
	total := len(m.transcriptItems)
	if total == 0 {
		out.WriteString("\n  " + tuiDimStyle.Render("No transcript lines available.") + "\n\n")
		out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
		out.WriteString(tuiDimStyle.Render("  Esc/q/t Back") + "\n")
		return out.String()
	}

	if m.transcriptScroll > max(0, total-maxVis) {
		m.transcriptScroll = max(0, total-maxVis)
	}
	if m.transcriptScroll < 0 {
		m.transcriptScroll = 0
	}

	targetW := min(80, max(20, m.width-4))
	end := min(total, m.transcriptScroll+maxVis)
	matches := m.matchingTranscriptIndices()
	digits := max(3, len(fmt.Sprintf("%d", total)))

	for i := m.transcriptScroll; i < end; i++ {
		isCurrent := len(matches) > 0 && m.transcriptMatchIdx < len(matches) && i == matches[m.transcriptMatchIdx]
		out.WriteString(renderSingleTranscriptLine(m.transcriptItems[i], m.transcriptViewMode, i, digits, targetW, m.searchQuery, isCurrent))
	}

	renderTranscriptFooter(m, total, end, dividerWidth, matches, out)
	return out.String()
}

func ensureTranscriptItemsLoaded(m *tuiModel) {
	if len(m.transcriptItems) == 0 && len(m.transcriptLines) > 0 {
		for _, l := range m.transcriptLines {
			if sub := timestampLineRegex.FindStringSubmatch(l); len(sub) == 3 {
				m.transcriptItems = append(m.transcriptItems, transcriptItem{
					timeFull:  sub[1],
					timeShort: sub[1],
					text:      sub[2],
				})
			} else {
				m.transcriptItems = append(m.transcriptItems, transcriptItem{text: l})
			}
		}
	}
}

func renderSingleTranscriptLine(item transcriptItem, mode, i, digits, targetW int, searchQuery string, isCurrent bool) string {
	prefix := "  "
	if isCurrent {
		prefix = "▶ "
	}

	switch mode {
	case 0, 1:
		ts := item.timeFull
		if mode == 1 && item.timeShort != "" {
			ts = item.timeShort
		}
		availTextW := max(10, targetW-len(ts)-3)
		truncText := truncate(item.text, availTextW)
		renderedText := ""
		if searchQuery != "" {
			renderedText = highlightTranscriptText(truncText, searchQuery, isCurrent)
		} else if item.isAd {
			renderedText = tuiAdStrikeStyle.Render(truncText)
		} else {
			renderedText = tuiTranscriptTextStyle.Render(truncText)
		}
		if item.isAd {
			return prefix + tuiAdStrikeStyle.Render(ts) + " " + renderedText + "\n"
		}
		return prefix + tuiYellowStyle.Render(ts) + " " + renderedText + "\n"

	case 2:
		lineNumPrefix := fmt.Sprintf("%*d │ ", digits, i+1)
		availTextW := max(10, targetW-digits-6)
		truncText := truncate(item.text, availTextW)
		renderedText := ""
		if searchQuery != "" {
			renderedText = highlightTranscriptText(truncText, searchQuery, isCurrent)
		} else if item.isAd {
			renderedText = tuiAdStrikeStyle.Render(truncText)
		} else {
			renderedText = tuiTranscriptTextStyle.Render(truncText)
		}
		return prefix + tuiDimStyle.Render(lineNumPrefix) + renderedText + "\n"
	}
	return ""
}

func renderTranscriptFooter(m *tuiModel, total, end, dividerWidth int, matches []int, out *strings.Builder) {
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")

	scrollPct := 0
	if total > 0 {
		scrollPct = int(float64(min(end, total)) / float64(total) * 100)
	}

	arrowHelp := "Tab Short Time"
	if m.transcriptViewMode == 1 {
		arrowHelp = "Tab Line Nums"
	} else if m.transcriptViewMode == 2 {
		arrowHelp = "Tab Time Arrows"
	}

	var helpText string
	if m.searchQuery != "" {
		matchInfo := fmt.Sprintf("[%d/%d matches]", m.transcriptMatchIdx+1, len(matches))
		if len(matches) == 0 {
			matchInfo = "[0 matches]"
		}
		helpText = fmt.Sprintf("Enter/n Next │ N Prev │ / Search │ Esc Clear │ s/e Export │ %s", matchInfo)
	} else {
		helpText = fmt.Sprintf("↑/↓ Scroll │ %s │ / Search │ s/e Export │ Esc/q/t Back │ [%d%%]", arrowHelp, scrollPct)
	}
	out.WriteString(tuiDimStyle.Render("  " + helpText + "\n"))
}
