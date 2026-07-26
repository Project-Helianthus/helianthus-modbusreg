#!/usr/bin/env bash
set -euo pipefail

expected_module="module github.com/Project-Helianthus/helianthus-modbusreg"
if [[ "$(head -n 1 go.mod)" != "$expected_module" ]]; then
  echo "Unexpected Go module path."
  exit 1
fi

for marker in \
    "One Registry, Multiple Vendors" \
    "standard family" \
    "vendor overlay" \
    "does not own"; do
  if ! grep -Fq "$marker" README.md; then
    echo "README is missing scope marker: $marker"
    exit 1
  fi
done

python3 scripts/validate_scope_policy.py
