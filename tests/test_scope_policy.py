from __future__ import annotations

import copy
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "validate_scope_policy.py"
SPEC = importlib.util.spec_from_file_location("validate_scope_policy", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
validator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validator)


class ScopePolicyTests(unittest.TestCase):
    def write_policy(self, root: Path, policy: dict[str, object]) -> None:
        path = root / "policy" / "registry-boundary.json"
        path.parent.mkdir(parents=True)
        path.write_text(json.dumps(policy), encoding="utf-8")

    def test_exact_policy_is_accepted(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.write_policy(root, validator.EXPECTED_POLICY)
            self.assertEqual(validator.load_policy(root), validator.EXPECTED_POLICY)

    def test_policy_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            policy = copy.deepcopy(validator.EXPECTED_POLICY)
            policy["repository_mode"] = "one_repo_per_vendor"
            self.write_policy(root, policy)
            with self.assertRaises(validator.PolicyError):
                validator.load_policy(root)

    def test_foreign_helianthus_dependency_is_rejected(self) -> None:
        with self.assertRaises(validator.PolicyError):
            validator.validate_imports(
                ["github.com/Project-Helianthus/helianthus-ebusreg/registry"],
                validator.EXPECTED_POLICY,
            )

    def test_network_dependency_is_rejected(self) -> None:
        with self.assertRaises(validator.PolicyError):
            validator.validate_imports(["net/http"], validator.EXPECTED_POLICY)

    def test_vendor_leak_in_standard_family_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            standard = root / "profiles" / "standard" / "example"
            standard.mkdir(parents=True)
            (standard / "profile.go").write_text(
                "package example\nconst family = \"huawei\"\n",
                encoding="utf-8",
            )
            with self.assertRaises(validator.PolicyError):
                validator.validate_standard_sources(root, validator.EXPECTED_POLICY)

    def test_vendor_overlay_without_evidence_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            overlay = root / "profiles" / "vendor" / "example"
            overlay.mkdir(parents=True)
            (overlay / "profile.go").write_text("package example\n", encoding="utf-8")
            with self.assertRaises(validator.PolicyError):
                validator.validate_vendor_evidence(root, validator.EXPECTED_POLICY)

    def test_vendor_overlay_with_exact_evidence_is_accepted(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            overlay = root / "profiles" / "vendor" / "example"
            overlay.mkdir(parents=True)
            (overlay / "profile.go").write_text("package example\n", encoding="utf-8")
            evidence = {
                "profile": "example/v1",
                "sources": ["public-evidence-id"],
                "claim_state": "PROVEN",
                "applicability": {"model": "example"},
                "license": "CC0-1.0",
            }
            (overlay / "evidence.json").write_text(
                json.dumps(evidence),
                encoding="utf-8",
            )
            validator.validate_vendor_evidence(root, validator.EXPECTED_POLICY)


if __name__ == "__main__":
    unittest.main()
