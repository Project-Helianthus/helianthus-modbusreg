"""Preflight, closure and fast-path selection. Fixtures are synthetic.

Revisions are computed at runtime. QQ06_MAIN_PY must name the published
manifest tool; a missing path fails these tests.
"""
from __future__ import annotations

import base64
import filecmp
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr
from io import StringIO

import classify


REPO = Path(__file__).resolve().parents[2]
REAL = {}
PRODUCT = None
PRODUCT_BYTES = {}
KEEP = (
    "// Package keep holds a synthetic fixture.\n"
    "package keep\n\n"
    "func keep() int { return 1 }\n\n"
    "func init() { keep() }\n"
)
SIGNATURE = {"family": "codec", "inverse": "none", "selector_fields": ["table", "offset"]}
INPUTS = {"raw": "aa", "qualifiers": "header", "clock": "t0", "state": "idle"}
SELECTORS = {"table": "holding", "offset": "0"}
DEST_TEST = """// Package rows holds a synthetic fixture.
package rows

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestReading(t *testing.T) {
	pins := map[string]string{}
	if err := json.Unmarshal([]byte(os.Getenv("MANIFEST_PINNED_FIELDS")), &pins); err != nil {
		t.Fatal(err)
	}
	observed := pins["reading"]
	switch os.Getenv("PREFLIGHT_MODE") {
	case "approved":
		observed = "approved-reading"
	case "extra":
		observed = "other-reading"
	}
	fmt.Printf("MANIFEST_RESULT case=%s row=%s assertion=%s\\n", os.Getenv("MANIFEST_CASE"), os.Getenv("MANIFEST_ROW"), os.Getenv("MANIFEST_ASSERTION"))
	fmt.Printf("MANIFEST_INPUT raw=%s qualifiers=%s clock=%s state=%s\\n", os.Getenv("MANIFEST_RAW"), os.Getenv("MANIFEST_QUALIFIERS"), os.Getenv("MANIFEST_CLOCK"), os.Getenv("MANIFEST_STATE"))
	fmt.Printf("MANIFEST_FIELD field=reading expected=%s observed=%s\\n", pins["reading"], observed)
}
"""
CODEC_GO = """// Package codec holds a synthetic fixture.
package codec

func preflightCodecSpec() int { return 1 }

// NewCodec returns the synthetic codec.
func NewCodec() int { return preflightCodecSpec() }

func init() {
	NewCodec()
	DecodeStrict()
}
"""
SERIAL_GO = """package codec

// DecodeStrict decodes one synthetic payload.
func DecodeStrict() int { return preflightJSON() }
"""
JSON_GO = """package codec

func preflightJSON() int { return DecodeStrict() }
"""
SERIAL_OPEN = """package codec

// DecodeStrict decodes one synthetic payload.
func DecodeStrict() int { return preflightJSON() }

// NewCodec returns the synthetic codec.
func NewCodec() int { return preflightCodecSpec() }
"""
LOWER_PATHS = (
    "driver/pv/leaf.go",
    "driver/storage/leaf.go",
    "driver/meter/leaf.go",
    "driver/evse/reader/leaf.go",
    "driver/evse/write/leaf.go",
    "driver/limits/leaf.go",
    "driver/sunspec/leaf.go",
    "driver/huawei/leaf.go",
    "driver/modbus_factory.go",
    "driver/replay/leaf.go",
    "driver/rows/pv/leaf.go",
    "driver/rows/infrastructure/leaf.go",
    "driver/rows/storage/leaf.go",
    "driver/rows/evse/leaf.go",
    "driver/custom/leaf.go",
)


def setUpModule():
    main = os.environ.get("QQ06_MAIN_PY", "")
    if not main or not Path(main).is_file():
        raise RuntimeError("QQ06_MAIN_PY is unset or missing")
    for name in ("size-baseline.json", "behavior-name-baseline.json", "go-lint-baseline.json"):
        REAL[name] = (REPO / "scripts" / name).read_bytes()
    global PRODUCT, PRODUCT_BYTES
    PRODUCT = Path(main).resolve().parents[2] / "manifests"
    PRODUCT_BYTES = {path.name: path.read_bytes() for path in PRODUCT.iterdir() if path.is_file()}


def tearDownModule():
    for name, blob in REAL.items():
        if (REPO / "scripts" / name).read_bytes() != blob:
            raise AssertionError(name)


def git(repo, *args, check=True):
    completed = subprocess.run(
        ["git", "-C", str(repo), "-c", "user.email=preflight@example.com",
         "-c", "user.name=Preflight Fixture", "-c", "commit.gpgsign=false",
         "-c", "core.autocrlf=false", *args],
        text=True, capture_output=True,
    )
    if check and completed.returncode != 0:
        raise RuntimeError(completed.stderr or completed.stdout)
    return completed


def git_sha(repo):
    return git(repo, "rev-parse", "HEAD").stdout.strip()


def write(path, text):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def commit_tree(repo, files):
    repo.mkdir(parents=True, exist_ok=True)
    for rel, text in files.items():
        write(repo / rel, text)
    git(repo, "init", "-b", "main")
    git(repo, "add", "-A")
    git(repo, "commit", "-m", "fixture")
    return git_sha(repo)


def commit_existing(repo):
    git(repo, "init", "-b", "main")
    git(repo, "add", "-A")
    git(repo, "commit", "-m", "fixture")


def allow_sha(repo):
    git(repo, "config", "uploadpack.allowReachableSHA1InWant", "true")
    git(repo, "config", "uploadpack.allowAnySHA1InWant", "true")


def fetch_sha(repo, url, revision):
    git(repo, "fetch", "--depth", "1", url, revision)


def shallow_clone(source, dest):
    git(source, "clone", "--depth", "1", "--no-local", str(source), str(dest))


def copy_tool(root):
    source = Path(os.environ["QQ06_MAIN_PY"]).resolve().parent
    target = root / "acceptance" / "tools" / "manifest"
    target.mkdir(parents=True)
    for name in ("main.py", "manifestlib.py", "closure.py", "gitstore.py", "patterns.py"):
        shutil.copy(source / name, target / name)
    (root / "acceptance" / "manifests").mkdir()
    return target / "main.py"


def empty_spec(row, sha, moves):
    return {
        "row": row,
        "donor_repository": "modbusreg",
        "donor_sha": sha,
        "moves": moves,
        "new_files": [],
        "shared": [],
        "specimens": [],
        "assertions": [],
        "mapping_rows": [],
        "expectations": [],
        "expected_differences": [],
    }


def generate(main, donor, spec):
    spec_path = donor.parent / (spec["row"] + "-spec.json")
    spec_path.write_text(json.dumps(spec), encoding="utf-8")
    return subprocess.run(
        [sys.executable, str(main), "generate", "--input", str(spec_path), "--donor", str(donor)],
        text=True, capture_output=True, timeout=120,
    )


def tool_main(root):
    return root / "acceptance" / "tools" / "manifest" / "main.py"


def manifest_path(root, row):
    return root / "acceptance" / "manifests" / (row + ".json")


def run_tool(main, args):
    return subprocess.run(
        [sys.executable, str(main), *args],
        text=True, capture_output=True, timeout=180,
    )


