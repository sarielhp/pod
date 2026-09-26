package tui

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/util"

	"github.com/charmbracelet/lipgloss"
)

var (
	testSyncPolicyMu   util.SyncMutex
	testSyncPolicyHook func(pod *tuiPodcast)
)

func getTestSyncPolicyHook() func(pod *tuiPodcast) {
	testSyncPolicyMu.Lock()
	defer testSyncPolicyMu.Unlock()
	return testSyncPolicyHook
}

func setTestSyncPolicyHook(h func(pod *tuiPodcast)) {
	testSyncPolicyMu.Lock()
	defer testSyncPolicyMu.Unlock()
	testSyncPolicyHook = h
}

func (m *tuiModel) drawDownloadPolicyModal() string {
	if m.podIdx >= len(m.podcasts) {
		return ""
	}
	pod := m.podcasts[m.podIdx]
	boxWidth := min(72, max(44, m.width-8))

	var lines []string
	lines = append(lines, tuiTitleStyle.Render("PODCAST POLICY"))
	lines = append(lines, tuiSubtitleStyle.Render(truncate("for "+util.DisplayName(pod.name), boxWidth-6)))
	lines = append(lines, tuiDividerStyle.Render(strings.Repeat("─", boxWidth-4)))
	lines = append(lines, "")

	dlVal := "[ Disabled ]"
	if m.policyAutoDownload {
		dlVal = "[ Enabled ]"
	}

	clVal := "[ Disabled ]"
	if m.policyAutoCleanup {
		clVal = "[ Enabled ]"
	}

	daysVal := "[ Keep Always (∞) ]"
	if m.policyAutoCleanup && m.policyCleanupDays > 0 {
		switch m.policyCleanupDays {
		case 30:
			daysVal = "[ Regular (30 days) ]"
		case 180:
			daysVal = "[ Favorite (6 months) ]"
		case 1:
			daysVal = "[ Hourly News (1 day) ]"
		default:
			daysVal = fmt.Sprintf("[ %d days ]", m.policyCleanupDays)
		}
	}

	adVal := "[ None ]"
	switch m.policyAdRemoval {
	case AdRemovalAll:
		adVal = "[ All Episodes ]"
	case AdRemovalLatest:
		adVal = "[ Latest Only ]"
	}

	items := []struct {
		num   int
		title string
		val   string
		desc  string
	}{
		{1, "Auto-Download", dlVal, "Fetch new episodes automatically from feed"},
		{2, "Auto-Cleanup", clVal, "Delete older episodes past retention period"},
		{3, "Cleanup Days", daysVal, "Days to keep: Always (∞), Regular (30d), Favorite (180d), Hourly (1d) (+/-)"},
		{4, "Ad Removal", adVal, "Commercial ad detection & removal (all, latest, none)"},
	}

	for i, it := range items {
		isSel := (i == m.downloadPolicyModalIdx)
		prefix := "( )"
		if isSel {
			prefix = "(•)"
		}
		lineTitle := fmt.Sprintf("%s %d. %-14s %s", prefix, it.num, it.title+":", it.val)
		if isSel {
			lines = append(lines, tuiSelectedStyle.Render(" "+lineTitle+" "))
		} else {
			lines = append(lines, tuiLabelStyle.Render("  "+lineTitle))
		}
		lines = append(lines, tuiDimStyle.Render("      "+it.desc))
		lines = append(lines, "")
	}

	lines = append(lines, tuiDividerStyle.Render(strings.Repeat("─", boxWidth-4)))
	lines = append(lines, tuiDimStyle.Render("↑/↓ Select │ Space/1-4 Toggle │ +/- Adjust Days │ x Prune Expired │ Enter Apply │ Esc Cancel"))

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)
}

