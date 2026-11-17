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
    link: /cli/digest
  - question: Which pins in this repo are behind or EOL?
    command: relgap outdated
    link: /cli/outdated
  - question: Which repos pin different versions of a tool?
    command: relgap drift
    link: /cli/drift
  - question: What should this Renovate PR's review say?
    command: relgap digest --pull-request
    link: /cli/digest
---

<!-- VitePress's home layout has no <main>, and Lighthouse wants one landmark. -->
<main>

<HomeHero />

<AnswerRows />

</main>
