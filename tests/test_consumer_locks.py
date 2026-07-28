from __future__ import annotations

import copy
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "validate_consumer_locks.py"
SPEC = importlib.util.spec_from_file_location("validate_consumer_locks", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
validator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validator)


class ConsumerLockTests(unittest.TestCase):
    def test_exact_documentation_lock_is_accepted(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "docs.json"
            path.write_text(json.dumps(validator.DOCS_LOCK), encoding="utf-8")
            self.assertEqual(
                validator.load_exact(path, validator.DOCS_LOCK),
                validator.DOCS_LOCK,
            )

    def test_documentation_commit_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "docs.json"
            changed = copy.deepcopy(validator.DOCS_LOCK)
            changed["merged_commit_sha"] = "0" * 40
            path.write_text(json.dumps(changed), encoding="utf-8")
            with self.assertRaises(validator.ConsumerLockError):
                validator.load_exact(path, validator.DOCS_LOCK)

    def test_manifest_digest_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "docs.json"
            changed = copy.deepcopy(validator.DOCS_LOCK)
            changed["manifest_sha256"] = "0" * 64
            path.write_text(json.dumps(changed), encoding="utf-8")
            with self.assertRaises(validator.ConsumerLockError):
                validator.load_exact(path, validator.DOCS_LOCK)

    def test_runtime_commit_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "runtime.json"
            changed = copy.deepcopy(validator.RUNTIME_LOCK)
            changed["commit_sha"] = "0" * 40
            path.write_text(json.dumps(changed), encoding="utf-8")
            with self.assertRaises(validator.ConsumerLockError):
                validator.load_exact(path, validator.RUNTIME_LOCK)


if __name__ == "__main__":
    unittest.main()
