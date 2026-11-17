---
description: Compare the tool versions that many repositories pin, see which tools drift, add the latest versions and gate CI on drift.
---

# Find version drift across repos

`relgap drift` scans directories for repositories and prints a matrix: a row per repository and
a column per tool that drifts. A tool drifts when its pins stand for more than one version across
the scanned repositories. drift reads files only and sends no request unless you add `--latest`.
[Drift](/concepts/drift) explains how relgap compares the pins.

## Scan a tree of repositories

Name the directories to scan. Each one may hold many repositories: a directory with `.git` is a
repository, and a root without any `.git` counts as one repository itself.

```sh
relgap drift ~/src/infra ~/src/apps
```

Without arguments, drift scans the directories in `scan.roots`, then the current directory. Set
`scan.roots` in your user config to scan the same trees from anywhere:

```yaml
# ~/.config/relgap/config.yaml
scan:
  roots: [~/src/infra, ~/src/apps]
```

The output below comes from a test tree of 60 repositories in six team folders:

```sh
relgap drift tree --format plain
```

<<< @/../testdata/golden/drift/tree.txt

A version marked `*` differs from the version most repositories pin. Tools that agree everywhere,
or that only one repository pins, are listed below the matrix.

## What counts as a version

Each cell shows the versions a repository pins for the tool. An exact pin counts as its version.
A value marked ≈ allows more than one version:

- A constraint such as `~> 1.15` counts as the highest exact pin of the tool in the same
  repository that satisfies it, otherwise as its lower bound (1.15.0). In the sample,
  `repo-07` pins terraform 1.14.2 next to `~> 1.14`, so it counts as 1.14.2.
- A prefix such as `26` and a channel such as `latest` aren't compared. They show in the cell,
  and the tool goes under "Not compared" when no other pin of it is.

`--drift-ignore-constraints` compares exact pins only. Constraint pins still show, but they no
longer count.

## Pick the tools

`--tool` limits the report to the named tools and gives each one a column, even when every
repository agrees on it:

```sh
relgap drift ~/src/infra --tool terraform --tool terragrunt
```

`ignore` rules in the config drop pins before drift compares them. A rule names a tool and,
optionally, a glob on the file path:

```yaml
ignore:
  - tool: terraform
    path: "**/legacy/**"
```

[Pins and detectors](/concepts/pins) lists the files each detector reads. `--submodules` also
scans checked-out git submodules as repositories.

## Add the latest versions

`--latest` adds a `latest` row with the newest stable version of each tool, and the agreement
line names the latest version too. It needs the network or the [cache](/guides/cache):

```sh
relgap drift ~/src/infra --latest --format md
```

To see how far each pin is behind latest, with release notes and EOL data, use
[`relgap outdated`](/guides/outdated) in each repository.

## Gate CI on drift

`--fail-on drift` counts the drifted pins, the cells marked `*`, and exits 3 once the count
reaches `--min-count` (default 1). A file that doesn't parse, or a tool whose latest version
can't be listed, exits 1 when the gate didn't match. The report is written either way.

```sh
relgap drift --fail-on drift --min-count 3 --format sarif > drift.sarif
```

`--format sarif` writes a result per drifted pin at its file and line, and a `failed` result per
file or tool that failed. [Upload SARIF to code scanning](/guides/sarif) shows the upload step,
and [SARIF rules](/reference/sarif-rules) lists the rules. `--out` writes several formats from
one run, as in [Gate CI on breaking changes, EOL and vulnerabilities](/guides/ci-gate).

To compare Terraform units that tfcensus found, see
[Feed tfcensus inventory into drift](/guides/from-tfcensus).
