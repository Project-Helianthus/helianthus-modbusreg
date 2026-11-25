#!/usr/bin/env python3
"""Execute the exact Growatt BMS RS-485 v2.02 storage mapping gate."""
from __future__ import annotations

import copy
import importlib.util
import json
import re
from datetime import datetime
from decimal import Decimal, InvalidOperation
from fractions import Fraction
from functools import lru_cache
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
GATES = ROOT / "driverdata/families/mapping-gates"
SEMANTIC = ROOT / ".tmp/references/docs-semantic"
PATH = GATES / "growatt-bms-rs485-v202-storage-v1.json"
DOC = GATES / "growatt-bms-rs485-v202-storage-v1.md"
PACK_TABLES = SEMANTIC / "api/v1/packs/storage-bms-contract-tables.json"
_GRAPH_SPEC = importlib.util.spec_from_file_location("mapping_gate_graph_rules", Path(__file__).resolve().parent / "mapping_gate_graph_rules.py")
graph_rules = importlib.util.module_from_spec(_GRAPH_SPEC)
_GRAPH_SPEC.loader.exec_module(graph_rules)
PACK = {"id": "helianthus.pack.storage", "version": "1.1.0"}
# Exact conversions of the unit rule in api/README.md: (native unit, canonical SI unit) -> factor and offset, SI = native x factor + offset.
ONE, ZERO = {"coefficient": "1", "exponent10": 0}, {"coefficient": "0", "exponent10": 0}
IDENTITY = {"factor": ONE, "offset": ZERO}
UNIT_CONVERSION = {
    ("%", "unit.ratio"): {"factor": {"coefficient": "1", "exponent10": -2}, "offset": ZERO},
    ("V", "unit.volt"): IDENTITY,
    ("A", "unit.ampere"): IDENTITY,
    ("degC", "unit.kelvin"): {"factor": ONE, "offset": {"coefficient": "27315", "exponent10": -2}},
    ("Ah", "unit.ampere_hour"): IDENTITY,
    ("mV", "unit.volt"): {"factor": {"coefficient": "1", "exponent10": -3}, "offset": ZERO},
}
# Members of a field rule; no other member, and so no second statement of a conversion, is accepted.
QUANTITY_RULE_MEMBERS = {"source", "requested", "target", "subject_kind", "measurement_point", "disposition", "unit", "native_unit", "conversion"}
SYMBOL_RULE_MEMBERS = {"source", "requested", "target", "subject_kind", "measurement_point", "disposition", "rule"}
# EN-1: the measurement point of each field rule, by target. The subject kind comes from the element catalog.
MEASUREMENT_POINTS = {
    "storage.status.operating": "pack_as_a_whole", "storage.state.soc": "pack_as_a_whole",
    "storage.pack.voltage": "pack_terminals", "storage.pack.current": "pack_terminals",
    "storage.temperature.pack": "pack_as_a_whole",
    "storage.capacity.charge": "pack_terminals_lifetime_counter", "storage.capacity.discharge": "pack_terminals_lifetime_counter",
    "storage.cell.voltage_minimum": "extreme_cell_unindexed", "storage.cell.voltage_maximum": "extreme_cell_unindexed",
}
ALLOCATED = "gateway_allocated_resource_id"
# Section 6.3: one gateway-allocated ResourceID per resource role; a role's kind is its dimension's kind in the element catalog.
# The pack and one cell are roles. The native contract does not name the cell that holds an extreme, so both extremes share that cell.
RESOURCE_ROLES = {
    "pack": {"dimension": "storage.dimension.pack", "source": ALLOCATED, "required": True},
    "cell": {"dimension": "storage.dimension.cell", "source": ALLOCATED, "required": True},
}
# Section 15 item 1: the statements the evidence of each resource role carries.
ROLE_STATEMENTS = {
    "pack": ("future_native_token", "native_owner_token_stability", "identification_scheme"),
    "cell": ("native_owner_token_stability", "identification_scheme"),
}
PARENTS = {"pack": None, "cell": "resource.battery_module"}
WITHHELD_WAITS = {
    "remaining_capacity_amp_hours": "storage.energy.remaining",
    "full_charge_capacity_amp_hours": "storage.energy.usable_capacity",
    "cycle_count": "storage.reason.no_exact_cycle_count_definition",
    "continuous_charge_seconds": "storage.reason.no_exact_charge_time_definition",
    "current_cycle_charge_amp_hours": "storage.reason.cycle_time_domain_not_represented",
    "average_cell_voltage_volts": "storage.reason.no_cell_identity_or_extrema_semantics",
    "floating_pack_voltage_volts": "storage.reason.not_interchangeable_with_observed_pack_voltage",
}
# Section 15 item 3 and section 6.4: the pack is read directly and relayed by no unit; a second view of it shares no
# native token with this one, so it joins only by an operator declaration.
TWO_VIEWS = ["read_directly_over_rs485", "seen_through_another_path"]
# Section 15 item 5 and section 6.3: each native interpretation selector of the gate, and its classification.
# The routed unit number is an address: never an anchor, never identity evidence.
SELECTORS = [
    {"selector": "profile", "classification": "provenance", "evidence": ["native_contract:profile", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]},
    {"selector": "revision", "classification": "provenance", "evidence": ["native_contract:revision", "graph:6.3:selector_is_provenance_until_native_owner_states_otherwise"]},
    {"selector": "unit_id", "classification": "provenance", "evidence": ["native_contract:unit", "graph:6.3:a_routed_unit_number_is_never_an_anchor"]},
]
NATIVE_IDENTITY_EVIDENCE = ["mcu_software_version", "gauge_version", "bms_company", "bms_generation", "pack_company", "pack_generation", "box_or_battery_topology", "cell_series_count"]
# Section 7.2: what withholds each storage relation the gate does not state; none is stated, none is absent.
UNSTATED_BY = {
    "storage.relation.battery_pack": "unsupported:repeated_pack_identity",
    "storage.relation.module_cell": "unsupported:pack_module_cell_string_topology",
    "storage.relation.pack_interface": "unsupported:charge_discharge_limits",
    "storage.relation.pack_module": "unsupported:pack_module_cell_string_topology",
    "storage.relation.pack_string": "unsupported:pack_module_cell_string_topology",
    "storage.relation.system_battery": "configuration:no_storage_system_declared_by_this_gate",
}
UNSTATED_OUTCOME = "unknown_not_absent"
REVISION = {"family": "1xSxxP ESS", "file_revision": "Rev2.01", "header_version": "V2.0", "cumulative_revision": "2.02"}
SLICES = [
    {"function": "FC03", "offset": "0x0001", "quantity": 7},
    {"function": "FC03", "offset": "0x000D", "quantity": 29},
    {"function": "FC03", "offset": "0x0100", "quantity": 12},
    {"function": "FC03", "offset": "0x010D", "quantity": 2},
]
PINS = {
    "docs_semantic_base": "ed33276cddb2dd86757efcf335c95936bdf4efe2",
    "semreg": "f3f761bc67e10d6a65eba6c13cb4dc51002d6955",
    "storage_docs": "c20fdef000fe95df95fca60c55d651a0614c4efd",
    "storage_runtime": "6556bf4b3fffd645b6c14ebfd194d8570b7c5207",
    "docs_modbus_lifecycle": "35151979c8561d5dc4215030899277aec36d2f9f",
    "modbusreg": "7853d903970a4fdded35abaef01fe30f7e93be6a",
    "native_source": "6c08d4d2acf70bea622da333f6d75e26d2d92621",
    "gateway_main": "32244901c4c8337266cd348bdad90fb9e8eb0a61",
    "gateway_tree": "0a850d5646d46f5782b1396d72a3d93bffa6974e",
    "gateway_reviewed_source": "15ab7f0f4197485c83c04f8fe24000ebe058cb7a",
}
LOSS_KINDS = {"unit", "range", "precision", "time", "symbol", "provenance", "identity", "capability", "operation", "policy"}
ERROR_PRECEDENCE = ["native_evidence_missing", "revision_or_unit_or_slice_invalid", "identity_missing_or_invalid", "lifecycle_missing", "mapping_unqualified", "unsupported_or_withheld"]
OPAQUE_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:/@-]*$")
UNSUPPORTED = ["pack_power", "state_of_health", "cell_temperature_meaning", "pack_module_cell_string_topology", "repeated_pack_identity", "warning_error_company_status", "extension_words", "calibration_control_adjacent_fields", "writable_ranges", "charge_discharge_limits", "interlock", "authority", "write_operations", "acknowledgement", "readback", "retry", "control_routes"]


