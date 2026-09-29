# Changelog

All notable changes to pod will be documented in this file.

Entries below 0.3.0 predate this file being maintained and are kept as they
were written; they are not in version order.

## [0.5.20] - 2026-09-29

### Changed
- **`pod analyze` with no argument analyses every podcast**, and reports a whole library
  compactly: one line for each podcast analysed (transcripts, phrases, how many are new or
  dropped), a single line saying how many were skipped for having fewer than 10 transcripts,
  and totals. Phrases are listed only with `--verbose`, and a dry run over the library no
  longer prints every phrase of every show. Analysing one podcast is unchanged.

## [0.5.19] - 2026-09-29

### Fixed
- **`pod server identify` and `rename-episodes` also handle episodes whose audio has been pruned.**
  A transcript, cuts file or status file with no audio beside it is an episode like any other: it is
  matched to its feed episode by its old name or given a local identity, and its files are renamed
  together to a short code. A first run had left these under their long old names. The tool is
  incremental, so a second run renames only what the first did not.
- **Lock files no longer pile up.** Every lock pod takes on a file (an episode being processed, a
  status file being updated, a queue) left an empty `.lock` file behind when released, so a library
  slowly filled with them: 1,752 in this one. A lock file is now deleted when its lock is released,
  with a check that keeps two processes from ending up holding "the same" lock on different files,
  and any stale ones (from older versions, or from a process that died) are swept away at the start
  of a run. Lock files that are held are never touched. The `gofrs/flock` dependency is gone.

## [0.5.18] - 2026-09-29

### Added
- **`pod server rename-episodes`** renames existing episode files to the short codes, so nothing
  is left depending on a long, title-based name. All of an episode's files move together
  (audio, uncut original, transcript, cuts, status, fingerprint), its short ID is kept, and the
  processing queue, player queue, status files, cache and published feeds are updated to match.
  Before anything moves, every transcript, cuts file, status file, queue and feed is copied to a
  backup directory beside the podcasts directory and each copy is checked against its original by
  SHA-256; the audio is not copied, since a rename does not touch it. Each rename is written to a
  journal as it happens, an episode that fails part-way is put back as it was, and
  `--undo <backup>` reverses the whole thing. Episodes without a recorded identity (run
  `pod server identify` first) are not touched. Episodes whose audio was pruned, which are just a
  transcript, cuts and status file, are identified and renamed too. `--dry-run` reports the plan. Note that the audio file names in the published feed
  change, so anything subscribed to it sees new file URLs; the episode GUIDs are unchanged.
- **New downloads get short opaque file names**, `2026-09-28_3f9a1c07be.mp3`: the publication
  date, so a directory sorts by age, and a code derived from the episode GUID. The title no
  longer appears in the name, so a long title in any script can no longer make a file name
  too long, and the same episode always gets the same name. What the episode is comes from
  its recorded identity. Existing files keep their names until `pod server rename-episodes`.
- **Each downloaded episode now records which feed episode it is.** The feed GUID, title,
  audio URL, publication date and episode number are stored in the episode's own status
  file when it is downloaded, and `pod fetch` and the other "is this already downloaded"
  checks match on that GUID first. Until now they guessed from the file's name, which broke
  whenever a title was reworded or shortened. Titles shown in lists come from the record too.
  `pod server identify` records it for episodes downloaded earlier: it matches each file to
  its feed episode by its old name, once, and gives any episode the feed no longer lists a
  stable local identity. `--dry-run` reports the counts first.

### Fixed
- **Listings show the recorded episode title, not the file name.** `pod info latest` and `pod repeats`
  now read the title from the episode's record, which matters once file names are short codes.
- **Errors stand out.** An error now starts on a line of its own after a blank line, so it
  can no longer run on from a progress line, and is bold red on a terminal (plain when
  output is piped, and off with `NO_COLOR`). This covers the final `Error:` line, the
  per-episode and per-podcast failures of the batch commands (`transcribe --missing`,
  `analyze`, `server prune`, `server rewind`, `detect`), the errors of ad removal and
  cutting, ffmpeg and ffprobe failures, Whisper server errors, and fatal errors.
  Warnings are unchanged.
- **A transcript or cuts file could not be saved when its name was close to the
  255-byte limit.** The atomic writer named its scratch file `<target name>.tmp.<pid>.<time>`,
  which is 32 or so bytes longer than the target, so a target that fitted still failed
  with "file name too long". Hebrew titles reach that easily, since a letter takes two
  bytes: the transcript of a 50-minute episode was transcribed and then lost. The scratch
  file now has a short name of its own, unique per write.
- **The transcription progress line no longer swallows the next message.** The
  redrawn "Transcribing audio... Elapsed" line was never ended, so the following
  output ("Saved raw Whisper JSON...", or an error) ran on after it on the same line.
  The line is now ended when the reply arrives.
