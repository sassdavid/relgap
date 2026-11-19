---
description: Add the dependencies Renovate extracts from a repository to relgap pins, for files relgap's own detectors don't read.
---

# Use Renovate's extract report as a pin source

relgap's detectors read the files on [Pins and detectors](/concepts/pins). Renovate knows many
more package files and custom regex managers. When Renovate already finds a pin that relgap
misses, its extract report can fill the gap.

A custom manager driven by a `# renovate: datasource=... depName=...` comment above the version
needs no report: relgap's [`renovate-comment` detector](/concepts/pins#renovate-comments) reads
those comments in Terraform, HCL, YAML, Makefiles, `pom.xml`, shell scripts and `.env` files.

## Write the report

Renovate's local platform runs in the current directory, with or without a Renovate config.
`--dry-run=extract` stops after the extract phase, and `--report-type=file` writes the report to
`--report-path`:

```sh
renovate --platform=local --dry-run=extract --report-type=file --report-path=renovate-report.json
```

Run it in the repository root, so the file paths in the report match the repository's.

## Add it to the pins

```sh
relgap pins . --renovate-report renovate-report.json
```

relgap scans the roots as usual and adds each dependency of the report as a pin with the detector
`renovate`. Only [`relgap pins`](/cli/pins) takes `--renovate-report`.

## What relgap reads

relgap reads `packageFiles`, at the top level or under `repositories.<name>`, and ignores every
other field. The repository name in the report becomes the pin's repository. For each
dependency it takes the name, the package name, the current value and the datasource:

| Datasource or manager | Pin |
|---|---|
| `docker` | an image pin, `image:<name>` |
| `terraform-provider` | a `tfprovider:` pin; a provider without a namespace is `hashicorp/<name>` |
| `helm` | a `chart:` pin |
| the `github-actions` manager | an action pin, `action:<name>` |
| `github-releases`, `github-tags` | a tool resolved as `github:<owner>/<repo>` |
| `aqua` | a tool resolved as `aqua:<package>` |
| anything else | a tool resolved by its package name, else its dependency name |

A dependency with a `skipReason`, without a current value or without a name is left out. Renovate
reports no line numbers, so these pins point at the file only.

A dependency name that resolves to no tool shows up in `relgap pins --unresolved`, and an alias
in the config maps it: [Map unresolved pins and private tools](/guides/unresolved).
