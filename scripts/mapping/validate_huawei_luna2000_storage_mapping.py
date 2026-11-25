#!/usr/bin/env python3
"""Execute the Huawei LUNA2000 storage gate: what helianthus.pack.storage@1.1.0 takes once a battery profile is admitted, and nothing before."""
from __future__ import annotations

import copy
import importlib.util
import json
import re
from datetime import datetime
from decimal import Decimal
from functools import lru_cache
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
GATES = ROOT / "driverdata/families/mapping-gates"
SEMANTIC = ROOT / ".tmp/references/docs-semantic"
PATH = GATES / "huawei-luna2000-storage-v1.json"
DOC = GATES / "huawei-luna2000-storage-v1.md"
PACK_TABLES = SEMANTIC / "api/v1/packs/storage-bms-contract-tables.json"
_GRAPH_SPEC = importlib.util.spec_from_file_location("mapping_gate_graph_rules", Path(__file__).resolve().parent / "mapping_gate_graph_rules.py")
graph_rules = importlib.util.module_from_spec(_GRAPH_SPEC)
_GRAPH_SPEC.loader.exec_module(graph_rules)

PACK = {"id": "helianthus.pack.storage", "version": "1.1.0"}
NATIVE_PREFIX = "native.huawei.luna2000."
LOSS_KINDS = {"unit", "range", "precision", "time", "symbol", "provenance", "identity", "capability", "operation", "policy"}
GRADES = ["unknown", "hypothesis", "proven"]
ERROR_PRECEDENCE = ["native_evidence_invalid", "identity_missing_or_invalid", "lifecycle_missing", "mapping_unqualified", "unsupported_or_withheld"]
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
    ("%", "unit.ratio"): {"factor": {"coefficient": "1", "exponent10": -2}, "offset": ZERO},
    ("kW", "unit.watt"): {"factor": {"coefficient": "1", "exponent10": 3}, "offset": ZERO},
    ("kWh", "unit.joule"): {"factor": {"coefficient": "36", "exponent10": 5}, "offset": ZERO},
}
# Members of a prospective row; no other member, and so no second statement of a conversion, is accepted.
QUANTITY_ROW_MEMBERS = {"in_force", "target", "dimension", "subject_kind", "measurement_point", "unit", "native", "conversion", "loss", "sign", "counter"}
SYMBOL_ROW_MEMBERS = {"in_force", "target", "dimension", "subject_kind", "measurement_point", "unit", "native", "symbols"}
# EN-1: the measurement point of each prospective row. Subject kind and dimension come from the element catalog.
MEASUREMENT_POINTS = {
    "battery_group_37000_30": "pack_as_a_whole", "site_battery_soc": "pack_as_a_whole",
    "site_battery_power": "pack_terminals", "plant_battery_power": "pack_terminals",
    "site_battery_energy_charged_total": "pack_terminals_lifetime_counter", "site_battery_energy_discharged_total": "pack_terminals_lifetime_counter",
    "site_rated_storage_capacity": "pack_as_a_whole",
}
FORBIDDEN_DERIVATIONS = ["serial_number", "esn", "vendor", "model", "firmware_or_version", "routed_unit_alone", "endpoint_address"]
# Section 6.3: a pack class token and a profile identifier are interpretation selectors, so neither derives the home asset.
ASSET_FORBIDDEN = FORBIDDEN_DERIVATIONS + ["class_token", "profile_id"]
ALLOCATED = "gateway_allocated_resource_id"
# One gateway-allocated ResourceID per resource role (section 6.3); a role's kind is its dimension's kind in the element catalog.
RESOURCE_ROLES = {
    "inverter": {"kind": "resource.inverter", "source": ALLOCATED, "required": True},
    "battery": {"dimension": "storage.dimension.battery", "source": ALLOCATED, "required": True},
    "packs": {"dimension": "storage.dimension.pack", "source": ALLOCATED, "required": False, "slots": 3},
    "system": {"dimension": "storage.dimension.system", "source": ALLOCATED, "required": True},
    "interfaces": {"dimension": "storage.dimension.interface", "source": ALLOCATED, "required": False, "slots": 3},
}
# A pack is anchored by its native position under the configured battery, never by an address or a label (rule 20).
NATIVE_POSITION_ROLES = {"packs"}
IDENTITY_CONTRACT = {
    "asset_id": {"source": "gateway_allocated_asset_id", "required": True, "non_secret": True, "granularity": "the_home_asset_of_the_inverter_child_that_relays_the_battery", "forbidden_derivations": ASSET_FORBIDDEN},
    "source_id": {"source": "gateway_managed_instance_source_id", "required": True, "non_secret": True, "granularity": "one_source_per_endpoint_instance", "distinct_from": "asset_id", "forbidden_derivations": FORBIDDEN_DERIVATIONS},
    "binding_id": {"source": "gateway_domain_separated_asset_source_profile_binding", "required": True, "non_secret": True, "stable_inputs": ["asset_id", "source_id", "profile_id"]},
    "resources": RESOURCE_ROLES,
    "failure": "missing_invalid_reused_or_ambiguous_identity_fails_closed",
}
# Section 15 item 1: the statement each resource role's evidence must carry, besides the native owner's token stability.
ROLE_STATEMENTS = {"inverter": "future_native_token", "battery": "relaying_path", "packs": "native_contract", "system": "configuration", "interfaces": "native_contract"}
# Section 15 item 2: (definition, basis class, scope of the rule) in the order the gate states them. The battery's
# attachment is configuration: no native owner documents it as observable. A pack relation's endpoint kinds come from
# the element catalog; those of relays from the resource roles.
STATED_RELATIONS = [
    ("relation.relays", "configuration", "each_battery_the_operator_attaches_to_an_inverter_child"),
    ("storage.relation.battery_pack", "gate_rule", "one_record_per_pack_at_its_native_position"),
    ("storage.relation.pack_interface", "gate_rule", "one_interface_per_pack_at_the_native_position_of_its_pack"),
    ("storage.relation.system_battery", "configuration", "the_storage_system_the_operator_declares_for_each_configured_battery"),
]
SCOPE_VERSION = "monitoring_contract_v1"
# Section 15 item 3 and section 6.4: the relaying path exposes no battery token and no direct path is documented,
# so a second view of the battery joins only by an operator declaration.
DIRECT_BATTERY_KEY = "none"
TWO_VIEWS = ["relayed_by_the_inverter", "seen_through_another_path"]
SELECTORS = [
    {"selector": "profile_id", "classification": "provenance", "evidence": ["admission:registry_disposition_profile", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]},
    {"selector": "class_token", "classification": "provenance", "evidence": ["graph:6.3:a_pack_class_is_never_an_anchor", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]},
]
# Section 7.2: what withholds each storage relation the gate does not state.
UNSTATED_BY = {
    "storage.relation.module_cell": "native_contract:no_module_or_cell_structure_stated_for_a_pack",
    "storage.relation.pack_module": "native_contract:no_module_or_cell_structure_stated_for_a_pack",
    "storage.relation.pack_string": "native_contract:no_string_structure_stated_for_a_pack",
}
UNSTATED_OUTCOME = "unknown_not_absent"
REGISTRY_FLAGS = {"catalog_registered": False, "automatic_runtime_admission": False, "live_qualified": False, "support_claim": False}
QUALIFICATION = {**REGISTRY_FLAGS, "mapping_qualified": True, "outbound_allowed": False}

