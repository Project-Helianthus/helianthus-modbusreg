#!/usr/bin/env python3
"""Mutation controls for the Huawei gateway-meter Infrastructure gate (run with python3 or python3 -m pytest)."""
import copy
import importlib.util
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("validate_huawei_gateway_meter_infrastructure_mapping", ROOT / "scripts/mapping/validate_huawei_gateway_meter_infrastructure_mapping.py")
v = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = v
SPEC.loader.exec_module(v)

SUPPORT_SPEC = importlib.util.spec_from_file_location("huawei_mapping_test_support", ROOT / "scripts/mapping/huawei_mapping_test_support.py")
support = importlib.util.module_from_spec(SUPPORT_SPEC)
sys.modules[SUPPORT_SPEC.name] = support
SUPPORT_SPEC.loader.exec_module(support)
parametrize, Raises = support.parametrize, support.Raises
fraction_of = support.fraction_of
# The member that stated a conversion as a decimal shift before the unit rule, assembled so that no file spells it.
EARLIER_SHIFT = "exponent10" + "_shift"
ONE, ZERO = {"coefficient": "1", "exponent10": 0}, {"coefficient": "0", "exponent10": 0}
# (native unit, canonical unit) -> exact factor and offset; the SI value is native x factor + offset.
CONVERSIONS = {
    ("kW", "unit.watt"): {"factor": {"coefficient": "1", "exponent10": 3}, "offset": ZERO},
    ("W", "unit.watt"): {"factor": ONE, "offset": ZERO},
    ("kWh", "unit.joule"): {"factor": {"coefficient": "36", "exponent10": 5}, "offset": ZERO},
    ("ratio", "unit.ratio"): {"factor": ONE, "offset": ZERO},
}


def contract():
    return copy.deepcopy(v.load())


def case():
    return copy.deepcopy(v.load()["baselines"]["default"])


def set_path(document, path, value):
    v.mutate(document, {"path": path, "value": value})


def project(document):
    return v.project(document, v.load())


def test_gate_passes_with_its_page():
    document = v.load()
    v.document(document)
    v.check_document(document, v.DOC.read_text(encoding="utf-8"))


def test_no_binary_floating_point():
    with Raises(ValueError, match="binary floating point"):
        v.loads('{"gain": 1000.0}')
    float_case = case()
    float_case["routed_unit"] = 11.0
    with Raises(ValueError):
        project(float_case)
    document = contract()
    set_path(document, "admission.decisions.0.grade", 0.5)
    with Raises(ValueError):
        v.validate_contract(document)


@parametrize("name,mutation,message", [
    ("pack", lambda d: set_path(d, "pack.id", "helianthus.pack.pv"), "contract"),
    ("pack-version", lambda d: set_path(d, "pack.version", "1.1.0"), "contract"),
    ("status", lambda d: set_path(d, "status", "accepted_source_specific_read_only_mapping_gate"), "contract"),
    ("evidence-amendment", lambda d: set_path(d, "evidence.meter_observation_amendment.issue", 255), "evidence"),
    ("evidence-page", lambda d: d["evidence"]["monitoring_contract"]["pages"].pop(), "evidence"),
    ("class-contract-claimed", lambda d: set_path(d, "admission.meter_class_contract", True), "admission"),
    ("exposure-not-from-the-documented-exposures", lambda d: set_path(d, "admission.meter_exposures.declared_from", "operator_label"), "admission"),
    ("plant-group-admitted-as-a-meter-exposure", lambda d: d["admission"]["meter_exposures"]["paths"].append({"path": "sdongle_plant_group", "routed_unit": 100, "listed_as_child": False, "grade": "proven"}), "admission"),
    ("listed-meter-exposure-dropped", lambda d: v.mutate(d, {"path": "admission.meter_exposures.paths.0", "remove": True}), "admission"),
    ("inverter-side-exposure-listed-as-a-child", lambda d: set_path(d, "admission.meter_exposures.paths.1.listed_as_child", True), "admission"),
    ("inverter-side-exposure-at-the-plant-unit", lambda d: set_path(d, "admission.meter_exposures.paths.1.routed_unit", 100), "admission"),
    ("plant-unit-moved", lambda d: set_path(d, "admission.plant_grid_exchange.routed_unit", 11), "admission"),
    ("plant-read-establishes-a-meter", lambda d: set_path(d, "admission.plant_grid_exchange.meter_identity", "the_meter_of_the_route"), "admission"),
    ("plant-claimed-as-the-same-meter", lambda d: set_path(d, "admission.plant_grid_exchange.same_meter_as_inverter_side", "yes"), "admission"),
    ("identity-mismatch-replaces", lambda d: set_path(d, "admission.meter_exposures.on_mismatch", "replace_the_declared_meter"), "admission"),
    ("identity-check-dropped", lambda d: v.mutate(d, {"path": "admission.meter_exposures.runtime_check", "remove": True}), "admission"),
    ("meter-identity-declared-from-the-listing", lambda d: set_path(d, "admission.meter_identity", {"declared_from": "documented_gateway_listing", "exposure": {"path": "sdongle_plant_group", "routed_unit": 100}}), "admission"),
    ("meter-identity-status-declared", lambda d: set_path(d, "admission.meter_identity.status", "declared_from_listing"), "admission"),
    ("meter-identity-decision-graded-hypothesis", lambda d: set_path(d, "admission.decisions.0.grade", "hypothesis"), "admission"),
    ("registry-profile-admitted", lambda d: d["admission"]["selection"]["refused"].clear(), "admission"),
    ("unresolved-publishes", lambda d: set_path(d, "admission.selection.unresolved_outcome", "published_as_zero"), "admission"),
    ("sign-reference-graded-proven", lambda d: set_path(d, "admission.decisions.3.grade", "proven"), "admission"),
    ("precondition-dropped", lambda d: v.mutate(d, {"path": "admission.decisions.3", "remove": True}), "admission"),
    ("status-withheld", lambda d: set_path(d, "status", "accepted_withheld_mapping_gate_no_meter_decoded"), "contract"),
    ("active-row-withheld", lambda d: set_path(d, "field_rules.5.disposition", {"outcome": "withheld", "reason": "infrastructure.reason.no_meter_class_contract"}), "active disposition"),
    ("active-row-word-moved", lambda d: set_path(d, "field_rules.5.word", 0), "active disposition"),
    ("active-row-read-moved", lambda d: set_path(d, "field_rules.5.read.offset", 37502), "active disposition"),
    ("other-row-given-a-read", lambda d: set_path(d, "field_rules.0.read", {"offset": 32260, "quantity": 33}), "active disposition"),
    ("active-prospective-not-in-force", lambda d: set_path(d, "field_rules.5.prospective.0.in_force", False), "prospective target"),
    ("other-prospective-in-force", lambda d: set_path(d, "field_rules.0.prospective.0.in_force", True), "prospective target"),
    ("role-derived", lambda d: d["role"]["never_derived_from"].remove("sign_or_magnitude_of_a_power_value"), "role"),
    ("role-default", lambda d: set_path(d, "role.undeclared", "assume_grid_connection"), "role"),
    ("role-value-added", lambda d: d["role"]["values"].append("consumption"), "role"),
    ("role-feeder-relation", lambda d: set_path(d, "role.relation_by_role.feeder", "infrastructure.relation.grid_connection_meter"), "role"),
    ("role-grid-relation", lambda d: set_path(d, "role.relation_by_role.grid_connection", "infrastructure.relation.site_grid_connection"), "role"),
    ("native-item-dropped", lambda d: v.mutate(d, {"path": "field_rules.3", "remove": True}), "native inventory"),
    ("native-item-duplicated", lambda d: d["field_rules"].append(copy.deepcopy(d["field_rules"][0])), "native inventory"),
    ("native-item-added", lambda d: d["field_rules"].append({**copy.deepcopy(d["field_rules"][0]), "native_id": "meter_voltage_group"}), "native inventory"),
    ("native-item-path", lambda d: set_path(d, "field_rules.0.gateway_path", "emma"), "native inventory"),
    ("outcome-mapped", lambda d: set_path(d, "field_rules.0.disposition.outcome", "exact"), "withheld reason"),
    ("withheld-without-reason", lambda d: v.mutate(d, {"path": "field_rules.0.disposition.reason", "remove": True}), "withheld reason"),
    ("withheld-unknown-reason", lambda d: set_path(d, "field_rules.0.disposition.reason", "infrastructure.reason.invented"), "withheld reason"),
    ("grade", lambda d: set_path(d, "field_rules.0.grade", "certain"), "withheld reason"),
    ("prospective-in-force", lambda d: set_path(d, "field_rules.0.prospective.0.in_force", True), "prospective target"),
    ("prospective-target-not-in-pack", lambda d: set_path(d, "field_rules.0.prospective.0.target", "infrastructure.power.net_active"), "prospective target"),
    ("prospective-unit", lambda d: set_path(d, "field_rules.0.prospective.0.unit", "unit.kilowatt_hour"), "prospective target"),
    ("prospective-dimension", lambda d: set_path(d, "field_rules.0.prospective.0.dimension", "infrastructure.dimension.meter"), "prospective target"),
    ("prospective-native-unit", lambda d: set_path(d, "field_rules.0.prospective.0.native.unit", "MW"), "prospective conversion"),
    ("prospective-role", lambda d: v.mutate(d, {"path": "field_rules.0.prospective.0.role", "remove": True}), "prospective role"),
    ("prospective-role-feeder", lambda d: set_path(d, "field_rules.0.prospective.0.role", "feeder"), "prospective role"),
    ("prospective-sign-assumed", lambda d: set_path(d, "field_rules.0.prospective.0.sign_reference", "positive_export_assumed"), "prospective sign"),
    ("prospective-sign-removed", lambda d: v.mutate(d, {"path": "field_rules.5.prospective.0.sign_reference", "remove": True}), "prospective sign"),
    ("prospective-direction", lambda d: set_path(d, "field_rules.1.prospective.0.direction", "guessed"), "prospective counter"),
    ("prospective-counter", lambda d: set_path(d, "field_rules.1.prospective.0.counter", "delta_published"), "prospective counter"),
    ("requested-added", lambda d: d["projection"]["requested_items"].append({"kind": "fact", "item_id": "infrastructure.status.breaker"}), "requested items"),
    ("requested-dropped", lambda d: v.mutate(d, {"path": "projection.requested_items.0", "remove": True}), "requested items"),
    ("reason-unused", lambda d: d["reasons"].update({"infrastructure.reason.unused": {"loss_kind": "policy", "description": "x", "reversible": True}}), "reason inventory"),
    ("reason-primary-removed", lambda d: d["reasons"].pop("infrastructure.reason.no_meter_class_contract"), "withheld reason"),
    ("identity-asset-allowed", lambda d: set_path(d, "identity.meter_asset_id", "meter-asset-demo"), "identity contract"),
    ("identity-forbidden", lambda d: set_path(d, "identity.source_id.forbidden_derivations.0", "vendor"), "identity contract"),
    ("lifecycle-zero", lambda d: set_path(d, "lifecycle.absent_optional_group", "zero"), "lifecycle contract"),
    ("lifecycle-negation", lambda d: set_path(d, "lifecycle.exception", "negates_the_gateway"), "lifecycle contract"),
    ("outcome-vocabulary", lambda d: set_path(d, "outcome_vocabulary.native_only", "exact"), "outcome vocabulary"),
    ("unsupported-dropped", lambda d: v.mutate(d, {"path": "unsupported.1", "remove": True}), "unsupported"),
    ("primary-reason", lambda d: set_path(d, "projection.primary_reason", "infrastructure.reason.answered_not_decoded"), "projection"),
    ("precedence", lambda d: set_path(d, "projection.error_precedence.3", "lifecycle_missing"), "projection"),
    ("operations", lambda d: set_path(d, "projection.operations", "allowed"), "projection"),
    ("consumer", lambda d: set_path(d, "consumer_cutover.until_implemented", "public_binding"), "consumer cutover"),
])
def test_contract_mutations_are_rejected(name, mutation, message):
    document = contract()
    mutation(document)
    with Raises(ValueError) as error:
        v.validate_contract(document)
    assert message in str(error.value)