def load():
    return json.loads(PATH.read_text())


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

    def subject_dimension(self, fact_id):
        """Section 6.2: the resource dimension of a fact, whose value is the subject's ResourceID."""
        return self.subject(fact_id).get("dimension", {}).get("id")

    def role_kinds(self):
        """RES-1 and section 6.1: the kind of each resource role, through its pack dimension."""
        return {role: self.dimensions.get(spec["dimension"]) for role, spec in RESOURCE_ROLES.items()}


@lru_cache(maxsize=1)
def element_catalog():
    return Catalog(graph_rules.load_catalog())


def mutate(value, mutation):
    path = mutation["path"]
    aliases = {
        "native.request_adu": "observation.slices.0.request_adu_hex",
        "native.words": "observation.slices.0.words",
        "native.revision": "observation.revision",
        "native.unit_id": "observation.slices.0.unit_id",
        "native.slices": "observation.slices",
        "native.typed": "observation.typed",
    }
    for old, new in aliases.items():
        if path == old or path.startswith(old + "."):
            path = new + path[len(old):]
            break
    if "source" in value and (path == "identity.source_id" or path.startswith("identity.source_id.")):
        path = "source.source_id" + path[len("identity.source_id"):]
    cursor = value
    parts = path.split(".")
    for part in parts[:-1]:
        cursor = cursor[int(part)] if isinstance(cursor, list) else cursor[part]
    last = parts[-1]
    cursor[int(last) if isinstance(cursor, list) else last] = mutation["value"]