def qualification_recorded(qualification):
    """ND02 decision D14: live qualification is a recorded level, never a rejection; the other flags and the mapping stay pinned."""
    if not isinstance(qualification, dict) or not isinstance(qualification.get("live_qualified"), bool):
        return False
    rest = {key: value for key, value in qualification.items() if key != "live_qualified"}
    return rest == {key: value for key, value in QUALIFICATION.items() if key != "live_qualified"}

EVIDENCE = {
    "monitoring_contract": {"repository": "helianthus-docs-modbus", "issue": 242, "pull_request": 253, "pages": ["protocols/huawei/monitoring-readonly-v1.md", "protocols/huawei/read-pacing-v1.md"]},
    "registry_profiles": {"repository": "helianthus-modbusreg", "issue": 348, "pull_request": 357},
    "program_issues": {"repository": "helianthus-modbusreg", "issues": [199, 201]},
    "multi_endpoint_contract": {"repository": "helianthus-docs-ebus", "issue": 746, "pull_request": 748},
    "sibling_gates": ["huawei-sun2000-pv-v1", "huawei-gateway-meter-infrastructure-v1"],
    "grades": ["proven", "hypothesis", "unknown"],
}
# The gate admits the facts of the native monitoring contract for the exact family; each remaining precondition is decided from the documents with its grade.
ADMISSION = {
    "profile_source": {"repository": "helianthus-docs-modbus", "pages": ["protocols/huawei/monitoring-readonly-v1.md", "protocols/huawei/inverter-battery-readonly-v1.md"], "profile": "huawei.luna2000", "scope": "hybrid_inverter_battery_and_monitoring_contract"},
    "selection": {"admitting": "PROFILE_SELECTED", "unresolved": ["INSUFFICIENT_EVIDENCE", "NO_ADMISSIBLE_PROFILE", "AMBIGUOUS"], "unresolved_outcome": "withheld_never_zero"},
    "facts_admitted": True,
    "registry_flags": REGISTRY_FLAGS,
    "decisions": [
        {"precondition": "battery_profile", "decision": "declared_from_the_profile_source_for_the_exact_family", "grade": "hypothesis"},
        {"precondition": "word_form", "decision": "type_word_order_gain_and_unit_stated_per_row", "grade": "hypothesis"},
        {"precondition": "battery_power_sign", "decision": "plant_group_states_positive_while_charging_and_site_group_states_none_so_site_power_is_withheld", "grade": "hypothesis"},
        {"precondition": "aggregation_basis", "decision": "site_or_plant_group_recorded_as_provenance", "grade": "proven"},
        {"precondition": "asset_granularity", "decision": "one_storage_system_with_its_battery_packs_as_the_pack_defines", "grade": "proven"},
        {"precondition": "executable_vectors", "decision": "the_scenarios_of_this_gate", "grade": "proven"},
    ],
}
ADMITTING, UNRESOLVED = "PROFILE_SELECTED", ["INSUFFICIENT_EVIDENCE", "NO_ADMISSIBLE_PROFILE", "AMBIGUOUS"]
# The aggregation basis of each mapped group reading, recorded as provenance on its fact.
AGGREGATION_BASIS = {"site_battery_soc": "site_group", "plant_battery_power": "plant_group", "site_battery_energy_charged_total": "site_group", "site_battery_energy_discharged_total": "site_group", "site_rated_storage_capacity": "site_group"}
# The rated storage capacity word is a native energy value with an SI target in the pack; no conversion from ampere-hours, voltage or state of charge exists.
CAPACITY = {"energy_units": "mapped", "source": "site_rated_storage_capacity", "target": "storage.energy.nominal_capacity", "conversion": "forbidden_from_ampere_hours_voltage_or_state_of_charge"}
GAINS = {1: 0, 10: -1, 100: -2, 1000: -3}
WORD_COUNT = {"U16": 1, "U32": 2, "I32": 2, "U64": 4}
# native item -> (source, read group, word index in the read). Registry items come from the LUNA2000 disposition; mapped items from the monitoring contract.
INVENTORY = [
    ("battery_group_37000_30", "registry_disposition", (37000, 30), None), ("battery_single_read_37000_1", "registry_disposition", (37000, 1), None),
    ("battery_group_37066_4", "registry_disposition", (37066, 4), None), ("battery_group_47000_1", "registry_disposition", (47000, 1), None),
    ("site_battery_soc", "monitoring_contract_planned", (30368, 1), 0), ("site_battery_power", "monitoring_contract_planned", (30354, 12), 6), ("plant_battery_power", "monitoring_contract_planned", (37498, 8), 6),
    ("site_battery_energy_charged_total", "monitoring_contract_planned", (30306, 46), 2), ("site_battery_energy_discharged_total", "monitoring_contract_planned", (30306, 46), 8),
    ("site_battery_energy_charged_day", "monitoring_contract_planned", (30306, 46), 0), ("site_battery_energy_discharged_day", "monitoring_contract_planned", (30306, 46), 6),
    ("site_rated_storage_capacity", "monitoring_contract_planned", (30322, 2), 0),
]
# The routed unit of each planned read: the EMMA site groups answer at unit 0 and the S-Dongle plant group at unit 100. The registry battery groups state no routed unit.
SITE_UNIT, PLANT_UNIT = 0, 100
ROUTES = {"site_battery_soc": SITE_UNIT, "site_battery_power": SITE_UNIT, "plant_battery_power": PLANT_UNIT, "site_battery_energy_charged_total": SITE_UNIT, "site_battery_energy_discharged_total": SITE_UNIT,
          "site_battery_energy_charged_day": SITE_UNIT, "site_battery_energy_discharged_day": SITE_UNIT, "site_rated_storage_capacity": SITE_UNIT}