@parametrize("field,value", [("loss_kind", "native_enum"), ("description", ""), ("reversible", "yes")])
def test_every_reason_declares_its_loss(field, value):
    document = contract()
    document["reasons"]["infrastructure.reason.profile_withholds_meter"][field] = value
    with Raises(ValueError, match="reason loss"):
        v.validate_contract(document)


def test_every_native_item_has_exactly_one_disposition_and_only_the_active_row_publishes():
    document = v.load()
    assert [rule["native_id"] for rule in document["field_rules"]] == [nid for nid, _ in v.INVENTORY]
    for scenario in document["scenarios"]:
        if scenario["polarity"] != "positive":
            continue
        output = v.run_scenario(scenario, document)
        assert [row["native_id"] for row in output["native_dispositions"]] == [nid for nid, _ in v.INVENTORY]
        assert {row["outcome"] for row in output["native_dispositions"]} <= {"withheld", "transformed"}
        assert {fact["provenance"]["native_id"] for fact in output["facts"]} <= set(v.ACTIVE)
        assert output["relations"] == [] and output["operations"] == [] and len(output["dispositions"]) == 8
        for item in output["dispositions"]:
            assert item["loss"] and (item["source_keys"] if item["outcome"] == "transformed" else item["reason"] and not item["source_keys"])
        assert [item["item_id"] for item in output["dispositions"] if item["outcome"] == "transformed"] == sorted(fact["item_id"] for fact in output["facts"])


def test_completeness_check_catches_damage():
    document = v.load()
    output = v.run_scenario(next(scenario for scenario in document["scenarios"] if scenario["id"] == "MTR-POS-002-plant-grid-import"), document)
    for damage in ("drop", "duplicate", "fact", "outcome", "reason"):
        broken = copy.deepcopy(output)
        if damage == "drop":
            del broken["native_dispositions"][-1]
        elif damage == "duplicate":
            broken["native_dispositions"].append(copy.deepcopy(broken["native_dispositions"][-1]))
        elif damage == "fact":
            del broken["facts"][0]
        elif damage == "outcome":
            broken["native_dispositions"][0]["outcome"] = "exact"
        else:
            broken["native_dispositions"][0]["reason"] = "infrastructure.reason.invented"
        with Raises(ValueError):
            v.assert_complete(broken, document)


def test_an_answering_group_is_neither_a_value_nor_an_identity():
    document = case()
    document["reads"] = [{"native_id": native_id, "outcome": "ANSWERED"} for native_id in v.FEEDABLE if native_id not in v.ACTIVE]
    output = project(document)
    assert output["facts"] == []
    rows = {row["native_id"]: row["reason"] for row in output["native_dispositions"]}
    assert all(rows[native_id] == "infrastructure.reason.answered_not_decoded" for native_id in v.FEEDABLE if native_id not in v.ACTIVE)
    assert rows["sdongle_plant_grid_power"] == "infrastructure.reason.group_not_read"
    assert rows["sun2000_inverter_exposed_meter_group_refused"] == "infrastructure.reason.refused_on_family"
    assert rows["profile_meter_groups"] == "infrastructure.reason.profile_withholds_meter"


def plant_read(raw):
    words = [0] * 8
    words[4], words[5] = (raw & 0xFFFFFFFF) >> 16, raw & 0xFFFF
    return {"native_id": "sdongle_plant_grid_power", "outcome": "ANSWERED", "words": words}


def with_plant(raw, **members):
    document = case()
    document["reads"] = [plant_read(raw)]
    for path, value in members.items():
        set_path(document, path.replace("__", "."), value)
    return document


def watts(fact):
    return fraction_of(fact["value"]["number"])


@parametrize("raw,item,expected,other", [(1500, "infrastructure.power.import_active", 1500, "infrastructure.power.export_active"), (-2000, "infrastructure.power.export_active", 2000, "infrastructure.power.import_active"), (0, "infrastructure.power.import_active", 0, "infrastructure.power.export_active")])
def test_a_documented_plant_grid_power_publishes_the_stated_direction_in_watts(raw, item, expected, other):
    output = project(with_plant(raw))
    assert [fact["item_id"] for fact in output["facts"]] == [item]
    fact = output["facts"][0]
    assert watts(fact) == expected and fact["value"]["unit"] == "unit.watt"
    assert fact["subject"] == {"dimension": "infrastructure.dimension.grid_connection", "resource": "resource:demo-grid-delta"}
    assert fact["provenance"]["sign_reference"] == "positive_import" and fact["provenance"]["plant_route"] == {"exposure": "sdongle_plant_group", "routed_unit": 100} and "meter" not in fact["provenance"] and "exposure" not in fact["provenance"]
    held = {row["item_id"]: row for row in output["dispositions"]}
    assert held[other]["outcome"] == "withheld" and held[other]["reason"] == "infrastructure.reason.opposite_direction"


SMARTLOGGER, INVERTER_SIDE = "smartlogger_listed_meter", "sdongle_inverter_side"
EMMA_GATEWAY = "emma_gateway_level"
SMARTLOGGER_GROUP = "smartlogger_meter_electrical_group"


def route(exposure, unit):
    return {"exposure": exposure, "routed_unit": unit}


def with_meter_group(native_id, unit, declared, identity_read):
    document = case()
    document["routed_unit"] = unit
    document["meter"]["declared"] = declared
    document["meter"]["identity_read"] = identity_read
    document["reads"] = [{"native_id": native_id, "outcome": "ANSWERED"}]
    return document


def row_reasons(output):
    return {row["native_id"]: row["reason"] for row in output["native_dispositions"] if "reason" in row}


@parametrize("native_id,exposure,unit", [(SMARTLOGGER_GROUP, SMARTLOGGER, 11), (SMARTLOGGER_GROUP, SMARTLOGGER, 19), ("smartlogger_meter_energy_totals", SMARTLOGGER, 11), ("sdongle_inverter_exposed_meter_group", INVERTER_SIDE, 1)])
def test_a_documented_meter_exposure_whose_runtime_route_agrees_is_not_an_identity_mismatch(native_id, exposure, unit):
    output = project(with_meter_group(native_id, unit, route(exposure, unit), route(exposure, unit)))
    assert output["facts"] == [] and row_reasons(output)[native_id] == "infrastructure.reason.answered_not_decoded"
    assert "infrastructure.reason.identity_mismatch" not in row_reasons(output).values()


