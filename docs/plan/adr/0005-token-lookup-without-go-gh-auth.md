# relgap ADR-0005: Look up the GitHub token without go-gh's pkg/auth

- Status: proposed (2026-11-10)

## Context
Engineering ADR-0005 says token resolution uses `go-gh/v2/pkg/auth.TokenForHost`. That function
reads `GH_TOKEN` and `GITHUB_TOKEN` with `os.Getenv`, and when they're empty it starts
`gh auth token` with `exec.Command`, with no context and no timeout.

relgap commands get their environment from `app.Deps.Env`, and external processes go through
`execx` (engineering standards, "Process & control flow"). `TokenForHost` bypasses both. Tests
that inject an environment would also pick up the developer's real token.

## Decision
`internal/github.Token` keeps gh's order but does each step itself:

1. `GH_TOKEN`, then `GITHUB_TOKEN`, read from the injected environment.
2. `oauth_token` for the host in gh's `hosts.yml`, read with go-gh's `pkg/config`.
3. `gh auth token --secure-storage --hostname <host>`, run through `execx` with a 10 s timeout.

## Consequences
- The lookup is testable with an injected environment, config reader and runner.
- go-gh stays a dependency for `pkg/config` only, which brings `gopkg.in/yaml.v3`.
- If gh changes where it keeps tokens, `pkg/config` and `gh auth token` follow it; the
  environment variable order is fixed here.
