package episode

import (
	"path/filepath"
	"pod/pkg/types"
	"pod/pkg/util"
	"strings"
	"time"
)

func applySourcePublication(path string, st *types.EpisodeStatusFile) {
	if !strings.HasSuffix(strings.ToLower(path), ".mp3") {
		return
	}
	if date, ok := SourcePublicationTime(path); ok {
		st.PublishedAt = ""
		st.PublicationSource = "source"
		if !date.IsZero() {
			st.PublishedAt = date.UTC().Format(time.RFC3339)
		}
	}
}

var publicationSourceMu util.SyncMutex
var publicationSourceRoot string
var publicationSourceDates map[string]time.Time

func SetPublicationSource(root string, dates map[string]time.Time) {
	publicationSourceMu.Lock()
	defer publicationSourceMu.Unlock()
	publicationSourceRoot, _ = filepath.Abs(root)
	publicationSourceDates = dates
}

func SourcePublicationTime(path string) (time.Time, bool) {
	publicationSourceMu.Lock()
	defer publicationSourceMu.Unlock()
	if publicationSourceDates == nil {
		return time.Time{}, false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return time.Time{}, false
	}
	rel, err := filepath.Rel(publicationSourceRoot, absolute)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return time.Time{}, false
	}
	return publicationSourceDates[absolute], true
}
