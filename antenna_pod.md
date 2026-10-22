# AntennaPod Setup Guide for Pod Feeds

This guide covers how to subscribe to and stream podcasts managed by `pod` using the **AntennaPod** mobile app.

---

## 1. Network Requirements

- The phone must be able to reach the **public HTTPS origin**:
  `https://pod.h.sarielhp.org/podcasts/`
- This is the origin Caddy serves the library from; the web pages and the OPML
  file are reachable there from the LAN and the Tailscale network.
- Do **not** subscribe with a Tailscale-only hostname (such as `tqed:8080`).
  The feed embeds absolute enclosure URLs built from `server_base_url`, and a
  phone that cannot resolve that hostname will report **"cannot find the
  server"** when it tries to download an episode.

Check the configured origin with:

```bash
pod config get server_base_url
# https://pod.h.sarielhp.org/podcasts
```

---

## 2. Setup Methods

### Method 1: Bulk Import (All Podcasts via OPML) — Recommended

All configured podcast feeds are exported to an OPML file hosted directly on the server:
```
https://pod.h.sarielhp.org/podcasts/antennapod.opml
```

#### Steps on Mobile Device:
1. Open your phone's web browser and navigate to:
   ```
   https://pod.h.sarielhp.org/podcasts/antennapod.opml
   ```
   Your browser will download the file `antennapod.opml`.
2. Open **AntennaPod**.
3. Go to **Subscriptions**.
4. Tap the three-dot menu (**⋮**) in the top-right corner → select **Import/Export**.
5. Select **OPML import**.
6. Choose the downloaded `antennapod.opml` file from your device's Downloads folder.
7. Tap **Import All** (or select individual shows).

---

### Method 2: Add an Individual Podcast Manually

To subscribe to a specific show by RSS address:
1. Open **AntennaPod**.
2. Tap **+** (**Add Podcast**).
3. Tap **Add Podcast by RSS address**.
4. Enter the show's feed URL:
   ```
   https://pod.h.sarielhp.org/podcasts/<Show-Folder>/feed.xml
   ```

   *Examples:*
   - `https://pod.h.sarielhp.org/podcasts/5_4/feed.xml`
   - `https://pod.h.sarielhp.org/podcasts/The_Daily/feed.xml`

5. Tap **Confirm** → tap **Subscribe**.

Opening a show page in the browser and using **Subscribe in AntennaPod** also
works: the page builds the feed URL from the origin it was loaded from.

---

## 3. Regenerating the OPML File

The OPML export is not part of `pod library do-all`. Publish every feed and
refresh the OPML after adding new subscriptions:

```bash
pod gen_rss
pod library opml export /media/podcasts/pod/antennapod.opml
```

`pod gen_rss` (run after the export) regenerates the catalog page so its
**AntennaPod OPML** button appears. Re-import the file into AntennaPod to pick
up new shows.

---

## 4. Technical Architecture & Features

### Audio Streaming & Range Requests
- Caddy serves audio files with full HTTP Range request support (`206 Partial Content`).
- AntennaPod can seek forward/backward within audio files and resume partially streamed episodes instantly without re-downloading.

### Feed Updates & Cover Art
- Each podcast folder in `/media/podcasts/pod/<show>/` contains a standard iTunes-compatible `feed.xml` and local cover image (`cover.jpg` / `cover.png`).
- When `pod` downloads new episodes, adds subscriptions, or completes ad removal, it updates `feed.xml` with:
  - Exact file size (`length`) and MIME type (`audio/mpeg`).
  - Audio duration (`itunes:duration`).
  - Stable, deterministic episode GUIDs.
  - Absolute enclosure URLs on `server_base_url` — these must be reachable by the phone.
  - Channel and episode cover art icons via both standard RSS `<image><url>` and `<itunes:image href="...">` for seamless AntennaPod icon rendering.
- Archived (cold-storage) episodes are published under
  `/podcasts/archive/<show>/<file>`; Caddy serves that path from the HDD mount.
- AntennaPod checks `feed.xml` on its standard refresh schedule or when pulled down manually.

### Troubleshooting: "cannot find the server"

The show page uses relative audio links, so it plays over whichever origin
served it. The RSS feed cannot: podcast clients require absolute enclosure
URLs. If the web page works but downloads fail, `server_base_url` points at a
host the phone cannot resolve or reach. Set it to the public origin and
republish:

```bash
pod config set server_base_url https://pod.h.sarielhp.org/podcasts
pod gen_rss
```
