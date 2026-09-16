package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"pod/pkg/backend"
	"pod/pkg/kitty"
	"pod/pkg/player"
	"pod/pkg/podcast"
	"pod/pkg/types"
	"pod/pkg/util"
)

func (m *tuiModel) drawEpisodeDetail() string {
	out := &strings.Builder{}

	if m.podIdx >= len(m.podcasts) {
		m.screen = screenPodcasts
		return m.drawPodcastsList()
	}

	pod := m.podcasts[m.podIdx]
	eps := m.filteredEpisodes()
	if m.epIdx >= len(eps) {
		m.screen = screenPodcastDetail
		return m.drawPodcastDetail()
	}

	ep := eps[m.epIdx]
	absEp := resolveABSEpisode(pod.dir, ep)

	if kitty.IsKittyTerminal() {
		out.WriteString(kitty.KittyClearGraphics())
	}

	fullTitle, dateStr, totalDurStr, badgeLeft, descClean := formatEpisodeDetailFields(pod, ep, absEp, m.epIdx, m.lib.Queue())
	maxLines := max(10, m.height-6)

	if m.showEpisodePlayerPane && m.width >= 70 {
		return renderEpisodeDetailSplitView(m, ep, fullTitle, dateStr, badgeLeft, descClean, totalDurStr, maxLines)
	}

	return renderEpisodeDetailFullView(m, fullTitle, dateStr, badgeLeft, descClean, maxLines)
}

func resolveABSEpisode(podDir string, ep tuiEpisode) *backend.Episode {
	absEp := ep.absData
	if absEp == nil {
		if cachedDet, err := podcast.LoadEpisodeDetails(podDir, ep.filename); err == nil && cachedDet != nil {
			absEp = cachedDet.RawABS
			if absEp == nil && (cachedDet.Description != "" || cachedDet.Subtitle != "") {
				absEp = &backend.Episode{
					Title:       cachedDet.Title,
					Subtitle:    cachedDet.Subtitle,
					Description: cachedDet.Description,
					EpisodeType: cachedDet.EpisodeType,
				}
			}
		}
	}
	return absEp
}

func formatEpisodeDetailFields(pod tuiPodcast, ep tuiEpisode, absEp *backend.Episode, epIdx int, dlQueue *podcast.DownloadQueue) (string, string, string, string, string) {
	epNum := ep.displayEpisodeNum(epIdx + 1)
	displayHeader := ep.displayTitle()
	d := ep.displayDate()
	dateStr := ""
	if !d.IsZero() {
		dateStr = d.Format("2006-01-02")
	}

	totalDurStr := "--:--"
	if ep.duration > 0 {
		totalDurStr = player.FormatPlayerTime(ep.duration)
	}

	titlePrefix := ""
	if epNum != "" {
		titlePrefix = epNum + " • "
	}
	fullTitle := titlePrefix + displayHeader

	epGUID := ""
	if ep.absData != nil {
		epGUID = ep.absData.ID
	}
	inDLQueue := dlQueue.IsEpisodeInQueue(epGUID, "", ep.displayTitle()) || dlQueue.IsEpisodeInQueue(epGUID, "", ep.filename)

	badgeLeft := ""
	if inDLQueue {
		badgeLeft += tuiBadgeQueued.Render("[⏳ Queued]") + " "
	}
	if ep.isFeedOnly {
		badgeLeft += tuiYellowStyle.Render("[☁ Online / Feed]") + " "
	} else if ep.hasAdsRemoved {
		badgeLeft += tuiBadgeAdFree.Render("✓ Ad-Free") + " "
	} else {
		badgeLeft += tuiBadgeHasAds.Render("✂ NeedAdR") + " "
	}
	if ep.hasTranscript {
		badgeLeft += tuiBadgeTranscript.Render("TX Transcript ('t')") + " "
	} else if !ep.isFeedOnly {
		badgeLeft += tuiDimStyle.Render("No Transcript") + " "
	}
	if totalDurStr != "" {
		badgeLeft += tuiBadgeDuration.Render(totalDurStr) + " "
	}
	badgeLeft += tuiSubtitleStyle.Render("• " + util.DisplayName(pod.name))

	descRaw := ""
	if absEp != nil && absEp.Description != "" {
		descRaw = absEp.Description
	} else if absEp != nil && absEp.Subtitle != "" {
		descRaw = absEp.Subtitle
	} else if ep.description != "" {
		descRaw = ep.description
	}
	descClean := strings.TrimSpace(renderHTML(descRaw))
	return fullTitle, dateStr, totalDurStr, badgeLeft, descClean
}

