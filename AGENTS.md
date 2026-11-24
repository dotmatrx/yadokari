# yadokari

A terminal (TUI) client for WaniKani reviews and lessons, in Go with the Charm v2 stack
(Bubble Tea, Lip Gloss, Bubbles, Huh under `charm.land/...` import paths). This file is
for coding agents and doubles as a map of the code for people.

**Planned and deferred work lives in GitHub issues** (`gh issue list`; the "Public
release" milestone is what's next). Add an issue rather than leaving an idea in chat or
in a TODO.

## Commands

Use `just` (no Makefile):

- `just build` — build `bin/yadokari`
- `just install` — static binary (`CGO_ENABLED=0`, version from `git describe`) to
  `~/.local/bin/yadokari`. It runs outside a dev container too (pure-Go SQLite,
  keyring over D-Bus).
- `just run [flags]` — run the app
- `just test [go test flags]` — all tests. Some tests also run
  against the newest local recording in `testdata/recordings/` and skip without one.
- `just check` — gofmt check + `go vet`
- `just mock-recorded` + `just dev [flags]` — serve the newest recording through the
  mock, then run the app against it with a scratch database in `.dev/` (gitignored).
  `just dev --sync` syncs and prints a summary without the TUI.
- `just mock [--fixtures=DIR] [--demo] [--per-page=N]` — fake API on `http://127.0.0.1:8642/v2/`
  (`--demo`: a believable level 8 account, `wkmock.DemoData`)
- `just preview [screen...] [--size=WxH,...] [--html=FILE]` — render screens
  once from demo data (kitty glyphs in a kitty-graphics terminal); `--html` writes a gallery page. Screens:
  `dashboard`, `review`, `review-reading`, `review-wrong`, `summary`, `item`, `item-vocab`, `lesson`. Use it to iterate on
  layout; `--font=NAME|all` draws glyphs in each roulette font; `--theme=NAME|all` each theme; `--field=STYLE` a glyph field style (`ui.FieldStyles`); `--background=light` the light themes on a light pane; `--fixtures=DIR` uses a recording (personal data: never publish that).
- `just icons` — render `docs/images/icon.svg` to PNGs in `docs/images/icons/` (16–1024 px;
  needs `rsvg-convert`, from `librsvg2-tools`). Edit the SVG, then rerun it.
- `just social-card` — render `docs/images/social-card.svg` to the 1280×640 GitHub social
  preview PNG (Inter, JetBrainsMono Nerd Font and Noto Serif CJK JP via fontconfig).
  GitHub has no API for it: upload the PNG by hand in the repo settings.
- `just cross` — build every release target (linux/darwin × amd64/arm64), as CI does
- `just release-check` — `goreleaser check` + a snapshot release into `dist/` (downloads
  GoReleaser; an agent's sandbox may refuse the toolchain download)
- `just probe-mock` — read-only full sync against the running mock
- `just probe-live` — read-only full sync against the live API; records fixtures to
  `testdata/recordings/` (gitignored: contains personal data). Needs a **read-only**
  `WANIKANI_API_TOKEN` in `.env`. Agents don't run it; a person does, with their own
  token.

## CI and releases

- `.github/workflows/ci.yml`: gofmt, vet, `go test -race` and the cross-build on
  Linux (with `fonts-noto-cjk`); macOS tests on tags and by hand while the repo is
  private (10× minutes), on every push once public.
- `.github/dependabot.yml`: one grouped GitHub Actions update PR a month. Go modules
  are updated by hand; Dependabot security updates (a repo setting) cover vulnerable ones.
- `.github/workflows/release.yml`: on a `v*` tag push, tests then GoReleaser
  (`.goreleaser.yaml`): static binaries for linux/darwin × amd64/arm64, tar.gz with
  README, LICENSE and the example config, checksums, a GitHub release, and a Homebrew
  cask in `dotmatrx/homebrew-tap` (skipped without the `HOMEBREW_TAP_TOKEN` secret; the
  cask clears the quarantine flag since the binary is unsigned).
- macOS: terminal ioctls by build tag (`gfx/termios_*.go`), `afplay` for audio, and
  font discovery by folder scan when there's no fontconfig (`glyph/fontscan.go`).
  Windows isn't supported. MIT license.
- Versions: annotated semver tags on `main`; the policy is in `CONTRIBUTING.md`
  ("Versions and releases"). `just build` leaves `bin/yadokari` unstamped (a
  development build); `just install` and releases stamp `git describe`, which shows in
  `--version`, the dashboard header and each `writes.log` entry. `go run` builds say
  `dev`.

## Safety rule: never write to the live API by accident

WaniKani review submissions (`POST /reviews`) and lesson starts
(`PUT /assignments/:id/start`) are **irreversible**. All development runs against the
mock server (`internal/wkmock`, `just dev`).

- **Only builds made to be used write to `api.wanikani.com`:** stamped at build time
  (`just install`, the GoReleaser release builds) or `go install` of a clean release
  tag (`cmd/yadokari`: `buildKind` → `liveWrites` → `wk.WithLiveWrites`). Development
  builds (`go run`, `just run`/`dev`/`build`, tests, a `+dirty` or pseudo-version) can
  never write to the live host, whatever their flags; there is no flag to allow it
  (`--live` was removed on 2026-11-14). Keep it that way: no new path may set
  `WithLiveWrites(true)`.
- **Never run an installed or release binary** (`~/.local/bin/yadokari`, `dist/`, a
  download) yourself, except `--version`/`--help`; the developer's keyring may hold a
  token that can write. Run development builds, against the mock.
- Tokens used during development (`.env`) are read-only.

## Layout

- `cmd/preview` — screen previews and the HTML gallery (`ui.Preview` does the work).
- `cmd/yadokari` — entry point. Flags: `--base`, `--db`, `--sync` (no TUI), `--logout`,
  `--gfx auto|kitty|plain`, `--default-config` (prints `docs/config.example.toml`,
  embedded by the `docs` package), `--terminal` (what the probe found and what was chosen: tier
  and why, cell size, font, background, glyph ink, theme; `setupDisplay` serves both it
  and the app), `--version`. Logs problems to `yadokari.log` next to the
  database (`log/slog`); the UI shows them in plain words via `ui.problem` (no Go
  error text, URLs or IDs on screen; `userError` for messages written for the user).
- `internal/wk` — typed API client: auth, pagination via `next_url` (same-host only),
  ETags, client-side rate limit + `RateLimit-*` handling, 429 retry; GETs retry on 5xx,
  writes never auto-retry; non-GET to the live host fails with `ErrLiveWritesDisabled`
  unless built `WithLiveWrites(true)` (only builds made to be used). `WithWriteLog` appends each write attempt
  (request, status, raw response; never the token) as a JSON line; the app writes it
  to `writes.log` next to the database (`~/.local/share/yadokari/`, or `.dev/`).
- `internal/wkmock` — in-memory fake API (fixtures or `SampleData`), logs all writes,
  approximates SRS stage changes.
- `internal/markup` — WaniKani mnemonic/hint markup (`<radical>`, `<kanji>`, `<reading>`,
  `<ja>`, `<a href>`…) → paragraphs of typed spans. Tolerant of malformed input (a close
  tag closes the innermost open tag). Pure; `ui.renderMarkup` styles and wraps it.
- `internal/audio` — picks a pronunciation recording (`audio.voice` by name, else the WaniKani default voice; primary reading;
  a format the player handles) and plays it with the first external player found
  (`pw-play`, `paplay`, `ffplay`, `mpv`, `afplay`; the static binary can't open a sound device),
  detached from the terminal. In the UI only `App` plays (`playAudioMsg`), showing
  `♪ loading…` / `🔊 playing` in the header (play numbers ignore stale ends); files come
  from the download cache (`~/.cache/yadokari/audio`). A dev container may have no
  player installed (e.g. `pipewire-utils` for `pw-play`).
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
  commented out at its default, which a test keeps in step; `ui.Settings` applies them over the WaniKani preferences;
  precedence flag → file → WaniKani preference → default), and the token store: env
  `WANIKANI_API_TOKEN` → OS keyring → `0600` file fallback. Tests use a fake keyring;
  don't touch the real keyring from tests or tooling without asking the user.
- `internal/ui` — colours only from `theme.go` (`Theme` roles → package styles via
  `SetTheme`; `display.theme` + `[theme]` overrides; add a theme to `ui.Themes` and
  `config.ThemeNames`, and its light version to `lightThemes` (`ForBackground`; a test
  holds every light text colour to 4.5:1 on `#eeeeee`); palette colours are allowed, `InkFor` picks the ink for them).
  Bubble Tea screens under a root `App`, all in one frame (`layout.go`:
  header on the top row via `headerLine` (ranked segments, fits any width; the audio
  badge comes from App's `shared` state), key/status bar on the bottom, rounded `panel`s in ANSI 8
  grey, no painted backgrounds; glyph fields are the terminal background in a half-block
  frame of the type colour by default (`field.go`; `display.field` picks `fill`, the old
  solid block, or another frame)): dashboard (from the store, sync in
  the background via a channel), review (session + IME + answer + gfx; snapshot saved
  after every answer; completions → outbox → `wksync.Uploader`), summary, and the
  first-run token form (Huh), and the item page (`item.go`: glyph + summary, tabs by
  subject type, mnemonics via `renderMarkup`; `i` opens it after a review answer, hiding
  the part not yet asked; lessons reuse it). Review sessions are refused against
  the live API in a development build (`ReviewDeps.Disabled`). Lessons (`lesson.go`): `l` on the dashboard picks a
  batch (the user's batch size and presentation order), steps through each item's page,
  then a quiz that is the review screen in lesson mode (`Review.name == "lessons"`):
  a finished item is queued as a start (`store.QueueStart`), misses cost nothing, and
  the quiz is saved and resumed like reviews. Refused in development builds too.
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
  Font roulette (`roulette.go`): `ListFonts` (fc-list, one face per Japanese family; no
  variable fonts, which sfnt can't read, no SC/TC/KR/Mono), `Pool.Pick` (random, never
  twice in a row, only fonts covering the subject's characters); the review screen picks
  per question (`Review.qfont`) and names the font under the glyph after answering.
- `internal/gfx` — tiers `kitty` (placeholders; image IDs < 256, recycled) and `plain`;
  half-block was tried and dropped (unreadable). `Draw` queues kitty uploads. In the app only
  `ui.App.Update` sends them: after every message it calls the visible screen's
  `DrawGlyph` and sends `Flush()` via `tea.Raw`, once the alt screen is up (App's `ready`).
  Screens never flush; the UI test harness fails if a shown glyph was never sent. `Choose` picks the tier: `--gfx` override, then
  env (`Detect`), then `Probe` (kitty graphics query + `CSI 16 t` + OSC 4/10/11 colours + DA1 on
  `/dev/tty`, before Bubble Tea starts; `ChooseWhy` also says why). The background:
  `ui.DetectBackground` (`display.background` → the terminal's OSC 11 answer →
  `COLORFGBG` → dark); a framed glyph's ink is the config's, else the terminal's text
  colour (OSC 10), else white/near-black; the plain tier leaves it to the terminal. `CellSize` uses the tty ioctl, else the probe: a
  container's pty may report no pixel size (e.g. 12×30 real vs the 10×20 guess). `Source.Mask` must
  return exactly w×h.
- `docs/wanikani-api.md` — API notes and known doc ambiguities.
- `internal/` — real packages. Keep `session`, `answer`, `markup` and `ime`
  free of I/O and UI code, and table-test them heavily.

## Environment notes

- Dev containers (Fedora toolbox, distrobox) often can't see the host's fonts through
  fontconfig; they're under `/run/host/usr/share/fonts/`, so font discovery also checks
  that path. Inside such a container `TERM` may be rewritten (e.g. to `xterm-256color`)
  and the terminal's own variables dropped, so the graphics tier comes from a terminal
  probe, not the environment.
- Terminals: Ghostty, kitty and Rio get the kitty tier (Ghostty is the main one
  tested). VTE terminals (Ptyxis, GNOME Terminal) support neither the kitty
  graphics/text sizing protocols nor sixel, and foot has no kitty graphics, so they
  get the plain tier. WezTerm
  answers the kitty graphics query but has no Unicode placeholders (boxes where the
  glyph should be, the image at the cursor, then a hang), so it gets plain too.
  `gfx.ChooseWhy` asks about placeholders with a kitty feature query (`e=T,U=1`,
  kitty 0.50+); a terminal that doesn't answer one is checked against
  `withoutPlaceholders` by `TERM_PROGRAM` or XTVERSION. Windows Terminal has no kitty
  graphics either, so native Windows would only get the plain tier.
- Sixel is deliberately out of scope for v1: it draws outside the cell grid, which fights
  Bubble Tea's diffing renderer.
- Agent-specific or machine-specific notes (your OS, terminals, containers) belong in
  your agent's own local memory or config, not in this file.