func (m *tuiModel) openDownloadPolicyModal() {
	if m.podIdx >= len(m.podcasts) {
		return
	}
	pod := &m.podcasts[m.podIdx]
	m.policyAutoDownload = pod.config.IsAutoDownloadEnabled()
	m.policyKeepPolicy = pod.config.EffectiveKeepPolicy()
	m.policyCleanupDays = pod.config.EffectiveCleanupDays()
	m.policyAutoCleanup = (m.policyKeepPolicy != config.KeepPolicyAlways && m.policyCleanupDays > 0)
	if !m.policyAutoCleanup {
		m.policyCleanupDays = -1
	}
	m.policyAdRemoval = config.NormalizeAdRemovalMode(pod.config.AdRemoval)
	m.downloadPolicyModalIdx = 0
	m.downloadPolicyModalK = pod.config.DownloadK
	if m.downloadPolicyModalK <= 0 {
		m.downloadPolicyModalK = 3
	}
	m.showDownloadPolicyModal = true
}

func (m *tuiModel) togglePolicyModalField(idx int) {
	switch idx {
	case 0:
		m.policyAutoDownload = !m.policyAutoDownload
	case 1:
		m.policyAutoCleanup = !m.policyAutoCleanup
		if m.policyAutoCleanup && m.policyCleanupDays <= 0 {
			m.policyCleanupDays = 30
			m.policyKeepPolicy = config.KeepPolicyMonth
		} else if !m.policyAutoCleanup {
			m.policyCleanupDays = -1
			m.policyKeepPolicy = config.KeepPolicyAlways
		}
	case 2:
		m.cycleKeepPolicyPreset()
	case 3:
		m.policyAdRemoval = config.CycleAdRemovalMode(m.policyAdRemoval)
	}
}

func (m *tuiModel) cycleKeepPolicyPreset() {
	if !m.policyAutoCleanup || m.policyCleanupDays <= 0 {
		m.policyAutoCleanup = true
		m.policyCleanupDays = 30
		m.policyKeepPolicy = config.KeepPolicyMonth
		return
	}
	switch m.policyCleanupDays {
	case 30:
		m.policyCleanupDays = 180
		m.policyKeepPolicy = config.KeepPolicyFavorite
	case 180:
		m.policyCleanupDays = 1
		m.policyKeepPolicy = config.KeepPolicyHourly
	default:
		m.policyAutoCleanup = false
		m.policyCleanupDays = -1
		m.policyKeepPolicy = config.KeepPolicyAlways
	}
}

func (m *tuiModel) adjustPolicyModalField(delta int) {
	switch m.downloadPolicyModalIdx {
	case 0:
		m.policyAutoDownload = (delta > 0)
	case 1:
		m.policyAutoCleanup = (delta > 0)
		if m.policyAutoCleanup && m.policyCleanupDays <= 0 {
			m.policyCleanupDays = 30
			m.policyKeepPolicy = config.KeepPolicyMonth
		} else if !m.policyAutoCleanup {
			m.policyCleanupDays = -1
			m.policyKeepPolicy = config.KeepPolicyAlways
		}
	case 2:
		m.adjustCleanupDaysField(delta)
	case 3:
		m.policyAdRemoval = config.CycleAdRemovalMode(m.policyAdRemoval)
	}
}

func (m *tuiModel) adjustCleanupDaysField(delta int) {
	if delta > 0 {
		m.policyAutoCleanup = true
		if m.policyCleanupDays <= 0 {
			m.policyCleanupDays = 1
		} else if m.policyCleanupDays < 3650 {
			m.policyCleanupDays += delta
		}
	} else {
		if m.policyCleanupDays > 1 {
			m.policyCleanupDays += delta
		} else {
			m.policyCleanupDays = -1
			m.policyAutoCleanup = false
		}
	}
	m.updateKeepPolicyFromDays()
}

func (m *tuiModel) updateKeepPolicyFromDays() {
	if !m.policyAutoCleanup || m.policyCleanupDays <= 0 {
		m.policyKeepPolicy = config.KeepPolicyAlways
		return
	}
	switch m.policyCleanupDays {
	case 30:
		m.policyKeepPolicy = config.KeepPolicyMonth
	case 180:
		m.policyKeepPolicy = config.KeepPolicyFavorite
	case 1:
		m.policyKeepPolicy = config.KeepPolicyHourly
	default:
		m.policyKeepPolicy = fmt.Sprintf("%dd", m.policyCleanupDays)
	}
}

