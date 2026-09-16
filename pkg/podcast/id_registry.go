package podcast

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pod/pkg/config"
	"pod/pkg/pipeline"
	"pod/pkg/types"
	"pod/pkg/util"
)

type ResolvedType int

const (
	ResolvedTypeNone ResolvedType = iota
	ResolvedTypePodcast
	ResolvedTypeEpisode
)

type ResolvedPodcast struct {
	Dir        string               `json:"dir"`
	Title      string               `json:"title"`
	ShortID    string               `json:"short_id"`
	FolderName string               `json:"folder_name"`
	UUID       string               `json:"uuid,omitempty"`
	Config     config.PodcastConfig `json:"config"`
}

type ResolvedEpisode struct {
	Path           string                   `json:"path"`
	Filename       string                   `json:"filename"`
	Title          string                   `json:"title"`
	ShortID        string                   `json:"short_id"`
	PodcastDir     string                   `json:"podcast_dir"`
	PodcastTitle   string                   `json:"podcast_title"`
	PodcastShortID string                   `json:"podcast_short_id"`
	Status         *types.EpisodeStatusFile `json:"status,omitempty"`
}

type ResolvedID struct {
	Type    ResolvedType     `json:"type"`
	Podcast *ResolvedPodcast `json:"podcast,omitempty"`
	Episode *ResolvedEpisode `json:"episode,omitempty"`
}

func (r *ResolvedID) IsPodcast() bool {
	return r != nil && r.Type == ResolvedTypePodcast && r.Podcast != nil
}

func (r *ResolvedID) IsEpisode() bool {
	return r != nil && r.Type == ResolvedTypeEpisode && r.Episode != nil
}

func EpisodeTitleFromPath(audioPath string) string {
	base := filepath.Base(audioPath)
	stem := util.StripExt(base)
	if strings.EqualFold(stem, "podcast") {
		parent := filepath.Base(filepath.Dir(audioPath))
		if parent != "." && parent != "/" && parent != "" {
			return parent
		}
	}
	return stem
}

func DetectPodcastDirForAudio(audioPath string) string {
	dir := filepath.Dir(audioPath)
	base := filepath.Base(audioPath)
	stem := util.StripExt(base)
	if strings.EqualFold(stem, "podcast") {
		parent := filepath.Dir(dir)
		if fi, err := os.Stat(parent); err == nil && fi.IsDir() {
			return parent
		}
	}
	return dir
}

func episodeUniqueKey(podDir, audioPath string) string {
	if podDir != "" {
		if rel, err := filepath.Rel(podDir, audioPath); err == nil && rel != "." && rel != "" {
			return filepath.ToSlash(rel)
		}
	}
	title := EpisodeTitleFromPath(audioPath)
	if title != "" && title != "podcast" {
		return title
	}
	return filepath.Base(audioPath)
}

func generateEpisodeShortID(podShortID, epKey string) string {
	cleanKey := strings.TrimSpace(epKey)
	cleanPod := strings.ToLower(strings.TrimSpace(podShortID))
	h := sha256.Sum256([]byte(cleanPod + ":" + cleanKey))
	return "e" + hex.EncodeToString(h[:])[:5]
}

func GetOrSetEpisodeShortID(podDir, podShortID, audioPath string) string {
	if podDir == "" {
		podDir = DetectPodcastDirForAudio(audioPath)
	}
	if podShortID == "" {
		podShortID = GetOrSetPodcastShortID(podDir, filepath.Base(podDir))
	}
	id := EpisodeShortIDReadOnly(podDir, podShortID, audioPath)
	st := pipeline.GetOrCreateEpisodeStatus(audioPath)
	if st.ID != id {
		st.ID = id
		_ = pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(audioPath), st)
	}
	return id
}

