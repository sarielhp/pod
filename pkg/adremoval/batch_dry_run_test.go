package adremoval

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"pod/pkg/types"
)

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		if fi.IsDir() {
			snap[rel+string(filepath.Separator)] = "dir"
			return nil
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		snap[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	return snap
}

func diffTrees(before, after map[string]string) []string {
	var diffs []string
	for k, v := range before {
		av, ok := after[k]
		if !ok {
			diffs = append(diffs, "deleted: "+k)
		} else if av != v {
			diffs = append(diffs, "changed: "+k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			diffs = append(diffs, "created: "+k)
		}
	}
	sort.Strings(diffs)
	return diffs
}

func TestProcDryRunMutatesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pod := filepath.Join(dir, "Show")
	if err := os.MkdirAll(filepath.Join(pod, ".work"), 0755); err != nil {
		t.Fatal(err)
	}
	// Stand in for a concurrently running instance's in-flight cut output.
	if err := os.WriteFile(filepath.Join(pod, ".work", "ep.mp3.tmp.mp3"), []byte("90 minutes of cut audio"), 0644); err != nil {
		t.Fatal(err)
	}
	writeRealMP3(t, filepath.Join(pod, "ep.mp3"), 3)

	before := snapshotTree(t, dir)

	opts := types.ProcOptions{
		Quiet:  true,
		DryRun: true,
	}
	_, _ = ProcessFiles([]string{filepath.Join(pod, "ep.mp3")}, opts, types.Config{})

	if diffs := diffTrees(before, snapshotTree(t, dir)); len(diffs) > 0 {
		t.Errorf("--dry-run modified the tree:\n  %v", diffs)
	}
}
