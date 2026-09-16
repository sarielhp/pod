package podcast

import (
	"path/filepath"
	"pod/pkg/backend"
	"pod/pkg/pipeline"
	"strings"
	"time"
)

func SetPublicationSource(root string, dates map[string]time.Time) {
	pipeline.SetPublicationSource(root, dates)
}

func SourcePublicationTime(path string) (time.Time, bool) {
	return pipeline.SourcePublicationTime(path)
}

func LoadSourcePublicationDates(b backend.Backend, root string) (map[string]time.Time, error) {
	var episodes []backend.CatalogEpisode
	if index, ok := b.(backend.CatalogIndexer); ok {
		var err error
		episodes, err = index.CatalogEpisodes()
		if err != nil {
			return nil, err
		}
	} else {
		pods, err := b.Podcasts()
		if err != nil {
			return nil, err
		}
		for _, pod := range pods {
			for _, ep := range pod.Media.Episodes {
				if ep.AudioFile == nil || ep.AudioFile.Metadata == nil {
					continue
				}
				path := ep.AudioFile.Metadata.Path
				if path == "" {
					path = ep.AudioFile.Metadata.RelPath
				}
				episodes = append(episodes, backend.CatalogEpisode{AudioPath: path, PublishedAt: pipeline.ParseABSEpisodePublishedAt(&ep)})
			}
		}
	}
	return CatalogPublicationDates(root, episodes)
}

func CatalogPublicationDates(root string, episodes []backend.CatalogEpisode) (map[string]time.Time, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	dates := make(map[string]time.Time)
	for _, ep := range episodes {
		path := publicationAudioPath(root, ep.AudioPath)
		if path == "" {
			continue
		}
		var published time.Time
		if ep.PublishedAt > 0 {
			published = time.UnixMilli(ep.PublishedAt)
		}
		if previous, ok := dates[path]; ok && !previous.Equal(published) {
			published = time.Time{}
		}
		dates[path] = published
	}
	return dates, nil
}

func publicationAudioPath(root, path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	name := strings.ToLower(filepath.Base(path))
	if !strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, "precut.mp3") {
		return ""
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == ".work" {
			return ""
		}
	}
	return path
}
