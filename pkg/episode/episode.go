package episode

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"pod/pkg/pipeline"
	"pod/pkg/types"
	"pod/pkg/util"
)

// DetectPodcastDirForAudio finds the podcast directory enclosing an audio file.
func DetectPodcastDirForAudio(audioPath string) string {
	dir := filepath.Dir(audioPath)
	base := filepath.Base(audioPath)
	stem := util.StripExt(base)
	if strings.EqualFold(stem, "podcast") {
		parent := filepath.Dir(dir)
		if fi, err := os.Stat(parent); err == nil && fi.IsDir() {
			return parent
		}
	}
	return dir
}

// Episode represents an episode and its associated media and metadata paths on disk.
type Episode struct {
	Main       string
	Precut     string
	Source     string
	Output     string
	Transcript string
	Cuts       string
	WorkDir    string
	Dir        string
}

// Resolve constructs an Episode by resolving the audio files and associated paths for the given input path.
func Resolve(path string) (Episode, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return Episode{}, errors.New("empty path")
	}
	cleanPath = filepath.Clean(cleanPath)

	mainMP3File, precutFile, sourceAudioFile := pipeline.ResolveAudioFiles(cleanPath, false)
	baseName := util.StripExt(mainMP3File)
	dir := filepath.Dir(mainMP3File)

	return Episode{
		Main:       mainMP3File,
		Precut:     precutFile,
		Source:     sourceAudioFile,
		Output:     mainMP3File,
		Transcript: baseName + ".transcript.json",
		Cuts:       baseName + ".cuts.json",
		WorkDir:    util.WorkDirFor(mainMP3File),
		Dir:        dir,
	}, nil
}

// Status reads the current episode status file from disk.
func (e Episode) Status() (types.EpisodeStatusFile, error) {
	st, err := pipeline.LoadEpisodeStatus(pipeline.StatusPathFor(e.Main))
	if err != nil {
		return types.EpisodeStatusFile{}, err
	}
	if st == nil {
		return types.EpisodeStatusFile{}, errors.New("nil episode status")
	}
	return *st, nil
}

// Update mutates the episode status file atomically using the provided update function.
func (e Episode) Update(fn func(*types.EpisodeStatusFile)) error {
	return pipeline.UpdateEpisodeStatus(e.Main, fn)
}

// IsClean reports whether this episode has completed ad removal and has a valid transcript.
func (e Episode) IsClean() bool {
	return pipeline.IsEpisodeClean(e.Main)
}