PLAN = {read: ROUTES.get(native_id) for native_id, _, read, _ in INVENTORY if read}
# What an answered zero word of an active row means: a state of charge word of zero is a value of zero; a lifetime counter is withheld when its whole read is zero words, a power or capacity word of zero is no battery value.
ZERO_WORDS = {"site_battery_soc": "value", "plant_battery_power": "withheld", "site_battery_energy_charged_total": "read_withheld", "site_battery_energy_discharged_total": "read_withheld", "site_rated_storage_capacity": "withheld"}
# The daily counters have no SemReg element: their words are served in the modbusreg namespace and never published as a lifetime fact.
DAY_COUNTERS = {"site_battery_energy_charged_day", "site_battery_energy_discharged_day"}
# The rows that publish a fact; the others stay withheld or native-only with their reason.
ACTIVE = {"site_battery_soc", "plant_battery_power", "site_battery_energy_charged_total", "site_battery_energy_discharged_total", "site_rated_storage_capacity"}
RUNTIME_REASONS = {"storage.reason.aggregate_without_pack_evidence", "storage.reason.battery_group_not_read", "storage.reason.zero_word_is_not_a_battery_value", "storage.reason.battery_group_exception_no_negation", "storage.reason.value_outside_pack_range"}
PRIMARY_REASON = "storage.reason.no_admitted_battery_profile"
NATIVE_ONLY_REASON = "storage.reason.group_native_only_no_catalog_row"
STATUS_REASON = "storage.reason.status_symbols_not_stated"
REQUESTED = ["storage.energy.charged", "storage.energy.discharged", "storage.energy.nominal_capacity", "storage.pack.power", "storage.state.soc", "storage.status.operating"]
NEVER_REQUESTED = ["storage.limit.charge_power", "storage.limit.discharge_power", "storage.status.interlock", "storage.operation.set_charge_limit", "storage.operation.set_discharge_limit"]
# Write authority is declared from the vendor's published interface; the charge and discharge executor belongs to the executor row, so this gate requests no operation.
WRITE_AUTHORITY = {"declared": True, "source": "vendor_published_interface", "pages": ["protocols/huawei/monitoring-readonly-v1.md", "protocols/huawei/inverter-battery-readonly-v1.md"], "executor": "battery_charge_and_discharge_executor_owned_by_MM09", "gate_requests_no_operation": True}
NATIVE_NAMESPACE = {"namespace": "modbusreg", "reason": "no_catalog_element"}
OPERATIONS_DISPOSITION = "write_authority_declared_executor_owned_by_MM09"
UNSUPPORTED = ["state_of_health", "cell_and_module_values", "charge_and_discharge_power_limits", "interlock", "working_mode_or_schedule", "remote_dispatch", "acknowledgement", "readback", "retry", "control_routes"]
SIGNS = {"unknown_no_value_without_a_graded_sign", "positive_while_charging_negative_while_discharging"}
COUNTERS = {"reset_wrap_and_continuity_never_inferred"}
DOC_TOKENS = ("gateway-allocated", "operator label", "no anchor key source", "unknown, not absent", "section 15", "en-1", "unknown, never zero", "no binary floating point", "rated storage capacity is mapped in joules", "outbound_allowed=false", "one atomic cutover", "never converted")


def fail(code):
    raise ValueError(code)


class Catalog:
    """Element-catalog lookups for this gate: kinds, relations, facts, and the storage pack's dimensions and relations."""

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

    def parent(self, kind):
        """RES-2: a component's parent is the whole of the pack's contains definition whose part is its kind."""
        for name in self.pack_relations:
            definition = self.index.relations[name]
            if definition["core_type"] == "contains" and definition["to_kinds"] == [kind]:
                return definition["from_kinds"][0]
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


