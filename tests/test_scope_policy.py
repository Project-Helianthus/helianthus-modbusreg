from __future__ import annotations

import copy
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "validate_scope_policy.py"
POLICY_PATH = ROOT / "policy" / "registry-boundary.json"
PUBLIC_MODBUS = "github.com/Project-Helianthus/helianthus-modbus"
SPEC = importlib.util.spec_from_file_location("validate_scope_policy", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
validator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validator)


class ScopePolicyTests(unittest.TestCase):
    def policy(self) -> dict[str, object]:
        return json.loads(POLICY_PATH.read_text(encoding="utf-8"))

    def write_policy(self, root: Path, policy: dict[str, object]) -> None:
        path = root / "policy" / "registry-boundary.json"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(policy), encoding="utf-8")

    def write_module(
        self,
        root: Path,
        *,
        require_public_modbus: bool = True,
        replace_public_modbus: bool = False,
    ) -> None:
        lines = ["module github.com/Project-Helianthus/helianthus-modbusreg", "", "go 1.22"]
        if require_public_modbus:
            lines.extend(["", f"require {PUBLIC_MODBUS} v0.0.0"])
        if replace_public_modbus:
            lines.extend(["", f"replace {PUBLIC_MODBUS} => ../private-modbus"])
        (root / "go.mod").write_text("\n".join(lines) + "\n", encoding="utf-8")

    def vendor_overlay(self, root: Path) -> tuple[Path, dict[str, object]]:
        overlay = root / "profiles" / "vendor" / "example"
        overlay.mkdir(parents=True)
        (overlay / "profile.go").write_text("package example\n", encoding="utf-8")
        evidence: dict[str, object] = {
            "schema": "helianthus-modbusreg-vendor-evidence/v1",
            "profile": "example",
            "profile_version": "1.0.0",
            "public_sources": [
                {
                    "id": "example-public-source",
                    "locator": "https://example.invalid/public/register-map",
                    "license": "public-vendor-documentation",
                }
            ],
            "applicability": ["model-a"],
            "license": "CC0-1.0",
        }
        return overlay, evidence

    def test_current_policy_and_tree_are_accepted(self) -> None:
        self.assertEqual(validator.load_policy(ROOT), self.policy())
        validator.validate(ROOT)

    def test_policy_mode_and_inventory_lock_drift_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            policy = copy.deepcopy(self.policy())
            policy["repository_mode"] = "one_repo_per_vendor"
            self.write_policy(root, policy)
            with self.assertRaises(validator.PolicyError):
                validator.load_policy(root)

            policy = copy.deepcopy(self.policy())
            policy["allowed_repository_files"] = ["domain.go"]
            self.write_policy(root, policy)
            with self.assertRaises(validator.PolicyError):
                validator.load_policy(root)

    def test_public_modbus_is_the_only_external_product_dependency(self) -> None:
        policy = self.policy()
        validator.validate_imports([PUBLIC_MODBUS, "encoding/json"], policy)
        for forbidden in (
            "github.com/Project-Helianthus/helianthus-ebusreg",
            "github.com/example/private-modbus",
            "go.bug.st/serial",
        ):
            with self.subTest(import_path=forbidden):
                with self.assertRaises(validator.PolicyError):
                    validator.validate_imports([forbidden], policy)

    def test_network_framing_and_syscall_imports_are_rejected(self) -> None:
        policy = self.policy()
        validator.validate_imports(["net/url"], policy)
        for forbidden in (
            "net",
            "net/http",
            "syscall",
            "golang.org/x/sys/unix",
            "example.invalid/modbus-framing",
        ):
            with self.subTest(import_path=forbidden):
                with self.assertRaises(validator.PolicyError):
                    validator.validate_imports([forbidden], policy)

    def test_product_import_mutation_is_discovered_offline(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "go.mod").write_text(
                "module example.invalid/scopefixture\n\ngo 1.22\n",
                encoding="utf-8",
            )
            (root / "domain.go").write_text(
                'package fixture\n\nimport _ "net/http"\n',
                encoding="utf-8",
            )
            imports = validator.go_imports(root)
            self.assertIn("net/http", imports)
            with self.assertRaises(validator.PolicyError):
                validator.validate_imports(imports, self.policy())

    def test_test_only_transport_import_does_not_become_product_ownership(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "go.mod").write_text(
                "module example.invalid/scopefixture\n\ngo 1.22\n",
                encoding="utf-8",
            )
            (root / "domain.go").write_text("package fixture\n", encoding="utf-8")
            (root / "domain_test.go").write_text(
                'package fixture\n\nimport _ "net/http"\n',
                encoding="utf-8",
            )
            self.assertNotIn("net/http", validator.go_imports(root))

    def test_ordinary_new_domain_file_needs_no_inventory_or_hash_update(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "identity.go").write_text(
                "package modbusreg\n\ntype IdentityContract struct{}\n",
                encoding="utf-8",
            )
            validator.validate_m2_01_scope(root, self.policy())

    def test_detector_probe_qualification_and_profile_scope_leakage_fails(self) -> None:
        policy = self.policy()
        for relative in (
            "detectors/example/detector.go",
            "probes/example/probe.go",
            "qualification/example/record.go",
            "profiles/standard/example/profile.go",
            "profiles/vendor/example/profile.go",
        ):
            with self.subTest(relative=relative), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                target = root / relative
                target.parent.mkdir(parents=True)
                target.write_text("package example\n", encoding="utf-8")
                with self.assertRaises(validator.PolicyError):
                    validator.validate_m2_01_scope(root, policy)

        for name in ("detector.go", "probe.go", "qualification.go"):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                (root / name).write_text("package modbusreg\n", encoding="utf-8")
                with self.assertRaises(validator.PolicyError):
                    validator.validate_m2_01_scope(root, policy)

    def test_gateway_private_binding_and_canonical_semantic_ownership_fails(self) -> None:
        policy = self.policy()
        for relative in (
            "gateway/service.go",
            "private/binding.go",
            "bindings/matter.go",
            "canonical/energy.go",
            "semantics/pv.go",
        ):
            with self.subTest(relative=relative), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                target = root / relative
                target.parent.mkdir(parents=True)
                target.write_text("package forbidden\n", encoding="utf-8")
                with self.assertRaises(validator.PolicyError):
                    validator.validate_m2_01_scope(root, policy)

    def test_standard_family_vendor_leak_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            standard = root / "profiles" / "standard" / "example"
            standard.mkdir(parents=True)
            (standard / "profile.go").write_text(
                'package example\nconst family = "huawei"\n',
                encoding="utf-8",
            )
            with self.assertRaises(validator.PolicyError):
                validator.validate_standard_sources(root, self.policy())

    def test_future_vendor_overlay_requires_minimal_public_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            overlay, evidence = self.vendor_overlay(root)
            with self.assertRaises(validator.PolicyError):
                validator.validate_vendor_evidence(root, self.policy())

            (overlay / "evidence.json").write_text(
                json.dumps(evidence),
                encoding="utf-8",
            )
            validator.validate_vendor_evidence(root, self.policy())

            evidence["public_sources"][0]["locator"] = "private-note"
            (overlay / "evidence.json").write_text(
                json.dumps(evidence),
                encoding="utf-8",
            )
            with self.assertRaises(validator.PolicyError):
                validator.validate_vendor_evidence(root, self.policy())

    def test_non_go_socket_import_is_discovered_without_an_allowlist(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            target = root / "scripts" / "new_check.py"
            target.parent.mkdir(parents=True)
            target.write_text("import socket\n", encoding="utf-8")
            with self.assertRaises(validator.PolicyError):
                validator.validate_non_go_code(root)

    def test_module_requires_unreplaced_public_modbus(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.write_module(root)
            validator.validate_module(root, self.policy())

            self.write_module(root, require_public_modbus=False)
            with self.assertRaises(validator.PolicyError):
                validator.validate_module(root, self.policy())

            self.write_module(root, replace_public_modbus=True)
            with self.assertRaises(validator.PolicyError):
                validator.validate_module(root, self.policy())


if __name__ == "__main__":
    unittest.main()