@parametrize("declared,identity_read,unit", [
    (route(SMARTLOGGER, 11), route(SMARTLOGGER, 19), 11),
    (route(SMARTLOGGER, 11), route(INVERTER_SIDE, 11), 11),
    (route(SMARTLOGGER, 11), route(SMARTLOGGER, 11), 19),
    (route(SMARTLOGGER, 11), route(SMARTLOGGER, 11), None),
    (route(INVERTER_SIDE, 1), route(INVERTER_SIDE, 1), 1),
])
def test_a_route_read_that_differs_from_its_own_exposure_is_reported_and_replaces_nothing(declared, identity_read, unit):
    document = with_meter_group(SMARTLOGGER_GROUP, unit, declared, identity_read)
    before = copy.deepcopy(document["meter"]["declared"])
    output = project(document)
    assert output["facts"] == [] and row_reasons(output)[SMARTLOGGER_GROUP] == "infrastructure.reason.identity_mismatch"
    assert document["meter"]["declared"] == before


@parametrize("unit,exposure_read", [(19, SMARTLOGGER), (None, SMARTLOGGER), (11, INVERTER_SIDE)])
def test_an_exception_on_the_wrong_route_still_reports_the_identity_mismatch(unit, exposure_read):
    document = with_meter_group(SMARTLOGGER_GROUP, unit, route(exposure_read, 11), route(SMARTLOGGER, 11))
    document["reads"] = [{"native_id": SMARTLOGGER_GROUP, "outcome": "EXCEPTION", "exception_code": 3}]
    output = project(document)
    assert output["facts"] == [] and row_reasons(output)[SMARTLOGGER_GROUP] == "infrastructure.reason.identity_mismatch"


def test_an_exception_on_the_exposures_own_route_stays_a_read_exception():
    document = with_meter_group(SMARTLOGGER_GROUP, 11, route(SMARTLOGGER, 11), route(SMARTLOGGER, 11))
    document["reads"] = [{"native_id": SMARTLOGGER_GROUP, "outcome": "EXCEPTION", "exception_code": 3}]
    assert row_reasons(project(document))[SMARTLOGGER_GROUP] == "infrastructure.reason.read_exception_no_negation"


def test_a_read_exception_does_not_erase_a_runtime_route_that_differs_from_the_declaration():
    document = with_meter_group(SMARTLOGGER_GROUP, 11, route(SMARTLOGGER, 11), route(SMARTLOGGER, 19))
    document["reads"] = [{"native_id": SMARTLOGGER_GROUP, "outcome": "EXCEPTION", "exception_code": 3}]
    output = project(document)
    assert output["facts"] == [] and row_reasons(output)[SMARTLOGGER_GROUP] == "infrastructure.reason.identity_mismatch"


@parametrize("outcome", ["FAILED", "NOT_READ", None])
def test_a_runtime_route_that_differs_is_reported_whether_or_not_the_group_answered(outcome):
    document = with_meter_group(SMARTLOGGER_GROUP, 11, route(SMARTLOGGER, 11), route(SMARTLOGGER, 19))
    document["reads"] = [] if outcome is None else [{"native_id": SMARTLOGGER_GROUP, "outcome": outcome}]
    output = project(document)
    assert output["facts"] == [] and row_reasons(output)[SMARTLOGGER_GROUP] == "infrastructure.reason.identity_mismatch"
    assert row_reasons(output)["sdongle_inverter_exposed_meter_group"] != "infrastructure.reason.identity_mismatch"


def test_an_unread_group_with_a_matching_runtime_route_keeps_its_own_reason():
    document = with_meter_group(SMARTLOGGER_GROUP, 11, route(SMARTLOGGER, 11), route(SMARTLOGGER, 11))
    document["reads"] = []
    assert "infrastructure.reason.identity_mismatch" not in row_reasons(project(document)).values()


@parametrize("declared,identity_read", [
    (route(SMARTLOGGER, 11), route(SMARTLOGGER, 100)),
    (route(INVERTER_SIDE, 1), route(INVERTER_SIDE, 100)),
])
def test_a_runtime_meter_route_at_unit_100_is_no_plant_route_evidence(declared, identity_read):
    document = with_plant(1500)
    document["meter"]["declared"], document["meter"]["identity_read"] = declared, identity_read
    document["reads"].append({"native_id": SMARTLOGGER_GROUP, "outcome": "ANSWERED"})
    output = project(document)
    assert output["facts"] == [] and row_reasons(output)["sdongle_plant_grid_power"] == "infrastructure.reason.group_not_read"
    assert row_reasons(output)[SMARTLOGGER_GROUP] == "infrastructure.reason.identity_mismatch"


def test_a_plant_read_publishes_beside_a_meter_route_read_at_another_unit():
    document = with_plant(1500)
    document["meter"]["declared"] = document["meter"]["identity_read"] = route(SMARTLOGGER, 11)
    assert [fact["item_id"] for fact in project(document)["facts"]] == ["infrastructure.power.import_active"]


def test_a_plant_read_at_unit_100_with_gateway_level_route_evidence_publishes_no_plant_fact():
    document = with_plant(1500)
    document["meter"]["identity_read"] = route(EMMA_GATEWAY, 0)
    output = project(document)
    assert output["facts"] == [] and row_reasons(output)["sdongle_plant_grid_power"] == "infrastructure.reason.group_not_read"


def test_the_emma_gateway_level_route_at_unit_zero_is_a_known_route_and_no_configured_meter_exposure():
    known = with_meter_group("emma_built_in_sensor_group", 0, None, route(EMMA_GATEWAY, 0))
    output = project(known)
    assert output["facts"] == [] and row_reasons(output)["emma_built_in_sensor_group"] == "infrastructure.reason.answered_not_decoded"
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(with_meter_group("emma_built_in_sensor_group", 0, route(EMMA_GATEWAY, 0), route(EMMA_GATEWAY, 0)))
    assert v.load()["admission"]["meter_identity"]["status"] == "unknown"


def test_the_gateway_plant_unit_cannot_satisfy_the_inverter_side_meter_exposure():
    output = project(with_meter_group("sdongle_inverter_exposed_meter_group", 100, route(INVERTER_SIDE, 100), route(INVERTER_SIDE, 100)))
    assert output["facts"] == [] and row_reasons(output)["sdongle_inverter_exposed_meter_group"] == "infrastructure.reason.identity_mismatch"


def test_unit_zero_is_no_route_for_a_routed_meter_exposure():
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(with_meter_group(SMARTLOGGER_GROUP, 0, route(SMARTLOGGER, 0), route(SMARTLOGGER, 0)))


def test_a_route_that_was_not_read_confirms_and_replaces_nothing():
    output = project(with_meter_group(SMARTLOGGER_GROUP, 11, route(SMARTLOGGER, 11), None))
    assert output["facts"] == [] and row_reasons(output)[SMARTLOGGER_GROUP] == "infrastructure.reason.identity_not_read"


def test_a_group_read_with_no_configured_meter_exposure_is_checked_against_none():
    output = project(with_meter_group(SMARTLOGGER_GROUP, 11, None, None))
    assert row_reasons(output)[SMARTLOGGER_GROUP] == "infrastructure.reason.answered_not_decoded"


def test_a_plant_grid_reading_publishes_without_any_meter_exposure_and_names_the_plant_route_only():
    document = with_plant(1500)
    assert document["meter"]["declared"] is None and document["meter"]["identity_read"] is None
    output = project(document)
    assert [fact["item_id"] for fact in output["facts"]] == ["infrastructure.power.import_active"]
    assert output["facts"][0]["provenance"]["plant_route"] == {"exposure": "sdongle_plant_group", "routed_unit": 100}
    assert v.load()["admission"]["plant_grid_exchange"]["meter_identity"] == "not_established_by_the_plant_read"


@parametrize("declared", [route("sdongle_plant_group", 100), route("emma_gateway", 100), route("", 100), route(SMARTLOGGER, 0), route(SMARTLOGGER, 248), {"exposure": SMARTLOGGER}])
def test_only_a_documented_meter_exposure_is_a_meter_declaration(declared):
    document = with_plant(1500)
    document["meter"]["declared"] = copy.deepcopy(declared)
    document["meter"]["identity_read"] = copy.deepcopy(declared)
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)
    document["meter"]["declared"] = None
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


@parametrize("unit", [None, 1, 11, 101])
def test_a_plant_group_read_from_a_routed_unit_other_than_the_plant_unit_publishes_nothing(unit):
    for declared in (None, route(SMARTLOGGER, 11)):
        document = with_plant(1500)
        document["routed_unit"] = unit
        document["meter"]["declared"] = declared
        document["meter"]["identity_read"] = copy.deepcopy(declared)
        output = project(document)
        assert output["facts"] == []
        assert row_reasons(output)["sdongle_plant_grid_power"] == "infrastructure.reason.group_not_read"


def test_a_meter_route_at_the_plant_unit_is_no_evidence_of_the_plant_route():
    for identity_read in (route(SMARTLOGGER, 100), route(SMARTLOGGER, 19), None):
        document = with_plant(1500)
        document["meter"]["declared"] = route(SMARTLOGGER, 100)
        document["meter"]["identity_read"] = copy.deepcopy(identity_read)
        output = project(document)
        assert output["facts"] == []
        assert row_reasons(output)["sdongle_plant_grid_power"] == "infrastructure.reason.group_not_read"


def test_a_configured_meter_at_another_unit_neither_withholds_nor_publishes_the_plant_group():
    for identity_read in (route(SMARTLOGGER, 11), route(SMARTLOGGER, 19)):
        document = with_plant(1500)
        document["meter"]["declared"] = route(SMARTLOGGER, 11)
        document["meter"]["identity_read"] = copy.deepcopy(identity_read)
        output = project(document)
        assert [fact["item_id"] for fact in output["facts"]] == ["infrastructure.power.import_active"]


