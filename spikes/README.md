# Spikes

Throwaway experiments from Phase 0. Code here is **not** production code: no tests
required, no reuse from `internal/` expected, and it may be deleted once findings are
written up in the design doc.

Each spike lives in its own directory as a `package main` and runs with
`just spike <dir>`. Record the outcome in the table below.

| Spike | Dir | Exit criterion | Result |
|---|---|---|---|
| 1. kitty Unicode placeholders in Bubble Tea v2 | `placeholders/` | 漢 in a 20×10 grid survives resize/re-render in kitty (toolbox, dnf); no flicker, no stray glyphs. Later: WezTerm/Rio flatpaks as a compatibility check | **Pass** (2026-11-07, kitty in toolbox; 2026-11-08, Ghostty on the host) |
| 2. OSC 66 text sizing in Bubble Tea v2 | `textsize/` | Renders at s=4 without corrupting surrounding rows | — |
| 5. Tier comparison | `tiers/` | Eyeball the tiers side by side in kitty and Ptyxis | Half-block **dropped** (blurry); found the lost-first-upload bug (2026-11-07). Ghostty: kitty tier drawn correctly (2026-11-08); the spike forces the kitty renderer, so it missed that env detection fails in the toolbox. It now also prints the probe result |
| 3. IME feel | `imefeel/` | kyouiku, gakkou, kon'nichiha, shinbun (incl. backspace) feel like WaniKani's site | **Pass** (2026-11-07) |

Spike 4 (API read path + mock) is not here: it graduates directly into `internal/wk`
and `internal/wkmock`.

## Spike 1 findings (kitty Unicode placeholders)

Passed: crisp glyphs at 10×5 to 60×26 cells, with and without a type-colour
background; no flicker at 10 Hz re-render; window resize works (font-size change not
explicitly tested);
both raw SGR and Lip Gloss foreground encodings carry the image ID.

- `tea.Raw` appends to the program's output buffer under a lock, so a whole
  multi-chunk transmit goes out in one write (answers a design-doc open question).
  Each tick flushes `tea.Raw` output *before* the renderer's frame.
- Charm's `x/ansi/kitty` has the encoder, `Placeholder` and the row/column
  `Diacritic` table; no need to hand-write them.
- Gotcha: `kitty.Options.Transmission` must be set to `kitty.Direct`; its zero
  value sends a command with no image data.
- `lipgloss.Width` / `ansi.StringWidth` count each placeholder cell (U+10EEEE +
  two diacritics) as one column, and Bubble Tea's renderer agrees in practice.
- Image IDs < 256 sent as `38;5;<id>` survive the renderer unchanged.
- Cell pixel size from `TIOCGWINSZ` works in kitty. Font-size changes should
  arrive as `WindowSizeMsg`; the spike re-measures and re-rasterizes there
  (untested).
- Anomaly: in one early run the first image (sent from `Init`) never appeared.
  Not reproducible, including with `--early`, which deterministically sends it
  before the alt-screen switch. Kept the simple mitigation (first send after the
  first frame); re-evaluate only if it recurs.

## Spike 3 findings (IME feel)

Passed: typing feels smooth and WaniKani-like, including backspace. The
converter is real code in `internal/ime` (round-trips all 11,346 readings in a
recorded account); only the input-line wrapper in `imefeel/` is throwaway.

- Bubble Tea v2's `View.Cursor` (bar shape) placed with `lipgloss.Width` of
  committed + pending text tracks correctly over double-width kana.
- `KeyPressMsg.Text` carries the typed text (including shifted uppercase and
  system-IME kana); `PasteMsg` covers bracketed paste.
- Matches WaniKani: `nn` → ん immediately, so こんにちは needs `kon'nichiha` or
  `konnnichiha`. **Future setting:** a "relaxed n" mode where `konnichiha` →
  こんにちは. Not needed now (user decision, 2026-11-07).
- Added Hepburn `tch` → っ (matcha, itchi), found by tests.

## Tier comparison findings

- Half-block (▀, two pixels per cell) is unreadable for anything but the simplest
  kanji, even at 24×12 cells; sextants/octants would only double that. Dropped:
  non-kitty terminals get the plain tier.
- The blank first image in kitty was a renderer API bug, reproducible: `Draw`
  marked an image uploaded when it returned the upload sequence, and the demo's
  View discarded that sequence before the first frame. Uploads are now queued in
  the renderer and sent with `Flush()` from Update. (Possibly also the Spike 1
  anomaly, though that code path differed.)
- Plain glyphs were blank in kitty because the toolbox had no CJK font visible to
  fontconfig; fixed by installing `google-noto-sans-cjk-fonts` in the toolbox.
