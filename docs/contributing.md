---
description: How to build and test relgap, add a registry entry, change the docs site, and send a pull request.
---

# Contributing

relgap is developed on GitHub at
[github.com/sassdavid/relgap](https://github.com/sassdavid/relgap). Bug reports and questions go
to [issues](https://github.com/sassdavid/relgap/issues), changes come as pull requests. The
engineering standards, stack choices and design records are in `docs/engineering/` in the
repository.

## Set up

relgap pins its whole toolchain in `mise.toml`, so [mise](https://mise.jdx.dev) installs the Go
version and every linter it uses:

```sh
git clone https://github.com/sassdavid/relgap.git
cd relgap
mise install
mise run check
```

`mise run check` runs the linters (Go, prose, workflows, licenses, docs formatting),
`go test -race ./...` and govulncheck. A pull request needs it to pass.

## Tasks

| Task | What it does |
|---|---|
| `mise run check` | every linter, the tests with the race detector, and govulncheck |
| `mise run test:golden -- -update` | regenerates the golden files; review the diff before you commit it |
| `mise run docs` | regenerates the CLI and reference pages of this site and fails when one was stale |
| `mise run fuzz` | runs every fuzz target for 60 seconds |
| `mise run registry:snapshot` | refreshes the embedded mise and aqua registry snapshots |
| `mise run snapshot` | builds the release archives locally with goreleaser |

Tests never call the network. HTTP answers are recorded once and replayed, and command tests
are testscript files under `testdata/script/`. A change to a command's output changes a golden
file under `testdata/golden/`, and the pages of this site that show that output include the
golden file directly.

## Add or fix a registry entry

A registry entry is the most common contribution: one YAML file in `registry/` that says where
a tool's release notes are. [Write or fix a registry entry](/guides/registry-entry) walks
through it. Run `relgap registry lint` and `relgap registry test` on the file, and put the
`registry test` output in the pull request.

## Change the docs site

The site is VitePress in `docs/`. Its tools come from `mise.docs.toml`:

```sh
MISE_ENV=docs mise install
cd docs && aube install --frozen-lockfile && cd ..
mise run docs:dev
```

`mise run docs:build` runs the stale checks, the snippet check that parses each `relgap` command
on the site, the build with its social cards, the card and third-party request checks, and an
offline link check. `mise run docs:a11y` then runs axe on the built site in both themes; it
needs Chrome, from `CHROME_PATH` or on `PATH`. Pages under
`docs/cli/` and most of `docs/reference/` are generated, so change the help text or schema they
come from and run `mise run docs`. Text follows the writing rules that `mise run lint:prose`
checks: plain words, sentence-case headings and no dashes as punctuation.

## Pull requests

Commit subjects follow [Conventional Commits](https://www.conventionalcommits.org): `feat:`,
`fix:`, `docs:` and the rest, in the imperative and under 72 characters. release-please builds
the changelog and the version from them. A pull request is squash-merged, so its title becomes
the commit subject. Say in the body what changed and why, and how you checked it.

A new dependency needs a reason in `docs/engineering/stack.md`. A new detector or source adapter
needs fixtures and a golden file.
