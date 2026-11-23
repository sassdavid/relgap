# relgap: milestones

**One milestone = one fresh Claude Code session.** Start each session with:
"Read `CLAUDE.md`, then `docs/plan/milestones.md` M<n> and the linked specs. Implement M<n> only."
Tick a box only when its check has passed, and quote the evidence (command plus result) in the commit body.

Release mapping (revised 2026-11-13): every release goes to GitHub only (archives, checksums,
SBOMs, the cosign bundle and attestations) until M19 turns the package channels on.
**v0.1.0 = M0-M11**, v0.2.0 = M12-M13, v0.3.0 = the `digest` pull request input (the first M14
item), v0.4.0 = M15-M18, and the rest of M14 releases the separate action repo
(revised 2026-11-13: v0.3.0 ships before M15; revised 2026-11-16: M17-M18 ship in v0.4.0, with no
v0.5.0). M19 has no fixed version or place in the order: the owner runs it when the
GitHub releases look right. M20 waits for external users. M22-M26 (docs site, added
2026-11-17) have no version: the site deploys with each release once M22 is done. They share
their numbers with tfcensus's docs milestones, so there is no M21 here, and M22, M23 and M26 run
in the same session in both repos. The research and decisions behind M11-M20 are in
[roadmap-research.md](roadmap-research.md).

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
- [x] Uses at most 2 GraphQL queries for "latest version" of 45 tools (counter). Revised 2026-11-16 by plan ADR-0013: at most 5, sent in parallel.

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
- Moved to M14: `digest` taking the upgraded tools of a PR. v0.2.0 shipped without it; v0.3.0
  has it.
- [x] Markdown truncation counts characters, defaults to 60,000 (GitHub's cap is 65,536), cuts at
  row or section boundaries and adds a footer naming what was cut (output-and-ci.md criterion 4,
  formerly `--md-max-bytes`). A property test checks the limit. The flag has the same name in both
  CLIs.
  Evidence: `--md-max-chars` in both CLIs; `TestDigestMarkdownTruncation` (300 generated digests)
  and the shared `TestTruncateMarkdownLimit`; `digest_kubectl.txtar` checks the cut of a
  digest over 60,000 characters. `docs/ci/outdated.yml` uses `--out` and the comment, and
  `jactionlint docs/ci/outdated.yml` exits 0.
- [x] v0.2.0 is released.
  Evidence (2026-11-13): release PR #28 merged; release run 43446534947 published v0.2.0 (tag on
  095beb4) with 14 assets and the `package-manifests` artifact. On
  `relgap_0.2.0_darwin_arm64.tar.gz` cosign printed "Verified OK", `shasum -c` "OK" and
  `gh attestation verify --signer-workflow sassdavid/relgap/.github/workflows/release.yml` exited
  0; `mise use github:sassdavid/relgap@0.2.0` installed it and `relgap version` prints 0.2.0. The
  tap and the bucket API still answer 409 "Git Repository is empty".

