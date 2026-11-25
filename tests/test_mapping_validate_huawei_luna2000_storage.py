#!/usr/bin/env python3
"""Mutation controls for the Huawei LUNA2000 storage gate (run with python3 or python3 -m pytest)."""
import copy
import importlib.util
import re
import sys
from fractions import Fraction
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("validate_huawei_luna2000_storage_mapping", ROOT / "scripts/mapping/validate_huawei_luna2000_storage_mapping.py")
v = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = v
SPEC.loader.exec_module(v)

SUPPORT_SPEC = importlib.util.spec_from_file_location("huawei_mapping_test_support", ROOT / "scripts/mapping/huawei_mapping_test_support.py")
support = importlib.util.module_from_spec(SUPPORT_SPEC)
sys.modules[SUPPORT_SPEC.name] = support
SUPPORT_SPEC.loader.exec_module(support)
parametrize, Raises = support.parametrize, support.Raises
fraction_of = support.fraction_of
ONE, ZERO = {"coefficient": "1", "exponent10": 0}, {"coefficient": "0", "exponent10": 0}
# (native unit, canonical SI unit) -> exact factor and offset; the SI value is native x factor + offset.
CONVERSIONS = {
    ("%", "unit.ratio"): {"factor": {"coefficient": "1", "exponent10": -2}, "offset": ZERO},
    ("kW", "unit.watt"): {"factor": {"coefficient": "1", "exponent10": 3}, "offset": ZERO},
    ("kWh", "unit.joule"): {"factor": {"coefficient": "36", "exponent10": 5}, "offset": ZERO},
}


def contract():
    return copy.deepcopy(v.load())


def case():
    return copy.deepcopy(v.load()["baselines"]["default"])


def set_path(document, path, value):
    v.mutate(document, {"path": path, "value": value})


def project(document):
    return v.project(document, v.load())


def rule_index(native_id):
    return [r["native_id"] for r in v.load()["field_rules"]].index(native_id)


def test_gate_passes_with_its_page():
    document = v.load()
    v.document(document)
    v.check_document(document, v.DOC.read_text(encoding="utf-8"))


def test_no_binary_floating_point():
    with Raises(ValueError, match="binary floating point"):
        v.loads('{"gain": 100.0}')
    document = contract()
    set_path(document, "admission.decisions.0.grade", 0.5)
    with Raises(ValueError):
        v.validate_contract(document)
    float_case = case()
    float_case["reads"] = [{"offset": 37066, "quantity": 4, "outcome": "ANSWERED", "words": [1.0, 2, 3, 4]}]
    with Raises(ValueError, match="native_evidence_invalid"):
        project(float_case)


