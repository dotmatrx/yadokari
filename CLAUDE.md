# yadokari

A terminal (TUI) client for WaniKani reviews and lessons, in Go with the Charm v2 stack
(Bubble Tea, Lip Gloss, Bubbles, Huh under `charm.land/...` import paths).

Design doc (Claude Doc): https://claude.ai/artifact/PWpS9zby83Lyos5tgJr7xF

**Start here in a new session:** `docs/roadmap.md` (milestone status, where work was
left, the next plan), then `docs/backlog.md` (deferred ideas). Keep both current.

## Commands

Use `just` (no Makefile):

- `just build` — build `bin/yadokari`
- `just run [flags]` — run the app
- `just test [go test flags]` — all tests except `spikes/`. Some tests also run
  against the newest local recording in `testdata/recordings/` and skip without one.
- `just check` — gofmt check + `go vet`
- `just spike <name>` — run `spikes/<name>`
- `just mock-recorded` + `just dev [flags]` — serve the newest recording through the
  mock, then run the app against it with a scratch database in `.dev/` (gitignored).
  `just dev --sync` syncs and prints a summary without the TUI.
- `just mock [--fixtures=DIR] [--per-page=N]` — fake API on `http://127.0.0.1:8642/v2/`
- `just probe-mock` — read-only full sync against the running mock
- `just probe-live` — read-only full sync against the live API; records fixtures to
  `testdata/recordings/` (gitignored: contains personal data). Needs a **read-only**
  `WANIKANI_API_TOKEN` in `.env`. Only the user runs this.

## Safety rule: never write to the live API by accident

WaniKani review submissions (`POST /reviews`) and lesson starts
(`PUT /assignments/:id/start`) are **irreversible**. All development runs against the
mock server (`internal/wkmock`). Code must not perform a write against
`api.wanikani.com` unless the user passed `--live`; dry-run is the default. Never run
anything with `--live` yourself. Tokens used during development are read-only.

## Layout

- `cmd/yadokari` — entry point. Flags: `--base`, `--db`, `--sync` (no TUI), `--logout`,
  `--live` (writes; never use it yourself).
- `internal/wk` — typed API client: auth, pagination via `next_url` (same-host only),
  ETags, client-side rate limit + `RateLimit-*` handling, 429 retry; GETs retry on 5xx,
  writes never auto-retry; non-GET to the live host fails with `ErrLiveWritesDisabled`
  unless built `WithLiveWrites(true)`.
- `internal/wkmock` — in-memory fake API (fixtures or `SampleData`), logs all writes,
  approximates SRS stage changes.
- `internal/ime` — incremental romaji→kana converter (WanaKana IME-mode behaviour) and
  kana helpers. Pure.
- `internal/store` — SQLite cache: raw API JSON + indexed columns, timestamps as Unix
  microseconds, embedded migrations (`internal/store/migrations/NNN_*.sql`, tracked
  by `PRAGMA user_version`). Dashboard queries live here.
- `internal/wksync` — full then incremental sync (`updated_after` bookmarks, user ETag);
  `Foreground` collections first, `review_statistics` in `Background`. Read-only.
- `internal/config` — XDG paths (config `~/.config/yadokari/config.toml`, database
  `~/.local/share/yadokari/yadokari.db`), settings, and the token store: env
  `WANIKANI_API_TOKEN` → OS keyring → `0600` file fallback. Tests use a fake keyring;
  don't touch the real keyring from tests or tooling without asking the user.
- `internal/ui` — Bubble Tea screens under a root `App`: dashboard (from the store, sync in
  the background via a channel), review (session + IME + answer + gfx; snapshot saved
  after every answer; completions → outbox → `wksync.Uploader`), summary, and the
  first-run token form (Huh). Review sessions are refused against the live API without
  `--live`.
- `internal/session` — review engine: backlog → active set (default 10) → random question; per-part
  misses; `Completion` = what `POST /reviews` needs; wrap-up, limit, ordering, back-to-back,
  undo of a wrong answer; JSON `Snapshot`/`Restore`. Pure.
- `internal/answer` — meaning/reading checks: blacklist → exact (meanings, whitelist,
  synonyms) → warnings → OSA typo tolerance (RadicalGuessr reconstruction of WK; unverified,
  hence `Strictness`). Readings exact after `ToHiragana`; kanji off-type readings warn. Pure.
- `internal/glyph` — font discovery (checks every candidate covers kanji and kana, since
  fontconfig may return a Latin font for `:lang=ja`), text → alpha masks,
  radical SVGs (normalized for oksvg: inline CSS classes, resolve `var()`, drop clip
  paths, scale stroke widths), disk cache for downloads. Radical PNG URLs are dead (403).
- `internal/gfx` — tiers `kitty` (placeholders; image IDs < 256, recycled) and `plain`;
  half-block was tried and dropped (unreadable). `Draw` queues kitty uploads; send
  `Flush()` via `tea.Raw` from Update. `Detect` from env, `CellSize`. `Source.Mask` must
  return exactly w×h.
- `docs/roadmap.md` — milestone status and next steps.
- `docs/wanikani-api.md` — API notes and known doc ambiguities.
- `docs/backlog.md` — deferred ideas; add to it rather than leaving them in chat.
- `internal/` — real packages (still planned: `audio`, lessons). Keep `session`, `answer` and `ime`
  free of I/O and UI code, and table-test them heavily.
- `spikes/` — throwaway experiments and their findings; see `spikes/README.md`

## Environment notes

- Developed in a Fedora toolbox (`yadokari`) on Bluefin. The toolbox's fontconfig can't
  see host fonts (they're under `/run/host/usr/share/fonts/`), so font discovery also
  checks that path. `google-noto-sans-cjk-fonts` is now installed in the toolbox.
- The daily-driver terminal is **Ghostty**, installed on the host (rpm-ostree). Run the
  app from the toolbox inside Ghostty; it gets the kitty tier (detected from
  `TERM=xterm-ghostty`, which toolbox passes through). Verified 2026-11-08: spikes and
  the review screen look right. The toolbox has no `xterm-ghostty` terminfo of its
  own (it's under `/run/host/usr/share/terminfo/`); the app doesn't need it, but shell
  tools may, so copy it to `~/.terminfo/x/` if they complain.
- Ptyxis (VTE 0.84) supports neither the kitty graphics/text sizing protocols nor
  sixel, so it gets the plain tier; it's now only a plain-tier check.
- kitty is installed *inside the toolbox* via dnf and stays a secondary check for the
  kitty tier; it needs the CJK fonts above. Mesa prints harmless NVIDIA probe warnings
  at startup. WezTerm, Rio and foot are installed as flatpaks for later checks.
- Sixel is deliberately out of scope for v1: it draws outside the cell grid, which fights
  Bubble Tea's diffing renderer.
