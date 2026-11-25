#!/usr/bin/env python3
"""Execute the source-specific Tesla Gen3 WC3 current-limit mapping gate."""
from __future__ import annotations

import copy
import importlib.util
import json
import re
from datetime import datetime, timedelta, timezone
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
IDENTITY_KEYS = ("asset_id", "evse_id", "connector_id", "meter_id")
# The meter relation is defined for a configured energy counter, so only an identity the operator configures is checked.
OPTIONAL_IDENTITY_KEYS = ("meter_id",)
IDENTITY_SOURCE = "gateway_allocated_resource_id"
FORBIDDEN_DERIVATIONS = ["unit_id", "firmware_text", "observation_values", "payload_bytes", "request_order", "network_address", "ordinal_of_sorted_identifiers", "asset_identifier"]
IDENTITY_FAILURE = "missing_blank_invalid_reused_or_ambiguous_identity_fails_closed"
# Section 15 item 2: the relations this gate states, all declared by configuration.
STATED_RELATIONS = ["evse.relation.asset_evse", "evse.relation.evse_connector", "evse.relation.evse_meter"]
STATED_BASIS = "configuration"
NATIVE_VERSION = "24.44.3"
UNSTATED_OUTCOME = "unknown_not_absent"
SELECTORS = [{"selector": "profile", "classification": "provenance", "evidence": ["native_contract:profile", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]}]
# EN-1: subject kind and dimension come from the element catalog; the gate states the measurement point.
FIELD_RULES = [
    {"source": "provisional.readback.ConfiguredCurrentAmps", "target": "evse.limit.configured_current", "measurement_point": "limit_defined_at_evse_output", "unit": "unit.ampere", "disposition": "exact_later_receipt_candidate_with_persistent"},
    {"source": "persistent.MaxOutputCurrentAmps", "target": "evse.limit.configured_current", "measurement_point": "limit_defined_at_evse_output", "unit": "unit.ampere", "disposition": "exact_later_receipt_candidate_with_provisional"},
    {"source": "provisional.readback.LimitCurrentMaxAmps", "target": "evse.limit.allocated_current", "measurement_point": "limit_defined_at_connector_output", "unit": "unit.ampere", "disposition": "exact_nonzero_when_not_inhibited; inhibited_is_zero_amperes; expiry_unknown_when_timeout_zero_or_unresolved"},
]
UNSUPPORTED = ["power", "energy", "phase", "session", "readiness", "connector_topology", "state_of_charge", "thermal", "fault", "interlock", "meter", "retry"]
# IEC 61851-1 Annex A: 6 A is the lowest charging current a charger can signal; the pause encodes 6 A with inhibit.
MIN_NONZERO_SET_AMPS = 6
MAX_INTEROPERABLE_TIMEOUT_SECONDS = 86399
# The documented call carries this wire timeout for set, pause and release; the caller does not select it.
WIRE_TIMEOUT_SECONDS = 900
# Argument and dispatch rules of evse.operation.set_allocated_current; set_allocated_current_disposition applies the same constants.
ARGUMENT_RULES = {
    "amperes": {
        "type": "non_negative_integer",
        "upper_bound": "configured_current_of_latest_t28_in_the_lifecycle_generation",
        "upper_bound_device_query": "the_device_limit_query_result_wins_where_offered",
        "pause": {"amperes": 0, "encoding": "6_A_with_inhibit_true", "requires_configured_bound_at_least": MIN_NONZERO_SET_AMPS},
        "refused_range": {"minimum_nonzero_amperes": MIN_NONZERO_SET_AMPS, "maximum_refused_nonzero_amperes": MIN_NONZERO_SET_AMPS - 1, "outcome": "invalid_nothing_sent", "source_clause": "IEC 61851-1 Annex A"},
        "above_configured": "invalid_nothing_sent",
        "no_t28_in_generation": "prerequisite_t27_to_t28_read_only_then_no_t25_when_above_configured",
    },
    "wire_timeout": {"seconds": WIRE_TIMEOUT_SECONDS, "selected_by": "the_operation_not_the_caller", "interoperable_range": [1, MAX_INTEROPERABLE_TIMEOUT_SECONDS], "zero": "not_an_interoperable_request_value", "refused_outcome": "invalid_nothing_sent"},
    "caller_timeout": "not_an_argument_of_the_operation",
    "connector": "own_connector_only",
    "effect_target": "own_connector_only_refused_before_any_read_or_send",
    "release": {"allocated_current_enabled": False, "amperes": "forbidden", "requires_configured_bound": "nonzero_known_configured_current"},
}
# Section 6 operations of this charger: the allocated-current set and the persistent setter of the modbusreg namespace.
OPERATIONS = [
    {"name": "evse.operation.set_allocated_current", "capability": "evse.capability.set_allocated_current", "service": "evse.service.connector", "effect": "evse.effect.set_allocated_current", "release_argument": "evse.limit.allocated_current_enabled", "release_effect": "evse.effect.release_allocated_current", "subject_kind": "resource.connector", "route": ["family_6.t25_to_t26", "family_6.t27_to_t28"], "authority": "setting_owner_through_semreg_call_path", "argument_rules": ARGUMENT_RULES},
    {"name": "ConfigurePersistentCurrent", "namespace": "modbusreg", "subject_kind": "resource.evse", "route": ["family_6.t7_to_t8"]},
]
OPERATION_NAMES = [operation["name"] for operation in OPERATIONS]
SET_ALLOCATED_CURRENT = "evse.operation.set_allocated_current"
# Arguments the semantic call takes; a connector and an effect target address the connector the operation acts on.
SET_REQUEST_KEYS = frozenset({"connector_id", "effect_target", "amperes"})
RELEASE_REQUEST_KEYS = frozenset({"connector_id", "effect_target", "allocated_current_enabled"})
# Evidence members a record must state, each equal to the case's own, to be eligible: its epoch, generations and semantic revision.
BINDING_KEYS = ("source_epoch", "driver_generation", "lifecycle_generation", "semantic_revision")
# The record kinds this gate reads: the persistent t7 to t8 record and the provisional t27 to t28 readback.
RECORD_KINDS = ("persistent", "provisional")
RECORD_ABSENT = "record_absent"
# One decision for every record this gate reads: only an eligible record is selected, suppresses another record, satisfies the evidence gate or contributes a derived value.
RECORD_ELIGIBILITY = {
    "records": ["persistent_t8", "provisional_t28"],
    "eligible_when": "lifecycle_epoch_generation_semantic_revision_and_required_evidence_present_and_equal_to_the_case",
    "binding": list(BINDING_KEYS),
    "ineligible": "reported_with_its_reason_and_no_other_effect_except_the_raw_native_timeout_of_a_readback_with_valid_evidence",
    "reads_only_eligible": ["selection", "suppression", "evidence_gate", "configured_current", "allocated_current", "expiry"],
}
EVIDENCE_MEMBERS = ("immutable_evidence_id", "payload_digest", "receipt_timestamp")
# Each relation the gate leaves unstated names the native statement that is missing.
UNSTATED_REASONS = {
    "evse.relation.connector_phase": "no_native_statement_assigns_per_phase_connector_measurements_to_the_wall_connector_vitals",
    "evse.relation.connector_session": "no_native_statement_gives_a_session_start_in_the_wall_connector_vitals",
}
# The gate admits operations (projection.operations), so its status is not read-only (P1-7; rule 2).
STATUS = "accepted_source_specific_mapping_gate"
PINS = {"docs_semantic_base": "f830ace6c2b9dd1af0e87ce808fa545662578418", "semreg": "f3f761bc67e10d6a65eba6c13cb4dc51002d6955", "docs_modbus_current_limit": "71d432a95145e256047dff37e49ed5251c7ee2d3", "docs_modbus_native_projection": "1127c333de1f1d02305952a58f8745ee05c42e12", "modbusreg": "57f7eb84f7d4e1173621711bf64624726c71bc75", "gateway_main": "32244901c4c8337266cd348bdad90fb9e8eb0a61"}
ERRORS = ["profile_or_version_invalid", "identity_invalid", "source_path_invalid", "evidence_binding_invalid", "lifecycle_invalid", "unsupported_fact_or_operation"]
OPAQUE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]*$")
DIGEST = re.compile(r"^sha256:[0-9a-f]{64}$")
PERSISTENT_REFS = ["persistent_request", "persistent_terminal"]
# Section 6.2 and the projection: the native timeout has no EVSE fact; it is served in the modbusreg namespace.
NATIVE_NAMESPACE = {"namespace": "modbusreg", "fields": ["LimitTimeoutSeconds", "AllocationExpiry"], "reason": "no_catalog_element"}
READBACK_REFS = ["provisional_readback_request", "provisional_readback_terminal"]
# Set and acknowledgement references exist only where a managed mutation ran; a standalone t27 to t28 read carries none.
MUTATION_REFS = ["provisional_set", "provisional_ack"]


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
        if name not in UNSTATED_REASONS:
            fail("unstated_relations")
        result.append({"definition": name, "outcome": UNSTATED_OUTCOME, "unpublished_endpoint": missing[0], "reason": UNSTATED_REASONS[name]})
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
    expected = {key: {"source": IDENTITY_SOURCE, "dimension": DIMENSION_PREFIX + identity_stem(key), "required": key not in OPTIONAL_IDENTITY_KEYS, "non_secret": True} for key in IDENTITY_KEYS}
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
    if contract.get("contract") != "helianthus.semantic.mapping.tesla-gen3-wc3-24443.evse-current-limit/v1" or contract.get("pack") != PACK or contract.get("pins") != PINS or contract.get("status") != STATUS:
        fail("contract")
    native = contract.get("native_contract")
    if native != {"profile": "wc3_24_44_3", "source_path": "modbus.v1.tesla.gen3.evse.current_limit.get", "function": "FC100", "read_only": True, "outbound_allowed": False, "persistent": {"request_response": "family_6.t7_to_t8", "field": "MaxOutputCurrentAmps", "unit": "unit.ampere"}, "provisional": {"set_ack": "family_6.t25_to_t26", "readback": "family_6.t27_to_t28", "configured_field": "ConfiguredCurrentAmps", "allocated_field": "LimitCurrentMaxAmps", "timeout": "LimitTimeoutSeconds", "inhibit": "InhibitCharging", "unit": "unit.ampere"}}:
        fail("native_contract")
    validate_graph(contract, catalog)
    validate_identity(contract.get("identity", {}))
    lifecycle = contract.get("lifecycle", {})
    if lifecycle != {"required": ["receipt_timestamp", "source_epoch", "driver_generation", "qualification", "semantic_revision", "lifecycle_generation"], "per_fact_evidence": {"persistent": ["immutable_evidence_id", "payload_digest", "persistent_request", "persistent_terminal"], "provisional": ["immutable_evidence_id", "payload_digest", "correlation_id", "provisional_readback_request", "provisional_readback_terminal"], "provisional_mutation": MUTATION_REFS}, "candidate_binding": {"configured_current": "eligible_records_only_then_later_receipt"}, "record_eligibility": RECORD_ELIGIBILITY, "atomicity": "persistent_and_provisional_each_have_one_evidence_id_one_source_epoch_one_driver_generation_one_semantic_revision", "withdrawal": "explicit_generation_fenced", "retention": "native_owner_only_permitted_last_known_good", "physical_qualified": False, "mapping_qualified": True}:
        fail("lifecycle_contract")
    if contract.get("field_rules") != expected_field_rules(catalog):
        fail("field_rules")
    if contract.get("withheld") != [] or contract.get("unsupported") != UNSUPPORTED:
        fail("withheld_or_unsupported")
    projection = contract.get("projection", {})
    if projection.get("source_path") != "modbus.v1.tesla.gen3.evse.current_limit.get" or projection.get("error_precedence") != ERRORS or projection.get("operations") != OPERATIONS or projection.get("native_namespace") != NATIVE_NAMESPACE or projection.get("missing_native") != "withheld_unavailable_never_zero":
        fail("projection")
    if contract.get("operation") != OPERATIONS[0]:
        fail("operation")
    cutover = contract.get("consumer_cutover", {})
    if cutover != {"sequence": ["one_semreg_projection", "versioned_semantic_mcp", "mtls_graphql_and_portal_parity", "home_assistant_adoption"], "forbidden": ["fallback", "comparator", "compatibility_adapter", "shadow_authority", "dual_publication", "caller_supplied_provenance", "write_route"], "until_implemented": "no_consumer_binding"}:
        fail("consumer_cutover")


