package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"pod/pkg/config"
	"pod/pkg/format"
	"pod/pkg/pipeline"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"sort"
	"strings"
	"time"
)

type PodcastInfoJSON struct {
	ID        string `json:"id"`
	UUID      string `json:"uuid,omitempty"`
	Title     string `json:"title"`
	Directory string `json:"directory"`
	Author    string `json:"author,omitempty"`
	FeedURL   string `json:"feed_url,omitempty"`

	// LocalFeedURL and LocalPageURL are where this show is published for
	// subscribing to. They are what a person actually needs from this command
	// — the address to paste into a podcast client — and were the one thing
	// it did not print.
	LocalFeedURL       string             `json:"local_feed_url,omitempty"`
	LocalPageURL       string             `json:"local_page_url,omitempty"`
	CoverPath          string             `json:"cover_path,omitempty"`
	Description        string             `json:"description,omitempty"`
	AutoDownload       bool               `json:"auto_download"`
	DownloadPolicy     string             `json:"download_policy"`
	DownloadK          int                `json:"download_k"`
	AutoCleanup        bool               `json:"auto_cleanup"`
	AutoCleanupDays    int                `json:"auto_cleanup_days"`
	AdRemoval          string             `json:"ad_removal"`
	TotalEpisodes      int                `json:"total_episodes"`
	CleanEpisodes      int                `json:"clean_episodes"`
	TotalDurationSec   float64            `json:"total_duration_sec"`
	TotalDiskSizeBytes int64              `json:"total_disk_size_bytes"`
	RecentEpisodes     []RecentEpisodeDTO `json:"recent_episodes,omitempty"`
}

type RecentEpisodeDTO struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Date     string `json:"date"`
	Status   string `json:"status"`
	Duration string `json:"duration"`
}

func inspectPodcastInfo(pod *ResolvedPodcast, cli CLIOptions, baseURL string) error {
	dto := buildPodcastInfoDTO(pod, cli.Count, baseURL)

	if cli.JSON {
		data, err := json.MarshalIndent(dto, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(outFor(cli), string(data))
		return nil
	}

	printPodcastInfoCard(outFor(cli), dto)
	return nil
}

func collectPodcastStatsAndRecent(pod *ResolvedPodcast, mp3s []string, maxEpisodes int) (int, float64, int64, []RecentEpisodeDTO) {
	cleanCount := 0
	var totalDur float64
	var totalSize int64

	type epTime struct {
		path string
		pt   time.Time
		fi   os.FileInfo
	}
	var epList []epTime

	for _, mp3 := range mp3s {
		fi, err := os.Stat(mp3)
		if err == nil {
			totalSize += fi.Size()
		}
		if pipeline.IsEpisodeClean(mp3) {
			cleanCount++
		}
		st := pipeline.GetOrCreateEpisodeStatus(mp3)
		od, _ := pipeline.EpisodeDurations(mp3, st)
		totalDur += od

		pt := resolveEpisodePublicationTime(mp3, st, fi)
		epList = append(epList, epTime{path: mp3, pt: pt, fi: fi})
	}

	sort.Slice(epList, func(i, j int) bool {
		return epList[i].pt.After(epList[j].pt)
	})

	var recent []RecentEpisodeDTO
	limit := 5
	if maxEpisodes > 0 {
		limit = maxEpisodes
	}
	if limit > len(epList) {
		limit = len(epList)
	}
	for i := 0; i < limit; i++ {
		mp3 := epList[i].path
		epID := podcast.GetOrSetEpisodeShortID(pod.Dir, pod.ShortID, mp3)
		st, _ := getEpisodeStatusLabel(mp3)
		od, _ := pipeline.EpisodeDurations(mp3, pipeline.GetOrCreateEpisodeStatus(mp3))
		recent = append(recent, RecentEpisodeDTO{
			ID:       epID,
			Title:    podcast.EpisodeTitleFromPath(mp3),
			Date:     publicationDateTime(epList[i].pt),
			Status:   formatShortStatus(st),
			Duration: format.FormatClock(od),
		})
	}
	return cleanCount, totalDur, totalSize, recent
}

func getPodcastMetadataFields(pod *ResolvedPodcast) (string, string, string, string, string) {
	cached, _ := podcast.LoadPodcastCache(pod.Dir)
	author, feedURL, desc, uuid := "", "", "", pod.UUID
	coverPath := findCoverImageInDir(pod.Dir)

	if cached != nil {
		author = cached.Author
		feedURL = cached.FeedURL
		desc = cached.Description
		if cached.ABSItemID != "" {
			uuid = cached.ABSItemID
		}
		if coverPath == "" && cached.CoverPath != "" {
			coverPath = cached.CoverPath
		}
	}
	return author, feedURL, coverPath, desc, uuid
}

func buildPodcastInfoDTO(pod *ResolvedPodcast, maxEpisodes int, baseURL string) PodcastInfoJSON {
	mp3s := util.FindMP3Files(pod.Dir)
	cleanCount, totalDur, totalSize, recent := collectPodcastStatsAndRecent(pod, mp3s, maxEpisodes)
	author, feedURL, coverPath, desc, uuid := getPodcastMetadataFields(pod)

	autoDl := pod.Config.IsAutoDownloadEnabled()
	autoCl := pod.Config.IsAutoCleanupEnabled()

	return PodcastInfoJSON{
		ID:                 pod.ShortID,
		UUID:               uuid,
		Title:              pod.Title,
		Directory:          pod.Dir,
		Author:             author,
		FeedURL:            feedURL,
		LocalFeedURL:       publishedURL(baseURL, pod.Dir, "feed.xml"),
		LocalPageURL:       publishedURL(baseURL, pod.Dir, "index.html"),
		CoverPath:          coverPath,
		Description:        desc,
		AutoDownload:       autoDl,
		DownloadPolicy:     pod.Config.DownloadPolicy,
		DownloadK:          pod.Config.DownloadK,
		AutoCleanup:        autoCl,
		AutoCleanupDays:    pod.Config.AutoCleanupDays,
		AdRemoval:          pod.Config.AdRemoval,
		TotalEpisodes:      len(mp3s),
		CleanEpisodes:      cleanCount,
		TotalDurationSec:   totalDur,
		TotalDiskSizeBytes: totalSize,
		RecentEpisodes:     recent,
	}
}

// publishedURL is where one of a show's published files is served, or empty
// when no base URL is configured and nothing is being served at all.
func publishedURL(baseURL, podDir, name string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" || podDir == "" {
		return ""
	}
	return base + "/" + url.PathEscape(filepath.Base(podDir)) + "/" + name
}

