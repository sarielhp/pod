package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

func loadTUIPodcasts(podcastsDir string) ([]tuiPodcast, error) {
	entries, err := os.ReadDir(podcastsDir)
	if err != nil {
		return nil, err
	}

	var podcasts []tuiPodcast
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".work" || strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), "-1") {
			continue
		}
		podDir := filepath.Join(podcastsDir, entry.Name())
		if pod := loadSingleTUIPodcast(podDir, entry.Name()); pod != nil {
			podcasts = append(podcasts, *pod)
		}
	}

	sort.Slice(podcasts, func(i, j int) bool {
		return strings.ToLower(podcasts[i].name) < strings.ToLower(podcasts[j].name)
	})

	return podcasts, nil
}

func loadSingleTUIPodcast(podDir, name string) *tuiPodcast {
	_ = episode.EnsureABSIgnore(podDir)
	pod := tuiPodcast{
		name:   name,
		dir:    podDir,
		config: config.LoadPodcastConfig(podDir, config.DefaultDiscoveredPodcastConfig(nil)),
	}

	cachedIdx, _ := podcast.LoadPodcastCache(podDir)
	cachedByPath := make(map[string]podcast.CachedEpisodeSummary)
	cachedByName := make(map[string]podcast.CachedEpisodeSummary)
	if cachedIdx != nil {
		pod.author = cachedIdx.Author
		pod.description = cachedIdx.Description
		pod.feedURL = cachedIdx.FeedURL
		pod.coverPath = cachedIdx.CoverPath
		for _, ce := range cachedIdx.Episodes {
			if ce.Path != "" {
				cachedByPath[ce.Path] = ce
			}
			if ce.Filename != "" {
				cachedByName[ce.Filename] = ce
			}
		}
	}

	mp3Files := util.FindMP3Files(podDir)
	if len(mp3Files) == 0 {
		return nil
	}

	var episodes []tuiEpisode
	for _, mp3 := range mp3Files {
		episodes = append(episodes, loadSingleTUIEpisode(mp3, cachedByPath, cachedByName))
	}

	sort.Slice(episodes, func(i, j int) bool {
		return episodes[i].displayDate().After(episodes[j].displayDate())
	})

	pod.episodes = episodes
	return &pod
}

func loadSingleTUIEpisode(mp3 string, cachedByPath, cachedByName map[string]podcast.CachedEpisodeSummary) tuiEpisode {
	absPath, _ := filepath.Abs(mp3)
	base := strings.TrimSuffix(mp3, ".mp3")
	hasCut := util.FileExists(base + ".cuts.json")
	hasTx := util.FileExists(base+".transcript.json") || util.FileExists(base+".transcript.txt")
	var fSize int64
	var modTime time.Time
	if fi, err := os.Stat(mp3); err == nil {
		fSize = fi.Size()
		modTime = fi.ModTime()
	}
	fn := filepath.Base(mp3)
	title := ""
	if strings.EqualFold(fn, "podcast.mp3") {
		title = filepath.Base(filepath.Dir(mp3))
		fn = title + ".mp3"
	}
	ep := tuiEpisode{
		filename:      fn,
		path:          absPath,
		title:         title,
		hasAdsRemoved: hasCut,
		hasTranscript: hasTx,
		fileSize:      fSize,
		modTime:       modTime,
	}
	if ce, ok := cachedByPath[absPath]; ok {
		applyCachedSummaryToEpisode(&ep, ce)
	} else if ce, ok := cachedByName[fn]; ok {
		applyCachedSummaryToEpisode(&ep, ce)
	}
	if id := episode.LoadIdentity(mp3); id != nil {
		if id.Title != "" {
			ep.title = id.Title
		}
		if ep.publishedAt == 0 {
			ep.publishedAt = id.PublishedAt
		}
	}
	if date, ok := podcast.SourcePublicationTime(mp3); ok {
		ep.publishedAt = 0
		if !date.IsZero() {
			ep.publishedAt = date.UnixMilli()
		}
	}
	return ep
}

