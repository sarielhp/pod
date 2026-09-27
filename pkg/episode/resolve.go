package episode

import (
	"os"
	"path/filepath"
	"strings"

	"pod/pkg/util"
)

// ResolveOutputFile calculates the destination file path for cutting/output.
func ResolveOutputFile(mainMP3File string, output string, totalFiles int) string {
	if totalFiles > 1 && output != "" {
		if info, err := os.Stat(output); err == nil && info.IsDir() {
			return filepath.Join(output, filepath.Base(mainMP3File))
		}
	}
	if output != "" {
		return output
	}
	return mainMP3File
}

// ResolveAudioFiles determines the main MP3 file path, precut backup file path,
// and the source audio file to be read based on file existence.
func ResolveAudioFiles(inputFile string, verbose bool) (mainMP3File, precutFile, sourceAudioFile string) {
	mainMP3File, precutFile = inputFile, inputFile+".precut"
	if strings.HasSuffix(inputFile, ".precut") {
		mainMP3File, precutFile = strings.TrimSuffix(inputFile, ".precut"), inputFile
	}

	switch {
	case util.FileExists(precutFile):
		sourceAudioFile = precutFile
	case util.FileExists(mainMP3File):
		sourceAudioFile = mainMP3File
	default:
		sourceAudioFile = inputFile
	}
	return mainMP3File, precutFile, sourceAudioFile
}
