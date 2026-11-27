#!/usr/bin/env python3
"""Validate certification-reference data files.

``python3 -I scripts/validate_certification_references.py [--mutation-check] FILE...``
checks each file against the contract ``helianthus.semreg.certification-references/v1``
and prints the first failure of a file as one stable reason, the zero-based entry
index and the member when one is named. With ``--mutation-check`` each file must
be valid, and a temporary copy is then changed once per field of each entry
(the field removed) and once per entry (``source`` set to another value); the
validator must fail every copy. Exit status 0 when every file holds, 1 otherwise.
The script reads no network and imports nothing from the repository.
"""
import copy
import datetime
import json
import re
import sys
import tempfile
from pathlib import Path

CONTRACT = "helianthus.semreg.certification-references/v1"
SOURCE = "certification"
COMMON = ("source", "category", "driver", "vendor", "flavor", "model", "firmware")
LOCATION = ("issuer", "url", "retrieved_on")
CATEGORY_MEMBERS = {
    "reference": COMMON + ("issuer", "certificate_number", "issue_date", "stated_value", "url", "retrieved_on"),
    "gap": COMMON + LOCATION,
    "unavailable": COMMON + LOCATION + ("refusal",),
}
TEXT_MEMBERS = ("driver", "vendor", "flavor", "model", "issuer", "certificate_number")
DATE_MEMBERS = ("issue_date", "retrieved_on")
IDENTITY = ("driver", "vendor", "flavor", "model", "firmware")
DECIMAL_MEMBERS = ("ratio_maximum", "measured_minimum", "measured_maximum", "range_minimum", "range_maximum")
STATED_MEMBERS = ("measurement_type", "measured", "statement") + DECIMAL_MEMBERS
UPPER_BOUND_SOURCES = ("certificate", "vendor_document")
FIRMWARE_MEMBERS = {
    "not_stated": ("form",),
    "release": ("form", "release"),
    "range": ("form", "from", "to", "upper_bound_stated_by"),
}
RELEASE = re.compile(r"[0-9]+(\.[0-9]+)*")
DATE = re.compile(r"[0-9]{4}-[0-9]{2}-[0-9]{2}")
COEFFICIENT = re.compile(r"0|-?[1-9]([0-9]*[1-9])?")
TOKEN = re.compile(r"[a-z][a-z0-9_]*")


class FailureError(Exception):
    """One rejection: the stable reason, the entry index and the member, when any."""

    def __init__(self, reason, entry=None, member=None):
        super().__init__(reason)
        self.reason, self.entry, self.member = reason, entry, member

    def __str__(self):
        parts = [self.reason]
        if self.entry is not None:
            parts.append(f"entry {self.entry}")
        if self.member is not None:
            parts.append(f"member {self.member}")
        return " ".join(parts)


def reject_repeats(pairs):
    """Build an object from member pairs, refusing a repeated member name."""
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"repeated member {key!r}")
        result[key] = value
    return result


def load(path):
    """Load a JSON file with repeated object members refused."""
    return json.loads(Path(path).read_text(encoding="utf-8"), object_pairs_hook=reject_repeats)


def release_key(release):
    return tuple(int(part) for part in release.split("."))


def compare_releases(left, right):
    """Compare two releases component by component, a missing component being zero."""
    a, b = release_key(left), release_key(right)
    size = max(len(a), len(b))
    a, b = a + (0,) * (size - len(a)), b + (0,) * (size - len(b))
    return (a > b) - (a < b)


def is_text(value):
    return isinstance(value, str) and bool(value)


def is_date(value):
    if not isinstance(value, str) or not DATE.fullmatch(value):
        return False
    try:
        datetime.date.fromisoformat(value)
    except ValueError:
        return False
    return True


def is_url(value):
    return isinstance(value, str) and re.fullmatch(r"https://[^ ]+", value) is not None


def is_release(value):
    return isinstance(value, str) and RELEASE.fullmatch(value) is not None


def is_decimal(value):
    if not isinstance(value, dict) or set(value) != {"coefficient", "exponent10"}:
        return False
    coefficient, exponent = value["coefficient"], value["exponent10"]
    if not isinstance(coefficient, str) or not COEFFICIENT.fullmatch(coefficient):
        return False
    if type(exponent) is not int or not -18 <= exponent <= 18:
        return False
    return coefficient != "0" or exponent == 0


def first_member(entry, names, bad):
    """Return the first listed member of the entry for which ``bad`` holds."""
    return next((name for name in names if name in entry and bad(entry[name])), None)


def check_source(entry):
    if "source" in entry and entry["source"] != SOURCE:
        return "source_not_certification", "source"
    return None


def check_category(entry):
    if "category" not in entry:
        return "missing_member", "category"
    if entry["category"] not in CATEGORY_MEMBERS:
        return "invalid_value", "category"
    return None


def check_members(entry):
    category = entry["category"]
    allowed = CATEGORY_MEMBERS[category]
    if category != "reference" and "stated_value" in entry:
        return "stated_value_not_allowed", "stated_value"
    extra = sorted(set(entry) - set(allowed))
    if extra:
        return "unknown_member", extra[0]
    if category == "unavailable" and not is_text(entry.get("refusal")):
        return "refusal_missing", "refusal"
    missing = [name for name in allowed if name not in entry]
    return ("missing_member", missing[0]) if missing else None


