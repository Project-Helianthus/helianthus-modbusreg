#!/usr/bin/env bash
# Published acquisition: resolve docs-semantic main, then the selected donor,
# each pattern exemplar, and each expectation or approved-difference revision.
set -euo pipefail

docs_semantic_url="https://github.com/Project-Helianthus/helianthus-docs-semantic.git"

gitc() {
  git -c core.autocrlf=false "$@"
}

stage_tool() {
  dest="$1"
  sha="$(gitc ls-remote "$docs_semantic_url" refs/heads/main | awk 'NR==1 { print $1 }')"
  case "$sha" in
    *[!0-9a-fA-F]* | "")
      echo "docs-semantic main is unresolved" >&2
      exit 1
      ;;
  esac
  if [ "${#sha}" -lt 40 ]; then
    echo "docs-semantic main is unresolved" >&2
    exit 1
  fi
  gitc clone "$docs_semantic_url" "$dest"
  gitc -C "$dest" fetch --depth 1 origin "$sha"
  gitc -C "$dest" checkout --detach "$sha"
  printf '%s\n' "$sha"
}

read_spec() {
  spec="$1"
  donor_fetches="$(mktemp)"
  tool_fetches="$(mktemp)"
  donor_url=""
  donor_sha=""
  while IFS= read -r kind a b; do
    case "$kind" in
      donor-url) donor_url="$a" ;;
      donor-sha) donor_sha="$a" ;;
      donor-fetch) printf '%s %s\n' "$a" "$b" >> "$donor_fetches" ;;
      tool-fetch) printf '%s %s\n' "$a" "$b" >> "$tool_fetches" ;;
      "") ;;
      *)
        echo "unknown acquisition record" >&2
        exit 1
        ;;
    esac
  done < "$spec"
}

fetch_each() {
  repo="$1"
  list="$2"
  while IFS= read -r url rev; do
    if [ -n "$url" ]; then
      gitc -C "$repo" fetch --depth 1 "$url" "$rev"
    fi
  done < "$list"
}

stage_rest() {
  tool_dir="$1"
  donor_dir="$2"
  spec="$3"
  read_spec "$spec"
  trap 'rm -f "$donor_fetches" "$tool_fetches"' EXIT
  if [ -z "$donor_url" ] || [ -z "$donor_sha" ]; then
    echo "selected manifest has no donor" >&2
    exit 1
  fi
  gitc clone "$donor_url" "$donor_dir"
  gitc -C "$donor_dir" fetch --depth 1 origin "$donor_sha"
  gitc -C "$donor_dir" checkout --detach "$donor_sha"
  fetch_each "$donor_dir" "$donor_fetches"
  fetch_each "$tool_dir" "$tool_fetches"
}

if [ "${1:-}" = "tool" ] && [ "$#" -eq 2 ]; then
  stage_tool "$2"
elif [ "${1:-}" = "rest" ] && [ "$#" -eq 4 ]; then
  stage_rest "$2" "$3" "$4"
else
  echo "usage: acquire.sh tool <dir> | acquire.sh rest <tool-dir> <donor-dir> <spec>" >&2
  exit 1
fi
