package podcast

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"pod/pkg/util"
	"strings"
	"time"
)

func RepairPublicationDates(root string, dates map[string]time.Time, dryRun bool) (int, int, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return 0, 0, err
	}
	statuses, caches := 0, 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".work" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(path, ".mp3.json") {
			return nil
		}
		audio := publicationAudioPath(root, strings.TrimSuffix(path, ".json"))
		if audio == "" {
			return nil
		}
		changed, err := repairStatusPublication(path, dates[audio], dryRun)
		if changed {
			statuses++
		}
		return err
	})
	if err != nil {
		return statuses, caches, err
	}
	for _, pod := range ScanPodcastDirsReadOnly(root) {
		changed, err := repairCachedPublication(pod.Dir, dates, dryRun)
		if changed {
			caches++
		}
		if err != nil {
			return statuses, caches, err
		}
	}
	return statuses, caches, nil
}

func repairStatusPublication(path string, date time.Time, dryRun bool) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(data, &record); err != nil {
		return false, fmt.Errorf("read status %s: %w", path, err)
	}
	if record == nil {
		return false, fmt.Errorf("invalid status %s", path)
	}
	want := ""
	if !date.IsZero() {
		want = date.UTC().Format(time.RFC3339)
	}
	var current, source string
	_ = json.Unmarshal(record["published_at"], &current)
	_ = json.Unmarshal(record["publication_source"], &source)
	if current == want && source == "source" {
		return false, nil
	}
	record["published_at"], _ = json.Marshal(want)
	record["publication_source"], _ = json.Marshal("source")
	return writePublicationRepair(path, record, dryRun)
}

func repairCachedPublication(dir string, dates map[string]time.Time, dryRun bool) (bool, error) {
	path := filepath.Join(CacheDirForPodcast(dir), "index.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(data, &record); err != nil {
		return false, err
	}
	var episodes []map[string]json.RawMessage
	if err := json.Unmarshal(record["episodes"], &episodes); err != nil {
		return false, err
	}
	changed := false
	for _, ep := range episodes {
		var audio, filename string
		var current int64
		_ = json.Unmarshal(ep["path"], &audio)
		_ = json.Unmarshal(ep["filename"], &filename)
		_ = json.Unmarshal(ep["published_at"], &current)
		if audio == "" {
			audio = filename
		}
		date := dates[publicationAudioPath(dir, audio)]
		var want int64
		if !date.IsZero() {
			want = date.UnixMilli()
		}
		if current == want {
			continue
		}
		ep["published_at"], _ = json.Marshal(want)
		changed = true
	}
	if !changed {
		return false, nil
	}
	record["episodes"], err = json.Marshal(episodes)
	if err != nil {
		return false, err
	}
	return writePublicationRepair(path, record, dryRun)
}

func writePublicationRepair(path string, record map[string]json.RawMessage, dryRun bool) (bool, error) {
	if dryRun {
		return true, nil
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	err = util.WriteFileAtomic(path, append(data, '\n'), info.Mode().Perm())
	return err == nil, err
}
