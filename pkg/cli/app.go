package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/clihelp"

	"pod/pkg/util"
)

// embeddedVersion is written once by main before anything else runs, and read
// whenever a command renders its version. The guard is not for that ordering —
// it is because tests write it too, and a test that does so runs alongside
// parallel tests that read it.
var (
	embeddedVersionMu util.Mutex
	embeddedVersion   string
)

// SetEmbeddedVersion sets the version string embedded from the build.
func SetEmbeddedVersion(v string) {
	embeddedVersionMu.Lock()
	defer embeddedVersionMu.Unlock()
	embeddedVersion = v
}

func embeddedVersionValue() string {
	embeddedVersionMu.Lock()
	defer embeddedVersionMu.Unlock()
	return embeddedVersion
}

func getVersion() string {
	if v := embeddedVersionValue(); v != "" {
		return v
	}
	if data, err := os.ReadFile("VERSION"); err == nil {
		return strings.TrimSpace(string(data))
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		if data, err := os.ReadFile(filepath.Join(execDir, "VERSION")); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return "dev"
}

func buildCLIApp(action *string, opts *CLIOptions) *clihelp.App {
	keepVal := -1
	countVal := -1
	fetchCountVal := 1

	return &clihelp.App{
		Name:                "pod",
		Description:         "Automatic Ad Segment Remover & Podcast Manager",
		UsageLine:           "pod [OPTIONS] <COMMAND>",
		Version:             getVersion(),
		GlobalNote:          "Run 'pod <command> --help' or 'pod help <command>' for command-specific options.",
		AbbrevCommands:      true,
		Pager:               true,
		InteractiveFallback: true,
		PersistentOptions: []clihelp.Option{
			clihelp.Bool(&opts.ShowExamples, "-E, --examples", false, "Show command examples"),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod fetch",
				Description: "Fetch and clean the latest episode for each active subscription",
			},
			{
				Line:        "pod queue run",
				Description: "Process ad removal on queued episodes",
			},
			{
				Line:        "pod server feeds",
				Description: "Check podcast feeds directly for newly published episodes",
			},
			{
				Line:        "pod server download",
				Description: "Download new episodes from server",
			},
			{
				Line:        "pod queue latest 10",
				Description: "Queue the 10 latest uncleaned episodes for ad removal",
			},
			{
				Line:        "pod tui",
				Description: "Launch interactive terminal UI browser",
			},
		},
		Commands: []clihelp.Command{
			buildFetchCommand(opts, action, &fetchCountVal),
			buildQueueCommand(opts, action),
			buildServerCommand(opts, action, &countVal, &keepVal),
			buildPlayerCommand(opts, action),
			buildInfoCommand(opts, action),
			buildConfigCommand(opts, action),
			buildUICommand(opts, action),
			buildRmAdsCommand(opts, action),
			buildDetectCommand(opts, action),
			buildRepeatsCommand(opts, action),
			buildGenRSSCommand(opts, action),
			buildTranscribeCommand(opts, action),
			buildCutCommand(opts, action),
		},
	}
}
