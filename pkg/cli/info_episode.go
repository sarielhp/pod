package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"pod/pkg/episode"
	"pod/pkg/format"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"strings"
)

type episodeInfoJSON struct {
	ID                  string           `json:"id"`
	PodcastID           string           `json:"podcast_id"`
	PodcastTitle        string           `json:"podcast_title"`
	Title               string           `json:"title"`
	PublishedDate       string           `json:"published_date"`
	AudioPath           string           `json:"audio_path"`
	FileSizeBytes       int64            `json:"file_size_bytes"`
	FileSizeFormatted   string           `json:"file_size_formatted"`
	Status              string           `json:"status"`
	OriginalDurationSec float64          `json:"original_duration_sec"`
	CleanDurationSec    float64          `json:"clean_duration_sec"`
	PercentReduction    float64          `json:"percent_reduction"`
	Favorite            bool             `json:"favorite"`
	HasTranscript       bool             `json:"has_transcript"`
	TranscriptPath      string           `json:"transcript_path,omitempty"`
	TranscriptSegments  int              `json:"transcript_segments,omitempty"`
	Description         string           `json:"description,omitempty"`
	Cuts                []episodeCutDTO  `json:"cuts,omitempty"`
	Files               []episodeFileDTO `json:"files,omitempty"`
}

type episodeFileDTO struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type episodeCutDTO struct {
	StartFormatted string  `json:"start"`
	EndFormatted   string  `json:"end"`
	DurationSec    float64 `json:"duration_sec"`
	DurationStr    string  `json:"duration"`
	Reason         string  `json:"reason,omitempty"`
}

func runTranscriptForEpisode(ep *ResolvedEpisode, cli CLIOptions) error {
	jsonPath := util.StripExt(ep.Path) + ".transcript.json"
	if _, err := os.Stat(jsonPath); err != nil {
		return fmt.Errorf("transcript file not found for episode [%s]: %s", ep.ShortID, jsonPath)
	}

	exportFormat := strings.ToLower(cli.ExportFormat)
	if cli.ExportTXT {
		exportFormat = "txt"
	} else if cli.ExportSRT {
		exportFormat = "srt"
	}

	if exportFormat == "txt" {
		out, _ := format.ConvertJSONToTXT(jsonPath, nil, 0, cli.Output, cli.Quiet)
		fmt.Fprintf(progressFor(cli), "Exported TXT: %s\n", out)
		return nil
	}

	if exportFormat == "srt" {
		out, _ := format.ConvertJSONToSRT(jsonPath, nil, cli.Output, cli.Quiet)
		fmt.Fprintf(progressFor(cli), "Exported SRT: %s\n", out)
		return nil
	}

	return printTranscriptText(outFor(cli), jsonPath)
}

