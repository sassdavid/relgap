# Spec: changelog source adapters

```go
type Adapter interface {
    Kind() string
    // Fetch returns notes for the requested versions; versions with no notes are omitted, not errors.
    Fetch(ctx context.Context, t ToolSpec, versions []version.V) ([]VersionNotes, error)
}
type VersionNotes struct{ Version version.V; Markdown string; Source SourceRef } // SourceRef = URL + heading path
```

## Adapter kinds

| Kind | How | Used by (examples) |
|---|---|---|
| `gh-release` | release body per tag, fetched via **GraphQL batch** (`releases(first:100)` with `description`; page size shrinks 100→50→25→10 on errors, as Renovate does) | terraform, opentofu, terragrunt, helm, argocd, gh, k9s, uv, ruff, most tools |
| `changelog-file` | fetch the file **once** per (repo, ref, path), index by version heading ([ADR-0001](../adr/0001-goldmark-section-index.md)). `path_template` supports `{major}`, `{minor}` for per-line files; `ref` may be a template (`v{major}.{minor}`) | kubernetes `CHANGELOG/CHANGELOG-1.xx.md`, node `doc/changelogs/CHANGELOG_V26.md`, claude-code `CHANGELOG.md`, terraform-provider-aws, uv/ruff `changelogs/0.x.x.md` |
| `structured-json` | fetch JSON per version, map fields → entries | awscli `.changes/<ver>.json` (`type` = api-change/enhancement/bugfix), Adoptium `api.adoptium.net/v3/info/release_notes/<ver>` |
| `web-doc` | allow-listed URL template → HTML → Markdown (`JohannesKaufmann/html-to-markdown`, verify) → index | python `docs.python.org/3/whatsnew/3.X.html`, go `go.dev/doc/go1.NN`, `go.dev/doc/devel/release` |
| `upgrade-guide` | URL or file template per minor transition (`{prev_minor}-{minor}.md`), whole document, attached as an "Upgrade guide" section | argocd `docs/operator-manual/upgrading/3.5-3.6.md`, terraform upgrade guides |

A tool's `sources` list is **composite**: all adapters run and results merge per version, each entry
keeping its own `SourceRef`. Duplicate bullets (release body repeating the changelog) are dropped by
normalized-text hash.

## Auto-discovery (no registry entry)
Following Renovate and Dependabot:
1. `gh-release` bodies when the repo has non-empty releases. Boilerplate detection: under 400 chars, or only download links or checksums, means "empty".
2. Otherwise look for a changelog file in the default-branch tree. Match the filename with Renovate's regex `^(?:(?:update|change|release)(?:s|[ \-_]*(?:logs?|histor(?:y|ies)))|histor(?:y|ies)|release[ \-_]*notes?)(?:\.[\da-z]+)?$`, preferring `.md` over `.rst` over `.txt`, skipping `.json` and files under 100 B.
3. Otherwise: compare-URL-only entries ("no notes found, see diff").
4. A **stale changelog guard**: if the file's newest version heading is older than the newest release, ignore the file. ArgoCD's `CHANGELOG.md` stops at 2.4.8 (2022).

## GitHub specifics
- Files over 1 MB: `Accept: application/vnd.github.raw` on the contents API, never raw.githubusercontent.com.
- Huge release bodies (terragrunt v1.2.0 is 96 KB): fetch bodies in small GraphQL pages; never `first:100` with bodies for big repos (adaptive).
- One GraphQL query aliases up to 25 repos for "latest release + tags" (`outdated` over 45 tools = 2 queries).

## Acceptance criteria
- [ ] Each adapter kind has cassette-backed tests with golden `VersionNotes` for at least 2 tools.
- [ ] terraform 1.15.8..1.17.1, kubectl 1.36.0..1.37.2, awscli 2.31.0..2.31.10, argocd 3.5.0..3.6.1 and claude-code 2.1.300..2.1.319 produce golden digests (the corpus subset).
- [ ] A changelog file is fetched exactly once per digest regardless of how many versions it contains (asserted via the request counter).
- [ ] The stale-changelog guard is covered by an argocd fixture.
- [ ] Auto-discovery picks `HISTORY.md` / `CHANGES.rst` / `RELEASE_NOTES.md` in fixtures and rejects `changelog.json`.
