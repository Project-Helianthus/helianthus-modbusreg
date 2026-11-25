#!/usr/bin/env python3
"""Focused mutations for the Tesla Gen3 WC3 EVSE mapping gate."""
import copy
import importlib.util
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("tesla_mapping", ROOT / "scripts/mapping/validate_tesla_gen3_wc3_24443_evse_current_limit_mapping.py")
mapping = importlib.util.module_from_spec(spec); sys.modules[spec.name] = mapping; spec.loader.exec_module(mapping)

def reject(name, path, value, expected):
    contract = copy.deepcopy(mapping.load()); mapping.mutate(contract, {"path": path, "value": value})
    try: mapping.validate_contract(contract)
    except ValueError as error:
        if str(error) != expected: raise AssertionError(f"{name}: {error}")
    else: raise AssertionError(f"{name} accepted")
    print(f"{name}: REJECTED")

def readback_sources():
    """Both limits are sourced from the provisional readback, not the ingested persistent exchange."""
    rules = mapping.load()["field_rules"]
    sources = [rule["source"] for rule in rules]
    if sources != ["provisional.readback.ConfiguredCurrentAmps", "provisional.readback.LimitCurrentMaxAmps"]:
        raise AssertionError("configured and allocated current are not both sourced from the provisional readback: " + repr(sources))
    print("readback-sources: PASS")


def readback_fields_are_independent():
    """ConfiguredCurrentAmps, LimitCurrentMaxAmps and the persistent amperes each move only their own published fact."""
    contract = mapping.load()
    baseline = copy.deepcopy(contract["scenarios"][0]["input"])
    configured = copy.deepcopy(baseline)
    mapping.mutate(configured, {"path": "provisional.ConfiguredCurrentAmps", "value": 32})
    mapping.mutate(configured, {"path": "persistent.MaxOutputCurrentAmps", "value": 7})
    result = mapping.project(configured, contract)
    if [fact["value"]["coefficient"] for fact in result["facts"]] != ["32", "12"]:
        raise AssertionError("ConfiguredCurrentAmps did not publish configured current")
    allocated = copy.deepcopy(baseline)
    mapping.mutate(allocated, {"path": "provisional.LimitCurrentMaxAmps", "value": 20})
    mapping.mutate(allocated, {"path": "persistent.MaxOutputCurrentAmps", "value": 9})
    result = mapping.project(allocated, contract)
    if [fact["value"]["coefficient"] for fact in result["facts"]] != ["16", "20"]:
        raise AssertionError("LimitCurrentMaxAmps did not publish allocated current")
    persistent = copy.deepcopy(baseline)
    mapping.mutate(persistent, {"path": "persistent.MaxOutputCurrentAmps", "value": 7})
    result = mapping.project(persistent, contract)
    if [fact["value"]["coefficient"] for fact in result["facts"]] != ["16", "12"]:
        raise AssertionError("persistent MaxOutputCurrentAmps changed a published limit")
    print("readback-fields: PASS")


