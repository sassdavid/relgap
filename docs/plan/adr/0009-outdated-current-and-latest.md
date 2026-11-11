# relgap ADR-0009: How `outdated` picks current and latest versions

- Status: accepted (2026-11-11)
- Amends [ADR-0003](0003-integrate-with-mise.md) and [ADR-0007](0007-parse-mise-config-for-pins.md):
  `outdated` and `digest --from-mise` run neither `mise outdated --json` nor `mise ls`.

## Context
`relgap outdated` compares each pin with the newest version of its tool. `digest --from-mise`
digests the same ranges. Pins come from the scanner, and many aren't exact: terraform
`required_version` constraints, mise prefixes such as `uv = "0.13"`, and channels such as
`latest` and `lts`.

ADR-0003 planned to read `mise outdated --json`. That would give mise's idea of latest, which
comes from whatever backend mise uses (aqua, ubi, http) and can differ from the GitHub releases
relgap digests. The BEHIND and BREAKING columns would then describe a range relgap never
digested. mise also needs trusted config files, which a scanned project may not have.

## Decision
- Every pin match is compared:
  - An exact pin is its own current version.
  - A constraint takes the highest exact pin of the same tool in the same repository that
    satisfies it (a `.terraform-version` or `mise.toml` next to `required_version`), else the
    lowest version that does (`version.ResolveConstraint`).
  - A prefix takes the highest stable version that starts with it, which is what mise installs.
  - The `latest` channel is the latest version. Other channels (`lts`, `corretto-17`) are listed
    with status `unknown`.
- Latest is the newest stable version on the tool's registry line, sorted by `version.Scheme`.
  One GraphQL query per 25 repositories reads the 30 newest releases (or tags, by tag commit
  date) of each. 45 tools take 2 queries (`TestSpec_source_adapters_6`).
- A pin behind latest gets the full digest `current..latest` from the same `report.Digester`
  that `relgap digest` uses. BEHIND, BREAKING and DEPRECATION are read from that digest. There
  is no second, lighter count. Pins with the same tool and current version share one digest.
- `digest --from-mise` keeps the pins the mise detector found; `digest --all` keeps every pin.
  Both print the digests of the `outdated` comparison.
- Exit codes: a tool that fails, or a file that doesn't parse, exits 1 after printing the rest.
  That wins over a `--fail-on` match (exit 3), so a CI gate never decides on partial data.
- `--fail-on outdated` (with `--max-behind`) and `--fail-on breaking` are checked. `eol`, `vuln`
  and `deprecation` exit 2 until relgap can check them; `drift` belongs to `relgap drift`.

## Consequences
- The newest version can fall outside the 30-release window when a project ships more than 30
  backports after it. A pin behind latest still gets the right range, because its digest reads
  the full list.
- relgap doesn't know installed versions. A constraint without an exact pin next to it shows
  the lowest satisfying version, marked ≈, and the notes say so.
- Pins of tools without a GitHub repository, and tfprovider, chart and image pins, are skipped
  and listed in the JSON output with the reason.
