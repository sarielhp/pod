package util

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func FindMP3Files(dir string) []string {
	files, err := FindMP3FilesErr(dir)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Warning: failed to read directory %s: %v\n", dir, err)
	}
	return files
}

func FindMP3FilesErr(dir string) ([]string, error) {
	visited := make(map[string]bool)
	return findMP3FilesHelper(dir, visited)
}

func findMP3FilesHelper(dir string, visited map[string]bool) ([]string, error) {
	realPath, err := filepath.EvalSymlinks(dir)
	if err != nil {
		realPath = dir
	}
	if visited[realPath] {
		return nil, nil
	}
	visited[realPath] = true

	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".work" || strings.HasPrefix(name, ".") {
			continue
		}
		fullPath := filepath.Join(dir, name)
		if entry.IsDir() {
			subFiles, subErr := findMP3FilesHelper(fullPath, visited)
			if subErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: cannot read subdirectory %s: %v\n", fullPath, subErr)
			}
			files = append(files, subFiles...)
		} else if strings.HasSuffix(strings.ToLower(name), ".mp3") {
			files = append(files, fullPath)
		}
	}
	return files, nil
}

var RenameFn = os.Rename

func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	tmp := fmt.Sprintf("%s.tmp.%d.%d", path, os.Getpid(), time.Now().UnixNano())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := RenameFn(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// WriteFileAtomicIfChanged writes data to path atomically only if the file does
// not already exist or its existing contents differ from data. It reports
// whether the file was written.
func WriteFileAtomicIfChanged(path string, data []byte, perm os.FileMode) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err := WriteFileAtomic(path, data, perm); err != nil {
		return false, err
	}
	return true, nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func CopyFileErr(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func SafeMove(src, dst string) error {
	err := RenameFn(src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return fmt.Errorf("move %s -> %s: %w", src, dst, err)
	}

	tmp := dst + ".partial"
	if cErr := CopyFileErr(src, tmp); cErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("move %s -> %s: %w", src, dst, cErr)
	}
	if rErr := RenameFn(tmp, dst); rErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("move %s -> %s: %w", src, dst, rErr)
	}
	return os.Remove(src)
}