def valid_id(value):
    return isinstance(value, str) and bool(OPAQUE.fullmatch(value))


def receipt_of(evidence):
    """A field-scoped receipt: an aware timestamp that orders the candidates of one epoch."""
    try:
        value = datetime.fromisoformat(str(evidence.get("receipt_timestamp")).replace("Z", "+00:00"))
    except ValueError:
        return None
    return value if value.tzinfo is not None else None


def valid_evidence(evidence, refs, correlation=False):
    if not isinstance(evidence, dict) or not valid_id(evidence.get("immutable_evidence_id")) or not DIGEST.fullmatch(str(evidence.get("payload_digest"))) or any(not DIGEST.fullmatch(str(evidence.get(key))) for key in refs) or receipt_of(evidence) is None:
        return False
    return not correlation or evidence.get("correlation_id") == evidence["immutable_evidence_id"]


def non_negative_int(value):
    return isinstance(value, int) and not isinstance(value, bool) and value >= 0


def evidence_refs(kind, evidence):
    """A persistent record needs its request and terminal; a readback needs its own, and a record that names a set or acknowledgement needs those too."""
    if kind == "persistent":
        return PERSISTENT_REFS
    return READBACK_REFS + MUTATION_REFS if any(key in evidence for key in MUTATION_REFS) else READBACK_REFS


