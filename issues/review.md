# Deep review of `pod`

- **Target:** version 0.5.12, HEAD `629ccc9`, branch `main`
- **Date:** 2026-09-27
- **Gate at review start:** `make check` green; `go test -race ./...` green; both audit
  baselines (`tools/staticcheck-baseline.txt`, `tools/go-audit-baseline.txt`) empty.
- **Nothing in this document is a build or test failure.** Every item is a latent defect,
  a design problem, or documentation drift that the current suite does not exercise.

## How this review was produced

Seven parallel dimension reviews (correctness & data-loss; concurrency; security &
untrusted input; error handling, resource lifetime & I/O atomicity; architecture; CLI/UX;
tests, tooling & documentation), followed by independent re-reading of every high-severity
claim. Extra mechanical passes: a project-shaped semgrep ruleset (which found the
discarded destructive errors, the non-atomic writes, and the TUI goroutine read), `errcheck
-blank`, `gocyclo`, and the race detector.

### Independent verification by an external model

The review's own claims were then handed to the **external `claude` CLI**
(`~/.local/bin/claude`) as an adversarial, read-only verifier
(`--restricted --allowedTools "Read Grep Glob"`, no file writes, no execution), told to
**try to refute** each claim.

An important caveat about which model did the judging: this machine routes the Claude CLI
through a LiteLLM gateway (`ANTHROPIC_BASE_URL=http://tqed:4000`) with
`ANTHROPIC_MODEL=deepseek-v4.1-flash`, so a default invocation runs the *same* model as
the review's author — a correlated judge, not an independent one. The independent run was
therefore pinned to a genuinely different family,
`openrouter/anthropic/claude-opus-5.5`, via `ANTHROPIC_MODEL`/`--model`.

**Verification status:**

| Batch | Contents | Independent judge (Claude Opus) |
|---|---|---|
| A | the six high-severity findings (A1–A6) | **run — all CONFIRMED, with refinements below** |
| B | the 15 medium findings (B1–B15) | **not completed** — the run was interrupted; a separate run had been rejected by the gateway's in-flight budget |
| C | the low/tooling findings (C1–C20) | **not completed** — the first attempt failed with a gateway 402; not retried |

So: **the six high-severity findings have been corroborated by an independent,
different-family model; the medium and low findings below have not yet been through that
process** (they have been re-read by the author where marked `[verified]`, or are
attributed to a dimension agent as `[reported]`). Re-running batches B and C through the
same judge is the obvious next step.

### Side effect of this review

Running `go test -race ./...` to establish the baseline triggered finding **T1**:
`pkg/player`'s tests write to the real `~/.config/pod/play_queue.json`, and its mtime was
changed to 2026-09-27 18:36:29 -0500. The file's contents were not read. The previous play
queue is not recoverable from here. The fix is the `TestMain` in T1.

---

## Executive summary

`pod` is unusually disciplined for its size: 62k lines, one `main` package plus 19 library
packages in an acyclic graph (`pkg/util` is the most-imported at 15 importers; no library
package imports `cli` or `tui`), every function inside the tiered complexity limits, and a
real file-locking and atomic-write story in the hot paths.

The defects cluster in the **less-exercised surfaces**, which is exactly where a green gate
cannot help:

1. **Retention is driven by the feed's `<pubDate>`, not by arrival time.** With the default
   30-day policy, subscribing to a feed whose newest episode is older than 30 days
   downloads that episode and then **deletes it — and does it again on every subsequent
   run**. Remote data decides a local deletion under a default the user never opted into.
2. **Error handling is silently lossy on the write paths that matter.** `podcast.json` and
   the transcript JSON are written truncate-then-write; 15 `_ = config.SaveConfig(cfg)`
   sites print "Updated" whether or not the write succeeded; a busy queue lock is
   indistinguishable from success; and `pod server clean-orphans` exits 0 after failing.
3. **Two deleters disagree about safety.** `retention.go` locks the episode and checks
   in-flight state before removing; `StandaloneBackend.ApplyKeepPolicy` (the `prune <N>`
   path) does neither and counts failures as successes. Likewise `pod cut` drops the error
   from its backup step and can overwrite the original, while the batch path aborts in the
   same situation.

Best payoff-to-risk change: **route the two remaining truncate-then-write paths through
`util.WriteFileAtomic` and stop discarding `SaveConfig`'s error** — four mechanical edits
that close the corruption windows and the "reports success while failing" class.

---

## High-severity findings

Each was independently confirmed by the external Claude Opus judge; its refinements are
folded in and marked **Judge note**.

### H1. Retention deletes a just-downloaded episode when the feed's `pubDate` is old
`[verified]` · judge: **CONFIRMED**

- `pkg/podcast/subscription_download.go:332-336` stamps a downloaded file with the feed's
  date: `st.PublishedAt = <feed pubDate>; st.PublicationSource = "feed"`.
