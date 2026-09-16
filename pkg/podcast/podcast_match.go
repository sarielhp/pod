package podcast

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"pod/pkg/backend"
)

var ErrAmbiguousPodcast = errors.New("ambiguous podcast pattern")

type AmbiguousPodcastMatch struct {
	ID   string
	Name string
}

type AmbiguousPodcastError struct {
	Query   string
	Matches []AmbiguousPodcastMatch
}

func (e *AmbiguousPodcastError) Error() string {
	return FormatPodcastMatches(e.Matches)
}

func (e *AmbiguousPodcastError) Is(target error) bool {
	return target == ErrAmbiguousPodcast
}

func FormatPodcastMatches(matches []AmbiguousPodcastMatch) string {
	var sb strings.Builder
	limit := len(matches)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		sb.WriteString(fmt.Sprintf("%s | %s\n", matches[i].ID, matches[i].Name))
	}
	if len(matches) > 5 {
		sb.WriteString("...\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func newAmbiguousPodcastError(query string, matches []AmbiguousPodcastMatch) error {
	return &AmbiguousPodcastError{
		Query:   query,
		Matches: matches,
	}
}

func matchesPodcastName(name, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	n := strings.ToLower(strings.TrimSpace(name))
	if q == "" || n == "" {
		return false
	}
	if strings.Contains(n, q) {
		return true
	}
	if re, err := regexp.Compile("(?i)" + query); err == nil {
		return re.MatchString(name)
	}
	return false
}

// matchByName runs the fuzzy-name phase that both the local and backend
// matchers share: one hit wins, several are ambiguous, none falls through to
// the caller's ID lookup. The ID phases stay separate because they genuinely
// differ — local matches folder names case-insensitively, backend matches
// opaque server IDs exactly — and collapsing them would change which podcast a
// query selects.
func matchByName[T any](items []T, search string, names func(T) []string, describe func(T) AmbiguousPodcastMatch, byID func([]T, string) (*T, error)) (*T, error) {
	var hits []T
	for _, it := range items {
		for _, n := range names(it) {
			if n != "" && matchesPodcastName(n, search) {
				hits = append(hits, it)
				break
			}
		}
	}
	if len(hits) == 1 {
		return &hits[0], nil
	}
	if len(hits) >= 2 {
		described := make([]AmbiguousPodcastMatch, 0, len(hits))
		for _, h := range hits {
			described = append(described, describe(h))
		}
		return nil, newAmbiguousPodcastError(search, described)
	}
	return byID(items, search)
}

func resolveLocalPodcastByID(entries []PodcastDirEntry, search string) (*PodcastDirEntry, error) {
	var idMatches []PodcastDirEntry
	for _, p := range entries {
		if strings.EqualFold(p.ShortID, search) {
			idMatches = append(idMatches, p)
		}
	}
	if len(idMatches) == 1 {
		return &idMatches[0], nil
	}
	if len(idMatches) >= 2 {
		return nil, fmt.Errorf("multiple podcasts match ID %q", search)
	}
	if idx, err := strconv.Atoi(search); err == nil && idx >= 1 && idx <= len(entries) {
		p := entries[idx-1]
		return &p, nil
	}
	for _, p := range entries {
		if strings.EqualFold(p.FolderName, search) {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("podcast matching %q not found", search)
}

func MatchLocalPodcasts(entries []PodcastDirEntry, query string) (*PodcastDirEntry, error) {
	search := strings.TrimSpace(query)
	if search == "" {
		return nil, fmt.Errorf("empty podcast query")
	}

	return matchByName(entries, search, localMatchNames, describeLocalPodcast, resolveLocalPodcastByID)
}

func localMatchNames(p PodcastDirEntry) []string {
	title := p.Title
	if title == "" {
		title = p.FolderName
	}
	return []string{title, p.FolderName}
}

func describeLocalPodcast(p PodcastDirEntry) AmbiguousPodcastMatch {
	title := p.Title
	if title == "" {
		title = p.FolderName
	}
	return AmbiguousPodcastMatch{ID: p.ShortID, Name: title}
}

func backendPodcastTitle(p backend.Podcast) string {
	if p.Media.Metadata.Title != "" {
		return p.Media.Metadata.Title
	}
	if p.RelPath != "" {
		return filepath.Base(p.RelPath)
	}
	return p.ID
}

func resolveBackendPodcastByID(podcasts []backend.Podcast, search string) (*backend.Podcast, error) {
	var idMatches []backend.Podcast
	for i := range podcasts {
		if podcasts[i].ID == search || podcasts[i].Media.ID == search {
			idMatches = append(idMatches, podcasts[i])
		}
	}
	if len(idMatches) == 1 {
		return &idMatches[0], nil
	}
	if len(idMatches) >= 2 {
		return nil, fmt.Errorf("multiple podcasts match ID %q", search)
	}
	for i := range podcasts {
		title := backendPodcastTitle(podcasts[i])
		if strings.EqualFold(GeneratePodcastShortID(title), search) {
			return &podcasts[i], nil
		}
	}
	if idx, err := strconv.Atoi(search); err == nil && idx >= 1 && idx <= len(podcasts) {
		return &podcasts[idx-1], nil
	}
	return nil, fmt.Errorf("podcast matching %q not found on server", search)
}

func MatchBackendPodcasts(podcasts []backend.Podcast, query string) (*backend.Podcast, error) {
	search := strings.TrimSpace(query)
	if search == "" {
		return nil, fmt.Errorf("empty podcast query")
	}

	return matchByName(podcasts, search, backendMatchNames, describeBackendPodcast, resolveBackendPodcastByID)
}

func backendMatchNames(p backend.Podcast) []string {
	return []string{backendPodcastTitle(p)}
}

func describeBackendPodcast(p backend.Podcast) AmbiguousPodcastMatch {
	title := backendPodcastTitle(p)
	id := p.ID
	if id == "" {
		id = p.Media.ID
	}
	if id == "" {
		id = GeneratePodcastShortID(title)
	}
	return AmbiguousPodcastMatch{ID: id, Name: title}
}