@parametrize("outcome", ["INSUFFICIENT_EVIDENCE", "NO_ADMISSIBLE_PROFILE", "AMBIGUOUS"])
def test_an_unresolved_selection_withholds_a_documented_reading(outcome):
    output = project(with_plant(1500, selection__outcome=outcome))
    assert output["facts"] == [] and {row["native_id"]: row["reason"] for row in output["native_dispositions"]}["sdongle_plant_grid_power"] == v.PRIMARY_REASON


@parametrize("role", [None, "feeder", "circuit"])
def test_a_role_other_than_the_declared_grid_connection_publishes_no_grid_fact(role):
    output = project(with_plant(1500, role={"declared": role, "source": "operator_configuration"}))
    assert output["facts"] == []
    assert {row["native_id"]: row["reason"] for row in output["native_dispositions"]}["sdongle_plant_grid_power"] == "infrastructure.reason.role_not_declared"


def test_a_power_outside_the_pack_bound_is_withheld_never_clamped():
    output = project(with_plant(20000000))
    assert output["facts"] == []
    assert {row["native_id"]: row["reason"] for row in output["native_dispositions"]}["sdongle_plant_grid_power"] == "infrastructure.reason.value_outside_pack_range"


@parametrize("meter", [None, {"declared": None, "identity_read": route(SMARTLOGGER, 11), "resources": {"grid_connection": "resource:demo-grid-delta"}}, {"declared": None, "identity_read": None, "resources": {"grid_connection": ""}},
                       {"declared": None, "identity_read": None, "resources": {"grid_connection": "source:gateway-demo-d"}},
                       {"declared": route(SMARTLOGGER, 11), "identity_read": {"exposure": SMARTLOGGER}, "resources": {"grid_connection": "resource:demo-grid-delta"}}])
def test_the_meter_route_shape_and_its_grid_resource_are_required(meter):
    document = with_plant(1500)
    document["meter"] = meter
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


def test_a_group_without_a_documented_word_position_publishes_nothing_even_when_it_answers():
    document = case()
    document["reads"] = [{"native_id": "smartlogger_meter_electrical_group", "outcome": "ANSWERED"}]
    assert project(document)["facts"] == []


def test_the_sign_reference_is_graded_hypothesis_and_the_meter_identity_stays_unknown_while_the_exposure_is_declared():
    admission = v.load()["admission"]
    decisions = {item["precondition"]: item for item in admission["decisions"]}
    assert decisions["sign_reference"]["grade"] == "hypothesis"
    assert decisions["meter_identity"]["grade"] == "unknown" and decisions["exposure_path"]["grade"] == "proven" and decisions["plant_grid_exchange"]["grade"] == "proven"
    assert admission["meter_identity"]["status"] == "unknown" and admission["meter_identity"]["published_as"] == "no_meter_attribution"
    assert admission["meter_exposures"]["declared_from"] == "documented_meter_exposures" and admission["meter_exposures"]["on_mismatch"] == "reported_never_guessed_and_never_a_replacement"
    assert [item["path"] for item in admission["meter_exposures"]["paths"]] == [SMARTLOGGER, INVERTER_SIDE] and [item["path"] for item in admission["gateway_sensor_routes"]["routes"]] == [EMMA_GATEWAY] and admission["plant_grid_exchange"]["path"] not in {item["path"] for item in admission["meter_exposures"]["paths"]}
    assert admission["selection"]["refused"] == ["PROFILE_SELECTED"]


@parametrize("read", [
    {"native_id": "sdongle_plant_grid_power", "outcome": "ANSWERED", "words": [0, 1200]},
    {"native_id": "sdongle_plant_grid_power", "outcome": "ANSWERED", "value": 1200},
    {"native_id": "sdongle_plant_grid_power", "outcome": "EXCEPTION"},
    {"native_id": "sdongle_plant_grid_power", "outcome": "ANSWERED", "exception_code": 3},
    {"native_id": "sdongle_plant_grid_power", "outcome": "TIMEOUT"},
    {"native_id": "sun2000_inverter_exposed_meter_group_refused", "outcome": "ANSWERED"},
    {"native_id": "profile_meter_groups", "outcome": "ANSWERED"},
    {"native_id": "invented_meter_group", "outcome": "ANSWERED"},
    {"outcome": "ANSWERED"},
    None,
])
def test_invalid_reads_are_rejected(read):
    document = case()
    document["reads"] = [read]
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


def test_a_duplicate_read_is_rejected():
    document = case()
    read = {"native_id": "emma_built_in_sensor_group", "outcome": "FAILED"}
    document["reads"] = [read, copy.deepcopy(read)]
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


def test_an_exception_negates_nothing_and_publishes_no_value():
    document = case()
    document["reads"] = [{"native_id": "emma_built_in_sensor_group", "outcome": "EXCEPTION", "exception_code": 3}, {"native_id": "emma_external_sensor_group", "outcome": "NOT_READ"}]
    rows = {row["native_id"]: row["reason"] for row in project(document)["native_dispositions"]}
    assert rows["emma_built_in_sensor_group"] == "infrastructure.reason.read_exception_no_negation"
    assert rows["emma_external_sensor_group"] == "infrastructure.reason.no_meter_class_contract"


@parametrize("declared,expected", [
    ("grid_connection", {"power": "infrastructure.dimension.grid_connection", "energy": "infrastructure.dimension.meter", "relation": "infrastructure.relation.grid_connection_meter"}),
    ("feeder", {"power": "withheld", "energy": "infrastructure.dimension.meter", "relation": "unknown"}),
    ("circuit", {"power": "withheld", "energy": "infrastructure.dimension.meter", "relation": "unknown"}),
    (None, "role_unknown_role_dependent_facts_not_published"),
])
def test_the_configured_role_selects_the_dimension_and_publishes_nothing(declared, expected):
    document = case()
    document["role"]["declared"] = declared
    output = project(document)
    assert output["role"]["dimension_if_admitted"] == expected and output["role"]["declared"] == declared
    assert output["role"]["keyed_to"] == {"source_id": "source:gateway-demo-d", "routed_unit": 100}
    assert output["facts"] == []


@parametrize("role", [
    {"declared": "grid_connection", "source": "derived_from_power_sign"}, {"declared": "grid_connection", "source": "derived_from_registers"},
    {"declared": "grid_connection", "source": "listing_label"}, {"declared": "grid_connection"}, {"declared": "consumption", "source": "operator_configuration"},
    {"declared": "", "source": "operator_configuration"}, {"declared": ["grid_connection"], "source": "operator_configuration"}, None, "grid_connection",
])
def test_a_role_is_configuration_and_never_derived(role):
    document = case()
    document["role"] = role
    with Raises(ValueError, match="role_invalid"):
        project(document)


@parametrize("unit", [0, 248, -1, True, "11", 11.5])
def test_the_routed_unit_that_keys_the_role_is_bounded(unit):
    document = case()
    document["routed_unit"] = unit
    with Raises(ValueError):
        project(document)


@parametrize("outcome,message", [("PROFILE_SELECTED", "gate_revision_required"), ("INSUFFICIENT_EVIDENCE", None), ("NO_ADMISSIBLE_PROFILE", None), ("AMBIGUOUS", None), ("", "native_evidence_invalid"), (None, "native_evidence_invalid")])
def test_a_selected_meter_fails_closed(outcome, message):
    document = case()
    document["selection"]["outcome"] = outcome
    if message is None:
        project(document)
        return
    with Raises(ValueError, match=message):
        project(document)


def test_a_meter_identity_is_never_claimed():
    for identity in ({"asset_id": "meter-asset-demo-delta"}, {"asset_id": ""}, {"asset_id": None}):
        document = case()
        document["identity"] = identity
        with Raises(ValueError, match="gate_revision_required"):
            project(document)


@parametrize("source", ["", None, "source gateway", "source:serial-1", "source:ESN-1", "source:10.1.2.3", "source:host:502", "100", "source:meter-huawei", "source:emma-a"])
def test_source_identifier_never_comes_from_forbidden_inputs(source):
    document = case()
    document["source"]["source_id"] = source
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


@parametrize("path,value", [("lifecycle.observation_id", ""), ("lifecycle.observation_revision", 0), ("lifecycle.receipt_wall", "2026-01-01T00:00:00"), ("lifecycle.receipt_monotonic", None), ("lifecycle.clock_epoch", "bad epoch"), ("lifecycle.source_epoch", None), ("lifecycle.driver_generation", True), ("lifecycle.connection_generation", 0)])
def test_every_lifecycle_axis_is_required(path, value):
    document = case()
    set_path(document, path, value)
    with Raises(ValueError, match="lifecycle_missing"):
        project(document)


@parametrize("flag", sorted(k for k in v.REGISTRY_FLAGS if k != "live_qualified") + ["mapping_qualified", "outbound_allowed"])
def test_every_qualification_flag_is_pinned(flag):
    document = case()
    document["qualification"][flag] = not document["qualification"][flag]
    with Raises(ValueError, match="mapping_unqualified"):
        project(document)


def test_live_qualification_is_a_recorded_level_not_a_rejection():
    """ND02 decision D14: a record that acquires a live qualification level is still the same accepted mapping."""
    document = case()
    document["qualification"]["live_qualified"] = True
    project(document)


@parametrize("member,value", [("request_operation", "any"), ("request_direction_split", "signed_power_to_import_export"), ("request_mixed_sign_references", True)])
def test_no_operation_split_or_mixed_sign_is_available(member, value):
    document = case()
    document[member] = value
    with Raises(ValueError, match="unsupported_or_withheld"):
        project(document)


