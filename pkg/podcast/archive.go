package podcast

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pod/pkg/episode"
	"pod/pkg/util"
)

// ArchiveManifestFileName is the cache file written to the SSD show folder.
const ArchiveManifestFileName = "archive.json"

// ArchivedEpisode represents metadata of an episode stored in cold HDD archive.
type ArchivedEpisode struct {
	Filename    string  `json:"filename"`
	Title       string  `json:"title"`
	GUID        string  `json:"guid,omitempty"`
	PubDate     string  `json:"pub_date,omitempty"`
	PublishedAt int64   `json:"published_at"`
	DurationSec float64 `json:"duration_sec,omitempty"`
	SizeBytes   int64   `json:"size_bytes,omitempty"`
	Description string  `json:"description,omitempty"`
	ReportHref  string  `json:"report_href,omitempty"`
	ArchivedAt  string  `json:"archived_at,omitempty"`
}

// ArchiveManifest represents the <podDir>/archive.json metadata cache.
type ArchiveManifest struct {
	Version   int               `json:"version"`
	UpdatedAt string            `json:"updated_at"`
	Episodes  []ArchivedEpisode `json:"episodes"`
}

// ArchiveManifestPath returns the path to archive.json for a podcast.
func ArchiveManifestPath(podDir string) string {
	return filepath.Join(podDir, ArchiveManifestFileName)
}

// LoadArchiveManifest loads the archive.json cache from podDir on fast SSD.
// If the file does not exist, an empty manifest is returned without error.
func LoadArchiveManifest(podDir string) (*ArchiveManifest, error) {
	path := ArchiveManifestPath(podDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ArchiveManifest{Version: 1, Episodes: []ArchivedEpisode{}}, nil
		}
		return nil, fmt.Errorf("read archive manifest %s: %w", path, err)
	}

	var m ArchiveManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse archive manifest %s: %w", path, err)
	}
	if m.Episodes == nil {
		m.Episodes = []ArchivedEpisode{}
	}
	return &m, nil
}

// SaveArchiveManifest writes the archive manifest atomically to podDir.
func SaveArchiveManifest(podDir string, m *ArchiveManifest) error {
	if m == nil {
		return nil
	}
	m.Version = 1
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	sort.SliceStable(m.Episodes, func(i, j int) bool {
		if m.Episodes[i].PublishedAt != m.Episodes[j].PublishedAt {
			return m.Episodes[i].PublishedAt > m.Episodes[j].PublishedAt
		}
		return m.Episodes[i].Filename < m.Episodes[j].Filename
	})

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal archive manifest: %w", err)
	}

	path := ArchiveManifestPath(podDir)
	return util.WriteFileAtomic(path, append(data, '\n'), 0644)
}

// AddOrUpdateArchivedEpisode records or updates an archived episode entry.
func AddOrUpdateArchivedEpisode(podDir string, ep ArchivedEpisode) error {
	m, err := LoadArchiveManifest(podDir)
	if err != nil {
		return err
	}

	found := false
	for i, existing := range m.Episodes {
		if existing.Filename == ep.Filename || (existing.GUID != "" && existing.GUID == ep.GUID) {
			m.Episodes[i] = ep
			found = true
			break
		}
	}
	if !found {
		m.Episodes = append(m.Episodes, ep)
	}
	return SaveArchiveManifest(podDir, m)
}

