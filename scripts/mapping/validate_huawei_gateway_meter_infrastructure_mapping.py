#!/usr/bin/env python3
"""Execute the Huawei gateway-meter gate: what helianthus.pack.infrastructure@1.0.0 will take from a meter once one is decoded, and nothing before."""
from __future__ import annotations

import copy
import importlib.util
import json
import re
from datetime import datetime
from functools import lru_cache
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
GATES = ROOT / "driverdata/families/mapping-gates"
SEMANTIC = ROOT / ".tmp/references/docs-semantic"
PATH = GATES / "huawei-gateway-meter-infrastructure-v1.json"
DOC = GATES / "huawei-gateway-meter-infrastructure-v1.md"
PACK_CATALOG = SEMANTIC / "api/v1/packs/infrastructure-acceptance-vectors.json"
LOCAL_ASSESSMENT = SEMANTIC / "mappings/infrastructure/gateway-runtime.md"
_GRAPH_SPEC = importlib.util.spec_from_file_location("mapping_gate_graph_rules", Path(__file__).resolve().parent / "mapping_gate_graph_rules.py")
graph_rules = importlib.util.module_from_spec(_GRAPH_SPEC)
_GRAPH_SPEC.loader.exec_module(graph_rules)

PACK = {"id": "helianthus.pack.infrastructure", "version": "1.0.0"}
NATIVE_PREFIX = "native.huawei.gateway_meter."
LOSS_KINDS = {"unit", "range", "precision", "time", "symbol", "provenance", "identity", "capability", "operation", "policy"}
GRADES = ["unknown", "hypothesis", "proven"]
ERROR_PRECEDENCE = ["gate_revision_required", "native_evidence_invalid", "identity_missing_or_invalid", "role_invalid", "lifecycle_missing", "mapping_unqualified", "unsupported_or_withheld"]
OPAQUE_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:/@-]*$")
HEX_RUN_RE = re.compile(r"(?<![0-9A-Za-z])[0-9a-fA-F]{32,}(?![0-9A-Za-z])")
# Approved immutable owner links: the pinned docs-semantic revision, owning path, optional non-line anchor.
OWNER_PATHS = ("api/v1/graph.md", "api/v1/element-catalog-v1.json", "api/README.md", "mappings/infrastructure/gateway-runtime.md")
OWNER_LINK_RE = re.compile(r"https://github\.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/(?:" + "|".join(re.escape(path) for path in OWNER_PATHS) + r")(?:#(?!L[0-9])(?![0-9a-fA-F]{32,}(?![0-9A-Za-z]))[A-Za-z][A-Za-z0-9_-]*)?(?![A-Za-z0-9_./#?=&%-])")
# A line anchor and a private or IPv4 endpoint are never published, whether or not an owner link surrounds them.
LINE_ANCHOR_RE = re.compile(r"#L[0-9]")
ENDPOINT_RE = re.compile(r"tcp://|(?<![0-9.])(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?![0-9.])")


def unowned_text(text):
    """The published text with approved owner links removed; every other revision literal stays visible."""
    return OWNER_LINK_RE.sub("", text)
ID_FORBIDDEN_TEXT = ("serial", "esn", "sun2000", "huawei", "luna", "smartlogger", "sdongle", "s-dongle", "emma")
ID_ADDRESS_RE = re.compile(r"(\d{1,3}\.){3}\d{1,3}|:\d{3,5}$")
# Exact conversions of the unit rule in api/README.md: (native unit, canonical SI unit) -> factor and offset, SI = native x factor + offset.
ONE, ZERO = {"coefficient": "1", "exponent10": 0}, {"coefficient": "0", "exponent10": 0}
IDENTITY = {"factor": ONE, "offset": ZERO}
UNIT_CONVERSION = {
    ("kW", "unit.watt"): {"factor": {"coefficient": "1", "exponent10": 3}, "offset": ZERO},
    ("W", "unit.watt"): IDENTITY,
    ("kWh", "unit.joule"): {"factor": {"coefficient": "36", "exponent10": 5}, "offset": ZERO},
    ("ratio", "unit.ratio"): IDENTITY,
}
# Members of a prospective row; no other member, and so no second statement of a conversion, is accepted.
PROSPECTIVE_ROW_MEMBERS = {"in_force", "target", "dimension", "subject_kind", "measurement_point", "unit", "native", "conversion", "loss", "sign_reference", "role", "direction", "counter"}
FORBIDDEN_DERIVATIONS = ["serial_number", "esn", "vendor", "model", "firmware_or_version", "routed_unit_alone", "endpoint_address"]
REGISTRY_FLAGS = {"catalog_registered": False, "automatic_runtime_admission": False, "live_qualified": False, "support_claim": False}
QUALIFICATION = {**REGISTRY_FLAGS, "mapping_qualified": True, "outbound_allowed": False}

