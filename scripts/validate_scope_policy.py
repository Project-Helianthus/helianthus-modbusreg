#!/usr/bin/env python3
"""Validate the public registry tree against stable ownership boundaries."""

from __future__ import annotations

import ast
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Iterable
from urllib.parse import urlparse


SCHEMA = "helianthus-modbusreg-boundary/v5"
REPOSITORY_MODE = "single_multi_vendor"
IMPLEMENTATION_SCOPE = "public_registry_semantics"
MODULE_PATH = "github.com/Project-Helianthus/helianthus-modbusreg"
PUBLIC_MODBUS = "github.com/Project-Helianthus/helianthus-modbus"
POLICY_KEYS = {
    "schema",
    "repository_mode",
    "implementation_scope",
    "module_path",
    "required_public_dependencies",
    "allowed_project_import_prefixes",
    "allowed_non_io_net_imports",
    "forbidden_product_import_prefixes",
    "forbidden_product_import_tokens",
    "forbidden_ownership_directory_names",
    "forbidden_product_file_stems",
    "standard_root",
    "vendor_root",
    "vendor_evidence_file",
    "vendor_identifiers",
    "standard_family_vendor_neutral",
    "vendor_overlay_requires_public_evidence",
}
REQUIRED_FORBIDDEN_IMPORT_PREFIXES = {"net", "syscall", "golang.org/x/sys/"}
REQUIRED_FORBIDDEN_IMPORT_TOKENS = {"framing", "serial", "socket"}
REQUIRED_SCOPE_DIRECTORIES = {
    "bindings",
    "canonical",
    "framing",
    "gateway",
    "private",
    "semantics",
    "serial",
    "sockets",
    "transport",
}
REQUIRED_SCOPE_FILE_STEMS = {
    "binding",
    "canonical",
    "framing",
    "gateway",
    "semantic",
    "serial",
    "socket",
    "transport",
}
EVIDENCE_KEYS = {
    "schema",
    "profile",
    "profile_version",
    "public_sources",
    "applicability",
    "license",
}
SOURCE_KEYS = {"id", "locator", "license"}
EVIDENCE_SCHEMA = "helianthus-modbusreg-vendor-evidence/v1"


class PolicyError(RuntimeError):
    pass


def string_list(policy: dict[str, object], key: str) -> list[str]:
    value = policy.get(key)
    if (
        not isinstance(value, list)
        or not value
        or any(not isinstance(item, str) or not item.strip() for item in value)
        or len(set(value)) != len(value)
    ):
        raise PolicyError(f"policy {key} must be a nonempty unique string list")
    return value


def safe_relative_path(value: object, expected: str, label: str) -> str:
    if value != expected:
        raise PolicyError(f"policy {label} must be {expected}")
    path = Path(expected)
    if path.is_absolute() or ".." in path.parts:
        raise PolicyError(f"policy {label} is unsafe")
    return expected


def load_policy(root: Path) -> dict[str, object]:
    path = root / "policy" / "registry-boundary.json"
    try:
        policy = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise PolicyError(f"cannot load {path}: {exc}") from exc
    if not isinstance(policy, dict) or set(policy) != POLICY_KEYS:
        raise PolicyError("registry policy contains missing or obsolete fields")
    expected_scalars = {
        "schema": SCHEMA,
        "repository_mode": REPOSITORY_MODE,
        "implementation_scope": IMPLEMENTATION_SCOPE,
        "module_path": MODULE_PATH,
    }
    for key, expected in expected_scalars.items():
        if policy[key] != expected:
            raise PolicyError(f"policy {key} must be {expected}")
    if policy["standard_family_vendor_neutral"] is not True:
        raise PolicyError("standard families must remain vendor-neutral")
    if policy["vendor_overlay_requires_public_evidence"] is not True:
        raise PolicyError("vendor overlays must require public evidence")

    required_dependencies = string_list(policy, "required_public_dependencies")
    if required_dependencies != [PUBLIC_MODBUS]:
        raise PolicyError("the public modbus module must be the sole dependency")
    allowed_projects = set(string_list(policy, "allowed_project_import_prefixes"))
    if allowed_projects != {PUBLIC_MODBUS, MODULE_PATH}:
        raise PolicyError("project import boundary is invalid")
    if set(string_list(policy, "allowed_non_io_net_imports")) != {"net/url"}:
        raise PolicyError("net/url must be the only non-I/O net exception")
    if not REQUIRED_FORBIDDEN_IMPORT_PREFIXES.issubset(
        string_list(policy, "forbidden_product_import_prefixes")
    ):
        raise PolicyError("product import boundary omits a required prefix")
    if not REQUIRED_FORBIDDEN_IMPORT_TOKENS.issubset(
        string_list(policy, "forbidden_product_import_tokens")
    ):
        raise PolicyError("product import boundary omits a required token")
    if not REQUIRED_SCOPE_DIRECTORIES.issubset(
        {
            item.casefold()
            for item in string_list(policy, "forbidden_ownership_directory_names")
        }
    ):
        raise PolicyError("registry ownership directory boundary is incomplete")
    if not REQUIRED_SCOPE_FILE_STEMS.issubset(
        {
            item.casefold()
            for item in string_list(policy, "forbidden_product_file_stems")
        }
    ):
        raise PolicyError("registry ownership filename boundary is incomplete")
    safe_relative_path(policy["standard_root"], "profiles/standard", "standard_root")
    safe_relative_path(policy["vendor_root"], "profiles/vendor", "vendor_root")
    safe_relative_path(
        policy["vendor_evidence_file"],
        "evidence.json",
        "vendor_evidence_file",
    )
    string_list(policy, "vendor_identifiers")
    return policy


