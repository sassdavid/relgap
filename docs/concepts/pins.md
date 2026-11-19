---
description: What relgap counts as a pin, how it finds repositories and files, which detector reads which file, and how config narrows the scan.
---

# Pins and detectors

A pin is one tool version written in a file of your repository: `terraform 1.15.8` in
`mise.toml`, `FROM golang:1.27.3` in a Dockerfile, `~> 6.0` for the AWS provider in `main.tf`.
[`relgap pins`](/cli/pins) lists them, and [`relgap outdated`](/cli/outdated),
[`relgap drift`](/cli/drift) and `relgap digest --all` start from the same scan. The scan reads
files only and makes no network call.

## What a pin holds

Each pin has the name as written (`TERRAFORM_VERSION`, `hashicorp/aws`), the value as written, the
file and line, the detector that found it, and the tool name relgap maps it to. A pin has one of
six kinds:

| Kind | Tool name | Example |
|---|---|---|
| tool | the registry name | `terraform`, `node` |
| image | `image:` and the image | `image:golang` from a Dockerfile `FROM` |
| action | `action:` and the action | `action:actions/checkout`, with `scan.detectors.ci_actions` |
| tfprovider | `tfprovider:` and the provider | `tfprovider:hashicorp/aws` |
| tfmodule | `tfmodule:` and the registry module | `tfmodule:terraform-aws-modules/vpc/aws` from a `module` block |
| chart | `chart:` and the chart | `chart:argo-cd` from `Chart.yaml` |

A chart pin also keeps the Helm repository URL it names (`registry_url` in JSON), from a
`Chart.yaml` dependency's `repository` or a Renovate comment's `registryUrl`. `relgap outdated`
and `relgap digest --base` use it to find the chart's Artifact Hub repository. A module whose
source is a git repository on github.com with `?ref=` is a tool pin of `github:owner/repo`, and
its ref counts as a version only when it is `1.2.3` or `v1.2.3`.

The value decides which versions the pin allows. An exact pin such as `1.15.8` allows one
version. relgap marks the other three forms with ≈:

- a constraint (`~> 1.15`, `>= 1.5, < 2`, `1.x`) allows a range;
- a prefix (`26`, `3.14`, `prefix:1.2`) allows every version that starts with it, as mise reads it;
- a channel (`latest`, `lts`, `ref:main`) names no version at all.

Terraform reads a bare `6.0` as exactly 6.0.0, so in `.tf` files a partial version is an exact
pin. [Versions, ranges and release lines](/concepts/ranges) says how each form becomes one
version when a command needs one.

## Finding repositories and files

Each root you pass (default: `scan.roots`, then the current directory) is searched for
directories that contain `.git`. Each one is a repository, and the scan doesn't descend into
another repository below it. A root without any `.git` is one repository itself. `--submodules`
scans checked-out git submodules as repositories of their own.

relgap lists a repository's files with `git ls-files`, so `.gitignore` applies. Without git, it
walks the directory and skips `.git`, `.terraform`, `.terragrunt-cache`, `node_modules` and
`vendor`. Files that match a `scan.exclude` glob are never read; the default excludes
`**/node_modules/**` and `**/.terragrunt-cache/**`. Files over 2 MB and binary files are
skipped.

## Detectors

A detector reads one family of files. Every file that matches a detector's names is read by it:

<!--@include: ../parts/detectors.md-->

Two detectors are off by default. `scan.detectors.ci_actions` adds the `uses:` pins of GitHub
Actions; CI files are read for tool versions either way. `scan.detectors.generic` reads the files
`scan.extra_files` names, such as `Makefile` or `**/*.env`, for `NAME_VERSION = x.y.z` lines.

## From a name to a tool

A raw name becomes a tool name through the same layers as [`relgap resolve`](/concepts/resolution),
without any network lookup. A `NAME_VERSION` variable maps to the tool `name`, with underscores
read as hyphens: `TERRAFORM_VERSION` is terraform. In a Dockerfile, a `# renovate:` comment above
an `ARG` or `ENV` names the tool for the line below, as Renovate's custom managers read it.

## Renovate comments

The `renovate-comment` detector reads the comments that Renovate's regex custom managers use, in
`.tf`, `.tfvars`, `.hcl`, YAML, Makefiles, `pom.xml`, shell scripts, `.env` files and the files
`scan.extra_files` names:

```hcl
variable "karpenter" {
  type = object({
    # renovate: datasource=helm depName=karpenter registryUrl=oci://public.ecr.aws/karpenter
    chart_version = optional(string, "1.8.1")
  })
}
```

The first line after the comment that is neither blank nor a comment must hold exactly one
version, which becomes the pin. `//` and `<!-- -->` comments work too. The datasource sets the
kind: `helm` gives a chart pin, `github-releases` and `github-tags` a `github:` pin,
`terraform-provider` and `terraform-module` a provider or module pin, `docker` an image pin, and
`aqua`, `npm` and `pypi` their package. Any other datasource looks the tool up by `packageName`,
then `depName`. When another detector found a pin on the same line with the same value, the
comment's pin replaces it. Dockerfiles keep their own detector, which reads these comments above
`ARG` and `ENV`.

## Unresolved pins

A pin whose name maps to no tool keeps an empty tool. `relgap pins --unresolved` lists them, and
`aliases` in the config maps a raw name to a tool. [Map unresolved pins](/guides/unresolved)
walks through it.

## Leaving pins out

`ignore` rules in the config drop the pins of a tool, everywhere or only under a path glob:

```yaml
ignore:
  - tool: java
  - tool: terraform
    path: "**/legacy/**"
```

The glob matches the repository and file (`infra/legacy/main.tf`) or the file alone, and `**`
stands for any number of directories. The tool may be an alias. Ignore rules apply to
`relgap outdated`, `relgap drift` and every digest that starts from a scan (`--from-mise`,
`--all`, `--base` and `--pull-request`). `relgap pins` lists every pin, ignored or not.

`relgap pins --renovate-report` adds the dependencies of a Renovate extract report to the scan;
[Use Renovate's extract report](/guides/renovate-report) has the details.
