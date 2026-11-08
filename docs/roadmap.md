# Roadmap and status

Where the project stands and what comes next. The design rationale lives in the design
doc (link in CLAUDE.md); deferred ideas live in `docs/backlog.md`. Update this file when
a milestone moves.

**Last updated:** 2026-11-07 (end of day 1).

## Milestones

| Milestone | Status | Notes |
|---|---|---|
| Phase 0: spikes and safety net | Done | API client + mock, kitty placeholders (pass), IME (pass); OSC 66 skipped; findings in the design doc |
| M1: read-only dashboard | Done | Store, incremental sync, token (keyring works in the toolbox), dashboard; verified on the live account |
| M2: reviews against the mock | Done, user testing | Session engine, answer checking, glyph/gfx (kitty + plain), outbox + uploader, review and summary screens |
| M3: live reviews | Next | See below |
| M4: lessons | Not started | Item pages with mnemonic markup, lesson quiz, `PUT /assignments/:id/start`, audio |
| M5: polish | Not started | Settings screen, add synonym, font roulette, themes, packaging |

## Where things were left (2026-11-07)

- All work is committed on `main`; the user pushes to origin themselves.
- The user was testing M2 hands-on in kitty with `just mock-recorded` + `just dev` and
  found it "feeling quite good". One change came out of that (the answer panel no longer
  reveals the unasked part). No other open issues were reported.
- `.dev/yadokari.db` (gitignored) may hold a saved review session; `r` resumes it.
- A read-only API token is in `.env` (used by `just probe-live`); the user's real
  token is in the desktop keyring (used by `go run ./cmd/yadokari`).
- Five suggestions in the design doc (dropping the half-block tier) await the user's
  review.

## M3 plan: live reviews (proposed, not yet agreed)

1. Mechanically, `--live` already works: it enables live writes in the client and lifts
   the dashboard's review block. M3 is about using it safely, not building it.
2. The user creates a token with `reviews:create` permission (and nothing else) for
   `--live` use; the read-only token stays for development. Decide how the two coexist
   (e.g. `--live` reads a separately stored write token).
3. Confirm against the live API what the docs leave open: success status of
   `POST /reviews`, the 422 body for a not-due assignment, and whether
   `assignment_id` alone is enough (see `docs/wanikani-api.md`).
4. Surface rejected (failed) outbox rows on the dashboard; today only pending counts
   are shown.
5. Offline handling: reviews completed offline stay queued and upload on the next run;
   test with the network down.
6. Dogfood daily for two weeks next to the website, with the summary's SRS movement
   ("Guru ↑") added from the `resources_updated` response.

## Open questions

- Should lessons be in v1, or ship reviews only first? (design doc)
- Distribution: `go install` only, or packages too? (design doc)
- Settings: which session options become user settings, and where (config file vs a
  settings screen)? See the backlog.