def project_import_allowed(import_path: str, allowed: set[str]) -> bool:
    return any(
        import_path == prefix or import_path.startswith(prefix + "/")
        for prefix in allowed
    )


def validate_imports(imports: Iterable[str], policy: dict[str, object]) -> None:
    allowed_projects = set(string_list(policy, "allowed_project_import_prefixes"))
    allowed_net = set(string_list(policy, "allowed_non_io_net_imports"))
    forbidden_prefixes = tuple(
        string_list(policy, "forbidden_product_import_prefixes")
    )
    forbidden_tokens = tuple(
        token.casefold()
        for token in string_list(policy, "forbidden_product_import_tokens")
    )

    for import_path in imports:
        if import_path in allowed_net:
            continue
        if project_import_allowed(import_path, allowed_projects):
            continue
        if import_path.startswith("github.com/Project-Helianthus/"):
            raise PolicyError(f"forbidden Helianthus dependency: {import_path}")
        if any(
            import_path == prefix.rstrip("/")
            or import_path.startswith(prefix)
            or import_path.startswith(prefix.rstrip("/") + "/")
            for prefix in forbidden_prefixes
        ):
            raise PolicyError(f"forbidden product I/O dependency: {import_path}")
        lowered = import_path.casefold()
        if any(token in lowered for token in forbidden_tokens):
            raise PolicyError(f"forbidden transport/framing dependency: {import_path}")
        if "." in import_path.split("/", 1)[0]:
            raise PolicyError(f"forbidden external dependency: {import_path}")


def current_files(root: Path) -> list[Path]:
    files: list[Path] = []
    for path in root.rglob("*"):
        relative = path.relative_to(root)
        if ".git" in relative.parts:
            continue
        if path.is_symlink():
            raise PolicyError(f"repository contains a symlink: {relative.as_posix()}")
        if path.is_file():
            files.append(path)
    return sorted(files)


def validate_registry_scope(root: Path, policy: dict[str, object]) -> None:
    if policy["implementation_scope"] != IMPLEMENTATION_SCOPE:
        raise PolicyError("implementation scope must remain public registry semantics")
    forbidden_directories = {
        item.casefold()
        for item in string_list(policy, "forbidden_ownership_directory_names")
    }
    forbidden_stems = {
        item.casefold()
        for item in string_list(policy, "forbidden_product_file_stems")
    }
    for path in current_files(root):
        relative = path.relative_to(root)
        directory_names = {part.casefold() for part in relative.parts[:-1]}
        leaked_directories = directory_names & forbidden_directories
        if leaked_directories:
            raise PolicyError(
                f"registry boundary forbids ownership directory in {relative.as_posix()}: "
                f"{sorted(leaked_directories)}"
            )
        if path.suffix == ".go" and not path.name.endswith("_test.go"):
            stem_tokens = {
                token for token in re.split(r"[^a-z0-9]+", path.stem.casefold()) if token
            }
            leaked_stems = stem_tokens & forbidden_stems
            if leaked_stems:
                raise PolicyError(
                    f"registry boundary forbids product ownership in {relative.as_posix()}: "
                    f"{sorted(leaked_stems)}"
                )


def validate_module(root: Path, policy: dict[str, object]) -> None:
    result = subprocess.run(
        ["go", "mod", "edit", "-json"],
        cwd=root,
        env=offline_go_environment(),
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise PolicyError(f"go.mod inspection failed: {result.stderr.strip()}")
    try:
        module = json.loads(result.stdout)
        module_path = module["Module"]["Path"]
    except (json.JSONDecodeError, KeyError, TypeError) as exc:
        raise PolicyError("go.mod inspection returned an invalid record") from exc
    if module_path != policy["module_path"]:
        raise PolicyError(f"unexpected Go module path: {module_path}")
    requirements = {
        item.get("Path")
        for item in (module.get("Require") or [])
        if isinstance(item, dict)
    }
    required = set(string_list(policy, "required_public_dependencies"))
    if requirements != required:
        raise PolicyError(
            f"Go dependencies differ from the public boundary: {sorted(requirements)}"
        )
    if module.get("Replace"):
        raise PolicyError("Go dependency replacements are forbidden")


def validate_non_go_code(root: Path) -> None:
    forbidden_python_modules = {
        "http.client",
        "pty",
        "requests",
        "serial",
        "socket",
        "urllib.request",
    }
    forbidden_command = re.compile(
        r"(?i)(?:^|[\s;'\"])(?:curl|gh|socat|wget)(?:[\s;'\"]|$)"
        r"|(?:^|[\s;'\"])git\s+(?:clone|fetch|log|pull|rev-list|show)(?:\s|$)"
        r"|/dev/(?:tty|cu)"
    )
    for path in current_files(root):
        if path.suffix not in {".py", ".sh"}:
            continue
        relative = path.relative_to(root).as_posix()
        try:
            source = path.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError) as exc:
            raise PolicyError(f"cannot inspect non-Go code {relative}: {exc}") from exc
        if path.suffix == ".py":
            try:
                tree = ast.parse(source, filename=relative)
            except SyntaxError as exc:
                raise PolicyError(f"non-Go code is not valid Python: {relative}") from exc
            for node in ast.walk(tree):
                modules: list[str] = []
                if isinstance(node, ast.Import):
                    modules = [alias.name for alias in node.names]
                elif isinstance(node, ast.ImportFrom) and node.module is not None:
                    modules = [node.module]
                for module in modules:
                    if any(
                        module == forbidden or module.startswith(forbidden + ".")
                        for forbidden in forbidden_python_modules
                    ):
                        raise PolicyError(
                            f"non-Go code imports forbidden I/O module: {relative}"
                        )
        if forbidden_command.search(source):
            raise PolicyError(
                f"non-Go code contains network, history, or device command: {relative}"
            )


