# relgap: open questions

Resolve each before the milestone listed. Record the answer as an ADR or a spec edit, then delete it here.

| # | Question | Default if unanswered | Needed by |
|---|---|---|---|
| ~~Q1/Q2~~ | **Resolved 2026-11-09:** name `relgap`; GitHub owner `sassdavid` (personal). An org can come later, since GitHub redirects transferred repos | — | — |
| Q3 | GitHub Enterprise / GitLab / Codeberg sources in v0.x? | GHES config field only; GitLab later via the adapter interface | M2 |
| Q4 | Include `npm:`/`pypi:` tools' changelogs from the package registries (e.g. PyPI `project_urls.Changelog`)? | yes, as a resolution source only | M8 |
| Q5 | Windows support level: full, or best-effort? | best-effort, but CI runs on windows | M0 |
| Q7 | Telemetry-free usage signals: rely on GitHub stars/issues only? | yes | — |
| Q8 | Ship an embedded aqua snapshot (≈2,353 entries, size impact) or fetch on first use? | embed a trimmed index (name → repo/prefix/filter), about 300 KB | M1 |