def test_first_error_of_the_precedence_list_wins():
    document = case()
    document["selection"]["outcome"] = "PROFILE_SELECTED"
    document["source"]["source_id"] = ""
    document["role"] = {"declared": "x", "source": "y"}
    document["request_operation"] = "any"
    with Raises(ValueError, match="gate_revision_required"):
        project(document)
    document["selection"]["outcome"] = "AMBIGUOUS"
    document["reads"] = [{"native_id": "invented", "outcome": "ANSWERED"}]
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)
    document["reads"] = []
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)
    document["source"]["source_id"] = "source:gateway-demo-d"
    with Raises(ValueError, match="role_invalid"):
        project(document)


def test_only_the_active_row_is_in_force_and_every_row_matches_the_pack():
    fields, dimensions, relations = v.pack_catalog()
    document = v.load()
    for rule in document["field_rules"]:
        for row in rule.get("prospective", []):
            assert row["in_force"] is (rule["native_id"] in v.ACTIVE)
            assert row["dimension"] == fields[row["target"]]["dimension"] and row["unit"] == fields[row["target"]]["unit"]
            if row["dimension"] == "infrastructure.dimension.grid_connection":
                assert row["role"] == "grid_connection"
    refs = {rule["native_id"]: {row["sign_reference"] for row in rule.get("prospective", []) if "sign_reference" in row} for rule in document["field_rules"]}
    assert refs["sdongle_plant_grid_power"] == {"positive_import"} and refs["sdongle_inverter_exposed_meter_group"] == {"positive_export"} and refs["smartlogger_meter_electrical_group"] == {"not_stated"}
    assert not any(token in str(document["field_rules"]) for token in ("infrastructure.status.", "operation"))


def test_the_conversion_table_states_an_exact_factor_and_offset():
    assert v.UNIT_CONVERSION == CONVERSIONS
    assert not hasattr(v, "UNIT_SHIFT")
    assert fraction_of(CONVERSIONS[("kWh", "unit.joule")]["factor"]) == 3600000
    assert fraction_of(CONVERSIONS[("kW", "unit.watt")]["factor"]) == 1000


def test_each_prospective_row_states_its_conversion_and_losses():
    energy_rows = 0
    for rule in v.load()["field_rules"]:
        for row in rule.get("prospective", []):
            assert EARLIER_SHIFT not in row
            assert row["conversion"] == CONVERSIONS[(row["native"]["unit"], row["unit"])]
            identity = row["conversion"] == {"factor": ONE, "offset": ZERO}
            assert ("unit" in row["loss"]) == (not identity), row["target"]
            if row["target"].startswith("infrastructure.energy."):
                energy_rows += 1
                assert row["unit"] == "unit.joule" and row["loss"] == ["unit", "policy"]
    assert energy_rows == 4


@parametrize("name,mutation,message", [
    # Key field native unit: a row whose native unit has no entry for its canonical unit.
    ("native-unit", lambda d: set_path(d, "field_rules.1.prospective.0.native.unit", "Wh"), "prospective conversion"),
    # Key field canonical unit: a row naming a unit other than the pack field's SI unit.
    ("canonical-unit-customary", lambda d: set_path(d, "field_rules.1.prospective.0.unit", "unit.kilowatt_hour"), "prospective target"),
    ("canonical-unit-other-si", lambda d: set_path(d, "field_rules.1.prospective.0.unit", "unit.watt"), "prospective target"),
    # Value field factor: coefficient and exponent each varied.
    ("factor-coefficient", lambda d: set_path(d, "field_rules.1.prospective.0.conversion.factor", {"coefficient": "1", "exponent10": 6}), "prospective conversion"),
    ("factor-exponent", lambda d: set_path(d, "field_rules.1.prospective.0.conversion.factor", {"coefficient": "36", "exponent10": 6}), "prospective conversion"),
    ("factor-identity", lambda d: set_path(d, "field_rules.0.prospective.0.conversion.factor", ONE), "prospective conversion"),
    # Value field offset.
    ("offset", lambda d: set_path(d, "field_rules.4.prospective.2.conversion.offset", {"coefficient": "27315", "exponent10": -2}), "prospective conversion"),
    ("conversion-missing", lambda d: v.mutate(d, {"path": "field_rules.4.prospective.0.conversion", "remove": True}), "prospective conversion"),
    ("conversion-extra-member", lambda d: d["field_rules"][1]["prospective"][1]["conversion"].update(rounding="half_even"), "prospective conversion"),
    ("earlier-shift-member", lambda d: d["field_rules"][5]["prospective"][0].update({EARLIER_SHIFT: 3}), "prospective conversion"),
    ("silent-unit-conversion", lambda d: set_path(d, "field_rules.1.prospective.0.loss", ["policy"]), "silent unit conversion"),
    ("silent-power-conversion", lambda d: set_path(d, "field_rules.5.prospective.1.loss", []), "silent unit conversion"),
    ("identity-with-unit-loss", lambda d: set_path(d, "field_rules.4.prospective.0.loss", ["unit"]), "silent unit conversion"),
    ("loss-kind", lambda d: set_path(d, "field_rules.0.prospective.0.loss", ["unit", "native_enum"]), "prospective conversion"),
    ("loss-duplicated", lambda d: set_path(d, "field_rules.0.prospective.0.loss", ["unit", "unit"]), "prospective conversion"),
    ("counter-without-policy-loss", lambda d: set_path(d, "field_rules.4.prospective.3.loss", ["unit"]), "prospective counter"),
])
def test_conversion_key_and_value_mutations_are_rejected(name, mutation, message):
    document = contract()
    mutation(document)
    with Raises(ValueError) as error:
        v.validate_contract(document)
    assert message in str(error.value), str(error.value)


def test_scenario_expectations_are_executed_not_trusted():
    for damage, message in (
        (lambda d: d["scenarios"][0]["expect"]["output"]["role"].update({"declared": "feeder"}), "scenario output"),
        (lambda d: d["scenarios"][3]["expect"]["summary"]["native_dispositions"].pop(), "scenario summary"),
        (lambda d: d["scenarios"][-1]["expect"].update({"error": "lifecycle_missing"}), "gate_revision_required"),
    ):
        document = contract()
        damage(document)
        with Raises(ValueError) as error:
            v.validate_scenarios(document)
        assert message in str(error.value)


def test_scenario_coverage_is_enforced():
    for dropped, message in ((("MTR-POS-008-groups-answer-nothing-decoded", "MTR-POS-012-listed-meter-route-agrees", "MTR-POS-013-inverter-side-route-agrees"), "cover every reason"), (("MTR-POS-009-meter-read-exception",), "cover every reason"), (("MTR-POS-004-identity-mismatch",), "cover every reason"), (("MTR-POS-005-identity-not-read",), "cover every reason"), (("MTR-POS-011-grid-power-outside-range",), "cover every reason"), (("MTR-NEG-009-role-derived-from-sign", "MTR-NEG-010-role-outside-set"), "cover every error")):
        document = contract()
        document["scenarios"] = [s for s in document["scenarios"] if s["id"] not in dropped]
        with Raises(ValueError, match=message):
            v.validate_scenarios(document)


def test_a_negative_scenario_that_is_accepted_fails():
    document = contract()
    next(s for s in document["scenarios"] if s["id"] == "MTR-NEG-008-source-missing")["mutations"] = []
    with Raises(ValueError, match="accepted"):
        v.validate_scenarios(document)


def page():
    return v.DOC.read_text(encoding="utf-8")


@parametrize("old,new", [
    ("| `emma_external_sensor_group` | emma | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |", "| `emma_external_sensor_group` | emma | Proven | `infrastructure.reason.no_meter_class_contract` | policy |"),
    ("| `infrastructure.power.import_active` | `infrastructure.dimension.grid_connection` | `sdongle_plant_grid_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `positive_import` | `grid_connection` |", "| `infrastructure.power.import_active` | `infrastructure.dimension.grid_connection` | `sdongle_plant_grid_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `positive_export` | `grid_connection` |"),
    ("| I64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 |", "| I64, kWh, gain 100 | `unit.kilowatt_hour`, factor 1, offset 0 |"),
    ("| I32, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy |", "| I32, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | policy |"),
    ("| I32, W, gain 1 | `unit.watt`, factor 1, offset 0 |", "| I32, W, gain 1 | `unit.watt`, factor 1000, offset 0 |"),
    ("| `feeder` | withheld | `infrastructure.dimension.meter` | unknown |", "| `feeder` | `infrastructure.dimension.grid_connection` | `infrastructure.dimension.meter` | unknown |"),
    ("| `MTR-NEG-016-error-precedence` |", "| `MTR-NEG-099-other` |"),
    ("role is configuration", "role is derived"),
    ("word positions the documents do not state", "word positions are guessed"),
    ("outbound_allowed=false", "outbound allowed"),
])
def test_page_must_match_the_contract(old, new):
    pattern = re.escape(old).replace(r"\ ", r"\s+")
    mutated, count = re.subn(pattern, lambda _: new, page())
    assert count >= 1
    with Raises(ValueError):
        v.check_document(v.load(), mutated)


def test_a_dropped_table_row_is_rejected():
    for prefix in ("| `profile_meter_groups` |", "| `circuit` |"):
        lines = [line for line in page().splitlines(keepends=True) if not line.startswith(prefix)]
        with Raises(ValueError, match="document tables"):
            v.check_document(v.load(), "".join(lines))


