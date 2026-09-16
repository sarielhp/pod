# AGENTS.md — Guidelines for AI-assisted development (Go)

## Build & Quality

- **Go 1.26+** — single `main` package with files organized by concern
- Build: `./tools/build_local` (or `make build`)
- Test: `go test -timeout 30s ./...` (all tests use `t.TempDir()` for isolation)
- Lint: `go vet ./...` then `staticcheck ./...`
- Format: `gofmt -s -w .` before committing

## Automation Tools (`tools/`)

| Script | Purpose |
|--------|---------|
| `tools/build_local` | Build local `./pod` binary strictly within repo directory |
| `tools/check` | Full quality gate: format → tidy → vet → staticcheck → test → build |
| `tools/format.sh` | Run `gofmt -s -w .` only |
| `tools/lint` | Static analysis: `go vet` + `staticcheck` (respecting baseline) + `go-audit` |
| `go-audit` | Cognitive complexity and role-tiered sizing, per `~/prog/standards/go/GUIDELINES.md`; baselined in `tools/go-audit-baseline.txt` |
| `tools/outline_symbols` | Index all Go types, structs, interfaces, and functions |
| `tools/show_symbol <sym>` | Display single symbol code block with line numbers |
| `tools/suggest_split` | Suggest logical file split boundaries for oversized files |
| `tools/generate_config_template` | Generate `examples/config.json.template` |
| `tools/map.sh` | Print package structure, key types, and exported functions |
| `tools/version.sh` | Print current version from `VERSION` file |
| `tools/visual_audit` | Live PTY visual audit: exercises and snapshots all 19 TUI screens/modes |
| `tools/bump` | Bump version, git add/commit/push (skips duplicate gate if .verified_head matches; silent, outputs "Success VERSION (commit+push)") |
| `tools/commit <msg>` | Quality gate + stage + commit + records .verified_head (silent, outputs "Success <msg>") |
| `tools/snapshot [msg]` | Fast WIP commit without gating (<0.1s, never pushes) |
| `tools/checkpoint.sh` | Auto micro-commit of all changes (delegates to `tools/snapshot`) |

## Makefile

A `Makefile` at the project root delegates to all scripts:

| Target | Action |
|--------|--------|
| `make check` | Quality gate for committing: format, vet, staticcheck, line audit, tests, build. No race detector — see `make ci` |
| `make visual` | Run full live PTY visual audit across all 19 TUI screens (`tools/visual_audit`) |
| `make lint` | Static analysis (vet + staticcheck + line audit) |
| `make audit` | Audit complexity and sizing (`go-audit`) |
| `make symbols` | Outline symbols (`tools/outline_symbols ARGS="..."`) |
| `make suggest-split` | Suggest file splits (`tools/suggest_split ARGS="..."`) |
| `make template` | Regenerate config template (`tools/generate_config_template`) |
| `make test` | Run tests |
| `make build` | Build binary |
| `make format` | Format code |
| `make map` | Show architecture overview |
| `make version` | Show current version |
| `make bump` | Bump patch version |
| `make commit` | Quality gate + commit |
| `make push` | Alias for `make bump` |
| `make snapshot` | Fast WIP micro-commit (`tools/snapshot ARGS="..."`) |
| `make snap` | Alias for `make snapshot` |
| `make checkpoint` | Micro-commit all changes (delegates to `make snapshot`) |
| `make ci` | Release gate: everything in `make check` plus the race detector and govulncheck |
| `make clean` | Remove binary |

### Workflow

#### Standard Development & Release Loop
```
make commit ARGS="feat: add new feature"   # quality gate + commits + records .verified_head (silent)
make bump                                   # deduplicates gate via .verified_head, bumps version, commits, pushes (silent)
```

#### Deduplicated Commit-to-Bump Pipeline
`tools/commit` executes `./tools/check`. Upon passing and committing, it records the committed HEAD SHA into `.verified_head`. When `tools/bump` (`make bump`) runs immediately afterward, it checks whether HEAD matches `.verified_head` and only `VERSION` (or nothing) is modified in the working tree. If verified, it skips the redundant full CI quality gate (`./tools/check --full`), saving ~60s of duplicate test runs, increments the patch version, commits, pushes, and removes `.verified_head`. If `.verified_head` is absent, mismatched, or other files are modified, it executes the full quality gate as usual before pushing.