def binding_reason(evidence, lifecycle):
    """The first binding member the evidence leaves out or states differently from the case lifecycle, else None."""
    for key in BINDING_KEYS:
        if key not in evidence:
            return f"{key}_missing"
        if type(evidence[key]) is not type(lifecycle.get(key)) or evidence[key] != lifecycle.get(key):
            return f"{key}_other"
    return None


def evidence_reason(kind, evidence):
    """Required evidence: every member and reference present is `evidence_missing` when left out and `evidence_invalid` when malformed."""
    refs = evidence_refs(kind, evidence)
    required = EVIDENCE_MEMBERS + tuple(refs) + (("correlation_id",) if kind == "provisional" else ())
    if any(evidence.get(key) is None for key in required):
        return "evidence_missing"
    return None if valid_evidence(evidence, refs, correlation=kind == "provisional") else "evidence_invalid"


def eligibility(case, kind):
    """The one eligibility decision: None when the record of this kind is eligible, RECORD_ABSENT when the case has none, else the reason it is not.
    A record is eligible only when its lifecycle, source epoch, driver generation, semantic revision and required evidence are present and equal to the case's own.
    Selection, suppression, the evidence gate and every derived value read only eligible records."""
    if kind not in case or (kind == "provisional" and not isinstance(case[kind], dict)):
        return RECORD_ABSENT
    evidence = case["evidence"].get(kind) if isinstance(case.get("evidence"), dict) else None
    if not isinstance(evidence, dict):
        return "evidence_missing"
    if not isinstance(case.get("lifecycle"), dict):
        return "lifecycle_missing"
    return binding_reason(evidence, case["lifecycle"]) or evidence_reason(kind, evidence)