def qualification_recorded(qualification):
    """ND02 decision D14: live qualification is a recorded level, never a rejection; the other flags and the mapping stay pinned."""
    if not isinstance(qualification, dict) or not isinstance(qualification.get("live_qualified"), bool):
        return False
    rest = {key: value for key, value in qualification.items() if key != "live_qualified"}
    return rest == {key: value for key, value in QUALIFICATION.items() if key != "live_qualified"}

EVIDENCE = {
    "monitoring_contract": {"repository": "helianthus-docs-modbus", "issue": 242, "pull_request": 253, "pages": ["protocols/huawei/monitoring-readonly-v1.md", "protocols/huawei/read-pacing-v1.md", "protocols/huawei/gateway-power-meter-v1.md"]},
    "meter_observation_amendment": {"repository": "helianthus-docs-modbus", "issue": 254},
    "registry_profiles": {"repository": "helianthus-modbusreg", "issue": 348, "pull_request": 357},
    "program_issues": {"repository": "helianthus-modbusreg", "issues": [199, 201]},
    "multi_endpoint_contract": {"repository": "helianthus-docs-ebus", "issue": 746, "pull_request": 748},
    "gateway_local_assessment": "mappings/infrastructure/gateway-runtime.md",
    "sibling_gates": ["huawei-sun2000-pv-v1", "huawei-luna2000-storage-v1"],
    "grades": ["proven", "hypothesis", "unknown"],
}
ADMISSION = {
    "meter_class_contract": False, "meter_identity": "INSUFFICIENT_EVIDENCE", "decoded_meter_words": 0,
    "selection_outcomes_allowed": ["INSUFFICIENT_EVIDENCE", "NO_ADMISSIBLE_PROFILE", "AMBIGUOUS"],
    "selected_meter": "fails_closed_until_this_gate_is_revised", "registry_flags": REGISTRY_FLAGS,
    "preconditions_for_any_mapping": [
        "a_meter_class_contract_exists_with_an_identity_read", "the_exposure_path_is_observed",
        "type_word_order_gain_unit_and_sign_of_each_consumed_word_are_stated", "a_sign_reference_is_graded_per_source_and_never_mixed",
        "the_role_is_declared_in_configuration", "this_gate_is_revised_with_executable_vectors"],
}
ROLE = {
    "source": "operator_configuration_keyed_to_endpoint_and_routed_unit",
    "values": ["grid_connection", "feeder", "circuit"],
    "undeclared": "role_unknown_and_role_dependent_facts_not_published",
    "never_derived_from": ["registers", "sign_or_magnitude_of_a_power_value", "label", "unit_number", "listing_order", "model", "presence_of_another_meter"],
}
# EN-8: each configured role names the pack dimension of the resource the meter measures. The role's relation is the
# catalog's measures definition between that resource and the meter, or unknown when the catalog has none.
ROLE_DIMENSIONS = {"grid_connection": "infrastructure.dimension.grid_connection", "feeder": "infrastructure.dimension.feeder", "circuit": "infrastructure.dimension.circuit"}
UNKNOWN_RELATION = "unknown"
# Section 15 item 1: one resource per role, in this order; a role's kind is its dimension's kind in the element catalog.
RESOURCE_ROLES = {"site": "infrastructure.dimension.site", "grid_connection": "infrastructure.dimension.grid_connection", "meter": "infrastructure.dimension.meter"}
# The statement each resource's evidence must carry, besides the native owner's token stability.
RESOURCE_STATEMENTS = {"site": "operator_configuration", "grid_connection": "operator_configuration", "meter": "native_contract"}
# EN-1: each measurement point names the resource role at whose boundary or terminal the quantity is measured or defined.
MEASUREMENT_POINTS = {"grid_connection_point": "grid_connection", "meter_register": "meter"}
# Section 15 item 2: (definition, basis class, scope of the rule) in the order the gate states them. Both are
# configuration: the native meter contract states that the role is configuration, and no site structure is observable.
STATED_RELATIONS = [
    ("infrastructure.relation.site_grid_connection", "configuration", "each_grid_connection_the_operator_declares_in_a_site"),
    ("infrastructure.relation.grid_connection_meter", "configuration", "each_meter_the_operator_declares_with_role_grid_connection"),
]
SCOPE = {"profile": "no_meter_class_contract", "version": "no_admitted_profile"}
# Section 6.4: no meter identity read exists, so two views of one meter join only by an operator declaration.
TWO_VIEWS = ["exposed_at_a_gateway_meter_unit", "exposed_by_an_inverter_child"]
SELECTORS = [
    {"selector": "gateway_path", "classification": "provenance", "evidence": ["native_contract:monitoring_contract", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]},
    {"selector": "profile_id", "classification": "provenance", "evidence": ["admission:registry_profiles_withhold_every_meter", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]},
]
# Section 7.2: what withholds each infrastructure relation the gate does not state.
UNSTATED_BY = {
    "infrastructure.relation.circuit_phase": "native_contract:phase_assignment_not_stated",
    "infrastructure.relation.feeder_circuit": "configuration:no_feeder_or_circuit_declared_by_this_gate",
    "infrastructure.relation.grid_connection_feeder": "configuration:no_feeder_or_circuit_declared_by_this_gate",
}
UNSTATED_OUTCOME = "unknown_not_absent"
INVENTORY = [
    ("smartlogger_meter_electrical_group", "smartlogger"), ("smartlogger_meter_energy_totals", "smartlogger"), ("emma_built_in_sensor_group", "emma"), ("emma_external_sensor_group", "emma"),
    ("sdongle_inverter_exposed_meter_group", "sdongle"), ("sdongle_plant_grid_power", "sdongle"), ("sun2000_inverter_exposed_meter_group_refused", "sun2000_m3"), ("profile_meter_groups", "sun2000_profiles"),
]
FEEDABLE = [nid for nid, path in INVENTORY[:6]]
PRIMARY_REASON = "infrastructure.reason.no_meter_class_contract"
RUNTIME_REASONS = {"infrastructure.reason.answered_not_decoded", "infrastructure.reason.read_exception_no_negation"}
REQUESTED = ["infrastructure.ac.active_power", "infrastructure.ac.current", "infrastructure.ac.power_factor", "infrastructure.ac.voltage", "infrastructure.energy.export", "infrastructure.energy.import", "infrastructure.power.export_active", "infrastructure.power.import_active"]
UNSUPPORTED = ["role_inference", "direction_split_without_a_stated_sign", "mixed_sign_references", "meter_identity", "connection_readiness_breaker_fault_interlock_state", "gateway_runtime_state_as_electrical_state", "write_operations", "acknowledgement", "readback", "retry", "control_routes"]
SIGN_REFERENCES = {"positive_export", "positive_import", "not_stated"}
DOC_TOKENS = ("gateway-configured", "unknown, never zero", "no binary floating point", "outbound_allowed=false", "one atomic cutover", "role is configuration", "no meter register is decoded",
              "section 15", "en-1", "en-8", "operator label", "unknown, not absent", "never the device that reports it")