def check_values(entry):
    bad = first_member(entry, TEXT_MEMBERS, lambda value: not is_text(value))
    bad = bad or first_member(entry, DATE_MEMBERS, lambda value: not is_date(value))
    bad = bad or first_member(entry, ("url",), lambda value: not is_url(value))
    return ("invalid_value", bad) if bad else None


def check_firmware(entry):
    firmware = entry["firmware"]
    invalid = ("firmware_invalid", "firmware")
    if not isinstance(firmware, dict) or firmware.get("form") not in FIRMWARE_MEMBERS:
        return invalid
    form = firmware["form"]
    if form == "range" and firmware.get("upper_bound_stated_by") not in UPPER_BOUND_SOURCES:
        return "firmware_upper_bound_unstated", "firmware"
    if set(firmware) != set(FIRMWARE_MEMBERS[form]):
        return invalid
    releases = [firmware[name] for name in ("release", "from", "to") if name in firmware]
    if not all(is_release(item) for item in releases):
        return invalid
    if form == "range" and compare_releases(firmware["from"], firmware["to"]) > 0:
        return invalid
    return None


def stated_value_valid(stated):
    """Whether a stated value holds only table members, one state, a non-empty statement and canonical decimals."""
    if not isinstance(stated, dict) or not set(stated) <= set(STATED_MEMBERS):
        return False
    kind = stated.get("measurement_type")
    if not isinstance(kind, str) or not TOKEN.fullmatch(kind):
        return False
    values = set(stated) - {"measurement_type"}
    if not values or ("measured" in values and type(stated["measured"]) is not bool):
        return False
    if "statement" in values and not is_text(stated["statement"]):
        return False
    return all(is_decimal(stated[name]) for name in values & set(DECIMAL_MEMBERS))


def check_stated_value(entry):
    if "stated_value" in entry and not stated_value_valid(entry["stated_value"]):
        return "stated_value_invalid", "stated_value"
    return None


ENTRY_CHECKS = (check_source, check_category, check_members, check_values, check_firmware, check_stated_value)


def check_entry(entry, index):
    """Raise the FailureError of one entry, if any."""
    if not isinstance(entry, dict):
        raise FailureError("schema", index)
    for check in ENTRY_CHECKS:
        found = check(entry)
        if found:
            raise FailureError(found[0], index, found[1])


def check_categories(entries):
    """Reject two entries of different categories for one driver, vendor, flavor, model and firmware."""
    seen = {}
    for index, entry in enumerate(entries):
        key = json.dumps([entry[name] for name in IDENTITY], sort_keys=True)
        if key in seen and seen[key] != entry["category"]:
            raise FailureError("category_conflict", index)
        seen.setdefault(key, entry["category"])


def check_document(document):
    """Raise the FailureError of a document, or return when it is valid."""
    entries = document.get("entries") if isinstance(document, dict) else None
    if not isinstance(entries, list):
        raise FailureError("schema")
    for index, entry in enumerate(entries):
        check_entry(entry, index)
    if set(document) != {"contract", "entries"} or document["contract"] != CONTRACT or not entries:
        raise FailureError("schema")
    check_categories(entries)


def validate_file(path):
    """Return None when the file holds, otherwise the FailureError."""
    try:
        document = load(path)
    except (OSError, ValueError):
        return FailureError("schema")
    try:
        check_document(document)
    except FailureError as failure:
        return failure
    return None


def mutations(document):
    """Yield a label and a changed copy per entry field removed and per entry source changed."""
    for index, entry in enumerate(document["entries"]):
        for name in entry:
            changed = copy.deepcopy(document)
            del changed["entries"][index][name]
            yield f"entry {index}: remove {name}", changed
        changed = copy.deepcopy(document)
        changed["entries"][index]["source"] = "device"
        yield f"entry {index}: source set to device", changed


def mutation_check(path):
    """Return the mutations of a valid file that the validator accepted."""
    document = load(path)
    survivors = []
    with tempfile.TemporaryDirectory() as directory:
        copy_path = Path(directory) / "mutated.json"
        for label, changed in mutations(document):
            copy_path.write_text(json.dumps(changed), encoding="utf-8")
            if validate_file(copy_path) is None:
                survivors.append(label)
    return survivors


def run_file(path, with_mutations):
    """Return the error lines of one file."""
    failure = validate_file(path)
    if failure is not None:
        return [f"FAIL: {path}: {failure}"]
    if not with_mutations:
        return []
    return [f"FAIL: {path}: mutation accepted: {label}" for label in mutation_check(path)]


def main(argv=None):
    args = sys.argv[1:] if argv is None else list(argv)
    with_mutations = "--mutation-check" in args
    paths = [item for item in args if item != "--mutation-check"]
    if not paths:
        print("usage: validate_certification_references.py [--mutation-check] FILE...", file=sys.stderr)
        return 1
    errors = [line for path in paths for line in run_file(path, with_mutations)]
    for line in errors:
        print(line, file=sys.stderr)
    if errors:
        return 1
    print(f"PASS: {len(paths)} certification reference file(s) hold")
    return 0


if __name__ == "__main__":
    sys.exit(main())
