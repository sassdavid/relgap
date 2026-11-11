# Spec: configuration

Files: user `$XDG_CONFIG_HOME/relgap/config.yaml`, project `.relgap.yaml` (walk up to the git root).
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
tools:                         # local overrides / additions (same schema as registry entries)
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
llm:
  provider: ""                 # off
  model: ""
  base_url: ""
  api_key_env: ""
  aws_profile: ""              # bedrock
```

## Rules
- Unknown keys → exit 2 with file:line (validated against the schema before unmarshal).
- `relgap config init` writes a commented template. With a TTY and huh available (post-MVP) it becomes a wizard.
- `ignore` rules drop pins in `drift`, `outdated` and `digest --from-mise|--all`. `tool` may be an alias. `path` is a glob on `repo/file` or the repo-relative file, with `**` for any directories. `pins` lists every pin.
- Env vars: `RELGAP_GITHUB_CONCURRENCY`, `RELGAP_CACHE_DIR`, `RELGAP_LLM_PROVIDER`, and so on. Tokens are **never** read from config files, only from env, gh or the keyring.

## Acceptance criteria
- [ ] `config show --sources` prints a key, its value and its source for every key (golden).
- [ ] Unknown key and wrong type each produce a precise error (testscript).
- [ ] Schema and struct stay in sync (generation test).
