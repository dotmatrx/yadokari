# Roadmap and status

Where the project stands and what comes next. The design rationale lives in the design
doc (link in CLAUDE.md); deferred ideas live in `docs/backlog.md`. Update this file when
a milestone moves.

**Last updated:** 2026-11-22 (moved to `dotmatrx`).

## Milestones

| Milestone | Status | Notes |
|---|---|---|
| Phase 0: spikes and safety net | Done | API client + mock, kitty placeholders (pass), IME (pass); OSC 66 skipped; findings in the design doc |
| M1: read-only dashboard | Done | Store, incremental sync, token (keyring works in the toolbox), dashboard; verified on the live account |
| M2: reviews against the mock | Done | Session engine, answer checking, glyph/gfx (kitty + plain), outbox + uploader, review and summary screens |
| M3: live reviews | Done | Two weeks of daily use to 2026-11-21: "no issues with the features or functionality" (the user) |
| UI pass | Done | Panels frame on every screen; approved 2026-11-11 |
| M4: lessons | Done (v0.4.0) | Item pages with mnemonic markup, lessons + quiz, `PUT /assignments/:id/start`, audio; live-verified 2026-11-11 |
| M5: polish | v0.5.0 (2026-11-14); packaging built | Config file, font roulette, themes (yadokari palette on trial); packaging + CI built 2026-11-14, first release not cut yet; add synonym deferred to the distribution decision |

## Versions

