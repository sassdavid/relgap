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
  breaking and 21 deprecation entries with the cursor. Breaking entries fill the pages first,
  but each page keeps the digest's version order, so the second page lists the 3.2 to 3.5
  deprecations before the 3.5.4 and 3.6.0 breaking entries (corrected 2026-11-13).
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

## Claude Code, 2026-11-13 (M10)

Setup: the relgap binary from main (8392522), built into the checkout and registered for this
project only:

```sh
mise exec -- go build -o ./relgap ./cmd/relgap
claude mcp add --transport stdio --scope local relgap -- "$PWD/relgap" mcp
claude mcp get relgap   # Scope: Local config, Status: Connected
```

The question ran headless in the relgap checkout, with only two relgap tools allowed:

```sh
claude -p "What breaks when Argo CD goes from 3.1.0 to the latest release? Use the relgap tools, \
follow the cursor until you have every page, and list the breaking changes I must act on, grouped \
by version, with a link for each." \
  --allowedTools mcp__relgap__breaking_changes,mcp__relgap__digest --output-format json
```

### Argo CD 3.1.0 to the latest, through the agent

- The agent called `breaking_changes` with `tool: argocd, from: 3.1.0`, got 61 entries
  (49,728 bytes) and a cursor, then called it again with only the cursor and got the other 38
  (35,286 bytes) with no cursor. It never called `digest`. That is all 99 entries, the same
  count as the stdio run on 2026-11-12, and the agent said so in its answer.
- The run took 4 turns, 85 s and 0.85 USD (9,212 output tokens, 138,655 cached input tokens).
- The answer grouped the entries that need action by version, from 3.2.0 to 3.6.0, each with a
  link to the upgrade guide or the release. Examples: Source Hydrator paths must be
  subdirectories (3.2.0), Server-Side Apply for installs and upgrades (3.3.0), and
  `ARGOCD_REPO_OTLP_HEADERS` renamed to `ARGOCD_REPO_SERVER_OTLP_HEADERS` (3.6.0). It ended with
  the deprecations that still work in 3.6.1.
- It dropped the upgrade-guide fragments itself ("Reduces repository clutter") and named them
  as relgap noise, the same problem the stdio run found.

### Follow-ups from the agent

- Upgrade-guide URLs point at `blob/master` with no section anchor. A tag and a heading anchor
  would keep a link valid after the next release.
- A backported security fix shows up once per release line (3.3.15, 3.4.10, 3.5.4). One entry
  that lists the three versions would be shorter.
- Inside a page, entries keep the digest's version order, so the second page mixes
  deprecations and breaking entries. Grouping by kind inside each page would read better.
- Every page repeats the 73-version list (the first follow-up above).

