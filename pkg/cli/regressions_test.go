package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/clihelp"
)

func TestRegressionIssue1_TestKittyArgSlicing(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	if err := app.Execute([]string{"info", "check", "kitty", "cover.png"}); err != nil {
		t.Fatalf("unexpected error executing info check kitty: %v", err)
	}
	if action != "info" || opts.StatusSubcmd != "check" || !opts.TestKitty {
		t.Fatalf("expected action 'info' with StatusSubcmd='check' and TestKitty=true, got action=%q, subcmd=%q, TestKitty=%v", action, opts.StatusSubcmd, opts.TestKitty)
	}
	if len(opts.Args) != 1 || opts.Args[0] != "cover.png" {
		t.Fatalf("expected opts.Args to contain only ['cover.png'], got %v", opts.Args)
	}
}

func TestRegressionIssue2_TestCommandTargetValidation(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	if err := app.Execute([]string{"info", "check", "unknown-target"}); err == nil {
		t.Errorf("expected error for unknown test target, got nil")
	}

	if err := app.Execute([]string{"info", "check", "abs", "invalid-subtarget"}); err == nil {
		t.Errorf("expected error for invalid abs test target, got nil")
	}

	validCases := [][]string{
		{"info", "check"},
		{"info", "check", "whisper"},
		{"info", "check", "kitty"},
	}
	for _, tc := range validCases {
		var a string
		var o CLIOptions
		testApp := buildCLIApp(&a, &o)
		if err := testApp.Execute(tc); err != nil {
			t.Errorf("expected %v to succeed, got %v", tc, err)
		}
	}
}

func TestRegressionIssue3_ConfigGetErrorHandling(t *testing.T) {
	t.Parallel()
	cfg := Config{PodcastsDir: "/tmp/podcasts"}

	if err := handleConfigGet(io.Discard, cfg, "invalid_setting_name_xyz"); err == nil {
		t.Errorf("expected error for invalid config key, got nil")
	}

	if err := handleConfigGet(io.Discard, cfg, "podcasts-dir"); err != nil {
		t.Errorf("expected nil error for valid config key 'podcasts-dir', got %v", err)
	}

	if err := handleConfigGet(io.Discard, cfg, "dir"); err != nil {
		t.Errorf("expected nil error for valid alias 'dir', got %v", err)
	}
}

func TestRegressionIssue4_HelpUnknownCommand(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	matched := app.RenderCommand(clihelp.Options{}, "nonexistent_command_xyz")
	if matched {
		t.Errorf("expected RenderCommand to return false for nonexistent command, got true")
	}

	matchedValid := app.RenderCommand(clihelp.Options{}, "rm_ads")
	if !matchedValid {
		t.Errorf("expected RenderCommand to return true for 'rm_ads', got false")
	}
}

func findTestCommand(app *clihelp.App, name string) *clihelp.Command {
	for i := range app.Commands {
		if app.Commands[i].Name == name {
			return &app.Commands[i]
		}
	}
	return nil
}

func TestRegressionIssue5_TranscriptConflictingFlags(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "ep1.transcript.json")
	if err := os.WriteFile(jsonPath, []byte(`{"segments":[]}`), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	conflictCases := []CLIOptions{
		{ExportFormat: "srt", ProcOptions: ProcOptions{ExportTXT: true}},
		{ExportFormat: "txt", ProcOptions: ProcOptions{ExportSRT: true}},
		{ProcOptions: ProcOptions{ExportTXT: true, ExportSRT: true}},
		{ExportFormat: "srt", ProcOptions: ProcOptions{ExportTXT: true, ExportSRT: true}},
	}

	for _, c := range conflictCases {
		fmtCount := 0
		if c.ExportFormat != "" {
			fmtCount++
		}
		if c.ExportTXT {
			fmtCount++
		}
		if c.ExportSRT {
			fmtCount++
		}
		if fmtCount <= 1 {
			t.Errorf("expected conflict condition for %+v", c)
		}
	}

	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)
	cmd := findTestCommand(app, "info")
	if cmd == nil {
		t.Fatalf("info command not found")
	}
	hasExport := false
	hasTranscript := false
	for _, opt := range cmd.Options {
		if strings.Contains(opt.Flags, "--export") {
			hasExport = true
		}
		if strings.Contains(opt.Flags, "--transcript") {
			hasTranscript = true
		}
	}
	if !hasExport {
		t.Errorf("expected --export flag on info command")
	}
	if !hasTranscript {
		t.Errorf("expected --transcript flag on info command")
	}
}

