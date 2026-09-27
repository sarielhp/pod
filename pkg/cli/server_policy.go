package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"strconv"
	"strings"

	"github.com/sarielhp/clihelp"
)

type podcastPolicyResult struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Favorite        bool   `json:"favorite"`
	AutoDownload    bool   `json:"auto_download"`
	DownloadPolicy  string `json:"download_policy"`
	DownloadK       int    `json:"download_k"`
	AutoCleanup     bool   `json:"auto_cleanup"`
	AutoCleanupDays int    `json:"auto_cleanup_days"`
	KeepPolicy      string `json:"keep_policy"`
	AdRemoval       string `json:"ad_removal"`
	BackendSync     string `json:"backend_sync"`
}

// policyUpdate translates the policy flags into the library's request type.
func policyUpdate(cli CLIOptions) podcast.PolicyUpdate {
	return podcast.PolicyUpdate{
		Favorite:       cli.FavoriteStr,
		AutoDownload:   cli.AutoDownloadStr,
		DownloadPolicy: cli.DownloadPolicy,
		DownloadK:      cli.DownloadK,
		AutoCleanup:    cli.AutoCleanupStr,
		CleanupDays:    cli.CleanupDays,
		KeepPolicy:     cli.KeepPolicy,
		AdRemoval:      cli.AdRemovalMode,
	}
}

func policyDefaults(cfg Config) config.PolicyDefaults {
	return config.PolicyDefaults{
		DownloadPolicy: cfg.DefaultDownloadPolicy,
		DownloadK:      cfg.DefaultDownloadK,
		AdRemoval:      cfg.DefaultAdRemoval,
		KeepPolicy:     cfg.DefaultKeepPolicy,
	}
}

func backendSyncMessage(s podcast.BackendSync) string {
	switch {
	case s.Backend == "":
		return "Local only (no backend)"
	case s.Err != nil:
		return fmt.Sprintf("Sync error: %v", s.Err)
	default:
		return fmt.Sprintf("Synced to %s", s.Backend)
	}
}

func backendConnectionMessage(name string) string {
	if name == "" {
		return "Backend not connected"
	}
	return fmt.Sprintf("Connected to %s", name)
}

func policyResult(st podcast.PolicyState, sync string) podcastPolicyResult {
	return podcastPolicyResult{
		ID:              st.ID,
		Title:           st.Title,
		Favorite:        st.Favorite,
		AutoDownload:    st.AutoDownload,
		DownloadPolicy:  st.DownloadPolicy,
		DownloadK:       st.DownloadK,
		AutoCleanup:     st.AutoCleanup,
		AutoCleanupDays: st.AutoCleanupDays,
		KeepPolicy:      st.KeepPolicy,
		AdRemoval:       st.AdRemoval,
		BackendSync:     sync,
	}
}

func runPolicyCommand(cfg Config, cli CLIOptions) error {
	podcastsDir := cfg.PodcastsDir
	if podcastsDir == "" {
		podcastsDir = "."
	}

	if len(cli.Args) == 0 && !cli.PolicyAll && !cli.NonFavorites {
		return fmt.Errorf("missing podcast identifier or group for policy command")
	}

	if err := parseShorthandNumberPolicy(&cli); err != nil {
		return err
	}

	target := ""
	if len(cli.Args) > 0 {
		target = cli.Args[0]
	}

	if strings.EqualFold(target, "default") {
		return handleDefaultPolicy(cli)
	}

	if cli.NonFavorites && target == "" {
		target = "not-fav"
	} else if cli.PolicyAll && target == "" {
		target = "all"
	}

	group, err := podcast.ResolvePodcastGroup(podcastsDir, target)
	if err != nil {
		return err
	}

	if group.Kind == podcast.GroupKindSingle {
		return handleSinglePodcastPolicy(cfg, podcastsDir, group.Entries[0].Dir, cli)
	}
	return handlePodcastGroupPolicy(cfg, group, cli)
}

