#!/usr/bin/env python3
"""Parse all Go sources and ratchet exact hard debt; soft debt remains visible.

File lines count physical lines, including comments and blanks. Function spans
run from func through the final token, inclusive, including nested literals.
Counts include declarations, methods and literals. Literal identities use their
enclosing symbol and lexical ordinal, independent of source line numbers.
The baseline records debt; it does not clear the release debt requirement.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

BUDGETS = {
    "production": {"file_lines": (500, 1000), "function_lines": (80, 150), "functions_per_file": (40, 60)},
    "test": {"file_lines": (800, 1500), "function_lines": (120, 200), "functions_per_file": (40, 60)},
}
EXCLUDED = {".git", ".tmp", "vendor", "node_modules", "references"}


def sources(root):
    paths, exclusions = [], []
    for directory, dirs, files in os.walk(root, followlinks=False):
        relative = Path(directory).relative_to(root)
        for name in sorted(dirs[:]):
            path = relative / name
            if name in EXCLUDED or (Path(directory) / name).is_symlink():
                dirs.remove(name)
                if name not in EXCLUDED:
                    raise ValueError(f"unverifiable source directory symlink: {path}")
                exclusions.append(path.as_posix() + "/")
        for name in sorted(files):
            if name.endswith(".go"):
                path = Path(directory) / name
                if path.is_symlink():
                    raise ValueError(f"unverifiable source symlink: {path.relative_to(root)}")
                paths.append((relative / name).as_posix())
    return sorted(paths), sorted(exclusions)


def measure(root):
    paths, exclusions = sources(root)
    if not paths:
        raise ValueError("no Go source files measured")
    scratch = root / ".tmp" / "sizeratchet"
    scratch.mkdir(parents=True, exist_ok=True)
    helper = scratch / "parser.go"
    shutil.copyfile(root / "scripts/sizeratchet/parser.go.txt", helper)
    env = dict(os.environ, GOWORK="off", GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOFLAGS="-buildvcs=false")
    result = subprocess.run(["go", "run", str(helper)], input=json.dumps(paths),
                            text=True, capture_output=True, cwd=root, env=env, timeout=150)
    if result.returncode:
        raise ValueError("Go parser failed: " + result.stderr.strip())
    rows = json.loads(result.stdout)
    return rows, exclusions


def limits(row):
    kind = "test" if row["path"].endswith("_test.go") else "production"
    return BUDGETS[kind][row["metric"]]


def key(row):
    return row["path"], row["symbol"], row["metric"]


def load_baseline(data):
    if not isinstance(data, dict) or set(data) != {"version", "violations"} or type(data["version"]) is not int or data["version"] != 1 or not isinstance(data["violations"], list):
        raise ValueError("invalid baseline envelope")
    seen = set()
    for row in data["violations"]:
        if not isinstance(row, dict) or set(row) != {"path", "symbol", "metric", "value"}:
            raise ValueError("invalid baseline row")
        path, symbol, metric, value = (row[x] for x in ("path", "symbol", "metric", "value"))
        if not isinstance(path, str) or not path.endswith(".go") or path.startswith("/") or any(x in path.split("/") for x in ("", ".", "..")) or "\\" in path or "*" in path or any(x in EXCLUDED for x in path.split("/")):
            raise ValueError("invalid baseline path")
        if not isinstance(symbol, str) or not symbol or "*" in symbol.replace("(*", "(") or not isinstance(metric, str) or metric not in BUDGETS["production"] or type(value) is not int:
            raise ValueError("invalid baseline identity or value")
        if (metric != "function_lines") != (symbol == "$file") or value <= limits(row)[1] or key(row) in seen:
            raise ValueError("baseline must contain unique exact hard violations")
        seen.add(key(row))
    return {key(row): row["value"] for row in data["violations"]}


def evaluate(rows, baseline):
    soft, failures = [], []
    for row in rows:
        low, high = limits(row)
        if row["value"] > low:
            soft.append(row)
        if row["value"] > high and row["value"] > baseline.get(key(row), high):
            failures.append(row)
    return soft, failures


def main():
    root = Path(__file__).resolve().parent.parent
    try:
        baseline = load_baseline(json.loads((root / "scripts/size-baseline.json").read_text()))
        rows, exclusions = measure(root)
        soft, failures = evaluate(rows, baseline)
        for path in exclusions:
            print("EXCLUDED", path)
        for row in soft:
            print("SOFT", json.dumps(row, sort_keys=True))
        for row in failures:
            print("FAIL", json.dumps(row, sort_keys=True))
        print(f"Measured {len({r['path'] for r in rows})} files; {len(soft)} soft violations; {len(failures)} new or growing hard violations")
        return bool(failures)
    except (ValueError, OSError, subprocess.SubprocessError) as exc:
        print(f"size ratchet error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