def ineligible_report(case):
    """Each present record that is not eligible, with its reason; it has no other effect."""
    reasons = [(kind, eligibility(case, kind)) for kind in RECORD_KINDS]
    return [{"record": kind, "reason": reason} for kind, reason in reasons if reason not in (None, RECORD_ABSENT)]


def publish_configured_current(case, facts, withheld):
    """Section 6.1: the t28 and the t8 configured current are each sufficient; the later receipt wins among eligible records only."""
    candidates = []
    if eligibility(case, "provisional") is None and "ConfiguredCurrentAmps" in case["provisional"]:
        candidates.append((case["evidence"]["provisional"], case["provisional"]["ConfiguredCurrentAmps"]))
    if eligibility(case, "persistent") is None:
        candidates.append((case["evidence"]["persistent"], case["persistent"].get("MaxOutputCurrentAmps") if isinstance(case["persistent"], dict) else None))
    if not candidates:
        withheld.append({"id": "evse.limit.configured_current", "reason": "configured_current_absent"})
        return
    _, configured = max(((receipt_of(evidence), value) for evidence, value in candidates), key=lambda candidate: candidate[0])
    if not non_negative_int(configured):
        withheld.append({"id": "evse.limit.configured_current", "reason": "provisional_record_malformed"})
    elif configured == 0:
        withheld.append({"id": "evse.limit.configured_current", "reason": "zero_configured_unknown"})
    else:
        facts.append({"id": "evse.limit.configured_current", "dimension": "evse", "value": {"coefficient": str(configured), "exponent10": 0}, "unit": "unit.ampere"})