def crc16(data):
    crc = 0xFFFF
    for byte in data:
        crc ^= byte
        for _ in range(8):
            crc = (crc >> 1) ^ 0xA001 if crc & 1 else crc >> 1
    return crc


def decode_adu(text):
    if not isinstance(text, str) or len(text) % 2:
        return None
    try:
        return bytes.fromhex(text)
    except ValueError:
        return None


def valid_request_adu(raw, unit, offset, quantity):
    return raw is not None and len(raw) == 8 and raw[0] == unit and raw[1] == 3 and int.from_bytes(raw[2:4], "big") == offset and int.from_bytes(raw[4:6], "big") == quantity and int.from_bytes(raw[-2:], "little") == crc16(raw[:-2])


def valid_response_adu(raw, unit, words):
    payload = b"".join(word.to_bytes(2, "big") for word in words)
    return raw is not None and len(raw) == 5 + len(payload) and raw[0] == unit and raw[1] == 3 and raw[2] == len(payload) and raw[3:-2] == payload and int.from_bytes(raw[-2:], "little") == crc16(raw[:-2])


def decimal(value):
    try:
        number = Decimal(str(value))
    except (InvalidOperation, ValueError):
        fail("native_evidence_missing")
    if not number.is_finite():
        fail("native_evidence_missing")
    if not number:
        return {"coefficient": "0", "exponent10": 0}
    sign, digits, exponent = number.normalize().as_tuple()
    coefficient = "".join(str(digit) for digit in digits)
    if sign:
        coefficient = "-" + coefficient
    return {"coefficient": coefficient, "exponent10": exponent}


def signed16(word):
    return word - 0x10000 if word & 0x8000 else word


def fact_key(item_id, resource_id):
    """RES-5: the subject dimension of the fact carries the pack's ResourceID, never the asset identifier."""
    dimension = element_catalog().subject_dimension(item_id)
    return {"pack_id": PACK["id"], "pack_version": PACK["version"], "fact_id": item_id, "dimensions": [{"id": dimension, "value": {"kind": "text", "text": resource_id}}]}


def quantity(value, unit):
    return {"kind": "quantity", "quantity": {"number": decimal(value), "unit": unit}}


def fraction_of(number):
    return Fraction(int(number["coefficient"])) * Fraction(10) ** number["exponent10"]


def convert(value, conversion):
    """Exact native x factor + offset in fractions; every factor and offset of the table terminates in decimal, so no rounding occurs."""
    result, exponent = Fraction(value) * fraction_of(conversion["factor"]) + fraction_of(conversion["offset"]), 0
    while result.denominator != 1:
        result *= 10
        exponent -= 1
        if exponent < -18:
            fail("unit conversion")
    return Decimal("%dE%d" % (result.numerator, exponent))


