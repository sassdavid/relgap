# Spec: versions and notes outside GitHub (M28)

relgap reads every version and every note from GitHub, Artifact Hub, the Terraform registry, npm
and PyPI today. Two kinds of pins found on 2026-11-20 resolve to no tool:

- Terraform module sources on other git hosts:
  `source = "git::git@bitbucket.org:<ws>/<repo>.git//.?ref=2.1.0"` and
  `git::https://gitlab.com/<group>/<sub>/<repo>.git?ref=v1.0.0`. `relgap pins --unresolved` lists
  them, and `tools.<name>.repo` takes only a GitHub `owner/repo`.
- Image pins (`image:` from Dockerfiles and `# renovate: datasource=docker` comments).
  `outdated` skips them.

Decisions are in [ADR-0018](../adr/0018-non-github-version-sources.md), accepted on
2026-11-20.

## Facts (checked 2026-11-20)

### Bitbucket Cloud
- Tags: `GET https://api.bitbucket.org/2.0/repositories/{workspace}/{repo_slug}/refs/tags`. The
  page holds `values`, `pagelen`, `size`, `page` and `next` (a full URL, absent on the last
  page). `pagelen` is at most 100 (101 returns "Invalid pagelen"). Without `sort` the order is
  plain alphabetical, as `git tag --list` prints it (`v10`, `v11`, `v9`); `sort=-target.date`
  works, and `fields=values.name,values.target.date,next` trims the page.
- Files: `GET /2.0/repositories/{workspace}/{repo_slug}/src/{ref}/{path}` returns the raw bytes.
  The reference documents a commit hash, but a tag and a branch work too. A ref with a `/`
  answers 404, raw or as `%2F`, so resolve such a tag to `target.hash` first. A path ending in
  `/` lists a directory as JSON (`values` with `type` `commit_file` or `commit_directory`).
- Bitbucket Cloud has no releases. A changelog file in the repository is the only source of notes.
- Auth (support.atlassian.com, "Using API tokens", "Using access tokens", "API request limits"):
  repository, project and workspace access tokens go in `Authorization: Bearer`. User API tokens
  take Basic auth with the Atlassian account email, or Bearer, which needs no email. New app
  passwords stopped on 2025-09-09 and existing ones stopped working after 2026-07-28. Anonymous
  requests get 60 an hour per IP; authenticated calls to `/2.0/repositories/*` get 1,000 an hour
  per user. A private or missing repository answers 404 without a token, with the same message.
- Rate-limit headers (`X-RateLimit-Limit`, `X-RateLimit-NearLimit`) come only from scaled-limit
  endpoints, with no remaining count or reset. An anonymous 200 carries none.
- Atlassian names no environment variable for a token. bitbucket-cli uses `BITBUCKET_API_TOKEN`,
  the Terraform provider `BITBUCKET_USERNAME` and `BITBUCKET_PASSWORD`.
- Renovate's terraform manager maps `git::https://bitbucket.org/<ws>/<repo>.git` to its
  `bitbucket-tags` datasource and the scp form `git@bitbucket.org:` to `git-tags`
  (`lib/modules/manager/terraform/extractors/others/modules.ts`, read for behaviour only).

### GitLab
- Tags: `GET {host}/api/v4/projects/{id}/repository/tags`, where `{id}` is the URL-encoded full
  path (`group%2Fsub%2Frepo`). `order_by` takes `name`, `updated` (the default) or `version`, and
  `sort` defaults to `desc`. `per_page` is at most 100: a larger value returns 100 items but
  computes `x-total-pages` from the value sent, so the client caps it. Pages come with a `Link`
  header and `x-next-page`; GitLab drops `x-total`, `x-total-pages` and `rel="last"` above 10,000
  records (docs.gitlab.com/api/rest/).
- Releases: `GET {host}/api/v4/projects/{id}/releases`, with `tag_name`, `description`
  (Markdown), `released_at` and `upcoming_release`, ordered by `released_at` and paged like tags.
- Files: `GET {host}/api/v4/projects/{id}/repository/files/{url-encoded path}/raw?ref={ref}`.
  `ref` is optional and defaults to `HEAD`. The root listing is
  `GET {host}/api/v4/projects/{id}/repository/tree?per_page=100`.
- Auth (docs.gitlab.com/api/rest/authentication/, docs.gitlab.com/ci/jobs/ci_job_token/): a
  personal, project or group token goes in `PRIVATE-TOKEN` (or `Authorization: Bearer`) and needs
  `read_api` (or `read_repository` for files). In CI, `JOB-TOKEN: $CI_JOB_TOKEN` may call the tags
  and releases endpoints and the raw file endpoint (not the tree), for public and internal
  projects and for private ones whose allowlist has the calling project. glab reads
  `GITLAB_TOKEN`, then `GITLAB_ACCESS_TOKEN`, then `OAUTH_TOKEN`, and sends it only to
  `GITLAB_HOST` and the hosts in its config file.
- gitlab.com allows 500 unauthenticated API requests a minute per IP and 2,000 authenticated ones
  per user, and 300 a minute for raw endpoints (docs.gitlab.com/user/gitlab_com/rate_limits/).
  Answers carry `RateLimit-Limit`, `RateLimit-Remaining` and `RateLimit-Reset`.
- A self-managed host can't be told from its name. glab and Renovate take the host from config.