func renderEpisodeDetailSplitView(m *tuiModel, ep tuiEpisode, fullTitle, dateStr, badgeLeft, descClean, totalDurStr string, maxLines int) string {
	out := &strings.Builder{}
	leftW := (m.width - 5) / 2
	rightW := m.width - 5 - leftW

	leftLines := renderEpisodeDetailLeftPane(m, fullTitle, dateStr, badgeLeft, descClean, leftW, maxLines)
	rightLines := renderEpisodeDetailPlayerPane(ep, totalDurStr, rightW)

	totalRows := max(len(leftLines), len(rightLines))
	for k := 0; k < totalRows; k++ {
		lPart := ""
		if k < len(leftLines) {
			lPart = leftLines[k]
		}
		lPad := max(0, leftW-visibleRuneCount(lPart))
		fullL := lPart + strings.Repeat(" ", lPad)

		rPart := ""
		if k < len(rightLines) {
			rPart = rightLines[k]
		}
		out.WriteString(fullL + tuiDividerStyle.Render(" │ ") + rPart + "\n")
	}
	return out.String()
}

func renderEpisodeDetailLeftPane(m *tuiModel, fullTitle, dateStr, badgeLeft, descClean string, leftW, maxLines int) []string {
	var leftLines []string
	leftLines = append(leftLines, tuiTitleStyle.Render(truncate(util.DisplayName(fullTitle), leftW-2)))

	dateRender := ""
	if dateStr != "" {
		dateRender = tuiStatStyle.Render(dateStr)
	}
	if dateRender != "" && leftW >= 50 {
		availGap := max(1, leftW-2-visibleRuneCount(badgeLeft)-visibleRuneCount(dateRender))
		leftLines = append(leftLines, truncate(badgeLeft, leftW-visibleRuneCount(dateRender)-4)+strings.Repeat(" ", availGap)+dateRender)
	} else {
		leftLines = append(leftLines, truncate(badgeLeft, leftW-2))
		if dateStr != "" {
			leftLines = append(leftLines, tuiStatStyle.Render("Released: "+dateStr))
		}
	}
	leftLines = append(leftLines, tuiDividerStyle.Render(strings.Repeat("─", leftW-2)))

	descWrapped := wrapDescription(descClean, leftW-4)
	availDescLines := max(3, maxLines-len(leftLines)-2)
	if m.descScroll > len(descWrapped)-availDescLines {
		m.descScroll = max(0, len(descWrapped)-availDescLines)
	}
	if m.descScroll < 0 {
		m.descScroll = 0
	}

	descHeader := "Show Notes"
	if len(descWrapped) > availDescLines {
		descHeader += fmt.Sprintf(" [%d-%d of %d (Scroll: ↑/↓)]", m.descScroll+1, min(len(descWrapped), m.descScroll+availDescLines), len(descWrapped))
	}
	leftLines = append(leftLines, tuiLabelStyle.Render(descHeader))

	endDesc := min(len(descWrapped), m.descScroll+availDescLines)
	for idx := m.descScroll; idx < endDesc; idx++ {
		leftLines = append(leftLines, descWrapped[idx])
	}
	return leftLines
}

// adCutTimelineLines renders the ad-cut timeline for an episode, or nothing
// when it has no cuts recorded or the timeline does not fit.
func adCutTimelineLines(ep tuiEpisode, rightW int) []string {
	data, err := os.ReadFile(strings.TrimSuffix(ep.path, ".mp3") + ".cuts.json")
	if err != nil {
		return nil
	}
	var cd types.CutsData
	if json.Unmarshal(data, &cd) != nil || len(cd.CutIntervals) == 0 {
		return nil
	}
	dur := ep.duration
	if dur <= 0 {
		dur = cd.OriginalDurationSec
	}
	timelineStr := renderVisualAdCutTimeline(dur, cd.CutIntervals, rightW-4)
	if timelineStr == "" {
		return nil
	}
	var out []string
	for _, tl := range strings.Split(timelineStr, "\n") {
		if strings.TrimSpace(tl) != "" {
			out = append(out, tl)
		}
	}
	return append(out, "")
}