def check_prospective_conversion(prospective, rule, catalog):
    """A quantity row repeats the rule's native form and states the exact conversion of its unit pair and its loss kinds: a unit loss exactly when the conversion is not the identity, a provenance loss where the catalog keeps the native sign convention and a policy loss on every counter."""
    native = prospective.get("native")
    if not isinstance(native, dict) or native != rule.get("native"):
        fail("prospective native form")
    expected = UNIT_CONVERSION.get((native.get("unit"), prospective["unit"]))
    losses = prospective.get("loss")
    if expected is None or prospective.get("conversion") != expected or not set(prospective) <= QUANTITY_ROW_MEMBERS or not isinstance(losses, list) or not all(isinstance(kind, str) and kind in LOSS_KINDS for kind in losses) or len(losses) != len(set(losses)):
        fail("prospective conversion")
    if (expected != IDENTITY) != ("unit" in losses):
        fail("silent unit conversion")
    if catalog.fact(prospective["target"]).get("sign_reference") == "native_convention" and "provenance" not in losses:
        fail("prospective sign")
    if prospective["target"] in ("storage.energy.charged", "storage.energy.discharged") and "policy" not in losses:
        fail("prospective counter")


def pack_tables():
    tables = json.loads(PACK_TABLES.read_text(encoding="utf-8"))
    return {field["id"]: field for field in tables["field_specs"]}, {operation["id"] for operation in tables["operations"]}


def check_shared(document, catalog):
    """Section 15: the shared graph rules first; a gate of this family states its graph."""
    failures = graph_rules.check(document, catalog.document)
    if failures:
        fail("graph {} {}".format(*failures[0]))
    if not graph_rules.graph_stated(document):
        fail("graph_not_stated")


def check_subject(rule, prospective, catalog):
    """EN-1: a prospective row names its target's catalog subject kind and the gate's measurement point."""
    if prospective.get("subject_kind") != catalog.subject(prospective.get("target")).get("kind") or prospective.get("measurement_point") != MEASUREMENT_POINTS.get(rule["native_id"]):
        fail("field rule subject")


def check_sign(prospective, catalog):
    """EN-4: a row states a sign rule exactly when its catalog direction is signed."""
    signed = catalog.fact(prospective["target"]).get("direction") == "signed"
    if signed != ("sign" in prospective) or (signed and prospective["sign"] not in SIGNS):
        fail("prospective sign")


def expected_basis(resource, anchor_class, role=None):
    """Section 6.3: a listed scheme anchors on a native token, a component or a pack on its native position, anything else on an operator label."""
    if resource["identification_scheme"] != graph_rules.NONE_LISTED:
        return "native_identity"
    return "native_slot" if anchor_class == "component" or role in NATIVE_POSITION_ROLES else "operator_slot"


def check_graph_resources(graph, catalog):
    """Section 15 item 1, RES-1 and RES-2: one resource per role, in role order, with its anchor, parent and evidence statements."""
    kinds = catalog.role_kinds()
    if [resource["kind"] for resource in graph["resources"]] != list(kinds.values()):
        fail("graph_resources")
    for role, resource in zip(kinds, graph["resources"]):
        anchor_class = catalog.index.kinds[resource["kind"]]["anchor_class"]
        if resource["anchor_basis"] != expected_basis(resource, anchor_class, role):
            fail("graph_anchor")
        if resource["parent_kind"] != (catalog.parent(resource["kind"]) if anchor_class == "component" else None):
            fail("graph_parent")
        stated = {item.split(":")[0] for item in resource["evidence"]}
        if not {"native_owner_token_stability", ROLE_STATEMENTS[role]} <= stated:
            fail("graph_evidence")


def check_graph_paths(graph, catalog):
    """Section 15 item 3 and section 6.4: the inverter relays the battery; a second view joins on a shared key or a declaration only."""
    kinds = catalog.role_kinds()
    battery = next(resource for resource in graph["resources"] if resource["kind"] == kinds["battery"])
    if graph["relayed"] != [{"relaying_kind": kinds["inverter"], "relayed_kind": kinds["battery"], "direct_anchor_key_source": DIRECT_BATTERY_KEY}]:
        fail("graph_relayed")
    native = battery["anchor_basis"] == "native_identity"
    join = {"kind": kinds["battery"], "observations": TWO_VIEWS, "basis": "same_anchor_key" if native else "operator_declaration", "native_tokens": "equal" if native else "not_exposed"}
    if graph["joins"] != [join]:
        fail("graph_joins")


def check_graph_relations(graph, document, catalog):
    """Section 15 item 2, REL-3 and section 7.2: stated basis and rule scope, the registry profile's scope, no absence statement."""
    scope = {"profile": document["admission"]["profile_source"]["profile"], "version": SCOPE_VERSION}
    kinds = catalog.role_kinds()
    if [(relation["definition"], relation["basis_class"], relation["validity_scope"]["rule_scope"]) for relation in graph["relations"]] != STATED_RELATIONS:
        fail("graph_relations")
    for relation in graph["relations"]:
        definition = catalog.index.relations[relation["definition"]]
        ends = (kinds["inverter"], kinds["battery"]) if definition["core_type"] == "relays" else (definition["from_kinds"][0], definition["to_kinds"][0])
        stated_scope = {key: relation["validity_scope"][key] for key in scope}
        if (relation["from_kind"], relation["to_kind"]) != ends or relation["absence_sides"] or stated_scope != scope:
            fail("graph_relations")


def expected_aggregates(document, catalog):
    """Section 15 item 4, EN-1 and EN-5: each prospective target the catalog calls an aggregate, on its catalog subject, without coverage."""
    aggregates = []
    for rule in document["field_rules"]:
        target = (rule.get("prospective") or {}).get("target")
        fact = catalog.fact(target)
        entry = {"fact": target, "subject_kind": fact.get("subject", {}).get("kind"), "coverage": []}
        if fact.get("aggregation") == "aggregate" and entry not in aggregates:
            aggregates.append(entry)
    return aggregates


