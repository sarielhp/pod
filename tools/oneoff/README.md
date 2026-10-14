# `tools/oneoff/` — archived one-off scripts

Dead code, kept only until its owner decides to delete it. **Do not build on
these or extend them.** Nothing in `Makefile`, `tools/check`, `tools/lint`, or
`AGENTS.md` invokes any of them, and most read files that no longer exist in
their original location (the pre-`pkg/` layout: a single `main.go`, `tui.go`,
`cli_parse.go`, `player.go`, `pm_download.go`, `transcribe.go`, `profiles.go`).

This file exists as a deliberate exception to the "no `README.md` inside
`tools/`" convention; see the global `AGENTS.md`. It is an index for a
graveyard, not documentation of supported tooling.

## `migration/` — one-shot data and naming migrations

| Script | What it did |
|---|---|
| `finalize_podcasts_cleanup.rb` | Repointed the Caddy compose mount from `/media/podcasts/abs` to `/media/podcasts/clean`, restarted Caddy, then removed the legacy `abs` directory and `podfetch` symlink. Guarded by a "clean dir has >10 items" safety check. |
| `merge_raw_podcasts.rb` | Copied missing `podcast.mp3` episodes (and HTML reports) from `/media/podcasts/raw` into `/media/podcasts/clean`, deleted `raw`, then regenerated feeds via the old `abs` binary. |
| `rename_to_pod.rb` | Renamed the Go module `abs` → `pod`: the `go.mod` module line, every `"abs/pkg/..."` import, and created `cmd/pod/main.go`. |
| `repair_episode_dates.rb` | Fetched each subscription's feed, corrected episode publication dates, renamed MP3s to a date prefix, and retired duplicate files. Dry-run by default; `-e`/`--execute` to apply. |
| `standardize_podcast_names.rb` | Consolidated duplicate show directories and episode files, removed stale lock files, and updated `podcasts.json` folder names. Dry-run by default; `-e`/`--execute` to apply. |
| `update_cli_names.rb` | Rewrote user-facing CLI strings `abs` → `pod` across `pkg/cli/*.go`, including usage lines and tests. |
| `update_docs.rb` | Rewrote `abs` → `pod` across `README.md`, `antenna_pod.md`, `architecture.md`, and `AGENTS.md`. |

## `refactor/` — Go restructuring aids

Written for the era when the project was a single `main` package. Several use
hard-coded line ranges, so they are inherently one-shot and only correct against
the exact revision they were written for.

| Script | What it did |
|---|---|
| `extract_tui_types` | Moved lines 27–117 of `tui.go` into a new `tui_types.go`. |
| `extract_tui_types_more` | Intended follow-up for `tui_types.go`, but the body is a no-op (empty `if`). |
| `extract_tui_views` | Moved three line ranges of `tui.go` into a new `tui_views.go`. |
| `fix_cli` | Removed the `opml`, `scan`, `new`, and `clean-orphans` command blocks from `cli_parse.go` and rewrote `normalizeCLIArgs`. |
| `kitty_fix_hebrew` | Set `force_ltr yes` in `~/.config/kitty/kitty.conf` and checked Hebrew font coverage. The only one that touches host config rather than the repo. |
| `refactor_os_exit` | Rewrote the profile helpers in `profiles.go` to return `error` instead of calling `os.Exit(1)`. |
| `revert_main` | Reversed the profile-helper error-handling rewrite in `main.go` (undoes `update_main`). |
| `revert_signatures` | Reversed the `error` return signatures of the profile helpers in `profiles.go` (undoes `refactor_os_exit`). |
| `ruby_split` | Generic mover: `ruby_split <src> <dst> <regex>` moves regex-matched blocks from `src` to `dst`. |
| `safe_fatal` | Converted `printError(...)` / `fmt.Fprintf(os.Stderr, ...)` / `fmt.Println(...)` followed by `os.Exit(1)` into `fatalError(...)` across all non-`main` Go files. |
| `split_player` | Moved lines 293–420 of `player.go` into `player_sink.go`. |
| `split_pm` | Moved lines 63–440 of `pm_download.go` into `pm_download_episodes.go`. |
| `split_transcribe` | Moved lines 170–394 of `transcribe.go` into `transcribe_chunks.go`. |
| `split_tui_keys` | Moved lines 294–538 of `tui_keys_test.go` into `tui_keys_extra_test.go`. |
| `split_tui_test` | Moved lines 267–540 of `tui_test.go` into `tui_nav_test.go`. |
| `strip_comments` | Removed full-line `//` comments (except `//go:`) and `/* */` block comments from all Go files. |
| `update_main` | Wrapped the profile-helper calls in `main.go` with error checks and `os.Exit(1)` (undone by `revert_main`). |
| `use_fatal` | Converted `fmt.Fprintf(os.Stderr, ...)` / `fmt.Println` / `fmt.Printf` followed by `os.Exit(1)` into `fatalError(...)`. |
| `use_fatal2` | Converted `printError(...)` followed by `os.Exit(1)` into `fatalError(...)`. |

## `review/` — the archived multi-lens review pair

| Script | What it did |
|---|---|
| `audit` | Multi-lens LLM code auditor. Picks a profile (`systems`, `security`, `correctness`, `resilience`, `performance`, `cli`, `random`), selects a backend and model tier, and runs the audit. |
| `review_cycle` | Autonomous review-cycle orchestrator: runs `audit`, dispatches remediation, runs the quality gate, verifies the remediation diff, and can loop across profiles. |

The live review entry point is `tools/review_loop`, which delegates to the
external `agy_review_code_loop` binary.
