#!/usr/bin/env bash
# Fetch one base ref from origin. A job token is sent only on this command
# and is not written into git config. Git must not prompt for a username.
set -euo pipefail

base_ref="${1:?base ref required}"
export GIT_TERMINAL_PROMPT=0

if [ -n "${GITHUB_TOKEN:-}" ]; then
  basic=$(printf 'x-access-token:%s' "$GITHUB_TOKEN" | base64 | tr -d '\n')
  git -c credential.helper= \
    -c "http.extraheader=AUTHORIZATION: basic ${basic}" \
    fetch --no-tags --depth=1 origin "$base_ref"
else
  git -c credential.helper= fetch --no-tags --depth=1 origin "$base_ref"
fi
