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
| M3: live reviews | In progress | Rejected reviews on the dashboard and SRS movement in the summary done; see below |
| M4: lessons | Not started | Item pages with mnemonic markup, lesson quiz, `PUT /assignments/:id/start`, audio |
| M5: polish | Not started | Settings screen, add synonym, font roulette, themes, packaging |

## Where things were left (2026-11-08)

- All work is committed on `main`; the user pushes to origin themselves.
- **Terminal switch:** the user now runs Ghostty (host, rpm-ostree) as the daily
  driver, with the app run from the `yadokari` toolbox inside it. The spikes and the
  review screen were checked there and look right (kitty tier). See CLAUDE.md.
- M3 items 4 and 6 (summary half) are done: rejected reviews and the pending count on
  the dashboard (`x` dismisses rejected rows), and per-item SRS movement in the
  summary, filled in as uploads land.
- `.dev/yadokari.db` (gitignored) may hold a saved review session; `r` resumes it.
- A read-only API token is in `.env` (used by `just probe-live`); the user's real
  token is in the desktop keyring (used by `go run ./cmd/yadokari`).
- Five suggestions in the design doc (dropping the half-block tier) await the user's
  review.

## M3 plan: live reviews (agreed 2026-11-08)

1. Mechanically, `--live` already works: it enables live writes in the client and lifts
   the dashboard's review block. M3 is about using it safely, not building it.
2. **Decided: one token.** The keyring holds a token with `reviews:create`, used for
   everything by the real app; live writes stay blocked unless `--live` is passed.
   `.env` keeps the read-only token for `probe-live` and anything Claude runs. Next:
   the user swaps the keyring token (`--logout`, then enter the new one).
3. Confirm against the live API what the docs leave open: success status of
   `POST /reviews`, the 422 body for a not-due assignment, and whether
   `assignment_id` alone is enough (see `docs/wanikani-api.md`). Add logging of the
   status and response body before the first `--live` session.
4. ~~Surface rejected (failed) outbox rows on the dashboard.~~ Done 2026-11-08.
5. Offline handling: reviews completed offline stay queued and upload on the next run;
   test with the network down. Today pending reviews only upload once the review
   screen opens; the dashboard should flush them too.
6. Dogfood daily for two weeks next to the website. ~~Summary SRS movement~~ done
   2026-11-08 (from the stored assignment that `resources_updated` returned).

## Open questions

- Should lessons be in v1, or ship reviews only first? (design doc)
- Distribution: `go install` only, or packages too? (design doc)
- Settings: which session options become user settings, and where (config file vs a
  settings screen)? See the backlog.
