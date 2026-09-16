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
- **Interactive TUI**: Browse podcasts, queue episodes, and track processing status
- **Feed & Web Generation**: Generate RSS feeds and static HTML5 web players natively

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
git clone https://github.com/yourusername/abs.git
cd abs

# Build the Go binary
go build -o pod . # (or ./tools/build_local to build ./pod and ./abs symlink)

# Or use the Makefile
make build
```

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
  "podcasts_dir": "",
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
| `whisper_speed_factor` | 7.0 | Estimated transcription speed multiplier (for ETA display) |
| `whisper_docker_container` | Auto-detected | Docker container name for progress polling |
| `chunk_duration_sec` | 0 (disabled) | Split audio into chunks of this duration (seconds) |
| `parallel_chunks` | 1 | Number of chunks to transcribe in parallel |
| `podcasts_dir` | "" | Default directory for podcast storage and processing |
| `server_base_url` | "" | Base URL for static HTTP serving of feeds and web pages |
| `gemini_api_key_file` | "" | Path to file containing Gemini API key |
| `gemini_chunk_sec` | 900 | Audio per Gemini request; above ~1200 Gemini returns an empty response |
| `backend_type` | "standalone" | Backend provider: `"standalone"` (native, default) |

## Standalone Backend Architecture

`pod` is a self-contained, native podcast management system:
- **Subscriptions**: Stored locally in `~/.config/pod/podcasts.json` (or `~/.config/abs/podcasts.json`).
- **Feed Checking**: `pod server feeds` checks upstream feeds directly with conditional HTTP GET (`ETag` / `If-Modified-Since`).
- **Downloads**: `pod server download` downloads episodes directly over HTTP with resume support.
- **Feeds & Web Players**: `pod rss_gen` generates standard Apple Podcasts `feed.xml` RSS and HTML5 `index.html` static web players for every show and the full catalog. Episodes that were never downloaded are published pointing at their original audio, so a local feed carries the show's whole run.
- **Mobile Sync**: Works with standard podcast clients (e.g. AntennaPod) over Tailscale/Caddy without proprietary server apps.

### Legacy Server Import (Optional)

External servers are deactivated from normal operations and only serve as optional migration sources for one-time subscription import:
- Import from OPML: `pod server import subscriptions.opml`
- Import from PodFetch: `pod server import` (reads configured `podfetch_db_path` or API)

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
pod info check              # Test external services (Whisper, ABS, Kitty)

# Feed sync & server operations (all under `server`)
pod server feeds            # Check upstream RSS feeds for new episodes (reads remote)
pod rss_gen          # Generate local feed.xml + index.html (writes local)
pod server download         # Download pending episodes according to podcast policy
pod server download p0001 -k 3 # Download up to 3 missing episodes for a podcast
pod server flush p0001 --dry-run # Preview removing this podcast's audio, keeping transcripts
pod server flush p0001       # Remove MP3/precut audio and disable automatic downloads
pod server publication-sync --dry-run # Preview correcting local publication metadata from the source catalog
pod server publication-sync # Correct cached/status dates; unknown source dates stay unknown
pod server prune            # Prune old episodes per retention policy
pod server policy p0001     # View or update download/AdR policy
pod server timeline         # Display online availability timestamps table

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

Commands may be abbreviated to any unambiguous prefix. Two letters are shared: `r` between `rm_ads` and `rss_gen` (use `rm` or `rs`), and `t` between `transcribe` and `tui` (use `tr` or `tu`). Every other initial is unique.

| Command | Prefix | Usage | Description |
|---------|--------|-------|-------------|
| `config` | `c` | `pod config [command]` | View and manage application configuration, profiles, and cache |
| `detect` | `d` | `pod detect [options] <path...>` | Detect ad segments in an existing transcript, without re-transcribing or cutting |
| `rss_gen` | `rs` | `pod rss_gen [id-or-title]` | Generate the RSS feed and web pages for local podcasts |
| `info` | `i` | `pod info [options] [id\|latest [N]\|status\|check]` | Library query, inspection, cuts breakdown, transcripts, and status diagnostics |
| `player` | `p` | `pod player [command]` | Control background audio playback (`play`, `stop`, `pause`, `status`) |
| `queue` | `q` | `pod queue [command]` | Manage the ad removal (AdR) processing queue (`list`, `add`, `remove`, `clear`) |
| `rm_ads` | `r` | `pod rm_ads [command] [paths...]` | Process audio files for ad removal (`recut`, `export`, `audit`) |
| `server` | `s` | `pod server [command] [options]` | Podcast RSS feed sync, episode downloads, and retention policies |
| `transcribe` | `tr` | `pod transcribe <path...>` | Transcribe an audio or video file; extracts the audio track from video |
| `tui` | `tu` | `pod tui [directory]` | Interactive TUI browser for podcasts and episodes |

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
