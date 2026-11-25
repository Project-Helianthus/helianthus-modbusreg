#!/usr/bin/env python3
"""Execute the Huawei SUN2000 PV mapping gate: admitted monitoring facts to helianthus.pack.pv@1.0.0."""
from __future__ import annotations

import copy
import importlib.util
import json
import re
from datetime import datetime
from decimal import Decimal, localcontext
from functools import lru_cache
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
GATES = ROOT / "driverdata/families/mapping-gates"
SEMANTIC = ROOT / ".tmp/references/docs-semantic"
PATH = GATES / "huawei-sun2000-pv-v1.json"
DOC = GATES / "huawei-sun2000-pv-v1.md"
PACK_CATALOG = SEMANTIC / "api/v1/packs/pv-inverter-acceptance-vectors.json"
_GRAPH_SPEC = importlib.util.spec_from_file_location("mapping_gate_graph_rules", Path(__file__).resolve().parent / "mapping_gate_graph_rules.py")
graph_rules = importlib.util.module_from_spec(_GRAPH_SPEC)
_GRAPH_SPEC.loader.exec_module(graph_rules)

PACK = {"id": "helianthus.pack.pv", "version": "1.0.0"}
M3 = "huawei.sun2000.m3.monitoring-readonly@1.0.0"
M1 = "huawei.sun2000.m1.monitoring-readonly@1.0.0"
NATIVE_PREFIX = "native.huawei.sun2000."
LOSS_KINDS = {"unit", "range", "precision", "time", "symbol", "provenance", "identity", "capability", "operation", "policy"}
OUTCOMES = {"exact", "transformed", "withheld", "native_only"}
GRADES = ["unknown", "hypothesis", "proven"]
ERROR_PRECEDENCE = ["profile_not_selected", "native_evidence_invalid", "identity_missing_or_invalid", "lifecycle_missing", "mapping_unqualified", "unsupported_or_withheld"]
OPAQUE_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:/@-]*$")
COEFFICIENT_RE = re.compile(r"^(0|-?[1-9][0-9]*)$")
HEX_RUN_RE = re.compile(r"(?<![0-9A-Za-z])[0-9a-fA-F]{32,}(?![0-9A-Za-z])")
# Approved immutable owner links: the pinned docs-semantic revision, owning path, optional non-line anchor.
OWNER_LINK_RE = re.compile(r"https://github\.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/[A-Za-z0-9_./-]+(?:#(?!L[0-9])[A-Za-z0-9_-]+)?(?![A-Za-z0-9_./#-])")


def unowned_text(text):
    """The published text with approved owner links removed; every other revision literal stays visible."""
    return OWNER_LINK_RE.sub("", text)
ID_FORBIDDEN_TEXT = ("serial", "esn", "sun2000", "huawei", "luna", "smartlogger", "sdongle", "s-dongle", "emma")
ID_ADDRESS_RE = re.compile(r"(\d{1,3}\.){3}\d{1,3}|:\d{3,5}$")
WORD_TYPES = {"I16": (1, True), "U16": (1, False), "I32": (2, True), "U32": (2, False)}
GAIN_EXPONENT = {1: 0, 10: -1, 100: -2, 1000: -3}
# Exact conversions of the unit rule in api/README.md: (native unit, canonical SI unit) -> factor and offset, SI = native x factor + offset.
ONE, ZERO = {"coefficient": "1", "exponent10": 0}, {"coefficient": "0", "exponent10": 0}
IDENTITY = {"factor": ONE, "offset": ZERO}
UNIT_CONVERSION = {
    ("V", "unit.volt"): IDENTITY,
    ("A", "unit.ampere"): IDENTITY,
    ("kW", "unit.watt"): {"factor": {"coefficient": "1", "exponent10": 3}, "offset": ZERO},
    ("kWh", "unit.joule"): {"factor": {"coefficient": "36", "exponent10": 5}, "offset": ZERO},
}
# Members of a mapped rule; no other member, and so no second statement of a conversion, is accepted.
QUANTITY_RULE_MEMBERS = {"native_id", "native", "requested", "target", "dimension", "subject_kind", "measurement_point", "unit", "conversion", "disposition", "grade_ceiling", "sign"}
STATUS_RULE_MEMBERS = {"native_id", "native", "requested", "target", "dimension", "subject_kind", "measurement_point", "unit", "disposition", "grade_ceiling", "symbols"}
# EN-1: the measurement point of each mapped native fact. Subject kind and dimension come from the element catalog.
MEASUREMENT_POINTS = {
    "pv_string_voltage": "dc_input_terminals", "pv_string_current": "dc_input_terminals",
    "input_power": "inverter_dc_side", "active_power": "inverter_ac_output",
    "device_status": "inverter_as_a_whole", "energy_accumulated": "inverter_ac_output_lifetime_counter",
    "energy_current_day": "inverter_ac_output_day_counter",
}
FORBIDDEN_DERIVATIONS = ["serial_number", "esn", "vendor", "model", "firmware_or_version", "routed_unit_alone", "endpoint_address"]
# Section 6.3: a pack class token and a profile identifier are interpretation selectors, so neither derives the home asset.
ASSET_FORBIDDEN = FORBIDDEN_DERIVATIONS + ["class_token", "profile_id"]
ALLOCATED = "gateway_allocated_resource_id"
# One gateway-allocated ResourceID per resource role (section 6.3); a role's kind is its dimension's kind in the element catalog.
RESOURCE_ROLES = {
    "logger": {"kind": "resource.equipment", "source": ALLOCATED, "required": True},
    "system": {"dimension": "pv.dimension.system", "source": ALLOCATED, "required": True},
    "inverter": {"dimension": "pv.dimension.inverter", "source": ALLOCATED, "required": True},
    "inputs": {"dimension": "pv.dimension.input", "source": ALLOCATED, "required": True, "slots": 8},
    "phases": {"dimension": "pv.dimension.phase", "source": "not_allocated_no_profile_admits_a_per_phase_word", "required": False},
}
IDENTITY_CONTRACT = {
    "asset_id": {"source": "gateway_allocated_asset_id", "required": True, "non_secret": True, "granularity": "one_home_asset_per_admitted_inverter_child_within_an_endpoint_instance", "forbidden_derivations": ASSET_FORBIDDEN},
    "source_id": {"source": "gateway_managed_instance_source_id", "required": True, "non_secret": True, "granularity": "one_source_per_endpoint_instance", "distinct_from": "asset_id", "forbidden_derivations": FORBIDDEN_DERIVATIONS},
    "binding_id": {"source": "gateway_domain_separated_asset_source_profile_binding", "required": True, "non_secret": True, "stable_inputs": ["asset_id", "source_id", "profile_id"]},
    "resources": RESOURCE_ROLES,
    "failure": "missing_invalid_reused_or_ambiguous_identity_fails_closed",
}
# Section 15 item 2: (definition, basis class, scope of the rule) in the order the gate states them. A pack relation's
# endpoint kinds come from the element catalog; those of relays from the resource roles.
STATED_RELATIONS = [
    ("relation.relays", "native_observation", "each_admitted_child_is_read_through_the_gateway_path_its_profile_names"),
    ("pv.relation.system_inverter", "gate_rule", "one_pv_system_per_admitted_inverter_child"),
    ("pv.relation.inverter_input", "gate_rule", "native_string_index_1_to_8_with_a_nonzero_voltage_or_current_word"),
]
# Section 6.4: an inverter relayed by its gateway path and also seen directly.
TWO_PATHS = ["relayed_by_the_gateway_path", "observed_directly"]
# EN-5: the native power words state no aggregation basis, so their aggregates stay on the inverter without coverage.
COVERAGE = {"pv.energy.generated": ["pv.relation.system_inverter"], "pv.energy.yield_day": ["pv.relation.system_inverter"]}
SELECTORS = [
    {"selector": "profile_id", "classification": "provenance", "evidence": ["native_contract:selection_outcomes", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]},
    {"selector": "class_token", "classification": "provenance", "evidence": ["graph:6.3:a_pack_class_is_never_an_anchor", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]},
]
# Section 7.2: what withholds each pack relation the gate does not state, and the child kind of each withheld group.
UNSTATED_BY = {"pv.relation.array_string": "unsupported:array_or_string_topology", "pv.relation.inverter_array": "unsupported:array_or_string_topology", "pv.relation.inverter_phase": "unsupported:per_phase_values"}
WITHHELD_CHILDREN = {"battery": "resource.battery", "meter": "resource.meter"}
UNSTATED_OUTCOME = "unknown_not_absent"
# The family segment is a fixed gateway namespace, not a device observation.
MANAGED_SOURCE_RE = re.compile(
    r"^modbus\.huawei\.(?:smartlogger|sdongle)(?:\.(?P<label>[a-z][a-z0-9]*(?:-[a-z0-9]+)*))?:"
    r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
)