def main():
    readback_sources()
    readback_fields_are_independent()
    cases = [("pin", "pins.gateway_main", "0" * 40, "contract"), ("profile", "native_contract.profile", "wc3_24_44_4", "native_contract"), ("outbound", "native_contract.outbound_allowed", True, "native_contract"), ("identity_derivation", "identity.forbidden_derivations.0", "serial", "identity_contract"), ("identity_reuse", "identity.failure", "allow_reuse", "identity_contract"), ("lifecycle", "lifecycle.required.0", "timestamp", "lifecycle_contract"), ("persistent_target", "field_rules.0.target", "evse.limit.allocated_current", "field_rules"), ("provisional_disposition", "field_rules.1.disposition", "always", "field_rules"), ("unsupported", "unsupported.0", "power_allowed", "withheld_or_unsupported"), ("precedence", "projection.error_precedence.0", "x", "projection"), ("operations", "projection.operations", "available", "projection"), ("cutover", "consumer_cutover.forbidden.0", "allowed", "consumer_cutover")]
    for case in cases: reject(*case)
    contract = mapping.load(); baseline = contract["scenarios"][0]["input"]
    for name, path, value, expected in [("blank_identity", "identity.asset_id", "", "identity_invalid"), ("wrong_unit_profile", "profile", "other", "profile_or_version_invalid"), ("missing_receipt", "lifecycle.receipt_timestamp", "", "lifecycle_invalid"), ("operation_attempt", "operation", "evse.operation.set_allocated_current", "unsupported_fact_or_operation"), ("persistent_evidence", "evidence.persistent.persistent_terminal", "bad", "evidence_binding_invalid")]:
        candidate = copy.deepcopy(baseline); mapping.mutate(candidate, {"path": path, "value": value})
        try: mapping.project(candidate, contract)
        except ValueError as error:
            if str(error) != expected: raise AssertionError(error)
        else: raise AssertionError(name + " accepted")
        print(name + ": REJECTED")
    for name, path, value, reason in [("persistent_only", "provisional", None, "provisional_record_absent"), ("inhibited_provisional", "provisional.InhibitCharging", True, "inhibit_state_not_semantically_representable"), ("malformed_provisional", "evidence.provisional.provisional_ack", "bad", "provisional_evidence_invalid"), ("mismatched_provisional", "evidence.provisional.correlation_id", "evidence.wc3.other", "provisional_evidence_invalid")]:
        candidate = copy.deepcopy(baseline); mapping.mutate(candidate, {"path": path, "value": value})
        if name == "persistent_only": mapping.mutate(candidate, {"path": "evidence.provisional", "value": None})
        result = mapping.project(candidate, contract)
        if name == "inhibited_provisional":
            expected_facts, expected_withheld = ["evse.limit.configured_current"], [{"id": "evse.limit.allocated_current", "reason": reason}]
        else:
            expected_facts, expected_withheld = [], [{"id": "evse.limit.configured_current", "reason": reason}, {"id": "evse.limit.allocated_current", "reason": reason}]
        if [fact["id"] for fact in result["facts"]] != expected_facts or result["withheld"] != expected_withheld:
            raise AssertionError(name + " did not follow the readback")
        print(name + ": READBACK")
    print("baseline: PASS; 21 focused mutations rejected or followed the readback")
    graph_cases()


SESSION = {"kind": "resource.charging_session", "anchor_class": "episode", "anchor_basis": "native_slot", "anchor_source": "native_position", "identification_scheme": "none_listed", "parent_kind": "resource.connector", "evidence": ["synthetic:session"]}
METER_RELATION = {"definition": "evse.relation.evse_meter", "basis_class": "configuration", "from_kind": "resource.evse", "to_kind": "resource.meter", "evidence": ["synthetic:meter"], "validity_scope": {"profile": "wc3_24_44_3", "version": "24.44.3", "rule_scope": "synthetic"}, "absence_sides": []}
ORDINAL_JOIN = {"kind": "resource.connector", "observations": ["connector.first", "connector.second"], "basis": "ordinal_of_sorted_identifiers", "native_tokens": "not_exposed"}
FALSE_JOIN = {"kind": "resource.connector", "observations": ["connector.first", "connector.second"], "basis": "operator_declaration", "native_tokens": "differ"}
DECLARED_JOIN = {"kind": "resource.connector", "observations": ["connector.first", "connector.second"], "basis": "operator_declaration", "native_tokens": "not_exposed"}
RELAYED = {"relaying_kind": "resource.charging_station", "relayed_kind": "resource.evse", "direct_anchor_key_source": "none"}
AGGREGATE = {"fact": "evse.limit.configured_current", "subject_kind": "resource.evse", "coverage": []}

