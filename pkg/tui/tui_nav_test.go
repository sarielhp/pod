package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUINavigationUpDown(t *testing.T) {
	m := makeTestModel()

	m.handleDown()
	if m.podIdx != 1 {
		t.Errorf("down should advance podIdx to 1, got %d", m.podIdx)
	}

	m.handleDown()
	if m.podIdx != 1 {
		t.Errorf("at bottom, podIdx should stay 1, got %d", m.podIdx)
	}

	m.handleUp()
	if m.podIdx != 0 {
		t.Errorf("up should go back to 0, got %d", m.podIdx)
	}

	m.handleUp()
	if m.podIdx != 0 {
		t.Errorf("at top, podIdx should stay 0, got %d", m.podIdx)
	}
}

func TestTUINavigationEpisodes(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcastDetail

	m.handleDown()
	if m.epIdx != 2 {
		t.Errorf("epIdx should be 2 after down from 1, got %d", m.epIdx)
	}

	m.handleDown()
	if m.epIdx != 2 {
		t.Errorf("at bottom epIdx should stay 2, got %d", m.epIdx)
	}

	m.handleUp()
	if m.epIdx != 1 {
		t.Errorf("epIdx should be 1 after up, got %d", m.epIdx)
	}
}

func TestTUINavigationEnter(t *testing.T) {
	m := makeTestModel()

	m.handleEnter()
	if m.screen != screenPodcastDetail {
		t.Errorf("enter should go to podcast detail, got %d", m.screen)
	}
	if m.epIdx != 0 {
		t.Errorf("epIdx should reset to 0, got %d", m.epIdx)
	}

	m.handleEnter()
	if m.screen != screenEpisodeDetail {
		t.Errorf("enter should go to episode detail, got %d", m.screen)
	}
}

func TestTUINavigationEscape(t *testing.T) {
	m := makeTestModel()

	m.screen = screenEpisodeDetail
	m.handleEscape()
	if m.screen != screenPodcastDetail {
		t.Errorf("esc from episode should go to podcast detail, got %d", m.screen)
	}

	m.handleEscape()
	if m.screen != screenPodcasts {
		t.Errorf("esc from podcast detail should go to podcasts, got %d", m.screen)
	}

	m.handleEscape()
	if m.screen != screenPodcasts {
		t.Errorf("esc at root should stay on podcasts, got %d", m.screen)
	}
}

func TestTUIKeyQuit(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcasts
	m.done = false
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil || !m.done {
		t.Errorf("q at screenPodcasts should trigger quit and set done")
	}

	subscreens := []tuiScreen{
		screenPodcastDetail,
		screenEpisodeDetail,
		screenPlayer,
		screenPlayQueue,
		screenAdQueue,
		screenTranscript,
		screenTimeline,
	}
	for _, s := range subscreens {
		sm := makeTestModel()
		sm.screen = s
		sm.done = false
		_, _ = sm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		if sm.done {
			t.Errorf("q at subscreen %v should not hard quit, should navigate back", s)
		}
	}
}

func TestTUIKeyCtrlC(t *testing.T) {
	m := makeTestModel()
	m.done = false
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.done {
		t.Error("ctrl+c should set done")
	}
}

func TestTUIQueueToggle(t *testing.T) {
	d := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = d
	m.queue[d] = []string{}
	m.screen = screenPodcastDetail

	m.handleQueueToggle()
	if len(m.queue[d]) != 1 || m.queue[d][0] != "ep102.mp3" {
		t.Errorf("r should add ep102.mp3, got %v", m.queue[d])
	}

	m.handleQueueToggle()
	if len(m.queue[d]) != 0 {
		t.Errorf("r should remove ep102.mp3, got %v", m.queue[d])
	}
}

func TestTUIQueueToggleOutOfRange(t *testing.T) {
	m := makeTestModel()
	m.screen = screenEpisodeDetail
	m.epIdx = 999
	m.handleQueueToggle()
}

