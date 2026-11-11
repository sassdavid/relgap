# relgap ADR-0007: Pins read mise config files, not `mise ls`

- Status: accepted (2026-11-11)
- Amends [ADR-0003](0003-integrate-with-mise.md) for pin discovery. ADR-0003 still holds for
  `mise registry --json`. [ADR-0009](0009-outdated-current-and-latest.md) drops
  `mise outdated --json` and `mise ls` from `outdated`.

## Context
ADR-0003 and the pin-discovery spec preferred `mise ls --json --current` when mise is on PATH.
Running mise 2026.11.6 against a scratch `mise.toml` showed:

- Each entry has `requested_version` (the value as written), the resolved `version` and an
  absolute `source.path`. There are no line numbers, and pins need file:line.
- An untrusted config makes mise exit with "Config files ... are not trusted". `drift` scans
  repositories the user has never trusted.
- mise merges config from parent directories and the global config, so a repository's pins would
  include files outside it.
- The spec asks for the same pins with and without mise on PATH. Only `requested_version` meets
  that, and the files hold the same value.
- `drift` over 60 repositories would start 60 mise processes inside a 5 s budget.

## Decision
- `relgap pins` (and later `drift`) parses mise config files with `go-toml/v2`'s `unstable`
  parser, which gives positions: `mise.toml`, `.mise.toml`, `mise.*.toml`, `.mise.*.toml`, and
  `config.toml`, `config.*.toml` and `conf.d/*.toml` inside a directory named `mise`, `.mise` or
  `_mise`.
- `[tools]` values may be strings, arrays of strings or tables with a `version` key, inline or as
  `[tools."backend:name"]`. Every array element is a pin.
- Tool keys are mapped the way mise maps them: options in brackets are dropped, `core:` is
  dropped, `nodejs`, `golang` and `dotnet-core` become `node`, `go` and `dotnet`, and
  `ubi:owner/repo` resolves as `github:owner/repo`.
- relgap doesn't run `mise ls` in this release. `outdated` (M6) may run it to learn the installed
  version of a constraint or `latest` pin, as an addition on top of the parsed pins.

## Consequences
- Pins have file:line, don't depend on `mise trust`, and match with and without mise on PATH.
  `TestSpec_pindiscovery_2` checks that a fake mise on PATH is never asked for `ls`.
- mise features that change which files load (`MISE_ENV`, `MISE_CONFIG_FILE`, idiomatic version
  files of other tools) aren't followed. The idiomatic detector covers the common version files.