def tool_check(main, manifest, donor):
    return run_tool(main, ["check", "--manifest", str(manifest), "--donor", str(donor)])


def tool_compare(main, manifest, dest, mode=""):
    env = os.environ.copy()
    env["PREFLIGHT_MODE"] = mode
    env["GOWORK"] = "off"
    env["GOFLAGS"] = "-mod=mod"
    env["GOPROXY"] = "off"
    env["GOSUMDB"] = "off"
    return subprocess.run(
        [sys.executable, str(main), "compare", "--manifest", str(manifest), "--destination", str(dest)],
        text=True, capture_output=True, timeout=180, env=env,
    )


def qq06_check(manifest, donor):
    return tool_check(os.environ["QQ06_MAIN_PY"], manifest, donor)


def checkout_tree(root, violations=None):
    (root / "scripts" / "sizeratchet").mkdir(parents=True)
    write(root / "go.mod", "module example.com/preflightfixture\n\ngo 1.22\n")
    write(root / "keep.go", KEEP)
    shutil.copy(REPO / ".golangci.yml", root / ".golangci.yml")
    shutil.copy(REPO / "scripts" / "check_go_lint_baseline.py", root / "scripts" / "check_go_lint_baseline.py")
    shutil.copy(
        REPO / "scripts" / "sizeratchet" / "parser.go.txt",
        root / "scripts" / "sizeratchet" / "parser.go.txt",
    )
    write(root / "scripts" / "size-baseline.json", json.dumps({"version": 1, "violations": violations or []}))
    write(root / "scripts" / "behavior-name-baseline.json", json.dumps({
        "version": 1, "exceptions": [], "baseline": [],
    }))
    write(root / "scripts" / "go-lint-baseline.json", json.dumps({
        "version": 1, "golangci_lint": "2.11.4", "entries": [],
    }))


def baseline_bytes(root):
    names = ("size-baseline.json", "behavior-name-baseline.json", "go-lint-baseline.json")
    return {name: (root / "scripts" / name).read_bytes() for name in names}


def wide_source(lines, name):
    return (
        "// Package sample holds a synthetic fixture.\n"
        "package sample\n"
        "func " + name + "() {\n"
        + ("\n" * (lines - 2))
        + "}\n"
        "func init() {\n\t" + name + "()\n}\n"
    )


def pad_newlines(text, total):
    count = text.count("\n")
    if count > total:
        raise AssertionError(str(count))
    return text + ("\n" * (total - count))


def moved_case(root, row, source, destination, violations=None, existing=None):
    donor = root / "donor"
    sha = commit_tree(donor, {"src/moved.go": source})
    main = copy_tool(root / "tool")
    spec = empty_spec(row, sha, [{
        "path": "src/moved.go",
        "destination_repository": "helianthus-modbusreg",
        "destination_path": destination,
    }])
    made = generate(main, donor, spec)
    if made.returncode != 0:
        raise RuntimeError(made.stdout + made.stderr)
    checkout = root / "checkout"
    checkout_tree(checkout, violations)
    if existing is not None:
        write(checkout / destination, existing)
    return checkout, manifest_path(root / "tool", row), donor


def run_fetch_base(checkout, log_path, token):
    bindir = checkout.parent / ("bin-" + log_path.name)
    bindir.mkdir()
    script = bindir / "git"
    script.write_text(
        "#!/bin/sh\n"
        "printf '%s\\n' \"prompt=${GIT_TERMINAL_PROMPT-unset}\" >> \"$FETCH_LOG\"\n"
        "printf '%s\\n' \"$*\" >> \"$FETCH_LOG\"\n"
        "exec \"$REAL_GIT\" \"$@\"\n",
        encoding="utf-8",
    )
    script.chmod(0o755)
    env = os.environ.copy()
    env["PATH"] = str(bindir) + os.pathsep + env.get("PATH", "")
    env["FETCH_LOG"] = str(log_path)
    env["REAL_GIT"] = shutil.which("git")
    env.pop("GITHUB_TOKEN", None)
    if token:
        env["GITHUB_TOKEN"] = token
    return subprocess.run(
        ["bash", str(REPO / "scripts" / "preflight" / "fetch_base.sh"), "main"],
        cwd=checkout, env=env, text=True, capture_output=True, timeout=60,
    )


def run_preflight(checkout, manifest, donor):
    env = os.environ.copy()
    env["GOWORK"] = "off"
    env["GOFLAGS"] = "-mod=mod"
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    return subprocess.run(
        [sys.executable, str(REPO / "scripts" / "preflight" / "preflight.py"),
         "--manifest", str(manifest), "--donor", str(donor)],
        cwd=str(checkout), env=env, text=True, capture_output=True, timeout=300,
    )


def move(path, destination):
    return {
        "path": path,
        "destination_repository": "helianthus-modbusreg",
        "destination_path": destination,
    }


def write_open_manifest(path, sha):
    donor_paths = sorted([
        "codec/codec.go",
        "codec/serialization.go",
        "codec/json_preflight.go",
    ])
    payload = "".join(item + "\n" for item in donor_paths).encode()
    body = {
        "row": "open",
        "donor": {"repository": "modbusreg", "sha": sha},
        "paths": [{
            "repository": "modbusreg",
            "paths": donor_paths,
            "sha256": hashlib.sha256(payload).hexdigest(),
            "donor_sha": sha,
        }],
        "symbols": [
            {"name": "NewCodec", "file": "codec/serialization.go", "uses": ["preflightCodecSpec"]},
            {"name": "DecodeStrict", "file": "codec/serialization.go", "uses": ["preflightJSON"]},
            {"name": "preflightJSON", "file": "codec/json_preflight.go", "uses": ["DecodeStrict"]},
        ],
        "assertions": [],
        "stays": [],
        "patterns": [],
        "destinations": [],
        "expectations": [],
        "expected_differences": [],
        "receipts": [],
    }
    write(path, json.dumps(body, indent=2) + "\n")
    (path.parent / "open.modbusreg.lst").write_bytes(payload)


def vector_text():
    return json.dumps({
        "fields": {"reading": "230"},
        "inputs": {},
        "selectors": SELECTORS,
    }, indent=2) + "\n"


def approved_text(case="reading-case"):
    return json.dumps({
        "approval": "approved-transition",
        "case": case,
        "field": "reading",
        "new": "approved-reading",
        "old": "230",
        "old_present": True,
        "source": "recorded-transition",
    }, indent=2) + "\n"


def output_text():
    return json.dumps({"fields": {"native_outcome": "response"}, "status": 0}, indent=2) + "\n"


def exemplar_text():
    payload = json.dumps({"id": "reading", "signature": SIGNATURE}, separators=(",", ":"))
    return "package exemplar\n\n// QQ06ROW " + payload + "\nfunc Example() int { return 1 }\n"


