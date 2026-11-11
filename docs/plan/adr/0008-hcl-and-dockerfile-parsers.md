# relgap ADR-0008: Read Terraform with hcl/v2 and Dockerfiles with an own parser

- Status: accepted (2026-11-11)

## Context
The pin-discovery spec named `terraform-config-inspect` for `*.tf`, and `stack.md` named
`moby/buildkit`'s Dockerfile parser, to be measured at M5. Both were checked before use:

- `terraform-config-inspect` v0.0.0-20260904064934-75d64de68c31 returns `RequiredCore` and
  `RequiredProviders` without source positions, so Terraform pins would have no line. It also
  pulls in `hashicorp/hcl` v1 (2017) and raises `agext/levenshtein` to v1.2.2, which tfcensus
  would have to mirror.
- buildkit v0.34.2's `frontend/dockerfile/parser` adds 2.9 MB to a stripped binary (4.46 MB
  against a 1.57 MB baseline) and four modules: `google.golang.org/protobuf`,
  `planetscale/vtprotobuf`, `containerd/typeurl/v2` and `pkg/errors`.
- `hashicorp/hcl/v2` v2.25.0 is needed anyway for `.terraform.lock.hcl` and terragrunt files.
  tfcensus already uses it with `zclconf/go-cty` v1.20.0.

## Decision
- The terraform detector reads `terraform { required_version, required_providers }` with
  `hcl/v2` (`hclsyntax` for `.tf`, `hcl/v2/json` for `.tf.json`). Values are evaluated without
  variables or functions; anything else is skipped.
- The dockerfile detector is a parser in `internal/scan` that handles the escape directive,
  continuation lines, comments between continuation lines, heredocs and the legacy
  `ENV NAME value` form. It reads FROM, ARG and ENV only.
- No module beyond `hcl/v2` and its dependencies is added, at the versions tfcensus pins.

## Consequences
- Every Terraform and Dockerfile pin has a line.
- Pre-0.12 HCL1 syntax in `*.tf` isn't read.
- Dockerfile features beyond FROM, ARG and ENV aren't parsed, so a new BuildKit syntax only
  matters if it changes how those three instructions or line joining work.
