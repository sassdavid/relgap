---
description: Install relgap, give it a GitHub token, read what changed between two kubectl versions, then check the pins of a repository.
---

# Getting started

Four steps take you from install to a digest and a scan of a repository. relgap only reads: it
changes no file and opens no pull request. Every output on this page is the output of relgap's
own tests, which replay recorded GitHub answers.

## 1. Install

```sh
mise use -g github:sassdavid/relgap
```

[Install](/install) has the release archives, `go install`, the signature checks and shell
completion.

## 2. Give it a GitHub token

relgap reads release notes and tags through the GitHub API. Without a token, GitHub allows 60
requests an hour, and relgap sends at most 2 at once. relgap looks for a token the way gh does:
`GH_TOKEN`, then `GITHUB_TOKEN`, then gh's config file, then `gh auth token`. If you're logged in
with gh, there is nothing to do. Otherwise, export a token:

```sh
export GH_TOKEN=<your token>
```

A token with no scopes reads public repositories. relgap sends it to `api.github.com` and nowhere
else; [Network, privacy and tokens](/concepts/network) lists every host relgap talks to.

## 3. Read what changed between two versions

`relgap digest` takes a tool and a range. The range <VersionRange from="1.36.0" to="1.37.2" />
covers every release after 1.36.0 up to and including 1.37.2. `--only` keeps two classes, so the
output stays short, and `--format plain` prints text without colours:

```sh
relgap digest kubectl 1.36.0..1.37.2 --only breaking --only deprecation --format plain
```

<<< @/../testdata/golden/digest/kubectl-breaking.txt

The first lines name the repository, the 9 versions in the range and the entries per class, of
which `--only` lists two. One entry is under the `--min-confidence` threshold and hidden. Each
entry keeps its links from the release notes. Drop `--only` to see every class, and drop
`--format plain` in a terminal to get colours.

## 4. Check the pins of a repository

In a repository, `relgap pins` lists every tool version it pins, with the file and line. It reads
files only and makes no network call. The output below is from a sample repository in relgap's
tests, with a `mise.toml`, a `Dockerfile`, a `main.tf` and a GitHub Actions workflow:

```sh
relgap pins --format plain
```

<<< @/../testdata/golden/pins/project.txt

A value marked ≈ allows more than one version: a constraint, a prefix or a channel.
`relgap outdated` compares each pin with the latest version on its release line:

```sh
relgap outdated --format plain
```

<<< @/../testdata/golden/outdated/project.txt

BEHIND counts the releases between CURRENT and LATEST. BREAKING and DEPRECATION count entries in
the digest of that range. EOL comes from endoflife.date and VULNS from OSV, for tools whose
registry entry names them. Image pins are skipped, and the last line counts the one in this
repository. `relgap digest --all` prints the digest of every pin that is behind.

## Next

- [`relgap digest`](/cli/digest) and [`relgap outdated`](/cli/outdated) list every flag.
- [Configuration](/reference/config) sets defaults such as `--min-confidence` and `--fail-on`.
- [Supported tools](/reference/tools) lists the tools with a curated registry entry.
- [Exit codes](/reference/exit-codes) and [fail-on values](/reference/fail-on) are what a CI job
  checks.
