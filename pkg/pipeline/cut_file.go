package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"pod/pkg/audio"
	"pod/pkg/format"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
)

// CutRequest is a one-off audio cut over existing cuts metadata.
//
// It is the third stage of the pipeline made available on its own, following
// TranscribeFile and DetectFile: it takes an audio file and cuts metadata (or finds
// .cuts.json beside the audio) and cuts the audio using ffmpeg. No library
// state or status file is touched.
type CutRequest struct {
	// Path is the audio file to cut.
	Path string

	// CutsPath is an optional path to a .cuts.json file. If empty, the file
	// is resolved beside Path (<stem>.cuts.json).
	CutsPath string

	// Output is the output file path. If empty, writes beside Path.
	Output string

	// DryRun reports what would be cut without executing ffmpeg.
	DryRun bool
}

// CutResult reports what cutting produced.
type CutResult struct {
	InputPath    string
	OutputPath   string
	CutsPath     string
	OriginalSec  float64
	CleanedSec   float64
	CutSec       float64
	KeepSegments [][2]float64
	SegmentsCut  int
}

// CutFile cuts advertisements from an audio file using cuts metadata.
func CutFile(req CutRequest, cfg types.Config, opts types.ProcOptions, rep progress.Reporter) (CutResult, error) {
	r := progress.Or(rep)
	var res CutResult

	info, err := os.Stat(req.Path)
	if err != nil {
		return res, fmt.Errorf("read %s: %w", req.Path, err)
	}
	if info.IsDir() {
		return res, fmt.Errorf("%s is a directory", req.Path)
	}

	mainMP3File, precutFile, sourceAudioFile := ResolveAudioFiles(req.Path, opts.Verbose)
	res.InputPath = mainMP3File
	res.OriginalSec = audio.GetAudioDuration(sourceAudioFile)
	if res.OriginalSec <= 0 {
		return res, fmt.Errorf("%s has no readable audio track", filepath.Base(sourceAudioFile))
	}

	cutsPath := req.CutsPath
	if cutsPath == "" {
		cutsPath = util.StripExt(mainMP3File) + ".cuts.json"
	}
	res.CutsPath = cutsPath

	cutsData, err := loadCutsMetadata(cutsPath)
	if err != nil {
		return res, err
	}

	keepSegments, cutDuration, numCutSegments := extractKeepSegments(cutsData, res.OriginalSec)
	res.KeepSegments = keepSegments
	res.CutSec = cutDuration
	res.SegmentsCut = numCutSegments
	res.CleanedSec = res.OriginalSec - cutDuration

	outputPath := req.Output
	if outputPath == "" {
		outputPath = mainMP3File
	}
	res.OutputPath = outputPath

	if req.DryRun {
		return res, nil
	}

	if len(keepSegments) == 0 {
		return res, fmt.Errorf("no keep segments found in cut metadata %s", filepath.Base(cutsPath))
	}

	r.Infof("Cutting ads in %s (%d non-ad segments)...", filepath.Base(sourceAudioFile), len(keepSegments))
	if err := executeCutProcessing(sourceAudioFile, mainMP3File, precutFile, outputPath, keepSegments); err != nil {
		return res, err
	}

	res.CleanedSec = audio.GetAudioDuration(outputPath)
	res.CutSec = res.OriginalSec - res.CleanedSec
	return res, nil
}

func loadCutsMetadata(cutsPath string) (types.CutsData, error) {
	var cd types.CutsData
	data, err := os.ReadFile(cutsPath)
	if err != nil {
		return cd, fmt.Errorf("read cuts file %s: %w", filepath.Base(cutsPath), err)
	}
	if err := json.Unmarshal(data, &cd); err != nil {
		return cd, fmt.Errorf("parse cuts file %s: %w", filepath.Base(cutsPath), err)
	}
	return cd, nil
}

func extractKeepSegments(cutsData types.CutsData, totalDuration float64) ([][2]float64, float64, int) {
	if len(cutsData.KeepIntervals) > 0 {
		var segs [][2]float64
		for _, k := range cutsData.KeepIntervals {
			segs = append(segs, [2]float64{k.Start, k.End})
		}
		cutDur := cutsData.TotalCutDurationSec
		if cutDur == 0 && len(cutsData.MergedCutIntervals) > 0 {
			for _, m := range cutsData.MergedCutIntervals {
				cutDur += (m.End - m.Start)
			}
		}
		return segs, cutDur, len(cutsData.CutIntervals)
	}

	var existingAds []types.AdSegment
	for _, c := range cutsData.CutIntervals {
		existingAds = append(existingAds, types.AdSegment{Start: c.StartSec, End: c.EndSec, Reason: c.Reason})
	}
	if len(existingAds) > 0 {
		existingAds = format.MergeIntervals(existingAds)
	}
	keepSegs := format.CalculateKeepSegments(totalDuration, existingAds)
	cutDur := 0.0
	for _, ad := range existingAds {
		cutDur += (ad.End - ad.Start)
	}
	return keepSegs, cutDur, len(existingAds)
}

func executeCutProcessing(sourceAudio, mainMP3, precut, outputPath string, keepSegments [][2]float64) error {
	workDir := util.WorkDirFor(outputPath)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("create work dir %s: %w", workDir, err)
	}
	tempOutput := filepath.Join(workDir, filepath.Base(outputPath)+".tmp"+filepath.Ext(outputPath))
	if err := util.VerifyTempFile(tempOutput); err != nil {
		_ = os.RemoveAll(workDir)
		return err
	}

	ctx := context.Background()
	if err := audio.DefaultProcessor.Cut(ctx, sourceAudio, keepSegments, tempOutput); err != nil {
		_ = os.Remove(tempOutput)
		_ = os.RemoveAll(workDir)
		return fmt.Errorf("cut audio for %s: %w", filepath.Base(sourceAudio), err)
	}

	if outputPath == mainMP3 && sourceAudio == mainMP3 && util.FileExists(mainMP3) {
		if !util.FileExists(precut) {
			if err := os.Link(mainMP3, precut); err != nil {
				_ = util.CopyFileErr(mainMP3, precut)
			}
		}
	}

	if err := util.SafeMove(tempOutput, outputPath); err != nil {
		_ = os.Remove(tempOutput)
		_ = os.RemoveAll(workDir)
		return fmt.Errorf("install cut audio to %s: %w", outputPath, err)
	}
	_ = os.RemoveAll(workDir)
	return nil
}
