package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"pod/pkg/util"
	"strings"
)

func IsQueueAudioPath(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	if !strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, "precut.mp3") {
		return false
	}
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if part == ".work" {
			return false
		}
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func ResolveQueueAudioPath(dir, entry string) (string, error) {
	clean, err := cleanQueueEntry(entry)
	if err != nil {
		return "", err
	}
	if err := rejectSymlinkedComponents(dir, clean); err != nil {
		return "", err
	}

	path := filepath.Join(dir, clean)
	if IsQueueAudioPath(path) {
		return path, nil
	}
	if found, err := audioInsideQueueDir(path, entry); found != "" || err != nil {
		return found, err
	}
	if filepath.Base(clean) != clean {
		return "", fmt.Errorf("queued audio is missing: %s", path)
	}
	return queueAudioByName(dir, clean, entry)
}

// cleanQueueEntry rejects an entry that is absolute or reaches outside the
// podcast, and returns the cleaned relative form.
func cleanQueueEntry(entry string) (string, error) {
	if entry == "" || filepath.IsAbs(entry) {
		return "", fmt.Errorf("invalid queue path %q", entry)
	}
	clean := filepath.Clean(entry)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("queue path escapes podcast: %q", entry)
	}
	return clean, nil
}

// rejectSymlinkedComponents refuses a path any component of which is a
// symlink, so a queue entry cannot be used to reach outside the library.
func rejectSymlinkedComponents(dir, clean string) error {
	current := dir
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if info != nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("queue path contains symlink: %s", current)
		}
	}
	return nil
}

// audioInsideQueueDir finds the single episode audio inside a queued
// directory. An empty result with no error means the path is not a directory
// holding exactly one episode, and the caller should keep looking.
func audioInsideQueueDir(path, entry string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil
	}
	if !info.IsDir() {
		return "", fmt.Errorf("queue entry is not episode audio: %s", path)
	}
	subFiles, err := util.FindMP3FilesErr(path)
	if err != nil {
		return "", nil
	}
	var match string
	for _, sf := range subFiles {
		if !IsQueueAudioPath(sf) {
			continue
		}
		if match != "" {
			return "", fmt.Errorf("ambiguous queue entry %q; use an episode-relative path", entry)
		}
		match = sf
	}
	return match, nil
}

// queueAudioByName searches the podcast for the episode a bare name refers
// to, matching either the file itself or the directory holding it.
func queueAudioByName(dir, clean, entry string) (string, error) {
	files, err := util.FindMP3FilesErr(dir)
	if err != nil {
		return "", err
	}
	var match string
	for _, candidate := range files {
		if !IsQueueAudioPath(candidate) || !queueNameMatches(dir, candidate, clean) {
			continue
		}
		if match != "" {
			return "", fmt.Errorf("ambiguous queue entry %q; use an episode-relative path", entry)
		}
		match = candidate
	}
	if match == "" {
		return "", fmt.Errorf("queued audio is missing: %s", filepath.Join(dir, clean))
	}
	return match, nil
}

// queueNameMatches reports whether a candidate is what the entry names: the
// file itself, or the directory it sits in, with or without an extension.
func queueNameMatches(dir, candidate, clean string) bool {
	if filepath.Base(candidate) == clean {
		return true
	}
	rel, _ := filepath.Rel(dir, candidate)
	candidateDir := filepath.Dir(rel)
	if candidateDir == "." {
		return false
	}
	stem := util.StripExt(clean)
	leaf := filepath.Base(candidateDir)
	return candidateDir == clean || candidateDir == stem || leaf == clean || leaf == stem
}

func RemoveQueuedAudio(dir, path string) (bool, error) {
	removed := false
	err := UpdateQueue(dir, func(entries []string) []string {
		result := make([]string, 0, len(entries))
		for _, entry := range entries {
			resolved, err := ResolveQueueAudioPath(dir, entry)
			if err == nil && filepath.Clean(resolved) == filepath.Clean(path) {
				removed = true
			} else {
				result = append(result, entry)
			}
		}
		return result
	})
	if removed && err == nil {
		err = ClearQueuePriority(path)
	}
	return removed && err == nil, err
}

func ClearQueuePriority(path string) error {
	st, err := LoadEpisodeStatus(StatusPathFor(path))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Priority == 0 {
		return nil
	}
	st.Priority = 0
	return SaveEpisodeStatus(StatusPathFor(path), st)
}
