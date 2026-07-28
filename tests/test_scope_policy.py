from __future__ import annotations

import copy
import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "validate_scope_policy.py"
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

    def authorized_go_layout(self, root: Path) -> None:
        for relative in validator.EXPECTED_POLICY["allowed_product_go_files"]:
            destination = root / str(relative)
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes((ROOT / str(relative)).read_bytes())
        for relative in validator.EXPECTED_POLICY["allowed_test_go_files"]:
            destination = root / str(relative)
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes((ROOT / str(relative)).read_bytes())
        for key in ("documentation_consumer_lock", "runtime_consumer_lock"):
            relative = Path(str(validator.EXPECTED_POLICY[key]))
            destination = root / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes((ROOT / relative).read_bytes())

    def authorized_repository_layout(self, root: Path) -> None:
        for relative in validator.EXPECTED_POLICY["allowed_repository_files"]:
            destination = root / str(relative)
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.touch()

    def authorized_non_go_code_layout(self, root: Path) -> None:
        for relative in validator.EXPECTED_POLICY[
            "allowed_code_bearing_non_go_files"
        ]:
            destination = root / str(relative)
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes((ROOT / str(relative)).read_bytes())

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

    def test_token_free_profile_file_is_rejected_by_m2_01_lock(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.authorized_go_layout(root)
            profile = root / "profiles" / "standard" / "example" / "profile.go"
            profile.parent.mkdir(parents=True)
            profile.write_text("package example\n", encoding="utf-8")
            with self.assertRaises(validator.PolicyError):
                validator.validate_implementation_lock(
                    root,
                    validator.EXPECTED_POLICY,
                )

    def test_profile_code_in_allowed_doc_file_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.authorized_go_layout(root)
            (root / "doc.go").write_text(
                "package modbusreg\nfunc profile(words []uint16) int { return len(words) }\n",
                encoding="utf-8",
            )
            with self.assertRaises(validator.PolicyError):
                validator.validate_implementation_lock(
                    root,
                    validator.EXPECTED_POLICY,
                )

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

    def test_vendor_python_probe_without_evidence_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            overlay = root / "profiles" / "vendor" / "fronius"
            overlay.mkdir(parents=True)
            (overlay / "probe.py").write_text(
                "def probe():\n    return None\n",
                encoding="utf-8",
            )
            with self.assertRaises(validator.PolicyError):
                validator.validate_vendor_evidence(root, validator.EXPECTED_POLICY)

    def test_non_go_socket_artifact_is_rejected_by_repository_inventory(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.authorized_repository_layout(root)
            artifact = root / "profiles" / "vendor" / "fronius" / "socket_probe.py"
            artifact.parent.mkdir(parents=True)
            artifact.write_text("import socket\n", encoding="utf-8")
            with self.assertRaises(validator.PolicyError):
                validator.validate_repository_inventory(
                    root,
                    validator.EXPECTED_POLICY,
                )

    def test_git_index_rejects_gitlink_mode(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            subprocess.run(
                ["git", "init", "-q"],
                cwd=root,
                check=True,
            )
            blob = subprocess.run(
                ["git", "hash-object", "-w", "--stdin"],
                cwd=root,
                input=b"not-a-submodule\n",
                check=True,
                capture_output=True,
            ).stdout.decode("ascii").strip()
            subprocess.run(
                [
                    "git",
                    "update-index",
                    "--add",
                    "--cacheinfo",
                    f"160000,{blob},nested-repository",
                ],
                cwd=root,
                check=True,
            )
            with self.assertRaisesRegex(validator.PolicyError, "non-regular mode"):
                validator.validate_git_index(root, validator.EXPECTED_POLICY)

    def test_git_index_rejects_unlisted_regular_entry(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            subprocess.run(
                ["git", "init", "-q"],
                cwd=root,
                check=True,
            )
            extra = root / "existing-name.py"
            extra.write_text("value = 1\n", encoding="utf-8")
            subprocess.run(
                ["git", "add", "existing-name.py"],
                cwd=root,
                check=True,
            )
            with self.assertRaisesRegex(validator.PolicyError, "unlisted"):
                validator.validate_git_index(root, validator.EXPECTED_POLICY)

    def test_existing_python_file_rejects_socket_import_mutation(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.authorized_non_go_code_layout(root)
            target = root / "scripts" / "validate_consumer_locks.py"
            target.write_text(
                target.read_text(encoding="utf-8") + "\nimport socket\n",
                encoding="utf-8",
            )
            with self.assertRaisesRegex(validator.PolicyError, "forbidden I/O"):
                validator.validate_non_go_code(
                    root,
                    validator.EXPECTED_POLICY,
                )

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