def project(case, contract):
    errors = case_errors(case, contract)
    for error in ERRORS:
        if error in errors: fail(error)
    persistent = case.get("persistent")
    if eligibility(case, "persistent") is None and (not isinstance(persistent, dict) or not non_negative_int(persistent.get("MaxOutputCurrentAmps"))): fail(ERRORS[3])
    facts, withheld, native = [], [], []
    publish_limits(case, facts, withheld, native)
    return {"facts": facts, "withheld": withheld, "native": native, "ineligible": ineligible_report(case), "operations": list(OPERATION_NAMES)}


def case_errors(case, contract):
    errors = set()
    if case.get("profile") != "wc3_24_44_3": errors.add(ERRORS[0])
    identity = case.get("identity", {})
    values = [identity.get(k) for k in IDENTITY_KEYS if k in identity or k not in OPTIONAL_IDENTITY_KEYS]
    if not all(valid_id(v) for v in values) or len(set(values)) != len(values): errors.add(ERRORS[1])
    if case.get("source_path") != contract["projection"]["source_path"]: errors.add(ERRORS[2])
    evidence = case.get("evidence", {})
    if not case_evidence_valid(case): errors.add(ERRORS[3])
    if not lifecycle_valid(case.get("lifecycle", {})): errors.add(ERRORS[4])
    if "managed_set" in case and not managed_set_valid(case["managed_set"]): errors.add(ERRORS[3])
    if case.get("unsupported_fact") or (case.get("operation") is not None and case.get("operation") not in OPERATION_NAMES): errors.add(ERRORS[5])
    return errors


def case_evidence_valid(case):
    """The evidence gate is satisfied only by an eligible record. A persistent record that names the case lifecycle and leaves out or malforms a member or reference of its evidence is a corrupt case and is rejected; any other ineligible record only fails to satisfy the gate."""
    if eligibility(case, "persistent") in ("evidence_invalid", "evidence_missing") and isinstance(case["evidence"].get("persistent"), dict):
        return False
    return any(eligibility(case, kind) is None for kind in RECORD_KINDS)


def lifecycle_valid(lifecycle):
    try:
        receipt = datetime.fromisoformat(str(lifecycle.get("receipt_timestamp")).replace("Z", "+00:00"))
        receipt_ok = receipt.tzinfo is not None
    except ValueError:
        receipt_ok = False
    return receipt_ok and valid_id(lifecycle.get("source_epoch")) and lifecycle.get("qualification") == "qualified" and lifecycle.get("semantic_revision") == "1.0.0" and not any(not isinstance(lifecycle.get(k), int) or isinstance(lifecycle.get(k), bool) or lifecycle[k] < 1 for k in ("driver_generation", "lifecycle_generation"))