### OCI image registries
- Tags: `GET https://<registry>/v2/<repository>/tags/list`, paged with `Link: rel=next`, after an
  anonymous token from the `WWW-Authenticate` challenge (`realm`, `service`,
  `scope=repository:<repo>:pull`). registry.k8s.io and mcr.microsoft.com ignore `n` and return
  every tag without a `Link`, so a short page doesn't mean the last one. registry.k8s.io answers
  with a 307 to Artifact Registry. public.ecr.aws's realm is `https://public.ecr.aws/token/` with
  `scope="aws"`. quay.io and mcr.microsoft.com answer without a token.
- Docker Hub (`registry-1.docker.io`, realm `https://auth.docker.io/token`, service
  `registry.docker.io`, `library/` for one-part names) also has
  `hub.docker.com/v2/namespaces/<ns>/repositories/<repo>/tags`, which adds timestamps. A manifest
  GET counts as a pull (100 per 6 hours per IP without a login); a HEAD doesn't
  (docs.docker.com/docker-hub/usage/pulls/).
- The source repository comes from the image's `org.opencontainers.image.source` label (manifest
  annotation or config label), then `org.label-schema.vcs-url`. That is how Renovate's docker
  datasource finds release notes (`lib/modules/datasource/docker/`, read for behaviour only).
- `extractVersion` applies to image tags as it does to GitHub tags.

## Behaviour

- New tool names: `bitbucket:<workspace>/<repo>`, `gitlab:<group>/<sub>/<repo>` for gitlab.com
  and `gitlab:<host>/<group>/<repo>` for a host in `gitlab.hosts`. Both take `@<tag-prefix>` like
  `github:`. A first segment is a host only when `gitlab.hosts` lists it, because GitLab group
  names can contain dots.
- The `terraform` and `terragrunt` detectors give these pins for Bitbucket and GitLab git sources
  with `?ref=`, in the https, `git::`, `ssh://` and scp (`git@host:`) forms, with the same ref rules
  as GitHub (`x.y.z` or `vx.y.z`; anything else is a channel pin). A `//subdir` and `.git` are
  dropped. A `# renovate:` comment with `bitbucket-tags`, `gitlab-tags`, `gitlab-releases` or
  `git-tags` on one of these hosts gives the same pin. A git source on any other host stays
  unresolved; relgap runs no `git ls-remote`.
- `tools.<name>` takes `host` next to `repo` (`bitbucket.org`, `gitlab.com` or a `gitlab.hosts`
  entry), so a private tool can point there.
- Versions come from the host's tags API through `internal/httpx` (retries, rate limits, ETag
  cache, `--offline`). A GitLab tool's notes come from its releases, then from an auto-discovered
  changelog file in the root, as for GitHub. A Bitbucket tool's notes come from changelog files
  only. Registry sources (`changelog-file` and the others that read repository files) read from
  the tool's host.
- Tokens: `BITBUCKET_API_TOKEN` goes to `api.bitbucket.org` as Bearer. `GITLAB_TOKEN`, then
  `GITLAB_ACCESS_TOKEN`, goes to `GITLAB_HOST` (default `gitlab.com`) in `PRIVATE-TOKEN`;
  `CI_JOB_TOKEN` goes to `CI_SERVER_HOST` in `JOB-TOKEN` when that host has no other token; a
  `gitlab.hosts` entry's `token_env` wins for its host. No token goes to any other host.
- `image:` pins compare in `outdated` against the registry's tags, from Docker Hub, ghcr.io,
  registry.k8s.io, quay.io, public.ecr.aws, mcr.microsoft.com and the hosts in
  `images.registries`, with anonymous tokens. A pin on another registry is skipped with the reason.
- An image tag `<version><suffix>` keeps its variant in the tool name: `nginx:1.27.0-alpine` is
  `image:nginx:*-alpine`, and only tags with that suffix and a numeric version count. A
  Renovate `extractVersion` of the form `^<prefix>(?<version>...)<suffix>$` gives
  `image:<name>:<prefix>*<suffix>`.
- An image's notes come from the GitHub repository its source label names
  (`org.opencontainers.image.source`, then `org.label-schema.vcs-url`), read from the manifest of
  the newest stable tag: index or manifest annotations first, then the config labels. The digest
  is that repository's digest over the same versions. Without a GitHub source the image has
  versions only, with a note, like a `chart` universe tool.
- `docs/concepts/network.md` lists every new host and token, and `docs/reference/environment.md`
  the new variables.

## Acceptance criteria
- [x] A Bitbucket Cloud module source with `?ref=2.1.0` gives a `bitbucket:` pin that `outdated`
  compares, and `digest` reads a Keep a Changelog `CHANGELOG.md` from the repository. Cassettes
  cover pagination, a 404 for a private repository without a token, and a 429 that succeeds on
  retry. (`TestSpec_non_github_sources_1`)
- [x] A gitlab.com module source with nested groups gives a `gitlab:` pin; `digest` reads its
  releases, and a self-managed host works once configured. (`TestSpec_non_github_sources_2`)
- [x] Tokens go only to the host they belong to, never into the cache or a cassette, and a
  missing token gives the anonymous limits with a warning when they run low.
  (`TestSpec_non_github_sources_3`)
- [x] `outdated` compares `image:` pins from Docker Hub, ghcr.io, registry.k8s.io and
  public.ecr.aws against their tags, honours `extractVersion`, and digests the source label's
  GitHub repository. (`TestSpec_non_github_sources_4`)
- [x] `relgap pins --unresolved` no longer lists the sources of the two kinds above, and
  `docs/concepts/network.md` names every host and token they use.
  (`TestSpec_non_github_sources_5`)
