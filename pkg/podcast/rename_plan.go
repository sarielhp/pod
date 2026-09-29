package podcast

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/episode"
)

// FileRename moves one file of an episode.
type FileRename struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// EpisodeRename moves every file of one episode from its old stem to a new one.
type EpisodeRename struct {
	OldStem string       `json:"old_stem"`
	NewStem string       `json:"new_stem"`
	GUID    string       `json:"guid"`
	Files   []FileRename `json:"files"`
}

// Audio is the renamed audio file, before and after.
func (e EpisodeRename) Audio() FileRename {
	for _, f := range e.Files {
		if strings.HasSuffix(f.From, ".mp3") {
			return f
		}
	}
	return FileRename{}
}

// RenamePlan is what renaming the episodes of one podcast directory would do.
type RenamePlan struct {
	Title   string          `json:"title"`
	Dir     string          `json:"dir"`
	Renames []EpisodeRename `json:"renames"`
	// Unidentified are episodes left alone because no identity is recorded for them.
	Unidentified int `json:"unidentified"`
	// Already are episodes whose name is already the one they would be given.
	Already int `json:"already"`
	// Orphans are files that belong to no episode at all. They are not touched.
	Orphans int `json:"orphans"`
	// Skipped says why an episode was left alone, by old stem.
	Skipped map[string]string `json:"skipped,omitempty"`
}

// PlanEpisodeRenames works out the new names for a podcast directory without
// touching anything. Only audio files directly in the directory are planned.
func PlanEpisodeRenames(entry PodcastDirEntry) RenamePlan {
	plan := RenamePlan{Title: entry.Title, Dir: entry.Dir, Skipped: map[string]string{}}
	names := listFileNames(entry.Dir)
	stems := append(audioStems(names), audiolessStems(names)...)
	groups, orphans := groupFilesByStem(names, stems)
	plan.Orphans = orphans

	taken := map[string]bool{}
	for _, s := range stems {
		taken[s] = true
	}
	for _, stem := range stems {
		plan.planOne(entry.Dir, stem, groups[stem], taken)
	}
	return plan
}

func (p *RenamePlan) planOne(dir, stem string, files []string, taken map[string]bool) {
	audio := filepath.Join(dir, stem+".mp3")
	id := episode.LoadIdentity(audio)
	switch {
	case stem == "podcast":
		p.Skipped[stem] = "a single-episode folder named after its podcast"
		return
	case id == nil:
		p.Unidentified++
		return
	}
	newStem := EpisodeStem(id.GUID, publishedMsFor(audio, id.PublishedAt, stem), func(candidate string) bool {
		return taken[candidate] && candidate != stem
	})
	if newStem == stem {
		p.Already++
		return
	}
	rename := EpisodeRename{OldStem: stem, NewStem: newStem, GUID: id.GUID}
	for _, name := range files {
		rename.Files = append(rename.Files, FileRename{
			From: filepath.Join(dir, name),
			To:   filepath.Join(dir, newStem+name[len(stem):]),
		})
	}
	if clash := existingTarget(rename.Files); clash != "" {
		p.Skipped[stem] = "the new name " + filepath.Base(clash) + " already exists"
		return
	}
	taken[newStem] = true
	p.Renames = append(p.Renames, rename)
}

// publishedMsFor is the publication time to put in the new name: the one recorded
// in the identity, else the one on the episode's status, else the date its old
// name began with.
func publishedMsFor(audio string, identityMs int64, stem string) int64 {
	if identityMs > 0 {
		return identityMs
	}
	if t := GetEpisodePublicationTime(audio); !t.IsZero() {
		return t.UnixMilli()
	}
	if t, ok := ParseDatePrefix(stem); ok {
		return t.UnixMilli()
	}
	return 0
}

func existingTarget(files []FileRename) string {
	from := map[string]bool{}
	for _, f := range files {
		from[f.From] = true
	}
	for _, f := range files {
		if _, err := os.Lstat(f.To); err == nil && !from[f.To] {
			return f.To
		}
	}
	return ""
}

func listFileNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// episodeMarkers are the files that show a stem is an episode even when its audio
// is gone: a transcript, a cuts file or a status file. Other sidecars (subtitles,
// text) also occur beside unrelated files, so on their own they prove nothing.
var episodeMarkers = []string{".transcript.json", ".cuts.json", ".mp3.json", ".ads.truth.json"}

// audiolessStems are the stems that have an episode's markers but no audio file:
// the transcripts and status of episodes whose audio was pruned. They are still
// episodes, and are identified and renamed like any other.
func audiolessStems(names []string) []string {
	withAudio := map[string]bool{}
	for _, s := range audioStems(names) {
		withAudio[s] = true
	}
	seen := map[string]bool{}
	var stems []string
	for _, n := range names {
		for _, marker := range episodeMarkers {
			stem, ok := strings.CutSuffix(n, marker)
			if ok && stem != "" && !withAudio[stem] && !seen[stem] {
				seen[stem] = true
				stems = append(stems, stem)
			}
		}
	}
	sort.Strings(stems)
	return stems
}

// audioStems are the stems of the audio files: the name without its ".mp3".
func audioStems(names []string) []string {
	var stems []string
	for _, n := range names {
		if strings.HasSuffix(n, ".mp3") && !strings.HasSuffix(n, "precut.mp3") {
			stems = append(stems, strings.TrimSuffix(n, ".mp3"))
		}
	}
	return stems
}

// groupFilesByStem assigns every file to the audio file it belongs to: the one
// whose stem, followed by a dot, is the longest prefix of the file's name.
// Lock files are not carried along; they are recreated on demand.
func groupFilesByStem(names, stems []string) (map[string][]string, int) {
	longestFirst := append([]string(nil), stems...)
	sort.Slice(longestFirst, func(i, j int) bool { return len(longestFirst[i]) > len(longestFirst[j]) })
	groups := map[string][]string{}
	orphans := 0
	for _, n := range names {
		if strings.HasSuffix(n, ".lock") {
			continue
		}
		owner := ""
		for _, s := range longestFirst {
			if strings.HasPrefix(n, s+".") {
				owner = s
				break
			}
		}
		if owner == "" {
			if isEpisodeSidecar(n) {
				orphans++
			}
			continue
		}
		groups[owner] = append(groups[owner], n)
	}
	return groups, orphans
}

// isEpisodeSidecar tells an episode's leftover files from the podcast's own
// (podcast.json, feed.xml, cover art), so that only the former count as orphans.
func isEpisodeSidecar(name string) bool {
	for _, suffix := range []string{".transcript.json", ".cuts.json", ".mp3.json", ".srt", ".txt", ".ads.truth.json"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