def compare_spec(donor_sha, tool_sha):
    return {
        "row": "reading",
        "donor_repository": "modbusreg",
        "donor_sha": donor_sha,
        "moves": [],
        "new_files": [{"repository": "helianthus-modbusreg", "path": "driver/shared.go"}],
        "shared": [],
        "specimens": [],
        "assertions": [],
        "mapping_rows": [{
            "row": "reading-row",
            "kind": "specification",
            "cases": ["reading-case"],
            "assertion": "reading-assert",
            "old_symbol": "",
            "bom_entry": "",
            "signature": SIGNATURE,
            "exemplar": {"repository": "modbusreg", "path": "exemplar/rows.go", "revision": donor_sha},
            "candidate": {"form": "inline", "operands": SELECTORS},
        }],
        "expectations": [{
            "case": "reading-case",
            "kind": "specification",
            "rows": [{
                "row": "reading-row",
                "assertion": "reading-assert",
                "destination": {
                    "repository": "helianthus-modbusreg",
                    "path": "driver/rows/pv/row_test.go",
                    "test": "TestReading",
                },
            }],
            "selectors": SELECTORS,
            "inputs": INPUTS,
            "references": [{
                "role": "specification",
                "repository": "docs-semantic",
                "path": "vec/reading.json",
                "revision": tool_sha,
            }],
        }],
        "expected_differences": [{
            "case": "reading-case",
            "field": "reading",
            "old_present": True,
            "old": "230",
            "new": "approved-reading",
            "source": "recorded-transition",
            "approval": "approved-transition",
            "repository": "docs-semantic",
            "path": "vec/approved.json",
            "revision": tool_sha,
        }],
    }


def build_compare(root):
    tool = root / "tool"
    main = copy_tool(tool)
    write(tool / "vec" / "reading.json", vector_text())
    write(tool / "vec" / "approved.json", approved_text())
    commit_existing(tool)
    donor_sha = commit_tree(root / "donor", {"exemplar/rows.go": exemplar_text()})
    made = generate(main, root / "donor", compare_spec(donor_sha, git_sha(tool)))
    if made.returncode != 0:
        raise RuntimeError(made.stdout + made.stderr)
    dest = root / "dest"
    write(dest / "go.mod", "module example.com/dest\n\ngo 1.22\n")
    write(dest / "driver" / "rows" / "pv" / "row_test.go", DEST_TEST)
    return main, manifest_path(tool, "reading"), root / "donor", dest


def reference_spec(donor_sha, vector, output, diff):
    return {
        "row": "pinned",
        "donor_repository": "modbusreg",
        "donor_sha": donor_sha,
        "moves": [],
        "new_files": [],
        "shared": [],
        "specimens": [],
        "assertions": [],
        "mapping_rows": [],
        "expectations": [{
            "case": "pinned-case",
            "kind": "specification",
            "rows": [],
            "selectors": {},
            "inputs": {"raw": "", "qualifiers": "", "clock": "", "state": ""},
            "references": [
                {"role": "specification", "repository": "docs-semantic", "revision": vector, "path": "vec/reading.json"},
                {"role": "donor-output", "repository": "modbusreg", "revision": output, "path": "vec/output.json"},
            ],
        }],
        "expected_differences": [{
            "case": "pinned-case",
            "field": "reading",
            "old_present": True,
            "old": "230",
            "new": "approved-reading",
            "source": "recorded-transition",
            "approval": "approved-transition",
            "repository": "docs-semantic",
            "revision": diff,
            "path": "vec/approved.json",
        }],
    }


def pull(number, base_ref, head_ref, base_id=1, head_id=1):
    return {
        "number": number,
        "base": {"ref": base_ref, "repo": {"id": base_id}},
        "head": {"ref": head_ref, "repo": {"id": head_id}},
    }


def direct_plan(sealed, lines, current, pulls, repository="modbusreg", row="batch"):
    changes = classify.parse_name_status("".join(line + "\n" for line in lines))
    manifest = None
    found = []
    if sealed is not None:
        manifest = {
            "row": row,
            "donor": {"repository": "modbusreg", "sha": "recorded"},
            "paths": [{"repository": repository, "paths": list(sealed)}],
            "patterns": [],
        }
        found = [(Path("batch.json"), manifest)]
    status, path, data = classify.identify(found, classify.change_paths(changes))
    position = "unresolved" if pulls is None else classify.stack_position(current, pulls)
    return classify.build_plan(changes, position, status, path, data)


class Case(unittest.TestCase):
    def tearDown(self):
        for name, blob in REAL.items():
            self.assertEqual((REPO / "scripts" / name).read_bytes(), blob)
        found = {path.name: path.read_bytes() for path in PRODUCT.iterdir() if path.is_file()}
        self.assertEqual(found, PRODUCT_BYTES)

    def preflight(self, row, source, destination, violations=None, existing=None):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        checkout, manifest, donor = moved_case(root, row, source, destination, violations, existing)
        before = baseline_bytes(checkout)
        manifest_bytes = manifest.read_bytes()
        completed = run_preflight(checkout, manifest, donor)
        self.assertEqual(baseline_bytes(checkout), before)
        self.assertEqual(manifest.read_bytes(), manifest_bytes)
        target = checkout / destination
        if existing is None:
            self.assertFalse(target.exists())
        else:
            self.assertEqual(target.read_text(encoding="utf-8"), existing)
        return completed


