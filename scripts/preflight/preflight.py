#!/usr/bin/env python3
"""Codec closure and prospective candidate checks for one manifest.

The command reads an existing manifest and the donor checkout. It calls the
published manifest check, then measures a disposable candidate with the
repository's size, behavior-name and lint gates. It does not fetch, publish,
or write a baseline.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

SCRIPTS = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SCRIPTS))

import check_size_ratchet  # noqa: E402


SKIP_NAMES = {".git", ".tmp"}


def install_map(manifest):
    by_name = {}
    for item in manifest.get("symbols") or []:
        if isinstance(item, dict) and item.get("name") and item.get("file"):
            by_name[item["name"]] = item["file"]
    found = {}
    for entry in manifest.get("destinations") or []:
        if not isinstance(entry, dict) or entry.get("kind") != "symbol":
            continue
        donor_file = by_name.get(entry.get("name"))
        destination = entry.get("path")
        if donor_file and destination:
            found.setdefault(destination, donor_file)
    return found


def copy_checkout(checkout, candidate):
    for path in sorted(checkout.iterdir(), key=lambda item: item.name):
        if path.name in SKIP_NAMES:
            continue
        target = candidate / path.name
        if path.is_dir() and not path.is_symlink():
            shutil.copytree(path, target, symlinks=False)
        elif path.is_file():
            shutil.copy2(path, target)


def install_donor(candidate, checkout, manifest, donor):
    for destination, donor_file in install_map(manifest).items():
        if (checkout / destination).exists():
            continue
        target = candidate / destination
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(donor / donor_file, target)


def closure_lines(manifest_path, donor):
    main_py = os.environ.get("QQ06_MAIN_PY", "")
    if not main_py or not Path(main_py).is_file():
        return ["QQ06 check unavailable"]
    completed = subprocess.run(
        [sys.executable, main_py, "check", "--manifest", str(manifest_path), "--donor", str(donor)],
        capture_output=True,
        text=True,
    )
    if completed.returncode == 0:
        return []
    text = completed.stdout.strip() or completed.stderr.strip() or "QQ06 check failed"
    return [line for line in text.splitlines() if line.strip()]


def split_text(row):
    _low, high = check_size_ratchet.limits(row)
    return (
        "split required: "
        + row["path"]
        + " "
        + row["symbol"]
        + " "
        + row["metric"]
        + " "
        + str(row["value"])
        + " > "
        + str(high)
    )


def size_lines(checkout, candidate):
    try:
        baseline_path = checkout / "scripts" / "size-baseline.json"
        baseline = check_size_ratchet.load_baseline(
            json.loads(baseline_path.read_text(encoding="utf-8"))
        )
        rows, _excluded = check_size_ratchet.measure(candidate)
        soft, failures = check_size_ratchet.evaluate(rows, baseline)
    except (OSError, ValueError, json.JSONDecodeError, subprocess.SubprocessError) as exc:
        text = "size ratchet error: " + str(exc)
        return [text], [text]
    output = []
    findings = []
    for row in soft:
        output.append("SOFT " + json.dumps(row, sort_keys=True))
    for row in failures:
        line = "FAIL " + json.dumps(row, sort_keys=True)
        output.append(line)
        findings.append(line)
        if "reason" not in row:
            required = split_text(row)
            output.append(required)
            findings.append(required)
    return output, findings


def behavior_lines(checkout, candidate):
    script = checkout / "scripts" / "check_behavior_names.py"
    baseline = checkout / "scripts" / "behavior-name-baseline.json"
    if not script.is_file():
        script = SCRIPTS / "check_behavior_names.py"
    env = _offline(os.environ.copy())
    completed = subprocess.run(
        [
            sys.executable,
            str(script),
            "--check",
            "--root",
            str(candidate),
            "--baseline",
            str(baseline),
        ],
        capture_output=True,
        text=True,
        env=env,
    )
    return _checker_lines(completed, "behavior-name guard:", "behavior-name scan failed")


def _offline(env):
    env["GOWORK"] = "off"
    env["GOFLAGS"] = "-mod=mod"
    env["GOPROXY"] = "off"
    env["GOSUMDB"] = "off"
    env["GOTOOLCHAIN"] = "local"
    return env


def _drop_scratch(candidate):
    scratch = candidate / ".tmp"
    if scratch.is_dir() and not scratch.is_symlink():
        shutil.rmtree(scratch)


def lint_lines(candidate):
    script = candidate / "scripts" / "check_go_lint_baseline.py"
    if not script.is_file():
        return ["lint baseline failed: checker is absent from the candidate"]
    env = _offline(os.environ.copy())
    completed = subprocess.run(
        [sys.executable, str(script), "--check"],
        cwd=candidate,
        capture_output=True,
        text=True,
        env=env,
    )
    return _lint_output(completed)


def _checker_lines(completed, summary_prefix, fallback):
    lines = []
    for line in completed.stdout.splitlines():
        if line.startswith(summary_prefix) or not line.strip():
            continue
        lines.append(line)
    if completed.returncode and completed.stderr.strip():
        lines.extend(line for line in completed.stderr.splitlines() if line.strip())
    if completed.returncode and not lines:
        lines.append(completed.stderr.strip() or fallback)
    return lines


def _lint_output(completed):
    lines = []
    for line in completed.stdout.splitlines():
        if line.startswith("debt:"):
            lines.append(line)
    for line in completed.stderr.splitlines():
        if line.strip() and not line.startswith("0 diagnostics"):
            lines.append(line)
    if completed.returncode and not lines:
        lines.append(completed.stderr.strip() or completed.stdout.strip() or "lint baseline failed")
    return lines


def _baseline_bytes(checkout):
    names = (
        "size-baseline.json",
        "behavior-name-baseline.json",
        "go-lint-baseline.json",
    )
    return {
        name: (checkout / "scripts" / name).read_bytes()
        for name in names
    }


def run(checkout, manifest_path, donor):
    checkout = Path(checkout)
    manifest_path = Path(manifest_path)
    donor = Path(donor)
    before = _baseline_bytes(checkout)
    manifest_bytes = manifest_path.read_bytes()
    manifest = json.loads(manifest_bytes.decode("utf-8"))
    output = []
    findings = []
    with tempfile.TemporaryDirectory(prefix="preflight-") as temporary:
        candidate = Path(temporary)
        copy_checkout(checkout, candidate)
        install_donor(candidate, checkout, manifest, donor)
        closure = closure_lines(manifest_path, donor)
        output.extend(closure)
        findings.extend(closure)
        size_output, size_findings = size_lines(checkout, candidate)
        _drop_scratch(candidate)
        output.extend(size_output)
        findings.extend(size_findings)
        names = behavior_lines(checkout, candidate)
        output.extend(names)
        findings.extend(names)
        lint = lint_lines(candidate)
        output.extend(lint)
        findings.extend(lint)
    if _baseline_bytes(checkout) != before:
        findings.append("preflight wrote a baseline")
        output.append("preflight wrote a baseline")
    if manifest_path.read_bytes() != manifest_bytes:
        findings.append("preflight wrote the manifest")
        output.append("preflight wrote the manifest")
    for line in output:
        print(line)
    return 1 if findings else 0


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--donor", required=True)
    args = parser.parse_args(argv)
    try:
        return run(Path.cwd(), args.manifest, args.donor)
    except (OSError, ValueError, json.JSONDecodeError, subprocess.SubprocessError) as exc:
        print("preflight error: " + str(exc))
        return 1


if __name__ == "__main__":
    sys.exit(main())