func handleDefaultPolicy(cli CLIOptions) error {
	globalCfg := loadConfig()
	applyDefaultPolicyChanges(&globalCfg, cli)
	if err := config.SaveConfig(&globalCfg); err != nil {
		return fmt.Errorf("failed to save global configuration: %w", err)
	}
	if cli.JSON {
		res := map[string]any{
			"default_download_policy": globalCfg.DefaultDownloadPolicy,
			"default_download_k":      globalCfg.DefaultDownloadK,
			"default_ad_removal":      globalCfg.DefaultAdRemoval,
			"default_keep_policy":     globalCfg.DefaultKeepPolicy,
		}
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(outFor(cli), string(data))
		return nil
	}
	fmt.Fprintf(outFor(cli), "Global default policy updated: download_policy=%s, download_k=%d, ad_removal=%s\n",
		globalCfg.DefaultDownloadPolicy, globalCfg.DefaultDownloadK, globalCfg.DefaultAdRemoval)
	return nil
}

func applyDefaultPolicyChanges(cfg *Config, cli CLIOptions) {
	if cli.AutoDownloadStr != "" {
		if podcast.ParsePolicyBool(cli.AutoDownloadStr) {
			if cfg.DefaultDownloadPolicy == "" || cfg.DefaultDownloadPolicy == config.DownloadPolicyNone {
				cfg.DefaultDownloadPolicy = config.DownloadPolicyLatest
			}
		} else {
			cfg.DefaultDownloadPolicy = config.DownloadPolicyNone
		}
	}
	if cli.DownloadPolicy != "" {
		cfg.DefaultDownloadPolicy = config.NormalizeDownloadPolicy(cli.DownloadPolicy)
	}
	if cli.DownloadK > 0 {
		cfg.DefaultDownloadK = cli.DownloadK
	}
	if cli.KeepPolicy != "" {
		cfg.DefaultKeepPolicy = config.NormalizeKeepPolicy(cli.KeepPolicy)
	}
	if cli.AdRemovalMode != "" {
		cfg.DefaultAdRemoval = config.NormalizeAdRemovalMode(cli.AdRemovalMode)
	}
}

func handlePodcastGroupPolicy(cfg Config, group *podcast.ResolvedPodcastGroup, cli CLIOptions) error {
	if len(group.Entries) == 0 {
		return fmt.Errorf("no matching podcasts found for %s", group.Label)
	}

	hasUpdates := !policyUpdate(cli).IsEmpty()
	if !hasUpdates {
		return displayPodcastGroupPolicy(group, cli)
	}

	return updatePodcastGroupPolicy(cfg, group, cli)
}

func updatePodcastGroupPolicy(cfg Config, group *podcast.ResolvedPodcastGroup, cli CLIOptions) error {
	lib := library(cfg, cli, mustBackend(cfg, cli))
	res, err := lib.SetGroupPolicy(group.Entries, policyUpdate(cli), policyDefaults(cfg))
	if err != nil {
		return err
	}

	defaultMsg := ""
	if cli.SetDefaultPolicy {
		globalCfg := loadConfig()
		applyDefaultPolicyChanges(&globalCfg, cli)
		if err := config.SaveConfig(&globalCfg); err != nil {
			return fmt.Errorf("failed to save global default configuration: %w", err)
		}
		defaultMsg = fmt.Sprintf(" (global default updated: %s)", globalCfg.DefaultDownloadPolicy)
	}

	if cli.JSON {
		data, _ := json.MarshalIndent(map[string]any{
			"updated_count":   res.Updated,
			"auto_download":   res.Applied.IsAutoDownloadEnabled(),
			"download_policy": res.Applied.DownloadPolicy,
			"ad_removal":      res.Applied.AdRemoval,
			"set_default":     cli.SetDefaultPolicy,
			"group":           group.Kind,
			"group_label":     group.Label,
		}, "", "  ")
		fmt.Fprintln(outFor(cli), string(data))
		return nil
	}

	scope := "podcast(s)"
	switch group.Kind {
	case podcast.GroupKindNonFavorites:
		scope = "non-favorite podcast(s)"
	case podcast.GroupKindFavorites:
		scope = "favorite podcast(s)"
	}
	dlBadge := config.DownloadPolicyBadge(res.Applied.DownloadPolicy, res.Applied.DownloadK)
	adBadge := config.AdRemovalModeBadge(res.Applied.AdRemoval)
	fmt.Fprintf(outFor(cli), "Policy updated for %d %s: AutoDownload=%v %s, AdRemoval=%s %s%s\n",
		res.Updated, scope, res.Applied.IsAutoDownloadEnabled(), dlBadge, res.Applied.AdRemoval, adBadge, defaultMsg)
	if cli.PolicyApply {
		var totalPruned int
		var totalFreed int64
		var totalTx int
		for _, e := range group.Entries {
			pr, err := lib.PrunePodcastKeepPolicy(e.Dir, e.Title, cli.DryRun)
			if err == nil {
				totalPruned += pr.DeletedEpisodes
				totalFreed += pr.FreedBytes
				totalTx += pr.PreservedTranscripts
			}
		}
		if cli.DryRun {
			fmt.Fprintf(outFor(cli), "[dry-run] Prune across %d podcasts: would prune %d audio file(s) (%s; transcripts preserved)\n",
				len(group.Entries), totalPruned, formatDiskSize(totalFreed))
		} else {
			fmt.Fprintf(outFor(cli), "✓ Pruned %d audio file(s) across %d podcasts (%s freed; %d transcript(s) preserved)\n",
				totalPruned, len(group.Entries), formatDiskSize(totalFreed), totalTx)
		}
	}
	return nil
}