def expected_unstated(document, catalog):
    """Section 7.2: a storage relation the gate does not state stays unknown, never absent."""
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
    fields, operations = pack_tables()
    check_luna_header(document)
    rules = document.get("field_rules", [])
    listed = [(rule.get("native_id"), rule.get("source"), (rule["read"]["offset"], rule["read"]["quantity"]) if rule.get("read") else None, rule.get("word")) for rule in rules]
    if listed != INVENTORY or [(type(rule["read"].get("unit")), rule["read"].get("unit")) for rule in rules] != [(type(ROUTES.get(rule["native_id"])), ROUTES.get(rule["native_id"])) for rule in rules]:
        fail("native inventory")
    reasons = document.get("reasons", {})
    check_luna_reasons(reasons)
    used, targets = check_luna_rules(rules, reasons, fields, catalog)
    if sorted(targets) != REQUESTED or document.get("projection", {}).get("requested_items") != [{"kind": "fact", "item_id": item} for item in REQUESTED]:
        fail("requested items")
    if set(reasons) != used | RUNTIME_REASONS | {PRIMARY_REASON, NATIVE_ONLY_REASON, STATUS_REASON}:
        fail("reason inventory")
    check_luna_policies(document, fields, operations)
    validate_graph(document, catalog)
    if contains_float(document):
        fail("binary floating point")


def check_luna_header(document):
    if document.get("contract") != "helianthus.semantic.mapping.huawei-luna2000.storage/v1" or document.get("mapping_id") != "storage.mapping.huawei.luna2000.battery" or document.get("pack") != PACK or document.get("status") != "accepted_source_specific_mapping_gate":
        fail("contract")
    if document.get("evidence") != EVIDENCE:
        fail("evidence")
    if document.get("admission") != ADMISSION:
        fail("admission")


def check_luna_reasons(reasons):
    for name, spec in reasons.items():
        if not name.startswith("storage.reason.") or spec.get("loss_kind") not in LOSS_KINDS or not spec.get("description") or type(spec.get("reversible")) is not bool:
            fail("reason loss")


def check_luna_rules(rules, reasons, fields, catalog):
    """A row either publishes its fact (an active row, with its loss) or is withheld or native-only with a declared reason; a row with a target states its prospective mapping."""
    used, targets = set(), set()
    for rule in rules:
        check_luna_disposition(rule, reasons, used)
        prospective = rule.get("prospective")
        if prospective is None:
            continue
        targets.add(check_prospective(rule, prospective, fields, catalog))
    return used, targets


def check_luna_disposition(rule, reasons, used):
    disposition = rule.get("disposition", {})
    if rule.get("grade") not in GRADES:
        fail("withheld reason")
    if rule.get("native_id") in ACTIVE:
        if set(disposition) != {"outcome", "loss"} or disposition["outcome"] != "transformed" or not isinstance(disposition["loss"], list) or not set(disposition["loss"]) <= LOSS_KINDS or "unit" not in disposition["loss"] or rule.get("zero_word") != ZERO_WORDS[rule["native_id"]]:
            fail("active disposition")
        return
    if "zero_word" in rule or set(disposition) != {"outcome", "reason"} or disposition["outcome"] not in allowed_outcomes(rule) or disposition["reason"] not in reasons:
        fail("withheld reason")
    used.add(disposition["reason"])


def allowed_outcomes(rule):
    if rule.get("native_id") in DAY_COUNTERS:
        return ("native_only",)
    return ("withheld", "native_only" if rule.get("source") == "registry_disposition" else "withheld")


def check_prospective(rule, prospective, fields, catalog):
    target = prospective.get("target")
    pack_field = fields.get(target)
    if prospective.get("in_force") is not (rule.get("native_id") in ACTIVE) or pack_field is None or prospective.get("dimension") != catalog.subject(target).get("dimension", {}).get("id") or prospective.get("unit") != pack_field.get("unit"):
        fail("prospective target")
    native = prospective.get("native")
    if pack_field["kind"] == "symbol":
        if prospective.get("symbols") != [] or native is not None or set(prospective) != SYMBOL_ROW_MEMBERS:
            fail("prospective symbols")
    else:
        check_prospective_conversion(prospective, rule, catalog)
    check_subject(rule, prospective, catalog)
    check_sign(prospective, catalog)
    if target in ("storage.energy.charged", "storage.energy.discharged") and prospective.get("counter") not in COUNTERS:
        fail("prospective counter")
    return target


def check_projection_member(projection):
    if projection.get("primary_reason") != PRIMARY_REASON or projection.get("error_precedence") != ERROR_PRECEDENCE or projection.get("operations") != OPERATIONS_DISPOSITION or projection.get("missing_native") != "unavailable_withheld_never_zero" or projection.get("target") != "SemReg_storage_projection":
        fail("projection")
    if projection.get("native_namespace") != NATIVE_NAMESPACE:
        fail("projection")


def check_luna_policies(document, fields, operations):
    if document.get("identity") != IDENTITY_CONTRACT:
        fail("identity contract")
    if document.get("lifecycle") != {
        "required_observation_evidence": ["observation_id", "observation_revision", "receipt_wall", "receipt_monotonic", "clock_epoch", "source_epoch", "driver_generation", "connection_generation", "luna_selection", "reads"],
        "absent_optional_group": "unknown_never_zero", "exception": "no_negation_of_the_identified_parent",
        "last_known_good": "retained_with_its_original_age_when_a_battery_value_exists", "withdrawal": "explicit_generation_fenced"}:
        fail("lifecycle contract")
    if document.get("outcome_vocabulary") != {"exact": "exact", "transformed": "transformed", "withheld": "withheld", "native_only": "unrepresentable"}:
        fail("outcome vocabulary")
    if document.get("capacity") != CAPACITY:
        fail("capacity")
    if document.get("unsupported") != UNSUPPORTED:
        fail("unsupported")
    if document.get("write_authority") != WRITE_AUTHORITY:
        fail("write authority")
    never = document.get("pack_items_never_requested")
    if never != NEVER_REQUESTED or any(item not in fields and item not in operations for item in never) or set(never) & set(REQUESTED):
        fail("control-adjacent items")
    check_projection_member(document.get("projection", {}))
    if document.get("consumer_cutover") != {"consumers": ["semantic_mcp", "graphql", "portal", "home_assistant"], "path": "shared_semantic_snapshot_only", "until_implemented": "no_public_consumer_binding"}:
        fail("consumer cutover")