def validate_standard_sources(root: Path, policy: dict[str, object]) -> None:
    standard_root = root / str(policy["standard_root"])
    if not standard_root.exists():
        return
    vendor_identifiers = tuple(
        item.casefold() for item in string_list(policy, "vendor_identifiers")
    )
    for path in current_files(standard_root):
        if path.suffix not in {".go", ".json", ".yaml", ".yml"}:
            continue
        source = path.read_text(encoding="utf-8").casefold()
        for vendor in vendor_identifiers:
            if vendor in source:
                raise PolicyError(
                    f"{path.relative_to(root)} leaks vendor {vendor} into a standard family"
                )
        if "profiles/vendor" in source:
            raise PolicyError(
                f"{path.relative_to(root)} depends on a vendor overlay"
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
                f"{profile_dir.relative_to(root)} lacks valid public evidence: {exc}"
            ) from exc
        if not isinstance(evidence, dict) or set(evidence) != EVIDENCE_KEYS:
            raise PolicyError(
                f"{evidence_path.relative_to(root)} has an invalid evidence shape"
            )
        if evidence["schema"] != EVIDENCE_SCHEMA:
            raise PolicyError(f"{evidence_path.relative_to(root)} has an invalid schema")
        if evidence["profile"] != profile_dir.name:
            raise PolicyError(f"{evidence_path.relative_to(root)} profile mismatch")
        if re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", str(evidence["profile_version"])) is None:
            raise PolicyError(f"{evidence_path.relative_to(root)} version is invalid")
        if evidence["license"] != "CC0-1.0":
            raise PolicyError(f"{evidence_path.relative_to(root)} facts must use CC0-1.0")
        applicability = evidence["applicability"]
        if (
            not isinstance(applicability, list)
            or not applicability
            or any(not isinstance(item, str) or not item.strip() for item in applicability)
        ):
            raise PolicyError(f"{evidence_path.relative_to(root)} lacks applicability")
        sources = evidence["public_sources"]
        if not isinstance(sources, list) or not sources:
            raise PolicyError(f"{evidence_path.relative_to(root)} lacks public sources")
        source_ids: set[str] = set()
        for source in sources:
            if not isinstance(source, dict) or set(source) != SOURCE_KEYS:
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} has invalid source metadata"
                )
            source_id = source["id"]
            if (
                not isinstance(source_id, str)
                or re.fullmatch(r"[a-z0-9][a-z0-9._-]*", source_id) is None
                or source_id in source_ids
            ):
                raise PolicyError(f"{evidence_path.relative_to(root)} source id is invalid")
            source_ids.add(source_id)
            locator = source["locator"]
            parsed = urlparse(locator) if isinstance(locator, str) else None
            if (
                parsed is None
                or parsed.scheme != "https"
                or not parsed.netloc
                or parsed.path in {"", "/"}
            ):
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} source is not publicly addressable"
                )
            if not isinstance(source["license"], str) or not source["license"].strip():
                raise PolicyError(
                    f"{evidence_path.relative_to(root)} source license is absent"
                )


def offline_go_environment() -> dict[str, str]:
    return {
        **os.environ,
        "GOWORK": "off",
        "GOPROXY": "off",
        "GOSUMDB": "off",
    }


def go_imports(root: Path) -> list[str]:
    result = subprocess.run(
        [
            "go",
            "list",
            "-mod=readonly",
            "-f",
            '{{range .Imports}}{{.}}{{"\\n"}}{{end}}',
            "./...",
        ],
        cwd=root,
        env=offline_go_environment(),
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise PolicyError(f"offline go list failed: {result.stderr.strip()}")
    return sorted({line for line in result.stdout.splitlines() if line})


def validate(root: Path) -> None:
    policy = load_policy(root)
    validate_module(root, policy)
    validate_registry_scope(root, policy)
    validate_non_go_code(root)
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
