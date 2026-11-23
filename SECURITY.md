# Security

## Reporting a vulnerability

Report it privately through GitHub:
[**Report a vulnerability**](https://github.com/dotmatrx/yadokari/security/advisories/new)
(the Security tab → "Report a vulnerability"). Please don't open a public issue for it.

You'll get an answer within a week. yadokari is a one-person project, so a fix may take
longer than that. Once it's fixed, the advisory is published with credit to you, unless
you'd rather not be named.

Only the [latest release](https://github.com/dotmatrx/yadokari/releases/latest) gets
fixes.

Problems with WaniKani itself (the website, the API, your account) go to
[WaniKani](https://www.wanikani.com), not here.

## What counts

yadokari holds a WaniKani API token and can make changes to your account that can't be
undone, so these are the areas that matter most:

- **The API token.** yadokari reads it from `WANIKANI_API_TOKEN`, then the OS keyring,
  then a `0600` file at `~/.config/yadokari/token` if there's no keyring. It must never
  appear on screen, in `yadokari.log` or `writes.log`, in a request to any host other
  than the configured API, or in a file other people can read.
- **Writes to WaniKani.** Submitting a review and starting a lesson are permanent.
  Development builds (`go run`, `just run`, `just dev`, `just build`, and any build of a
  modified tree) must never write to `api.wanikani.com`. Only release builds,
  `just install` and `go install` of a release tag do. Any way to make a development
  build write to the live API, or to make yadokari write something you didn't answer,
  is a vulnerability.
- **Content from the API.** Mnemonics, hints, radical SVGs and audio come from
  WaniKani and are drawn in your terminal. Content that could inject terminal escape
  sequences, crash the app, or read or write files outside yadokari's own config,
  data and cache folders is in scope.
- **Release artifacts.** The release archives, `checksums.txt` and the Homebrew cask in
  [dotmatrx/homebrew-tap](https://github.com/dotmatrx/homebrew-tap).

## If your token leaks

Revoke it in WaniKani's
[API token settings](https://www.wanikani.com/settings/personal_access_tokens), then run
`yadokari --logout` to remove the stored copy. The next run asks for a new token.