func TestTUIQueueToggleOnPodcastScreen(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcasts
	was := len(m.queue["/tmp/pods/tech"])
	m.handleQueueToggle()
	if len(m.queue["/tmp/pods/tech"]) != was {
		t.Error("queue toggle should not work on podcast list screen")
	}
}

func TestTUILoadPodcastsMsg(t *testing.T) {
	m := makeTestModel()
	m.loading = true
	m.podcasts = nil

	msg := loadedPodcastsMsg{
		podcasts: []tuiPodcast{
			{name: "Test", dir: "/tmp/test", episodes: []tuiEpisode{{filename: "e.mp3"}}},
		},
		queue: map[string][]string{"/tmp/test": {"e.mp3"}},
	}

	m.Update(msg)
	if m.loading {
		t.Error("loading should be false after msg")
	}
	if len(m.podcasts) != 1 {
		t.Errorf("expected 1 podcast, got %d", len(m.podcasts))
	}
	if m.queue["/tmp/test"][0] != "e.mp3" {
		t.Error("queue not set correctly")
	}
}

func TestTUILoadPodcastsError(t *testing.T) {
	m := makeTestModel()
	m.Update(loadedPodcastsMsg{err: "test error"})
	if m.loadErr != "test error" {
		t.Errorf("expected loadErr 'test error', got %q", m.loadErr)
	}
}

// findLoadedPodcastsMsg runs the commands in a batch and returns the first
// loadedPodcastsMsg among their results.
func findLoadedPodcastsMsg(batch tea.BatchMsg) (loadedPodcastsMsg, bool) {
	for _, subCmd := range batch {
		if subCmd == nil {
			continue
		}
		if lp, ok := subCmd().(loadedPodcastsMsg); ok {
			return lp, true
		}
	}
	return loadedPodcastsMsg{}, false
}

func TestTUIModelInit(t *testing.T) {
	bk := &TuiBackend{
		LoadPodcasts: func(dir string) ([]tuiPodcast, error) {
			return []tuiPodcast{{name: "Test", dir: "/tmp/test"}}, nil
		},
		LoadQueues: func(pods []tuiPodcast) map[string][]string {
			return map[string][]string{"/tmp/test": {"a.mp3"}}
		},
	}
	m := newTuiModel(bk, "/tmp/test", nil, testLibrary())
	cmd := m.Init()

	msg := cmd()
	switch v := msg.(type) {
	case tea.BatchMsg:
		lp, found := findLoadedPodcastsMsg(v)
		if !found {
			t.Errorf("expected loadedPodcastsMsg in batch")
		}
		if lp.err != "" {
			t.Errorf("unexpected error: %s", lp.err)
		}
	case loadedPodcastsMsg:
		if v.err != "" {
			t.Errorf("unexpected error: %s", v.err)
		}
	default:
		t.Errorf("unexpected message type: %T", v)
	}
}

func TestTUIModelInitError(t *testing.T) {
	bk := &TuiBackend{
		LoadPodcasts: func(dir string) ([]tuiPodcast, error) {
			return nil, os.ErrNotExist
		},
		LoadQueues: func(pods []tuiPodcast) map[string][]string {
			return map[string][]string{}
		},
	}
	m := newTuiModel(bk, "/tmp/test", nil, testLibrary())
	cmd := m.Init()

	msg := cmd()
	switch v := msg.(type) {
	case tea.BatchMsg:
		lp, found := findLoadedPodcastsMsg(v)
		if !found {
			t.Errorf("expected loadedPodcastsMsg in batch")
		}
		if lp.err == "" {
			t.Error("expected error")
		}
	case loadedPodcastsMsg:
		if v.err == "" {
			t.Error("expected error")
		}
	default:
		t.Errorf("expected loadedPodcastsMsg, got %T", v)
	}
}

func TestTUIEpisodeDurationMsg(t *testing.T) {
	m := makeTestModel()
	m.podcasts[0].episodes[0].durationDone = false
	m.podcasts[0].episodes[0].duration = 0

	m.Update(episodeDurationMsg{idx: 0, duration: 1234.5})
	if !m.podcasts[0].episodes[0].durationDone {
		t.Error("durationDone should be true")
	}
}
