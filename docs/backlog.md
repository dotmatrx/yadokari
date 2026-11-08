# Backlog

Ideas and deferred work, newest first. Move an item into a milestone when it's picked
up; delete it when done or dropped.

| Added | Item | Notes |
|---|---|---|
| 2026-11-07 | Expose settings that exist in code but have no way to set them: answer strictness (`answer.Strictness`), session order / back-to-back / reading-first / active-set size / item limit (`session.Options`), retype-after-typo (`AllowUndo`, a key like ctrl+z), graphics tier override (`--gfx` flag exists; config key missing), font path (`glyph.FindFont`) | Config file first, settings screen in M5 |
| 2026-11-07 | Add the typed answer as a user synonym (ctrl+s after a wrong meaning) | Needs `POST/PUT /study_materials`; design doc puts it in M5 |
| 2026-11-07 | Mnemonics and hints in the answer panel | Needs the WK markup renderer (`<kanji>`, `<reading>`, `<ja>`…), planned for lessons (M4) |
| 2026-11-07 | Pre-upload the next 2–3 glyphs to kitty so Enter never waits | Design doc idea; not needed so far |
| 2026-11-07 | Lightning mode (auto-advance after a correct answer) | Design doc option; not built |
| 2026-11-07 | SRS stage chart on the dashboard (Apprentice / Guru / Master / Enlightened / Burned), like WaniKani's | Counts by `srs_stage` group from `assignments`; do with dashboard polish after M2 changes the layout |
| 2026-11-07 | "Relaxed n" input setting: `konnichiha` → こんにちは | Default stays WaniKani's `nn` → ん (Spike 3) |
| 2026-11-07 | Resend kitty images when the terminal reports them missing (`ENOENT`) | Only if a blank image ever recurs. The reproducible case was a renderer bug, fixed in `f25cbb5`; the Spike 1 one-off may have been related |
| 2026-11-07 | Check kitty placeholders in WezTerm and Rio; test re-rasterizing on font-size change | Spike 1 tested kitty only |
| 2026-11-07 | Spike 2: kitty text sizing (OSC 66) as an optional kitty-only tier | Skipped in Phase 0 |
