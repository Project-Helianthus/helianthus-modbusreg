#!/usr/bin/env python3
"""Execute the source-specific Tesla Gen3 WC3 current-limit mapping gate."""
from __future__ import annotations

import copy
import importlib.util
import json
import re
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
GATES = ROOT / "driverdata/families/mapping-gates"
SEMANTIC = ROOT / ".tmp/references/docs-semantic"
PATH = GATES / "tesla-gen3-wc3-24443-evse-current-limit-v1.json"
DOC = GATES / "tesla-gen3-wc3-24443-evse-current-limit-v1.md"
_GRAPH_SPEC = importlib.util.spec_from_file_location("mapping_gate_graph_rules", Path(__file__).resolve().parent / "mapping_gate_graph_rules.py")
graph_rules = importlib.util.module_from_spec(_GRAPH_SPEC)
_GRAPH_SPEC.loader.exec_module(graph_rules)
PACK = {"id": "helianthus.pack.evse", "version": "1.0.0"}
DIMENSION_PREFIX = "evse.dimension."
# Identity keys name the pack's resource dimensions; each value is the gateway-allocated ResourceID (graph.md 6.3).
IDENTITY_KEYS = ("asset_id", "evse_id", "connector_id")
IDENTITY_SOURCE = "gateway_allocated_resource_id"
FORBIDDEN_DERIVATIONS = ["unit_id", "firmware_text", "observation_values", "payload_bytes", "request_order", "network_address", "ordinal_of_sorted_identifiers", "asset_identifier"]
IDENTITY_FAILURE = "missing_blank_invalid_reused_or_ambiguous_identity_fails_closed"
# Section 15 item 2: the relations this gate states, all declared by configuration.
STATED_RELATIONS = ["evse.relation.asset_evse", "evse.relation.evse_connector"]
STATED_BASIS = "configuration"
NATIVE_VERSION = "24.44.3"
UNSTATED_OUTCOME = "unknown_not_absent"
SELECTORS = [{"selector": "profile", "classification": "provenance", "evidence": ["native_contract:profile", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]}]
# EN-1: subject kind and dimension come from the element catalog; the gate states the measurement point.
FIELD_RULES = [
    {"source": "provisional.readback.ConfiguredCurrentAmps", "target": "evse.limit.configured_current", "measurement_point": "limit_defined_at_evse_output", "unit": "unit.ampere", "disposition": "exact"},
    {"source": "provisional.readback.LimitCurrentMaxAmps", "target": "evse.limit.allocated_current", "measurement_point": "limit_defined_at_connector_output", "unit": "unit.ampere", "disposition": "exact_when_finite_timeout_and_not_inhibited"},
]
PINS = {"docs_semantic_base": "f830ace6c2b9dd1af0e87ce808fa545662578418", "semreg": "f3f761bc67e10d6a65eba6c13cb4dc51002d6955", "docs_modbus_current_limit": "6611e20ac8b2c3e8b953934757346f8a847b30bf", "docs_modbus_native_projection": "1127c333de1f1d02305952a58f8745ee05c42e12", "modbusreg": "57f7eb84f7d4e1173621711bf64624726c71bc75", "gateway_main": "32244901c4c8337266cd348bdad90fb9e8eb0a61"}
ERRORS = ["profile_or_version_invalid", "identity_invalid", "source_path_invalid", "evidence_binding_invalid", "lifecycle_invalid", "unsupported_fact_or_operation"]
OPAQUE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]*$")
DIGEST = re.compile(r"^sha256:[0-9a-f]{64}$")
PERSISTENT_REFS = ["persistent_request", "persistent_terminal"]
PROVISIONAL_REFS = ["provisional_set", "provisional_ack", "provisional_readback_request", "provisional_readback_terminal"]


def load():
    return json.loads(PATH.read_text(encoding="utf-8"))


def fail(code):
    raise ValueError(code)


def mutate(value, mutation):
    parts = mutation["path"].split(".")
    current = value
    for part in parts[:-1]:
        current = current[int(part)] if isinstance(current, list) else current.setdefault(part, {})
    key = int(parts[-1]) if isinstance(current, list) else parts[-1]
    if mutation.get("op") == "delete":
        current.pop(key)
    else:
        current[key] = mutation["value"]


class Catalog:
    """Element-catalog lookups for the pack of this gate."""

    def __init__(self, document):
        self.document = document
        self.index = graph_rules.Index(document)
        self.dimensions = {}
        self.pack_relations = []
        for element in document.get("elements", []):
            ref = element["ref"]
            if element.get("disposition") != "member" or ref["owner"].get("pack") != PACK:
                continue
            if element["element_kind"] == "dimension":
                self.dimensions[ref["id"]] = element["attributes"]
            elif element["element_kind"] == "relation":
                self.pack_relations.append(ref["id"])

    def dimension_kind(self, stem):
        attributes = self.dimensions.get(DIMENSION_PREFIX + stem)
        if attributes is None:
            fail("catalog_dimension_missing")
        return attributes["resource_kind"]