def fail(code):
    raise ValueError(code)


class Catalog:
    """Element-catalog lookups for this gate: kinds, relations, facts, and the infrastructure pack's dimensions and relations."""

    def __init__(self, document):
        self.document = document
        self.index = graph_rules.Index(document)
        self.dimensions, self.pack_relations = {}, []
        for element in document.get("elements", []):
            ref = element["ref"]
            if element.get("disposition") != "member" or ref["owner"].get("pack") != PACK:
                continue
            if element["element_kind"] == "dimension":
                self.dimensions[ref["id"]] = element["attributes"]["resource_kind"]
            elif element["element_kind"] == "relation":
                self.pack_relations.append(ref["id"])

    def fact(self, fact_id):
        return (self.index.facts.get(fact_id) if isinstance(fact_id, str) else None) or {}

    def subject(self, fact_id):
        return self.fact(fact_id).get("subject", {})

    def role_kinds(self):
        """RES-1 and section 6.1: the kind of each resource role, through its pack dimension."""
        return {role: self.dimensions.get(dimension) for role, dimension in RESOURCE_ROLES.items()}

    def role_relation(self, role):
        """EN-8: the pack's measures definition from the role's measured resource to the meter, or unknown."""
        measured, meter = self.dimensions.get(ROLE_DIMENSIONS[role]), self.role_kinds()["meter"]
        found = []
        for name in self.pack_relations:
            definition = self.index.relations[name]
            ends = [[measured], [meter]] if definition["orientation"] == "inverse" else [[meter], [measured]]
            if definition["core_type"] == "measures" and [definition["from_kinds"], definition["to_kinds"]] == ends:
                found.append(name)
        return found[0] if len(found) == 1 else UNKNOWN_RELATION


@lru_cache(maxsize=1)
def element_catalog():
    return Catalog(graph_rules.load_catalog())


def check_prospective_conversion(row):
    """A prospective row states the exact conversion of its unit pair and its loss kinds: a unit loss exactly when the conversion is not the identity, and a policy loss on every counter."""
    expected = UNIT_CONVERSION.get(((row.get("native") or {}).get("unit"), row.get("unit")))
    losses = row.get("loss")
    if expected is None or row.get("conversion") != expected or not set(row) <= PROSPECTIVE_ROW_MEMBERS or not isinstance(losses, list) or not all(isinstance(kind, str) and kind in LOSS_KINDS for kind in losses) or len(losses) != len(set(losses)):
        fail("prospective conversion")
    if (expected != IDENTITY) != ("unit" in losses):
        fail("silent unit conversion")
    if row["target"].startswith("infrastructure.energy.") and (row.get("direction") not in ("stated", "not_stated") or row.get("counter") != "reset_wrap_and_continuity_never_inferred" or "policy" not in losses):
        fail("prospective counter")


def plain(number):
    """A kernel Decimal written without an exponent, for the page."""
    digits, exponent = number["coefficient"].lstrip("-"), number["exponent10"]
    sign = "-" if number["coefficient"].startswith("-") else ""
    if exponent >= 0:
        return sign + digits + "0" * exponent
    digits = digits.rjust(1 - exponent, "0")
    return sign + digits[:exponent] + "." + digits[exponent:]


