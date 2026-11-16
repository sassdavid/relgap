# relgap: testing plan

Strategy and layers: [engineering ADR-0006](../engineering/adr/0006-testing-strategy.md). This file lists what is relgap-specific.

## The 30-tool corpus (`testdata/corpus.yaml`)
Each entry: tool, range, adapters expected, expected breaking count (± tolerance, default 2), golden digest path,
or `skip` with the reason a tool isn't digested yet (1password-cli has no public repository).

| Group | Tools |
|---|---|
| HashiCorp/IaC | terraform, opentofu, terragrunt, tflint, terraform-docs, `tfprovider:hashicorp/aws`, `tfprovider:hashicorp/kubernetes` |
| Kubernetes | kubectl, helm (v3 line + v4 line), argocd, k9s, kubeconform, `chart:argo/argo-cd` |
| Cloud CLIs | awscli, gh, 1password-cli, atlascli (prefix `atlascli/v`) |
| Runtimes | node, python, go, temurin 21 |
| Dev tools | mise, uv, ruff, claude-code, golangci-lint, gitleaks, hk, usage, fnox |

The corpus mirrors the author's real toolchain (dotfriedrice `_mise/config.toml` and a typical Terraform/AWS CI image).

- **PR CI:** replays cassettes recorded from the live corpus. The corpus subset (terraform, kubectl, awscli,
  argocd, claude-code) has golden digests in `testdata/golden/corpus/` (`TestSpec_source_adapters_2`).
- **Nightly `live.yml`** (04:41 UTC and on demand): `mise run test:live` runs `TestLiveCorpus`
  (`//go:build live`) against GitHub and the web with `GITHUB_TOKEN`, from a job that can only read the
  repository. For each tool it checks the expected adapters (at `--min-confidence 0`), the breaking count and,
  with a golden, the exact digest data. It writes `results.json` and, for a golden that differs, the expected
  and actual data. A second job with `issues: write` runs `scripts/corpus-issues.sh`: it opens an issue titled
  "corpus drift: <tool>" (label `corpus-drift`) with what differs and the first 60,000 characters of the
  diff, comments on the open one instead when there is one, and closes it when the tool is clean again. The
  test refuses to run with `GOLDEN_UPDATE` set, and nothing in the workflow writes to the repository.

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
  Results are in [Measured performance](#measured-performance).

## Measured performance
`BenchmarkScan60Repos` (`internal/cli`) runs `relgap pins` over the 60 generated repositories of
`TestSpec_pindiscovery_5`, once walking the files and once with git listing them.
`BenchmarkIndex1MB` (`internal/mdindex`) indexes 1 MiB of kubernetes changelog: CHANGELOG-1.37.md
and CHANGELOG-1.36.md from the kubectl digest cassette, repeated with a new major version per copy.
To repeat the benchmarks and compare them with a later run:

```sh
go test -run '^$' -bench . -benchmem -count 10 ./internal/cli/ ./internal/mdindex/ > bench.txt
go run golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e bench.txt
```

The CLI targets come from [overview.md](overview.md). Cold means a built binary with an empty
`XDG_CACHE_HOME`, the live network and `GITHUB_TOKEN` from `gh auth token`. Warm means the same
command rerun right after with that cache. Each row is the median of 5 runs of hyperfine 1.20.0
(`mise install hyperfine@1.20.0`), run in a dotfriedrice checkout:

```sh
hyperfine --runs 5 --prepare 'rm -rf "$XDG_CACHE_HOME"' 'relgap outdated --format json'
hyperfine --warmup 1 --runs 5 'relgap outdated --format json'
hyperfine --runs 5 --prepare 'rm -rf "$XDG_CACHE_HOME"' 'relgap digest terraform 1.15.8..1.17.1 --format json'
hyperfine --warmup 1 --runs 5 'relgap digest terraform 1.15.8..1.17.1 --format json'
```

### 2026-11-16, before ADR-0013
Apple M4 Pro, 48 GiB, macOS 27.2.1, Go 1.27.4, relgap at 4089fb2, dotfriedrice at ebdcb83, from a
home connection served by GitHub's Frankfurt edge.

| Benchmark | Time/op | Spread | Memory/op | Allocs/op |
|---|---|---|---|---|
| `Scan60Repos/walk` | 28.0 ms | ±3% | 20.3 MiB | 194k |
| `Scan60Repos/git` | 178 ms | ±7% | 22.3 MiB | 159k |
| `Index1MB` (27 sections, 206 MiB/s) | 4.87 ms | ±2% | 8.3 MiB | 56.4k |

| Command | Target | Median | Min to max | Requests | Result |
|---|---|---|---|---|---|
| `outdated` cold (55 pins, 50 tools) | under 10 s | 10.57 s | 9.42 to 11.37 s | 26 | missed |
| `outdated` warm | under 2 s | 0.087 s | 0.081 to 0.089 s | 0 (26 cache hits) | met |
| `digest terraform` cold | none | 5.22 s | 4.92 to 6.39 s | | |
| `digest terraform` warm | under 2 s | 0.033 s | 0.033 to 0.034 s | 0 | met |

A cold `outdated` uses 0.15 s of CPU; the rest is waiting on the network. In the traced run
(`-vvv --log-format json`), the two batched "latest" GraphQL queries of 25 repositories each took
4.2 s and 4.7 s on GitHub's side. Then the full version lists of the prefix pins `node = "26"`
and `python = "3.14"` (13 pages of 100) took about 4 s, one page after another for each tool.
endoflife.date and python.org (0.5 s) and then OSV (1.5 s) ran only after the GitHub phase.

### 2026-11-16, with ADR-0013
The same machine, network and dotfriedrice checkout, with
[ADR-0013](adr/0013-parallel-latest-lookup.md): 5 batched queries of 10 repositories in parallel,
the prefix pins' version lists loading at the same time, and endoflife.date and OSV asked
together. The report has the same 55 rows as before.

| Command | Target | Median | Min to max | Requests | Result |
|---|---|---|---|---|---|
| `outdated` cold (55 pins, 50 tools) | under 10 s | 5.75 s | 5.37 to 7.46 s | 29 | met |
| `outdated` warm | under 2 s | 0.081 s | 0.081 to 0.082 s | 0 | met |
| `digest terraform` cold | none | 5.09 s | 4.58 to 5.67 s | | |
| `digest terraform` warm | under 2 s | 0.032 s | 0.031 to 0.032 s | 0 | met |

In the traced run, the GitHub phase ended at 3.8 s instead of 8.6 s. The two version lists set
its length now: a cursor-paged list can't be split. OSV (1.5 s) follows, because it needs
python's current version from its list.
