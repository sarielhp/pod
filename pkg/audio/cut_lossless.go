package audio

import (
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/util"
)

const (
	losslessBaseSlackSec       = 0.5
	losslessPerSegmentSlackSec = 0.1
)

func cutLossless(absInput string, keepSegments [][2]float64, absOutput string, duration float64) bool {
	workDir, err := cutWorkDir(absOutput)
	if err != nil {
		return false
	}
	var tiers []func() bool
	if strings.EqualFold(filepath.Ext(absInput), ".mp3") && hasMp3splt() {
		tiers = append(tiers, func() bool { return cutMp3splt(absInput, keepSegments, absOutput, workDir, duration) })
	}
	tiers = append(tiers, func() bool { return cutStreamCopy(absInput, keepSegments, absOutput, workDir) })
	for _, tier := range tiers {
		if tier() && cutLengthMatches(absOutput, keepSegments, duration) {
			return true
		}
		_ = os.Remove(absOutput)
	}
	return false
}

func hasMp3splt() bool {
	_, err := exec.LookPath("mp3splt")
	return err == nil
}

func cutWorkDir(absOutput string) (string, error) {
	if strings.Contains(absOutput, "/.work/") {
		return filepath.Dir(absOutput), nil
	}
	dir := util.WorkDirFor(absOutput)
	return dir, os.MkdirAll(dir, 0755)
}

func cutLengthMatches(absOutput string, keepSegments [][2]float64, duration float64) bool {
	want := 0.0
	for _, seg := range keepSegments {
		end := seg[1]
		if duration > 0 {
			end = math.Min(end, duration)
		}
		want += math.Max(0, end-seg[0])
	}
	got := GetAudioDuration(absOutput)
	slack := losslessBaseSlackSec + losslessPerSegmentSlackSec*float64(len(keepSegments))
	if got > 0 && math.Abs(got-want) <= slack {
		return true
	}
	util.Errorf("lossless cut of %s is %.1fs, expected %.1fs; re-encoding instead", filepath.Base(absOutput), got, want)
	return false
}

func cutMp3splt(absInput string, keepSegments [][2]float64, absOutput, workDir string, duration float64) bool {
	splitPoints := ComputeSplitPoints(keepSegments, duration)
	if len(splitPoints) <= 1 {
		return false
	}
	cueFile := filepath.Join(workDir, "split.cue")
	if util.VerifyTempFile(cueFile) != nil {
		return false
	}
	if err := os.WriteFile(cueFile, []byte(BuildCUEContent(absInput, splitPoints)), 0644); err != nil {
		return false
	}
	defer os.Remove(cueFile)

	pfx := fmt.Sprintf("mp3splt_%d", time.Now().UnixNano()%1000000)
	defer removeChunks(workDir, pfx, len(splitPoints))
	cmd := exec.Command("mp3splt", "-q", "-n", "-x", "-c", cueFile, "-d", workDir, "-o", pfx+"_chunk_@n2", absInput)
	if out, err := cmd.CombinedOutput(); err != nil {
		util.Errorf("mp3splt failed: %v, output: %s", err, string(out))
		return false
	}

	var kept []string
	for _, chunk := range keepChunks(splitPoints, keepSegments, workDir, duration, pfx) {
		if util.FileExists(chunk) {
			kept = append(kept, chunk)
		}
	}
	if len(kept) == 0 {
		return false
	}
	merged := filepath.Join(workDir, pfx+"_merged.mp3")
	if util.VerifyTempFile(merged) != nil {
		return false
	}
	defer os.Remove(merged)
	if concatFiles(kept, merged) != nil {
		return false
	}
	return CopyTagsAndArt(merged, absInput, absOutput)
}

func chunkPath(workDir, pfx string, index int) string {
	return filepath.Join(workDir, fmt.Sprintf("%s_chunk_%02d.mp3", pfx, index))
}

func removeChunks(workDir, pfx string, count int) {
	for i := 1; i <= count+2; i++ {
		_ = os.Remove(chunkPath(workDir, pfx, i))
	}
}

func keepChunks(splitPoints []float64, keepSegments [][2]float64, workDir string, duration float64, pfx string) []string {
	var keep []string
	for i, start := range splitPoints {
		end := duration
		if i+1 < len(splitPoints) {
			end = splitPoints[i+1]
		}
		if IsMidpointInKeep((start+end)/2.0, keepSegments) {
			keep = append(keep, chunkPath(workDir, pfx, i+1))
		}
	}
	return keep
}

func concatFiles(srcFiles []string, dstFile string) error {
	out, err := os.Create(dstFile)
	if err != nil {
		return err
	}
	defer out.Close()
	for _, src := range srcFiles {
		if err := appendFile(out, src); err != nil {
			return err
		}
	}
	return nil
}

func appendFile(dst io.Writer, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(dst, in)
	return err
}

func cutStreamCopy(absInput string, keepSegments [][2]float64, absOutput, workDir string) bool {
	ext := filepath.Ext(absInput)
	if ext == "" {
		ext = ".mp3"
	}
	pfx := fmt.Sprintf("stream_%d", time.Now().UnixNano()%1000000)
	parts, ok := extractParts(absInput, keepSegments, workDir, pfx, ext)
	defer removeAll(parts)
	if !ok {
		return false
	}
	manifest := filepath.Join(workDir, pfx+"_concat.txt")
	merged := filepath.Join(workDir, pfx+"_merged"+ext)
	if util.VerifyTempFile(manifest) != nil || util.VerifyTempFile(merged) != nil {
		return false
	}
	defer os.Remove(manifest)
	defer os.Remove(merged)
	if writeConcatManifest(manifest, parts) != nil {
		return false
	}
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "concat", "-safe", "0", "-i", manifest, "-c", "copy", merged)
	if out, err := cmd.CombinedOutput(); err != nil {
		util.Errorf("stream concat failed: %v, output: %s", err, string(out))
		return false
	}
	return CopyTagsAndArt(merged, absInput, absOutput)
}

func extractParts(absInput string, keepSegments [][2]float64, workDir, pfx, ext string) ([]string, bool) {
	var parts []string
	for idx, seg := range keepSegments {
		part := filepath.Join(workDir, fmt.Sprintf("%s_part_%03d%s", pfx, idx, ext))
		if util.VerifyTempFile(part) != nil {
			return parts, false
		}
		parts = append(parts, part)
		cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
			"-ss", fmt.Sprintf("%.3f", seg[0]),
			"-i", absInput,
			"-t", fmt.Sprintf("%.3f", seg[1]-seg[0]),
			"-c", "copy", part)
		if out, err := cmd.CombinedOutput(); err != nil {
			util.Errorf("stream segment failed: %v, output: %s", err, string(out))
			return parts, false
		}
	}
	return parts, true
}

func removeAll(paths []string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

func writeConcatManifest(manifestPath string, paths []string) error {
	var sb strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&sb, "file '%s'\n", strings.ReplaceAll(p, "'", "'\\''"))
	}
	return os.WriteFile(manifestPath, []byte(sb.String()), 0644)
}
