# Roadmap and status

Where the project stands and what comes next. The design rationale lives in the design
doc (link in CLAUDE.md); deferred ideas live in `docs/backlog.md`. Update this file when
a milestone moves.

**Last updated:** 2026-11-08 (day 2).

## Milestones

| Milestone | Status | Notes |
|---|---|---|
| Phase 0: spikes and safety net | Done | API client + mock, kitty placeholders (pass), IME (pass); OSC 66 skipped; findings in the design doc |
| M1: read-only dashboard | Done | Store, incremental sync, token (keyring works in the toolbox), dashboard; verified on the live account |
| M2: reviews against the mock | Done | Session engine, answer checking, glyph/gfx (kitty + plain), outbox + uploader, review and summary screens |
| M3: live reviews | Dogfooding | All items done except the two weeks of daily use; see below |
| UI pass | In progress | Calmer, grid-like, full-width frame; see below |
| M4: lessons | Not started | Item pages with mnemonic markup, lesson quiz, `PUT /assignments/:id/start`, audio |
| M5: polish | Not started | Settings screen, add synonym, font roulette, themes, packaging |

## Where things were left (2026-11-08)

- All work is committed on `main`; the user pushes to origin themselves.
- **Terminal switch:** the user now runs Ghostty (host, rpm-ostree) as the daily
  driver, with the app run from the `yadokari` toolbox inside it. The spikes looked
  right, but the app fell back to the plain tier: toolbox hides Ghostty's `TERM`. Fixed
  by probing the terminal (`gfx.Probe`) and a `--gfx` override. The probe also reads the
  cell size (the toolbox pty reports none). Confirmed in Ghostty: `env tier plain`,
  probe `{Kitty:true CellW:12 CellH:30}`. See CLAUDE.md.
- **First live session done (2026-11-08):** 11 reviews, all accepted, outbox empty
  afterwards; the user finds it "already feeling very usable". Dogfooding has begun.
- M3 items 4 and 6 (summary half) are done: rejected reviews and the pending count on
  the dashboard (`x` dismisses rejected rows), and per-item SRS movement in the
  summary, filled in as uploads land.
- `.dev/yadokari.db` (gitignored) may hold a saved review session; `r` resumes it.
- A read-only API token is in `.env` (used by `just probe-live`); the user's real
  token is in the desktop keyring (used by `go run ./cmd/yadokari`).
- Five suggestions in the design doc (dropping the half-block tier) await the user's
  review.

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
  Selected by the temporary `Deps.Layout`; remove the loser and the old renderer once
  the user picks.
- Next: apply the chosen frame to the review and summary screens (the review screen's
  prompt bar and answer boxes still paint backgrounds).

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

## Open questions

- Should lessons be in v1, or ship reviews only first? (design doc)
- Distribution: `go install` only, or packages too? (design doc)
- Settings: which session options become user settings, and where (config file vs a
  settings screen)? See the backlog.