EDITION = {"title": "SUN2000MC V200R023C00 Modbus Interface Definitions", "issue": "Issue 01", "date": "2023-03-17", "use": "identity_only_inspection_only_never_quoted"}
REGISTRY_FLAGS = {"catalog_registered": False, "automatic_runtime_admission": False, "live_qualified": False, "support_claim": False, "write_authority": False}
QUALIFICATION = {**REGISTRY_FLAGS, "mapping_qualified": True, "outbound_allowed": False}
EVIDENCE = {
    "monitoring_contract": {"repository": "helianthus-docs-modbus", "issue": 242, "pull_request": 253, "pages": ["protocols/huawei/monitoring-readonly-v1.md", "protocols/huawei/read-pacing-v1.md"]},
    "registry_profiles": {"repository": "helianthus-modbusreg", "issue": 348, "pull_request": 357},
    "program_issues": {"repository": "helianthus-modbusreg", "issues": [199, 201]},
    "multi_endpoint_contract": {"repository": "helianthus-docs-ebus", "issue": 746, "pull_request": 748},
    "grades": ["proven", "hypothesis", "unknown"],
}
_COMMON = [("status_group_words", "retained_native"), ("string_group_tail_words", "retained_native")]
PROFILE_FIELDS = {
    M3: [("pv_string_voltage", "admitted"), ("pv_string_current", "admitted"), ("input_power", "refused_group"), ("active_power", "refused_group"), ("device_status", "refused_group"),
         ("energy_accumulated", "admitted"), ("energy_current_hour", "admitted"), ("energy_current_day", "admitted")] + _COMMON
        + [("dc_input_energy_counter", "retained_native"), ("statistics_time_words", "retained_native"), ("battery", "withheld_group"), ("meter", "withheld_group")],
    M1: [("pv_string_voltage", "admitted"), ("pv_string_current", "admitted"), ("input_power", "admitted"), ("active_power", "admitted"), ("device_status", "admitted"),
         ("energy_accumulated", "admitted"), ("energy_current_hour", "admitted"), ("energy_current_day", "admitted")] + _COMMON
        + [("power_group_other_words", "retained_native"), ("dc_input_energy_counter", "retained_native"), ("statistics_time_words", "retained_native"), ("battery", "withheld_group"), ("meter", "withheld_group")],
}
FACT_COUNT = {M3: 19, M1: 22}
READ_PLAN = {
    M3: [(32000, 16, "retained_no_facts"), (32016, 24, "consumed"), (32106, 10, "consumed")],
    M1: [(32000, 16, "retained_no_facts"), (32016, 24, "consumed"), (32064, 32, "consumed"), (32106, 10, "consumed")],
}
REFUSED = {M3: [(32064, 32, 3), (37100, 38, 3)], M1: []}
RUNTIME_REASONS = {"pv.reason.native_fact_unknown", "pv.reason.value_outside_pack_range", "pv.reason.input_presence_unknown", "pv.reason.state_code_not_observed", "pv.reason.power_group_refused"}
UNSUPPORTED = ["per_phase_values", "array_or_string_topology", "tracker_pairing", "plant_or_site_aggregate", "planned_plant_groups", "derived_energy", "active_power_limit", "export_power_limit", "enable_disable", "write_operations", "acknowledgement", "readback", "retry", "control_routes"]
REQUESTED = ["pv.ac.aggregate_active_power", "pv.dc.aggregate_power", "pv.dc.current", "pv.dc.voltage", "pv.energy.generated", "pv.energy.yield_day", "pv.status.operating"]
DOC_TOKENS = ("gateway-allocated", "operator label", "class token", "unknown, not absent", "section 15", "en-1", "per-asset configuration field is not required", "no binary floating point", "no derived energy", "unknown is never zero", "outbound_allowed=false", "one atomic cutover", "proven only where the monitoring contract is proven")


def fail(code):
    raise ValueError(code)


class Catalog:
    """Element-catalog lookups for this gate: kinds, relations, facts, and the PV pack's dimensions and relations."""

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

    def subject(self, fact_id):
        attributes = self.index.facts.get(fact_id) if isinstance(fact_id, str) else None
        return attributes["subject"] if attributes else {}

    def parent(self, kind):
        """RES-2: a component's parent is the whole of the pack's contains definition whose part is its kind."""
        for name in self.pack_relations:
            definition = self.index.relations[name]
            whole, part = (definition["from_kinds"], definition["to_kinds"]) if definition["orientation"] == "as_core" else (definition["to_kinds"], definition["from_kinds"])
            if definition["core_type"] == "contains" and part == [kind]:
                return whole[0]
        return None

    def role_kinds(self):
        """RES-1 and section 6.1: the kind of each resource role, through its pack dimension."""
        return {role: spec.get("kind") or self.dimensions.get(spec["dimension"]) for role, spec in RESOURCE_ROLES.items()}


@lru_cache(maxsize=1)
def element_catalog():
    return Catalog(graph_rules.load_catalog())


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
    return {field["id"]: field for field in catalog["fields"]}, {rel["id"]: rel for rel in catalog["relations"]}, {dim["id"] for dim in catalog["fact_dimensions"]}


def decimal_of(number):
    with localcontext() as context:
        context.prec = 60
        return Decimal(int(number["coefficient"])).scaleb(number["exponent10"])