// ExtractArchivedEpisodeMeta reads all available metadata for path before it is pruned.
func ExtractArchivedEpisodeMeta(path, podDir string) ArchivedEpisode {
	relName := filepath.Base(path)
	if podDir != "" {
		if r, err := filepath.Rel(podDir, path); err == nil && r != "" && !strings.HasPrefix(r, "..") {
			relName = filepath.ToSlash(r)
		}
	}

	title := EpisodeTitleFromPath(path)
	guid := ""
	pubMs := int64(0)
	durSec := float64(0)
	var sizeBytes int64

	if fi, err := os.Stat(path); err == nil {
		sizeBytes = fi.Size()
		pubMs = fi.ModTime().UnixMilli()
	}
	if pubTime := GetEpisodePublicationTime(path); !pubTime.IsZero() {
		pubMs = pubTime.UnixMilli()
	}
	if id := episode.LoadIdentity(path); id != nil {
		guid = id.GUID
	}
	if st, _ := episode.LoadEpisodeStatus(episode.StatusPathFor(path)); st != nil {
		if st.Cleaned.DurationSec > 0 {
			durSec = st.Cleaned.DurationSec
		} else if st.Original.DurationSec > 0 {
			durSec = st.Original.DurationSec
		}
	}

	pubDateStr := ""
	if pubMs > 0 {
		pubDateStr = time.UnixMilli(pubMs).UTC().Format(time.RFC1123Z)
	}

	return ArchivedEpisode{
		Filename:    relName,
		Title:       title,
		GUID:        guid,
		PubDate:     pubDateStr,
		PublishedAt: pubMs,
		DurationSec: durSec,
		SizeBytes:   sizeBytes,
		Description: title,
		ReportHref:  episodeReportHref(podDir, relName),
		ArchivedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}

// TransferEpisodeToArchive moves an expired MP3 to the HDD archive mirror folder,
// mirrors any existing sidecars, removes the MP3 from SSD, and updates archive.json.
func TransferEpisodeToArchive(path, podDir, archiveDir string, dryRun bool) (ArchivedEpisode, int64, error) {
	meta := ExtractArchivedEpisodeMeta(path, podDir)
	bytesFreed := meta.SizeBytes

	precut := path + ".precut"
	if fi, err := os.Stat(precut); err == nil {
		bytesFreed += fi.Size()
	}
	if dryRun {
		return meta, bytesFreed, nil
	}

	folder := filepath.Base(podDir)
	destDir := filepath.Join(archiveDir, folder)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return meta, 0, fmt.Errorf("create archive directory %s: %w", destDir, err)
	}

	lock, err := util.AcquireFileLock(path)
	if err != nil {
		return meta, 0, fmt.Errorf("lock %s: %w", path, err)
	}
	if lock == nil {
		return meta, 0, fmt.Errorf("%s is being processed by another pod instance", path)
	}
	defer lock.Release()

	destPath := filepath.Join(destDir, filepath.Base(path))
	if err := safeCrossDeviceMove(path, destPath); err != nil {
		return meta, 0, fmt.Errorf("archive move %s -> %s: %w", path, destPath, err)
	}

	if util.FileExists(precut) {
		_ = os.Remove(precut)
	}

	mirrorSidecarsToArchive(path, destDir)

	if err := AddOrUpdateArchivedEpisode(podDir, meta); err != nil {
		return meta, bytesFreed, fmt.Errorf("update archive manifest: %w", err)
	}
	return meta, bytesFreed, nil
}

func safeCrossDeviceMove(src, dest string) error {
	tmpDest := dest + ".tmp"
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcStat, err := srcFile.Stat()
	if err != nil {
		return err
	}

	outFile, err := os.OpenFile(tmpDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(outFile, srcFile)
	syncErr := outFile.Sync()
	closeErr := outFile.Close()

	if copyErr != nil {
		_ = os.Remove(tmpDest)
		return copyErr
	}
	if syncErr != nil {
		_ = os.Remove(tmpDest)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpDest)
		return closeErr
	}

	destStat, err := os.Stat(tmpDest)
	if err != nil || destStat.Size() != srcStat.Size() {
		_ = os.Remove(tmpDest)
		return fmt.Errorf("size mismatch or stat error verifying copy (%d vs %d)", destStat.Size(), srcStat.Size())
	}

	if err := os.Rename(tmpDest, dest); err != nil {
		_ = os.Remove(tmpDest)
		return err
	}

	return os.Remove(src)
}

func mirrorSidecarsToArchive(srcPath, destDir string) {
	base := util.StripExt(srcPath)
	sidecarSuffixes := []string{
		".transcript.json",
		".transcript.txt",
		".txt",
		".srt",
		".status.json",
		".cuts.json",
		".report.html",
	}

	for _, suffix := range sidecarSuffixes {
		sidecar := base + suffix
		if util.FileExists(sidecar) {
			dest := filepath.Join(destDir, filepath.Base(sidecar))
			_ = util.CopyFileErr(sidecar, dest)
		}
	}
}