def pack_fields():
    return {field["id"]: field for field in json.loads(PACK_TABLES.read_text(encoding="utf-8"))["field_specs"]}


def check_conversion(rule, fields):
    """A quantity rule names the SI unit of its pack field and the exact conversion of its unit pair; one that is not the identity declares a unit loss. A symbol rule names no unit and no conversion."""
    pack_field = fields.get(rule.get("target"))
    if pack_field is not None and pack_field.get("kind") == "symbol":
        if set(rule) != SYMBOL_RULE_MEMBERS:
            fail("unit conversion")
        return
    if pack_field is None or pack_field.get("unit") != rule.get("unit") or set(rule) != QUANTITY_RULE_MEMBERS:
        fail("unit conversion")
    expected = UNIT_CONVERSION.get((rule.get("native_unit"), rule.get("unit")))
    if expected is None or rule.get("conversion") != expected:
        fail("unit conversion")
    if (expected != IDENTITY) != ("unit" in [loss.get("kind") for loss in rule["disposition"].get("loss") or []]):
        fail("silent unit conversion")


def check_subject(rule, catalog):
    """EN-1: a field rule names its target's catalog subject kind and the gate's measurement point; the subject is one published role."""
    subject = catalog.subject(rule.get("target"))
    dimension = subject.get("dimension", {}).get("id")
    if rule.get("subject_kind") != subject.get("kind") or rule.get("measurement_point") != MEASUREMENT_POINTS.get(rule.get("target")):
        fail("field rule subject")
    if dimension not in {spec["dimension"] for spec in RESOURCE_ROLES.values()}:
        fail("field rule subject")


def check_sign(rule, catalog):
    """EN-4: a rule whose catalog entry keeps the native sign convention keeps that convention as a provenance loss."""
    if catalog.fact(rule.get("target")).get("sign_reference") == "native_convention":
        if "provenance" not in [loss.get("kind") for loss in rule["disposition"].get("loss") or []]:
            fail("field rule sign")


def check_shared(document, catalog):
    """Section 15: the shared graph rules first; a gate of this family states its graph."""
    failures = graph_rules.check(document, catalog.document)
    if failures:
        fail("graph {} {}".format(*failures[0]))
    if not graph_rules.graph_stated(document):
        fail("graph_not_stated")


def check_graph_resources(graph, catalog):
    """Section 15 item 1, RES-1 and RES-2: one resource per role, an operator label until a scheme is listed, its statements."""
    kinds = catalog.role_kinds()
    if [resource["kind"] for resource in graph["resources"]] != list(kinds.values()):
        fail("graph_resources")
    for role, resource in zip(kinds, graph["resources"]):
        listed = resource["identification_scheme"] != graph_rules.NONE_LISTED
        if resource["anchor_basis"] != ("native_identity" if listed else "operator_slot"):
            fail("graph_anchor")
        if resource["parent_kind"] != PARENTS[role]:
            fail("graph_parent")
        stated = {item.split(":")[0] for item in resource["evidence"]}
        if not set(ROLE_STATEMENTS[role]) <= stated:
            fail("graph_evidence")


def check_graph_paths(graph, catalog):
    """Section 15 items 2 and 3 and section 6.4: no relation is stated, nothing relays the pack, a second view joins by declaration."""
    if graph["relations"]:
        fail("graph_relations")
    if graph["relayed"]:
        fail("graph_relayed")
    kind = catalog.role_kinds()["pack"]
    pack = graph["resources"][0]
    native = pack["anchor_basis"] == "native_identity"
    join = {"kind": kind, "observations": TWO_VIEWS, "basis": "same_anchor_key" if native else "operator_declaration", "native_tokens": "equal" if native else "not_exposed"}
    if graph["joins"] != [join]:
        fail("graph_joins")


def expected_aggregates(document, catalog):
    """Section 15 item 4, EN-1 and EN-5: each target the catalog calls an aggregate, on its catalog subject, without coverage."""
    aggregates = []
    for rule in document["field_rules"]:
        fact = catalog.fact(rule.get("target"))
        entry = {"fact": rule.get("target"), "subject_kind": fact.get("subject", {}).get("kind"), "coverage": []}
        if fact.get("aggregation") == "aggregate" and entry not in aggregates:
            aggregates.append(entry)
    return aggregates


