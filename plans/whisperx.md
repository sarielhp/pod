# Task: add a WhisperX transcription + speaker diarization service beside the existing Whisper container

You are running on the GPU host that already serves Whisper (whisper.cpp) to a
podcast tool called `pod`. Your job is to add a **second, separate** container that
does transcription **with speaker diarization**, exposing an HTTP API compatible with
the one `pod` already speaks, so `pod` can be pointed at it by changing one URL.

## Ground rules

- **Do not modify, stop, restart or remove any existing container, image, volume,
  network or Traefik/Sablier rule.** This is purely additive, with the single
  exception described under Deployment: adding the new router, service and
  middleware to Traefik's configuration, after a backup. Before you start, list
  what is running (`docker ps -a`, the compose files in use) and tell me what you found.
- Ask me before anything that needs root/sudo, and before opening any port to the
  network beyond what the existing Whisper service already uses.
- Never write the Hugging Face token into a file that is committed or into the image.
  It goes in an env file (mode 600) or a Docker secret that I create.
- Put everything in one new directory, e.g. `~/whisperx-service/`, containing
  `Dockerfile`, `app.py`, `requirements.txt`, `docker-compose.yml` (or a snippet to
  merge into my existing compose), `.env.example` and a `README.md` with the exact
  commands to build, start, stop and test.

## Goal

Given an audio file, return the transcript as timed segments, **each labelled with
who is speaking** (`SPEAKER_00`, `SPEAKER_01`, ...), with accurate word timing where
possible. Podcasts are 10 minutes to 3 hours long. Languages: mostly English, plus
**Hebrew** for some shows, so the language must be selectable per request and the
model must be swappable (see "Configuration").

## Components

1. **Transcription:** `faster-whisper` through the `whisperx` package, on the GPU,
   `float16` (fall back to `int8_float16` if memory is short). Batched inference
   (`batch_size` configurable, default 16).
2. **Voice activity detection:** whatever WhisperX uses by default; keep it on.
3. **Forced alignment** (optional per request, on by default): the language-specific
   wav2vec2 alignment model, for accurate word timestamps. If no alignment model
   exists for the requested language (Hebrew may be missing), **do not fail**: skip
   alignment, keep Whisper's own segment timestamps, and say so in the response
   (`"aligned": false`).
4. **Diarization:** `pyannote/speaker-diarization-3.1` through WhisperX's diarization
   pipeline, using a Hugging Face token. The models are gated: I will accept the
   licences for `pyannote/speaker-diarization-3.1` and `pyannote/segmentation-3.0`
   on huggingface.co and supply the token as `HF_TOKEN`. Assign a speaker to every
   segment and, when words are available, to every word. Accept optional
   `min_speakers`, `max_speakers` and `num_speakers` hints.
5. If diarization fails or no token is set, still return the transcription with
   `"diarized": false` and an explanatory `"warning"`; never lose a finished
   transcription because of a diarization error.

## HTTP API (must match what `pod` already sends)

`POST /inference` — `multipart/form-data`:

| field | meaning |
|---|---|
| `file` | the audio (mp3 or 16 kHz mono WAV; use ffmpeg to decode anything else) |
| `response_format` | always `verbose_json` from `pod`; accept and ignore other values |
| `temperature` | `0.0` from `pod`; accept and pass through |
| `language` | `en`, `he`, ... or `auto`/absent to detect |
| `prompt` | optional initial prompt; pass to Whisper |
| `diarize` | `true`/`false`, default **true** |
| `align` | `true`/`false`, default true |
| `min_speakers`, `max_speakers`, `num_speakers` | optional integers |
| `model` | optional; overrides the default model for this request |

Response `200`, `Content-Type: application/json`:

```json
{
  "text": "full transcript as one string",
  "language": "en",
  "whisper_backend": "whisperx",
  "whisper_model": "large-v3-turbo",
  "aligned": true,
  "diarized": true,
  "speakers": ["SPEAKER_00", "SPEAKER_01"],
  "segments": [
    {
      "start": 0.0, "end": 4.2,
      "text": "Welcome back to the show.",
      "speaker": "SPEAKER_00",
      "words": [{"start": 0.1, "end": 0.4, "word": "Welcome", "speaker": "SPEAKER_00"}]
    }
  ]
}
```

`start`, `end` in seconds (float). `text`, `start`, `end`, `words[].word/start/end`
are already read by `pod`; `speaker`, `speakers`, `aligned`, `diarized` are new and
extra fields must not break it. Errors: a non-200 status with a plain-text or JSON
message; never return HTML.

Other endpoints:

- `GET /health` → `200 {"status":"ok","model":"...","gpu":true}` without loading models.
- Models load **lazily on the first request** and stay resident until the container
  is stopped, so container start is fast and Sablier's wake-up is quick.

## Behaviour requirements

- Requests can run for many minutes (a 3-hour episode). No short server timeouts;
  stream the upload to a temp file instead of holding it in RAM; delete temp files
  afterwards, including on error.
