package episode

import (
	"strings"

	"pod/pkg/util"
)

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
