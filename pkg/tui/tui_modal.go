package tui

import (
	"fmt"
	"strings"

	"pod/pkg/config"
	"pod/pkg/types"
	"pod/pkg/util"

	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) drawHelpModal() string {
	boxWidth := min(82, max(56, m.width-6))
	colW := (boxWidth - 7) / 2

	var lines []string
	lines = append(lines, tuiTitleStyle.Render("KEYBOARD SHORTCUTS & NAVIGATION"))
	lines = append(lines, tuiDividerStyle.Render(strings.Repeat("─", boxWidth-4)))
	lines = append(lines, "")

	leftLines := []string{
		tuiLabelStyle.Render("Navigation & Tabs:"),
		"  1-5, F2-4, F6  Switch tabs",
		"  ↑/k, ↓/j     Navigate rows",
		"  Enter        Open / Select",
		"  Esc / q      Back / Close",
		"  /            Search / filter",
		"  F1 / ? / h   Toggle help",
		"  F12          Take snapshot",
		"",
		tuiLabelStyle.Render("Episode & Notes:"),
		"  F5           Toggle player",
		"  t            Transcript",
		"  Tab          Cycle format",
		"  ↑/↓          Scroll notes",
	}

	rightLines := []string{
		tuiLabelStyle.Render("Playback & Volume:"),
		"  Space        Play / Pause",
		"  p            Play track",
		"  ← / →        Seek ±30s",
		"  + / -        Volume ±",
		"  m            Mute / Unmute",
		"  s            Cycle sink",
		"  n            Next in queue",
		"",
		tuiLabelStyle.Render("Queues & Actions:"),
		"  F            Fetch full feed",
		"  D            Download / DL All",
		"  c            Ad policy",
		"  F9           Toggle favorite ♥",
		"  e / o        Timeline",
		"  v / Space    Multi-select",
		"  r            Queue AdR",
		"  x            Delete queue item",
		"  i            Cover art",
	}

	totalRows := max(len(leftLines), len(rightLines))
	for k := 0; k < totalRows; k++ {
		lL := ""
		if k < len(leftLines) {
			lL = leftLines[k]
		}
		lPad := max(0, colW-visibleRuneCount(lL))
		fullL := lL + strings.Repeat(" ", lPad)

		rL := ""
		if k < len(rightLines) {
			rL = rightLines[k]
		}
		rPad := max(0, colW-visibleRuneCount(rL))
		fullR := rL + strings.Repeat(" ", rPad)

		lines = append(lines, fullL+tuiDividerStyle.Render(" │ ")+fullR)
	}

	lines = append(lines, "")
	lines = append(lines, tuiDividerStyle.Render(strings.Repeat("─", boxWidth-4)))
	lines = append(lines, tuiDimStyle.Render("Press Esc, ?, or q to close this help window"))

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)
}

func (m *tuiModel) drawAdPolicyModal() string {
	if m.podIdx >= len(m.podcasts) {
		return ""
	}
	pod := m.podcasts[m.podIdx]
	boxWidth := min(65, max(40, m.width-8))

	var lines []string
	lines = append(lines, tuiTitleStyle.Render("AD REMOVAL POLICY"))
	lines = append(lines, tuiSubtitleStyle.Render(truncate("for "+util.DisplayName(pod.name), boxWidth-6)))
	lines = append(lines, tuiDividerStyle.Render(strings.Repeat("─", boxWidth-4)))
	lines = append(lines, "")

	options := []struct {
		num  int
		mode string
		name string
		desc string
	}{
		{1, config.AdRemovalNone, "1. No Ad Removal (none)", "Leave audio files completely untouched (default)"},
		{2, config.AdRemovalLatest, "2. Latest Episode Only (latest)", "Auto-clean only the newest downloaded episode"},
		{3, config.AdRemovalAll, "3. All Episodes (all)", "Remove ads from every episode in this directory"},
	}

	curMode := config.NormalizeAdRemovalMode(pod.config.AdRemoval)
	for i, opt := range options {
		isSel := (i == m.policyModalIdx)
		isCurrent := (opt.mode == curMode)

		check := "( )"
		if isCurrent {
			check = "(•)"
		}

		optTitle := fmt.Sprintf("%s %s", check, opt.name)
		optDesc := fmt.Sprintf("    %s", tuiDimStyle.Render(opt.desc))

		if isSel {
			lines = append(lines, tuiSelectedStyle.Render(" "+optTitle+" "))
		} else {
			lines = append(lines, tuiLabelStyle.Render(optTitle))
		}
		lines = append(lines, optDesc)
		lines = append(lines, "")
	}

	lines = append(lines, tuiDividerStyle.Render(strings.Repeat("─", boxWidth-4)))
	lines = append(lines, tuiDimStyle.Render("↑/↓ Select │ 1-3 Choose │ Enter Apply & Save │ Esc Cancel"))

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPurple).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)
}

