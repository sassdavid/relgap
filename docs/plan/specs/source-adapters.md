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
| `changelog-file` | fetch the file **once** per (repo, ref, path), index by version heading ([ADR-0001](../adr/0001-goldmark-section-index.md)). `path_template` supports `{major}`, `{minor}` for per-line files; `ref` may be a template (`v{major}.{minor}`). `.rst` files get Markdown headings first | kubernetes `CHANGELOG/CHANGELOG-1.xx.md`, node `doc/changelogs/CHANGELOG_V26.md`, claude-code `CHANGELOG.md`, terraform-provider-aws, uv/ruff `changelogs/0.x.x.md` |
| `structured-json` | fetch JSON per version from a repo file (`path_template`) or an allowed URL (`url_template`), map fields → entries with the `json` mapping | awscli `.changes/<ver>.json` (`type` = api-change/enhancement/bugfix), Adoptium `api.adoptium.net/v3/info/release_notes/<ver>` |
| `web-doc` | allow-listed URL template → HTML → Markdown (`JohannesKaufmann/html-to-markdown/v2`) → index | python `docs.python.org/3/whatsnew/3.X.html`, go `go.dev/doc/go1.NN`, `go.dev/doc/devel/release` |
| `upgrade-guide` | URL or file template per release line transition, whole document, attached as an "Upgrade guide" section | argocd `docs/operator-manual/upgrading/3.5-3.6.md`, terraform-provider-aws `website/docs/guides/version-6-upgrade.html.markdown` |
| `upgrade-notes` | the section under a heading (`from_heading`) of each version's changelog-file section, filed under "Upgrade notes" | kubernetes "Urgent Upgrade Notes" |

A tool's `sources` list is **composite**: all adapters run and results merge per version, each entry
keeping its own `SourceRef`. Duplicate bullets (release body repeating the changelog) are dropped by
normalized-text hash. Sources run in the order listed, except that upgrade-notes runs first, so
its entries keep their own source when the changelog repeats them.

### Template placeholders
`{version}`, `{major}`, `{minor}` (the minor number alone, so a minor line is
`{major}.{minor}`) and `{tag}`. upgrade-guide also has `{prev_minor}` and `{prev_major}`: the
minor line (such as `3.5`) and the major of the version before the transition. The previous
version is the real one, so argocd's `2.14-3.0.md` gets its name. `relgap registry lint` rejects
other placeholders.

### structured-json
The `json` mapping has `items` (dotted path to the array; empty means the document is the
array), `heading` (field whose value groups entries under a heading), `text` (template with
`{field}` placeholders) and `link` (field with an https URL). reStructuredText code (``` ``x`` ```)
becomes Markdown code. awscli maps `{type}` to the heading and `{category}: {description}` to the
text. Classified as other at heading confidence, awscli's `api-change` entries are visible by
default.

### web-doc
- Hosts: go.dev, docs.python.org and api.adoptium.net (the last for structured-json), compiled
  into relgap, https only, every redirect checked, 8 MB per page. Lint rejects other hosts
  ([ADR-0011](../adr/0011-enrichment-and-web-sources.md)).
- `content` selects the element to convert (default `main`, else `body`); `strip` drops elements
  (python's `a.headerlink` permalinks, which would leak a `¶` into all 143 headings of the 3.14
  page). Scripts, styles, forms, navigation and comments are always dropped. Selectors are a tag,
  `#id` and `.class` parts without spaces. Relative links resolve against the page URL.
- Without `anchor`, a page holds a release line: it goes to the first version of the range whose
  URL is that page, and not at all when the version before the range has the same page. go's
  1.27 notes go to 1.27.0.
- With `anchor`, each version gets the element with that id. For a heading that is the heading
  and what follows up to the next heading of its level. go's point releases are paragraphs
  `<p id="go1.27.3">` on `go.dev/doc/devel/release`.

