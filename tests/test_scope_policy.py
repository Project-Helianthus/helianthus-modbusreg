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

    def vendor_overlay(self, root: Path) -> tuple[Path, dict[str, object]]:
        overlay = root / "profiles" / "vendor" / "example"
        overlay.mkdir(parents=True)
        (overlay / "profile.go").write_text("package example\n", encoding="utf-8")
        fixture = root / "fixtures" / "example.json"
        fixture.parent.mkdir()
        fixture.write_text("{}\n", encoding="utf-8")
        evidence: dict[str, object] = {
            "profile": "example",
            "profile_version": "1.0.0",
            "sources": [
                {
                    "id": "example-public-source",
                    "locator": "https://example.invalid/stable-source",
                    "permission": "independent_fact_publication_permitted",
                    "license": "source-metadata-only",
                    "sha256": "a" * 64,
                    "redistribution": "metadata_only",
                }
            ],
            "transformation": "Mapped documented words into a sanitized fixture.",
            "claim_state": "PROVEN",
            "applicability": {
                "vendor": "example",
                "models": ["model-a"],
                "gateways": ["direct"],
                "firmware": ["1.x"],
                "protocol_modes": ["modbus_tcp"],
                "address_basis": ["documentary_one_based"],
                "exclusions": [],
            },
            "sanitization": {
                "credentials_removed": True,
                "private_addresses_removed": True,
                "customer_identifiers_removed": True,
                "unrelated_payload_removed": True,
                "method": "minimum reproducible words only",
            },
            "license": "CC0-1.0",
            "code_mapping": {
                "package": (
                    "github.com/Project-Helianthus/helianthus-modbusreg/"
                    "profiles/vendor/example"
                ),
                "profile_version": "1.0.0",
                "fixtures": ["fixtures/example.json"],
            },
        }
        return overlay, evidence

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

    def test_token_free_profile_file_is_rejected_by_bootstrap_lock(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "doc.go").write_text("package modbusreg\n", encoding="utf-8")
            profile = root / "profiles" / "standard" / "example" / "profile.go"
            profile.parent.mkdir(parents=True)
            profile.write_text("package example\n", encoding="utf-8")
            with self.assertRaises(validator.PolicyError):
                validator.validate_bootstrap_lock(root, validator.EXPECTED_POLICY)

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
            overlay, evidence = self.vendor_overlay(root)
            (overlay / "evidence.json").write_text(
                json.dumps(evidence),
                encoding="utf-8",
            )
            validator.validate_vendor_evidence(root, validator.EXPECTED_POLICY)

    def test_private_unverifiable_source_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            overlay, evidence = self.vendor_overlay(root)
            evidence["sources"][0]["locator"] = "private-unverifiable-note"
            (overlay / "evidence.json").write_text(
                json.dumps(evidence),
                encoding="utf-8",
            )
            with self.assertRaises(validator.PolicyError):
                validator.validate_vendor_evidence(root, validator.EXPECTED_POLICY)

    def test_empty_applicability_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            overlay, evidence = self.vendor_overlay(root)
            evidence["applicability"]["models"] = []
            (overlay / "evidence.json").write_text(
                json.dumps(evidence),
                encoding="utf-8",
            )
            with self.assertRaises(validator.PolicyError):
                validator.validate_vendor_evidence(root, validator.EXPECTED_POLICY)

    def test_unadmitted_source_permission_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            overlay, evidence = self.vendor_overlay(root)
            evidence["sources"][0]["permission"] = "unknown_private_permission"
            (overlay / "evidence.json").write_text(
                json.dumps(evidence),
                encoding="utf-8",
            )
            with self.assertRaises(validator.PolicyError):
                validator.validate_vendor_evidence(root, validator.EXPECTED_POLICY)

    def test_unrelated_code_mapping_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            overlay, evidence = self.vendor_overlay(root)
            evidence["code_mapping"]["package"] = "example.invalid/unrelated"
            (overlay / "evidence.json").write_text(
                json.dumps(evidence),
                encoding="utf-8",
            )
            with self.assertRaises(validator.PolicyError):
                validator.validate_vendor_evidence(root, validator.EXPECTED_POLICY)


if __name__ == "__main__":
    unittest.main()