def decode(words, native_type):
    count, signed = WORD_TYPES[native_type]
    if not isinstance(words, list) or len(words) != count or any(type(word) is not int or not 0 <= word <= 0xFFFF for word in words):
        fail("native_evidence_invalid")
    raw = 0
    for word in words:
        raw = (raw << 16) | word
    if signed and raw >= 1 << (16 * count - 1):
        raw -= 1 << (16 * count)
    return raw


def canonical(coefficient, exponent):
    if coefficient == 0:
        return {"coefficient": "0", "exponent10": 0}
    while coefficient % 10 == 0:
        coefficient //= 10
        exponent += 1
    if not -18 <= exponent <= 18:
        fail("native_evidence_invalid")
    return {"coefficient": str(coefficient), "exponent10": exponent}


def convert(coefficient, exponent, conversion):
    """Exact native x factor + offset in integers; the native value and unit stay in the evidence."""
    factor, offset = conversion["factor"], conversion["offset"]
    product, product_exponent = coefficient * int(factor["coefficient"]), exponent + factor["exponent10"]
    shared = min(product_exponent, offset["exponent10"])
    return canonical(product * 10 ** (product_exponent - shared) + int(offset["coefficient"]) * 10 ** (offset["exponent10"] - shared), shared)


def check_conversion(rule, spec, losses):
    """A quantity rule states the exact conversion of its unit pair; one that is not the identity declares a unit loss."""
    expected = UNIT_CONVERSION.get((spec.get("unit"), rule.get("unit")))
    if expected is None or rule.get("conversion") != expected or not set(rule) <= QUANTITY_RULE_MEMBERS:
        fail("unit conversion")
    if (expected != IDENTITY) != ("unit" in [loss["kind"] for loss in losses]):
        fail("silent unit conversion")


def plain(number):
    """A kernel Decimal written without an exponent, for the page."""
    digits, exponent = number["coefficient"].lstrip("-"), number["exponent10"]
    sign = "-" if number["coefficient"].startswith("-") else ""
    if exponent >= 0:
        return sign + digits + "0" * exponent
    digits = digits.rjust(1 - exponent, "0")
    return sign + digits[:exponent] + "." + digits[exponent:]


def grade_min(left, right):
    return GRADES[min(GRADES.index(left), GRADES.index(right))]


def check_shared(document, catalog):
    """Section 15: the shared graph rules first; a gate of this family states its graph."""
    failures = graph_rules.check(document, catalog.document)
    if failures:
        fail("graph {} {}".format(*failures[0]))
    if not graph_rules.graph_stated(document):
        fail("graph_not_stated")


def check_subject(rule, catalog):
    """EN-1: a mapped rule names its target's catalog subject kind and dimension and the gate's measurement point."""
    subject = catalog.subject(rule.get("target"))
    if rule.get("subject_kind") != subject.get("kind") or rule.get("dimension") != subject.get("dimension", {}).get("id") or rule.get("measurement_point") != MEASUREMENT_POINTS.get(rule["native_id"]):
        fail("field rule subject")


def validate_identity(document):
    """Section 6.3: kernel lineage plus one gateway-allocated ResourceID per role; no derivation, class token or dimension text."""
    if document.get("identity") != IDENTITY_CONTRACT:
        fail("identity contract")


def expected_basis(resource, anchor_class):
    """Section 6.3: a listed scheme anchors on a native token, a component on its native position, anything else on an operator label."""
    if resource["identification_scheme"] != graph_rules.NONE_LISTED:
        return "native_identity"
    return "native_slot" if anchor_class == "component" else "operator_slot"


def check_graph_resources(graph, catalog):
    """Section 15 item 1, RES-1 and RES-2: one resource per role, in role order, with its anchor, parent and token-stability statement."""
    kinds = catalog.role_kinds()
    if [resource["kind"] for resource in graph["resources"]] != list(kinds.values()):
        fail("graph_resources")
    for resource in graph["resources"]:
        anchor_class = catalog.index.kinds[resource["kind"]]["anchor_class"]
        if resource["anchor_basis"] != expected_basis(resource, anchor_class):
            fail("graph_anchor")
        if resource["parent_kind"] != (catalog.parent(resource["kind"]) if anchor_class == "component" else None):
            fail("graph_parent")
        stated = {item.split(":")[0] for item in resource["evidence"]}
        if "native_owner_token_stability" not in stated or (resource["kind"] == kinds["inverter"] and "future_native_token" not in stated):
            fail("graph_evidence")


def check_graph_paths(graph, catalog):
    """Section 15 item 3 and section 6.4: the gateway path relays the inverter; seen directly too, it joins on a shared key only."""
    kinds = catalog.role_kinds()
    inverter = next(resource for resource in graph["resources"] if resource["kind"] == kinds["inverter"])
    relayed = {"relaying_kind": kinds["logger"], "relayed_kind": kinds["inverter"], "direct_anchor_key_source": inverter["anchor_source"]}
    if graph["relayed"] != [relayed]:
        fail("graph_relayed")
    native = inverter["anchor_basis"] == "native_identity"
    join = {"kind": kinds["inverter"], "observations": TWO_PATHS, "basis": "same_anchor_key" if native else "operator_declaration", "native_tokens": "equal" if native else "not_exposed"}
    if graph["joins"] != [join]:
        fail("graph_joins")


def check_graph_relations(graph, document, catalog):
    """Section 15 item 2, REL-3 and section 7.2: stated basis and rule scope, the admitted profiles' scope, no absence statement."""
    profiles = document["native_contract"]["profiles"]
    scope = {"profile": "_and_".join(profile["key"] for profile in profiles), "version": "+".join(sorted({profile["profile_id"].rsplit("@", 1)[-1] for profile in profiles}))}
    kinds = catalog.role_kinds()
    if [(relation["definition"], relation["basis_class"], relation["validity_scope"]["rule_scope"]) for relation in graph["relations"]] != STATED_RELATIONS:
        fail("graph_relations")
    for relation in graph["relations"]:
        definition = catalog.index.relations[relation["definition"]]
        ends = (kinds["logger"], kinds["inverter"]) if definition["core_type"] == "relays" else (definition["from_kinds"][0], definition["to_kinds"][0])
        stated_scope = {key: relation["validity_scope"][key] for key in scope}
        if (relation["from_kind"], relation["to_kind"]) != ends or relation["absence_sides"] or stated_scope != scope:
            fail("graph_relations")


def expected_aggregates(document, catalog):
    """Section 15 item 4, EN-1 and EN-5: each mapped aggregate with its catalog subject kind and the coverage the gate can state."""
    aggregates = []
    for rule in document["field_rules"]:
        fact = catalog.index.facts.get(rule.get("target"))
        if fact is not None and fact.get("aggregation") == "aggregate":
            aggregates.append({"fact": rule["target"], "subject_kind": fact["subject"]["kind"], "coverage": COVERAGE.get(rule["target"], [])})
    return aggregates


