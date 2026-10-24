# pod

Automatically detects and removes advertisement, sponsor, and promotional segments from podcast MP3 files using local Whisper transcription and configurable LLM detection.

## Features

- **Local Whisper Transcription**: Uses local Whisper GPU server for fast, offline transcription
- **Configurable LLM Detection**: Supports multiple LLM providers (local Ollama, OpenRouter, etc.)
- **Smart Ad Detection**: Detects host-read sponsor plugs, midroll/preroll ads, promotional breaks
- **Batch Processing**: Process multiple files in sequence, or pass a directory to process all MP3s
- **SRT/TXT Export**: Export transcripts in multiple formats
- **Recut Mode**: Re-cut audio using existing metadata without re-transcribing
- **Chunked Transcription**: Split long audio into overlapping chunks for reliability (`--use-chunks`)
- **Docker Progress Monitoring**: Auto-detects local whisper Docker container and shows real progress/ETA
- **Auto-fallback**: Detects whisper decode failures and automatically retries with chunked transcription
- **Optional music removal**: Runs YAMNet alongside Whisper, recovers missing speech in transcript gaps, and cuts confidently music-only sections
- **Interactive TUI**: Browse podcasts, queue episodes, and track processing status
- **Feed & Web Generation**: Generate RSS feeds and static HTML5 web players natively
- **Podcast Search**: Find shows by name and subscribe in one step (`pod search`)

## Workflow

```
1. Transcribe audio → Whisper GPU server (HTTP API)
2. Detect ads → LLM (local or remote)
3. Merge intervals → Combine adjacent ad segments
4. Cut audio → ffmpeg
5. Save metadata → .cuts.json
```

## Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/pod.git
cd pod

# Build the Go binary
./tools/build # builds ./pod inside the repository

# Or use the Makefile
make build
```

`rtk proxy tools/install` builds and installs `~/bin/pod`, verifies that its
contents match the local build, and then removes the legacy `~/bin/abs` binary.

For a release, use `rtk proxy tools/release "fix: description"` to run the
quality gate, commit, bump the patch version, push to the current branch's
configured upstream, and install the released build to `~/bin/pod`. The command
verifies the remote commit and stops on any failure. If a push fails,
the version commit remains local; resolve the failure before retrying the push
so another version bump is not created accidentally.

## Configuration

The program creates a config file at `~/.config/pod/config.json` (with fallback to `~/.config/abs/config.json`) on first run, using the local machine's IP address automatically.

```json
{
  "whisper_url": "http://<local_ip>:8088/inference",
  "whisper_speed_factor": 7.0,
  "whisper_docker_container": "",
  "chunk_duration_sec": 0,
  "parallel_chunks": 1,
  "active_profile_id": 1,
  "whisper_language": "",
  "whisper_prompt": "",
  "profiles": [
    {
      "id": 1,
      "name": "Ollama Local (llama3.1:8b)",
      "type": "ollama",
      "url": "http://<local_ip>:11434/v1/chat/completions",
      "model": "llama3.1:8b",
      "api_key": ""
    }
  ],
  "backend_type": "standalone",
  "podcasts_dir": "/media/podcasts/clean/",
  "server_base_url": "http://100.123.184.3:8080/podcasts"
}
```

### Config Options

| Option | Default | Description |
|--------|---------|-------------|
| `whisper_url` | Auto-detected | Whisper server HTTP endpoint |
| `yamnet_url` | "" (disabled) | YAMNet service base URL; enables parallel classification, transcript gap recovery, and music-only cuts during ad removal |
| `whisper_speed_factor` | 7.0 | Estimated transcription speed multiplier (for ETA display) |
| `whisper_docker_container` | Auto-detected | Docker container name for progress polling |
| `chunk_duration_sec` | 0 (disabled) | Split audio into chunks of this duration (seconds) |
| `parallel_chunks` | 1 | Number of chunks to transcribe in parallel |
| `podcasts_dir` | "" | Default directory for podcast storage and processing |
| `server_base_url` | "" | Base URL for static HTTP serving of feeds and web pages |
| `gemini_api_key_file` | "" | Path to file containing Gemini API key |
| `gemini_chunk_sec` | 900 | Audio per Gemini request; above ~1200 Gemini returns an empty response |
| `backend_type` | "standalone" | Backend provider: `"standalone"` (native, default) |

### Music detection and transcript gap recovery

Enable the local classifier with `pod config set yamnet-url http://tqed:8090`,
or use `YAMNET_URL=http://tqed:8090 ./pod fetch` for one run. An empty
`yamnet_url` disables the feature. The service must provide JSON `/health`
and a multipart `/inference` endpoint with `include_frames=true` support.

