# relgap: architecture

## Pipeline

```
 inputs                     resolve                 versions                 notes                    report
┌──────────────┐   Pin   ┌──────────────┐ Tool  ┌───────────────┐ Range ┌────────────────┐ Entries ┌──────────────┐
│ CLI args     │────────▶│ Resolver     │──────▶│ Version engine│──────▶│ Source adapters│───────▶│ Classifier   │
│ mise (json)  │         │ override reg │       │ tags+releases │       │ gh-release     │        │ breaking/... │
│ Scanner      │         │ mise registry│       │ schemes       │       │ changelog-file │        └──────┬───────┘
│ (Docker, TF, │         │ aqua registry│       │ range select  │       │ structured-json│               │
│ CI, lock.hcl)│         │ heuristics   │       └───────────────┘       │ web-doc        │        ┌──────▼───────┐
│ Renovate JSON│         └──────────────┘                               │ upgrade-guide  │        │ Enrichers    │
└──────────────┘                                                        └───────┬────────┘        │ EOL · OSV    │
                                  ┌────────────── httpx (retry · rate · ETag cache) ◀─────────────└──────┬───────┘
                                  │                                                                      │
                                  │                     renderers: tty · plain · md · json · sarif ◀─────┤
                                  │                     mcp server (same JSON)                     ◀─────┤
                                  │                     summarizer (optional LLM)                  ◀─────┘
```

## Packages (`internal/`)

| Package | Responsibility | Spec |
|---|---|---|
| `cli` | cobra commands; flags → options; calls domain; picks renderer | [cli.md](cli.md) |
| `scan` | walk repos, run file detectors, produce `[]Pin` with `Location` | [specs/pin-discovery.md](specs/pin-discovery.md) |
| `misex` | exec `mise registry --json`, parse; pins and `outdated` read config files instead ([ADR-0007](adr/0007-parse-mise-config-for-pins.md), [ADR-0009](adr/0009-outdated-current-and-latest.md)) | [specs/pin-discovery.md](specs/pin-discovery.md) |
| `registry` | tool name/alias → `ToolSpec` (repo, schemes, sources); embedded + remote override registry | [specs/tool-resolution.md](specs/tool-resolution.md) |
| `version` | `Scheme` interface (semver, loose, calver, regex-extract), parse/compare/range | [specs/versions-and-ranges.md](specs/versions-and-ranges.md) |
| `github` | REST + GraphQL batch; tags, releases, file fetch (raw accept) | [specs/source-adapters.md](specs/source-adapters.md) |
| `source` | `Adapter` interface + 5 adapter kinds; composite per tool | [specs/source-adapters.md](specs/source-adapters.md) |
| `mdindex` | goldmark AST → version-keyed section index (parse once per file) | [adr/0001](adr/0001-goldmark-section-index.md) |
| `classify` | entry kind + confidence from headings, markers, context | [specs/classification.md](specs/classification.md) |
| `enrich` | endoflife.date, OSV (+ Go buildinfo of installed binaries) | [specs/enrichment.md](specs/enrichment.md) |
| `report` | report model, `--fail-on` evaluation | [specs/output-and-ci.md](specs/output-and-ci.md) |
| `render` | tty/plain/md/json/sarif | [specs/output-and-ci.md](specs/output-and-ci.md) |
| `summarize` | optional LLM provider abstraction | [specs/mcp-and-llm.md](specs/mcp-and-llm.md) |
| `mcpserver` | MCP tools over the domain API | [specs/mcp-and-llm.md](specs/mcp-and-llm.md) |
| `httpx`, `cache` | shared HTTP client + disk cache | [engineering ADR-0005](../engineering/adr/0005-http-and-api-clients.md) |
| `config` | koanf load, schema, provenance | [specs/config.md](specs/config.md) |

## Core data model (sketch)

```go
type Pin struct {             // internal/scan; JSON in schemas/pins-v1.json
    Tool     string          // canonical name after resolution ("terraform", "tfprovider:hashicorp/aws")
    RawName  string          // as found ("TERRAFORM_VERSION", "hashicorp/aws")
    Kind     Kind            // tool | tfprovider | chart | image | action
    Value    string          // "1.15.8", "~> 6.0", "1.15"
    Version  string          // Value without a leading v
    Exact    bool            // constraint vs exact pin
    Match    Match           // exact | constraint | prefix | channel
    Location Location        // repo, file, line, detector
}

type ToolSpec struct {         // from registry
    Name, Repo string          // "terraform", "hashicorp/terraform"
    Scheme      version.SchemeConfig // prefix, regex, prerelease rules, line (e.g. helm v3 vs v4)
    Sources     []source.Config      // ordered adapters
    EOLProduct  string               // endoflife.date product
    OSV         []OSVPackage         // ecosystem+name(s)
}

type Entry struct {
    Version  string; Kind EntryKind   // breaking|deprecation|security|feature|fix|other
    Text     string; Source SourceRef // URL + adapter + heading path
    Confidence float32                // 1.0 explicit heading … 0.4 keyword-only
}

type ToolReport struct {
    Tool ToolSpec; From, To string; Versions []string
    Entries []Entry; EOL *EOLInfo; Vulns []Vuln; Pins []Pin; Errors []string
}
```

## Key flows
- **digest:** resolve → list versions (GraphQL tags/releases, cached) → select (from, to] → for each adapter, fetch once (file) or per version (release body) → mdindex → classify → enrich → render.
- **outdated:** scan pins in the project → group by tool → newest 30 releases per tool, 25 tools per GraphQL query → current version per pin ([ADR-0009](adr/0009-outdated-current-and-latest.md)) → for each pin behind latest, the full digest `current..latest` from the same digester as `relgap digest` → table. `digest --from-mise` and `--all` print those digests.
- **drift:** scan roots (offline) → group pins by tool → matrix; with `--latest` also fetch latest versions.

## Cross-cutting
- Concurrency: errgroup per tool (default 8), GitHub limiter shared.
- Determinism: output sorted by tool, then version. Timestamps only in the JSON envelope header.
- Errors are per tool: one tool failing to resolve doesn't fail the run (exit 1 with partial report).