def _no_float(text):
    raise ValueError("binary floating point literal in JSON: " + text)


def _no_duplicates(pairs):
    keys = [key for key, _ in pairs]
    if len(keys) != len(set(keys)):
        raise ValueError("duplicate JSON member")
    return dict(pairs)


def loads(text):
    return json.loads(text, parse_float=_no_float, object_pairs_hook=_no_duplicates)


def load():
    return loads(PATH.read_text(encoding="utf-8"))


def contains_float(value):
    if isinstance(value, float):
        return True
    if isinstance(value, dict):
        return any(contains_float(item) for item in value.values())
    if isinstance(value, list):
        return any(contains_float(item) for item in value)
    return False


def mutate(value, mutation):
    cursor, parts = value, mutation["path"].split(".")
    for part in parts[:-1]:
        cursor = cursor[int(part)] if isinstance(cursor, list) else cursor[part]
    last = parts[-1]
    key = int(last) if isinstance(cursor, list) else last
    if mutation.get("remove"):
        del cursor[key]
    else:
        cursor[key] = mutation["value"]


def pack_catalog():
    catalog = json.loads(PACK_CATALOG.read_text(encoding="utf-8"))["catalog"]["definitions"]
    return {field["id"]: field for field in catalog["fields"]}, {dimension["id"] for dimension in catalog["fact_dimensions"]}, {rel["id"] for rel in catalog["relations"]}


def check_shared(document, catalog):
    """Section 15: the shared graph rules first; a gate of this family states its graph."""
    failures = graph_rules.check(document, catalog.document)
    if failures:
        fail("graph {} {}".format(*failures[0]))
    if not graph_rules.graph_stated(document):
        fail("graph_not_stated")


def measured_end(definition):
    """The measured resource kinds of a measures definition: its starting end when it reads the core type backwards."""
    return definition["from_kinds"] if definition["orientation"] == "inverse" else definition["to_kinds"]


def check_role(document, catalog):
    """EN-8: each role's relation is the catalog's measures definition, or unknown; a stated role relation has an instance with basis configuration."""
    if document.get("role") != {**ROLE, "relation_by_role": {role: catalog.role_relation(role) for role in ROLE["values"]}}:
        fail("role")
    stated = {relation["definition"]: relation for relation in document["graph"]["relations"]}
    for relation in document["role"]["relation_by_role"].values():
        if relation != UNKNOWN_RELATION and stated.get(relation, {}).get("basis_class") != "configuration":
            fail("role relation")


def check_subject(row, catalog):
    """EN-1: a row names its target's catalog subject kind and a measurement point at the boundary of that kind."""
    point = row.get("measurement_point")
    role = MEASUREMENT_POINTS.get(point) if isinstance(point, str) else None
    if row.get("subject_kind") != catalog.subject(row.get("target")).get("kind") or role is None or catalog.role_kinds()[role] != row["subject_kind"]:
        fail("field rule subject")


def check_row_role(row, document, catalog):
    """EN-8: a quantity of the measured resource needs a role whose relation measures that resource; a meter quantity takes any role."""
    if row["subject_kind"] == catalog.role_kinds()["meter"]:
        if "role" in row:
            fail("prospective role")
        return
    role = row.get("role")
    relation = document["role"]["relation_by_role"].get(role) if isinstance(role, str) else None
    definition = catalog.index.relations.get(relation) if isinstance(relation, str) else None
    if definition is None or row["subject_kind"] not in measured_end(definition):
        fail("prospective role")


def role_placement(contract, role, catalog=None):
    """Where a role places the measured-resource quantities and the meter quantities: subjects from the catalog, relation from the role."""
    catalog = element_catalog() if catalog is None else catalog
    meter = catalog.role_kinds()["meter"]
    measured, metered = set(), set()
    for rule in contract["field_rules"]:
        for row in rule.get("prospective", []):
            subject = catalog.subject(row.get("target"))
            (metered if subject.get("kind") == meter else measured).add(subject.get("dimension", {}).get("id"))
    if len(measured) != 1 or len(metered) != 1:
        fail("role placement")
    relation = contract["role"]["relation_by_role"][role]
    return {"power": "withheld" if relation == UNKNOWN_RELATION else measured.pop(), "energy": metered.pop(), "relation": relation}


def expected_basis(resource, anchor_class):
    """Section 6.3: a listed scheme anchors on a native token, a component on its native position, anything else on an operator label."""
    if resource["identification_scheme"] != graph_rules.NONE_LISTED:
        return "native_identity"
    return "native_slot" if anchor_class == "component" else "operator_slot"


