# relgap

Go CLI: release-notes range digests, toolchain drift across repos, EOL/OSV enrichment, MCP server.

## Verify (run before claiming anything works)
- `mise run check`: lint (golangci-lint, prose, actionlint) + `go test -race ./...` + govulncheck. Must pass before every commit.
- `mise run test:golden -- -update` regenerates goldens. Review the diff and never update blindly.
- `mise run docs` regenerates `docs/cli/`. CI fails if it's stale.

## Plan
- Milestones: `docs/plan/milestones.md`. Work on **one milestone per session**, the one named in the prompt.
- Before coding, read that milestone's linked spec(s) in `docs/plan/specs/`. Specs are the definition of done.
- Tick a milestone box only after its check passed. Put the command and its result in the commit body.
- Decisions live in `docs/plan/adr/` and `docs/engineering/adr/`. Don't contradict an ADR silently: propose a new one.

## Layout
- `cmd/relgap/main.go` only calls `cli.Run`. Commands are in `internal/cli/` (constructor functions, no globals).
- Domain packages: `scan`, `misex`, `registry`, `version`, `github`, `source`, `mdindex`, `classify`, `enrich`, `report`, `render`, `summarize`, `mcpserver`, `httpx`, `cache`, `config`.
- Curated tool entries: `registry/*.yaml` (schema `schemas/registry-v1.json`).
- Fixtures: `testdata/cassettes`, `testdata/script/*.txtar`, `testdata/repos/`, `testdata/labels/`.

## Reference sources (read-only, for inspiration)
- Local clones live in `~/src/github/_reference/<owner>/<repo>` (list and license notes: `docs/plan/reference-repos.md`). Read them to verify formats and behaviour. **Never copy code** from BSL (terraform), AGPL (renovate) or Prosperity (dependabot) repos.
- Dependency sources are already in `$(go env GOMODCACHE)`, so read those instead of cloning.

## Conventions that aren't obvious from the code
- stdout = data, stderr = logs. Output goes through the injected `io.Writer`; never `fmt.Print*` (forbidigo).
- Logger comes from `Deps`, never `slog.Default()` (sloglint).
- Every network call goes through `internal/httpx` (retry, rate limit, ETag cache). Never `http.Get`.
- GitHub large files: use the contents API with the raw Accept header, never raw.githubusercontent.com.
- Sort versions with `version.Scheme`, never by publish date.
- A changelog file is parsed once per digest (`mdindex`). Don't re-parse per version.
- New adapter or detector → fixtures + golden + a spec acceptance-criterion test named `TestSpec_<spec>_<n>`.
- Conventional Commits. release-please owns versions and tags.

## Sibling repo
- tfcensus (`~/src/github/tfcensus`) shares this repo's build, lint, test and release setup. Follow `.claude/rules/siblings.md`: reuse its files instead of writing new ones, keep shared files identical (`mise run sibling:diff`), and make shared changes in both repos.

## Writing and comments
- All text (docs, comments, CLI help, commits, PRs) follows `.claude/rules/writing.md`. `mise run lint:prose` checks part of it.
- Go code keeps comments few: package docs in `doc.go`, one-sentence doc comments on exported names, why-comments only. Details in `.claude/rules/go.md`.

## Don't
- Don't add dependencies without a line in `docs/engineering/stack.md`.
- Don't make network calls in unit tests (cassettes only). Live tests are `//go:build live`.
- Don't put an LLM in any default code path.