def identity_stem(key):
    return key.removesuffix("_id")


def published_kinds(contract, catalog):
    """RES-1 and section 6.1: one resource kind per identity dimension; a session only with a native session token."""
    kinds = [catalog.dimension_kind(identity_stem(key)) for key in IDENTITY_KEYS]
    if "session_token" in contract.get("native_contract", {}):
        kinds.append(catalog.dimension_kind("session"))
    return kinds


def expected_basis(resource, native):
    """Section 6.3: a listed scheme gives a native identity, a native position a native slot, else an operator label."""
    if resource["identification_scheme"] != graph_rules.NONE_LISTED:
        return "native_identity"
    if resource["anchor_class"] != "equipment" and "position" in native:
        return "native_slot"
    return "operator_slot"


def check_graph_parents(graph, catalog):
    """RES-2: a component's parent kind is the whole of the contains relation the gate states for it."""
    wholes = {}
    for relation in graph["relations"]:
        definition = catalog.index.relations.get(relation["definition"], {})
        if definition.get("core_type") == "contains":
            whole, part = (relation["from_kind"], relation["to_kind"]) if definition["orientation"] == "as_core" else (relation["to_kind"], relation["from_kind"])
            wholes[part] = whole
    for resource in graph["resources"]:
        if resource["parent_kind"] != wholes.get(resource["kind"]):
            fail("graph_parent")


def check_graph_relations(graph, native):
    """Section 15 item 2 and REL-3: configuration basis, the native profile scope and no absence statement."""
    if [relation["definition"] for relation in graph["relations"]] != STATED_RELATIONS:
        fail("graph_relations")
    scope = {"profile": native.get("profile"), "version": NATIVE_VERSION}
    for relation in graph["relations"]:
        stated_scope = {key: relation["validity_scope"].get(key) for key in scope}
        if relation["basis_class"] != STATED_BASIS or relation["absence_sides"] or stated_scope != scope:
            fail("graph_relations")


def expected_unstated(catalog, stated, published):
    """Section 7.2: a pack relation the gate does not state stays unknown, never absent."""
    result = []
    for name in catalog.pack_relations:
        if name in stated:
            continue
        definition = catalog.index.relations[name]
        missing = [kind for kind in definition["from_kinds"] + definition["to_kinds"] if kind not in published]
        if not missing:
            fail("unstated_relations")
        result.append({"definition": name, "outcome": UNSTATED_OUTCOME, "unpublished_endpoint": missing[0]})
    return result


def validate_graph(contract, catalog):
    """Section 15: the shared checker first, then this gate's own resource and relation rules."""
    failures = graph_rules.check(contract, catalog.document)
    if failures:
        fail("graph {} {}".format(*failures[0]))
    if not graph_rules.graph_stated(contract):
        fail("graph_not_stated")
    graph, native = contract["graph"], contract.get("native_contract", {})
    published = published_kinds(contract, catalog)
    if [resource["kind"] for resource in graph["resources"]] != published:
        fail("graph_resources")
    if any(resource["anchor_basis"] != expected_basis(resource, native) for resource in graph["resources"]):
        fail("graph_anchor")
    check_graph_parents(graph, catalog)
    check_graph_relations(graph, native)
    for member in ("relayed", "joins", "aggregates"):
        if graph[member]:
            fail(f"graph_{member}")
    if graph["selectors"] != SELECTORS:
        fail("graph_selectors")
    stated = [relation["definition"] for relation in graph["relations"]]
    if contract.get("unstated_relations") != expected_unstated(catalog, stated, published):
        fail("unstated_relations")


def validate_identity(identity):
    expected = {key: {"source": IDENTITY_SOURCE, "dimension": DIMENSION_PREFIX + identity_stem(key), "required": True, "non_secret": True} for key in IDENTITY_KEYS}
    expected.update(forbidden_derivations=FORBIDDEN_DERIVATIONS, failure=IDENTITY_FAILURE)
    if identity != expected:
        fail("identity_contract")


def expected_field_rules(catalog):
    """EN-1: each rule names the catalog subject kind of its target and the measurement point."""
    rules = []
    for rule in FIELD_RULES:
        fact = catalog.index.facts.get(rule["target"])
        if fact is None:
            fail("field_rules")
        subject = fact["subject"]
        rules.append({"source": rule["source"], "target": rule["target"], "subject_kind": subject["kind"], "dimension": subject["dimension"]["id"].removeprefix(DIMENSION_PREFIX), "measurement_point": rule["measurement_point"], "unit": rule["unit"], "disposition": rule["disposition"]})
    return rules