def validate_case(case, contract):
    if not isinstance(case, dict):
        fail("native_evidence_invalid")
    errors = set()
    selection = case.get("luna_selection")
    outcome = selection.get("outcome") if isinstance(selection, dict) else None
    if outcome not in [ADMITTING] + UNRESOLVED or contains_float(case):
        errors.add("native_evidence_invalid")
    reads = check_case_reads(case, errors)
    check_case_identity(case, errors)
    check_case_lifecycle(case, errors)
    if not qualification_recorded(case.get("qualification")):
        errors.add("mapping_unqualified")
    if case.get("request_operation") or case.get("request_conversion") or case.get("request_item"):
        errors.add("unsupported_or_withheld")
    for code in ERROR_PRECEDENCE:
        if code in errors:
            fail(code)
    return reads


def check_case_reads(case, errors):
    reads = {}
    for read in case.get("reads") if isinstance(case.get("reads"), list) else [None]:
        key = (read.get("offset"), read.get("quantity")) if isinstance(read, dict) else None
        if key not in PLAN or key in reads or read.get("outcome") not in ("ANSWERED", "EXCEPTION", "FAILED", "NOT_READ") or not on_route(read, PLAN[key]):
            errors.add("native_evidence_invalid")
            continue
        if not read_is_valid(read, key):
            errors.add("native_evidence_invalid")
        reads[key] = read
    return reads


def on_route(read, route):
    """A read of a routed group is evidence only from that group's routed unit; a group with no stated route is identified by offset and quantity."""
    unit = read.get("unit")
    return route is None or (type(unit) is int and unit == route)


def read_is_valid(read, key):
    words, code = read.get("words"), read.get("exception_code")
    if read["outcome"] == "ANSWERED":
        return isinstance(words, list) and len(words) == key[1] and all(type(word) is int and 0 <= word <= 0xFFFF for word in words) and code is None
    if read["outcome"] == "EXCEPTION":
        return words is None and type(code) is int and code >= 1
    return words is None and code is None


def case_packs(identity):
    """The gateway-allocated pack ResourceIDs of the configured battery, by native position 1 to 3."""
    packs = (identity.get("resources") or {}).get("packs") if isinstance(identity, dict) else None
    return packs if isinstance(packs, list) and 1 <= len(packs) <= RESOURCE_ROLES["packs"]["slots"] else [None]


def check_case_identity(case, errors):
    identity, source = case.get("identity"), case.get("source")
    values = (identity.get("asset_id") if isinstance(identity, dict) else None, identity.get("binding_id") if isinstance(identity, dict) else None, source.get("source_id") if isinstance(source, dict) else None, *case_packs(identity))
    if any(not isinstance(item, str) or not OPAQUE_RE.fullmatch(item) or any(token in item.lower() for token in ID_FORBIDDEN_TEXT) or ID_ADDRESS_RE.search(item) for item in values) or len(set(values)) != len(values):
        errors.add("identity_missing_or_invalid")


def check_case_lifecycle(case, errors):
    life = case.get("lifecycle") if isinstance(case.get("lifecycle"), dict) else {}
    if any(not isinstance(life.get(key), str) or not OPAQUE_RE.fullmatch(life[key]) for key in ("observation_id", "clock_epoch", "source_epoch")) or any(type(life.get(key)) is not int or life[key] < 1 for key in ("observation_revision", "driver_generation", "connection_generation")) or type(life.get("receipt_monotonic")) is not int or life.get("receipt_monotonic", 0) < 1:
        errors.add("lifecycle_missing")
    try:
        if datetime.fromisoformat(life.get("receipt_wall", "").replace("Z", "+00:00")).tzinfo is None:
            raise ValueError
    except (AttributeError, ValueError):
        errors.add("lifecycle_missing")


def decimal_of(number):
    return Decimal(int(number["coefficient"])).scaleb(number["exponent10"])


def canonical(coefficient, exponent):
    if coefficient == 0:
        return {"coefficient": "0", "exponent10": 0}
    while coefficient % 10 == 0:
        coefficient //= 10
        exponent += 1
    return {"coefficient": str(coefficient), "exponent10": exponent}


def convert(coefficient, exponent, conversion):
    """Exact native x factor + offset in integers; the native value and unit stay in the evidence."""
    factor, offset = conversion["factor"], conversion["offset"]
    product, product_exponent = coefficient * int(factor["coefficient"]), exponent + factor["exponent10"]
    shared = min(product_exponent, offset["exponent10"])
    return canonical(product * 10 ** (product_exponent - shared) + int(offset["coefficient"]) * 10 ** (offset["exponent10"] - shared), shared)


def native_value(words, native):
    """The native word form of a row: big-endian words, two's complement for a signed type, the gain as a decimal exponent."""
    raw = 0
    for word in words:
        raw = (raw << 16) | word
    if native["type"].startswith("I") and raw >= 1 << (16 * len(words) - 1):
        raw -= 1 << (16 * len(words))
    return raw, GAINS[native["gain"]]


def within_bounds(number, bounds):
    value = decimal_of(number)
    return all(limit is None or (value >= decimal_of(limit) if name == "minimum" else value <= decimal_of(limit)) for name, limit in bounds.items())


ZERO_REASON = "storage.reason.zero_word_is_not_a_battery_value"


def unanswered_reason(read):
    """The reason a read withholds its words, or None when it answered."""
    if read is None or read["outcome"] in ("FAILED", "NOT_READ"):
        return "storage.reason.battery_group_not_read"
    if read["outcome"] == "EXCEPTION":
        return "storage.reason.battery_group_exception_no_negation"
    return None


