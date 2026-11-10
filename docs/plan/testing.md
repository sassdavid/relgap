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
- `mdindex`: fuzz on random Markdown with nested fences. Golden indexes (version, heading path, lines, byte range of each section) for the kubernetes, claude-code and node changelogs. They are small JSON, so they are stored uncompressed.
- `classify`: labelled set `testdata/labels/*.yaml` (300 entries, 15 tools). The precision report is printed in CI logs.

## Labelled set
`testdata/labels/<tool>.yaml` holds 20 entries per tool, drafted from live release notes and labelled by hand.
Each entry has the version, the headings above it inside the notes, the entry text and `breaking: true|false`.
The draft took every entry the classifier might call breaking (up to 8 per tool), up to 4 deprecations or
keyword-only guesses, and filled the rest at random, so precision has enough positives to measure.
Labellers saw only the text and headings, never the classifier's output.

An entry is breaking when upgrading past it can break a working setup, a script or a pipeline unless the user
acts. That includes:
- a removed or renamed command, flag, option, config key, API field, metric or output field;
- changed defaults or behaviour that existing users rely on, including stricter validation that rejects
  input that used to work;
- a raised minimum version of a runtime, platform, OS or dependency, and dropped platforms;
- "ACTION REQUIRED" or similar notes that tell the user to change something before or after upgrading.

An entry is not breaking when it only adds something, fixes a bug back to the documented behaviour,
deprecates something that still works, updates a dependency without a user-visible change, or patches a
security issue without changing behaviour users rely on. When the text doesn't say enough to decide, label
it `false`.

`TestSpec_classification_1` re-classifies every labelled entry and fails when breaking precision at the
default `--min-confidence` (0.5) is below 0.9. It logs recall.
- `scan`: fixture trees under `testdata/repos/` (dotfriedrice-like, an infra monorepo (`live/prod/...`, `live/staging/...`) with Dockerfiles and terragrunt, a node project).
- `cli`: testscript per command × format, exit codes, `--fail-on` matrix, no-TTY behaviour, `NO_COLOR`.
- `mcp`: in-process client vs the JSON goldens.
- Benchmarks: `BenchmarkScan60Repos`, `BenchmarkIndex1MB`, tracked with `benchstat` in PRs that touch hot paths.
