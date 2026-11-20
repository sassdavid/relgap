# Spec: configuration

Files: user `$XDG_CONFIG_HOME/relgap/config.yaml`, project `.relgap.yaml` (walk up to the git root;
outside a repository only the working directory, and `relgap mcp` stops at its first `--root`).
Precedence and provenance follow [engineering standards](../../engineering/engineering-standards.md#configuration).
Schema: `schemas/config-v1.json` (generated from the Go struct).

```yaml
# .relgap.yaml
schema_version: 1
github:
  host: github.com            # GHES later; GitLab/Codeberg out of scope for v0.x
  concurrency: 8
cache:
  dir: ""                     # default $XDG_CACHE_HOME/relgap
  ttl_override: {}            # e.g. { endoflife: 24h }
scan:
  roots: [~/src/infra, ~/src/github]
  exclude: ["**/node_modules/**", "**/.terragrunt-cache/**"]
  detectors: { ci_actions: false, generic: false }
  extra_files: ["Makefile", "**/*.env"]
tools:                         # local additions: repo (GitHub owner/repo) is the only field
  hk:
    repo: jdx/hk
  internal-cli:
    repo: acme/internal-cli    # private repos work with a token that can read them
aliases:
  TG_VERSION: terragrunt
ignore:
  - tool: java                 # don't report
  - tool: terraform
    path: "**/legacy/**"
policy:                         # defaults for --fail-on in CI
  fail_on: [eol, vuln]
  max_behind: "minor:2"
  min_confidence: 0.5
  vuln_severity: high          # empty counts every advisory
llm:                            # only for digest --summarize
  provider: ""                 # off; openai-compatible, anthropic, bedrock or bedrock-mantle
  model: ""                    # required, e.g. claude-sonnet-5-5 or global.anthropic.claude-sonnet-4-6
  base_url: ""                 # required for openai-compatible; anthropic: https://api.anthropic.com
                               # bedrock-mantle: https://bedrock-mantle.<region>.api.aws/anthropic or /v1
  api_key_env: ""              # anthropic: ANTHROPIC_API_KEY; openai-compatible and bedrock*: no key
  api: ""                      # bedrock-mantle: messages or chat-completions; empty picks by model ID
  aws_profile: ""              # bedrock*; empty means the AWS default chain
  max_input_tokens: 30000
  timeout: 120s                # at most 2m
```

## Rules
- Unknown keys → exit 2 with file:line (validated against the schema before unmarshal).
- `relgap config init [path]` writes a template with every key commented out at its default, so it changes
  nothing until edited. It writes `.relgap.yaml` in the working directory, the path, or with `--user` the
  user file, and refuses to replace a file (exit 2) without `--force`. A wizard (huh, with a TTY) is post-MVP.
- `tools.<name>` takes only `repo`. It wins over every registry layer for that field; versions, sources, EOL and OSV come from the lower layers or the defaults.
- An `aliases` key is a raw pin name, matched ignoring case. It wins over every other way of naming a pin, and over a prefix in it, so `"git::git@host:ws/repo.git//.": mod` maps a git source on a host relgap doesn't read.
- `ignore` rules drop pins in `drift`, `outdated`, `digest --from-mise|--all` and `digest --base|--pull-request`. A rule sets at least one of `tool`, `name` and `path` and drops the pins that match every field it sets ([ADR-0017](../adr/0017-chart-versions-extract-version-and-floors.md)). `tool` may be an alias. `name` is a glob on the raw name, ignoring case, whose `*` spans slashes, so it matches unresolved pins. `path` is a glob on `repo/file` or the repo-relative file, with `**` for any directories. Ignored pins leave the "no known tool" count. `pins` lists every pin.
- Env vars: `RELGAP_GITHUB_CONCURRENCY`, `RELGAP_CACHE_DIR`, `RELGAP_LLM_PROVIDER`, and so on. Tokens are **never** read from config files, only from env, gh or the keyring.

## Acceptance criteria
- [x] `config show --sources` prints a key, its value and its source for every key (golden).
- [x] Unknown key and wrong type each produce a precise error (testscript).
- [x] Schema and struct stay in sync (generation test).
- [x] The `config init` template validates, sets nothing as written, and lists every key at its default
  once uncommented (`TestSpec_config_4`). `testdata/script/config_init.txtar` covers the command.
- [x] The project-file search stops at the git root or `config.Options.StopDir` and reads only the
  working directory outside both (`TestSpec_config_5`).
- [x] An ignore rule with only `name` or only `path` drops the pins it matches, unresolved ones
  included, and a rule with none of the three fields is a schema error (`TestIgnoredByName`,
  `TestSpec_config_3`).