- Process **one job at a time** (a lock); a second request waits rather than
  fighting for GPU memory. Log start, finish, audio duration and elapsed time.
- Free GPU memory between stages (transcription → alignment → diarization) with
  `gc.collect()` and `torch.cuda.empty_cache()`, so the container fits next to other
  GPU workloads. Report the peak GPU memory used in the logs.
- Cache downloaded models in a named volume (`/root/.cache/huggingface` and torch
  hub) so rebuilding the container does not re-download them.

## Configuration (environment variables)

- `WHISPERX_MODEL` — default `large-v3-turbo`. Must accept any faster-whisper model
  name **or a Hugging Face repo id of a CTranslate2 model**, so a Hebrew fine-tune
  (for example one of the ivrit.ai faster-whisper models) can be selected without a
  rebuild. Note in the README which Hebrew model you verified, or say you could not.
- `WHISPERX_COMPUTE_TYPE` (default `float16`), `WHISPERX_BATCH_SIZE` (default 16),
  `WHISPERX_DEVICE` (default `cuda`).
- `HF_TOKEN` — for the pyannote models.
- `PORT` — internal port, default 8000 (what Traefik forwards to).

## Deployment

- **It must use the standard Traefik + Sablier setup, exactly like my existing Whisper
  container.** This is a hard requirement, not a nicety: the container must be
  **stopped when idle and started by the first request**, reached only through Traefik.
  Concretely:
  1. Read how the existing Whisper service is wired (its compose labels, the Traefik
     static and dynamic config, the Sablier container and its version, the shared
     Docker network, the session duration, the middleware name) and reproduce that
     pattern for the new service. Show me what you found before you change anything.
  2. Give the new service the Sablier labels (`sablier.enable=true` and a
     `sablier.group=<name>` of its own) and the Traefik labels for a router and service
     on the same entrypoint, network and TLS/host rules the existing one uses, with the
     same Sablier middleware type but its **own** middleware and group names, its own
     session duration (default 10 minutes idle) and the same waiting strategy
     (dynamic or blocking) as the existing one.
  3. The new service must have **no published host port** if the existing one has none;
     traffic goes through Traefik. If the existing one does publish a port, publish the
     new one on a different port and say so. Use a new hostname or path prefix that
     cannot shadow the existing router; tell me the exact URL `pod` should use.
  4. Add the new router, service and middleware. If they live in a Traefik dynamic
     config file, this is the **one allowed change to existing files**: back the file
     up first (`cp file file.bak-$(date +%F)`), add only the new entries, validate the
     YAML, and confirm Traefik reloaded without dropping the existing routes. If you
     cannot do it without touching more than that, stop and ask me.
  5. Start the new container **stopped** (`docker compose up --no-start`, or the
     Sablier-managed equivalent) so that the first request wakes it, exactly as for the
     existing one.
  Sablier answers with an HTML "waking up" page and status 200 while the container
  starts, which `pod` already knows how to retry, so the app itself needs no special
  wake-up handling. The app must, however, become ready quickly: models load lazily on
  the first real request, and `/health` must answer as soon as the web server is up,
  because Sablier decides the container is ready from its Docker health check. Add a
  Docker `HEALTHCHECK` on `/health` (interval 5s, start period short).
- Use the NVIDIA runtime the same way the existing container does. Tell me which GPU,
  driver and CUDA version you found and confirm the base image matches (a
  `pytorch/pytorch` or `nvidia/cuda` runtime image with a CUDA version the driver
  supports).
- Pin package versions in `requirements.txt` (`whisperx`, `faster-whisper`, `torch`,
  `pyannote.audio`, `fastapi`, `uvicorn`, `python-multipart`). WhisperX and pyannote
  are sensitive to version combinations; use a known-working set and note it.

## Verification (do all of this and show me the output)

1. `docker compose build` succeeds; container starts; `GET /health` is ok.
2. Take a short English clip with two speakers (I'll supply one, or generate a test
   with two different TTS voices if none is available), `POST` it with `curl`, and
   show the JSON: at least two distinct speakers, sensible segment times.
3. Repeat with a short **Hebrew** clip and `language=he`. Report honestly whether
   transcription, alignment (`aligned`) and diarization worked, and the quality you
   observed. If alignment is unavailable for Hebrew, confirm the fallback works.
4. Time a real ~30-minute episode: seconds of processing per minute of audio, peak
   GPU memory, and how that compares with the existing whisper.cpp container on the
   same file (run both, one at a time; do not run them concurrently).
5. Confirm the **existing Whisper container is untouched and still answers** its own
   endpoint after all of this.
6. With the container stopped, send one request through Traefik and show that Sablier
   starts it (the HTML waking page first, then JSON), that the transcription then
   completes, and that after the idle period Sablier stops it again. Show `docker ps`
   before, during and after. Do the same for the existing Whisper container to confirm
   it behaves as before.

## Deliverable

A short report: what you found on the host, what you created and where, the exact
URL to use (`http://<host>:<port>/inference`), the measurements from step 4, and
anything that did not work (especially for Hebrew). Do not claim something works
unless you ran it.
