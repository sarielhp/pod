package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIViewPodcasts(t *testing.T) {
	m := makeTestModel()
	m.ready = true
	m.screen = screenPodcasts

	view := m.View()

	if !strings.Contains(view, " PODCASTS ") {
		t.Error("should contain ' PODCASTS ' heading")
	}
	if !strings.Contains(view, "Tech Podcast") {
		t.Error("should contain podcast name")
	}
	if !strings.Contains(view, "News Daily") {
		t.Error("should contain podcast name")
	}
	if !strings.Contains(view, "2 podcasts") {
		t.Error("should show podcast count")
	}
	if !strings.Contains(view, "4 episodes") {
		t.Error("should show total episode count")
	}
	if !strings.Contains(view, "2 ad-free") {
		t.Error("should show ad-free count")
	}
}

func TestTUIViewLoading(t *testing.T) {
	m := makeTestModel()
	m.loading = true
	m.ready = false
	view := m.View()
	if view != "Loading podcasts..." {
		t.Errorf("expected loading, got %q", view)
	}
}

func TestTUIViewError(t *testing.T) {
	m := makeTestModel()
	m.loadErr = "something went wrong"
	view := m.View()
	if !strings.Contains(view, "something went wrong") {
		t.Errorf("expected error message in view, got %q", view)
	}
}

func TestTUIViewPodcastDetail(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcastDetail

	view := m.View()

	if !strings.Contains(view, "Tech Podcast") {
		t.Error("should show podcast name")
	}
	if !strings.Contains(view, "3 episodes") {
		t.Error("should show episode count")
	}
	if !strings.Contains(view, "2 ad-free") {
		t.Error("should show ad-free count")
	}
	if !strings.Contains(view, "1 queued") {
		t.Error("should show queued count")
	}
	if !strings.Contains(view, "ep102.mp3") {
		t.Error("should show episode filename")
	}
}

func TestTUIViewPodcastDetailCheckmark(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcastDetail

	view := m.View()

	if strings.Count(view, "✓") != 2 {
		t.Errorf("expected 2 checkmarks for ad-free episodes, found %d", strings.Count(view, "✓"))
	}
}

func TestTUIViewPodcastDetailQ(t *testing.T) {
	m := makeTestModel()
	m.screen = screenPodcastDetail

	view := m.View()

	if !strings.Contains(view, "[Q]") {
		t.Error("queued episode should show [Q] indicator")
	}
}

func TestTUIViewEpisodeDetail(t *testing.T) {
	m := makeTestModel()
	m.epIdx = 1
	m.screen = screenEpisodeDetail

	view := m.View()

	if !strings.Contains(view, "ep102.mp3") {
		t.Error("should show episode filename")
	}
	if !strings.Contains(view, "Show Notes") {
		t.Error("should show Show Notes")
	}
	if !strings.Contains(view, "NeedAdR") {
		t.Error("should show NeedAdR status")
	}
	if !strings.Contains(view, "F5 Show Player") {
		t.Error("should show F5 Show Player legend by default")
	}

	m.showEpisodePlayerPane = true
	viewWithPlayer := m.View()
	if !strings.Contains(viewWithPlayer, "AUDIO PLAYER") {
		t.Error("should show AUDIO PLAYER when player pane is enabled")
	}
	if !strings.Contains(viewWithPlayer, "F5 Hide Player") {
		t.Error("should show F5 Hide Player legend when player pane is enabled")
	}
}

func TestTUIViewEpisodeDetailAdRemoved(t *testing.T) {
	m := makeTestModel()
	m.epIdx = 0
	m.screen = screenEpisodeDetail

	view := m.View()

	if !strings.Contains(view, "Ad-Free") {
		t.Error("should show Ad-Free status")
	}
	if !strings.Contains(view, "✓") {
		t.Error("should show checkmark for ad-free")
	}
}

