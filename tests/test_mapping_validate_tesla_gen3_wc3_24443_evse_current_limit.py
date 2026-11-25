#!/usr/bin/env python3
"""Focused mutations for the Tesla Gen3 WC3 EVSE mapping gate."""
import copy
import importlib.util
import sys
import tempfile
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
    """Allocated current comes only from the provisional readback; configured current also has the persistent candidate."""
    rules = mapping.load()["field_rules"]
    sources = [rule["source"] for rule in rules]
    expected = ["provisional.readback.ConfiguredCurrentAmps", "persistent.MaxOutputCurrentAmps", "provisional.readback.LimitCurrentMaxAmps"]
    if sources != expected:
        raise AssertionError("configured and allocated current sources differ from the readback contract: " + repr(sources))
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


def facts_of(result):
    return [(fact["id"], fact["value"]["coefficient"]) for fact in result["facts"]]


def withheld_of(result):
    return [(entry["id"], entry["reason"]) for entry in result["withheld"]]


def managed_set(set_ref="sha256:" + "e" * 64, ack_ref="sha256:" + "f" * 64):
    """The managed set of the baseline readback: its acknowledgement receipt, 60 s duration, and the set and acknowledgement it names."""
    return {"ack_receipt_timestamp": "2026-09-10T00:00:00Z", "duration_seconds": 60, "provisional_set": set_ref, "provisional_ack": ack_ref}


def readback_cases(baseline, contract):
    """Each readback rule of the allocated and configured current, through project()."""
    cases = [
        ("persistent_only", [("provisional", None), ("evidence.provisional", None)], [("evse.limit.configured_current", "16")], [("evse.limit.allocated_current", "provisional_record_absent")]),
        ("inhibited_provisional", [("provisional.InhibitCharging", True)], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "0")], []),
        ("malformed_provisional", [("evidence.provisional.provisional_ack", "bad")], [("evse.limit.configured_current", "16")], [("evse.limit.allocated_current", "provisional_evidence_invalid")]),
        ("mismatched_provisional", [("evidence.provisional.correlation_id", "evidence.wc3.other")], [("evse.limit.configured_current", "16")], [("evse.limit.allocated_current", "provisional_evidence_invalid")]),
        ("zero_timeout_keeps_allocated", [("provisional.LimitTimeoutSeconds", 0)], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "12")], []),
        ("zero_current_unresolved", [("provisional.LimitCurrentMaxAmps", 0)], [("evse.limit.configured_current", "16")], [("evse.limit.allocated_current", "zero_current_unresolved")]),
        ("zero_configured_unknown", [("provisional.ConfiguredCurrentAmps", 0)], [("evse.limit.allocated_current", "12")], [("evse.limit.configured_current", "zero_configured_unknown")]),
        ("later_persistent_candidate", [("persistent.MaxOutputCurrentAmps", 20), ("evidence.persistent.receipt_timestamp", "2026-09-10T00:00:02Z")], [("evse.limit.configured_current", "20"), ("evse.limit.allocated_current", "12")], []),
        ("managed_within_cutoff", [("managed_set", managed_set()), ("evidence.provisional.receipt_timestamp", "2026-09-10T00:00:59Z")], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "12")], []),
        ("managed_received_70s_after_60s_set", [("managed_set", managed_set()), ("evidence.provisional.receipt_timestamp", "2026-09-10T00:01:10Z")], [("evse.limit.configured_current", "16")], [("evse.limit.allocated_current", "allocation_expired")]),
        ("managed_at_cutoff", [("managed_set", managed_set()), ("evidence.provisional.receipt_timestamp", "2026-09-10T00:01:00Z")], [("evse.limit.configured_current", "16")], [("evse.limit.allocated_current", "allocation_expired")]),
        ("newer_readback_of_another_acknowledged_set_is_not_expired", [("managed_set", managed_set(ack_ref="sha256:" + "a" * 64)), ("evidence.provisional.receipt_timestamp", "2026-09-10T00:05:00Z")], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "12")], []),
        ("newer_readback_of_another_set_is_not_expired", [("managed_set", managed_set(set_ref="sha256:" + "a" * 64)), ("evidence.provisional.receipt_timestamp", "2026-09-10T00:05:00Z")], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "12")], []),
        ("newer_inhibited_readback_of_another_acknowledged_set_maps_to_zero", [("provisional.InhibitCharging", True), ("managed_set", managed_set(ack_ref="sha256:" + "a" * 64)), ("evidence.provisional.receipt_timestamp", "2026-09-10T00:05:00Z")], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "0")], []),
        ("inhibited_managed_within_cutoff", [("provisional.InhibitCharging", True), ("managed_set", managed_set()), ("evidence.provisional.receipt_timestamp", "2026-09-10T00:00:59Z")], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "0")], []),
        ("inhibited_managed_received_70s_after_60s_set", [("provisional.InhibitCharging", True), ("managed_set", managed_set()), ("evidence.provisional.receipt_timestamp", "2026-09-10T00:01:10Z")], [("evse.limit.configured_current", "16")], [("evse.limit.allocated_current", "allocation_expired")]),
        ("provisional_evidence_absent_keeps_persistent", [("evidence.provisional", None)], [("evse.limit.configured_current", "16")], [("evse.limit.allocated_current", "provisional_evidence_invalid")]),
    ]
    for name, mutations, facts, withheld in cases:
        candidate = copy.deepcopy(baseline)
        for path, value in mutations:
            mapping.mutate(candidate, {"path": path, "value": value})
        result = mapping.project(candidate, contract)
        if facts_of(result) != facts or withheld_of(result) != withheld:
            raise AssertionError(f"{name}: facts {facts_of(result)} withheld {withheld_of(result)}")
        print(name + ": READBACK")


