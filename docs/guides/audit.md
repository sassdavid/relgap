---
description: List the known vulnerabilities of every pinned tool version across your repositories, see which pins OSV couldn't check, gate CI on the findings, accept risks with vulns.ignore or OpenVEX, and add CISA KEV and EPSS.
---

# Audit pinned versions for vulnerabilities

`relgap audit` asks OSV about the version of every pin under one or more directories and lists
the advisories that affect them. Unlike `relgap outdated`, it reports vulnerabilities only, reads
a whole tree of repositories the way `relgap drift` does, and fails by default when it finds one.
It also lists every pin it couldn't check, so a clean report means OSV was asked about everything
it shows.

## Run it

```sh
relgap audit
relgap audit ~/src/infra ~/src/apps
relgap audit --tool terraform --tool kubectl
```

The roots default to `scan.roots` in the config, then the current directory. relgap finds every
git repository under them, and a root that is itself a project counts as one repository.
`--tool` limits the run to the tools you name.

relgap sends one query per tool, version and OSV package, however many repositories pin it. The
output below comes from a test tree of 20 repositories, where every repository pins terraform
1.5.0 and OSV still gets one query for it:

```sh
relgap audit tree --format md
```

<<< @/../testdata/golden/audit/tree.md

## Read the report

Each row is a finding: one advisory that affects one tool version. PINS names the first pin of
that version and counts the others. SEVERITY is the rating and its CVSS score. A Go
vulnerability database record without a rating takes the rating of its GHSA alias, and a record
with neither shows `unknown`. FIXED IN is the lowest version above the pinned one that fixes the
advisory, and `-` means no fixed version is known.

The sections under the table hold the rest:

- "Advisories" gives each advisory's aliases, summary and osv.dev page.
- "Fix targets" names one release per tool version that clears every advisory with a fix. It
  says when the target is outside the pinned line, when no release fixes an advisory, and when
  the latest release is still affected.
- "Not checked for vulnerabilities" lists the pins OSV wasn't asked about, with the reason.
- "Suppressed" lists the advisories you accepted, with your reason.

`--format json` has the same data for scripts. `checked` counts the tool versions OSV answered
for, and each finding has its `fix` object:

```sh
relgap audit --format json | jq '.data.findings[] | {tool, version, id, fix}'
```

[EOL and vulnerabilities](/concepts/enrichment) explains the severity rules, the records relgap
drops and how it picks a fix target.

## What relgap checks