- `pkg/podcast/cache.go:341-349` (`GetEpisodePublicationTime`) returns that stored date
  first, before ever consulting the file's mtime.
- `pkg/podcast/retention.go:76-85` (`isEpisodeExpired`) compares that date to `now-30d`,
  falling back to `fi.ModTime()` only when the date is zero.
- `pkg/podcast/subscription_download.go:263,305` calls `autoCleanupPodcast` immediately
  after every subscription download.
- `pkg/config/podcast.go:189,203-210`: the default keep policy is `month` = 30 days with
  `AutoCleanup = true`; `DefaultDiscoveredPodcastConfig` passes the same.

**Failure path:** subscribe to a feed whose episodes carry a `pubDate` older than 30 days
(a back-catalogue feed, a show republishing old episodes, or a hostile feed) → `pod sync`
→ `downloadEpisode` writes the MP3 and stamps the old date → `autoCleanupPodcast` →
`pruneExpiredEpisode` → `os.Remove(path)`. The audio is gone before the user can play it,
and the ad-free cut the queue is about to make is lost too. `IsEpisodeInRemoteFlight`
(`retention.go:97`) does not cover a local download: a fresh episode is `StateDownloaded`.

**Judge note — this is worse than the finding first stated.** After the delete, the
episode locator no longer finds the file, so **every later run re-downloads and re-deletes
it**, an indefinite download/delete loop. Explicit `--count`/`--all` back-catalogue
downloads are affected the same way. Setting the podcast's keep policy to `always` avoids
it.

**Fix:** floor retention at arrival time — in `isEpisodeExpired` use the later of the
publication time and the file mtime, or store an explicit `DownloadedAt` and use it
whenever `PublicationSource == "feed"`. Visible behaviour change → `CHANGELOG.md`.

**Verification test:** write an MP3 with mtime `now`, a status with
`PublicationSource:"feed"`, `PublishedAt: 2020`, run `ApplyPodcastKeepPolicy` with the
30-day policy, assert the file survives.

### H2. `pod cut` can destroy the original audio when its backup step fails
`[verified]` · judge: **CONFIRMED** (narrow trigger)

`pkg/pipeline/cut_file.go:172-178`:
```go
if outputPath == mainMP3 && sourceAudio == mainMP3 && util.FileExists(mainMP3) {
    if !util.FileExists(precut) {
        if err := os.Link(mainMP3, precut); err != nil {
            _ = util.CopyFileErr(mainMP3, precut)   // error discarded
        }
    }
}
```
`pod cut file.mp3` defaults the output to the input. If the link fails and the copy also
fails, execution continues to `util.SafeMove(tempOutput, outputPath)` and the original is
replaced with no usable backup. The batch path does this correctly
(`pkg/adremoval/batch_proc_file.go:566-575` aborts on copy failure).

**Judge note — the trigger is narrower than first stated.** A read-only directory is *not*
a trigger: link and copy fail, but `SafeMove`'s rename fails too, so the original survives.
The realistic case is a filesystem without hard links (vfat/exFAT, some SMB or FUSE mounts)
that is nearly full: the link fails, the copy fails partway with ENOSPC, and the rename
still succeeds because it needs no space. A **truncated `.precut`** is then left behind,
and `ResolveAudioFiles` picks it as the source for any later recut — compounding the loss.

**Fix:** mirror the batch path — return an error if the backup cannot be created.

**Correction to the first draft of this finding.** The claimed symlink variant was stated
too broadly. The judge established: a **live** symlink (target exists) is *not* written
through — `FileExists` follows it, `ResolveAudioFiles` selects `.precut` as the source, and
the backup branch never runs. Only a **dangling** `.precut` symlink triggers the
write-through (`FileExists` false → `os.Link` EEXIST → `CopyFileErr` opens the target with
`O_CREATE|O_TRUNC` and follows it). It also requires write access to the podcast directory
to plant the symlink. The batch path still guards this with `checkPrecutSymlink`
(`pkg/adremoval/batch_pipeline_helpers.go:144`), which `executeCutProcessing` never calls —
that asymmetry is the finding; merely "a symlink" is not.

### H3. Player daemon `stop`/`quit` kills the wrong process after a seek
`[verified]` · judge: **CONFIRMED** (narrower reachability)

`pkg/player/ipc.go:329` hands the **original** `*exec.Cmd` to the connection handler
forever. The `quit`/`stop` branch (`ipc.go:409-414`) kills that captured `cmd`:
```go
case "quit", "stop":
    if cmd != nil && cmd.Process != nil { _ = cmd.Process.Kill() }
    _ = os.Remove(PlayerSocketPath)
```
`HandleDaemonSeek` (`ipc.go:528-534`) kills the old process and replaces it:
`state.cmd = newCmd`. `HandleDaemonCycle` and `HandleDaemonSetProp` correctly read
`state.cmd`; only `stop` uses the stale parameter.