// mustBackend returns the configured backend, or nil when none is reachable.
// Policy sync is best-effort: a missing backend is reported, not fatal.
func mustBackend(cfg Config, cli CLIOptions) backend.Backend {
	b, err := backend.FromAppConfig(&cfg, reporter(cli))
	if err != nil {
		return nil
	}
	return b
}

func displayPodcastGroupPolicy(group *podcast.ResolvedPodcastGroup, cli CLIOptions) error {
	states := podcast.GroupPolicies(group.Entries)
	results := make([]podcastPolicyResult, 0, len(states))
	for _, st := range states {
		results = append(results, policyResult(st, ""))
	}

	if cli.JSON {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(outFor(cli), string(data))
		return nil
	}

	fmt.Fprintf(outFor(cli), "\nPolicies for %s (%d total):\n", group.Label, len(results))
	fmt.Fprintf(outFor(cli), "%-8s  %-30s  %-15s  %-12s\n", "ID", "TITLE", "AUTO DOWNLOAD", "AD REMOVAL")
	fmt.Fprintln(outFor(cli), strings.Repeat("-", 72))
	for _, r := range results {
		dlBadge := config.DownloadPolicyBadge(r.DownloadPolicy, r.DownloadK)
		title := util.TruncateDisplayName(r.Title, 30)
		fmt.Fprintf(outFor(cli), "%-8s  %s  %-15s  %-12s\n", r.ID, util.PadRight(title, 30), dlBadge, r.AdRemoval)
	}
	fmt.Fprintln(progressFor(cli))
	return nil
}

func handleSinglePodcastPolicy(cfg Config, podcastsDir, target string, cli CLIOptions) error {
	resolved, err := podcast.ResolveAnyID(podcastsDir, target)
	if err != nil {
		return err
	}

	if !resolved.IsPodcast() {
		return fmt.Errorf("identifier %q resolved to an episode, expected a podcast", target)
	}

	pod := resolved.Podcast
	hasUpdates := !policyUpdate(cli).IsEmpty()

	if !hasUpdates {
		return displayPodcastPolicy(cfg, cli, pod)
	}

	if err := updatePodcastPolicy(cfg, cli, pod); err != nil {
		return err
	}

	if cli.SetDefaultPolicy {
		globalCfg := loadConfig()
		applyDefaultPolicyChanges(&globalCfg, cli)
		if err := config.SaveConfig(&globalCfg); err != nil {
			return fmt.Errorf("failed to save global default configuration: %w", err)
		}
		fmt.Fprintf(outFor(cli), "Global default policy updated: default_download_policy=%s\n", globalCfg.DefaultDownloadPolicy)
	}

	return nil
}

