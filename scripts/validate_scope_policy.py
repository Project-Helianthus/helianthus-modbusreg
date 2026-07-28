#!/usr/bin/env python3
"""Validate registry ownership and standard/vendor separation."""

from __future__ import annotations

import hashlib
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Iterable


EXPECTED_POLICY = {
    "schema": "helianthus-modbusreg-boundary/v2",
    "repository_mode": "single_multi_vendor",
    "implementation_lock": "m2_01_contracts_only",
    "allowed_repository_files": [
        ".github/CODEOWNERS",
        ".github/workflows/ci.yml",
        ".gitignore",
        "AGENTS.md",
        "CONTRIBUTING.md",
        "LICENSE",
        "README.md",
        "adversarial_round1_test.go",
        "codec.go",
        "contracts_test.go",
        "doc.go",
        "docs/m2-01-api.md",
        "go.mod",
        "go.sum",
        "observation.go",
        "policy/modbus-companion-consumer-lock-v1.json",
        "policy/modbus-runtime-consumer-lock-v1.json",
        "policy/registry-boundary.json",
        "profile.go",
        "scripts/ci_local.sh",
        "scripts/scope_gate.sh",
        "scripts/validate_companion_lock.sh",
        "scripts/validate_consumer_locks.py",
        "scripts/validate_scope_policy.py",
        "serialization.go",
        "tests/test_consumer_locks.py",
        "tests/test_scope_policy.py",
        "version.go",
    ],
    "allowed_product_go_files": [
        "codec.go",
        "doc.go",
        "observation.go",
        "profile.go",
        "serialization.go",
        "version.go",
    ],
    "allowed_product_go_sha256": {
        "codec.go": "42ad189f96889e64a15c370dc6aae22d0d42b967fb48ecc7c47a6b0b4657572a",
        "doc.go": "2bb397c67dba2394c500c0e11be9e46d17006fcba85d36ce430d1ec5ca5891e3",
        "observation.go": (
            "410934b3efe09cea112b77f1b73297238e8b087b0b6e55fb03375b5dbaa9d838"
        ),
        "profile.go": (
            "541c1b170b9e8ff58c98b6ba9cae92e663fc80d0b0630e34cebf42f1b53208f3"
        ),
        "serialization.go": (
            "b7e04ac61fb4f0aee5187dd723bada23ef22b5028e5245720886eeb75b557e6c"
        ),
        "version.go": (
            "cb70b1065e77831d5cd9d1171ee555037fe6369b3149e120f9e5236dec7d0865"
        ),
    },
    "allowed_test_go_files": [
        "adversarial_round1_test.go",
        "contracts_test.go",
    ],
    "documentation_consumer_lock": "policy/modbus-companion-consumer-lock-v1.json",
    "runtime_consumer_lock": "policy/modbus-runtime-consumer-lock-v1.json",
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


def validate_repository_inventory(root: Path, policy: dict[str, object]) -> None:
    allowed = {str(item) for item in policy["allowed_repository_files"]}
    actual: set[str] = set()
    for path in root.rglob("*"):
        if ".git" in path.parts:
            continue
        if path.is_symlink():
            raise PolicyError(
                f"repository inventory contains a symlink: {path.relative_to(root)}"
            )
        if path.is_file():
            actual.add(path.relative_to(root).as_posix())
    unexpected = actual - allowed
    missing = allowed - actual
    if unexpected or missing:
        raise PolicyError(
            f"repository file inventory mismatch: unexpected={sorted(unexpected)} "
            f"missing={sorted(missing)}"
        )


def validate_implementation_lock(root: Path, policy: dict[str, object]) -> None:
    if policy["implementation_lock"] != "m2_01_contracts_only":
        raise PolicyError("implementation lock must remain m2_01_contracts_only")
    allowed = {str(item) for item in policy["allowed_product_go_files"]}
    actual = {
        path.relative_to(root).as_posix()
        for path in root.rglob("*.go")
        if ".git" not in path.parts and not path.name.endswith("_test.go")
    }
    unexpected = actual - allowed
    missing = allowed - actual
    if unexpected or missing:
        raise PolicyError(
            f"M2-01 product Go-file lock mismatch: unexpected={sorted(unexpected)} "
            f"missing={sorted(missing)}"
        )
    allowed_tests = {str(item) for item in policy["allowed_test_go_files"]}
    actual_tests = {
        path.relative_to(root).as_posix()
        for path in root.rglob("*_test.go")
        if ".git" not in path.parts
    }
    if actual_tests != allowed_tests:
        raise PolicyError(
            f"M2-01 test Go-file lock mismatch: actual={sorted(actual_tests)}"
        )
    expected_hashes = {
        str(path): str(digest)
        for path, digest in policy["allowed_product_go_sha256"].items()
    }
    if set(expected_hashes) != allowed:
        raise PolicyError("M2-01 Go-file hash inventory differs from allowed files")
    for relative, expected in expected_hashes.items():
        actual = hashlib.sha256((root / relative).read_bytes()).hexdigest()
        if actual != expected:
            raise PolicyError(f"M2-01 product Go-file content changed: {relative}")
    for key in ("documentation_consumer_lock", "runtime_consumer_lock"):
        relative = Path(str(policy[key]))
        if relative.is_absolute() or ".." in relative.parts or not (root / relative).is_file():
            raise PolicyError(f"consumer lock is absent or unsafe: {relative}")


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
        artifacts = [
            path
            for path in profile_dir.rglob("*")
            if path.is_file() and path.name != evidence_name
        ]
        if not artifacts:
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
    validate_repository_inventory(root, policy)
    validate_implementation_lock(root, policy)
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
