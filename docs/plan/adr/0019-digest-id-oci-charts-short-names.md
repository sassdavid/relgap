# relgap ADR-0019: Digest ids, charts in OCI registries and short chart names

- Status: accepted (2026-11-20)
- Extends [ADR-0016](0016-tool-identity-and-chart-registry-urls.md),
  [ADR-0017](0017-chart-versions-extract-version-and-floors.md) and
  [ADR-0018](0018-non-github-version-sources.md). Found by wiring relgap 0.11.0 into a Terraform
  module repository.

## Context
Facts checked on 2026-11-20:

- `digest --base` JSON put the spec's short name in `tool` (`aws-for-fluent-bit`), while
  `outdated` puts the tool's identity there (`chart:aws/aws-for-fluent-bit`). After ADR-0016 a
  chart and its application can both be named `aws-for-fluent-bit`, so two digests had the same
  label and couldn't be matched to `outdated` rows. relgap-action doesn't read the field.
- ADR-0016 stopped a `registryUrl` that Artifact Hub doesn't list from falling back to a search by
  name. So `datasource=helm depName=cert-manager registryUrl=oci://quay.io/jetstack/charts`,
  which 0.10 resolved by name, failed in 0.11.
- Renovate resolves a chart in an OCI registry with its docker datasource, as the image
  `<registry>/<path>/<chart>` (`lib/modules/manager/helmv3/oci.ts`, read for behaviour only).
- `helm push` writes the chart's first `sources` entry as the manifest annotation
  `org.opencontainers.image.source`, next to `title`, `version`, `url` (the chart's `home`) and
  `created` (`generateChartOCIAnnotations` in helm/helm `pkg/registry/chart.go`). Helm can't
  write `+` in a tag, so a version's `+` is `_` in the registry.
- relgap 0.11 has an OCI client for the image allow list: Docker Hub, ghcr.io, registry.k8s.io,
  quay.io, public.ecr.aws, mcr.microsoft.com and `images.registries` (ADR-0018). It reads the
  source annotation of an image's manifest.
- A chart named by a GitHub Pages URL kept the URL in its name, so
  `chart:https://aws.github.io/secrets-store-csi-driver-provider-aws/secrets-store-csi-driver-provider-aws`
  made the `outdated` table twice as wide.

## Decision
The user chose each option on 2026-11-20.

### Digest ids
- Each digest gains `id`: the tool's identity, which `outdated` prints in `tool` and
  `relgap digest` accepts. `tool` keeps the short name. The change adds a field to `digest-v1`
  and breaks no consumer.

### Charts in OCI registries
- A `chart:oci://<registry>/<path>/<chart>` that Artifact Hub doesn't list reads the tags of the
  image `<registry>/<path>/<chart>` through the OCI client, with the same allow list and anonymous
  tokens as images. `_` in a tag is read as `+`. A registry off the list fails with a hint to add it
  to `images.registries` and gets no request.
- The source annotation of the newest stable version names the chart's repository. With a GitHub
  repository, ADR-0017's tag prefix rule decides between notes and the universe `chart`.
- Without a GitHub source the chart still gets the universe `chart`, with no repository:
  `outdated` shows latest and behind with `-` counts and a note, and a digest lists the versions.

### Short chart names
- Tables (text and Markdown of `outdated`, `drift` and `pins`) show a chart named by URL in a
  short form: `chart:<owner>/<repo>[/<dir>]/<chart>` for a GitHub Pages site and
  `chart:<registry>/<path>/<chart>` for `oci://`. Any other URL stays as it is.
- `relgap resolve` and `relgap digest` accept the short forms, so a name copied from a table
  works. A first part with a dot or a port is a registry. Three or more parts without one are a
  GitHub Pages site. Two parts without one stay an Artifact Hub repository and chart, whose
  repository names have no dots.
- JSON, SARIF and the digest `id` keep the URL form, which stays the tool's identity.

## Consequences
- Consumers can join digests to `outdated` rows and tell a chart from its application.
- oci:// charts that Artifact Hub doesn't list resolve again, now from their own registry instead
  of a search by name.
- relgap may contact an allowed image registry for a chart pin, which `docs/concepts/network.md`
  lists.
- A table name and a JSON name can differ for the same chart. Both resolve to the same tool.
