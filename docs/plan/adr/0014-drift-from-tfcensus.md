# relgap ADR-0014: `drift --from-tfcensus` reads the tfcensus inventory

- Status: accepted (2026-11-16)
- Amends the row rule of [ADR-0010](0010-drift-offline-matrix.md). Implements the tfcensus pin
  source in [integration.md](../integration.md).

## Context
`tfcensus inventory --format json` lists every Terraform unit and workspace with the version
that last wrote its state (`terraform_version`) and the provider versions from the unit's
`.terraform.lock.hcl` (`version_source: lockfile`). integration.md names the relgap side: a pin
source with detector `tfcensus` that reads tfcensus `schemas/inventory-v1.json`. That schema has
`schema_version` "1" and hasn't changed between tfcensus v0.1.0 and v0.2.0.

ADR-0010 gives `drift` a row per scanned repository. A tfcensus repository often holds many
units, and each unit's state can run a different version, so one row per repository would merge
the answer to "which units still run aws 5.x".

## Decision
- `relgap drift --from-tfcensus <file>` reads the inventory, or stdin for `-`. Without roots it
  scans nothing, not even `scan.roots` or the current directory. With roots, the scan and the
  inventory make one matrix, so code pins and applied versions sit side by side.
- relgap accepts `tool: tfcensus` with `schema_version` "1" (the JSON string) and ignores fields
  it doesn't know, so additive changes inside v1 don't break it. Another tool, another or a
  missing schema version, JSON that doesn't parse or a missing `data.units` is a usage error
  (exit 2) that names what it found.
- Each unit and workspace is a row named `repo/path`, plus `:workspace` when the workspace isn't
  `default`. That is the target form tfcensus uses in its errors. This amends ADR-0010's row per
  repository for inventory rows only.
- `terraform_version` is an exact pin of `opentofu` when the unit's `engine` is `opentofu`,
  otherwise of `terraform`. It counts whenever tfcensus reports it, including on `unsupported`
  states, because it is the version that really wrote the state. Its file is the base name of
  the state key (`terraform.tfstate`), since the state has no file in the repository.
- Each provider with a `version` from `version_source: lockfile` is an exact
  `tfprovider:<source>` pin in `.terraform.lock.hcl`, named the way the `tf-lock` detector names
  it. Providers without a version add no pin. Units that are absent, encrypted or failed add a
  row and no pins.
- Inventory pins have line 0 and detector `tfcensus`. The inventory's `errors[]` become
  envelope errors with the tfcensus target and the message prefixed `tfcensus:`. They make the
  report partial (exit 1) under ADR-0012's rules.
- The MCP `drift` tool doesn't take an inventory. `relgap pins` doesn't read one either.

## Consequences
- tfcensus can add optional fields to inventory v1 without a relgap release. A schema v2 needs a
  relgap change, and the exit 2 says so.
- Rows, JSON locations and SARIF URIs can carry a unit name such as `tf-fleet/edge:staging`,
  which isn't a directory. The SARIF URI of a state pin is the unit plus the state's base name.
- An OpenTofu state never drifts against a Terraform state, because their versions sit in
  different columns.
- The fixture `testdata/tfcensus/tf-fleet.json` is a copy of a tfcensus golden. It is copied
  again when that golden changes, and never edited in relgap.
