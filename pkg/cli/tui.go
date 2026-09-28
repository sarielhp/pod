package cli

import "github.com/sarielhp/clihelp"

func buildUICommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "tui",
		Description: "Interactive TUI browser for podcasts and episodes",
		UsageLine:   "pod tui [options] [directory]",
		Args:        clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.PodcastsDir, "--podcasts-dir <dir>", "", "Podcasts directory"),
			clihelp.Bool(&opts.Debug, "-d, --debug", false, "Enable debug mode with key logging and screen snapshots (F12)"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "tui"
			opts.Args = ctx.Args
			return nil
		},
	}
}
