"""Synthetic acceptance coverage for thresholds, identities and parser behavior."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("ratchet", ROOT / "scripts/check_size_ratchet.py")
R = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(R)


def row(path="unit.go", symbol="$file", metric="file_lines", value=1001):
    return dict(path=path, symbol=symbol, metric=metric, value=value)


def baseline(*rows):
    return R.load_baseline(dict(version=1, violations=list(rows)))


class RatchetTests(unittest.TestCase):
    def test_all_threshold_boundaries(self):
        for kind, metrics in R.BUDGETS.items():
            for metric, (soft, hard) in metrics.items():
                for threshold in (soft, hard):
                    for delta in (-1, 0, 1):
                        with self.subTest(kind=kind, metric=metric, threshold=threshold, delta=delta):
                            item = row(path="unit_test.go" if kind == "test" else "unit.go",
                                       symbol="F" if metric == "function_lines" else "$file",
                                       metric=metric, value=threshold+delta)
                            warnings, errors = R.evaluate([item], {})
                            self.assertEqual(bool(warnings), item["value"] > soft)
                            self.assertEqual(bool(errors), item["value"] > hard)

    def test_independent_identity_and_value_mutations(self):
        original = row(symbol="F", metric="function_lines", value=1001)
        saved = baseline(original)
        mutations = dict(path="other.go", symbol="G", metric="file_lines", value=1002)
        for field, value in mutations.items():
            with self.subTest(field=field):
                changed = dict(original, **{field: value})
                self.assertTrue(R.evaluate([changed], saved)[1])
        self.assertFalse(R.evaluate([original], saved)[1])

    def test_reduction_requires_tightening_and_blocks_regrowth(self):
        original = row(value=1200)
        saved = baseline(original)
        self.assertFalse(R.evaluate([original], saved)[1])
        self.assertTrue(R.evaluate([dict(original, path="renamed.go")], saved)[1])
        for value in (1100, 1000, 999, None):
            with self.subTest(value=value):
                reduced = [] if value is None else [dict(original, value=value)]
                self.assertTrue(R.evaluate(reduced, saved)[1])
                tightened = baseline(*[r for r in reduced if r["value"] > 1000])
                self.assertFalse(R.evaluate(reduced, tightened)[1])
                self.assertTrue(R.evaluate([original], tightened)[1])
                if value is not None:
                    regrown = dict(original, value=max(1001, value + 1))
                    self.assertTrue(R.evaluate([regrown], tightened)[1])
        self.assertEqual(saved, baseline(original))

    def test_source_directory_scan_errors_are_fatal(self):
        root = self.fixture()
        (root / "unit.go").write_text("package sample\n")
        blocked = root / "blocked"
        blocked.mkdir()
        (blocked / "hidden.go").write_text("package sample\n")
        scandir = R.os.scandir

        def scan(path):
            if Path(path) == blocked:
                raise PermissionError("source directory denied")
            return scandir(path)

        with mock.patch.object(R.os, "scandir", side_effect=scan):
            with self.assertRaisesRegex(PermissionError, "source directory denied"):
                R.sources(root)

    def test_malformed_baselines(self):
        good = dict(version=1, violations=[row()])
        invalid = [None, [], {}, dict(good, version=True), dict(good, extra=1),
                   dict(good, violations={}), dict(good, violations=[row(), row()])]
        for field, values in dict(path=["*", "../unit.go", "/unit.go", "vendor/unit.go", "./unit.go", "a//unit.go", 3],
                                  symbol=["*", "", None, "F"], metric=["unknown", None],
                                  value=[True, "1001", 1000, -1]).items():
            for value in values:
                invalid.append(dict(good, violations=[dict(row(), **{field: value})]))
        invalid.append(dict(good, violations=[dict(row(), extra=1)]))
        for data in invalid:
            with self.subTest(data=data), self.assertRaises(ValueError):
                R.load_baseline(data)

    def fixture(self):
        temp = tempfile.TemporaryDirectory(dir=ROOT / ".tmp")
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        (root / "scripts/sizeratchet").mkdir(parents=True)
        (root / "scripts/sizeratchet/parser.go.txt").write_bytes((ROOT / "scripts/sizeratchet/parser.go.txt").read_bytes())
        return root

    def test_methods_literals_build_tags_and_line_spans(self):
        root = self.fixture()
        source = "//go:build imaginary_platform\n\npackage sample\ntype T int\nfunc (t *T) Method() {\n f := func() {\n  _ = func() {}\n }\n f()\n}\n"
        (root / "foreign_imaginary.go").write_text(source)
        (root / "foreign_test.go").write_text("package sample\nfunc TestThing() {}")
        rows, excluded = R.measure(root)
        found = {(r["path"], r["symbol"], r["metric"]): r["value"] for r in rows}
        self.assertEqual(found[("foreign_imaginary.go", "(*T).Method", "function_lines")], 6)
        self.assertEqual(found[("foreign_imaginary.go", "(*T).Method/$literal[1]", "function_lines")], 3)
        self.assertEqual(found[("foreign_imaginary.go", "(*T).Method/$literal[1]/$literal[1]", "function_lines")], 1)
        self.assertEqual(found[("foreign_imaginary.go", "$file", "functions_per_file")], 3)
        self.assertEqual(found[("foreign_imaginary.go", "$file", "file_lines")], 10)
        self.assertEqual(found[("foreign_test.go", "$file", "file_lines")], 2)
        (root / "foreign_imaginary.go").write_text("\n" + source)
        shifted, _ = R.measure(root)
        self.assertEqual([r for r in rows if r["metric"] != "file_lines"],
                         [r for r in shifted if r["metric"] != "file_lines"])
        self.assertEqual(excluded, [])

    def test_line_directives_do_not_change_measured_spans(self):
        root = self.fixture()
        source = "package sample\nfunc F() {\n//line other.go:900\n_ = 1\n//line other.go:1\n_ = 2\n}\n"
        (root / "directive.go").write_text(source)
        rows, _ = R.measure(root)
        found = {R.key(r): r["value"] for r in rows}
        self.assertEqual(found[("directive.go", "F", "function_lines")], 6)

    def test_parser_threshold_spans_and_counts(self):
        root = self.fixture()
        expected = {}
        for kind, metrics in R.BUDGETS.items():
            for metric, thresholds in metrics.items():
                for threshold in thresholds:
                    for delta in (-1, 0, 1):
                        value = threshold + delta
                        suffix = "_test.go" if kind == "test" else ".go"
                        name = f"{kind}_{metric}_{value}{suffix}"
                        if metric == "file_lines":
                            source = "package sample\n" + "\n" * (value - 1)
                            symbol = "$file"
                        elif metric == "function_lines":
                            source = "package sample\nfunc F() {\n" + "\n" * (value - 2) + "}\n"
                            symbol = "F"
                        else:
                            source = "package sample\n" + "".join(f"func F{i}() {{}}\n" for i in range(value))
                            symbol = "$file"
                        (root / name).write_text(source)
                        expected[(name, symbol, metric)] = value
        rows, _ = R.measure(root)
        actual = {R.key(item): item["value"] for item in rows}
        for identity, value in expected.items():
            with self.subTest(identity=identity):
                self.assertEqual(actual[identity], value)

    def test_parse_failure_is_fatal_in_any_platform(self):
        root = self.fixture()
        (root / "broken_windows.go").write_text("//go:build windows\npackage broken\nfunc (")
        with self.assertRaisesRegex(ValueError, "Go parser failed"):
            R.measure(root)

    def test_exclusions_reported_no_package_or_generated_exemption(self):
        root = self.fixture()
        for name in ("vendor", "references", "node_modules"):
            (root / name).mkdir()
            (root / name / "bad.go").write_text("bad")
        (root / "generated.go").write_text("// Code generated by unknown. DO NOT EDIT.\npackage sample\n")
        paths, exclusions = R.sources(root)
        self.assertEqual(paths, ["generated.go"])
        self.assertEqual(exclusions, ["node_modules/", "references/", "vendor/"])
        rows, _ = R.measure(root)
        self.assertTrue(rows)
        (root / "generated.go").unlink()
        with self.assertRaisesRegex(ValueError, "no Go source"):
            R.measure(root)


if __name__ == "__main__":
    unittest.main()
