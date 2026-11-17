---
description: Read the Terraform and OpenTofu versions that tfcensus found in your state files into relgap drift, alone or next to scanned repositories.
---

# Feed tfcensus inventory into drift

[tfcensus](https://tfcensus.sasskovacs.dev/) reads Terraform state and lock files and lists every
unit with the version that last applied it. `relgap drift --from-tfcensus` turns that inventory
into drift rows, so you see which units run which Terraform, OpenTofu and provider versions. It
compares what was applied, where scanning repositories compares what is pinned.

## Pipe the inventory into drift

```sh
tfcensus inventory ~/src/infra/* --format json | relgap drift --from-tfcensus -
```

`-` reads the inventory from stdin. A file works too:

```sh
tfcensus inventory ~/src/infra/* --format json > inventory.json
relgap drift --from-tfcensus inventory.json
```

relgap reads the JSON of `tfcensus inventory --format json` with `schema_version` 1. Input from
another tool or another schema version exits 2. tfcensus's
[How state is read](https://tfcensus.sasskovacs.dev/concepts/state#feed-relgap) describes what
the inventory holds and how it feeds relgap.

## Rows and pins

Each unit becomes a row named `repo/path`. A workspace other than `default` adds `:workspace`, as
in `tf-fleet/edge:staging`. Each row gets these exact pins:

- The version that last wrote the unit's state, as a `terraform` pin, or an `opentofu` pin when
  tfcensus says OpenTofu applied it.
- Each provider version from the unit's `.terraform.lock.hcl`, as a `tfprovider:` pin such as
  `tfprovider:hashicorp/aws`.

A unit without a version adds no pins. When the inventory lists errors, for example a state file
tfcensus couldn't read, relgap prints the rest, says the report is partial and exits 1 unless a
`--fail-on` value matched.

## Combine with repositories

With roots, drift scans them as well and puts both kinds of rows in one matrix. Without roots,
`--from-tfcensus` scans nothing.

```sh
relgap drift ~/src/infra --from-tfcensus inventory.json --tool terraform
```

The matrix then shows the version a repository pins next to the version its state was written
with. Everything else works as in [Find version drift across repos](/guides/drift): `--tool`,
`--latest`, `ignore` rules, `--fail-on drift` and SARIF. [Drift](/concepts/drift) explains how
versions are compared.