func formatPodcastInfo(info PodcastInfoJSON) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s\n", strings.Repeat("=", 80)))
	sb.WriteString(fmt.Sprintf("Podcast: %s [%s]\n", util.Bold(util.DisplayName(info.Title)), util.BoldCyan(info.ID)))
	sb.WriteString(fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
	sb.WriteString(fmt.Sprintf("  Short ID:         %s\n", util.BoldCyan(info.ID)))
	if info.UUID != "" {
		sb.WriteString(fmt.Sprintf("  UUID:             %s\n", info.UUID))
	}
	sb.WriteString(fmt.Sprintf("  Directory:        %s\n", info.Directory))
	if info.Author != "" {
		sb.WriteString(fmt.Sprintf("  Author:           %s\n", util.DisplayName(info.Author)))
	}
	if info.FeedURL != "" {
		sb.WriteString(fmt.Sprintf("  Source Feed:      %s\n", info.FeedURL))
	}
	if info.LocalFeedURL != "" {
		sb.WriteString(fmt.Sprintf("  Subscribe (RSS):  %s\n", util.BoldCyan(info.LocalFeedURL)))
	}
	if info.LocalPageURL != "" {
		sb.WriteString(fmt.Sprintf("  Web Page:         %s\n", info.LocalPageURL))
	}
	if info.CoverPath != "" {
		sb.WriteString(fmt.Sprintf("  Cover:            %s\n", info.CoverPath))
	}

	sb.WriteString("\n  Policy & Sync:\n")
	dlBadge := config.DownloadPolicyBadge(info.DownloadPolicy, info.DownloadK)
	sb.WriteString(fmt.Sprintf("    Auto Download:  %v %s\n", info.AutoDownload, dlBadge))
	retStr := "Disabled"
	if info.AutoCleanupDays > 0 {
		retStr = fmt.Sprintf("%dd retention", info.AutoCleanupDays)
	}
	sb.WriteString(fmt.Sprintf("    Auto Cleanup:   %v (%s)\n", info.AutoCleanup, retStr))
	adBadge := config.AdRemovalModeBadge(info.AdRemoval)
	sb.WriteString(fmt.Sprintf("    AdR Policy:     %s %s\n", config.AdRemovalModeLabel(info.AdRemoval), adBadge))

	sb.WriteString("\n  Library Stats:\n")
	cleanPct := 0.0
	if info.TotalEpisodes > 0 {
		cleanPct = float64(info.CleanEpisodes) / float64(info.TotalEpisodes) * 100
	}
	sb.WriteString(fmt.Sprintf("    Episodes:       %d total (%d clean, %.1f%% clean)\n", info.TotalEpisodes, info.CleanEpisodes, cleanPct))
	sb.WriteString(fmt.Sprintf("    Total Duration: %s\n", formatDurationHours(info.TotalDurationSec)))
	sb.WriteString(fmt.Sprintf("    Disk Usage:     %s\n", formatDiskSize(info.TotalDiskSizeBytes)))

	if info.Description != "" {
		sb.WriteString("\n  Description:\n")
		formatted := cleanAndFormatNotes(info.Description, 4, 76)
		sb.WriteString(formatted + "\n")
	}

	if len(info.RecentEpisodes) > 0 {
		sb.WriteString("\n  Recent Episodes:\n")
		for _, ep := range info.RecentEpisodes {
			statusStr := fmt.Sprintf("[%s]", ep.Status)
			sb.WriteString(fmt.Sprintf("    %-6s  %s  %s  %s  %s\n",
				util.BoldCyan(ep.ID),
				util.PadRight(ep.Date, 16),
				util.PadRight(statusStr, 12),
				util.PadRight(ep.Duration, 8),
				util.TruncateDisplayName(ep.Title, 35)))
		}
	}
	sb.WriteString(fmt.Sprintf("%s\n\n", strings.Repeat("=", 80)))
	return sb.String()
}

func printPodcastInfoCard(w io.Writer, info PodcastInfoJSON) {
	fmt.Fprint(w, formatPodcastInfo(info))
}
