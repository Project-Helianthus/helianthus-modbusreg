#!/usr/bin/env python3
"""Validate immutable M2-01 documentation and runtime consumer locks."""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path


DOCS_LOCK = {
    "content_revision": 1,
    "contract_id": "HELIANTHUS_MODBUS_FOUNDATION_PROFILE_V1",
    "contract_version": 1,
    "manifest_sha256": (
        "c411e3e8a464e4b9d3a59d3f5a0c82b57e176e24dec9550b9bc0c8b3e4b28c70"
    ),
    "merged_commit_sha": "711a556fee344c6fe7f1ecf3253fcdb3f5f22d06",
    "repository": "Project-Helianthus/helianthus-docs-ebus",
    "schema": "helianthus.modbus.companion-consumer-lock",
    "schema_version": 1,
}

RUNTIME_LOCK = {
    "commit_sha": "4f81cbeb6321e64fa51676ed6e375ce36b60d16d",
    "module": "github.com/Project-Helianthus/helianthus-modbus",
    "module_sum": "h1:c9LpMMmUtYUloYbIKeCrKqoV/vl3KCwtTSDV9WfubjQ=",
    "module_version": "v0.0.0-20260728175116-4f81cbeb6321",
    "schema": "helianthus.modbusreg.runtime-consumer-lock",
    "schema_version": 1,
}


class ConsumerLockError(RuntimeError):
    pass


def load_exact(path: Path, expected: dict[str, object]) -> dict[str, object]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ConsumerLockError(f"cannot load {path}: {exc}") from exc
    if value != expected:
        raise ConsumerLockError(f"{path.name} differs from the authorized lock")
    return value


def command_json(root: Path, command: list[str]) -> dict[str, object]:
    result = subprocess.run(
        command,
        cwd=root,
        env={**os.environ, "GOWORK": "off"},
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise ConsumerLockError(
            f"{' '.join(command)} failed: {result.stderr.strip()}"
        )
    try:
        value = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise ConsumerLockError(f"{' '.join(command)} returned invalid JSON") from exc
    if not isinstance(value, dict):
        raise ConsumerLockError(f"{' '.join(command)} returned a non-object")
    return value


def validate_runtime(root: Path, lock: dict[str, object]) -> None:
    module = str(lock["module"])
    version = str(lock["module_version"])
    listed = command_json(root, ["go", "list", "-m", "-json", module])
    if listed.get("Path") != module or listed.get("Version") != version:
        raise ConsumerLockError("go.mod does not pin the authorized runtime version")
    if listed.get("Indirect") is True:
        raise ConsumerLockError("runtime module must be a direct contract dependency")
    if listed.get("Replace") is not None:
        raise ConsumerLockError("runtime module replacement is forbidden")

    downloaded = command_json(
        root,
        ["go", "mod", "download", "-json", f"{module}@{version}"],
    )
    origin = downloaded.get("Origin")
    if not isinstance(origin, dict):
        raise ConsumerLockError("runtime module lacks VCS origin provenance")
    if (
        downloaded.get("Path") != module
        or downloaded.get("Version") != version
        or downloaded.get("Sum") != lock["module_sum"]
        or origin.get("VCS") != "git"
        or origin.get("URL")
        != "https://github.com/Project-Helianthus/helianthus-modbus"
        or origin.get("Hash") != lock["commit_sha"]
    ):
        raise ConsumerLockError("runtime module provenance differs from M1-04")


def validate(root: Path) -> None:
    policy = root / "policy"
    load_exact(policy / "modbus-companion-consumer-lock-v1.json", DOCS_LOCK)
    runtime = load_exact(
        policy / "modbus-runtime-consumer-lock-v1.json",
        RUNTIME_LOCK,
    )
    validate_runtime(root, runtime)


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    try:
        validate(root)
    except ConsumerLockError as exc:
        print(f"consumer lock validation failed: {exc}", file=sys.stderr)
        return 1
    print("Consumer locks passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