def operation_cases(baseline, contract):
    """Listed operations are emitted by project() and accepted; an unlisted one is rejected there."""
    result = mapping.project(baseline, contract)
    if result["operations"] != ["evse.operation.set_allocated_current", "ConfigurePersistentCurrent"]:
        raise AssertionError("project() did not emit the listed operations: " + repr(result["operations"]))
    listed = copy.deepcopy(baseline)
    mapping.mutate(listed, {"path": "operation", "value": "evse.operation.set_allocated_current"})
    mapping.project(listed, contract)
    unlisted = copy.deepcopy(baseline)
    mapping.mutate(unlisted, {"path": "operation", "value": "evse.operation.release_allocated_current"})
    try:
        mapping.project(unlisted, contract)
    except ValueError as error:
        if str(error) != "unsupported_fact_or_operation":
            raise
    else:
        raise AssertionError("unlisted operation accepted")
    print("operations: PASS")


SET = {"connector_id": "connector.wc3.alpha", "timeout_seconds": 600}
RELEASE = {"connector_id": "connector.wc3.alpha", "allocated_current_enabled": False}
KNOWN = {"t28_known": True, "configured_current": 16}
UNREAD = {"t28_known": False, "configured_current": None}


def operation_vectors():
    """Allocated-current disposition: a refused request sends nothing, a read-first case sends no t25 until the read."""
    vectors = [
        ("positive_16A", dict(SET, amperes=16), KNOWN, "send_set"),
        ("positive_pause", dict(SET, amperes=0), KNOWN, "send_pause"),
        ("negative_3A", dict(SET, amperes=3), KNOWN, "rejected_below_minimum"),
        ("negative_3A_no_read", dict(SET, amperes=3), UNREAD, "rejected_below_minimum"),
        ("negative_above_configured_known", dict(SET, amperes=20), KNOWN, "rejected_above_configured"),
        ("above_configured_read_first", dict(SET, amperes=16), UNREAD, "read_first"),
        ("above_configured_after_read", dict(SET, amperes=16), {"t28_known": True, "configured_current": 12}, "rejected_above_configured"),
        ("negative_zero_timeout", dict(SET, amperes=16, timeout_seconds=0), KNOWN, "rejected_invalid"),
        ("negative_other_connector", dict(SET, amperes=16, connector_id="connector.wc3.beta"), KNOWN, "rejected_other_connector"),
        ("pause_bound_below_6A", dict(SET, amperes=0), {"t28_known": True, "configured_current": 5}, "rejected_pause_bound"),
        ("malformed_bound_nan_16A", dict(SET, amperes=16), {"t28_known": True, "configured_current": float("nan")}, "rejected_no_configured_bound"),
        ("malformed_bound_nan_pause", dict(SET, amperes=0), {"t28_known": True, "configured_current": float("nan")}, "rejected_no_configured_bound"),
        ("malformed_bound_float", dict(SET, amperes=16), {"t28_known": True, "configured_current": 16.0}, "rejected_no_configured_bound"),
        ("malformed_bound_bool", dict(SET, amperes=16), {"t28_known": True, "configured_current": True}, "rejected_no_configured_bound"),
        ("malformed_bound_string", dict(SET, amperes=16), {"t28_known": True, "configured_current": "16"}, "rejected_no_configured_bound"),
        ("release_known_bound", RELEASE, KNOWN, "send_release"),
        ("release_read_first", RELEASE, UNREAD, "read_first"),
        ("release_zero_bound", RELEASE, {"t28_known": True, "configured_current": 0}, "rejected_no_release_bound"),
        ("release_nan_bound", RELEASE, {"t28_known": True, "configured_current": float("nan")}, "rejected_no_release_bound"),
        ("release_with_amperes", dict(RELEASE, amperes=0), KNOWN, "rejected_invalid"),
        ("release_other_connector", dict(RELEASE, connector_id="connector.wc3.beta"), KNOWN, "rejected_other_connector"),
    ]
    for name, request, generation, expected in vectors:
        got = mapping.set_allocated_current_disposition(request, generation, "connector.wc3.alpha")
        if got != expected:
            raise AssertionError(f"{name}: expected {expected}, got {got}")
        print(name + ": " + got)


