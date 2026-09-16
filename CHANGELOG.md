# Changelog

All notable changes to pod will be documented in this file.

Entries below 0.3.0 predate this file being maintained and are kept as they
were written; they are not in version order.

## [Unreleased]

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

### Changed
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