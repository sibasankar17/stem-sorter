# Stem Sorter → Multi-Tenant Platform

**Plan to convert the single-file browser app into a Go + React, single-binary, multi-tenant, encrypted music analysis platform.**

---

## 1. Guiding principles

- **Split by strength.** The browser is good at *real-time* audio (playback, the AnalyserNode-driven visualizers, the FX/EQ graph). Go is good at *batch, heavy, persistent* work (ingest, decode, DSP analysis, encryption, storage, serving). Keep playback in the UI; move everything else to Go. Do **not** move playback into Go — it would discard the working Web Audio visualizers and FX chain for no gain.
- **The server holds plaintext only transiently.** Audio is encrypted at rest; it is decrypted into memory only during ingest (to analyze) and during playback (to stream). This is "encrypted at rest, plaintext transiently in RAM" — *not* end-to-end zero-knowledge. See §5 for the unavoidable tradeoff.
- **Storage is pluggable per tenant.** A single `Store` interface with two backends: shared Postgres (RLS) and SQLite-per-tenant.
- **Heavy work is a queue, not a request handler.** Scanning ~800+ files happens on a worker pool fed by a persistent job queue, never on an HTTP goroutine.
- **Preserve the brains, replace the shell.** The DSP, the zone/multi-bucket model, visualizers, FX/EQ, and UI logic carry over. What changes is decode, storage, packaging, auth, and serving.

---

## 2. Topology

One Go binary containing:

- HTTP/JSON API + WebSocket/SSE for progress.
- The compiled React build, served from `embed.FS`.
- The analysis worker pool, in-process.
- Pluggable storage + encryption layers.

The browser keeps playback, visualizers, and FX/EQ, played over an **authenticated HTTP range-request stream** from the server.

```
┌────────────────────────── Single Go binary ──────────────────────────┐
│  HTTP API  ·  embedded React (embed.FS)  ·  WS/SSE progress           │
│  Auth (Ed25519 challenge-response)  ·  Session/DEK manager            │
│  Job queue → worker pool (NumCPU, per-tenant fair scheduling)         │
│     ffmpeg→PCM  ·  gonum FFT/key/tempo/zones  ·  dhowden/tag + art    │
│  Store interface ── Postgres (RLS)  |  SQLite-per-tenant              │
│  Encrypted blob store (audio + features), streaming AEAD             │
└───────────────────────────────────────────────────────────────────────┘
        ▲ range stream / API                         ▲ WS progress
        │                                            │
┌───────┴───────────── Browser (React) ──────────────┴──────────────────┐
│  Playback (<audio>/Web Audio)  ·  Visualizers  ·  FX/EQ                │
│  Board / grouping / multi-bucket · Now-playing · Queue · Cues         │
│  Passphrase-encrypted private key (never leaves client)               │
└───────────────────────────────────────────────────────────────────────┘
```

---

## 3. The Go core

| Concern | Approach | Notes |
|---|---|---|
| Decode | Shell to **ffmpeg** (`os/exec` → PCM) | Library is 320k AAC/m4a; pure-Go decoders handle it poorly. Removes the browser's ~1 GB decode-memory ceiling hit at 762 files. |
| Analysis | Port DSP to Go using `gonum/dsp/fourier` | FFT, Krumhansl key, tempo (onset autocorrelation), texture, zones / multi-bucket. |
| Concurrency | Goroutine pool over a buffered channel, sized to `runtime.NumCPU()` | Replaces the entire hand-built Web Worker pool + decode-decoupling + ready-buffer machinery. Stream PCM in chunks and discard → no giant retained buffers. |
| Tags + art | `github.com/dhowden/tag` | Reads ID3 / MP4 / FLAC / Vorbis tags **and** embedded cover art — replaces the hand-written JS parser. |
| Filesystem | Native scan + `fsnotify` watch | Live add/remove; persistent across sessions — eliminates the "↺ re-add to play" friction entirely. |
| Audio serving | Localhost endpoint with HTTP **range** support | React Web Audio plays from it and runs visualizers/FX on top. |

---

## 4. Multi-tenancy & storage

A single backend-neutral `Store` interface that the pipeline and API depend on, selected by config. **Constraint:** no query may assume cross-tenant visibility (SQLite tenants are physically separate).

### Backend A — Shared Postgres
- One schema; `tenant_id` on every row.
- Isolation enforced by **row-level security** (`SET app.tenant_id` per connection), so isolation lives in the DB, not just app code.
- `pgx` connection pool. Migrations run once.
- Best for **many small tenants** and any cross-tenant/admin operations.

### Backend B — SQLite-per-tenant
- One file per tenant via `modernc.org/sqlite` (pure Go, no cgo → trivial cross-compile), **WAL** mode.
- LRU `map[tenantID]*sql.DB` with idle-close to avoid exhausting file handles at scale.
- Migrations run lazily on first open of each file.
- Strong **physical isolation**, easy per-tenant backup/export, no noisy-neighbor.

