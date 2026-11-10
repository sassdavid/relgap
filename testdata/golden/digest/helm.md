## helm 3.21.0..3.22.1

Repository: [helm/helm](https://github.com/helm/helm). 6 versions: [3.21.1](https://github.com/helm/helm/releases/tag/v3.21.1), [3.21.2](https://github.com/helm/helm/releases/tag/v3.21.2), [3.21.3](https://github.com/helm/helm/releases/tag/v3.21.3), [3.21.4](https://github.com/helm/helm/releases/tag/v3.21.4), [3.22.0](https://github.com/helm/helm/releases/tag/v3.22.0), [3.22.1](https://github.com/helm/helm/releases/tag/v3.22.1).

breaking 0, deprecation 0, security 0, feature 0, fix 19, other 90; 44 below --min-confidence hidden

<details><summary>3.21.1 (2026-06-12)</summary>

#### Fixes

- Fixed nil pointer panic that could happen with helm template in ClientOnly flows. Now correctly returns a template error https://github.com/helm/helm/pull/31920 ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- fix(action): avoid nil REST client getter panic when installing CRDs c56dd0095fd76da5d7b30ecdf506103e7f26745e (sergiochan) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- fix(registry): keep credentials on plain-HTTP fallback with oras-go v2.6.1 702529f90a0021e4d9df4880d6589198ec0e05f7 (Terry Howe) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- fix(deps): bump golang.org/x/net to v0.55.0 to address GO-2026-5026 bad6cd478f5b3f3c96b795f4d6a010f04a89624f (Terry Howe) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))

#### Other

- Bumped golang.org/x/net to v0.55.0 to address GO-2026-5026 #32152 ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- Bumped Go from 1.25 to 1.26 #32168 ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- Dependency version updates ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump oras.land/oras-go/v2 from 2.6.0 to 2.6.1 178e120e16f5d61f769ee2c56a0d2a45ab7303bd (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump golang.org/x/crypto from 0.52.0 to 0.53.0 dcf35f86551322d93c1cb695f08435b3287e5ad0 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump golang.org/x/term from 0.43.0 to 0.44.0 44aff8bf51809ec9f8a906d050818776cd47b264 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump golang.org/x/text from 0.37.0 to 0.38.0 ae2f31f5a7d0cd789ca9fd83a6a2fe5fc7c3a1a3 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- ci: bump golangci-lint to v2.11.3 for go 1.26 00eac21a89cd977868c0304917acd90220e07895 (Terry Howe) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore: bump go to 1.26 bec346ac983fd0b860a13ef7750616e088143f01 (Terry Howe) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump github.com/lib/pq from 1.11.2 to 1.12.3 58b6ccffd3e3ade58543c50e76e6fb6c39393be5 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump github.com/distribution/distribution/v3 30b9f51d80b5853ef4031af168dd02491bbef9ab (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump github.com/containerd/containerd from 1.7.30 to 1.7.32 858aa47b455f08516a6d977f93d609c24f985853 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump github.com/Masterminds/semver/v3 from 3.4.0 to 3.5.0 1af25d46939584205f0e3810fc547fcda727b1ec (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump github.com/mattn/go-shellwords from 1.0.12 to 1.0.13 97e4bc30e57290d81b36972ce49f908d68fd9241 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump golang.org/x/crypto from 0.51.0 to 0.52.0 29bdd1bedd28bf816c7e26d356126489edf22945 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump k8s.io/klog/v2 from 2.130.1 to 2.140.0 75c2e9bf370eabf35a00e60cfceaee2b969ebe00 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))
- chore(deps): bump golang.org/x/text from 0.35.0 to 0.37.0 cd4dac334caf38105aaae00ee8d4a9cb03de0b2c (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.1))

</details>

<details><summary>3.21.2 (2026-06-20)</summary>

#### Other

- Update Kubernetes client libraries to v1.36 ([source](https://github.com/helm/helm/releases/tag/v3.21.2))
- chore(deps): bump the k8s-io group with 2 updates 125963406833fe0525be91f46c8b5b0f22fb9e32 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.2))
- chore(deps): bump the k8s-io group across 1 directory with 2 updates 3342dbfec8f39776a9accd50fa91a52d68673af1 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.2))

</details>

<details><summary>3.21.3 (2026-07-09)</summary>

#### Fixes

- fix: drop containerd v1 dep to resolve govulncheck CVEs 037733e7d51b08e30a0233bd546c345ab3ea3bba (Benoit Tigeot) ([source](https://github.com/helm/helm/releases/tag/v3.21.3))

#### Other

- chore(deps): bump github.com/containerd/containerd from 1.7.32 to 1.7.33 d3e178ba06a8a1eeacaab1df9162b658b1e07fe9 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.3))

