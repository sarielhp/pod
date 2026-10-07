package cli

import (
	"fmt"
	"os"
	"strings"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"

	"github.com/sarielhp/clihelp"
)

func buildServerOPMLSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "opml",
		Description: "Import or export podcast subscriptions using OPML files",
		UsageLine:   "pod library opml <command> [args]",
		Subcommands: []clihelp.Command{
			buildServerOPMLImportSubcommand(opts, action),
			buildServerOPMLExportSubcommand(opts, action),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod library opml export podcasts.opml",
				Description: "Export all library podcast feeds into an OPML file",
			},
			{
				Line:        "pod library opml import subscriptions.opml",
				Description: "Import podcasts from an OPML file into the library",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "library"
			opts.ServerSubcmd = "opml"
			opts.SyncSubcmd = "opml"
			if len(ctx.Args) > 0 {
				switch strings.ToLower(ctx.Args[0]) {
				case "import":
					opts.OPMLSubcmd = "import"
					if len(ctx.Args) > 1 {
						opts.OPMLFile = ctx.Args[1]
					}
				case "export":
					opts.OPMLSubcmd = "export"
					if len(ctx.Args) > 1 {
						opts.OPMLFile = ctx.Args[1]
					}
				default:
					opts.OPMLFile = ctx.Args[0]
				}
			}
			return nil
		},
	}
}

func buildServerOPMLImportSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "import",
		Description: "Import podcast subscriptions from an OPML file into the library",
		UsageLine:   "pod library opml import <file> [options]",
		Parameters:  []clihelp.Param{{Name: "<file>", Description: "Path to the OPML file to import"}},
		Args:        clihelp.ExactArgs(1),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug output"),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod library opml import subscriptions.opml",
				Description: "Import podcasts from subscriptions.opml into the library",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "library"
			opts.ServerSubcmd = "opml"
			opts.SyncSubcmd = "opml"
			opts.OPMLSubcmd = "import"
			if len(ctx.Args) > 0 {
				opts.OPMLFile = ctx.Args[0]
			}
			return nil
		},
	}
}

func buildServerOPMLExportSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "export",
		Description: "Export the library podcast RSS feeds into an OPML file",
		UsageLine:   "pod library opml export <file> [options]",
		Parameters:  []clihelp.Param{{Name: "<file>", Description: "Path to write the exported OPML file"}},
		Args:        clihelp.ExactArgs(1),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug output"),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod library opml export podcasts.opml",
				Description: "Export all library podcast feeds into podcasts.opml",
			},
			{
				Line:        "pod library opml export ~/Downloads/podcasts.opml --verbose",
				Description: "Export feeds with detailed progress and show each feed URL",
			},
		},
		Notes: []clihelp.Note{
			{
				Heading: "What This Command Does",
				Text:    "Queries the active podcast server for all hosted podcast RSS feeds and compiles them into a standard OPML 2.0 XML file. This allows subscribing to your entire library on the server in any podcast player app in one swoop without adding feeds one by one.",
			},
			{
				Heading: "Importing Into AntennaPod",
				Text:    "To import all server podcasts into AntennaPod at once:\n1. Run 'pod library opml export podcasts.opml' and transfer the file to your mobile device (via Nextcloud, Syncthing, email, or USB).\n2. Open AntennaPod on your device.\n3. Navigate to Subscriptions -> tap the top-right menu (⋮) -> 'Import/Export'.\n4. Select 'OPML import' and choose the exported 'podcasts.opml' file.\n5. AntennaPod will subscribe to all server-provided podcast feeds in one single step.",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "library"
			opts.ServerSubcmd = "opml"
			opts.SyncSubcmd = "opml"
			opts.OPMLSubcmd = "export"
			if len(ctx.Args) > 0 {
				opts.OPMLFile = ctx.Args[0]
			}
			return nil
		},
	}
}

func handleServerOPML(cfg Config, cli CLIOptions) error {
	if cfg.BackendType == "standalone" || cfg.BackendType == "local" {
		return handleStandaloneOPML(cfg, cli)
	}
	b, err := backend.FromAppConfig(&cfg, reporter(cli))
	if err != nil {
		return handleStandaloneOPML(cfg, cli)
	}
	targetFile := cli.OPMLFile
	if targetFile == "" && len(cli.Args) > 0 {
		targetFile = cli.Args[0]
	}
	switch cli.OPMLSubcmd {
	case "export":
		if targetFile == "" {
			return fmt.Errorf("missing required <file> argument for 'pod library opml export <file>'")
		}
		data, err := b.ExportOPML(backend.OPMLExportOptions{Quiet: cli.Quiet, Verbose: cli.Verbose})
		if err != nil {
			return fmt.Errorf("OPML export failed: %w", err)
		}
		if err := os.WriteFile(targetFile, data, 0644); err != nil {
			return fmt.Errorf("failed to write OPML file: %w", err)
		}
		fmt.Fprintf(progressFor(cli), "Exported podcast subscriptions to %s\n", targetFile)
		return nil
	case "import":
		if targetFile == "" {
			return fmt.Errorf("missing required <file> argument for 'pod library opml import <file>'")
		}
		data, err := os.ReadFile(targetFile)
		if err != nil {
			return fmt.Errorf("failed to read OPML file: %w", err)
		}
		res, err := b.ImportOPML(data, backend.OPMLImportOptions{Quiet: cli.Quiet, Verbose: cli.Verbose})
		if err != nil {
			return fmt.Errorf("OPML import failed: %w", err)
		}
		fmt.Fprintf(progressFor(cli), "Imported %d new feed(s) from %s\n", res.Subscribed, targetFile)
		return nil
	default:
		return fmt.Errorf("must specify 'import <file>' or 'export <file>'")
	}
}

func handleStandaloneOPML(cfg Config, cli CLIOptions) error {
	store, err := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if err != nil {
		return fmt.Errorf("open subscriptions store: %w", err)
	}
	targetFile := cli.OPMLFile
	if targetFile == "" && len(cli.Args) > 0 {
		targetFile = cli.Args[0]
	}
	switch cli.OPMLSubcmd {
	case "export":
		if targetFile == "" {
			return fmt.Errorf("missing required <file> argument for 'pod library opml export <file>'")
		}
		data, err := store.ExportToOPML(cfg.ServerBaseURL)
		if err != nil {
			return fmt.Errorf("OPML export failed: %w", err)
		}
		if err := os.WriteFile(targetFile, data, 0644); err != nil {
			return fmt.Errorf("failed to write OPML file: %w", err)
		}
		fmt.Fprintf(progressFor(cli), "Exported podcast subscriptions to %s\n", targetFile)
		return nil
	case "import":
		if targetFile == "" {
			return fmt.Errorf("missing required <file> argument for 'pod library opml import <file>'")
		}
		data, err := os.ReadFile(targetFile)
		if err != nil {
			return fmt.Errorf("failed to read OPML file: %w", err)
		}
		n, err := store.ImportFromOPML(data)
		if err != nil {
			return fmt.Errorf("OPML import failed: %w", err)
		}
		fmt.Fprintf(progressFor(cli), "Imported %d new feed(s) from %s\n", n, targetFile)
		return nil
	default:
		return fmt.Errorf("must specify 'import <file>' or 'export <file>'")
	}
}