@parametrize("name,mutation,message", [
    ("pack", lambda d: set_path(d, "pack.id", "helianthus.pack.pv"), "contract"),
    ("pack-version", lambda d: set_path(d, "pack.version", "1.0.0"), "contract"),
    ("status", lambda d: set_path(d, "status", "accepted_source_specific_read_only_mapping_gate"), "contract"),
    ("evidence-issue", lambda d: set_path(d, "evidence.registry_profiles.issue", 349), "evidence"),
    ("unresolved-outcome-dropped", lambda d: d["admission"]["selection"]["unresolved"].remove("AMBIGUOUS"), "admission"),
    ("facts-not-admitted", lambda d: set_path(d, "admission.facts_admitted", False), "admission"),
    ("profile-source-dropped", lambda d: v.mutate(d, {"path": "admission.profile_source.pages.1", "remove": True}), "admission"),
    ("unresolved-publishes", lambda d: set_path(d, "admission.selection.unresolved_outcome", "published_as_zero"), "admission"),
    ("precondition-dropped", lambda d: v.mutate(d, {"path": "admission.decisions.2", "remove": True}), "admission"),
    ("power-sign-graded-proven", lambda d: set_path(d, "admission.decisions.2.grade", "proven"), "admission"),
    ("aggregation-basis-unrecorded", lambda d: set_path(d, "admission.decisions.3.decision", "not_recorded"), "admission"),
    ("status-read-only-withheld", lambda d: set_path(d, "status", "accepted_withheld_mapping_gate_no_battery_profile"), "contract"),
    ("native-item-dropped", lambda d: v.mutate(d, {"path": "field_rules.4", "remove": True}), "native inventory"),
    ("native-item-duplicated", lambda d: d["field_rules"].append(copy.deepcopy(d["field_rules"][0])), "native inventory"),
    ("native-item-added", lambda d: d["field_rules"].append({**copy.deepcopy(d["field_rules"][0]), "native_id": "battery_cell_voltage"}), "native inventory"),
    ("native-item-source", lambda d: set_path(d, "field_rules.4.source", "registry_disposition"), "native inventory"),
    ("native-item-read", lambda d: set_path(d, "field_rules.0.read.quantity", 31), "native inventory"),
    ("outcome-mapped", lambda d: set_path(d, "field_rules.9.disposition.outcome", "exact"), "withheld reason"),
    ("active-outcome-exact", lambda d: set_path(d, "field_rules.4.disposition.outcome", "exact"), "active disposition"),
    ("active-without-unit-loss", lambda d: set_path(d, "field_rules.4.disposition.loss", []), "active disposition"),
    ("active-row-withheld", lambda d: set_path(d, "field_rules.4.disposition", {"outcome": "withheld", "reason": "storage.reason.battery_group_not_read"}), "active disposition"),
    ("withheld-without-reason", lambda d: v.mutate(d, {"path": "field_rules.9.disposition.reason", "remove": True}), "withheld reason"),
    ("withheld-unknown-reason", lambda d: set_path(d, "field_rules.9.disposition.reason", "storage.reason.invented"), "withheld reason"),
    ("site-power-published", lambda d: set_path(d, "field_rules.5.prospective.in_force", True), "prospective target"),
    ("grade", lambda d: set_path(d, "field_rules.4.grade", "certain"), "withheld reason"),
    ("prospective-not-in-force", lambda d: set_path(d, "field_rules.4.prospective.in_force", False), "prospective target"),
    ("prospective-target-not-in-pack", lambda d: set_path(d, "field_rules.4.prospective.target", "storage.capacity.rated_energy"), "prospective target"),
    ("prospective-other-fact", lambda d: set_path(d, "field_rules.4.prospective.target", "storage.state.soh"), "requested items"),
    ("prospective-unit", lambda d: set_path(d, "field_rules.4.prospective.unit", "unit.ampere_hour"), "prospective target"),
    ("prospective-dimension", lambda d: set_path(d, "field_rules.4.prospective.dimension", "storage.dimension.cell"), "prospective target"),
    ("prospective-native-form", lambda d: set_path(d, "field_rules.4.prospective.native.gain", 1000), "prospective native form"),
    ("prospective-kilowatt-hour-as-ampere-hour", lambda d: set_path(d, "field_rules.7.prospective.unit", "unit.ampere_hour"), "prospective target"),
    ("prospective-sign-removed", lambda d: v.mutate(d, {"path": "field_rules.5.prospective.sign", "remove": True}), "prospective sign"),
    ("prospective-sign-assumed", lambda d: set_path(d, "field_rules.5.prospective.sign", "positive_while_charging_negative_while_discharging_assumed"), "prospective sign"),
    ("prospective-counter-removed", lambda d: v.mutate(d, {"path": "field_rules.7.prospective.counter", "remove": True}), "prospective counter"),
    ("prospective-symbols", lambda d: set_path(d, "field_rules.0.prospective.symbols", ["storage.status.operating.active"]), "prospective symbols"),
    ("reason-unused", lambda d: d["reasons"].update({"storage.reason.unused": {"loss_kind": "policy", "description": "x", "reversible": True}}), "reason inventory"),
    ("reason-primary-removed", lambda d: d["reasons"].pop("storage.reason.no_admitted_battery_profile"), "reason inventory"),
    ("capacity-converted", lambda d: set_path(d, "capacity.conversion", "allowed"), "capacity"),
    ("capacity-published", lambda d: set_path(d, "capacity.energy_units", "published"), "capacity"),
    ("unsupported-dropped", lambda d: v.mutate(d, {"path": "unsupported.0", "remove": True}), "unsupported"),
    ("limit-item-requested", lambda d: set_path(d, "pack_items_never_requested.0", "storage.limit.discharge_power"), "control-adjacent items"),
    ("operation-item-not-in-pack", lambda d: set_path(d, "pack_items_never_requested.4", "storage.operation.set_dispatch"), "control-adjacent items"),
    ("requested-added", lambda d: d["projection"]["requested_items"].append({"kind": "fact", "item_id": "storage.limit.charge_power"}), "requested items"),
    ("requested-dropped", lambda d: v.mutate(d, {"path": "projection.requested_items.0", "remove": True}), "requested items"),
    ("primary-reason", lambda d: set_path(d, "projection.primary_reason", "storage.reason.battery_group_not_read"), "projection"),
    ("precedence", lambda d: set_path(d, "projection.error_precedence.0", "identity_missing_or_invalid"), "projection"),
    ("operations", lambda d: set_path(d, "projection.operations", "allowed"), "projection"),
    ("identity-forbidden", lambda d: set_path(d, "identity.asset_id.forbidden_derivations.0", "vendor"), "identity contract"),
    ("identity-distinct", lambda d: set_path(d, "identity.source_id.distinct_from", "binding_id"), "identity contract"),
    ("identity-binding", lambda d: set_path(d, "identity.binding_id.stable_inputs.1", "routed_unit"), "identity contract"),
    ("identity-granularity", lambda d: set_path(d, "identity.storage_asset_granularity", "one_asset_per_battery"), "identity contract"),
    ("lifecycle-zero", lambda d: set_path(d, "lifecycle.absent_optional_group", "zero"), "lifecycle contract"),
    ("lifecycle-negation", lambda d: set_path(d, "lifecycle.exception", "negates_the_parent"), "lifecycle contract"),
    ("outcome-vocabulary", lambda d: set_path(d, "outcome_vocabulary.native_only", "exact"), "outcome vocabulary"),
    ("consumer", lambda d: set_path(d, "consumer_cutover.path", "growatt_single_asset_projection"), "consumer cutover"),
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
    document["reasons"]["storage.reason.sign_not_graded"][field] = value
    with Raises(ValueError, match="reason loss"):
        v.validate_contract(document)


def test_every_native_item_has_exactly_one_disposition_and_only_an_active_row_publishes():
    document = v.load()
    assert [rule["native_id"] for rule in document["field_rules"]] == [nid for nid, _, _, _ in v.INVENTORY]
    for scenario in document["scenarios"]:
        if scenario["polarity"] != "positive":
            continue
        output = v.run_scenario(scenario, document)
        assert [row["native_id"] for row in output["native_dispositions"]] == [nid for nid, _, _, _ in v.INVENTORY]
        assert {row["outcome"] for row in output["native_dispositions"]} <= {"withheld", "native_only", "transformed"}
        assert {row["native_id"] for row in output["native_dispositions"] if row["outcome"] == "transformed"} == {fact["provenance"]["native_id"] for fact in output["facts"]} <= v.ACTIVE
        assert output["relations"] == [] and output["operations"] == []
        assert len(output["dispositions"]) == len(v.REQUESTED)
        for item in output["dispositions"]:
            assert item["loss"] and (item["source_keys"] if item["outcome"] == "transformed" else item["reason"] and not item["source_keys"])
        assert [item["item_id"] for item in output["dispositions"] if item["outcome"] == "transformed"] == [fact["item_id"] for fact in output["facts"]] or not output["facts"]


def test_completeness_check_catches_damage():
    document = v.load()
    output = v.run_scenario(next(scenario for scenario in document["scenarios"] if scenario["id"] == "STG-POS-002-site-state-of-charge"), document)
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
            broken["native_dispositions"][0]["reason"] = "storage.reason.invented"
        with Raises(ValueError):
            v.assert_complete(broken, document)


@parametrize("reads,expected", [
    ([], {"battery_group_37000_30": "battery_group_not_read", "battery_single_read_37000_1": "battery_group_not_read", "battery_group_37066_4": "battery_group_not_read", "battery_group_47000_1": "battery_group_not_read"}),
    ([{"offset": 37000, "quantity": 1, "outcome": "ANSWERED", "words": [0]}, {"offset": 47000, "quantity": 1, "outcome": "ANSWERED", "words": [0]}],
     {"battery_group_37000_30": "battery_group_not_read", "battery_single_read_37000_1": "zero_word_is_not_a_battery_value", "battery_group_37066_4": "battery_group_not_read", "battery_group_47000_1": "zero_word_is_not_a_battery_value"}),
    ([{"offset": 37066, "quantity": 4, "outcome": "ANSWERED", "words": [0, 0, 0, 0]}], {"battery_group_37066_4": "zero_word_is_not_a_battery_value"}),
    ([{"offset": 37066, "quantity": 4, "outcome": "ANSWERED", "words": [0, 0, 0, 1]}], {"battery_group_37066_4": "group_native_only_no_catalog_row"}),
    ([{"offset": 37000, "quantity": 30, "outcome": "EXCEPTION", "exception_code": 3}], {"battery_group_37000_30": "battery_group_exception_no_negation"}),
    ([{"offset": 47000, "quantity": 1, "outcome": "FAILED"}, {"offset": 37000, "quantity": 1, "outcome": "NOT_READ"}], {"battery_group_47000_1": "battery_group_not_read", "battery_single_read_37000_1": "battery_group_not_read"}),
])
def test_reads_select_the_reason_and_never_a_value(reads, expected):
    document = case()
    document["reads"] = reads
    output = project(document)
    rows = {row["native_id"]: row.get("reason") for row in output["native_dispositions"]}
    for native_id, reason in expected.items():
        assert rows[native_id] == "storage.reason." + reason
    assert output["facts"] == []
    assert not any("quantity" in str(item) for item in output["facts"])


def test_withheld_items_keep_their_reason_whatever_is_read():
    document = case()
    document["reads"] = [{"offset": 37000, "quantity": 30, "outcome": "ANSWERED", "words": [1] * 30}]
    rows = {row["native_id"]: row["reason"] for row in project(document)["native_dispositions"]}
    assert rows["site_battery_soc"] == "storage.reason.battery_group_not_read"
    assert rows["site_battery_power"] == "storage.reason.sign_not_graded"
    assert rows["site_rated_storage_capacity"] == "storage.reason.battery_group_not_read"
    assert rows["site_battery_energy_charged_day"] == "storage.reason.period_counter_time_domain_not_represented"


@parametrize("read", [
    {"offset": 30369, "quantity": 1, "outcome": "ANSWERED", "words": [7550]},
    {"offset": 30368, "quantity": 2, "outcome": "ANSWERED", "words": [7550, 0]},
    {"offset": 37000, "quantity": 29, "outcome": "ANSWERED", "words": [0] * 29},
    {"offset": 37066, "quantity": 4, "outcome": "ANSWERED", "words": [1, 2]},
    {"offset": 37066, "quantity": 4, "outcome": "ANSWERED", "words": [1, 2, 3, 70000]},
    {"offset": 37066, "quantity": 4, "outcome": "ANSWERED", "words": [1, 2, 3, True]},
    {"offset": 37066, "quantity": 4, "outcome": "ANSWERED"},
    {"offset": 37066, "quantity": 4, "outcome": "EXCEPTION"},
    {"offset": 37066, "quantity": 4, "outcome": "FAILED", "words": [1, 2, 3, 4]},
    {"offset": 37066, "quantity": 4, "outcome": "ANSWERED", "words": [1, 2, 3, 4], "exception_code": 3},
    {"offset": 37066, "quantity": 4, "outcome": "TIMEOUT"},
    None,
])
def test_invalid_or_planned_reads_are_rejected(read):
    document = case()
    document["reads"] = [read]
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


def test_a_duplicate_read_is_rejected():
    document = case()
    read = {"offset": 37066, "quantity": 4, "outcome": "FAILED"}
    document["reads"] = [read, copy.deepcopy(read)]
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


@parametrize("outcome,message", [("PROFILE_SELECTED", None), ("INSUFFICIENT_EVIDENCE", None), ("NO_ADMISSIBLE_PROFILE", None), ("AMBIGUOUS", None), ("", "native_evidence_invalid"), (None, "native_evidence_invalid"), ("OFFLINE_PROFILE_ADMITTED", "native_evidence_invalid")])
def test_only_the_registry_outcomes_are_accepted(outcome, message):
    document = case()
    document["luna_selection"]["outcome"] = outcome
    if message is None:
        project(document)
        return
    with Raises(ValueError, match=message):
        project(document)


def published(document):
    return {fact["provenance"]["native_id"]: fact for fact in project(document)["facts"]}


def value_of(fact):
    return fraction_of(fact["value"]["number"])


def with_reads(*reads, outcome=None):
    document = case()
    document["reads"] = list(reads)
    if outcome:
        document["luna_selection"]["outcome"] = outcome
    return document


def words_at(length, index, values):
    words = [0] * length
    words[index:index + len(values)] = values
    return words


def test_a_documented_state_of_charge_publishes_a_ratio_on_its_pack():
    facts = published(with_reads({"offset": 30368, "quantity": 1, "outcome": "ANSWERED", "words": [7550]}))
    fact = facts["site_battery_soc"]
    assert fact["item_id"] == "storage.state.soc" and fact["value"]["unit"] == "unit.ratio"
    assert value_of(fact) == Fraction(755, 1000)
    assert fact["subject"] == {"dimension": "storage.dimension.pack", "resource": "resource:demo-pack-gamma-1"}
    assert fact["provenance"] == {"native_id": "site_battery_soc", "aggregation_basis": "site_group"} and fact["grade"] == "hypothesis"


@parametrize("raw,watts", [(2500, 2500), (-1500, -1500), (1, 1)])
def test_plant_battery_power_keeps_its_documented_sign_in_watts(raw, watts):
    read = {"offset": 37498, "quantity": 8, "outcome": "ANSWERED", "words": words_at(8, 6, [(raw & 0xFFFFFFFF) >> 16, raw & 0xFFFF])}
    fact = published(with_reads(read))["plant_battery_power"]
    assert fact["value"]["unit"] == "unit.watt" and value_of(fact) == watts
    assert fact["provenance"]["aggregation_basis"] == "plant_group" and fact["provenance"]["sign"] == "positive_while_charging_negative_while_discharging"


def test_site_lifetime_counters_publish_joules_and_the_day_counters_stay_withheld():
    words = words_at(46, 2, [0, 0, 1, 0xE240])
    words[8:12] = [0, 0, 9, 0xFBF1]
    output = project(with_reads({"offset": 30306, "quantity": 46, "outcome": "ANSWERED", "words": words}))
    facts = {fact["provenance"]["native_id"]: fact for fact in output["facts"]}
    assert value_of(facts["site_battery_energy_charged_total"]) == Fraction(123456, 100) * 3600000
    assert value_of(facts["site_battery_energy_discharged_total"]) == Fraction(654321, 100) * 3600000
    rows = {row["native_id"]: row for row in output["native_dispositions"]}
    assert rows["site_battery_energy_charged_day"]["reason"] == "storage.reason.period_counter_time_domain_not_represented"
    assert "storage.pack.power" not in {fact["item_id"] for fact in output["facts"]}


def test_the_site_power_group_states_no_sign_so_its_battery_power_is_withheld():
    output = project(with_reads({"offset": 30354, "quantity": 12, "outcome": "ANSWERED", "words": words_at(12, 6, [0, 1500])}))
    assert output["facts"] == []
    assert {row["native_id"]: row["reason"] for row in output["native_dispositions"]}["site_battery_power"] == "storage.reason.sign_not_graded"


@parametrize("outcome", ["INSUFFICIENT_EVIDENCE", "NO_ADMISSIBLE_PROFILE", "AMBIGUOUS"])
def test_an_unresolved_selection_withholds_a_documented_reading(outcome):
    output = project(with_reads({"offset": 30368, "quantity": 1, "outcome": "ANSWERED", "words": [7550]}, outcome=outcome))
    assert output["facts"] == []
    assert {row["native_id"]: row["reason"] for row in output["native_dispositions"]}["site_battery_soc"] == "storage.reason.no_admitted_battery_profile"


def test_a_zero_or_out_of_range_or_unread_state_of_charge_is_withheld_never_published():
    for words, reason in (([0], "zero_word_is_not_a_battery_value"), ([10001], "value_outside_pack_range")):
        output = project(with_reads({"offset": 30368, "quantity": 1, "outcome": "ANSWERED", "words": words}))
        assert output["facts"] == []
        assert {row["native_id"]: row["reason"] for row in output["native_dispositions"]}["site_battery_soc"] == "storage.reason." + reason
    exception = project(with_reads({"offset": 30368, "quantity": 1, "outcome": "EXCEPTION", "exception_code": 2}))
    assert exception["facts"] == []


@parametrize("packs", [[], ["resource:demo-pack-gamma-1", "resource:demo-pack-gamma-1"], ["resource:demo-pack-gamma-1", "resource:2", "resource:3", "resource:4"], ["storage-asset-demo-gamma"]])
def test_a_pack_identity_is_required_and_distinct(packs):
    document = case()
    document["identity"]["resources"]["packs"] = packs
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


def test_the_battery_power_sign_is_graded_hypothesis_and_the_aggregation_basis_is_provenance():
    decisions = {decision["precondition"]: decision for decision in v.load()["admission"]["decisions"]}
    assert decisions["battery_power_sign"]["grade"] == "hypothesis"
    assert decisions["aggregation_basis"]["decision"] == "site_or_plant_group_recorded_as_provenance"
    assert set(v.AGGREGATION_BASIS) == v.ACTIVE


def test_capacity_is_never_converted_to_energy_units_or_published():
    for member, value in (("request_conversion", "ampere_hour_to_kilowatt_hour"), ("request_conversion", "voltage_times_ampere_hour"), ("request_item", "storage.capacity.charge")):
        document = case()
        document[member] = value
        with Raises(ValueError, match="unsupported_or_withheld"):
            project(document)
    output = project(case())
    assert not [item for item in output["requested"] if item["item_id"].startswith("storage.capacity.")]
    assert {row["native_id"]: row["reason"] for row in output["native_dispositions"]}["site_rated_storage_capacity"] == "storage.reason.battery_group_not_read"


@parametrize("words,joules", [([0, 10000], 36000000), ([0, 7500], 27000000), ([1, 0], 235929600)])
def test_a_supplied_rated_storage_capacity_publishes_its_native_energy_in_joules(words, joules):
    output = project(with_reads({"offset": 30322, "quantity": 2, "outcome": "ANSWERED", "words": words}))
    facts = [fact for fact in output["facts"] if fact["item_id"] == "storage.energy.nominal_capacity"]
    assert len(facts) == 1 and facts[0]["value"]["unit"] == "unit.joule" and facts[0]["provenance"]["aggregation_basis"] == "site_group"
    number = facts[0]["value"]["number"]
    assert int(number["coefficient"]) * 10 ** number["exponent10"] == joules
    held = {item["item_id"]: item for item in output["dispositions"]}["storage.energy.nominal_capacity"]
    assert held["outcome"] == "transformed" and held["source_keys"] == ["site_rated_storage_capacity"]


def test_a_missing_or_zero_rated_storage_capacity_is_withheld_not_a_missing_catalog_element():
    withheld = {row["native_id"]: row for row in project(with_reads({"offset": 30322, "quantity": 2, "outcome": "ANSWERED", "words": [0, 0]}))["native_dispositions"]}
    assert withheld["site_rated_storage_capacity"]["reason"] == "storage.reason.zero_word_is_not_a_battery_value"
    unbound = {row["native_id"]: row for row in project(with_reads({"offset": 30322, "quantity": 2, "outcome": "EXCEPTION", "exception_code": 2}))["native_dispositions"]}
    assert unbound["site_rated_storage_capacity"]["reason"] == "storage.reason.battery_group_exception_no_negation"
    assert "storage.reason.no_energy_capacity_fact_in_pack" not in v.load()["reasons"]


SOC_READ = {"offset": 30368, "quantity": 1, "outcome": "ANSWERED", "words": [7550]}
CAPACITY_READ = {"offset": 30322, "quantity": 2, "outcome": "ANSWERED", "words": [0, 10000]}


@parametrize("packs", [["resource:demo-pack-gamma-1", "resource:demo-pack-gamma-2"], ["resource:demo-pack-gamma-2", "resource:demo-pack-gamma-1"], ["resource:demo-pack-gamma-1", "resource:demo-pack-gamma-2", "resource:demo-pack-gamma-3"]])
def test_an_aggregate_reading_is_never_assigned_to_a_pack_by_list_position(packs):
    document = with_reads(SOC_READ, CAPACITY_READ)
    document["identity"]["resources"]["packs"] = packs
    output = project(document)
    assert output["facts"] == []
    rows = {row["native_id"]: row["reason"] for row in output["native_dispositions"]}
    assert rows["site_battery_soc"] == rows["site_rated_storage_capacity"] == "storage.reason.aggregate_without_pack_evidence"
    assert {item["item_id"]: item["reason"] for item in output["dispositions"]}["storage.state.soc"] == "storage.reason.aggregate_without_pack_evidence"


def test_the_single_pack_of_the_configured_battery_holds_the_aggregate_reading():
    output = project(with_reads(SOC_READ))
    assert [(fact["item_id"], fact["subject"]["resource"]) for fact in output["facts"]] == [("storage.state.soc", "resource:demo-pack-gamma-1")]


def test_no_limit_interlock_or_operation_is_requested_or_available():
    for operation in ("storage.operation.set_charge_limit", "storage.operation.set_discharge_limit"):
        document = case()
        document["request_operation"] = operation
        with Raises(ValueError, match="unsupported_or_withheld"):
            project(document)
    requested = {item["item_id"] for item in project(case())["requested"]}
    assert requested == set(v.REQUESTED) and not requested & set(v.NEVER_REQUESTED)
    for item in v.NEVER_REQUESTED:
        document = case()
        document["request_item"] = item
        with Raises(ValueError, match="unsupported_or_withheld"):
            project(document)


def test_only_the_active_rows_are_in_force_and_every_row_matches_the_pack():
    fields, operations = v.pack_tables()
    document = v.load()
    seen = set()
    for rule in document["field_rules"]:
        plan = rule.get("prospective")
        if not plan:
            continue
        assert plan["in_force"] is (rule["native_id"] in v.ACTIVE)
        assert plan["dimension"] == fields[plan["target"]]["dimension"] == "storage.dimension.pack"
        assert plan.get("unit") == fields[plan["target"]].get("unit")
        seen.add(plan["target"])
    assert seen == set(v.REQUESTED)
    assert "storage.pack.power" in seen and "storage.capacity.charge" not in seen and "storage.capacity.discharge" not in seen
    assert not any(token in str(document["field_rules"]) for token in ("storage.limit.", "storage.status.interlock", "storage.operation."))


def test_the_conversion_table_states_an_exact_factor_and_offset():
    assert v.UNIT_CONVERSION == CONVERSIONS
    assert not hasattr(v, "UNIT_SHIFT")
    assert fraction_of(CONVERSIONS[("%", "unit.ratio")]["factor"]) * 100 == 1
    assert fraction_of(CONVERSIONS[("kW", "unit.watt")]["factor"]) == 1000
    assert fraction_of(CONVERSIONS[("kWh", "unit.joule")]["factor"]) == 3600000
    assert all(fraction_of(entry["offset"]) == 0 for entry in CONVERSIONS.values())


def test_each_prospective_row_states_its_si_unit_conversion_and_losses():
    fields, _ = v.pack_tables()
    rows = {}
    for rule in v.load()["field_rules"]:
        plan = rule.get("prospective")
        if not plan:
            continue
        rows.setdefault(plan["target"], []).append(plan)
        if fields[plan["target"]]["kind"] == "symbol":
            assert "conversion" not in plan and "loss" not in plan and plan["unit"] is None
            continue
        assert plan["conversion"] == CONVERSIONS[(plan["native"]["unit"], plan["unit"])]
        assert "unit" in plan["loss"] and plan["loss"][0] == "unit"
    assert [row["unit"] for row in rows["storage.state.soc"]] == ["unit.ratio"]
    assert [row["loss"] for row in rows["storage.pack.power"]] == [["unit", "provenance"]] * 2
    assert [(row["unit"], row["loss"]) for row in rows["storage.energy.charged"] + rows["storage.energy.discharged"]] == [("unit.joule", ["unit", "policy"])] * 2


def test_a_full_scale_native_value_converts_exactly():
    """A synthetic native maximum through each row's factor and offset, computed with exact fractions."""
    for rule in v.load()["field_rules"]:
        plan = rule.get("prospective")
        if not plan or not plan.get("conversion"):
            continue
        native = plan["native"]
        raw = {"U16": 0xFFFF, "U32": 0xFFFFFFFF, "I32": 0x7FFFFFFF, "U64": 0xFFFFFFFFFFFFFFFF}[native["type"]]
        value = Fraction(raw, native["gain"])
        canonical = value * fraction_of(plan["conversion"]["factor"]) + fraction_of(plan["conversion"]["offset"])
        assert support.kernel_decimal(canonical)
        assert (canonical - fraction_of(plan["conversion"]["offset"])) / fraction_of(plan["conversion"]["factor"]) == value


@parametrize("name,mutation,message", [
    # Key field native unit: both statements of the native form name a unit with no entry for the canonical unit.
    ("native-unit", lambda d: [set_path(d, "field_rules.4.native.unit", "permille"), set_path(d, "field_rules.4.prospective.native.unit", "permille")], "prospective conversion"),
    ("native-unit-energy", lambda d: [set_path(d, "field_rules.7.native.unit", "Wh"), set_path(d, "field_rules.7.prospective.native.unit", "Wh")], "prospective conversion"),
    # Key field canonical unit: a row naming a customary unit, or an SI unit other than the pack field's.
    ("canonical-unit-percent", lambda d: set_path(d, "field_rules.4.prospective.unit", "unit.percent"), "prospective target"),
    ("canonical-unit-kilowatt-hour", lambda d: set_path(d, "field_rules.7.prospective.unit", "unit.kilowatt_hour"), "prospective target"),
    ("canonical-unit-other-si", lambda d: set_path(d, "field_rules.8.prospective.unit", "unit.watt"), "prospective target"),
    # Value field factor: coefficient and exponent each varied.
    ("factor-coefficient", lambda d: set_path(d, "field_rules.7.prospective.conversion.factor", {"coefficient": "1", "exponent10": 5}), "prospective conversion"),
    ("factor-exponent", lambda d: set_path(d, "field_rules.7.prospective.conversion.factor", {"coefficient": "36", "exponent10": 4}), "prospective conversion"),
    ("factor-identity", lambda d: set_path(d, "field_rules.4.prospective.conversion.factor", ONE), "prospective conversion"),
    ("factor-percent-exponent", lambda d: set_path(d, "field_rules.4.prospective.conversion.factor", {"coefficient": "1", "exponent10": -1}), "prospective conversion"),
    # Value field offset: coefficient and exponent each varied.
    ("offset-coefficient", lambda d: set_path(d, "field_rules.5.prospective.conversion.offset", {"coefficient": "1", "exponent10": 0}), "prospective conversion"),
    ("offset-exponent", lambda d: set_path(d, "field_rules.5.prospective.conversion.offset", {"coefficient": "1", "exponent10": -2}), "prospective conversion"),
    ("conversion-missing", lambda d: v.mutate(d, {"path": "field_rules.6.prospective.conversion", "remove": True}), "prospective conversion"),
    ("conversion-extra-member", lambda d: d["field_rules"][8]["prospective"]["conversion"].update(rounding="half_even"), "prospective conversion"),
    ("row-extra-member", lambda d: d["field_rules"][4]["prospective"].update(scale=-2), "prospective conversion"),
    ("loss-missing", lambda d: v.mutate(d, {"path": "field_rules.4.prospective.loss", "remove": True}), "prospective conversion"),
    ("loss-kind", lambda d: set_path(d, "field_rules.4.prospective.loss", ["unit", "native_enum"]), "prospective conversion"),
    ("loss-duplicated", lambda d: set_path(d, "field_rules.4.prospective.loss", ["unit", "unit"]), "prospective conversion"),
    ("silent-ratio-conversion", lambda d: set_path(d, "field_rules.4.prospective.loss", []), "silent unit conversion"),
    ("silent-power-conversion", lambda d: set_path(d, "field_rules.6.prospective.loss", ["provenance"]), "silent unit conversion"),
    ("silent-energy-conversion", lambda d: set_path(d, "field_rules.8.prospective.loss", ["policy"]), "silent unit conversion"),
    ("power-without-provenance-loss", lambda d: set_path(d, "field_rules.5.prospective.loss", ["unit"]), "prospective sign"),
    ("counter-without-policy-loss", lambda d: set_path(d, "field_rules.7.prospective.loss", ["unit"]), "prospective counter"),
    ("symbol-with-conversion", lambda d: d["field_rules"][0]["prospective"].update(conversion={"factor": ONE, "offset": ZERO}), "prospective symbols"),
])
def test_conversion_key_and_value_mutations_are_rejected(name, mutation, message):
    document = contract()
    mutation(document)
    with Raises(ValueError) as error:
        v.validate_contract(document)
    assert message in str(error.value), str(error.value)


@parametrize("asset", ["", None, "storage asset", "storage-asset-serial-1", "storage-asset-ESN-1", "storage-asset-10.2.3.4", "storage-asset-host:502", "source:gateway-demo-c", "binding:semantic-storage:demo-gamma", "storage-asset-luna", "storage-asset-huawei"])
def test_asset_identifier_never_comes_from_forbidden_inputs(asset):
    document = case()
    document["identity"]["asset_id"] = asset
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


@parametrize("path,value", [("source.source_id", ""), ("source.source_id", "source:192.168.0.1"), ("identity.binding_id", None), ("identity.binding_id", "storage-asset-demo-gamma"), ("source", None)])
def test_source_and_binding_identifiers_fail_closed(path, value):
    document = case()
    set_path(document, path, value)
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


@parametrize("path,value", [("lifecycle.observation_id", ""), ("lifecycle.observation_revision", 0), ("lifecycle.receipt_wall", "2026-01-01T00:00:00"), ("lifecycle.receipt_wall", None), ("lifecycle.receipt_monotonic", 0), ("lifecycle.clock_epoch", "bad epoch"), ("lifecycle.source_epoch", None), ("lifecycle.driver_generation", True), ("lifecycle.connection_generation", 0)])
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


def test_first_error_of_the_precedence_list_wins():
    document = case()
    document["luna_selection"]["outcome"] = "AMBIGUOUS"
    document["identity"]["asset_id"] = ""
    document["request_operation"] = "storage.operation.set_charge_limit"
    document["reads"] = [{"offset": 1, "quantity": 1, "outcome": "ANSWERED", "words": [0]}]
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)
    document["reads"] = []
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)
    document["identity"]["asset_id"] = "storage-asset-demo-gamma"
    with Raises(ValueError, match="unsupported_or_withheld"):
        project(document)