def expected_unstated(document, catalog):
    """Section 7.2: a pack relation the gate does not state, and the relay of each withheld child group, stay unknown, never absent."""
    stated = {relation["definition"] for relation in document["graph"]["relations"]}
    rows = []
    for name in catalog.pack_relations:
        if name not in stated:
            definition = catalog.index.relations[name]
            rows.append({"definition": name, "from_kind": definition["from_kinds"][0], "to_kind": definition["to_kinds"][0], "outcome": UNSTATED_OUTCOME, "withheld_by": UNSTATED_BY.get(name)})
    inverter = catalog.role_kinds()["inverter"]
    for rule in document["field_rules"]:
        if rule.get("native_id") in WITHHELD_CHILDREN:
            rows.append({"definition": "relation.relays", "from_kind": inverter, "to_kind": WITHHELD_CHILDREN[rule["native_id"]], "outcome": UNSTATED_OUTCOME, "withheld_by": rule["disposition"].get("reason")})
    return rows


def validate_graph(document, catalog):
    """Section 15: this gate's own rules after the shared checker; no member decides a plane."""
    graph = document["graph"]
    check_graph_resources(graph, catalog)
    check_graph_paths(graph, catalog)
    check_graph_relations(graph, document, catalog)
    if graph["aggregates"] != expected_aggregates(document, catalog):
        fail("graph_aggregates")
    if graph["selectors"] != SELECTORS:
        fail("graph_selectors")
    if document.get("unstated_relations") != expected_unstated(document, catalog):
        fail("unstated_relations")


def validate_contract(document, catalog=None):
    catalog = element_catalog() if catalog is None else Catalog(catalog)
    check_shared(document, catalog)
    fields, _, _ = pack_catalog()
    check_contract_header(document)
    native = document.get("native_contract", {})
    check_native_contract(native)
    profiles = native.get("profiles", [])
    if [profile.get("profile_id") for profile in profiles] != [M3, M1]:
        fail("profiles")
    rules = document.get("field_rules", [])
    rule_ids = [rule.get("native_id") for rule in rules]
    if len(rule_ids) != len(set(rule_ids)):
        fail("duplicate disposition")
    for profile in profiles:
        check_profile(profile, rules, rule_ids)
    if set(rule_ids) != {nid for fields_ in PROFILE_FIELDS.values() for nid, _ in fields_}:
        fail("disposition without native field")
    reasons = document.get("reasons", {})
    check_reasons(reasons)
    used, targets = check_field_rules(rules, fields, catalog, reasons)
    if sorted(targets) != REQUESTED or document.get("projection", {}).get("requested_items") != [{"kind": "fact", "item_id": item} for item in REQUESTED]:
        fail("requested items")
    if set(reasons) != used | RUNTIME_REASONS:
        fail("reason inventory")
    for rule in rules:
        if rule.get("native_id") in ("pv_string_voltage", "pv_string_current") and rule["grade_ceiling"]["m1"] != "proven":
            fail("grade ceiling")
    validate_identity(document)
    validate_graph(document, catalog)
    check_contract_policies(document)


def check_contract_header(document):
    if document.get("contract") != "helianthus.semantic.mapping.huawei-sun2000.pv/v1" or document.get("mapping_id") != "pv.mapping.huawei.sun2000.monitoring" or document.get("pack") != PACK or document.get("status") != "accepted_source_specific_read_only_mapping_gate":
        fail("contract")
    if document.get("evidence") != EVIDENCE:
        fail("evidence")


def check_native_contract(native):
    if native.get("function") != "FC03" or native.get("read_only") is not True or native.get("outbound_allowed") is not False or native.get("word_order") != "big_endian_most_significant_word_first" or native.get("value") != "twos_complement_integer_over_gain" or native.get("registry_flags") != REGISTRY_FLAGS or native.get("selection_outcomes") != ["PROFILE_SELECTED", "INSUFFICIENT_EVIDENCE", "NO_ADMISSIBLE_PROFILE", "AMBIGUOUS"]:
        fail("native contract")
    if native.get("edition") != EDITION:
        fail("edition")
    if native.get("unmapped_families") != [{"family": "SUN2000 MAP0 series", "gateway_path": "EMMA", "outcome": "NO_ADMISSIBLE_PROFILE", "reason": "no_consulted_edition", "fields": []}]:
        fail("unmapped family")


def check_profile(profile, rules, rule_ids):
    pid = profile["profile_id"]
    listed = [(row.get("native_id"), row.get("state")) for row in profile.get("fields", [])]
    if listed != PROFILE_FIELDS[pid]:
        fail("profile field inventory")
    if profile.get("fact_count") != FACT_COUNT[pid] or [(g.get("offset"), g.get("quantity"), g.get("use")) for g in profile.get("read_plan", [])] != READ_PLAN[pid] or [(g.get("offset"), g.get("quantity"), g.get("exception")) for g in profile.get("refused_groups", [])] != REFUSED[pid]:
        fail("profile read plan")
    for nid, state in PROFILE_FIELDS[pid]:
        if nid not in rule_ids:
            fail("missing disposition")
    counts = {rule["native_id"]: (rule.get("native") or {}).get("count") for rule in rules}
    if sum(counts[nid] or 0 for nid, state in PROFILE_FIELDS[pid] if state == "admitted") != FACT_COUNT[pid]:
        fail("profile fact count")


def check_reasons(reasons):
    for name, spec in reasons.items():
        if not name.startswith("pv.reason.") or spec.get("loss_kind") not in LOSS_KINDS or not spec.get("description") or type(spec.get("reversible")) is not bool:
            fail("reason loss")


def check_field_rules(rules, fields, catalog, reasons):
    used = set()
    targets = []
    for rule in rules:
        disposition = rule.get("disposition", {})
        outcome = disposition.get("outcome")
        if outcome not in OUTCOMES or set(rule.get("grade_ceiling", {})) != {"m3", "m1"} or any(value not in GRADES for value in rule["grade_ceiling"].values()):
            fail("disposition")
        if outcome in ("withheld", "native_only"):
            check_withheld_rule(rule, disposition, fields, reasons)
            used.add(disposition["reason"])
            continue
        targets.append(check_mapped_rule(rule, disposition, outcome, fields, catalog))
    return used, targets


def check_withheld_rule(rule, disposition, fields, reasons):
    if disposition.get("reason") not in reasons or "loss" in disposition or rule.get("target"):
        fail("withheld reason")
    for target in rule.get("prospective_targets", []):
        if target not in fields:
            fail("prospective target")
    if rule["native_id"] == "energy_current_hour" and rule.get("prospective_targets") != ["pv.energy.yield_month", "pv.energy.yield_year"]:
        fail("prospective target")