def native_timeout_retained(case, reason):
    """The raw native timeout of a readback with valid evidence stays served when only its lifecycle binding is not the case's."""
    return reason in {f"{key}_{kind}" for key in BINDING_KEYS for kind in ("missing", "other")} and valid_evidence(case["evidence"]["provisional"], evidence_refs("provisional", case["evidence"]["provisional"]), correlation=True)


def publish_limits(case, facts, withheld, native):
    provisional = case.get("provisional")
    reason = eligibility(case, "provisional")
    if reason == RECORD_ABSENT:
        withheld.append({"id": "evse.limit.allocated_current", "reason": "provisional_record_absent"})
    elif reason is not None:
        # An ineligible readback yields no allocated current, candidate or expiry.
        withheld.append({"id": "evse.limit.allocated_current", "reason": "provisional_evidence_invalid"})
    publish_configured_current(case, facts, withheld)
    if reason is None:
        publish_allocated_current(case, provisional, facts, withheld)
    if reason is None or (reason != RECORD_ABSENT and native_timeout_retained(case, reason)):
        publish_native_timeout(provisional, native)
    if reason is None:
        publish_native_expiry(case, native)


def managed_set_valid(managed):
    """A managed set names its t25 acknowledgement receipt, the requested duration in seconds, and the set and acknowledgement it belongs to."""
    return isinstance(managed, dict) and receipt_text(managed.get("ack_receipt_timestamp")) is not None and non_negative_int(managed.get("duration_seconds")) and all(DIGEST.fullmatch(str(managed.get(key))) for key in MUTATION_REFS)


def receipt_text(text):
    try:
        value = datetime.fromisoformat(str(text).replace("Z", "+00:00"))
    except ValueError:
        return None
    return value if value.tzinfo is not None else None


def allocation_cutoff(case):
    """ND02 SemReg mapping, field rules: a known nonzero managed timeout ends at the set acknowledgement plus the requested duration; the cutoff is known only for a readback that names that set and acknowledgement. An unresolved timeout gives no cutoff."""
    managed = case.get("managed_set")
    if managed is None or managed["duration_seconds"] == 0 or eligibility(case, "provisional") is not None:
        return None
    timeout = case["provisional"].get("LimitTimeoutSeconds")
    if not non_negative_int(timeout) or timeout == 0:
        return None
    if any(managed[key] != case["evidence"]["provisional"].get(key) for key in MUTATION_REFS):
        return None
    return receipt_text(managed["ack_receipt_timestamp"]) + timedelta(seconds=managed["duration_seconds"])


def allocation_expired(case):
    """A readback received at or after its known cutoff is expired."""
    cutoff = allocation_cutoff(case)
    return cutoff is not None and receipt_of(case["evidence"]["provisional"]) >= cutoff


def publish_native_expiry(case, native):
    """ND02 SemReg mapping, field rules: at or after expiry the current allocation is withheld and the known expiry is retained, served natively because no catalog element gives it one."""
    if allocation_expired(case):
        native.append({"namespace": NATIVE_NAMESPACE["namespace"], "field": "AllocationExpiry", "value": allocation_cutoff(case).astimezone(timezone.utc).isoformat().replace("+00:00", "Z")})


def publish_native_timeout(provisional, native):
    """Section 6.2: the timeout only decides the expiry, which stays unknown; the value is served natively, never as a fact."""
    timeout = provisional.get("LimitTimeoutSeconds")
    if non_negative_int(timeout):
        native.append({"namespace": NATIVE_NAMESPACE["namespace"], "field": "LimitTimeoutSeconds", "value": timeout})


def amperes_fact(fact_id, dimension, amperes):
    return {"id": fact_id, "dimension": dimension, "value": {"coefficient": str(amperes), "exponent10": 0}, "unit": "unit.ampere"}


