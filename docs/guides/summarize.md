---
description: Configure a model provider for digest --summarize, see what relgap sends and how it checks the citations, and keep the summary out of your CI gates.
---

# Add an LLM summary

`relgap digest --summarize` adds an upgrade-risk summary to a digest: up to 5 risks and the
required actions, each citing the entry IDs it rests on. A model you configure writes it. It is
off until you set `llm.provider` and `llm.model`, and relgap never turns it on by itself.

## Configure a provider

Set the `llm` keys in `.relgap.yaml`, your user config or `RELGAP_LLM_*` variables
([Configuration](/reference/config#llm)). `llm.provider` takes one of four values.

`anthropic` posts to the Anthropic Messages API at `https://api.anthropic.com`, or at
`llm.base_url` when set. The key comes from `ANTHROPIC_API_KEY`, or from the variable that
`llm.api_key_env` names:

```yaml
llm:
  provider: anthropic
  model: claude-sonnet-5-5
```

`openai-compatible` posts to `<llm.base_url>/chat/completions` on any server that speaks that
API, local ones included. `llm.base_url` is required. A key is sent only when `llm.api_key_env`
names a variable:

```yaml
llm:
  provider: openai-compatible
  model: my-model
  base_url: http://localhost:11434/v1
```

`bedrock` calls the Amazon Bedrock Converse API with the AWS credentials of `llm.aws_profile`, or
the default AWS chain when it's empty. The region comes from `AWS_REGION` or the profile. With
`llm.api_key_env` set, relgap sends that variable's Amazon Bedrock API key as a bearer token
instead of signing the request.

`bedrock-mantle` calls Bedrock's Mantle endpoint, `https://bedrock-mantle.<region>.api.aws`, with
the Messages API for `anthropic.*` model IDs and the Chat Completions API for the rest. `llm.api`
picks one of them by hand. It signs requests with the AWS credentials, or sends the key that
`llm.api_key_env` names. With a key and `llm.base_url`, it needs no AWS config at all.

relgap reads keys from environment variables only, never from a file, and never writes them to
the cache or the logs.

## Run it

```sh
relgap digest terraform 1.15.8..1.17.1 --format md --summarize > upgrade-notes.md
relgap digest --from-mise --format md --summarize > upgrade-notes.md
```

`--summarize` works in every digest mode, including `--all`, `--base` and `--pull-request`. Each
run makes one model call for the whole digest. The summary comes first in every format, labelled
"LLM-generated, verify against cited sources" with the provider and model. In `--format json` it
is `data.summary`, with the risks and actions, each with its text and cited IDs, and the counts of
entries sent, entries left out and claims dropped.

## What relgap sends

The model gets the tool, the range and the entries the digest lists, each with its ID, version,
kind, text and source URL. It never gets a local path, a pin location or a token.
`--only` and `--min-confidence` shrink what is sent. [Network, privacy and
tokens](/concepts/network#summaries-from-a-model) lists every field and the host each provider
uses.

The prompt marks the entries as untrusted text copied from release notes and tells the model not
to follow instructions in them. When the entries would exceed `llm.max_input_tokens` (default
30,000 estimated tokens), relgap leaves out the lowest-confidence entries first and warns how many.

## How relgap checks the answer

relgap keeps a risk or action only when it cites at least one entry ID and every ID it cites was
in the request. It drops every other line and counts them. When no line is left, there is no
summary. In Markdown, each kept line links its IDs to the entries' sources, so you can check
every claim against the release notes.

## What it never changes

The summary is advisory. `--fail-on`, the exit code and SARIF never read it, so a CI gate gives
the same answer with or without `--summarize`. Every problem is a warning on stderr and the digest
prints without a summary:

- no `llm` config, an unknown provider or a missing key
- `--offline`, which skips the call
- a provider error after the retries
- no answer within `llm.timeout` (default 120s, at most 2m)

[Gate CI on breaking changes, EOL and vulnerabilities](/guides/ci-gate) and
[Comment upgrade notes on Renovate pull requests](/guides/renovate-pr) show the digest in CI; add
`--summarize` there when the job has the provider's key.
