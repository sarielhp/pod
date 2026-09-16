package adremoval

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"pod/pkg/audio"
	"pod/pkg/pipeline"
	"pod/pkg/podcast"
	"pod/pkg/types"
	"pod/pkg/util"
)

type transcriptAuditItem struct {
	audioPath      string
	transcriptPath string
	statusPath     string
	cutsPath       string
	audioDur       float64
	textChars      int
	speechDur      float64
	coverageRatio  float64
	isCompleted    bool
	isCorrupted    bool
	isSuspicious   bool
	suspiciousMsg  string
	cleanStateMsg  string
	adFailed       bool
}

func RunTranscriptAudit(cfg types.Config, targets []string, opts types.ProcOptions) error {
	if len(targets) == 0 {
		if cfg.PodcastsDir == "" {
			return fmt.Errorf("podcasts_dir not configured and no target paths provided")
		}
		targets = []string{cfg.PodcastsDir}
	}

	minRatio, minChars := auditThresholds(opts)

	audioFiles := collectAudioFilesForAudit(targets)
	if len(audioFiles) == 0 {
		if !opts.Quiet {
			fmt.Println("No audio files found to audit.")
		}
		return nil
	}

	if !opts.Quiet {
		fmt.Printf("Auditing transcripts across %d audio file(s)...\n", len(audioFiles))
	}

	var tally auditTally
	var failures []error

	for _, audioPath := range audioFiles {
		item := inspectEpisodeTranscript(audioPath, minRatio, minChars)
		if item == nil {
			continue
		}
		tally.count(item, opts)
		if !item.needsRepair() {
			continue
		}
		if err := repairAuditedEpisode(item, cfg, opts.DryRun, opts.Quiet); err != nil {
			failures = append(failures, fmt.Errorf("audit %s: %w", audioPath, err))
		}
	}

	printAuditSummary(tally.scanned, tally.suspicious, tally.uncut, tally.adFailed, opts.DryRun, opts.Quiet)
	return errors.Join(failures...)
}

// auditThresholds reads the audit's tuning, falling back to defaults for a
// value that is absent or unparseable.
func auditThresholds(opts types.ProcOptions) (minRatio float64, minChars int) {
	minRatio, minChars = 0.15, opts.AuditMinChars
	if opts.AuditMinRatioStr != "" {
		if v, err := strconv.ParseFloat(opts.AuditMinRatioStr, 64); err == nil && v > 0 {
			minRatio = v
		}
	}
	if minChars <= 0 {
		minChars = 50
	}
	return minRatio, minChars
}

// auditTally counts what the audit found.
type auditTally struct {
	scanned    int
	suspicious int
	uncut      int
	adFailed   int
}

// count records one inspected episode, reporting the healthy ones when asked
// to be verbose.
func (t *auditTally) count(item *transcriptAuditItem, opts types.ProcOptions) {
	t.scanned++
	switch {
	case item.cleanStateMsg != "":
		t.uncut++
	case item.isSuspicious:
		t.suspicious++
	case item.adFailed:
		t.adFailed++
	case opts.Verbose && !opts.Quiet:
		fmt.Printf("  [OK] %s (%.0fs, %d chars, %.1f%% coverage)\n",
			auditDisplayName(item.audioPath), item.audioDur, item.textChars, item.coverageRatio*100)
	}
}

// needsRepair reports an episode the audit should try to heal.
func (i *transcriptAuditItem) needsRepair() bool {
	return i.cleanStateMsg != "" || i.isSuspicious || i.adFailed
}

func auditDisplayName(p string) string {
	dir := filepath.Base(filepath.Dir(p))
	base := filepath.Base(p)
	if dir != "" && dir != "." && dir != "/" && base == "podcast.mp3" {
		return dir
	}
	return base
}

