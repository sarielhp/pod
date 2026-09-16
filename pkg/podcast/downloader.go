package podcast

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pod/pkg/progress"
	"pod/pkg/util"
)

const PodcastUserAgent = "pod/1.0 (+https://github.com/sarielhp/pod; Podcast Downloader)"

type Downloader struct {
	Client *http.Client
}

func NewDownloader() *Downloader {
	return &Downloader{
		Client: &http.Client{
			Timeout: 30 * time.Minute,
		},
	}
}

func (d *Downloader) DownloadEpisode(ctx context.Context, enclosureURL, destPath string, rep progress.Reporter) error {
	if enclosureURL == "" {
		return fmt.Errorf("empty enclosure URL")
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("create dest directory: %w", err)
	}

	workDir := filepath.Join(filepath.Dir(destPath), util.WorkDirName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("create .work directory: %w", err)
	}

	tempPath := filepath.Join(workDir, filepath.Base(destPath)+".download")
	if err := util.VerifyTempFile(tempPath); err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if err := d.fetchToFile(ctx, enclosureURL, tempPath, rep); err != nil {
		return err
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("atomic rename %s to %s: %w", tempPath, destPath, err)
	}
	return nil
}

func (d *Downloader) fetchToFile(ctx context.Context, enclosureURL, tempPath string, rep progress.Reporter) error {
	req, err := http.NewRequestWithContext(ctx, "GET", enclosureURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", PodcastUserAgent)

	resp, err := d.Client.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("download HTTP %d %s", resp.StatusCode, resp.Status)
	}

	out, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("stream audio: %w", err)
	}
	if written == 0 {
		return fmt.Errorf("downloaded 0 bytes from %s", enclosureURL)
	}

	progress.Or(rep).Infof("Downloaded %s (%.2f MB)", filepath.Base(tempPath), float64(written)/(1024*1024))
	return nil
}

func downloadCoverImage(imageURL, destPath string) error {
	if strings.TrimSpace(imageURL) == "" {
		return fmt.Errorf("empty image URL")
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("create dest directory: %w", err)
	}

	workDir := filepath.Join(filepath.Dir(destPath), util.WorkDirName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("create .work directory: %w", err)
	}

	tempPath := filepath.Join(workDir, filepath.Base(destPath)+".cover.download")
	if err := util.VerifyTempFile(tempPath); err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tempPath)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", PodcastUserAgent)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download cover HTTP %d", resp.StatusCode)
	}

	out, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	const maxCoverSize = 10 * 1024 * 1024
	written, err := io.Copy(out, io.LimitReader(resp.Body, maxCoverSize))
	_ = out.Close()
	if err != nil {
		return err
	}
	if written == 0 {
		return fmt.Errorf("downloaded 0 bytes for cover")
	}

	return os.Rename(tempPath, destPath)
}
