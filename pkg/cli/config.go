package cli

import (
	"fmt"
	"pod/pkg/podcast"
	"strconv"
	"strings"

	"github.com/sarielhp/clihelp"
)

func buildConfigCommand(opts *CLIOptions, action *string) clihelp.Command {
	subcmds := buildConfigBasicSubcommands(opts, action)
	subcmds = append(subcmds,
		buildConfigLLMSubcommand(opts, action),
		buildConfigWhisperSubcommand(opts, action),
		buildConfigCacheSubcommand(opts, action),
		buildConfigProcessorSubcommand(opts, action),
		buildConfigMigrateSubcommand(opts, action),
		buildConfigCompletionCommand(),
	)

	return clihelp.Command{
		Name:        "config",
		Description: "View and manage application configuration",
		UsageLine:   "pod config [command]",
		Subcommands: subcmds,
		Run: func(ctx *clihelp.Context) error {
			*action = "config"
			if len(ctx.Args) > 0 {
				return fmt.Errorf("unknown config subcommand %q", ctx.Args[0])
			}
			opts.ConfigCmd = "show"
			return nil
		},
	}
}

func buildConfigBasicSubcommands(opts *CLIOptions, action *string) []clihelp.Command {
	return []clihelp.Command{
		{
			Name:        "get",
			Description: "Get the value of a configuration key",
			UsageLine:   "pod config get <key>",
			Parameters: []clihelp.Param{
				{Name: "<key>", Description: "Configuration key name (e.g., 'podcasts-dir', 'whisper-url', 'gemini-model')"},
			},
			Args: clihelp.ExactArgs(1),
			Run: func(ctx *clihelp.Context) error {
				*action = "config"
				opts.ConfigCmd = "get"
				opts.ConfigKey = ctx.Args[0]
				return nil
			},
		},
		{
			Name:        "set",
			Description: "Set the value of a configuration key",
			UsageLine:   "pod config set <key> <value>",
			Parameters: []clihelp.Param{
				{Name: "<key>", Description: "Configuration key name"},
				{Name: "<value>", Description: "New configuration value"},
			},
			Args: clihelp.ExactArgs(2),
			Run: func(ctx *clihelp.Context) error {
				*action = "config"
				opts.ConfigCmd = "set"
				opts.ConfigKey = ctx.Args[0]
				opts.ConfigVal = ctx.Args[1]
				return nil
			},
		},
		{
			Name:        "show",
			Description: "Display current configuration summary table",
			UsageLine:   "pod config show",
			Args:        clihelp.NoArgs,
			Run: func(_ *clihelp.Context) error {
				*action = "config"
				opts.ConfigCmd = "show"
				return nil
			},
		},
	}
}

func buildConfigLLMSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "llm",
		Description: "Manage LLM profiles for ad detection",
		Subcommands: []clihelp.Command{
			buildConfigLLMTestSubcommand(opts, action),
			{
				Name:        "list",
				Description: "List all configured LLM profiles",
				UsageLine:   "pod config llm list",
				Args:        clihelp.NoArgs,
				Run: func(_ *clihelp.Context) error {
					*action = "config"
					opts.ConfigCmd = "llm-list"
					return nil
				},
			},
			{
				Name:        "default",
				Description: "Set default LLM profile ID",
				UsageLine:   "pod config llm default <id>",
				Parameters: []clihelp.Param{
					{Name: "<id>", Description: "Profile ID to set as default"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					*action = "config"
					opts.ConfigCmd = "llm-default"
					opts.ConfigVal = ctx.Args[0]
					return nil
				},
			},
			{
				Name:        "import",
				Description: "Import LLM settings from OpenCode",
				UsageLine:   "pod config llm import",
				Args:        clihelp.NoArgs,
				Run: func(_ *clihelp.Context) error {
					*action = "config"
					opts.ConfigCmd = "llm-import"
					return nil
				},
			},
		},
		Run: func(_ *clihelp.Context) error {
			*action = "config"
			opts.ConfigCmd = "llm-list"
			return nil
		},
	}
}

func buildConfigWhisperSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "whisper",
		Description: "Manage Whisper transcription profiles",
		Subcommands: []clihelp.Command{
			{
				Name:        "list",
				Description: "List all configured Whisper profiles",
				UsageLine:   "pod config whisper list",
				Args:        clihelp.NoArgs,
				Run: func(_ *clihelp.Context) error {
					*action = "config"
					opts.ConfigCmd = "whisper-list"
					return nil
				},
			},
			{
				Name:        "default",
				Description: "Set default Whisper profile ID",
				UsageLine:   "pod config whisper default <id>",
				Parameters: []clihelp.Param{
					{Name: "<id>", Description: "Whisper profile ID to set as default"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					*action = "config"
					opts.ConfigCmd = "whisper-default"
					opts.ConfigVal = ctx.Args[0]
					return nil
				},
			},
			{
				Name:        "add",
				Description: "Add a new Whisper profile (name:url:speed[:container[:lang[:prompt]]])",
				UsageLine:   "pod config whisper add <spec>",
				Parameters: []clihelp.Param{
					{Name: "<spec>", Description: "Profile specification formatted string"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					*action = "config"
					opts.ConfigCmd = "whisper-add"
					opts.ConfigVal = ctx.Args[0]
					return nil
				},
			},
			{
				Name:        "del",
				Description: "Remove a Whisper profile by ID",
				UsageLine:   "pod config whisper del <id>",
				Parameters: []clihelp.Param{
					{Name: "<id>", Description: "Whisper profile ID to delete"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					*action = "config"
					opts.ConfigCmd = "whisper-del"
					opts.ConfigVal = ctx.Args[0]
					return nil
				},
			},
		},
		Run: func(_ *clihelp.Context) error {
			*action = "config"
			opts.ConfigCmd = "whisper-list"
			return nil
		},
	}
}

func buildConfigCacheSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "cache",
		Description: "Manage and clear local cache",
		UsageLine:   "pod config cache [clear]",
		Parameters: []clihelp.Param{
			{Name: "[clear]", Description: "Clear local cache"},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			*action = "config"
			if len(ctx.Args) == 0 {
				opts.ConfigCmd = "cache-show"
				return nil
			}
			switch strings.ToLower(ctx.Args[0]) {
			case "clear":
				opts.ConfigCmd = "cache-reset"
			case "show":
				opts.ConfigCmd = "cache-show"
			default:
				return fmt.Errorf("unknown cache action %q (want clear or show)", ctx.Args[0])
			}
			return nil
		},
	}
}

func buildConfigProcessorSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "processor",
		Description: "Manage post-processing programs",
		Subcommands: []clihelp.Command{
			{
				Name:        "set",
				Description: "Add or replace a post-processor program",
				UsageLine:   "pod config processor set <program>",
				Parameters: []clihelp.Param{
					{Name: "<program>", Description: "The command line or path of the program to run"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					*action = "config"
					opts.ProcessorCmd = "set"
					opts.ProcessorValue = ctx.Args[0]
					return nil
				},
			},
			{
				Name:        "list",
				Description: "List configured post-processor programs",
				UsageLine:   "pod config processor list",
				Args:        clihelp.NoArgs,
				Run: func(_ *clihelp.Context) error {
					*action = "config"
					opts.ProcessorCmd = "list"
					return nil
				},
			},
			{
				Name:        "del",
				Description: "Remove a post-processor program by number",
				UsageLine:   "pod config processor del <number>",
				Parameters: []clihelp.Param{
					{Name: "<number>", Description: "The index or number of the post-processor to delete"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					*action = "config"
					opts.ProcessorCmd = "del"
					opts.ProcessorValue = ctx.Args[0]
					return nil
				},
			},
		},
		Run: func(_ *clihelp.Context) error {
			*action = "config"
			opts.ProcessorCmd = "list"
			return nil
		},
	}
}

func buildConfigMigrateSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "migrate",
		Description: "Migrate configuration from legacy podcasts_manager",
		UsageLine:   "pod config migrate [source]",
		Parameters: []clihelp.Param{
			{Name: "[source]", Description: "Optional migration source ('pm', 'podcasts_manager', or 'all')"},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			*action = "config"
			opts.ConfigCmd = "migrate"
			if len(ctx.Args) > 0 {
				opts.ConfigVal = ctx.Args[0]
			}
			return nil
		},
	}
}

func buildConfigCompletionCommand() clihelp.Command {
	cmd := clihelp.CompletionCommand()
	cmd.Examples = []clihelp.Example{
		{Line: "config completion zsh", Description: "Generate Zsh tab-completion script"},
		{Line: "config completion install", Description: "Install tab-completions for the active shell"},
	}
	for i := range cmd.Subcommands {
		if cmd.Subcommands[i].Name == "install" {
			cmd.Subcommands[i].Examples = []clihelp.Example{
				{Line: "config completion install zsh", Description: "Install completions to ~/.local/share/zsh/site-functions"},
			}
		}
	}
	return cmd
}

func runConfigCommand(config *Config, cli CLIOptions) error {
	switch cli.ConfigCmd {
	case "get":
		if err := handleConfigGet(outFor(cli), *config, cli.ConfigKey); err != nil {
			return err
		}
	case "set":
		if err := handleConfigSet(outFor(cli), config, cli.ConfigKey, cli.ConfigVal); err != nil {
			return err
		}
	case "show":
		printConfig(outFor(cli), *config)
	case "llm-list":
		listProfiles(outFor(cli), *config)
	case "llm-test":
		return testLLMProfile(outFor(cli), *config, cli.ConfigVal)
	case "llm-default":
		id, err := strconv.Atoi(cli.ConfigVal)
		if err != nil || id <= 0 {
			return fmt.Errorf("invalid profile ID %q", cli.ConfigVal)
		}
		return setDefaultProfile(outFor(cli), config, id)
	case "llm-import":
		return copyLLMFromOpenCode(outFor(cli), config)
	case "whisper-list", "whisper-default", "whisper-add", "whisper-del":
		return runWhisperConfig(config, cli)
	case "cache-show":
		dir, entries, size := podcast.CacheStats()
		fmt.Fprintf(outFor(cli), "Cache directory: %q\n", dir)
		fmt.Fprintf(outFor(cli), "  entries: %d\n", entries)
		fmt.Fprintf(outFor(cli), "  size:    %.1f MB\n", float64(size)/(1024*1024))
		fmt.Fprintln(outFor(cli), "Run 'pod config cache clear' to delete it.")
	case "cache-reset":
		if err := podcast.ResetCache(); err != nil {
			return fmt.Errorf("error resetting cache: %w", err)
		}
		fmt.Fprintln(progressFor(cli), "Cache reset successfully.")
	case "migrate":
		return handleConfigMigrate(outFor(cli), config, cli.ConfigVal)
	default:
		if cli.ProcessorCmd != "" {
			return handleConfigProcessor(outFor(cli), config, cli.ProcessorCmd, cli.ProcessorValue)
		}
		if cli.PodcastsDir != "" {
			return setPodcastsDir(outFor(cli), config, cli.PodcastsDir)
		}
		printConfig(outFor(cli), *config)
	}
	return nil
}

func runWhisperConfig(config *Config, cli CLIOptions) error {
	switch cli.ConfigCmd {
	case "whisper-list":
		listWhispers(outFor(cli), *config)
	case "whisper-default":
		id, err := strconv.Atoi(cli.ConfigVal)
		if err != nil || id <= 0 {
			return fmt.Errorf("invalid Whisper profile ID %q", cli.ConfigVal)
		}
		return setDefaultWhisperProfile(outFor(cli), config, id)
	case "whisper-add":
		return addWhisperProfile(outFor(cli), config, cli.ConfigVal)
	case "whisper-del":
		id, err := strconv.Atoi(cli.ConfigVal)
		if err != nil || id <= 0 {
			return fmt.Errorf("invalid Whisper profile ID %q", cli.ConfigVal)
		}
		return removeWhisperProfile(outFor(cli), config, id)
	}
	return nil
}