def test_scenario_expectations_are_executed_not_trusted():
    for damage, message in (
        (lambda d: d["scenarios"][0]["expect"]["output"]["native_dispositions"][0].update({"reason": "storage.reason.zero_word_is_not_a_battery_value"}), "scenario output"),
        (lambda d: d["scenarios"][1]["expect"]["summary"]["native_dispositions"].pop(), "scenario summary"),
        (lambda d: d["scenarios"][-1]["expect"].update({"error": "lifecycle_missing"}), "native_evidence_invalid"),
    ):
        document = contract()
        damage(document)
        with Raises(ValueError) as error:
            v.validate_scenarios(document)
        assert message in str(error.value)


def test_scenario_coverage_is_enforced():
    for dropped, message in (("STG-POS-007-zero-words-without-battery", "cover every reason"), ("STG-POS-009-battery-read-exception", "cover every reason"), ("STG-POS-011-state-of-charge-outside-range", "cover every reason"), ("STG-NEG-006-lifecycle-missing", "cover every error")):
        document = contract()
        document["scenarios"] = [s for s in document["scenarios"] if s["id"] != dropped]
        with Raises(ValueError, match=message):
            v.validate_scenarios(document)


def test_a_negative_scenario_that_is_accepted_fails():
    document = contract()
    next(s for s in document["scenarios"] if s["id"] == "STG-NEG-004-asset-missing")["mutations"] = []
    with Raises(ValueError, match="accepted"):
        v.validate_scenarios(document)


