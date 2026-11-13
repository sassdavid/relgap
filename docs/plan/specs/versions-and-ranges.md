# Spec: version schemes and range selection

## Schemes (`version.Scheme` interface: `Parse`, `Compare`, `IsPrerelease`, `Line(level)`)

| Scheme | Examples | Implementation |
|---|---|---|
| `semver` | `1.17.1`, `v3.6.0-rc.1` | Masterminds/semver v3 |
| `loose` | `go1.20` (no `.0`), `2.35.3`, `21.0.9+10` | hashicorp/go-version + normalizers |
| `calver` | `2026.11.5` (mise), `2026.1` | numeric segments, compare as ints |
| `python` | `3.13.0a1`, `3.12.7rc1` | PEP 440 subset (a/b/rc), custom |
| `regex` | anything, via registry `extract: "jdk-(\\d+\\.\\d+\\.\\d+)\\+(\\d+)"` | capture groups → loose |

- Tag → version: strip the registry `tag_prefix` (`v`, `go`, `atlascli/v`, `helm-chart-`, `rel/`, `jdk-`), then parse. Unparseable tags are dropped and logged at debug.
- Equal versions under the scheme are deduplicated (`3.7.12` vs `v3.7.12`), the way Renovate does.

## Version universe
- `universe: releases` (default when the repo has GitHub releases) or `tags` (awscli, go, python, kubernetes: tags exist where releases don't, or releases are boilerplate).
- Drafts are always dropped. Prereleases are dropped unless `--include-prereleases`, or unless `from` or `to` is itself a prerelease.
- **Sort by version, never by date:** helm publishes v3.22.1 after v4.3.0.

## Range selection
- `(from, to]`: from is excluded, to is included. `to` defaults to the latest stable on the same **line policy**:
  - default `line: none` (any newer version)
  - `--line major` keeps the major (helm v3), `--line minor` keeps the minor (LTS-style)
  - registry entries can set a default line (helm, temurin per-major repos)
- If `from` isn't in the universe (a yanked or private build), take the nearest lower version and warn.
- Downgrade ranges (from > to) are refused with exit 2. Hint: swap the arguments.
- Constraint pins (`~> 1.15`) take the highest *installed or locked* version if known, else the lowest satisfying version, and the output says which was used.

## Acceptance criteria
- [x] Table tests for every row above, plus Go's `go1.21rc2`, `go1.20` < `go1.20.1`, and Python `3.13.0a1` < `3.13.0`.
- [x] Fuzz: `Parse` never panics; `Compare` is antisymmetric and transitive on parsed values (property test over the fuzz corpus).
- [x] The helm fixture (interleaved v3/v4 publish dates) gives the correct ordered range for `3.19.0..3.22.1` and `--line major`.
- [x] The awscli fixture (no releases, tags only) yields the correct range from tags.
- [x] `atlascli/v1.30.0` tags are selected and bare `v1.2.3` strays are ignored, using the registry prefix (MongoDB Atlas CLI, as in the dotfriedrice mise config).