### M14: GitHub Action → `sassdavid/relgap-action` v1.0.0 (clone at `~/src/github/relgap-action`, empty) (shared with tfcensus-action)
Scope: a composite action in its own repo that installs a verified release and wires M13's flags.
Design in [roadmap-research.md](roadmap-research.md#github-action). `comment-on-renovate` becomes
mode `comment` with `command: digest`; update output-and-ci.md to match.
- [x] Moved from M13, needed by mode `comment` with `command: digest`: `digest` can take the
  upgraded tools of a PR: the pins at the base ref against the head ref, or Renovate's branch name
  and JSON comment (output-and-ci.md). It ships in relgap v0.3.0, before the action's v1.0.0.
  Evidence (2026-11-13, local): `digest --base <ref> [--head <ref>]` and `digest --pull-request`;
  `TestSpec_output_and_ci_8` runs `spec_output_and_ci_8.txtar` on a real git repository
  (upgrades, added, removed and constraint pins, `--fail-on`, the Renovate branch filter, the
  base fallbacks and the usage errors), and `TestDiff`, `TestRenovateTopicMatches`,
  `TestRenovateTargetBranch` and `TestExtractRef` cover `internal/scan`. `mise run check` passes.
- [x] `scripts/sibling-diff.sh` reads the tool name from a `.sibling` file or the directory name
  instead of `go.mod`, stays verbatim in all four repos, and each action repo has its file list.
  Evidence (2026-11-15, local): `mise run sibling:diff` prints "shared files match" in relgap,
  tfcensus, relgap-action and tfcensus-action. An action repo also compares the verbatim files
  and `writing.md` with its tool's repo. A copy in a directory with another name exits 2 and
  passes with `.sibling`. `.release-please-manifest.json` moved to the similar list, since the
  two tools release different versions.
- [x] Skeleton: `action.yml`, `scripts/install.sh`, `scripts/run.sh` and `scripts/tool.env` (the
  only tool-specific file besides the README). Install checks sha256 and
  `gh attestation verify --signer-workflow sassdavid/relgap/.github/workflows/release.yml`, with
  cosign and checksum-only as options. A tampered archive fails in CI. CI runs on ubuntu, macos
  and windows, zizmor and actionlint pass, every `uses:` is pinned by SHA, and release-please
  publishes immutable releases with a moving `v1` tag.
  Evidence (2026-11-15, relgap-action v1.0.0 = 4c18623): `ci` run 43727575743 passed `lint`
  (jactionlint, shellcheck, prose), `test` on ubuntu, macos and windows (`scripts/test-install.sh`
  fails a tampered archive in every verify mode) and the action job for attestation, cosign and
  checksum on all three. `security` run 43727575763 passed zizmor. Every `uses:` is a SHA or `$/`.
  release-please drafted v1.0.0 from #2. After it was published, the API reports
  `"immutable": true`, `gh release verify v1.0.0` prints "Release v1.0.0 verified!", and
  `major-tag` run 43728377470 moved `v1` to 4c18623, the commit of `v1.0.0`.
- [x] Report mode: `outdated` or `drift` writes json, SARIF, md and annotations in one run. SARIF
  shows in code scanning on a test repo, and `fail-on` fails the step on exit 3.
  Evidence (2026-11-15, sassdavid/action-test, relgap-action#3): push run 43722958845 uploaded
  `relgap-outdated`, and code scanning lists 16 open alerts on `mise.toml` lines (eol, outdated,
  breaking, deprecation and 6 advisories). The macOS and Windows jobs wrote all three files. The
  dispatched `gate` job (run 43723750568) printed `--fail-on eol matched 2 pins` and failed with
  exit code 3.
- [x] Comment mode: one sticky comment on a test repo's Renovate PR. A re-run edits it with no
  duplicate, and a fork PR falls back to the step summary.
  Evidence (2026-11-15, sassdavid/action-test): the self-hosted `sassdavid-renovate` app opened
  #4 (gh) and #5 (helm) on `renovate-self-hosted/` branches, and each got one comment that
  digests only its tool. A re-run of 43726011622 logged `unchanged` and left one comment. On #1 a
  second commit edited the same comment (new hash). `scripts/test-run.sh` runs the fork and
  Dependabot fallback with a fake event on ubuntu, macos and windows.
- [x] v1.0.0 is on the Marketplace under a name that doesn't collide.
  Evidence (2026-11-15): published at 15:10 UTC as https://github.com/marketplace/actions/relgap
  (HTTP 200). Before that the page answered 404, and GitHub has no user or org named relgap.

### M15: MCP prompts and the LLM summary (shared, different per repo) → part of v0.4.0 ([mcp-and-llm](specs/mcp-and-llm.md))
Scope: relgap gets an opt-in `--summarize`. tfcensus gets MCP prompts but no built-in LLM call
(decided 2026-11-13; see [roadmap-research.md](roadmap-research.md#llm-features)).
- [x] MCP prompt support in the shared `internal/mcpkit`. relgap ships
  `upgrade_review(tool, from, to)`, which tells the agent to call `breaking_changes` and cite entry
  IDs. The mcptest lists and fetches it.
  Evidence: `TestPrompt_*` and `TestAddPrompt_duplicateArgument` (`internal/mcpkit/prompt_test.go`,
  identical in tfcensus); `TestMCP_upgradeReview` (`internal/cli/mcp_test.go`) lists and fetches
  the prompt and checks that `breaking_changes` and `digest` give an entry the same `id`;
  `TestDigestEntryIDs` (`internal/report`). Entries had no ID, so every digest entry now has a
  stable `id` (mcp-and-llm.md, "Decisions (M15, prompts and entry IDs)").
- [x] mcp-and-llm.md criteria 3-4: a fake-provider test drops uncited claims and claims with
  unknown IDs and respects the token budget; a Bedrock smoke test runs only in `live.yml`.
  Evidence: `TestSpec_mcp_and_llm_3` (`internal/summarize`) drops claims without an ID, with an
  unknown ID and outside the headings, caps risks at 5, and with a 520-token budget leaves out
  the two lowest-confidence entries and drops a claim citing one of them.
  `TestSpec_mcp_and_llm_4` (build tag `live`, `mise run test:live:bedrock`) runs in the
  `bedrock` job of `live.yml` only when `RELGAP_BEDROCK_ROLE_ARN` and `RELGAP_BEDROCK_MODEL` are
  set, and skips without a model or credentials. Neither variable is set yet, so it hasn't run
  against Bedrock; `TestBedrock` checks the Converse request against a fake endpoint.
- [x] The summary is advisory only: it never changes `--fail-on`, exit codes or SARIF levels.
  Upstream text goes to the model as delimited untrusted data, repo paths and pin locations are
  never sent, and a provider failure or timeout is a warning. An ADR-0004 amendment records this.
  Evidence: `TestSummarize_advisory` (`internal/cli`) gets exit 3 with `--fail-on breaking` with
  and without `--summarize` while the model claims "No breaking changes", and the same entries;
  `digest` writes no SARIF. `TestSummarize_untrustedData` checks that an entry containing
  `</release_notes>` can't close the block and that only the tool, range and entries are sent.
  `TestSummarize_warnings` covers a 503 (one call, no resend), a 50ms `llm.timeout`, `--offline`
  and a missing key. ADR-0004 has the 2026-11-15 amendment.
- [x] With no provider configured, `--summarize` makes no network call (test).
  Evidence: `TestSummarize_noProviderNoNetwork` records the same requests with and without
  `--summarize`, none to the model host; `TestNew` builds no client and fails on any request.
- [x] `.claude/rules/siblings.md` lists `--summarize` as an intended difference. It already did
  ("relgap only: `--summarize` and its `llm` config"); the box waited for `--summarize` to work.
  All four boxes: `mise run check` passed on 2026-11-15 (lint, `go test -race ./...` with 26
  packages ok, govulncheck "No vulnerabilities found"), and `mise run sibling:diff` reports
  "shared files match".

### M16: Planned CLI gaps (shared where both repos lack it) → v0.4.0
- [x] `config init` writes a commented `.relgap.yaml` and refuses to overwrite without `--force`.
  The same shape as tfcensus's.
- [x] The config search stops at the git root, through an option in the shared
  `internal/config/load.go` (the M10 decision in mcp-and-llm.md). `relgap mcp` passes its first
  `--root` as `config.Options.StopDir`, and outside a repository only the working directory counts.
- [x] `--only` is a global flag as `cli.md` says, or `cli.md` is corrected. `cli.md` now lists it as a
  `digest` flag, and it works with `--from-mise` and `--all` (ADR-0003's example).
- [x] A CI job fails when the embedded mise or aqua snapshot is older than 30 days
  (tool-resolution.md): `registry.yml`, weekly and on snapshot changes.

### M17: Resolution gaps → v0.4.0 (relgap only) ([tool-resolution](specs/tool-resolution.md))
- [x] `tfprovider:` resolves any registry.terraform.io provider, not only curated ones.
- [x] `chart:` (Artifact Hub), `npm:` and `pypi:` resolve, and argo-cd joins the live corpus.
- [x] `relgap registry update` and `--guess` from the resolution order in tool-resolution.md.

### M18: Performance and integration → v0.4.0 (relgap only)
- [x] `BenchmarkScan60Repos` and `BenchmarkIndex1MB` from testing.md exist. The overview targets
  (outdated with about 45 tools under 10 s cold and 2 s warm, a warm digest under 2 s) are
  measured and recorded. (testing.md#measured-performance: cold 5.75 s, warm 0.081 s, warm digest
  0.032 s, after plan ADR-0013.)
- [x] The `docs/dogfood.md` follow-ups: split digests keep only the cited versions, upgrade-guide
  paragraphs merge, and the MCP cursor works in Claude Code. (`TestSpec_mcp_and_llm_5`,
  `TestSpec_classification_5`; dogfood.md "Claude Code, 2026-11-16": argocd 3.1.0 to 3.6.1 in two
  pages of 52 and 6 entries with 8 and 1 versions.)
- [x] `drift --from-tfcensus` and the tfcensus pin detector from integration.md, using the
  tfcensus v0.1 inventory schema. (`TestSpec_pindiscovery_7`, `TestSpec_pindiscovery_1`,
  plan ADR-0014; fixture copied from tfcensus `testdata/golden/inventory/tf-fleet.json`.)

### M19: Package channels (shared, the owner picks the release)
Scope: from the next release on, publish to Homebrew and Scoop as well as GitHub.
Earlier versions stay GitHub only; the tap and the bucket start at that version.
- [x] The tap and bucket tokens use `{{ .Env.HOMEBREW_TAP_TOKEN }}` and
  `{{ .Env.SCOOP_BUCKET_TOKEN }}`. goreleaser renders `repository.token` with
  `ApplySingleEnvOnly` (`internal/client/client.go`), so the old `envOrDefault` form would have failed on
  the first real publish. Skipped runs never read the token, which is why nothing caught it.
  Evidence (2026-11-16, goreleaser 2.20.2): `envOnlyRe` in `internal/tmpl/tmpl.go` accepts only
  `{{ .Env.NAME }}`, and the cask and Scoop `doPublish` return on `skip_upload: true` before
  `client.NewIfToken` reads the token. With both variables unset, `goreleaser check` validates
  the file and `goreleaser release --snapshot --clean --skip=sign,sbom` renders the cask and the
  manifest.
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

### M22: Docs site foundation (shared) ([docs-site](specs/docs-site.md), [docs-design](specs/docs-design.md))
Scope: the site builds in CI, deploys on release to `https://relgap.sasskovacs.dev`, has its
theme, fonts and home page, and serves the existing user docs. The CLI reference stays excluded
until M23. Decisions: engineering [ADR-0016](../engineering/adr/0016-docs-site.md) (proposed
2026-11-17; this milestone accepts it in both repos). tfcensus does the same milestone with its
own theme in the same session.
- [x] ADR-0016 is accepted in both repos and listed in `docs/engineering/README.md`.
- [x] `mise.docs.toml` pins node 26.13.1, aube 2.10.1, lychee and oxfmt, `mise.lock` has their
  entries, and `MISE_ENV=docs mise install` works on a clean checkout. The entries are in
  `mise.docs.lock`, as `mise.e2e.lock` is for the e2e tools, and oxfmt's npm lockfile is under
  `.mise/locks/`.
- [x] `docs/package.json` (exact versions), `docs/aube-lock.yaml` and `docs/aube-workspace.yaml`
  exist. `aube install --frozen-lockfile` passes with lifecycle scripts denied, or each allowed
  package is listed with a reason.
- [x] `docs/.vitepress/config.mts` follows docs-site.md "Site configuration": cleanUrls,
  lastUpdated, sitemap, local search, editLink, srcExclude (including `cli/**` for now), the
  version nav item from `.release-please-manifest.json`, and the `description` check in
  `transformPageData`.
- [x] `theme/tokens.css` has every token in docs-design.md for both themes, fonts load from the
  Fontsource packages, and the contrast check passes (docs-design criterion 1).
- [x] `HomeHero`, `StrataStack`, `EntryMark`, `VersionRange`, `AnswerRows` and `SiteFooter` exist.
  The stack reads `home-strata.json`, written by a generator from the kubectl digest golden. The home page matches the
  wireframe at 1280 px and 400 px (screenshots in the PR).
- [x] The existing user pages (install, CI) build and have a `description`. The
  build has no dead links. relgap has no CI page yet (`docs/ci/` holds only workflows), so
  `install.md` is the one page.
- [x] mise tasks `docs:dev`, `docs:build`, `docs:preview` and `lint:docs` exist. `docs:build` runs
  the checks in docs-site.md "Checks" that exist at this point (stale check, unit tests, build,
  lychee). `mise run check` includes `lint:docs`.
- [x] `.github/workflows/docs.yml` builds on pull requests and deploys through `workflow_call`
  from `release-please.yml` and through `workflow_dispatch`. jactionlint passes. A dispatch run
  from the latest tag deployed the site: `curl -sI https://relgap.sasskovacs.dev/` shows 200 and
  the `http://` URL redirects. "Enforce HTTPS" is on.
  The dispatch ran from `main` (run 44052789931), because v0.4.0 predates the site; the spec allows
  `ref: main` for a manual deploy.
- [ ] `.github/workflows/aube-lock.yml` and the `aube-lock` environment exist. A test Renovate
  branch that bumps a docs dependency gets a lockfile commit and green CI. `renovate.json` covers
  `docs/package.json`, and `docs/ci/` stays out of scope as before.
  The workflow, the environment (branch rule, secret, variable) and the `renovate.json` rule are in
  place; the test Renovate branch is still to come.
- [x] `lint:prose` covers the published pages, with a test that it doesn't skip them.
- [x] `scripts/sibling-diff.sh` lists the new files as docs-site.md "Sibling sharing" says, and
  `mise run sibling:diff` passes in both repos.

### M23: Generated reference (shared) ([docs-site](specs/docs-site.md#generated-content-shared-generators))
Scope: every fact the code can state reaches the site from code, with a test per page. The
generators are shared with tfcensus; tool-specific catalogues wait for M24.
- [x] `scripts/gendocs` writes the nested CLI reference with frontmatter, flag tables, formats,
  `--fail-on` values, exit codes, examples, subcommands and related links; one global flags page;
  one completion page; escaped `Long` text with indented blocks as fences; `commands.json`. The
  flat `relgap_*.md` files are gone, links to them are updated, and `cli/**` leaves
  `srcExclude`. (docs-site criterion 4)
- [x] `Annotations["formats"]`, `["fail-on"]` and `["exit-codes"]` are set on every command, with
  tests that run the commands and compare. The stale help texts in docs-content.md (`registry lint`,
  `outdated` skip list after its cassette check, `--concurrency`, `scan.detectors.ci_actions`,
  `github.host`, `--no-input`) are fixed.
  Every runnable command has the annotations; group commands run nothing. The root checks
  `--format`, `--out` and `--fail-on` against them before the command runs. The exit-code test
  reads the code behind each RunE (`TestAnnotations_exitCodes`); `mcp` is the one exception, with
  its reason in the test.
  The `outdated` skip list is confirmed by `testdata/script/outdated_skip.txtar`, which runs
  offline with an empty cache, so the lookups fail without a request instead of replaying a
  cassette.
- [x] A test fails for a non-hidden command without `Long` or `Example` outside the allowlist,
  and the missing texts listed in docs-content.md are written.
- [x] `/reference/config`, `/reference/environment`, `/reference/exit-codes`,
  `/reference/fail-on`, `/reference/schemas` and `/reference/mcp` are generated, each with the test docs-site.md names.
  Schemas are served at `/schemas/<name>.json` and their `$id` matches. (docs-site criterion 5)
  The tests: `TestConfigSchema`, `TestEnvironment`, `TestAnnotations_exitCodes`, `TestSchemaIDs`
  and the MCP page in `TestGenerated`. Every schema's `$id` moved to the site URL.
- [x] `internal/docsurl` exists, and help texts, runtime messages and the config template use it.
  A test checks every path against the built site. (docs-site criterion 6)
  `docs:build` runs `TestPaths` in `internal/docsurl` with `DOCS_DIST` set, which also checks anchors.
- [x] The snippet check runs in `docs:build` and finds nothing. (docs-site criterion 7)
- [x] `sidebar.mts` builds the CLI section from `commands.json`, and `llms.txt` is generated and
  stale-checked.
- [x] `mise run docs` regenerates everything and exits 1 on any change. A PR that changes a flag
  without regenerating fails CI. (docs-site criterion 3)

### M24: Getting started and tool reference (relgap only) ([docs-content](specs/docs-content.md))
Scope: the tutorial, install and every relgap-specific reference and concept table. Rerun the
audit for the pages this milestone writes before writing them.
- [x] `scripts/gensite` generates supported tools, registry snapshots, the registry entry schema,
  SARIF rules, the detector table, source kinds with placeholders and allowed hosts, the
  classification confidence table and the network host list, each with a stale check in
  `mise run docs`. Constants the tables need are exported or covered by a doc test, so no
  generator parses Go source.
  The reference pages are under `docs/reference/`, and the concept tables are fragments in
  `docs/parts/` (srcExclude) for the M25 pages to include. The new exports are
  `scan.DetectorDocs`, `source.Kinds`, `registry.Placeholders`, `classify.Signals`,
  `render.SARIFRule`, `OutdatedRules` and `DriftRules`. The doc tests are `TestDetectorDocs`,
  `TestSignals`, `TestSourceKinds`, `TestPlaceholders`, and `TestNetworkHosts`, which checks every
  host in the cassettes. docs-content.md "Generated by scripts/gensite" lists each file.
- [x] The relgap-action inputs and outputs table is generated from the action repo's `action.yml`
  at its pinned release.
  `docs/ci/relgap-action.yml` pins v1.1.0 (`28d7a84`), and Renovate moves that pin.
  `scripts/gensite/relgap-action.yml` is a copy of `action.yml` at that commit, fetched through
  httpx by `mise run docs:action`. gensite fails when the copy and the pin name different commits.
- [x] Getting started runs a first digest and a repo scan against cassettes, with every output
  included from goldens. Install, `/concepts/network` and the changelog page are written.
  Two new goldens: `digest/kubectl-breaking.txt` (`digest_kubectl.txtar`) and `pins/project.txt`
  (`spec_output_and_ci_5.txtar`, which compared an inline copy before). gensite's transcript check
  fails when an included golden doesn't follow the command a script compares with it. Install
  drops the Homebrew and Scoop steps until M19. The changelog includes `CHANGELOG.md` verbatim, and
  the nav version item links to it.
- [x] `SentToModel` lists what `--summarize` sends, read from the code that builds the request.
  `summarize.SentToModel` feeds `theme/data/sent-to-model.json` and the `SentToModel` component on
  `/concepts/network`. `TestSentToModel` compares its fields with the JSON lines of the user
  message that `Summarize` builds. Checking the page's claims found the Anthropic `X-Api-Key`
  header unredacted in trace logs, so httpx now redacts it (`TestTraceRedactsCredentials`).
- [x] The never-on-the-site checks pass. (docs-content criterion 4)
  `TestNeverOnSite` in `scripts/gensite`, the same shape as tfcensus's, checks for tokens, local
  paths, links into internal docs, milestone names (not in the changelog) and test internals.
  It checks the sources, and the built site with `DOCS_DIST`, which `docs:build` sets.
  Evidence (2026-11-17): `mise run docs` exits 0. `mise run docs:build` passes: snippet check,
  vitepress build, `TestPaths`, `TestNeverOnSite`, and lychee with 0 errors. `mise run check`
  passes.

### M25: Guides and concepts (relgap only) ([docs-content](specs/docs-content.md))
Scope: every guide, concept, help and project page in docs-content.md, and every gap closed.
If one session can't finish, the open pages carry over to a follow-up session before M26.
- [x] Every guide and concept page in docs-content.md exists with a `description` and passes the
  snippet check. The CI guides embed `docs/ci/*.yml`. (docs-content criteria 1 and 2)
  15 guides in `docs/guides/` and 7 new concept pages; ci-gate, renovate-pr and action embed
  `outdated.yml`, `digest.yml` and `relgap-action.yml`. Two structure pages no milestone owned
  are written too: `/reference/notify` and `/reference/output`. Shell completion moved from
  `install.md` to `/guides/completion`. Command pages link their guides through `setRelated`,
  which showed that gendocs titled every Related link with the generated-file comment: fixed in
  its own PR (7c94f11, #73, `TestTitleOf`), paired with tfcensus#90.
- [x] Every row in "wrong or stale statements" is fixed, each with its commit in the evidence.
  (docs-content criterion 3)
  `scan.detectors.ci_actions`, `github.host`, `--no-input`, the `outdated` skip list,
  `registry lint` and `--concurrency`: 8cae757 (M23). README's kind order and its dogfood and
  plan links, the cache directory (said once on `/guides/cache`), `docs/plan/cli.md`,
  `specs/config.md` and the `specs/mcp-and-llm.md` tool table: b1e6a8e (#74). `docs/README.md` stays
  with M26, as the row says. Checking the new pages against the code fixed four more in b1e6a8e:
  the `digest --line` default, the cache help's 1-day list, the `breaking_markers` schema text
  (regular expressions, not literal text), and the version-jump row in `classify.Signals`.
- [x] README is reduced to the introduction, install and links. (docs-content criterion 5)
- [x] Troubleshooting, FAQ, security and contributing pages exist, and this site and tfcensus's
  inventory guide link to each other.
  The four pages exist. `/guides/from-tfcensus` links tfcensus's `/concepts/state#feed-relgap`
  (tfcensus has no inventory guide; its M25 session chose that section). tfcensus#91 (e676d37)
  put the link back on its main: the "Feed relgap" section of `docs/concepts/state.md` and the
  FAQ entry "Can relgap read its inventory?" both link `/guides/from-tfcensus`.
  Evidence (2026-11-17): `mise run docs` exits 0. `mise run docs:build` passes: snippet check,
  transcripts, vitepress build, `TestPaths`, `TestNeverOnSite`, and lychee with 0 errors.
  `mise run check` passes.

Shared follow-ups found during M25 (also listed under tfcensus's M25), now fixed in both repos:
- The snippet check (`checkValue` in `scripts/gendocs/snippets.go`) splits values on commas for
  every flag that completes a comma-separated list, not only for `stringSlice` flags
  (`TestCheckValue`).
- `SECURITY.md` is in both tool repos, with the same text once the tool name is swapped, and
  `/security` links it.

### M26: Polish and launch (shared) ([docs-site](specs/docs-site.md), [docs-design](specs/docs-design.md))
Scope: social cards, icons, accessibility and performance, then the first deploy by a real
release.
- [x] `social.mjs` and `social-check.mjs` generate and check a card per page with the bundled
  font and its license file. The glyph, favicons and manifest are in place. (docs-design
  criterion 4, docs-site criterion 8)
  `vitepress build` renders a 1200 by 630 card per page with `@napi-rs/canvas` 1.1.1 and the
  site's Atkinson Hyperlegible Next variable font from its fontsource package (OFL `LICENSE` in
  the package), at weights 700 and 800. `social-check: 68 pages have a card and every icon`.
  `favicon.svg` (with a dark variant), `site.webmanifest`, and `favicon.ico`,
  `apple-touch-icon.png`, `icon-192.png` and `icon-512.png`, which `node
  docs/.vitepress/social.mjs` renders from `glyph.svg` with a stale check. `social.test.mjs`
  covers wrapping, sections, the ICO and PNG headers and the check.
- [x] axe shows no WCAG 2.2 AA violation on the pages docs-site.md lists in both themes, and
  Lighthouse gives accessibility 100 and best practices at least 95 on the home page and a CLI
  page. (docs-site criterion 9, docs-design criterion 5)
  `mise run docs:a11y` (`a11y.mjs`: `@axe-core/playwright` 4.14.0 in headless Chrome driven by
  playwright-core 1.65.1, WCAG 2.0 to 2.2 A and AA plus best practices) reports 0 violations on
  `/`, `/guides/digest`, `/cli/digest` and `/reference/config` in light and dark, and the home
  page has 11 animations with motion and 0 with `prefers-reduced-motion: reduce`. `docs.yml` runs
  it after `docs:build`, where it passed on the runner's Chrome. Lighthouse 13.6.0, run by hand
  against `vitepress preview` (2026-11-17, chrome-headless-shell), gives accessibility 100 and
  best practices 100 on `/` and `/cli/digest`, desktop and mobile. Fixed on the way, in both
  repos: the search button's K key moved into CSS
  (axe `label-content-name-mismatch`), the CLI sidebar group links `/cli/` (`nested-interactive`),
  the home page wraps its components in `<main>` (Lighthouse `landmark-one-main`), and tfcensus's
  hidden tally table no longer scrolls (`scrollable-region-focusable`).
- [x] The built site makes no third-party request. (docs-site criterion 10)
  `third-party-check: 299 files load nothing from another host` (HTML `src` and resource `href`,
  CSS `url()` and `@import`, and `fetch`, `import()`, WebSocket, EventSource and beacons in the
  JS), and `docs:a11y` logs every request the four pages make in both themes and found none that
  left the site.
- [x] `docs/README.md` is the contributor guide for the site: tasks, where pages go, generated
  files, voice.
- [x] A release created by release-please deployed the site through `workflow_call`, with the run
  ID in the evidence. (docs-site criterion 2) v0.5.0, release-please run 44053615930: the docs build
  and deploy jobs passed, and the site answers 200.
- [x] `mise run sibling:diff` passes in both repos. (docs-site criterion 11)
  "shared files match" in tfcensus, relgap and both action repos, with the six new shared files of
  `docs/.vitepress` and `docs/.oxlintrc.json` on the identical list and `docs/README.md` on the
  similar list.

Found while writing M26:
- aube refused puppeteer-core and lighthouse: chromium-bidi has published without provenance since
  10.4.0, and `chromium-bidi@157.0.8090-0` is a trust downgrade (`ERR_AUBE_TRUST_DOWNGRADE`). The
  trust policy stays as it is. `@axe-core/playwright` and playwright-core run the axe check, and
  Lighthouse is measured by hand (docs-site.md "Checks").
- Every Node dependency and docs tool was reviewed against npm (release dates, weekly downloads,
  provenance), jdx's sites in `_reference` and current guides. `@resvg/resvg-js` (no stable release
  since 2024-03, no variable font weights) became `@napi-rs/canvas`, the hand-written DevTools
  client became playwright-core with `@axe-core/playwright`, oxlint joined oxfmt in `lint:docs`,
  and the icons follow Evil Martians' favicon guide. VitePress stays at 1.6.4 (2.0 is alpha.21),
  and oxfmt at 0.78.0 (0.79.0 is under the 3-day minimum age).

### M27: Renovate annotations and Terraform module pins (relgap only) ([pin-discovery](specs/pin-discovery.md#renovate-annotations-and-terraform-modules-m27))
Scope: pins that only a Renovate regex manager or a Terraform `module` block names today. Found on
2026-11-19 while testing 0.9.1 on a Terraform and Helm module repository, whose chart versions are
variable defaults such as `chart_version = optional(string, "1.23.0")` under a `# renovate:`
comment. Decisions: [ADR-0015](adr/0015-renovate-annotations-and-terraform-modules.md).
- [x] A `# renovate: datasource=<ds> depName=<name>` comment above a line with a version string
  makes that version a pin in the files the spec lists, with the datasource mapping it gives.
  (pin-discovery criterion 8, `TestSpec_pindiscovery_8`: the new `renovate-comment` detector,
  fixture `pins_renovate_comment.txtar`. `--renovate-report` helm dependencies keep their
  `registryUrls` too.)
- [x] `module` blocks with a registry source and a `version` give `tfmodule:` pins that resolve
  through the Terraform registry, and GitHub git sources with `?ref=` give `github:` pins.
  (pin-discovery criterion 9, `TestSpec_pindiscovery_9` and `TestRemoteModule`. Terragrunt
  `terraform { source }` with `tfr://` or `?ref=` too.)
- [x] `outdated`, `digest --base`, `drift` and `--pull-request` work on a fixture repository with
  both kinds of pins.
  (pin-discovery criterion 10, `TestSpec_pindiscovery_10` and `TestRemoteChartRepo`: the chart's
  `registryUrl` picks the external-dns Artifact Hub repository, which a bare `chart:external-dns`
  can't, and `Chart.yaml` `repository` URLs do the same.)
- [x] Hugo `{{<ref "...">}}` and `{{< relref >}}` link targets in a changelog file become working
  links: karpenter's upgrade guide gives `blob/HEAD/...{{<ref ...` URLs in its digest entries today
  (found with PR #87).
  (source-adapters criterion 7, `TestSpec_source_adapters_7`: karpenter's entry sets
  `site_url: https://karpenter.sh/`, and `{{% alert %}}` lines are dropped. The 11 karpenter.sh
  links in a live `digest karpenter 1.0.0..1.12.0` answer 200, and the 9 with a fragment have that
  id on the page.)

Found with 0.10.0 on a Terraform module repository (Helm chart versions in variable defaults under
`# renovate:` comments):
- [x] A chart pin and a `github:` pin whose specs share a name (`aws-for-fluent-bit`) each compare
  against their own repository: caches key on the full tool name or on what they fetch, never on
  the spec's name. A chart `registryUrl` that Artifact Hub doesn't list becomes
  `chart:<url>/<name>` and never falls back to a search by name.
  ([ADR-0016](adr/0016-tool-identity-and-chart-registry-urls.md), tool-resolution criterion 7,
  `TestSpec_tool_resolution_7` and `TestOutdatedSameNameDifferentTools`)
- [x] A chart in a repository without a tag per chart version (aws/eks-charts tags `v0.0.N`)
  shows latest and behind from Artifact Hub, with a note that names the reason and no counts, and
  its digest says so. A GitHub Pages `registryUrl` that Artifact Hub doesn't list reads the
  `gh-pages` `index.yaml`. (tool-resolution criterion 8, `TestSpec_tool_resolution_8`)
- [x] A `# renovate:` comment's `extractVersion=^<prefix>(?<version>...)$` on a github datasource
  names `github:<owner>/<repo>@<prefix>`. (pin-discovery criterion 8, `TestExtractPrefix`,
  `TestResolveTagPrefix`)
- [x] `digest --base` digests a raised lower bound from the old bound to the new one, and each
  digest lists the changed pins it covers. (output-and-ci criterion 9, `TestSpec_output_and_ci_9`)
- [x] Ignore rules match a raw name (glob) or a path alone, so unresolved pins can be silenced.
  (config criterion 6, `TestIgnoredByName`)
  Decisions: [ADR-0017](adr/0017-chart-versions-extract-version-and-floors.md). Bitbucket Cloud and
  GitLab module sources and image tags in `outdated` are M28.

### M28: Versions and notes outside GitHub (relgap only) ([non-github-sources](specs/non-github-sources.md))
Scope: Terraform module sources on Bitbucket Cloud and GitLab, and `image:` pins in `outdated`.
Found on 2026-11-20 with relgap 0.10.0 on a Terraform module repository. Decisions:
[ADR-0018](adr/0018-non-github-version-sources.md), accepted on 2026-11-20 with the owner's answers.
- [x] Bitbucket Cloud module sources compare and digest from the repository's changelog file.
  (non-github-sources criterion 1)
- [x] gitlab.com and configured self-managed GitLab sources compare and digest from releases.
  (non-github-sources criterion 2)
- [x] Tokens reach only their host and never the cache or a cassette. (non-github-sources
  criterion 3)
- [x] `outdated` compares `image:` pins against registry tags and digests the source label's
  repository. (non-github-sources criterion 4)
- [x] `pins --unresolved` no longer lists these sources, and the network page names every new
  host and token. (non-github-sources criterion 5)

### M29: Vulnerability audit (relgap only) ([vulnerability-audit](specs/vulnerability-audit.md))
Scope: OSV checks for every pin, not only the 14 curated tools; a `relgap audit` command that
lists what it couldn't check; and the suppression, prioritisation and CI outputs other audit tools
have. Found on 2026-11-23 from mise's `mise audit` (jdx/mise#15942). Decisions:
[ADR-0020](adr/0020-vulnerability-audit.md), accepted on 2026-11-23 with the owner's answers;
its open details were settled on 2026-11-23. The boxes are in build order, and each ships on its own. If
one session can't finish, the open boxes carry over to a follow-up session.
- [x] Pins without a curated `osv` list get OSV packages from their backend, `go` also queries
  `toolchain`, and private sources are never sent. (vulnerability-audit criterion 1)
- [x] Unchecked pins are listed with a reason in every format of `outdated`, and VULNS shows `?`
  for them. `audit`'s formats list them with box 3. (vulnerability-audit criterion 2)
- [x] `relgap audit [roots...]` with its formats, `--tool`, the default `vuln` gate and
  `--fail-on unchecked`. (vulnerability-audit criterion 3)
- [x] Unrated records take their GHSA alias's rating and otherwise count at every threshold,
  withdrawn records are dropped, and records are cached by `modified`. (vulnerability-audit
  criterion 4)
- [x] Each finding has a fix target, and an affected latest version warns. (vulnerability-audit
  criterion 5)
- [x] Action pins are checked against the GitHub Actions ecosystem. (vulnerability-audit
  criterion 6)
- [x] `vulns.ignore` with reason and expiry, OpenVEX input and `--format openvex`.
  (vulnerability-audit criterion 7)
- [x] Opt-in KEV and EPSS annotations and `--fail-on kev`. (vulnerability-audit criterion 8)
- [ ] The MCP `audit` tool, `notify` kind `audit`, and relgap-action `command: audit`.
  (vulnerability-audit criterion 9)
- [x] The audit guide, the network page and the generated reference. (vulnerability-audit
  criterion 10)
- [ ] Private repositories on github.com and gitlab.com, private `go:` modules, private scoped
  npm packages and the main modules of `outdated --binaries` executables reach OSV only as an
  ADR-0020 detail settled with this box decides. Found with box 10 on 2026-11-23; today the
  network page names an `ignore` rule as the way out (vulnerability-audit, Network).