### upgrade-guide
A template with `{minor}` or `{prev_minor}` names one document per minor transition; otherwise
one per major (terraform-provider-aws, terraform-provider-kubernetes). The document goes to the
first version of the new line, headings one level down under "Upgrade guide", HTML comments
removed. A transition without a document is skipped. Terraform's own upgrade guides are left
out: they moved to hashicorp/web-unified-docs and are under 1 KB, mostly a link to the changelog.

## Auto-discovery (no registry entry)
Following Renovate and Dependabot:
1. `gh-release` bodies when the repo has non-empty releases. Boilerplate detection: under 400 chars, or only download links or checksums, means "empty". When at least one body in the range is more than boilerplate, every non-empty body is used.
2. Otherwise look for a changelog file in the default-branch tree. Match the filename with Renovate's regex `^(?:(?:update|change|release)(?:s|[ \-_]*(?:logs?|histor(?:y|ies)))|histor(?:y|ies)|release[ \-_]*notes?)(?:\.[\da-z]+)?$`, preferring `.md` over `.rst` over `.txt`, skipping `.json` and files under 100 B. relgap lists the top directory with the contents API, which gives the sizes.
3. Otherwise: the boilerplate bodies there are. A range with no entries at all gets a warning with the compare URL ("no release notes found; compare the versions at https://github.com/o/r/compare/v1.0.0...v1.1.0").
4. A **stale changelog guard**: if the file's newest version heading is older than the newest release, ignore the file, with a warning in the digest. ArgoCD's `CHANGELOG.md` stops at 2.4.8 (2022).

## GitHub specifics
- Hosts: github.com only in v0.x (open question Q3, recorded here at M11). `github.host` exists
  in the config schema so a later GitHub Enterprise Server adapter needs no new key. relgap
  doesn't read the key yet, and every request goes to github.com. GitLab and Codeberg are out
  of scope for v0.x and would be new adapters behind the same interface.
- Files over 1 MB: `Accept: application/vnd.github.raw` on the contents API, never raw.githubusercontent.com.
- Huge release bodies (terragrunt v1.2.0 is 96 KB): fetch bodies in small GraphQL pages; never `first:100` with bodies for big repos (adaptive).
- One GraphQL query aliases up to 25 repos for "latest release + tags" (`outdated` over 45 tools = 2 queries).
- A file that doesn't exist (404) is remembered for an hour, so per-version files cost no request in a second run and work with `--offline` ([ADR-0011](../adr/0011-enrichment-and-web-sources.md)).

## Acceptance criteria
- [x] Each adapter kind has cassette-backed tests with golden `VersionNotes` for at least 2 tools. (gh-release: terraform, helm in `TestSpec_source_adapters_1`; changelog-file: claude-code, node, terraform in `TestSpec_source_adapters_1_changelog`; structured-json: awscli, temurin; web-doc: go, go point releases, python; upgrade-guide: argocd, terraform-provider-kubernetes, each in `TestSpec_source_adapters_1_<kind>`. upgrade-notes, used only by kubectl: `TestUpgradeNotesKubectl`.)
- [x] terraform 1.15.8..1.17.1, kubectl 1.36.0..1.37.2, awscli 2.31.0..2.31.10, argocd 3.5.0..3.6.1 and claude-code 2.1.300..2.1.319 produce golden digests (the corpus subset). (`TestSpec_source_adapters_2`, goldens in `testdata/golden/corpus/`; the live corpus compares with the same files.)
- [x] A changelog file is fetched exactly once per digest regardless of how many versions it contains (asserted via the request counter).
- [x] The stale-changelog guard is covered by an argocd fixture. (`TestSpec_source_adapters_4`, with the head of argocd's real `CHANGELOG.md` in `testdata/autodiscovery/argocd/`.)
- [x] Auto-discovery picks `HISTORY.md` / `CHANGES.rst` / `RELEASE_NOTES.md` in fixtures and rejects `changelog.json`. (`TestSpec_source_adapters_5`; `TestAutoUsesFreshChangelog` reads a `CHANGES.rst`.)
- [x] The "latest version" lookup for 45 tools sends at most 2 GraphQL queries (request counter on a cassette, `TestSpec_source_adapters_6`).