def page():
    return v.DOC.read_text(encoding="utf-8")


@parametrize("old,new", [
    ("| `storage.energy.nominal_capacity` | `storage.dimension.pack` | `site_rated_storage_capacity` | U32, kWh, gain 1000 | `unit.joule`, factor 3600000, offset 0 | unit |", "| `storage.energy.nominal_capacity` | `storage.dimension.pack` | `site_rated_storage_capacity` | U32, kWh, gain 1000 | `unit.joule`, factor 360000, offset 0 | unit |"),
    ("| `storage.state.soc` | `storage.dimension.pack` | `site_battery_soc` | U16, %, gain 100 | `unit.ratio`, factor 0.01, offset 0 | unit | none | Hypothesis |", "| `storage.state.soc` | `storage.dimension.pack` | `site_battery_soc` | U16, %, gain 10 | `unit.ratio`, factor 0.01, offset 0 | unit | none | Hypothesis |"),
    ("| `storage.state.soc` | `storage.dimension.pack` | `site_battery_soc` | U16, %, gain 100 | `unit.ratio`, factor 0.01, offset 0 | unit | none | Hypothesis |", "| `storage.state.soc` | `storage.dimension.pack` | `site_battery_soc` | U16, %, gain 100 | `unit.percent`, factor 1, offset 0 | none | none | Hypothesis |"),
    ("| `storage.energy.charged` | `storage.dimension.pack` | `site_battery_energy_charged_total` | U64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy |", "| `storage.energy.charged` | `storage.dimension.pack` | `site_battery_energy_charged_total` | U64, kWh, gain 100 | `unit.joule`, factor 360000, offset 0 | unit+policy |"),
    ("| `storage.pack.power` | `storage.dimension.pack` | `site_battery_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit+provenance | `unknown_no_value_without_a_graded_sign` | Hypothesis |", "| `storage.pack.power` | `storage.dimension.pack` | `site_battery_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `unknown_no_value_without_a_graded_sign` | Hypothesis |"),
    ("| `storage.pack.power` | `storage.dimension.pack` | `site_battery_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit+provenance | `unknown_no_value_without_a_graded_sign` | Hypothesis |", "| `storage.pack.power` | `storage.dimension.pack` | `site_battery_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit+provenance | `positive_while_charging_negative_while_discharging` | Hypothesis |"),
    ("| `STG-NEG-011-error-precedence` |", "| `STG-NEG-099-other` |"),
    ("rated storage capacity is mapped in joules", "rated storage capacity is withheld"),
    ("never converted", "converted"),
    ("outbound_allowed=false", "outbound allowed"),
])
def test_page_must_match_the_contract(old, new):
    pattern = re.escape(old).replace(r"\ ", r"\s+")
    mutated, count = re.subn(pattern, lambda _: new, page())
    assert count >= 1
    with Raises(ValueError):
        v.check_document(v.load(), mutated)