**Failure path:** seek → the new process becomes `state.cmd`, and the waiter goroutine sees
`state.cmd != current` and keeps waiting on it; then `pod player stop` → the handler kills
the already-dead original, removes the socket, returns `success`. The live audio process is
orphaned and plays on; because the socket file is gone, `IsPlayerSocketAlive()` is false and
a second `pod player stop` prints "Player is not running."

**Judge note — reachability.** This requires `mpv` to be absent, because `StartPlayerTrack`
prefers `SpawnDetachedMpv`; and with `mpg123`/`ffplay -autoexit` playback self-terminates
at the end of the track. The indefinite-playback case is realistically `cvlc` (whose command
line has no `--play-and-exit`). The logic defect is unconditional; only the visible impact
is conditional.

**Fix:** read `state.cmd` under `state.mu` in the quit branch (as `HandleDaemonCycle`
does), or drop the `cmd` parameter entirely.

### H4. Transcript JSON is rewritten non-atomically after ad detection
`[verified]` · judge: **CONFIRMED**

`pkg/adremoval/batch_pipeline_helpers.go:197-201` reads the transcript JSON, mutates the
ad-detection fields, and writes it back with `os.WriteFile` (truncate-then-write). The
normal writers — `format.SaveJSONTranscript` (`pkg/format/subtitles.go:150`) and
`pipeline.SaveJSONTranscript` — use `util.WriteFileAtomic`. A kill between the truncate and
the completed write leaves a 0-byte or partial transcript: the most expensive artifact the
tool produces, forcing a re-transcription. It runs on every ad-detection pass, success or
failure.

**Judge note:** the window is short (one write of an already-marshalled buffer), and ext4's
`auto_da_alloc` partly protects this pattern — but not every filesystem has an equivalent.

**Fix:** `return util.WriteFileAtomic(jsonFile, append(content, '\n'), 0644)`.

### H5. Episode downloads are unbounded
`[verified]` · judge: **CONFIRMED**

`pkg/podcast/downloader.go:85`: `written, err := io.Copy(out, resp.Body)` has no
`io.LimitReader` and no `Content-Length` check. Feed bodies are capped at 32 MiB
(`feed_fetch.go:195`) and cover images at 10 MiB (`downloader.go:144`); episode audio is
capped only by the 30-minute client timeout, with `context.Background()`.

**Judge note:** the temp file lives in `.work/` and the deferred `os.Remove` deletes it on
error or timeout, so the disk usage is *transient* rather than permanent — but it can still
fill the volume while it runs (on the order of 180 GB at 100 MB/s over 30 minutes), after
which the atomic writes everywhere else start failing.

**Fix:** cap the copy (`io.LimitReader(resp.Body, max+1)`, error on overflow), make the cap
configurable, and reject non-audio content types.

### H6. `pod server clean-orphans` exits 0 after failing to delete
`[verified]` · judge: **CONFIRMED**

`pkg/podcast/orphans.go:187-188` accumulates `res.FailedCount`/`res.Errors` and then
`return res, nil`. `pkg/cli/server_clean_orphans.go:47` does `_, err = ...; return err`, so
`Execute` returns 0 and the failures are never printed (and `--quiet` hides the printed
"N failed" line even in non-quiet mode). Separately, `confirmOrphanDeletion`
(`orphans.go:208-223`) prints the `[y/N]` prompt with no `--quiet` guard, and
`handleServerCleanOrphans` passes `Out: os.Stdout` rather than `outFor(cli)`.

**Judge note:** printing the prompt under `--quiet` is arguably intended — reading stdin
without showing a prompt would look like a hang, and `--force` skips it. The `os.Stdout`
choice mainly matters for tests or embedding that set `cli.Out`.

**Fix:** return a joined error when `FailedCount > 0`; print failing titles via `outFor`;
pass `outFor(cli)`. Exit-code change → `CHANGELOG.md`.

---

## Medium-severity findings

`[verified]` = re-read by the author. `[reported]` = from a dimension agent, not
independently re-derived. None of these has yet been through the independent judge.

