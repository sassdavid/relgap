# relgap: milestones

**One milestone = one fresh Claude Code session.** Start each session with:
"Read `CLAUDE.md`, then `docs/plan/milestones.md` M<n> and the linked specs. Implement M<n> only."
Tick a box only when its check has passed, and quote the evidence (command plus result) in the commit body.

Release mapping (revised 2026-11-13): every release goes to GitHub only (archives, checksums,
SBOMs, the cosign bundle and attestations) until M19 turns the package channels on.
**v0.1.0 = M0-M11**, v0.2.0 = M12-M13, v0.3.0 = M15-M16, v0.4.0 = M17-M18, and M14 releases the
separate action repo. M19 has no fixed version or place in the order: the owner runs it when the
GitHub releases look right. M20 waits for external users. The research and decisions behind
M11-M20 are in [roadmap-research.md](roadmap-research.md).

---

### M0: Skeleton and pipeline
Scope: repo, `cmd/relgap`, cobra root with `version` and `completion`, koanf config + `config show|path|validate`, slog/tint, `Deps` struct, render package stub, mise tasks, golden lint config, CI, goreleaser snapshot, release-please.
- [x] `mise run check` passes on a clean clone (lint + `go test -race` + govulncheck).
- [x] `relgap version --format json` validates against `schemas/report-v1.json` (envelope only).
- [x] `goreleaser release --snapshot --clean` produces 6 archives, checksums and an SBOM.
- [x] CI is green on ubuntu, macos and windows. Actions are pinned by SHA and zizmor is clean.

### M1: Versions and resolution → [versions](specs/versions-and-ranges.md), [resolution](specs/tool-resolution.md)
Scope: `version` schemes, embedded mise and aqua snapshots + `registry:snapshot` task, override registry loader + 5 seed entries, `relgap resolve`.
- [x] All acceptance criteria in versions-and-ranges.md, plus resolution criteria 1, 2 and 4.
- [x] Fuzz targets run for 60 s in CI without failures.

### M2: GitHub client and `gh-release` digest → [adapters](specs/source-adapters.md)
Scope: `httpx` (retry, rate, no cache yet), go-github + githubv4 batch, token discovery, `gh-release` adapter, minimal classifier (heading rules only), `digest` with tty/plain/json.
- [x] `relgap digest terraform 1.15.8..1.17.1 --format json` matches the golden file (cassette).
- [x] Without a token: a single warning, concurrency drops to 2, the result is still correct.
- [x] Ctrl-C mid-digest exits 130 within 1 s (testscript with a slow fake server).

### M3: `changelog-file`, section index, full classifier → [classification](specs/classification.md)
- [x] mdindex parses each file once (request counter test). kubernetes, claude-code and node fixtures are golden.
- [x] All classification.md criteria. The precision check over the labelled set passes ≥ 0.9
  (0.900, 36 of 40, labels reviewed 2026-11-13).
- [x] `md` renderer golden output for 3 tools.

### M4: Cache, ETag, offline
- [x] A second identical digest makes 0 non-304 requests (counter). `--offline` works with the network disabled (testscript sets a bogus proxy).
- [x] Age-based TTLs are unit-tested with an injected clock. `cache info|clear|prune` are covered.

### M5: Pin discovery → [pin-discovery](specs/pin-discovery.md)
- [x] All pin-discovery.md criteria. `relgap pins` works against a dotfriedrice-shaped fixture.

### M6: `outdated` + `digest --from-mise`
- [x] Fixture project with 10 pins: correct table in tty/md/json, with breaking counts matching the full digests.
- [x] Uses at most 2 GraphQL queries for "latest version" of 45 tools (counter).

### M7: `drift`
- [x] A generated tree of 60 repos gives a correct matrix, constraint pins marked `≈`, `--fail-on drift` exits 3 (`TestSpec_output_and_ci_6`).
- [x] Offline by default. `--latest` adds the latest versions, a row in the repo-by-tool layout of [ADR-0010](adr/0010-drift-offline-matrix.md) (`TestSpec_output_and_ci_7`).

### M8: Remaining adapters, enrichment, curated registry → v0.1.0
Scope: `structured-json`, `web-doc`, `upgrade-guide` adapters, EOL + OSV, all 20 curated registry entries, `registry lint|test`, live corpus job.
- [x] Every enrichment.md criterion, plus source-adapters criteria 2, 4 and 5. (`TestSpec_enrichment_1` to `_4`, `TestSpec_source_adapters_2`, `_4`, `_5`; `mise run check` passes. Decisions: [ADR-0011](adr/0011-enrichment-and-web-sources.md).)
- [x] The nightly live corpus (30 tools) passes once on main: live run 43396597428 (8392522) passed 28 tools; chart:argo/argo-cd and 1password-cli are skipped with the reason in `testdata/corpus.yaml`.
- Moved to M11: the first tagged release v0.1.0 and its install checks.

