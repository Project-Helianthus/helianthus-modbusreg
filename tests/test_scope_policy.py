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
PUBLIC_SEMREG = "github.com/Project-Helianthus/helianthus-semreg"
MODULE = "github.com/Project-Helianthus/helianthus-modbusreg"
DRIVER_CONTRACT_IMPORTS = (
    PUBLIC_SEMREG + "/semreg/v1/driver",
    PUBLIC_SEMREG + "/semreg/v1/roweval",
    PUBLIC_SEMREG + "/semreg/v1",
    PUBLIC_SEMREG + "/semreg/v1/catalog",
    PUBLIC_SEMREG + "/semreg/v1/operation",
)
NON_CONTRACT_SEMREG_IMPORTS = (
    PUBLIC_SEMREG,
    PUBLIC_SEMREG + "/semreg/v1/driverextra",
    PUBLIC_SEMREG + "/semreg/v1/rowevalextra",
    PUBLIC_SEMREG + "/semreg/v1/runtime",
    PUBLIC_SEMREG + "/semreg/v1/driver/extra",
)
MODBUS_REQUIRE = {"Path": PUBLIC_MODBUS, "Version": "v0.0.0", "Indirect": False}
SEMREG_REQUIRE = {"Path": PUBLIC_SEMREG, "Version": "v0.0.0", "Indirect": False}
TEXT_REQUIRE = {"Path": "golang.org/x/text", "Version": "v0.0.0", "Indirect": True}
SYS_REQUIRE = {"Path": "golang.org/x/sys", "Version": "v0.30.0", "Indirect": True}
GRAPH_WITH_SYS_EDGE = (
    f"{MODULE} {PUBLIC_MODBUS}@v0.0.0\n"
    f"{PUBLIC_MODBUS}@v0.0.0 golang.org/x/sys@v0.30.0\n"
)
GRAPH_WITHOUT_SYS_EDGE = f"{MODULE} {PUBLIC_MODBUS}@v0.0.0\n"
SPEC = importlib.util.spec_from_file_location("validate_scope_policy", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
validator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validator)


def from_root(*imports: str) -> list[tuple[str, str]]:
    return [(import_path, ".") for import_path in imports]


def from_directory(directory: str, *imports: str) -> list[tuple[str, str]]:
    return [(import_path, directory) for import_path in imports]


def module_record(*requires: dict[str, object]) -> dict[str, object]:
    return {"Module": {"Path": MODULE}, "Require": list(requires), "Replace": None}


