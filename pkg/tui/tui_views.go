package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"pod/pkg/kitty"
)

func (m *tuiModel) drawPopup() string {
	if m.popupMsg == "" {
		return ""
	}
	return tuiPopupStyle.Render("  " + m.popupMsg)
}
func (m *tuiModel) drawErrorScreen() string {
	out := &strings.Builder{}
	if kitty.IsKittyTerminal() {
		out.WriteString(kitty.KittyClearGraphics())
	}
	out.WriteString(tuiTitleStyle.Render("  Connection Error"))
	out.WriteByte('\n')
	out.WriteByte('\n')
	out.WriteString(tuiDimStyle.Render("  " + m.loadErr))
	out.WriteByte('\n')
	out.WriteByte('\n')
	out.WriteString(tuiHelpStyle.Render("  R - Retry  Q - Quit"))
	out.WriteByte('\n')
	return out.String()
}
func (m *tuiModel) View() string {
	if m.loading {
		return "Loading podcasts..."
	}
	if m.loadErr != "" {
		return m.drawErrorScreen()
	}
	if !m.ready {
		return "Initializing..."
	}

	if m.screen == screenPodcastDetail && m.podIdx < len(m.podcasts) {
		m.setTerminalTitle("pod - " + m.podcasts[m.podIdx].name)
	} else if m.screen == screenEpisodeDetail && m.podIdx < len(m.podcasts) && m.epIdx < len(m.podcasts[m.podIdx].episodes) {
		m.setTerminalTitle("pod - " + m.podcasts[m.podIdx].episodes[m.epIdx].filename)
	} else {
		m.setTerminalTitle("pod")
	}

	if m.showHelpModal {
		return m.renderModalOverlay(m.drawHelpModal())
	}
	if m.showPolicyModal {
		return m.renderModalOverlay(m.drawAdPolicyModal())
	}
	if m.showDownloadPolicyModal {
		return m.renderModalOverlay(m.drawDownloadPolicyModal())
	}

	var body strings.Builder
	if kitty.IsKittyTerminal() {
		body.WriteString(kitty.KittyClearGraphics())
	}
	body.WriteString(m.renderTopNavBar())

	switch m.screen {
	case screenPodcasts:
		body.WriteString(m.drawPodcastsList())
	case screenPodcastDetail:
		body.WriteString(m.drawPodcastDetail())
	case screenEpisodeDetail:
		body.WriteString(m.drawEpisodeDetail())
	case screenPlayer:
		body.WriteString(m.drawPlayerScreen())
	case screenPlayQueue:
		body.WriteString(m.drawPlayQueueScreen())
	case screenAdQueue:
		body.WriteString(m.drawAdQueueScreen())
	case screenDownloadQueue:
		body.WriteString(m.drawDownloadQueueScreen())
	case screenLatestEpisodes:
		body.WriteString(m.drawLatestEpisodesScreen())
	case screenTranscript:
		body.WriteString(m.drawTranscriptScreen())
	case screenTimeline:
		body.WriteString(m.drawTimelineScreen())
	}

	miniPlayer := m.renderMiniPlayerBar()
	if miniPlayer != "" {
		body.WriteString(miniPlayer)
	}

	toast := m.renderToastNotification()
	if toast != "" {
		body.WriteString(toast)
	} else {
		popup := m.drawPopup()
		if popup != "" {
			body.WriteString("\n" + popup)
		}
	}

	return body.String()
}

func (m *tuiModel) renderModalOverlay(content string) string {
	if content == "" {
		return ""
	}
	box := content
	if m.width > 0 && m.height > 0 {
		box = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	if kitty.IsKittyTerminal() {
		return kitty.KittyClearGraphics() + box
	}
	return box
}
