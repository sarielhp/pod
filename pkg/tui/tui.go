package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/audio"
	"pod/pkg/backend"
	"pod/pkg/kitty"
	"pod/pkg/podcast"
	"pod/pkg/types"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type tuiModel struct {
	// lib is the podcast library this TUI is a view of. Everything that used
	// to reach podcast.DefaultDownloadQueue() goes through it, so the TUI
	// holds library state instead of sharing a process global with the CLI.
	lib *podcast.Library

	// cfg is the application config this session was started with. The TUI
	// used to reload it from disk at seven call sites; a full-screen session
	// should act on the configuration it was launched with, not on whatever
	// the file happens to say mid-run.
	cfg *types.Config

	width                   int
	height                  int
	ready                   bool
	screen                  tuiScreen
	podcasts                []tuiPodcast
	podIdx                  int
	podScroll               int
	epIdx                   int
	epScroll                int
	queue                   map[string][]string
	loading                 bool
	loadErr                 string
	done                    bool
	bk                      *TuiBackend
	podcastsDir             string
	vp                      viewport.Model
	showCover               bool
	searchMode              bool
	searchQuery             string
	showHelp                bool
	popupMsg                string
	popupTimer              int
	marqueeTick             int
	marqueePos              int
	marqueeDir              int
	lastMarqueeSelection    string
	descScroll              int
	prevScreen              tuiScreen
	pqIdx                   int
	pqScroll                int
	pqGrabbed               bool
	adqIdx                  int
	adqScroll               int
	adqGrabbed              bool
	dlqIdx                  int
	dlqScroll               int
	latestIdx               int
	latestScroll            int
	transcriptScroll        int
	transcriptLines         []string
	transcriptItems         []transcriptItem
	transcriptViewMode      int
	transcriptLoadedFor     string
	timelineScroll          int
	transcriptMatchIdx      int
	toast                   *Toast
	showHelpModal           bool
	showPolicyModal         bool
	policyModalIdx          int
	showDownloadPolicyModal bool
	downloadPolicyModalIdx  int
	downloadPolicyModalK    int
	policyAutoDownload      bool
	policyAutoCleanup       bool
	policyCleanupDays       int
	policyKeepPolicy        string
	policyAdRemoval         string
	selectedEpisodes        map[string]bool
	showEpisodePlayerPane   bool
}

type playerTickMsg time.Time

func playerTickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return playerTickMsg(t)
	})
}

type loadedPodcastsMsg struct {
	podcasts []tuiPodcast
	queue    map[string][]string
	err      string
}

type episodeDurationMsg struct {
	idx      int
	duration float64
}

func newTuiModel(bk *TuiBackend, podcastsDir string, cfg *types.Config, lib *podcast.Library) *tuiModel {
	if cfg != nil {
		applyTUIColorConfig(cfg.TUIColor)
	}
	return &tuiModel{
		lib:              lib,
		cfg:              cfg,
		screen:           screenPodcasts,
		loading:          true,
		bk:               bk,
		podcastsDir:      podcastsDir,
		vp:               viewport.New(70, 20),
		showCover:        kitty.IsKittySupported(),
		selectedEpisodes: make(map[string]bool),
	}
}

func (m *tuiModel) Init() tea.Cmd {
	loadCmd := func() tea.Msg {
		pods, err := m.bk.LoadPodcasts(m.podcastsDir)
		if err != nil {
			return loadedPodcastsMsg{err: err.Error()}
		}
		queue := m.bk.LoadQueues(pods)
		return loadedPodcastsMsg{podcasts: pods, queue: queue}
	}
	return tea.Batch(loadCmd, playerTickCmd())
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = max(20, msg.Width-4)
		m.vp.Height = max(5, msg.Height-8)
		m.ready = true

	case loadedPodcastsMsg:
		m.loading = false
		if msg.err != "" {
			m.loadErr = msg.err
		} else {
			m.podcasts = msg.podcasts
			m.queue = msg.queue
			if n := countLoadNotices(msg.podcasts); n > 0 {
				m.showToast(fmt.Sprintf("Quarantined abandoned duplicates in %d podcast(s)", n), ToastWarning)
			}
			if len(msg.podcasts) > 0 && m.showCover {
				prewarmPodcastCovers(msg.podcasts, 24, 7)
			}
		}

	case episodeDurationMsg:
		if m.podIdx >= 0 && m.podIdx < len(m.podcasts) {
			if msg.idx >= 0 && msg.idx < len(m.podcasts[m.podIdx].episodes) {
				ep := &m.podcasts[m.podIdx].episodes[msg.idx]
				ep.duration = msg.duration
				ep.durationDone = true
			}
		}

	case playerTickMsg:
		globalPlayer.UpdatePosition()
		return m, playerTickCmd()

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	default:
		m.tickPopup()
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) visibleLines(headerLines int) int {
	n := m.height - headerLines - 2
	if n < 1 {
		n = 1
	}
	return n
}

