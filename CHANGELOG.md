# Changelog

## [0.13.0](https://github.com/sassdavid/relgap/compare/v0.12.1...v0.13.0) (2026-11-23)


### Features

* add relgap audit for the vulnerabilities of every pin ([#111](https://github.com/sassdavid/relgap/issues/111)) ([9ad5f63](https://github.com/sassdavid/relgap/commit/9ad5f637adc2b693a9bbc9eb2a5cdf2e07912c41))
* add the MCP audit tool and notify kind audit ([#119](https://github.com/sassdavid/relgap/issues/119)) ([e3a1710](https://github.com/sassdavid/relgap/commit/e3a17100c59fbed120bab5dba91558a55fb1dea6))
* annotate advisories with CISA KEV and EPSS ([#117](https://github.com/sassdavid/relgap/issues/117)) ([284793f](https://github.com/sassdavid/relgap/commit/284793f8a34447a912c771361ff0b9aaeb0dfb8a))
* check action pins against the GitHub Actions ecosystem ([#115](https://github.com/sassdavid/relgap/issues/115)) ([bb306cc](https://github.com/sassdavid/relgap/commit/bb306ccdab1fa77ee88ce3ef30af49cd7efe2633))
* check every pin against OSV by its backend ([#102](https://github.com/sassdavid/relgap/issues/102)) ([27eb875](https://github.com/sassdavid/relgap/commit/27eb8754e09937f94210ddfb166cfe020a34874d))
* list pins OSV didn't check, with a reason ([#110](https://github.com/sassdavid/relgap/issues/110)) ([c24191c](https://github.com/sassdavid/relgap/commit/c24191c4df23e5150c2c9e4640af57c74ec4cda9))
* report a fix target for each vulnerable pin ([#114](https://github.com/sassdavid/relgap/issues/114)) ([9e15bb1](https://github.com/sassdavid/relgap/commit/9e15bb116e31d8c73a8faf67ca8b57a95eaeac5f))
* suppress advisories with vulns.ignore and OpenVEX ([#116](https://github.com/sassdavid/relgap/issues/116)) ([cd70ff0](https://github.com/sassdavid/relgap/commit/cd70ff0ac25f42c9952cf93ee0c84565676edc72))


### Bug Fixes

* **ci:** let aube-lock rewrite a stale docs lockfile ([#112](https://github.com/sassdavid/relgap/issues/112)) ([9d12fc3](https://github.com/sassdavid/relgap/commit/9d12fc361305cbd2004ab614c0f0a9663d3780f3))
* **deps:** update aws-sdk-go-v2 monorepo ([#106](https://github.com/sassdavid/relgap/issues/106)) ([91e1038](https://github.com/sassdavid/relgap/commit/91e10389afd0b48f53a227d67e7e61e43a653c6b))
* **deps:** update dependency oxfmt to v0.79.0 ([#108](https://github.com/sassdavid/relgap/issues/108)) ([5b3f569](https://github.com/sassdavid/relgap/commit/5b3f5698493b54311ce4fd521036fac185256680))
* **deps:** update module github.com/klauspost/compress to v1.20.5 ([#118](https://github.com/sassdavid/relgap/issues/118)) ([04ce94e](https://github.com/sassdavid/relgap/commit/04ce94e543b4c2b8779f5d5905b957048c2de163))
* **deps:** update module golang.org/x/net to v0.63.0 ([#109](https://github.com/sassdavid/relgap/issues/109)) ([c0ee400](https://github.com/sassdavid/relgap/commit/c0ee400b1801cf6d5069441d1214fcbee7d570b8))
* keep private repositories and modules away from OSV ([#121](https://github.com/sassdavid/relgap/issues/121)) ([356702a](https://github.com/sassdavid/relgap/commit/356702a73ea9af3f6c719699e2f093a425f17f42))
* rate GO advisories from their GHSA alias and count unrated ones ([#113](https://github.com/sassdavid/relgap/issues/113)) ([cec74d6](https://github.com/sassdavid/relgap/commit/cec74d6a19487c4cb9b8113df7ad475ce9a83379))

## [0.12.1](https://github.com/sassdavid/relgap/compare/v0.12.0...v0.12.1) (2026-11-20)


### Bug Fixes

* sort by shown names, list failures in digest markdown, plural ([#99](https://github.com/sassdavid/relgap/issues/99)) ([453f7b6](https://github.com/sassdavid/relgap/commit/453f7b6a4fae419d37dee378fa131be432c20d62))

## [0.12.0](https://github.com/sassdavid/relgap/compare/v0.11.0...v0.12.0) (2026-11-20)


### Features

* add digest ids, read charts from OCI registries and shorten chart names ([#97](https://github.com/sassdavid/relgap/issues/97)) ([5e4c6d6](https://github.com/sassdavid/relgap/commit/5e4c6d6c0234ca08e8e1ade1ae13e53cedd38a75))


### Bug Fixes

* read forge SSH hosts, match aliases with colons, show digest errors ([#96](https://github.com/sassdavid/relgap/issues/96)) ([b4a4231](https://github.com/sassdavid/relgap/commit/b4a4231d5378a18bcb2451420d6ee66cd753cc70))

## [0.11.0](https://github.com/sassdavid/relgap/compare/v0.10.0...v0.11.0) (2026-11-20)


### Features

* read chart versions without per-chart tags and Renovate extractVersion ([#92](https://github.com/sassdavid/relgap/issues/92)) ([772a876](https://github.com/sassdavid/relgap/commit/772a876eddf21102bb2a84e9a54d5bef81ddaa11))
* read versions and notes from Bitbucket, GitLab and image registries ([#95](https://github.com/sassdavid/relgap/issues/95)) ([9323e4a](https://github.com/sassdavid/relgap/commit/9323e4a0dcee30bc30a144c07009fa72aafd5588))


### Bug Fixes

* key caches by tool identity and keep chart registry URLs ([#91](https://github.com/sassdavid/relgap/issues/91)) ([6e5413c](https://github.com/sassdavid/relgap/commit/6e5413c52b5b09704367315535ff3e401ed360db))

## [0.10.0](https://github.com/sassdavid/relgap/compare/v0.9.1...v0.10.0) (2026-11-19)


### Features

* read Renovate comments, Terraform modules and Hugo links ([#90](https://github.com/sassdavid/relgap/issues/90)) ([c7ed92c](https://github.com/sassdavid/relgap/commit/c7ed92c854df13172a2ba9af2defefa6ee8dd087))


### Bug Fixes

* infer chart tag prefixes from tags and close digest gaps ([#87](https://github.com/sassdavid/relgap/issues/87)) ([d390ee0](https://github.com/sassdavid/relgap/commit/d390ee0bdadb5f8071d2e93b195b942400888a67))

## [0.9.1](https://github.com/sassdavid/relgap/compare/v0.9.0...v0.9.1) (2026-11-18)


### Bug Fixes

* **registry:** pin upgrade guides to the release tag ([#83](https://github.com/sassdavid/relgap/issues/83)) ([df826d1](https://github.com/sassdavid/relgap/commit/df826d1a0881eebbd35903109bae767e06bb3f28))

## [0.9.0](https://github.com/sassdavid/relgap/compare/v0.8.0...v0.9.0) (2026-11-17)


### Features

* **docs:** add social cards, icons and the accessibility checks (M26) ([#80](https://github.com/sassdavid/relgap/issues/80)) ([6a511bf](https://github.com/sassdavid/relgap/commit/6a511bf59cce100fdbc02e9b85a841ae203c7dfe))

## [0.8.0](https://github.com/sassdavid/relgap/compare/v0.7.0...v0.8.0) (2026-11-17)


### Features

* **docs:** add the guides, concepts and help pages (M25) ([#74](https://github.com/sassdavid/relgap/issues/74)) ([b1e6a8e](https://github.com/sassdavid/relgap/commit/b1e6a8e967aaf45364f8075b9f487e277334cfd1))


### Bug Fixes

* **docs:** check comma lists in snippets and add SECURITY.md ([#76](https://github.com/sassdavid/relgap/issues/76)) ([d1ea325](https://github.com/sassdavid/relgap/commit/d1ea32513c92cb4b03ad117010ba3afe21a3dfb1))
* **docs:** take related link titles from the frontmatter title ([#73](https://github.com/sassdavid/relgap/issues/73)) ([7c94f11](https://github.com/sassdavid/relgap/commit/7c94f11452d8e9842eef7fd13c3befc875a27fed))

## [0.7.0](https://github.com/sassdavid/relgap/compare/v0.6.0...v0.7.0) (2026-11-17)


### Features

* **docs:** add getting started and the generated tool reference (M24) ([#71](https://github.com/sassdavid/relgap/issues/71)) ([6e9b7e6](https://github.com/sassdavid/relgap/commit/6e9b7e69d026e74ea84192d7530e684838cbef6b))


### Bug Fixes

* **httpx:** redact X-Api-Key in trace logs ([#70](https://github.com/sassdavid/relgap/issues/70)) ([9edb12c](https://github.com/sassdavid/relgap/commit/9edb12cd16a1f3ab38c38be57d87e1e166991878))

## [0.6.0](https://github.com/sassdavid/relgap/compare/v0.5.0...v0.6.0) (2026-11-17)


### Features

* **docs:** generate the CLI and reference pages from code ([#69](https://github.com/sassdavid/relgap/issues/69)) ([8cae757](https://github.com/sassdavid/relgap/commit/8cae7576dc6cbbfa77eda6d53ac45d3b49f1f03a))


### Bug Fixes

* **deps:** update module github.com/carapace-sh/carapace to v1.18.2 ([#65](https://github.com/sassdavid/relgap/issues/65)) ([97bf39b](https://github.com/sassdavid/relgap/commit/97bf39b91fdeeb6d77f616afc6a9e23b2713bfd1))

## [0.5.0](https://github.com/sassdavid/relgap/compare/v0.4.0...v0.5.0) (2026-11-17)


### Features

* **docs:** build the docs site foundation (M22) ([#61](https://github.com/sassdavid/relgap/issues/61)) ([65b9a37](https://github.com/sassdavid/relgap/commit/65b9a374151c15a1c6322a7dff52ba52369c9e1c))


### Bug Fixes

* **render:** stop digest Markdown from mentioning users ([#63](https://github.com/sassdavid/relgap/issues/63)) ([8cdfbad](https://github.com/sassdavid/relgap/commit/8cdfbade96af68f444be8991c89a085cb39ca815))

## [0.4.0](https://github.com/sassdavid/relgap/compare/v0.3.0...v0.4.0) (2026-11-16)


### Features

* **cli:** add config init and close the other M16 gaps ([#51](https://github.com/sassdavid/relgap/issues/51)) ([42238f0](https://github.com/sassdavid/relgap/commit/42238f02cd99e0d29aeb5c2afeed84199893f16e))
* **digest:** add --summarize with cited, advisory LLM summaries ([#47](https://github.com/sassdavid/relgap/issues/47)) ([2d1f503](https://github.com/sassdavid/relgap/commit/2d1f503a671d2b774d036fab7aef6b0518ec024a))
* **drift:** read a tfcensus inventory with --from-tfcensus ([#57](https://github.com/sassdavid/relgap/issues/57)) ([34abdd7](https://github.com/sassdavid/relgap/commit/34abdd7e9f6aff21b9b937c00462641847de6e68))
* **mcp:** add the upgrade_review prompt and stable entry IDs ([#46](https://github.com/sassdavid/relgap/issues/46)) ([f4cdb45](https://github.com/sassdavid/relgap/commit/f4cdb45c800d05bd65cfb7b617d22c04139bf8ef))
* **mcp:** trim page versions and merge upgrade-guide sections ([#55](https://github.com/sassdavid/relgap/issues/55)) ([ff09f0c](https://github.com/sassdavid/relgap/commit/ff09f0c9fae8e8de1f411e582b314397fa3accc6))
* **registry:** resolve tfprovider, chart, npm and pypi names, add registry update and --guess ([#52](https://github.com/sassdavid/relgap/issues/52)) ([6ccea3d](https://github.com/sassdavid/relgap/commit/6ccea3d46035fa784f7965e25a784b1687f823a6))
* **summarize:** add the bedrock-mantle provider and opt-in Bedrock API keys ([#50](https://github.com/sassdavid/relgap/issues/50)) ([79db409](https://github.com/sassdavid/relgap/commit/79db409ef497f48ba61cb751f0ef1bde1bf1a5fe))


### Bug Fixes

* **deps:** update module gopkg.in/dnaeon/go-vcr.v4 to v4.0.8 ([#40](https://github.com/sassdavid/relgap/issues/40)) ([af55531](https://github.com/sassdavid/relgap/commit/af555316afc67baf9c520d1388d1466022d23d44))


### Performance Improvements

* **outdated:** parallel latest lookup, and record the M18 performance targets ([#54](https://github.com/sassdavid/relgap/issues/54)) ([b525e24](https://github.com/sassdavid/relgap/commit/b525e24383966e50ccd0d2ec7b3221760a0c8b51))

## [0.3.0](https://github.com/sassdavid/relgap/compare/v0.2.0...v0.3.0) (2026-11-13)


### Features

* add digest --base and --pull-request for upgraded pins ([#33](https://github.com/sassdavid/relgap/issues/33)) ([8ef226c](https://github.com/sassdavid/relgap/commit/8ef226ccb13707dd0f62a42484ba26de8ae9c961))

## [0.2.0](https://github.com/sassdavid/relgap/compare/v0.1.0...v0.2.0) (2026-11-13)


### Features

* add --out, Markdown truncation and the GitHub PR comment (M13) ([#30](https://github.com/sassdavid/relgap/issues/30)) ([599ebe2](https://github.com/sassdavid/relgap/commit/599ebe28ac8d2b8392a263a0f5c625f7a8973255))
* add shell completion, carapace shells and man pages (M12) ([#27](https://github.com/sassdavid/relgap/issues/27)) ([ae8306d](https://github.com/sassdavid/relgap/commit/ae8306ded8e1091aad682c48dd82ceefe953a1ea))

## 0.1.0 (2026-11-13)


### Features

* add changelog-file, section index and full classifier (M3) ([#10](https://github.com/sassdavid/relgap/issues/10)) ([31a739b](https://github.com/sassdavid/relgap/commit/31a739b98a0f5c58b8bb05f5c7bfa42a997877c6))
* add enrichment, web sources and the curated registry (M8) ([#16](https://github.com/sassdavid/relgap/issues/16)) ([b6fca0d](https://github.com/sassdavid/relgap/commit/b6fca0d4d643745e57cc301ebaea6e4da22f7915))
* add outdated and digest --from-mise (M6) ([#13](https://github.com/sassdavid/relgap/issues/13)) ([a92f66a](https://github.com/sassdavid/relgap/commit/a92f66ab727a11b1c140f2339ea66cca173aede9))
* add pin discovery and the pins command (M5) ([#12](https://github.com/sassdavid/relgap/issues/12)) ([301dbd1](https://github.com/sassdavid/relgap/commit/301dbd1b1a1254a733aadeb3018c79db120a1f4b))
* add SARIF output and the full --fail-on matrix (M9) ([#17](https://github.com/sassdavid/relgap/issues/17)) ([91d7e92](https://github.com/sassdavid/relgap/commit/91d7e92997c59b4d054339ac7dd87b3dc15d1453))
* add the drift command (M7) ([#14](https://github.com/sassdavid/relgap/issues/14)) ([a48a9aa](https://github.com/sassdavid/relgap/commit/a48a9aa368f07586749167aa72662307247af692))
* add the GitHub client and gh-release digest (M2) ([#8](https://github.com/sassdavid/relgap/issues/8)) ([5e3111b](https://github.com/sassdavid/relgap/commit/5e3111b2ad9c649f19b8b3270e3c7fa5adb3a3bb))
* add the HTTP cache, --offline and cache commands (M4) ([#11](https://github.com/sassdavid/relgap/issues/11)) ([0d7e2d3](https://github.com/sassdavid/relgap/commit/0d7e2d3b07e65de854e42fa078afb72386bcb434))
* add the relgap mcp server (M10) ([#18](https://github.com/sassdavid/relgap/issues/18)) ([a456126](https://github.com/sassdavid/relgap/commit/a456126f625887998cf37ca8e9253440c98ec6f1))
* add version schemes and tool resolution (M1) ([#5](https://github.com/sassdavid/relgap/issues/5)) ([26bf7e8](https://github.com/sassdavid/relgap/commit/26bf7e8d83336df60ae1e72c383b2f288180bc76))
* set up the CLI foundation (M0) ([#1](https://github.com/sassdavid/relgap/issues/1)) ([40b4bc7](https://github.com/sassdavid/relgap/commit/40b4bc775e1756e1d2a114fc2367e01e3c6ca5be))


### Bug Fixes

* start releases at 0.1.0 ([#3](https://github.com/sassdavid/relgap/issues/3)) ([efe354f](https://github.com/sassdavid/relgap/commit/efe354f3a824ffc54026a02964e2b3e5fcb71c1b))