def test_a_dropped_table_row_is_rejected():
    for prefix in ("| `battery_group_37066_4` |", "| `storage.energy.discharged` |"):
        lines = [line for line in page().splitlines(keepends=True) if not line.startswith(prefix)]
        with Raises(ValueError, match="document tables"):
            v.check_document(v.load(), "".join(lines))


@parametrize("literal", ["b" * 40, "0123456789abcdef" * 4, "F0" * 20])
def test_publication_rule_rejects_revision_and_digest_literals(literal):
    with Raises(ValueError, match="publication"):
        v.check_document(v.load(), page() + "\n" + literal + "\n")


def test_published_files_cite_no_revision_link_or_endpoint():
    for path in (v.PATH, v.DOC):
        text = path.read_text(encoding="utf-8")
        assert not v.HEX_RUN_RE.search(v.unowned_text(text))
        assert "/blob/" not in v.unowned_text(text) and "/commit/" not in text and "#L" not in text and "tcp://" not in text


# The member name used before the in-place rename, assembled so that no file spells it.
EARLIER_MEMBER = "relation" + "ships"


def test_expected_output_rejects_the_earlier_relation_member():
    document = contract()
    expected = next(s for s in document["scenarios"] if s["id"] == "STG-POS-001-no-battery-read")["expect"]["output"]
    expected[EARLIER_MEMBER] = expected.pop("relations")
    with Raises(ValueError) as error:
        v.validate_scenarios(document)
    assert str(error.value) == "scenario output STG-POS-001-no-battery-read"


