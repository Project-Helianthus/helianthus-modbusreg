"""Tests of the certification-reference validator and its mutation check."""
import copy
import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "validate_certification_references.py"
DATA = ROOT / "driverdata" / "certification" / "testdata"
SPEC = importlib.util.spec_from_file_location("validate_certification_references", SCRIPT)
VALIDATOR = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = VALIDATOR
SPEC.loader.exec_module(VALIDATOR)


def valid_document():
    return json.loads((DATA / "valid.json").read_text(encoding="utf-8"))


def failure_of(document):
    with tempfile.TemporaryDirectory() as directory:
        path = Path(directory) / "document.json"
        path.write_text(json.dumps(document), encoding="utf-8")
        return VALIDATOR.validate_file(path)


class ValidFileTests(unittest.TestCase):
    def test_valid_file_holds(self):
        self.assertIsNone(VALIDATOR.validate_file(DATA / "valid.json"))

    def test_mutation_check_passes_on_valid_file(self):
        self.assertEqual(VALIDATOR.mutation_check(DATA / "valid.json"), [])

    def test_command_line_exit_status(self):
        run = [sys.executable, "-I", str(SCRIPT)]
        good = subprocess.run(run + ["--mutation-check", str(DATA / "valid.json")], capture_output=True, text=True, check=False)
        self.assertEqual(good.returncode, 0, good.stderr)
        for name in ("category_conflict.json", "device_published_member.json"):
            bad = subprocess.run(run + [str(DATA / name)], capture_output=True, text=True, check=False)
            self.assertEqual(bad.returncode, 1, name)
        none = subprocess.run(run, capture_output=True, text=True, check=False)
        self.assertEqual(none.returncode, 1)

    def test_mutation_check_fails_on_a_file_that_is_not_valid(self):
        bad = DATA / "category_conflict.json"
        self.assertEqual(VALIDATOR.run_file(bad, True)[0].split(": ")[2], "category_conflict entry 1")


class MutationEnumerationTests(unittest.TestCase):
    def test_every_field_of_every_entry_and_every_source_is_mutated(self):
        document = valid_document()
        labels = [label for label, _ in VALIDATOR.mutations(document)]
        expected = sum(len(entry) + 1 for entry in document["entries"])
        self.assertEqual(len(labels), expected)
        self.assertEqual(len(set(labels)), expected)

    def test_a_validator_that_accepts_a_mutation_is_reported(self):
        original = VALIDATOR.validate_file
        VALIDATOR.validate_file = lambda path: None
        try:
            survivors = VALIDATOR.mutation_check(DATA / "valid.json")
        finally:
            VALIDATOR.validate_file = original
        self.assertEqual(len(survivors), len(list(VALIDATOR.mutations(valid_document()))))