def expected_unstated(document, catalog):
    """Section 7.2: a storage relation the gate does not state stays unknown, never absent; what withholds it is unsupported or configuration."""
    stated = {relation["definition"] for relation in document["graph"]["relations"]}
    rows = []
    for name in catalog.pack_relations:
        if name not in stated:
            definition = catalog.index.relations[name]
            rows.append({"definition": name, "from_kind": definition["from_kinds"][0], "to_kind": definition["to_kinds"][0], "outcome": UNSTATED_OUTCOME, "withheld_by": UNSTATED_BY.get(name)})
    unsupported = {"unsupported:" + item for item in document.get("unsupported", [])}
    if any(row["withheld_by"].startswith("unsupported:") and row["withheld_by"] not in unsupported for row in rows):
        fail("unstated_relations")
    return rows


def validate_graph(document, catalog):
    """Section 15: this gate's own rules after the shared checker; no member decides a plane."""
    check_shared(document, catalog)
    graph = document["graph"]
    check_graph_resources(graph, catalog)
    check_graph_paths(graph, catalog)
    if graph["aggregates"] != expected_aggregates(document, catalog):
        fail("graph_aggregates")
    if graph["selectors"] != SELECTORS:
        fail("graph_selectors")
    if document.get("unstated_relations") != expected_unstated(document, catalog):
        fail("unstated_relations")


def check_resource_identity(identity, catalog):
    """Section 6.3: the pack's ResourceID is gateway-allocated per its dimension; no selector is identity evidence."""
    if identity.get("resources") != RESOURCE_ROLES or None in catalog.role_kinds().values():
        fail("identity contract")
    if identity.get("native_identity_evidence") != NATIVE_IDENTITY_EVIDENCE or {s["selector"] for s in SELECTORS} & set(identity["native_identity_evidence"]):
        fail("identity contract")


def symbol(token):
    return {"kind": "symbol", "symbol": {"namespace": "storage.status.operating", "token": token, "known": True}}


def validate_contract(document, catalog=None):
    catalog = element_catalog() if catalog is None else Catalog(catalog)
    validate_graph(document, catalog)
    check_growatt_header(document)
    check_growatt_native(document)
    check_growatt_identity(document, catalog)
    check_growatt_lifecycle(document)
    check_growatt_rules(document, catalog)
    check_growatt_policies(document)


def check_growatt_header(document):
    if document.get("contract") != "helianthus.semantic.mapping.growatt-bms-rs485-v202.storage/v1" or document.get("mapping_id") != "storage.mapping.growatt.rs485.1xsxxp.v202" or document.get("pack") != PACK or document.get("status") != "accepted_source_specific_mapping_gate" or document.get("pins") != PINS:
        fail("contract")


def check_growatt_native(document):
    native = document.get("native_contract", {})
    if native.get("revision") != REVISION or native.get("unit") != {"selection": "explicit_unicast", "allowed": "1..247", "broadcast_zero": "no_send"} or native.get("slices") != SLICES or native.get("typed_source") != "GrowattBMSTypedReadOnlyStatus" or native.get("qualification") != {"physical_qualified": False, "mapping_qualified": True, "outbound_allowed": False}:
        fail("native contract")


def check_growatt_identity(document, catalog):
    identity = document.get("identity", {})
    for key in ("asset_id", "source_id"):
        value = identity.get(key, {})
        if value.get("required") is not True or value.get("non_secret") is not True or value.get("forbidden_derivations") != ["unit_id", "vendor", "version", "company", "generation"]:
            fail("identity contract")
    if identity.get("source_id", {}).get("distinct_from") != "asset_id" or identity.get("failure") != "missing_or_invalid_asset_or_source_identity_fails_closed":
        fail("identity contract")
    if identity.get("binding_id") != {"source": "gateway_domain_separated_asset_source_profile_binding", "required": True, "non_secret": True, "stable_inputs": ["asset_id", "source_id", "profile"]}:
        fail("identity contract")
    check_resource_identity(identity, catalog)


