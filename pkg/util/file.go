package util

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	visited := map[string]bool{canonicalPath(dir): true}
	return findMP3FilesHelper(dir, visited)
}

// canonicalPath is the identity a directory or file is deduplicated by. A
// library reached through a symlink, or a show with alias symlinks beside its
// real directory, must yield each file once, and the resolved path is the only
// name both routes share.
func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// findMP3FilesHelper walks dir, returning paths under dir as the caller named
// it. Real entries are taken before symlinked ones so that, when a show is
// reachable both ways, the path reported is the real directory's and matches
// what the podcast index was built from.
func findMP3FilesHelper(dir string, visited map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	var links []os.DirEntry
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			links = append(links, entry)
			continue
		}
		files = append(files, collectMP3Entry(dir, entry.Name(), entry.IsDir(), visited)...)
	}
	for _, entry := range links {
		fi, err := os.Stat(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		files = append(files, collectMP3Entry(dir, entry.Name(), fi.IsDir(), visited)...)
	}
	return files, nil
}

func collectMP3Entry(dir, name string, isDir bool, visited map[string]bool) []string {
	fullPath := filepath.Join(dir, name)
	key := canonicalPath(fullPath)
	if visited[key] {
		return nil
	}
	if isDir {
		visited[key] = true
		subFiles, err := findMP3FilesHelper(fullPath, visited)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot read subdirectory %s: %v\n", fullPath, err)
		}
		return subFiles
	}
	if !strings.HasSuffix(strings.ToLower(name), ".mp3") {
		return nil
	}
	visited[key] = true
	return []string{fullPath}
}

var RenameFn = os.Rename

var atomicSeq atomic.Uint64

// atomicTempPath names the scratch file for an atomic write, in the target's own
// directory so the final rename stays on one filesystem. The name is short and
// independent of the target's: a file name is limited to 255 bytes, and a
// target named close to that (a long episode title, in a script that spends two
// bytes per letter) left no room for a suffix, so the write failed with "file
// name too long" although the final name fitted. The ".tmp." keeps it matched
// by the library's ignore patterns.
func atomicTempPath(path string) string {
	name := fmt.Sprintf(".atomic.tmp.%d.%d.%d", os.Getpid(), time.Now().UnixNano(), atomicSeq.Add(1))
	return filepath.Join(filepath.Dir(path), name)
}

func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	tmp := atomicTempPath(path)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
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

// RejectSymlink fails when path is a symbolic link. Writers that open a file
// with O_CREATE|O_TRUNC follow links, so a planted link would let them write
// through to whatever it points at.
func RejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q is a symlink, refusing to write through it", path)
	}
	return nil
}