@parametrize("literal", ["c" * 40, "0123456789abcdef" * 4, "E1" * 20])
def test_publication_rule_rejects_revision_and_digest_literals(literal):
    with Raises(ValueError, match="publication"):
        v.check_document(v.load(), page() + "\n" + literal + "\n")


OWNER_LINK = "https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/graph.md"


def test_publication_accepts_approved_full_sha_owner_link():
    v.check_document(v.load(), page() + "\n[graph](" + OWNER_LINK + ")\n")


@parametrize("link", [OWNER_LINK.replace("de3a0b49535582ad51a0ec23011d3aebf9c04c76", "0123456789abcdef0123456789abcdef01234567"), OWNER_LINK.replace("/blob/", "/commit/"), OWNER_LINK + "#L10"])
def test_publication_rejects_other_owner_links(link):
    with Raises(ValueError, match="publication"):
        v.check_document(v.load(), page() + "\n[graph](" + link + ")\n")


def test_published_files_cite_no_revision_link_or_endpoint():
    for path in (v.PATH, v.DOC):
        text = path.read_text(encoding="utf-8")
        assert not v.HEX_RUN_RE.search(v.unowned_text(text))
        assert "/blob/" not in v.unowned_text(text) and "/commit/" not in text and "#L" not in text and "tcp://" not in text


# The member names used before the in-place rename, assembled so that no file spells them.
EARLIER_MEMBER = "relation" + "ships"
EARLIER_ROLE_MEMBER = "relation" + "ship"


@parametrize("member", [EARLIER_ROLE_MEMBER + "_by_role", "dimension_by_role"])
def test_the_role_member_rejects_earlier_spellings(member):
    document = contract()
    document["role"][member] = document["role"].pop("relation_by_role")
    with Raises(ValueError) as error:
        v.validate_contract(document)
    assert str(error.value) == "role"


@parametrize("scenario_id,member,path", [
    ("MTR-POS-001-declared-meter-no-read", "output", ()),
    ("MTR-POS-001-declared-meter-no-read", "output", ("role", "dimension_if_admitted")),
    ("MTR-POS-007-feeder-role", "summary", ("role", "dimension_if_admitted")),
])
def test_expected_outputs_reject_the_earlier_relation_members(scenario_id, member, path):
    document = contract()
    target = next(s for s in document["scenarios"] if s["id"] == scenario_id)["expect"][member]
    for key in path:
        target = target[key]
    if path:
        target[EARLIER_ROLE_MEMBER] = target.pop("relation")
    else:
        target[EARLIER_MEMBER] = target.pop("relations")
    with Raises(ValueError) as error:
        v.validate_scenarios(document)
    assert str(error.value) == "scenario %s %s" % (member, scenario_id)


GRAPH_SPEC = importlib.util.spec_from_file_location("mapping_gate_graph_rules", ROOT / "scripts/mapping/mapping_gate_graph_rules.py")
graph_rules = importlib.util.module_from_spec(GRAPH_SPEC)
GRAPH_SPEC.loader.exec_module(graph_rules)
GRID_METER = "infrastructure.relation.grid_connection_meter"


def graph_rejection(edits, catalog=None):
    document = contract()
    for op, path, value in edits:
        if op == "append":
            target = document
            for part in path.split("."):
                target = target[int(part)] if isinstance(target, list) else target[part]
            target.append(copy.deepcopy(value))
        elif op == "delete":
            v.mutate(document, {"path": path, "remove": True})
        else:
            v.mutate(document, {"path": path, "value": copy.deepcopy(value)})
    try:
        v.validate_contract(document, catalog)
    except ValueError as error:
        return str(error)
    return None


def test_gate_states_its_graph_and_passes_the_shared_checker():
    document = contract()
    assert "graph" in document
    assert graph_rules.check(document, graph_rules.load_catalog()) == []
    assert not re.search(r'"planes?"', v.PATH.read_text(encoding="utf-8"))


def test_the_meter_role_is_a_relation_instance_held_by_the_grid_connection():
    document = contract()
    graph = document["graph"]
    assert [resource["kind"] for resource in graph["resources"]] == ["resource.site", "resource.grid_connection", "resource.meter"]
    assert {resource["anchor_source"] for resource in graph["resources"]} == {"operator_label"}
    role = graph["relations"][1]
    assert (role["definition"], role["basis_class"], role["from_kind"], role["to_kind"]) == (GRID_METER, "configuration", "resource.grid_connection", "resource.meter")
    assert all(relation["absence_sides"] == [] for relation in graph["relations"])
    assert document["role"]["relation_by_role"] == {"grid_connection": GRID_METER, "feeder": "unknown", "circuit": "unknown"}
    assert graph["relayed"] == [] and graph["aggregates"] == []


def relation_record(definition, from_kind, to_kind, sides=()):
    return {"definition": definition, "basis_class": "configuration", "from_kind": from_kind, "to_kind": to_kind, "evidence": ["synthetic:relation"],
            "validity_scope": {"profile": "no_meter_class_contract", "version": "no_admitted_profile", "rule_scope": "synthetic"}, "absence_sides": list(sides)}


GRID_POWER_AGGREGATE = {"fact": "infrastructure.power.import_active", "subject_kind": "resource.grid_connection", "coverage": []}
GRID_RELAY = {"relaying_kind": "resource.grid_connection", "relayed_kind": "resource.meter", "direct_anchor_key_source": "none"}

