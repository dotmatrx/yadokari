# Backlog

Ideas and deferred work, newest first. Move an item into a milestone when it's picked
up; delete it when done or dropped.

| Added | Item | Notes |
|---|---|---|
| 2026-11-11 | Audio progress bar (or spinner) next to the 🔊 badge | Players report no position: it would be wall-clock time over the clip length (needs mp3 parsing), off by the player's ~0.1–0.2 s start-up, and needs a redraw ticker, for 1–2 s clips. Revisit if the badge feels too static |
| 2026-11-11 | Audio voice choice in the config file | Player override and autoplay overrides are config keys (2026-11-14); the voice still follows WaniKani's default voice |
| 2026-11-14 | Linux packages (nfpm deb/rpm in GoReleaser), COPR/AUR | After the tar.gz release and Homebrew tap prove out |
| 2026-11-14 | macOS code signing and notarization | Needs an Apple Developer account; the cask clears the quarantine flag meanwhile |
| 2026-11-14 | Check the git history for personal data before going public | Recordings and .env are gitignored; check old commits, docs and test fixtures anyway |
| 2026-11-08 | A config setting to make `--live` the default | Needs the user to relax the safety rule in CLAUDE.md (writes only with `--live`); a shell alias does the job meanwhile. More pressing with friends using it (2026-11-14): every run needs the flag |
| 2026-11-07 | Add the typed answer as a user synonym (ctrl+s after a wrong meaning) | Needs `POST/PUT /study_materials`. **Deferred 2026-11-14 until the distribution decision** (the user doesn't use synonyms). Forum search: no usage numbers; a steady trickle of threads over years asking for more synonyms and for adding them in lessons (three generations of a lesson-synonyms userscript), so a real but minority habit, probably expected by people coming from the website or Tsurukame. Synonyms added on the website already work here (they sync and the answer check accepts them) |
| 2026-11-07 | Mnemonics and hints in the answer panel | `i` opens the full item page after an answer (2026-11-11); this would put the mnemonic inline too. Revisit after dogfooding `i` |
| 2026-11-07 | Pre-upload the next 2–3 glyphs to kitty so Enter never waits | Design doc idea; not needed so far |
| 2026-11-07 | Lightning mode (auto-advance after a correct answer) | Design doc option; not built |
| 2026-11-07 | "Relaxed n" input setting: `konnichiha` → こんにちは | Default stays WaniKani's `nn` → ん (Spike 3) |
| 2026-11-07 | Resend kitty images when the terminal reports them missing (`ENOENT`) | Only if a blank image ever recurs. The reproducible case was a renderer bug, fixed in `f25cbb5`; the Spike 1 one-off may have been related |
| 2026-11-07 | Check kitty placeholders in WezTerm and Rio; test re-rasterizing on font-size change | Spike 1 tested kitty only |
| 2026-11-07 | Spike 2: kitty text sizing (OSC 66) as an optional kitty-only tier | Skipped in Phase 0 |