func EpisodeShortIDReadOnly(podDir, podShortID, audioPath string) string {
	bogusID := generateEpisodeShortID(podShortID, "podcast")
	key := episodeUniqueKey(podDir, audioPath)

	statPath := pipeline.StatusPathFor(audioPath)
	if st, err := pipeline.LoadEpisodeStatus(statPath); err == nil && st != nil {
		id := strings.TrimSpace(st.ID)
		if id != "" && (id != bogusID || key == "podcast") {
			return id
		}
	}

	altStatPath := util.StripExt(audioPath) + ".json"
	if altStatPath != statPath {
		if st, err := pipeline.LoadEpisodeStatus(altStatPath); err == nil && st != nil {
			id := strings.TrimSpace(st.ID)
			if id != "" && (id != bogusID || key == "podcast") {
				return id
			}
		}
	}

	if cached, _ := LoadPodcastCache(podDir); cached != nil {
		for _, ep := range cached.Episodes {
			path := ep.Path
			if path == "" {
				path = ep.Filename
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(podDir, path)
			}
			if filepath.Clean(path) == filepath.Clean(audioPath) && strings.TrimSpace(ep.ID) != "" {
				id := strings.TrimSpace(ep.ID)
				if id != bogusID || key == "podcast" {
					return id
				}
			}
		}
	}

	return generateEpisodeShortID(podShortID, key)
}

func ResolveAnyID(podcastsDir, query string) (*ResolvedID, error) {
	search := strings.TrimSpace(query)
	if search == "" {
		return nil, fmt.Errorf("empty query")
	}

	if podcastsDir == "" {
		podcastsDir = "."
	}

	if res, ok := resolveDirectPath(podcastsDir, search); ok {
		return res, nil
	}

	podEntries := ScanPodcastDirs(podcastsDir)
	if len(podEntries) == 0 {
		return nil, fmt.Errorf("no podcasts found in %s", podcastsDir)
	}

	if res, ok := resolveByEpisodeShortID(podEntries, search); ok {
		return res, nil
	}

	matchedPod, err := MatchLocalPodcasts(podEntries, search)
	if err != nil && errors.Is(err, ErrAmbiguousPodcast) {
		return nil, err
	}
	if matchedPod != nil {
		cfg := config.LoadPodcastConfig(matchedPod.Dir, config.DefaultPodcastConfig(nil))
		return &ResolvedID{
			Type: ResolvedTypePodcast,
			Podcast: &ResolvedPodcast{
				Dir:        matchedPod.Dir,
				Title:      matchedPod.Title,
				ShortID:    matchedPod.ShortID,
				FolderName: matchedPod.FolderName,
				UUID:       cfg.ID,
				Config:     cfg,
			},
		}, nil
	}

	if res, ok := resolveByPodcastUUID(podEntries, search); ok {
		return res, nil
	}

	if res, ok := resolveByEpisodeFileOrTitle(podEntries, search); ok {
		return res, nil
	}

	if res, ok := resolveBySubstring(podEntries, search); ok {
		return res, nil
	}

	return nil, fmt.Errorf("identifier %q not found in %s", query, podcastsDir)
}

func resolveDirectPath(podcastsDir, search string) (*ResolvedID, bool) {
	candidates := []string{search}
	if !filepath.IsAbs(search) {
		candidates = append(candidates, filepath.Join(podcastsDir, search))
	}

	for _, cand := range candidates {
		fi, err := os.Stat(cand)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			title := filepath.Base(cand)
			if cached, _ := LoadPodcastCache(cand); cached != nil && strings.TrimSpace(cached.PodcastName) != "" {
				title = strings.TrimSpace(cached.PodcastName)
			}
			shortID := GetOrSetPodcastShortID(cand, title)
			cfg := config.LoadPodcastConfig(cand, config.DefaultPodcastConfig(nil))
			return &ResolvedID{
				Type: ResolvedTypePodcast,
				Podcast: &ResolvedPodcast{
					Dir:        cand,
					Title:      title,
					ShortID:    shortID,
					FolderName: filepath.Base(cand),
					UUID:       cfg.ID,
					Config:     cfg,
				},
			}, true
		}

		if strings.HasSuffix(strings.ToLower(cand), ".mp3") {
			return buildResolvedEpisodeFromPath(cand), true
		}
	}

	return nil, false
}