def check_mapped_rule(rule, disposition, outcome, fields, catalog):
    target = rule.get("target")
    pack_field = fields.get(target)
    losses = disposition.get("loss")
    if pack_field is None or rule.get("requested") != {"kind": "fact", "item_id": target} or rule.get("dimension") != pack_field["dimension"] or rule.get("unit") != pack_field.get("unit"):
        fail("target definition")
    check_subject(rule, catalog)
    spec = rule.get("native", {})
    words, signed = WORD_TYPES.get(spec.get("type"), (None, None))
    if spec.get("words") != words or spec.get("gain") not in GAIN_EXPONENT or type(spec.get("count")) is not int or spec.get("group_offset") not in (32016, 32064, 32106):
        fail("native form")
    if outcome == "exact" and losses != []:
        fail("exact loss")
    if outcome == "transformed":
        if not losses or any(loss.get("kind") not in LOSS_KINDS or loss.get("source_items") != [NATIVE_PREFIX + rule["native_id"]] or not loss.get("description") or type(loss.get("reversible")) is not bool for loss in losses):
            fail("transformed loss")
    if target == "pv.status.operating":
        check_status_rule(rule, spec, losses, fields[target])
    else:
        check_conversion(rule, spec, losses)
    if rule["native_id"] == "active_power" and (rule.get("sign") != {"native_reference": "positive_while_producing", "negative_value": "published"} or "provenance" not in [loss["kind"] for loss in losses]):
        fail("sign convention")
    if rule["native_id"] in ("energy_accumulated", "energy_current_day") and "policy" not in [loss["kind"] for loss in losses]:
        fail("counter policy loss")
    return target


def check_status_rule(rule, spec, losses, field):
    if rule.get("symbols") != {"grid_connected": "generating"} or "pv.status.operating." + rule["symbols"]["grid_connected"] not in field["symbols"] or spec.get("unit") != "" or rule.get("unit") is not None or set(rule) != STATUS_RULE_MEMBERS:
        fail("status symbols")
    if "symbol" not in [loss["kind"] for loss in losses]:
        fail("status loss")


def check_contract_policies(document):
    if document.get("lifecycle") != {
        "required_observation_evidence": ["observation_id", "observation_revision", "receipt_wall", "receipt_monotonic", "clock_epoch", "source_epoch", "driver_generation", "connection_generation", "profile_selection", "groups", "facts"],
        "freshness": "native_receipt_axes_preserved", "device_time": "statistics_time_words_never_used",
        "retention": "last_complete_group_reading_keeps_its_original_observation", "retention_fence": "never_across_profile_unit_or_connection_identity",
        "withdrawal": "explicit_generation_fenced", "qualification": "mapping_qualified_offline_profile_and_live_unqualified"}:
        fail("lifecycle contract")
    if document.get("decimal_policy") != {"form": "kernel_decimal_coefficient_exponent10", "exponent10_range": [-18, 18], "zero": "coefficient_0_exponent10_0", "trailing_zero": "moves_to_exponent10", "binary_floating_point": "forbidden", "derived_energy": "forbidden"}:
        fail("decimal policy")
    if document.get("counter_policy") != {"fact": "pv.energy.generated", "source": "energy_accumulated", "decrease": "never_inferred_as_reset_or_wrap", "continuity": "native_evidence_required", "day_yield": {"fact": "pv.energy.yield_day", "source": "energy_current_day", "decrease": "never_inferred_as_reset_or_wrap", "continuity": "native_evidence_required"}, "hour_counter": "not_published", "month_yield": "no_native_word", "year_yield": "no_native_word", "sum_across_inverters": "forbidden"}:
        fail("counter policy")
    if document.get("outcome_vocabulary") != {"exact": "exact", "transformed": "transformed", "withheld": "withheld", "native_only": "unrepresentable"}:
        fail("outcome vocabulary")
    if document.get("unsupported") != UNSUPPORTED:
        fail("unsupported")
    projection = document.get("projection", {})
    if projection.get("error_precedence") != ERROR_PRECEDENCE or projection.get("operations") != "unsupported_no_authority" or projection.get("missing_native") != "unavailable_withheld_never_zero" or projection.get("target") != "SemReg_pv_projection":
        fail("projection")
    if document.get("consumer_cutover") != {"consumers": ["semantic_mcp", "graphql", "portal", "home_assistant"], "path": "shared_semantic_snapshot_only", "until_implemented": "no_public_consumer_binding"}:
        fail("consumer cutover")
    if contains_float(document):
        fail("binary floating point")


def expected_facts(profile, rules):
    expected = []
    for nid, state in PROFILE_FIELDS[profile["profile_id"]]:
        if state == "admitted":
            count = rules[nid]["native"]["count"]
            expected += [(nid, n) for n in (range(1, count + 1) if count > 1 else [0])]
    return expected


def validate_case(case, contract):
    if not isinstance(case, dict):
        fail("native_evidence_invalid")
    profiles = {profile["profile_id"]: profile for profile in contract["native_contract"]["profiles"]}
    selection = case.get("selection")
    unit = selection.get("routed_unit") if isinstance(selection, dict) else None
    if not isinstance(selection, dict) or selection.get("outcome") != "PROFILE_SELECTED" or selection.get("profile_id") not in profiles or type(unit) is not int or not 1 <= unit <= 247:
        fail("profile_not_selected")
    profile = profiles[selection["profile_id"]]
    rules = {rule["native_id"]: rule for rule in contract["field_rules"]}
    life = case.get("lifecycle") if isinstance(case.get("lifecycle"), dict) else {}
    errors = set()
    if contains_float(case):
        errors.add("native_evidence_invalid")
    groups = check_case_groups(case, profile, errors)
    facts, wanted = check_case_facts(case, profile, rules, groups, life, selection, errors)
    check_case_lineage(facts, wanted, rules, errors)
    identity, source = case.get("identity"), case.get("source")
    asset = identity.get("asset_id") if isinstance(identity, dict) else None
    binding = identity.get("binding_id") if isinstance(identity, dict) else None
    source_id = source.get("source_id") if isinstance(source, dict) else None
    values = (asset, binding, source_id)
    resources = case_resources(identity, unit, values)
    if not valid_identity(asset, unit) or not valid_identity(binding, unit) or not valid_identity(source_id, unit, managed_source=True) or len(set(values)) != 3 or resources is None:
        errors.add("identity_missing_or_invalid")
    check_case_origins(case, values, errors)
    check_case_lifecycle(life, errors)
    if case.get("qualification") != QUALIFICATION:
        errors.add("mapping_unqualified")
    if case.get("request_operation") or case.get("derive_energy"):
        errors.add("unsupported_or_withheld")
    for code in ERROR_PRECEDENCE:
        if code in errors:
            fail(code)
    return profile, rules, facts, groups, resources, binding, source_id, life


def check_case_groups(case, profile, errors):
    groups = {}
    plan = {(g["offset"], g["quantity"]) for g in profile["read_plan"]}
    refused = {(g["offset"], g["quantity"]) for g in profile["refused_groups"]}
    for group in case.get("groups") if isinstance(case.get("groups"), list) else [None]:
        key = (group.get("offset"), group.get("quantity")) if isinstance(group, dict) else None
        if key not in plan | refused or key in groups or group.get("outcome") not in ("ANSWERED", "EXCEPTION", "FAILED", "NOT_READ"):
            errors.add("native_evidence_invalid")
            continue
        code = group.get("exception_code")
        if (group["outcome"] == "EXCEPTION") != (type(code) is int and code >= 1) or (key in refused and (group["outcome"] != "EXCEPTION" or code != 3)):
            errors.add("native_evidence_invalid")
        groups[key] = group["outcome"]
    if not plan <= set(groups):
        errors.add("native_evidence_invalid")
    return groups