# (name, edits, expected error). Indices: resources site, grid connection, meter; relations site to grid connection,
# grid connection to meter; selectors gateway path, profile identifier. Prospective rows: field rule 0 rows 0-2
# (logger power and power factor), rule 1 rows 0-1 (logger energy), rule 4 rows 0-3 (inverter-exposed power and
# energy), rule 5 rows 0-1 (dongle plant grid power).
GRAPH_NEGATIVES = [
    # EN-8: a meter role is an instance of the measures definition, held by the grid connection, with basis configuration.
    ("role_without_relation_instance", [("delete", "graph.relations.1", None)], "role relation"),
    ("role_relation_native_observation", [("set", "graph.relations.1.basis_class", "native_observation")], "graph basis_class_not_admitted $.graph.relations[1].basis_class"),
    ("role_relation_gate_rule", [("set", "graph.relations.1.basis_class", "gate_rule")], "graph basis_class_not_admitted $.graph.relations[1].basis_class"),
    ("role_relation_held_by_the_meter", [("set", "graph.relations.1.from_kind", "resource.meter"), ("set", "graph.relations.1.to_kind", "resource.grid_connection")],
     "graph endpoint_kind_mismatch $.graph.relations[1].from_kind"),
    ("role_relation_from_the_site", [("set", "graph.relations.1.from_kind", "resource.site")], "graph endpoint_kind_mismatch $.graph.relations[1].from_kind"),
    ("role_relation_absence", [("set", "graph.relations.1.absence_sides", ["to"])], "graph_relations"),
    ("grid_role_stated_unknown", [("set", "role.relation_by_role.grid_connection", "unknown")], "role"),
    ("feeder_role_given_the_grid_relation", [("set", "role.relation_by_role.feeder", GRID_METER)], "role"),
    ("feeder_role_given_a_supplies_relation", [("set", "role.relation_by_role.feeder", "infrastructure.relation.grid_connection_feeder")], "role"),
    ("circuit_role_stated_absent", [("set", "role.relation_by_role.circuit", "absent")], "role"),
    ("role_member_dropped", [("delete", "role.relation_by_role.circuit", None)], "role"),
    ("earlier_dimension_member", [("set", "role.dimension_by_role", {"feeder": {"relation": "none_" + "decided"}})], "role"),
    # Dangling endpoint: a feeder statement whose endpoint the gate does not publish.
    ("dangling_feeder_absence", [("append", "graph.relations", relation_record("infrastructure.relation.grid_connection_feeder", "resource.grid_connection", "resource.feeder", ["to"]))],
     "graph dangling_endpoint $.graph.relations[2].to_kind"),
    ("dangling_feeder_circuit", [("append", "graph.relations", relation_record("infrastructure.relation.feeder_circuit", "resource.feeder", "resource.electrical_circuit"))],
     "graph dangling_endpoint $.graph.relations[2].from_kind"),
    ("dangling_site_dropped", [("delete", "graph.resources.0", None)], "graph dangling_endpoint $.graph.relations[0].from_kind"),
    # EN-1: a power value is placed on the resource it is measured at, never on the device that reports it.
    ("power_on_the_reporting_meter", [("set", "field_rules.0.prospective.0.subject_kind", "resource.meter"), ("set", "field_rules.0.prospective.0.measurement_point", "meter_register")], "field rule subject"),
    ("power_on_the_reporting_gateway", [("set", "field_rules.5.prospective.0.subject_kind", "resource.equipment")], "field rule subject"),
    ("power_on_the_inverter_child", [("set", "field_rules.4.prospective.1.subject_kind", "resource.inverter")], "field rule subject"),
    ("power_factor_subject_missing", [("delete", "field_rules.0.prospective.2.subject_kind", None)], "field rule subject"),
    ("energy_on_the_grid_connection", [("set", "field_rules.1.prospective.0.subject_kind", "resource.grid_connection"), ("set", "field_rules.1.prospective.0.measurement_point", "grid_connection_point")], "field rule subject"),
    ("measurement_point_missing", [("delete", "field_rules.4.prospective.2.measurement_point", None)], "field rule subject"),
    ("measurement_point_blank", [("set", "field_rules.1.prospective.1.measurement_point", "")], "field rule subject"),
    ("measurement_point_moved", [("set", "field_rules.5.prospective.1.measurement_point", "meter_register")], "field rule subject"),
    ("measurement_point_reporting_unit", [("set", "field_rules.4.prospective.0.measurement_point", "gateway_unit")], "field rule subject"),
    ("energy_row_with_a_role", [("set", "field_rules.1.prospective.0.role", "grid_connection")], "prospective role"),
    # Resources (section 15 item 1): kind, class, basis, source, scheme, parent and evidence each varied.
    ("meter_kind_not_member", [("set", "graph.resources.2.kind", "resource.not_a_catalog_kind")], "graph kind_not_member $.graph.resources[2].kind"),
    ("grid_connection_anchor_class", [("set", "graph.resources.1.anchor_class", "equipment")], "graph anchor_class_mismatch $.graph.resources[1].anchor_class"),
    ("meter_anchored_on_routed_unit", [("set", "graph.resources.2.anchor_source", "routed_unit")], "graph anchor_source_not_allowed $.graph.resources[2].anchor_source"),
    ("meter_anchored_on_address", [("set", "graph.resources.2.anchor_source", "network_address")], "graph anchor_source_not_allowed $.graph.resources[2].anchor_source"),
    ("meter_anchored_on_position", [("set", "graph.resources.2.anchor_basis", "native_slot"), ("set", "graph.resources.2.anchor_source", "native_position")], "graph_anchor"),
    ("site_basis_source_mismatch", [("set", "graph.resources.0.anchor_basis", "native_identity")], "graph anchor_basis_mismatch $.graph.resources[0].anchor_basis"),
    ("meter_native_token_without_scheme", [("set", "graph.resources.2.anchor_basis", "native_identity"), ("set", "graph.resources.2.anchor_source", "native_token")],
     "graph scheme_not_listed $.graph.resources[2].identification_scheme"),
    ("meter_scheme_unlisted", [("set", "graph.resources.2.identification_scheme", {"id": "scheme.unlisted", "issuer_registry": "none"})], "graph scheme_not_listed $.graph.resources[2].identification_scheme"),
    ("meter_parent_grid_connection", [("set", "graph.resources.2.parent_kind", "resource.grid_connection")], "graph_parent"),
    ("grid_connection_parent_site", [("set", "graph.resources.1.parent_kind", "resource.site")], "graph_parent"),
    ("meter_token_stability_unstated", [("delete", "graph.resources.2.evidence.2", None)], "graph_evidence"),
    ("meter_native_contract_unstated", [("set", "graph.resources.2.evidence", ["native_owner_token_stability:unknown_until_a_meter_class_contract_states_an_identity_read"])], "graph_evidence"),
    ("site_configuration_unstated", [("delete", "graph.resources.0.evidence.0", None)], "graph_evidence"),
    ("grid_connection_token_stability_unstated", [("delete", "graph.resources.1.evidence.1", None)], "graph_evidence"),
    ("meter_evidence_empty", [("set", "graph.resources.2.evidence", [])], "graph invalid_value $.graph.resources[2].evidence"),
    ("feeder_resource_added", [("append", "graph.resources", {"kind": "resource.feeder", "anchor_class": "function", "anchor_basis": "operator_slot", "anchor_source": "operator_label",
                                                              "identification_scheme": "none_listed", "parent_kind": None, "evidence": ["synthetic:feeder"]})], "graph_resources"),
    # Relations (section 15 item 2): definition, basis class, endpoints, evidence, each scope member and absence sides.
    ("site_relation_native_observation", [("set", "graph.relations.0.basis_class", "native_observation")], "graph basis_class_not_admitted $.graph.relations[0].basis_class"),
    ("site_relation_reversed", [("set", "graph.relations.0.from_kind", "resource.grid_connection"), ("set", "graph.relations.0.to_kind", "resource.site")],
     "graph endpoint_kind_mismatch $.graph.relations[0].from_kind"),
    ("relation_not_member", [("set", "graph.relations.0.definition", "infrastructure.relation.invented")], "graph relation_not_member $.graph.relations[0].definition"),
    ("relation_evidence_empty", [("set", "graph.relations.1.evidence", [])], "graph invalid_value $.graph.relations[1].evidence"),
    ("scope_profile", [("set", "graph.relations.1.validity_scope.profile", "synthetic.other_profile")], "graph_relations"),
    ("scope_version", [("set", "graph.relations.0.validity_scope.version", "1.0.0")], "graph_relations"),
    ("scope_rule_blank", [("set", "graph.relations.1.validity_scope.rule_scope", "")], "graph invalid_value $.graph.relations[1].validity_scope.rule_scope"),
    ("scope_rule_widened", [("set", "graph.relations.1.validity_scope.rule_scope", "every_meter_of_the_site")], "graph_relations"),
    ("site_relation_dropped", [("delete", "graph.relations.0", None)], "graph_relations"),
    # Joins (section 6.4): one meter seen through two paths joins by an operator declaration only.
    ("meter_joined_by_name", [("set", "graph.joins.0.basis", "display_name")], "graph join_basis_not_allowed $.graph.joins[0].basis"),
    ("meter_joined_by_routed_unit", [("set", "graph.joins.0.basis", "routed_unit")], "graph join_basis_not_allowed $.graph.joins[0].basis"),
    ("meter_joined_over_differing_tokens", [("set", "graph.joins.0.native_tokens", "differ")], "graph false_join $.graph.joins[0].native_tokens"),
    ("meter_joined_on_a_key_no_path_exposes", [("set", "graph.joins.0.basis", "same_anchor_key"), ("set", "graph.joins.0.native_tokens", "equal")], "graph_joins"),
    ("meter_join_dropped", [("set", "graph.joins", [])], "graph_joins"),
    ("meter_join_of_one_view", [("set", "graph.joins.0.observations", ["exposed_at_a_gateway_meter_unit"])], "graph invalid_value $.graph.joins[0].observations"),
    ("join_of_the_grid_connection", [("set", "graph.joins.0.kind", "resource.grid_connection")], "graph_joins"),
    # Relayed (section 15 item 3), aggregates (item 4, EN-5) and selectors (item 5).
    ("relay_claimed_without_an_observed_path", [("set", "graph.relayed", [GRID_RELAY])], "graph_relayed"),
    ("relay_by_an_unpublished_kind", [("set", "graph.relayed", [{**GRID_RELAY, "relaying_kind": "resource.equipment"}])], "graph kind_not_published $.graph.relayed[0].relaying_kind"),
    ("member_as_aggregate", [("append", "graph.aggregates", GRID_POWER_AGGREGATE)], "graph not_an_aggregate $.graph.aggregates[0].fact"),
    ("gateway_path_as_identity_evidence", [("set", "graph.selectors.0.classification", "identity_evidence")], "graph_selectors"),
    ("profile_as_resource_attribute", [("set", "graph.selectors.1.classification", "resource_attribute")], "graph_selectors"),
    ("selector_dropped", [("delete", "graph.selectors.1", None)], "graph_selectors"),
    # Section 15 item 6 and PLN-1.
    ("plane_on_resource", [("set", "graph.resources.2.planes", ["physical"])], "graph plane_membership_stated $.graph.resources[2].planes"),
    ("plane_on_field_rule", [("set", "field_rules.0.planes", ["energy"])], "graph plane_membership_stated $.field_rules[0].planes"),
    # Section 7.2: the infrastructure relations the gate does not state stay unknown, never absent.
    ("unstated_dropped", [("delete", "unstated_relations.0", None)], "unstated_relations"),
    ("unstated_absent", [("set", "unstated_relations.1.outcome", "absent")], "unstated_relations"),
    ("unstated_endpoint", [("set", "unstated_relations.2.to_kind", "resource.electrical_circuit")], "unstated_relations"),
    ("unstated_reason", [("set", "unstated_relations.0.withheld_by", "unsupported:breaker_state")], "unstated_relations"),
]


@parametrize("name,edits,expected", GRAPH_NEGATIVES)
def test_graph_role_and_subject_negatives_are_rejected(name, edits, expected):
    assert graph_rejection(edits) == expected, name


def catalog_with(element_id, mutate_attributes):
    catalog = copy.deepcopy(graph_rules.load_catalog())
    for element in catalog["elements"]:
        if element["ref"]["id"] == element_id:
            mutate_attributes(element["attributes"])
    return catalog


def as_meter_measures(attributes):
    attributes.update(core_type="measures", orientation="inverse", from_kinds=["resource.feeder"], to_kinds=["resource.meter"])


# Constants that repeat a catalog fact are lookups: a changed catalog entry changes the outcome.
@parametrize("element_id,mutate_attributes,expected", [
    ("infrastructure.power.import_active", lambda a: a["subject"].update(kind="resource.meter"), "field rule subject"),
    ("infrastructure.energy.export", lambda a: a["subject"].update(kind="resource.grid_connection"), "field rule subject"),
    ("infrastructure.energy.import", lambda a: a["subject"]["dimension"].update(id="infrastructure.dimension.grid_connection"), "prospective target"),
    ("infrastructure.dimension.meter", lambda a: a.update(resource_kind="resource.equipment"), "role"),
    ("infrastructure.dimension.site", lambda a: a.update(resource_kind="resource.equipment"), "graph_resources"),
    (GRID_METER, lambda a: a.update(basis_classes=["native_observation"]), "graph basis_class_not_admitted $.graph.relations[1].basis_class"),
    (GRID_METER, lambda a: a.update(from_kinds=["resource.site"]), "graph endpoint_kind_mismatch $.graph.relations[1].from_kind"),
    (GRID_METER, lambda a: a.update(core_type="supplies"), "role"),
    ("infrastructure.relation.grid_connection_feeder", as_meter_measures, "role"),
    ("infrastructure.power.import_active", lambda a: a.update(aggregation="aggregate"), "graph_aggregates"),
    ("infrastructure.relation.feeder_circuit", lambda a: a.update(from_kinds=["resource.grid_connection"]), "unstated_relations"),
])
def test_catalog_facts_are_looked_up_not_repeated(element_id, mutate_attributes, expected):
    assert graph_rejection([], catalog_with(element_id, mutate_attributes)) == expected