A tool whose registry entry has an `osv` list queries those packages. Every other pin gets its
OSV package from its backend, as `mise audit` maps them: `npm:`, `pypi:`, `cargo:`, `gem:`,
`dotnet:` and `go:` name their package, and a tool on github.com or gitlab.com queries its Go
module and its repository. The [coverage table](/concepts/enrichment#vulnerabilities) lists every
mapping.

An exact pin is checked at its own version. A value marked ≈ is checked at the version it stands
for, the CURRENT of `relgap outdated`: `node = "22"` at the newest 22.x release, a constraint at
the version that satisfies it. GitHub Action pins are checked when `scan.detectors.ci_actions` is
on. A pin by commit SHA is checked at the version its line comment names, such as `# v4.1.3`.

## Pins it can't check

An unchecked pin never counts as clean. Each has one reason:

| Reason | Example |
|---|---|
| `no-advisory-source` | an image, a chart or a `tfmodule:` pin; OSV has no records for them |
| `private-source` | a tool on Bitbucket or a self-managed GitLab host, an image on a private registry, a private repository you named with `github:` or `tools.<name>.repo`, a module `GOPRIVATE` names, an npm package from your own registry |
| `not-exact` | `node = "lts"`, a branch, a commit SHA without a version comment |
| `unresolved` | a name relgap can't map to a tool ([Map unresolved pins](/guides/unresolved)) |
| `offline` | `--offline` and no cached answer |
| `failed` | OSV, the version lookup or the repository's visibility lookup failed; the warnings name the request |

`--fail-on unchecked` turns them into findings. A pin is `unresolved` until you map it, and an
image stays `no-advisory-source`, so drop the pins you never want checked with an
[`ignore`](/reference/config#ignore) rule first.

## Gate CI on the findings

`relgap audit` gates `vuln` without any flag: a finding exits 3, and the report is still written.
The `--fail-on` flag replaces that default:

```sh
relgap audit --fail-on=
relgap audit --fail-on unchecked
relgap audit --fail-on vuln --fail-on unchecked
```

The first line reports without gating, the second gates only unchecked pins, the third both. An
`audit` value in `policy.fail_on` adds to `vuln`, so a shared config with `fail_on: [eol, unchecked]`
makes `audit` gate `vuln` and `unchecked`.

Two flags narrow `vuln`. `--vuln-severity high` counts advisories rated high or critical, and
those of unknown severity, since nothing says they are lower. `--vuln-fixable` counts only the
advisories with a fixed version above the pinned one:

```sh
relgap audit --vuln-severity high --vuln-fixable
```

When a file, a version lookup or OSV fails and no value matched, `audit` exits 1, because a
pass on partial data proves nothing. [Gate CI on breaking changes, EOL and vulnerabilities](/guides/ci-gate) has the exit
codes, `--min-count` and the `policy` keys.

To run it in a workflow, take the one in [Gate CI](/guides/ci-gate#a-workflow) and swap the relgap
step for this one, with `category: relgap-audit` for the SARIF upload and
`relgap notify --from audit.json`:

```sh
relgap audit --format github --out sarif=audit.sarif --out md=audit.md --out json=audit.json
```

`--format github` prints one annotation per finding at the pin's file and line, and the SARIF
goes to code scanning ([Upload SARIF to code scanning](/guides/sarif)). `relgap notify` comments
on the pull request when the report has a finding or an error, or a pin failed, was offline or
didn't resolve.

## Accept a risk

Some advisories don't apply to how you run a tool. `vulns.ignore` in the config suppresses one by
its ID or any alias, with a reason that the report repeats:

```yaml
# .relgap.yaml
vulns:
  ignore:
    - id: GO-2024-3080
      reason: kanister runs only in the test cluster
      tools: ["github:kanisterio/*"]
    - id: GHSA-3wgm-2gw2-vh5m
      reason: no cluster mounts a gitRepo volume
      until: 2026-12-31
    - id: CVE-2024-7598
      reason: waiting for the network policy rollout
      until: 2026-11-01
    - id: GHSA-cxww-7g56-2vh6
      reason: download-artifact is pinned to 4.1.3
```

`tools` limits an entry to tool names that match its globs. `until` ends it after that day in
UTC. On a project that pins kubectl 1.37.2 and kanister 0.120.0, run on 2026-11-23:

```sh
relgap audit --format plain
```

<<< @/../testdata/golden/audit/suppressed.txt

The CVE-2024-7598 entry has expired, so GO-2025-3547 is a finding again and relgap warns about the
entry. The GHSA-cxww-7g56-2vh6 entry matched nothing, and relgap warns about it as well, so stale
entries show up. A suppressed advisory doesn't count for `--fail-on`, doesn't hold back the fix
target, and stays in every format with its reason. `relgap outdated` applies the same entries.

### OpenVEX

`--vex` reads OpenVEX v0.2.0 documents, such as the ones govulncheck writes or a team keeps next
to its config. A `not_affected` or `fixed` statement suppresses the advisory for the products it
names:

```json
{
  "@context": "https://openvex.dev/ns/v0.2.0",
  "@id": "urn:uuid:2f4e9c1a-6b4d-4c55-9b0e-1d2a3c4b5e6f",
  "author": "platform team",
  "timestamp": "2026-11-20T09:00:00Z",
  "version": 1,
  "statements": [
    {
      "vulnerability": { "name": "CVE-2024-7598" },
      "products": [{ "@id": "pkg:golang/k8s.io/kubernetes@v1.37.2" }],
      "status": "not_affected",
      "justification": "vulnerable_code_not_in_execute_path"
    }
  ]
}
```

```sh
relgap audit --vex vex/kubectl.json
```

`vulns.vex` in the config lists documents too, relative to the config file, and `--vex` replaces
that list. A product names a pin by its purl: `pkg:golang/<module>@v<version>`,
`pkg:npm/<name>@<version>`, `pkg:github/<owner>/<repo>@<tag>` or
`pkg:githubactions/<owner>/<repo>@<tag>`. A purl without a version covers every version. When an
ignore entry and a statement both match, the ignore entry wins.

`--format openvex` writes the report as an OpenVEX document, one statement per advisory and tool
version. Findings are `affected`, with the fix target as the action statement, and suppressed
advisories keep the status that suppressed them. Grype and Trivy read it with their own `--vex`:

```sh
relgap audit --format openvex > audit.vex.json
```

A report without findings or suppressed advisories writes an empty `statements` list, which the
OpenVEX schema doesn't accept.

## Find the advisories attackers use

`--kev` marks the findings in the CISA Known Exploited Vulnerabilities catalog, and `--epss` adds
each finding's FIRST EPSS score, the probability of exploitation in the next 30 days:

```sh
relgap audit --kev --epss
relgap audit --kev --fail-on kev
```

The table gets a KEV column, with the date CISA added the CVE and `ransomware` when CISA knows of
ransomware use, and an EPSS column with the score and its percentile, such as `99.7% (99.9th)`.
Findings in the catalog sort first, then by severity, then by EPSS score. `--fail-on kev` counts
the findings in the catalog whatever their rating. Without KEV on, it is a usage error that
exits 2 before any request.

Both are off by default, and `vulns.kev` and `vulns.epss` turn them on in the config. With `--kev`
relgap downloads the whole catalog from `www.cisa.gov` and matches it locally. With `--epss` it
sends the CVE IDs of the findings to `api.first.org`. When a feed is down, relgap warns and the
report goes on without that column.

## Work offline

`--offline` answers from the HTTP cache. A pin without a cached OSV answer is unchecked with
reason `offline`, and the JSON says when the oldest answer it used was stored:

```sh
relgap audit --offline --format json | jq '.data.oldest_answer'
```

[Work offline and manage the cache](/guides/cache) says how long each answer stays fresh.

## From an agent or a saved report

The MCP server has an `audit` tool with `roots`, `tools` and `vuln_severity`
([Use relgap from an MCP client](/guides/mcp)). `relgap notify --from audit.json` posts a saved
JSON report as a pull request comment ([notify](/reference/notify)).

OSV receives package names, versions and the URLs of github.com and gitlab.com repositories,
private ones included. [What OSV receives](/concepts/network#what-osv-receives) lists every value
and how to keep a tool out.