def check_case_facts(case, profile, rules, groups, life, selection, errors):
    facts = {}
    listed = case.get("facts") if isinstance(case.get("facts"), list) else []
    for fact in listed:
        if not isinstance(fact, dict) or (fact.get("key"), fact.get("index")) in facts:
            errors.add("native_evidence_invalid")
            continue
        facts[(fact.get("key"), fact.get("index"))] = fact
    wanted = expected_facts(profile, rules)
    if sorted(facts, key=str) != sorted(wanted, key=str) or len(listed) != FACT_COUNT[profile["profile_id"]]:
        errors.add("native_evidence_invalid")
    for (nid, index), fact in facts.items():
        if (nid, index) not in wanted:
            continue
        try:
            check_fact(fact, rules[nid], groups, life, selection)
        except ValueError:
            errors.add("native_evidence_invalid")
    return facts, wanted


def check_case_lineage(facts, wanted, rules, errors):
    lineage = {}
    for (nid, index), fact in facts.items():
        if (nid, index) in wanted and fact.get("retained") is True and isinstance(fact.get("origin"), dict):
            offset = rules[nid]["native"]["group_offset"]
            mark = (fact.get("observation_id"), fact.get("observation_revision"), tuple(sorted(fact["origin"].items(), key=str)))
            if lineage.setdefault(offset, mark) != mark:
                errors.add("native_evidence_invalid")


def check_case_origins(case, values, errors):
    for fact in case.get("facts") or []:
        origin = fact.get("retained") and fact.get("origin") if isinstance(fact, dict) else None
        if isinstance(origin, dict) and (origin.get("asset_id"), origin.get("binding_id"), origin.get("source_id")) != values:
            errors.add("native_evidence_invalid")


def check_case_lifecycle(life, errors):
    opaque, numeric = ("observation_id", "clock_epoch", "source_epoch"), ("observation_revision", "driver_generation", "connection_generation")
    if any(not isinstance(life.get(key), str) or not OPAQUE_RE.fullmatch(life[key]) for key in opaque) or any(type(life.get(key)) is not int or life[key] < 1 for key in numeric) or type(life.get("receipt_monotonic")) is not int or life.get("receipt_monotonic", 0) < 1:
        errors.add("lifecycle_missing")
    try:
        if datetime.fromisoformat(life.get("receipt_wall", "").replace("Z", "+00:00")).tzinfo is None:
            raise ValueError
    except (AttributeError, ValueError):
        errors.add("lifecycle_missing")


def valid_identity(item, unit, managed_source=False):
    if not isinstance(item, str) or not OPAQUE_RE.fullmatch(item) or item == str(unit):
        return False
    managed = MANAGED_SOURCE_RE.fullmatch(item) if managed_source else None
    if managed:
        label = managed.group("label") or "default"
        return len(label) <= 32 and not any(token in label for token in ID_FORBIDDEN_TEXT)
    return not any(token in item.lower() for token in ID_FORBIDDEN_TEXT) and not ID_ADDRESS_RE.search(item)


def case_resources(identity, unit, lineage):
    """Section 6.3: one valid, distinct ResourceID per allocated role and none for a role that is not allocated."""
    resources = identity.get("resources") if isinstance(identity, dict) else None
    allocated = [role for role, spec in RESOURCE_ROLES.items() if spec["source"] == ALLOCATED]
    if not isinstance(resources, dict) or set(resources) != set(allocated) or not isinstance(resources["inputs"], list) or len(resources["inputs"]) != RESOURCE_ROLES["inputs"]["slots"]:
        return None
    values = [resources[role] for role in allocated if role != "inputs"] + resources["inputs"]
    if not all(valid_identity(item, unit) for item in values) or len(set(values)) != len(values) or set(values) & {item for item in lineage if isinstance(item, str)}:
        return None
    return resources


def check_origin(fact, life, selection):
    """A retained reading carries the receipt coordinates and lineage of the observation that produced it."""
    origin = fact.get("origin")
    keys = {"receipt_wall", "receipt_monotonic", "clock_epoch", "source_epoch", "driver_generation", "connection_generation", "profile_id", "routed_unit", "asset_id", "binding_id", "source_id"}
    if not isinstance(origin, dict) or set(origin) != keys:
        fail("retained origin")
    try:
        if datetime.fromisoformat(origin["receipt_wall"].replace("Z", "+00:00")).tzinfo is None:
            raise ValueError
    except (AttributeError, ValueError):
        fail("retained origin")
    if type(origin["receipt_monotonic"]) is not int or origin["receipt_monotonic"] < 1 or not isinstance(origin["clock_epoch"], str) or not OPAQUE_RE.fullmatch(origin["clock_epoch"]):
        fail("retained origin")
    if origin["clock_epoch"] == life.get("clock_epoch") and origin["receipt_monotonic"] >= life.get("receipt_monotonic", 0):
        fail("retained origin")
    if origin["source_epoch"] != life.get("source_epoch") or origin["driver_generation"] != life.get("driver_generation"):
        fail("retained reading crosses a source epoch or driver generation")
    if origin["connection_generation"] != life.get("connection_generation") or origin["profile_id"] != selection.get("profile_id") or origin["routed_unit"] != selection.get("routed_unit"):
        fail("retained reading crosses a connection, profile or unit identity")


def check_fact(fact, rule, groups, life, selection):
    spec = rule["native"]
    required = {"key", "index", "unit", "availability", "grade", "retained", "observation_id", "observation_revision"}
    if not required <= set(fact) or fact["unit"] != spec["unit"] or type(fact["retained"]) is not bool or fact["grade"] not in GRADES or type(fact["observation_revision"]) is not int or not isinstance(fact["observation_id"], str):
        fail("fact shape")
    group = groups.get(next((g for g in groups if g[0] == spec["group_offset"]), None))
    if fact["availability"] == "UNKNOWN":
        if set(fact) & {"value", "words", "state", "origin"} or fact["grade"] != "unknown" or fact["retained"]:
            fail("unknown fact carries a value")
        return
    if fact["availability"] != "VALUE" or fact["grade"] == "unknown":
        fail("fact availability")
    if fact["retained"]:
        check_retained_fact(fact, group, life, selection)
    elif group != "ANSWERED" or fact["observation_id"] != life.get("observation_id") or fact["observation_revision"] != life.get("observation_revision"):
        fail("fact without an answered group")
    if rule["native_id"] == "device_status":
        if fact.get("state") not in rule.get("symbols", {}) or set(fact) & {"value", "words"}:
            fail("status state")
        return
    check_fact_value(fact, spec)


def check_retained_fact(fact, group, life, selection):
    if not fact["observation_id"] or type(fact["observation_revision"]) is not int or fact["observation_revision"] < 1:
        fail("retained fact lineage")
    if group == "ANSWERED" or fact["observation_revision"] >= life.get("observation_revision", 0) or fact["observation_id"] == life.get("observation_id"):
        fail("retained fact")
    check_origin(fact, life, selection)


