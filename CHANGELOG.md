# Changelog

All notable changes to pod will be documented in this file.

Entries below 0.3.0 predate this file being maintained and are kept as they
were written; they are not in version order.

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