def read_reason(read):
    """The reason a read of a registry battery group withholds its words: not answered, or answered with zero words only."""
    return unanswered_reason(read) or (ZERO_REASON if all(word == 0 for word in read["words"]) else None)


def field_words(rule, read):
    return read["words"][rule["word"]:rule["word"] + WORD_COUNT[rule["native"]["type"]]]


def active_reason(rule, read, selected, pack_count):
    """Selection, then the read, then the row's own zero disposition, then the pack evidence."""
    reason = None if selected else PRIMARY_REASON
    reason = reason or unanswered_reason(read)
    if reason:
        return reason
    if rule["zero_word"] == "withheld" and not any(field_words(rule, read)):
        return ZERO_REASON
    if rule["zero_word"] == "read_withheld" and not any(read["words"]):
        return ZERO_REASON
    return "storage.reason.aggregate_without_pack_evidence" if pack_count != 1 else None


def mapped_row(rule, reads, selected, fields, pack_count):
    """(native row, fact) of one active row: withheld by selection, read or pack evidence, withheld out of range, or its fact in SI.
    A site or plant group reading belongs to the pack only when the configured battery has exactly one; with several, no pack holds the evidence."""
    nid = rule["native_id"]
    read = reads.get((rule["read"]["offset"], rule["read"]["quantity"]))
    reason = active_reason(rule, read, selected, pack_count)
    if reason:
        return {"native_id": nid, "outcome": "withheld", "reason": reason}, None
    prospective = rule["prospective"]
    raw, exponent = native_value(field_words(rule, read), rule["native"])
    number = convert(raw, exponent, prospective["conversion"])
    if not within_bounds(number, fields[prospective["target"]]["bounds"]):
        return {"native_id": nid, "outcome": "withheld", "reason": "storage.reason.value_outside_pack_range"}, None
    return {"native_id": nid, "outcome": "transformed", "target": prospective["target"]}, {"rule": rule, "number": number}


def native_row(rule, reads, selected, fields, pack_count):
    if rule["native_id"] in ACTIVE:
        return mapped_row(rule, reads, selected, fields, pack_count)
    reason = rule["disposition"]["reason"]
    read = reads.get((rule["read"]["offset"], rule["read"]["quantity"])) if rule.get("read") else None
    if rule["source"] == "registry_disposition":
        reason = read_reason(read) or NATIVE_ONLY_REASON
        return {"native_id": rule["native_id"], "outcome": "native_only" if reason == NATIVE_ONLY_REASON else "withheld", "reason": reason}, None
    if rule["native_id"] in DAY_COUNTERS:
        unread = unanswered_reason(read)
        return {"native_id": rule["native_id"], "outcome": "withheld" if unread else "native_only", "reason": unread or reason}, None
    return {"native_id": rule["native_id"], "outcome": "withheld", "reason": reason}, None


def native_entry(rule, read):
    """A registry group is served whole; a daily counter is served as its own words with its routed unit and native form, never as a lifetime fact."""
    if rule["native_id"] not in DAY_COUNTERS:
        return {"namespace": NATIVE_NAMESPACE["namespace"], "native_id": rule["native_id"], "offset": rule["read"]["offset"], "quantity": rule["read"]["quantity"], "words": list(read["words"])}
    return {"namespace": NATIVE_NAMESPACE["namespace"], "native_id": rule["native_id"], "unit": rule["read"]["unit"], "offset": rule["read"]["offset"] + rule["word"], "quantity": WORD_COUNT[rule["native"]["type"]],
            "words": field_words(rule, read), "native": rule["native"]}


def native_values(rules, rows, reads):
    """The answered words of every item that has no catalog row, served in the modbusreg namespace with the item's native identity."""
    served = {row["native_id"] for row in rows if row["outcome"] == "native_only"}
    return [native_entry(rule, reads[(rule["read"]["offset"], rule["read"]["quantity"])]) for rule in rules if rule["native_id"] in served]


def item_disposition(item_id, rules, rows, facts, reasons, lifecycle):
    """One requested item: published when a row of its target published, otherwise withheld with the reason of its active row first."""
    mine = [fact for fact in facts if fact["rule"]["prospective"]["target"] == item_id]
    if mine:
        kinds = [kind for kind in LOSS_ORDER if any(kind in fact["rule"]["disposition"]["loss"] for fact in mine)]
        return {"kind": "fact", "item_id": item_id, "outcome": "transformed", "source_keys": [fact["rule"]["native_id"] for fact in mine], "loss": [{"kind": kind, "source_items": [NATIVE_PREFIX + fact["rule"]["native_id"] for fact in mine]} for kind in kinds]}
    targeted = sorted((rule for rule in rules if (rule.get("prospective") or {}).get("target") == item_id), key=lambda rule: rule["native_id"] not in ACTIVE)
    reason = STATUS_REASON if item_id == "storage.status.operating" else (next(row["reason"] for row in rows if row["native_id"] == targeted[0]["native_id"]) if targeted else PRIMARY_REASON)
    spec = reasons[reason]
    return {"kind": "fact", "item_id": item_id, "outcome": "withheld", "source_keys": [], "reason": reason,
            "loss": [{"kind": spec["loss_kind"], "source_items": [NATIVE_PREFIX + "battery"], "description": spec["description"], "reversible": spec["reversible"]}]}


LOSS_ORDER = ["unit", "provenance", "policy"]


