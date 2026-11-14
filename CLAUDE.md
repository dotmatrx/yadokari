# yadokari

A terminal (TUI) client for WaniKani reviews and lessons, in Go with the Charm v2 stack
(Bubble Tea, Lip Gloss, Bubbles, Huh under `charm.land/...` import paths).

Design doc (Claude Doc): https://claude.ai/artifact/PWpS9zby83Lyos5tgJr7xF

**Start here in a new session:** `docs/roadmap.md` (milestone status, where work was
left, the next plan), then `docs/backlog.md` (deferred ideas). Keep both current.

## Commands

Use `just` (no Makefile):

- `just build` — build `bin/yadokari`
- `just install` — static binary (`CGO_ENABLED=0`, version from `git describe`) to
  `~/.local/bin/yadokari`, which the host and the toolbox share; it runs on the host too
  (pure-Go SQLite, keyring over D-Bus).
- `just run [flags]` — run the app
- `just test [go test flags]` — all tests except `spikes/`. Some tests also run
  against the newest local recording in `testdata/recordings/` and skip without one.
- `just check` — gofmt check + `go vet`
- `just spike <name>` — run `spikes/<name>`
- `just mock-recorded` + `just dev [flags]` — serve the newest recording through the
  mock, then run the app against it with a scratch database in `.dev/` (gitignored).
  `just dev --sync` syncs and prints a summary without the TUI.
- `just mock [--fixtures=DIR] [--demo] [--per-page=N]` — fake API on `http://127.0.0.1:8642/v2/`
  (`--demo`: a believable level 8 account, `wkmock.DemoData`)
- `just preview [screen...] [--size=WxH,...] [--html=FILE]` — render screens
  once from demo data (kitty glyphs in Ghostty); `--html` writes a gallery page. Screens:
  `dashboard`, `review`, `review-reading`, `review-wrong`, `summary`, `item`, `item-vocab`, `lesson`. Use it to iterate on
  layout; `--fixtures=DIR` uses a recording (personal data: never publish that).
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

- `cmd/preview` — screen previews and the HTML gallery (`ui.Preview` does the work).
- `cmd/yadokari` — entry point. Flags: `--base`, `--db`, `--sync` (no TUI), `--logout`,
  `--gfx auto|kitty|plain`,
  `--live` (writes; never use it yourself). Logs problems to `yadokari.log` next to the
  database (`log/slog`); the UI shows them in plain words via `ui.problem` (no Go
  error text, URLs or IDs on screen; `userError` for messages written for the user).
- `internal/wk` — typed API client: auth, pagination via `next_url` (same-host only),
  ETags, client-side rate limit + `RateLimit-*` handling, 429 retry; GETs retry on 5xx,
  writes never auto-retry; non-GET to the live host fails with `ErrLiveWritesDisabled`
  unless built `WithLiveWrites(true)`. `WithWriteLog` appends each write attempt
  (request, status, raw response; never the token) as a JSON line; the app writes it
  to `writes.log` next to the database (`~/.local/share/yadokari/`, or `.dev/`).
- `internal/wkmock` — in-memory fake API (fixtures or `SampleData`), logs all writes,
  approximates SRS stage changes.
- `internal/markup` — WaniKani mnemonic/hint markup (`<radical>`, `<kanji>`, `<reading>`,
  `<ja>`, `<a href>`…) → paragraphs of typed spans. Tolerant of malformed input (a close
  tag closes the innermost open tag). Pure; `ui.renderMarkup` styles and wraps it.