def candidate_cases(baseline, contract):
    """Section 6.1: either configured-current record alone is sufficient; with both in one epoch the later receipt wins."""
    cases = [
        ("provisional_only_t28", [("delete", "persistent", None), ("delete", "evidence.persistent", None)], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "12")], []),
        ("provisional_only_without_configured_field", [("delete", "persistent", None), ("delete", "evidence.persistent", None), ("delete", "provisional.ConfiguredCurrentAmps", None)], [("evse.limit.allocated_current", "12")], [("evse.limit.configured_current", "configured_current_absent")]),
        ("later_provisional_candidate", [("set", "persistent.MaxOutputCurrentAmps", 7), ("set", "provisional.ConfiguredCurrentAmps", 32)], [("evse.limit.configured_current", "32"), ("evse.limit.allocated_current", "12")], []),
        ("standalone_t28_read_without_mutation_refs", [("delete", "persistent", None), ("delete", "evidence.persistent", None), ("delete", "evidence.provisional.provisional_set", None), ("delete", "evidence.provisional.provisional_ack", None)], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "12")], []),
        ("standalone_t28_unrelated_old_managed_set", [("delete", "persistent", None), ("delete", "evidence.persistent", None), ("delete", "evidence.provisional.provisional_set", None), ("delete", "evidence.provisional.provisional_ack", None), ("set", "managed_set", managed_set()), ("set", "evidence.provisional.receipt_timestamp", "2026-09-10T00:05:00Z")], [("evse.limit.configured_current", "16"), ("evse.limit.allocated_current", "12")], []),
    ]
    for name, edits, facts, withheld in cases:
        candidate = copy.deepcopy(baseline)
        for op, path, value in edits:
            edit(candidate, op, path, value)
        result = mapping.project(candidate, contract)
        if facts_of(result) != facts or withheld_of(result) != withheld:
            raise AssertionError(f"{name}: facts {facts_of(result)} withheld {withheld_of(result)}")
        print(name + ": CANDIDATE")
    neither = copy.deepcopy(baseline)
    for op, path, value in [("delete", "persistent", None), ("delete", "evidence.persistent", None), ("delete", "provisional", None), ("delete", "evidence.provisional", None)]:
        edit(neither, op, path, value)
    try:
        mapping.project(neither, contract)
    except ValueError as error:
        if str(error) != "evidence_binding_invalid":
            raise AssertionError(f"no configured candidate: {error}")
    else:
        raise AssertionError("no configured candidate accepted")
    print("no_configured_candidate: REJECTED")