def check_growatt_lifecycle(document):
    lifecycle = document.get("lifecycle", {})
    required = ["observation_id", "observation_revision", "receipt_wall", "receipt_monotonic", "clock_epoch", "source_epoch", "driver_generation", "transport_generation", "unit_id", "revision", "slices", "request_id", "request_adu", "response_adu", "words"]
    if lifecycle.get("required_observation_evidence") != required or lifecycle.get("qualification") != "mapping_qualified_and_physical_unqualified":
        fail("lifecycle contract")


def check_growatt_rules(document, catalog):
    rules = document.get("field_rules", [])
    fields = pack_fields()
    requested = sorted((rule.get("requested") for rule in rules), key=lambda item: item.get("item_id", ""))
    if len(rules) != 9 or document.get("projection", {}).get("requested_items") != requested:
        fail("requested items")
    for rule in rules:
        check_subject(rule, catalog)
        disposition = rule.get("disposition", {})
        losses = disposition.get("loss")
        if disposition.get("outcome") == "exact" and losses != []:
            fail("exact loss")
        if disposition.get("outcome") == "transformed" and (not losses or any(loss.get("kind") not in LOSS_KINDS or not loss.get("source_items") or not loss.get("description") or type(loss.get("reversible")) is not bool for loss in losses)):
            fail("transformed loss")
        check_conversion(rule, fields)
        check_sign(rule, catalog)


def check_growatt_policies(document):
    withheld = document.get("withheld", [])
    if len(withheld) != 7 or [(row.get("source"), row.get("waits_for")) for row in withheld] != list(WITHHELD_WAITS.items()) or any(not row.get("reason") for row in withheld) or document.get("unsupported") != UNSUPPORTED:
        fail("withheld unsupported")
    projection = document.get("projection", {})
    if projection.get("error_precedence") != ERROR_PRECEDENCE or projection.get("operations") != "unsupported_no_authority" or projection.get("missing_native") != "unavailable_withheld_never_zero":
        fail("projection")
    cutover = document.get("consumer_cutover", {})
    if cutover != {"consumers": ["semantic_mcp", "graphql", "portal", "home_assistant"], "implementation_requirement": "one_atomic_cutover", "required_removals": ["legacy_semantic_path", "fallback", "comparator", "compatibility_only_path", "dual_publication"], "until_implemented": "no_public_consumer_binding"}:
        fail("consumer cutover")


def opaque(value):
    return isinstance(value, str) and OPAQUE_RE.fullmatch(value) is not None


def case_identity(identity, source, errors):
    """Asset, source and binding identifiers, and one ResourceID per allocated role, each opaque and all distinct (RES-5, REL-8)."""
    identity = identity if isinstance(identity, dict) else {}
    asset_id, binding_id = identity.get("asset_id"), identity.get("binding_id")
    source_id = source.get("source_id") if isinstance(source, dict) else None
    resources = identity.get("resources")
    roles = [role for role, spec in RESOURCE_ROLES.items() if spec["source"] == ALLOCATED]
    resource_ids = [resources.get(role) for role in roles] if isinstance(resources, dict) and set(resources) == set(roles) else [None]
    values = [asset_id, source_id, binding_id] + resource_ids
    if not all(opaque(value) for value in values) or len(set(values)) != len(values):
        errors.add("identity_missing_or_invalid")
    return asset_id, source_id, binding_id, dict(zip(roles, resource_ids))


def validate_observation(case):
    errors = set()
    observation, lifecycle = case.get("observation"), case.get("lifecycle")
    identity, source = case.get("identity"), case.get("source")
    if not isinstance(observation, dict) or not isinstance(lifecycle, dict):
        fail("native_evidence_missing")
    check_observation_slices(observation, lifecycle, errors)
    asset_id, source_id, binding_id, pack_id = case_identity(identity, source, errors)
    check_observation_lifecycle(lifecycle, errors)
    if case.get("qualification") != {"physical_qualified": False, "mapping_qualified": True, "outbound_allowed": False}:
        errors.add("mapping_unqualified")
    if case.get("request_operation"):
        errors.add("unsupported_or_withheld")
    for code in ERROR_PRECEDENCE:
        if code in errors:
            fail(code)
    return observation, lifecycle, pack_id, source_id, binding_id


