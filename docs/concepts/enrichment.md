---
description: How relgap adds end-of-life dates from endoflife.date and known vulnerabilities from OSV to the pins relgap outdated compares, and what it does when either source is down.
---

# EOL and vulnerabilities

Release notes say what changes when you upgrade. They don't say whether the version you run still
gets fixes, or whether it has a known vulnerability. [`relgap outdated`](/cli/outdated) adds both
from two public sources: release cycles from [endoflife.date](https://endoflife.date) and
advisories from [OSV](https://osv.dev). EOL needs a curated registry entry that names an
endoflife.date product; [Supported tools](/reference/tools) lists which. Every pin's tool can get
advisories, from its curated entry or from its backend.

## End of life

An entry's `eol` block names the endoflife.date product and how a version maps to its cycle:

```yaml
eol: { product: kubernetes, cycle: "{major}.{minor}" }
```

The cycle template takes the [source placeholders](/concepts/sources#templates), and an empty one
means `{major}.{minor}`. kubectl 1.34.2 is in cycle 1.34 of the kubernetes product. relgap fetches
the product's cycles once per run and looks up the cycle of CURRENT, the version the pin stands
for.

The EOL column then says yes, no, or ? when endoflife.date doesn't list the cycle. `--format json`
also has the cycle's end date, the newest release in the cycle, and the same state for LATEST's
cycle, so you can see whether the upgrade lands on a supported line.

## Vulnerabilities

An entry's `osv` list names the packages that carry the tool's advisories, each with an OSV
ecosystem: `Go` and `k8s.io/kubernetes` for kubectl, `npm` or `PyPI` for others. The `GIT`
ecosystem takes the repository URL as the name and queries the version's tag, for projects whose
advisories name commits instead of package versions. The `go` entry queries `stdlib` and
`toolchain`, the `go` command.

A tool without an `osv` list gets its packages from its backend, the way `mise audit` maps them:

| Backend | OSV queries |
|---|---|
| `npm:<name>` | `npm` / name |
| `pypi:`, `pipx:` | `PyPI` / the PEP 503 name, extras dropped |
| `cargo:<name>` | `crates.io` / name |
| `gem:<name>` | `RubyGems` / name |
| `dotnet:`, `nuget:` | `NuGet` / name |
| `go:<path>` | `Go` / the longest prefix of the path that OSV has records of |
| a github.com repository (`github:`, `aqua:`, `tfprovider:`) | `Go` / `github.com/<owner>/<repo>`, with `/vN` from major 2 on, and `GIT` / the repository URL at the tag |
| `gitlab:<path>` on gitlab.com | `GIT` / the repository URL at the tag |

A tool known by name takes the first backend of its mise registry entry. Tools on Bitbucket or a
self-managed GitLab host, images on a private registry, and package names with `:` or `@` after
the scope are never sent to OSV. Charts, `tfmodule:` pins and images have no OSV source.

relgap sends one batch of queries for CURRENT of every pin, fetches each advisory once, and then
checks the advisory's affected ranges itself:

- An advisory affects a version that its versions list names or that falls inside an
  introduced..fixed (or ..last_affected) range.
- When OSV returns an advisory but relgap can't order its range locally, such as a range of git
  commits, the advisory counts as affecting CURRENT.
- The lowest fixed version above CURRENT is reported with each advisory, and so is whether LATEST
  is outside every affected range. The VULNS column counts the advisories of CURRENT and how many
  of them the upgrade to LATEST fixes.
- One vulnerability that several databases describe, such as a GO- and a GHSA- ID of the same
  issue, is listed once.

A Go module whose path ends in `/v2` or higher belongs to that major version only. An entry can
list `github.com/owner/mod` and `github.com/owner/mod/v2`, and a pin of 2.x queries only the second.

## Severity

Each advisory gets the highest base score among its CVSS vectors (v2, v3.0, v3.1 or v4.0), the
way osv-scanner reads it. Without a vector, relgap takes the database's own rating, such as
GitHub's MODERATE or HIGH. Records with neither, which includes the Go vulnerability database,
have no rating.

The rating follows the CVSS bounds: low from 0.1, medium from 4.0, high from 7.0, critical from
9.0. `--vuln-severity` (config `policy.vuln_severity`) sets the lowest rating that
`--fail-on vuln` counts. Empty counts every advisory; with a value set, an advisory without a
rating doesn't count. The [SARIF output](/reference/sarif-rules#vulnerabilities) turns the rating
into the alert level.

## When a source is down

Both sources are cached for a day, so repeated runs ask once. When endoflife.date or OSV fails,
relgap warns and prints the rest of the report with that column empty. A gate that depends on the
missing data can't pass on it: under `--fail-on eol` an endoflife.date outage exits 1, and so does
an OSV outage under `--fail-on vuln`, unless another value matched and exited 3.
[Exit codes](/reference/exit-codes) has the full order.

## Installed binaries

`relgap outdated --binaries` looks one step further. It finds the installed executables of each
pinned tool, on `PATH` and in mise's installs directory (following mise shims), and reads the Go
build info of each. The Go version becomes a query for the standard library, and the main module
a query for its own advisories, unless it was built as `(devel)`. This is govulncheck's binary
mode without the check that the binary calls the vulnerable code, so a listed advisory may not
reach any code the tool runs. An entry's `bin` list names the executables when they differ from
the tool name.

```sh
relgap outdated --binaries --format json
```
