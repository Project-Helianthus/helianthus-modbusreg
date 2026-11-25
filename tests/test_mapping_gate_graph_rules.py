#!/usr/bin/env python3
"""Tests for the shared mapping-gate graph rules over synthetic gate members."""
import copy
import io
import json
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path

import importlib.util

_SPEC = importlib.util.spec_from_file_location(
    "mapping_gate_graph_rules",
    Path(__file__).resolve().parents[1] / "scripts/mapping/mapping_gate_graph_rules.py",
)
rules = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(rules)

CATALOG = rules.load_catalog()
EVIDENCE = ["synthetic:evidence-1"]
SCOPE = {"profile": "synthetic.profile", "version": "1", "rule_scope": "synthetic family"}
SCHEME = {"id": "scheme.synthetic_serial", "issuer_registry": "registry.synthetic"}


def resource(kind, anchor_class, basis, source, parent=None):
    return {
        "kind": kind,
        "anchor_class": anchor_class,
        "anchor_basis": basis,
        "anchor_source": source,
        "identification_scheme": "none_listed",
        "parent_kind": parent,
        "evidence": list(EVIDENCE),
    }


def relation(definition, basis, from_kind, to_kind):
    return {
        "definition": definition,
        "basis_class": basis,
        "from_kind": from_kind,
        "to_kind": to_kind,
        "evidence": list(EVIDENCE),
        "validity_scope": dict(SCOPE),
        "absence_sides": [],
    }


def valid_gate():
    return {
        "contract": "synthetic.mapping/v1",
        "field_rules": [{"field": "synthetic.field"}],
        "graph": {
            "resources": [
                resource("resource.equipment", "equipment", "operator_slot", "operator_label"),
                resource("resource.inverter", "equipment", "operator_slot", "operator_label"),
                resource("resource.dc_input", "component", "native_slot", "native_position", "resource.inverter"),
                resource("resource.pv_system", "function", "operator_slot", "operator_label"),
                resource("resource.battery", "equipment", "operator_slot", "operator_label"),
            ],
            "relations": [
                relation("relation.relays", "configuration", "resource.equipment", "resource.inverter"),
                relation("pv.relation.inverter_input", "gate_rule", "resource.inverter", "resource.dc_input"),
                relation("pv.relation.system_inverter", "configuration", "resource.pv_system", "resource.inverter"),
                relation("relation.relays", "configuration", "resource.inverter", "resource.battery"),
            ],
            "relayed": [
                {
                    "relaying_kind": "resource.inverter",
                    "relayed_kind": "resource.battery",
                    "direct_anchor_key_source": "none",
                }
            ],
            "joins": [
                {
                    "kind": "resource.battery",
                    "observations": ["direct", "relayed"],
                    "basis": "operator_declaration",
                    "native_tokens": "not_exposed",
                }
            ],
            "aggregates": [
                {
                    "fact": "pv.energy.generated",
                    "subject_kind": "resource.pv_system",
                    "coverage": ["pv.relation.system_inverter"],
                }
            ],
            "selectors": [
                {"selector": "profile", "classification": "provenance", "evidence": list(EVIDENCE)}
            ],
        },
    }


def catalog_with_scheme():
    catalog = copy.deepcopy(CATALOG)
    catalog["identification_schemes"] = [
        dict(SCHEME, token_form="synthetic canonical form", admission={"arm": "public_registry", "evidence": []})
    ]
    return catalog