#### Fast Inner-Loop Testing Advice
During active development, use lightweight commands to maximize iteration speed:
- **Instant Snapshots (<0.1s)**: Use `make snap` or `make snapshot [ARGS="..."]` (or `make checkpoint`) freely to checkpoint dirty working trees into fast WIP commits (`--no-verify`, never pushes).
- **Fast Static Analysis**: Run `make lint` for fast feedback (`go vet` + baseline-aware `staticcheck` + line audit). `tools/lint` respects `tools/staticcheck-baseline.txt` so it succeeds when there are 0 new findings rather than failing on pre-existing baselined findings, running in seconds.
- **Targeted Unit Tests**: Run `go test ./...` or `make test` rather than full race/vulnerability scans on every minor change.
- **Full Gate Verification**: Run `make check` before preparing commits to verify formatting, vet, linting, tests, and build.
- **Publishing**: Run `make commit ARGS="..."` followed by `make bump` for a fast, zero-redundancy gated release.

## Version Management

- Version is stored in `VERSION` file (semver: `major.minor.patch`)
- Current version: 0.1.3
- After every successful push, the patch version is automatically bumped by 0.0.1
- Run `make bump` to bump, commit, and push version in one silent step
- The version is not embedded in the Go binary (VERSION file is the source of truth)

## Sizing & Complexity

**The authority is `~/prog/standards/go/GUIDELINES.md`**, with the reasoning in
`RATIONALE.md` beside it. This project follows those standards rather than local
rules; when the two conflict, the standards win and this project's tooling should be
updated to match. `tools/check` enforces them through `go-audit`.

### Cognitive complexity is the metric; line count is a proxy

The primary limits are on how much state a reader must hold, not on length:

- **Nesting depth** — hard limit **4**, warn at 3. Flatten with guard clauses and
  early returns.
- **Branch decision points** — hard limit **15** (`if`, `for`, `switch`, `select`);
  **20** for builders and dispatchers. A `switch` counts once, and flat `case`
  branches that delegate do not add to the count.

### Function length is tiered by role

| Tier | Naming patterns | Soft warn | Hard limit |
|---|---|---|---|
| Standard logic | general logic, handlers, computations | 80 | **110** |
| Declarative builders | `build*`, `init*`, `render*`, `generate*`, `View` | 120 | **160** |
| Event/key dispatchers | `handle*`, `dispatch*`, `*Key`, `*Route` | 150 | **200** |
| Table-driven tests | `Test*` with case slices | 180 | **250** |

A 90-line run of straight-line logic is a soft warning, not a defect, and must not be
split on length alone. Splitting a function that was never hard to read costs real
effort and buys nothing. What does warrant a fix is genuine complexity: depth over 4,
more than 15 branches, an `else` after a terminal statement, or a naked return.

### Files — warn over 800 lines, hard limit 1100

File length is a *comfort* metric, not a correctness one. Keep functions within the
tiers above and files land in the 300–700 range on their own.

### Validation — `go-audit`, baselined

`tools/check` runs `go-audit --quiet .` and compares against
`tools/go-audit-baseline.txt`, the same pattern used for staticcheck. **New findings
fail the gate; baselined ones do not.**

**The baseline is currently empty, and should stay that way.** Every finding it once
carried has been fixed, so any entry appearing in it now is a regression rather than
inherited debt. Never add to it to silence a finding you introduced — fix the code.
The file survives only so that a future wholesale change has somewhere to record
deliberate, explained exceptions.

Install the tooling once per machine:

```bash
ln -sf ~/prog/standards/go/bin/go-audit ~/bin/go-audit
ln -sf ~/prog/standards/go/bin/go-static-analysis ~/bin/go-static-analysis
```