- **A sleeping Whisper server is woken by retrying, not reported as an error.** The
  server is kept asleep behind a Traefik proxy with a Sablier middleware, which
  answers every request with an HTML "waking up" page and status 200 until the
  container is up. pod read that page as a broken transcript ("Whisper server error
  (attempt 1/5): failed to parse transcription JSON: invalid character '<'") and
  spent one of its five attempts on each try. It now recognises the page, says
  "Whisper is waking up", tries again every 3 seconds for up to about a minute and a
  half without using the ordinary attempts, and, should the server never wake, fails
  with what the server actually sent. The mechanism is documented in
  `pkg/transcribe/client.go`.

## [0.5.17] - 2026-09-29

### Added
- **`pod transcribe --missing [podcast|directory...]`** transcribes every downloaded
  episode that has no transcript, newest first; with no argument it covers the whole
  library. It only writes `<episode>.transcript.json` beside the audio: no status
  change, no queue entry, no cut. An episode already cut is transcribed from its
  uncut `.precut` original, so the transcript's timestamps match the original, but
  the transcript is still named after the episode. It skips an episode another pod
  process holds (a rerun picks it up), carries on past a failure, and reports elapsed
  time and roughly how long is left. `--dry-run` lists the episodes and their total
  length, and `-n <count>` limits the run. Rerunning picks up where an interrupted
  run stopped, and the transcripts feed `pod analyze`, `pod repeats` and
  `pod rm_ads recut --boilerplate`.
- **`pod rm_ads recut --boilerplate <podcast|directory|file>`** (also `pod queue recut`)
  refreshes already-processed episodes with their podcast's recorded boilerplate (see
  `pod analyze`), without calling any model. The passages are matched against each
  episode's transcript and added to its existing `.cuts.json`, which is otherwise left
  as it is, and the audio is recut from the uncut `.precut` original. `--dry-run`
  reports what would change and writes nothing. An episode is skipped, with the
  reason, when it has no cuts file or transcript, when the transcript does not match
  the original audio, when it is being processed remotely, when its `.precut` original
  was deleted after something was cut from it, or when the boilerplate would add under
  two seconds. If a recut fails the previous cuts file is put back, and a second run
  over the same episodes does nothing. Unlike `pod rm_ads <podcast>`, it never queues
  an episode for ad removal.

## [0.5.16] - 2026-09-28

### Added
- **`pod analyze --show-bp <podcast>`** prints the boilerplate already recorded in a
  podcast's `podcast.json`, in full and word-wrapped, each phrase under a rule with how
  many episodes carry it, where in the episode it sits, its length in words and seconds,
  and any `disabled` or `manual` flag. It only reads. Rerunning `pod analyze` now also
  records each phrase's measured length; entries recorded earlier show an estimate
  marked `~` until then.

### Fixed
- **`pod analyze` reported phrases as "dropped" on a rerun that changed nothing.** When
  several recorded phrases were variants of one read, each newly found phrase was
  matched to only the first similar entry, so the others were counted as dropped
  (four on Dan Snow's History Hit) although their text was still in the list. Nothing
  was lost from `podcast.json`, but the report was wrong, and a `disabled` flag could
  in principle have been read from the wrong variant. Each recorded phrase now maps to
  the found phrase it most resembles, so an unchanged catalogue gives `0 new, 0 dropped`.

## [0.5.15] - 2026-09-27

### Changed
- **TUI function keys moved one to the right so F1 can be help.** F1 now toggles
  help (as `?` does). The player is F2, the play queue F3, the ad queue F4, the
  episode-view player pane F5 and the download queue F6. The number keys 1-6
  and F12 (snapshot) are unchanged.

### Added
- **`pod server status`** reports what the library holds and what it costs: podcasts
  (and how many are favorites), subscriptions, episodes on disk, how many have ads
  removed or a transcript, disk use split into audio, uncut originals, transcripts
  and cuts, work leftovers and everything else, free space on the filesystem, the
  largest podcasts (`--top <n>`), and how much `pod server prune 5
  --skip-favorites` would free. `--json` gives the same figures.
- **`pod server prune <n>` keeps the newest n episodes of every podcast**, and takes
  `--skip-favorites` to leave favorites alone and `-f/--force` to skip the
  confirmation. It deletes the audio and its uncut original and keeps transcripts,
  cuts and status files, so they can still feed `pod analyze`. It reports the space
  freed, and `--dry-run` lists every file.
- **TUI: F9 toggles a podcast's favorite status, and favorites show a heart (♥)**
  in the podcast list. It works from the podcast list or from inside a podcast,
  and has the same effect as `pod server favorite` (auto-download of new
  episodes and ad removal).
- **`pod analyze <podcast|directory>` learns a show's boilerplate and ad removal
  cuts it first.** It compares the show's saved transcripts and records text
  that recurs in six or more episodes (intros, credits, standing promos and
  sponsor reads) in the podcast's `podcast.json` under `boilerplate`, needing at
  least 10 transcripts. `--dry-run` previews it; a phrase can be switched off
  with `"disabled": true`, and hand-added entries marked `"manual": true`
  survive a rerun. When `pod fetch` or `pod queue run` removes ads, matching
  passages in the new transcript are cut before detection and left out of what
  the model reads, so it can't lump them in with an ad. Held out from its own
  learning, this cut 58% of the labelled ad time on Dan Snow's History Hit at 98%
  precision, with no model call. It applies to the Whisper-and-LLM path; the
  Gemini direct-audio path has no transcript to match.
- **`pod repeats <directory|transcript...>`** finds text that recurs across a show's
  episodes (ads, self-promos, intros, credits, patron-name lists) by matching
  runs of shared words, with no model call: 53 episodes take about a second.
  It reports how much of each episode repeats, `--episode <text>` lists the
  spans with their text, `--catalog` lists the texts that recur most (with
  where in the episode they sit), and `--curve` scores it against labelled
  episodes as the number of compared episodes grows. On 17 labelled History
  Hit episodes, comparing against the other 52 found 81% of the ad time at 89%
  precision; adding five other shows moved that by under two points, because
  the rest are one-off dynamically inserted ads. Repetition also catches
  content that legitimately recurs, so it is a detector to review, not a
  cutter.
- **`pod detect` can now score ad detection against labelled truth.** `--save-truth`
  writes an episode's segments to `<name>.ads.truth.json` (plain JSON, meant to be
  corrected by hand); any later `pod detect` on that episode prints precision,
  recall, seconds of programme wrongly cut and seconds of ads missed, and totals
  across several episodes. It makes profile and prompt changes comparable
  instead of judged by eye. `--model <id>` runs the chosen profile's endpoint with a
  different model, and `--timeout <duration>` allows slow reasoning models longer than
  the default 120s, so candidates can be compared without adding a profile for each.
