package episode

import (
	"time"

	"pod/pkg/types"
)

// LoadIdentity returns the identity recorded for an episode's audio file, or nil
// when it has none: the episode was downloaded before identities were recorded,
// or is not a feed episode at all.
func LoadIdentity(audioPath string) *types.EpisodeIdentity {
	st, err := LoadEpisodeStatus(StatusPathFor(audioPath))
	if err != nil || st == nil || st.Identity == nil || st.Identity.GUID == "" {
		return nil
	}
	id := *st.Identity
	return &id
}

// SaveIdentity records which feed episode an audio file is, creating the status
// file if the episode has none. It also fills in the publication date the
// identity carries, unless a better-sourced one is already on record, so the
// date no longer has to be recovered from the file's name.
func SaveIdentity(audioPath string, id types.EpisodeIdentity) error {
	return UpdateEpisodeStatus(audioPath, func(st *types.EpisodeStatusFile) {
		st.Identity = &id
		if id.PublishedAt > 0 && st.PublishedAt == "" {
			st.PublishedAt = msToRFC3339(id.PublishedAt)
			st.PublicationSource = "feed"
		}
	})
}

func msToRFC3339(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
