package podcast

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/backend"
	"pod/pkg/episode"
	"pod/pkg/types"
	"pod/pkg/util"
)

// enclosureURLOf is the audio URL of a feed episode.
func enclosureURLOf(fe backend.FeedEpisode) string {
	if fe.Enclosure != nil && fe.Enclosure.URL != "" {
		return fe.Enclosure.URL
	}
	return fe.EnclosureURL
}

// SyntheticGUID makes a stable identifier for an episode whose feed gave none, or
// that can no longer be found in its feed. Whatever is passed is hashed, so the
// same inputs always give the same GUID.
func SyntheticGUID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "pod:ep:" + hex.EncodeToString(sum[:8])
}

// feedGUID is the identifier to record for a feed episode: the feed's own GUID,
// or a synthetic one when the feed has none. The boolean is true for a synthetic one.
func feedGUID(fe backend.FeedEpisode) (string, bool) {
	if guid := strings.TrimSpace(fe.GUID); guid != "" {
		return guid, false
	}
	return SyntheticGUID(enclosureURLOf(fe), fe.Title, time.UnixMilli(GetPubMS(fe)).UTC().Format(time.RFC3339)), true
}

// IdentityOfFeedEpisode describes a feed episode for recording with its file.
func IdentityOfFeedEpisode(fe backend.FeedEpisode, feedURL string) types.EpisodeIdentity {
	guid, synthetic := feedGUID(fe)
	return types.EpisodeIdentity{
		GUID:         guid,
		Synthetic:    synthetic,
		Title:        strings.TrimSpace(fe.Title),
		FeedURL:      feedURL,
		EnclosureURL: enclosureURLOf(fe),
		PublishedAt:  GetPubMS(fe),
		Season:       fe.Season,
		Episode:      fe.Episode,
	}
}

// RecordFeedEpisode writes the identity of a freshly downloaded feed episode to
// its status file. A failure is not fatal to the download, so callers may ignore it.
func RecordFeedEpisode(audioPath string, fe backend.FeedEpisode, feedURL string) error {
	return episode.SaveIdentity(audioPath, IdentityOfFeedEpisode(fe, feedURL))
}

// LocalEpisodes finds the files of a podcast directory that hold given feed
// episodes. It goes by the identity recorded with each file; only a file with no
// identity, downloaded before identities were kept, is matched by the name it was
// given, which is the old and fragile way.
type LocalEpisodes struct {
	byGUID map[string]string
	byName map[string]string
}

// NewLocalEpisodes indexes the audio files under podDir.
func NewLocalEpisodes(podDir string) *LocalEpisodes {
	l := &LocalEpisodes{byGUID: map[string]string{}, byName: map[string]string{}}
	for _, f := range util.FindMP3Files(podDir) {
		if id := episode.LoadIdentity(f); id != nil {
			if _, seen := l.byGUID[id.GUID]; !seen {
				l.byGUID[id.GUID] = f
			}
			continue
		}
		l.addLegacyNames(f)
	}
	return l
}

func (l *LocalEpisodes) addLegacyNames(path string) {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ".mp3"))
	for _, key := range []string{name, strings.ToLower(StripEpisodeFilenamePrefix(name))} {
		if _, seen := l.byName[key]; !seen {
			l.byName[key] = path
		}
	}
}

// Find returns the file that holds a feed episode.
func (l *LocalEpisodes) Find(fe backend.FeedEpisode) (string, bool) {
	guid, _ := feedGUID(fe)
	if path, ok := l.byGUID[guid]; ok {
		return path, true
	}
	pubMs := GetPubMS(fe)
	var pubTime time.Time
	if pubMs > 0 {
		pubTime = time.UnixMilli(pubMs).UTC()
	}
	for _, key := range []string{
		strings.ToLower(strings.TrimSuffix(FormatEpisodeFilename(pubTime, fe.Episode, fe.Title), ".mp3")),
		strings.ToLower(SanitizeTitle(fe.Title)),
		strings.ToLower(strings.TrimSpace(fe.Title)),
	} {
		if path, ok := l.byName[key]; ok {
			return path, true
		}
	}
	return "", false
}

// NewEpisodeFilename names the file for a newly downloaded feed episode:
// "YYYY-MM-DD_" and a short code derived from the episode's GUID, for example
// "2026-09-28_3f9a1c07be.mp3". The name says nothing about the episode and does
// not need to: what the episode is lives in its recorded identity, and the date
// is kept only so a directory listing sorts by age. Because the code comes from
// the GUID, downloading the same episode twice gives the same name; if two
// different episodes should ever share a code, the longer of the two is used.
// The name is a fixed short length, so it cannot come near the file name limit
// however long or in whatever script the title is.
func NewEpisodeFilename(podDir string, fe backend.FeedEpisode) string {
	guid, _ := feedGUID(fe)
	sum := sha256.Sum256([]byte(guid))
	code := hex.EncodeToString(sum[:])
	prefix := ""
	if ms := GetPubMS(fe); ms > 0 {
		prefix = time.UnixMilli(ms).UTC().Format("2006-01-02") + "_"
	}
	for n := 10; n < len(code); n += 6 {
		name := prefix + code[:n] + ".mp3"
		if !fileHoldsOtherEpisode(filepath.Join(podDir, name), guid) {
			return name
		}
	}
	return prefix + code + ".mp3"
}

// fileHoldsOtherEpisode reports whether path exists and belongs to an episode
// other than the one with the given GUID.
func fileHoldsOtherEpisode(path, guid string) bool {
	if !util.FileExists(path) {
		return false
	}
	id := episode.LoadIdentity(path)
	return id == nil || id.GUID != guid
}
