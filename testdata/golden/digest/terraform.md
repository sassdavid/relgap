## terraform 1.15.8..1.17.1

Repository: [hashicorp/terraform](https://github.com/hashicorp/terraform). 11 versions: [1.15.9](https://github.com/hashicorp/terraform/releases/tag/v1.15.9), [1.16.0](https://github.com/hashicorp/terraform/releases/tag/v1.16.0), [1.16.1](https://github.com/hashicorp/terraform/releases/tag/v1.16.1), [1.16.2](https://github.com/hashicorp/terraform/releases/tag/v1.16.2), [1.16.3](https://github.com/hashicorp/terraform/releases/tag/v1.16.3), [1.16.4](https://github.com/hashicorp/terraform/releases/tag/v1.16.4), [1.16.5](https://github.com/hashicorp/terraform/releases/tag/v1.16.5), [1.16.6](https://github.com/hashicorp/terraform/releases/tag/v1.16.6), [1.16.7](https://github.com/hashicorp/terraform/releases/tag/v1.16.7), [1.17.0](https://github.com/hashicorp/terraform/releases/tag/v1.17.0), [1.17.1](https://github.com/hashicorp/terraform/releases/tag/v1.17.1).

breaking 1, deprecation 0, security 0, feature 26, fix 41, other 0; 15 below --min-confidence hidden

### Breaking changes

- **1.16.0** `bastion_host_key` is now correctly applied by provisioners. Review your provisioner configurations to verify the configured key is correct before upgrading. ([#38318](https://github.com/hashicorp/terraform/issues/38318)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L174))

<details><summary>1.15.9 (2026-08-19)</summary>

#### Fixes

- validate: Child module validation has been fixed and will now raise errors or warning diagnostics for invalid blocks. (`list`, `import`, `backend`, and `cloud`) ([#38994](https://github.com/hashicorp/terraform/issues/38994)) ([source](https://github.com/hashicorp/terraform/blob/v1.15/CHANGELOG.md#L8))

</details>

<details><summary>1.16.0 (2026-08-26)</summary>

#### Features

- Terraform now stores planned private data for providers, allowing provider-specific state to be preserved across plan and apply. ([#37986](https://github.com/hashicorp/terraform/issues/37986)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L98))
- `terraform_data`: The new `store` block can hold ephemeral and sensitive values across plan and apply. ([#38298](https://github.com/hashicorp/terraform/issues/38298)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L100))
- Providers can now use nested blocks as computed values ([#38305](https://github.com/hashicorp/terraform/issues/38305)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L102))
- import: `import` blocks inside modules are now supported. ([#38352](https://github.com/hashicorp/terraform/issues/38352)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L104))
- Terraform is now available as a pre-built binary for Linux s390x (zLinux). ([#38384](https://github.com/hashicorp/terraform/issues/38384)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L106))
- Resource action triggers can now use `on_failure` modes of `halt`, `taint`, or `continue`. ([#38722](https://github.com/hashicorp/terraform/issues/38722)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L108))
- state show: The `state show` command can now produce machine-readable output when supplied with the `-json` flag ([#23940](https://github.com/hashicorp/terraform/issues/23940)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L113))
- workspace: The `workspace list` command can now produce machine-readable output when supplied with the `-json` flag ([#38397](https://github.com/hashicorp/terraform/issues/38397)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L115))
- test: Terraform now reports which resources were left behind when `skip_cleanup` is set. ([#38449](https://github.com/hashicorp/terraform/issues/38449)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L117))
- stacks: Action configurations now have access to a `caller` symbol containing the object value of the calling resource. ([#38668](https://github.com/hashicorp/terraform/issues/38668)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L119))
- Actions can now use `before_destroy` and `after_destroy` events. ([#38668](https://github.com/hashicorp/terraform/issues/38668)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L121))
- cloud: Terraform now displays a summary of policy evaluation outcomes for `plan` and `apply` runs against HCP Terraform. ([#38715](https://github.com/hashicorp/terraform/issues/38715)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L123))
- policy: Terraform now resolves policy plugin credentials from the configured cloud or remote backend during `init`, `plan`, and `apply`, rather than requiring the plugin to read credentials itself. ([#38716](https://github.com/hashicorp/terraform/issues/38716)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L125))
- graph: The `terraform graph` command can now output graphs in Mermaid format using the `-format=mermaid` flag. ([#38719](https://github.com/hashicorp/terraform/issues/38719)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L127))
- Child module outputs with unreferenced deprecated nested attributes no longer return deprecation warnings. ([#38778](https://github.com/hashicorp/terraform/issues/38778)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L129))
- Resource `lifecycle` blocks now support `destroy = false` to prevent a resource from being destroyed. ([#38784](https://github.com/hashicorp/terraform/issues/38784)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L131))
- The `contains()` function can now test for `null` values. ([#38792](https://github.com/hashicorp/terraform/issues/38792)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L133))
- console: The `terraform console` command now accepts an optional `-scope=<module address>` flag, which can be used to evaluate expressions within the scope of a module or a specific module instance. ([#31861](https://github.com/hashicorp/terraform/issues/31861)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L135))
- `-invoke` can now be combined with `-target` to specify the calling resource instance when multiple resources trigger the same action. ([#38845](https://github.com/hashicorp/terraform/issues/38845)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L137))
- The `terraform stacks` command now automatically infers the target hostname from the local credentials file (`credentials.tfrc.json`) when neither `TF_STACKS_HOSTNAME` nor `TF_CLOUD_HOSTNAME` is set ([#38896](https://github.com/hashicorp/terraform/issues/38896)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L139))

#### Fixes

- `import` blocks now correctly respect provider local names. ([#38338](https://github.com/hashicorp/terraform/issues/38338)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L144))
- `terraform apply` no longer panics when the plan contains a no-op change for a deposed resource that has `lifecycle.precondition` or `lifecycle.postcondition` blocks. ([#38586](https://github.com/hashicorp/terraform/issues/38586)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L146))
- workspace: Terraform now raises an error if an invalid workspace name becomes selected due to out-of-band changes. ([#38594](https://github.com/hashicorp/terraform/issues/38594)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L148))
- test: Terraform now raises a warning when a file referenced via the `-filter` flag does not exist. ([#38603](https://github.com/hashicorp/terraform/issues/38603)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L150))
- init: Terraform no longer removes locks from the dependency lock file for providers configured as `dev_override`. ([#38634](https://github.com/hashicorp/terraform/issues/38634)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L152))
- init: Terraform now warns when unmanaged providers are in use and may impact provider installation. ([#38656](https://github.com/hashicorp/terraform/issues/38656)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L154))
- Actions are now invoked with respect to all resource dependencies. ([#38668](https://github.com/hashicorp/terraform/issues/38668)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L156))
- Terraform now returns the correct error when an `import` target exists in state but has no corresponding configuration. ([#38782](https://github.com/hashicorp/terraform/issues/38782)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L158))
- The `merge()` function no longer panics when passed `null` objects. ([#38792](https://github.com/hashicorp/terraform/issues/38792)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L160))
- Allow underscores in provider source address namespaces, so private registry provider addresses are no longer rejected as invalid ([#38894](https://github.com/hashicorp/terraform/issues/38894)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L162))
- test: Optional ephemeral values do not have to be set at plan time ([#38974](https://github.com/hashicorp/terraform/issues/38974)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L164))

</details>

<details><summary>1.16.1 (2026-09-02)</summary>

#### Fixes

- cloud: Fixed a bug causing the CLI to pause indefinitely after a run task failure with pending policy evaluations ([#38751](https://github.com/hashicorp/terraform/issues/38751)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L78))
- Support referencing modules containing dynamic sources in Terraform Test ([#38950](https://github.com/hashicorp/terraform/issues/38950)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L80))
- stacks: Fixed validation to ensure the provider versions in the lock file and configuration are compatible. ([#38829](https://github.com/hashicorp/terraform/issues/38829)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L82))
- Fix panic when import identity references sensitive value ([#39013](https://github.com/hashicorp/terraform/issues/39013)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L84))
- import: Fixed a bug where import blocks would be ignored when multiple imports targeted different instances of a resource config using `for_each` or `count`. ([#39068](https://github.com/hashicorp/terraform/issues/39068)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L86))
- state show: Fix a panic when given an attribute path instead of a resource instance address ([#39087](https://github.com/hashicorp/terraform/issues/39087)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L88))
- Fix create_before_destroy ordering in some combinations of changes ([#39091](https://github.com/hashicorp/terraform/issues/39091)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L90))

</details>

<details><summary>1.16.2 (2026-09-09)</summary>

#### Fixes

- Fix panic in module installation when encoutering invalid module calls ([#39129](https://github.com/hashicorp/terraform/issues/39129)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L70))

</details>

<details><summary>1.16.3 (2026-09-16)</summary>

#### Fixes

- Fix handling of destroy=false around create_before_destroy instances ([#39169](https://github.com/hashicorp/terraform/issues/39169)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L56))
- Fix function result comparison when there are multiple marks ([#39170](https://github.com/hashicorp/terraform/issues/39170)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L58))
- Filter logic for marks could cause values with multiple marks to erroneously fail validations ([#39171](https://github.com/hashicorp/terraform/issues/39171)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L60))
- Fix issue with import provider resolution ([#39185](https://github.com/hashicorp/terraform/issues/39185)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L62))

</details>

<details><summary>1.16.4 (2026-09-23)</summary>

#### Fixes

- Fixed an issue where Terraform fails when rendering policy evaluation outcomes for older versions of Terraform Enterprise ([#39095](https://github.com/hashicorp/terraform/issues/39095)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L46))
- stacks: Fix invalid deferred error triggered by provider returning a deferral when a resource also has an unknown count/for_each. ([#39237](https://github.com/hashicorp/terraform/issues/39237)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L48))

</details>

<details><summary>1.16.5 (2026-10-02)</summary>

#### Fixes

- Fixed a crash that happens when a tainted instance state is seen without a valid status ([#39287](https://github.com/hashicorp/terraform/issues/39287)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L36))
- resource-identity: fixed an issue where resource identity could be nil during delete ([#39285](https://github.com/hashicorp/terraform/issues/39285)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L38))

</details>

<details><summary>1.16.6 (2026-10-14)</summary>

#### Fixes

- cloud: `terraform init` can now find workspaces selected by key/value tags whose keys contain uppercase letters. ([#39306](https://github.com/hashicorp/terraform/issues/39306)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L21))
- test: Allow module under test to use deprecated outputs. ([#39207](https://github.com/hashicorp/terraform/issues/39207)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L23))

</details>

<details><summary>1.16.7 (2026-10-22)</summary>

#### Fixes

- command/test: Avoid panic when testing schema w/ deprecated values ([#39406](https://github.com/hashicorp/terraform/issues/39406)) ([source](https://github.com/hashicorp/terraform/blob/v1.16/CHANGELOG.md#L8))

</details>

<details><summary>1.17.0 (2026-10-21)</summary>

#### Features

- Terraform now supports variables and locals in provider requirements ([#39153](https://github.com/hashicorp/terraform/issues/39153)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L20))
- A new `-minimal-refresh` planning option has been added, which will only refresh resources that have proposed changes. ([#35290](https://github.com/hashicorp/terraform/issues/35290)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L22))
- policy: Terraform Policy is now generally available. The `-policies` flag for `plan`, `apply`, and `query` no longer requires an experimental build or the `-allow-experimental-features` flag. Query policies evaluate resources discovered by list blocks and report human-readable or JSON results. See https://developer.hashicorp.com/terraform/policy for more information. ([#38970](https://github.com/hashicorp/terraform/issues/38970)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L24))
- command/init: Enrich log messages with provider versions ([#38918](https://github.com/hashicorp/terraform/issues/38918)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L29))
- test: Add mock_provider support for ephemeral resources ([#38928](https://github.com/hashicorp/terraform/issues/38928)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L31))
- command/login: display warning after successful login if user is subject to an organization's TTL policy ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L33))

#### Fixes

- funcs: pow and log no longer panic when result is not a number ([#38912](https://github.com/hashicorp/terraform/issues/38912)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L38))
- ephemeral: Terraform will now use and display diagnostics raised when _renewing_ an ephemeral resource. This may cause warnings to appear that previously were lost. We expect that any error diagnostics that were previously lost would have caused confusing downstream errors, so we do not anticipate this change to be breaking. ([#38989](https://github.com/hashicorp/terraform/issues/38989)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L40))
- test: Deterministic dependency ordering in cleanup graph ([#38247](https://github.com/hashicorp/terraform/issues/38247)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L42))
- query: report Unknown and N/A results for policy evaluations of discovered resources ([#39058](https://github.com/hashicorp/terraform/issues/39058)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L44))
- query: `terraform query -policies` now rejects a path that is not a directory, instead of passing it on to the policy engine ([#39238](https://github.com/hashicorp/terraform/issues/39238)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L46))
- funcs: The `log` and `pow` functions now raise errors when they return +/- Infinity, as this result is not usable in downstream code. ([#39304](https://github.com/hashicorp/terraform/issues/39304)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L48))
- show: Read variable values from the plan file when showing a local plan ([#39362](https://github.com/hashicorp/terraform/issues/39362)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L50))

</details>

<details><summary>1.17.1 (2026-10-28)</summary>

#### Fixes

- fmt: The original content of a file is now preserved if Terraform experiences an error writing to the original file. ([#39303](https://github.com/hashicorp/terraform/issues/39303)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L8))
- Always evaluate module policies during destroy plan ([#39455](https://github.com/hashicorp/terraform/issues/39455)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L10))
- stacks: report every policy result for a replaced resource instead of only one of them ([#39453](https://github.com/hashicorp/terraform/issues/39453)) ([source](https://github.com/hashicorp/terraform/blob/v1.17/CHANGELOG.md#L12))

</details>