func buildResolvedEpisodeFromPath(audioPath string) *ResolvedID {
	podDir := DetectPodcastDirForAudio(audioPath)
	podTitle := filepath.Base(podDir)
	if cached, _ := LoadPodcastCache(podDir); cached != nil && strings.TrimSpace(cached.PodcastName) != "" {
		podTitle = strings.TrimSpace(cached.PodcastName)
	}
	podShortID := GetOrSetPodcastShortID(podDir, podTitle)
	epShortID := GetOrSetEpisodeShortID(podDir, podShortID, audioPath)
	epTitle := EpisodeTitleFromPath(audioPath)
	st := pipeline.GetOrCreateEpisodeStatus(audioPath)

	return &ResolvedID{
		Type: ResolvedTypeEpisode,
		Episode: &ResolvedEpisode{
			Path:           audioPath,
			Filename:       filepath.Base(audioPath),
			Title:          epTitle,
			ShortID:        epShortID,
			PodcastDir:     podDir,
			PodcastTitle:   podTitle,
			PodcastShortID: podShortID,
			Status:         st,
		},
	}
}

func resolveByEpisodeShortID(podEntries []PodcastDirEntry, search string) (*ResolvedID, bool) {
	if len(search) != 6 || !strings.HasPrefix(strings.ToLower(search), "e") {
		return nil, false
	}
	for _, p := range podEntries {
		mp3s := util.FindMP3Files(p.Dir)
		for _, mp3 := range mp3s {
			epID := GetOrSetEpisodeShortID(p.Dir, p.ShortID, mp3)
			if strings.EqualFold(epID, search) {
				return buildResolvedEpisodeFromParams(p, mp3, epID), true
			}
		}
	}
	return nil, false
}

func buildResolvedEpisodeFromParams(p PodcastDirEntry, mp3Path, epID string) *ResolvedID {
	epTitle := EpisodeTitleFromPath(mp3Path)
	st := pipeline.GetOrCreateEpisodeStatus(mp3Path)
	return &ResolvedID{
		Type: ResolvedTypeEpisode,
		Episode: &ResolvedEpisode{
			Path:           mp3Path,
			Filename:       filepath.Base(mp3Path),
			Title:          epTitle,
			ShortID:        epID,
			PodcastDir:     p.Dir,
			PodcastTitle:   p.Title,
			PodcastShortID: p.ShortID,
			Status:         st,
		},
	}
}

func resolveByPodcastUUID(podEntries []PodcastDirEntry, search string) (*ResolvedID, bool) {
	for _, p := range podEntries {
		cfg := config.LoadPodcastConfig(p.Dir, config.DefaultPodcastConfig(nil))
		cached, _ := LoadPodcastCache(p.Dir)
		matched := strings.EqualFold(cfg.ID, search) || (cached != nil && strings.EqualFold(cached.ABSItemID, search))
		if matched {
			return &ResolvedID{
				Type: ResolvedTypePodcast,
				Podcast: &ResolvedPodcast{
					Dir:        p.Dir,
					Title:      p.Title,
					ShortID:    p.ShortID,
					FolderName: p.FolderName,
					UUID:       cfg.ID,
					Config:     cfg,
				},
			}, true
		}
	}
	return nil, false
}

func resolveByEpisodeFileOrTitle(podEntries []PodcastDirEntry, search string) (*ResolvedID, bool) {
	cleanSearch := strings.ToLower(strings.TrimSuffix(search, filepath.Ext(search)))
	for _, p := range podEntries {
		mp3s := util.FindMP3Files(p.Dir)
		for _, mp3 := range mp3s {
			fn := filepath.Base(mp3)
			base := util.StripExt(fn)
			title := EpisodeTitleFromPath(mp3)
			if strings.EqualFold(fn, search) || strings.EqualFold(base, cleanSearch) || strings.EqualFold(title, search) {
				epID := GetOrSetEpisodeShortID(p.Dir, p.ShortID, mp3)
				return buildResolvedEpisodeFromParams(p, mp3, epID), true
			}
		}
	}
	return nil, false
}

func resolveBySubstring(podEntries []PodcastDirEntry, search string) (*ResolvedID, bool) {
	lower := strings.ToLower(search)
	for _, p := range podEntries {
		mp3s := util.FindMP3Files(p.Dir)
		for _, mp3 := range mp3s {
			fn := filepath.Base(mp3)
			title := EpisodeTitleFromPath(mp3)
			if strings.Contains(strings.ToLower(fn), lower) || strings.Contains(strings.ToLower(title), lower) {
				epID := GetOrSetEpisodeShortID(p.Dir, p.ShortID, mp3)
				return buildResolvedEpisodeFromParams(p, mp3, epID), true
			}
		}
	}
	return nil, false
}