func (m *tuiModel) applyDownloadPolicyModal() {
	if m.podIdx >= len(m.podcasts) {
		m.showDownloadPolicyModal = false
		return
	}
	pod := &m.podcasts[m.podIdx]
	pod.config.SetAutoDownload(m.policyAutoDownload)
	if !m.policyAutoCleanup || m.policyCleanupDays <= 0 {
		pod.config.SetAutoCleanup(false)
		pod.config.AutoCleanupDays = -1
		pod.config.SetKeepPolicy(config.KeepPolicyAlways)
	} else {
		pod.config.SetAutoCleanup(true)
		pod.config.AutoCleanupDays = m.policyCleanupDays
		if m.policyKeepPolicy != "" {
			pod.config.SetKeepPolicy(m.policyKeepPolicy)
		} else {
			m.updateKeepPolicyFromDays()
			pod.config.SetKeepPolicy(m.policyKeepPolicy)
		}
	}
	pod.config.AdRemoval = m.policyAdRemoval
	if m.downloadPolicyModalK > 0 {
		pod.config.DownloadK = m.downloadPolicyModalK
	}

	if err := config.SavePodcastConfig(pod.dir, pod.config); err != nil {
		m.showToast("Failed to save config: "+err.Error(), ToastError)
	} else {
		keepDesc := config.KeepPolicyLabel(pod.config.EffectiveKeepPolicy(), pod.config.EffectiveCleanupDays())
		m.showToast("Policy saved: DL="+boolStatus(m.policyAutoDownload)+", Keep="+keepDesc+", Ads="+m.policyAdRemoval, ToastSuccess)
	}

	autoDownload := m.policyAutoDownload
	autoCleanup := m.policyAutoCleanup
	cleanupDays := pod.config.AutoCleanupDays
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic syncing policy to backend: %v\n%s", r, debug.Stack())
			}
		}()
		syncPolicyToBackend(m.lib.Backend(), pod, autoDownload, autoCleanup, cleanupDays)
	}()
	m.showDownloadPolicyModal = false
}

func (m *tuiModel) pruneSelectedPodcastKeepPolicy() {
	if m.podIdx >= len(m.podcasts) {
		return
	}
	pod := &m.podcasts[m.podIdx]
	res, err := podcast.ApplyPodcastKeepPolicy(pod.dir, pod.name, pod.config, time.Now(), false)
	if err != nil {
		m.showToast("Prune error: "+err.Error(), ToastError)
		return
	}
	if res.DeletedEpisodes == 0 {
		m.showToast("No expired episodes to prune (all within keep policy)", ToastInfo)
		return
	}
	if refreshed := loadSingleTUIPodcast(pod.dir, pod.name); refreshed != nil {
		pod.episodes = refreshed.episodes
	} else {
		pod.episodes = nil
	}
	savePodcastToCache(pod)
	freedMB := float64(res.FreedBytes) / (1024 * 1024)
	m.showToast(fmt.Sprintf("Pruned %d expired episode(s), freed %.1f MB (transcripts kept)", res.DeletedEpisodes, freedMB), ToastSuccess)
}

func syncPolicyToBackend(b backend.Backend, pod *tuiPodcast, autoDownload, autoCleanup bool, autoCleanupDays int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in syncPolicyToBackend: %v\n%s", r, debug.Stack())
		}
	}()
	if hook := getTestSyncPolicyHook(); hook != nil {
		hook(pod)
		return
	}
	if b == nil {
		return
	}
	id := pod.config.ID
	if id == "" && pod.absData != nil {
		id = pod.absData.ID
	}
	if id == "" {
		id = filepath.Base(pod.dir)
	}
	_ = b.UpdatePodcastSettings(id, autoDownload, autoCleanup, autoCleanupDays)
}

func boolStatus(b bool) string {
	if b {
		return "Enabled"
	}
	return "Disabled"
}
