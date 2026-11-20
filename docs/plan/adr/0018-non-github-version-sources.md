# relgap ADR-0018: Versions and notes outside GitHub

- Status: accepted (2026-11-20). The owner settled the open questions at the start of M28.
- Spec: [non-github-sources.md](../specs/non-github-sources.md), which also holds the facts.

## Context
relgap's tool model has one repository field, a GitHub `owner/repo`, and every version list and
note comes from the GitHub API or a package registry that names a GitHub repository. Terraform
modules on Bitbucket Cloud and GitLab, and container images, are pinned in the same repositories
as the tools relgap already reads, and they stay unresolved or skipped.

## Decision
- A tool has a host next to its repository: `github.com` (the default), `bitbucket.org`,
  `gitlab.com` or a self-managed GitLab host from the config. `ToolSpec.Repo` keeps its meaning
  for GitHub, `ToolSpec.Host` names the other hosts, and `relgap resolve` shows both.
  `tools.<name>` takes `host` next to `repo`.
- New tool names: `bitbucket:<workspace>/<repo>`, `gitlab:<group>/<sub>/<repo>` and
  `gitlab:<host>/<group>/<repo>` for a configured host, each with an optional `@<tag-prefix>`.
- Each host gets a client in its own package (`internal/bitbucket`, `internal/gitlab`,
  `internal/oci`) that implements the interfaces the digester and the source adapters already
  use (version list, releases where the host has them, root listing, file at a ref), through
  `internal/httpx`. Plain REST, no SDK: each host needs a few endpoints, and a client library
  would bypass the shared HTTP client. No new dependency.
- Bitbucket Cloud and gitlab.com are known hosts. Requests to them happen only for pins that name
  them, as GitHub requests happen only for pins that name GitHub.

### Tokens (open question 1)
Atlassian names no environment variable, and a Bearer header works for user API tokens and for
repository, project and workspace access tokens alike (support.atlassian.com, "Using API
tokens"). glab's rules are the GitLab convention.

- `BITBUCKET_API_TOKEN` goes only to `api.bitbucket.org`, as `Authorization: Bearer`. relgap reads
  no username or email.
- `GITLAB_TOKEN`, then `GITLAB_ACCESS_TOKEN`, goes only to the host in `GITLAB_HOST` (default
  `gitlab.com`), in `PRIVATE-TOKEN`. In GitLab CI, `CI_JOB_TOKEN` goes only to `CI_SERVER_HOST`,
  in `JOB-TOKEN`, when no other token is set for that host.
- A `gitlab.hosts` entry can name its own variable with `token_env`, which then wins for that host.

### Self-managed GitLab (open question 2)
`gitlab.hosts` lists them, because a host can't be recognized by name. A git source on an
unlisted host stays unresolved: `relgap pins --unresolved` lists it, and the guide says to add the
host.

### Image registries (open questions 3 and 4)
Scanned files name the registries, so they would pick the hosts relgap contacts. relgap reads
image tags from Docker Hub, ghcr.io, registry.k8s.io, quay.io, public.ecr.aws and
mcr.microsoft.com, and from another registry only when `images.registries` lists it. It asks
every registry for anonymous tokens only and reads no Docker credentials. A pin on another or a
private registry stays skipped with the reason.

The source label of an image (`org.opencontainers.image.source`, then `org.label-schema.vcs-url`)
names the GitHub repository whose notes the digest reads. Without one, the image compares versions
only, like a `chart` universe tool (ADR-0017).

Only tags with the pin's variant count, as in Renovate's docker versioning. The variant is part
of the tool name as a tag pattern: `nginx:1.27.0-alpine` is `image:nginx:*-alpine`, and a Renovate
`extractVersion` of `^v?(?<version>.+)-alpine$` gives the same pattern.

### Other git hosts (open question 5)
No `git ls-remote` fallback. relgap never runs git against a URL a scanned file chose, and the
module stays unresolved.

## Consequences
- `docs/concepts/network.md` stops being a fixed list of hosts and becomes a list of host kinds
  with the rule above.
- The cassette scrubber drops `PRIVATE-TOKEN` and `JOB-TOKEN` next to `Authorization`, and the HTTP
  cache key records whether any of them was sent.
- Unresolved Bitbucket and GitLab module pins and skipped image pins become comparable.
- Reading an image's source label costs one manifest request, which counts as a Docker Hub pull.
