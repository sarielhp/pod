package podcast

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"pod/pkg/episode"
	"pod/pkg/types"
	"pod/pkg/util"
)

const (
	backupManifestName = "manifest.json"
	backupJournalName  = "renames.jsonl"
	backupStateDir     = "_state"
)

// errEpisodeBusy marks an episode left alone because another process holds it.
var errEpisodeBusy = errors.New("episode is busy")

// BackupManifest records what a metadata backup holds and where each part came from.
type BackupManifest struct {
	Dir      string            `json:"dir"`
	Files    int               `json:"files"`
	Bytes    int64             `json:"bytes"`
	Podcasts map[string]string `json:"podcasts"`
	State    map[string]string `json:"state,omitempty"`
}

// backupSkips are the files a metadata backup leaves out: audio, which is large
// and untouched by a rename, and files that are recreated on demand.
func backupSkips(name string) bool {
	for _, suffix := range []string{".mp3", ".precut", ".precut.mp3", ".lock", ".wav", ".fp"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// BackupMetadata copies everything about the episodes except their audio (the
// transcripts, cuts, status files, queues and feeds) into dest, and checks each
// copy against its original by SHA-256 before returning. Nothing is renamed until
// this has succeeded. stateFiles are extra files outside the podcast folders that
// a rename may rewrite.
func BackupMetadata(entries []PodcastDirEntry, stateFiles []string, dest string) (BackupManifest, error) {
	m := BackupManifest{Dir: dest, Podcasts: map[string]string{}, State: map[string]string{}}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return m, err
	}
	for _, entry := range entries {
		folder := filepath.Base(entry.Dir)
		m.Podcasts[folder] = entry.Dir
		for _, name := range listFileNames(entry.Dir) {
			if backupSkips(name) {
				continue
			}
			if err := backupFile(filepath.Join(entry.Dir, name), filepath.Join(dest, folder, name), &m); err != nil {
				return m, err
			}
		}
	}
	for _, f := range stateFiles {
		if !util.FileExists(f) {
			continue
		}
		m.State[filepath.Base(f)] = f
		if err := backupFile(f, filepath.Join(dest, backupStateDir, filepath.Base(f)), &m); err != nil {
			return m, err
		}
	}
	return m, writeJSON(filepath.Join(dest, backupManifestName), m)
}

func backupFile(src, dst string, m *BackupManifest) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("back up %s: %w", src, err)
	}
	if err := util.WriteFileAtomic(dst, data, 0o644); err != nil {
		return fmt.Errorf("back up %s: %w", src, err)
	}
	copied, err := os.ReadFile(dst)
	if err != nil || sha256.Sum256(copied) != sha256.Sum256(data) {
		return fmt.Errorf("back up %s: the copy does not match the original", src)
	}
	m.Files++
	m.Bytes += int64(len(data))
	return nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(path, append(data, '\n'), 0o644)
}

// RenameResult totals what applying the renames did.
type RenameResult struct {
	Episodes int
	Files    int
	Busy     int
	Failures []error
	// Paths maps each renamed file's old path to its new one.
	Paths map[string]string
}