func parseShorthandNumberPolicy(cli *CLIOptions) error {
	if len(cli.Args) <= 1 {
		return nil
	}
	k, err := strconv.Atoi(cli.Args[1])
	if err != nil || k <= 0 {
		return fmt.Errorf("invalid episode count %q: must be a positive integer", cli.Args[1])
	}
	if cli.AutoDownloadStr == "" {
		cli.AutoDownloadStr = "true"
	}
	if cli.DownloadPolicy == "" {
		cli.DownloadPolicy = DownloadPolicyLatestK
	}
	if cli.DownloadK <= 0 {
		cli.DownloadK = k
	}
	if cli.AdRemovalMode == "" {
		cli.AdRemovalMode = AdRemovalAll
	}
	return nil
}

func displayPodcastPolicy(cfg Config, cli CLIOptions, pod *ResolvedPodcast) error {
	lib := library(cfg, cli, mustBackend(cfg, cli))
	res := policyResult(
		podcast.PolicyStateOf(pod.ShortID, pod.Title, pod.Config),
		backendConnectionMessage(lib.BackendName()))

	if cli.JSON {
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(outFor(cli), string(data))
		return nil
	}
	printPodcastPolicyDetails(outFor(cli), res)
	return nil
}

func printPodcastPolicyDetails(w io.Writer, res podcastPolicyResult) {
	fmt.Fprintf(w, "\nPolicy for %s [%s]:\n", util.Bold(util.DisplayName(res.Title)), util.BoldCyan(res.ID))
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 65))
	favStr := "No"
	if res.Favorite {
		favStr = util.BoldGreen("⭐ Yes")
	}
	fmt.Fprintf(w, "  Favorite:         %s\n", favStr)
	dlBadge := config.DownloadPolicyBadge(res.DownloadPolicy, res.DownloadK)
	fmt.Fprintf(w, "  Auto Download:    %-5v %s\n", res.AutoDownload, dlBadge)
	retStr := "Disabled"
	if res.AutoCleanupDays > 0 {
		retStr = fmt.Sprintf("%d days retention", res.AutoCleanupDays)
	}
	fmt.Fprintf(w, "  Auto Cleanup:     %-5v (%s)\n", res.AutoCleanup, retStr)
	keepLabel := config.KeepPolicyLabel(res.KeepPolicy, res.AutoCleanupDays)
	fmt.Fprintf(w, "  Keep Policy:      %-5s (%s)\n", res.KeepPolicy, keepLabel)
	adBadge := config.AdRemovalModeBadge(res.AdRemoval)
	fmt.Fprintf(w, "  Ad Removal:       %-8s %s\n", res.AdRemoval, adBadge)
	fmt.Fprintf(w, "  Backend Sync:     %s\n", res.BackendSync)
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("=", 65))
}

func updatePodcastPolicy(cfg Config, cli CLIOptions, pod *ResolvedPodcast) error {
	lib := library(cfg, cli, mustBackend(cfg, cli))
	applied, sync, err := lib.SetPodcastPolicy(pod.Dir, pod.UUID, pod.ShortID, pod.Config, policyUpdate(cli))
	if err != nil {
		return err
	}
	pod.Config = applied
	syncMsg := backendSyncMessage(sync)
	res := policyResult(podcast.PolicyStateOf(pod.ShortID, pod.Title, applied), syncMsg)

	if cli.JSON {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(outFor(cli), string(data))
		return nil
	}

	favBadge := ""
	if applied.Favorite {
		favBadge = " ⭐ [Favorite]"
	}
	fmt.Fprintf(outFor(cli), "Policy updated for %s [%s]%s: DL=%v (%s), Cleanup=%v (%dd), Keep=%s, Ads=%s (%s)\n",
		util.Bold(util.DisplayName(pod.Title)), util.BoldCyan(pod.ShortID), favBadge,
		res.AutoDownload, applied.DownloadPolicy, res.AutoCleanup, applied.AutoCleanupDays,
		applied.EffectiveKeepPolicy(), applied.AdRemoval, syncMsg)

	if cli.PolicyApply {
		return prunePodcastKeepPolicyOnCLI(lib, pod.Dir, pod.Title, cli)
	}
	return nil
}

