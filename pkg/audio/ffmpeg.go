package audio

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/util"
)

const minKeepFraction = 0.25

func GetAudioDuration(filePath string) float64 {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		util.Errorf("failed to get absolute path: %v", err)
		return 0.0
	}
	cmd := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		absPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		util.Errorf("ffprobe failed: %v, output: %s", err, string(output))
		return 0.0
	}
	var dur float64
	fmt.Sscanf(string(output), "%f", &dur)
	return dur
}

func ExtractID3Tags(filePath string) map[string]string {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		util.Errorf("failed to get absolute path: %v", err)
		return nil
	}
	cmd := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format_tags",
		"-of", "default=noprint_wrappers=1",
		absPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		util.Errorf("ffprobe failed: %v, output: %s", err, string(output))
		return nil
	}

	tags := make(map[string]string)
	for _, line := range util.SplitLines(string(output)) {
		if len(line) > 4 && line[:4] == "TAG:" {
			eqIdx := -1
			for i := 4; i < len(line); i++ {
				if line[i] == '=' {
					eqIdx = i
					break
				}
			}
			if eqIdx > 4 {
				key := util.ToLower(line[4:eqIdx])
				val := line[eqIdx+1:]
				if val != "" {
					tags[key] = val
				}
			}
		}
	}
	return tags
}

func ValidateWavFile(filePath string) bool {
	dur := GetAudioDuration(filePath)
	return dur > 0
}

// CheckKeepFraction returns an error when a cut would keep so little of the
// source that the detector must have returned implausible timestamps, and nil
// when the cut is plausible or the source duration is unknown.
func CheckKeepFraction(inputFile string, keepSegments [][2]float64) error {
	sourceDuration := GetAudioDuration(inputFile)
	if sourceDuration <= 0 {
		return nil
	}
	kept := 0.0
	for _, seg := range keepSegments {
		if seg[1] > seg[0] {
			kept += seg[1] - seg[0]
		}
	}
	if kept >= sourceDuration*minKeepFraction {
		return nil
	}
	return fmt.Errorf("refusing to cut %s: the requested cut would keep only %.1fs of %.1fs (%.1f%%, floor %.0f%%), which usually means the ad detector returned implausible timestamps; --force llm discards the saved cuts and detects again",
		inputFile, kept, sourceDuration, kept/sourceDuration*100, minKeepFraction*100)
}

func KeepFractionIsPlausible(inputFile string, keepSegments [][2]float64) bool {
	err := CheckKeepFraction(inputFile, keepSegments)
	if err == nil {
		return true
	}
	util.Errorf("%v. The file was left unchanged.", err)
	return false
}

func BuildCutFilterComplex(keepSegments [][2]float64) string {
	var filter strings.Builder
	filter.Grow(len(keepSegments) * 80)
	for idx, seg := range keepSegments {
		fmt.Fprintf(&filter, "[0:a]atrim=start=%.3f:end=%.3f,asetpts=PTS-STARTPTS[a%d];", seg[0], seg[1], idx)
	}
	for idx := range keepSegments {
		fmt.Fprintf(&filter, "[a%d]", idx)
	}
	fmt.Fprintf(&filter, "concat=n=%d:v=0:a=1[aout]", len(keepSegments))
	return filter.String()
}

func FormatCUETime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	totalSecs := int(seconds)
	mins := totalSecs / 60
	secs := totalSecs % 60
	frames := int((seconds - float64(totalSecs)) * 75.0)
	if frames > 74 {
		frames = 74
	}
	return fmt.Sprintf("%02d:%02d:%02d", mins, secs, frames)
}

func BuildCUEContent(inputFile string, splitPoints []float64) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "FILE %q MP3\n", inputFile)
	for idx, pt := range splitPoints {
		fmt.Fprintf(&sb, "  TRACK %d AUDIO\n", idx+1)
		fmt.Fprintf(&sb, "    INDEX 01 %s\n", FormatCUETime(pt))
	}
	return sb.String()
}

