package tui

import (
	"fmt"

	"pod/pkg/player"
	"pod/pkg/util"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) handleKeyPart2(s string) (tea.Model, tea.Cmd) {
	if m.screen == screenPodcasts && (s == "s" || s == "S") {
		return m.handleBatchSummarizeKey()
	}
	if handlePlayerControlKey(m, s) {
		return m, nil
	}
	if handleActionModalOrTriggerKey(m, s) {
		return m, nil
	}
	return handleNavigationAndSearchKey(m, s)
}

func (m *tuiModel) handleBatchSummarizeKey() (tea.Model, tea.Cmd) {
	if m.summarizing {
		m.showPopup("AI summarization already running in background")
		return m, nil
	}
	var missing []tuiPodcast
	for _, p := range m.podcasts {
		if p.config.Summary == "" || p.config.Icon == "" {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		m.showPopup("All podcasts already have AI summaries")
		return m, nil
	}
	m.summarizing = true
	m.showPopup(fmt.Sprintf("Generating AI summaries for %d podcast(s)...", len(missing)))
	return m, m.cmdGenerateSummaries(missing)
}

func handlePlayerControlKey(m *tuiModel, s string) bool {
	switch s {
	case " ":
		globalPlayer.TogglePause()
		kv := globalPlayer.View()
		if kv.IsPaused {
			m.showPopup("Paused")
		} else if kv.IsPlaying {
			m.showPopup("Resumed")
		}
		return true
	case "right", "l", ">":
		if globalPlayer.View().IsPlaying {
			globalPlayer.Seek(30)
			m.showPopup("+30s (" + player.FormatPlayerTime(globalPlayer.View().Position) + ")")
		}
		return true
	case "left", "h", "<":
		if globalPlayer.View().IsPlaying {
			globalPlayer.Seek(-30)
			m.showPopup("-30s (" + player.FormatPlayerTime(globalPlayer.View().Position) + ")")
		}
		return true
	case "+", "=", "]":
		globalPlayer.VolumeUp()
		m.showPopup(fmt.Sprintf("Volume: %d%%", globalPlayer.View().Volume))
		return true
	case "-", "_", "[":
		globalPlayer.VolumeDown()
		m.showPopup(fmt.Sprintf("Volume: %d%%", globalPlayer.View().Volume))
		return true
	case "m", "M":
		globalPlayer.ToggleMute()
		if globalPlayer.View().Muted {
			m.showPopup("Muted")
		} else {
			m.showPopup("Unmuted")
		}
		return true
	case "s", "S":
		globalPlayer.CycleSpeaker()
		m.showPopup("Speaker: " + globalPlayer.View().CurrentSpeaker)
		return true
	case "n", "N":
		globalPlayer.Next()
		m.showPopup("Next track")
		return true
	}
	return false
}

func handleActionModalOrTriggerKey(m *tuiModel, s string) bool {
	switch s {
	case "p", "P":
		if m.screen == screenPodcastDetail && len(m.selectedEpisodes) > 0 {
			m.batchQueuePlayback()
		} else if m.screen == screenPodcastDetail || m.screen == screenEpisodeDetail {
			m.playSelectedEpisode()
		}
		return true
	case "v", "V":
		if m.screen == screenPodcastDetail {
			eps := m.filteredEpisodes()
			if m.epIdx >= 0 && m.epIdx < len(eps) {
				m.toggleEpisodeSelection(eps[m.epIdx].path)
			}
		}
		return true
	case "a", "A":
		if m.screen == screenPodcastDetail && len(m.selectedEpisodes) > 0 {
			m.batchQueueAdRemoval()
		}
		return true
	case "c", "C":
		if m.screen == screenPodcasts || m.screen == screenPodcastDetail {
			m.openPolicyModal()
		} else {
			globalPlayer.ClearQueue()
			m.showPopup("Queue cleared")
		}
		return true
	case "f", "F":
		if m.screen == screenPodcasts || m.screen == screenPodcastDetail || m.screen == screenEpisodeDetail {
			m.fetchPodcastFullFeed()
		}
		return true
	case "d":
		if m.screen == screenPodcasts {
			m.openDownloadPolicyModal()
		} else {
			m.handleEpisodeDownloadAction()
		}
		return true
	case "D":
		if m.screen == screenPodcasts {
			m.downloadAllForSelectedPodcast()
		} else {
			m.handleEpisodeDownloadAction()
		}
		return true
	case "t", "T":
		if m.screen == screenPodcastDetail || m.screen == screenEpisodeDetail {
			m.openTranscriptViewer()
		}
		return true
	case "e", "E", "o", "O":
		if m.screen == screenPodcasts || m.screen == screenPodcastDetail {
			m.openTimelineViewer()
		}
		return true
	case "K":
		if m.screen == screenPodcasts || m.screen == screenPodcastDetail {
			m.openDownloadPolicyModal()
		}
		return true
	case "x", "X":
		if m.screen == screenPodcasts || m.screen == screenPodcastDetail {
			m.pruneSelectedPodcastKeepPolicy()
		}
		return true
	}
	return false
}

func handleNavigationAndSearchKey(m *tuiModel, s string) (tea.Model, tea.Cmd) {
	switch s {
	case "up", "k":
		if m.screen == screenEpisodeDetail {
			if m.descScroll > 0 {
				m.descScroll--
			}
		} else {
			m.handleUp()
		}
	case "down", "j":
		if m.screen == screenEpisodeDetail {
			m.descScroll++
		} else {
			m.handleDown()
		}
	case "enter":
		return m.handleEnter()
	case "esc":
		m.handleEscape()
	case "r", "R":
		m.handleQueueToggle()
	case "tab":
		if m.screen == screenPodcasts {
			m.showPodcastDetailPane = !m.showPodcastDetailPane
			if m.showPodcastDetailPane {
				m.showPopup("Split view with podcast details")
			} else {
				m.showPopup("Full-width podcast table")
			}
		}
	case "i", "I":
		if m.screen == screenPodcasts {
			m.showPodcastDetailPane = !m.showPodcastDetailPane
			if m.showPodcastDetailPane {
				m.showPopup("Split view with podcast details")
			} else {
				m.showPopup("Full-width podcast table")
			}
		} else {
			m.showCover = !m.showCover
		}
	case "b", "B":
		m.showHelp = !m.showHelp
	case "/":
		m.searchMode = true
		m.searchQuery = ""
	case "ctrl+s":
		m.handleSortToggle()
	}
	return m, nil
}

func (m *tuiModel) playSelectedEpisode() {
	if m.podIdx >= len(m.podcasts) {
		return
	}
	pod := m.podcasts[m.podIdx]
	eps := m.filteredEpisodes()
	if m.epIdx >= len(eps) {
		return
	}
	ep := eps[m.epIdx]
	track := PlayerTrack{
		Title:    ep.displayTitle(),
		Podcast:  pod.name,
		Path:     ep.path,
		Duration: ep.duration,
	}

	if !ep.hasAdsRemoved {
		entries := m.queue[pod.dir]
		found := false
		for _, q := range entries {
			if q == ep.filename {
				found = true
				break
			}
		}
		if !found {
			m.queue[pod.dir] = append(entries, ep.filename)
			if m.bk != nil && m.bk.SaveQueue != nil {
				m.bk.SaveQueue(pod.dir, m.queue[pod.dir])
			}
			m.showPopup(fmt.Sprintf("Playing %s (Added to ad queue)", truncate(util.DisplayName(track.Title), 20)))
		} else {
			m.showPopup("Playing " + truncate(util.DisplayName(track.Title), 25))
		}
	} else {
		m.showPopup("Playing " + truncate(util.DisplayName(track.Title), 25))
	}

	globalPlayer.PlayTrack(track)
	if m.screen != screenPlayer && m.screen != screenPlayQueue && m.screen != screenAdQueue && m.screen != screenTranscript && m.screen != screenTimeline {
		m.prevScreen = m.screen
	}
	m.screen = screenPlayer
}

func (m *tuiModel) handleEpisodeDownloadAction() {
	if m.screen == screenPodcastDetail {
		if len(m.selectedEpisodes) > 0 {
			m.batchQueueDownload()
		} else {
			m.enqueueCurrentEpisodeDownload()
		}
	} else if m.screen == screenEpisodeDetail {
		m.enqueueCurrentEpisodeDownload()
	}
}
