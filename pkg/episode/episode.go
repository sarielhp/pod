package episode

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

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

// EpisodeTitleFromPath is the display title of an episode: the title recorded
// with the episode when there is one, and otherwise one derived from the file's
// name, which is all that episodes downloaded before identities were recorded have.
func EpisodeTitleFromPath(audioPath string) string {
	if id := LoadIdentity(audioPath); id != nil && strings.TrimSpace(id.Title) != "" {
		return id.Title
	}
	base := filepath.Base(audioPath)
	stem := util.StripExt(base)
	if strings.EqualFold(stem, "podcast") {
		parent := filepath.Base(filepath.Dir(audioPath))
		if parent != "." && parent != "/" && parent != "" {
			return parent
		}
	}
	return stem
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

	mainMP3File, precutFile, sourceAudioFile := ResolveAudioFiles(cleanPath, false)
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
	st, err := LoadEpisodeStatus(StatusPathFor(e.Main))
	if err != nil {
		return types.EpisodeStatusFile{}, err
	}
	if st == nil {
		return types.EpisodeStatusFile{}, errors.New("nil episode status")
	}
	return *st, nil
}

// IsClean reports whether this episode has completed ad removal and has a valid transcript.
func (e Episode) IsClean() bool {
	return IsEpisodeClean(e.Main)
}
