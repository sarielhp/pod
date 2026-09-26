package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/config"
	"pod/pkg/util"

	tea "github.com/charmbracelet/bubbletea"
)

func testDownloadPolicyIndexNavigation(t *testing.T, m *tuiModel) {
	m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.downloadPolicyModalIdx != 1 {
		t.Errorf("expected idx 1 after Down, got %d", m.downloadPolicyModalIdx)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.downloadPolicyModalIdx != 2 {
		t.Errorf("expected idx 2 after j, got %d", m.downloadPolicyModalIdx)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.downloadPolicyModalIdx != 3 {
		t.Errorf("expected idx 3 after Down, got %d", m.downloadPolicyModalIdx)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.downloadPolicyModalIdx != 3 {
		t.Errorf("expected idx to remain 3 at bottom, got %d", m.downloadPolicyModalIdx)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.downloadPolicyModalIdx != 2 {
		t.Errorf("expected idx 2 after Up, got %d", m.downloadPolicyModalIdx)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.downloadPolicyModalIdx != 1 {
		t.Errorf("expected idx 1 after k, got %d", m.downloadPolicyModalIdx)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	if m.downloadPolicyModalIdx != 3 {
		t.Errorf("expected idx 3 after pressing '4', got %d", m.downloadPolicyModalIdx)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if m.downloadPolicyModalIdx != 0 {
		t.Errorf("expected idx 0 after pressing '1', got %d", m.downloadPolicyModalIdx)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if m.downloadPolicyModalIdx != 2 {
		t.Errorf("expected idx 2 after pressing '3', got %d", m.downloadPolicyModalIdx)
	}
}

func testDownloadPolicyDaysAdjustment(t *testing.T, m *tuiModel) {
	m.downloadPolicyModalIdx = 2
	m.policyAutoCleanup = true
	m.policyCleanupDays = 30
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'='}})
	if m.policyCleanupDays != 31 {
		t.Errorf("expected days 31 after '=', got %d", m.policyCleanupDays)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'_'}})
	if m.policyCleanupDays != 30 {
		t.Errorf("expected days 30 after '_', got %d", m.policyCleanupDays)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.policyCleanupDays != 31 {
		t.Errorf("expected days 31 after 'l', got %d", m.policyCleanupDays)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.policyCleanupDays != 30 {
		t.Errorf("expected days 30 after 'h', got %d", m.policyCleanupDays)
	}
}

func TestTUIDownloadPolicyModalNavigationAndKeys(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podIdx = 0
	m.width = 80
	m.screen = screenPodcasts

	m.openDownloadPolicyModal()
	if !m.showDownloadPolicyModal || m.downloadPolicyModalIdx != 0 {
		t.Fatalf("expected modal open with idx 0, got show=%v idx=%d", m.showDownloadPolicyModal, m.downloadPolicyModalIdx)
	}

	testDownloadPolicyIndexNavigation(t, m)
	testDownloadPolicyDaysAdjustment(t, m)

	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.showDownloadPolicyModal {
		t.Errorf("expected modal to close after Enter")
	}
	if !m.podcasts[0].config.IsAutoCleanupEnabled() || m.podcasts[0].config.AutoCleanupDays != 30 {
		t.Errorf("expected auto cleanup enabled 30 days, got %+v", m.podcasts[0].config)
	}

	saved := config.LoadPodcastConfig(tempDir, config.PodcastConfig{})
	if !saved.IsAutoCleanupEnabled() || saved.AutoCleanupDays != 30 {
		t.Errorf("expected saved config to match cleanup 30 days, got %+v", saved)
	}
}

func TestTUIDownloadPolicyModalRenderingAndCancel(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podcasts[0].config.DownloadPolicy = config.DownloadPolicyAll
	m.podIdx = 0
	m.width = 80

	m.openDownloadPolicyModal()
	rendered := m.drawDownloadPolicyModal()
	if !strings.Contains(rendered, "PODCAST POLICY") {
		t.Errorf("expected title in modal, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "1. Auto-Download") || !strings.Contains(rendered, "4. Ad Removal") {
		t.Errorf("expected options in modal, got:\n%s", rendered)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if m.showDownloadPolicyModal {
		t.Errorf("expected modal to close on 'q'")
	}
	if m.podcasts[0].config.DownloadPolicy != DownloadPolicyAll {
		t.Errorf("expected unchanged policy after cancel, got %s", m.podcasts[0].config.DownloadPolicy)
	}
}

func TestTUIDownloadPolicyKeyPress(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcasts
	m.podIdx = 0
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !m.showDownloadPolicyModal {
		t.Errorf("expected pressing 'd' to open download policy modal")
	}
}

func TestSyncPolicyToBackend_PanicRecovery(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podIdx = 0

	ch := make(chan struct{}, 1)
	setTestSyncPolicyHook(func(pod *tuiPodcast) {
		select {
		case ch <- struct{}{}:
		default:
		}
		panic("simulated backend panic during policy sync")
	})
	defer setTestSyncPolicyHook(nil)

	m.openDownloadPolicyModal()
	m.applyDownloadPolicyModal()

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected testSyncPolicyHook to run")
	}

	time.Sleep(10 * time.Millisecond)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("syncPolicyToBackend failed to catch internal panic: %v", r)
		}
	}()
	syncPolicyToBackend(nil, &m.podcasts[0], true, false, 0)
}

func TestTUIDownloadPolicyKeepPolicyCycling(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podIdx = 0
	m.openDownloadPolicyModal()

	m.downloadPolicyModalIdx = 2
	m.policyAutoCleanup = false
	m.policyCleanupDays = -1

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.policyAutoCleanup || m.policyCleanupDays != 30 || m.policyKeepPolicy != config.KeepPolicyMonth {
		t.Fatalf("expected cycle to month (30d), got cleanup=%v days=%d policy=%s", m.policyAutoCleanup, m.policyCleanupDays, m.policyKeepPolicy)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.policyAutoCleanup || m.policyCleanupDays != 180 || m.policyKeepPolicy != config.KeepPolicyFavorite {
		t.Fatalf("expected cycle to favorite (180d), got cleanup=%v days=%d policy=%s", m.policyAutoCleanup, m.policyCleanupDays, m.policyKeepPolicy)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.policyAutoCleanup || m.policyCleanupDays != 1 || m.policyKeepPolicy != config.KeepPolicyHourly {
		t.Fatalf("expected cycle to hourly (1d), got cleanup=%v days=%d policy=%s", m.policyAutoCleanup, m.policyCleanupDays, m.policyKeepPolicy)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.policyAutoCleanup || m.policyCleanupDays != -1 || m.policyKeepPolicy != config.KeepPolicyAlways {
		t.Fatalf("expected cycle to always (-1), got cleanup=%v days=%d policy=%s", m.policyAutoCleanup, m.policyCleanupDays, m.policyKeepPolicy)
	}
}

func TestTUIDownloadPolicyModalPruneExpired(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podcasts[0].name = "Test Pod"
	m.podIdx = 0

	mp3Path := filepath.Join(tempDir, "ep1.mp3")
	txPath := filepath.Join(tempDir, "ep1.transcript.json")
	_ = os.WriteFile(mp3Path, []byte("fake mp3 data"), 0644)
	_ = os.WriteFile(txPath, []byte(`{"text":"hello"}`), 0644)
	oldTime := time.Now().AddDate(0, 0, -40)
	_ = os.Chtimes(mp3Path, oldTime, oldTime)

	m.openDownloadPolicyModal()
	m.downloadPolicyModalIdx = 2
	m.policyAutoCleanup = true
	m.policyCleanupDays = 30
	m.policyKeepPolicy = config.KeepPolicyMonth

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if util.FileExists(mp3Path) {
		t.Errorf("expected expired mp3 to be deleted by 'x'")
	}
	if !util.FileExists(txPath) {
		t.Errorf("expected transcript file to be preserved")
	}
	if m.toast == nil || !strings.Contains(m.toast.Message, "Pruned 1 expired episode") {
		t.Errorf("expected prune toast message, got %+v", m.toast)
	}
}

func TestTUIKeepPolicyShortcutKeys(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcasts
	m.podIdx = 0

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	if !m.showDownloadPolicyModal {
		t.Errorf("expected 'K' on screenPodcasts to open download policy modal")
	}
	m.showDownloadPolicyModal = false

	m.screen = screenPodcastDetail
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	if !m.showDownloadPolicyModal {
		t.Errorf("expected 'K' on screenPodcastDetail to open download policy modal")
	}
}