def native_timeout_cases(baseline, contract):
    """Section 6.2: the timeout is served in the modbusreg namespace and never becomes an EVSE expiry fact."""
    result = mapping.project(baseline, contract)
    if result["native"] != [{"namespace": "modbusreg", "field": "LimitTimeoutSeconds", "value": 600}]:
        raise AssertionError("timeout not served natively: " + repr(result["native"]))
    if any("timeout" in fact["id"] or "expiry" in fact["id"] for fact in result["facts"]):
        raise AssertionError("timeout published as a semantic fact")
    zero = copy.deepcopy(baseline)
    mapping.mutate(zero, {"path": "provisional.LimitTimeoutSeconds", "value": 0})
    zero_result = mapping.project(zero, contract)
    if zero_result["native"] != [{"namespace": "modbusreg", "field": "LimitTimeoutSeconds", "value": 0}] or facts_of(zero_result) != facts_of(result):
        raise AssertionError("zero timeout is not served natively with the allocated current kept")
    malformed = copy.deepcopy(baseline)
    mapping.mutate(malformed, {"path": "provisional.LimitTimeoutSeconds", "value": -1})
    if mapping.project(malformed, contract)["native"] != []:
        raise AssertionError("malformed timeout served natively")
    print("native-timeout: PASS")


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


def main():
    readback_sources()
    readback_fields_are_independent()
    contract = mapping.load(); baseline = contract["scenarios"][0]["input"]
    operation_cases(baseline, contract)
    readback_cases(baseline, contract)
    operation_vectors()
    candidate_cases(baseline, contract)
    native_timeout_cases(baseline, contract)
    cases = [("status_read_only", "status", "accepted_source_specific_read_only_mapping_gate", "contract"), ("pin", "pins.gateway_main", "0" * 40, "contract"), ("profile", "native_contract.profile", "wc3_24_44_4", "native_contract"), ("outbound", "native_contract.outbound_allowed", True, "native_contract"), ("identity_derivation", "identity.forbidden_derivations.0", "serial", "identity_contract"), ("identity_reuse", "identity.failure", "allow_reuse", "identity_contract"), ("lifecycle", "lifecycle.required.0", "timestamp", "lifecycle_contract"), ("persistent_target", "field_rules.0.target", "evse.limit.allocated_current", "field_rules"), ("provisional_disposition", "field_rules.2.disposition", "always", "field_rules"), ("unsupported", "unsupported.0", "power_allowed", "withheld_or_unsupported"), ("precedence", "projection.error_precedence.0", "x", "projection"), ("operations", "projection.operations", "available", "projection"), ("operation_unlisted", "projection.operations.0.name", "evse.operation.release_allocated_current", "projection"), ("operation_timeout_range_open", "projection.operations.0.argument_rules.timeout_seconds.interoperable_range", [0, 86399], "projection"), ("operation_pause_encoding_dropped", "projection.operations.0.argument_rules.amperes.pause.encoding", "none", "projection"), ("operation_refused_range_widened", "projection.operations.0.argument_rules.amperes.refused_range.maximum_refused_nonzero_amperes", 4, "projection"), ("cutover", "consumer_cutover.forbidden.0", "allowed", "consumer_cutover")]
    for case in cases: reject(*case)
    for name, path, value, expected in [("blank_identity", "identity.asset_id", "", "identity_invalid"), ("managed_set_without_its_acknowledgement_ref", "managed_set", {"ack_receipt_timestamp": "2026-09-10T00:00:00Z", "duration_seconds": 60}, "evidence_binding_invalid"), ("wrong_unit_profile", "profile", "other", "profile_or_version_invalid"), ("missing_receipt", "lifecycle.receipt_timestamp", "", "lifecycle_invalid"), ("operation_attempt", "operation", "evse.operation.release_allocated_current", "unsupported_fact_or_operation"), ("persistent_evidence", "evidence.persistent.persistent_terminal", "bad", "evidence_binding_invalid"), ("persistent_receipt", "evidence.persistent.receipt_timestamp", "", "evidence_binding_invalid")]:
        candidate = copy.deepcopy(baseline); mapping.mutate(candidate, {"path": path, "value": value})
        try: mapping.project(candidate, contract)
        except ValueError as error:
            if str(error) != expected: raise AssertionError(error)
        else: raise AssertionError(name + " accepted")
        print(name + ": REJECTED")
    print("baseline: PASS; readback, operation and rejection cases checked through project()")
    native_page_cases()
    graph_cases()


