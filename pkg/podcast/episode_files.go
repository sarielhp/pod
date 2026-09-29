package podcast

import (
	"path/filepath"
)

// EpisodeFiles lists every file that belongs to an episode, by full path: the
// audio, its status, transcript in every form it was written (JSON, subtitles,
// text, the readable Markdown), cuts, labelled truth, and the uncut original. A
// file belongs to the episode whose name, followed by a dot, is the longest
// prefix of the file's name, so an episode is not credited with the files of one
// whose name merely begins the same way. Lock files are left out: they come and
// go and say nothing about the episode. The audio file need not exist, since an
// episode whose audio was pruned still has its transcript and status.
func EpisodeFiles(audioPath string) []string {
	dir := filepath.Dir(audioPath)
	stem := filepath.Base(audioPath)
	if ext := filepath.Ext(stem); ext != "" {
		stem = stem[:len(stem)-len(ext)]
	}
	names := listFileNames(dir)
	stems := append(audioStems(names), audiolessStems(names)...)
	stems = append(stems, stem)
	groups, _ := groupFilesByStem(names, stems)
	files := make([]string, 0, len(groups[stem]))
	for _, name := range groups[stem] {
		files = append(files, filepath.Join(dir, name))
	}
	return files
}
