# relgap ADR-0010: How `drift` compares pins without the network

- Status: accepted (2026-11-11)
- Builds on [ADR-0009](0009-outdated-current-and-latest.md), which covers `outdated`. Answers
  open question Q6.

## Context
`relgap drift` compares the pins of each tool across many local repositories. It reads files
only, unless `--latest` is set. ADR-0009 resolves a constraint to the highest exact pin of the
tool in the same repository, else the lowest released version that satisfies it. That fallback
needs the tool's release list, which drift doesn't fetch.

The CLI plan called the output a "tool × repo matrix". A tree of 60 repositories gives 60
columns, which neither a terminal nor a GitHub comment shows well.

## Decision
- A tool drifts when its compared pins stand for more than one distinct version, by
  `version.Scheme`, across all scanned repositories.
- An exact pin counts as its version. A constraint counts as `version.ResolveConstraint` over
  the exact pins of the tool in the same repository, else as its own lower bound
  (`Constraint.Floor`: `~> 1.15` is 1.15.0, `>= 1.5, < 2` is 1.5.0). A constraint without an
  inclusive lower bound (`< 2`, `> 1.0`) isn't compared. `--drift-ignore-constraints` leaves
  every constraint out.
- Prefix pins (`26`) and channels (`latest`) aren't compared. The matrix shows them as written,
  marked ≈.
- The matrix has a row per repository and a column per drifting tool. Tools that agree, or that
  one repository pins, go on a line below it. `--tool` limits the report to the named tools and
  gives each a column. Outliers are cells that differ from the version most repositories pin,
  the newest on a tie.
- Repository labels are the paths `relgap pins` prints. Several roots make one matrix.
- `--latest` adds a latest row from one `Client.Recent` lookup (one GraphQL query per 25
  tools), on the registry line of each tool's reference version. The comparison doesn't change.
- Config ignore rules drop pins in `drift`, `outdated` and `digest --from-mise|--all`. `pins`
  keeps listing every pin.
- Exit codes follow ADR-0009: a file that doesn't parse, or a tool whose latest version can't be
  listed, exits 1, which wins over a `--fail-on drift` match (exit 3). Superseded by
  [ADR-0012](0012-fail-on-gate-and-sarif.md): a match exits 3, and exit 1 needs a partial report
  without one.
- Q6: drift reads local clones only. Remote repositories through the GitHub or Bitbucket API
  are out of scope for v0.x.

## Consequences
- A lone `~> 1.15` and an exact `1.15.0` count as the same version, though the constraint may
  run 1.17 in practice. The ≈ mark and the JSON `via` field say so.
- `drift` and `outdated` can show different versions for the same constraint pin: `outdated`
  uses the lowest released version, `drift` the lower bound as written.
- SARIF output for `drift` lands with the rest of SARIF in M9.
