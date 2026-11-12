#!/usr/bin/env bash
# Turns the live corpus results into issues: one "corpus drift: <tool>" issue per drifting tool,
# opened or commented on, and closed again once the tool is clean. It never changes the
# repository, goldens included. .github/workflows/live.yml runs it after `mise run test:live`.
#   scripts/corpus-issues.sh <results dir>
# Needs GH_TOKEN, GH_REPO, RUN_URL and COMMIT in the environment.
set -euo pipefail

dir=${1:?usage: corpus-issues.sh <results dir>}
: "${RUN_URL:?}" "${COMMIT:?}"
results="$dir/results.json"
label=corpus-drift
max_diff=60000

gh label create "$label" --color D93F0B --force \
  --description "The nightly live corpus differs from testdata/corpus.yaml or a golden" >/dev/null

open_issues=$(gh issue list --label "$label" --state open --limit 500 --json number,title)

# open_issue prints the number of the open drift issue for a tool, if there is one.
open_issue() {
  jq -r --arg title "corpus drift: $1" '.[] | select(.title == $title) | .number' <<<"$open_issues" | head -n 1
}

# write_body writes the issue text for one drifting tool.
write_body() {
  local row=$1 out=$2 tool range slug golden
  tool=$(jq -r .tool <<<"$row")
  range=$(jq -r '.range // ""' <<<"$row")
  slug=$(jq -r .slug <<<"$row")
  golden=$(jq -r '.golden // ""' <<<"$row")
  {
    echo "The nightly live corpus run found a difference for $tool $range."
    echo
    echo "- Run: $RUN_URL"
    echo "- Commit: $COMMIT"
    if [[ -n $golden ]]; then
      echo "- Golden: testdata/golden/$golden"
    fi
    echo
    echo "What differs:"
    echo
    jq -r '.problems[] | "- " + .' <<<"$row"
    if [[ -f "$dir/$slug.want.json" ]]; then
      echo
      echo "<details><summary>Diff of the digest data (first $max_diff characters; the run's corpus artifact has both files)</summary>"
      echo
      echo '```diff'
      { diff -u --label "golden $golden" --label "live run" "$dir/$slug.want.json" "$dir/$slug.got.json" || true; } |
        head -c "$max_diff"
      echo
      echo '```'
      echo
      echo "</details>"
    fi
    echo
    echo "If upstream changed on purpose, update testdata/corpus.yaml, or the golden with"
    echo "\`mise run test:golden -- -update\`, review the diff and open a pull request. This job never"
    echo "updates either."
  } >"$out"
}

body=$(mktemp)
trap 'rm -f "$body"' EXIT
while IFS= read -r row; do
  tool=$(jq -r .tool <<<"$row")
  status=$(jq -r .status <<<"$row")
  number=$(open_issue "$tool")
  case "$status" in
    drift)
      write_body "$row" "$body"
      if [[ -n $number ]]; then
        gh issue comment "$number" --body-file "$body" >/dev/null
        echo "commented on #$number: $tool"
      else
        gh issue create --title "corpus drift: $tool" --label "$label" --body-file "$body"
      fi
      ;;
    ok)
      if [[ -n $number ]]; then
        gh issue close "$number" --comment "The live corpus run $RUN_URL found no drift for $tool." >/dev/null
        echo "closed #$number: $tool"
      fi
      ;;
  esac
done < <(jq -c '.[]' "$results")
