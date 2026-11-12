# Dogfood notes

Notes from using relgap on real upgrades. Each entry says what was asked, what relgap answered
and what to change.

## MCP server, 2026-11-12 (M10)

Setup: `relgap mcp -v` built from the M10 branch and driven over stdio by a small JSON-RPC
client, with `GH_TOKEN` set and the normal HTTP cache. It wasn't registered in Claude Code
yet: Claude Code starts MCP servers when a session starts, so the session that built the server
couldn't call it. Register it for this project only with:

```sh
go build -o "$HOME/.local/bin/relgap" ./cmd/relgap
claude mcp add --transport stdio --scope local relgap -- "$HOME/.local/bin/relgap" mcp
```

### Argo CD 3.1.0 to the latest

Question: what breaks when Argo CD goes from 3.1.0 to the latest release?

- `breaking_changes` with `tool: argocd, from: 3.1.0` picked 3.6.1 as the latest and found 99
  entries (78 breaking, 21 deprecation) in 6.6 s. The same call again took 0.07 s from the
  HTTP cache.
- The answer was 85 KB, so it came in two pages: 61 breaking entries (49,734 bytes), then 17
  breaking and 21 deprecation entries with the cursor. Every breaking entry came before any
  deprecation.
- The upgrade guide gives the useful entries: 3.3.0 needs Server-Side Apply for the
  ApplicationSet CRD when Argo CD manages itself, and 3.2.0 changes the Source Hydrator's
  `syncSource.path` rules. The deprecations name the replacement, such as
  `.spec.signatureKeys` in `AppProject` giving way to `sourceIntegrity` in 3.5.0.
- Noise: the upgrade-guide adapter splits one section into many entries, so sentences such as
  "Reduces repository clutter: fewer unnecessary commits." show up as breaking at 0.95. Entries
  average 645 bytes.
- Each page repeats `versions` (72 versions with URLs, 9.9 KB), a fifth of the page.

### Terraform 1.14.0 to the latest

`breaking_changes` with `tool: terraform, from: 1.14.0` picked 1.17.2 and returned 2 breaking
entries in 5.4 s, in one page of 5.7 KB: the S3 backend's `AWS_USE_FIPS_ENDPOINT` and
`AWS_USE_DUALSTACK_ENDPOINT` accept only `true` or `false` from 1.15.0, and provisioners apply
`bastion_host_key` from 1.16.0.

### relgap's own pins

`outdated` with no path used the working directory as the root and compared 8 pins in 7.2 s:
go 1.27.0 (the go.mod directive) and goreleaser 2.20.0 are behind, the rest are current. No
errors.

### Follow-ups

- Keep only the versions that some entry cites when a digest is split, or move `versions` to the
  first page. That would fit about 15 more argocd entries per page.
- Merge upgrade-guide paragraphs under one heading into a single entry, which would cut the
  argocd breaking count and the fragments.
- Register the server in Claude Code at local scope and ask the same Argo CD question there, to
  see how the agent handles the cursor.