func applyCachedSummaryToEpisode(ep *tuiEpisode, ce podcast.CachedEpisodeSummary) {
	ep.title = ce.Title
	ep.publishedAt = ce.PublishedAt
	ep.duration = ce.DurationSec
	ep.season = ce.Season
	ep.episode = ce.Episode
	if ce.HasTranscript {
		ep.hasTranscript = true
	}
}

func savePodcastToCache(pod *tuiPodcast) {
	if pod == nil {
		return
	}
	absPodDir, _ := filepath.Abs(pod.dir)
	var summaries []podcast.CachedEpisodeSummary
	for _, ep := range pod.episodes {
		sum := buildCachedEpisodeSummary(ep, pod.dir, pod.absData)
		summaries = append(summaries, sum)
	}

	absItemID := ""
	author := pod.author
	description := pod.description
	feedURL := pod.feedURL
	coverPath := pod.coverPath
	if pod.absData != nil {
		absItemID = pod.absData.ID
		if pod.absData.Media.Metadata.Author != "" {
			author = pod.absData.Media.Metadata.Author
		}
		if pod.absData.Media.Metadata.Description != "" {
			description = pod.absData.Media.Metadata.Description
		}
		if pod.absData.Media.Metadata.FeedURL != "" {
			feedURL = pod.absData.Media.Metadata.FeedURL
		}
	}
	index := podcast.CachedPodcastIndex{
		PodcastName: pod.name,
		PodcastDir:  absPodDir,
		ABSItemID:   absItemID,
		Author:      author,
		Description: description,
		FeedURL:     feedURL,
		CoverPath:   coverPath,
		UpdatedAt:   time.Now(),
		Episodes:    summaries,
	}
	_ = podcast.SavePodcastCache(pod.dir, &index)
}

func buildCachedEpisodeSummary(ep tuiEpisode, podDir string, podABSData *backend.Podcast) podcast.CachedEpisodeSummary {
	absPath, _ := filepath.Abs(ep.path)
	title := ep.displayTitle()
	pubAt := ep.publishedAt
	if date, ok := podcast.SourcePublicationTime(ep.path); ok {
		pubAt = 0
		if !date.IsZero() {
			pubAt = date.UnixMilli()
		}
	}
	dur := ep.duration
	season := ep.season
	epNum := ep.episode
	if ep.absData != nil {
		if ep.absData.Title != "" {
			title = ep.absData.Title
		}
		if pub := episode.ParseABSEpisodePublishedAt(ep.absData); pub > 0 {
			pubAt = pub
		}
		if ep.absData.Duration > 0 {
			dur = ep.absData.Duration
		}
		if ep.absData.Season != "" {
			season = ep.absData.Season
		}
		if ep.absData.Episode != "" {
			epNum = ep.absData.Episode
		}

		det := podcast.CachedEpisodeDetails{
			Path:        absPath,
			Filename:    ep.filename,
			Title:       title,
			Description: ep.absData.Description,
			Subtitle:    ep.absData.Subtitle,
			EpisodeType: ep.absData.EpisodeType,
			RawABS:      ep.absData,
		}
		if podABSData != nil {
			det.Author = podABSData.Media.Metadata.Author
			det.FeedURL = podABSData.Media.Metadata.FeedURL
		}
		_ = podcast.SaveEpisodeDetails(podDir, ep.filename, &det)
	}

	return podcast.CachedEpisodeSummary{
		EpisodeFile: podcast.EpisodeFile{
			Path:        absPath,
			Filename:    ep.filename,
			Title:       title,
			PublishedAt: pubAt,
			DurationSec: dur,
			SizeBytes:   ep.fileSize,
		},
		Season:        season,
		Episode:       epNum,
		HasAdsRemoved: ep.hasAdsRemoved,
		HasTranscript: ep.hasTranscript,
	}
}