def check_graph_resources(graph, catalog):
    """Section 15 item 1, RES-1 and RES-2: one resource per role, in role order, with its anchor, no parent and its evidence statements."""
    kinds = catalog.role_kinds()
    if [resource["kind"] for resource in graph["resources"]] != list(kinds.values()):
        fail("graph_resources")
    for role, resource in zip(kinds, graph["resources"]):
        if resource["anchor_basis"] != expected_basis(resource, catalog.index.kinds[resource["kind"]]["anchor_class"]):
            fail("graph_anchor")
        if resource["parent_kind"] is not None:
            fail("graph_parent")
        stated = {item.split(":")[0] for item in resource["evidence"]}
        if not {"native_owner_token_stability", RESOURCE_STATEMENTS[role]} <= stated:
            fail("graph_evidence")


def check_graph_paths(graph, catalog):
    """Section 15 item 3 and section 6.4: no relay while the exposure path is unobserved; a second view joins on a shared key or a declaration only."""
    if graph["relayed"] != []:
        fail("graph_relayed")
    kind = catalog.role_kinds()["meter"]
    meter = next(resource for resource in graph["resources"] if resource["kind"] == kind)
    native = meter["anchor_basis"] == "native_identity"
    join = {"kind": kind, "observations": TWO_VIEWS, "basis": "same_anchor_key" if native else "operator_declaration", "native_tokens": "equal" if native else "not_exposed"}
    if graph["joins"] != [join]:
        fail("graph_joins")


def check_graph_relations(graph, catalog):
    """Section 15 item 2, REL-3 and section 7.2: stated basis and rule scope, catalog endpoints, the gate's scope, no absence statement."""
    if [(relation["definition"], relation["basis_class"], relation["validity_scope"]["rule_scope"]) for relation in graph["relations"]] != STATED_RELATIONS:
        fail("graph_relations")
    for relation in graph["relations"]:
        definition = catalog.index.relations[relation["definition"]]
        stated_scope = {key: relation["validity_scope"][key] for key in SCOPE}
        if (relation["from_kind"], relation["to_kind"]) != (definition["from_kinds"][0], definition["to_kinds"][0]) or relation["absence_sides"] or stated_scope != SCOPE:
            fail("graph_relations")


def expected_aggregates(document, catalog):
    """Section 15 item 4, EN-1 and EN-5: each prospective target the catalog calls an aggregate, on its catalog subject, without coverage."""
    aggregates = []
    for rule in document["field_rules"]:
        for row in rule.get("prospective", []):
            fact = catalog.fact(row["target"])
            entry = {"fact": row["target"], "subject_kind": fact.get("subject", {}).get("kind"), "coverage": []}
            if fact.get("aggregation") == "aggregate" and entry not in aggregates:
                aggregates.append(entry)
    return aggregates


def expected_unstated(document, catalog):
    """Section 7.2: an infrastructure relation the gate does not state stays unknown, never absent."""
    stated = {relation["definition"] for relation in document["graph"]["relations"]}
    rows = []
    for name in catalog.pack_relations:
        if name not in stated:
            definition = catalog.index.relations[name]
            rows.append({"definition": name, "from_kind": definition["from_kinds"][0], "to_kind": definition["to_kinds"][0], "outcome": UNSTATED_OUTCOME, "withheld_by": UNSTATED_BY.get(name)})
    return rows


def validate_graph(document, catalog):
    """Section 15: this gate's own rules after the shared checker; no member decides a plane."""
    graph = document["graph"]
    check_graph_resources(graph, catalog)
    check_graph_paths(graph, catalog)
    check_graph_relations(graph, catalog)
    if graph["aggregates"] != expected_aggregates(document, catalog):
        fail("graph_aggregates")
    if graph["selectors"] != SELECTORS:
        fail("graph_selectors")
    if document.get("unstated_relations") != expected_unstated(document, catalog):
        fail("unstated_relations")


def validate_contract(document, catalog=None):
    catalog = element_catalog() if catalog is None else Catalog(catalog)
    check_shared(document, catalog)
    fields = pack_catalog()[0]
    check_meter_header(document)
    check_role(document, catalog)
    rules = document.get("field_rules", [])
    if [(rule.get("native_id"), rule.get("gateway_path")) for rule in rules] != INVENTORY:
        fail("native inventory")
    reasons = document.get("reasons", {})
    check_meter_reasons(reasons)
    used, targets = check_meter_rules(document, rules, reasons, fields, catalog)
    if not targets <= set(REQUESTED) or document.get("projection", {}).get("requested_items") != [{"kind": "fact", "item_id": item} for item in REQUESTED] or any(item not in fields for item in REQUESTED):
        fail("requested items")
    if set(reasons) != used | RUNTIME_REASONS | {PRIMARY_REASON}:
        fail("reason inventory")
    check_meter_policies(document)
    for role in ROLE["values"]:
        role_placement(document, role, catalog)
    validate_graph(document, catalog)
    if contains_float(document):
        fail("binary floating point")


def check_meter_header(document):
    if document.get("contract") != "helianthus.semantic.mapping.huawei-gateway-meter.infrastructure/v1" or document.get("mapping_id") != "infrastructure.mapping.huawei.gateway_meter" or document.get("pack") != PACK or document.get("status") != "accepted_withheld_mapping_gate_no_meter_decoded":
        fail("contract")
    if document.get("evidence") != EVIDENCE or not LOCAL_ASSESSMENT.is_file():
        fail("evidence")
    if document.get("admission") != ADMISSION:
        fail("admission")