Semver tags (`vMAJOR.MINOR.PATCH`, annotated) on `main`, starting at v0.3.0 on
2026-11-10. While on 0.x, the minor version follows the milestone (0.3 = M3, 0.4 = M4
lessons, 0.5 = M5 polish); patches are fixes found while dogfooding. Tag when installing
a build you'll use for a while, not every commit. v1.0.0 when it's ready to hand to
someone else (after M4 and M5's packaging). `just build`/`just install` stamp
`git describe` (e.g. `v0.3.0-2-gabc1234-dirty` between tags) into the binary: shown by
`--version`, in the dashboard header and in each `writes.log` entry. `go run` builds
(`just run`, `just dev`) say `dev`.

## Release prep (started 2026-11-21)

The user's plan, in order:
1. **Icon and README** (this step). Icon chosen 2026-11-21: 宀, the roof radical at
   the top of 宿, over a prompt and an orange block cursor, on a black tile
   (`docs/images/icon.svg`; all the concepts are on the icon canvas,
   https://claude.ai/artifact/FApsYBJ1jA7wugc6XjG6WV). README has a centred icon header.
   Screenshot (`docs/images/review.png`, 2026-11-21): the review screen in Ghostty
   with demo data (`just mock --demo`, `just dev`). Retake it the same way when the
   screen changes; never from a recording (personal data). `just icons` renders the
   icon as PNGs (`docs/images/icons/`). **Left:** a 1280×640 social preview card for
   GitHub (icon, name, tagline), set by hand in the repo settings once public.
2. **Move the repo to the `dotmatrx` organization**, then make it public. Moved
   2026-11-22 (still private); the Go module path is now `github.com/dotmatrx/yadokari`
   and the Homebrew tap is `dotmatrx/homebrew-tap` (`brew install dotmatrx/tap/yadokari`).
   **Left:** tag a release after the rename (v0.5.2 has the old module path, so
   `go install …@v0.5.2` fails from the new one; main also has the macOS font fix);
   create the tap repo and the `HOMEBREW_TAP_TOKEN` secret under `dotmatrx`; decide on the commit email
   (every commit has the personal address) and the private claude.ai links in the
   docs; `SECURITY.md`/`CONTRIBUTING.md`. Once public: macOS CI on every push, CI
   badges in the README, repo description/topics/social card, pkg.go.dev.
   **History pass done 2026-11-22** (all 83 commits, deleted files included): no
   token, no recording, database or log ever committed; none of the account's
   username, user ID, assignment/review-statistic/study-material IDs or synonyms
   appear anywhere. Left as known: the author email on every commit, the
   screenshot's PNG metadata (creation time with time zone), the dogfooding notes
   in the docs (environment, live review counts), and the private claude.ai links.
3. Installation: Homebrew (built), Linux packages (backlog), then a website, likely
   on Cloudflare Pages.

## Where things were left (2026-11-11)

- All work is committed on `main`; the user pushes to origin themselves.
- **M3 dogfooding:** 55 live reviews over 2026-11-08 to 11-10, all 201. Still unseen:
  a 422 body and a real network-down run; we watch for them rather than wait (every
  write is in `writes.log`). The keyring token has full write permissions, including
  `assignments:start` for lessons.
- **UI pass done:** the user approved round 3 on 2026-11-11.
- **Design doc:** the half-block suggestions (five edits) are accepted; none pending.
- **M4 done, tagged v0.4.0 (2026-11-11):** markup renderer, item page, lessons (first
  live batch of 5, all accepted), audio with a loading/playing badge. Glyph uploads go
  through one place (`App.Update`), enforced by the test harness; headers through
  `headerLine`. Optional: a window-resize check in Ghostty. The item page's empty space
  is left as is until lessons have been used more.
- **M3 dogfooding** continues to about 2026-11-22. Offline run done 2026-11-14 (live):
  reviews queued while offline, the head row retried, all sent exactly once after
  reconnecting. The user's feedback: on-screen errors were raw Go errors (URLs, IDs);
  fixed the same day — plain words on screen, details in `yadokari.log`.
- **M5 started 2026-11-14:** step 1, the config file, is done (plan below). Add synonym
  is deferred until deciding whether to distribute (backlog). Font roulette, themes,
  packaging remain; the
  backlog holds the candidates.
- Daily driver: Ghostty on the host, app from the `yadokari` toolbox (kitty tier via
  `gfx.Probe`); `just install` puts a static binary in `~/.local/bin`.
- `.dev/yadokari.db` (gitignored) may hold a saved review session; `r` resumes it.
- A read-only API token is in `.env` (used by `just probe-live`); the user's real
  token is in the desktop keyring.

## UI pass (started 2026-11-08)

The user finds the screens sparse and "too fluid" and wants a more TUI feel: borders,
a grid that uses the full width. Their direction: **calm** (few panels, generous
space, thin borders), **colour only where it means something** (type marks, bars),
**no painted backgrounds** (they use a translucent terminal).

- Workflow: `just preview` renders screens once (real Lip Gloss output, kitty glyphs in
  Ghostty); `--html` turns them into a gallery published as an artifact for comments:
  https://claude.ai/artifact/5tc3ofXbRg3m2gtBs3rztb (demo data only).
- Round 1 (2026-11-08): two dashboard directions next to the current one. **A,
  panels**: rounded boxes with titles in the border, Now | Level, forecast, SRS stages.
  **B, rules**: no boxes, titled full-width rules. Both pin the header to the top row
  and the status/key bar to the bottom, and draw lines in ANSI 8 (theme grey).
  The user chose **panels** (checked in Ghostty too); rules and the old renderer are
  gone.
- Round 2 (2026-11-08): the same frame on every screen. Review: header (progress,
  accuracy, uploads) and key hints pinned; the glyph field (still type-coloured, the
  one painted background) takes half the height; the answer is a centred panel titled
  with the question ("Kanji reading") whose border turns green/red with ✓/✗; the
  answer panel below matches its width. Summary: panels of items in aligned columns
  with SRS movement right-aligned. Every screen is tested to fill the terminal exactly
  at 120×36, 80×24 and 60×20.
- Round 3 (2026-11-08): the input mode moved out of the answer box (a stray-looking
  "A"/"あ") into the right of its top border as a small grey badge plus a label
  (`[A] English`, `[あ] Kana`); an empty box shows a placeholder (答え / "Your
  answer"), as on WaniKani's site.
- Approved 2026-11-11; the lesson screens reuse this frame.

## M3 plan: live reviews (agreed 2026-11-08)

1. Mechanically, `--live` already works: it enables live writes in the client and lifts
   the dashboard's review block. M3 is about using it safely, not building it.
2. **Decided: one token.** The keyring holds a token with `reviews:create`, used for
   everything by the real app; live writes stay blocked unless `--live` is passed.
   `.env` keeps the read-only token for `probe-live` and anything Claude runs. Next:
   the user swapped the keyring token on 2026-11-08 (a full-write token).
3. ~~Confirm the open write behaviour against the live API.~~ Done 2026-11-08: first
   `--live` session, 11 reviews, all 201; findings in `docs/wanikani-api.md`. Still
   unseen: a 422 body. Every write is logged to `~/.local/share/yadokari/writes.log`.
4. ~~Surface rejected (failed) outbox rows on the dashboard.~~ Done 2026-11-08.
5. ~~Offline handling.~~ Done 2026-11-08: the dashboard sends queued reviews before
   every sync (startup and `s`), `--sync` does too, and flushes are serialized so the
   dashboard and review screen can't post a row twice. Tested offline → online
   against the mock; a real network-down run is part of dogfooding.
6. Dogfood daily for two weeks next to the website. ~~Summary SRS movement~~ done
   2026-11-08 (from the stored assignment that `resources_updated` returned).

## M4 plan: lessons (started 2026-11-11)

Groundwork exists: `wk.Client.StartAssignment` and the mock's
`PUT /assignments/:id/start`. Build against the mock; lessons are refused against the
live API without `--live`, as reviews are.

1. ~~**WaniKani markup renderer**~~ Done 2026-11-11: `internal/markup` (parse, pure)
   + `ui.renderMarkup` (type colours, bold readings/meanings, italic emphasis, OSC 8
   links, wrapped). Parses all 22,525 recorded mnemonics and hints. Not shown anywhere
   yet; first use is the item page, and it unlocks mnemonics in the review answer
   panel (backlog).
2. ~~Item page~~ Done 2026-11-11: `ui.ItemPage` (glyph + summary panel; tabs by type:
   radical Name/Found in, kanji Radicals/Meaning/Reading/Found in, vocabulary
   Kanji/Meaning/Reading/Context; scrolls). Opened with `i` after a review answer; the
   part not yet asked stays hidden. Previews: `item`, `item-vocab`.
3. ~~Lesson flow~~ Done 2026-11-11 (against the mock): `l` → batch (WaniKani batch size
   and presentation order) → item pages (←/→ walk tabs across items; Enter at the end
   starts the quiz) → quiz (review screen in lesson mode; all items active; misses
   free) → each finished item queued as a start in the outbox (migration 003 adds
   `kind`), sent as `PUT /assignments/:id/start` with `started_at` = when it was
   finished → summary ("Learned"). Quiz saved after each answer and resumed by `l`.
   Next: the user tries it on the mock (`just mock --demo` + `just dev`), then a first
   live lesson batch (irreversible; the user runs it).
4. ~~Audio~~ Done 2026-11-11: `internal/audio` + `p` on item pages, lesson pages and
   after a graded answer (not while the reading is hidden); autoplay per the user's
   WaniKani settings (correct reading in reviews/lesson quiz; arriving on a lesson's
   Reading tab). Heard on the real system 2026-11-11 (toolbox needed `pipewire-utils`).
   Header badge `♪ loading…` / `🔊 playing` (2026-11-11), after moving every screen's
   header onto one `headerLine` helper.
5. ~~Tag v0.4.0~~ Done 2026-11-11.

## M5 plan, step 1: config file (done 2026-11-14)

Expose settings that already exist in code through `~/.config/yadokari/config.toml`.
No settings screen yet; the file is the interface. Nothing new is built except the undo
key and a player override.

**Precedence:** flag → config file → WaniKani preference (where one exists) → built-in
default. A key that mirrors a WaniKani preference defaults to `"wanikani"` (or unset),
so an empty file behaves exactly like today.

```toml
api_base_url = ""            # exists today

[reviews]
order = "wanikani"           # wanikani | shuffled | lower_levels_first | lower_stages_first | by_type
active_size = 10             # items in play at once
limit = 0                    # max items per session; 0 = all
back_to_back = false         # an item's meaning and reading consecutively
reading_first = false        # with back_to_back
strictness = "wanikani"      # meaning typos: wanikani | strict | lenient
allow_undo = false           # a key to retype after a wrong answer (see below)
# autoplay_audio = true      # unset: WaniKani's reviews_autoplay_audio

[lessons]
batch_size = 0               # 0: WaniKani's lessons_batch_size
order = "wanikani"           # wanikani | ascending_level_then_shuffled | shuffled | ascending_level_then_subject
# autoplay_audio = true      # unset: WaniKani's lessons_autoplay_audio

[audio]
player = ""                  # command line, e.g. "mpv --no-video"; empty: first of pw-play, paplay, ffplay, mpv

[display]
gfx = "auto"                 # auto | kitty | plain; --gfx overrides
font = ""                    # path to a Japanese font; empty: fontconfig, then known paths
```

Wiring (no new dependencies; BurntSushi/toml is already used):

1. `internal/config`: the sections above as typed structs; `*bool` for "unset follows
   WaniKani". `Load` validates enum values and ranges and names the bad key and the
   allowed values. Unknown keys stay an error, as today.
2. `cmd/yadokari`: map the config to `ui.ReviewDeps` (session options, strictness,
   autoplay overrides, lesson batch/order), `gfx.Choose` (flag still wins),
   `glyph.FindFont(font)`, and `audio.New` for a player override (the content types
   come from the command name, falling back to MPEG).
3. `internal/ui`: `LoadReview` / `pickLessons` / autoplay read the resolved settings
   instead of `wk.Preferences` directly. Resolve once in one function, table-tested
   for precedence.
4. Undo (`allow_undo`): `session.Undo` exists; add a key on a graded wrong answer
   that puts the question back with the typed answer for editing. The key hint shows
   only when it's on.
5. `docs/config.example.toml`: every key, commented, with its default. A test decodes
   it so it can't drift from the code. `yadokari --help` points to it.
6. A setting changes the next session, not a saved one: a resumed session keeps the
   options in its snapshot.

Built as planned. Undo is Backspace on a graded wrong answer ("Backspace retype" in
the key hints); the app never writes a config file itself; `--help` points to the
example. Verified live 2026-11-14 (v0.4.0-3): order, batch size and undo overrides
behaved; an undone reading miss (市町村) went up as `incorrect_reading_answers: 0`.

Not in this step: the settings screen, lightning mode and relaxed `n` (new features;
they become keys when built), voice choice (later).

## M5 plan, step 2: font roulette (built 2026-11-14)

From the design doc: draw each review glyph in a randomly chosen Japanese typeface
(gothic, mincho, textbook, handwritten) so recognition survives real-world typography.
Kitty tier only; the plain tier draws in the terminal's own font, so the setting does
nothing there.

**Findings (2026-11-14):**
- `x/image/font/sfnt` can't read variable fonts (CFF2): the host's only mincho,
  `NotoSerifCJK-VF.ttc`, and `NotoSansCJK-VF.ttc` fail the kanji/kana coverage check.
  Static fonts work (all seven Noto Sans CJK weights load and draw).
- Fedora packages static fonts: `google-noto-serif-cjk-fonts` (mincho),
  `ipa-exmincho-fonts`, `ipa-exgothic-fonts`, `horai-ume-mincho-fonts`. Textbook and
  handwritten styles (e.g. Klee One, Yomogi; OFL, Google Fonts) aren't packaged: they go
  in `~/.local/share/fonts`, which the host and the toolbox share.

**Proposed:**

```toml
[display]
roulette = false        # a random Japanese font for each review question
roulette_fonts = []     # font files; empty: every installed Japanese font, one per family
```

1. `internal/glyph`: `FindFonts()` lists installed Japanese fonts (`fc-list :lang=ja`
   plus the `/run/host` and `~/.local/share/fonts` paths fontconfig may miss), keeps one
   face per family (Regular, else the first one found), and drops families
   that aren't Japanese (`… SC/TC/HK/KR` members, Mono) so no kanji gets a Chinese
   form. Variable fonts are skipped (logged). Fonts load lazily and stay cached.
2. Per question, not per item: an item asked again (or its other part) gets a new
   draw, which is the point. No font twice in a row when there's a choice.
3. After answering, the font's family shows in muted text (e.g. in the answer panel's
   border), so a miss can be blamed on the typeface. Hidden while answering.
4. Reviews and the lesson quiz use it; item pages and lesson pages keep the default
   font (learning wants the clearest form).
5. `gfx.Source.Key` includes the font, so kitty caches each (glyph, font) once.
6. Radicals drawn from images (no Unicode character) are unaffected.
7. Tests: discovery/filtering from a fake font list, the per-question draw, the key.
   `just preview` gets a `--font` flag (or a roulette gallery) to eyeball fonts.

**Built** as planned (the user chose all three recommendations). Added on the way:
- Coverage: a pick must have every character of the subject. Against all 9,401
  subjects with characters, Noto Sans/Serif CJK JP miss none, IPAex Gothic/Mincho 14,
  Klee One 29, Yomogi 55 (mostly radicals like 扌, 艹, 氵). No covering font → the
  default font.
- Installed on the user's machine 2026-11-14: `google-noto-serif-cjk-fonts`,
  `ipa-exmincho-fonts`, `ipa-exgothic-fonts` (toolbox, dnf), Klee One and Yomogi
  Regular (+ OFL texts) in `~/.local/share/fonts`. Six families in the roulette.
- `roulette_fonts` takes font files (`~/` expanded) or installed family names; an
  unknown name stops startup and lists the installed families.
- `just preview review-wrong --font=all` shows a screen per font (or `--font=NAME`).
- Fonts are read from disk on demand (`ParseCollectionReaderAt`), not held in memory.

Not tried yet in the real app (Ghostty).

## M5 plan, step 3: themes (started 2026-11-14)

1. Refactor: every colour into one `ui.Theme` of named roles (three type colours,
   lesson/review marks, correct/wrong/warn/error, glyph ink, lines). No visual change.
2. `display.theme` + a `[theme]` section of per-role hex overrides.
3. Themes: `wanikani` (today's), `yadokari` (own brand palette, below), `terminal`
   (the terminal's ANSI palette, read with an OSC 4 query during the startup probe;
   falls back), `high-contrast` + `NO_COLOR` (no text colour, grey glyph field),
   `muted` optional.
4. `just preview --theme=all` and an HTML gallery (demo data) to choose.

**Brand (the user's idea, 2026-11-14):** if released, yadokari should look like its own
thing, not WaniKani's, and say it is unofficial ("not affiliated with WaniKani or
Tofugu LLC", the usual wording of third-party apps) in the README, `--version`, the
first-run token form and any website. A starting palette from the name (宿借り, hermit
crab: a borrowed shell, a tide pool), warm and sea-coloured where WaniKani is
blue/pink/purple: radical sea green `#2e8b57` (was teal `#1f8a7d`; greener per the user, 2026-11-14), kanji coral `#d9572b`, vocabulary
deep-sea indigo `#4c4fae`, accent sand `#d8b27a`. All carry white glyphs (large text,
≥ 3:1). To be judged in the gallery. Open: which theme is the default at release.
**Replaced 2026-11-21** (the user found the sea palette unappealing after two weeks):
a palette gallery of six electric black-terminal palettes; the user chose "ion":
radical ice `#5cd6ff`, kanji orange `#ff7b29`, vocabulary cobalt `#3b7bff`,
near-black ink `#05070c`, navy borders `#163a5c`, blue-grey muted `#6d93b8`.
Runner-up "blackbox" (deep fields, white ink: `#0a62e8` / `#d9530a` / `#6a2be0`)
added as its own theme. Glow (a bloom shader) is the terminal's business: not documented.
Then, in the app the bright fields glared; dimming them was tried, then frames: the
user chose a half-block frame (▛▀▜) of the type colour around the terminal's own
background, glyph white, for **every** theme (`ui/field.go`). Other frames and the old
fill stay in code for a later `display.field` setting (backlog).
Light terminals (2026-11-21): the probe also asks for the text and background
colours (OSC 10/11); `display.background = auto|dark|light`; framed glyphs take the
terminal's text colour; `yadokari --terminal` reports it all. Then light versions of
wanikani, yadokari and blackbox (`ui.lightThemes`, chosen in the gallery: each colour's
hue darkened to 4.5:1 on `#eeeeee`, pale tinted borders; the input badge takes the
terminal's text colour). Not yet seen in a real light terminal: check Ghostty with a
light theme and Ptyxis in light mode.

**Done 2026-11-14:**
- `ui/theme.go`: `Theme` roles → package styles (`SetTheme`); the refactor kept every
  colour code.
- `display.theme`: `wanikani` (default), `yadokari`, `terminal`, `high-contrast`.
- `[theme]` per-role overrides: `#rrggbb`, or a palette number `0`–`255` (not for
  `ink`, which is drawn into the image).
- Glyph fields may be palette colours: the cell background is the field (the kitty
  image is transparent but for the ink), so `gfx.BgSGR` sends `4x`/`10x`/`48;5;N` and
  the terminal paints its own shade.
- `terminal`: cyan radicals, magenta kanji, blue vocabulary, ANSI text colours. The
  probe also asks for palette 4–6 (OSC 4); `Theme.InkFor` picks white or dark ink,
  whichever has the better worst-case contrast (white if nothing is reported).
- `high-contrast`: WaniKani hues deep enough for 7:1+ behind white ink
  (`#005a9e`, `#a3005c`, `#5c00a3`), ANSI text colours in bold, borders and muted
  text in the full text colour (`Muted`, `Strong`, `Line: NoColor`).
- `NO_COLOR`: Bubble Tea then drops all colour, kitty placeholders included, so
  `gfx.Choose` returns plain whatever the override; glyphs are bold text.
- Gallery (4 themes, demo data): https://claude.ai/artifact/SaUCBaDFb3FWyb4eWbUYHL

The user is living with yadokari draft 2 (sea green) before deciding (2026-11-14);
watch whether the correct-answer green `#6aa84f` feels too close to the radical
green, and whether the sand bars and the gold/green text read as pastel. Palette
changes are one-line edits in `ui/theme.go` until a public release. Not tried in the
real app yet: `terminal`, `high-contrast`, `NO_COLOR`. Open: the default theme at
release.

## M5 plan, step 4: packaging and CI (built 2026-11-14)

For the user's Mac and a soft launch to friends. Decided with the user: MIT license;
the repo stays private for now (public later, after a history check for personal
data); Homebrew tap for macOS.

Built:
- macOS support: `gfx/termios_{linux,bsd}.go` (TCGETS vs TIOCGETA); `afplay`;
  `glyph.ListFonts` falls back to scanning font folders (macOS: /System/Library/Fonts,
  /Library/Fonts, ~/Library/Fonts, /System/Library/AssetsV2) with a list of Japanese
  families + "… JP", one face per family by typographic family/subfamily (Hiragino
  W3 over W0). On Linux the scan finds the same six families as fc-list in 80 ms.
- `--version` from build info for `go install` builds; the "unofficial, not
  affiliated with WaniKani or Tofugu LLC" line in `--version`, `--help`, the first-run
  form and the README. The first-run form now asks for reviews:create and
  assignments:start (it still said read-only). The "needs --live" notice no longer
  talks about the mock.
- LICENSE (MIT), README, `.goreleaser.yaml`, CI and release workflows, `just cross`,
  `just release-check`.

Not verified yet:
- `.goreleaser.yaml` hasn't been run (`goreleaser check` needs Go ≥ 1.27.4 and the
  sandbox refused the toolchain download): the user runs `just release-check`.
- Nothing has run on a Mac: CI's macOS job (tags, or by hand) runs the tests; the
  user tries the app on their Mac (Ghostty or kitty for glyphs, Hiragino, afplay,
  keyring via `security`, the roulette scan).
- First release: **v0.5.2** (2026-11-14), all four archives + checksums on the GitHub
  release. v0.5.1's tag push started no workflow (cause unknown; it would have failed
  `TestPlay` on Ubuntu anyway: dash forks `sleep`, fixed by killing the player's
  process group). `gh` is installed and authenticated: Claude can watch runs.
- The first macOS CI run (v0.5.2 tag) failed on font discovery: Homebrew fontconfig on
  macOS lists hidden and pan-CJK fonts and Hiragino as eight families. Fixed on main
  (macOS CI green); not in a release yet.
- Homebrew: create the public repo `dotmatrx/homebrew-tap` and a fine-grained token
  with contents:write on it as the `HOMEBREW_TAP_TOKEN` secret. While yadokari's repo
  is private, the cask can't download the release, so the tap waits for going public;
  until then the user's Mac takes the release tar.gz (via the GitHub site or `gh`).

## Live writes by build, not `--live` (2026-11-14)

The user decided `--live` had served its purpose (~100 live writes, an offline run,
lessons and undo, all as expected). Now builds made to be used (`just install`,
releases, `go install` of a tag) write to the live API by default, and development
builds never can (no flag; `buildKind` in `cmd/yadokari`). `--live` is gone, with no
compatibility shim (the user's choice: one user, no scripts). No `--dry-run` either:
the mock (`--base`, `just dev`) is the development path, and `cmd/wkmock` isn't linked
into the app. `just build` no longer stamps a version, so `bin/yadokari` is a
development build. The safety rule in CLAUDE.md was rewritten accordingly.

## Open questions

- Distribution: GitHub releases + Homebrew tap (decided 2026-11-14); public repo when the user decides.
- Settings: which session options become user settings, and where (config file vs a
  settings screen)? See the backlog.