| ID | Finding | Sev | Loc | Fix |
|---|---|---|---|---|
| M1 | `SavePodcastConfig` writes `podcast.json` truncate-then-write, unlike every other store; `LoadPodcastConfig` silently returns defaults on parse failure, so corruption becomes a silent policy reset | med | `pkg/config/podcast.go:645`; load fallback `:519-526`; callers incl. `pkg/tui/tui.go:226` `[verified]` | `util.WriteFileAtomic`; distinguish not-exist from corrupt on load |
| M2 | `StandaloneBackend.ApplyKeepPolicy` (the `prune <N>` count path) removes MP3s with no file lock, no in-flight check, and `deleted++` regardless of the `os.Remove` result; `retention.go` does all three correctly | med | `pkg/podcast/standalone_backend.go:366-399`; caller `pkg/cli/server_keep.go:72`; contrast `pkg/podcast/retention.go:96-125` `[verified]` | unify behind one implementation, or add lock + in-flight + real error counting |
| M3 | 15 sites do `_ = config.SaveConfig(cfg)` and then print `Updated '<key>' = '<value>'`; a failed write (read-only dir, ENOSPC) is reported as success | med | `pkg/cli/config_get_set.go:88,94,136,195`, `config_whisper.go:272,298,306,314`, `config_llm.go:51`, `config_llm_import.go:59`, `config_migrate.go:80,117,141` `[verified via errcheck]` | return/propagate the save error |
| M4 | Queue `Claim`/`Finalize` call `util.AcquireFileLock`, which returns `(nil, nil)` on contention, then `return err` — a busy lock is indistinguishable from success; `Finalize` can leave an item stuck as `downloading` | med | `pkg/podcast/queue.go:270-273, 295-299` `[verified]` | use `AcquireFileLockWithTimeout` and return a real error on failure |
| M5 | `Enqueue`/`Remove`/`Clear`/`Items` do whole-file read-modify-write under only the in-process mutex, while `Claim`/`Finalize` take the cross-process file lock — two pod processes (TUI + CLI) can lose updates or resurrect flushed items | med | `pkg/podcast/queue.go:190,229,250,258` `[verified]` | one `withQueueFileLock` helper used by every RMW path |
| M6 | `removeWorkDirs` `RemoveAll`s any `.work/` in a scanned directory with no lock and no staleness check (unlike `CleanupStaleWorkDirs`), so `pod rm_ads <dir>` can delete a live worker's temp files | med | `pkg/cli/rmads_expand.go:128-138` `[verified]` | reuse the lock-aware cleanup, or require staleness |
| M7 | Blind SSRF: feed-controlled enclosure and cover URLs are fetched with no private-address guard, redirects are followed unvalidated, and the http→https rewrite explicitly exempts `127.0.0.1`/`localhost` | med | `pkg/podcast/feed_xml.go:259`; `pkg/podcast/downloader.go:63,121` `[verified]` | shared client that rejects loopback/private/link-local addresses and re-validates redirects, behind a config opt-out for LAN feeds |
| M8 | The download worker can run twice: `runWorkerLoop` clears `workerRunning` (`queue.go:418`) and `TriggerWorker`'s deferred clears it again (`:393-395`), so a new worker can start in between and then have its ownership clobbered | med | `pkg/podcast/queue.go:378-422` `[verified]` | single owner for the flag; clear only on the panic path |
| M9 | `AudioPlayer` holds `p.mu` across blocking socket IPC (`IsPlayerSocketAlive` + five sequential `QueryPlayerStatus` round-trips with 1 s deadlines each), so a stalled player freezes the TUI for seconds per 500 ms tick | med | `pkg/player/controller.go:311-336`; also `TogglePause:198-212`, `Seek:230-246` `[verified]` | query outside the lock, then lock to store |
| M10 | The player IPC socket is created with no `chmod` and no peer-credential check; cross-user access depends on umask | med | `pkg/player/ipc.go:275-278` `[verified]` | `os.Chmod(..., 0600)` after listen; check `SO_PEERCRED` is own uid |
| M11 | `util.VerifyTempFile(tmp)` return value discarded inside `installNoAdsOutput`, so the temp-file policy is not enforced on that path | med-low | `pkg/adremoval/batch_proc_file.go:473` `[verified]` | return the error |
| M12 | `pod server flush` deletes every `.mp3`/`.mp3.precut` with no confirmation and no `--force` gate (only an opt-in `--dry-run`), and prints the warning afterwards; `clean-orphans` prompts for far less | med | `pkg/cli/server_flush.go:112-157` `[verified]` | confirmation naming the count, `-f/--force` to bypass |
| M13 | `pod server download` and `pod fetch` print per-episode failures but return nil, so a run where downloads failed exits 0 | med | `pkg/cli/server_subscription.go:350-357`; `pkg/cli/fetch.go:136-141` `[verified]` | return non-nil when `len(res.Failures) > 0` |
| M14 | Four aliases exist, contrary to AGENTS.md rule 11: `tui`→`ui`, `queue list`→`queue ls`, `server feeds`→`server feeds update`, `info check whisper`→`whisper-server`; tests and CHANGELOG defend them | med | `pkg/cli/tui.go:8`; `pkg/cli/queue.go:48,476`; `pkg/cli/server_feeds.go:178`; `pkg/cli/parse.go:19` `[verified]` | remove; update the tests that encode them; add a no-alias guard test |
| M15 | README claims every command abbreviates to one letter (`c,d,g,i,p,q,r,s,t,u`), but `detect`/`gen_rss`/`rm_ads`/`transcribe` are `Hidden` and clihelp excludes hidden commands from prefix matching — so `pod r`, `pod d`, `pod g`, `pod u` fail; README also documents the alias `ui` and omits `fetch` and `cut` entirely | med | `README.md:195-208` `[verified]` | regenerate the table from the router; document the hidden set |

