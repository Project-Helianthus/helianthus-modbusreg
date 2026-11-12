"""Synthetic acceptance tests for the offline Python debt ratchet."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location(
    "python_lint", Path(__file__).resolve().parents[1] / "scripts/check_python_lint.py")
LINT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(LINT)


def complex_source(count):
    return "def branching(value):\n" + "".join(
        f"    if value == {index}:\n        return {index}\n" for index in range(count))


def statement_source(count):
    return "def statements():\n" + "    value = 0\n" * count


class PythonLintTests(unittest.TestCase):
    def measure(self, source):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "sample.py").write_text(source)
            return LINT.measure(root)

    def test_new_rules_in_indebted_file(self):
        original = "def OldName():\n    pass\n"
        baseline = self.measure(original)
        for rule, addition in [("N802", "def NewName():\n    pass\n"),
                               ("C901", complex_source(11)),
                               ("PLR0915", statement_source(51))]:
            with self.subTest(rule=rule):
                current = self.measure(original + "\n" + addition)
                self.assertTrue(any(row["rule"] == rule for row in current))
                self.assertFalse(LINT.compare(baseline, current))

    def test_measured_growth_and_deletion(self):
        for builder, count in [(complex_source, 11), (statement_source, 51)]:
            with self.subTest(builder=builder.__name__):
                baseline = self.measure(builder(count))
                self.assertTrue(baseline)
                self.assertTrue(all(row["value"] is not None for row in baseline))
                self.assertFalse(LINT.compare(baseline, self.measure(builder(count + 1))))
                self.assertFalse(LINT.compare(baseline, self.measure("")))
                self.assertFalse(LINT.compare(baseline, self.measure(builder(count - 1))))

    def test_every_identity_field_and_value(self):
        row = self.measure("def BadName():\n    pass\n")[0]
        for field in ("path", "rule", "symbol", "detail", "value"):
            with self.subTest(field=field):
                changed = dict(row)
                changed[field] = 99 if field == "value" else "different"
                self.assertFalse(LINT.compare([row], [changed]))
        self.assertFalse(LINT.compare([row], [row, row]))
        self.assertTrue(LINT.compare([row], [dict(row)]))

    def test_ignored_file_and_noqa_still_scanned(self):
        rows = self.measure("def BadName():  # noqa: N802\n    pass\n")
        self.assertEqual(rows[0]["rule"], "N802")


if __name__ == "__main__":
    unittest.main()
