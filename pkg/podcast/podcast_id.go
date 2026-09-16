package podcast

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/util"
)

type PodcastDirEntry struct {
	Dir        string
	FolderName string
	Title      string
	ShortID    string
}

func GeneratePodcastShortID(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		h := sha256.Sum256([]byte("podcast"))
		return hex.EncodeToString(h[:])[:5]
	}

	cleanWords := extractCleanAlphanumericWords(t)
	if len(cleanWords) >= 3 {
		if id, ok := shortIDFromManyWords(cleanWords); ok {
			return id
		}
	} else if len(cleanWords) == 2 {
		if id, ok := shortIDFromTwoWords(cleanWords[0], cleanWords[1]); ok {
			return id
		}
	} else if len(cleanWords) == 1 {
		if id, ok := shortIDFromOneWord(cleanWords[0]); ok {
			return id
		}
	}

	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])[:5]
}

func extractCleanAlphanumericWords(t string) []string {
	fields := strings.FieldsFunc(t, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == ':' || r == ',' || r == '.' || r == '\'' || r == '"'
	})

	var cleanWords []string
	for _, f := range fields {
		var b strings.Builder
		for _, r := range f {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			} else if r >= 'A' && r <= 'Z' {
				b.WriteRune(r + ('a' - 'A'))
			}
		}
		if b.Len() > 0 {
			cleanWords = append(cleanWords, b.String())
		}
	}
	return cleanWords
}

func shortIDFromManyWords(cleanWords []string) (string, bool) {
	var initials strings.Builder
	for _, w := range cleanWords {
		initials.WriteByte(w[0])
	}
	if initials.Len() >= 5 {
		return initials.String()[:5], true
	}
	var b strings.Builder
	for _, w := range cleanWords {
		b.WriteByte(w[0])
		for i := 1; i < len(w); i++ {
			c := w[i]
			if c != 'a' && c != 'e' && c != 'i' && c != 'o' && c != 'u' {
				b.WriteByte(c)
				if b.Len() == 5 {
					return b.String(), true
				}
			}
		}
	}
	if b.Len() >= 5 {
		return b.String()[:5], true
	}
	return "", false
}

func shortIDFromTwoWords(w1, w2 string) (string, bool) {
	var b strings.Builder
	b.WriteByte(w1[0])
	for i := 1; i < len(w1); i++ {
		if w1[i] != 'a' && w1[i] != 'e' && w1[i] != 'i' && w1[i] != 'o' && w1[i] != 'u' {
			b.WriteByte(w1[i])
		}
	}
	b.WriteByte(w2[0])
	for i := 1; i < len(w2); i++ {
		if w2[i] != 'a' && w2[i] != 'e' && w2[i] != 'i' && w2[i] != 'o' && w2[i] != 'u' {
			b.WriteByte(w2[i])
		}
	}
	if b.Len() >= 5 {
		return b.String()[:5], true
	}
	comb := w1 + w2
	if len(comb) >= 5 {
		return comb[:5], true
	}
	return "", false
}

func shortIDFromOneWord(w string) (string, bool) {
	var b strings.Builder
	b.WriteByte(w[0])
	for i := 1; i < len(w); i++ {
		if w[i] != 'a' && w[i] != 'e' && w[i] != 'i' && w[i] != 'o' && w[i] != 'u' {
			b.WriteByte(w[i])
		}
	}
	if b.Len() >= 5 {
		return b.String()[:5], true
	}
	if len(w) >= 5 {
		return w[:5], true
	}
	return "", false
}

func GetOrSetPodcastShortID(podDir, title string) string {
	cfg := config.LoadPodcastConfig(podDir, config.DefaultPodcastConfig(nil))
	if id := strings.TrimSpace(cfg.ID); id != "" {
		return id
	}

	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" || cleanTitle == "." {
		if c, _ := LoadPodcastCache(podDir); c != nil && strings.TrimSpace(c.PodcastName) != "" {
			cleanTitle = strings.TrimSpace(c.PodcastName)
		}
	}
	if cleanTitle == "" || cleanTitle == "." {
		cleanTitle = filepath.Base(podDir)
	}

	id := GeneratePodcastShortID(cleanTitle)
	cfg.ID = id
	_ = config.SavePodcastConfig(podDir, cfg)
	return id
}