def native_page_cases():
    """The staged ND02 page must exist and state the rules the gate cites; an absent page is never a pass."""
    mapping.check_native_page()
    original = mapping.NATIVE_PAGE
    try:
        mapping.NATIVE_PAGE = ROOT / ".tmp/does-not-exist/gen3-semreg-mapping-v1.md"
        try:
            mapping.check_native_page()
        except ValueError as error:
            if str(error) != "native_page_missing":
                raise
        else:
            raise AssertionError("missing ND02 page accepted")
    finally:
        mapping.NATIVE_PAGE = original
    native_page_text_cases()
    print("native-page: PASS")


def native_page_text_cases():
    """A page with only the operation statements, or without the relation statements, fails the native check."""
    original = mapping.NATIVE_PAGE
    full = " ".join(original.read_text(encoding="utf-8").split())
    without_relations = full
    for statement in mapping.NATIVE_STATEMENTS[4:]:
        without_relations = without_relations.replace(statement, "")
    pages = [("operation_statements_only", " ".join(mapping.NATIVE_STATEMENTS[:4])), ("relation_statements_absent", without_relations)]
    with tempfile.TemporaryDirectory() as directory:
        try:
            for name, text in pages:
                mapping.NATIVE_PAGE = Path(directory) / f"{name}.md"
                mapping.NATIVE_PAGE.write_text(text, encoding="utf-8")
                try:
                    mapping.check_native_page()
                except ValueError as error:
                    if str(error) != "native_page_disagrees":
                        raise
                else:
                    raise AssertionError(f"{name} page accepted")
                print(f"{name}: REJECTED")
        finally:
            mapping.NATIVE_PAGE = original
    companion = mapping.DOC.read_text(encoding="utf-8")
    mapping.check_companion_relations(companion)
    try:
        mapping.check_companion_relations(companion.replace("connector-to-session", "session"))
    except ValueError as error:
        if str(error) != "document_relations":
            raise
    else:
        raise AssertionError("companion without connector-to-session accepted")
    print("companion-relations: REJECTED")


SESSION = {"kind": "resource.charging_session", "anchor_class": "episode", "anchor_basis": "native_slot", "anchor_source": "native_position", "identification_scheme": "none_listed", "parent_kind": "resource.connector", "evidence": ["synthetic:session"]}
METER_UNSTATED = {"definition": "evse.relation.evse_meter", "outcome": "unknown_not_absent", "unpublished_endpoint": "resource.meter"}
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
    ("meter_relation_endpoint_kind", [("set", "graph.relations.2.to_kind", "resource.phase")], "graph endpoint_kind_mismatch $.graph.relations[2].to_kind"),
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
    ("meter_relation_dropped", [("delete", "graph.relations.2", None)], "graph_relations"),
    ("meter_absence_asserted", [("set", "graph.relations.2.absence_sides", ["to"])], "graph_relations"),
    ("meter_relation_also_unstated", [("append", "unstated_relations", METER_UNSTATED)], "unstated_relations"),
    ("meter_resource_dropped", [("delete", "graph.resources.3", None)], "graph dangling_endpoint $.graph.relations[2].to_kind"),
    ("identity_meter_dimension_unpublished", [("set", "identity.meter_id.dimension", "evse.dimension.phase")], "identity_contract"),
    ("identity_configured_text", [("set", "identity.asset_id.source", "gateway_configured_semantic_asset_id")], "identity_contract"),
    ("identity_dimension_unpublished", [("set", "identity.connector_id.dimension", "evse.dimension.phase")], "identity_contract"),
    ("identity_address_allowed", [("delete", "identity.forbidden_derivations.5", None)], "identity_contract"),
    ("field_rule_subject_kind", [("set", "field_rules.2.subject_kind", "resource.evse")], "field_rules"),
    ("field_rule_measurement_point", [("set", "field_rules.0.measurement_point", "")], "field_rules"),
    ("field_rule_dimension", [("set", "field_rules.0.dimension", "connector")], "field_rules"),
]


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
