package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"

	"pod/pkg/player"
	"pod/pkg/podcast"
)

func buildServerRenameEpisodesSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "rename-episodes",
		Description: "Rename episode files to short codes, after backing up every transcript and status file",
		UsageLine:   "pod server rename-episodes [-p <podcast>] [--dry-run] [--backup <dir>] [-f] | --undo <backup>",
		Args:        clihelp.MaximumNArgs(0),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Limit to one podcast"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Show what would be renamed without changing anything"),
			clihelp.String(&opts.RenameBackup, "--backup <dir>", "", "Where to save the metadata backup (default: beside the podcasts directory)"),
			clihelp.String(&opts.RenameUndo, "--undo <backup>", "", "Reverse a rename using its backup directory"),
			clihelp.Bool(&opts.ForceDelete, "-f, --force", false, "Do not ask for confirmation"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress the per-podcast lines"),
		},
		Examples: []clihelp.Example{
			{Line: "pod server identify && pod server rename-episodes --dry-run", Description: "Preview the renames once every episode has an identity"},
			{Line: "pod server rename-episodes --undo /media/podcasts/pod-backup-20260929-101500", Description: "Put every file and status back as it was"},
		},
		Run: func(_ *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "rename-episodes"
			return nil
		},
	}
}

func handleServerRenameEpisodes(cfg Config, cli CLIOptions) error {
	if cli.RenameUndo != "" {
		return undoEpisodeRenames(cli)
	}
	lib := library(cfg, cli, nil)
	root := lib.Config().PodcastsDir
	if root == "" {
		return fmt.Errorf("podcasts_dir is not configured")
	}
	entries, err := pruneScope(lib, cli)
	if err != nil {
		return err
	}
	plans := make([]podcast.RenamePlan, 0, len(entries))
	for _, entry := range entries {
		plans = append(plans, podcast.PlanEpisodeRenames(entry))
	}
	if !reportRenamePlans(cli, plans) || cli.DryRun {
		if cli.DryRun {
			fmt.Fprintln(outFor(cli), "[dry-run] Nothing was renamed.")
		}
		return nil
	}
	backup := renameBackupDir(cli, root)
	if !cli.ForceDelete && !confirmRename(cli, plans, backup) {
		return nil
	}
	return runEpisodeRenames(lib, cli, entries, plans, backup)
}

func renameBackupDir(cli CLIOptions, root string) string {
	if cli.RenameBackup != "" {
		return cli.RenameBackup
	}
	return filepath.Join(filepath.Dir(filepath.Clean(root)), "pod-backup-"+time.Now().Format("20060102-150405"))
}

func runEpisodeRenames(lib *podcast.Library, cli CLIOptions, entries []podcast.PodcastDirEntry, plans []podcast.RenamePlan, backup string) error {
	w := progressFor(cli)
	fmt.Fprintf(w, "Backing up transcripts, cuts, status files and queues to %s ...\n", backup)
	manifest, err := podcast.BackupMetadata(entries, []string{player.GetPlayQueueFilePath()}, backup)
	if err != nil {
		return fmt.Errorf("backup failed, nothing was renamed: %w", err)
	}
	fmt.Fprintf(w, "Backed up and verified %d files (%s).\nRenaming ...\n", manifest.Files, humanBytes(manifest.Bytes))
	res := podcast.ApplyEpisodeRenames(plans, backup)
	_, _ = podcast.RewritePathsIn(player.GetPlayQueueFilePath(), res.Paths)
	republishAfterRename(lib, cli)
	return finishRename(cli, res, backup)
}

// republishAfterRename regenerates each podcast's feed and page, and the catalog,
// so they point at the new file names.
func republishAfterRename(lib *podcast.Library, cli CLIOptions) {
	store, err := lib.Subscriptions()
	if err != nil {
		return
	}
	for _, sub := range store.List() {
		if err := lib.Publish(sub, nil); err != nil {
			fmt.Fprintf(errFor(cli), "republish %s: %v\n", sub.Title, err)
		}
	}
	_ = lib.PublishCatalog(store.List())
}

func finishRename(cli CLIOptions, res podcast.RenameResult, backup string) error {
	w := outFor(cli)
	fmt.Fprintf(w, "\nRenamed %d episode(s), %d file(s).", res.Episodes, res.Files)
	if res.Busy > 0 {
		fmt.Fprintf(w, " %d busy episode(s) were left for a later run.", res.Busy)
	}
	fmt.Fprintf(w, "\nTo reverse this: pod server rename-episodes --undo %s\n", backup)
	for _, f := range res.Failures {
		fmt.Fprintf(errFor(cli), "%v\n", f)
	}
	if len(res.Failures) > 0 {
		return fmt.Errorf("%d episode(s) could not be renamed and were left as they were", len(res.Failures))
	}
	return nil
}

func reportRenamePlans(cli CLIOptions, plans []podcast.RenamePlan) bool {
	w := progressFor(cli)
	var rename, unidentified, already, orphans, skipped int
	for _, p := range plans {
		rename += len(p.Renames)
		unidentified += p.Unidentified
		already += p.Already
		orphans += p.Orphans
		skipped += len(p.Skipped)
		if len(p.Renames) > 0 && cli.Verbose {
			fmt.Fprintf(w, "%s: %d episode(s)\n", listTitle(p.Title), len(p.Renames))
		}
	}
	out := outFor(cli)
	fmt.Fprintf(out, "%d episode(s) to rename. %d already have short names, %d have no identity yet, %d stray file(s) belong to no episode and are left alone, %d skipped.\n",
		rename, already, unidentified, orphans, skipped)
	if unidentified > 0 {
		fmt.Fprintln(out, "Run 'pod server identify' first to include the episodes without an identity.")
	}
	return rename > 0
}

func confirmRename(cli CLIOptions, plans []podcast.RenamePlan, backup string) bool {
	episodes := 0
	for _, p := range plans {
		episodes += len(p.Renames)
	}
	fmt.Fprintf(outFor(cli), "Rename %d episode(s)? Everything except the audio is backed up to %s first, and the rename can be undone. [y/N]: ", episodes, backup)
	line, err := bufio.NewReader(inFor(cli)).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if (err != nil && err != io.EOF) || (answer != "y" && answer != "yes") {
		fmt.Fprintln(outFor(cli), "Aborted. Nothing was renamed.")
		return false
	}
	return true
}

func undoEpisodeRenames(cli CLIOptions) error {
	res, err := podcast.UndoEpisodeRenames(cli.RenameUndo)
	if err != nil {
		return err
	}
	fmt.Fprintf(outFor(cli), "Restored %d file name(s) and %d backed-up file(s) from %s.\n", res.Renamed, res.Restored, cli.RenameUndo)
	return nil
}
