# relgap ADR-0017: Chart versions, extractVersion, raised floors and ignore by name

- Status: accepted (2026-11-20)
- Extends [ADR-0015](0015-renovate-annotations-and-terraform-modules.md) and
  [ADR-0016](0016-tool-identity-and-chart-registry-urls.md). Found by wiring relgap 0.10.0 into
  a Terraform module repository whose Helm chart versions sit in variable defaults under
  `# renovate:` comments.

## Context
Facts checked on 2026-11-20:

- aws/eks-charts tags every release `v0.0.N` for all its charts (`v0.0.252` changed
  aws-for-fluent-bit and aws-load-balancer-controller). Artifact Hub lists aws-for-fluent-bit's own
  versions up to 0.2.1 in `available_versions`, with a timestamp each. No tag prefix covers them,
  so relgap 0.10.0 said "no stable version on the line of 0.1.35".
- `https://aws.github.io/secrets-store-csi-driver-provider-aws` is no Artifact Hub repository.
  Its `index.yaml` lists the chart up to 3.2.0, and the packages are release assets of
  aws/secrets-store-csi-driver-provider-aws. That repository tagged
  `secrets-store-csi-driver-provider-aws-<version>` up to 2.1.1 and plain `<version>` from 3.0.0.
- chart-releaser publishes the index on `--pages-branch` (default `gh-pages`) at
  `--pages-index-path` (default `index.yaml`) (helm/chart-releaser README). GitHub's Pages API
  (`GET /repos/{owner}/{repo}/pages`) answers 404 without a token, so relgap can't ask it which
  branch serves the site.
- Renovate's helm datasource reads `<registryUrl>/index.yaml` from any host
  (`lib/modules/datasource/helm/index.ts`, read for behaviour only).
- Renovate applies `extractVersion` to every release the datasource returns, before versioning,
  and drops a release that doesn't match. The `version` named group is the version
  (`lib/modules/datasource/common.ts`, docs.renovatebot.com/configuration-options/#extractversion).
  kubernetes/autoscaler tags `cluster-autoscaler-1.37.0` next to `cluster-autoscaler-chart-9.60.0`
  and `vertical-pod-autoscaler-chart-0.17.0`, and its users write
  `extractVersion=^cluster-autoscaler-(?<version>.+)$`. Go's `regexp` accepts `(?<name>)`.
- Module authors move `>= 6.62.0` to `>= 6.72.0` in `required_providers`. Renovate's `bump` range
  strategy makes the same change. `digest --base` skipped it as "not an exact version on both
  sides".
- An ignore rule needed a tool, so an unresolved pin (a module source on Bitbucket) couldn't be
  silenced, and every run ended with "N pins have no known tool". Renovate's `ignoreDeps` matches
  names exactly, and its `matchDepNames` takes globs.

## Decision
### Chart versions without a tag per chart version
- A chart's tag prefix must cover the newest stable version the chart lists. Of the candidates
  that do, the one that covers the most versions wins, as before. The seven tag lists in
  `testdata/charttags` keep their prefixes.
- When no candidate covers it, the chart's own versions become the universe `chart`: Artifact
  Hub's `available_versions`, or a Helm repository index. `relgap resolve` shows
  `version.universe: chart` and `version.versions_url`. A registry entry can't set either.
- A `chart` tool asks GitHub for no versions. `outdated` shows latest and behind, `-` for the
  breaking and deprecation counts, and a note: "<repo> has no tag per <chart> chart version, so
  the versions come from <url> without notes; for notes, pin the application's repository too
  (datasource=github-releases)". A digest lists the versions with the same warning.

### GitHub Pages Helm repositories
- `chart:<url>/<name>` with `https://<owner>.github.io/<repo>[/<dir>]` that Artifact Hub doesn't
  list reads `<dir>/index.yaml` from the `gh-pages` branch of owner/repo through the contents API,
  so relgap contacts no new host. The chart's repository is owner/repo, unless the newest
  package URL is a GitHub release asset of another repository. Its tag prefix follows the rule
  above, with the index's versions.
- Any other URL that Artifact Hub doesn't list still fails as ADR-0016 says. relgap doesn't fetch
  `index.yaml` from arbitrary hosts, because the files it scans would then pick the hosts it
  contacts.

### extractVersion
- On `github-releases` and `github-tags`, an `extractVersion` of the form
  `^<literal>(?<version>...)$` (also `(?P<version>`, an optional trailing `v?` in the literal, the
  `$` optional) names the tool `github:<owner>/<repo>@<literal>`. The literal is the tag prefix.
  `^v(?<version>...)` and an empty literal add nothing.
- The prefix is part of the tool's name, so the pin never takes the tool a registry lists for the
  repository, and `outdated`, `drift`, ignore rules and caches keep it apart from
  `github:<owner>/<repo>`. `relgap resolve` and `relgap digest` accept the same name.
- Any other form, such as a pattern after the group or a wildcard before it, leaves the pin
  unresolved. relgap ignores `versioning` and orders versions with the tool's scheme.
- Image pins with `extractVersion` stay as they are: `outdated` doesn't compare image pins yet
  (M28).

### Raised floors in `digest --base`
- On by default. A change digests from the old bound to the new one when each side is a
  comma-separated constraint with exactly one clause using `>=`, `~>`, `^` or `~`, the operators
  match, the other clauses are equal and the bound moved. stderr says "digested from the old lower
  bound to the new one". An alternative (`||`), a hyphen range or a wildcard isn't read.
- Every other change keeps its note, now "neither an exact version on both sides nor a raised
  lower bound".

### Pin locations in digests
- Each digest of `--base` and `--pull-request` has `pins`: `repo`, `file`, `line`, `raw_name`,
  and `base` and `head` (the values as written) for every changed pin it covers.

### Ignore by name or path
- An ignore rule sets at least one of `tool`, `name` and `path`, and drops the pins that match all
  it sets. `name` is a glob on the raw name whose `*` spans any characters, slashes included, and
  ignores case. A rule with only `path` drops every pin under the glob.
- Ignored pins leave the "no known tool" count of `outdated`, `drift` and `pins`. `relgap pins`
  still lists every pin.

## Consequences
- Charts from repositories without a tag per chart version show how far behind they are. Their
  breaking changes need a second pin on the application.
- A chart repository that stopped tagging per chart reads its versions from the chart's list
  instead of tags that end before the newest version.
- Tag-prefixed applications can be annotated the way Renovate users already do.
- A `bump` pull request on a module repository gets a digest.
- The digest schema gains `pins`, the config schema gains `ignore[].name`, and `tool` is no longer
  required in an ignore rule.