---

## Low-severity and tooling/documentation findings

| ID | Finding | Sev | Loc | Fix |
|---|---|---|---|---|
| L1 | `stale_work.go`'s `.worker`/`.collect` lock guard is dead code — nothing in the repo creates those lock files; the test's "worker" scenario fabricates the lock and so certifies a guarantee production does not have. The remaining real guard (the sibling-MP3 flock) is only taken by the batch path, so a long-stalled `transcribe`/`cut`/`gemini` job can have its `.work/` reclaimed | med-low | `pkg/util/stale_work.go:104-135`; `pkg/util/stale_work_test.go:49-58` `[verified]` | delete the dead branch or have every processing path take the lock; fix the test |
| L2 | `fmt.Errorf("...: %w", err)` is called with a nil `err` on the lock-busy branch, producing `%!w(<nil>)` in user-visible messages | low | `pkg/episode/status.go:179`, `pkg/podcast/retention.go:114`, `pkg/tui/tui_data_queue.go:63` `[verified]` | `errors.New("... is locked")` |
| L3 | `pod info list` takes a file lock and **writes** episode status while rendering a listing (a read command mutates state, without holding the lock across the write) | low | `pkg/cli/info_list.go:347-372` `[verified]` | move the stale-status repair to an explicit command |
| L4 | `PreserveMetadata` has a fallback that writes `dst + ".meta" + ext` **outside** `.work/` with no verification (unreachable for realistic paths, but a latent policy hole) | low | `pkg/audio/processor.go:57-61` `[verified]` | return an error instead of falling outside `.work/` |
| L5 | Gemini upload leaks the `io.Pipe` reader and the writer goroutine if `http.NewRequestWithContext` fails after the goroutine starts | low | `pkg/gemini/studio.go:122-131` `[verified]` | `defer pr.Close()` on the error paths |
| L6 | `DeleteGeminiStudioFile` ignores every failure and never checks the response status, so an uploaded episode can persist in the user's Google account | low | `pkg/gemini/studio.go:155-175` `[verified]` | check status and report; delete with a fresh short-timeout context |
| L7 | `runs[0]` is indexed after an `err != nil` guard that no longer covers it; a `(nil, _, nil)` return would panic | low | `pkg/cli/detect.go:118-126` `[verified]` | hoist the empty check |
| L8 | `SaveCutsJSON` reports `Changed: false` on a write failure instead of an error, so callers can mark an episode done without its cuts metadata | low | `pkg/format/cuts.go:137-145` `[reported]` | return the error / a distinct failed flag |
| L9 | Docs drift: `AGENTS.md:81` "Current version: 0.1.3" (VERSION is 0.5.12); "no external dependencies beyond stdlib" when `go.mod` has 14 direct requires; a `pkg/remote` that does not exist; "pipeline still prints" although it has zero direct writes; `verifyTempFile` vs `util.VerifyTempFile`; Agent Rule #7's "no function over 80 lines" contradicts the tiered sizing section; `tools/check` does not run `go mod tidy` as claimed | low | `AGENTS.md:7,16,81,178,193,197,206,243,249` `[verified]` | update the file |
| L10 | Stale binary name in user-facing help: "`abs help <command>`" and "use abs info transcript" | low | `pkg/cli/app.go:61`, `pkg/cli/info.go:25` `[verified]` | s/abs/pod/ |
| L11 | `pod config get` documents non-existent keys (`rffmpeg`, `abs-url`) and points a wrong key at `pod config show`, which does not list keys | low | `pkg/cli/config.go:46`; `pkg/cli/config_get_set.go:188` `[verified]` | fix the key list and the remediation text |
| L12 | Dead dispatch cases for removed names `sync` and `ui` | low | `pkg/cli/execute.go:78,88` `[verified]` | delete |
| L13 | The feed `LimitReader` silently truncates a >32 MiB feed rather than erroring, yielding a partial document | low | `pkg/podcast/feed_fetch.go:195` `[reported]` | read `max+1` and error on overflow |
| L14 | `pod config migrate` imports post-processor executable paths verbatim, bypassing the `exec.LookPath` validation `config processor set` applies (argv, not shell, so arbitrary-executable, not injection) | low | `pkg/cli/config_migrate.go:56-57`; run at `pkg/cli/server_download.go:263` `[reported]` | validate or confirm on import |
| L15 | `Subscription.Folder` is only sanitised when empty; a value loaded from `podcasts.json` (or copied from a legacy backend's `RelPath`) is joined to `PodcastsDir` unvalidated | low | `pkg/podcast/subscription.go:61-74,140-142` `[reported]` | `filepath.Clean` + containment check against `PodcastsDir` |
| T1 | **`pkg/player` has no `TestMain` and writes to the user's real config dir** (`~/.config/pod/play_queue.json`), and uses `t.Skipf` when no file was written, masking the very failure under test | high | `pkg/player/player_test.go:173-200`; `pkg/player/persist.go:11-26` `[verified]` | add a `TestMain` isolating `XDG_CONFIG_HOME`/`HOME`; change the skip to a fatal |
| T2 | `RunCleanOrphans`/`DeletePodcastEpisode` have 0.0% coverage; the destructive path is untested | high | `pkg/podcast/orphans.go:135-188`; `pkg/podcast/standalone_backend.go:330-352` `[verified]` | test through the existing mock backend |
| T3 | No shared test isolation harness; five packages hand-roll `TestMain` and `player`/`config`/`gemini`/`transcribe` omit it; `podcast.Open` always binds the process-wide queue/cache at real config paths | high | `pkg/podcast/library.go:49-57`; `pkg/player/persist.go:12` `[verified]` | promote the body into `podtest.IsolateMain` |
| T4 | Coverage absent where consequence is highest: `audio` 17.8% (ffmpeg primitives 0%), pipeline cut orchestration 0%, `SaveConfig`/`SavePodcastConfig`/`LoadPodcastConfig` 0%, `player/ipc.go` ~0%, `DeletePodcastEpisode` 0% | high | `pkg/audio/ffmpeg.go`, `pkg/audio/processor.go:56-66`, `pkg/config/config.go:104`, `pkg/config/podcast.go:517,603`, `pkg/pipeline/cut_file.go:154` `[reported]` | test the deleters and config stores first; add an `AudioProcessor` fake |
| T5 | Docs contradict the tree (see L9, plus `README.md:33-34,190,195` and `architecture.md:74,92-129` still showing the `abs` binary and two non-existent files) | med | as noted `[verified]` | correct each claim |
| T6 | Tests that cannot fail: `TestDetectImageFormat` asserts a constant against itself; several tests have no assertion; `TestDisplayNameRTL` only logs and is the sole coverage of the RTL display logic | med | `pkg/kitty/kitty_test.go:8`; `pkg/tui/tui_screens_test.go:35,67,72`; `pkg/util/util_test.go:89-103` `[reported]` | assert observable behaviour or delete |
| T7 | The `stale_work` "worker" test certifies a lock no production writer takes (same as L1) | med | `pkg/util/stale_work_test.go:49-56` `[verified]` | wire a real lock or delete the branch and scenario |
| T8 | The offline-transport safety net (which stops tests reaching the network) has no test of its own | med | `pkg/podcast/podtest/podtest.go:19-27` `[reported]` | unit-test `RoundTrip`/`isLoopback` |
| T9 | ~20 one-off migration scripts committed under `tools/`, none referenced by `Makefile`, `tools/check` or AGENTS.md | low | `tools/` `[reported]` | delete or quarantine with a README line |
| T10 | `make ci` runs the gate twice (`ci: check` prerequisite *plus* `./tools/check --full`), and its govulncheck step prints "Passed" on **both** branches — the branch matching "Your code is affected by" prints "Passed" and the exit status is ignored, so a real vulnerability cannot fail the release gate | med | `Makefile:11,73`; `tools/check:130-136` `[verified]` | drop the duplicate prerequisite; exit non-zero on a real finding |
| T11 | Timeout/package-count drift: docs say `-timeout 30s` and 17 packages; the gate uses 180s and there are 20 package dirs | low | `AGENTS.md:7,249`; `tools/check:139,143` `[reported]` | update |
| T12 | README's config example has a duplicate `podcasts_dir` key | low | `README.md:55,69` `[reported]` | keep one |
| T13 | ffmpeg-dependent tests `t.Skip` silently when ffmpeg is absent, so "the cutter is tested" can be false on a bare machine | low | `pkg/pipeline/cut_file_test.go:13-22`; `pkg/cli/cut_test.go:14-20` `[reported]` | make the skip visible or fail CI when ffmpeg is missing |

---

## Architecture

The layering is real and test-enforced (`TestPackageLayering` passes; no import cycles;
`pkg/podsite` imports only `pkg/format`; no library package imports `cli`/`tui`; `pkg/util`
is a leaf imported by 15 packages). The issues are structural, not correctness.

| ID | Finding | Sev | Loc | Fix |
|---|---|---|---|---|
| A1 | The domain model lives in the *adapter* package `pkg/backend`: `Podcast` (~134 refs), `FeedEpisode` (~180), `Episode` (~36), plus generic helpers (`ParsePubDate`, `SanitizePodcastTitle`, MP3 duration). The domain package `pkg/episode` imports it (`episode/title.go:21`, `episode/quarantine.go:29`), so retiring the "legacy importer" is a whole-tree change | high (structural) | `pkg/backend/types.go:45,81,98`; `frequency.go:8`; `sanitize.go:13` | move the structs+helpers to `pkg/types`, leave `type X = types.X` aliases in `backend` so no call site changes (`TestCacheWireFormat` guards the JSON tags); rename the package to `pkg/podfetch` last |
| A2 | `pkg/podcast` is a god package (7,788 src lines, 190 exported funcs), and `cli`/`tui` bypass `Library` with ~50 package-level call sites. The TUI is the *only* writer of the podcast cache (`tui_data.go:175`) and fetches feeds itself (`tui_feed_fetch.go:72`), duplicating the library path | med | `pkg/podcast/*`; `pkg/tui/tui_data.go:175,221`; `pkg/tui/tui_feed_fetch.go:72,119` | make queue/feed-cache injectable as an `Option`, add the missing `Library` methods, migrate call sites one commit at a time, then split by concern |
| A3 | The "library packages don't print" boundary is a stale three-entry whitelist. `pkg/pipeline` has **zero** direct writes yet is documented as exempt; `pkg/remote` does not exist; the packages that actually print (`transcribe` 16 sites, `audio` 9, `format` 7, `gemini` 5, `detect` 2, `util` 3) are neither listed nor documented | med | `pkg/progress/silent_packages_test.go:16`; `AGENTS.md:205-206` | add `../pipeline`; delete the `remote` claim; thread `progress.Reporter` through `gemini`, `transcribe`, `detect` (or record them honestly as grandfathered) |
| A4 | Two competing output mechanisms: a threaded `quiet bool` *and* the `Reporter`. `gemini/pipeline.go:307` hardcodes `false` and then prints directly at `:308`, so `progress.Discard`/`--quiet` cannot suppress Gemini chatter | med | `pkg/transcribe/announce.go:12,20,58`; `pkg/detect/announce.go:10`; `pkg/gemini/pipeline.go:307-308` | delete the `quiet bool` params from library packages; the CLI already converts `--quiet` to `progress.Discard` |
| A5 | `HandleRecut` takes `rep ...progress.Reporter` and its only caller omits it (`pkg/adremoval/batch_proc_file.go:89`), so the Reporter is nil → `progress.Or(nil)` = Discard and the batch recut path reports nothing | med | `pkg/pipeline/pipeline.go:49-54` | drop the variadic, require the parameter, pass it at the call site |
| A6 | Dead abstractions: `EpisodeStateStore`/`DefaultStateStore` (test-only), `Episode.Update` (no callers), unreachable backend registry names, `IsStandalone` returning literal `true`, no-op `SyncEpisodeDuration` | low | `pkg/episode/store.go:10,45`; `pkg/episode/episode.go:89`; `pkg/backend/factory.go:10,56`; `pkg/podcast/standalone_backend.go:26-35` | delete or wire |
| A7 | Large parameter clusters threaded positionally (a 10-arg set repeated across `adremoval`/`pipeline`; `types.Config` passed by value in 51 functions, `ProcOptions` in 47) | low | `pkg/adremoval/batch_proc_file.go:58,147,191,212,231`; `pkg/pipeline/pipeline.go:94` | introduce request structs like the existing `pipeline.CutRequest`/`DetectRequest` |

**Correction to a dimension agent's claim.** The architecture agent reported that
`Library`'s doc comment ("It owns the state that used to be reached through package-level
singletons…") is false because `Open` wires the process-wide singletons. I checked
`pkg/podcast/library.go:43-57`: the comment **explicitly documents** the sharing and gives
the reason — "each serialises access to one file through its own mutex, so two instances
over the same path would serialise against different mutexes and race on it." The code and
its comment agree; that finding is **withdrawn**. The separate observation that `cli`/`tui`
bypass `Library` with package-level calls stands (A2).

---

## What was disproved (so it is not re-investigated)

- **`VerifyTempFile`'s substring check is adequate here.** It is weak in general, but every
  caller builds the path via `filepath.Join(WorkDirFor(...))` and `filepath.Abs` dissolves
  `..` lexically first, so no attacker input reaches it. The real gaps are the *chunk*
  write, which never calls it, and `PreserveMetadata`'s fallback (L4).
- **XXE / billion laughs are unreachable.** Go's `encoding/xml` does not process DTDs or
  external entities, and feed bodies are capped.
- **Title → path traversal is impossible.** `SanitizeTitle` reduces titles to
  letters/digits/underscore and maps empty/`.`/`..` to `untitled`; the extension is
  hardcoded `.mp3`. Podcast folders go through the same sanitiser.
- **`whisper_wake_command` is not remotely settable.** `pod config migrate` does not copy
  it, and the `.bw.jsonc`/`.bws` files are bubblewrap configs unreferenced by any Go file.
  The only non-typed channel is the `WHISPER_WAKE_COMMAND` env var (a legitimate, if
  undocumented, config channel); it is echoed into output, which is worth fixing.
- **`util.SyncMu` etc. are not misused.** No anonymous embedding and no value copies;
  `go vet`'s copylocks pass is clean.
- **The race detector is green but blind to the interesting paths.** Tests disable audio
  spawn and install hooks, and cannot exercise cross-process file locking — which is where
  M4/M5, M8 and H3 live.
- **`DeletePodcast` does not delete audio** — it removes only the subscription record, so
  `clean-orphans` cannot destroy files (it can strand directories, and H6 makes it *look*
  like it worked).
- **`pkg/podcast` does not print to the terminal.** Its writes go to a caller-supplied
  `io.Writer` (`orphans.go`), which the rule permits.

---

## Ordered roadmap

Each step leaves the tree safer than the one before it; none depends on a later step.

**Now — mechanical, low risk**
1. H4 — `util.WriteFileAtomic` for the transcript status rewrite (1 line).
2. M1 — `util.WriteFileAtomic` for `SavePodcastConfig` (1 line).
3. M3 — stop discarding `config.SaveConfig`'s error and stop printing "Updated" on failure.
4. H6 — non-zero exit + printed failures for `clean-orphans` (**exit-code change**).
5. H2 — abort `pod cut` when the backup cannot be created; add the `.precut` symlink guard
   (**behaviour change: abort instead of destroy**).
6. M11 — return the discarded `VerifyTempFile` error.
7. T1 — `pkg/player` `TestMain` isolating `XDG_CONFIG_HOME`/`HOME`; change its `t.Skipf` to
   a fatal.

**Next — small, targeted**
8. H1 — make retention use arrival time as a floor (**behaviour change, announce it**).
9. H5 — bound the episode download; reject non-audio content types.
10. M2 — give `ApplyKeepPolicy` the lock + in-flight check + honest error counting (or
    delegate it to `retention.go`).
11. M4 + M5 — one queue-lock helper for every read-modify-write path.
12. H3 — read `state.cmd` under the lock in the player `quit`/`stop` branch.
13. M8 — single owner for `workerRunning`.
14. M13 — non-zero exit when downloads failed (**exit-code change**).
15. M6 — lock/age-check `removeWorkDirs`.
16. T10 — make `make ci` run the gate once and let govulncheck fail.

**Later — hardening and surface work**
17. M9 — query the player socket outside `p.mu`.
18. M7 — SSRF guard behind a config opt-out (**behaviour change**).
19. M10 — `chmod 0600` the player socket.
20. M12 — confirmation + `--force` for `server flush` (**behaviour change for scripts**).
21. M14 + M15 — remove the four aliases and rewrite the README command table from the router.
22. A3–A5 — make `--quiet` real inside the library: add `pipeline` to the silent list, delete
    the `remote` claim, replace `quiet bool` with `progress.Reporter` in the announce
    helpers, and pass the Reporter `HandleRecut` currently drops.
23. L1 / A7 — lock the episode audio in every processing path so the startup reaper cannot
    delete a live `.work/`.
24. L2–L15, T2–T9, T11–T13 — the remaining table entries, in whatever order they are touched.
25. A1 + A2 — the structural work: `types` aliases for the backend DTOs first (compile-only),
    then `Library` injection and call-site migration, then any package split. Never split a
    file through a function body.
26. Re-run verification batches B and C through the independent judge.

**Tests to add alongside:** regression tests for H1–H6 and M4/M8, and a two-`DownloadQueue`-
over-one-file test — the current suite cannot see any of them.

---

## What I would look at with more time

- **ffmpeg as an attack surface.** Bytes of any content type flow from a remote feed into
  `pkg/audio/ffmpeg.go` and `pkg/gemini/pipeline.go`; whether the `filter_complex`/concat
  invocation can be steered by a filename or path from a feed was not settled.
- **A multi-process integration harness.** There is no test that runs two `pod` processes
  over one library/queue/cooldown file. That single piece of infrastructure would let the
  race detector plus an integration runner catch M4/M5/M8 and their successors.
- **The `port` cooldown meter** is a cross-process read-modify-write with no flock
  (`pkg/port/cooldown.go:123-151`); two processes can clobber each other's model rests and
  burn quota on an exhausted model. `[reported]`
- **Crash-consistency of the cut install** (`.precut` hardlink + `SafeMove`) and whether
  every writer of `.precut` is symlink-guarded after H2.
- **`WriteFileAtomic` does not fsync the parent directory**, so a rename may not be durable
  across power loss — a hardening item affecting every store.
- **The `EpisodeStatusFile` reconciliation matrix** — ~10 write sites across `pipeline`,
  `adremoval`, `cli`, `podcast`, several using the unlocked `GetOrCreateEpisodeStatus` +
  `SaveEpisodeStatus` rather than `UpdateEpisodeStatus`. Worth a lost-update analysis.
- **The legacy importers** (`pod config migrate`, `pod server import`) as the one place
  external state enters configuration.
