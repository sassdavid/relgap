---
description: How relgap adds end-of-life dates from endoflife.date, known vulnerabilities from OSV and the opt-in CISA KEV and EPSS feeds to the pins relgap outdated compares, and what it does when a source is down.
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
| an action pin (`scan.detectors.ci_actions`) | `GitHub Actions` / `<owner>/<repo>` without a version, once per action, its ranges matched by relgap |

A tool known by name takes the first backend of its mise registry entry. Tools on Bitbucket or a
self-managed GitLab host, images on a private registry, and package names with `:` or `@` after
the scope are never sent to OSV. Charts, `tfmodule:` pins and images have no OSV source.

relgap sends one batch of queries for CURRENT of every pin, fetches each advisory once, and then
checks the advisory's affected ranges itself. A pin marked ≈ is checked at the version CURRENT
resolved to, the way `mise audit` resolves `node = "22"` and pip-audit resolves a range. An exact
pin whose version lookup failed is checked at its own version.

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

## Unchecked pins

A pin that relgap didn't ask OSV about is never counted as clean. VULNS shows `?` for it, and `0`
only for a checked pin without advisories. Each unchecked pin has one reason:

| Reason | When |
|---|---|
| `no-advisory-source` | No curated `osv` list and no backend or repository OSV knows: images, charts, `tfmodule:` pins, tools without a repository |
| `private-source` | A host or registry relgap never sends to OSV: Bitbucket, a self-managed GitLab host, a private image registry, a registry that wants an account |
| `not-exact` | The pin resolves to no single version: a channel other than `latest` (`lts`), a branch, a commit SHA without a version comment, a value that isn't a version |
| `unresolved` | The pin resolves to no tool |
| `offline` | `--offline` and no cached answer |
| `failed` | The version lookup or OSV failed for this pin; the warnings and the JSON `errors` name the request |

The table and Markdown list them after the notes, under "Not checked for vulnerabilities".
`--format json` gives each pin an `unchecked` field and lists them in `data.unchecked`, one entry
per tool, version and reason with every pin location:

```sh
relgap outdated --format json | jq '.data.unchecked[] | select(.reason != "no-advisory-source")'
```

SARIF has a note-level [`unchecked`](/reference/sarif-rules) result per pin.

## Severity

Each advisory gets the highest base score among its CVSS vectors (v2, v3.0, v3.1 or v4.0), the
way osv-scanner reads it. Without a vector, relgap takes the database's own rating, such as
GitHub's MODERATE or HIGH. Go vulnerability database records have neither, but list their GitHub
advisory as an alias: GO-2024-2687 names GHSA-4v7x-pqxf-cx7m. relgap fetches each `GHSA-` alias
once and takes its highest score, and `severity.from` in JSON names that alias. A record still
without a rating has severity `unknown`, shown as `unknown` in the table.

The rating follows the CVSS bounds: low from 0.1, medium from 4.0, high from 7.0, critical from
9.0. `--vuln-severity` (config `policy.vuln_severity`) sets the lowest rating that
`--fail-on vuln` counts. Empty counts every advisory. An advisory of unknown severity counts at
every threshold, in `audit` and in `outdated`. The
[SARIF output](/reference/sarif-rules#vulnerabilities) turns the rating into the alert level, and
gives unknown severity level `warning`.

## Records

relgap drops a record marked `withdrawn` and logs it at debug level. OSV's batch query returns
each record's `modified` time, and relgap caches the record under that time: a record whose
`modified` didn't change stays cached for 7 days, and a changed one is fetched again. A `GHSA-`
alias, fetched without a batch answer, is cached for a day like the other OSV answers.

## Fix targets

relgap never edits a pin, so it names the release to move to, the way `mise upgrade --vulnerable`
picks one. A tool version with a vulnerability lists its tool's versions, and the candidates are
the stable releases above it. `lowest` is the first candidate that no advisory with a fix
affects, checked against the advisories' ranges. `lowest_in_line` is the same within the
version's line: its major, or major.minor for tools with `version.line: minor`. relgap then asks
OSV about `lowest`, `lowest_in_line` and the latest release, for every package of the tool (core
`go` needs both `stdlib` and `toolchain` clean). A candidate with an advisory of its own moves the
target to the next release, for at most three batch queries.

An advisory the latest release still has is one no release fixes. It is listed in `unfixed`, and
in `audit` its finding has `fix: null`. When the latest release has any advisory, relgap warns
on stderr and sets `latest_affected`. A version list or OSV that fails leaves `fix` out and
warns. A clean exact pin never lists versions.

```sh
relgap audit --format json | jq '.data.findings[] | {tool, version, id, fix}'
```

The table and Markdown list one line per tool version under "Fix targets":

```text
kubectl 1.33.5: upgrade to 1.33.6; no release fixes GO-2025-3521, GO-2025-3547; latest 1.37.2 still has GO-2025-3521, GO-2025-3547
```

SARIF and `--format github` messages add "upgrade to 1.33.6", "latest 1.37.2 still affected" or
"no release fixes it". `relgap outdated` puts the same object in each pin's `fix`.

`--vuln-fixable` (config `policy.vuln_fixable`) makes `--fail-on vuln` count only advisories with
a fixed version above the pinned one, judged per advisory like Grype's `--only-fixed` and Trivy's
`--ignore-unfixed`. An advisory without one, including one with only git commit ranges, doesn't
count and still shows in the report.

## Known exploited and likely exploited

Two opt-in feeds say which advisories attackers use. Both work on the advisory's CVE aliases, in
`relgap outdated` and `relgap audit`:

| Flag | Config | Source | Adds |
|---|---|---|---|
| `--kev` | `vulns.kev` | the CISA [Known Exploited Vulnerabilities](https://www.cisa.gov/known-exploited-vulnerabilities-catalog) catalog, one file from `www.cisa.gov` | `kev`: the CVE, `date_added`, `due_date` and `ransomware` |
| `--epss` | `vulns.epss` | FIRST [EPSS](https://www.first.org/epss/), the CVE IDs sent to `api.first.org` | `epss`: the CVE, `score` (the probability of exploitation in the next 30 days), `percentile` and `date` |

An advisory with several CVE aliases takes the earliest catalog entry and the highest EPSS score.
Advisories in the catalog sort first, then by severity, then by EPSS score. `relgap audit` shows a
KEV and an EPSS column, VULNS in `relgap outdated` counts catalog entries as `1 KEV`, and SARIF
results carry `kev` and `epss` as properties, with the catalog entry in the message. The SARIF level
stays the one the CVSS rating gives.

`--fail-on kev` counts the unsuppressed advisories in the catalog, whatever their rating, and needs
`--kev`. Grype shows KEV and EPSS too, from its database, and sorts by a risk score built from
both; relgap keeps the order above and leaves gating on an EPSS threshold to you.

## When a source is down

The sources are cached for a day, so repeated runs ask once. When endoflife.date or OSV fails,
relgap warns and prints the rest of the report. The EOL column stays empty, and VULNS shows `?`
with reason `failed`. A KEV or EPSS outage is a warning and an entry in the JSON `errors`, and the
report goes on without that field. A gate that depends on the missing data can't pass on it:
under `--fail-on eol` an endoflife.date outage exits 1, and so do an OSV outage under
`--fail-on vuln` and a KEV outage under `--fail-on kev`, unless another value matched and exited 3.
An EPSS outage never changes the exit code.
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
