package util

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundFloat(t *testing.T) {
	t.Parallel()
	if got := RoundFloat(3.14159, 2); got != 3.14 {
		t.Errorf("RoundFloat(3.14159, 2) = %f; want 3.14", got)
	}
	if got := RoundFloat(3.145, 2); got != 3.15 {
		t.Errorf("RoundFloat(3.145, 2) = %f; want 3.15", got)
	}
}

func TestShellQuote(t *testing.T) {
	t.Parallel()
	if got := ShellQuote("simple"); got != "'simple'" {
		t.Errorf("ShellQuote(simple) = %q; want 'simple'", got)
	}
	if got := ShellQuote("don't"); got != `'don'\''t'` {
		t.Errorf("ShellQuote(don't) = %q; want 'don'\\''t'", got)
	}
}

func TestRepeatStrAndTruncate(t *testing.T) {
	t.Parallel()
	if got := RepeatStr("ab", 3); got != "ababab" {
		t.Errorf("RepeatStr(ab, 3) = %q; want ababab", got)
	}
	if got := Truncate("hello world", 5); got != "he..." {
		t.Errorf("Truncate(hello world, 5) = %q; want he...", got)
	}
	if got := Truncate("hi", 5); got != "hi" {
		t.Errorf("Truncate(hi, 5) = %q; want hi", got)
	}
}

func TestTempFileValidation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	sourceAudio := filepath.Join(tmpDir, "episode.mp3")
	workDir := WorkDirFor(sourceAudio)

	if filepath.Base(filepath.Dir(workDir)) != WorkDirName {
		t.Errorf("expected parent of workDir to be .work, got %s", workDir)
	}

	validTemp := filepath.Join(tmpDir, ".work", "audio.tmp.mp3")
	if err := VerifyTempFile(validTemp); err != nil {
		t.Errorf("expected %s to be valid temp file: %v", validTemp, err)
	}

	invalidTemp := filepath.Join(tmpDir, "audio.tmp.mp3")
	if err := VerifyTempFile(invalidTemp); err == nil {
		t.Errorf("expected %s to fail temp file verification", invalidTemp)
	}
}

func TestFileLock(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "job")

	lock, err := AcquireFileLock(target)
	if err != nil {
		t.Fatalf("AcquireFileLock failed: %v", err)
	}
	if lock == nil {
		t.Fatal("expected lock to be acquired")
	}

	lock2, err := AcquireFileLockWithTimeout(target, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("AcquireFileLockWithTimeout error: %v", err)
	}
	if lock2 != nil {
		t.Fatal("expected second lock acquisition to fail")
	}

	lock.Release()
	_ = os.Remove(target + ".lock")
}

func TestDisplayNameRTL(t *testing.T) {
	t.Parallel()
	testCases := []string{
		"השבוע - פודקאסט הארץ",
		"המרקרים",
		"207be",
		"שיר אחד One Song",
		"Kan Hourly News כאן רשת ב חדשות - מהדורת השעה",
		"תרבות יום א' - הפודקאסט של גלריה",
		"שלום (עולם)",
	}
	for _, tc := range testCases {
		t.Logf("In: %q -> Out: %q", tc, DisplayName(tc))
	}
}

func TestWriteFileAtomicIfChanged(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "test.txt")

	// 1. Initial write when file does not exist.
	changed, err := WriteFileAtomicIfChanged(target, []byte("hello"), 0644)
	if err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on initial write")
	}

	info1, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	// 2. Write identical content: should skip write and return changed=false.
	changed, err = WriteFileAtomicIfChanged(target, []byte("hello"), 0644)
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false when content is identical")
	}

	info2, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("modtime changed: %v -> %v", info1.ModTime(), info2.ModTime())
	}

	// 3. Write different content: should write and return changed=true.
	changed, err = WriteFileAtomicIfChanged(target, []byte("world"), 0644)
	if err != nil {
		t.Fatalf("third write failed: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when content changed")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(data) != "world" {
		t.Errorf("got %q, want 'world'", string(data))
	}
}