// ApplyEpisodeRenames renames the episodes of the given plans, one at a time and
// each under its own file lock. Every file rename is written to the journal in
// backupDir as it happens, so an interrupted run can still be undone. If any step
// of an episode fails, that episode's earlier renames are reversed and it is left
// as it was; the other episodes carry on.
func ApplyEpisodeRenames(plans []RenamePlan, backupDir string) RenameResult {
	res := RenameResult{Paths: map[string]string{}}
	journal, err := os.OpenFile(filepath.Join(backupDir, backupJournalName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		res.Failures = append(res.Failures, fmt.Errorf("open the rename journal: %w", err))
		return res
	}
	defer journal.Close()
	for _, plan := range plans {
		renameOnePodcast(plan, journal, &res)
	}
	return res
}

func renameOnePodcast(plan RenamePlan, journal io.Writer, res *RenameResult) {
	moved := map[string]string{}
	for _, r := range plan.Renames {
		err := renameEpisode(plan.Dir, r, journal)
		switch {
		case errors.Is(err, errEpisodeBusy):
			res.Busy++
		case err != nil:
			res.Failures = append(res.Failures, fmt.Errorf("%s: %s: %w", plan.Title, r.OldStem, err))
		default:
			res.Episodes++
			res.Files += len(r.Files)
			moved[r.OldStem+".mp3"] = r.NewStem + ".mp3"
			for _, f := range r.Files {
				res.Paths[f.From] = f.To
			}
		}
	}
	if len(moved) > 0 {
		if err := refreshAfterRename(plan.Dir, moved); err != nil {
			res.Failures = append(res.Failures, fmt.Errorf("%s: %w", plan.Title, err))
		}
	}
}

func renameEpisode(dir string, r EpisodeRename, journal io.Writer) error {
	oldAudio := filepath.Join(dir, r.OldStem+".mp3")
	if episode.IsEpisodeInRemoteFlight(oldAudio) {
		return errEpisodeBusy
	}
	lock, err := util.AcquireFileLock(oldAudio)
	if err != nil {
		return err
	}
	if lock == nil {
		return errEpisodeBusy
	}
	// The short ID is kept in the status file; make sure it is written there
	// before the name it may have been derived from changes.
	GetOrSetEpisodeShortID(dir, "", oldAudio)

	var done []FileRename
	for _, f := range r.Files {
		if err := os.Rename(f.From, f.To); err != nil {
			rollback(done)
			lock.Release()
			return err
		}
		done = append(done, f)
		_ = json.NewEncoder(journal).Encode(f)
	}
	lock.Release()
	_ = os.Remove(oldAudio + ".lock")
	_ = os.Remove(oldAudio + ".json.lock")
	fixRecordedNames(filepath.Join(dir, r.NewStem+".mp3"), r)
	return nil
}

func rollback(done []FileRename) {
	for i := len(done) - 1; i >= 0; i-- {
		_ = os.Rename(done[i].To, done[i].From)
	}
}

// fixRecordedNames updates the file names a status file records about itself.
func fixRecordedNames(newAudio string, r EpisodeRename) {
	_ = episode.UpdateEpisodeStatus(newAudio, func(st *types.EpisodeStatusFile) {
		st.MediaFile = filepath.Base(newAudio)
		st.Original.Filename = swapStem(st.Original.Filename, r)
		st.Cleaned.Filename = swapStem(st.Cleaned.Filename, r)
	})
}

func swapStem(name string, r EpisodeRename) string {
	if strings.HasPrefix(name, r.OldStem+".") {
		return r.NewStem + name[len(r.OldStem):]
	}
	return name
}

// refreshAfterRename brings what refers to episodes by name up to date: the
// processing queue, and the cache of the podcast's listing, which is rebuilt
// from the files on demand.
func refreshAfterRename(dir string, moved map[string]string) error {
	cache := CacheDirForPodcast(dir)
	_ = os.Remove(filepath.Join(cache, "index.json"))
	_ = os.RemoveAll(filepath.Join(cache, "details"))
	return episode.UpdateQueue(dir, func(entries []string) []string {
		out := make([]string, len(entries))
		for i, e := range entries {
			if to, ok := moved[e]; ok {
				e = to
			}
			out[i] = e
		}
		return out
	})
}

// RewritePathsIn replaces old paths with new ones in a JSON state file, such as
// the player's queue. It reports whether the file changed.
func RewritePathsIn(file string, paths map[string]string) (bool, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	out := data
	for from, to := range paths {
		out = bytes.ReplaceAll(out, []byte(from), []byte(to))
	}
	if bytes.Equal(out, data) {
		return false, nil
	}
	return true, util.WriteFileAtomic(file, out, 0o644)
}

// UndoResult reports what undoing a rename restored.
type UndoResult struct {
	Renamed  int
	Restored int
}

// UndoEpisodeRenames reverses a rename using its backup directory: every
// journaled rename is undone, newest first, and then the backed-up metadata is
// copied back over the current files, restoring the status files, queues and
// player state to what they were before.
func UndoEpisodeRenames(backupDir string) (UndoResult, error) {
	var res UndoResult
	renames, err := readJournal(filepath.Join(backupDir, backupJournalName))
	if err != nil {
		return res, err
	}
	for i := len(renames) - 1; i >= 0; i-- {
		r := renames[i]
		if util.FileExists(r.To) && !util.FileExists(r.From) {
			if err := os.Rename(r.To, r.From); err != nil {
				return res, err
			}
			res.Renamed++
		}
	}
	restored, err := restoreBackup(backupDir)
	res.Restored = restored
	return res, err
}

func readJournal(path string) ([]FileRename, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []FileRename
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var r FileRename
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.From != "" {
			out = append(out, r)
		}
	}
	return out, sc.Err()
}

func restoreBackup(backupDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(backupDir, backupManifestName))
	if err != nil {
		return 0, fmt.Errorf("read the backup manifest: %w", err)
	}
	var m BackupManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return 0, err
	}
	restored := 0
	for folder, dir := range m.Podcasts {
		n, err := restoreTree(filepath.Join(backupDir, folder), dir)
		restored += n
		if err != nil {
			return restored, err
		}
	}
	for name, path := range m.State {
		if err := copyBack(filepath.Join(backupDir, backupStateDir, name), path); err != nil {
			return restored, err
		}
		restored++
	}
	return restored, nil
}

func restoreTree(from, to string) (int, error) {
	n := 0
	for _, name := range listFileNames(from) {
		if err := copyBack(filepath.Join(from, name), filepath.Join(to, name)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func copyBack(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(dst, data, 0o644)
}