`go-static-analysis` (gocritic, shadow, revive, govulncheck, dupl) is the deep review
pass — run it before releases, not on every commit.

### Decomposition discipline

From GUIDELINES.md §5, and binding when an audit finding must be fixed:

1. **No artificial continuation helpers.** Never extract `processPart2`,
   `handleStepB` or `runRemainder`. Every helper must be one cohesive,
   domain-named responsibility.
2. **No parameter dumping.** Do not extract a helper needing more than 4 parameters,
   or one that takes pointers to locals merely to share state. If the state
   transitions are linear, keep them in place and simplify with guard clauses.
3. **Decompose in place first**, into named helpers in the same file, before moving
   anything into a new file.


### Never split a file through a function body

Splitting a file is a *physical* edit — a byte range is moved to a new file, and
nothing checks that the control flow survived. Two such splits in this repo's history
did real damage and both compiled cleanly and passed the gate:

- `c9b68f4` (code-review-002) dropped every `continue` from the batch loop, so a
  single failing episode silently reprocessed the whole batch.
- `1c49c7b` (code-review-010) replaced `return len(episodesToDownload)` with
  `return 0`, so downloads reported that nothing was fetched.

When a file grows past the warning threshold, **decompose its long functions in place**
and let the file stay long until the decomposition makes a genuine module boundary
obvious. A file that is only long because one function inside it is long is a function
problem, not a file problem. `tools/suggest_split` proposes file boundaries — treat
its output as a hypothesis, and never accept a boundary that falls inside a function.

## Temp File Policy

All temporary/intermediate files **must** be written to a `.work/` subdirectory
alongside the source audio file. The `verifyTempFile()` function enforces this
at runtime — any temp file path outside `.work/` causes an immediate abort.

### Rules

1. **WAV cache file** → `.work/<input>.wav` (single 16kHz mono WAV converted from original)
2. **Truncated previews** → `.work/*.truncated.wav`
3. **FFmpeg cut output** → `.work/*.tmp.mp3`
4. **Final output files** (transcript, SRT, TXT, cuts, ad-free MP3) → original
   directory (these are moved from `.work/` after processing)
5. **Cleanup** — `.work/` is removed after successful processing and on failure

### Adding new temp files

Always use `workDirFor(path)` to compute the `.work/` path, then call
`verifyTempFile(path)` before writing via ffmpeg or any other tool.

## Code Style

- Go 1.26+, no external dependencies beyond stdlib
- No comments in code (keep it self-documenting), except doc comments on exported
  library API — a package boundary has to say what it is for
- **Library packages must not write to the terminal.** `pkg/podcast`, `pkg/backend`
  and `pkg/podsite` take a `progress.Reporter` and let the caller decide where
  output goes; a nil Reporter is silent. `fmt.Print*` and `os.Stdout`/`os.Stderr`
  are banned there and the ban is enforced by
  `TestLibraryPackagesDoNotWriteToTheTerminal` in `pkg/progress`. Writing to an
  `io.Writer` the caller supplied is fine. The processing packages (`pipeline`,
  `adremoval`, `remote`) still print and are not yet on the list
- **In `pkg/cli`, print through `outFor(cli)`, not `fmt.Printf`.** `outFor`
  returns stdout, or `io.Discard` under `--quiet`, so honouring the flag is a
  property of the writer instead of something each call site remembers. A
  forgotten `if !cli.Quiet` is what made `server download --dry-run --quiet`
  print 11 KB. Keep an explicit guard only where the block does real work
  besides printing, or writes to stderr
- `os/exec` for external commands (ffmpeg, ffprobe, docker)
- Custom `syncMu` / `syncMutex` / `syncWG` for thread safety (no sync package)
- All errors are returned; `os.Exit(1)` only in `main()` and fatal helpers
- Functions must be at most 80 lines; files warn over 800 and are capped at 1100 (see **Sizing**)

## File Organization

The codebase is organized into modular Go packages under `pkg/` with a lean entrypoint:

