package podcast

import (
	"os"
	"path/filepath"
	"strings"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/types"
)

// FrequencyOptions controls a release-frequency analysis run.
type FrequencyOptions struct {
	// Refresh re-reads each feed instead of using the cached episode list.
	Refresh bool

	// DisableHourly rewrites the download and ad-removal policy of anything
	// found to publish hourly, so a firehose feed stops filling the library.
	DisableHourly bool
}

// pathSafeTitle replaces the characters a filesystem will not accept in a name,
// and nothing else.
//
// It is deliberately much gentler than SanitizeTitle, which strips a title down
// to letters, digits and underscores for matching. This one is used to create a
// podcast directory, where spaces and punctuation should survive.
func pathSafeTitle(title string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, title)
}

// AnalyzeFrequencies works out how often each podcast publishes, records the
// result in each podcast's configuration, and — when asked — switches off
// downloads for the ones publishing hourly.
func (l *Library) AnalyzeFrequencies(items []backend.Podcast, opts FrequencyOptions) []PodcastFreqResult {
	results := make([]PodcastFreqResult, len(items))
	for i, item := range items {
		results[i] = l.analyzeOneFrequency(item, opts)
	}
	_ = l.feedCache.Save()
	return results
}

func (l *Library) analyzeOneFrequency(item backend.Podcast, opts FrequencyOptions) PodcastFreqResult {
	title := item.Media.Metadata.Title
	if title == "" {
		title = item.ID
	}
	eps, err := getEpisodesForFrequency(l.backend, item, l.cfg.PodcastsDir, opts.Refresh, nil)
	if err != nil {
		return PodcastFreqResult{Title: title, Item: item, Err: err}
	}

	freq := backend.AnalyzePodcastFrequency(eps)
	podDir := l.frequencyPodcastDir(item, title, freq, opts)
	if podDir == "" {
		return PodcastFreqResult{Title: title, Item: item, Freq: freq}
	}

	podCfg := config.LoadPodcastConfig(podDir, config.DefaultPodcastConfig(nil))
	podCfg.Frequency = &freq
	disabled := false
	if opts.DisableHourly && freq.Type == string(backend.CadenceHourly) {
		l.disableHourlyPodcast(item, &podCfg)
		disabled = true
	}
	saved := config.SavePodcastConfig(podDir, podCfg) == nil

	return PodcastFreqResult{
		Title:       title,
		Item:        item,
		Freq:        freq,
		PodDir:      podDir,
		Disabled:    disabled,
		PolicySaved: saved,
	}
}

// frequencyPodcastDir locates the podcast's directory, creating one for an
// hourly podcast that has none: without a directory there is nowhere to record
// that its downloads were switched off, so the next run would switch them off
// again.
func (l *Library) frequencyPodcastDir(item backend.Podcast, title string, freq types.PodcastFrequencyInfo, opts FrequencyOptions) string {
	if dir := findPodcastDirForItem(item, l.cfg.PodcastsDir); dir != "" {
		return dir
	}
	if !opts.DisableHourly || freq.Type != string(backend.CadenceHourly) || l.cfg.PodcastsDir == "" {
		return ""
	}
	candidate := filepath.Join(l.cfg.PodcastsDir, strings.TrimSpace(pathSafeTitle(title)))
	if err := os.MkdirAll(candidate, 0755); err != nil {
		return ""
	}
	return candidate
}

func (l *Library) disableHourlyPodcast(item backend.Podcast, podCfg *config.PodcastConfig) {
	podCfg.DownloadPolicy = config.DownloadPolicyNone
	autoDl := false
	podCfg.AutoDownload = &autoDl
	podCfg.AdRemoval = config.AdRemovalNone

	if l.backend == nil {
		return
	}
	targetID := item.ID
	if targetID == "" {
		targetID = item.Media.ID
	}
	_ = l.backend.UpdatePodcastSettings(targetID, false, podCfg.IsAutoCleanupEnabled(), podCfg.AutoCleanupDays)
}