### M9: SARIF + `--fail-on` matrix → [output-and-ci](specs/output-and-ci.md)
- [x] output-and-ci.md criteria 1-3. (`TestSpec_output_and_ci_1`, `_2`, `_3`; decisions in the spec and [ADR-0012](adr/0012-fail-on-gate-and-sarif.md). The upload-sarif dry run is the `sarif` job in `ci.yml`.)

### M10: MCP server → [mcp-and-llm](specs/mcp-and-llm.md)
- [x] MCP criteria 1–2. Registered in the author's Claude Code config and used for a real upgrade (terraform or argocd), notes recorded in `docs/dogfood.md`.
  Criteria 1-2 pass (`TestSpec_mcp_and_llm_1`, `_2`). On 2026-11-13 the server was registered
  at local scope and answered the Argo CD 3.1.0 to 3.6.1 question in Claude Code, following
  the cursor to all 99 entries ([dogfood.md](../dogfood.md)).

---

From M11 on, relgap and tfcensus use the same milestone numbers. A milestone marked "shared" lands
in both repos in the same session (`.claude/rules/siblings.md`). If one half can't land, the final
message says so. Where a shared milestone has no spec yet, the session writes the spec or ADR first.
The old M11 (GitHub Action) is now M13 plus M14, and the old M12 (LLM summary) is now M15. The old
M12 `--binaries` item was already done (`TestSpec_enrichment_3`).

### M11: Release readiness → v0.1.0 on GitHub only (shared)
Scope: fix what blocks the first release, then cut it on GitHub only (decided 2026-11-13). No
package channel gets anything until M19. No new features.
- [x] Package channels are off in the committed config: the cask and Scoop `skip_upload: true`
  (no token check yet).
  goreleaser still renders the cask and the manifest into `dist/`, and `release.yml` keeps them as
  a workflow artifact so each release shows what M19 would publish.
- [x] A person reviews `testdata/labels/*.yaml`, re-runs the precision test and ticks the M3 box.
  Reviewed 2026-11-13 against upstream release notes: no label changed, and
  `TestSpec_classification_1` gives 0.900 (36 of 40), recall 0.783 (36 of 46).
- [x] `gh workflow run live.yml` gives one green run on main and ticks the M8 corpus box: run
  43396597428 on 2026-11-13 (28 passed, 2 skipped); it closed the false drift issues #21 and #22.
- [x] The Homebrew cask installs real completion scripts. Today `args: [completion]` plus
  `shell_parameter_format: cobra` makes brew run `relgap completion completion bash`, which prints
  help text. Drop `args` (M12 replaces the stanza) and check the rendered cask in `dist/`.
- [x] Paperwork from the 2026-11-13 audit: tick config.md criteria 1-3, tool-resolution criteria 1,
  2 and 4, and versions-and-ranges criteria 1-5 (their `TestSpec_` tests pass). Move the answered
  open questions Q3 (`github.host`) and Q8 (embedded aqua snapshot) into a spec or ADR and delete
  them from `open-questions.md`.
- [x] Release PR #2 is merged and v0.1.0 is a GitHub release. `cosign verify-blob` and
  `gh attestation verify` pass on a downloaded archive, `mise use github:sassdavid/relgap` works,
  and the tap and the bucket get no commit. Evidence from 2026-11-13: release run 43401197743
  published 14 assets and the `package-manifests` artifact; on `relgap_0.1.0_darwin_arm64.tar.gz`
  cosign printed "Verified OK", `shasum -c` "OK" and `gh attestation verify` succeeded (signer
  workflow `release.yml`); `mise use github:sassdavid/relgap` installed 0.1.0; both the tap and
  the bucket API answer 409 "Git Repository is empty".