</details>

<details><summary>3.21.4 (2026-08-14)</summary>

#### Fixes

- fix(engine): prevent Files.Lines panic on empty file (backport to v3)- #32303 by @mahesh-sadupalli ([source](https://github.com/helm/helm/releases/tag/v3.21.4))
- fix(provenance): migrate to ProtonMail/go-crypto to resolve GO-2026-5932- #32463 by @karan-vk ([source](https://github.com/helm/helm/releases/tag/v3.21.4))
- [dev-v3 backport] fix: bump go.opentelemetry.io/otel@v1.44.0 for GO-2026-5158- #32535 by @scottrigby ([source](https://github.com/helm/helm/releases/tag/v3.21.4))
- fix: bump go.opentelemetry.io/otel@v1.44.0 for GO-2026-5158 57ce7aeec1eb82422c5c349dfadda20ea3f743fe (Scott Rigby) ([source](https://github.com/helm/helm/releases/tag/v3.21.4))
- fix(provenance): migrate to ProtonMail/go-crypto to resolve GO-2026-5932 ab71449c76f66e55064ae4503a6e7adae21dcfa9 (Karan V) ([source](https://github.com/helm/helm/releases/tag/v3.21.4))
- fix(engine): prevent Files.Lines panic on empty file 955dfab1e6bdd70b5ccd7b932b2d500e9e26566c (Mahesh Sadupalli) ([source](https://github.com/helm/helm/releases/tag/v3.21.4))

#### Other

- [dev-v3 backport] deps: bump google.golang.org/grpc@v1.82.1 for GO-2026-6061- #32536 by @scottrigby ([source](https://github.com/helm/helm/releases/tag/v3.21.4))
- chore(deps): bump golang.org/x/crypto from 0.53.0 to 0.54.0- (includes golang.org/x/text v0.40.0 to fix GO-2026-5970) #32308 ([source](https://github.com/helm/helm/releases/tag/v3.21.4))
- chore(deps): bump golang.org/x/crypto from 0.53.0 to 0.54.0 (#32308) 813176c51bb5c181dbbd7901298ddcc104cd3417 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.21.4))
- [dev-v3 backport] deps: bump google.golang.org/grpc@v1.82.1 for GO-2026-6061 b6aa8b1d71140347f75ef0b77fac91f28ffe10b3 (Scott Rigby) ([source](https://github.com/helm/helm/releases/tag/v3.21.4))

</details>

<details><summary>3.22.0 (2026-09-10)</summary>

#### Fixes

- fix: set [pull,push] scope when helm push to a registry(use token auth) (backport) (#32362) 9dbcb9f1edb4cdf2b055870dc9a07e0c0f4b05d1 (kimsungmin1) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- fix: bump go.opentelemetry.io/otel@v1.44.0 for GO-2026-5158 (#32535) adab0ef15d40085b4ddad4334418dc1bf41c85c7 (Scott Rigby) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- fix(provenance): migrate to ProtonMail/go-crypto to resolve GO-2026-5932 bd392c6a60d09a6c9f467dc52dbfd249a641f80d (Karan V) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- fix(engine): prevent Files.Lines panic on empty file c02a9aed3452c0c7ee9d79ded44dd170ce5001ca (Mahesh Sadupalli) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- fix: drop containerd v1 dep to resolve govulncheck CVEs a061886b724181526f5283789e78ec4369e25ff3 (Benoit Tigeot) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- fix(registry): keep credentials on plain-HTTP fallback with oras-go v2.6.1 2fb05f8a35f6c0d5760c03c12d60c46adf2b3d12 (Terry Howe) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- fix(deps): bump golang.org/x/net to v0.55.0 to address GO-2026-5026 1dde6c7551746a74a6f69db06eaf4fc58a90042b (Terry Howe) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- fix(action): avoid nil REST client getter panic when installing CRDs 711a4eaf40a911b0864a01b99f59f62817443003 (sergiochan) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))

#### Other

- primarily dependency updates and k8s-io group to 0.37.0 ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the k8s-io group across 1 directory with 6 updates 144ca65f8501953fa8b41cd1d37c7223051c85b7 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the github-actions group across 1 directory with 4 updates (#32575) 665ab55b05ce3d8c5a9d40e22b14f920d84fa68a (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the k8s-io group with 7 updates (#32573) 0841093658d96f4787cc9599cd2a891a3c590971 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/stretchr/testify from 1.12.0 to 1.12.1 (#32563) d0569c7d625151ee8c7b858d87218dd06087d613 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the github-actions group across 1 directory with 4 updates (#32557) bcbdb1e8ae5f1b1dd26a41652bc2f3e1e8f0f920 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/stretchr/testify from 1.11.1 to 1.12.0 (#32554) 6cdcc8f91de2dbe181b19be8f5de0701b9c30acd (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang.org/x/crypto from 0.54.0 to 0.55.0 (#32542) 158719fb4346eb65ee789abc09a922a84bf6ec3b (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- [dev-v3 backport] deps: bump google.golang.org/grpc@v1.82.1 for GO-2026-6061 (#32536) a442b8cf64f5942ca9e920b46fceb15e0878ca80 (Scott Rigby) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the github-actions group with 4 updates (#32524) c068b543256b64515c019644381cf57276a6fec6 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the github-actions group with 4 updates 74271c4258a35186cb621330f5026461d01e9032 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/santhosh-tekuri/jsonschema/v6 1c50d2f8bde0738c7f6df584d77fb469ec52848d (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the github-actions group with 4 updates (#32509) 88215407a8901557b2318842d56e8e25aba9d84b (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump actions/stale in the github-actions group (#32488) 05a27980df8f4b22454334dbb124494510de4af6 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump ossf/scorecard-action in the github-actions group (#32461) c27e7577e3e2793b80550315dace983ec22a0664 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the k8s-io group with 7 updates 66a6f5601d26bb640e0de5f877268a48de2148dd (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the github-actions group with 4 updates 7cd01254d33ba41ec5020fd1af780cd26a6ca02a (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github/codeql-action/upload-sarif (#32449) 79330fceb94f73508d136f389802b41face2dc41 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump actions/labeler from 6.2.0 to 7.0.0 (#32446) b29c7309f12ff72d631425b9398aceba979a81f0 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github/codeql-action/analyze from 4.37.1 to 4.37.2 973bb346b83480f4daff9eb5e8f9e0180fc48b5d (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github/codeql-action/autobuild from 4.37.1 to 4.37.2 e25efcf2837674c3b6ab1449c8b69f3fb6ee4bc4 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github/codeql-action/init from 4.37.1 to 4.37.2 98e547f8ed82720aca8b6287e1e76aad1ba55f9b (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump actions/checkout from 7.0.0 to 7.0.1 (#32412) 7f4b72a711e1d6d3c40f904affd99991e84456c3 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github/codeql-action/autobuild from 4.37.0 to 4.37.1 (#32381) 2cf1437c200dcd8586788d637340e7af3ffcc9d0 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump actions/setup-go from 6.5.0 to 7.0.0 (#32383) 270f73b2df1887425cbe5eccceb7cafcfda81a89 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github/codeql-action/upload-sarif (#32382) 9e3a0100a994e4cf08372398f31cecab92ec3b2e (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- ci: auto-label PRs targeting dev-v3 (#32340) c9ed0caebae6b24b8aa64c5f587d6c11297c7dd9 (Benoit Tigeot) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump oras.land/oras-go/v2 from 2.6.1 to 2.6.2 (#32331) b842ddfccba6a388df1dd2f959d16db3ce105e16 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/mattn/go-shellwords from 1.0.13 to 1.0.14 (#32332) f6047c96c7a83f8004752f0be201e03181d3ef59 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github/codeql-action/analyze from 3.26.6 to 4.37.0 (#32357) f2f6bf6e99c8d1331c7894304eb9bacc8fb7a401 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump actions/checkout from 4.2.2 to 7.0.0 (#32368) 69f70a86748edae2d32b6106bddacba9a1c6e359 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump actions/stale from 9.0.0 to 10.4.0 (#32369) c95222ab5af136ce9e5f3624212b31f5a496c0ec (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump actions/setup-go from 5.1.0 to 6.5.0 (#32358) fc71f51069104729717d718c324c07420059b89c (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github/codeql-action/upload-sarif (#32360) 6596318b34e93427ebdde2cfce83b94e81c229c6 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github/codeql-action/init from 3.26.6 to 4.37.0 (#32359) 5d9943ca09fce64c9e98c68aedbc5ddb43eae793 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang/govulncheck-action from 1.0.4 to 1.1.0 (#32356) a3d7a6c50a69008c97f42c456df1318135786c10 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golangci/golangci-lint-action from 6.1.1 to 9.3.0 (#32354) 8e199708cc2a3323f31274026a16839a78984a3e (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump ossf/scorecard-action from 2.4.0 to 2.4.3 (#32353) 24ef09ac19305a8c225287d983ebd26dbcb4b260 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang.org/x/text from 0.38.0 to 0.40.0 (#32310) 8082dd4a0d9f1ca3dbd2052a8b0e1bba843a85c7 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang.org/x/crypto from 0.53.0 to 0.54.0 (#32308) 2c24490679681b0825c258576e9f7699f8dc8e54 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang.org/x/term from 0.44.0 to 0.45.0 (#32306) 953d280f928c4bc76912236bc2a8a0d11a39c016 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/containerd/containerd from 1.7.32 to 1.7.33 f9798360983d9d964e2088427c693317a69f326e (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/cyphar/filepath-securejoin c4446659f68e94db06f0e142376f8cf1b970a878 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the k8s-io group with 2 updates 1bf9b5fbba4df88a562a35eef40e6c60f12b5a90 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump the k8s-io group across 1 directory with 2 updates 119a7d9babffcec9e51d285153e09d8c4f54a69d (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump oras.land/oras-go/v2 from 2.6.0 to 2.6.1 acc867b88e68e5f9c074c1c8c584a57b3ae6c127 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang.org/x/crypto from 0.52.0 to 0.53.0 b920935feb6b6c8729cdf6b98f4c8efc824779c5 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang.org/x/term from 0.43.0 to 0.44.0 5b5ef192c773f6448e4dcd845e643c573aff906b (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang.org/x/text from 0.37.0 to 0.38.0 af67af8951d5f9a7631efc3a97b6dc6b58247148 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- ci: bump golangci-lint to v2.11.3 for go 1.26 0ed92f0696329495d87f8acdc42687c30b31d431 (Terry Howe) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore: bump go to 1.26 c678c0c9c088fc2f9ddbad8c88416c77f4e048cc (Terry Howe) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/lib/pq from 1.11.2 to 1.12.3 12c4d45b146a8be1ca70bd15e3cad00e7d12733a (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/distribution/distribution/v3 a0f97111f5607a5724a8af110480384827a81b41 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/containerd/containerd from 1.7.30 to 1.7.32 995c31e20bf3ab9a4feb85cd4a54951d9418ea51 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/Masterminds/semver/v3 from 3.4.0 to 3.5.0 8c171688281d3c1f14d908cf682c742585848db0 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump github.com/mattn/go-shellwords from 1.0.12 to 1.0.13 fe141bd537ea42e64bec3b3b181dcf955260e939 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang.org/x/crypto from 0.51.0 to 0.52.0 980836f726fdfbd6a2f0662a0aa48fe35da3fd64 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump k8s.io/klog/v2 from 2.130.1 to 2.140.0 0162feda1d4d6dd0bd1f227ecdfe9f489e6d7284 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))
- chore(deps): bump golang.org/x/text from 0.35.0 to 0.37.0 a664a58f43c742987376bca589891fda3bf55668 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.0))

</details>

<details><summary>3.22.1 (2026-10-15)</summary>

#### Other

- Bumps dependencies to remove Go package vulnerabilities ([source](https://github.com/helm/helm/releases/tag/v3.22.1))
- Updates the k8s.io Go packages ([source](https://github.com/helm/helm/releases/tag/v3.22.1))
- chore(deps): bump github.com/distribution/distribution/v3 (#32697) d6c1a0e4027caa6abc7892e7ea9f0cda7447baa3 (dependabot[bot]) ([source](https://github.com/helm/helm/releases/tag/v3.22.1))
- [v3] chore(deps): bump k8s.io group to v0.37.1 (#32761) 2c7472f482d9fe255e5cd51feb51eaab18df198f (Scott Rigby) ([source](https://github.com/helm/helm/releases/tag/v3.22.1))
- [v3] chore(deps): bump go.opentelemetry.io/otel/exporters/otlp/otlptrace otlptracegrpc and otlptracehttp to v1.45.0 (#32760) 8d91c8f55d86b62091d25f9f688bc26d51a98dbb (Scott Rigby) ([source](https://github.com/helm/helm/releases/tag/v3.22.1))
- chore(deps): bump golang.org/x/net to v0.60.0 for GO-2026-6617 GO-2026-6612 GO-2026-6611 GO-2026-6610 GO-2026-6603 (#32759) 13edc4f2fd73578fc517f7050e3ca4ef6af7e289 (Scott Rigby) ([source](https://github.com/helm/helm/releases/tag/v3.22.1))

</details>
