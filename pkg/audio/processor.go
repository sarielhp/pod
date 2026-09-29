package audio

import (
	"context"
	"fmt"
	"path/filepath"

	"pod/pkg/util"
)

// AudioProcessor defines the operations required for audio inspection, cutting,
// truncation, and metadata preservation.
type AudioProcessor interface {
	Duration(ctx context.Context, audioPath string) (float64, error)
	Truncate(ctx context.Context, inputPath, outputPath string, durSec float64) error
	Cut(ctx context.Context, inputPath string, keepSegments [][2]float64, outputPath string) error
	ExtractTags(ctx context.Context, audioPath string) (map[string]string, error)
	PreserveMetadata(ctx context.Context, srcPath, dstPath string) error
}

// FFmpegProcessor implements AudioProcessor using local ffmpeg/ffprobe commands.
type FFmpegProcessor struct{}

func NewFFmpegProcessor() *FFmpegProcessor {
	return &FFmpegProcessor{}
}

func (p *FFmpegProcessor) Duration(ctx context.Context, audioPath string) (float64, error) {
	dur := GetAudioDuration(audioPath)
	if dur <= 0 {
		return 0, fmt.Errorf("failed to determine audio duration for '%s'", audioPath)
	}
	return dur, nil
}

func (p *FFmpegProcessor) Truncate(ctx context.Context, inputPath, outputPath string, durSec float64) error {
	if !TruncateAudio(inputPath, outputPath, durSec) {
		return fmt.Errorf("failed to truncate audio '%s' to %.2fs", inputPath, durSec)
	}
	return nil
}

func (p *FFmpegProcessor) Cut(ctx context.Context, inputPath string, keepSegments [][2]float64, outputPath string) error {
	if err := CheckKeepFraction(inputPath, keepSegments); err != nil {
		return err
	}
	if !CutAudioFFmpeg(inputPath, keepSegments, outputPath) {
		return fmt.Errorf("failed to cut audio '%s' with %d segments", inputPath, len(keepSegments))
	}
	return nil
}

func (p *FFmpegProcessor) ExtractTags(ctx context.Context, audioPath string) (map[string]string, error) {
	tags := ExtractID3Tags(audioPath)
	return tags, nil
}

func (p *FFmpegProcessor) PreserveMetadata(ctx context.Context, srcPath, dstPath string) error {
	workDir := util.WorkDirFor(dstPath)
	tagged := filepath.Join(workDir, filepath.Base(dstPath)+".meta"+filepath.Ext(dstPath))
	if err := util.VerifyTempFile(tagged); err != nil {
		return err
	}
	if !CopyTagsAndArt(dstPath, srcPath, tagged) {
		return fmt.Errorf("failed to copy tags and art from '%s' to '%s'", srcPath, dstPath)
	}
	return util.SafeMove(tagged, dstPath)
}

var DefaultProcessor AudioProcessor = NewFFmpegProcessor()
