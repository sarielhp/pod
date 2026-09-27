package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) handleUp() {
	switch m.screen {
	case screenPodcasts:
		if m.podIdx > 0 {
			m.podIdx--
			if m.podIdx < m.podScroll {
				m.podScroll = m.podIdx
			}
		}
	case screenPodcastDetail:
		if m.epIdx > 0 {
			m.epIdx--
			if m.epIdx < m.epScroll {
				m.epScroll = m.epIdx
			}
		}
	}
}

func (m *tuiModel) handleDown() {
	switch m.screen {
	case screenPodcasts:
		pods := m.filteredPodcasts()
		if m.podIdx < len(pods)-1 {
			m.podIdx++
			maxVis := m.podcastTableVisibleLines()
			if m.showPodcastDetailPane {
				maxVis = m.visibleLines(4)
			}
			if m.podIdx >= m.podScroll+maxVis {
				m.podScroll = m.podIdx - maxVis + 1
			}
		}
	case screenPodcastDetail:
		eps := m.filteredEpisodes()
		if m.epIdx < len(eps)-1 {
			m.epIdx++
			overhead := 8
			if globalPlayer.View().Has {
				overhead = 10
			}
			maxVis := max(3, m.height-overhead)
			if m.epIdx >= m.epScroll+maxVis {
				m.epScroll = m.epIdx - maxVis + 1
			}
		}
	}
}

func (m *tuiModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenPodcasts:
		pods := m.filteredPodcasts()
		if m.podIdx < len(pods) {
			m.screen = screenPodcastDetail
			m.epIdx = 0
			m.epScroll = 0
			selectedPod := pods[m.podIdx]
			for i, p := range m.podcasts {
				if p.dir == selectedPod.dir {
					m.podIdx = i
					break
				}
			}
		}

	case screenPodcastDetail:
		eps := m.filteredEpisodes()
		if m.podIdx < len(m.podcasts) && m.epIdx < len(eps) {
			m.screen = screenEpisodeDetail
			selectedEp := eps[m.epIdx]
			pod := &m.podcasts[m.podIdx]
			for i, e := range pod.episodes {
				if e.path == selectedEp.path {
					m.epIdx = i
					break
				}
			}
			ep := &pod.episodes[m.epIdx]
			if !ep.durationDone {
				idx := m.epIdx
				return m, func() tea.Msg {
					dur := m.bk.GetDuration(ep.path)
					return episodeDurationMsg{idx: idx, duration: dur}
				}
			}
		}
	}
	return m, nil
}

func (m *tuiModel) handleEscape() {
	if m.searchMode {
		m.searchMode = false
		m.searchQuery = ""
		return
	}
	if m.searchQuery != "" {
		m.searchQuery = ""
		return
	}
	switch m.screen {
	case screenPodcastDetail:
		m.screen = screenPodcasts
	case screenEpisodeDetail:
		if m.prevScreen == screenLatestEpisodes {
			m.screen = screenLatestEpisodes
		} else {
			m.screen = screenPodcastDetail
		}
	case screenPlayer, screenPlayQueue, screenAdQueue, screenDownloadQueue, screenLatestEpisodes, screenTranscript, screenTimeline:
		if m.prevScreen != 0 && m.prevScreen != m.screen {
			m.screen = m.prevScreen
		} else {
			m.screen = screenPodcasts
		}
		m.pqGrabbed = false
		m.adqGrabbed = false
	}
}