func inspectEpisodeTranscript(audioPath string, minRatio float64, minChars int) *transcriptAuditItem {
	base := util.StripExt(audioPath)
	transPath := base + ".transcript.json"
	statPath := pipeline.StatusPathFor(audioPath)
	cutsPath := base + ".cuts.json"

	if !auditMP3Exists(audioPath) || pipeline.IsEpisodeInRemoteFlight(audioPath) {
		return nil
	}

	item := &transcriptAuditItem{
		audioPath:      audioPath,
		transcriptPath: transPath,
		statusPath:     statPath,
		cutsPath:       cutsPath,
	}

	st, _ := pipeline.LoadEpisodeStatus(statPath)
	diskDuration := audio.GetAudioDuration(audioPath)
	if st != nil && st.Original.DurationSec > 0 {
		item.audioDur = st.Original.DurationSec
	} else {
		item.audioDur = diskDuration
	}
	item.cleanStateMsg = invalidCleanStateMessage(st, diskDuration, transPath)
	item.isCompleted = st != nil && (st.Status == types.StateDone || st.Status == types.StateCopiedBack)
	if st != nil && st.AdDetectionSuccessful != nil && !*st.AdDetectionSuccessful {
		item.adFailed = true
	}

	if !util.FileExists(transPath) {
		item.isSuspicious = true
		item.suspiciousMsg = "missing transcript"
		return item
	}

	data, err := os.ReadFile(transPath)
	if err != nil {
		item.isSuspicious = true
		item.suspiciousMsg = "unreadable transcript file"
		return item
	}

	var td types.TranscriptionData
	if err := json.Unmarshal(data, &td); err != nil {
		item.isSuspicious = true
		item.isCorrupted = true
		item.suspiciousMsg = "corrupted JSON"
		return item
	}

	var rawMap map[string]interface{}
	if json.Unmarshal(data, &rawMap) == nil {
		if val, ok := rawMap["ad_detection_successful"].(bool); ok && !val {
			item.adFailed = true
		}
	}

	evaluateTranscriptMetrics(item, td, minRatio, minChars)
	return item
}

func invalidCleanStateMessage(st *types.EpisodeStatusFile, diskDuration float64, transcriptPath string) string {
	if st == nil || (st.Status != types.StateDone && st.Status != types.StateCopiedBack && st.Status != types.StateArchived) {
		return ""
	}
	info, err := os.Stat(transcriptPath)
	if err != nil {
		return "completed status has no transcript on disk"
	}
	if info.Size() == 0 {
		return "completed status has an empty transcript"
	}
	return staleCleanAudioMessage(st, diskDuration)
}

func staleCleanAudioMessage(st *types.EpisodeStatusFile, diskDuration float64) string {
	if diskDuration <= 0 || st.Cleaned.DurationSec <= 0 {
		return ""
	}
	if math.Abs(diskDuration-st.Cleaned.DurationSec) <= 2 {
		return ""
	}
	return fmt.Sprintf("on-disk duration %.1fs differs from recorded cleaned duration %.1fs", diskDuration, st.Cleaned.DurationSec)
}

func evaluateTranscriptMetrics(item *transcriptAuditItem, td types.TranscriptionData, minRatio float64, minChars int) {
	text := strings.TrimSpace(td.Text)
	item.textChars = len([]rune(text))

	totalSpeech := 0.0
	for _, seg := range td.Segments {
		totalSpeech += (seg.End - seg.Start)
	}
	item.speechDur = totalSpeech

	if item.audioDur > 0 {
		item.coverageRatio = item.speechDur / item.audioDur
	}

	if item.textChars < minChars {
		item.isSuspicious = true
		item.suspiciousMsg = fmt.Sprintf("transcript too short (%d chars < %d min)", item.textChars, minChars)
		return
	}

	if item.audioDur > 60 {
		if item.textChars < minChars {
			item.isSuspicious = true
			item.suspiciousMsg = fmt.Sprintf("near empty text (%d chars < %d min)", item.textChars, minChars)
		} else if len(td.Segments) == 0 {
			item.isSuspicious = true
			item.suspiciousMsg = "no transcription segments found"
		} else if item.audioDur > 120 && item.coverageRatio < minRatio {
			item.isSuspicious = true
			item.suspiciousMsg = fmt.Sprintf("speech coverage %.1f%% below %.1f%% threshold", item.coverageRatio*100, minRatio*100)
		}
	}
}

