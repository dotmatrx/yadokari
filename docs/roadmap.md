# Roadmap and status

Where the project stands and what comes next. The design rationale lives in the design
doc (link in CLAUDE.md); deferred ideas live in `docs/backlog.md`. Update this file when
a milestone moves.

**Last updated:** 2026-11-11.

## Milestones

| Milestone | Status | Notes |
|---|---|---|
| Phase 0: spikes and safety net | Done | API client + mock, kitty placeholders (pass), IME (pass); OSC 66 skipped; findings in the design doc |
| M1: read-only dashboard | Done | Store, incremental sync, token (keyring works in the toolbox), dashboard; verified on the live account |
| M2: reviews against the mock | Done | Session engine, answer checking, glyph/gfx (kitty + plain), outbox + uploader, review and summary screens |
| M3: live reviews | Dogfooding | Built; the two weeks of daily use (to 2026-11-22) run alongside M4 |
| UI pass | Done | Panels frame on every screen; approved 2026-11-11 |
| M4: lessons | In progress | Item pages with mnemonic markup, lesson quiz, `PUT /assignments/:id/start`, audio |
| M5: polish | Not started | Settings screen, add synonym, font roulette, themes, packaging |

## Versions

Semver tags (`vMAJOR.MINOR.PATCH`, annotated) on `main`, starting at v0.3.0 on
2026-11-10. While on 0.x, the minor version follows the milestone (0.3 = M3, 0.4 = M4
lessons, 0.5 = M5 polish); patches are fixes found while dogfooding. Tag when installing
a build you'll use for a while, not every commit. v1.0.0 when it's ready to hand to
someone else (after M4 and M5's packaging). `just build`/`just install` stamp
`git describe` (e.g. `v0.3.0-2-gabc1234-dirty` between tags) into the binary: shown by
`--version`, in the dashboard header and in each `writes.log` entry. `go run` builds
(`just run`, `just dev`) say `dev`.

## Where things were left (2026-11-11)

- All work is committed on `main`; the user pushes to origin themselves.
- **M3 dogfooding:** 55 live reviews over 2026-11-08 to 11-10, all 201. Still unseen:
  a 422 body and a real network-down run; we watch for them rather than wait (every
  write is in `writes.log`). The keyring token has full write permissions, including
  `assignments:start` for lessons.
- **UI pass done:** the user approved round 3 on 2026-11-11.
- **Design doc:** the half-block suggestions (five edits) are accepted; none pending.
- **M4:** markup renderer, item page and lesson flow done against the mock; next is the
  user trying lessons on the mock, then a first live batch, then audio. The user liked
  the item page in Ghostty; its empty space is left as is until lessons have been used.
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
4. Audio, last.

## Open questions

- Distribution: `go install` only, or packages too? (design doc)
- Settings: which session options become user settings, and where (config file vs a
  settings screen)? See the backlog.