def test_a_role_relation_admitted_as_native_observation_is_still_configuration():
    catalog = catalog_with(GRID_METER, lambda a: a.update(basis_classes=["configuration", "native_observation"]))
    assert graph_rejection([], catalog) is None
    assert graph_rejection([("set", "graph.relations.1.basis_class", "native_observation")], catalog) == "role relation"


def test_each_prospective_row_names_its_catalog_subject_and_a_measurement_point():
    catalog = graph_rules.load_catalog()
    facts = {element["ref"]["id"]: element["attributes"] for element in catalog["elements"] if element["element_kind"] == "fact"}
    rows = [(rule["native_id"], row) for rule in contract()["field_rules"] for row in rule.get("prospective", [])]
    assert len(rows) == 11
    for native_id, row in rows:
        subject = facts[row["target"]]["subject"]
        assert row["subject_kind"] == subject["kind"] and row["dimension"] == subject["dimension"]["id"], native_id
        expected = "meter_register" if subject["kind"] == "resource.meter" else "grid_connection_point"
        assert row["measurement_point"] == expected, native_id


def test_the_role_placement_is_derived_from_the_catalog():
    placement = v.role_placement(v.load(), "grid_connection")
    assert placement == {"power": "infrastructure.dimension.grid_connection", "energy": "infrastructure.dimension.meter", "relation": GRID_METER}
    for role in ("feeder", "circuit"):
        assert v.role_placement(v.load(), role) == {"power": "withheld", "energy": "infrastructure.dimension.meter", "relation": "unknown"}


@parametrize("old,new", [
    ("| `resource.meter` | equipment | `operator_slot` | `operator_label` | none |", "| `resource.meter` | equipment | `native_slot` | `native_position` | none |"),
    ("| `infrastructure.relation.grid_connection_meter` | `configuration` |", "| `infrastructure.relation.grid_connection_meter` | `native_observation` |"),
    ("| `infrastructure.relation.feeder_circuit` | `resource.feeder` | `resource.electrical_circuit` |", "| `infrastructure.relation.feeder_circuit` | `resource.feeder` | `resource.phase` |"),
    ("| `sdongle_plant_grid_power` | `infrastructure.power.import_active` | `resource.grid_connection` | `grid_connection_point` |",
     "| `sdongle_plant_grid_power` | `infrastructure.power.import_active` | `resource.equipment` | `grid_connection_point` |"),
    ("| `feeder` | withheld | `infrastructure.dimension.meter` | unknown |", "| `feeder` | withheld | `infrastructure.dimension.meter` | none decided |"),
])
def test_page_graph_tables_must_match_the_contract(old, new):
    pattern = re.escape(old).replace(r"\ ", r"\s+")
    mutated, count = re.subn(pattern, lambda _: new, page())
    assert count >= 1
    with Raises(ValueError, match="document tables"):
        v.check_document(v.load(), mutated)


OWNER_BASE = "https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/"


def test_publication_accepts_each_approved_owner_path():
    for path in ("api/v1/graph.md", "api/v1/element-catalog-v1.json", "api/README.md", "mappings/infrastructure/gateway-runtime.md"):
        v.check_document(v.load(), page() + "\n[owner](" + OWNER_BASE + path + ")\n")


@parametrize("link", [
    OWNER_BASE + "api/v1/" + "a" * 40 + ".md",
    OWNER_BASE + "api/v1/unlisted.md",
    OWNER_BASE + "api/v1/graph.md?plain=1",
    OWNER_BASE + "api/v1/graph.md.bak",
    OWNER_BASE + "api/v1/element-catalog-v1.json#L10",
])
def test_publication_rejects_unapproved_owner_paths_and_suffixes(link):
    with Raises(ValueError, match="publication"):
        v.check_document(v.load(), page() + "\n[owner](" + link + ")\n")


@parametrize("suffix", ["\n#L12\n", "\ntcp://192.0.2.10:502\n", "\n192.0.2.10:502\n"])
def test_publication_rejects_standalone_line_anchor_and_endpoint(suffix):
    with Raises(ValueError, match="publication"):
        v.check_document(v.load(), page() + suffix)


def test_publication_rejects_digest_shaped_fragment_on_an_approved_owner_link():
    with Raises(ValueError, match="publication"):
        v.check_document(v.load(), page() + "\n[owner](" + OWNER_BASE + "api/v1/graph.md#" + "a" * 40 + ")\n")


@parametrize("destination", [
    "https://example.invalid/page?u=" + OWNER_BASE + "api/v1/graph.md",
    "https://example.invalid/" + OWNER_BASE + "api/v1/graph.md",
    "https://example.invalid/@" + OWNER_BASE + "api/v1/graph.md",
    "https://example.invalid/page#" + OWNER_BASE + "api/v1/graph.md",
])
def test_owner_link_exception_applies_only_to_the_actual_destination(destination):
    with Raises(ValueError, match="publication"):
        v.check_document(v.load(), page() + "\n[owner](" + destination + ")\n")


def test_owner_link_exception_leaves_labels_and_nested_foreign_destinations_checked():
    approved = OWNER_BASE + "api/v1/graph.md"
    texts = [
        "[https://example.invalid/?q=[a](" + approved + ")](https://example.invalid/?q=[a](" + approved + "))",
        "[a](https://example.invalid/?q=[b](" + approved + "))",
        "[a](https://example.invalid/?q=(x)[b](" + approved + "))",
        "[" + "a1" * 20 + "](" + approved + ")",
        "[page#L12](" + approved + ")",
        "[tcp://host:1](" + approved + ")",
    ]
    for text in texts:
        with Raises(ValueError, match="publication"):
            v.check_document(v.load(), page() + "\n" + text + "\n")


def test_owner_link_inside_a_deeply_nested_foreign_destination_gets_no_exception():
    approved = OWNER_BASE + "api/v1/graph.md"
    for depth in (3, 4, 6):
        text = "[a](https://example.invalid/?q=" + "(" * depth + "x" + ")" * depth + "[b](" + approved + "))"
        with Raises(ValueError, match="publication"):
            v.check_document(v.load(), page() + "\n" + text + "\n")
    v.check_document(v.load(), page() + "\n[b](" + approved + ")\n")


def test_owner_link_in_a_foreign_link_title_gets_no_exception():
    for quote in ('"', "'"):
        with Raises(ValueError, match="publication"):
            v.check_document(v.load(), page() + "\n[owner](https://example.invalid/x " + quote + "see " + OWNER_BASE + "api/v1/graph.md" + quote + ")\n")


def test_owner_link_in_a_reference_definition_title_gets_no_exception():
    approved = OWNER_BASE + "api/v1/graph.md"
    for title in ('"see [b](' + approved + ')"', "'see [b](" + approved + ")'", "(see [b](" + approved + "))"):
        with Raises(ValueError, match="publication"):
            v.check_document(v.load(), page() + "\n[r]: https://example.invalid/x " + title + "\n")


def test_owner_link_in_code_or_a_continuation_title_gets_no_exception():
    approved = OWNER_BASE + "api/v1/graph.md"
    for text in ("```\n[a](" + approved + ")\n```", "~~~\n[a](" + approved + ")\n~~~", "`[a](" + approved + ")`", "    [a](" + approved + ")", "[r]: https://example.invalid/x\n  \"" + approved + "\"", "[r]: https://example.invalid/x\n  (" + approved + ")"):
        with Raises(ValueError, match="publication"):
            v.check_document(v.load(), page() + "\n" + text + "\n")


def test_owner_link_exception_needs_a_complete_approved_destination_not_a_markdown_shaped_title():
    approved = OWNER_BASE + "api/v1/graph.md"
    for text in ("[owner](" + approved + ";other)", "[owner](<" + approved + ";other>)", "[a](https://example.invalid/x \"[b](" + approved + ")\")", "[a]( https://example.invalid/x \"[b](" + approved + ")\")", "[a](\t<https://example.invalid/x> '[b](" + approved + ")')", "[a](https://example.invalid/x '[b](" + approved + ")')", "[owner](" + approved + " \"title\")"):
        with Raises(ValueError, match="publication"):
            v.check_document(v.load(), page() + "\n" + text + "\n")


def test_an_escaped_opening_bracket_is_literal_text_and_gets_no_owner_link_exception():
    approved = OWNER_BASE + "api/v1/graph.md"
    for text in ("\\[owner](" + approved + ")", "\\\\\\[owner](" + approved + ")", "[owner\\](" + approved + ")"):
        with Raises(ValueError, match="publication"):
            v.check_document(v.load(), page() + "\n" + text + "\n")
    v.check_document(v.load(), page() + "\n\\\\[owner](" + approved + ")\n")


if __name__ == "__main__":
    raise SystemExit(support.run(globals()))