def validate_contract(contract, catalog=None):
    catalog = Catalog(graph_rules.load_catalog() if catalog is None else catalog)
    if contract.get("contract") != "helianthus.semantic.mapping.tesla-gen3-wc3-24443.evse-current-limit/v1" or contract.get("pack") != PACK or contract.get("pins") != PINS:
        fail("contract")
    native = contract.get("native_contract")
    if native != {"profile": "wc3_24_44_3", "source_path": "modbus.v1.tesla.gen3.evse.current_limit.get", "function": "FC100", "read_only": True, "outbound_allowed": False, "persistent": {"request_response": "family_6.t7_to_t8", "field": "MaxOutputCurrentAmps", "unit": "unit.ampere"}, "provisional": {"set_ack": "family_6.t25_to_t26", "readback": "family_6.t27_to_t28", "configured_field": "ConfiguredCurrentAmps", "allocated_field": "LimitCurrentMaxAmps", "timeout": "LimitTimeoutSeconds", "inhibit": "InhibitCharging", "unit": "unit.ampere"}}:
        fail("native_contract")
    validate_graph(contract, catalog)
    validate_identity(contract.get("identity", {}))
    lifecycle = contract.get("lifecycle", {})
    if lifecycle != {"required": ["receipt_timestamp", "source_epoch", "driver_generation", "qualification", "semantic_revision", "lifecycle_generation"], "per_fact_evidence": {"persistent": ["immutable_evidence_id", "payload_digest", "persistent_request", "persistent_terminal"], "provisional": ["immutable_evidence_id", "payload_digest", "correlation_id", "provisional_set", "provisional_ack", "provisional_readback_request", "provisional_readback_terminal"]}, "atomicity": "persistent_and_provisional_each_have_one_evidence_id_one_source_epoch_one_driver_generation_one_semantic_revision", "withdrawal": "explicit_generation_fenced", "retention": "native_owner_only_permitted_last_known_good", "physical_qualified": False, "mapping_qualified": True}:
        fail("lifecycle_contract")
    if contract.get("field_rules") != expected_field_rules(catalog):
        fail("field_rules")
    if len(contract.get("withheld", [])) != 3 or contract.get("unsupported") != ["power", "energy", "phase", "session", "readiness", "connector_topology", "state_of_charge", "thermal", "fault", "interlock", "meter", "set_allocated_current", "sender", "route", "authority", "acknowledgement_authority", "readback_authority", "retry", "live_control"]:
        fail("withheld_or_unsupported")
    projection = contract.get("projection", {})
    if projection.get("source_path") != "modbus.v1.tesla.gen3.evse.current_limit.get" or projection.get("error_precedence") != ERRORS or projection.get("operations") != "all_unavailable_no_authority" or projection.get("missing_native") != "withheld_unavailable_never_zero":
        fail("projection")
    cutover = contract.get("consumer_cutover", {})
    if cutover != {"sequence": ["one_semreg_projection", "versioned_semantic_mcp", "mtls_graphql_and_portal_parity", "home_assistant_adoption"], "forbidden": ["fallback", "comparator", "compatibility_adapter", "shadow_authority", "dual_publication", "caller_supplied_provenance", "write_route"], "until_implemented": "no_consumer_binding"}:
        fail("consumer_cutover")


def valid_id(value):
    return isinstance(value, str) and bool(OPAQUE.fullmatch(value))


def valid_evidence(evidence, refs, correlation=False):
    if not isinstance(evidence, dict) or not valid_id(evidence.get("immutable_evidence_id")) or not DIGEST.fullmatch(str(evidence.get("payload_digest"))) or any(not DIGEST.fullmatch(str(evidence.get(key))) for key in refs):
        return False
    return not correlation or evidence.get("correlation_id") == evidence["immutable_evidence_id"]


def non_negative_int(value):
    return isinstance(value, int) and not isinstance(value, bool) and value >= 0


def publish_configured_current(facts, withheld, provisional):
    configured = provisional.get("ConfiguredCurrentAmps")
    if non_negative_int(configured):
        facts.append({"id": "evse.limit.configured_current", "dimension": "evse", "value": {"coefficient": str(configured), "exponent10": 0}, "unit": "unit.ampere"})
    else:
        withheld.append({"id": "evse.limit.configured_current", "reason": "provisional_record_malformed"})


def project(case, contract):
    errors = case_errors(case, contract)
    for error in ERRORS:
        if error in errors: fail(error)
    persistent, provisional = case.get("persistent"), case.get("provisional")
    if not isinstance(persistent, dict) or not isinstance(persistent.get("MaxOutputCurrentAmps"), int) or isinstance(persistent["MaxOutputCurrentAmps"], bool) or persistent["MaxOutputCurrentAmps"] < 0: fail(ERRORS[3])
    facts, withheld = [], []
    publish_limits(case, provisional, facts, withheld)
    return {"facts": facts, "withheld": withheld, "operations": []}


