package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInfoCheckPlayerTargetParses(t *testing.T) {
	var action string
	var opts CLIOptions
	if err := buildCLIApp(&action, &opts).Execute([]string{"info", "check", "player"}); err != nil {
		t.Fatal(err)
	}
	if !opts.TestPlayer {
		t.Fatal("'info check player' did not select the player check")
	}
}

func TestReportPlayerBackends(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "cvlc"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	var out bytes.Buffer
	if err := reportPlayerBackends(&out); err != nil {
		t.Fatalf("cvlc alone should be usable: %v", err)
	}
	if !strings.Contains(out.String(), "will use: cvlc") || !strings.Contains(out.String(), "mpv is absent") {
		t.Fatalf("report does not name the selected player and the missing mpv:\n%s", out.String())
	}

	t.Setenv("PATH", t.TempDir())
	out.Reset()
	if err := reportPlayerBackends(&out); err == nil {
		t.Fatalf("no player installed, yet the check passed:\n%s", out.String())
	}
}
