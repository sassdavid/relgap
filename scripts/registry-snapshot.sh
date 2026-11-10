#!/usr/bin/env bash
# Regenerates the embedded mise and aqua registry snapshots in internal/registry/snapshot from the
# default branches of jdx/mise and aquaproj/aqua-registry (both MIT). Review the diff, then commit.
#   mise run registry:snapshot
#   MISE_DIR=~/src/github/_reference/jdx/mise AQUA_DIR=... mise run registry:snapshot  # use checkouts
set -euo pipefail

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

checkout() { # repo dir paths...
  local repo=$1 dir=$2
  shift 2
  git clone -q --depth 1 --filter=blob:none --sparse "https://github.com/$repo.git" "$dir"
  git -C "$dir" sparse-checkout set --no-cone "$@"
}

mise_dir=${MISE_DIR:-}
if [[ -z "$mise_dir" ]]; then
  mise_dir=$tmp/mise
  checkout jdx/mise "$mise_dir" /registry/ /LICENSE
fi
aqua_dir=${AQUA_DIR:-}
if [[ -z "$aqua_dir" ]]; then
  aqua_dir=$tmp/aqua-registry
  checkout aquaproj/aqua-registry "$aqua_dir" /registry.yaml /LICENSE
fi

out=internal/registry/snapshot
go run ./scripts/regsnapshot \
  -mise "$mise_dir" -mise-commit "$(git -C "$mise_dir" log -1 --format=%H)" \
  -mise-date "$(git -C "$mise_dir" log -1 --format=%cI)" \
  -aqua "$aqua_dir" -aqua-commit "$(git -C "$aqua_dir" log -1 --format=%H)" \
  -aqua-date "$(git -C "$aqua_dir" log -1 --format=%cI)" \
  -out "$out"
cp "$mise_dir/LICENSE" "$out/mise.LICENSE"
cp "$aqua_dir/LICENSE" "$out/aqua-registry.LICENSE"
echo "snapshots updated; review them with: git diff --stat -- $out" >&2
