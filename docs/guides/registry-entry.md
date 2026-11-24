---
description: Write a relgap registry entry for a tool, check it with registry lint, try it live with registry test, and send it upstream.
---

# Write or fix a registry entry

A registry entry tells relgap where a tool's release notes are, how its tags become versions,
and where its EOL and vulnerability data live. relgap embeds 21 curated entries
([Supported tools](/reference/tools)). Every other tool gets the defaults: versions from its
GitHub releases and release notes by auto-discovery. Write an entry when the defaults miss the
notes, pick up the wrong tags, or when you want EOL and vulnerability columns in
[`relgap outdated`](/guides/outdated).

## The file

One file per tool, `registry/<name>.yaml` in the relgap repository, with `name` equal to the
file name. This is the entry for terraform:

```yaml
# yaml-language-server: $schema=../schemas/registry-v1.json
# The release body links to CHANGELOG.md, which lives on the v<major>.<minor> release branch.
name: terraform
aliases: [tf]
repo: hashicorp/terraform
version:
  scheme: semver
  tag_prefix: "v"
  universe: releases
sources:
  - kind: changelog-file
    path_template: CHANGELOG.md
    ref: "v{major}.{minor}"
    heading: "^{version} \\("
  - kind: gh-release
    ignore_sections: ["Previous Releases"]
eol: { product: terraform, cycle: "{major}.{minor}" }
osv:
  - { ecosystem: Go, name: github.com/hashicorp/terraform }
```

The first comment points editors that read `yaml-language-server` comments at the schema, so
they check the entry as you type. Outside the repository, use
`https://relgap.sasskovacs.dev/schemas/registry-v1.json`. `name` and `repo` are the only required
keys. [Registry entry schema](/reference/registry-schema) lists every key.

## Versions

`version` says which tags are versions and how they sort:

- `scheme` is `semver`, `loose` (the default), `calver`, `python` or `regex` with `extract`.
- `tag_prefix` is stripped before parsing, and tags without it are ignored. Use it for tags such
  as `jdk-21.0.9+10` or `go1.25.4`. A tool whose tags changed prefix takes a list, newest first,
  such as `["lychee-v", ""]`, where `""` keeps the unprefixed tags.
- `exclude` is a regular expression for tags to drop, such as release candidates.
- `universe` is `releases` (GitHub releases, the default) or `tags`, for projects that tag
  without publishing releases.
- `line` is the release line that `outdated` compares with, and that a range like `1.15.8..`
  ends on when `digest --line` isn't given.

[Versions, ranges and release lines](/concepts/ranges) explains how relgap orders versions and
selects a range.

## Sources

`sources` lists where the notes are. relgap reads every source for each version in the range and
merges them in order, and a bullet that two sources share shows once. An entry without `sources`
uses auto-discovery: the release bodies when they hold more than boilerplate, else a changelog
file in the repository's top directory.

The terraform entry reads `CHANGELOG.md` on the release branch of each version, where `heading`
finds each version's section, then the release bodies with one section dropped.
`path_template`, `url_template`, `ref` and `anchor` take placeholders such as `{version}` and
`{major}`. [Release-note sources](/concepts/sources) describes each kind, the placeholders and the
hosts a `url_template` may use.

A changelog file from a Hugo site, such as karpenter's upgrade guide, links pages with Hugo's
`ref` and `relref` shortcodes:

```md
See [Drift]({{< ref "../concepts/disruption#drift" >}}).
```

Set `site_url` to the site's base URL and those links point at its pages
(`https://karpenter.sh/docs/concepts/disruption/#drift`). relgap finds
the content directory from the path, such as `website/content/en`; set `content_dir` when that
guess is wrong. Without `site_url` the links keep their text and lose the link.

`ignore_sections` drops headings that hold no changes, such as install steps.
`breaking_markers` lists literal text that marks an entry as breaking, such as kubectl's
`ACTION REQUIRED`. [Classification and confidence](/concepts/classification) shows how the
headings and markers set each entry's kind.

## EOL and vulnerabilities

`eol.product` names the [endoflife.date](https://endoflife.date) product, and `eol.cycle` builds
the cycle name from a version (default `{major}.{minor}`). `osv` lists the packages whose
[OSV](https://osv.dev) advisories apply, each with an ecosystem and a name. A Go module with a
`/v2` or `/v3` suffix needs one line per major, as the argocd entry has.
[EOL and vulnerabilities](/concepts/enrichment) explains how relgap matches them.

## Check the entry

`relgap registry lint` checks the schema, the regular expressions, the placeholders, the
`url_template` hosts and the CSS selectors, and names each problem with file, line and column:

```sh
relgap registry lint registry/terraform.yaml
```

Without arguments it checks the entries embedded in relgap. It exits 2 when there is a problem.

## Try it live

`relgap registry test` runs the entry against GitHub before it's embedded. It digests the range
once per source and once with all of them, and prints the versions each source found notes for,
its entry counts and the documents they came from:

```sh
relgap registry test registry/terraform.yaml 1.15.8..1.17.1
```

A file path runs that entry on its own, after the lint checks. A tool name runs the entry relgap
resolves for it. A source that fails is listed with its error and the command exits 1. A source
without notes in the range isn't an error: an upgrade guide only has notes where a new release
line starts. The counts include entries below `--min-confidence`. To read the entries and the
kinds they got, run the digest with every entry shown:

```sh
relgap digest terraform 1.15.8..1.17.1 --min-confidence 0
```

## Send it upstream

Open a pull request that adds or changes the file under `registry/` in
[sassdavid/relgap](https://github.com/sassdavid/relgap). [Contributing](/contributing) has the
development setup.

Once the change is on the main branch, `relgap registry update` downloads it into the cache
without a new release. An entry whose `min_relgap` is newer than your relgap, or that fails the
lint checks, is skipped and the embedded entry stays in use. `relgap resolve` marks the updated
entries, and `relgap cache clear` removes them.
