# Changelog

All notable changes to pod will be documented in this file.

Entries below 0.3.0 predate this file being maintained and are kept as they
were written; they are not in version order.

## [Unreleased]

### Fixed
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

## [Unreleased]

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