func ScanPodcastDirs(podcastsDir string) []PodcastDirEntry {
	return scanPodcastDirs(podcastsDir, true)
}

func ScanPodcastDirsReadOnly(podcastsDir string) []PodcastDirEntry {
	return scanPodcastDirs(podcastsDir, false)
}

func scanPodcastDirs(podcastsDir string, persist bool) []PodcastDirEntry {
	if podcastsDir == "" {
		podcastsDir = "."
	}

	var entries []PodcastDirEntry
	dirEntries, err := os.ReadDir(podcastsDir)
	if err == nil {
		for _, de := range dirEntries {
			if !de.IsDir() || strings.HasPrefix(de.Name(), ".") || de.Name() == ".work" || strings.HasSuffix(de.Name(), "-1") {
				continue
			}
			podPath := filepath.Join(podcastsDir, de.Name())
			mp3s := util.FindMP3Files(podPath)
			cfgPath := filepath.Join(podPath, config.PodcastConfigFileName)
			_, errCfg := os.Stat(cfgPath)
			if len(mp3s) == 0 && errCfg != nil {
				continue
			}

			title := de.Name()
			if cached, _ := LoadPodcastCache(podPath); cached != nil && strings.TrimSpace(cached.PodcastName) != "" {
				title = strings.TrimSpace(cached.PodcastName)
			}
			entries = append(entries, PodcastDirEntry{
				Dir:        podPath,
				FolderName: de.Name(),
				Title:      title,
			})
		}
	}

	if len(entries) == 0 {
		mp3s := util.FindMP3Files(podcastsDir)
		if len(mp3s) > 0 {
			title := filepath.Base(podcastsDir)
			if cached, _ := LoadPodcastCache(podcastsDir); cached != nil && strings.TrimSpace(cached.PodcastName) != "" {
				title = strings.TrimSpace(cached.PodcastName)
			}
			entries = append(entries, PodcastDirEntry{
				Dir:        podcastsDir,
				FolderName: filepath.Base(podcastsDir),
				Title:      title,
			})
		}
	}

	assignUniqueShortIDsMode(entries, persist)
	return entries
}

func assignUniqueShortIDsMode(entries []PodcastDirEntry, persist bool) {
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].FolderName) < strings.ToLower(entries[j].FolderName)
	})

	seenIDs := make(map[string]string)
	for i := range entries {
		cfg := config.LoadPodcastConfig(entries[i].Dir, config.DefaultPodcastConfig(nil))
		candID := strings.TrimSpace(cfg.ID)
		if candID == "" || seenIDs[candID] != "" {
			candID = GeneratePodcastShortID(entries[i].Title)
		}
		if seenIDs[candID] != "" {
			h := sha256.Sum256([]byte(entries[i].Title))
			hexStr := hex.EncodeToString(h[:])
			for off := 0; off+5 <= len(hexStr); off++ {
				slice := hexStr[off : off+5]
				if seenIDs[slice] == "" {
					candID = slice
					break
				}
			}
		}
		seenIDs[candID] = entries[i].Dir
		entries[i].ShortID = candID
		if persist && cfg.ID != candID {
			cfg.ID = candID
			_ = config.SavePodcastConfig(entries[i].Dir, cfg)
		}
	}
}

func findPodcastDirForItem(item backend.Podcast, podcastsDir string) string {
	if podcastsDir == "" {
		return ""
	}

	title := item.Media.Metadata.Title
	if title != "" {
		p := filepath.Join(podcastsDir, title)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
		safeName := strings.Map(func(r rune) rune {
			if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
				return '_'
			}
			return r
		}, title)
		safeName = strings.TrimSpace(safeName)
		if safeName != "" {
			p := filepath.Join(podcastsDir, safeName)
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				return p
			}
		}
	}

	entries, err := os.ReadDir(podcastsDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(podcastsDir, e.Name())
			if cached, _ := LoadPodcastCache(dir); cached != nil {
				if cached.ABSItemID == item.ID || cached.ABSItemID == item.Media.ID || (cached.FeedURL != "" && cached.FeedURL == item.Media.Metadata.FeedURL) {
					return dir
				}
			}
			if title != "" && strings.EqualFold(e.Name(), title) {
				return dir
			}
		}
	}

	return ""
}