func renderVisualAdCutTimeline(totalDuration float64, cuts []types.CutEntry, barWidth int) string {
	if totalDuration <= 0 || barWidth < 10 {
		return ""
	}

	if barWidth > 60 {
		barWidth = 60
	}

	grid := make([]rune, barWidth)
	for i := range grid {
		grid[i] = '█'
	}

	var totalCutSec float64
	for _, c := range cuts {
		st := max(0, min(totalDuration, c.StartSec))
		en := max(0, min(totalDuration, c.EndSec))
		if en > st {
			totalCutSec += (en - st)
			stIdx := int((st / totalDuration) * float64(barWidth))
			enIdx := int((en / totalDuration) * float64(barWidth))
			for k := stIdx; k <= enIdx && k < barWidth; k++ {
				grid[k] = '░'
			}
		}
	}

	out := &strings.Builder{}
	out.WriteString(tuiLabelStyle.Render("Audio Cut Map: ") + "[")
	for _, r := range grid {
		if r == '█' {
			out.WriteString(tuiGreenStyle.Render(string(r)))
		} else {
			out.WriteString(tuiRedStyle.Render(string(r)))
		}
	}
	out.WriteString("]\n")

	pctCut := 0.0
	if totalDuration > 0 {
		pctCut = (totalCutSec / totalDuration) * 100
	}
	summary := fmt.Sprintf("  %s %s (%.1f%% cut, %d ad blocks)",
		tuiGreenStyle.Render("█ Keep"),
		tuiRedStyle.Render("░ Removed"),
		pctCut,
		len(cuts),
	)
	out.WriteString(tuiDimStyle.Render(summary))
	return out.String()
}

func (m *tuiModel) handlePodcastConfigToggle() {
	if m.podIdx >= len(m.podcasts) {
		return
	}
	pod := &m.podcasts[m.podIdx]
	pod.config.AdRemoval = config.CycleAdRemovalMode(pod.config.AdRemoval)
	if err := config.SavePodcastConfig(pod.dir, pod.config); err != nil {
		m.showToast("Failed to save config: "+err.Error(), ToastError)
		return
	}
	m.showToast("Ad removal: "+config.AdRemovalModeLabel(pod.config.AdRemoval)+" (saved)", ToastSuccess)
}

// handleFavoriteToggle flips the favorite flag of the podcast being viewed,
// with the same effects as `pod server favorite`: a favorite downloads all new
// episodes and has ads removed.
func (m *tuiModel) handleFavoriteToggle() {
	switch m.screen {
	case screenPodcasts, screenPodcastDetail, screenEpisodeDetail:
	default:
		m.showToast("Open a podcast to toggle its favorite status", ToastWarning)
		return
	}
	if m.podIdx >= len(m.podcasts) {
		return
	}
	pod := &m.podcasts[m.podIdx]
	pod.config.SetFavorite(!pod.config.Favorite)
	if err := config.SavePodcastConfig(pod.dir, pod.config); err != nil {
		m.showToast("Failed to save config: "+err.Error(), ToastError)
		return
	}
	if pod.config.Favorite {
		m.showToast(favoriteMark+" Favorite: "+pod.name+" (auto-download and ad removal)", ToastSuccess)
		return
	}
	m.showToast("Removed favorite: "+pod.name, ToastSuccess)
}

func (m *tuiModel) openPolicyModal() {
	if m.podIdx >= len(m.podcasts) {
		return
	}
	pod := &m.podcasts[m.podIdx]
	cur := config.NormalizeAdRemovalMode(pod.config.AdRemoval)
	if cur == config.AdRemovalLatest {
		m.policyModalIdx = 1
	} else if cur == config.AdRemovalAll {
		m.policyModalIdx = 2
	} else {
		m.policyModalIdx = 0
	}
	m.showPolicyModal = true
}

func (m *tuiModel) applyPolicyModal() {
	if m.podIdx >= len(m.podcasts) {
		m.showPolicyModal = false
		return
	}
	pod := &m.podcasts[m.podIdx]
	modes := []string{config.AdRemovalNone, config.AdRemovalLatest, config.AdRemovalAll}
	if m.policyModalIdx >= 0 && m.policyModalIdx < len(modes) {
		pod.config.AdRemoval = modes[m.policyModalIdx]
		if err := config.SavePodcastConfig(pod.dir, pod.config); err != nil {
			m.showToast("Failed to save config: "+err.Error(), ToastError)
		} else {
			m.showToast("Saved ad policy: "+config.AdRemovalModeLabel(pod.config.AdRemoval), ToastSuccess)
		}
	}
	m.showPolicyModal = false
}