| Directory / Package | Purpose |
|---------------------|---------|
| `main.go` | The single entrypoint: embeds `VERSION` and delegates to `pkg/cli.Execute(os.Args[1:])` |
| `pkg/types` | Core domain types, state enums, configuration data structures, manifests |
| `pkg/util` | Cross-cutting utilities: safe atomic file operations, locks, shell quoting, ANSI colors |
| `pkg/config` | Configuration loading/saving, profile cost estimation, environment overrides, podcast configs |
| `pkg/backend` | Standalone backend interface and legacy import adapters |
| `pkg/audio` | Audio processing via ffmpeg/ffprobe: duration probing, cutting, filtering, ID3 tags |
| `pkg/format` | Formatting routines: time formatters, cut intervals merging, SRT/TXT export |
| `pkg/transcribe` | Whisper API client, audio WAV preparation, chunking, Docker container log progress |
| `pkg/detect` | AI-driven ad detection via LLMs (Ollama, OpenRouter), prompt generation, speculative racing |
| `pkg/gemini` | Direct audio transcription and processing with Gemini Flash 2.5 API |
| `pkg/pipeline` | Core processing pipeline: transcription → detection → cutting, episode status tracking |
| `pkg/player` | Background audio playback daemon, IPC control socket (`/tmp/pod_player.sock`), MPRIS |
| `pkg/podcast` | Standalone podcast manager, subscription store, native downloader; owns publishing (`PublishPodcast`, `PublishCatalog`) |
| `pkg/podsite` | Pure static-site renderer: RSS feed and HTML player bytes. Imports no podcast code — callers pass in resolved data |
| `pkg/podcast` (cont.) | `podcast.Library` (`Open`) is the entry point for anything needing library-wide state; it owns the feed cache and download queue. `podcast.Config` is three fields and must stay that way (`TestConfigStaysThreeFields`) |
| `pkg/podcast` (cont. 2) | `EpisodeFile` is the shared core of an episode on disk; representations embed it rather than restating the six fields. Cache wire format is pinned by `TestCacheWireFormat` |
| `pkg/progress` | `progress.Reporter`: how library packages report progress without choosing where it goes. A nil Reporter is silent |
| `pkg/kitty` | Kitty graphics protocol image rendering and cover art caching |
| `pkg/tui` | Full-featured interactive terminal UI (Bubbletea/Lipgloss) spanning 19 screens and modes |
| `pkg/cli` | Command-line router (`clihelp`), top-level flags, subcommands (`sync`, `queue`, `info`, `config`, etc.) |

## Test Suite

- All packages contain focused, isolated unit and integration tests in `*_test.go` files
- Tests use `t.TempDir()` for strict isolation and never write to real filesystem paths
- Run `go test -timeout 30s ./...` to execute the full test suite across all 17 packages

## External Dependencies

- `ffmpeg` — audio splitting, cutting, truncation
- `docker` — whisper container log polling (optional, local only)
- `whisper.cpp` server — HTTP API for transcription
- `libmp3lame` — MP3 encoding (via ffmpeg)

## Key Architecture & Configuration
 
 - Transcription: HTTP POST to whisper server with `verbose_json` response format
 - Progress: Docker log polling via `docker logs --tail N`
 - Ad detection: LLM API (Ollama, OpenRouter, etc.)
 - Audio cutting: ffmpeg filter_complex with concat
 - Backend Architecture (`architecture.md`):
   - **Standalone (Native)**: `pod` is its own native podcast backend (`backend_type: "standalone"`). It manages subscriptions in `~/.config/pod/podcasts.json`, directly fetches upstream feeds, downloads episodes, and generates local `feed.xml` RSS and `index.html` static web players.
   - **Legacy Importers**: External podcast servers (e.g., PodFetch) have a minimal presence strictly as optional read-only migration sources for one-time subscription import via `pod server import`. All operational features (downloads, feeds, pipeline) run natively in standalone mode.
 - Playback Architecture: Headless background playback daemon spawned via `pod player <play|stop|pause|status> [id]`, controlled through IPC socket `/tmp/pod_player.sock` with MPRIS D-Bus integration (supports mpv and cvlc fallback).
 - Terminology: Standardized on "AdR" (Ad Removal) and "NeedAdR" across CLI, tables, status badges, and TUI.
 - Config: `~/.config/pod/config.json`
 - Migration: Run `pod config migrate` to import settings from legacy `podcasts_manager` configs.
 - Environment Overrides: Supported env vars override config values:
   - `WHISPER_URL`
   - `PODFETCH_URL`
   - `PODFETCH_USER`
   - `PODFETCH_PASS`
   - `PODFETCH_DB_PATH`
   - `PODCASTS_DIR`
   - `WHISPER_LANGUAGE`
   - `WHISPER_DOCKER_CONTAINER`
 - Security & Wake Command: `whisper_wake_command` executes via `/bin/sh -c` under the executing user's privileges. Ensure `~/.config/pod/config.json` permissions remain restricted to the local user.