def check_meter_reasons(reasons):
    for name, spec in reasons.items():
        if not name.startswith("infrastructure.reason.") or spec.get("loss_kind") not in LOSS_KINDS or not spec.get("description") or type(spec.get("reversible")) is not bool:
            fail("reason loss")


def check_meter_rules(document, rules, reasons, fields, catalog):
    used, targets = set(), set()
    for rule in rules:
        disposition = rule.get("disposition", {})
        if set(disposition) != {"outcome", "reason"} or disposition["outcome"] != "withheld" or disposition["reason"] not in reasons or rule.get("grade") not in GRADES:
            fail("withheld reason")
        used.add(disposition["reason"])
        for row in rule.get("prospective", []):
            targets.add(check_meter_prospective(row, document, fields, catalog))
    return used, targets


def check_meter_prospective(row, document, fields, catalog):
    pack_field = fields.get(row.get("target"))
    native = row.get("native")
    if row.get("in_force") is not False or pack_field is None or row.get("dimension") != catalog.subject(row["target"]).get("dimension", {}).get("id") or row.get("unit") != pack_field.get("unit") or not isinstance(native, dict):
        fail("prospective target")
    check_subject(row, catalog)
    check_row_role(row, document, catalog)
    if row["target"].startswith("infrastructure.power.") and row.get("sign_reference") not in SIGN_REFERENCES:
        fail("prospective sign")
    check_prospective_conversion(row)
    return row["target"]


def check_meter_policies(document):
    identity = document.get("identity", {})
    if identity != {"source_id": {"source": "gateway_configured_semantic_source_id", "required": True, "non_secret": True, "forbidden_derivations": FORBIDDEN_DERIVATIONS},
                    "meter_asset_id": "none_exists_until_a_meter_class_contract_gives_an_identity", "failure": "missing_or_invalid_source_identity_fails_closed"}:
        fail("identity contract")
    if document.get("lifecycle") != {
        "required_observation_evidence": ["observation_id", "observation_revision", "receipt_wall", "receipt_monotonic", "clock_epoch", "source_epoch", "driver_generation", "connection_generation", "selection", "role", "reads"],
        "absent_optional_group": "unknown_never_zero", "exception": "no_negation_of_the_gateway_or_an_identified_inverter", "withdrawal": "explicit_generation_fenced"}:
        fail("lifecycle contract")
    if document.get("outcome_vocabulary") != {"exact": "exact", "transformed": "transformed", "withheld": "withheld", "native_only": "unrepresentable"}:
        fail("outcome vocabulary")
    if document.get("unsupported") != UNSUPPORTED:
        fail("unsupported")
    projection = document.get("projection", {})
    if projection.get("primary_reason") != PRIMARY_REASON or projection.get("error_precedence") != ERROR_PRECEDENCE or projection.get("operations") != "unsupported_no_authority" or projection.get("missing_native") != "unavailable_withheld_never_zero" or projection.get("target") != "SemReg_infrastructure_projection":
        fail("projection")
    if document.get("consumer_cutover") != {"consumers": ["semantic_mcp", "graphql", "portal", "home_assistant"], "path": "shared_semantic_snapshot_only", "until_implemented": "no_public_consumer_binding"}:
        fail("consumer cutover")


def validate_case(case, contract):
    if not isinstance(case, dict):
        fail("native_evidence_invalid")
    selection = case.get("selection")
    outcome = selection.get("outcome") if isinstance(selection, dict) else None
    if outcome == "PROFILE_SELECTED" or "asset_id" in (case.get("identity") if isinstance(case.get("identity"), dict) else {}):
        fail("gate_revision_required")
    errors = set()
    if outcome not in ADMISSION["selection_outcomes_allowed"] or contains_float(case):
        errors.add("native_evidence_invalid")
    reads = check_case_reads(case, errors)
    source_id, unit = check_case_source(case, errors)
    declared = check_case_role(case, errors)
    check_case_lifecycle(case, errors)
    if not qualification_recorded(case.get("qualification")):
        errors.add("mapping_unqualified")
    if case.get("request_operation") or case.get("request_direction_split") or case.get("request_mixed_sign_references"):
        errors.add("unsupported_or_withheld")
    for code in ERROR_PRECEDENCE:
        if code in errors:
            fail(code)
    return reads, source_id, unit, declared


def check_case_reads(case, errors):
    reads = {}
    for read in case.get("reads") if isinstance(case.get("reads"), list) else [None]:
        key = read.get("native_id") if isinstance(read, dict) else None
        if key not in FEEDABLE or key in reads or read.get("outcome") not in ("ANSWERED", "EXCEPTION", "FAILED", "NOT_READ") or set(read) - {"native_id", "outcome", "exception_code"}:
            errors.add("native_evidence_invalid")
            continue
        code = read.get("exception_code")
        if (read["outcome"] == "EXCEPTION") != (type(code) is int and code >= 1):
            errors.add("native_evidence_invalid")
        reads[key] = read
    return reads