- `internal/audio` — picks a pronunciation recording (default voice, primary reading,
  a format the player handles) and plays it with the first external player found
  (`pw-play`, `paplay`, `ffplay`, `mpv`; the static binary can't open a sound device),
  detached from the terminal. In the UI only `App` plays (`playAudioMsg`), showing
  `♪ loading…` / `🔊 playing` in the header (play numbers ignore stale ends); files come
  from the download cache (`~/.cache/yadokari/audio`). The toolbox has no player unless
  `pipewire-utils` is installed in it; the host has `pw-play`.
- `internal/ime` — incremental romaji→kana converter (WanaKana IME-mode behaviour) and
  kana helpers. Pure.
- `internal/store` — SQLite cache: raw API JSON + indexed columns, timestamps as Unix
  microseconds, embedded migrations (`internal/store/migrations/NNN_*.sql`, tracked
  by `PRAGMA user_version`). Dashboard queries live here.
- `internal/wksync` — full then incremental sync (`updated_after` bookmarks, user ETag);
  `Foreground` collections first, `review_statistics` in `Background`. Read-only;
  `Uploader` (outbox → `POST /reviews` or `PUT /assignments/:id/start` by row `kind`,
  serialized) is the one writer. The dashboard
  and `--sync` flush the outbox before syncing.
- `internal/config` — XDG paths (config `~/.config/yadokari/config.toml`, database
  `~/.local/share/yadokari/yadokari.db`), settings (`[reviews]`, `[lessons]`, `[audio]`,
  `[display]`, validated on load; every key documented in `docs/config.example.toml`,
  which a test keeps in step; `ui.Settings` applies them over the WaniKani preferences;
  precedence flag → file → WaniKani preference → default), and the token store: env
  `WANIKANI_API_TOKEN` → OS keyring → `0600` file fallback. Tests use a fake keyring;
  don't touch the real keyring from tests or tooling without asking the user.
- `internal/ui` — Bubble Tea screens under a root `App`, all in one frame (`layout.go`:
  header on the top row via `headerLine` (ranked segments, fits any width; the audio
  badge comes from App's `shared` state), key/status bar on the bottom, rounded `panel`s in ANSI 8
  grey, no painted backgrounds except the review glyph field): dashboard (from the store, sync in
  the background via a channel), review (session + IME + answer + gfx; snapshot saved
  after every answer; completions → outbox → `wksync.Uploader`), summary, and the
  first-run token form (Huh), and the item page (`item.go`: glyph + summary, tabs by
  subject type, mnemonics via `renderMarkup`; `i` opens it after a review answer, hiding
  the part not yet asked; lessons will reuse it). Review sessions are refused against
  the live API without `--live`. Lessons (`lesson.go`): `l` on the dashboard picks a
  batch (the user's batch size and presentation order), steps through each item's page,
  then a quiz that is the review screen in lesson mode (`Review.name == "lessons"`):
  a finished item is queued as a start (`store.QueueStart`), misses cost nothing, and
  the quiz is saved and resumed like reviews. Refused without `--live` too.
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
  half-block was tried and dropped (unreadable). `Draw` queues kitty uploads. In the app only
  `ui.App.Update` sends them: after every message it calls the visible screen's
  `DrawGlyph` and sends `Flush()` via `tea.Raw`, once the alt screen is up (App's `ready`).
  Screens never flush; the UI test harness fails if a shown glyph was never sent. `Choose` picks the tier: `--gfx` override, then
  env (`Detect`), then `Probe` (kitty graphics query + `CSI 16 t` + DA1 on `/dev/tty`,
  before Bubble Tea starts). `CellSize` uses the tty ioctl, else the probe: the
  toolbox pty reports no pixel size (Ghostty: 12×30 real vs the 10×20 guess). `Source.Mask` must
  return exactly w×h.
- `docs/roadmap.md` — milestone status and next steps.
- `docs/wanikani-api.md` — API notes and known doc ambiguities.
- `docs/backlog.md` — deferred ideas; add to it rather than leaving them in chat.
- `internal/` — real packages Keep `session`, `answer`, `markup` and `ime`
  free of I/O and UI code, and table-test them heavily.
- `spikes/` — throwaway experiments and their findings; see `spikes/README.md`

## Environment notes

- Developed in a Fedora toolbox (`yadokari`) on Bluefin. The toolbox's fontconfig can't
  see host fonts (they're under `/run/host/usr/share/fonts/`), so font discovery also
  checks that path. `google-noto-sans-cjk-fonts` is now installed in the toolbox.
- The daily-driver terminal is **Ghostty**, installed on the host (rpm-ostree). Run the
  app from the toolbox inside Ghostty; it gets the kitty tier. Inside the toolbox
  `TERM` is `xterm-256color` and no Ghostty variables are set (toolbox seems to
  replace a `TERM` whose terminfo the container lacks; `xterm-ghostty`'s is only under
  `/run/host/usr/share/terminfo/`), so the tier comes from a terminal probe, not the
  environment. Ghostty passed Spike 1 and the tier spike on 2026-11-08.
- Ptyxis (VTE 0.84) supports neither the kitty graphics/text sizing protocols nor
  sixel, so it gets the plain tier; it's now only a plain-tier check.
- kitty is installed *inside the toolbox* via dnf and stays a secondary check for the
  kitty tier; it needs the CJK fonts above. Mesa prints harmless NVIDIA probe warnings
  at startup. WezTerm, Rio and foot are installed as flatpaks for later checks.
- Sixel is deliberately out of scope for v1: it draws outside the cell grid, which fights
  Bubble Tea's diffing renderer.