### Tenant resolution
Decide one: **subdomain** (`tenant.host`), **path prefix** (`/t/{tenant}`), or **token claim**. Affects routing, cookies/CORS, and TLS.

---

## 5. Identity, auth & encryption

### Per-user keypair (private key + passphrase)
- **Ed25519** identity keypair per user.
- The **private key never leaves the client**. Stored passphrase-encrypted: `argon2id(passphrase)` → key → **XChaCha20-Poly1305** wrap (e.g. libsodium-wasm in the browser).
- **Login = challenge-response:** server sends a nonce → client unlocks the private key with the passphrase and signs → server verifies against the registered public key → issues a short-lived session token (**PASETO v4 / EdDSA**).
- No passwords on the wire; none stored server-side.

### Encryption at rest
- Each tenant has a random **data key (DEK)**; sensitive blobs (audio, metadata, features) are encrypted with it.
- The DEK is **wrapped** by either a passphrase-derived KEK or sealed to the user's public key, so the server cannot read tenant data at rest without an active session.
- Seekable playback: store encrypted audio as **streaming AEAD** (per-chunk nonces — e.g. Tink/age STREAM) so a range request decrypts only the requested chunks.

### The unavoidable tradeoff
**Server-side analysis requires plaintext.** You cannot have true zero-knowledge *and* have Go analyze the audio. The workable model is **analyze-on-ingest**: decode + extract features while plaintext is transiently in memory, then persist encrypted audio + encrypted features; the DEK lives in session memory only during ingest and playback. A consequence: **search/grouping over features requires an unlocked session** (the features are encrypted too). Communicate this honestly to users — it is not E2E.

---

## 6. Analysis pipeline

- **Persistent job queue** (survives restarts) — never scan a folder on a request goroutine.
- Worker pool sized to `NumCPU`, with **per-tenant fair scheduling** so one big import can't starve other tenants.
- Per job: `ffmpeg → PCM (chunked)` → DSP features (key/tempo/zones/multi-bucket) + `dhowden/tag` (tags + art) → encrypt → `Store`.
- Progress (the ETA / throughput / "N analyzing · M waiting" we built) streamed to the browser via **WebSocket/SSE**.

---

## 7. Carried over vs. rewritten

**Rewritten in Go:** decode, DSP analysis, tag/art parsing, storage, filesystem scan, the processing pipeline, encryption.

**Ported to React largely intact:** the board, grouping + **multi-bucket** model, now-playing, up-next queue, hot cues, playlist export, keyboard shortcuts, Media Session, and — kept in **Web Audio** — the visualizers and FX/EQ.

---

## 8. Delivery phases

1. **Skeleton.** Go module; embedded React; `Store` interface + **both** backends with migrations; tenant routing; config. **Prove encrypted range playback through the stream early** — it's the riskiest integration.
2. **Auth / crypto.** Keypair registration; passphrase-encrypted key in the browser; challenge-response; session tokens; DEK wrap/unwrap; encrypted-at-rest blobs.
3. **Ingest + DSP port.** Queue; worker pool; ffmpeg; tags; analyze-on-ingest writing encrypted features. **QA lever:** the exported CSVs are *golden fixtures* — assert Go output matches the JS within tolerance (expect minor float/FFT drift; relative major/minor ambiguity remains inherent).
4. **React UI.** Port board / grouping / multi-bucket / now-playing / queue / cues; keep visualizers + FX in Web Audio; wire scan progress.
5. **Productionize.** Virtualized list rendering (the per-column cap was a stopgap); ffmpeg-based **quality scan** (spectral ceiling → transcode flag, true-peak, loudness grade); per-tenant rate-limiting/quotas; backups; TLS / reverse-proxy; signed cross-platform builds; CI.

---

## 9. Decisions to lock before coding

- **ffmpeg:** host dependency vs. embed-and-extract at first run (true single binary, but LGPL/GPL implications).
- **Tenant resolution:** subdomain vs. path prefix vs. token claim.
- **DEK wrapping:** passphrase-derived KEK vs. public-key-sealed; and exactly where the DEK lives during a session.
- **Encrypted features:** confirm that search/grouping requiring an unlocked session is acceptable (encryption says it must).
- **DSP parity tolerance** against the CSV fixtures.
- **Postgres driver/SQLite:** pure-Go throughout (no cgo) to keep cross-compilation simple.

---

## 10. Suggested first step

Scaffold Phase 1: a Go module with the `Store` interface, **both** Postgres-RLS and SQLite-per-tenant implementations, tenant routing, and a stubbed **encrypted range-streaming** endpoint — the parts most expensive to get wrong later — plus the embedded-React wiring.