func TestTUIViewEpisodeDetailNotQueued(t *testing.T) {
	m := makeTestModel()
	m.epIdx = 0
	m.queue["/tmp/pods/tech"] = nil
	m.screen = screenEpisodeDetail
	m.showEpisodePlayerPane = true

	view := m.View()

	if !strings.Contains(view, "AUDIO PLAYER") {
		t.Error("should show player section")
	}
}

func TestTUIViewEpisodeDetailFallback(t *testing.T) {
	m := makeTestModel()
	m.podcasts = append(m.podcasts, tuiPodcast{name: "Empty", dir: "/tmp/empty"})
	m.podIdx = 2
	m.epIdx = 5
	m.screen = screenEpisodeDetail

	view := m.View()

	if m.screen != screenPodcastDetail {
		t.Error("should fall back to podcast detail when epIdx out of range")
	}
	if !strings.Contains(view, "Empty") {
		t.Error("should show empty podcast name after fallback")
	}
}

func TestTUIViewEpisodeDetailDuration(t *testing.T) {
	m := makeTestModel()
	m.epIdx = 0
	m.screen = screenEpisodeDetail

	view := m.View()

	if !strings.Contains(view, "30:00") {
		t.Errorf("should show duration 30:00, got %s", view)
	}
}

func TestTUIScrollIndicator(t *testing.T) {
	m := makeTestModel()
	m.podcasts = make([]tuiPodcast, 20)
	for i := range m.podcasts {
		m.podcasts[i] = tuiPodcast{
			name:     string(rune('A' + i%26)),
			episodes: []tuiEpisode{{filename: "ep.mp3"}},
		}
	}

	view := m.View()

	if !strings.Contains(view, "%") {
		t.Error("scroll indicator should show percentage")
	}
}

func TestTUIWindowSize(t *testing.T) {
	m := makeTestModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 || m.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", m.width, m.height)
	}
	if !m.ready {
		t.Error("model should be ready after WindowSizeMsg")
	}
}

func TestTUIHandleKeyUp(t *testing.T) {
	m := makeTestModel()
	m.podIdx = 0

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("up")})
	if m.podIdx != 0 {
		t.Errorf("up at top should keep podIdx=0, got %d", m.podIdx)
	}

	sendKey(m, "up")
	if m.podIdx != 0 {
		t.Errorf("k at top should keep podIdx=0, got %d", m.podIdx)
	}
}

func TestTUIHandleKeyDown(t *testing.T) {
	m := makeTestModel()
	m.podIdx = 0

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("down")})
	if m.podIdx != 1 {
		t.Errorf("down should advance to 1, got %d", m.podIdx)
	}

	sendKey(m, "j")
	if m.podIdx != 1 {
		t.Errorf("j at bottom should stay at 1, got %d", m.podIdx)
	}
}

func TestTUIHandleKeyEnter(t *testing.T) {
	m := makeTestModel()
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.screen != screenPodcastDetail {
		t.Errorf("enter should go to detail, got %d", m.screen)
	}
}

func TestTUIHandleKeyEscape(t *testing.T) {
	m := makeTestModel()
	m.screen = screenEpisodeDetail
	m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m.screen != screenPodcastDetail {
		t.Errorf("esc should go back, got %d", m.screen)
	}
}

func TestTUIHandleKeyR(t *testing.T) {
	d := t.TempDir()
	m := makeTestModel()
	m.podcasts[0].dir = d
	m.queue[d] = []string{}
	m.screen = screenPodcastDetail

	saveCalled := false
	m.bk.SaveQueue = func(dir string, entries []string) {
		saveCalled = true
	}

	sendKey(m, "r")
	if len(m.queue[d]) != 1 || m.queue[d][0] != "ep102.mp3" {
		t.Errorf("r should add ep102.mp3, got %v", m.queue[d])
	}
	if !saveCalled {
		t.Error("SaveQueue should have been called")
	}
}

func TestTUIHandleKeyCtrlC(t *testing.T) {
	m := makeTestModel()
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.done {
		t.Error("ctrl+c should set done")
	}
}