def check_observation_slices(observation, lifecycle, errors):
    slices = observation.get("slices")
    if not isinstance(slices, list) or len(slices) != 4:
        errors.add("native_evidence_missing")
        slices = []
    request_ids, units = [], []
    for index, row in enumerate(slices):
        if not isinstance(row, dict):
            errors.add("native_evidence_missing")
            continue
        request_id, unit = check_observation_slice(index, row, lifecycle, errors)
        if request_id is not None:
            request_ids.append(request_id)
        units.append(unit)
    if len(request_ids) != len(set(request_ids)) or len({repr(unit) for unit in units}) > 1:
        errors.add("native_evidence_missing")
    if observation.get("revision") != REVISION:
        errors.add("revision_or_unit_or_slice_invalid")


def check_observation_slice(index, row, lifecycle, errors):
    """Check one slice; return (valid request_id or None, unit_id)."""
    words, request_id, unit = row.get("words"), row.get("request_id"), row.get("unit_id")
    words_valid = isinstance(words, list) and all(isinstance(word, int) and not isinstance(word, bool) and 0 <= word <= 0xFFFF for word in words)
    valid_request_id = None
    if not isinstance(request_id, int) or isinstance(request_id, bool) or request_id < 1 or not words_valid:
        errors.add("native_evidence_missing")
    else:
        valid_request_id = request_id
    required = SLICES[index] if index < len(SLICES) else {}
    try:
        offset = int(str(row.get("offset")), 16)
    except (TypeError, ValueError):
        offset = -1
    if {key: row.get(key) for key in ("function", "offset", "quantity")} != required or not isinstance(unit, int) or isinstance(unit, bool) or not 1 <= unit <= 247 or row.get("transport_generation") != lifecycle.get("transport_generation"):
        errors.add("revision_or_unit_or_slice_invalid")
    elif words_valid:
        request = decode_adu(row.get("request_adu_hex"))
        response = decode_adu(row.get("response_adu_hex"))
        if len(words) != row.get("quantity") or not valid_request_adu(request, unit, offset, row["quantity"]) or not valid_response_adu(response, unit, words):
            errors.add("native_evidence_missing")
    return valid_request_id, unit


def check_observation_lifecycle(lifecycle, errors):
    opaque_lifecycle = ("observation_id", "clock_epoch", "source_epoch")
    numeric_lifecycle = ("observation_revision", "driver_generation", "transport_generation")
    if any(not isinstance(lifecycle.get(key), str) or not OPAQUE_RE.fullmatch(lifecycle[key]) for key in opaque_lifecycle) or any(not isinstance(lifecycle.get(key), int) or isinstance(lifecycle[key], bool) or lifecycle[key] < 1 for key in numeric_lifecycle) or not isinstance(lifecycle.get("receipt_monotonic"), int) or isinstance(lifecycle.get("receipt_monotonic"), bool) or lifecycle.get("receipt_monotonic", 0) < 1:
        errors.add("lifecycle_missing")
    try:
        received = datetime.fromisoformat(lifecycle.get("receipt_wall", "").replace("Z", "+00:00"))
        if received.tzinfo is None:
            raise ValueError
    except (AttributeError, ValueError):
        errors.add("lifecycle_missing")


def subject_resource(item_id, resource_ids):
    """The ResourceID of the role whose dimension is the fact's catalog subject."""
    dimension = element_catalog().subject_dimension(item_id)
    for role, spec in RESOURCE_ROLES.items():
        if spec["dimension"] == dimension:
            return resource_ids[role]
    fail("identity_missing_or_invalid")


def cell_millivolts(typed, name):
    """A typed cell extreme in millivolts. A missing or negative value is not evidence."""
    value = typed.get(name)
    if not isinstance(value, int) or isinstance(value, bool) or value < 0:
        fail("native_evidence_missing")
    return Decimal(value)