GRAPH_SPEC = importlib.util.spec_from_file_location("mapping_gate_graph_rules", ROOT / "scripts/mapping/mapping_gate_graph_rules.py")
graph_rules = importlib.util.module_from_spec(GRAPH_SPEC)
GRAPH_SPEC.loader.exec_module(graph_rules)


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


def test_the_battery_is_relayed_by_the_inverter_and_exposes_no_anchor_key_source():
    graph = contract()["graph"]
    assert [resource["kind"] for resource in graph["resources"]] == ["resource.inverter", "resource.battery", "resource.battery_pack", "resource.storage_system", "resource.storage_interface"]
    assert {resource["kind"] for resource in graph["resources"] if resource["anchor_source"] == "native_position"} == {"resource.battery_pack", "resource.storage_interface"}
    assert {resource["kind"] for resource in graph["resources"] if resource["anchor_source"] == "operator_label"} == {"resource.inverter", "resource.battery", "resource.storage_system"}
    assert graph["relayed"] == [{"relaying_kind": "resource.inverter", "relayed_kind": "resource.battery", "direct_anchor_key_source": "none"}]
    assert graph["joins"] == [{"kind": "resource.battery", "observations": ["relayed_by_the_inverter", "seen_through_another_path"], "basis": "operator_declaration", "native_tokens": "not_exposed"}]
    relays = graph["relations"][0]
    assert (relays["definition"], relays["basis_class"], relays["from_kind"], relays["to_kind"]) == ("relation.relays", "configuration", "resource.inverter", "resource.battery")
    assert all(relation["absence_sides"] == [] for relation in graph["relations"])
    assert graph["aggregates"] == []


def relation_record(definition, from_kind, to_kind):
    return {"definition": definition, "basis_class": "gate_rule", "from_kind": from_kind, "to_kind": to_kind, "evidence": ["synthetic:relation"],
            "validity_scope": {"profile": "huawei.luna2000", "version": "no_admitted_profile", "rule_scope": "synthetic"}, "absence_sides": []}


PACK_POWER_AGGREGATE = {"fact": "storage.pack.power", "subject_kind": "resource.battery_pack", "coverage": []}