def check_case_source(case, errors):
    source = case.get("source")
    source_id = source.get("source_id") if isinstance(source, dict) else None
    unit = case.get("routed_unit")
    if not isinstance(source_id, str) or not OPAQUE_RE.fullmatch(source_id) or any(token in source_id.lower() for token in ID_FORBIDDEN_TEXT) or ID_ADDRESS_RE.search(source_id) or (unit is not None and (type(unit) is not int or not 1 <= unit <= 247)) or source_id == str(unit):
        errors.add("identity_missing_or_invalid")
    return source_id, unit


def check_case_role(case, errors):
    role = case.get("role")
    declared = role.get("declared") if isinstance(role, dict) else None
    if not isinstance(role, dict) or role.get("source") != "operator_configuration" or (declared is not None and declared not in ROLE["values"]):
        errors.add("role_invalid")
    return declared


def check_case_lifecycle(case, errors):
    life = case.get("lifecycle") if isinstance(case.get("lifecycle"), dict) else {}
    if any(not isinstance(life.get(key), str) or not OPAQUE_RE.fullmatch(life[key]) for key in ("observation_id", "clock_epoch", "source_epoch")) or any(type(life.get(key)) is not int or life[key] < 1 for key in ("observation_revision", "driver_generation", "connection_generation")) or type(life.get("receipt_monotonic")) is not int or life.get("receipt_monotonic", 0) < 1:
        errors.add("lifecycle_missing")
    try:
        if datetime.fromisoformat(life.get("receipt_wall", "").replace("Z", "+00:00")).tzinfo is None:
            raise ValueError
    except (AttributeError, ValueError):
        errors.add("lifecycle_missing")


def project(case, contract):
    reads, source_id, unit, declared = validate_case(case, contract)
    rows = []
    for rule in contract["field_rules"]:
        reason = rule["disposition"]["reason"]
        read = reads.get(rule["native_id"])
        if read is not None and read["outcome"] == "ANSWERED":
            reason = "infrastructure.reason.answered_not_decoded"
        elif read is not None and read["outcome"] == "EXCEPTION":
            reason = "infrastructure.reason.read_exception_no_negation"
        rows.append({"native_id": rule["native_id"], "outcome": "withheld", "reason": reason})
    spec = contract["reasons"][PRIMARY_REASON]
    dispositions = [{"kind": "fact", "item_id": item["item_id"], "outcome": "withheld", "source_keys": [], "reason": PRIMARY_REASON,
                     "loss": [{"kind": spec["loss_kind"], "source_items": [NATIVE_PREFIX + "meter"], "description": spec["description"], "reversible": spec["reversible"]}]} for item in contract["projection"]["requested_items"]]
    dimensions = role_placement(contract, declared) if declared else "role_unknown_role_dependent_facts_not_published"
    role = {"declared": declared, "keyed_to": {"source_id": source_id, "routed_unit": unit}, "dimension_if_admitted": dimensions}
    output = {"facts": [], "relations": [], "requested": copy.deepcopy(contract["projection"]["requested_items"]), "dispositions": dispositions, "native_dispositions": rows, "role": role, "operations": []}
    assert_complete(output, contract)
    return output


def assert_complete(output, contract):
    """Every native item carries exactly one disposition, a reason, and no value."""
    if [row["native_id"] for row in output["native_dispositions"]] != [nid for nid, _ in INVENTORY] or output["facts"] or output["relations"]:
        fail("native disposition completeness")
    if any(row["outcome"] != "withheld" or row["reason"] not in contract["reasons"] for row in output["native_dispositions"]):
        fail("withheld without reason")


def summarize(output):
    return {"facts": output["facts"], "dispositions": [{"item_id": item["item_id"], "outcome": item["outcome"], "published": len(item["source_keys"]), "reason": item["reason"], "loss": [loss["kind"] for loss in item["loss"]]} for item in output["dispositions"]],
            "native_dispositions": output["native_dispositions"], "role": output["role"], "operations": output["operations"]}


def run_scenario(scenario, contract):
    case = copy.deepcopy(contract["baselines"][scenario["baseline"]])
    for mutation in scenario.get("mutations", []):
        mutate(case, mutation)
    return project(case, contract)


def validate_scenarios(contract):
    scenarios = contract.get("scenarios", [])
    if set(contract.get("baselines", {})) != {"default"} or not scenarios or scenarios[0].get("mutations") or scenarios[0].get("polarity") != "positive":
        fail("scenario baselines")
    ids = [scenario.get("id") for scenario in scenarios]
    if len(ids) != len(set(ids)):
        fail("scenario ids")
    seen_reasons, seen_errors = set(), set()
    for scenario in scenarios:
        expected = scenario.get("expect", {})
        if scenario.get("polarity") == "negative":
            check_negative_scenario(scenario, expected, contract, seen_errors)
            continue
        output = run_scenario(scenario, contract)
        check_positive_scenario(scenario, expected, output)
        seen_reasons |= {row["reason"] for row in output["native_dispositions"]} | {item["reason"] for item in output["dispositions"]}
    if seen_errors != set(ERROR_PRECEDENCE):
        fail("negative scenarios do not cover every error code")
    if seen_reasons != set(contract["reasons"]):
        fail("scenarios do not cover every reason")


