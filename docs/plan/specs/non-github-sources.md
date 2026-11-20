# Spec: versions and notes outside GitHub (M28)

relgap reads every version and every note from GitHub, Artifact Hub, the Terraform registry, npm
and PyPI today. Two kinds of pins found on 2026-11-20 resolve to no tool:

- Terraform module sources on other git hosts:
  `source = "git::git@bitbucket.org:<ws>/<repo>.git//.?ref=2.1.0"` and
  `git::https://gitlab.com/<group>/<sub>/<repo>.git?ref=v1.0.0`. `relgap pins --unresolved` lists
  them, and `tools.<name>.repo` takes only a GitHub `owner/repo`.
- Image pins (`image:` from Dockerfiles and `# renovate: datasource=docker` comments).
  `outdated` skips them.

Decisions are proposed in [ADR-0018](../adr/0018-non-github-version-sources.md). The open
questions there must be settled with the owner before the code starts.

## Facts (checked 2026-11-20)

### Bitbucket Cloud
- Tags: `GET https://api.bitbucket.org/2.0/repositories/{workspace}/{repo_slug}/refs/tags`. The
  page holds `values`, `pagelen`, `size`, `page` and `next` (a full URL, absent on the last
  page). `pagelen` is at most 100 (101 returns "Invalid pagelen"). Without `sort` the order is
  by name; `sort=-target.date` works, and `fields=values.name,values.target.date,next` trims the
  page.
- Files: `GET /2.0/repositories/{workspace}/{repo_slug}/src/{ref}/{path}` returns the raw bytes. A
  ref with a `/` is ambiguous in that path, so resolve such a tag to `target.hash` first.
- Bitbucket Cloud has no releases. A changelog file in the repository is the only source of notes.
- Auth (support.atlassian.com, "Using API tokens", "Using access tokens", "API request limits"):
  repository, project and workspace access tokens go in `Authorization: Bearer`. User API tokens
  take Basic auth with the Atlassian account email, or Bearer. App passwords stopped working after
  2026-07-28. Anonymous requests get 60 an hour per IP; a private or missing repository answers
  404.
- Renovate's terraform manager maps `git::https://bitbucket.org/<ws>/<repo>.git` to its
  `bitbucket-tags` datasource and the scp form `git@bitbucket.org:` to `git-tags`
  (`lib/modules/manager/terraform/extractors/others/modules.ts`, read for behaviour only).

### GitLab
- Tags: `GET {host}/api/v4/projects/{id}/repository/tags`, where `{id}` is the URL-encoded full
  path (`group%2Fsub%2Frepo`). `order_by` takes `name`, `updated` or `version`, `per_page` is at
  most 100, and pages come with a `Link` header, `x-next-page` and `x-total-pages`.
- Releases: `GET {host}/api/v4/projects/{id}/releases`, with `tag_name`, `description`
  (Markdown), `released_at` and `upcoming_release`.
- Files: `GET {host}/api/v4/projects/{id}/repository/files/{url-encoded path}/raw?ref={ref}`.
- Auth (docs.gitlab.com/api/rest/authentication/, docs.gitlab.com/ci/jobs/ci_job_token/): a
  personal, project or group token goes in `PRIVATE-TOKEN` (or `Authorization: Bearer`) and needs
  `read_api`. In CI, `JOB-TOKEN: $CI_JOB_TOKEN` reads the tags, releases and files of public and
  internal projects, and of private ones that allow the calling project. glab reads
  `GITLAB_TOKEN`, then `GITLAB_ACCESS_TOKEN`, and sends it only to `GITLAB_HOST`.
- gitlab.com allows 500 unauthenticated API requests a minute per IP and 2,000 authenticated ones
  per user (docs.gitlab.com/user/gitlab_com/rate_limits/).
- A self-managed host can't be told from its name. glab and Renovate take the host from config.

### OCI image registries
- Tags: `GET https://<registry>/v2/<repository>/tags/list`, paged with `Link: rel=next`, after an
  anonymous token from the `WWW-Authenticate` challenge. Docker Hub (`index.docker.io`, with
  `library/` for one-part names) also has `hub.docker.com/v2/repositories/<repo>/tags`, which adds
  timestamps.
- The source repository comes from the image's `org.opencontainers.image.source` label (manifest
  annotation or config label), then `org.label-schema.vcs-url`. That is how Renovate's docker
  datasource finds release notes (`lib/modules/datasource/docker/`, read for behaviour only).
- `extractVersion` applies to image tags as it does to GitHub tags.

## Proposed behaviour

- New tool names: `bitbucket:<workspace>/<repo>`, `gitlab:<group>/<sub>/<repo>` for gitlab.com
  and `gitlab:<host>/<group>/<repo>` for a configured self-managed host. Both take
  `@<tag-prefix>` like `github:`.
- The `terraform` and `terragrunt` detectors give these pins for Bitbucket and GitLab git sources
  with `?ref=`, with the same ref rules as GitHub (`x.y.z` or `vx.y.z`; anything else is a channel
  pin). A `# renovate:` comment with `bitbucket-tags`, `gitlab-tags`, `gitlab-releases` or
  `git-tags` on one of these hosts gives the same pin.
- `tools.<name>` can name a repository on these hosts, so a private tool can point there.
- Versions come from the host's tags API through `internal/httpx` (retries, rate limits, ETag
  cache, `--offline`). A GitLab tool's notes come from its releases, then from auto-discovered
  changelog files, as for GitHub. A Bitbucket tool's notes come from changelog files only.
- `image:` pins compare in `outdated` against the registry's tags. Their notes come from the
  GitHub repository the source label names, when there is one.
- Hosts are opt-in where the scanned files would otherwise choose them: see the ADR.
- `docs/concepts/network.md` lists every new host and token, and `docs/reference/environment.md`
  the new variables.

## Acceptance criteria
- [ ] A Bitbucket Cloud module source with `?ref=2.1.0` gives a `bitbucket:` pin that `outdated`
  compares, and `digest` reads a Keep a Changelog `CHANGELOG.md` from the repository. Cassettes
  cover pagination, a 404 for a private repository without a token, and a 429 that succeeds on
  retry. (`TestSpec_non_github_sources_1`)
- [ ] A gitlab.com module source with nested groups gives a `gitlab:` pin; `digest` reads its
  releases, and a self-managed host works once configured. (`TestSpec_non_github_sources_2`)
- [ ] Tokens go only to the host they belong to, never into the cache or a cassette, and a
  missing token gives the anonymous limits with a warning when they run low.
  (`TestSpec_non_github_sources_3`)
- [ ] `outdated` compares `image:` pins from Docker Hub, ghcr.io, registry.k8s.io and
  public.ecr.aws against their tags, honours `extractVersion`, and digests the source label's
  GitHub repository. (`TestSpec_non_github_sources_4`)
- [ ] `relgap pins --unresolved` no longer lists the sources of the two kinds above, and
  `docs/concepts/network.md` names every host and token they use.