func ComputeSplitPoints(keepSegments [][2]float64, duration float64) []float64 {
	pts := make([]float64, 0, len(keepSegments)*2+1)
	pts = append(pts, 0.0)
	for _, seg := range keepSegments {
		if seg[0] > 0 {
			pts = append(pts, seg[0])
		}
		if seg[1] > 0 && (duration <= 0 || seg[1] < duration) {
			pts = append(pts, seg[1])
		}
	}
	sort.Float64s(pts)
	dedup := make([]float64, 0, len(pts))
	for _, p := range pts {
		if len(dedup) == 0 || p-dedup[len(dedup)-1] >= 0.05 {
			dedup = append(dedup, p)
		}
	}
	return dedup
}

func IsMidpointInKeep(mid float64, keepSegments [][2]float64) bool {
	for _, seg := range keepSegments {
		if mid >= seg[0] && mid <= seg[1] {
			return true
		}
	}
	return false
}

func ConvertToWAV(inputPath, wavPath string) bool {
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-i", inputPath,
		"-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wavPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		util.Errorf("ffmpeg convert failed: %v, output: %s", err, string(out))
		return false
	}
	return true
}

func TruncateAudio(inputPath, outputPath string, durationSec float64) bool {
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-ss", "0", "-i", inputPath,
		"-to", fmt.Sprintf("%.3f", durationSec),
		"-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", outputPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		util.Errorf("ffmpeg truncate failed: %v, output: %s", err, string(out))
		return false
	}
	return true
}

func BuildRemoteCutCleanupCmd(remIn, remOut string) string {
	return fmt.Sprintf("rm -f %s %s", util.ShellQuote(remIn), util.ShellQuote(remOut))
}

func BuildRemoteFFmpegCmd(remIn, filterComplex, remOut string) string {
	return fmt.Sprintf("ffmpeg -y -loglevel error -i %s -filter_complex %s -map '[aout]' -c:a libmp3lame -b:a 192k %s",
		util.ShellQuote(remIn), util.ShellQuote(filterComplex), util.ShellQuote(remOut))
}

func CopyTagsAndArt(cutFile, srcFile, dstFile string) bool {
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-i", cutFile,
		"-i", srcFile,
		"-map", "0:a",
		"-map", "1:v?",
		"-c", "copy",
		"-map_metadata", "1",
		"-id3v2_version", "3",
		dstFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		util.Errorf("tags copy failed: %v, output: %s", err, string(out))
		return copyFileDirect(cutFile, dstFile) == nil
	}
	return true
}

func copyFileDirect(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func CutAudioFilterComplex(absInput string, keepSegments [][2]float64, absOutput string) bool {
	filterComplex := BuildCutFilterComplex(keepSegments)
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-i", absInput,
		"-filter_complex", filterComplex,
		"-map", "[aout]",
		"-c:a", "libmp3lame",
		"-b:a", "192k",
		absOutput)
	out, err := cmd.CombinedOutput()
	if err != nil {
		util.Errorf("ffmpeg filter complex failed: %v, output: %s", err, string(out))
		return false
	}
	return true
}

func CutAudioFFmpeg(inputFile string, keepSegments [][2]float64, outputFile string) bool {
	if len(keepSegments) == 0 {
		return false
	}
	if !KeepFractionIsPlausible(inputFile, keepSegments) {
		return false
	}
	absInput, _ := filepath.Abs(inputFile)
	absOutput, _ := filepath.Abs(outputFile)
	if !CutAudioFilterComplex(absInput, keepSegments, absOutput) {
		return false
	}
	taggedOutput := absOutput + ".tagged" + filepath.Ext(absOutput)
	if !strings.Contains(taggedOutput, "/.work/") {
		workDir := util.WorkDirFor(absOutput)
		if err := os.MkdirAll(workDir, 0755); err != nil {
			return true
		}
		taggedOutput = filepath.Join(workDir, filepath.Base(absOutput)+".tagged"+filepath.Ext(absOutput))
	}
	if util.VerifyTempFile(taggedOutput) == nil && CopyTagsAndArt(absOutput, absInput, taggedOutput) {
		if err := util.SafeMove(taggedOutput, absOutput); err != nil {
			_ = os.Remove(taggedOutput)
		}
	}
	return true
}
