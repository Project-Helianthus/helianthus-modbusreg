#!/usr/bin/env python3
"""Validate registry ownership and standard/vendor separation."""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Iterable


EXPECTED_POLICY = {
    "schema": "helianthus-modbusreg-boundary/v1",
    "repository_mode": "single_multi_vendor",
    "implementation_lock": "bootstrap_only",
    "allowed_product_go_files": ["doc.go"],
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
    "profile_version",
    "sources",
    "transformation",
    "claim_state",
    "applicability",
    "sanitization",
    "license",
    "code_mapping",
}
SOURCE_KEYS = {"id", "locator", "permission", "license", "sha256", "redistribution"}
APPLICABILITY_KEYS = {
    "vendor",
    "models",
    "gateways",
    "firmware",
    "protocol_modes",
    "address_basis",
    "exclusions",
}
SANITIZATION_KEYS = {
    "credentials_removed",
    "private_addresses_removed",
    "customer_identifiers_removed",
    "unrelated_payload_removed",
    "method",
}
CODE_MAPPING_KEYS = {"package", "profile_version", "fixtures"}


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


def validate_bootstrap_lock(root: Path, policy: dict[str, object]) -> None:
    if policy["implementation_lock"] != "bootstrap_only":
        raise PolicyError("implementation lock must remain bootstrap_only")
    allowed = {str(item) for item in policy["allowed_product_go_files"]}
    actual = {
        path.relative_to(root).as_posix()
        for path in root.rglob("*.go")
        if ".git" not in path.parts
    }
    unexpected = actual - allowed
    missing = allowed - actual
    if unexpected or missing:
        raise PolicyError(
            f"bootstrap Go-file lock mismatch: unexpected={sorted(unexpected)} "
            f"missing={sorted(missing)}"
        )


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
        if evidence["profile"] != profile_dir.name:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} profile must match its directory"
            )
        if not isinstance(evidence["profile_version"], str) or not evidence[
            "profile_version"
        ].strip():
            raise PolicyError(
                f"{evidence_path.relative_to(root)} requires profile_version"
            )
        sources = evidence["sources"]
        if not isinstance(sources, list) or not sources:
            raise PolicyError(f"{evidence_path.relative_to(root)} requires public sources")
        source_ids: set[str] = set()
        for source in sources:
            if not isinstance(source, dict) or set(source) != SOURCE_KEYS:
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} source keys are incomplete"
                )
            if not all(
                isinstance(source[key], str) and source[key].strip()
                for key in ("id", "locator", "permission", "license")
            ):
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} source metadata is incomplete"
                )
            if source["id"] in source_ids:
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} has duplicate source ids"
                )
            source_ids.add(source["id"])
            if not (
                source["locator"].startswith("https://")
                or source["locator"].startswith("urn:helianthus:evidence:")
            ):
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} source locator is not stable/public"
                )
            if source["permission"] not in {
                "independent_fact_publication_permitted",
                "source_redistribution_permitted",
                "operator_owned_capture",
            }:
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} source permission is not admissible"
                )
            if re.fullmatch(r"[0-9a-f]{64}", source["sha256"]) is None:
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} source sha256 is invalid"
                )
            if source["redistribution"] not in {
                "allowed",
                "metadata_only",
                "prohibited_source_copy",
            }:
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} source redistribution is invalid"
                )
        if not isinstance(evidence["transformation"], str) or not evidence[
            "transformation"
        ].strip():
            raise PolicyError(
                f"{evidence_path.relative_to(root)} requires a transformation record"
            )
        if evidence["claim_state"] != "PROVEN":
            raise PolicyError(
                f"{evidence_path.relative_to(root)} vendor code requires PROVEN claims"
            )
        applicability = evidence["applicability"]
        if not isinstance(applicability, dict) or set(applicability) != APPLICABILITY_KEYS:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} applicability keys are incomplete"
            )
        if applicability["vendor"] != profile_dir.name:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} applicability vendor mismatch"
            )
        for key in ("models", "gateways", "firmware", "protocol_modes", "address_basis"):
            if not isinstance(applicability[key], list) or not applicability[key]:
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} applicability {key} is empty"
                )
        if not isinstance(applicability["exclusions"], list):
            raise PolicyError(
                f"{evidence_path.relative_to(root)} applicability exclusions must be a list"
            )
        sanitization = evidence["sanitization"]
        if not isinstance(sanitization, dict) or set(sanitization) != SANITIZATION_KEYS:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} sanitization keys are incomplete"
            )
        for key in SANITIZATION_KEYS - {"method"}:
            if sanitization[key] is not True:
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} sanitization {key} must be true"
                )
        if not isinstance(sanitization["method"], str) or not sanitization[
            "method"
        ].strip():
            raise PolicyError(
                f"{evidence_path.relative_to(root)} sanitization method is required"
            )
        if evidence["license"] != "CC0-1.0":
            raise PolicyError(
                f"{evidence_path.relative_to(root)} public facts must use CC0-1.0"
            )
        code_mapping = evidence["code_mapping"]
        if not isinstance(code_mapping, dict) or set(code_mapping) != CODE_MAPPING_KEYS:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} code_mapping keys are incomplete"
            )
        expected_package = (
            "github.com/Project-Helianthus/helianthus-modbusreg/"
            + profile_dir.relative_to(root).as_posix()
        )
        if code_mapping["package"] != expected_package:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} code_mapping package mismatch"
            )
        if code_mapping["profile_version"] != evidence["profile_version"]:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} code_mapping version mismatch"
            )
        fixtures = code_mapping["fixtures"]
        if not isinstance(fixtures, list) or not fixtures:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} requires mapped fixtures"
            )
        for fixture in fixtures:
            fixture_path = Path(str(fixture))
            if (
                fixture_path.is_absolute()
                or ".." in fixture_path.parts
                or not (root / fixture_path).is_file()
            ):
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} fixture is absent or unsafe: {fixture}"
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
    validate_bootstrap_lock(root, policy)
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