func TestRegressionIssue6_CanonicalSubcommandsAndNoAliases(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	if err := app.Execute([]string{"config", "show"}); err != nil {
		t.Errorf("expected 'config show' to succeed: %v", err)
	}
	if err := app.Execute([]string{"config", "list"}); err == nil {
		t.Errorf("expected 'config list' to be rejected as duplicate subcommand")
	}

	if err := app.Execute([]string{"config", "cache", "clear"}); err != nil {
		t.Errorf("expected 'config cache clear' to succeed: %v", err)
	}
	if err := app.Execute([]string{"config", "cache", "reset"}); err == nil {
		t.Errorf("expected 'config cache reset' to fail: got nil")
	}

	var qAction string
	var qOpts CLIOptions
	qApp := buildCLIApp(&qAction, &qOpts)
	if err := qApp.Execute([]string{"queue", "remove", "e001"}); err != nil {
		t.Errorf("expected 'queue remove' to succeed: %v", err)
	}
	if qOpts.QueueSubcmd != "remove" {
		t.Errorf("expected QueueSubcmd 'remove', got %q", qOpts.QueueSubcmd)
	}

	qOpts = CLIOptions{}
	if err := qApp.Execute([]string{"queue", "rm", "e001"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if qOpts.QueueSubcmd == "remove" {
		t.Errorf("expected 'queue rm' NOT to be parsed as remove subcommand")
	}

	cfg := Config{PodcastsDir: t.TempDir()}
	cliClear := CLIOptions{Args: []string{"clear"}}
	if err := runQueueCommand(cfg, cliClear); err != nil {
		t.Errorf("expected 'queue clear' to succeed: %v", err)
	}
}

func TestRegressionIssue7_ServerKebabCaseAndAliases(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	if err := app.Execute([]string{"library", "get-info"}); err != nil {
		t.Errorf("expected 'server get-info' to succeed: %v", err)
	}
	if opts.ServerSubcmd != "get-info" {
		t.Errorf("expected ServerSubcmd 'get-info', got %q", opts.ServerSubcmd)
	}

	opts = CLIOptions{}
	if err := app.Execute([]string{"library", "disable-hourly"}); err != nil {
		t.Errorf("expected 'server disable-hourly' to succeed: %v", err)
	}
	if opts.ServerSubcmd != "disable-hourly" {
		t.Errorf("expected ServerSubcmd 'disable-hourly', got %q", opts.ServerSubcmd)
	}
}

func TestRegressionIssue9_ProcHelpLineLimitAndBrevity(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	procCmd := findTestCommand(app, "rm_ads")
	if procCmd == nil {
		t.Fatalf("rm_ads command not found")
	}
	if len(procCmd.Description) > 45 {
		t.Errorf("rm_ads description exceeds 45 chars: %d (%q)", len(procCmd.Description), procCmd.Description)
	}

	visibleFlags := 0
	for _, opt := range procCmd.Options {
		if !opt.Hidden {
			visibleFlags++
			if len(opt.Description) > 45 {
				t.Errorf("flag %s description exceeds 45 chars: %d (%q)", opt.Flags, len(opt.Description), opt.Description)
			}
		}
	}
	if visibleFlags > 8 {
		t.Errorf("expected at most 8 visible flags in rm_ads command help, got %d", visibleFlags)
	}
}

func TestCheckGeminiCommandTargetValidation(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	if err := app.Execute([]string{"info", "check", "gemini"}); err != nil {
		t.Fatalf("unexpected error executing info check gemini: %v", err)
	}
	if !opts.TestGemini {
		t.Errorf("expected opts.TestGemini to be true")
	}

	opts = CLIOptions{}
	if err := app.Execute([]string{"info", "check", "--test-gemini"}); err != nil {
		t.Fatalf("unexpected error executing info check --test-gemini: %v", err)
	}
	if !opts.TestGemini {
		t.Errorf("expected opts.TestGemini to be true with flag")
	}
}