### M12: Shell completion, carapace and man pages (shared)
Scope: every flag and argument completes from local data in every common shell, and every channel
ships the scripts and man pages. bash, zsh, fish and PowerShell use cobra's scripts. nushell,
elvish, xonsh, tcsh, oil and clink use embedded carapace, so users install nothing extra.
- [x] Every enum flag completes its values with descriptions and `ShellCompDirectiveNoFileComp`:
  `--format`, `--color`, `--log-format`, `--fail-on`, `--vuln-severity` and `--only`, with hints
  for `--min-confidence` and `--max-behind`. One list feeds both the help text and the completion
  (the gh `StringEnumFlag` pattern). `--config` filters by yaml, `--renovate-report` by json,
  `--log-file` takes files, `--root` uses `FilterDirs`.
  Evidence (2026-11-13, local): `testdata/script/complete.txtar` goldens for `--format`,
  `outdated --fail-on`, `digest --only`, `outdated --max-behind`, `--config`, `--renovate-report`
  and `mcp --root`; `TestCompletionFuncs` and `TestJoinAndUsage` in `internal/complete`.
- [x] A test walks `NewRootCmd`: every non-bool flag has a completion func, a file or directory
  annotation, or an allowlist entry with a reason.
  Evidence: `TestCompletionCoverage` (`internal/cli/completion_test.go`) also checks argument
  completion; the allowlist is empty.
- [x] Dynamic completion reads local data only. Tool names for `digest`, `resolve`, `--tool` and
  `registry test` come from the embedded snapshots and the override registry (`registry.Load`),
  never from `mise registry` or GitHub. Each func has a 1 s timeout and returns
  `ShellCompDirectiveError` with empty stdout on failure. A test or depguard rule proves no
  `httpx` call is reachable.
  Evidence: `TestTools` and `TestNoNetworkReachable` (`go list -deps`: no `httpx`,
  `internal/github` or `net/http`) in `internal/complete`, `TestDynamic` for the timeout, and
  the resolve, digest and drift goldens in `complete.txtar`.
- [x] `__complete` writes only results to stdout and does no I/O in `PersistentPreRunE`. Warm
  runs stay under 100 ms.
  Evidence: `TestCompleteSkipsSetup` (an unwritable `--log-file` doesn't fail
  `__complete`) and `TestCompleteIsFast` (best of 5 in-process runs under 100 ms with `-race`).
- [x] carapace is embedded at the same pinned version in both repos. `Gen` runs outside the
  constructors parallel tests use, and `go test -race` passes.
  `relgap completion nushell|elvish|xonsh|tcsh|oil|clink` prints the snippet. The owner has read
  carapace's `LICENSE.appendix`, and `lint:licenses` passes.
- [x] testscript goldens cover `__complete`, `__completeNoDesc` and `_carapace export` for static,
  enum, dynamic, missing-file and timeout cases. They live outside the shared `completion.txtar`.
  Evidence: `testdata/script/complete.txtar` covers static, enum, dynamic and missing-file cases. No
  relgap source can hang in a testscript (the config loader accepts regular files only), so the
  timeout is covered by `TestDynamic` in `internal/complete` instead (engineering ADR-0013).
