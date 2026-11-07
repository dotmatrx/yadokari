# Spikes

Throwaway experiments from Phase 0. Code here is **not** production code: no tests
required, no reuse from `internal/` expected, and it may be deleted once findings are
written up in the design doc.

Each spike lives in its own directory as a `package main` and runs with
`just spike <dir>`. Record the outcome in the table below.

| Spike | Dir | Exit criterion | Result |
|---|---|---|---|
| 1. kitty Unicode placeholders in Bubble Tea v2 | `placeholders/` | 漢 in a 20×10 grid survives resize/re-render in kitty (toolbox, dnf); no flicker, no stray glyphs. Later: WezTerm/Rio flatpaks as a compatibility check | — |
| 2. OSC 66 text sizing in Bubble Tea v2 | `textsize/` | Renders at s=4 without corrupting surrounding rows | — |
| 3. IME feel | `imefeel/` | kyouiku, gakkou, kon'nichiha, shinbun (incl. backspace) feel like WaniKani's site | — |

Spike 4 (API read path + mock) is not here: it graduates directly into `internal/wk`
and `internal/wkmock`.