func repairAuditedEpisode(item *transcriptAuditItem, cfg types.Config, dryRun, quiet bool) error {
	if !auditMP3Exists(item.audioPath) {
		return fmt.Errorf("audio is no longer a regular MP3: %s", item.audioPath)
	}
	if dryRun {
		if !quiet {
			fmt.Printf("  [DRY RUN] %s: would mark NeedAdR and queue\n", item.audioPath)
		}
		return nil
	}
	if item.isSuspicious {
		for _, path := range []string{item.transcriptPath, item.cutsPath} {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove invalid metadata %s: %w", path, err)
			}
		}
	}
	if err := pipeline.UpdateEpisodeStatus(item.audioPath, func(st *types.EpisodeStatusFile) {
		st.Status = types.StateNeedsAdR
		st.Cleaned = types.EpisodeAudioMeta{}
		if item.isSuspicious {
			st.Ads = nil
			st.AdDetectionSuccessful = nil
			st.AdDetectionStatus = ""
			st.AdDetectionError = ""
		}
	}); err != nil {
		return err
	}
	if err := queueAuditedEpisode(cfg, item.audioPath); err != nil {
		return err
	}
	if !quiet {
		fmt.Printf("  [NeedAdR] %s: %s%s; queued\n", item.audioPath, item.cleanStateMsg, item.suspiciousMsg)
	}
	return nil
}

func queueAuditedEpisode(cfg types.Config, audioPath string) error {
	if !auditMP3Exists(audioPath) {
		return fmt.Errorf("audio is no longer a regular MP3: %s", audioPath)
	}
	podDir := podcast.DetectPodcastDirForAudio(audioPath)
	if cfg.PodcastsDir != "" {
		root, err := filepath.Abs(cfg.PodcastsDir)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, audioPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			podDir = root
			if parts := strings.Split(rel, string(filepath.Separator)); len(parts) > 1 {
				podDir = filepath.Join(root, parts[0])
			}
		}
	}
	rel, err := filepath.Rel(podDir, audioPath)
	if err != nil {
		return err
	}
	return pipeline.UpdateQueue(podDir, func(entries []string) []string {
		for _, entry := range entries {
			if entry == rel {
				return entries
			}
		}
		return append(entries, rel)
	})
}

func auditMP3Exists(path string) bool {
	if strings.HasSuffix(strings.ToLower(filepath.Base(path)), "precut.mp3") {
		return false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, part := range strings.Split(absolute, string(filepath.Separator)) {
		if part == ".work" {
			return false
		}
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(path), ".mp3")
}

func printAuditSummary(scanned, suspicious, uncut, adFailed int, dryRun, quiet bool) {
	if quiet {
		return
	}
	fmt.Printf("\n%s\n", util.RepeatStr("-", 50))
	fmt.Println("TRANSCRIPT AUDIT SUMMARY:")
	fmt.Printf("  - Scanned episodes:        %d\n", scanned)
	fmt.Printf("  - Suspicious transcripts:  %d\n", suspicious)
	fmt.Printf("  - Invalid clean states:    %d\n", uncut)
	fmt.Printf("  - Failed ad detections:    %d\n", adFailed)
	if dryRun {
		fmt.Println("  (Dry-run mode: no files were modified or deleted)")
	}
	fmt.Printf("%s\n\n", util.RepeatStr("-", 50))
}

func collectAudioFilesForAudit(targets []string) []string {
	var files []string
	seen := make(map[string]bool)
	for _, target := range targets {
		target, err := filepath.Abs(target)
		if err != nil {
			continue
		}
		fi, err := os.Stat(target)
		if err != nil {
			continue
		}
		candidates := []string{target}
		if fi.IsDir() {
			candidates = util.FindMP3Files(target)
		}
		for _, path := range candidates {
			if auditMP3Exists(path) && !seen[path] {
				files = append(files, path)
				seen[path] = true
			}
		}
	}
	return files
}