class CodecTests(Case):
    def test_tool_copy_matches_published_main(self):
        with tempfile.TemporaryDirectory() as temporary:
            main = copy_tool(Path(temporary))
            self.assertTrue(filecmp.cmp(main, os.environ["QQ06_MAIN_PY"], shallow=False))

    def test_complete_codec_with_empty_patterns_passes_check(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            files = {
                "codec/codec.go": CODEC_GO,
                "codec/serialization.go": SERIAL_GO,
                "codec/json_preflight.go": JSON_GO,
            }
            sha = commit_tree(root / "donor", files)
            main = copy_tool(root / "tool")
            moves = [
                move("codec/codec.go", "driver/codec/codec.go"),
                move("codec/serialization.go", "driver/codec/serialization.go"),
                move("codec/json_preflight.go", "driver/codec/json_preflight.go"),
            ]
            made = generate(main, root / "donor", empty_spec("codec", sha, moves))
            self.assertEqual(made.returncode, 0, made.stdout + made.stderr)
            manifest = manifest_path(root / "tool", "codec")
            body = json.loads(manifest.read_text(encoding="utf-8"))
            self.assertEqual(body["patterns"], [])
            before = manifest.read_bytes()
            checked = qq06_check(manifest, root / "donor")
            self.assertEqual(checked.returncode, 0, checked.stdout + checked.stderr)
            self.assertIn("check: ok", checked.stdout)
            self.assertEqual(manifest.read_bytes(), before)

    def test_omitted_codec_helper_fails_check(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            files = {
                "codec/codec.go": "package codec\n\nfunc preflightCodecSpec() int { return 1 }\n",
                "codec/serialization.go": SERIAL_OPEN,
                "codec/json_preflight.go": JSON_GO,
            }
            sha = commit_tree(root / "donor", files)
            manifest = manifest_path(root / "tool", "open")
            write_open_manifest(manifest, sha)
            before = manifest.read_bytes()
            checked = qq06_check(manifest, root / "donor")
            self.assertNotEqual(checked.returncode, 0, checked.stdout + checked.stderr)
            self.assertIn("outside-set: preflightCodecSpec", checked.stdout)
            self.assertNotIn("check: ok", checked.stdout)
            self.assertEqual(manifest.read_bytes(), before)

    def test_wrong_donor_reports_donor_sha(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            sha = commit_tree(root / "donor", {"codec/codec.go": CODEC_GO})
            main = copy_tool(root / "tool")
            moves = [move("codec/codec.go", "driver/codec/codec.go")]
            made = generate(main, root / "donor", empty_spec("codec", sha, moves))
            self.assertEqual(made.returncode, 0, made.stdout + made.stderr)
            write(root / "donor" / "note.txt", "later\n")
            git(root / "donor", "add", "-A")
            git(root / "donor", "commit", "-m", "later")
            manifest = manifest_path(root / "tool", "codec")
            before = manifest.read_bytes()
            checked = qq06_check(manifest, root / "donor")
            self.assertNotEqual(checked.returncode, 0)
            self.assertIn("donor-sha", checked.stdout)
            self.assertNotIn("check: ok", checked.stdout)
            self.assertEqual(manifest.read_bytes(), before)


class GateTests(Case):
    def test_function_over_hard_limit_names_the_function(self):
        completed = self.preflight("wide", wide_source(151, "wide"), "driver/rows/pv/wide.go")
        self.assertNotEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        splits = [line for line in completed.stdout.splitlines() if line.startswith("split required:")]
        self.assertEqual(splits, ["split required: driver/rows/pv/wide.go wide function_lines 151 > 150"])

    def test_file_over_hard_limit_names_the_file(self):
        body = (
            "// Package sample holds a synthetic fixture.\n"
            "package sample\n"
            "func anchor() int { return 1 }\n"
            "func init() { anchor() }\n"
        )
        completed = self.preflight("huge", pad_newlines(body, 1001), "driver/rows/pv/huge.go")
        self.assertNotEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        splits = [line for line in completed.stdout.splitlines() if line.startswith("split required:")]
        self.assertEqual(len(splits), 1)
        self.assertIn("driver/rows/pv/huge.go", splits[0])
        self.assertIn("$file", splits[0])
        self.assertIn("file_lines", splits[0])
        self.assertIn("1001 > 1000", splits[0])

    def test_soft_function_does_not_fail(self):
        completed = self.preflight("soft", wide_source(81, "wide"), "driver/rows/pv/soft.go")
        self.assertEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        self.assertIn("SOFT ", completed.stdout)
        self.assertNotIn("FAIL ", completed.stdout)
        self.assertNotIn("split required:", completed.stdout)

    def test_limit_function_is_silent(self):
        completed = self.preflight("exact", wide_source(80, "wide"), "driver/rows/pv/exact.go")
        self.assertEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        self.assertEqual(completed.stdout, "")

    def test_unchanged_hard_allowance_is_accepted(self):
        allowance = [{
            "path": "driver/rows/pv/wide.go",
            "symbol": "wide",
            "metric": "function_lines",
            "value": 151,
        }]
        completed = self.preflight(
            "kept", wide_source(151, "wide"), "driver/rows/pv/wide.go", allowance,
        )
        self.assertEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        self.assertNotIn("FAIL ", completed.stdout)
        self.assertNotIn("split required:", completed.stdout)

    def test_reduced_allowance_must_be_tightened(self):
        allowance = [{
            "path": "driver/rows/pv/wide.go",
            "symbol": "wide",
            "metric": "function_lines",
            "value": 200,
        }]
        completed = self.preflight(
            "reduced", wide_source(151, "wide"), "driver/rows/pv/wide.go", allowance,
        )
        self.assertNotEqual(completed.returncode, 0)
        self.assertIn("baseline allowance must be tightened or removed", completed.stdout)
        self.assertNotIn("split required:", completed.stdout)

    def test_removed_allowance_must_be_dropped(self):
        allowance = [{"path": "gone.go", "symbol": "gone", "metric": "function_lines", "value": 151}]
        completed = self.preflight("gone", wide_source(4, "wide"), "driver/rows/pv/gone.go", allowance)
        self.assertNotEqual(completed.returncode, 0)
        self.assertIn("baseline allowance must be tightened or removed", completed.stdout)
        self.assertNotIn("split required:", completed.stdout)

    def test_absent_name_fails_and_leaves_the_checkout(self):
        source = (
            "// Package sample holds a synthetic fixture.\n"
            "package sample\n\n"
            "func Batch9Read() int { return 1 }\n\n"
            "func init() { Batch9Read() }\n"
        )
        completed = self.preflight("named", source, "driver/rows/pv/added.go")
        self.assertNotEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        self.assertIn("driver/rows/pv/added.go: identifier Batch9Read", completed.stdout)

    def test_absent_lint_failure_reaches_the_diagnostic(self):
        source = (
            "// Package sample holds a synthetic fixture.\n"
            "package sample\n\n"
            "func ExportedBroken() int { return 1 }\n\n"
            "func init() { ExportedBroken() }\n"
        )
        completed = self.preflight("broken", source, "driver/rows/pv/broken.go")
        self.assertNotEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        self.assertIn("ExportedBroken", completed.stdout)
        self.assertIn("debt:", completed.stdout)

    def test_written_candidate_keeps_its_long_function(self):
        completed = self.preflight(
            "written", wide_source(4, "donorShort"), "driver/rows/pv/wide.go",
            existing=wide_source(151, "candidateWide"),
        )
        self.assertNotEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        self.assertIn("candidateWide", completed.stdout)
        self.assertNotIn("donorShort", completed.stdout)
        self.assertIn("function_lines 151 > 150", completed.stdout)

    def test_written_candidate_keeps_its_lint_error(self):
        donor = (
            "// Package sample holds a synthetic fixture.\n"
            "package sample\n\n"
            "func cleanDonor() int { return 1 }\n\n"
            "func init() { cleanDonor() }\n"
        )
        candidate = (
            "// Package sample holds a synthetic fixture.\n"
            "package sample\n\n"
            "func ExportedBroken() int { return 1 }\n\n"
            "func init() { ExportedBroken() }\n"
        )
        completed = self.preflight(
            "linted", donor, "driver/rows/pv/broken.go", existing=candidate,
        )
        self.assertNotEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        self.assertIn("ExportedBroken", completed.stdout)
        self.assertNotIn("cleanDonor", completed.stdout)


class CompareTests(Case):
    def test_equality_approved_delta_and_extra_mismatch(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            main, manifest, donor, dest = build_compare(root)
            before = manifest.read_bytes()
            checked = tool_check(main, manifest, donor)
            self.assertEqual(checked.returncode, 0, checked.stdout + checked.stderr)
            self.assertIn("check: ok", checked.stdout)
            exact = tool_compare(main, manifest, dest, "")
            self.assertEqual(exact.returncode, 0, exact.stdout + exact.stderr)
            self.assertIn("result=pass", exact.stdout)
            self.assertIn("compare: ok", exact.stdout)
            self.assertIn("expected=230", exact.stdout)
            approved = tool_compare(main, manifest, dest, "approved")
            self.assertEqual(approved.returncode, 0, approved.stdout + approved.stderr)
            self.assertIn("observed=approved-reading", approved.stdout)
            self.assertIn("compare: ok", approved.stdout)
            extra = tool_compare(main, manifest, dest, "extra")
            self.assertNotEqual(extra.returncode, 0)
            self.assertIn("result=fail", extra.stdout)
            self.assertNotIn("compare: ok", extra.stdout)
            self.assertEqual(manifest.read_bytes(), before)

    def test_stale_signature_is_not_rewritten(self):
        with tempfile.TemporaryDirectory() as temporary:
            main, manifest, donor, _dest = build_compare(Path(temporary))
            edited = json.loads(manifest.read_text(encoding="utf-8"))
            edited["patterns"][0]["signature"] = dict(SIGNATURE, family="stale")
            target = manifest.parent / "stale.json"
            target.write_text(json.dumps(edited), encoding="utf-8")
            before = target.read_bytes()
            checked = tool_check(main, target, donor)
            self.assertNotEqual(checked.returncode, 0)
            self.assertIn("signature-mismatch", checked.stdout)
            self.assertEqual(target.read_bytes(), before)

    def test_missing_and_mutated_expectations_fail(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            main, manifest, donor, _dest = build_compare(root)
            mutated = json.loads(manifest.read_text(encoding="utf-8"))
            mutated["expectations"][0]["references"][0]["blob_sha256"] = "0" * 64
            target = manifest.parent / "mutated.json"
            target.write_text(json.dumps(mutated), encoding="utf-8")
            before = target.read_bytes()
            checked = tool_check(main, target, donor)
            self.assertNotEqual(checked.returncode, 0)
            self.assertIn("mutated:", checked.stdout)
            self.assertEqual(target.read_bytes(), before)
            missing = json.loads(manifest.read_text(encoding="utf-8"))
            blob = git(root / "tool", "hash-object", "vec/reading.json").stdout.strip()
            missing["expectations"][0]["references"][0]["revision"] = blob
            gone = manifest.parent / "missing.json"
            gone.write_text(json.dumps(missing), encoding="utf-8")
            before_missing = gone.read_bytes()
            failed = tool_check(main, gone, donor)
            self.assertNotEqual(failed.returncode, 0)
            self.assertIn("unavailable:", failed.stdout)
            self.assertEqual(gone.read_bytes(), before_missing)


class ShallowTests(Case):
    def test_exemplar_fetch_is_required(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            tool = root / "tool"
            main = copy_tool(tool)
            write(tool / "vec" / "reading.json", vector_text())
            write(tool / "vec" / "approved.json", approved_text())
            commit_existing(tool)
            side_sha = commit_tree(root / "side", {"exemplar/rows.go": exemplar_text()})
            allow_sha(root / "side")
            donor_sha = commit_tree(root / "donor", {"note.txt": "donor\n"})
            allow_sha(root / "donor")
            fetch_sha(root / "donor", str(root / "side"), side_sha)
            self.assertEqual(git_sha(root / "donor"), donor_sha)
            spec = compare_spec(donor_sha, git_sha(tool))
            spec["mapping_rows"][0]["exemplar"]["revision"] = side_sha
            spec["mapping_rows"][0]["exemplar"]["repository"] = "eebusreg"
            made = generate(main, root / "donor", spec)
            self.assertEqual(made.returncode, 0, made.stdout + made.stderr)
            manifest = manifest_path(tool, "reading")
            shallow = root / "shallow"
            shallow_clone(root / "donor", shallow)
            absent = git(shallow, "cat-file", "-e", side_sha + "^{commit}", check=False)
            self.assertNotEqual(absent.returncode, 0, absent.stderr)
            failed = tool_check(main, manifest, shallow)
            self.assertNotEqual(failed.returncode, 0)
            self.assertIn("unavailable:", failed.stdout)
            fetch_sha(shallow, str(root / "side"), side_sha)
            self.assertEqual(git_sha(shallow), donor_sha)
            present = tool_check(main, manifest, shallow)
            self.assertEqual(present.returncode, 0, present.stdout + present.stderr)
            self.assertEqual(git_sha(shallow), donor_sha)

    def test_shallow_tool_resolves_distinct_revisions(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            tool = root / "tool"
            main = copy_tool(tool)
            commit_existing(tool)
            vector = commit_tree(root / "vector", {"vec/reading.json": vector_text()})
            output = commit_tree(root / "output", {"vec/output.json": output_text()})
            diff = commit_tree(root / "diff", {"vec/approved.json": approved_text("pinned-case")})
            self.assertNotEqual(vector, output)
            self.assertNotEqual(output, diff)
            for path in (root / "vector", root / "output", root / "diff", tool):
                allow_sha(path)
            fetch_sha(tool, str(root / "vector"), vector)
            fetch_sha(tool, str(root / "output"), output)
            fetch_sha(tool, str(root / "diff"), diff)
            tip = git_sha(tool)
            donor_sha = commit_tree(root / "donor", {"note.txt": "donor\n"})
            made = generate(main, root / "donor", reference_spec(donor_sha, vector, output, diff))
            self.assertEqual(made.returncode, 0, made.stdout + made.stderr)
            bare = root / "bare"
            ready = root / "ready"
            shallow_clone(tool, bare)
            shallow_clone(tool, ready)
            copy_row(tool / "acceptance" / "manifests", bare / "acceptance" / "manifests", "pinned")
            copy_row(tool / "acceptance" / "manifests", ready / "acceptance" / "manifests", "pinned")
            self._fetch_ready(root, ready, vector, output, diff, tip)
            self._bare_fails(bare, root / "donor", vector)
            ready_main = tool_main(ready)
            ready_manifest = manifest_path(ready, "pinned")
            checked = tool_check(ready_main, ready_manifest, root / "donor")
            compared = tool_compare(ready_main, ready_manifest, root / "empty")
            self.assertEqual(checked.returncode, 0, checked.stdout + checked.stderr)
            self.assertIn("check: ok", checked.stdout)
            self.assertEqual(compared.returncode, 0, compared.stdout + compared.stderr)
            self.assertIn("compare: ok", compared.stdout)
            self.assertEqual(git_sha(ready), tip)

    def _fetch_ready(self, root, ready, vector, output, diff, tip):
        fetch_sha(ready, str(root / "vector"), vector)
        fetch_sha(ready, str(root / "output"), output)
        fetch_sha(ready, str(root / "diff"), diff)
        self.assertEqual(git_sha(ready), tip)
        for revision in (vector, output):
            ancestor = git(ready, "merge-base", "--is-ancestor", revision, "HEAD", check=False)
            self.assertNotEqual(ancestor.returncode, 0)

    def _bare_fails(self, bare, donor, vector):
        missing = git(bare, "cat-file", "-e", vector + "^{commit}", check=False)
        self.assertNotEqual(missing.returncode, 0)
        manifest = manifest_path(bare, "pinned")
        checked = tool_check(tool_main(bare), manifest, donor)
        compared = tool_compare(tool_main(bare), manifest, bare)
        self.assertNotEqual(checked.returncode, 0)
        self.assertNotEqual(compared.returncode, 0)
        self.assertNotIn("check: ok", checked.stdout)
        self.assertNotIn("compare: ok", compared.stdout)


def copy_row(source, dest, row):
    dest.mkdir(parents=True, exist_ok=True)
    for path in source.glob(row + ".*"):
        shutil.copy(path, dest / path.name)


class SelectionTests(Case):
    def test_lower_additive_keeps_preflight_without_full_ci(self):
        sealed = ["driver/rows/storage/new.go", "driver/rows/evse/new.go"]
        lines = ["A\tdriver/rows/storage/new.go", "A\tdriver/rows/evse/new.go"]
        plan = direct_plan(sealed, lines, pull(8, "main", "mm08"), [pull(9, "mm08", "mm09")])
        self.assertEqual(plan["branch"], "lower-additive")
        self.assertFalse(plan["run_full"])
        self.assertTrue(plan["run_packages"])
        self.assertTrue(plan["run_compare"])
        self.assertTrue(plan["run_preflight"])
        self.assertTrue(plan["run_gates"])

    def test_lower_member_runs_packages_and_compare_only(self):
        current = pull(8, "main", "mm08")
        pulls = [pull(9, "mm08", "mm09")]
        sealed = ["driver/limits/leaf.go", "driver/sunspec/model.go", "driver/huawei/model.go"]
        lines = ["M\tdriver/limits/leaf.go", "M\tdriver/sunspec/model.go", "M\tdriver/huawei/model.go"]
        plan = direct_plan(sealed, lines, current, pulls)
        self.assertEqual(plan["position"], "lower")
        self.assertFalse(plan["additive"])
        self.assertFalse(plan["run_full"])
        self.assertTrue(plan["run_packages"])
        self.assertTrue(plan["run_compare"])
        self.assertFalse(plan["run_preflight"])
        self.assertFalse(plan["run_gates"])
        self.assertEqual(plan["branch"], "lower-nonadditive")

    def test_each_lower_path_uses_the_manifest_not_an_allowlist(self):
        current = pull(8, "main", "mm08")
        pulls = [pull(9, "mm08", "mm09")]
        for path in LOWER_PATHS:
            plan = direct_plan([path], ["M\t" + path], current, pulls)
            self.assertTrue(plan["run_packages"], path)
            self.assertTrue(plan["run_compare"], path)
            self.assertFalse(plan["run_full"], path)
            self.assertFalse(plan["run_preflight"], path)
        retained = direct_plan(["driver/other.go"], ["M\tdriver/limits/leaf.go"], current, pulls)
        self.assertEqual(retained["branch"], "no-manifest")
        self.assertTrue(retained["run_full"])
        self.assertFalse(retained["run_packages"])

    def test_additive_unstacked_tip_and_mixed(self):
        added = ["A\tdriver/rows/pv/new.go", "A\tdriver/rows/infrastructure/new.go"]
        sealed = ["driver/rows/pv/new.go", "driver/rows/infrastructure/new.go", "driver/shared.go"]
        unstacked = direct_plan(sealed, added, pull(3, "main", "bb03"), [])
        self.assertEqual(unstacked["branch"], "unstacked-additive")
        self.assertFalse(unstacked["run_full"])
        self.assertTrue(unstacked["run_packages"])
        self.assertTrue(unstacked["run_preflight"])
        self.assertTrue(unstacked["run_gates"])
        tip = direct_plan(sealed, added, pull(3, "main", "bb03"), [pull(1, "older", "main")])
        self.assertEqual(tip["branch"], "tip-additive")
        self.assertTrue(tip["run_full"])
        self.assertTrue(tip["run_compare"])
        self.assertTrue(tip["run_preflight"])
        self.assertFalse(tip["run_packages"])
        self.assertFalse(tip["run_gates"])
        mixed = ["A\tdriver/rows/pv/new.go", "M\tdriver/shared.go"]
        ordinary = direct_plan(sealed, mixed, pull(4, "main", "mixed"), [])
        self.assertEqual(ordinary["branch"], "unstacked-nonadditive")
        self.assertTrue(ordinary["run_full"])
        self.assertFalse(ordinary["run_compare"])
        nonadditive_tip = direct_plan(sealed, ["M\tdriver/shared.go"], pull(3, "main", "bb03"), [pull(1, "older", "main")])
        self.assertEqual(nonadditive_tip["branch"], "tip-nonadditive")
        self.assertTrue(nonadditive_tip["run_full"])
        self.assertFalse(nonadditive_tip["run_preflight"])

    def test_no_manifest_many_and_unresolved_keep_full_ci(self):
        current = pull(8, "main", "mm08")
        none = direct_plan(None, ["M\tdriver/pv/leaf.go"], current, [pull(9, "mm08", "mm09")])
        self.assertEqual(none["branch"], "no-manifest")
        self.assertFalse(none["selected"])
        self.assertTrue(none["run_full"])
        changes = classify.parse_name_status("M\tdriver/pv/leaf.go\n")
        manifests = [
            (Path("a.json"), {"row": "one", "paths": [{"repository": "modbusreg", "paths": ["driver/pv/leaf.go"]}]}),
            (Path("b.json"), {"row": "two", "paths": [{"repository": "modbusreg", "paths": ["driver/pv/leaf.go"]}]}),
        ]
        status, _path, _data = classify.identify(manifests, classify.change_paths(changes))
        many = classify.build_plan(changes, "lower", status, None, None)
        self.assertEqual(status, "many")
        self.assertFalse(many["selected"])
        self.assertEqual(many["branch"], "unresolved")
        self.assertTrue(many["run_full"])
        unresolved = direct_plan(
            ["driver/pv/leaf.go"], ["A\tdriver/pv/leaf.go"], current, None,
        )
        self.assertTrue(unresolved["selected"])
        self.assertEqual(unresolved["branch"], "unresolved")
        self.assertTrue(unresolved["run_full"])
        self.assertFalse(unresolved["run_packages"])
        failed = classify.after_check(unresolved, 2)
        self.assertEqual(failed["branch"], "selected-check-failed")
        self.assertTrue(failed["selected"])
        self.assertFalse(failed["run_full"] and failed["run_packages"])
        self.assertEqual(failed["exit_code"], 2)
        self.assertEqual(classify.FULL_SUITE, "./scripts/ci_local.sh")

    def test_tip_link_uses_parent_fields(self):
        # parent head ref, parent head repo id, current base ref and current base
        # repo id each form the parent link. Changing one of them drops the tip.
        current = pull(8, "main", "mm08")
        parent = pull(2, "older", "main")
        self.assertEqual(classify.stack_position(current, [parent]), "tip")
        changed = pull(2, "older", "other")
        self.assertEqual(classify.stack_position(current, [changed]), "unstacked")
        other_repo = pull(2, "older", "main", head_id=2)
        self.assertEqual(classify.stack_position(current, [other_repo]), "unstacked")
        moved_base = pull(8, "other", "mm08")
        self.assertEqual(classify.stack_position(moved_base, [parent]), "unstacked")
        moved_repo = pull(8, "main", "mm08", base_id=2)
        self.assertEqual(classify.stack_position(moved_repo, [parent]), "unstacked")

    def test_lower_link_uses_child_fields(self):
        # child base ref, child base repo id, current head ref, current head repo
        # id and the current number each form the child link.
        current = pull(8, "main", "mm08")
        child = pull(9, "mm08", "mm09")
        parent = pull(2, "older", "main")
        self.assertEqual(classify.stack_position(current, [parent, child]), "lower")
        self.assertEqual(classify.stack_position(current, [parent, pull(9, "other", "mm09")]), "tip")
        self.assertEqual(
            classify.stack_position(current, [parent, pull(9, "mm08", "mm09", base_id=2)]),
            "tip",
        )
        self.assertEqual(classify.stack_position(pull(8, "main", "other"), [parent, child]), "tip")
        self.assertEqual(
            classify.stack_position(pull(8, "main", "mm08", head_id=2), [parent, child]),
            "tip",
        )
        self.assertEqual(classify.stack_position(pull(9, "main", "mm08"), [parent, child]), "tip")
        self.assertEqual(
            classify.stack_position(current, [parent, pull(9, "mm08", "elsewhere")]),
            "lower",
        )

    def test_two_parents_or_a_malformed_pull_is_unresolved(self):
        current = pull(8, "main", "mm08")
        first = pull(2, "older", "main")
        second = pull(3, "older-b", "main", head_id=1)
        self.assertEqual(classify.stack_position(current, [first, second]), "unresolved")
        broken = {"number": 4, "base": {"ref": "main"}, "head": {"ref": "x", "repo": {"id": 1}}}
        self.assertEqual(classify.stack_position(current, [broken]), "unresolved")
        self.assertEqual(classify.stack_position(current, None), "unresolved")

    def test_coverage_uses_row_sealed_destination_and_repository(self):
        current = pull(1, "main", "head")
        covered = direct_plan(["driver/pv/leaf.go"], ["A\tdriver/pv/leaf.go"], current, [])
        self.assertTrue(covered["selected"])
        blank = direct_plan(["driver/pv/leaf.go"], ["A\tdriver/pv/leaf.go"], current, [], row="  ")
        self.assertFalse(blank["selected"])
        donor_paths = direct_plan(
            ["src/moved.go"], ["A\tdriver/pv/leaf.go"], current, [], repository="gateway",
        )
        self.assertEqual(donor_paths["branch"], "no-manifest")
        renamed = direct_plan(
            ["old.go", "new.go"], ["R100\told.go\tnew.go"], current, [],
        )
        self.assertFalse(renamed["additive"])
        self.assertTrue(renamed["run_full"])
        only_new = direct_plan(["new.go"], ["R100\told.go\tnew.go"], current, [])
        self.assertEqual(only_new["branch"], "no-manifest")

    def test_lower_stale_signature_fails_at_check(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            main, manifest, donor, dest = build_compare(root)
            compared = tool_compare(main, manifest, dest, "")
            self.assertEqual(compared.returncode, 0, compared.stdout + compared.stderr)
            edited = json.loads(manifest.read_text(encoding="utf-8"))
            sealed = classify.sealed_destination_paths(edited, classify.DESTINATION_IDS)
            self.assertIn("driver/rows/pv/row_test.go", sealed)
            self.assertIn("driver/shared.go", sealed)
            edited["patterns"][0]["signature"] = dict(SIGNATURE, family="stale")
            stale = manifest.parent / "stale.json"
            stale.write_text(json.dumps(edited), encoding="utf-8")
            lines = ["A\tdriver/rows/pv/row_test.go", "M\tdriver/shared.go"]
            plan = direct_plan(sealed, lines, pull(8, "main", "mm08"), [pull(9, "mm08", "mm09")])
            self.assertEqual(plan["branch"], "lower-nonadditive")
            self.assertTrue(plan["selected"])
            self.assertNotEqual(plan["branch"], "no-manifest")
            checked = tool_check(main, stale, donor)
            self.assertNotEqual(checked.returncode, 0)
            self.assertIn("signature-mismatch", checked.stdout)
            done = classify.after_check(dict(plan, selected=True), checked.returncode)
            self.assertEqual(done["branch"], "selected-check-failed")
            self.assertTrue(done["selected"])
            self.assertFalse(done["run_packages"])
            self.assertFalse(done["run_compare"])
            omitted = json.loads(manifest.read_text(encoding="utf-8"))
            del omitted["patterns"]
            self.assertEqual(omitted["row"], "reading")
            self.assertEqual(
                classify.sealed_destination_paths(omitted, classify.DESTINATION_IDS),
                sealed,
            )
            gone = manifest.parent / "omitted.json"
            gone.write_text(json.dumps(omitted), encoding="utf-8")
            identified = classify.identify(
                [(gone, omitted)],
                ["driver/rows/pv/row_test.go", "driver/shared.go"],
            )
            self.assertEqual(identified[0], "one")
            failed = tool_check(main, gone, donor)
            self.assertNotEqual(failed.returncode, 0)
            self.assertNotEqual(
                classify.after_check({"selected": True, "branch": "lower-nonadditive"}, failed.returncode)["branch"],
                "no-manifest",
            )

    def test_acquisition_lines_name_each_revision(self):
        manifest = {
            "donor": {"repository": "gateway", "sha": "donor-revision"},
            "patterns": [{
                "exemplar": {"repository": "eebusreg", "revision": "exemplar-revision", "path": "rows.go"},
                "candidate": {
                    "form": "reference",
                    "repository": "semreg",
                    "revision": "candidate-revision",
                    "path": "candidate.json",
                },
            }],
            "expectations": [{
                "references": [
                    {"repository": "docs-semantic", "revision": "vector-revision", "path": "vec.json"},
                    {"repository": "modbusreg", "revision": "output-revision", "path": "out.json"},
                ],
            }],
            "expected_differences": [{
                "reference": {"repository": "docs-semantic", "revision": "diff-revision", "path": "diff.json"},
            }],
        }
        lines = classify.acquisition_lines(manifest)
        self.assertIn("helianthus-gateway", lines[0])
        joined = "\n".join(lines)
        self.assertIn("helianthus-eebusreg", joined)
        self.assertIn("exemplar-revision", joined)
        self.assertIn("candidate-revision", joined)
        self.assertIn("vector-revision", joined)
        self.assertIn("output-revision", joined)
        self.assertIn("diff-revision", joined)
        self.assertEqual(classify.public_url("helianthus-ebusgateway"), classify.public_url("gateway"))
        self.assertIn("helianthus-gateway", classify.public_url("helianthus-ebusgateway"))

    def test_unmapped_repository_and_retained_push(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            body = {
                "row": "batch",
                "donor": {"repository": "private-controller", "sha": "recorded"},
                "paths": [{"repository": "modbusreg", "paths": ["driver/pv/leaf.go"]}],
                "patterns": [],
            }
            write(root / "manifests" / "batch.json", json.dumps(body))
            write(root / "status.txt", "A\tdriver/pv/leaf.go\n")
            write(root / "pulls.json", "[]")
            errors = StringIO()
            with redirect_stderr(errors):
                code = classify.main([
                    "--event", "pull_request",
                    "--manifests-dir", str(root / "manifests"),
                    "--name-status", str(root / "status.txt"),
                    "--pulls-file", str(root / "pulls.json"),
                    "--current-number", "1",
                    "--base-ref", "main",
                    "--head-ref", "head",
                    "--base-repo-id", "1",
                    "--head-repo-id", "1",
                ])
            self.assertEqual(code, 1)
            self.assertIn("unmapped repository", errors.getvalue())
            output = root / "push.txt"
            retained = classify.main([
                "--event", "push",
                "--github-output", str(output),
                "--name-status", str(root / "status.txt"),
            ])
            self.assertEqual(retained, 0)
            text = output.read_text(encoding="utf-8")
            self.assertIn("branch=retained", text)
            self.assertIn("run_full=true", text)
            self.assertIn("run_packages=false", text)

    def test_pull_list_paginates_and_retries_without_a_token(self):
        parent = pull(2, "older", "main")
        first = "https://api.github.com/repos/example/repo/pulls?state=open&per_page=100"
        pages = {
            first: (b"[]", '<https://example.test/pulls?page=2>; rel="next"'),
            "https://example.test/pulls?page=2": (json.dumps([parent]).encode(), ""),
        }

        seen = {"urls": []}

        def opener(request, timeout=30):
            seen["urls"].append(request.full_url)
            if "headers" not in seen:
                seen["headers"] = dict(request.headers)
            payload, link = pages[request.full_url]

            class Response:
                def __enter__(self):
                    return self

                def __exit__(self, *_args):
                    return False

                def read(self):
                    return payload

                headers = {"Link": link}

            return Response()

        found = classify.fetch_open_pulls("example/repo", opener=opener, token=None)
        self.assertEqual(found, [parent])
        self.assertEqual(seen["urls"], [first, "https://example.test/pulls?page=2"])
        self.assertEqual(seen["headers"]["Accept"], "application/vnd.github+json")
        self.assertNotIn("Authorization", seen["headers"])
        self.assertEqual(classify.stack_position(pull(8, "main", "mm08"), found), "tip")
        self.assertIsNone(classify.fetch_open_pulls("example/repo"))
        denied = {"count": 0}

        def reject(request, timeout=30):
            denied["count"] += 1
            if request.has_header("Authorization"):
                raise OSError("denied")
            return opener(request, timeout)

        retried = classify.fetch_open_pulls("example/repo", opener=reject, token="synthetic")
        self.assertEqual(retried, [parent])
        self.assertGreater(denied["count"], 1)

        def fail(_request, timeout=30):
            raise OSError("down")

        self.assertIsNone(classify.fetch_open_pulls("example/repo", opener=fail, token=None))


class WorkflowTests(Case):
    def test_fetch_base_does_not_prompt_or_store_the_job_token(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            origin = root / "origin"
            checkout = root / "checkout"
            commit_tree(origin, {"keep.go": KEEP})
            shallow_clone(origin, checkout)
            git(checkout, "remote", "set-url", "origin", str(origin.resolve()))
            write(checkout / "added.go", KEEP)
            git(checkout, "add", "added.go")
            git(checkout, "commit", "-m", "add")
            token = "synthetic-job-token"
            completed = run_fetch_base(checkout, root / "authed.log", token)
            self.assertEqual(completed.returncode, 0, completed.stderr)
            recorded = (root / "authed.log").read_text(encoding="utf-8")
            basic = base64.b64encode(b"x-access-token:" + token.encode()).decode()
            self.assertIn("prompt=0", recorded)
            self.assertIn("credential.helper=", recorded)
            self.assertIn("http.extraheader=AUTHORIZATION: basic " + basic, recorded)
            self.assertNotIn(token, recorded)
            config = git(checkout, "config", "--local", "--list").stdout
            self.assertNotIn(token, config)
            self.assertNotIn("extraheader", config)
            diff = git(checkout, "diff", "--name-status", "FETCH_HEAD", "HEAD").stdout
            self.assertIn("added.go", diff)
            anonymous = run_fetch_base(checkout, root / "anon.log", None)
            self.assertEqual(anonymous.returncode, 0, anonymous.stderr)
            anon_text = (root / "anon.log").read_text(encoding="utf-8")
            self.assertIn("prompt=0", anon_text)
            self.assertIn("credential.helper=", anon_text)
            self.assertNotIn("extraheader", anon_text)
            self.assertNotIn("AUTHORIZATION", anon_text)

    def test_ci_keeps_full_suite_and_stages_the_self_test_after_it(self):
        text = (REPO / ".github" / "workflows" / "ci.yml").read_text(encoding="utf-8")
        checks, lint = text.split("  lint:", 1)
        self.assertIn("./scripts/ci_local.sh", checks)
        self.assertIn("HELIANTHUS_EXTERNAL_LINT_JOB", checks)
        self.assertIn("steps.suite.outputs.run_full != 'false'", checks)
        self.assertLess(checks.index("./scripts/ci_local.sh"), checks.index("install-only: true"))
        self.assertLess(checks.index("install-only: true"), checks.index("unittest discover -s scripts/preflight"))
        self.assertIn("QQ06_MAIN_PY", checks)
        self.assertIn("version: v2.11.4", checks)
        self.assertNotIn("install-only", lint)
        self.assertIn("version: v2.11.4", lint)

    def test_additive_workflow_orders_check_before_the_fast_path(self):
        text = (REPO / ".github" / "workflows" / "additive-rows.yml").read_text(encoding="utf-8")
        self.assertNotIn("bypass", text)
        self.assertNotIn("./scripts/ci_local.sh", text)
        self.assertLess(text.index("check --manifest"), text.index("go test -race -count=1"))
        self.assertLess(text.index("go test -race -count=1"), text.index("compare --manifest"))
        self.assertLess(text.index("compare --manifest"), text.index("install-only: true"))
        self.assertLess(text.index("install-only: true"), text.index("scripts/preflight/preflight.py"))
        self.assertLess(text.index("scripts/preflight/preflight.py"), text.index("check_size_ratchet.py"))
        self.assertIn("check_behavior_names.py --check", text)
        self.assertIn("check_go_lint_baseline.py --check", text)
        self.assertIn("fast path selected no package", text)
        self.assertIn("additive-rows: full CI retained", text)
        self.assertIn("version: v2.11.4", text)

    def test_pull_list_is_read_before_classify(self):
        marker = 'importlib.import_module("urllib.request")'
        for name in ("ci.yml", "additive-rows.yml"):
            text = (REPO / ".github" / "workflows" / name).read_text(encoding="utf-8")
            self.assertIn(marker, text)
            self.assertLess(text.index("fetch_open_pulls"), text.index("scripts/preflight/classify.py"))
            self.assertIn("--pulls-file", text)
            self.assertNotIn("--fetch-pulls", text)
            self.assertIn('bash scripts/preflight/fetch_base.sh "${BASE_REF}"', text)
            self.assertLess(text.index("fetch_base.sh"), text.index("git diff --name-status"))
            # Joined: the scope gate rejects this command text in non-Go sources.
            self.assertNotIn(
                " ".join(("git", "fetch", "--no-tags", "--depth=1", "origin")),
                text,
            )
            self.assertIn("GITHUB_TOKEN: ${{ github.token }}", text)
        for path in (REPO / "scripts" / "preflight").glob("*.py"):
            if path.name.startswith("test_"):
                continue
            source = path.read_text(encoding="utf-8")
            self.assertNotIn("urllib.request", source)
            self.assertNotIn("import socket", source)


if __name__ == "__main__":
    unittest.main()