# (name, edits, expected error). Indices: resources inverter, battery, battery pack, storage system, interface; relations relays, battery to pack, pack to interface, system to battery;
# selectors profile identifier, class token; prospective rows at field rules 0, 4, 5, 6, 7 and 8.
GRAPH_NEGATIVES = [
    # False join: a battery joined to a second view by name or by position, or over differing native tokens.
    ("battery_joined_by_name", [("set", "graph.joins.0.basis", "display_name")], "graph join_basis_not_allowed $.graph.joins[0].basis"),
    ("battery_joined_by_label_text", [("set", "graph.joins.0.basis", "operator_label")], "graph join_basis_not_allowed $.graph.joins[0].basis"),
    ("battery_joined_by_position", [("set", "graph.joins.0.basis", "native_position")], "graph join_basis_not_allowed $.graph.joins[0].basis"),
    ("battery_joined_over_differing_tokens", [("set", "graph.joins.0.native_tokens", "differ")], "graph false_join $.graph.joins[0].native_tokens"),
    ("battery_joined_on_a_shared_key_the_path_does_not_expose", [("set", "graph.joins.0.basis", "same_anchor_key"), ("set", "graph.joins.0.native_tokens", "equal")], "graph_joins"),
    ("second_view_two_identities", [("set", "graph.joins", [])], "graph_joins"),
    ("join_of_one_view", [("set", "graph.joins.0.observations", ["relayed_by_the_inverter"])], "graph invalid_value $.graph.joins[0].observations"),
    ("join_of_the_inverter", [("set", "graph.joins.0.kind", "resource.inverter")], "graph_joins"),
    # Dangling endpoint: a module or cell relation whose endpoint the gate does not publish.
    ("dangling_module_cell", [("append", "graph.relations", relation_record("storage.relation.module_cell", "resource.battery_module", "resource.battery_cell"))],
     "graph dangling_endpoint $.graph.relations[4].from_kind"),
    ("dangling_pack_module", [("append", "graph.relations", relation_record("storage.relation.pack_module", "resource.battery_pack", "resource.battery_module"))],
     "graph dangling_endpoint $.graph.relations[4].to_kind"),
    ("dangling_battery_after_kind_change", [("set", "graph.resources.1.kind", "resource.meter")], "graph dangling_endpoint $.graph.relations[0].to_kind"),
    # Resources (section 15 item 1): kind, class, basis, source, scheme, parent and evidence each varied.
    ("battery_kind_not_member", [("set", "graph.resources.1.kind", "resource.not_a_catalog_kind")], "graph kind_not_member $.graph.resources[1].kind"),
    ("battery_anchor_class", [("set", "graph.resources.1.anchor_class", "component")], "graph anchor_class_mismatch $.graph.resources[1].anchor_class"),
    ("battery_anchored_on_position", [("set", "graph.resources.1.anchor_basis", "native_slot"), ("set", "graph.resources.1.anchor_source", "native_position")], "graph_anchor"),
    ("battery_anchored_on_routed_unit", [("set", "graph.resources.1.anchor_source", "routed_unit")], "graph anchor_source_not_allowed $.graph.resources[1].anchor_source"),
    ("battery_anchored_on_address", [("set", "graph.resources.1.anchor_source", "network_address")], "graph anchor_source_not_allowed $.graph.resources[1].anchor_source"),
    ("battery_basis_source_mismatch", [("set", "graph.resources.1.anchor_basis", "native_identity")], "graph anchor_basis_mismatch $.graph.resources[1].anchor_basis"),
    ("battery_native_token_without_scheme", [("set", "graph.resources.1.anchor_basis", "native_identity"), ("set", "graph.resources.1.anchor_source", "native_token")],
     "graph scheme_not_listed $.graph.resources[1].identification_scheme"),
    ("battery_scheme_unlisted", [("set", "graph.resources.1.identification_scheme", {"id": "scheme.unlisted", "issuer_registry": "none"})], "graph scheme_not_listed $.graph.resources[1].identification_scheme"),
    ("battery_parent_inverter", [("set", "graph.resources.1.parent_kind", "resource.inverter")], "graph_parent"),
    ("pack_parent_battery", [("set", "graph.resources.2.parent_kind", "resource.battery")], "graph_parent"),
    ("battery_relay_statement_dropped", [("set", "graph.resources.1.evidence", ["registry_disposition:battery_groups_present_not_admitted", "native_owner_token_stability:no_battery_token_on_the_relaying_path"])], "graph_evidence"),
    ("battery_token_stability_unstated", [("delete", "graph.resources.1.evidence.2", None)], "graph_evidence"),
    ("pack_token_stability_unstated", [("delete", "graph.resources.2.evidence.1", None)], "graph_evidence"),
    ("inverter_future_token_unstated", [("delete", "graph.resources.0.evidence.1", None)], "graph_evidence"),
    ("battery_evidence_empty", [("set", "graph.resources.1.evidence", [])], "graph invalid_value $.graph.resources[1].evidence"),
    ("pack_resource_dropped", [("delete", "graph.resources.2", None)], "graph dangling_endpoint $.graph.relations[1].to_kind"),
    ("module_resource_added", [("append", "graph.resources", {"kind": "resource.battery_module", "anchor_class": "component", "anchor_basis": "native_slot", "anchor_source": "native_position",
                                                              "identification_scheme": "none_listed", "parent_kind": "resource.battery_pack", "evidence": ["synthetic:module"]})], "graph_resources"),
    # Relations (section 15 item 2): definition, basis class, both endpoints, evidence, each scope member and absence sides.
    ("relays_by_native_observation", [("set", "graph.relations.0.basis_class", "native_observation")], "graph_relations"),
    ("relays_by_gate_rule", [("set", "graph.relations.0.basis_class", "gate_rule")], "graph basis_class_not_admitted $.graph.relations[0].basis_class"),
    ("relays_reversed", [("set", "graph.relations.0.from_kind", "resource.battery"), ("set", "graph.relations.0.to_kind", "resource.inverter")], "graph_relations"),
    ("battery_pack_by_configuration", [("set", "graph.relations.1.basis_class", "configuration")], "graph basis_class_not_admitted $.graph.relations[1].basis_class"),
    ("battery_pack_from_inverter", [("set", "graph.relations.1.from_kind", "resource.inverter")], "graph endpoint_kind_mismatch $.graph.relations[1].from_kind"),
    ("relation_not_member", [("set", "graph.relations.1.definition", "storage.relation.invented")], "graph relation_not_member $.graph.relations[1].definition"),
    ("relation_evidence_empty", [("set", "graph.relations.0.evidence", [])], "graph invalid_value $.graph.relations[0].evidence"),
    ("scope_profile", [("set", "graph.relations.0.validity_scope.profile", "huawei.sun2000.m1")], "graph_relations"),
    ("scope_version", [("set", "graph.relations.1.validity_scope.version", "1.0.0")], "graph_relations"),
    ("scope_rule_blank", [("set", "graph.relations.0.validity_scope.rule_scope", "")], "graph invalid_value $.graph.relations[0].validity_scope.rule_scope"),
    ("scope_rule_widened", [("set", "graph.relations.1.validity_scope.rule_scope", "every_pack_of_the_site")], "graph_relations"),
    ("absence_claimed", [("set", "graph.relations.0.absence_sides", ["to"])], "graph_relations"),
    ("relation_dropped", [("delete", "graph.relations.1", None)], "graph_relations"),
    # The three required storage relations are stated, each with evidence and scope; a pack is anchored on its native position.
    ("pack_anchored_on_label", [("set", "graph.resources.2.anchor_basis", "operator_slot"), ("set", "graph.resources.2.anchor_source", "operator_label")], "graph_anchor"),
    ("pack_anchored_on_address", [("set", "graph.resources.2.anchor_source", "network_address")], "graph anchor_source_not_allowed $.graph.resources[2].anchor_source"),
    ("pack_interface_dropped", [("delete", "graph.relations.2", None)], "graph_relations"),
    ("pack_interface_by_configuration", [("set", "graph.relations.2.basis_class", "configuration")], "graph basis_class_not_admitted $.graph.relations[2].basis_class"),
    ("pack_interface_evidence_empty", [("set", "graph.relations.2.evidence", [])], "graph invalid_value $.graph.relations[2].evidence"),
    ("pack_interface_scope_widened", [("set", "graph.relations.2.validity_scope.rule_scope", "every_interface")], "graph_relations"),
    ("system_battery_dropped", [("delete", "graph.relations.3", None)], "graph_relations"),
    ("system_battery_by_gate_rule", [("set", "graph.relations.3.basis_class", "gate_rule")], "graph basis_class_not_admitted $.graph.relations[3].basis_class"),
    ("system_battery_evidence_empty", [("set", "graph.relations.3.evidence", [])], "graph invalid_value $.graph.relations[3].evidence"),
    ("system_resource_dropped", [("delete", "graph.resources.3", None)], "graph dangling_endpoint $.graph.relations[3].from_kind"),
    ("interface_parent_battery", [("set", "graph.resources.4.parent_kind", "resource.battery")], "graph_parent"),
    ("unstated_pack_interface_returned", [("append", "unstated_relations", {"definition": "storage.relation.pack_interface", "from_kind": "resource.battery_pack", "to_kind": "resource.storage_interface", "outcome": "unknown_not_absent", "withheld_by": "native_contract:none"})], "unstated_relations"),
    # Relayed (section 15 item 3).
    ("relayed_direct_key_claimed", [("set", "graph.relayed.0.direct_anchor_key_source", "operator_label")], "graph_relayed"),
    ("relayed_direct_key_address", [("set", "graph.relayed.0.direct_anchor_key_source", "network_address")], "graph anchor_source_not_allowed $.graph.relayed[0].direct_anchor_key_source"),
    ("relayed_dropped", [("set", "graph.relayed", [])], "graph_relayed"),
    ("relayed_by_the_pack", [("set", "graph.relayed.0.relaying_kind", "resource.battery_pack")], "graph_relayed"),
    # Aggregates (section 15 item 4, EN-5) and selectors (item 5).
    ("member_as_aggregate", [("append", "graph.aggregates", PACK_POWER_AGGREGATE)], "graph not_an_aggregate $.graph.aggregates[0].fact"),
    ("class_token_as_identity_evidence", [("set", "graph.selectors.1.classification", "identity_evidence")], "graph_selectors"),
    ("profile_as_resource_attribute", [("set", "graph.selectors.0.classification", "resource_attribute")], "graph_selectors"),
    ("class_token_selector_dropped", [("delete", "graph.selectors.1", None)], "graph_selectors"),
    # Section 15 item 6 and PLN-1.
    ("plane_on_resource", [("set", "graph.resources.1.planes", ["physical"])], "graph plane_membership_stated $.graph.resources[1].planes"),
    ("plane_on_field_rule", [("set", "field_rules.4.planes", ["energy"])], "graph plane_membership_stated $.field_rules[4].planes"),
    # Section 7.2: the storage relations the gate does not state stay unknown, never absent.
    ("unstated_module_cell_dropped", [("delete", "unstated_relations.0", None)], "unstated_relations"),
    ("unstated_absent", [("set", "unstated_relations.1.outcome", "absent")], "unstated_relations"),
    ("unstated_endpoint", [("set", "unstated_relations.2.to_kind", "resource.battery_pack")], "unstated_relations"),
    ("unstated_reason", [("set", "unstated_relations.2.withheld_by", "unsupported:state_of_health")], "unstated_relations"),
    # Identity (section 6.3): one field of each identifier and each resource role varied.
    ("identity_asset_configured", [("set", "identity.asset_id.source", "gateway_configured_semantic_asset_id")], "identity contract"),
    ("identity_asset_granularity", [("set", "identity.asset_id.granularity", "one_asset_per_battery")], "identity contract"),
    ("identity_class_token_allowed", [("delete", "identity.asset_id.forbidden_derivations.7", None)], "identity contract"),
    ("identity_profile_allowed", [("delete", "identity.asset_id.forbidden_derivations.8", None)], "identity contract"),
    ("identity_source_configured", [("set", "identity.source_id.source", "gateway_configured_semantic_source_id")], "identity contract"),
    ("identity_source_granularity", [("set", "identity.source_id.granularity", "one_source_per_battery")], "identity contract"),
    ("identity_battery_dimension", [("set", "identity.resources.battery.dimension", "storage.dimension.pack")], "identity contract"),
    ("identity_battery_source", [("set", "identity.resources.battery.source", "gateway_derived_semantic_asset_id")], "identity contract"),
    ("identity_battery_optional", [("set", "identity.resources.battery.required", False)], "identity contract"),
    ("identity_inverter_kind", [("set", "identity.resources.inverter.kind", "resource.equipment")], "identity contract"),
    ("identity_packs_not_allocated", [("set", "identity.resources.packs.source", "not_allocated_no_profile_states_the_packs_of_a_battery")], "identity contract"),
    ("identity_pack_slots_open", [("set", "identity.resources.packs.slots", 4)], "identity contract"),
    ("identity_system_not_required", [("set", "identity.resources.system.required", False)], "identity contract"),
    ("identity_unresolved_granularity_restored", [("set", "identity.storage_asset_granularity", "unresolved_batteries_are_registers_of_the_inverter_units_not_units_of_their_own")], "identity contract"),
    ("identity_failure", [("set", "identity.failure", "missing_or_invalid_asset_or_source_identity_fails_closed")], "identity contract"),
    # EN-1: every prospective row names its subject kind and measurement point.
    ("row_subject_kind", [("set", "field_rules.4.prospective.subject_kind", "resource.battery")], "field rule subject"),
    ("row_subject_kind_inverter", [("set", "field_rules.5.prospective.subject_kind", "resource.inverter")], "field rule subject"),
    ("row_subject_missing", [("delete", "field_rules.7.prospective.subject_kind", None)], "field rule subject"),
    ("row_measurement_point_blank", [("set", "field_rules.6.prospective.measurement_point", "")], "field rule subject"),
    ("row_measurement_point_moved", [("set", "field_rules.8.prospective.measurement_point", "pack_as_a_whole")], "field rule subject"),
    ("row_measurement_point_missing", [("delete", "field_rules.4.prospective.measurement_point", None)], "field rule subject"),
    ("symbol_row_subject_kind", [("set", "field_rules.0.prospective.subject_kind", "resource.battery")], "field rule subject"),
]