# (name, [(op, path, value)], expected error). Every member of a resource
# statement (kind, anchor class, basis, source, scheme, parent, evidence) and of
# a relation statement (definition, basis class, both endpoints, evidence, each
# validity-scope member, absence sides) is varied at least once.
GRAPH_NEGATIVES = [
    ("connector_numbered_by_sorted_order", [("set", "graph.resources.2.anchor_source", "ordinal_of_sorted_identifiers")], "graph anchor_source_not_allowed $.graph.resources[2].anchor_source"),
    ("connector_ordinal_join", [("set", "graph.joins", [ORDINAL_JOIN])], "graph join_basis_not_allowed $.graph.joins[0].basis"),
    ("station_identity_from_address", [("set", "graph.resources.0.anchor_source", "network_address")], "graph anchor_source_not_allowed $.graph.resources[0].anchor_source"),
    ("station_native_token_without_scheme", [("set", "graph.resources.0.anchor_basis", "native_identity"), ("set", "graph.resources.0.anchor_source", "native_token")], "graph scheme_not_listed $.graph.resources[0].identification_scheme"),
    ("resource_scheme_unlisted", [("set", "graph.resources.0.identification_scheme", {"id": "scheme.unlisted", "issuer_registry": "none"})], "graph scheme_not_listed $.graph.resources[0].identification_scheme"),
    ("resource_kind_dangling_endpoint", [("set", "graph.resources.2.kind", "resource.phase")], "graph dangling_endpoint $.graph.relations[1].to_kind"),
    ("resource_anchor_class", [("set", "graph.resources.2.anchor_class", "equipment")], "graph anchor_class_mismatch $.graph.resources[2].anchor_class"),
    ("resource_anchor_basis", [("set", "graph.resources.1.anchor_basis", "native_slot")], "graph anchor_basis_mismatch $.graph.resources[1].anchor_basis"),
    ("evse_position_not_in_native_evidence", [("set", "graph.resources.1.anchor_basis", "native_slot"), ("set", "graph.resources.1.anchor_source", "native_position")], "graph_anchor"),
    ("resource_parent_skips_evse", [("set", "graph.resources.2.parent_kind", "resource.charging_station")], "graph_parent"),
    ("resource_parent_missing", [("set", "graph.resources.1.parent_kind", None)], "graph parent_kind_missing $.graph.resources[1].parent_kind"),
    ("resource_evidence_empty", [("set", "graph.resources.0.evidence", [])], "graph invalid_value $.graph.resources[0].evidence"),
    ("session_without_native_token", [("append", "graph.resources", SESSION)], "graph_resources"),
    ("relation_definition", [("set", "graph.relations.0.definition", "relation.invented")], "graph relation_not_member $.graph.relations[0].definition"),
    ("relation_basis_class", [("set", "graph.relations.0.basis_class", "gate_rule")], "graph basis_class_not_admitted $.graph.relations[0].basis_class"),
    ("relation_from_kind", [("set", "graph.relations.1.from_kind", "resource.charging_station")], "graph endpoint_kind_mismatch $.graph.relations[1].from_kind"),
    ("relation_to_kind", [("set", "graph.relations.0.to_kind", "resource.connector")], "graph endpoint_kind_mismatch $.graph.relations[0].to_kind"),
    ("meter_relation_dangling", [("append", "graph.relations", METER_RELATION)], "graph dangling_endpoint $.graph.relations[2].to_kind"),
    ("relation_evidence_empty", [("set", "graph.relations.1.evidence", [])], "graph invalid_value $.graph.relations[1].evidence"),
    ("relation_scope_profile", [("set", "graph.relations.0.validity_scope.profile", "wc3_24_44_4")], "graph_relations"),
    ("relation_scope_version", [("set", "graph.relations.1.validity_scope.version", "24.44.4")], "graph_relations"),
    ("relation_scope_rule", [("set", "graph.relations.0.validity_scope.rule_scope", "")], "graph invalid_value $.graph.relations[0].validity_scope.rule_scope"),
    ("relation_absence_claimed", [("set", "graph.relations.1.absence_sides", ["to"])], "graph_relations"),
    ("false_join", [("set", "graph.joins", [FALSE_JOIN])], "graph false_join $.graph.joins[0].native_tokens"),
    ("declared_join", [("set", "graph.joins", [DECLARED_JOIN])], "graph_joins"),
    ("relayed_stated", [("set", "graph.relayed", [RELAYED])], "graph_relayed"),
    ("limit_as_aggregate", [("set", "graph.aggregates", [AGGREGATE])], "graph not_an_aggregate $.graph.aggregates[0].fact"),
    ("selector_as_identity", [("set", "graph.selectors.0.classification", "identity_evidence")], "graph_selectors"),
    ("selector_renamed", [("set", "graph.selectors.0.selector", "unit_id")], "graph_selectors"),
    ("plane_on_resource", [("set", "graph.resources.0.planes", ["physical"])], "graph plane_membership_stated $.graph.resources[0].planes"),
    ("plane_on_field_rule", [("set", "field_rules.0.planes", ["energy"])], "graph plane_membership_stated $.field_rules[0].planes"),
    ("meter_relation_dropped", [("delete", "unstated_relations.2", None)], "unstated_relations"),
    ("meter_absence_asserted", [("set", "unstated_relations.2.outcome", "absent")], "unstated_relations"),
    ("identity_configured_text", [("set", "identity.asset_id.source", "gateway_configured_semantic_asset_id")], "identity_contract"),
    ("identity_dimension_unpublished", [("set", "identity.connector_id.dimension", "evse.dimension.phase")], "identity_contract"),
    ("identity_address_allowed", [("delete", "identity.forbidden_derivations.5", None)], "identity_contract"),
    ("field_rule_subject_kind", [("set", "field_rules.1.subject_kind", "resource.evse")], "field_rules"),
    ("field_rule_measurement_point", [("set", "field_rules.0.measurement_point", "")], "field_rules"),
    ("field_rule_dimension", [("set", "field_rules.0.dimension", "connector")], "field_rules"),
]


