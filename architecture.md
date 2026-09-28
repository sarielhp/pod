# Architecture of `pod`

## Overview

`pod` is a standalone, self-contained podcast management and automatic ad removal system.

Historically, `pod` started as an ad-removal companion for external podcast servers. **Today, `pod` has been completely decoupled from external servers and operates as its own native backend.**

PodFetch plays only an optional, legacy role (one-time subscription import via `pod server import`). During normal operations, external servers are deactivated and blocked by runtime policy guards.

---

## Core Principles

1. **Self-Contained Backend**:
   - `pod` manages its own podcast subscriptions, upstream feed updates, episode downloads, ad detection, and audio cutting without requiring any running podcast server daemon.
2. **Deterministic Configuration**:
   - Subscriptions live in `~/.config/pod/podcasts.json`.
   - Global configuration lives in `~/.config/pod/config.json` with `backend_type: "standalone"`.
   - Per-podcast configurations (`podcast.json`) reside directly inside each podcast folder in `podcasts_dir`.
3. **Static Serving & Mobile Integration**:
   - For every podcast, `pod` produces:
     - A standard, iTunes-compatible RSS feed (`feed.xml`).
     - A responsive, self-contained HTML5 web player (`index.html`).
   - A root catalog (`index.html`) and OPML export (`antennapod.opml`) allow standard web browsers and mobile podcast clients (e.g., AntennaPod over Tailscale via Caddy) to stream and sync episodes without proprietary APIs.
4. **Ad Removal Engine**:
   - Audio transcription via local `whisper.cpp` or Google Gemini API.
   - LLM-based ad segment detection and verification.
   - Lossless or re-encoded ffmpeg cut-and-splice with ID3 tag preservation.

---

## System Architecture

```
                       +-----------------------------------+
                       |    Upstream Podcast RSS Feeds     |
                       +-----------------+-----------------+
                                         |
                                         v HTTP(S) GET
                       +-----------------+-----------------+
                       |       pod native downloader       |
                       |      (pkg/podcast/downloader)     |
                       +-----------------+-----------------+
                                         |
                                         v MP3 files
                       +-----------------+-----------------+
                       |    Local Storage (podcasts_dir)   |
                       |       /media/podcasts/clean/      |
                       +-----------------+-----------------+
                                         |
            +----------------------------+----------------------------+
            |                                                         |
            v                                                         v
+-----------------------+                                 +-----------------------+
|  Ad Removal Pipeline  |                                 | Static Web & Feeds    |
|   - Whisper / Gemini  |                                 |  - <Show>/feed.xml    |
|   - Ad Detection LLM  |                                 |  - <Show>/index.html  |
|   - ffmpeg audio cut  |                                 |  - Root index.html    |
+-----------+-----------+                                 |  - antennapod.opml    |
            |                                             +-----------+-----------+
            v                                                         |
   Cleaned Audio Files                                                v
            |                                             +-----------------------+
            +-------------------------------------------->|     Static Server     |
                                                          |  (Caddy / Tailscale)  |
                                                          |       Port 8080       |
                                                          +-----------+-----------+
                                                                      |
                                                          +-----------+-----------+
                                                          |    Clients / Players  |
                                                          |  - AntennaPod (Mobile)|
                                                          |  - Web Browser Player |
                                                          |  - pod TUI & CLI      |
                                                          +-----------------------+
```

---

## Component Breakdown

### 1. Subscription Management (`pkg/podcast/subscription.go`)
- **Storage**: `~/.config/pod/podcasts.json`
- Stores subscribed podcasts with:
  - `id`: Short deterministic identifier
  - `title`: Podcast display title
  - `feed_url`: Upstream RSS feed URL
  - `folder`: Subdirectory inside `podcasts_dir`
  - `download_policy`: Retention and download strategy (`latest`, `all`, `none`, etc.)
  - `download_k`: Number of episodes to maintain
- **CLI Commands**:
  - `pod server add <feed-url> [title]`: Subscribe to a new podcast.
  - `pod server remove <id-or-title>`: Remove a subscription.
  - `pod server list`: List all active subscriptions with local episode counts.
  - `pod server import [file.opml]`: Import feeds from OPML or PodFetch database.

### 2. Native Feed Checker & Downloader (`pkg/podcast/`)
- **Feed Checking (`pkg/podcast/feed_check.go`)**:
  - `pod server feeds`: Fetches upstream RSS feeds directly and concurrently using conditional HTTP GET (`ETag` and `If-Modified-Since` headers stored in `feed_cache.json`).
  - Reports newly published episodes without requiring external servers.
- **Episode Downloader (`pkg/podcast/downloader.go`)**:
  - `pod server download`: Evaluates download policies, detects missing episodes, and downloads audio directly via HTTP with resume support.

### 3. Static Webpage & RSS Generator (`pkg/podsite/`, `pkg/podcast/publish.go`)
- **RSS Feeds (`feed.xml`)**:
  - Fully compliant with Apple Podcasts / iTunes RSS specifications.
  - Uses local duration and exact byte counts.
  - Formats enclosure URLs using `server_base_url` (e.g. `http://100.123.184.3:8080/podcasts/...`).
- **Static Web Player (`index.html`)**:
  - Each show directory contains an `index.html` featuring embedded HTML5 `<audio>` players, episode notes, durations, and ad-removal report links.
  - The root `podcasts_dir` contains a catalog `index.html` listing all shows, episode counts, artwork, and feed links.
  - Styled with clean CSS supporting system light/dark mode without external dependencies.
- **Command**:
  - `pod gen_rss [podcast] [N]`: Regenerates `feed.xml` and `index.html` for specific shows or the entire catalog.

### 4. Ad Removal Engine (`pkg/adremoval/`, `pkg/detect/`, `pkg/pipeline/`)
- **Queue**:
  - `pod queue run`: Processes pending audio files in the queue.
- **Transcription**:
  - Local `whisper.cpp` container/daemon or direct Gemini Flash 2.5 audio API.
- **Ad Detection**:
  - Prompt-engineered LLM ad detection (Gemini / Ollama) locating sponsor breaks, intros, outros, and network promos.
- **Audio Splicing**:
  - Lossless or stream-copy cut via `ffmpeg`.
  - ID3 metadata, chapters, and cover art preserved.

### 5. Playback Daemon & TUI (`pkg/player/`, `pkg/tui/`)
- Headless background playback daemon with MPRIS D-Bus support.
- Full-featured interactive terminal UI (`pod tui`) across 19 views.

---

## Optional Legacy Importer

- **PodFetch**:
  - `pod` contains a minimal read-only import adapter in `pkg/backend/podfetch*.go`.
  - It is not an operational backend (`backend_type: "standalone"` is the sole primary runtime backend).
  - PodFetch integration exists solely for optional initial subscription migration via `pod server import`.