def check_fact_value(fact, spec):
    value = fact.get("value")
    if not isinstance(value, dict) or not isinstance(value.get("coefficient"), str) or not COEFFICIENT_RE.fullmatch(value["coefficient"]) or value.get("exponent10") != GAIN_EXPONENT[spec["gain"]]:
        fail("fact value")
    if decode(fact.get("words"), spec["type"]) != int(value["coefficient"]):
        fail("words do not equal the value")


def project(case, contract):
    profile, rules, facts, groups, resources, binding, source_id, life = validate_case(case, contract)
    catalog, _, _ = pack_catalog()
    key_of = profile["key"]
    roles = {spec.get("dimension"): role for role, spec in RESOURCE_ROLES.items()}
    published, native_rows, sources, item_reasons = [], [], {}, {}
    for nid, state in PROFILE_FIELDS[profile["profile_id"]]:
        rule = rules[nid]
        if state == "refused_group":
            hold(native_rows, item_reasons, rule, None, "pv.reason.power_group_refused")
            continue
        if state != "admitted" or not rule.get("target"):
            native_rows.append({"native_id": nid, "outcome": rule["disposition"]["outcome"], "reason": rule["disposition"]["reason"]})
            continue
        count = rule["native"]["count"]
        for n in range(1, count + 1) if count > 1 else [0]:
            index = n if count > 1 else None
            fact = facts[(nid, n)]
            reason, value = fact_outcome(nid, rule, n, fact, facts, groups, catalog)
            if reason:
                hold(native_rows, item_reasons, rule, index, reason)
                continue
            project_fact(rule, nid, n, index, fact, value, resources, roles, key_of, life, binding, source_id, published, sources, native_rows)
    relations, records = graph_records(published, resources, contract)
    published.sort(key=lambda item: (item["key"]["fact_id"], item["key"]["dimensions"][0]["value"]["text"]))
    dispositions = project_dispositions(contract, sources, item_reasons)
    output = {"facts": published, "resources": records, "relations": relations, "requested": copy.deepcopy(contract["projection"]["requested_items"]), "dispositions": dispositions, "native_dispositions": native_rows, "operations": []}
    assert_complete(output, profile, rules)
    assert_graph(output)
    return output


def hold(native_rows, item_reasons, rule, index, reason):
    row = {"native_id": rule["native_id"]}
    if index is not None:
        row["index"] = index
    native_rows.append({**row, "outcome": "withheld", "reason": reason})
    if rule.get("target"):
        item_reasons.setdefault(rule["target"], []).append(reason)


def fact_outcome(nid, rule, n, fact, facts, groups, catalog):
    """Return (reason, value) for one native fact: a withheld reason, or the published value."""
    if fact["availability"] != "VALUE":
        answered = groups.get((32064, 32)) == "ANSWERED"
        return ("pv.reason.state_code_not_observed" if nid == "device_status" and answered else "pv.reason.native_fact_unknown"), None
    if nid == "device_status":
        return None, {"kind": "symbol", "symbol": {"namespace": rule["target"], "token": rule["symbols"][fact["state"]], "known": True}}
    raw = int(fact["value"]["coefficient"])
    if nid in ("pv_string_voltage", "pv_string_current"):
        pair = [facts[("pv_string_voltage", n)], facts[("pv_string_current", n)]]
        if not any(int(item["value"]["coefficient"]) for item in pair if item["availability"] == "VALUE"):
            return "pv.reason.input_presence_unknown", None
    number = convert(raw, fact["value"]["exponent10"], rule["conversion"])
    bounds = catalog[rule["target"]]["bounds"]
    if decimal_of(bounds["minimum"]) <= decimal_of(number) <= decimal_of(bounds["maximum"]):
        return None, {"kind": "quantity", "quantity": {"number": number, "unit": rule["unit"]}}
    return "pv.reason.value_outside_pack_range", None


def project_fact(rule, nid, n, index, fact, value, resources, roles, key_of, life, binding, source_id, published, sources, native_rows):
    subject = resources[roles[rule["dimension"]]]
    text = subject[n - 1] if isinstance(subject, list) else subject
    key = {"pack_id": PACK["id"], "pack_version": PACK["version"], "fact_id": rule["target"], "dimensions": [{"id": rule["dimension"], "value": {"kind": "text", "text": text}}]}
    origin = fact["origin"] if fact["retained"] else life
    published.append({"key": key, "value": value, "receipt_wall": origin["receipt_wall"], "receipt_monotonic": origin["receipt_monotonic"], "clock_epoch": origin["clock_epoch"],
                      "connection_generation": origin.get("connection_generation"), "source_path": {"binding_id": binding, "source_id": source_id, "source_epoch_id": origin["source_epoch"], "driver_generation": origin["driver_generation"]},
                      "observation_id": fact["observation_id"], "observation_revision": fact["observation_revision"],
                      "evidence_grade": grade_min(fact["grade"], rule["grade_ceiling"][key_of]), "retained": fact["retained"]})
    sources.setdefault(rule["target"], []).append(key)
    row = {"native_id": nid}
    if index is not None:
        row["index"] = index
    native_rows.append({**row, "outcome": rule["disposition"]["outcome"], "target": rule["target"]})


def project_dispositions(contract, sources, item_reasons):
    dispositions = []
    for requested in contract["projection"]["requested_items"]:
        item_id = requested["item_id"]
        rule = next(r for r in contract["field_rules"] if r.get("target") == item_id)
        keys = sorted(sources.get(item_id, []), key=lambda k: k["dimensions"][0]["value"]["text"])
        if keys:
            dispositions.append({"kind": "fact", "item_id": item_id, "outcome": rule["disposition"]["outcome"], "source_keys": keys, "loss": copy.deepcopy(rule["disposition"]["loss"])})
            continue
        reason = item_reasons[item_id][0]
        spec = contract["reasons"][reason]
        dispositions.append({"kind": "fact", "item_id": item_id, "outcome": "withheld", "source_keys": [], "reason": reason,
                             "loss": [{"kind": spec["loss_kind"], "source_items": [NATIVE_PREFIX + rule["native_id"]], "description": spec["description"], "reversible": spec["reversible"]}]})
    return dispositions


def graph_records(published, resources, contract):
    """REL-8 and RES-5: relation endpoints and fact subjects are the ResourceIDs of published resources, never dimension text."""
    basis = {relation["definition"]: relation["basis_class"] for relation in contract["graph"]["relations"]}
    subjects = {item["key"]["dimensions"][0]["value"]["text"] for item in published}
    inverter, system = resources["inverter"], resources["system"]
    pairs = [("pv.relation.system_inverter", system, inverter)] if subjects & {inverter, system} else []
    pairs += [("pv.relation.inverter_input", inverter, item) for item in resources["inputs"] if item in subjects]
    if inverter in subjects or pairs:
        pairs.append(("relation.relays", resources["logger"], inverter))
    relations = [{"definition": name, "from": start, "to": end, "basis_class": basis[name]} for name, start, end in sorted(pairs)]
    kinds = element_catalog().role_kinds()
    kind_of = {resources[role]: kinds[role] for role in ("logger", "system", "inverter")} | {item: kinds["inputs"] for item in resources["inputs"]}
    named = subjects | {end for pair in pairs for end in pair[1:]}
    records = []
    for item in sorted(named):
        records.append({"resource_id": item, "kind": kind_of[item], "parent": inverter} if kind_of[item] == kinds["inputs"] else {"resource_id": item, "kind": kind_of[item]})
    return relations, records