def edit(contract, op, path, value):
    if op == "append":
        target = contract
        for part in path.split("."):
            target = target[int(part)] if isinstance(target, list) else target[part]
        target.append(copy.deepcopy(value))
    elif op == "delete":
        mapping.mutate(contract, {"path": path, "op": "delete"})
    else:
        mapping.mutate(contract, {"path": path, "value": copy.deepcopy(value)})


def rejection(edits, catalog=None):
    contract = copy.deepcopy(mapping.load())
    try:
        for op, path, value in edits:
            edit(contract, op, path, value)
        mapping.validate_contract(contract, catalog)
    except (ValueError, KeyError, IndexError, TypeError, AttributeError) as error:
        return f"{type(error).__name__}: {error}" if not isinstance(error, ValueError) else str(error)
    return None


def catalog_lookup_case(member, value):
    """The subject kind and dimension of a field rule come from the element catalog."""
    catalog = copy.deepcopy(mapping.graph_rules.load_catalog())
    for element in catalog["elements"]:
        if element["ref"]["id"] == "evse.limit.allocated_current":
            element["attributes"]["subject"][member] = value
    return rejection([], catalog)


CATALOG_NEGATIVES = [
    ("catalog_subject_kind_lookup", "kind", "resource.evse"),
    ("catalog_subject_dimension_lookup", "dimension", {"owner": {"kind": "pack", "pack": mapping.PACK}, "id": "evse.dimension.evse", "version": "1.0.0"}),
]


def graph_cases():
    failures = []
    contract = mapping.load()
    found = mapping.graph_rules.check(contract, mapping.graph_rules.load_catalog())
    if "graph" not in contract or found:
        failures.append(f"shared checker: graph stated={'graph' in contract} failures={found}")
    for name, edits, expected in GRAPH_NEGATIVES:
        result = rejection(edits)
        if result != expected:
            failures.append(f"{name}: expected {expected!r}, got {result!r}")
        else:
            print(f"{name}: REJECTED")
    for name, member, value in CATALOG_NEGATIVES:
        result = catalog_lookup_case(member, value)
        if result != "field_rules":
            failures.append(f"{name}: expected 'field_rules', got {result!r}")
        else:
            print(f"{name}: REJECTED")
    if failures:
        raise AssertionError("\n".join(failures))
    print(f"graph: PASS; {len(GRAPH_NEGATIVES) + len(CATALOG_NEGATIVES)} graph, identity and subject mutations rejected")


if __name__ == "__main__": main()