func (m *tuiModel) filteredPodcasts() []tuiPodcast {
	if m.searchQuery == "" {
		return m.podcasts
	}
	q := strings.ToLower(m.searchQuery)
	var exact []tuiPodcast
	for _, p := range m.podcasts {
		if strings.Contains(strings.ToLower(p.name), q) || strings.Contains(strings.ToLower(p.displayAuthor()), q) {
			exact = append(exact, p)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	var fuzzy []tuiPodcast
	for _, p := range m.podcasts {
		if matched, _, _ := fuzzyMatch(m.searchQuery, p.name); matched {
			fuzzy = append(fuzzy, p)
		}
	}
	return fuzzy
}

func (m *tuiModel) filteredEpisodes() []tuiEpisode {
	if m.podIdx >= len(m.podcasts) {
		return nil
	}
	if m.searchQuery == "" {
		return m.podcasts[m.podIdx].episodes
	}
	q := strings.ToLower(m.searchQuery)
	var exact []tuiEpisode
	for _, e := range m.podcasts[m.podIdx].episodes {
		if strings.Contains(strings.ToLower(e.displayTitle()), q) || strings.Contains(strings.ToLower(e.filename), q) {
			exact = append(exact, e)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	var fuzzy []tuiEpisode
	for _, e := range m.podcasts[m.podIdx].episodes {
		matched, _, _ := fuzzyMatch(m.searchQuery, e.displayTitle())
		if !matched {
			matched, _, _ = fuzzyMatch(m.searchQuery, e.filename)
		}
		if matched {
			fuzzy = append(fuzzy, e)
		}
	}
	return fuzzy
}

func (m *tuiModel) showPopup(msg string) {
	m.popupMsg = msg
	m.popupTimer = 10
}

func (m *tuiModel) tickPopup() {
	if m.popupTimer > 0 {
		m.popupTimer--
		if m.popupTimer == 0 {
			m.popupMsg = ""
		}
	}
}

func (m *tuiModel) marqueeText(text string, maxWidth int) string {
	if len(text) <= maxWidth {
		return text
	}
	selKey := fmt.Sprintf("%d-%d", m.podIdx, m.epIdx)
	if selKey != m.lastMarqueeSelection {
		m.lastMarqueeSelection = selKey
		m.marqueePos = 0
		m.marqueeDir = 1
		m.marqueeTick = 0
	}
	m.marqueeTick++
	if m.marqueeTick%3 == 0 {
		m.marqueePos += m.marqueeDir
		if m.marqueePos >= len(text)-maxWidth {
			m.marqueeDir = -1
		}
		if m.marqueePos <= 0 {
			m.marqueeDir = 1
		}
	}
	start := m.marqueePos
	if start < 0 {
		start = 0
	}
	if start+maxWidth > len(text) {
		start = len(text) - maxWidth
	}
	return text[start : start+maxWidth]
}

func (m *tuiModel) setTerminalTitle(title string) {
	fmt.Printf("\033]0;%s\007", title)
}

func (m *tuiModel) searchBar() string {
	if !m.searchMode {
		return ""
	}
	prompt := fmt.Sprintf("  Search: %s█", m.searchQuery)
	if m.screen == screenTranscript {
		prompt = fmt.Sprintf("  Search Transcript: %s█", m.searchQuery)
	}
	return tuiSearchStyle.Render(prompt)
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func resolveLocalPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func RunTUI(cfg *types.Config, podcastsDir string) error {
	if podcastsDir == "" && cfg != nil && cfg.PodcastsDir != "" {
		podcastsDir = cfg.PodcastsDir
	}
	if podcastsDir == "" {
		podcastsDir = "~/podcasts"
	}
	podcastsDir = resolveLocalPath(podcastsDir)

	// The backend is built once here. Reaching it through the Library is what
	// lets the screens stop constructing their own.
	b, _ := backend.FromAppConfig(cfg, nil)
	lib := podcast.Open(podcast.Config{
		PodcastsDir:       podcastsDir,
		SubscriptionsFile: cfg.SubscriptionsFile,
		ServerBaseURL:     cfg.ServerBaseURL,
	}, b, nil)

	bk := &TuiBackend{
		LoadPodcasts: func(dir string) ([]tuiPodcast, error) {
			return loadTUIPodcastsABS(dir, b)
		},
		LoadQueues: loadAllQueues,
		SaveQueue: func(dir string, entries []string) {
			_ = saveQueue(dir, entries)
		},
		GetDuration: audio.GetAudioDuration,
	}

	p := tea.NewProgram(newTuiModel(bk, podcastsDir, cfg, lib), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func countLoadNotices(podcasts []tuiPodcast) int {
	n := 0
	for _, p := range podcasts {
		if p.notice != "" {
			n++
		}
	}
	return n
}

func prewarmPodcastCovers(podcasts []tuiPodcast, cols, rows int) {
	go func() {
		for _, pod := range podcasts {
			cp := pod.coverPath
			if cp == "" {
				cp = kitty.FindCoverImage(pod.dir)
			}
			if cp != "" {
				_, _ = kitty.EncodeKittyGraphicsFile(cp, cols, rows)
			}
		}
	}()
}
