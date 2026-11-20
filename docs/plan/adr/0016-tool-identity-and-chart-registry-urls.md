# relgap ADR-0016: Tool identity in caches and chart registry URLs

- Status: accepted (2026-11-20)
- Changes the "Helm repository URLs" part of
  [ADR-0015](0015-renovate-annotations-and-terraform-modules.md): a chart pin whose URL Artifact
  Hub doesn't list no longer falls back to `chart:<name>`.

## Context
A Terraform module repository annotated a chart and its application in one file:

```hcl
# renovate: datasource=helm depName=aws-for-fluent-bit registryUrl=https://aws.github.io/eks-charts
a = optional(string, "0.1.35")
# renovate: datasource=github-releases depName=aws/aws-for-fluent-bit
b = optional(string, "3.5.0")
```

relgap 0.10.0 reported the `github:` pin as `LATEST 0.0.252, BEHIND 0` in about two runs of
three. Both tools resolve to a spec named `aws-for-fluent-bit`, one in aws/eks-charts and one in
aws/aws-for-fluent-bit. The digester listed each tool's releases once, keyed by the spec's name,
so whichever goroutine came first filled the list for both. `outdated` built each (tool, from)
digest once under the same name, and the batched lookup of the newest releases kept one answer per
repository even when one tool asked for tags and another for releases.

A second pin, `datasource=helm depName=secrets-store-csi-driver-provider-aws
registryUrl=https://aws.github.io/secrets-store-csi-driver-provider-aws`, failed with "ambiguous
name: 2 Artifact Hub repositories". Checked on 2026-11-20:
`artifacthub.io/api/v1/repositories/search?url=<that URL>` returns `[]`, and the name search
returns two unrelated repositories (portefaix-hub and a personal one). ADR-0015 let a pin whose
URL Artifact Hub doesn't list fall back to `chart:<name>`, which searched by name and could have
picked one of them.

## Decision
- A tool's identity is its full name as pins carry it (`chart:aws/aws-for-fluent-bit`,
  `github:aws/aws-for-fluent-bit`), never the resolved spec's `name`. Caches key on what they
  fetch: a version list by repository and universe (releases or tags), a digest by tool name and
  start version, and the newest-releases lookup by repository and universe.
- A chart name can carry its Helm repository URL: `chart:<url>/<name>`, such as
  `chart:https://charts.jetstack.io/cert-manager` or
  `chart:oci://public.ecr.aws/karpenter/karpenter`. The chart is the part after the last `/`.
  It resolves through the Artifact Hub repository at the URL (the lookup ADR-0015 describes) and
  never searches by the chart's name.
- `outdated`, `digest --base` and `--pull-request` rename a chart pin with a `registry_url` to
  `chart:<repo>/<name>` when Artifact Hub has the chart at that URL, as before. Otherwise the pin
  becomes `chart:<url>/<name>`, and its resolution fails with "registryUrl <url> is not an
  Artifact Hub repository with a chart named <name>", or with the lookup's error. The row is
  skipped with that note.
- `relgap resolve` and `relgap digest` accept `chart:<url>/<name>` too.

## Consequences
- A chart and its application can sit in one file, and each compares against its own repository.
- A pin whose registry URL Artifact Hub doesn't list stops resolving to whatever chart of that
  name the search finds. It is skipped with a reason until relgap can read the chart's versions
  another way.
- `drift` still groups chart pins by `chart:<name>`, because it makes no request.