def check_negative_scenario(scenario, expected, contract, seen_errors):
    if set(expected) != {"error"} or expected["error"] not in ERROR_PRECEDENCE:
        fail("negative scenario shape")
    try:
        run_scenario(scenario, contract)
    except ValueError as error:
        if str(error) != expected["error"]:
            raise
        seen_errors.add(expected["error"])
    else:
        fail(scenario["id"] + " accepted")


def check_positive_scenario(scenario, expected, output):
    if "output" in expected:
        if output != expected["output"]:
            fail("scenario output " + scenario["id"])
    elif summarize(output) != expected.get("summary"):
        fail("scenario summary " + scenario["id"])


def document(contract):
    validate_contract(contract)
    validate_scenarios(contract)


def table(text, heading):
    lines = text.splitlines()
    try:
        start = lines.index(heading)
    except ValueError:
        fail("document heading " + heading)
    rows = []
    for line in lines[start + 1:]:
        if line.startswith("## "):
            break
        if line.startswith("|"):
            rows.append([cell.strip().replace("`", "") for cell in line.strip().strip("|").split("|")])
    return rows[2:]


def native_form(native):
    return "%s, %s, gain %d" % (native["type"], native["unit"], native["gain"])


def expected_tables(contract):
    withheld, prospective = [], []
    for rule in contract["field_rules"]:
        reason = rule["disposition"]["reason"]
        withheld.append([rule["native_id"], rule["gateway_path"], rule["grade"].capitalize(), reason, contract["reasons"][reason]["loss_kind"]])
        for row in rule.get("prospective", []):
            rule_text = row.get("sign_reference") or ("direction %s; %s" % (row["direction"], row["counter"]) if "direction" in row else "none")
            conversion = "%s, factor %s, offset %s" % (row["unit"], plain(row["conversion"]["factor"]), plain(row["conversion"]["offset"]))
            prospective.append([row["target"], row["dimension"], rule["native_id"], native_form(row["native"]), conversion, "+".join(row["loss"]) or "none", rule_text, row.get("role", "any role")])
    return withheld, prospective


def graph_tables(contract):
    """The page's role, resource, relation, unstated-relation and subject tables, as the gate states them."""
    graph = contract["graph"]
    placements = {role: role_placement(contract, role) for role in contract["role"]["values"]}
    return {
        "## Role and dimension": [[role, p["power"], p["energy"], p["relation"]] for role, p in placements.items()] + [["undeclared", "withheld", "withheld", "none"]],
        "## Resources": [[r["kind"], r["anchor_class"], r["anchor_basis"], r["anchor_source"], r["parent_kind"] or "none"] for r in graph["resources"]],
        "## Relations": [[r["definition"], r["basis_class"], r["from_kind"], r["to_kind"], r["validity_scope"]["rule_scope"]] for r in graph["relations"]],
        "## Unknown, not absent": [[r["definition"], r["from_kind"], r["to_kind"], r["withheld_by"]] for r in contract["unstated_relations"]],
        "## Subjects and measurement points": [[rule["native_id"], row["target"], row["subject_kind"], row["measurement_point"]] for rule in contract["field_rules"] for row in rule.get("prospective", [])],
    }


def check_document(contract, text):
    withheld, prospective = expected_tables(contract)
    if table(text, "## Withheld items") != withheld or table(text, "## Prospective intake") != prospective:
        fail("document tables")
    if any(table(text, heading) != rows for heading, rows in graph_tables(contract).items()):
        fail("document tables")
    if [row[0] for row in table(text, "## Acceptance vectors")] != [scenario["id"] for scenario in contract["scenarios"]]:
        fail("document vectors")
    flat = " ".join(text.split()).lower()
    for token in DOC_TOKENS:
        if token not in flat:
            fail("document boundary")
    unowned = unowned_text(text)
    if HEX_RUN_RE.search(unowned) or HEX_RUN_RE.search(json.dumps(contract)) or re.search(r"/(?:blob|commit|tree)/", unowned) or LINE_ANCHOR_RE.search(unowned) or ENDPOINT_RE.search(unowned):
        fail("publication: digest or revision literal")


def main():
    contract = load()
    document(contract)
    check_document(contract, DOC.read_text(encoding="utf-8"))
    positives = sum(scenario["polarity"] == "positive" for scenario in contract["scenarios"])
    print(f"Huawei gateway-meter infrastructure mapping: PASS; {positives} executable outputs and {len(contract['scenarios']) - positives} rejected scenarios")


if __name__ == "__main__":
    main()
