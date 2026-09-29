package tui

import (
	"strings"
	"testing"

	"pod/pkg/config"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUITimelineViewer(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcasts
	m.podIdx = 0

	m.openTimelineViewer()
	if m.screen != screenTimeline {
		t.Fatalf("expected screenTimeline, got %v", m.screen)
	}

	view := m.View()
	if !strings.Contains(view, "ONLINE AVAILABILITY TIMELINE") {
		t.Errorf("expected timeline header, got: %s", view)
	}
	if !strings.Contains(view, "Release Time") {
		t.Errorf("expected table columns in view, got: %s", view)
	}

	m.handleEscape()
	if m.screen != screenPodcasts {
		t.Errorf("expected escape to return to screenPodcasts, got %v", m.screen)
	}
}

func TestTUIMarqueeText(t *testing.T) {
	m := makeTestModel()

	short := m.marqueeText("hello", 10)
	if short != "hello" {
		t.Errorf("short text should not be truncated, got %q", short)
	}

	long := "This is a very long title that should scroll"
	m.marqueePos = 0
	m.marqueeDir = 1
	result := m.marqueeText(long, 20)
	if len(result) > 20 {
		t.Errorf("marquee result should be at most 20 chars, got %d", len(result))
	}
}

func TestTUIMarqueeResetOnSelectionChange(t *testing.T) {
	m := makeTestModel()
	long := "This is a very long title that should scroll"

	m.marqueePos = 5
	m.marqueeDir = -1
	m.marqueeTick = 10
	m.podIdx = 0
	m.epIdx = 0
	m.lastMarqueeSelection = "0-0"

	m.marqueeText(long, 20)
	posBefore := m.marqueePos

	m.epIdx = 1
	m.marqueeText(long, 20)

	if m.marqueePos == posBefore {
		t.Error("marqueePos should reset when selection changes")
	}
	if m.marqueePos != 0 {
		t.Errorf("marqueePos should reset to 0, got %d", m.marqueePos)
	}
}

func TestTUIErrorScreen(t *testing.T) {
	m := makeTestModel()
	m.loadErr = "test error"
	view := m.View()
	if !strings.Contains(view, "Connection Error") {
		t.Error("error screen should show Connection Error")
	}
	if !strings.Contains(view, "test error") {
		t.Error("error screen should show error message")
	}
	if !strings.Contains(view, "Retry") {
		t.Error("error screen should show Retry option")
	}
}

func TestTUIErrorScreenRetry(t *testing.T) {
	m := makeTestModel()
	m.loadErr = "test error"

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if m.loadErr != "" {
		t.Error("loadErr should be cleared on retry")
	}
	if !m.loading {
		t.Error("loading should be true on retry")
	}
}

func TestTUIErrorScreenQuit(t *testing.T) {
	m := makeTestModel()
	m.loadErr = "test error"

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !m.done {
		t.Error("done should be true on quit from error screen")
	}
}

func TestTUISearchSlashActivationScreens(t *testing.T) {
	screens := []tuiScreen{screenPodcasts, screenPodcastDetail, screenTranscript}
	for _, scr := range screens {
		m := makeTestModel()
		m.screen = scr
		if scr == screenTranscript {
			m.transcriptLines = []string{"[00:00.0 -> 00:05.0] test line"}
		}
		m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		if !m.searchMode {
			t.Errorf("screen %v should activate searchMode on '/'", scr)
		}
		m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
		if m.searchMode {
			t.Errorf("screen %v should exit searchMode on escape", scr)
		}
	}
}

func TestTUITranscriptScreenExportKeys(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.screen = screenTranscript
	m.transcriptLoadedFor = tempDir + "/ep.mp3"
	m.transcriptLines = []string{"[00:00.0 -> 00:05.0] Line 1"}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.toast == nil || m.toast.Type != ToastSuccess {
		t.Errorf("expected success toast on 's' key, got %+v", m.toast)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.toast == nil || m.toast.Type != ToastSuccess {
		t.Errorf("expected success toast on 'e' key, got %+v", m.toast)
	}
}

func testDownloadPolicyModalKeys(t *testing.T, m *tuiModel, tempDir string) {
	daysBefore := m.policyCleanupDays
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	if m.policyCleanupDays != daysBefore+1 {
		t.Errorf("expected days to increase to %d, got %d", daysBefore+1, m.policyCleanupDays)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	if m.policyCleanupDays != daysBefore {
		t.Errorf("expected days to return to %d, got %d", daysBefore, m.policyCleanupDays)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	if m.policyCleanupDays != daysBefore+1 {
		t.Errorf("expected days to increase with right arrow, got %d", m.policyCleanupDays)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if m.policyCleanupDays != daysBefore {
		t.Errorf("expected days to decrease with left arrow, got %d", m.policyCleanupDays)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if m.showDownloadPolicyModal {
		t.Errorf("expected modal to close after pressing Enter")
	}
	if !m.podcasts[0].config.IsAutoCleanupEnabled() || m.podcasts[0].config.AutoCleanupDays != daysBefore+2 {
		t.Errorf("expected auto cleanup enabled with days %d, got %+v", daysBefore+2, m.podcasts[0].config)
	}

	diskCfg := config.LoadPodcastConfig(tempDir, config.PodcastConfig{})
	if !diskCfg.IsAutoCleanupEnabled() || diskCfg.AutoCleanupDays != daysBefore+2 {
		t.Errorf("expected saved config on disk to match, got %+v", diskCfg)
	}
}

func TestInteractiveDownloadPolicyModal(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podIdx = 0
	m.width = 80
	m.screen = screenPodcasts

	m.openDownloadPolicyModal()
	if !m.showDownloadPolicyModal {
		t.Fatalf("expected showDownloadPolicyModal to be true")
	}

	modal := m.drawDownloadPolicyModal()
	if !strings.Contains(modal, "PODCAST POLICY") || !strings.Contains(modal, "Auto-Download") || !strings.Contains(modal, "Cleanup Days") {
		t.Errorf("expected download policy modal contents, got:\n%s", modal)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if m.downloadPolicyModalIdx != 2 {
		t.Errorf("expected downloadPolicyModalIdx 2, got %d", m.downloadPolicyModalIdx)
	}
	m.policyAutoCleanup = true
	m.policyCleanupDays = 30

	testDownloadPolicyModalKeys(t, m, tempDir)

	m.screen = screenPodcasts
	m.openDownloadPolicyModal()
	if !m.showDownloadPolicyModal {
		t.Fatalf("expected modal to open with openDownloadPolicyModal()")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m.showDownloadPolicyModal {
		t.Errorf("expected modal closed after Esc")
	}

	m.screen = screenPodcastDetail
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if m.toast == nil || !strings.Contains(m.toast.Message, "download") {
		t.Errorf("expected toast feedback when pressing D in episode list, got %+v", m.toast)
	}
}

func TestDownloadPolicyBadgesView(t *testing.T) {
	m := makeTestModel()
	m.width = 80
	m.podIdx = 0
	m.podcasts[0].config.DownloadPolicy = DownloadPolicyLatestK
	m.podcasts[0].config.DownloadK = 4

	m.screen = screenPodcasts
	viewList := m.View()
	if !strings.Contains(viewList, "Download:") || !strings.Contains(viewList, "Latest 4 episodes") {
		t.Errorf("expected list view to render download policy info, got:\n%s", viewList)
	}

	m.screen = screenPodcastDetail
	viewDetail := m.View()
	if !strings.Contains(viewDetail, "DL:") || !strings.Contains(viewDetail, "Latest 4 episodes") {
		t.Errorf("expected detail view to render DL info, got:\n%s", viewDetail)
	}
}

func TestTUIEpisodeF4PlayerToggle(t *testing.T) {
	m := makeTestModel()
	m.screen = screenEpisodeDetail
	m.showEpisodePlayerPane = false

	m.handleKey(tea.KeyMsg{Type: tea.KeyF5})
	if !m.showEpisodePlayerPane {
		t.Error("F5 on screenEpisodeDetail should enable showEpisodePlayerPane")
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyF5})
	if m.showEpisodePlayerPane {
		t.Error("second F5 on screenEpisodeDetail should collapse showEpisodePlayerPane")
	}
}