class GraphRulesTest(unittest.TestCase):
    def assert_fails(self, gate, error, path, catalog=CATALOG):
        failures = rules.check(gate, catalog)
        self.assertIn((error, path), failures, failures)

    def test_valid_gate_passes(self):
        self.assertEqual(rules.check(valid_gate(), CATALOG), [])

    def test_gate_without_graph_is_reported_not_rejected(self):
        gate = valid_gate()
        del gate["graph"]
        self.assertEqual(rules.check(gate, CATALOG), [])
        self.assertFalse(rules.graph_stated(gate))
        self.assertTrue(rules.graph_stated(valid_gate()))

    def test_forbidden_anchor_sources(self):
        for source in (
            "bus_address", "network_address", "routed_unit_number", "entity_address_path",
            "credential", "certificate_fingerprint", "pack_class", "profile_selector",
            "display_name", "",
        ):
            with self.subTest(source=source):
                gate = valid_gate()
                gate["graph"]["resources"][1]["anchor_source"] = source
                self.assert_fails(gate, "anchor_source_not_allowed", "$.graph.resources[1].anchor_source")

    def test_kind_not_member(self):
        for kind in ("resource.not_a_catalog_kind", "resource.invented", "pv.ac.voltage", "relation.relays"):
            with self.subTest(kind=kind):
                gate = valid_gate()
                gate["graph"]["resources"][3]["kind"] = kind
                self.assert_fails(gate, "kind_not_member", "$.graph.resources[3].kind")

    def test_parent_kind_not_member(self):
        gate = valid_gate()
        gate["graph"]["resources"][2]["parent_kind"] = "resource.not_a_catalog_kind"
        self.assert_fails(gate, "kind_not_member", "$.graph.resources[2].parent_kind")

    def test_anchor_class_differs_from_catalog(self):
        for index, anchor_class in ((1, "component"), (2, "equipment"), (3, "episode")):
            with self.subTest(index=index):
                gate = valid_gate()
                gate["graph"]["resources"][index]["anchor_class"] = anchor_class
                self.assert_fails(gate, "anchor_class_mismatch", f"$.graph.resources[{index}].anchor_class")

    def test_anchor_basis_and_source_disagree(self):
        gate = valid_gate()
        gate["graph"]["resources"][1]["anchor_basis"] = "native_slot"
        self.assert_fails(gate, "anchor_basis_mismatch", "$.graph.resources[1].anchor_basis")
        gate = valid_gate()
        gate["graph"]["resources"][1]["anchor_basis"] = "address_basis"
        self.assert_fails(gate, "invalid_value", "$.graph.resources[1].anchor_basis")

    def test_component_without_parent_kind(self):
        gate = valid_gate()
        gate["graph"]["resources"][2]["parent_kind"] = None
        self.assert_fails(gate, "parent_kind_missing", "$.graph.resources[2].parent_kind")

    def test_scheme_not_listed(self):
        gate = valid_gate()
        gate["graph"]["resources"][1].update(
            anchor_basis="native_identity", anchor_source="native_token", identification_scheme=dict(SCHEME)
        )
        self.assert_fails(gate, "scheme_not_listed", "$.graph.resources[1].identification_scheme")
        self.assertEqual(rules.check(gate, catalog_with_scheme()), [])
        other = dict(SCHEME, id="scheme.other_serial")
        gate["graph"]["resources"][1]["identification_scheme"] = other
        self.assert_fails(gate, "scheme_not_listed", "$.graph.resources[1].identification_scheme", catalog_with_scheme())
        gate["graph"]["resources"][1]["identification_scheme"] = dict(SCHEME, issuer_registry="none")
        self.assert_fails(gate, "scheme_not_listed", "$.graph.resources[1].identification_scheme", catalog_with_scheme())

    def test_native_identity_requires_listed_scheme(self):
        gate = valid_gate()
        gate["graph"]["resources"][1].update(anchor_basis="native_identity", anchor_source="native_token")
        self.assert_fails(gate, "scheme_not_listed", "$.graph.resources[1].identification_scheme", catalog_with_scheme())

    def test_relation_definition_not_held(self):
        for definition in ("pv.relation.logger_inverter", "resource.inverter", "relation.relays_v2"):
            with self.subTest(definition=definition):
                gate = valid_gate()
                gate["graph"]["relations"][0]["definition"] = definition
                self.assert_fails(gate, "relation_not_member", "$.graph.relations[0].definition")

    def test_basis_class_not_admitted(self):
        for index, basis in ((1, "configuration"), (1, "native_observation"), (2, "native_observation"), (0, "gate_rule"), (0, "address_arithmetic")):
            with self.subTest(index=index, basis=basis):
                gate = valid_gate()
                gate["graph"]["relations"][index]["basis_class"] = basis
                self.assert_fails(gate, "basis_class_not_admitted", f"$.graph.relations[{index}].basis_class")

    def test_endpoint_kind_outside_definition(self):
        gate = valid_gate()
        gate["graph"]["relations"][1]["from_kind"] = "resource.equipment"
        self.assert_fails(gate, "endpoint_kind_mismatch", "$.graph.relations[1].from_kind")
        gate = valid_gate()
        gate["graph"]["relations"][2]["to_kind"] = "resource.battery"
        self.assert_fails(gate, "endpoint_kind_mismatch", "$.graph.relations[2].to_kind")

    def test_dangling_endpoint(self):
        gate = valid_gate()
        gate["graph"]["relations"].append(
            relation("pv.relation.inverter_array", "configuration", "resource.inverter", "resource.pv_array")
        )
        self.assert_fails(gate, "dangling_endpoint", "$.graph.relations[4].to_kind")
        gate = valid_gate()
        gate["graph"]["relations"].append(
            relation("pv.relation.array_string", "configuration", "resource.pv_array", "resource.pv_string")
        )
        self.assert_fails(gate, "dangling_endpoint", "$.graph.relations[4].from_kind")
        self.assert_fails(gate, "dangling_endpoint", "$.graph.relations[4].to_kind")

    def test_relation_statement_members(self):
        gate = valid_gate()
        gate["graph"]["relations"][0]["evidence"] = []
        self.assert_fails(gate, "invalid_value", "$.graph.relations[0].evidence")
        gate = valid_gate()
        del gate["graph"]["relations"][0]["validity_scope"]["rule_scope"]
        self.assert_fails(gate, "missing_member", "$.graph.relations[0].validity_scope.rule_scope")
        gate = valid_gate()
        gate["graph"]["relations"][0]["absence_sides"] = ["from", "from"]
        self.assert_fails(gate, "invalid_value", "$.graph.relations[0].absence_sides")
        gate = valid_gate()
        gate["graph"]["relations"].append(copy.deepcopy(gate["graph"]["relations"][3]))
        self.assert_fails(gate, "duplicate_entry", "$.graph.relations[4]")

    def test_false_join(self):
        for basis in ("label", "same_anchor_key", "operator_declaration"):
            with self.subTest(basis=basis):
                gate = valid_gate()
                gate["graph"]["joins"][0].update(basis=basis, native_tokens="differ")
                self.assert_fails(gate, "false_join", "$.graph.joins[0].native_tokens")

    def test_join_by_label_ordinal_or_address(self):
        for basis in ("label", "ordinal", "address", "similar_values"):
            with self.subTest(basis=basis):
                gate = valid_gate()
                gate["graph"]["joins"][0]["basis"] = basis
                self.assert_fails(gate, "join_basis_not_allowed", "$.graph.joins[0].basis")

    def test_join_kind_not_published(self):
        gate = valid_gate()
        gate["graph"]["joins"][0]["kind"] = "resource.meter"
        self.assert_fails(gate, "kind_not_published", "$.graph.joins[0].kind")

    def test_plane_member_anywhere(self):
        for key in ("planes", "plane"):
            with self.subTest(key=key):
                gate = valid_gate()
                gate["graph"]["resources"][0][key] = ["physical"]
                self.assert_fails(gate, "plane_membership_stated", f"$.graph.resources[0].{key}")
                gate = valid_gate()
                gate["field_rules"][0][key] = "energy"
                self.assert_fails(gate, "plane_membership_stated", f"$.field_rules[0].{key}")
                gate = valid_gate()
                gate[key] = []
                del gate["graph"]
                self.assert_fails(gate, "plane_membership_stated", f"$.{key}")

    def test_relayed_statement(self):
        gate = valid_gate()
        gate["graph"]["relayed"][0]["direct_anchor_key_source"] = "routed_unit_number"
        self.assert_fails(gate, "anchor_source_not_allowed", "$.graph.relayed[0].direct_anchor_key_source")
        gate = valid_gate()
        gate["graph"]["relayed"][0]["relaying_kind"] = "resource.meter"
        self.assert_fails(gate, "kind_not_published", "$.graph.relayed[0].relaying_kind")
        gate = valid_gate()
        gate["graph"]["relayed"][0]["relayed_kind"] = "resource.meter"
        self.assert_fails(gate, "kind_not_published", "$.graph.relayed[0].relayed_kind")

    def test_aggregate_without_subject_kind(self):
        gate = valid_gate()
        del gate["graph"]["aggregates"][0]["subject_kind"]
        self.assert_fails(gate, "aggregate_subject_missing", "$.graph.aggregates[0].subject_kind")
        gate = valid_gate()
        gate["graph"]["aggregates"][0]["subject_kind"] = ""
        self.assert_fails(gate, "aggregate_subject_missing", "$.graph.aggregates[0].subject_kind")

    def test_aggregate_statement(self):
        gate = valid_gate()
        gate["graph"]["aggregates"][0]["fact"] = "pv.dc.power"
        self.assert_fails(gate, "not_an_aggregate", "$.graph.aggregates[0].fact")
        gate = valid_gate()
        gate["graph"]["aggregates"][0]["subject_kind"] = "resource.inverter"
        self.assert_fails(gate, "subject_kind_mismatch", "$.graph.aggregates[0].subject_kind")
        gate = valid_gate()
        gate["graph"]["aggregates"][0]["coverage"] = ["pv.relation.inverter_input"]
        self.assert_fails(gate, "coverage_not_stated", "$.graph.aggregates[0].coverage[0]")
        gate = valid_gate()
        gate["graph"]["aggregates"][0]["coverage"] = ["relation.relays"]
        self.assert_fails(gate, "coverage_not_stated", "$.graph.aggregates[0].coverage[0]")
        gate = valid_gate()
        gate["graph"]["aggregates"][0]["coverage"] = []
        self.assertEqual(rules.check(gate, CATALOG), [])

    def test_aggregate_coverage_is_groups_or_contains_of_its_subject(self):
        gate = valid_gate()
        gate["graph"]["aggregates"].append(
            {"fact": "pv.ac.aggregate_active_power", "subject_kind": "resource.inverter",
             "coverage": ["pv.relation.inverter_input"]}
        )
        self.assertEqual(rules.check(gate, CATALOG), [])
        gate["graph"]["aggregates"][1]["coverage"] = ["relation.relays"]
        self.assert_fails(gate, "coverage_not_stated", "$.graph.aggregates[1].coverage[0]")

    def test_selector_classification(self):
        gate = valid_gate()
        gate["graph"]["selectors"][0]["classification"] = "anchor"
        self.assert_fails(gate, "invalid_value", "$.graph.selectors[0].classification")

    def test_unknown_and_missing_members(self):
        gate = valid_gate()
        gate["graph"]["extra"] = []
        self.assert_fails(gate, "unknown_member", "$.graph.extra")
        gate = valid_gate()
        del gate["graph"]["selectors"]
        self.assert_fails(gate, "missing_member", "$.graph.selectors")
        gate = valid_gate()
        gate["graph"]["resources"][0]["address"] = "synthetic"
        self.assert_fails(gate, "unknown_member", "$.graph.resources[0].address")
        gate = valid_gate()
        gate["graph"]["resources"].append(copy.deepcopy(gate["graph"]["resources"][4]))
        self.assert_fails(gate, "duplicate_entry", "$.graph.resources[5]")
        gate = valid_gate()
        gate["graph"] = []
        self.assert_fails(gate, "invalid_value", "$.graph")

    def test_self_test_passes(self):
        with redirect_stdout(io.StringIO()):
            self.assertEqual(rules.main(["--self-test"]), 0)

    def test_cli_on_gate_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            good = Path(tmp) / "good.json"
            good.write_text(json.dumps(valid_gate()), encoding="utf-8")
            bad_gate = valid_gate()
            bad_gate["graph"]["resources"][1]["anchor_source"] = "network_address"
            bad = Path(tmp) / "bad.json"
            bad.write_text(json.dumps(bad_gate), encoding="utf-8")
            absent_gate = valid_gate()
            del absent_gate["graph"]
            absent = Path(tmp) / "absent.json"
            absent.write_text(json.dumps(absent_gate), encoding="utf-8")
            duplicate = Path(tmp) / "duplicate.json"
            duplicate.write_text('{"graph": {}, "graph": {}}', encoding="utf-8")
            out = io.StringIO()
            with redirect_stdout(out):
                self.assertEqual(rules.main([str(good)]), 0)
                self.assertEqual(rules.main([str(bad)]), 1)
                self.assertEqual(rules.main([str(absent)]), 0)
                self.assertEqual(rules.main([str(duplicate)]), 1)
            self.assertIn("anchor_source_not_allowed $.graph.resources[1].anchor_source", out.getvalue())
            self.assertIn("graph_not_stated $.graph", out.getvalue())

    def test_accepted_gates_are_reported_until_their_slice(self):
        for path in sorted(rules.ROOT.glob("driverdata/families/mapping-gates/*.json")):
            with self.subTest(path=path.name):
                gate = rules.load_json(path)
                self.assertEqual(rules.check(gate, CATALOG), [])


if __name__ == "__main__":
    unittest.main()