func prunePodcastKeepPolicyOnCLI(lib *podcast.Library, dir, title string, cli CLIOptions) error {
	pruneRes, pruneErr := lib.PrunePodcastKeepPolicy(dir, title, cli.DryRun)
	if pruneErr != nil {
		return pruneErr
	}
	if cli.DryRun {
		fmt.Fprintf(outFor(cli), "[dry-run] %s: would prune %d expired audio file(s) (%s; transcripts preserved)\n",
			title, pruneRes.DeletedEpisodes, formatDiskSize(pruneRes.FreedBytes))
	} else {
		fmt.Fprintf(outFor(cli), "✓ %s: pruned %d expired audio file(s) (%s freed; %d transcript(s) preserved)\n",
			title, pruneRes.DeletedEpisodes, formatDiskSize(pruneRes.FreedBytes), pruneRes.PreservedTranscripts)
	}
	return nil
}

func buildServerPolicySubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "policy",
		Description: "View or update podcast download and AdR policy",
		UsageLine:   "pod server policy [<podcast-id>|all|default|non-favorites] [<number>] [options]",
		Parameters: []clihelp.Param{
			{Name: "[<podcast-id>|all|default|non-favorites]", Description: "Target podcast identifier, 'all' for all podcasts, 'non-favorites' for non-favorites, or 'default' for global config"},
			{Name: "[<number>]", Description: "Shorthand: auto-download latest K episodes with ad-removal all"},
		},
		Args: clihelp.RangeArgs(0, 2),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.PolicyAll, "--all", false, "Apply policy to all podcasts in library"),
			clihelp.Bool(&opts.NonFavorites, "--non-favorites", false, "Apply policy or filter only to podcasts that are not marked as favorite"),
			clihelp.Bool(&opts.SetDefaultPolicy, "--set-default", false, "Also update global default configuration for new podcasts"),
			clihelp.String(&opts.FavoriteStr, "--favorite <bool>", "", "Set as favorite (auto-downloads all new episodes and removes ads)"),
			clihelp.String(&opts.AutoDownloadStr, "--auto-download <bool>", "", "Enable automatic downloads (true/false)"),
			clihelp.String(&opts.DownloadPolicy, "--download-policy <mode>", "", "Policy mode ('none', 'latest', 'latest_k', 'all')"),
			clihelp.Int(&opts.DownloadK, "--download-k <num>", 0, "Number of latest episodes to download"),
			clihelp.String(&opts.AutoCleanupStr, "--auto-cleanup <bool>", "", "Enable automatic cleanup (true/false)"),
			clihelp.Int(&opts.CleanupDays, "--cleanup-days <days>", 0, "Retention window in days"),
			clihelp.String(&opts.KeepPolicy, "--keep-policy <policy>", "", "Keep policy: 'always' (forever), 'month' (30d regular), 'favorite' (180d 6mo), 'hourly' (1d news), or days (e.g. '14d')"),
			clihelp.Bool(&opts.PolicyApply, "--apply", false, "Immediately prune expired MP3 files per keep policy (preserves transcripts)"),
			clihelp.String(&opts.AdRemovalMode, "--ad-removal <mode>", "", "Ad removal policy mode ('none', 'latest', 'all')"),
			clihelp.Bool(&opts.JSON, "--json", false, "Output results in JSON format"),
		},
		Examples: []clihelp.Example{
			{
				Line:        "pod server policy 42 1",
				Description: "Shorthand: auto-download latest 1 episode and remove all ads",
			},
			{
				Line:        "pod server policy non-favorites --download-policy none",
				Description: "Disable auto-download for all podcasts that are not marked as favorite",
			},
			{
				Line:        "pod server policy all --auto-download false",
				Description: "Mark all podcasts as not auto-download",
			},
			{
				Line:        "pod server policy all --auto-download false --set-default",
				Description: "Disable auto-download for all podcasts and set global default",
			},
			{
				Line:        "pod server policy default --download-policy none",
				Description: "Set default download policy for new podcasts to none",
			},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "policy"
			opts.SyncSubcmd = "policy"
			opts.Args = ctx.Args
			return nil
		},
	}
}