def project(case, contract):
    reads = validate_case(case, contract)
    fields, _ = pack_tables()
    selected = case["luna_selection"]["outcome"] == ADMITTING
    packs, lifecycle = case_packs(case["identity"]), case["lifecycle"]
    pack = packs[0]
    rows, facts = [], []
    for rule in contract["field_rules"]:
        row, fact = native_row(rule, reads, selected, fields, len(packs))
        rows.append(row)
        if fact:
            facts.append(fact)
    published = [{"item_id": fact["rule"]["prospective"]["target"], "subject": {"dimension": fact["rule"]["prospective"]["dimension"], "resource": pack},
                  "value": {"number": fact["number"], "unit": fact["rule"]["prospective"]["unit"]}, "grade": fact["rule"]["grade"],
                  "provenance": {"native_id": fact["rule"]["native_id"], "aggregation_basis": AGGREGATION_BASIS[fact["rule"]["native_id"]], **({"sign": fact["rule"]["prospective"]["sign"]} if "sign" in fact["rule"]["prospective"] else {})},
                  "observation_id": lifecycle["observation_id"], "observation_revision": lifecycle["observation_revision"]} for fact in facts]
    requested = copy.deepcopy(contract["projection"]["requested_items"])
    dispositions = [item_disposition(item["item_id"], contract["field_rules"], rows, facts, contract["reasons"], lifecycle) for item in requested]
    output = {"facts": published, "relations": [], "requested": requested, "dispositions": dispositions, "native_dispositions": rows, "native": native_values(contract["field_rules"], rows, reads), "operations": []}
    assert_complete(output, contract)
    return output


def assert_complete(output, contract):
    """Every native item carries exactly one disposition; a withheld or native-only row a declared reason; a transformed row exactly its fact."""
    rows = output["native_dispositions"]
    if [row["native_id"] for row in rows] != [nid for nid, _, _, _ in INVENTORY] or output["relations"]:
        fail("native disposition completeness")
    if any(row["outcome"] not in ("withheld", "native_only", "transformed") or (row["outcome"] != "transformed" and row["reason"] not in contract["reasons"]) for row in rows):
        fail("withheld without reason")
    if [row["native_id"] for row in rows if row["outcome"] == "transformed"] != [fact["provenance"]["native_id"] for fact in output["facts"]]:
        fail("native disposition completeness")


def summarize(output):
    return {"facts": [{"item_id": fact["item_id"], "number": fact["value"]["number"], "unit": fact["value"]["unit"], "native_id": fact["provenance"]["native_id"], "aggregation_basis": fact["provenance"]["aggregation_basis"]} for fact in output["facts"]],
            "dispositions": [{"item_id": item["item_id"], "outcome": item["outcome"], "published": len(item["source_keys"]), "reason": item.get("reason"), "loss": [loss["kind"] for loss in item["loss"]]} for item in output["dispositions"]],
            "native_dispositions": output["native_dispositions"], "native": output["native"], "operations": output["operations"]}


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
        seen_reasons |= {row["reason"] for row in output["native_dispositions"] if "reason" in row} | {item["reason"] for item in output["dispositions"] if "reason" in item}
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
    return "%s, %s, gain %d" % (native["type"], native["unit"], native["gain"]) if native else "none decoded"


def plain(number):
    """A kernel Decimal written without an exponent, for the page."""
    digits, exponent = number["coefficient"].lstrip("-"), number["exponent10"]
    sign = "-" if number["coefficient"].startswith("-") else ""
    if exponent >= 0:
        return sign + digits + "0" * exponent
    digits = digits.rjust(1 - exponent, "0")
    return sign + digits[:exponent] + "." + digits[exponent:]


def expected_tables(contract):
    withheld, prospective = [], []
    for rule in contract["field_rules"]:
        reason = rule["disposition"].get("reason")
        if reason:
            withheld.append([rule["native_id"], rule["source"], rule["grade"].capitalize(), reason, contract["reasons"][reason]["loss_kind"]])
        plan = rule.get("prospective")
        if plan:
            rule_text = plan.get("sign") or plan.get("counter") or ("empty table, observed states only" if plan.get("symbols") == [] else "none")
            conversion = "%s, factor %s, offset %s" % (plan["unit"], plain(plan["conversion"]["factor"]), plain(plan["conversion"]["offset"])) if plan.get("conversion") else "none"
            prospective.append([plan["target"], plan["dimension"], rule["native_id"], native_form(plan.get("native") or rule.get("native")), conversion, "+".join(plan.get("loss", [])) or "none", rule_text, rule["grade"].capitalize(), "yes" if plan["in_force"] else "no"])
    return withheld, prospective


def graph_tables(contract):
    """The page's resource, relation, unstated-relation and subject tables, as the gate states them."""
    graph = contract["graph"]
    return {
        "## Resources": [[r["kind"], r["anchor_class"], r["anchor_basis"], r["anchor_source"], r["parent_kind"] or "none"] for r in graph["resources"]],
        "## Relations": [[r["definition"], r["basis_class"], r["from_kind"], r["to_kind"], r["validity_scope"]["rule_scope"]] for r in graph["relations"]],
        "## Unknown, not absent": [[r["definition"], r["from_kind"], r["to_kind"], r["withheld_by"]] for r in contract["unstated_relations"]],
        "## Daily counter native form": [[r["native_id"], native_form(r["native"])] for r in contract["field_rules"] if r["native_id"] in DAY_COUNTERS],
        "## Routes and zero words": [[r["native_id"], str(r["read"]["offset"]), str(r["read"]["quantity"]), str(r["read"]["unit"]) if "unit" in r["read"] else "not stated", r.get("zero_word", "not applicable")] for r in contract["field_rules"]],
        "## Subjects and measurement points": [[r["native_id"], r["prospective"]["target"], r["prospective"]["subject_kind"], r["prospective"]["measurement_point"]] for r in contract["field_rules"] if r.get("prospective")],
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
    print(f"Huawei LUNA2000 storage mapping: PASS; {positives} executable outputs and {len(contract['scenarios']) - positives} rejected scenarios")


if __name__ == "__main__":
    main()
