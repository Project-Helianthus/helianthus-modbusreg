#!/usr/bin/env python3
"""Validate registry ownership and standard/vendor separation."""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Iterable


EXPECTED_POLICY = {
    "schema": "helianthus-modbusreg-boundary/v1",
    "repository_mode": "single_multi_vendor",
    "standard_root": "profiles/standard",
    "vendor_root": "profiles/vendor",
    "vendor_evidence_file": "evidence.json",
    "allowed_project_import_prefixes": [
        "github.com/Project-Helianthus/helianthus-modbus",
        "github.com/Project-Helianthus/helianthus-modbusreg",
    ],
    "forbidden_import_prefixes": ["net", "syscall", "golang.org/x/sys/"],
    "forbidden_import_tokens": ["serial", "modbus"],
    "vendor_identifiers": ["fronius", "growatt", "huawei"],
    "standard_family_vendor_neutral": True,
    "vendor_overlay_requires_public_evidence": True,
}

EVIDENCE_KEYS = {
    "profile",
    "sources",
    "claim_state",
    "applicability",
    "license",
}


class PolicyError(RuntimeError):
    pass


def load_policy(root: Path) -> dict[str, object]:
    path = root / "policy" / "registry-boundary.json"
    try:
        policy = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise PolicyError(f"cannot load {path}: {exc}") from exc
    if policy != EXPECTED_POLICY:
        raise PolicyError("registry policy differs from the authorized boundary")
    return policy


def project_import_allowed(import_path: str, allowed: tuple[str, ...]) -> bool:
    return any(
        import_path == prefix or import_path.startswith(prefix + "/")
        for prefix in allowed
    )


def validate_imports(imports: Iterable[str], policy: dict[str, object]) -> None:
    allowed = tuple(str(item) for item in policy["allowed_project_import_prefixes"])
    forbidden_prefixes = tuple(str(item) for item in policy["forbidden_import_prefixes"])
    forbidden_tokens = tuple(str(item) for item in policy["forbidden_import_tokens"])
    runtime_prefix = "github.com/Project-Helianthus/helianthus-modbus"

    for import_path in imports:
        lowered = import_path.lower()
        if import_path.startswith("github.com/Project-Helianthus/"):
            if not project_import_allowed(import_path, allowed):
                raise PolicyError(f"forbidden Helianthus dependency: {import_path}")
        if import_path == "net" or import_path.startswith("net/"):
            raise PolicyError(f"registry must not own network transport: {import_path}")
        if any(
            import_path == prefix or import_path.startswith(prefix)
            for prefix in forbidden_prefixes
            if prefix != "net"
        ):
            raise PolicyError(f"forbidden transport dependency: {import_path}")
        for token in forbidden_tokens:
            if token in lowered and not import_path.startswith(runtime_prefix):
                raise PolicyError(f"forbidden transport/framing dependency: {import_path}")


def validate_standard_sources(root: Path, policy: dict[str, object]) -> None:
    standard_root = root / str(policy["standard_root"])
    vendor_identifiers = tuple(str(item) for item in policy["vendor_identifiers"])
    if not standard_root.exists():
        return
    for path in sorted(standard_root.rglob("*.go")):
        lowered = path.read_text(encoding="utf-8").lower()
        for vendor in vendor_identifiers:
            if vendor in lowered:
                raise PolicyError(
                    f"{path.relative_to(root)} leaks vendor {vendor} into a standard family"
                )
        if "/profiles/vendor/" in lowered:
            raise PolicyError(
                f"{path.relative_to(root)} imports a vendor overlay from a standard family"
            )


def validate_vendor_evidence(root: Path, policy: dict[str, object]) -> None:
    vendor_root = root / str(policy["vendor_root"])
    if not vendor_root.exists():
        return
    evidence_name = str(policy["vendor_evidence_file"])
    for profile_dir in sorted(path for path in vendor_root.iterdir() if path.is_dir()):
        if not any(profile_dir.rglob("*.go")):
            continue
        evidence_path = profile_dir / evidence_name
        try:
            evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise PolicyError(
                f"{profile_dir.relative_to(root)} lacks valid {evidence_name}: {exc}"
            ) from exc
        if not isinstance(evidence, dict) or set(evidence) != EVIDENCE_KEYS:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} must contain exact evidence keys"
            )
        if evidence["claim_state"] not in {"PROVEN", "HYPOTHESIS", "UNKNOWN"}:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} has invalid claim_state"
            )
        if not isinstance(evidence["sources"], list) or not evidence["sources"]:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} requires at least one public source"
            )


def go_imports(root: Path) -> list[str]:
    result = subprocess.run(
        [
            "go",
            "list",
            "-f",
            '{{range .Imports}}{{.}}{{"\\n"}}{{end}}',
            "./...",
        ],
        cwd=root,
        env={**os.environ, "GOWORK": "off"},
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise PolicyError(f"go list failed: {result.stderr.strip()}")
    return [line for line in result.stdout.splitlines() if line]


def validate(root: Path) -> None:
    policy = load_policy(root)
    validate_imports(go_imports(root), policy)
    validate_standard_sources(root, policy)
    validate_vendor_evidence(root, policy)


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    try:
        validate(root)
    except PolicyError as exc:
        print(f"scope policy failed: {exc}", file=sys.stderr)
        return 1
    print("Scope policy passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
