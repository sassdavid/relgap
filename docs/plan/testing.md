# relgap: testing plan

Strategy and layers: [engineering ADR-0006](../engineering/adr/0006-testing-strategy.md). This file lists what is relgap-specific.

## The 30-tool corpus (`testdata/corpus.yaml`)
Each entry: tool, range, adapters expected, expected breaking count (± tolerance), golden digest path.

| Group | Tools |
|---|---|
| HashiCorp/IaC | terraform, opentofu, terragrunt, tflint, terraform-docs, `tfprovider:hashicorp/aws`, `tfprovider:hashicorp/kubernetes` |
| Kubernetes | kubectl, helm (v3 line + v4 line), argocd, k9s, kubeconform, `chart:argo/argo-cd` |
| Cloud CLIs | awscli, gh, 1password-cli, atlascli (prefix `atlascli/v`) |
| Runtimes | node, python, go, temurin 21 |
| Dev tools | mise, uv, ruff, claude-code, golangci-lint, gitleaks, hk, usage, fnox |

The corpus mirrors the author's real toolchain (dotfriedrice `_mise/config.toml` and a typical Terraform/AWS CI image).

- **PR CI:** replays cassettes recorded from the live corpus.
- **Nightly `live.yml`:** re-runs live. On diff it opens or updates an issue "corpus drift: <tool>" with the diff attached, and never auto-updates goldens.

## Specific test suites
- `version`: table tests + fuzz (`FuzzParse`, `FuzzCompareProperties`).
- `mdindex`: fuzz on random Markdown with nested fences. Golden indexes for the 1 MB+ changelogs (stored compressed).
- `classify`: labelled set `testdata/labels/*.yaml` (300 entries, 15 tools). The precision report is printed in CI logs.
- `scan`: fixture trees under `testdata/repos/` (dotfriedrice-like, an infra monorepo (`live/prod/...`, `live/staging/...`) with Dockerfiles and terragrunt, a node project).
- `cli`: testscript per command × format, exit codes, `--fail-on` matrix, no-TTY behaviour, `NO_COLOR`.
- `mcp`: in-process client vs the JSON goldens.
- Benchmarks: `BenchmarkScan60Repos`, `BenchmarkIndex1MB`, tracked with `benchstat` in PRs that touch hot paths.
