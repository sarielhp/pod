package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"pod/pkg/kitty"
)

func TestTopNavBarAndBreadcrumbs(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcasts
	m.width = 80

	nav := m.renderTopNavBar()
	if !strings.Contains(nav, "1: Podcasts") || !strings.Contains(nav, "2: Player") || !strings.Contains(nav, "3: Play Queue") {
		t.Errorf("expected nav to contain tabs, got:\n%s", nav)
	}
	if !strings.Contains(nav, "Location:") {
		t.Errorf("expected breadcrumbs in nav, got:\n%s", nav)
	}

	m.screen = screenEpisodeDetail
	m.podIdx = 0
	m.epIdx = 0
	navDetail := m.renderTopNavBar()
	if !strings.Contains(navDetail, "Podcasts > ") {
		t.Errorf("expected detailed breadcrumbs, got:\n%s", navDetail)
	}
}

func TestPersistentMiniPlayerBar(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcasts
	m.width = 80

	globalPlayer.Stop()
	if bar := m.renderMiniPlayerBar(); bar != "" {
		t.Errorf("expected empty mini player when stopped, got: %s", bar)
	}

	globalPlayer.PlayTrack(PlayerTrack{
		Title:    "Ep 101 Test",
		Podcast:  "Tech Talks",
		Duration: 120,
	})
	t.Cleanup(globalPlayer.Stop)

	bar := m.renderMiniPlayerBar()
	if !strings.Contains(bar, "Ep 101 Test") || !strings.Contains(bar, "Tech Talks") {
		t.Errorf("expected mini player bar to contain track info, got:\n%s", bar)
	}
	globalPlayer.Stop()
}

func TestInteractiveHelpModal(t *testing.T) {
	m := makeTestModel()
	m.width = 80
	m.showHelpModal = true

	modal := m.drawHelpModal()
	if !strings.Contains(modal, "KEYBOARD SHORTCUTS") || !strings.Contains(modal, "Playback & Volume") {
		t.Errorf("expected help modal contents, got:\n%s", modal)
	}

	view := m.View()
	if !strings.Contains(view, "KEYBOARD SHORTCUTS") {
		t.Errorf("expected View() to show help modal, got:\n%s", view)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.showHelpModal {
		t.Errorf("expected help modal to close on ?, got true")
	}
}

func TestInteractivePolicyModal(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podIdx = 0
	m.width = 80

	m.openPolicyModal()
	if !m.showPolicyModal {
		t.Fatalf("expected showPolicyModal to be true")
	}

	modal := m.drawAdPolicyModal()
	if !strings.Contains(modal, "AD REMOVAL POLICY") || !strings.Contains(modal, "Latest Episode Only") {
		t.Errorf("expected policy modal contents, got:\n%s", modal)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if m.showPolicyModal {
		t.Errorf("expected policy modal to close after selection")
	}
	if m.podcasts[0].config.AdRemoval != AdRemovalLatest {
		t.Errorf("expected AdRemovalLatest, got %s", m.podcasts[0].config.AdRemoval)
	}
}

func TestFuzzySearch(t *testing.T) {
	matched, score, indices := fuzzyMatch("swr", "Software Engineering Daily")
	if !matched {
		t.Errorf("expected 'swr' to fuzzy match 'Software Engineering Daily'")
	}
	if score <= 0 || len(indices) != 3 {
		t.Errorf("expected score and indices for match, got score=%d, indices=%v", score, indices)
	}

	matchedFail, _, _ := fuzzyMatch("xyz", "Software Engineering Daily")
	if matchedFail {
		t.Errorf("expected 'xyz' not to match")
	}

	highlighted := renderHighlightedText("test", []int{0, 2}, tuiDimStyle, tuiGreenStyle)
	if highlighted == "" {
		t.Errorf("expected non-empty highlighted output")
	}
}

func TestVisualAdCutTimeline(t *testing.T) {
	cuts := []CutEntry{
		{StartSec: 10, EndSec: 20, Reason: "Ad 1"},
		{StartSec: 50, EndSec: 60, Reason: "Ad 2"},
	}
	timeline := renderVisualAdCutTimeline(100.0, cuts, 40)
	if !strings.Contains(timeline, "Audio Cut Map") || !strings.Contains(timeline, "░ Removed") {
		t.Errorf("expected timeline bar with cut indicators, got:\n%s", timeline)
	}

	empty := renderVisualAdCutTimeline(0, cuts, 40)
	if empty != "" {
		t.Errorf("expected empty string for zero duration, got: %s", empty)
	}
}

func TestMultiSelectAndBatchOperations(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podIdx = 0
	m.screen = screenPodcastDetail
	m.epIdx = 0

	ep1 := m.podcasts[0].episodes[0].path
	ep2 := m.podcasts[0].episodes[1].path

	m.toggleEpisodeSelection(ep1)
	if !m.isEpisodeSelected(ep1) {
		t.Errorf("expected ep1 to be selected")
	}
	if m.isEpisodeSelected(ep2) {
		t.Errorf("expected ep2 not to be selected")
	}

	m.toggleEpisodeSelection(ep2)
	if len(m.selectedEpisodes) != 2 {
		t.Errorf("expected 2 selected episodes, got %d", len(m.selectedEpisodes))
	}

	m.batchQueueAdRemoval()
	if len(m.selectedEpisodes) != 0 {
		t.Errorf("expected selection cleared after batch action, got %d", len(m.selectedEpisodes))
	}
}

func TestToastNotifications(t *testing.T) {
	m := makeTestModel()
	m.showToast("Test success toast", ToastSuccess)
	if m.toast == nil || m.toast.Message != "Test success toast" {
		t.Fatalf("expected toast to be set, got %+v", m.toast)
	}

	toastView := m.renderToastNotification()
	if !strings.Contains(toastView, "Test success toast") {
		t.Errorf("expected toast output to contain message, got: %s", toastView)
	}

	m.toast.ExpiresAt = time.Now().Add(-1 * time.Second)
	if expired := m.renderToastNotification(); expired != "" {
		t.Errorf("expected expired toast to return empty string, got: %s", expired)
	}
}

func TestMouseWheelScrolling(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcasts
	m.podIdx = 0

	m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.podIdx != 1 {
		t.Errorf("expected podIdx to increment to 1 on wheel down, got %d", m.podIdx)
	}

	m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if m.podIdx != 0 {
		t.Errorf("expected podIdx to decrement to 0 on wheel up, got %d", m.podIdx)
	}
}

func TestModalOverlayKittyClear(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	m := makeTestModel()
	m.width = 100
	m.height = 30

	clearSeq := kitty.KittyClearGraphics()

	m.showHelpModal = true
	view := m.View()
	if !strings.HasPrefix(view, clearSeq) {
		t.Errorf("expected View() with showHelpModal to start with KittyClearGraphics")
	}
	m.showHelpModal = false

	m.podIdx = 0
	m.showPolicyModal = true
	view = m.View()
	if !strings.HasPrefix(view, clearSeq) {
		t.Errorf("expected View() with showPolicyModal to start with KittyClearGraphics")
	}
	m.showPolicyModal = false

	m.showDownloadPolicyModal = true
	view = m.View()
	if !strings.HasPrefix(view, clearSeq) {
		t.Errorf("expected View() with showDownloadPolicyModal to start with KittyClearGraphics")
	}
	m.showDownloadPolicyModal = false

	m.loadErr = "test error"
	view = m.View()
	if !strings.HasPrefix(view, clearSeq) {
		t.Errorf("expected View() with loadErr to start with KittyClearGraphics")
	}
}