## Agent Development Rules

1. **Verification**: After modifying any Go file, run `make check` to verify.
2. **Error Resolution**: If `make check` fails, focus on fixing the first reported
   error before making additional changes.
3. **Exploration**: Run `make map` before introducing new types to inspect
   existing structs and interfaces.
4. **Checkpointing & Snapshots**: Run `make snapshot` (or `make snap` / `make checkpoint`)
   to preserve working states in sub-0.1s without gating or pushing.
5. **Surgical Editing**: Never perform full-file rewrites (`replaceAll`) on existing
   files. Read the target section, locate the specific function/struct, and apply
   localized targeted diffs.
6. **Plan Before Build**: For multi-step refactoring or subtle bug fixes, always
   use `plan` mode / consult the `planner` subagent to formulate the exact steps
   before modifying code in `build` mode.
7. **Function Length**: No function over 80 lines — decompose it in place, into
   named helpers in the same file. Files warn at 800 lines and are capped at 1100.
   Never split a file through a function body: that edit is unchecked by the
   compiler and has silently dropped control flow here twice (see **Sizing**).
8. **Commit Messages**: Use conventional commits format:
   `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`
8a. **Changelog**: A change a user could notice — a command or flag removed, an
   exit code or output format changed, a bug they may have been working around —
   goes in `CHANGELOG.md` under the version it ships in. Internal refactoring
   does not. The file went unmaintained through 0.2.x; do not let that happen
   again, because the entries most worth having are exactly the ones that are
   hard to reconstruct later.
9. **Subdirectory Isolation**: Under no circumstances modify files outside this
   subdirectory. Find operations must be restricted to this subdirectory.
10. **Dependency Updates**: Use standard Go tooling to check for updates (`go list -m -u all`
    or `go list -m -u <pkg>`) and upgrade with `go get <pkg>@latest`.
11. **No CLI Aliases**: Avoid defining command or subcommand aliases in CLI apps (`clihelp`). Each command and subcommand must have a single canonical name to maintain clarity, prevent command-space collisions, and keep documentation consistent.
12. **Local Build Isolation**: All local Go builds must be executed using `./tools/build_local` (or `make build`). The build script must generate a local binary (`./pod`) within this repository directory and must NEVER write into or modify directories outside this repository.

## Test Suite

- All tests in `*_test.go` files (single package)
- Use `t.TempDir()` for temp files — never write to real filesystem paths
- Key test categories:
  - Time formatting (`formatTime`, `formatClock`, `formatSRTTime`)
  - Interval merging (`mergeIntervals`, `mergeBounds`, `calculateKeepSegments`)
  - JSON/transcript conversion (`saveCutsJSON`, `convertJSONToSRT`, `convertJSONToTXT`)
  - Config management (`saveConfig`, `loadConfig`, `ensureConfigExists`)
  - Docker helpers (`fetchDockerLogs`, `pollWhisperDockerProgress`)
  - LLM interaction (`detectAdsLLM`, `extractKeywordsLLM`, `extractJSONArray`)
  - Audio processing (`buildWavHeader`, `cutAudioFFmpeg`)
  - Utility functions (`stripExt`, `filepathBase`, `splitLines`, `toLower`)
- Run `go test -v -timeout 30s ./...` to verify all tests