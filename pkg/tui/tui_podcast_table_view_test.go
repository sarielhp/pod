package tui

import (
	"strings"
	"testing"

	"pod/pkg/config"
	"pod/pkg/detect"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIPodcastsFullLineTableView(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.width = 140
	m.height = 30
	m.screen = screenPodcasts
	m.podcasts[0].dir = tempDir
	m.podcasts[0].config.Icon = "🧠"
	m.podcasts[0].config.Summary = "A leading podcast about brain science and performance protocols."
	m.podcasts[0].config.DownloadPolicy = config.DownloadPolicyLatest
	m.podcasts[0].config.KeepPolicy = config.KeepPolicyMonth
	m.podcasts[0].config.AdRemoval = config.AdRemovalAll

	view := m.View()

	// Verify table columns exist
	if !strings.Contains(view, "Icon") || !strings.Contains(view, "Policy (DL•Ret•AdR)") || !strings.Contains(view, "Episodes") {
		t.Fatalf("expected table headers in full-line view, got:\n%s", view)
	}

	// Verify user-defined emoji icon appears
	if !strings.Contains(view, "🧠") {
		t.Errorf("expected user-defined icon 🧠 in view, got:\n%s", view)
	}

	// Verify policy badges in table row
	if !strings.Contains(view, "📥New") || !strings.Contains(view, "🗓️30d") || !strings.Contains(view, "✂️All") {
		t.Errorf("expected policy badges in table row, got:\n%s", view)
	}

	// Verify bottom policy line
	if !strings.Contains(view, "Policy: 📥 Download:") || !strings.Contains(view, "Retention:") || !strings.Contains(view, "Ad Removal:") {
		t.Errorf("expected bottom policy line, got:\n%s", view)
	}

	// Verify summary appears in bottom drawer
	if !strings.Contains(view, "Summary:") || !strings.Contains(view, "A leading podcast about brain science") {
		t.Errorf("expected summary in bottom drawer, got:\n%s", view)
	}
}

func TestTUIPodcastsTableViewToggleSplit(t *testing.T) {
	m := makeTestModel()
	m.width = 100
	m.height = 30
	m.screen = screenPodcasts
	if m.showPodcastDetailPane {
		t.Errorf("expected showPodcastDetailPane to be false by default")
	}

	// Press Tab to toggle split view
	m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if !m.showPodcastDetailPane {
		t.Errorf("expected showPodcastDetailPane to be true after Tab")
	}

	viewSplit := m.View()
	// Split view has sidebar divider │
	if !strings.Contains(viewSplit, "│") {
		t.Errorf("expected split view with sidebar divider, got:\n%s", viewSplit)
	}

	// Press Tab again to toggle back to full table view
	m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.showPodcastDetailPane {
		t.Errorf("expected showPodcastDetailPane to be false after second Tab")
	}

	viewTable := m.View()
	if !strings.Contains(viewTable, "Policy (DL•Ret•AdR)") {
		t.Errorf("expected full-width table view after toggling back, got:\n%s", viewTable)
	}
}

func TestTUIBatchSummariesMsgHandling(t *testing.T) {
	tempDir := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = tempDir
	m.podcasts[0].config.Icon = ""
	m.podcasts[0].config.Summary = ""
	m.summarizing = true

	results := map[string]detect.PodcastSummaryOutput{
		"Tech Podcast": {
			Icon:    "💻",
			Summary: "A weekly conversation with developers and tech leaders.",
		},
	}

	m.handlePodcastSummariesMsg(podcastSummariesMsg{results: results})

	if m.summarizing {
		t.Errorf("expected summarizing to be false after msg handled")
	}
	if m.podcasts[0].config.Icon != "💻" {
		t.Errorf("expected icon 💻, got %q", m.podcasts[0].config.Icon)
	}
	if m.podcasts[0].config.Summary != "A weekly conversation with developers and tech leaders." {
		t.Errorf("summary mismatch: %q", m.podcasts[0].config.Summary)
	}

	// Verify config was saved to disk
	loaded := config.LoadPodcastConfig(tempDir, config.PodcastConfig{})
	if loaded.Icon != "💻" {
		t.Errorf("persisted icon mismatch: %q", loaded.Icon)
	}
	if loaded.Summary != "A weekly conversation with developers and tech leaders." {
		t.Errorf("persisted summary mismatch: %q", loaded.Summary)
	}
}

func TestTUIBatchSummarizeKeyTrigger(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcasts
	m.podcasts[0].config.Summary = ""
	m.podcasts[1].config.Summary = ""

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if !m.summarizing {
		t.Errorf("expected summarizing to be set to true on 's' key")
	}
	if cmd == nil {
		t.Errorf("expected a non-nil tea.Cmd for background batch summarization")
	}
}
