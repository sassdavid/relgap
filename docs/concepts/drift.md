---
description: What relgap means by drift, how each kind of pin counts toward it, how the majority version is chosen, and what the drift matrix shows.
---

# Drift

A team that keeps many repositories rarely pins the same terraform or node everywhere. Drift is
that spread: a tool drifts when its pins across the scanned repositories stand for more than one
version. [`relgap drift`](/cli/drift) shows which tools drift and which repositories differ, so
you can see where an upgrade has landed and where it hasn't.

Drift is a property of your files, so `relgap drift` reads files only. It makes no network call
unless you ask for `--latest`, and it needs no token. [Find version drift across repos](/guides/drift)
shows how to run it.

## What counts

Every repository under the roots is scanned the way [`relgap pins`](/concepts/pins) scans it.
Pins that map to no tool, and pins that an `ignore` rule drops, stay out. Each remaining pin
counts as one version:

| Pin | Counts as |
|---|---|
| exact (`1.15.8`) | itself |
| constraint (`~> 1.15`) | the highest exact pin of the tool in the same repository that satisfies it, else its lower bound (1.15.0) |
| prefix (`26`) or channel (`latest`) | nothing: it is shown but not compared |

A constraint borrows an exact pin from its own repository because that is usually what it
resolves to: a `required_version = "~> 1.14"` next to a `.terraform-version` of 1.14.2 means
1.14.2. Without one, relgap has no version list to pick from, since drift stays offline, so it
takes the lowest version the constraint allows. `--drift-ignore-constraints` shows constraint
pins without counting them.

Versions that the tool's scheme reads as equal are one version: 1.15 and 1.15.0 are the same
cell, shown as 1.15.0. Images, actions, providers and charts drift like tools, under their
`image:`, `action:`, `tfprovider:` and `chart:` names.

## The matrix

The matrix has a row per repository and a column per drifting tool. A cell lists what the
repository pins of that tool, versions first, then the values that aren't compared, each marked
≈ when it allows more than one version.

For each tool, the reference version is the one the most repositories pin, the newest on a tie.
A version marked * differs from it. Those cells are the drifted pins that `--fail-on drift`
counts and that `--format sarif` reports at the pin's file and line. The reference is a majority,
not a target: it says what most repositories run, not what they should run.

Below the matrix, "In agreement" lists the tools every repository pins the same way, or that
only one repository pins, and "Not compared" lists the tools whose pins are all prefixes or
channels. `--tool` gives the named tools a column even when they agree.

## Latest

`relgap drift --latest` adds a latest row with the newest stable version of each tool that has a
GitHub repository, on the [release line](/concepts/ranges#open-ranges-and-release-lines) of the
tool's reference version. It looks the versions up in batched GraphQL queries through the HTTP
cache, so it needs the network or a warm cache. The row tells you how far the whole fleet is
behind, next to how far apart its repositories are.

## Other sources of rows

`--from-tfcensus` reads the inventory JSON of tfcensus instead of scanning, or next to the scanned
roots. Each Terraform unit and workspace becomes a row, with the version that last wrote its state
as an exact terraform or opentofu pin and each provider version from its lock file as an exact
provider pin. Those rows show the versions that last applied each unit, next to the versions
the files ask for.
[Feed tfcensus inventory into drift](/guides/from-tfcensus) shows the setup.
