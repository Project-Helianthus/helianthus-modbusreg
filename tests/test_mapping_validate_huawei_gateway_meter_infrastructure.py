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
    set_path(document, "admission.decoded_meter_words", 0.0)
    with Raises(ValueError):
        v.validate_contract(document)


@parametrize("name,mutation,message", [
    ("pack", lambda d: set_path(d, "pack.id", "helianthus.pack.pv"), "contract"),
    ("pack-version", lambda d: set_path(d, "pack.version", "1.1.0"), "contract"),
    ("status", lambda d: set_path(d, "status", "accepted_source_specific_read_only_mapping_gate"), "contract"),
    ("evidence-amendment", lambda d: set_path(d, "evidence.meter_observation_amendment.issue", 255), "evidence"),
    ("evidence-page", lambda d: d["evidence"]["monitoring_contract"]["pages"].pop(), "evidence"),
    ("class-contract-claimed", lambda d: set_path(d, "admission.meter_class_contract", True), "admission"),
    ("meter-identity-claimed", lambda d: set_path(d, "admission.meter_identity", "PROFILE_SELECTED"), "admission"),
    ("decoded-words", lambda d: set_path(d, "admission.decoded_meter_words", 4), "admission"),
    ("meter-selectable", lambda d: d["admission"]["selection_outcomes_allowed"].append("PROFILE_SELECTED"), "admission"),
    ("precondition-dropped", lambda d: v.mutate(d, {"path": "admission.preconditions_for_any_mapping.3", "remove": True}), "admission"),
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


def test_every_native_item_has_exactly_one_withheld_disposition_and_nothing_is_published():
    document = v.load()
    assert [rule["native_id"] for rule in document["field_rules"]] == [nid for nid, _ in v.INVENTORY]
    for scenario in document["scenarios"]:
        if scenario["polarity"] != "positive":
            continue
        output = v.run_scenario(scenario, document)
        assert [row["native_id"] for row in output["native_dispositions"]] == [nid for nid, _ in v.INVENTORY]
        assert {row["outcome"] for row in output["native_dispositions"]} == {"withheld"}
        assert output["facts"] == [] and output["relations"] == [] and output["operations"] == []
        assert [item["outcome"] for item in output["dispositions"]] == ["withheld"] * 8
        assert all(item["source_keys"] == [] and item["reason"] == v.PRIMARY_REASON and item["loss"] for item in output["dispositions"])


def test_completeness_check_catches_damage():
    document = v.load()
    output = v.run_scenario(document["scenarios"][0], document)
    for damage in ("drop", "duplicate", "fact", "outcome", "reason"):
        broken = copy.deepcopy(output)
        if damage == "drop":
            del broken["native_dispositions"][-1]
        elif damage == "duplicate":
            broken["native_dispositions"].append(copy.deepcopy(broken["native_dispositions"][-1]))
        elif damage == "fact":
            broken["facts"].append({"key": {}})
        elif damage == "outcome":
            broken["native_dispositions"][0]["outcome"] = "exact"
        else:
            broken["native_dispositions"][0]["reason"] = "infrastructure.reason.invented"
        with Raises(ValueError):
            v.assert_complete(broken, document)


def test_an_answering_group_is_neither_a_value_nor_an_identity():
    document = case()
    document["reads"] = [{"native_id": native_id, "outcome": "ANSWERED"} for native_id in v.FEEDABLE]
    output = project(document)
    assert output["facts"] == []
    rows = {row["native_id"]: row["reason"] for row in output["native_dispositions"]}
    assert all(rows[native_id] == "infrastructure.reason.answered_not_decoded" for native_id in v.FEEDABLE)
    assert rows["sun2000_inverter_exposed_meter_group_refused"] == "infrastructure.reason.refused_on_family"
    assert rows["profile_meter_groups"] == "infrastructure.reason.profile_withholds_meter"


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
    assert output["role"]["keyed_to"] == {"source_id": "source:gateway-demo-d", "routed_unit": 11}
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


@parametrize("source", ["", None, "source gateway", "source:serial-1", "source:ESN-1", "source:10.1.2.3", "source:host:502", "11", "source:meter-huawei", "source:emma-a"])
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


def test_prospective_rows_are_not_in_force_and_match_the_pack():
    fields, dimensions, relations = v.pack_catalog()
    document = v.load()
    for rule in document["field_rules"]:
        for row in rule.get("prospective", []):
            assert row["in_force"] is False
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
    for dropped, message in ((("MTR-POS-004-groups-answer-nothing-decoded",), "cover every reason"), (("MTR-POS-005-meter-read-exception",), "cover every reason"), (("MTR-NEG-007-role-derived-from-sign", "MTR-NEG-008-role-outside-set"), "cover every error")):
        document = contract()
        document["scenarios"] = [s for s in document["scenarios"] if s["id"] not in dropped]
        with Raises(ValueError, match=message):
            v.validate_scenarios(document)


def test_a_negative_scenario_that_is_accepted_fails():
    document = contract()
    next(s for s in document["scenarios"] if s["id"] == "MTR-NEG-006-source-missing")["mutations"] = []
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
    ("| `MTR-NEG-014-error-precedence` |", "| `MTR-NEG-099-other` |"),
    ("role is configuration", "role is derived"),
    ("no meter register is decoded", "a meter register is decoded"),
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
    ("MTR-POS-001-grid-role-no-meter-read", "output", ()),
    ("MTR-POS-001-grid-role-no-meter-read", "output", ("role", "dimension_if_admitted")),
    ("MTR-POS-003-feeder-role", "summary", ("role", "dimension_if_admitted")),
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


if __name__ == "__main__":
    raise SystemExit(support.run(globals()))
