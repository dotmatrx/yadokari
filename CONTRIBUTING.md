# Contributing

Thanks for looking. yadokari is a small, one-person project, so for anything bigger than
a fix, open an [issue](https://github.com/dotmatrx/yadokari/issues) first to check it
fits. Planned and deferred work is already there. Security problems go through
[SECURITY.md](SECURITY.md), not an issue.

## Setup

You need Go 1.27 and [just](https://github.com/casey/just). `just` with no arguments
lists the recipes. For the glyph tests and font roulette, install a Japanese font
(Noto Sans CJK JP or Noto Serif CJK JP). Linux and macOS are supported; Windows isn't.

## Never write to the live API

Submitting a review or starting a lesson on WaniKani **can't be undone**, so all
development runs against a fake API:

```sh
just mock --demo   # a believable level 8 account on http://127.0.0.1:8642
just dev           # in another terminal: the app against the mock, data in .dev/
```

Development builds (`go run`, `just run`, `just dev`, `just build`, anything from a
modified tree) refuse to write to `api.wanikani.com` whatever their flags. Keep it that
way: a change that lets a development build write to the live API won't be merged.

- Don't run an installed or release `yadokari` while developing. If it finds a token
  that can write, anything you answer in it goes to your real account.
- If you point a development build at the live API to read data, use a **read-only**
  token (WaniKani lets you choose its permissions when you make one).
- Never commit tokens, recordings, databases or logs: they hold personal data. The
  usual places are gitignored (`.env`, `testdata/recordings/`, `.dev/`), and push
  protection blocks common token formats, but check `git status` anyway.

`just preview` renders screens from demo data without running the app, which is the
quickest way to work on layout. `--html=FILE` writes a gallery of them.

## Before you send a pull request

```sh
just fmt     # gofmt
just check   # gofmt check and go vet
just test    # all tests
```

CI runs the same on Linux and macOS, with `-race`, and cross-builds every release target
(`just cross`).

- Keep `internal/session`, `internal/answer`, `internal/markup` and `internal/ime` free of
  I/O and UI code, and add table tests for changes there.
- Colours come from the theme (`internal/ui/theme.go`), never hard-coded in a screen.
- A new setting goes in `docs/config.example.toml`, documented and commented out at
  its default (a test checks). Don't rename or remove a setting outright: unknown
  keys are rejected on load, so that breaks people's configs. Keep accepting the old
  name, with a warning, for at least one release.
- Messages on screen are plain words. Go errors, URLs and IDs go to `yadokari.log`.
- [`AGENTS.md`](AGENTS.md) is the map of the code: what each package does and where
  things live. It's written for coding agents but reads fine for people.

## Versions and releases

- Releases are annotated semver tags (`vMAJOR.MINOR.PATCH`) on `main`. Pushing one runs
  GoReleaser, which publishes the GitHub release and updates the Homebrew cask.
- From v1.0.0: a patch release (`1.0.x`) is fixes only; a minor release (`1.x.0`) adds
  features and settings; a major release is the only one that may break something people
  rely on: a setting, a key binding, a file's place or format. Settings are never renamed
  or removed outright (see above).
- A release is tagged when there's a build worth using for a while, not on every commit.
  Release notes on GitHub say what changed for people using yadokari.
