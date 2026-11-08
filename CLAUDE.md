# yadokari

A terminal (TUI) client for WaniKani reviews and lessons, in Go with the Charm v2 stack
(Bubble Tea, Lip Gloss, Bubbles, Huh under `charm.land/...` import paths).

Design doc (Claude Doc): https://claude.ai/artifact/PWpS9zby83Lyos5tgJr7xF

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
mock server (`internal/wkmock`, Phase 0.2). Code must not perform a write against
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
- `internal/ui` — Bubble Tea screens: dashboard (from the store, sync in the
  background via a channel) and the first-run token form (Huh).
- `docs/wanikani-api.md` — API notes and known doc ambiguities.
- `internal/` — real packages (still planned: `session`, `answer`, `glyph`, `gfx`,
  `audio`). Keep `session`, `answer` and `ime`
  free of I/O and UI code, and table-test them heavily.
- `spikes/` — throwaway Phase 0 experiments; see `spikes/README.md`

## Environment notes

- Developed in a Fedora toolbox on Bluefin. The toolbox has no fontconfig; host fonts
  (including Noto Sans CJK JP) are under `/run/host/usr/share/fonts/`. Font discovery
  must fall back to that path.
- The default terminal is Ptyxis (VTE 0.84). It supports neither the kitty graphics/text
  sizing protocols nor sixel, so it gets the half-block tier (C).
- Graphics tiers A/B are developed in kitty installed *inside the toolbox* via dnf
  (avoids flatpak sandboxing). WezTerm, Rio and foot are installed as flatpaks for later
  compatibility checks.
- Sixel is deliberately out of scope for v1: it draws outside the cell grid, which fights
  Bubble Tea's diffing renderer.