During ad removal, pod starts full-original-audio YAMNet classification alongside
Whisper and wakes both services together, including through Sablier. This mode uses
Whisper first rather than Gemini's direct audio cuts or speculative racing. YAMNet
results are cached in `<episode>.yamnet.json`, keyed by endpoint, source size, and
modification time. A completed transcript review is also reused when its source and
transcript still match.

Pod examines transcript gaps of at least two seconds, using word timestamps when
available. Non-silent gaps get short Whisper retries, including gaps labeled as
music. Recovered speech is merged into the original timestamps before boilerplate
and ad detection. Retries use 30-second clips with context and are capped at 15
minutes of gap audio and 96 requests per episode. Failed retries and limits leave
the unfilled gaps in the audio while processing continues. Recovery progress and
preserved gaps are recorded in the transcript.

Remaining music is cut throughout the episode when every overlapping YAMNet
window has music confidence at least 0.8 and speech confidence at most 0.05,
the gap was retried successfully, and the cut stays 0.75 seconds away from
transcribed speech. Silence and ambiguous audio are kept. Residual speech-like gaps
are preserved, including when old or new ad cuts span them. YAMNet and an empty Whisper result cannot guarantee that speech is
absent under loud music; these conservative checks reduce that risk without an
independent speech detector.

Finished episodes remain skipped by normal fetching. To apply music review to one,
enable the endpoint and use the existing recut mode. It preserves existing ad cuts
and adds music cuts from the original `.precut` audio. Recut does not run ad detection
again; use forced LLM detection when newly recovered speech also needs ad analysis.
One-off `pod transcribe` remains a transcript-only command.

## Standalone Backend Architecture

`pod` is a self-contained, native podcast management system:
- **Subscriptions**: Stored locally in `~/.config/pod/podcasts.json` (or `~/.config/abs/podcasts.json`).
- **Feed Checking**: `pod library feeds` checks upstream feeds directly with conditional HTTP GET (`ETag` / `If-Modified-Since`).
- **Downloads**: `pod library download` downloads episodes directly over HTTP with resume support.
- **Feeds & Web Players**: `pod gen_rss` generates standard Apple Podcasts `feed.xml` RSS and HTML5 `index.html` static web players for every show and the full catalog. Episodes that were never downloaded are published pointing at their original audio, so a local feed carries the show's whole run.
- **Archive gaps**: Older versions of the cold-storage archive deleted an episode's mp3 instead of moving it, so some old episodes have only their sidecar files left (`.transcript.json`, `.cuts.json`, `.mp3.json`). This is expected: those episodes are no longer published, and current archiving moves the audio to the HDD archive instead.
- **Mobile Sync**: Works with standard podcast clients (e.g. AntennaPod) over Tailscale/Caddy without proprietary server apps.

### Legacy Server Import (Optional)

External servers are deactivated from normal operations and only serve as optional migration sources for one-time subscription import:
- Import from OPML: `pod library import subscriptions.opml`
- Import from PodFetch: `pod library import` (reads configured `podfetch_db_path` or API)

### Search & Subscribe

`pod search <query>` lists matching shows and `pod search add <id-or-feed-url>` subscribes to one:

```bash
pod search huberman             # find a show by name
pod search add 75075            # subscribe to the result with that ID
pod search add https://example.com/feed.xml   # subscribe straight to a feed URL
```

Search uses **Podcast Index** when credentials are configured and otherwise falls back to Apple's
keyless search, so it works without any setup. To use Podcast Index, put your key and secret in
`~/.config/auth/podcast_index_key.json`:

```json
{ "api_key": "YOUR_KEY", "api_secret": "YOUR_SECRET" }
```

The path can be changed with `pod config set podcastindex-api-key-file <path>`; the credentials
themselves are never written to `config.json`. A single run can be pointed at another account with
`PODCASTINDEX_API_KEY` and `PODCASTINDEX_API_SECRET`.