func inspectEpisodeInfo(ep *ResolvedEpisode, cli CLIOptions) error {
	dto := buildEpisodeInfoDTO(ep)

	if cli.JSON {
		data, err := json.MarshalIndent(dto, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(outFor(cli), string(data))
		return nil
	}

	printEpisodeInfoCard(outFor(cli), dto, cli.ShowCuts)
	return nil
}

func getEpisodeTranscriptInfo(epPath string) (bool, string, int) {
	txPath := util.StripExt(epPath) + ".transcript.json"
	hasTx := false
	txSegments := 0
	if data, err := os.ReadFile(txPath); err == nil {
		hasTx = true
		var td TranscriptionData
		if json.Unmarshal(data, &td) == nil {
			txSegments = len(td.Segments)
		}
	}
	return hasTx, txPath, txSegments
}

func collectEpisodeCuts(epPath string, st *EpisodeStatusFile) []episodeCutDTO {
	var cuts []episodeCutDTO
	cutsFile := util.StripExt(epPath) + ".cuts.json"
	if data, err := os.ReadFile(cutsFile); err == nil {
		var cd CutsData
		if json.Unmarshal(data, &cd) == nil {
			for _, c := range cd.CutIntervals {
				cuts = append(cuts, episodeCutDTO{
					StartFormatted: c.StartFormatted,
					EndFormatted:   c.EndFormatted,
					DurationSec:    c.DurationSec,
					DurationStr:    format.FormatClock(c.DurationSec),
					Reason:         c.Reason,
				})
			}
		}
	}

	if len(cuts) == 0 && st != nil && len(st.Ads) > 0 {
		for _, ad := range st.Ads {
			dur := ad.End - ad.Start
			cuts = append(cuts, episodeCutDTO{
				StartFormatted: format.FormatClock(ad.Start),
				EndFormatted:   format.FormatClock(ad.End),
				DurationSec:    dur,
				DurationStr:    format.FormatClock(dur),
				Reason:         ad.Reason,
			})
		}
	}
	return cuts
}

func buildEpisodeInfoDTO(ep *ResolvedEpisode) episodeInfoJSON {
	fi, _ := os.Stat(ep.Path)
	var fileSize int64
	if fi != nil {
		fileSize = fi.Size()
	}

	st := episode.GetOrCreateEpisodeStatus(ep.Path)
	statusStr, _ := getEpisodeStatusLabel(ep.Path)
	origDur, cleanDur := episode.EpisodeDurations(ep.Path, st)

	pctReduction := 0.0
	if origDur > 0 && cleanDur > 0 && origDur > cleanDur {
		pctReduction = (origDur - cleanDur) / origDur * 100
	}

	pubTime := resolveEpisodePublicationTime(ep.Path, st, fi)
	pubDateStr := "-"
	if !pubTime.IsZero() {
		pubDateStr = publicationDateTime(pubTime)
	}

	hasTx, txPath, txSegments := getEpisodeTranscriptInfo(ep.Path)

	desc := ""
	detailKey := ep.Filename
	if strings.EqualFold(ep.Filename, "podcast.mp3") && ep.Title != "" {
		detailKey = ep.Title + ".mp3"
	}
	if det, _ := podcast.LoadEpisodeDetails(ep.PodcastDir, detailKey); det != nil && det.Description != "" {
		desc = det.Description
	} else if det, _ := podcast.LoadEpisodeDetails(ep.PodcastDir, ep.Filename); det != nil && det.Description != "" {
		desc = det.Description
	}

	cuts := collectEpisodeCuts(ep.Path, st)
	files := collectEpisodeFiles(ep.Path)

	return episodeInfoJSON{
		ID:                  ep.ShortID,
		PodcastID:           ep.PodcastShortID,
		PodcastTitle:        ep.PodcastTitle,
		Title:               ep.Title,
		PublishedDate:       pubDateStr,
		AudioPath:           ep.Path,
		FileSizeBytes:       fileSize,
		FileSizeFormatted:   formatDiskSize(fileSize),
		Status:              statusStr,
		OriginalDurationSec: origDur,
		CleanDurationSec:    cleanDur,
		PercentReduction:    pctReduction,
		Favorite:            st.IsFavorite(),
		HasTranscript:       hasTx,
		TranscriptPath:      txPath,
		TranscriptSegments:  txSegments,
		Description:         desc,
		Cuts:                cuts,
		Files:               files,
	}
}

// collectEpisodeFiles lists the files of the episode with their sizes.
func collectEpisodeFiles(audioPath string) []episodeFileDTO {
	var out []episodeFileDTO
	for _, p := range podcast.EpisodeFiles(audioPath) {
		var size int64
		if fi, err := os.Stat(p); err == nil {
			size = fi.Size()
		}
		out = append(out, episodeFileDTO{Path: p, SizeBytes: size})
	}
	return out
}

func formatEpisodeInfo(info episodeInfoJSON, showCuts ...bool) string {
	sc := false
	if len(showCuts) > 0 {
		sc = showCuts[0]
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s\n", strings.Repeat("=", 80)))
	sb.WriteString(fmt.Sprintf("Episode: %s [%s]\n", util.Bold(util.DisplayName(info.Title)), util.BoldCyan(info.ID)))
	sb.WriteString(fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
	sb.WriteString(fmt.Sprintf("  Episode ID:       %s\n", util.BoldCyan(info.ID)))
	sb.WriteString(fmt.Sprintf("  Podcast:          %s [%s]\n", util.DisplayName(info.PodcastTitle), util.BoldCyan(info.PodcastID)))
	sb.WriteString(fmt.Sprintf("  Published Date:   %s\n", info.PublishedDate))
	sb.WriteString(fmt.Sprintf("  Audio Path:       %s\n", info.AudioPath))
	sb.WriteString(fmt.Sprintf("  File Size:        %s\n", info.FileSizeFormatted))
	sb.WriteString(fmt.Sprintf("  AdR Status:       %s\n", util.Bold(info.Status)))
	favStr := "No"
	if info.Favorite {
		favStr = "⭐ Yes"
	}
	sb.WriteString(fmt.Sprintf("  Favorite:         %s\n", favStr))

	sb.WriteString("\n  Audio & Processing Stats:\n")
	sb.WriteString(fmt.Sprintf("    Original Dur:   %s (%.1fs)\n", format.FormatClock(info.OriginalDurationSec), info.OriginalDurationSec))
	cleanStr := "-"
	if info.CleanDurationSec > 0 {
		cleanStr = fmt.Sprintf("%s (%.1fs)", format.FormatClock(info.CleanDurationSec), info.CleanDurationSec)
	}
	sb.WriteString(fmt.Sprintf("    Cleaned Dur:    %s\n", cleanStr))
	if info.PercentReduction > 0 {
		diffSec := info.OriginalDurationSec - info.CleanDurationSec
		sb.WriteString(fmt.Sprintf("    Reduction:      -%s (-%.1f%%)\n", format.FormatClock(diffSec), info.PercentReduction))
	}

	sb.WriteString(formatEpisodeCutsAndTranscript(info, sc))
	sb.WriteString(fmt.Sprintf("%s\n\n", strings.Repeat("=", 80)))
	return sb.String()
}

func formatEpisodeCutsAndTranscript(info episodeInfoJSON, showCuts bool) string {
	var sb strings.Builder
	if len(info.Cuts) > 0 || showCuts {
		sb.WriteString(fmt.Sprintf("\n  Commercial Cuts (%d cuts detected):\n", len(info.Cuts)))
		if len(info.Cuts) == 0 {
			sb.WriteString("    No cuts detected.\n")
		} else {
			for idx, cut := range info.Cuts {
				reason := cut.Reason
				if reason == "" {
					reason = "Advertisement segment"
				}
				sb.WriteString(fmt.Sprintf("    [%d] %-8s - %-8s (%s) : %s\n",
					idx+1, cut.StartFormatted, cut.EndFormatted, cut.DurationStr, reason))
			}
		}
	}

	sb.WriteString("\n  Transcript Info:\n")
	if info.HasTranscript {
		sb.WriteString(fmt.Sprintf("    Status:         Available (%d segments)\n", info.TranscriptSegments))
		sb.WriteString(fmt.Sprintf("    Path:           %s\n", info.TranscriptPath))
	} else {
		sb.WriteString("    Status:         Not available\n")
	}

	if len(info.Files) > 0 {
		sb.WriteString(fmt.Sprintf("\n  Files (%d):\n", len(info.Files)))
		for _, f := range info.Files {
			sb.WriteString(fmt.Sprintf("    %8s  %s\n", formatDiskSize(f.SizeBytes), f.Path))
		}
	}

	if info.Description != "" {
		sb.WriteString("\n  Show Notes / Description:\n")
		formatted := cleanAndFormatNotes(info.Description, 4, 76)
		sb.WriteString(formatted + "\n")
	}
	return sb.String()
}

func printEpisodeInfoCard(w io.Writer, info episodeInfoJSON, showCuts bool) {
	fmt.Fprint(w, formatEpisodeInfo(info, showCuts))
}