class RejectionTests(unittest.TestCase):
    def expect(self, document, reason, entry=None, member=None):
        failure = failure_of(document)
        self.assertIsNotNone(failure)
        self.assertEqual((failure.reason, failure.entry, failure.member), (reason, entry, member))

    def test_two_categories_for_one_device_fail(self):
        self.expect(json.loads((DATA / "category_conflict.json").read_text(encoding="utf-8")), "category_conflict", 1)

    def test_conflict_needs_every_identity_field_equal(self):
        changes = {
            "driver": "other-driver",
            "vendor": "Other Vendor",
            "flavor": "other-profile",
            "model": "EX-9999",
            "firmware": {"form": "release", "release": "1.0"},
        }
        for name, value in changes.items():
            document = valid_document()
            gap = copy.deepcopy(document["entries"][4])
            for field in changes:
                gap[field] = document["entries"][3][field]
            document["entries"].append(gap)
            self.expect(document, "category_conflict", 7)
            gap[name] = value
            self.assertIsNone(failure_of(document), name)

    def test_member_naming_a_device_published_value_fails(self):
        for name in ("published_value", "reading", "measured_value", "device_value", "accuracy", "value"):
            for index in range(len(valid_document()["entries"])):
                document = valid_document()
                document["entries"][index][name] = "x"
                self.expect(document, "unknown_member", index, name)

    def test_source_other_than_certification_fails(self):
        for index in range(len(valid_document()["entries"])):
            document = valid_document()
            document["entries"][index]["source"] = "device"
            self.expect(document, "source_not_certification", index, "source")

    def test_value_on_gap_and_unavailable_fails(self):
        for index in (4, 6):
            document = valid_document()
            document["entries"][index]["stated_value"] = {"measurement_type": "active_power", "measured": True}
            self.expect(document, "stated_value_not_allowed", index, "stated_value")

    def test_refusal_text_is_required(self):
        document = valid_document()
        document["entries"][6]["refusal"] = ""
        self.expect(document, "refusal_missing", 6, "refusal")
        del document["entries"][6]["refusal"]
        self.expect(document, "refusal_missing", 6, "refusal")

    def test_range_needs_a_stated_upper_bound(self):
        document = valid_document()
        del document["entries"][2]["firmware"]["upper_bound_stated_by"]
        self.expect(document, "firmware_upper_bound_unstated", 2, "firmware")

    def test_range_from_above_to_fails(self):
        document = valid_document()
        document["entries"][2]["firmware"].update({"from": "3.0", "to": "2.0"})
        self.expect(document, "firmware_invalid", 2, "firmware")

    def test_release_order_is_numeric(self):
        document = valid_document()
        document["entries"][2]["firmware"].update({"from": "1.9", "to": "1.10"})
        self.assertIsNone(failure_of(document))

    def test_statement_only_stated_value_passes_and_empty_statement_fails(self):
        """Stated members: measurement_type, statement.

        A loader or validator that drops measurement_type, accepts an empty or
        non-string statement, or accepts a stated value with the statement removed
        is rejected. Each field is varied.
        """
        document = json.loads((DATA / "statement.json").read_text(encoding="utf-8"))
        self.assertIsNone(failure_of(document))
        stated = document["entries"][0]["stated_value"]
        self.assertEqual(set(stated), {"measurement_type", "statement"})
        for measurement_type, statement in (
            ("active_energy", "Accuracy class designation: class B, with the standard the certificate names."),
            ("reactive_energy", "Accuracy class designation: class C, with the standard the certificate names."),
        ):
            varied = json.loads((DATA / "statement.json").read_text(encoding="utf-8"))
            varied["entries"][0]["stated_value"] = {
                "measurement_type": measurement_type,
                "statement": statement,
            }
            self.assertIsNone(failure_of(varied), (measurement_type, statement))
        empty = json.loads((DATA / "statement.json").read_text(encoding="utf-8"))
        empty["entries"][0]["stated_value"]["statement"] = ""
        self.expect(empty, "stated_value_invalid", 0, "stated_value")
        not_text = json.loads((DATA / "statement.json").read_text(encoding="utf-8"))
        not_text["entries"][0]["stated_value"]["statement"] = 1
        self.expect(not_text, "stated_value_invalid", 0, "stated_value")
        removed = json.loads((DATA / "statement.json").read_text(encoding="utf-8"))
        del removed["entries"][0]["stated_value"]["statement"]
        self.expect(removed, "stated_value_invalid", 0, "stated_value")
        missing_type = json.loads((DATA / "statement.json").read_text(encoding="utf-8"))
        del missing_type["entries"][0]["stated_value"]["measurement_type"]
        self.expect(missing_type, "stated_value_invalid", 0, "stated_value")

    def test_stated_value_rules(self):
        cases = (
            {"measurement_type": "active_power"},
            {"measurement_type": "Active Power", "measured": True},
            {"measurement_type": "active_power", "measured": None},
            {"measurement_type": "active_power", "ratio_maximum": {"coefficient": "10", "exponent10": -3}},
            {"measurement_type": "active_power", "ratio_maximum": {"coefficient": "1", "exponent10": 1.5}},
            {"measurement_type": "active_power", "measured": True, "reading": True},
        )
        for stated in cases:
            document = valid_document()
            document["entries"][0]["stated_value"] = stated
            self.expect(document, "stated_value_invalid", 0, "stated_value")

    def test_value_rules(self):
        cases = (
            ("issue_date", "2000-02-30"),
            ("retrieved_on", "2000/02/01"),
            ("url", "http://example.invalid/x"),
            ("driver", ""),
            ("category", "estimate"),
        )
        for member, value in cases:
            document = valid_document()
            document["entries"][0][member] = value
            self.expect(document, "invalid_value", 0, member)

    def test_top_level_shape(self):
        for change in (
            lambda d: d.update(contract="other"),
            lambda d: d.update(entries=[]),
            lambda d: d.update(note="x"),
        ):
            document = valid_document()
            change(document)
            self.expect(document, "schema")

    def test_repeated_member_is_refused(self):
        text = (DATA / "valid.json").read_text(encoding="utf-8").replace(
            '"driver": "example-driver"', '"driver": "example-driver", "driver": "other"', 1)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "repeated.json"
            path.write_text(text, encoding="utf-8")
            self.assertEqual(VALIDATOR.validate_file(path).reason, "schema")


class SchemaCopyTests(unittest.TestCase):
    def test_schema_states_the_contract(self):
        schema = json.loads((ROOT / "driverdata" / "certification" / "references.schema.json").read_text(encoding="utf-8"))
        self.assertEqual(schema["properties"]["contract"]["const"], VALIDATOR.CONTRACT)


if __name__ == "__main__":
    unittest.main()