Subcommands resolve by prefix, so a query that is itself a prefix of `add` — `a` or `ad` — is taken
as the subcommand. Spell the word out, or quote a longer phrase, when that happens.

## Ad Removal Terminology (AdR)

The codebase and CLI standardize on concise **AdR** (Ad Removal) terminology:
- **`✂ NeedAdR`**: Episode has not yet undergone ad removal.
- **`✓ Ad-Free`**: Commercial segments have been detected and cut.
- **`AdR Policy`**: Per-podcast ad removal setting (`none`, `latest`, `all`).
- **`AdR Queue`**: Priority queue of episodes pending commercial excision.

## Background Audio Player (`pod player`)

Headless audio playback daemon with MPRIS D-Bus integration:
```bash
pod player play [id]     # Play episode by ID (eXXXXX) or resume playback
pod player pause         # Toggle playback pause state
pod player stop          # Stop playback and shutdown daemon
pod player status        # Display current track, position, duration, and status
```
Spawns a detached background player (prefers headless `mpv` with `--input-ipc-server=/tmp/pod_player.sock`, with graceful fallback to `cvlc --control dbus` or built-in players). Player state reflects in real time inside the interactive TUI.

## Usage

### Basic Usage

```bash
# Process audio files or directories for ad removal
pod rm_ads episode.mp3
pod rm_ads /path/to/podcasts/
pod rm_ads recut episode.mp3
pod rm_ads export srt episode.transcript.json

# Library query and inspection (absorbs ls, transcript, and status diagnostics)
pod info                    # List all podcasts in library
pod info latest 10          # List latest 10 episodes across library
pod info p0001              # Display podcast metadata and episode list
pod info e12345             # Display episode cuts and metadata
pod info e12345 --cuts      # Show detailed cuts breakdown
pod info e12345 --transcript # Display transcript text
pod info transcript e12345  # Read transcript in $PAGER or the system pager
pod info e12345 --export srt # Export transcript to SRT
pod info status             # Show library summary and worker status
pod info check              # Test external services (Whisper, Gemini, Kitty)

# Feed sync & library operations (all under `library`)
pod library feeds            # Check upstream RSS feeds for new episodes (reads remote)
pod gen_rss          # Generate local feed.xml + index.html (writes local)
pod gen_rss 10       # Fetch, ad-strip and publish the 10 newest episodes
pod library download         # Download pending episodes according to podcast policy
pod library download p0001 -k 3 # Download up to 3 missing episodes for a podcast
pod library flush p0001 --dry-run # Preview removing this podcast's audio, keeping transcripts
pod library flush p0001       # Remove MP3/precut audio and disable automatic downloads
pod library publication-sync --dry-run # Preview correcting local publication metadata from the source catalog
pod library publication-sync # Correct cached/status dates; unknown source dates stay unknown
pod library prune            # Prune old episodes per retention policy
pod library policy p0001     # View or update download/AdR policy
pod library timeline         # Display online availability timestamps table

# Manage AdR queue
pod queue list
pod queue add e12345
pod queue today             # Queue downloaded, uncleaned episodes published today (local date)
pod queue priority tdbwg 8   # Persist podcast priority (0–10; default 0)
pod queue priority tdbwg     # Show the podcast's priority
pod rm_ads e79636            # Queue this episode at priority 10 and process it immediately
pod queue remove e12345
pod queue clear

# Background playback
pod player play e12345
pod player pause
pod player status
pod player stop


# Configuration
pod config show
pod config get <key>
pod config set <key> <value>

# Ad detection on a transcript you already have
pod detect episode.transcript.json      # Report ad segments; touches nothing
pod detect --profile 3 episode.mp3      # Use a specific LLM profile
pod detect -n 5 --json episode.mp3      # Detect 5 times and report how much the runs agree
pod detect --raw episode.mp3            # Segments as the model returned them, unmerged
pod detect --write-cuts episode.mp3     # Also save a .cuts.json

# Interactive TUI browser
pod tui
```

### Commands Overview

Commands resolve by unique prefix, and every command has exactly one name (no aliases). The eight visible commands each start with a different letter, so each abbreviates to that letter (`c`, `f`, `i`, `l`, `p`, `q`, `s`, `t`).

