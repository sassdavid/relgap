---
layout: home
title: relgap
titleTemplate: See everything between your pin and latest
description: relgap merges the release notes of every version between your pin and latest, puts breaking changes first, and adds EOL and vulnerability status.
headline: See everything between your pin and latest.
lead: relgap merges the release notes of every version in the range, puts breaking changes first, and adds EOL and vulnerability status.
install: mise use github:sassdavid/relgap
actions:
  - text: Install
    link: /install
answers:
  - question: What breaks if I upgrade terraform to latest?
    command: relgap digest
    link: https://github.com/sassdavid/relgap/blob/v0.4.0/docs/cli/relgap_digest.md
  - question: Which pins in this repo are behind or EOL?
    command: relgap outdated
    link: https://github.com/sassdavid/relgap/blob/v0.4.0/docs/cli/relgap_outdated.md
  - question: Which repos pin different versions of a tool?
    command: relgap drift
    link: https://github.com/sassdavid/relgap/blob/v0.4.0/docs/cli/relgap_drift.md
  - question: What should this Renovate PR's review say?
    command: relgap digest --pull-request
    link: https://github.com/sassdavid/relgap/blob/v0.4.0/docs/cli/relgap_digest.md
---

<HomeHero />

<AnswerRows />
