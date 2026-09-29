package tui

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := msg.String()
	debugLog("KEY: %q (screen: %s, podIdx: %d, epIdx: %d)", s, m.screenName(), m.podIdx, m.epIdx)

	if s == "f12" || s == "F12" {
		snapPath := m.takeSnapshot()
		m.showPopup("📸 Snapshot: " + filepath.Base(snapPath))
		return m, nil
	}

	if m.searchMode {
		return m.handleSearchKey(msg)
	}

	if m.showHelpModal {
		switch s {
		case "esc", "?", "h", "q", "Q", "enter", "f1", "F1":
			m.showHelpModal = false
		}
		return m, nil
	}

	if m.showPolicyModal {
		handlePolicyModalKey(m, s)
		return m, nil
	}

	if m.showDownloadPolicyModal {
		handleDownloadPolicyModalKey(m, s)
		return m, nil
	}

	if m.loadErr != "" {
		switch s {
		case "r", "R":
			m.loadErr = ""
			m.loading = true
			return m, m.Init()
		case "esc", "q", "Q", "ctrl+c", "ctrl+d":
			m.done = true
			return m, tea.Quit
		}
		return m, nil
	}

	if model, cmd, handled := handleGlobalScreenSwitchKey(m, s); handled {
		return model, cmd
	}

	if m.screen == screenPlayer && (s == "esc" || s == "q" || s == "Q") {
		m.handleEscape()
		return m, nil
	}
	if m.screen == screenLatestEpisodes {
		return m.handleLatestViewKey(s)
	}
	if m.screen == screenDownloadQueue {
		return m.handleDownloadQueueKey(s)
	}
	if m.screen == screenTranscript {
		return m.handleTranscriptKey(s)
	}
	if m.screen == screenTimeline {
		return m.handleTimelineKey(s)
	}
	if m.screen == screenPlayQueue {
		return m.handlePlayQueueKey(s)
	}
	if m.screen == screenAdQueue {
		return m.handleAdQueueKey(s)
	}

	return m.handleKeyPart2(s)
}

func handlePolicyModalKey(m *tuiModel, s string) {
	switch s {
	case "up", "k":
		if m.policyModalIdx > 0 {
			m.policyModalIdx--
		}
	case "down", "j":
		if m.policyModalIdx < 2 {
			m.policyModalIdx++
		}
	case "1":
		m.policyModalIdx = 0
		m.applyPolicyModal()
	case "2":
		m.policyModalIdx = 1
		m.applyPolicyModal()
	case "3":
		m.policyModalIdx = 2
		m.applyPolicyModal()
	case "enter":
		m.applyPolicyModal()
	case "esc", "q", "Q", "c", "C":
		m.showPolicyModal = false
	}
}

func handleDownloadPolicyModalKey(m *tuiModel, s string) {
	switch s {
	case "up", "k":
		if m.downloadPolicyModalIdx > 0 {
			m.downloadPolicyModalIdx--
		}
	case "down", "j":
		if m.downloadPolicyModalIdx < 3 {
			m.downloadPolicyModalIdx++
		}
	case "1":
		m.downloadPolicyModalIdx = 0
		m.togglePolicyModalField(0)
	case "2":
		m.downloadPolicyModalIdx = 1
		m.togglePolicyModalField(1)
	case "3":
		m.downloadPolicyModalIdx = 2
	case "4":
		m.downloadPolicyModalIdx = 3
		m.togglePolicyModalField(3)
	case " ", "tab":
		m.togglePolicyModalField(m.downloadPolicyModalIdx)
	case "+", "=", "right", "l":
		m.adjustPolicyModalField(1)
	case "-", "_", "left", "h":
		m.adjustPolicyModalField(-1)
	case "enter":
		m.applyDownloadPolicyModal()
	case "x", "X":
		m.applyDownloadPolicyModal()
		m.pruneSelectedPodcastKeepPolicy()
	case "esc", "q", "d", "D":
		m.showDownloadPolicyModal = false
	}
}

func handleGlobalScreenSwitchKey(m *tuiModel, s string) (tea.Model, tea.Cmd, bool) {
	switch s {
	case "ctrl+c", "ctrl+d":
		globalPlayer.Stop()
		m.done = true
		return m, tea.Quit, true
	case "esc", "q", "Q":
		if m.screen == screenPodcasts {
			globalPlayer.Stop()
			m.done = true
			return m, tea.Quit, true
		}
		m.handleEscape()
		return m, nil, true
	case "1":
		m.screen = screenPodcasts
		return m, nil, true
	case "f1", "F1":
		m.showHelpModal = !m.showHelpModal
		return m, nil, true
	case "2", "f2", "F2":
		savePrevScreenForSwitch(m)
		m.screen = screenPlayer
		return m, nil, true
	case "3", "f3", "F3":
		savePrevScreenForSwitch(m)
		m.screen = screenPlayQueue
		m.pqGrabbed = false
		return m, nil, true
	case "4", "f4", "F4":
		savePrevScreenForSwitch(m)
		m.screen = screenAdQueue
		m.adqGrabbed = false
		return m, nil, true
	case "5", "f6", "F6":
		savePrevScreenForSwitch(m)
		m.screen = screenDownloadQueue
		return m, nil, true
	case "f5", "F5":
		if m.screen == screenEpisodeDetail {
			m.showEpisodePlayerPane = !m.showEpisodePlayerPane
		}
		return m, nil, true
	case "f9", "F9":
		m.handleFavoriteToggle()
		return m, nil, true
	case "6":
		m.openTimelineViewer()
		return m, nil, true
	case "l", "L":
		if m.screen != screenLatestEpisodes {
			m.prevScreen = m.screen
			m.screen = screenLatestEpisodes
			m.latestIdx = 0
			m.latestScroll = 0
		} else {
			m.handleEscape()
		}
		return m, nil, true
	case "?", "h", "H":
		if m.screen != screenTranscript {
			m.showHelpModal = !m.showHelpModal
			return m, nil, true
		}
	}
	return nil, nil, false
}

func savePrevScreenForSwitch(m *tuiModel) {
	if m.screen != screenPlayer && m.screen != screenPlayQueue && m.screen != screenAdQueue && m.screen != screenDownloadQueue {
		m.prevScreen = m.screen
	}
}