func renderEpisodeDetailPlayerPane(ep tuiEpisode, totalDurStr string, rightW int) []string {
	var rightLines []string
	rightLines = append(rightLines, tuiSectionTitle.Render(" AUDIO PLAYER (F4 to hide) "))

	pv := globalPlayer.View()
	if pv.Has {
		statusBadge := tuiPlayerPlaying.Render("▶ PLAYING")
		if pv.IsPaused {
			statusBadge = tuiPlayerPaused.Render("⏸ PAUSED")
		}
		rightLines = append(rightLines, statusBadge+" "+tuiSelectedStyle.Render(" "+truncate(util.DisplayName(pv.Title), rightW-16)+" "))
		rightLines = append(rightLines, tuiSubtextStyle.Render("  in "+util.DisplayName(pv.Podcast)))
		rightLines = append(rightLines, "  "+tuiCyanStyle.Render(globalPlayer.RenderProgressBar(rightW-4)))
	} else {
		rightLines = append(rightLines, tuiPlayerStopped.Render("⏹ STOPPED")+" "+tuiDimStyle.Render("Press 'p' or Enter to play"))
		rightLines = append(rightLines, tuiDimStyle.Render("  [────────────────────] 00:00 / "+totalDurStr))
	}
	rightLines = append(rightLines, "")

	rightLines = append(rightLines, adCutTimelineLines(ep, rightW)...)

	speaker := pv.CurrentSpeaker
	if speaker == "" {
		speaker = "Default Audio Sink"
	}
	rightLines = append(rightLines, tuiLabelStyle.Render("Audio Output & Volume:"))
	rightLines = append(rightLines, fmt.Sprintf("  Speaker: %s %s", tuiYellowStyle.Render(speaker), tuiDimStyle.Render("('s' switch)")))
	rightLines = append(rightLines, fmt.Sprintf("  Volume:  %s %s", tuiGreenStyle.Render(globalPlayer.RenderVolumeBar(rightW-20)), tuiDimStyle.Render("(+/- vol, 'm' mute)")))
	rightLines = append(rightLines, "")

	queueTitle := fmt.Sprintf("Playback Queue (%d queued)", len(pv.Queue))
	rightLines = append(rightLines, tuiLabelStyle.Render(queueTitle))
	if len(pv.Queue) > 0 {
		for qIdx, qTrack := range pv.Queue {
			if qIdx >= 3 {
				rightLines = append(rightLines, tuiDimStyle.Render(fmt.Sprintf("  ... and %d more", len(pv.Queue)-3)))
				break
			}
			rightLines = append(rightLines, tuiSubtextStyle.Render(fmt.Sprintf("  %d. %s", qIdx+1, truncate(util.DisplayName(qTrack.Title), rightW-8))))
		}
		rightLines = append(rightLines, tuiDimStyle.Render("  ('n' next track, 'c' clear queue)"))
	} else {
		rightLines = append(rightLines, tuiDimStyle.Render("  Queue is empty."))
	}

	rightLines = append(rightLines, "")
	rightLines = append(rightLines, tuiDividerStyle.Render(strings.Repeat("─", rightW-2)))
	rightLines = append(rightLines, tuiDimStyle.Render("Space Play/Pause │ p Play │ t Transcript │ F4 Hide Player │ Esc/q Back"))
	rightLines = append(rightLines, tuiDimStyle.Render("←/→   -30s / +30s │ +/- Volume     │ s Speaker"))
	rightLines = append(rightLines, tuiDimStyle.Render("↑/↓   Scroll Notes│ n Next in Queue│ m Mute"))
	return rightLines
}

func renderEpisodeDetailFullView(m *tuiModel, fullTitle, dateStr, badgeLeft, descClean string, maxLines int) string {
	out := &strings.Builder{}
	contentW := max(20, m.width-4)
	out.WriteString("  " + tuiTitleStyle.Render(truncate(util.DisplayName(fullTitle), contentW)) + "\n")

	dateRender := ""
	if dateStr != "" {
		dateRender = tuiStatStyle.Render("Released: " + dateStr)
	}
	if dateRender != "" && contentW >= 60 {
		availGap := max(1, contentW-visibleRuneCount(badgeLeft)-visibleRuneCount(dateRender))
		out.WriteString("  " + truncate(badgeLeft, contentW-visibleRuneCount(dateRender)-2) + strings.Repeat(" ", availGap) + dateRender + "\n")
	} else {
		out.WriteString("  " + truncate(badgeLeft, contentW) + "\n")
	}
	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", contentW)) + "\n")

	descWrapped := wrapDescription(descClean, contentW-2)
	availDescLines := max(3, maxLines-3)
	if m.descScroll > len(descWrapped)-availDescLines {
		m.descScroll = max(0, len(descWrapped)-availDescLines)
	}
	if m.descScroll < 0 {
		m.descScroll = 0
	}

	descHeader := "Show Notes"
	if len(descWrapped) > availDescLines {
		descHeader += fmt.Sprintf(" [%d-%d of %d (Scroll: ↑/↓)]", m.descScroll+1, min(len(descWrapped), m.descScroll+availDescLines), len(descWrapped))
	}
	out.WriteString("  " + tuiLabelStyle.Render(descHeader) + "\n")

	endDesc := min(len(descWrapped), m.descScroll+availDescLines)
	for idx := m.descScroll; idx < endDesc; idx++ {
		out.WriteString("  " + descWrapped[idx] + "\n")
	}

	out.WriteString(tuiDividerStyle.Render("  "+strings.Repeat("─", contentW)) + "\n")
	out.WriteString(tuiDimStyle.Render("  ↑/↓ Scroll Notes │ Space Play/Pause │ p Play │ D Download │ t Transcript │ F4 Show Player │ Esc/q Back") + "\n")
	return out.String()
}