def graph_must_not_be_read() -> str:
    raise AssertionError("the module graph must not be consulted")


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
        overlay = root / "profiles" / "vendors" / "example"
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

    def test_public_modbus_is_the_only_product_import_outside_the_driver_leaf(self) -> None:
        policy = self.policy()
        validator.validate_imports(from_root(PUBLIC_MODBUS, "encoding/json"), policy)
        for forbidden in (
            "github.com/Project-Helianthus/helianthus-ebusreg",
            "github.com/example/private-modbus",
            "go.bug.st/serial",
            PUBLIC_SEMREG,
        ):
            with self.subTest(import_path=forbidden):
                with self.assertRaises(validator.PolicyError):
                    validator.validate_imports(from_root(forbidden), policy)
                with self.assertRaises(validator.PolicyError):
                    validator.validate_imports(from_directory("driver", forbidden), policy)

    def test_network_framing_and_syscall_imports_are_rejected(self) -> None:
        policy = self.policy()
        validator.validate_imports(from_root("net/url"), policy)
        for forbidden in (
            "net",
            "net/http",
            "syscall",
            "golang.org/x/sys/unix",
            "example.invalid/modbus-framing",
        ):
            with self.subTest(import_path=forbidden):
                with self.assertRaises(validator.PolicyError):
                    validator.validate_imports(from_root(forbidden), policy)

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
            self.assertIn(("net/http", "."), imports)
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
            self.assertNotIn(
                "net/http", [import_path for import_path, _ in validator.go_imports(root)]
            )

    def test_each_exact_semreg_contract_package_is_accepted_from_the_driver_leaf(self) -> None:
        policy = self.policy()
        for import_path in DRIVER_CONTRACT_IMPORTS:
            for directory in ("driver", "driver/example"):
                with self.subTest(import_path=import_path, directory=directory):
                    validator.validate_imports(
                        from_directory(directory, import_path), policy
                    )
        validator.validate_imports(
            from_directory("driver/example", PUBLIC_SEMREG + "/semreg/v1/roweval"),
            policy,
        )

    def test_each_exact_semreg_contract_package_is_rejected_outside_the_driver_leaf(self) -> None:
        policy = self.policy()
        for import_path in DRIVER_CONTRACT_IMPORTS:
            for directory in (".", "huawei", "driverx"):
                with self.subTest(import_path=import_path, directory=directory):
                    with self.assertRaises(validator.PolicyError):
                        validator.validate_imports(
                            from_directory(directory, import_path), policy
                        )

    def test_non_contract_semreg_paths_are_rejected_even_from_the_driver_leaf(self) -> None:
        policy = self.policy()
        for import_path in NON_CONTRACT_SEMREG_IMPORTS:
            with self.subTest(import_path=import_path):
                with self.assertRaises(validator.PolicyError):
                    validator.validate_imports(
                        from_directory("driver/example", import_path), policy
                    )

    def test_driver_leaf_keeps_sockets_and_syscalls_out(self) -> None:
        policy = self.policy()
        for forbidden in ("net", "syscall", "golang.org/x/sys/unix"):
            with self.subTest(import_path=forbidden):
                with self.assertRaises(validator.PolicyError):
                    validator.validate_imports(
                        from_directory("driver", forbidden), policy
                    )

    def test_pre_exemplar_modbus_only_module_is_accepted(self) -> None:
        validator.validate_module_record(
            module_record(MODBUS_REQUIRE), self.policy(), graph_must_not_be_read
        )

    def test_semreg_graph_admits_only_indirect_golang_text(self) -> None:
        policy = self.policy()
        validator.validate_module_record(
            module_record(MODBUS_REQUIRE, SEMREG_REQUIRE, TEXT_REQUIRE),
            policy,
            graph_must_not_be_read,
        )
        for name, record in (
            ("direct x/text", module_record(MODBUS_REQUIRE, SEMREG_REQUIRE, dict(TEXT_REQUIRE, Indirect=False))),
            ("x/text without SemReg", module_record(MODBUS_REQUIRE, TEXT_REQUIRE)),
            (
                "unrelated module beside SemReg",
                module_record(
                    MODBUS_REQUIRE,
                    SEMREG_REQUIRE,
                    {"Path": "example.invalid/unrelated", "Version": "v0.0.0", "Indirect": True},
                ),
            ),
            (
                "unrelated module",
                module_record(
                    MODBUS_REQUIRE,
                    {"Path": "example.invalid/unrelated", "Version": "v0.0.0", "Indirect": True},
                ),
            ),
        ):
            with self.subTest(name=name):
                with self.assertRaises(validator.PolicyError):
                    validator.validate_module_record(record, policy, graph_must_not_be_read)

    def test_indirect_sys_is_admitted_only_through_the_modbus_edge(self) -> None:
        policy = self.policy()
        validator.validate_module_record(
            module_record(MODBUS_REQUIRE, SYS_REQUIRE),
            policy,
            lambda: GRAPH_WITH_SYS_EDGE,
        )
        with self.subTest(name="edge absent"):
            with self.assertRaises(validator.PolicyError):
                validator.validate_module_record(
                    module_record(MODBUS_REQUIRE, SYS_REQUIRE),
                    policy,
                    lambda: GRAPH_WITHOUT_SYS_EDGE,
                )
        with self.subTest(name="edge from another origin"):
            with self.assertRaises(validator.PolicyError):
                validator.validate_module_record(
                    module_record(MODBUS_REQUIRE, SYS_REQUIRE),
                    policy,
                    lambda: f"{MODULE} example.invalid/other@v0.0.0\n"
                    "example.invalid/other@v0.0.0 golang.org/x/sys@v0.30.0\n",
                )
        with self.subTest(name="direct x/sys"):
            with self.assertRaises(validator.PolicyError):
                validator.validate_module_record(
                    module_record(MODBUS_REQUIRE, dict(SYS_REQUIRE, Indirect=False)),
                    policy,
                    lambda: GRAPH_WITH_SYS_EDGE,
                )
        with self.subTest(name="other x/sys version"):
            with self.assertRaises(validator.PolicyError):
                validator.validate_module_record(
                    module_record(MODBUS_REQUIRE, dict(SYS_REQUIRE, Version="v0.31.0")),
                    policy,
                    lambda: GRAPH_WITH_SYS_EDGE,
                )
        with self.subTest(name="injected x/sys without the edge"):
            with self.assertRaises(validator.PolicyError):
                validator.validate_module_record(
                    module_record(MODBUS_REQUIRE, SYS_REQUIRE),
                    policy,
                    lambda: GRAPH_WITHOUT_SYS_EDGE,
                )

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

    def test_local_ci_uses_portable_git_inventory(self) -> None:
        script = (ROOT / "scripts" / "ci_local.sh").read_text(encoding="utf-8")
        self.assertIn("git grep", script)
        self.assertIn("git ls-files", script)
        self.assertNotIn("rg ", script)


if __name__ == "__main__":
    unittest.main()