def publish_allocated_current(case, provisional, facts, withheld):
    """Section 6.2: an inhibited readback is 0 A; a nonzero current is withheld at or after a known managed cutoff, else kept with its expiry unknown."""
    inhibit, current = provisional.get("InhibitCharging"), provisional.get("LimitCurrentMaxAmps")
    if not isinstance(inhibit, bool) or not non_negative_int(current):
        withheld.append({"id": "evse.limit.allocated_current", "reason": "provisional_record_malformed"})
    elif inhibit and not allocation_expired(case):
        facts.append(amperes_fact("evse.limit.allocated_current", "connector", 0))
    elif not inhibit and current == 0:
        withheld.append({"id": "evse.limit.allocated_current", "reason": "zero_current_unresolved"})
    elif allocation_expired(case):
        withheld.append({"id": "evse.limit.allocated_current", "reason": "allocation_expired"})
    else:
        facts.append(amperes_fact("evse.limit.allocated_current", "connector", current))


def valid_wire_timeout(timeout):
    """A wire timeout is a whole number of seconds from 1 through the interoperable maximum; zero is not an interoperable request value."""
    return non_negative_int(timeout) and 1 <= timeout <= MAX_INTEROPERABLE_TIMEOUT_SECONDS


def wire_request(disposition, amperes, bound):
    """The t25 triple of a send disposition: current, wire timeout and inhibit; a pause encodes 6 A with inhibit. The wire timeout must be a valid one."""
    if disposition not in ("send_set", "send_pause", "send_release"):
        fail("wire_request_not_sent")
    if not valid_wire_timeout(WIRE_TIMEOUT_SECONDS):
        fail("wire_timeout_invalid")
    if disposition == "send_set":
        return (amperes, WIRE_TIMEOUT_SECONDS, False)
    if disposition == "send_pause":
        return (MIN_NONZERO_SET_AMPS, WIRE_TIMEOUT_SECONDS, True)
    return (bound, WIRE_TIMEOUT_SECONDS, False)


def release_disposition(request, generation):
    """ND02 SemReg mapping, operations: boolean false for allocated_current_enabled is a release with no amperage; it needs a valid nonzero configured current."""
    if not set(request) <= RELEASE_REQUEST_KEYS:
        return "rejected_invalid"
    if not generation["t28_known"]:
        return "read_first"
    bound = generation["configured_current"]
    if not non_negative_int(bound) or bound == 0:
        return "rejected_no_release_bound"
    return "send_release"


def set_allocated_current_disposition(request, generation, own_connector):
    """Operation rules for evse.operation.set_allocated_current; a refused request sends nothing, and a foreign target is refused before any read."""
    if request.get("connector_id") != own_connector or request.get("effect_target", own_connector) != own_connector:
        return "rejected_other_connector"
    if "allocated_current_enabled" in request:
        if request["allocated_current_enabled"] is not False:
            return "rejected_invalid"
        return release_disposition(request, generation)
    amperes = request.get("amperes")
    if not set(request) <= SET_REQUEST_KEYS or not non_negative_int(amperes):
        return "rejected_invalid"
    if 0 < amperes < MIN_NONZERO_SET_AMPS:
        return "rejected_below_minimum"
    if not generation["t28_known"]:
        return "read_first"
    bound = generation["configured_current"]
    if not non_negative_int(bound):
        return "rejected_no_configured_bound"
    if amperes == 0:
        return "send_pause" if bound >= MIN_NONZERO_SET_AMPS else "rejected_pause_bound"
    return "rejected_above_configured" if amperes > bound else "send_set"