def assert_graph(output):
    """REL-8 and RES-5: every relation endpoint, fact subject and parent names a resource the output publishes."""
    published = {item["resource_id"] for item in output["resources"]}
    named = {record[side] for record in output["relations"] for side in ("from", "to")}
    named |= {fact["key"]["dimensions"][0]["value"]["text"] for fact in output["facts"]}
    named |= {item["parent"] for item in output["resources"] if "parent" in item}
    if not named <= published:
        fail("dangling endpoint")


def assert_complete(output, profile, rules):
    """Every native field of the profile, per string index, carries exactly one disposition."""
    expected = []
    for nid, _ in PROFILE_FIELDS[profile["profile_id"]]:
        count = rules[nid]["native"]["count"] if rules[nid].get("native") else 1
        expected += [(nid, n) for n in range(1, count + 1)] if count > 1 else [(nid, None)]
    seen = [(row["native_id"], row.get("index")) for row in output["native_dispositions"]]
    if seen != expected or any(row["outcome"] not in OUTCOMES for row in output["native_dispositions"]):
        fail("native disposition completeness")
    for row in output["native_dispositions"]:
        if row["outcome"] in ("withheld", "native_only") and ("reason" not in row or "target" in row):
            fail("withheld without reason")


def summarize(output):
    facts = [{"fact_id": item["key"]["fact_id"], "dimension": item["key"]["dimensions"][0]["value"]["text"], "value": item["value"], "evidence_grade": item["evidence_grade"], "retained": item["retained"], "observation_revision": item["observation_revision"]} for item in output["facts"]]
    relations = [{"definition": item["definition"], "from": item["from"], "to": item["to"]} for item in output["relations"]]
    dispositions = []
    for item in output["dispositions"]:
        row = {"item_id": item["item_id"], "outcome": item["outcome"], "published": len(item["source_keys"]), "loss": [loss["kind"] for loss in item["loss"]]}
        if "reason" in item:
            row["reason"] = item["reason"]
        dispositions.append(row)
    return {"facts": facts, "relations": relations, "dispositions": dispositions, "native_dispositions": output["native_dispositions"], "operations": output["operations"]}


def run_scenario(scenario, contract):
    case = copy.deepcopy(contract["baselines"][scenario["baseline"]])
    for mutation in scenario.get("mutations", []):
        mutate(case, mutation)
    return project(case, contract)


def validate_scenarios(contract):
    scenarios = contract.get("scenarios", [])
    if set(contract.get("baselines", {})) != {"m1", "m3"} or [s.get("baseline") for s in scenarios[:2]] != ["m1", "m3"] or any(s.get("mutations") for s in scenarios[:2]):
        fail("scenario baselines")
    ids = [s.get("id") for s in scenarios]
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
        seen_reasons |= {row["reason"] for row in output["native_dispositions"] if "reason" in row}
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


# The staged ND02 relations page the PV gate's relation statements cite; its absence is never a pass.
NATIVE_PAGE = ROOT / ".tmp/references/docs-modbus/protocols/huawei/sun2000-semreg-relations-v1.md"


def check_native_page(path=NATIVE_PAGE):
    if not path.is_file():
        fail("native_page_missing")


def document(contract):
    check_native_page()
    validate_contract(contract)
    validate_scenarios(contract)


def table(text, heading):
    """Rows of the first Markdown table under `heading`, without the header and separator, cells stripped of backticks."""
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


def native_form(spec):
    return "%s, %s, gain %d" % (spec["type"], spec["unit"], spec["gain"]) if spec["unit"] else "%s, code" % spec["type"]


def expected_tables(contract):
    mapped, held = [], []
    for rule in contract["field_rules"]:
        outcome = rule["disposition"]["outcome"]
        if outcome in ("exact", "transformed"):
            kinds = "+".join(loss["kind"] for loss in rule["disposition"]["loss"]) or "none"
            ceiling = rule["grade_ceiling"]
            conversion = "%s, factor %s, offset %s" % (rule["unit"], plain(rule["conversion"]["factor"]), plain(rule["conversion"]["offset"])) if rule.get("unit") else "none"
            mapped.append([rule["native_id"], native_form(rule["native"]), rule["target"], conversion, rule["dimension"], outcome, kinds, ceiling["m3"].capitalize(), ceiling["m1"].capitalize()])
        else:
            reason = rule["disposition"]["reason"]
            held.append([rule["native_id"], outcome, reason, contract["reasons"][reason]["loss_kind"], contract["outcome_vocabulary"][outcome]])
    return mapped, held


def graph_tables(contract):
    """The page's resource, relation, unstated-relation and subject tables, as the gate states them."""
    graph = contract["graph"]
    return {
        "## Resources": [[r["kind"], r["anchor_class"], r["anchor_basis"], r["anchor_source"], r["parent_kind"] or "none"] for r in graph["resources"]],
        "## Relations": [[r["definition"], r["basis_class"], r["from_kind"], r["to_kind"], r["validity_scope"]["rule_scope"]] for r in graph["relations"]],
        "## Unknown, not absent": [[r["definition"], r["from_kind"], r["to_kind"], r["withheld_by"]] for r in contract["unstated_relations"]],
        "## Subjects and measurement points": [[r["native_id"], r["target"], r["subject_kind"], r["measurement_point"]] for r in contract["field_rules"] if r.get("target")],
    }


def check_document(contract, text):
    mapped, held = expected_tables(contract)
    if table(text, "## Field mapping") != mapped or table(text, "## Withheld and native-only") != held:
        fail("document tables")
    if any(table(text, heading) != rows for heading, rows in graph_tables(contract).items()):
        fail("document graph tables")
    if [row[0] for row in table(text, "## Acceptance vectors")] != [scenario["id"] for scenario in contract["scenarios"]]:
        fail("document vectors")
    flat = " ".join(text.split()).lower()
    for token in DOC_TOKENS:
        if token not in flat:
            fail("document boundary")
    if HEX_RUN_RE.search(unowned_text(text)) or HEX_RUN_RE.search(json.dumps(contract)):
        fail("publication: digest or revision literal")
    for profile in contract["native_contract"]["profiles"]:
        if "`%s`" % profile["profile_id"] not in text:
            fail("document profile")


def main():
    contract = load()
    document(contract)
    check_document(contract, DOC.read_text(encoding="utf-8"))
    positives = sum(scenario["polarity"] == "positive" for scenario in contract["scenarios"])
    print(f"Huawei SUN2000 PV mapping: PASS; {positives} executable outputs and {len(contract['scenarios']) - positives} rejected scenarios")


if __name__ == "__main__":
    main()
