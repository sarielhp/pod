package tui

import (
	"fmt"
	"strings"

	"pod/pkg/player"
	"pod/pkg/types"
	"pod/pkg/util"
)

func (m *tuiModel) drawPlayerScreen() string {
	out := &strings.Builder{}

	banner := tuiHeaderBanner.Render(" AUDIO PLAYER (F2) ")
	out.WriteString("  " + banner + "\n\n")

	dividerWidth := max(20, m.width-4)
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n\n")

	pv := globalPlayer.View()
	if pv.Has {
		statusBadge := tuiPlayerPlaying.Render("▶ PLAYING")
		if pv.IsPaused {
			statusBadge = tuiPlayerPaused.Render("⏸ PAUSED")
		} else if !pv.IsPlaying {
			statusBadge = tuiPlayerPaused.Render("⏸ LOADED")
		}
		out.WriteString("  " + statusBadge + "  " + tuiTitleStyle.Render(util.DisplayName(pv.Title)) + "\n")
		out.WriteString("    " + tuiSubtitleStyle.Render("in "+util.DisplayName(pv.Podcast)) + "\n\n")

		barW := max(20, m.width-8)
		out.WriteString("    " + tuiCyanStyle.Render(globalPlayer.RenderProgressBar(barW)) + "\n\n")
	} else {
		out.WriteString(tuiPlayerStopped.Render("  ⏹ STOPPED") + "  " + tuiDimStyle.Render("No track playing. Press 'p' on any episode to play.") + "\n\n")
	}

	out.WriteString(tuiLabelStyle.Render("  Volume & Output:") + "\n")
	spk := pv.CurrentSpeaker
	if spk == "" {
		spk = "Default Audio Sink"
	}
	out.WriteString(fmt.Sprintf("    Speaker: %s  %s\n", tuiYellowStyle.Render(spk), tuiDimStyle.Render("('s' to switch)")))
	out.WriteString(fmt.Sprintf("    Volume:  %s\n\n", tuiGreenStyle.Render(globalPlayer.RenderVolumeBar(24))))

	out.WriteString(tuiLabelStyle.Render("  Upcoming in Queue:") + "\n")
	if len(pv.Queue) == 0 {
		out.WriteString(tuiDimStyle.Render("    Queue is empty.") + "\n")
	} else {
		for i, track := range pv.Queue {
			if i >= 5 {
				out.WriteString(tuiDimStyle.Render(fmt.Sprintf("    ... and %d more", len(pv.Queue)-5)) + "\n")
				break
			}
			out.WriteString(fmt.Sprintf("    %d. %s %s\n", i+1, util.DisplayName(track.Title), tuiDimStyle.Render("("+util.DisplayName(track.Podcast)+")")))
		}
	}

	out.WriteString("\n" + tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
	out.WriteString(tuiDimStyle.Render("  Space Play/Pause │ ←/→ Seek 30s │ +/- Volume │ s Speaker │ n Next │ F3 Queue │ Esc/q Back") + "\n")

	return out.String()
}

func (m *tuiModel) drawPlayQueueScreen() string {
	out := &strings.Builder{}

	unified := globalPlayer.GetUnifiedQueue()
	total := len(unified)
	banner := tuiHeaderBanner.Render(fmt.Sprintf(" PLAYING QUEUE (F3) — %d items ", total))
	out.WriteString("  " + banner)

	if m.pqGrabbed {
		out.WriteString("  " + tuiPopupStyle.Render(" ⚡ REORDER MODE (↑/↓ move, Space/Enter drop) "))
	}
	out.WriteString("\n\n")

	dividerWidth := max(20, m.width-4)
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")

	if total == 0 {
		out.WriteString("\n  " + tuiDimStyle.Render("Playing queue is empty. Press 'p' on any episode in podcast view to enqueue.") + "\n\n")
		out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
		out.WriteString(tuiDimStyle.Render("  F2 Player │ F4 Ad Queue │ Esc/q Back") + "\n")
		return out.String()
	}

	maxVis := max(5, m.height-8)
	adjustPlayQueueScroll(m, total, maxVis)

	end := min(total, m.pqScroll+maxVis)
	for i := m.pqScroll; i < end; i++ {
		out.WriteString(renderPlayQueueItem(unified[i], i, m, max(20, m.width-6)))
		out.WriteByte('\n')
	}

	out.WriteString("\n" + tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
	out.WriteString(tuiDimStyle.Render("  ↑/↓ Navigate │ Space Grab/Drop (Reorder) │ Enter Play │ d/x Delete │ c Clear │ Esc/q Back") + "\n")

	return out.String()
}

func adjustPlayQueueScroll(m *tuiModel, total, maxVis int) {
	if m.pqIdx >= total {
		m.pqIdx = total - 1
	}
	if m.pqIdx < 0 {
		m.pqIdx = 0
	}
	if m.pqIdx < m.pqScroll {
		m.pqScroll = m.pqIdx
	}
	if m.pqIdx-m.pqScroll >= maxVis {
		m.pqScroll = m.pqIdx - maxVis + 1
	}
}

func renderPlayQueueItem(item types.UnifiedQueueItem, i int, m *tuiModel, availW int) string {
	track := item.Track
	titleStr := util.DisplayName(track.Title)
	podStr := " [" + util.DisplayName(track.Podcast) + "]"

	durStr := ""
	if item.IsCurrent {
		curT := player.FormatPlayerTime(item.Position)
		totT := player.FormatPlayerTime(item.Duration)
		durStr = fmt.Sprintf(" (%s / %s)", curT, totT)
	} else if track.Duration > 0 {
		durStr = " (" + player.FormatPlayerTime(track.Duration) + ")"
	}

	if item.IsCurrent {
		badge := tuiPlayerPlaying.Render("▶ PLAYING")
		if item.IsPaused {
			badge = tuiPlayerPaused.Render("⏸ PAUSED")
		}
		rawRow := badge + "  " + titleStr + podStr + durStr
		truncRow := truncate(rawRow, availW)

		if i == m.pqIdx {
			if m.pqGrabbed {
				grabRow := "  ⚡ " + truncRow + "  [MOVING]"
				fullPad := max(0, availW-len([]rune(grabRow)))
				return tuiPopupStyle.Render(grabRow + strings.Repeat(" ", fullPad))
			}
			fullPad := max(0, availW-visibleRuneCount(truncRow))
			return "  " + tuiSelectedStyle.Render(truncRow+strings.Repeat(" ", fullPad))
		}
		return "  " + truncRow
	}

	idxPrefix := fmt.Sprintf("%2d. ", i+1)
	rawRow := idxPrefix + titleStr + podStr + durStr
	truncRow := truncate(rawRow, availW)
	fullPad := max(0, availW-len([]rune(truncRow)))
	fullRow := "  " + truncRow + strings.Repeat(" ", fullPad)

	if i == m.pqIdx {
		if m.pqGrabbed {
			grabRow := "  ⚡ " + truncRow + strings.Repeat(" ", max(0, availW-len([]rune(truncRow))-2))
			return tuiPopupStyle.Render(grabRow)
		}
		return tuiSelectedStyle.Render(fullRow)
	}
	return fullRow
}

func (m *tuiModel) drawAdQueueScreen() string {
	out := &strings.Builder{}

	adItems := getAllAdQueueItems(m.podcasts, m.queue)
	total := len(adItems)

	banner := tuiHeaderBanner.Render(fmt.Sprintf(" ADR QUEUE (F4) — %d items ", total))
	out.WriteString("  " + banner)

	if m.adqGrabbed {
		out.WriteString("  " + tuiPopupStyle.Render(" ⚡ REORDER MODE (↑/↓ move, Space/Enter drop) "))
	}
	out.WriteString("\n\n")

	dividerWidth := max(20, m.width-4)
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")

	if total == 0 {
		out.WriteString("\n  " + tuiDimStyle.Render("AdR queue is empty. Press 'r' on an episode or 'p' to enqueue for playback & ad removal.") + "\n\n")
		out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
		out.WriteString(tuiDimStyle.Render("  F2 Player │ F3 Play Queue │ Esc/q Back") + "\n")
		return out.String()
	}

	maxVis := max(5, m.height-8)
	if m.adqIdx >= total {
		m.adqIdx = total - 1
	}
	if m.adqIdx < 0 {
		m.adqIdx = 0
	}
	if m.adqIdx < m.adqScroll {
		m.adqScroll = m.adqIdx
	}
	if m.adqIdx-m.adqScroll >= maxVis {
		m.adqScroll = m.adqIdx - maxVis + 1
	}

	end := min(total, m.adqScroll+maxVis)
	for i := m.adqScroll; i < end; i++ {
		item := adItems[i]
		idxPrefix := fmt.Sprintf("%2d. ", i+1)
		podPrefix := "[" + util.DisplayName(item.PodcastName) + "] "
		titleStr := util.DisplayName(item.Title)

		status := ""
		if item.HasAdsRemoved {
			status = "  " + tuiBadgeAdFree.Render("✓ Ad-Free")
		} else {
			status = "  " + tuiBadgeHasAds.Render("Has Ads")
		}

		availW := max(20, m.width-6)
		rawRow := idxPrefix + podPrefix + titleStr
		truncRow := truncate(rawRow, max(10, availW-14))
		fullRow := "  " + truncRow + status

		if i == m.adqIdx {
			if m.adqGrabbed {
				grabRow := "  ⚡ " + truncRow + "  [MOVING]"
				fullPad := max(0, availW-len([]rune(grabRow)))
				out.WriteString(tuiPopupStyle.Render(grabRow + strings.Repeat(" ", fullPad)))
			} else {
				fullPad := max(0, availW-visibleRuneCount(fullRow))
				out.WriteString(tuiSelectedStyle.Render("  " + truncRow + strings.Repeat(" ", fullPad-2) + "  "))
			}
		} else {
			out.WriteString(fullRow)
		}
		out.WriteByte('\n')
	}

	out.WriteString("\n" + tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
	out.WriteString(tuiDimStyle.Render("  ↑/↓ Navigate │ Space Grab/Drop (Reorder) │ d/x/r Delete │ Esc/q Back") + "\n")

	return out.String()
}

func (m *tuiModel) drawDownloadQueueScreen() string {
	out := &strings.Builder{}

	items := m.lib.Queue().Items()
	total := len(items)

	banner := tuiHeaderBanner.Render(fmt.Sprintf(" DOWNLOAD QUEUE — %d items ", total))
	out.WriteString("  " + banner + "\n\n")

	dividerWidth := max(20, m.width-4)
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")

	if total == 0 {
		out.WriteString("\n  " + tuiDimStyle.Render("Download queue is empty. Press 'D' on any episode to enqueue.") + "\n\n")
		out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
		out.WriteString(tuiDimStyle.Render("  F2 Player │ F3 Play Queue │ F4 Ad Queue │ Esc/q Back") + "\n")
		return out.String()
	}

	maxVis := max(5, m.height-8)
	if m.dlqIdx >= total {
		m.dlqIdx = total - 1
	}
	if m.dlqIdx < 0 {
		m.dlqIdx = 0
	}
	if m.dlqIdx < m.dlqScroll {
		m.dlqScroll = m.dlqIdx
	}
	if m.dlqIdx-m.dlqScroll >= maxVis {
		m.dlqScroll = m.dlqIdx - maxVis + 1
	}

	end := min(total, m.dlqScroll+maxVis)
	for i := m.dlqScroll; i < end; i++ {
		item := items[i]
		idxPrefix := fmt.Sprintf("%2d. ", i+1)
		podPrefix := "[" + util.DisplayName(item.PodcastTitle) + "] "
		titleStr := util.DisplayName(item.EpisodeTitle)

		statusBadge := ""
		switch item.Status {
		case "downloading":
			statusBadge = "  " + tuiPlayerPlaying.Render("↓ Downloading")
		case "completed":
			statusBadge = "  " + tuiBadgeAdFree.Render("✓ Done")
		case "failed":
			statusBadge = "  " + tuiBadgeHasAds.Render("✗ Failed")
		default:
			statusBadge = "  " + tuiBadgeQueued.Render("⏳ Queued")
		}

		availW := max(20, m.width-6)
		rawRow := idxPrefix + podPrefix + titleStr
		truncRow := truncate(rawRow, max(10, availW-16))
		fullRow := "  " + truncRow + statusBadge

		if i == m.dlqIdx {
			fullPad := max(0, availW-visibleRuneCount(fullRow))
			out.WriteString(tuiSelectedStyle.Render("  "+truncRow+strings.Repeat(" ", fullPad)) + statusBadge)
		} else {
			out.WriteString(fullRow)
		}
		out.WriteByte('\n')
	}

	out.WriteString("\n" + tuiDividerStyle.Render("  "+strings.Repeat("─", dividerWidth)) + "\n")
	out.WriteString(tuiDimStyle.Render("  ↑/↓ Navigate │ d/x Remove │ c Clear │ Enter Retry │ Esc/q Back") + "\n")

	return out.String()
}