- **`pod server rewind <window> [podcast]`** undoes recent downloads so `pod fetch`
  can be exercised from a clean state. It deletes every episode whose audio was
  written within the window (`90m`, `24h`, `7d`) together with its transcript,
  SRT/TXT, cuts, `.precut` original, status file, `.work/` leftovers and cached
  details, drops it from the podcast queue, clears the feed's ETag, Last-Modified
  and latest-episode markers so the next check sees the episodes as new, and
  republishes the affected feeds. `--dry-run` lists what would go; otherwise it
  asks first (`-f` skips the prompt). Age is the audio file's mtime, since pod
  records no separate download time.
- **`pod info check player`** lists which of mpv, cvlc, ffplay and mpg123 are
  installed, which one `pod player play` will use, and what is lost without
  mpv (in-place seeking, MPRIS media keys). It fails when none is installed.
- **`pod player play` says when it is not using mpv** ("Playing with cvlc
  (install mpv for in-place seeking and media keys)"), on the progress stream
  so `--quiet` hides it, and fails with the list of players to install when
  none exists instead of timing out on the socket.

### Fixed
- **`pod server prune 5` pruned only podcast number 5.** The count was also taken as
  a podcast selector, so the command touched a single podcast instead of the library,
  and its dry run said "pruned" rather than "would prune". The count no longer selects
  a podcast; `-p <podcast>` does.
- **Ad detection survives replies it used to reject.** A rewind-and-fetch of a
  whole day showed 6 of 23 episodes failing in ad removal. Small models
  sometimes answered in the transcript's own `[95.4s -> 147.1s, "text"]` line
  format instead of JSON, or lumped 48 minutes of a 50-minute episode into one
  "ad", or (on a short news show) labelled every news story an ad. A reply that
  is not valid JSON, or that holds a single segment over 15 minutes, is now
  re-asked up to twice with the required format spelled out; the prompt now says
  that news, interviews and the show's own content are never ads. The cut that
  refused the result (it would have kept 1.6% of the audio) still protects the
  file.
- **`--force llm` now really re-detects ads.** It re-ran detection but merged
  the result into the episode's existing `.cuts.json`, so a bad cut from an
  earlier run (a 342-second "plug" that made the cutter refuse the file) could
  never be removed and every retry failed the same way. A forced detection now
  replaces the saved cuts; without `--force` they still accumulate.
- **Ad-detection errors now show what went wrong.** An unparseable reply quotes
  the text around the error, and a truncated one shows how it ended. The final
  "failed to cut audio with 5 segments" error now carries the reason the cut
  was refused instead of only printing it earlier in the log.
- **Episodes were listed and processed twice when a show is reachable through
  a symlink.** With a symlinked podcasts directory, or the alias symlinks pod
  leaves beside a renamed show, the MP3 walker visited the same directory once
  through the link and once directly, so `pod info latest` showed every such
  episode twice (under two podcast IDs) and `pod rm_ads <dir>` visited each
  file twice. Files are now deduplicated by resolved path, and the real
  directory's path is the one reported. A test that had required the duplicate
  was corrected.
- `pod server prune` and `pod server prune --dry-run` now print a summary line
  when no podcast has anything to prune, instead of nothing at all.
- `pod queue list` showed no publication date for a freshly downloaded episode
  until the podcast cache was next refreshed; it now falls back to the date
  stamped in the episode's status file.

## [0.5.14] - 2026-09-27

### Fixed
- **`pod player play` on a machine without mpv never played anything.** The
  fallback spawns `pod player daemon <file> --title ... --podcast ...`, and the
  daemon subcommand rejected those two flags as unknown, so it exited before
  opening the socket while `play` still printed "Started background playback".
  The daemon now declares the flags, and `play` fails with an error if the
  player has not opened its control socket within three seconds.
- **A detached player's stdout and stderr are appended to
  `~/.cache/pod/player.log`** (honouring `XDG_CACHE_HOME`), so a player that
  dies on startup leaves its reason behind.

## [0.5.13] - 2026-09-27

Fixes from the 2026-09-27 deep review (`issues/review.md`). Findings are cited
by their IDs there.

### Changed (behaviour a user or script will notice)
- **Retention ages episodes from arrival, not the feed's pubDate** (H1). A newly
  downloaded episode whose feed date is older than the keep window is no longer
  deleted by the same `pod sync` that fetched it, so the download/delete loop on
  back-catalogue feeds is gone. An episode that has genuinely sat on disk past the
  window is still pruned.
- **`pod cut` aborts instead of overwriting the original** when the `.precut`
  backup cannot be created, and refuses a symlinked `.precut` (H2).
- **`pod server clean-orphans` exits 1 when any deletion fails** and names the
  failed podcasts on stderr, even under `--quiet` (H6).
- **`pod server download` and `pod fetch` exit 1 when any podcast's download
  failed**, after reporting what did succeed (M13).
- **`pod server flush` now asks for confirmation**, naming the file count; pass
  `-f`/`--force` to skip the prompt in scripts (M12).
- **`pod server keep <N>` locks each episode, skips episodes being transcribed or
  cut remotely, and reports the ones it could not remove** instead of counting
  them as deleted (M2).
- **Feeds, enclosures and covers on private or local addresses are refused**
  (loopback, RFC 1918, link-local, cloud metadata). Set `allow_private_hosts`
  in `config.json` for feeds that really live on the LAN (M7).
- **Episode downloads are capped** at `max_episode_mb` (default 2048), and a
  server that answers with an HTML, XML or JSON document where audio was expected
  is refused; a feed over 32 MiB is an error instead of a truncated parse (H5, L13).
- **`pod config set` and every other config mutator report a failed save** with
  a non-zero exit instead of printing "Updated" over an unwritten file (M3).
- **`pod player stop` kills the process a seek started** rather than the stale
  original, so playback no longer continues after stop (H3). The player's
  control socket is created mode 0600 (M10).
- **`pod info latest`, `pod info <podcast>` and `pod info <episode>` no longer rewrite episode status files** while rendering (L3).
- **`pod cut` and `pod transcribe` take the per-episode lock** and refuse an
  episode another `pod` instance holds (L1).
- **`--quiet` silences Gemini transcription and ad-detection announcements**, and
  batch `--recut` prints its cutting and success lines again (A4, A5).
- **Removed the four command aliases**: `pod ui` (use `pod tui`), `pod queue ls`
  (use `pod queue list`), `pod server feeds update` (use `pod server feeds`),
  `pod info check whisper-server` (use `whisper`). A test now forbids aliases (M14).
- **`pod config migrate` skips legacy post-processor entries that do not resolve
  to an executable**, with a warning, instead of importing them verbatim (L14).
- **`pod detect` reports a path that produced no result as a failure** instead of
  panicking (L7).
- **A `folder` in `podcasts.json` that is absolute or climbs out of the podcasts
  directory is ignored** and replaced by the sanitised title (L15).
- **A failed delete of an uploaded Gemini studio file is reported** instead of
  silently ignored (L6).
- **`make ci` runs the gate once, and a govulncheck finding against a dependency
  now fails it** (T10).

### Fixed (no visible change in normal use)
- The transcript JSON status rewrite and `podcast.json` are written atomically; a
  corrupt `podcast.json` is preserved as `podcast.json.corrupt-<timestamp>`
  rather than silently replaced with defaults (H4, M1).
- The download queue takes its file lock on every write, so the TUI and a CLI
  command over one queue file no longer lose each other's updates; a busy lock is
  an error rather than a silent success; the worker can no longer run twice
  (M4, M5, M8).
- The TUI no longer freezes while a stalled player daemon is polled (M9).
- `pod rm_ads <dir>` no longer deletes a live worker's `.work/` (M6).
- A failed cuts-metadata write aborts the cut instead of marking the episode
  done without it (L8); the temp-file policy is enforced on the no-ads path (M11).
- Lock-busy errors no longer print `%!w(<nil>)` (L2).
- Help text named the old `abs` binary; `config get` listed nonexistent keys;
  README's command table and config example, AGENTS.md and architecture.md were
  corrected against the tree (L9, L10, L11, L12, M15, T5, T11, T12).
- Tests no longer write to the real `~/.config/pod` (every test binary runs under
  `podtest.IsolateMain`); `POD_TEST_REQUIRE_FFMPEG=1` makes ffmpeg-backed tests
  fail rather than skip (T1, T3, T13).

## [0.5.12] - 2026-09-27

### Added
- **Full-Line Podcast View in TUI**: By default, `pod tui` root screen now presents podcasts in a full-line table across the entire terminal width, displaying Icon, ID, Title, Episodes count with ad-free badge, Policy indicators, and Latest episode publication date.
- **Policy Column with Emojis**: Added a dedicated Policy column in the table rendering compact emojis and badges for auto-download policy (`📥New`, `📥All`, `📥TopK`, `📥Off`), retention / auto-cleanup policy (`🗓️30d`, `⭐180d`, `⏱️1d`, `♾️Keep`), and ad removal policy (`✂️All`, `⚡New`, `🚫Off`).
- **Bottom Policy Status Bar**: Detailed status line below the table describing the selected podcast's exact auto-download, retention, and ad removal policies (`Policy: 📥 Download: ... │ 🗓️ Retention: ... │ ✂️ Ad Removal: ...`).
- **AI Batch Summaries & Icon Generator**:
  - Pressing `s` on the root podcasts screen scans all podcasts missing summaries or icons, generates concise 2-3 sentence summaries and 1-character/emoji icons using the configured LLM profile in efficient batches, and permanently caches them in each podcast's `podcast.json`.
  - Displayed in the bottom drawer below the policy status bar for the selected podcast.
  - Supports user-defined or AI-generated emoji icons per podcast (`"icon": "..."`).
- **Sidebar Detail Pane Toggle**: Pressing `Tab` or `i` toggles between the full-width podcast table and the split detail view with cover art.

## [0.5.10] - 2026-09-26

### Fixed
- **TUI Modal Graphics & Overlay Bleed**: Fixed Kitty graphics protocol cover art persisting underneath modals (Help modal `?`, Ad Policy modal `c`, Download Policy modal `d`) by clearing the Kitty graphics GPU layer when modals are open and centering modal overlays across the full terminal window to blank out background screen artifacts.

## [0.5.7] - 2026-09-26

### Added
- **Podcast Keep Policies**: Added configurable retention policies per podcast (`always`, `month` [30 days, default], `favorite` [180 days / 6 months], `hourly` [1 day], or custom `Nd`), auto-pruning expired MP3 audio files while strictly preserving all transcripts (`.transcript.json`, `.transcript.txt`, `.srt`, `.txt`) and metadata.
- **CLI Keep Policy management**:
  - `pod server policy <id> --keep-policy <policy>` to view and configure policies (with `--apply` to prune expired MP3s immediately).
  - `pod server prune [id]` prunes expired audio by keep policy when count argument is omitted.
- **TUI Keep Policy interface**:
  - Configurable in the Podcast Policy modal (`d` or `K`), cycling presets (`always` -> `month` -> `favorite` -> `hourly`) with Space/Tab and adjusting days with `+/-`.
  - Immediate audio prune action (`x`/`X`) in policy modal, podcast list, and podcast detail view, with status toast reporting deleted audio count and freed megabytes.
  - Keep Policy badges and retention labels in podcast lists, detail panes, and status pills.

### Fixed
- **Unconfigured podcast defaults & status reporting** (`issues/001.md`): Introduced `DefaultDiscoveredPodcastConfig` so unconfigured local podcasts on disk default to `ad_removal: "all"` (matching the ad-removal engine) without accidentally enabling `auto_download`, and normalized ad-removal mode comparisons in `pod info status`.

## [0.5.6] - 2026-09-26

### Added
- **OpenRouter Gemini 2.5 Flash Lite default**: Configured `google/gemini-2.5-flash-lite` as the default LLM profile (ID 5) for podcast ad detection, providing sub-2-second latency and minimal token costs.

### Changed
- **Ad detection prompt guidelines**: Enhanced `SystemPrompt` in `pkg/detect` with explicit boundary rules for pre-roll narrative/storytelling hooks and conversational pivots, ensuring cold-open ad anecdotes are captured starting at 0.0s rather than waiting for the sponsor brand name.

## [0.5.5] - 2026-09-21

### Added
- **`pod fetch` command**: Primary daily ingestion and cleaning command that
  checks feeds of active subscriptions, inspects the latest published episode,
  downloads it if not already on disk (never backfilling older archive
  episodes), automatically enqueues newly downloaded episodes into `.queue`,
  and executes ad removal on queued episodes (with `--no-clean` /
  `--download-only` to skip ad removal). Skips hourly news podcasts by default
  unless `--hourly` is specified.
- **`queue` subcommands**: `pod queue recut`, `pod queue export`, and
  `pod queue audit` absorbed into `queue`.

### Changed
- **Top-level command consolidation**: Consolidated visible top-level commands
  down to 7 cohesive commands (`fetch`, `queue`, `server`, `player`, `info`,
  `config`, `tui`). Diagnostic and legacy commands (`detect`, `transcribe`,
  `gen_rss`, `rm_ads`) are hidden from `pod --help` while maintaining 100%
  backward compatibility.

### Fixed
- **`pod info latest` duplicate episodes**: Stripped optional episode tag prefixes
  (`ep<NNN>_`) and added Unicode support in `catalogKey` matching so downloaded
  episodes with episode number tags or non-ASCII titles match their RSS catalog
  entries instead of appearing twice.

## [0.5.4] - 2026-09-19

### Added
- **Generated RSS feeds now include `<lastBuildDate>` and `<pubDate>`.** The
  channel timestamps reflect the most recent episode in the feed, allowing
  podcast clients and aggregators to detect feed updates without re-parsing
  all items.

### Changed
- **Published feeds and catalog pages are only rewritten when content changes.**
  `PublishPodcast` and `PublishCatalog` now check file contents before writing,
  preserving file modification times and avoiding unnecessary disk writes when
  regenerating unchanged feeds or catalog pages.

## [0.5.3] - 2026-09-16

### Added
- **`pod info check --models`** compares the Gemini model chain against the
  models the key can actually reach. The chain is hand-written and rots
  silently in two directions: a model can be withdrawn — Google removed
  gemini-2.5-flash from new users — and a model can appear that would have
  served. `gemini-3.6-flash` was missing from the chain here and turned out to
  be the one model with quota left on the day it mattered. The check names
  retired entries, exits non-zero when it finds one, and lists comparable
  models the chain does not mention, with the caveat that being listed is not
  the same as being callable.

### Changed
- **Ad detection now samples at temperature 0, and the temperature is
  configurable.** It was hardcoded at 0.1, which bought nothing — detection is
  an extraction task with a right answer in the transcript, not a creative one
  — and cost reproducibility. Measured over five runs of one episode, Gemini
  2.5 Flash went from 0.9952 agreement and 4/5 identical runs to exactly
  1.0000 and 5/5, and three separate invocations then returned the same three
  cuts to the tenth of a second. A profile may set `temperature`, and
  `pod detect --temperature` overrides it for one run so the effect can be
  measured rather than argued about.
- **The complexity baseline is empty: the codebase has no outstanding
  findings.** All 21 inherited violations are fixed — seven `else` blocks
  after a terminal statement, one naked return, five functions nested five
  deep, and eight carrying 16 to 22 decision points against a limit of 15.
  Three of the branch-heavy ones shared the same inline logic, so extracting
  it (`maxIndex`, `missingAfter`, `fetchableAfter`, `reverseEpisodes`,
  `trimToCount`) fixed all three at once. Anything the auditor reports from
  here is a regression rather than inherited debt.

### Fixed
- `pod detect --repeat` no longer prints "agreement min 100%" beside "NOT
  reproducible". 0.9952 rounded up at zero decimals, which reads as a bug in
  the measurement rather than as a near miss; figures below certainty are now
  shown to a tenth and never rounded up to 100%.

## [0.5.2] - 2026-09-16

### Added
- **`pod gen_rss <podcast> N`** scopes the fetch-clean-publish run to one show,
  so a single podcast can be brought up to date without touching the rest of
  the library.

### Changed
- A podcast can be named by its short id, its folder, its title or a fragment
  of any of them. `SubscriptionMatches` accepted only the subscription's UUID
  or a title substring, so `tdbwg` — the very id `pod info` prints back —
  matched nothing, and neither did the folder name. Separators are normalised,
  so `daily blast`, `DAILY-BLAST` and `THE_DAILY_BLAST_with_Greg_Sargent` all
  find the same show.
- `pod rm_ads -n/--limit` is documented rather than hidden. Bounding a run is
  what makes ad removal safe to schedule: an unbounded sweep over a library
  with a backlog is many hours of GPU and a real detection bill, and it holds
  the library lock for all of it. The cap was already enforced, and the
  dry-run output already referred to `-n`, but the flag did not appear in
  help.

## [0.5.1] - 2026-09-16

### Fixed
- **Published feeds were not in date order, so clients did not show the newest
  episode.** Ordering compared the RFC1123 publication date as text, and an
  RSS date leads with a weekday and carries a textual month — `Wed, 29 Apr`
  sorts above `Wed, 16 Sep`. Years interleaved, and the day's episode landed
  tenth of a hundred in the Daily Blast feed, where AntennaPod never surfaced
  it. Episodes are now ordered by publication instant. All 84 feeds were
  affected; all 84 are now correctly ordered.

### Added
- `pod info <podcast>` prints the published subscribe URL and web page
  address. Those are what a person wants from the command — the link to paste
  into a podcast client — and it showed the local directory and cover path but
  not either of them.
- **`pod gen_rss N`** — fetch, ad-strip and publish the N most recently
  published episodes across the library in one command. Doing it by hand meant
  `server download`, then `rm_ads` per episode, then `gen_rss`, and forgetting
  the last step left the site describing audio that had changed underneath it.
  Each podcast's download policy is deliberately ignored: the policy governs
  unattended downloading, while this is an explicit request for the newest N
  whatever their shows are configured to do, and favourite status is not
  consulted either. Hourly news bulletins are skipped unless `--hourly` is
  given, since one of them publishes enough episodes to take every slot.
  Episodes already downloaded but never cleaned are included, because that is
  exactly what such a request means to catch.

## [0.5.0] - 2026-09-16

### Added
- **A local feed now carries the show's whole run.** Episodes that were never
  downloaded are published pointing at their original audio, so subscribing to
  a local feed gives at least what subscribing upstream would. Previously a
  feed held one item per downloaded file — a show with nothing downloaded
  published an empty feed — which made the local feed strictly worse than the
  original for any show not fully mirrored. Across this library that took the
  published feeds from 973 items to 6,838: 973 served locally and ad-free,
  5,865 passed through.
- The feed cache retains each episode's enclosure URL, GUID, duration and show
  notes alongside its title and publication time, which is what makes the
  passthrough possible. Notes are stripped of markup and capped at 700
  characters — whole ones average about a thousand and would add some seven
  megabytes — so a passthrough item carries real notes rather than repeating
  its own title. Markup is also stripped when reading, so entries written by
  an earlier version are cleaned rather than keeping their tags forever, and a
  truncated description cannot end mid-tag.

### Changed
- **`pod tui` is now `pod ui`.** With `transcribe` the only other command
  starting with `t`, and `gen_rss` freeing `r`, every command again abbreviates
  to a single unique letter.
- **`pod server feed` is now the top-level `pod gen_rss`.** `feed` and `feeds`
  differed by one letter and did opposite things — one writes the local site,
  the other reads remote feeds — so reaching for the wrong one was easy and
  the mistake was silent. Generating the site is also not an operation on a
  server, which is why it left the `server` group entirely. With no `feed`
  subcommand remaining, `pod server feed` is simply an unambiguous prefix of
  `feeds` and does what it looks like.

### Fixed
- **`pod gen_rss <target>` silently did nothing when the target matched no
  subscription** — no output, exit zero, the stale feed left in place. It now
  reports the mismatch and exits non-zero.
- **Ad removal by file path or directory never republished the feed.** Only
  the episode-id path did, so identical work left the published site correct
  or stale depending on how the argument was typed.
- The catalogue page is regenerated whenever any show is republished, not only
  on a run with no target.
- **The feed catalogue never refreshed.** A feed check carried the retained
  publication history across unchanged and never added the episodes it had
  just read, so the catalogue froze at whenever it was first written — one
  feed's newest entry was nine months stale. Fetched episodes are now merged
  into the history, deduplicated, newest first, capped per feed.

### Changed
- **`pod info latest` lists what has been published, not what is on disk.** It
  read MP3 files and sorted them by file modification time, so it answered
  "what did I fetch recently" and could not mention an episode that was never
  downloaded. With most podcasts configured not to download automatically,
  those were exactly the episodes worth seeing. It now merges the feeds'
  publication history with the library, sorts by publication date, and marks
  each row downloaded or not. `--downloaded` restores the previous behaviour.
- **`pod info latest` hides hourly news bulletins.** A rolling bulletin
  publishes over a hundred episodes a week — one here runs at 169 — so a
  listing of the most recent episodes across the library was mostly that one
  show. `--hourly` includes them. This is the same judgement
  `pod server disable-hourly` already makes about downloading them, and only
  applies to podcasts whose analysed cadence is `hourly`; a podcast with no
  cadence recorded is never hidden on a guess.
- `pod info latest` marks each row with one glyph per fact — 🎧 for downloaded,
  ✓ for advertisements removed — instead of printing a status word on a second
  line. A row that is merely listed carries neither and needs no word to say
  so, and the listing is half as tall.

## [0.4.0] - 2026-09-16

### Added
- **`pod transcribe <path...>`** — transcribe an audio *or video* file without
  ad removal. A video has its audio track extracted automatically; any
  container ffmpeg can read works. Directories are scanned for media.
  `--format json,srt,txt` selects outputs, `-o` redirects them, `-t N` takes
  only the first N minutes, and `--keep-audio` preserves the extracted 16 kHz
  mono audio beside the transcript.
- Transcripts are written beside the input, and nothing about the podcast
  library is touched: no status file, no queue entry, no short ID. A lecture
  recording is not an episode.

- **`pod detect <path...>`** — run ad detection over a transcript you already
  have, without re-transcribing and without cutting anything. Takes a
  transcript or any media file whose transcript sits beside it, touches no
  library state, and reports the segments with reasons. `--profile` picks the
  LLM, `--raw` shows the model's own intervals before merging, `--write-cuts`
  saves a `.cuts.json`, and `--json` emits machine-readable output.
- **`pod detect -n/--repeat <count>`** — detect several times over identical
  input and report how much the runs agree, as shared ad time over claimed ad
  time across every pair of runs. Ad detection is not reproducible: the
  detector sends a non-zero temperature and some providers vary regardless, so
  a single run says what a model answered once rather than what it thinks.
  Measured on one episode, DeepSeek V4 Flash agreed with itself as little as
  51% while Gemini 2.5 Flash reached 98%.
- Ad detection now reports token usage, including how much of the prompt the
  provider served from cache. Every provider returns this on every response
  and pod discarded it, so there was no way to see what detection cost, nor to
  tell a cached call from a recomputed one. The total covers the empty-answer
  confirmation re-asks, which can silently triple the cost of a detection.

- **`pkg/port`** — access to a metered upstream now goes through a port that
  owns retrying, the model chain and the cooldown. Transcription and ad
  detection speak different protocols to the same Gemini endpoint and draw on
  one quota, but each kept its own retry loop and neither could see the
  other: a transcription run would exhaust the per-minute budget and
  detection would then spend three more requests rediscovering it. Only a
  verdict crosses the boundary, never a payload, so the two protocols stay
  separate while their policies cannot drift apart. Ports are keyed by
  endpoint rather than by model family — Gemini via OpenRouter and Gemini
  direct are two meters, measured, not assumed.

### Fixed
- **Ad detection could not survive a rate limit at all.** `pkg/detect` retried
  three times over three seconds against an error that asks for 58 seconds,
  had no model chain, and could not see the circuit breaker. Every protection
  added for transcription now applies to detection too.
- **A per-model quota closed the whole endpoint.** Google meters 20 requests
  per day *per model*, so an exhausted model says nothing about its
  neighbours; closing the port over one idled every model that still had
  quota, including models outside the configured chain.
- **A spent daily quota was read as a per-minute one.** The refusal says
  "please retry in 23s" even when the day's allowance is gone — that is the
  rate bucket refilling, not the day turning — so pod retried a model that
  could not answer again until tomorrow. The structured `quotaId`
  (`GenerateRequestsPerDayPerProjectPerModel`) is now believed over the prose.
- **Each chunk rediscovered the same exhausted model.** Model rests are now
  recorded and persisted, so one chunk's discovery serves the rest; a
  four-model chain had been quadrupling consumption of the quota it exists to
  conserve.
- **The uploaded audio's MIME type was misdeclared.** Chunks are converted to
  WAV but the request hardcoded `audio/mpeg`, and stricter models refuse with
  "MIME type audio/mpeg does not match parent MIME type audio/wav".
- **A model answering in the wrong format aborted the whole chain.** Unusable
  output is a property of the model, not of the request, so the next model is
  now tried. Clock-style timestamps (`"start": 01:39`, which is not valid
  JSON) are also converted to seconds rather than discarded.
- **A transcript produced by the Whisper fallback was labelled `"Gemini"`.**
  When a Gemini request failed, `runWhisperTranscription` fell back to local
  whisper.cpp but returned before adopting the fallback profile, so the
  deferred `StampBackend` still recorded the engine that had been *asked* for.
  The saved transcript therefore claimed a provenance it did not have, which
  is silent and unrecoverable after the fact. A successful Gemini run now also
  records its model, which was previously written as `null`.
- **Gemini could not transcribe anything longer than ~20 minutes.** Chunks were
  fixed at 30 minutes, and Gemini answers a chunk that long with an empty
  candidate and `blockReason: "OTHER"` — the verbatim transcript does not fit
  in one response. With the fallback mislabelling above, this surfaced as a
  local-quality transcript stamped "Gemini" rather than as an error.
- **A one-minute quota blip disabled Gemini for an hour.** Any 429 tripped the
  circuit breaker for a fixed hour, including the free tier's per-minute
  request quota, whose own error says `please retry in 58.8s`. The breaker now
  believes that number when the server supplies one, with a 90-second floor and
  the hour still the ceiling. Losing the only good free transcription backend
  for an hour over a one-minute limit cost far more than the retry it saved.
- Gemini requests that fail with 503/504 are now retried six times with
  exponential backoff (~60s total) rather than three times over six seconds.
  "High demand" spikes are transient, and giving up on one meant a whole
  transcript was silently produced by a much weaker engine.

- **`--whisper-model` was silently ignored when the engine was Gemini.** Gemini
  takes its model from the config rather than the whisper profile, so the flag
  did nothing. It matters because the free tier meters requests per model:
  naming another model is the difference between a transcript and a fallback.

### Changed
- **Quality gate now measures cognitive complexity, not line count.** The
  project followed a local rule of a blanket 80-line hard limit per function;
  it now follows `~/prog/standards/go/GUIDELINES.md`, applied by `go-audit`:
  nesting depth at most 4, at most 15 branch decision points, and function
  length tiered by role (110 for standard logic, 160 for builders, 200 for
  dispatchers, 250 for table-driven tests). A long run of straight-line logic
  is no longer a failure, while depth 5, 17 branches, an `else` after a
  `return` and a naked return now are — 22 such findings existed and are
  baselined in `tools/go-audit-baseline.txt`, so the gate blocks new ones
  without requiring a repo-wide cleanup. Splitting functions that were never
  hard to read cost effort that the real complexity deserved.
- Gemini now tries a chain of models rather than a single one, pinned to
  `gemini-3.8-flash` and falling back through `gemini-3.7-flash` to
  `gemini-3.5-flash` when a model is rate-limited or retired. The default was
  the alias `gemini-flash-latest`, which silently follows Google's newest
  model — and the newest model carries the smallest free-tier allowance, so
  the alias drifted onto whatever was most rate-limited. A model that is out
  of quota is now abandoned immediately rather than waited on, since another
  model with untouched quota answers at once.
- Transcripts record the model the API reports as having answered, rather than
  the one that was requested. An alias never named a real model, and with a
  chain the two routinely differ.
- Gemini audio is now sent in 15-minute chunks instead of 30, configurable as
  `gemini_chunk_sec`. Chunks are transcribed in parallel, so this is not
  slower. `chunk_duration_sec` (a whisper.cpp setting) may still shorten a
  Gemini chunk but can no longer lengthen one past what Gemini can answer.
- `pod t` is now ambiguous between `transcribe` and `tui` and reports both.
  Use `pod tr` or `pod tu`; the full names are unaffected. `pod t` was never a
  documented shortcut, only a side effect of prefix matching.

## [0.3.3] - 2026-09-15

### Changed
- **`pod server download` now exits non-zero when a configured post-processor
  fails.** Previously the exit status of every post-processor was discarded, so
  one that had never worked was indistinguishable from one that did: the
  command printed `Running post-processor: ...` and exited 0 either way. If you
  have `post_processors` configured and one of them has been failing silently,
  scripts that call `pod server download` will start reporting it. That is the
  fix working, but it is a contract change worth knowing about before you
  upgrade.
- A post-processor's own output now goes to the command's output streams rather
  than straight to the process streams. In ordinary use this is the same place.

### Fixed
- Each failing post-processor is named on stderr, and the rest still run.

## [0.3.2] - 2026-09-15

### Changed
- Updated `clihelp` to v0.3.8, which **changes how help is rendered**. The
  top-level command list is compact and no longer repeats each command's
  argument hints, making `pod --help` 41 lines shorter. Nothing is lost: the
  hints still appear in each command's own `--help` usage line, and the
  unknown-command error text is unchanged.

### Fixed
- A data race inside `clihelp`, which saved and restored `fatih/color`'s
  package-level `NoColor` around every render. Fixed upstream in v0.3.8.

## [0.3.1] - 2026-09-15

### Fixed
- Three data races on package-level variables written by one goroutine and read
  by another: the embedded version string, the player's audio-spawn flag, and
  the extra whisper model directories. The last is the most consequential — it
  is a slice, set from configuration and read on every transcription, so a
  reader could observe a half-assigned value.

## [0.3.0] - 2026-09-15

### Removed
- **The remote/offload processing feature, in full.** This removes the `pod
  offload` command and all eight of its subcommands, the `rm_ads collect` and
  `rm_ads clear` subcommands, the `--remote`, `--local`, `--remote-host`,
  `--rffmpeg` and `--no-collect` flags, and the `remote_host`,
  `remote_ffmpeg_host`, `remote_work_dir` and `default_processing`
  configuration keys along with their environment overrides. Ad removal now
  always runs locally.
- Unused keys left in an existing `config.json` are ignored, so no edit is
  required.
- The code is recoverable from the `pre-remote-removal` git tag.

### Fixed
- The TUI wrote to stdout from under its own full-screen display, corrupting it
  whenever a podcast had abandoned duplicate episodes.
- `pod server download --dry-run --quiet` printed about 11 KB of output.
- Episodes queued for download from the Latest Episodes screen carried no
  enclosure URL, so the worker skipped them, reported no error, and marked them
  complete. They were never downloaded.
- `feed.xml` and `index.html` were each written twice on every download and
  every episode deletion.

### Changed
- Episodes left in a remote processing state by a previous version are reported
  by `pod rm_ads --dry-run` as "Stranded in a remote state" rather than being
  silently uncounted.

## [0.1.7] - 2026-08-13

### Added
- Display `Scanning: <Directory>` status output when starting folder scan in default mode and `dir` mode
- Dedicated user temporary directory `/tmp/$USER/abs/` created on demand for temporary log files and fallbacks

## Earlier (pre-0.3.0)

### Added
- Dynamic `read_timeout` based on audio duration (fixes timeout on long files)
- Chunked transcription: split long audio into overlapping 10-minute chunks (`--use-chunks`)
- Parallel chunk transcription support (`parallel_chunks` config)
- Auto-detection of whisper Docker container for progress monitoring
- Real-time progress/ETA from Docker logs during transcription
- Auto-fallback to chunked transcription when whisper fails to decode/encode
- Directory argument support: recursively finds and processes all `.mp3` files
- `--use-chunks` CLI flag to enable chunked transcription
- `whisper_docker_container` config option for manual container specification
- `chunk_duration_sec` and `parallel_chunks` config options
- Error context display from Docker logs (5 lines before/after failure)
- Local IP auto-detection for default config file generation

### Changed
- Default config now uses local machine IP instead of hardcoded address
- Temp files stored in `.work/` directory alongside audio files
- Chunk merge logic uses overlap midpoint for clean deduplication
- Progress thread polls every 2 seconds instead of 0.5
- `Open3.capture3` used for Docker log fetching (fixes stderr leak)

### Fixed
- Whisper server timeout on files longer than ~70 minutes
- Docker log output leaking to terminal (stderr vs stdout issue)
- Redundant retries on whisper decode/encode failures
- `.work/` directory cleanup on all exit paths (success, failure, early return)

## [1.0.0] - 2026-08-07

### Added
- Initial release
- Whisper GPU transcription integration
- LLM-based ad detection
- Audio cutting with ffmpeg
- Multiple output formats (JSON, SRT, TXT)
- Configurable profiles
- Batch processing
- Quiet mode
- Transcript export options
- Recut mode
- Force options (--force-llm, --force-transcribe)
- Interval merging algorithm
- Skip logic for existing files

### Technical Details
- Ruby implementation
- Rainbow gem for colored output
- Open3 for subprocess management
- Net::HTTP for API calls
- FFmpeg for audio processing