@parametrize("name,edits,expected", GRAPH_NEGATIVES)
def test_graph_identity_and_subject_negatives_are_rejected(name, edits, expected):
    assert graph_rejection(edits) == expected, name


def catalog_with(element_id, mutate_attributes):
    catalog = copy.deepcopy(graph_rules.load_catalog())
    for element in catalog["elements"]:
        if element["ref"]["id"] == element_id:
            mutate_attributes(element["attributes"])
    return catalog


# Constants that repeat a catalog fact are lookups: a changed catalog entry changes the outcome.
@parametrize("element_id,mutate_attributes,expected", [
    ("storage.state.soc", lambda a: a["subject"].update(kind="resource.battery"), "field rule subject"),
    ("storage.state.soc", lambda a: a["subject"]["dimension"].update(id="storage.dimension.battery"), "prospective target"),
    ("storage.dimension.battery", lambda a: a.update(resource_kind="resource.storage_system"), "graph_resources"),
    ("storage.relation.battery_pack", lambda a: a.update(basis_classes=["configuration"]), "graph basis_class_not_admitted $.graph.relations[1].basis_class"),
    ("storage.relation.battery_pack", lambda a: a.update(to_kinds=["resource.battery_module"]), "graph endpoint_kind_mismatch $.graph.relations[1].to_kind"),
    ("storage.pack.power", lambda a: a.update(aggregation="aggregate"), "graph_aggregates"),
    ("storage.pack.power", lambda a: a.update(direction="not_applicable"), "prospective sign"),
    ("storage.state.soc", lambda a: a.update(sign_reference="native_convention"), "prospective sign"),
    ("storage.relation.pack_string", lambda a: a.update(from_kinds=["resource.battery"]), "unstated_relations"),
    ("relation.relays", lambda a: a.update(basis_classes=["native_observation"]), "graph basis_class_not_admitted $.graph.relations[0].basis_class"),
])
def test_catalog_facts_are_looked_up_not_repeated(element_id, mutate_attributes, expected):
    assert graph_rejection([], catalog_with(element_id, mutate_attributes)) == expected


def test_each_prospective_row_names_its_catalog_subject_and_a_measurement_point():
    catalog = graph_rules.load_catalog()
    facts = {element["ref"]["id"]: element["attributes"] for element in catalog["elements"] if element["element_kind"] == "fact"}
    rows = [(rule["native_id"], rule["prospective"]) for rule in contract()["field_rules"] if rule.get("prospective")]
    assert rows
    for native_id, plan in rows:
        assert plan["subject_kind"] == facts[plan["target"]]["subject"]["kind"] == "resource.battery_pack", native_id
        assert plan["dimension"] == facts[plan["target"]]["subject"]["dimension"]["id"], native_id
        assert isinstance(plan["measurement_point"], str) and plan["measurement_point"].startswith("pack_"), native_id


@parametrize("old,new", [
    ("| `resource.battery` | equipment | `operator_slot` | `operator_label` | none |", "| `resource.battery` | equipment | `native_slot` | `native_position` | none |"),
    ("| `relation.relays` | `configuration` |", "| `relation.relays` | `native_observation` |"),
    ("| `storage.relation.module_cell` | `resource.battery_module` | `resource.battery_cell` | `native_contract:no_module_or_cell_structure_stated_for_a_pack` |", "| `storage.relation.module_cell` | `resource.battery_module` | `resource.battery_cell` | `none` |"),
    ("| `storage.relation.pack_interface` | `gate_rule` |", "| `storage.relation.pack_interface` | `configuration` |"),
    ("| `storage.relation.system_battery` | `configuration` |", "| `storage.relation.system_battery` | `gate_rule` |"),
    ("| `resource.battery_pack` | equipment | `native_slot` | `native_position` | none |", "| `resource.battery_pack` | equipment | `operator_slot` | `operator_label` | none |"),
    ("| `site_battery_soc` | `storage.state.soc` | `resource.battery_pack` | `pack_as_a_whole` |", "| `site_battery_soc` | `storage.state.soc` | `resource.battery` | `pack_as_a_whole` |"),
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