def project(case, contract):
    observation, lifecycle, resource_ids, source_id, binding_id = validate_observation(case)
    typed = observation.get("typed")
    if not isinstance(typed, dict):
        fail("native_evidence_missing")
    words = [row["words"] for row in observation["slices"]]
    native_state = {1: "soft_starting", 2: "charging", 4: "discharging", 8: "standby"}.get(words[1][6])
    raw_values = {"soc_percent": Decimal(words[1][8]), "pack_voltage_volts": Decimal(words[1][9]) / 100, "pack_current_amps": Decimal(signed16(words[1][10])) / 100, "temperature_celsius": Decimal(signed16(words[1][11])), "cumulative_charge_amp_hours": Decimal(words[2][5]) / 10, "cumulative_discharge_amp_hours": Decimal(words[2][6]) / 10, "minimum_cell_voltage_millivolts": cell_millivolts(typed, "minimum_cell_voltage_millivolts"), "maximum_cell_voltage_millivolts": cell_millivolts(typed, "maximum_cell_voltage_millivolts")}
    if native_state is None or typed.get("operating_state") != native_state:
        fail("native_evidence_missing")
    for name, raw_value in raw_values.items():
        try:
            if Decimal(str(typed.get(name))) != raw_value:
                fail("native_evidence_missing")
        except InvalidOperation:
            fail("native_evidence_missing")
    if not 0 <= raw_values["soc_percent"] <= 100 or raw_values["pack_voltage_volts"] < 0 or raw_values["cumulative_charge_amp_hours"] < 0 or raw_values["cumulative_discharge_amp_hours"] < 0:
        fail("native_evidence_missing")
    requested = contract["projection"]["requested_items"]
    rules = {rule["target"]: rule for rule in contract["field_rules"]}
    # Each quantity is published in the SI unit its rule names, by the rule's exact factor and offset; the native value and unit stay in the evidence.
    native_values = {target: quantity(convert(raw_values[rule["source"]], rule["conversion"]), rule["unit"]) for target, rule in rules.items() if "conversion" in rule}
    operating_token = {"charging": "active", "discharging": "active", "standby": "standby"}.get(native_state)
    facts, dispositions = [], []
    for requested_item in requested:
        item_id = requested_item["item_id"]
        key = fact_key(item_id, subject_resource(item_id, resource_ids))
        if item_id == "storage.status.operating" and operating_token is None:
            dispositions.append({"kind": "fact", "item_id": item_id, "outcome": "withheld", "source_keys": [], "loss": [], "reason": "storage.reason.soft_starting_not_representable"})
            continue
        value = symbol(operating_token) if item_id == "storage.status.operating" else native_values[item_id]
        facts.append({"key": key, "value": value, "source_path": {"binding_id": binding_id, "source_id": source_id, "source_epoch_id": lifecycle["source_epoch"], "driver_generation": lifecycle["driver_generation"]}, "observation_id": lifecycle["observation_id"], "observation_revision": lifecycle["observation_revision"]})
        rule = rules[item_id]
        dispositions.append({"kind": "fact", "item_id": item_id, "outcome": rule["disposition"]["outcome"], "source_keys": [key], "loss": copy.deepcopy(rule["disposition"]["loss"])})
    return {"facts": facts, "requested": copy.deepcopy(requested), "dispositions": dispositions, "withheld_native": copy.deepcopy(contract["withheld"]), "operations": []}


def document(contract):
    validate_contract(contract)
    scenarios = contract.get("scenarios", [])
    if not scenarios or scenarios[0].get("polarity") != "positive":
        fail("scenario coverage")
    baseline = scenarios[0]
    for scenario in scenarios:
        case = copy.deepcopy(baseline)
        for key in ("expect", "id", "polarity"):
            case.pop(key, None)
        for mutation in scenario.get("mutations", []):
            mutate(case, mutation)
        expected = scenario.get("expect", {})
        if "output" in expected:
            if project(case, contract) != expected["output"]:
                fail("scenario output")
            continue
        try:
            project(case, contract)
        except ValueError as error:
            if str(error) != expected.get("error"):
                raise
        else:
            fail(scenario.get("id", "scenario") + " accepted")


def main():
    contract = load()
    document(contract)
    text = DOC.read_text()
    for token in ("gateway-configured", "physical qualification", "ampere-hours", "one atomic SemReg cutover", "outbound_allowed=false", "offline synthetic", "section 15", "operator label", "unknown, not absent", "dangling endpoint", "EN-1", "never identity evidence"):
        if token not in text:
            fail("document boundary")
    outputs = sum("output" in scenario.get("expect", {}) for scenario in contract["scenarios"])
    print(f"Growatt BMS RS-485 v2.02 storage mapping: PASS; {outputs} executable outputs and {len(contract['scenarios']) - outputs} rejected scenarios")


if __name__ == "__main__":
    main()