| Command | Prefix | Usage | Description |
|---------|--------|-------|-------------|
| `config` | `c` | `pod config [command]` | View and manage application configuration |
| `fetch` | `f` | `pod fetch [podcast-id] [options]` | Fetch latest podcast episodes and remove ad segments |
| `info` | `i` | `pod info [options] [id\|latest [N]\|status\|check]` | Library query, inspection, cuts and transcripts |
| `library` | `l` | `pod library [command] [options] [podcast-id]` | Manage the podcast library: feeds, downloads, and policies |
| `player` | `p` | `pod player [command]` | Control background audio playback (`play`, `stop`, `pause`, `status`) |
| `queue` | `q` | `pod queue [command]` | Manage the ad removal (AdR) processing queue |
| `search` | `s` | `pod search <query> [options]` | Search podcast directories and subscribe to a result |
| `tui` | `t` | `pod tui [options] [directory]` | Interactive TUI browser for podcasts and episodes |

The processing commands below are hidden: they do not appear in `pod --help` and are excluded from prefix matching, so they must be spelled out in full (`pod d` and `pod r` do not resolve).

| Command | Usage | Description |
|---------|-------|-------------|
| `cut` | `pod cut [options] <episode>` | Cut advertisements from audio using cuts metadata |
| `detect` | `pod detect [options] <path...>` | Detect ad segments in an existing transcript |
| `gen_rss` | `pod gen_rss [podcast] [N]` | Generate the RSS feed and web pages for local podcasts |
| `rm_ads` | `pod rm_ads [command]` | Process audio files for ad removal (`recut`, `export`, `audit`) |
| `transcribe` | `pod transcribe [options] <path...>` | Transcribe an audio or video file |

### Chunked Transcription

For long files that whisper fails to decode, use chunked transcription:

```bash
# Enable chunking (10-minute chunks with 30s overlap)
pod rm_ads --use-chunks episode.mp3

# Or enable permanently in config:
# "chunk_duration_sec": 600
```

The script also auto-detects whisper decode failures and falls back to chunking automatically.

### Export Options

```bash
# Export transcript to SRT or plain text format
pod rm_ads export srt episode.transcript.json
pod rm_ads export txt episode.transcript.json

# Or export via info command
pod info e12345 --export srt
pod info e12345 --export txt
```

### Recut Mode

```bash
# Re-cut using existing .cuts.json metadata
pod rm_ads recut episode.mp3
```

### Force Options

```bash
# Force re-transcribe
pod rm_ads -f whisper episode.mp3

# Force re-run LLM detection
pod rm_ads -f llm episode.mp3

# Force all pipeline stages
pod rm_ads -f all episode.mp3
```

### LLM Profiles

```bash
# List available profiles
pod config llm list
pod config llm test 3       # Test profile 3 with a sample ad-detection request

# Use specific profile
pod rm_ads --profile 2 episode.mp3

# Set default profile
pod config llm default 2
```

### External Service Diagnostics

```bash
# Test connection to Whisper server
pod info check whisper
```

## Output Files

- `episode.mp3` → `episode_adfree.mp3` (original preserved as `episode.mp3.precut`)
- `episode.transcript.json` (Whisper's raw output)
- `episode.transcript.txt` (human-readable transcript)
- `episode.srt` (subtitle file)
- `episode.cuts.json` (ad cut metadata)
- `.work/` (temporary chunk files, auto-cleaned)

## Architecture

### Whisper Integration
- Local Whisper GPU server for transcription via HTTP API
- Docker log polling for real-time progress and error detection
- Auto-detects local whisper container by image name or exposed port
- Falls back to chunked transcription on decode failures

### LLM Integration
- Supports multiple providers:
  - Local Ollama
  - OpenRouter (Claude, DeepSeek, Gemini, etc.)
  - Any OpenAI-compatible API
- Configurable profiles with pricing info

### Ad Detection Logic
- Analyzes transcript for ad patterns
- Merges adjacent intervals with small gaps
- Configurable gap tolerance based on segment duration

## Requirements

- Go 1.26+
- FFmpeg
- Whisper GPU server (local or remote)
- LLM API access (local or remote)
- Docker (optional, for progress monitoring)

## License

MIT