NATIVE_PAGE = ROOT / ".tmp/references/docs-modbus/protocols/tesla/gen3-semreg-mapping-v1.md"
OPERATION_STATEMENTS = (
    "`evse.operation.set_allocated_current`",
    "6 A as its minimum nonzero set",
    "1 A through 5 A is refused",
    "0 A requests a pause",
    "The pause encodes 6 A with inhibit and therefore needs a usable same-generation t28 configured bound of at least 6 A before t25.",
    "Release sends only the documented triple with a valid nonzero configured current",
    "Missing, zero or invalid configured current sends no release t25",
    "is the configured current of the latest t28 in the same lifecycle generation",
    "A value refused before a read is needed sends nothing.",
    "Where the generation has no t28, the t27 to t28 limit read may run first, and a value above the limit then read sends no t25",
    "do not reinterpret the specified 26.34.0 removal behavior or authorize a zero-timeout send",
)
# Section 7 relations: the connector-phase, connector-session and charger-meter statements and their dispositions.
RELATION_STATEMENTS = (
    "`evse.relation.connector_phase` is a gate rule where the pages document per-phase values",
    "`evse.relation.connector_session` is a gate rule where the pages document a session per connector",
    "A session episode requires a documented native session start.",
    "`evse.relation.evse_meter` is configuration for the charger's own energy counter",
    "Without those native facts the phase and session relations are unknown, not absent.",
    "A relation the pages do not support is unknown with a reason naming the missing statement.",
)
NATIVE_STATEMENTS = OPERATION_STATEMENTS + RELATION_STATEMENTS


CHARGING_CITATION = "The 6 A floor and the 0 A pause follow IEC 61851-1 Annex A, where 6 A is the lowest charging current a charger can signal."


def visible_paragraphs(source):
    """Paragraphs of the rendered page outside link definitions and HTML comments, whitespace-normalized; the citation must open a sentence."""
    shown = re.sub(r"(?m)^ {0,3}\[[^\]\n]+\]:[^\n]*(?:\n[ \t]+[\"'(][^\n]*)?$", " ", source)
    shown = re.sub(r"(?<!`)(`+)(?!`)(.+?)(?<!`)\1(?!`)", lambda m: m.group(2).replace("<", "&lt;"), shown, flags=re.DOTALL)
    shown = re.sub(r"<!--.*?-->", " ", shown, flags=re.DOTALL)
    shown = shown.replace("*", "")
    shown = re.sub(r"(?<!\w)_+|_+(?!\w)", "", shown)
    for block in re.split(r"\n\s*\n", shown):
        paragraph = " ".join(block.split())
        if paragraph:
            yield paragraph


def cites_charging_clause(source):
    pattern = re.compile(r"(?:^|[.!?] )" + re.escape(CHARGING_CITATION))
    return any(pattern.search(paragraph) for paragraph in visible_paragraphs(source))


def check_native_page():
    """The staged ND02 page must exist and state what this gate relies on; absence is never a pass."""
    if not NATIVE_PAGE.is_file():
        fail("native_page_missing")
    text = " ".join(NATIVE_PAGE.read_text(encoding="utf-8").split())
    if any(statement not in text for statement in NATIVE_STATEMENTS):
        fail("native_page_disagrees")
    if not cites_charging_clause(NATIVE_PAGE.read_text(encoding="utf-8")):
        fail("native_page_disagrees")


RELATION_TOKENS = ("connector-to-phase", "connector-to-session", "EVSE-to-meter", "unknown, not absent")


def check_companion_relations(text):
    """The companion states the phase, session and meter relations and their unknown disposition; missing ones fail."""
    if any(token not in text for token in RELATION_TOKENS):
        fail("document_relations")


def document(contract):
    check_native_page()
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
    check_companion_relations(text)
    for token in ("wire timeout", "effect target", "same source epoch", "`operation` member", "FC100", "outbound_allowed=false", "field-scoped immutable evidence", "evse.operation.set_allocated_current", "ConfigurePersistentCurrent", "mTLS GraphQL", "operator label", "unknown, not absent", "EN-1", "section 15"):
        if token not in text: fail("document_boundary")
    positive = sum(s["polarity"] == "positive" for s in contract["scenarios"])
    print(f"Tesla Gen3 WC3 24.44.3 EVSE current-limit mapping: PASS; {positive} executable outputs and {len(contract['scenarios']) - positive} rejected scenarios")


if __name__ == "__main__": main()
