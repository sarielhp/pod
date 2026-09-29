package tui

import (
	"strings"
	"testing"

	"pod/pkg/config"

	tea "github.com/charmbracelet/bubbletea"
)

func TestF1OpensAndClosesHelpOnAnyScreen(t *testing.T) {
	for _, screen := range []tuiScreen{screenPodcasts, screenEpisodeDetail, screenPlayQueue} {
		m := makeTestModel()
		m.screen = screen
		m.handleKey(tea.KeyMsg{Type: tea.KeyF1})
		if !m.showHelpModal {
			t.Errorf("F1 should open help on screen %v", screen)
		}
		m.handleKey(tea.KeyMsg{Type: tea.KeyF1})
		if m.showHelpModal {
			t.Errorf("F1 should close help again on screen %v", screen)
		}
	}
}

func TestFunctionKeysMovedOneToTheRight(t *testing.T) {
	cases := map[tea.KeyType]tuiScreen{
		tea.KeyF2: screenPlayer,
		tea.KeyF3: screenPlayQueue,
		tea.KeyF4: screenAdQueue,
		tea.KeyF6: screenDownloadQueue,
	}
	for key, want := range cases {
		m := makeTestModel()
		m.handleKey(tea.KeyMsg{Type: key})
		if m.screen != want {
			t.Errorf("%v: want screen %v, got %v", key, want, m.screen)
		}
	}
}

func TestF9TogglesFavoriteAndSavesIt(t *testing.T) {
	m := makeTestModel()
	dir := t.TempDir()
	m.podcasts[0].dir = dir

	m.handleKey(tea.KeyMsg{Type: tea.KeyF9})
	if !m.podcasts[0].config.Favorite {
		t.Fatal("F9 should mark the selected podcast as a favorite")
	}
	saved := config.LoadPodcastConfig(dir, config.PodcastConfig{})
	if !saved.Favorite || saved.AdRemoval != config.AdRemovalAll {
		t.Errorf("a favorite is saved with ad removal on, got %+v", saved)
	}
	if !strings.Contains(m.View(), "♥ Tech") {
		t.Errorf("a favorite should carry a heart in the list:\n%s", m.View())
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyF9})
	if m.podcasts[0].config.Favorite || config.LoadPodcastConfig(dir, config.PodcastConfig{}).Favorite {
		t.Error("a second F9 should clear the favorite")
	}
	if strings.Contains(m.View(), "♥ Tech") {
		t.Error("no heart on the row once the favorite is cleared")
	}
}

func TestF9WorksInsideAPodcastAndNotOnQueueScreens(t *testing.T) {
	m := makeTestModel()
	m.podcasts[0].dir = t.TempDir()
	m.screen = screenEpisodeDetail
	m.handleKey(tea.KeyMsg{Type: tea.KeyF9})
	if !m.podcasts[0].config.Favorite {
		t.Error("F9 inside a podcast should favorite that podcast")
	}

	q := makeTestModel()
	q.podcasts[0].dir = t.TempDir()
	q.screen = screenAdQueue
	q.handleKey(tea.KeyMsg{Type: tea.KeyF9})
	if q.podcasts[0].config.Favorite {
		t.Error("F9 on a queue screen must not favorite an arbitrary podcast")
	}
}
