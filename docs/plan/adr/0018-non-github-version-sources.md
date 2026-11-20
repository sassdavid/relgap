# relgap ADR-0018: Versions and notes outside GitHub

- Status: proposed (2026-11-20). Settle the open questions with the owner at the start of M28.
- Spec: [non-github-sources.md](../specs/non-github-sources.md), which also holds the facts.

## Context
relgap's tool model has one repository field, a GitHub `owner/repo`, and every version list and
note comes from the GitHub API or a package registry that names a GitHub repository. Terraform
modules on Bitbucket Cloud and GitLab, and container images, are pinned in the same repositories
as the tools relgap already reads, and they stay unresolved or skipped.

## Proposed decision
- A tool has a host next to its repository: `github` (the default), `bitbucket` or `gitlab`, with
  the GitLab host when it isn't gitlab.com. `ToolSpec.Repo` keeps its meaning for GitHub, and
  `relgap resolve` shows the host.
- Each host gets a client in its own package that implements the interfaces the digester and the
  source adapters already use (version list, releases where the host has them, file at a ref),
  through `internal/httpx`. Plain REST, no SDK: each host needs three endpoints, and a client
  library would bypass the shared HTTP client. No new dependency.
- Bitbucket Cloud and gitlab.com are known hosts. Requests to them happen only for pins that name
  them, as GitHub requests happen only for pins that name GitHub.
- Image registries follow the same rule: a request goes to the registry an `image:` pin names.
- The source label of an image names the GitHub repository whose notes the digest reads. Without
  one, the image compares versions only, like a `chart` universe tool (ADR-0017).

## Open questions for the owner
1. Token variables. Proposed: `BITBUCKET_TOKEN` (Bearer; access token or API token) with
   `BITBUCKET_USERNAME` for Basic auth with an API token, and `GITLAB_TOKEN` then
   `CI_JOB_TOKEN` for GitLab, as glab reads them. Alternative: a `hosts` config section that names
   the variable per host, like `llm.api_key_env`.
2. Self-managed GitLab. Proposed: `gitlab.hosts: [gitlab.example.com]` in the config, because a
   host can't be recognized by name. Should a module source on an unlisted host stay unresolved
   (proposed), or be tried as GitLab?
3. Image registries. Scanned Dockerfiles name the registries, so they would pick the hosts relgap
   contacts. Proposed: Docker Hub, ghcr.io, registry.k8s.io, quay.io, public.ecr.aws and
   mcr.microsoft.com by default, any other only when `images.registries` lists it.
4. Private registries need credentials. Proposed: read `~/.docker/config.json` only for
   registries listed in the config, never credential helpers, or leave private registries out of
   M28.
5. Should `git-tags` (plain `git ls-remote --tags`) be the fallback for any other git host? It
   needs no API, but it runs git against a URL that a scanned file chose.

## Consequences
- `docs/concepts/network.md` stops being a fixed list of hosts and becomes a list of host kinds
  with the rule above.
- The cassette scrubber learns the new auth headers (`PRIVATE-TOKEN`, `JOB-TOKEN`, Basic auth).
- Unresolved Bitbucket and GitLab module pins and skipped image pins become comparable.