- [x] A CI job loads the scripts in real shells: bash 5 with bash-completion (ubuntu), bash 3.2
  with brew bash-completion v1 (macos), zsh with `compinit`, fish with `complete -C`, pwsh
  `TabExpansion2` on all three OSes, Windows PowerShell 5.1, and nushell through the carapace
  snippet.
  Evidence (2026-11-13): the `completion-shells` jobs of ci run 43433798621 on main (after #27)
  passed on ubuntu, macos and windows. The logs print "bash: ok" for bash 5 and bash 3.2, "zsh: ok",
  "fish: ok", "nushell: ok", and "powershell: ok" for pwsh 7.6 on each OS and for Windows
  PowerShell 5.1.26100.
- [x] A shared `scripts/completions.sh` (on the sibling-diff "identical" list) writes
  `completions/` (bash, zsh, fish, ps1) and `manpages/*.1.gz` (`GenManTree` from
  `scripts/gendocs`, dates from `SOURCE_DATE_EPOCH`, the same bytes on a second run). Every
  archive contains both directories.
  Evidence (local): `goreleaser release --snapshot --clean --skip=sign,sbom` put `completions/`
  (4 files) and 30 `manpages/*.1.gz` in all 6 archives; two runs of `bash scripts/completions.sh`
  gave the same man page checksum. The CI snapshot job checks both.
- [x] The cask uses goreleaser's `completions:` and `manpages:` keys instead of
  `generate_completions_from_executable`. Scoop's `post_install` prints the `$PROFILE` line with
  the `current` path.
  Evidence: the snapshot's `dist/homebrew/Casks/relgap.rb` has `bash_completion`,
  `zsh_completion`, `fish_completion` and 30 `manpage` lines; `dist/scoop/bucket/relgap.json`
  has the `post_install` lines.
- [x] An install page covers each shell and channel and the pitfalls in
  [roadmap-research.md](roadmap-research.md). An ADR records the shell, carapace and man page
  choices. ADR-0002's man page claim matches what ships.
  Evidence: [docs/install.md](../install.md) and engineering
  [ADR-0013](../engineering/adr/0013-shell-completion-and-man-pages.md).

### M13: CI outputs and the GitHub PR comment (shared) → v0.2.0 ([output-and-ci](specs/output-and-ci.md))
Scope: everything the action (M14) needs from the CLI, so the action only wires flags.
Decisions: [engineering ADR-0014](../engineering/adr/0014-report-outputs-and-pr-comment.md).
- [x] A repeatable `--out <format>=<path>` flag in the shared `internal/render`. One run writes
  json, sarif, md and github output with the same exit code as `--format`, shown by a testscript.
  It replaces the `-o, --output` flag planned in `cli.md`.
  Evidence (2026-11-13, local): `testdata/script/out.txtar` runs drift over the generated tree
  once with `--format json` and four `--out` files, exits 3 under `--fail-on drift`, and each
  file equals the single-format run. `digest`, `outdated`, `drift` and `pins` take `--out`;
  `outdated` and `drift` gain `--format github`. `cli.md` lists `--out`.
- [x] A shared `internal/ghcomment` package on `internal/httpx`, the same code as tfcensus.
  `relgap notify --from <json> --notify github-pr-comment` keeps one comment per kind per PR with
  the `<!-- relgap:v1 ... -->` marker. An unchanged hash means no write, and a resolved report
  becomes "Nothing to report". The PR number comes from `GITHUB_EVENT_PATH`. Cassettes cover
  create, unchanged (0 writes), update and resolve.
  Evidence: `TestPublish` replays `testdata/cassettes/github/pr-comment-{create,unchanged,update,resolve,skip}.yaml`
  and counts writes (1, 0, 1, 1, 0; the dry run 0); `TestNotifyPRComment` runs the lifecycle on
  saved drift reports and `TestNotifyErrors` the exit codes.
- [ ] `digest` can take the upgraded tools of a PR: the pins at the base ref against the head ref,
  or Renovate's branch name and JSON comment (output-and-ci.md). Not started.
- [x] Markdown truncation counts characters, defaults to 60,000 (GitHub's cap is 65,536), cuts at
  row or section boundaries and adds a footer naming what was cut (output-and-ci.md criterion 4,
  formerly `--md-max-bytes`). A property test checks the limit. The flag has the same name in both
  CLIs.
  Evidence: `--md-max-chars` in both CLIs; `TestDigestMarkdownTruncation` (300 generated digests)
  and the shared `TestTruncateMarkdownLimit`; `digest_kubectl.txtar` checks the cut of a
  digest over 60,000 characters. `docs/ci/outdated.yml` uses `--out` and the comment, and
  `jactionlint docs/ci/outdated.yml` exits 0.
- [ ] v0.2.0 is released.

### M14: GitHub Action → `sassdavid/relgap-action` v1.0.0 (clone at `~/src/github/relgap-action`, empty) (shared with tfcensus-action)
Scope: a composite action in its own repo that installs a verified release and wires M13's flags.
Design in [roadmap-research.md](roadmap-research.md#github-action). `comment-on-renovate` becomes
mode `comment` with `command: digest`; update output-and-ci.md to match.
- [ ] `scripts/sibling-diff.sh` reads the tool name from a `.sibling` file or the directory name
  instead of `go.mod`, stays verbatim in all four repos, and each action repo has its file list.
- [ ] Skeleton: `action.yml`, `scripts/install.sh`, `scripts/run.sh` and `scripts/tool.env` (the
  only tool-specific file besides the README). Install checks sha256 and
  `gh attestation verify --signer-workflow sassdavid/relgap/.github/workflows/release.yml`, with
  cosign and checksum-only as options. A tampered archive fails in CI. CI runs on ubuntu, macos
  and windows, zizmor and actionlint pass, every `uses:` is pinned by SHA, and release-please
  publishes immutable releases with a moving `v1` tag.
- [ ] Report mode: `outdated` or `drift` writes json, SARIF, md and annotations in one run. SARIF
  shows in code scanning on a test repo, and `fail-on` fails the step on exit 3.
- [ ] Comment mode: one sticky comment on a test repo's Renovate PR. A re-run edits it with no
  duplicate, and a fork PR falls back to the step summary.
- [ ] v1.0.0 is on the Marketplace under a name that doesn't collide.

### M15: MCP prompts and the LLM summary (shared, different per repo) → part of v0.3.0 ([mcp-and-llm](specs/mcp-and-llm.md))
Scope: relgap gets an opt-in `--summarize`. tfcensus gets MCP prompts but no built-in LLM call
(decided 2026-11-13; see [roadmap-research.md](roadmap-research.md#llm-features)).
- [ ] MCP prompt support in the shared `internal/mcpkit`. relgap ships
  `upgrade_review(tool, from, to)`, which tells the agent to call `breaking_changes` and cite entry
  IDs. The mcptest lists and fetches it.
- [ ] mcp-and-llm.md criteria 3-4: a fake-provider test drops uncited claims and claims with
  unknown IDs and respects the token budget; a Bedrock smoke test runs only in `live.yml`.
- [ ] The summary is advisory only: it never changes `--fail-on`, exit codes or SARIF levels.
  Upstream text goes to the model as delimited untrusted data, repo paths and pin locations are
  never sent, and a provider failure or timeout is a warning. An ADR-0004 amendment records this.
- [ ] With no provider configured, `--summarize` makes no network call (test).
- [ ] `.claude/rules/siblings.md` lists `--summarize` as an intended difference.

### M16: Planned CLI gaps (shared where both repos lack it) → v0.3.0
- [ ] `config init` writes a commented `relgap.yaml` and refuses to overwrite without `--force`.
  The same shape as tfcensus's.
- [ ] The config search stops at the git root, through an option in the shared
  `internal/config/load.go` (the M10 decision in mcp-and-llm.md).
- [ ] `--only` is a global flag as `cli.md` says, or `cli.md` is corrected.
- [ ] A CI job fails when the embedded mise or aqua snapshot is older than 30 days
  (tool-resolution.md).

### M17: Resolution gaps → v0.4.0 (relgap only) ([tool-resolution](specs/tool-resolution.md))
- [ ] `tfprovider:` resolves any registry.terraform.io provider, not only curated ones.
- [ ] `chart:` (Artifact Hub), `npm:` and `pypi:` resolve, and argo-cd joins the live corpus.
- [ ] `relgap registry update` and `--guess` from the resolution order in tool-resolution.md.

### M18: Performance and integration → v0.4.0 (relgap only)
- [ ] `BenchmarkScan60Repos` and `BenchmarkIndex1MB` from testing.md exist. The overview targets
  (outdated with about 45 tools under 10 s cold and 2 s warm, a warm digest under 2 s) are
  measured and recorded.
- [ ] The `docs/dogfood.md` follow-ups: split digests keep only the cited versions, upgrade-guide
  paragraphs merge, and the MCP cursor works in Claude Code.
- [ ] `drift --from-tfcensus` and the tfcensus pin detector from integration.md, using the
  tfcensus v0.1 inventory schema.

### M19: Package channels (shared, the owner picks the release)
Scope: from the next release on, publish to Homebrew and Scoop as well as GitHub.
Earlier versions stay GitHub only; the tap and the bucket start at that version.
- [ ] The tap and bucket tokens use `{{ .Env.HOMEBREW_TAP_TOKEN }}` and
  `{{ .Env.SCOOP_BUCKET_TOKEN }}`. goreleaser renders `repository.token` with
  `ApplySingleEnvOnly` (`internal/client/client.go`), so the current `envOrDefault` form fails on
  the first real publish. Skipped runs never read the token, which is why nothing caught it.
- [ ] `sassdavid/homebrew-tap` and `sassdavid/scoop-bucket` have a first commit on their default
  branch, and the owner confirms both secrets exist.
- [ ] `skip_upload` becomes
  `'{{ if and (isEnvSet "HOMEBREW_TAP_TOKEN") (not .Prerelease) }}false{{ else }}true{{ end }}'`
  (and the Scoop equivalent), in one `build:` commit that release-please lists in the changelog.
  A later `-rc` tag, if one is ever cut, stays GitHub only, like goreleaser's `skip_upload: auto`.
- [ ] The cask from the last GitHub-only release's artifact installs from a local test tap
  (`brew tap-new`) before the switch, with completions and man pages in brew's directories.
- [ ] After the next release, `brew install --cask sassdavid/tap/relgap` and
  `scoop install relgap` install it on a clean machine or runner.

### M20: Package registries (shared)
- [ ] An aqua-registry PR for `sassdavid/relgap` is merged (any time after v0.1.0).
- [ ] A mise registry PR is merged, once there are 3 or more external users. mise's `packslip:`
  backend, which carries completions and man pages, is checked again for goreleaser support.