def case_errors(case, contract):
    errors = set()
    if case.get("profile") != "wc3_24_44_3": errors.add(ERRORS[0])
    identity = case.get("identity", {})
    values = [identity.get(k) for k in ("asset_id", "evse_id", "connector_id")]
    if not all(valid_id(v) for v in values) or len(set(values)) != 3: errors.add(ERRORS[1])
    if case.get("source_path") != contract["projection"]["source_path"]: errors.add(ERRORS[2])
    evidence = case.get("evidence", {})
    if not valid_evidence(evidence.get("persistent"), PERSISTENT_REFS): errors.add(ERRORS[3])
    if not lifecycle_valid(case.get("lifecycle", {})): errors.add(ERRORS[4])
    if case.get("unsupported_fact") or case.get("operation"): errors.add(ERRORS[5])
    return errors


def lifecycle_valid(lifecycle):
    try:
        receipt = datetime.fromisoformat(str(lifecycle.get("receipt_timestamp")).replace("Z", "+00:00"))
        receipt_ok = receipt.tzinfo is not None
    except ValueError:
        receipt_ok = False
    return receipt_ok and valid_id(lifecycle.get("source_epoch")) and lifecycle.get("qualification") == "qualified" and lifecycle.get("semantic_revision") == "1.0.0" and not any(not isinstance(lifecycle.get(k), int) or isinstance(lifecycle.get(k), bool) or lifecycle[k] < 1 for k in ("driver_generation", "lifecycle_generation"))


def publish_limits(case, provisional, facts, withheld):
    evidence = case.get("evidence", {})
    both = ("evse.limit.configured_current", "evse.limit.allocated_current")
    if not isinstance(provisional, dict):
        withheld.extend({"id": fact_id, "reason": "provisional_record_absent"} for fact_id in both)
    elif not valid_evidence(evidence.get("provisional"), PROVISIONAL_REFS, correlation=True):
        withheld.extend({"id": fact_id, "reason": "provisional_evidence_invalid"} for fact_id in both)
    else:
        publish_configured_current(facts, withheld, provisional)
        publish_allocated_current(provisional, facts, withheld)


def publish_allocated_current(provisional, facts, withheld):
    if provisional.get("LimitTimeoutSeconds") == 0:
        withheld.append({"id": "evse.limit.allocated_current", "reason": "zero_timeout_unresolved"})
    elif provisional.get("InhibitCharging") is not False:
        withheld.append({"id": "evse.limit.allocated_current", "reason": "inhibit_state_not_semantically_representable"})
    elif not isinstance(provisional.get("LimitCurrentMaxAmps"), int) or isinstance(provisional["LimitCurrentMaxAmps"], bool) or not 6 <= provisional["LimitCurrentMaxAmps"] <= 1000 or not isinstance(provisional.get("LimitTimeoutSeconds"), int) or not 1 <= provisional["LimitTimeoutSeconds"] <= 86399:
        withheld.append({"id": "evse.limit.allocated_current", "reason": "provisional_record_malformed"})
    else:
        facts.append({"id": "evse.limit.allocated_current", "dimension": "connector", "value": {"coefficient": str(provisional["LimitCurrentMaxAmps"]), "exponent10": 0}, "unit": "unit.ampere"})


def document(contract):
    validate_contract(contract)
    scenarios = contract.get("scenarios", [])
    if not scenarios or scenarios[0].get("polarity") != "positive": fail("scenario_coverage")
    baseline = scenarios[0]["input"]
    for scenario in scenarios:
        case = copy.deepcopy(baseline)
        for mutation in scenario.get("mutations", []): mutate(case, mutation)
        if scenario["polarity"] == "positive":
            if project(case, contract) != scenario["expect"]: fail("scenario_output")
        else:
            try: project(case, contract)
            except ValueError as error:
                if str(error) != scenario.get("expect_error"): raise
            else: fail("scenario_accepted")


def main():
    contract = load(); document(contract)
    text = DOC.read_text(encoding="utf-8")
    for token in ("FC100", "outbound_allowed=false", "field-scoped immutable evidence", "Every operation is unavailable", "mTLS GraphQL", "operator label", "unknown, not absent", "EN-1", "section 15"):
        if token not in text: fail("document_boundary")
    positive = sum(s["polarity"] == "positive" for s in contract["scenarios"])
    print(f"Tesla Gen3 WC3 24.44.3 EVSE current-limit mapping: PASS; {positive} executable outputs and {len(contract['scenarios']) - positive} rejected scenarios")


if __name__ == "__main__": main()
