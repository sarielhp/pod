package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetMP3DiskDurationNative_NonExistent(t *testing.T) {
	t.Parallel()
	dur := getMP3DiskDurationNative("/non/existent/path/audio.mp3")
	if dur != 0 {
		t.Fatalf("expected 0 for non-existent file, got %v", dur)
	}
}

func TestGetMP3DiskDurationNative_CorruptStreamReturnsZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	corruptFile := filepath.Join(dir, "corrupt.mp3")
	// Invalid MP3 bytes that fail decoding before reaching EOF cleanly
	corruptData := []byte{0xFF, 0xFB, 0x90, 0x64, 0x00, 0x00, 0x00, 0x00, 0x12, 0x34, 0x56}
	if err := os.WriteFile(corruptFile, corruptData, 0644); err != nil {
		t.Fatalf("failed writing test file: %v", err)
	}

	dur := getMP3DiskDurationNative(corruptFile)
	if dur != 0 {
		t.Fatalf("expected 0 on decode failure, got %v", dur)
	}
}
