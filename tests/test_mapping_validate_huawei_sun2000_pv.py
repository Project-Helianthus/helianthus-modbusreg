#!/usr/bin/env python3
"""Mutation controls for the Huawei SUN2000 PV mapping gate (run with python3 or python3 -m pytest)."""
import copy
import importlib.util
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("validate_huawei_sun2000_pv_mapping", ROOT / "scripts/mapping/validate_huawei_sun2000_pv_mapping.py")
v = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = v
SPEC.loader.exec_module(v)

GRAPH_SPEC = importlib.util.spec_from_file_location("mapping_gate_graph_rules", ROOT / "scripts/mapping/mapping_gate_graph_rules.py")
graph_rules = importlib.util.module_from_spec(GRAPH_SPEC)
GRAPH_SPEC.loader.exec_module(graph_rules)

SUPPORT_SPEC = importlib.util.spec_from_file_location("huawei_mapping_test_support", ROOT / "scripts/mapping/huawei_mapping_test_support.py")
support = importlib.util.module_from_spec(SUPPORT_SPEC)
sys.modules[SUPPORT_SPEC.name] = support
SUPPORT_SPEC.loader.exec_module(support)
parametrize, Raises = support.parametrize, support.Raises
fraction_of, kernel_decimal = support.fraction_of, support.kernel_decimal

M1, M3 = v.M1, v.M3
# The member name used before the in-place rename, assembled so that no file spells it.
EARLIER_MEMBER = "relation" + "ships"
# The member that stated a conversion as a decimal shift before the unit rule, assembled the same way.
EARLIER_SHIFT = "exponent10" + "_shift"
ONE, ZERO = {"coefficient": "1", "exponent10": 0}, {"coefficient": "0", "exponent10": 0}
# (native unit, canonical unit) -> exact factor and offset; the SI value is native x factor + offset.
CONVERSIONS = {
    ("V", "unit.volt"): {"factor": ONE, "offset": ZERO},
    ("A", "unit.ampere"): {"factor": ONE, "offset": ZERO},
    ("kW", "unit.watt"): {"factor": {"coefficient": "1", "exponent10": 3}, "offset": ZERO},
    ("kWh", "unit.joule"): {"factor": {"coefficient": "36", "exponent10": 5}, "offset": ZERO},
}
ENERGY_RULE = 5
# Gateway-allocated resource identifiers of the m1 baseline (graph.md 6.3); dimension values and relation endpoints carry them.
LOGGER, SYSTEM, INVERTER = "resource:demo-logger-a", "resource:demo-system-alpha", "resource:demo-inverter-alpha"
INPUTS = ["resource:demo-input-alpha-%d" % n for n in range(1, 9)]


def contract():
    return copy.deepcopy(v.load())


def case(name="m1"):
    return copy.deepcopy(v.load()["baselines"][name])


def fact_position(document, key, index):
    return next(position for position, fact in enumerate(document["facts"]) if fact["key"] == key and fact["index"] == index)


def set_path(document, path, value):
    v.mutate(document, {"path": path, "value": value})


def assert_rejected(mutation, message):
    document = contract()
    mutation(document)
    with Raises(ValueError) as error:
        v.validate_contract(document)
    assert message in str(error.value)


def project(document):
    return v.project(document, v.load())


def by_key(output):
    return {(f["key"]["fact_id"], f["key"]["dimensions"][0]["value"]["text"]): f for f in output["facts"]}


def split_words(raw, count, signed):
    if raw < 0:
        assert signed
        raw += 1 << (16 * count)
    return [(raw >> (16 * (count - 1 - k))) & 0xFFFF for k in range(count)]


def test_gate_passes_with_its_page():
    document = v.load()
    v.document(document)
    v.check_document(document, v.DOC.read_text(encoding="utf-8"))


def test_no_binary_floating_point_anywhere():
    with Raises(ValueError, match="binary floating point"):
        v.loads('{"coefficient": 601.2}')
    document = contract()
    set_path(document, "decimal_policy.exponent10_range.0", -18.0)
    with Raises(ValueError, match="binary floating point"):
        v.validate_contract(document)
    float_case = case()
    set_path(float_case, "lifecycle.receipt_monotonic", 5.0e9)
    with Raises(ValueError, match="native_evidence_invalid"):
        project(float_case)


@parametrize("name,mutation,message", [
    ("pack", lambda d: set_path(d, "pack.id", "helianthus.pack.storage"), "contract"),
    ("pack-version", lambda d: set_path(d, "pack.version", "1.1.0"), "contract"),
    ("status", lambda d: set_path(d, "status", "accepted"), "contract"),
    ("evidence-pull-request", lambda d: set_path(d, "evidence.registry_profiles.pull_request", 358), "evidence"),
    ("evidence-contract-issue", lambda d: set_path(d, "evidence.monitoring_contract.issue", 241), "evidence"),
    ("edition-title", lambda d: set_path(d, "native_contract.edition.title", "SUN2000MC V200R023C00 Modbus Interface Definitions (draft)"), "edition"),
    ("edition-issue", lambda d: set_path(d, "native_contract.edition.issue", "Issue 02"), "edition"),
    ("edition-date", lambda d: set_path(d, "native_contract.edition.date", "2023-04-20"), "edition"),
    ("edition-quoted", lambda d: set_path(d, "native_contract.edition.use", "quoted"), "edition"),
    ("registry-flag", lambda d: set_path(d, "native_contract.registry_flags.live_qualified", True), "native contract"),
    ("outbound", lambda d: set_path(d, "native_contract.outbound_allowed", True), "native contract"),
    ("word-order", lambda d: set_path(d, "native_contract.word_order", "least_significant_word_first"), "native contract"),
    ("map0-gains-a-field", lambda d: set_path(d, "native_contract.unmapped_families.0.fields", ["pv_string_voltage"]), "unmapped family"),
    ("map0-outcome", lambda d: set_path(d, "native_contract.unmapped_families.0.outcome", "PROFILE_SELECTED"), "unmapped family"),
    ("profile-removed", lambda d: v.mutate(d, {"path": "native_contract.profiles.1", "remove": True}), "profiles"),
    ("field-dropped-from-profile", lambda d: v.mutate(d, {"path": "native_contract.profiles.1.fields.4", "remove": True}), "profile field inventory"),
    ("field-duplicated-in-profile", lambda d: set_path(d, "native_contract.profiles.1.fields.1.native_id", "pv_string_voltage"), "profile field inventory"),
    ("field-added-to-profile", lambda d: d["native_contract"]["profiles"][0]["fields"].append({"native_id": "battery_power", "state": "admitted"}), "profile field inventory"),
    ("refused-field-admitted", lambda d: set_path(d, "native_contract.profiles.0.fields.2.state", "admitted"), "profile field inventory"),
    ("fact-count", lambda d: set_path(d, "native_contract.profiles.0.fact_count", 20), "profile read plan"),
    ("read-plan-extent", lambda d: set_path(d, "native_contract.profiles.1.read_plan.2.quantity", 31), "profile read plan"),
    ("refusal-dropped", lambda d: v.mutate(d, {"path": "native_contract.profiles.0.refused_groups.0", "remove": True}), "profile read plan"),
    ("rule-removed", lambda d: v.mutate(d, {"path": "field_rules.0", "remove": True}), "missing disposition"),
    ("rule-duplicated", lambda d: d["field_rules"].append(copy.deepcopy(d["field_rules"][0])), "duplicate disposition"),
    ("rule-without-native-field", lambda d: d["field_rules"].append({**copy.deepcopy(d["field_rules"][-1]), "native_id": "battery_power"}), "disposition without native field"),
    ("exact-with-loss", lambda d: set_path(d, "field_rules.0.disposition.loss", [{"kind": "policy"}]), "exact loss"),
    ("transformed-without-loss", lambda d: set_path(d, "field_rules.2.disposition.loss", []), "transformed loss"),
    ("loss-kind", lambda d: set_path(d, "field_rules.5.disposition.loss.0.kind", "native_enum"), "transformed loss"),
    ("loss-not-reversible-flag", lambda d: set_path(d, "field_rules.5.disposition.loss.0.reversible", "no"), "transformed loss"),
    ("loss-wrong-native-item", lambda d: set_path(d, "field_rules.5.disposition.loss.0.source_items", ["native.huawei.sun2000.active_power"]), "transformed loss"),
    ("withheld-without-reason", lambda d: v.mutate(d, {"path": "field_rules.6.disposition.reason", "remove": True}), "withheld reason"),
    ("withheld-unknown-reason", lambda d: set_path(d, "field_rules.6.disposition.reason", "pv.reason.invented"), "withheld reason"),
    ("withheld-with-loss", lambda d: set_path(d, "field_rules.6.disposition.loss", []), "withheld reason"),
    ("target-other-fact", lambda d: set_path(d, "field_rules.0.target", "pv.dc.current"), "target definition"),
    ("target-not-in-pack", lambda d: set_path(d, "field_rules.0.target", "pv.dc.invented"), "target definition"),
    ("unit-other-than-pack", lambda d: set_path(d, "field_rules.0.unit", "unit.watt"), "target definition"),
    ("dimension-other-than-pack", lambda d: set_path(d, "field_rules.0.dimension", "pv.dimension.string"), "target definition"),
    ("dimension-text-restored", lambda d: set_path(d, "field_rules.0.dimension_text", "input:{n}"), "unit conversion"),
    ("status-dimension-text-restored", lambda d: set_path(d, "field_rules.4.dimension_text", "inverter:{asset_suffix}"), "status symbols"),
    ("native-gain", lambda d: set_path(d, "field_rules.0.native.gain", 7), "native form"),
    ("native-words", lambda d: set_path(d, "field_rules.2.native.words", 1), "native form"),
    ("unit-conversion-factor", lambda d: set_path(d, "field_rules.2.conversion.factor", ONE), "unit conversion"),
    ("silent-unit-conversion", lambda d: set_path(d, "field_rules.2.disposition.loss", [{"kind": "provenance", "source_items": ["native.huawei.sun2000.input_power"], "description": "x", "reversible": True}]), "silent unit conversion"),
    ("sign-convention", lambda d: set_path(d, "field_rules.3.sign.negative_value", "withheld"), "sign convention"),
    ("sign-provenance-loss", lambda d: set_path(d, "field_rules.3.disposition.loss", [d["field_rules"][3]["disposition"]["loss"][0]]), "sign convention"),
    ("status-symbol", lambda d: set_path(d, "field_rules.4.symbols", {"grid_connected": "idle"}), "status symbols"),
    ("status-extra-code", lambda d: set_path(d, "field_rules.4.symbols", {"grid_connected": "generating", "standby": "standby"}), "status symbols"),
    ("status-symbol-loss", lambda d: set_path(d, "field_rules.4.disposition.loss", [{"kind": "policy", "source_items": ["native.huawei.sun2000.device_status"], "description": "x", "reversible": False}]), "status loss"),
    ("energy-policy-loss", lambda d: set_path(d, "field_rules.5.disposition.loss", [d["field_rules"][5]["disposition"]["loss"][0], {"kind": "time", "source_items": ["native.huawei.sun2000.energy_accumulated"], "description": "x", "reversible": False}]), "counter policy loss"),
    ("ceiling-grade", lambda d: set_path(d, "field_rules.0.grade_ceiling.m3", "certain"), "disposition"),
    ("ceiling-member", lambda d: v.mutate(d, {"path": "field_rules.0.grade_ceiling.m1", "remove": True}), "disposition"),
    ("reason-unused", lambda d: d["reasons"].update({"pv.reason.unused": {"loss_kind": "policy", "description": "x", "reversible": True}}), "reason inventory"),
    ("identity-forbidden-derivation", lambda d: set_path(d, "identity.asset_id.forbidden_derivations.0", "vendor"), "identity contract"),
    ("identity-source-distinct", lambda d: set_path(d, "identity.source_id.distinct_from", "binding_id"), "identity contract"),
    ("identity-binding-inputs", lambda d: set_path(d, "identity.binding_id.stable_inputs.0", "routed_unit"), "identity contract"),
    ("identity-granularity", lambda d: set_path(d, "identity.asset_id.granularity", "one_asset_per_endpoint"), "identity contract"),
    ("identity-dimension-text-restored", lambda d: d["identity"].update({"dimension_text": {"pv.dimension.system": "system:{asset_suffix}"}}), "identity contract"),
    ("identity-relation-list-restored", lambda d: d["identity"].update({"relations": ["pv.relation.system_inverter", "pv.relation.inverter_input"]}), "identity contract"),
    ("identity-relation-member-earlier-name", lambda d: d["identity"].update({EARLIER_MEMBER: ["pv.relation.system_inverter"]}), "identity contract"),
    ("identity-phase-allocated", lambda d: set_path(d, "identity.resources.phases.source", "gateway_allocated_resource_id"), "identity contract"),
    ("lifecycle-retention", lambda d: set_path(d, "lifecycle.retention_fence", "none"), "lifecycle contract"),
    ("lifecycle-device-time", lambda d: set_path(d, "lifecycle.device_time", "statistics_time_used"), "lifecycle contract"),
    ("decimal-policy", lambda d: set_path(d, "decimal_policy.binary_floating_point", "allowed"), "decimal policy"),
    ("derived-energy", lambda d: set_path(d, "decimal_policy.derived_energy", "allowed"), "decimal policy"),
    ("counter-reset", lambda d: set_path(d, "counter_policy.decrease", "reset"), "counter policy"),
    ("counter-sum", lambda d: set_path(d, "counter_policy.sum_across_inverters", "allowed"), "counter policy"),
    ("day-yield-fact", lambda d: set_path(d, "counter_policy.day_yield.fact", "pv.energy.generated"), "counter policy"),
    ("day-yield-source", lambda d: set_path(d, "counter_policy.day_yield.source", "energy_current_hour"), "counter policy"),
    ("hour-counter-published", lambda d: set_path(d, "counter_policy.hour_counter", "published"), "counter policy"),
    ("month-yield-named-as-published", lambda d: set_path(d, "counter_policy.month_yield", "published"), "counter policy"),
    ("year-yield-named-as-published", lambda d: set_path(d, "counter_policy.year_yield", "published"), "counter policy"),
    ("prospective-month", lambda d: set_path(d, "field_rules.6.prospective_targets.0", "pv.energy.yield_day"), "prospective target"),
    ("prospective-year", lambda d: set_path(d, "field_rules.6.prospective_targets.1", "pv.energy.generated"), "prospective target"),
    ("unsupported-dropped", lambda d: v.mutate(d, {"path": "unsupported.0", "remove": True}), "unsupported"),
    ("requested-dropped", lambda d: v.mutate(d, {"path": "projection.requested_items.0", "remove": True}), "requested items"),
    ("precedence", lambda d: set_path(d, "projection.error_precedence.0", "native_evidence_invalid"), "projection"),
    ("operations", lambda d: set_path(d, "projection.operations", "allowed"), "projection"),
    ("consumer", lambda d: set_path(d, "consumer_cutover.path", "single_asset_projection"), "consumer cutover"),
])
def test_contract_mutations_are_rejected(name, mutation, message):
    assert_rejected(mutation, message)


@parametrize("reason,field,value", [
    ("loss kind", "loss_kind", "native_enum"), ("description", "description", ""), ("reversible flag", "reversible", "yes"),
])
def test_every_reason_declares_its_loss(reason, field, value):
    document = contract()
    document["reasons"]["pv.reason.value_outside_pack_range"][field] = value
    with Raises(ValueError, match="reason loss"):
        v.validate_contract(document)


def test_every_native_field_of_each_profile_has_exactly_one_disposition():
    document = v.load()
    rules = {rule["native_id"]: rule for rule in document["field_rules"]}
    for profile in document["native_contract"]["profiles"]:
        expected = [nid for nid, _ in v.PROFILE_FIELDS[profile["profile_id"]]]
        assert [row["native_id"] for row in profile["fields"]] == expected
        for nid in expected:
            assert len([rule for rule in document["field_rules"] if rule["native_id"] == nid]) == 1
            assert rules[nid]["disposition"]["outcome"] in v.OUTCOMES
    for scenario in document["scenarios"]:
        if scenario["polarity"] != "positive":
            continue
        output = v.run_scenario(scenario, document)
        profile = next(p for p in document["native_contract"]["profiles"] if p["profile_id"] == document["baselines"][scenario["baseline"]]["selection"]["profile_id"])
        v.assert_complete(output, profile, rules)
        rows = [(row["native_id"], row.get("index")) for row in output["native_dispositions"]]
        assert len(rows) == len(set(rows))
        assert len(output["native_dispositions"]) == len(profile["fields"]) - 2 + 16  # eight string rows for voltage and for current, one row for every other field


def test_completeness_check_catches_a_dropped_duplicated_or_unexplained_row():
    document = v.load()
    rules = {rule["native_id"]: rule for rule in document["field_rules"]}
    scenario = document["scenarios"][0]
    profile = document["native_contract"]["profiles"][1]
    output = v.run_scenario(scenario, document)
    for damage in ("drop", "duplicate", "reason", "target"):
        broken = copy.deepcopy(output)
        rows = broken["native_dispositions"]
        if damage == "drop":
            del rows[-1]
        elif damage == "duplicate":
            rows.append(copy.deepcopy(rows[-1]))
        elif damage == "reason":
            del next(row for row in rows if row["outcome"] == "withheld")["reason"]
        else:
            next(row for row in rows if row["outcome"] == "withheld")["target"] = "pv.dc.voltage"
        with Raises(ValueError):
            v.assert_complete(broken, profile, rules)


def test_quantities_are_canonical_decimals():
    assert v.canonical(6000, -1) == {"coefficient": "6", "exponent10": 2}
    assert v.canonical(750, -2) == {"coefficient": "75", "exponent10": -1}
    assert v.canonical(0, -3) == {"coefficient": "0", "exponent10": 0}
    assert v.canonical(-300, 0) == {"coefficient": "-3", "exponent10": 2}
    assert v.canonical(1234567, -2) == {"coefficient": "1234567", "exponent10": -2}
    with Raises(ValueError):
        v.canonical(1, 19)


def test_words_decode_with_the_contract_word_form():
    assert v.decode([0xFFFF], "I16") == -1
    assert v.decode([0x7FFF], "I16") == 32767
    assert v.decode([0xFFFF], "U16") == 65535
    assert v.decode([0x0012, 0xD687], "U32") == 1234567
    assert v.decode([0xFFFF, 0xFED4], "I32") == -300
    assert v.decode([0xD687, 0x0012], "U32") != 1234567
    for bad in ([], [1, 2], [0x10000], [-1], [True]):
        with Raises(ValueError):
            v.decode(bad, "I16")


ADMITTED = [("pv_string_voltage", 1), ("pv_string_voltage", 2), ("pv_string_current", 1), ("pv_string_current", 2),
            ("input_power", 0), ("active_power", 0), ("energy_accumulated", 0), ("energy_current_hour", 0), ("energy_current_day", 0)]
OWN_FACT = {("pv_string_voltage", 1): ("pv.dc.voltage", INPUTS[0]), ("pv_string_voltage", 2): ("pv.dc.voltage", INPUTS[1]),
            ("pv_string_current", 1): ("pv.dc.current", INPUTS[0]), ("pv_string_current", 2): ("pv.dc.current", INPUTS[1]),
            ("input_power", 0): ("pv.dc.aggregate_power", INVERTER), ("active_power", 0): ("pv.ac.aggregate_active_power", INVERTER),
            ("energy_accumulated", 0): ("pv.energy.generated", SYSTEM), ("energy_current_hour", 0): None, ("energy_current_day", 0): ("pv.energy.yield_day", SYSTEM)}


@parametrize("key,index", ADMITTED)
def test_each_consumed_word_changes_exactly_its_own_fact(key, index):
    base = case()
    before = by_key(project(base))
    position = fact_position(base, key, index)
    fact = base["facts"][position]
    spec = next(rule for rule in v.load()["field_rules"] if rule["native_id"] == key)["native"]
    raw = int(fact["value"]["coefficient"]) + 1
    fact["value"]["coefficient"] = str(raw)
    fact["words"] = split_words(raw, spec["words"], spec["type"].startswith("I"))
    after = by_key(project(base))
    changed = {name for name in set(before) | set(after) if before.get(name) != after.get(name)}
    expected = OWN_FACT[(key, index)]
    assert changed == (set() if expected is None else {expected})


def test_scale_gain_and_unit_conversion_for_each_form():
    out = by_key(project(case()))
    assert out[("pv.dc.voltage", INPUTS[0])]["value"]["quantity"] == {"number": {"coefficient": "6012", "exponent10": -1}, "unit": "unit.volt"}
    assert out[("pv.dc.current", INPUTS[0])]["value"]["quantity"] == {"number": {"coefficient": "75", "exponent10": -1}, "unit": "unit.ampere"}
    assert out[("pv.dc.aggregate_power", INVERTER)]["value"]["quantity"] == {"number": {"coefficient": "9", "exponent10": 3}, "unit": "unit.watt"}
    assert out[("pv.ac.aggregate_active_power", INVERTER)]["value"]["quantity"] == {"number": {"coefficient": "864", "exponent10": 1}, "unit": "unit.watt"}
    native = next(fact for fact in case()["facts"] if fact["key"] == "energy_accumulated")["value"]
    joule = kernel_decimal(fraction_of(native) * 3600000)
    assert out[("pv.energy.generated", SYSTEM)]["value"]["quantity"] == {"number": joule, "unit": "unit.joule"}
    assert joule == {"coefficient": "44444412", "exponent10": 3} and fraction_of(joule) / 3600000 == fraction_of(native)


def test_the_conversion_table_states_an_exact_factor_and_offset():
    assert v.UNIT_CONVERSION == CONVERSIONS
    assert not hasattr(v, "UNIT_SHIFT")
    for (native_unit, unit), conversion in CONVERSIONS.items():
        identity = conversion == {"factor": ONE, "offset": ZERO}
        assert identity == (native_unit in ("V", "A")), (native_unit, unit)


def test_each_quantity_rule_states_its_conversion_and_a_unit_loss_when_not_identity():
    for rule in v.load()["field_rules"]:
        assert EARLIER_SHIFT not in rule
        if not rule.get("unit"):
            assert "conversion" not in rule
            continue
        assert rule["conversion"] == CONVERSIONS[(rule["native"]["unit"], rule["unit"])]
        kinds = [loss["kind"] for loss in rule["disposition"]["loss"]]
        assert ("unit" in kinds) == (rule["conversion"] != {"factor": ONE, "offset": ZERO}), rule["native_id"]


@parametrize("name,mutation,message", [
    # Key field native unit: a rule whose native unit has no entry for its canonical unit.
    ("native-unit", lambda d: set_path(d, "field_rules.%d.native.unit" % ENERGY_RULE, "Wh"), "unit conversion"),
    # Key field canonical unit: a rule naming a unit other than the pack field's SI unit.
    ("canonical-unit-customary", lambda d: set_path(d, "field_rules.%d.unit" % ENERGY_RULE, "unit.kilowatt_hour"), "target definition"),
    ("canonical-unit-other-si", lambda d: set_path(d, "field_rules.%d.unit" % ENERGY_RULE, "unit.watt"), "target definition"),
    # Value field factor: coefficient and exponent each varied.
    ("factor-coefficient", lambda d: set_path(d, "field_rules.%d.conversion.factor" % ENERGY_RULE, {"coefficient": "1", "exponent10": 6}), "unit conversion"),
    ("factor-exponent", lambda d: set_path(d, "field_rules.%d.conversion.factor" % ENERGY_RULE, {"coefficient": "36", "exponent10": 6}), "unit conversion"),
    ("factor-identity", lambda d: set_path(d, "field_rules.%d.conversion.factor" % ENERGY_RULE, ONE), "unit conversion"),
    # Value field offset.
    ("offset", lambda d: set_path(d, "field_rules.%d.conversion.offset" % ENERGY_RULE, {"coefficient": "27315", "exponent10": -2}), "unit conversion"),
    ("conversion-missing", lambda d: v.mutate(d, {"path": "field_rules.%d.conversion" % ENERGY_RULE, "remove": True}), "unit conversion"),
    ("conversion-extra-member", lambda d: d["field_rules"][ENERGY_RULE]["conversion"].update(rounding="half_even"), "unit conversion"),
    ("earlier-shift-member", lambda d: d["field_rules"][ENERGY_RULE].update({EARLIER_SHIFT: 0}), "unit conversion"),
    ("identity-earlier-shift-member", lambda d: d["field_rules"][0].update({EARLIER_SHIFT: 0}), "unit conversion"),
    ("silent-joule-conversion", lambda d: set_path(d, "field_rules.%d.disposition.loss" % ENERGY_RULE, d["field_rules"][ENERGY_RULE]["disposition"]["loss"][1:]), "silent unit conversion"),
    ("status-with-conversion", lambda d: d["field_rules"][4].update(conversion={"factor": ONE, "offset": ZERO}), "status symbols"),
])
def test_conversion_key_and_value_mutations_are_rejected(name, mutation, message):
    assert_rejected(mutation, message)


def test_a_gain_or_word_order_mistake_is_rejected():
    for path, value in (("value.exponent10", -2), ("value.coefficient", "90000"), ("words", [9000, 0])):
        document = case()
        set_path(document, "facts.%d.%s" % (fact_position(document, "input_power", 0), path), value)
        with Raises(ValueError, match="native_evidence_invalid"):
            project(document)


def test_day_yield_signed_power_and_prospective_period_yields_are_mapped():
    """The day yield and the signed active power are mapped; month and year are named where no native word exists."""
    rules = {rule["native_id"]: rule for rule in v.load()["field_rules"]}
    if rules["energy_current_day"].get("target") != "pv.energy.yield_day":
        raise AssertionError("daily yield is not mapped")
    if rules["active_power"].get("sign", {}).get("negative_value") != "published":
        raise AssertionError("signed active power is not published")
    if rules["energy_current_hour"].get("prospective_targets") != ["pv.energy.yield_month", "pv.energy.yield_year"]:
        raise AssertionError("month and year yields are not named")


def test_negative_active_power_is_published_as_signed_watts():
    document = case()
    position = fact_position(document, "active_power", 0)
    document["facts"][position]["value"] = {"coefficient": "-300", "exponent10": -3}
    document["facts"][position]["words"] = [0xFFFF, 0xFED4]
    output = project(document)
    published = by_key(output)[("pv.ac.aggregate_active_power", INVERTER)]
    assert published["value"]["quantity"]["number"] == {"coefficient": "-3", "exponent10": 2}
    row = next(row for row in output["native_dispositions"] if row["native_id"] == "active_power")
    assert row == {"native_id": "active_power", "outcome": "transformed", "target": "pv.ac.aggregate_active_power"}


def test_zero_pair_never_creates_an_input_and_a_single_nonzero_word_does():
    document = case()
    for key, raw in (("pv_string_voltage", 0), ("pv_string_current", 0)):
        position = fact_position(document, key, 3)
        assert int(document["facts"][position]["value"]["coefficient"]) == raw
    assert not any(text == INPUTS[2] for _, text in by_key(project(document)))
    position = fact_position(document, "pv_string_current", 3)
    document["facts"][position]["value"]["coefficient"] = "1"
    document["facts"][position]["words"] = [1]
    out = by_key(project(document))
    assert out[("pv.dc.current", INPUTS[2])]["value"]["quantity"]["number"] == {"coefficient": "1", "exponent10": -2}
    assert out[("pv.dc.voltage", INPUTS[2])]["value"]["quantity"]["number"] == {"coefficient": "0", "exponent10": 0}


def test_unknown_is_absence_and_never_zero():
    document = case()
    for key in ("input_power", "active_power"):
        position = fact_position(document, key, 0)
        for member in ("value", "words"):
            del document["facts"][position][member]
        document["facts"][position].update({"availability": "UNKNOWN", "grade": "unknown"})
    out = by_key(project(document))
    assert ("pv.dc.aggregate_power", INVERTER) not in out and ("pv.ac.aggregate_active_power", INVERTER) not in out
    reasons = {row["native_id"]: row["reason"] for row in project(document)["native_dispositions"] if row["native_id"] in ("input_power", "active_power")}
    assert reasons == {"input_power": "pv.reason.native_fact_unknown", "active_power": "pv.reason.native_fact_unknown"}


def test_unknown_fact_that_carries_a_value_is_rejected():
    document = case()
    position = fact_position(document, "input_power", 0)
    document["facts"][position].update({"availability": "UNKNOWN", "grade": "unknown"})
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


def test_status_publishes_only_the_observed_state():
    out = by_key(project(case()))
    symbol = out[("pv.status.operating", INVERTER)]["value"]["symbol"]
    assert symbol == {"namespace": "pv.status.operating", "token": "generating", "known": True}
    document = case()
    document["facts"][fact_position(document, "device_status", 0)]["state"] = "standby"
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


def test_energy_counter_policy_publishes_lifetime_and_day_and_no_derivation():
    out = project(case())
    energy = [f for f in out["facts"] if f["key"]["fact_id"] == "pv.energy.generated"]
    day = [f for f in out["facts"] if f["key"]["fact_id"] == "pv.energy.yield_day"]
    assert len(energy) == 1 and len(day) == 1
    assert next(row["reason"] for row in out["native_dispositions"] if row["native_id"] == "energy_current_hour") == "pv.reason.period_counter_time_domain_not_represented"
    assert next(row["target"] for row in out["native_dispositions"] if row["native_id"] == "energy_current_day") == "pv.energy.yield_day"
    disposition = next(item for item in out["dispositions"] if item["item_id"] == "pv.energy.generated")
    assert [(loss["kind"], loss["reversible"]) for loss in disposition["loss"]] == [("unit", True), ("policy", False)]
    document = case()
    document["derive_energy"] = "difference_of_counters"
    with Raises(ValueError, match="unsupported_or_withheld"):
        project(document)


def test_grade_is_inherited_and_never_raised():
    m3 = case("m3")
    for position, fact in enumerate(m3["facts"]):
        if fact["key"] == "pv_string_current":
            fact["grade"] = "proven"
    out = project(m3)
    grades = {(f["key"]["fact_id"]): f["evidence_grade"] for f in out["facts"]}
    assert grades["pv.dc.current"] == "hypothesis" and grades["pv.dc.voltage"] == "proven" and grades["pv.energy.generated"] == "proven"
    m1 = project(case())
    grades = {f["key"]["fact_id"]: f["evidence_grade"] for f in m1["facts"]}
    assert grades["pv.energy.generated"] == "hypothesis" and grades["pv.status.operating"] == "hypothesis" and grades["pv.dc.aggregate_power"] == "proven"
    lowered = case()
    lowered["facts"][fact_position(lowered, "input_power", 0)]["grade"] = "hypothesis"
    assert {f["key"]["fact_id"]: f["evidence_grade"] for f in project(lowered)["facts"]}["pv.dc.aggregate_power"] == "hypothesis"


def test_refused_power_group_withholds_power_and_status_on_the_m3_profile():
    out = project(case("m3"))
    reasons = {row["native_id"]: row["reason"] for row in out["native_dispositions"] if row["native_id"] in ("input_power", "active_power", "device_status")}
    assert set(reasons.values()) == {"pv.reason.power_group_refused"}
    withheld = {item["item_id"]: item for item in out["dispositions"] if item["outcome"] == "withheld"}
    assert set(withheld) == {"pv.dc.aggregate_power", "pv.ac.aggregate_active_power", "pv.status.operating"}
    assert all(item["loss"] and item["reason"] == "pv.reason.power_group_refused" for item in withheld.values())
    document = case("m3")
    document["facts"].append(copy.deepcopy(case()["facts"][fact_position(case(), "input_power", 0)]))
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


def test_retained_reading_keeps_its_original_observation():
    document = case()
    document["groups"][3]["outcome"] = "FAILED"
    for key in ("energy_accumulated", "energy_current_hour", "energy_current_day"):
        fact = document["facts"][fact_position(document, key, 0)]
        fact.update({"retained": True, "observation_id": "observation:demo-0003", "observation_revision": 3, "origin": retained_case()["facts"][fact_position(document, key, 0)]["origin"]})
    energy = next(f for f in project(document)["facts"] if f["key"]["fact_id"] == "pv.energy.generated")
    assert energy["retained"] is True and energy["observation_id"] == "observation:demo-0003" and energy["observation_revision"] == 3
    for member, value in (("observation_revision", 5), ("observation_id", "observation:demo-0005")):
        broken = copy.deepcopy(document)
        broken["facts"][fact_position(broken, "energy_accumulated", 0)][member] = value
        with Raises(ValueError, match="native_evidence_invalid"):
            project(broken)


def test_retained_group_with_mixed_observations_is_rejected():
    for member, value in (("observation_id", "observation:demo-0002"), ("observation_revision", 2)):
        broken = retained_case()
        broken["facts"][fact_position(broken, "energy_current_day", 0)][member] = value
        with Raises(ValueError, match="native_evidence_invalid"):
            project(broken)
    broken = retained_case()
    broken["facts"][fact_position(broken, "energy_current_day", 0)]["origin"]["receipt_monotonic"] = 2999999999
    with Raises(ValueError, match="native_evidence_invalid"):
        project(broken)


def test_retained_reading_rejects_invalid_lineage():
    for member, value in (("observation_id", ""), ("observation_revision", -1), ("observation_revision", 0)):
        broken = retained_case()
        broken["facts"][fact_position(broken, "energy_accumulated", 0)][member] = value
        with Raises(ValueError, match="native_evidence_invalid"):
            project(broken)


def retained_case():
    document = case()
    document["groups"][3]["outcome"] = "FAILED"
    origin = {"receipt_wall": "2025-12-31T23:00:00Z", "receipt_monotonic": 3000000000, "clock_epoch": "clock-epoch:demo-0", "source_epoch": "source-epoch:1", "driver_generation": 1, "connection_generation": 1,
              "profile_id": "huawei.sun2000.m1.monitoring-readonly@1.0.0", "routed_unit": 1,
              "asset_id": "asset:demo-alpha", "binding_id": "binding:semantic-pv:demo-alpha", "source_id": "source:gateway-demo-a"}
    for key in ("energy_accumulated", "energy_current_hour", "energy_current_day"):
        document["facts"][fact_position(document, key, 0)].update({"retained": True, "observation_id": "observation:demo-0003", "observation_revision": 3, "origin": dict(origin)})
    return document


def test_retained_reading_preserves_its_original_receipt_coordinates():
    document = retained_case()
    energy = next(f for f in project(document)["facts"] if f["key"]["fact_id"] == "pv.energy.generated")
    assert (energy["receipt_wall"], energy["receipt_monotonic"], energy["clock_epoch"]) == ("2025-12-31T23:00:00Z", 3000000000, "clock-epoch:demo-0")
    current = next(f for f in project(case())["facts"] if f["key"]["fact_id"] == "pv.energy.generated")
    assert current["receipt_monotonic"] == 5000000000 and current["clock_epoch"] == "clock-epoch:demo-1"


def test_retained_reading_orders_ticks_only_within_its_clock_epoch():
    document = retained_case()
    origins = [document["facts"][fact_position(document, key, 0)]["origin"] for key in ("energy_accumulated", "energy_current_hour", "energy_current_day")]
    for origin in origins:
        origin["receipt_monotonic"] = 9000000000
    energy = next(f for f in project(document)["facts"] if f["key"]["fact_id"] == "pv.energy.generated")
    assert energy["receipt_monotonic"] == 9000000000 and energy["clock_epoch"] == "clock-epoch:demo-0"
    for origin in origins:
        origin["clock_epoch"] = "clock-epoch:demo-1"
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


def test_retained_reading_publishes_its_original_source_lineage():
    energy = next(f for f in project(retained_case())["facts"] if f["key"]["fact_id"] == "pv.energy.generated")
    assert energy["source_path"]["source_epoch_id"] == "source-epoch:1" and energy["source_path"]["driver_generation"] == 1


def test_retained_reading_is_fenced_by_its_original_lineage():
    for member, value in (("connection_generation", 2), ("profile_id", "huawei.sun2000.m3.monitoring-readonly@1.0.0"), ("routed_unit", 2),
                          ("source_epoch", "source-epoch:2"), ("driver_generation", 2), ("receipt_wall", "not-a-time"), ("clock_epoch", "")):
        broken = retained_case()
        broken["facts"][fact_position(broken, "energy_accumulated", 0)]["origin"][member] = value
        with Raises(ValueError, match="native_evidence_invalid"):
            project(broken)
    broken = retained_case()
    del broken["facts"][fact_position(broken, "energy_accumulated", 0)]["origin"]
    with Raises(ValueError, match="native_evidence_invalid"):
        project(broken)


def test_retained_reading_is_fenced_by_its_original_identity():
    for member, value in (("asset_id", "asset:demo-beta"), ("binding_id", "binding:semantic-pv:demo-beta"), ("source_id", "source:gateway-demo-b")):
        broken = retained_case()
        broken["facts"][fact_position(broken, "energy_accumulated", 0)]["origin"][member] = value
        with Raises(ValueError, match="native_evidence_invalid"):
            project(broken)
    reassigned = retained_case()
    reassigned["identity"] = {"asset_id": "asset:demo-beta", "binding_id": "binding:semantic-pv:demo-beta"}
    with Raises(ValueError, match="native_evidence_invalid"):
        project(reassigned)
    broken = retained_case()
    del broken["facts"][fact_position(broken, "energy_accumulated", 0)]["origin"]["asset_id"]
    with Raises(ValueError, match="native_evidence_invalid"):
        project(broken)


def test_fact_keys_follow_the_pack_dimensions_and_relations():
    out = project(case())
    pack = v.pack_catalog()[0]
    for fact in out["facts"]:
        key = fact["key"]
        assert key["pack_id"] == "helianthus.pack.pv" and key["pack_version"] == "1.0.0"
        assert [dimension["id"] for dimension in key["dimensions"]] == [pack[key["fact_id"]]["dimension"]]
        assert not any(dimension["id"] == "pv.dimension.phase" for dimension in key["dimensions"])
    assert [(r["definition"], r["from"], r["to"], r["basis_class"]) for r in out["relations"]] == [
        ("pv.relation.inverter_input", INVERTER, INPUTS[0], "gate_rule"), ("pv.relation.inverter_input", INVERTER, INPUTS[1], "gate_rule"),
        ("pv.relation.system_inverter", SYSTEM, INVERTER, "gate_rule"), ("relation.relays", LOGGER, INVERTER, "native_observation")]


def test_relation_endpoints_and_dimension_values_are_published_resources():
    """REL-8 and RES-5: every endpoint and every subject is a published ResourceID, never dimension text."""
    out = project(case())
    kinds = {r["resource_id"]: r["kind"] for r in out["resources"]}
    assert kinds == {LOGGER: "resource.equipment", SYSTEM: "resource.pv_system", INVERTER: "resource.inverter", INPUTS[0]: "resource.dc_input", INPUTS[1]: "resource.dc_input"}
    assert [r["resource_id"] for r in out["resources"]] == sorted(kinds)
    assert {r["resource_id"]: r.get("parent") for r in out["resources"]} == {LOGGER: None, SYSTEM: None, INVERTER: None, INPUTS[0]: INVERTER, INPUTS[1]: INVERTER}
    for record in out["relations"]:
        assert record["from"] in kinds and record["to"] in kinds
    for fact in out["facts"]:
        assert fact["key"]["dimensions"][0]["value"]["text"] in kinds
    v.assert_graph(out)


@parametrize("damage", [
    lambda o: o["relations"].append({"definition": "relation.relays", "from": INVERTER, "to": "resource:demo-battery-alpha", "basis_class": "native_observation"}),
    lambda o: o["relations"].append({"definition": "pv.relation.inverter_input", "from": INVERTER, "to": "input:1", "basis_class": "gate_rule"}),
    lambda o: o.update(resources=[r for r in o["resources"] if r["resource_id"] != INVERTER]),
    lambda o: o.update(resources=[r for r in o["resources"] if r["resource_id"] != INPUTS[0]]),
])
def test_a_dangling_endpoint_or_subject_is_rejected(damage):
    out = project(case())
    damage(out)
    with Raises(ValueError, match="dangling endpoint"):
        v.assert_graph(out)


NEW_ID = "resource:demo-varied-zeta"


def mentions(output, value):
    """Where an identifier appears in a projection output, by record class."""
    places = set()
    for fact in output["facts"]:
        if fact["key"]["dimensions"][0]["value"]["text"] == value:
            places.add("fact:" + fact["key"]["fact_id"])
    for item in output["dispositions"]:
        places |= {"source_key:" + item["item_id"] for key in item["source_keys"] if key["dimensions"][0]["value"]["text"] == value}
    for record in output["relations"]:
        places |= {"relation:%s:%s" % (record["definition"], side) for side in ("from", "to") if record[side] == value}
    for resource in output["resources"]:
        places |= {"resource" for _ in [0] if resource["resource_id"] == value} | {"parent" for _ in [0] if resource.get("parent") == value}
    return places


def normalized(value):
    """The output with every list ordered canonically: records sort by identifier, so a varied identifier may reorder them."""
    if isinstance(value, dict):
        return {name: normalized(item) for name, item in value.items()}
    if isinstance(value, list):
        return sorted((normalized(item) for item in value), key=lambda item: json.dumps(item, sort_keys=True))
    return value


INVERTER_PLACES = {"resource", "parent", "fact:pv.ac.aggregate_active_power", "fact:pv.dc.aggregate_power", "fact:pv.status.operating",
                   "source_key:pv.ac.aggregate_active_power", "source_key:pv.dc.aggregate_power", "source_key:pv.status.operating",
                   "relation:pv.relation.system_inverter:to", "relation:pv.relation.inverter_input:from", "relation:relation.relays:to"}
INPUT_PLACES = {"resource", "fact:pv.dc.voltage", "fact:pv.dc.current", "source_key:pv.dc.voltage", "source_key:pv.dc.current", "relation:pv.relation.inverter_input:to"}


# Resource identity coverage: each row varies one field of `identity.resources` (and the asset) and states where it must
# appear. The comment names the production mutation the row catches.
@parametrize("path,old,places", [
    ("identity.resources.logger", LOGGER, {"resource", "relation:relation.relays:from"}),  # relays drawn from the asset or the source
    ("identity.resources.system", SYSTEM, {"resource", "fact:pv.energy.generated", "fact:pv.energy.yield_day", "source_key:pv.energy.generated", "source_key:pv.energy.yield_day", "relation:pv.relation.system_inverter:from"}),  # system text derived from the asset
    ("identity.resources.inverter", INVERTER, INVERTER_PLACES),  # inverter text derived from the asset, or the input parent taken from the system
    ("identity.resources.inputs.0", INPUTS[0], INPUT_PLACES),  # input keyed by "input:<n>" instead of its allocated ResourceID
    ("identity.resources.inputs.1", INPUTS[1], INPUT_PLACES),  # inputs keyed by position in the published list instead of the native index
    ("identity.resources.inputs.2", INPUTS[2], set()),  # an input with a zero pair published as a resource
    ("identity.resources.inputs.7", INPUTS[7], set()),  # the eighth slot off by one
    ("identity.asset_id", "asset:demo-alpha", set()),  # a dimension value or endpoint still derived from the asset
])
def test_each_resource_identity_reaches_exactly_its_own_records(path, old, places):
    base = case()
    varied = copy.deepcopy(base)
    set_path(varied, path, NEW_ID)
    before, after = project(base), project(varied)
    assert mentions(before, old) == places and mentions(after, NEW_ID) == places
    assert normalized(json.loads(json.dumps(before).replace(old, NEW_ID))) == normalized(after)


@parametrize("path,value", [
    ("identity.resources.inverter", None), ("identity.resources.logger", ""), ("identity.resources", None),
    ("identity.resources.inputs", INPUTS[:7]), ("identity.resources.inputs.2", INPUTS[0]),
    ("identity.resources.system", INVERTER), ("identity.resources.logger", "asset:demo-alpha"), ("identity.resources.inverter", "binding:semantic-pv:demo-alpha"),
    ("identity.resources.inverter", "resource:serial-0001"), ("identity.resources.system", "resource:huawei-a"), ("identity.resources.inverter", "1"),
    ("identity.resources.phases", ["resource:demo-phase-alpha-1"]),
])
def test_resource_identities_fail_closed(path, value):
    """A missing, reused or derived ResourceID fails closed; one identifier never names two resources."""
    document = case()
    set_path(document, path, value)
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


def test_published_facts_do_not_depend_on_the_asset():
    base, varied = case(), case()
    set_path(varied, "identity.asset_id", "asset:demo-omega")
    left = {f["key"]["dimensions"][0]["value"]["text"] for f in project(base)["facts"] if f["key"]["fact_id"] == "pv.dc.voltage"}
    right = {f["key"]["dimensions"][0]["value"]["text"] for f in project(varied)["facts"] if f["key"]["fact_id"] == "pv.dc.voltage"}
    assert left == right == set(INPUTS[:2])


@parametrize("scenario_id,member,message", [
    ("PV-POS-001-m1-baseline", "output", "scenario output PV-POS-001-m1-baseline"),
    ("PV-POS-003-negative-active-power", "summary", "scenario summary PV-POS-003-negative-active-power"),
])
def test_expected_outputs_reject_the_earlier_relation_member(scenario_id, member, message):
    document = contract()
    expected = next(s for s in document["scenarios"] if s["id"] == scenario_id)["expect"][member]
    expected[EARLIER_MEMBER] = expected.pop("relations")
    with Raises(ValueError) as error:
        v.validate_scenarios(document)
    assert str(error.value) == message


# Lineage coverage. Each row varies one field and names the production mutation the assertion catches.
@parametrize("path,value,check", [
    ("identity.binding_id", "binding:semantic-pv:demo-omega", lambda a, b: a["source_path"]["binding_id"] != b["source_path"]["binding_id"]),  # binding dropped from the source path
    ("source.source_id", "source:gateway-demo-z", lambda a, b: a["source_path"]["source_id"] != b["source_path"]["source_id"]),  # source taken from the asset
    ("lifecycle.source_epoch", "source-epoch:9", lambda a, b: a["source_path"]["source_epoch_id"] != b["source_path"]["source_epoch_id"]),  # epoch not carried
    ("lifecycle.driver_generation", 7, lambda a, b: a["source_path"]["driver_generation"] != b["source_path"]["driver_generation"]),  # generation not carried
])
def test_identity_and_lifecycle_fields_reach_the_published_fact(path, value, check):
    base = case()
    varied = copy.deepcopy(base)
    set_path(varied, path, value)
    first = next(f for f in project(base)["facts"] if f["key"]["fact_id"] == "pv.energy.generated")
    second = next(f for f in project(varied)["facts"] if f["key"]["fact_id"] == "pv.energy.generated")
    assert check(first, second)


@parametrize("asset", ["", None, "asset one", "asset:serial-0001", "asset:ESN-1", "asset:10.0.0.7", "asset:host:502", "1", "source:gateway-demo-a", "binding:semantic-pv:demo-alpha", "asset:sun2000-a", "asset:huawei", "asset:smartlogger-a", INVERTER])
def test_asset_identifier_never_comes_from_forbidden_inputs(asset):
    document = case()
    document["identity"]["asset_id"] = asset
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


@parametrize("path,value", [("source.source_id", ""), ("source.source_id", "source:10.1.1.1"), ("identity.binding_id", None), ("identity.binding_id", "asset:demo-alpha"), ("source", None), ("identity", {})])
def test_source_and_binding_identifiers_fail_closed(path, value):
    document = case()
    set_path(document, path, value)
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


@parametrize("flag", sorted(v.REGISTRY_FLAGS) + ["mapping_qualified", "outbound_allowed"])
def test_every_qualification_flag_is_pinned(flag):
    document = case()
    document["qualification"][flag] = not document["qualification"][flag]
    with Raises(ValueError, match="mapping_unqualified"):
        project(document)


@parametrize("outcome", ["INSUFFICIENT_EVIDENCE", "NO_ADMISSIBLE_PROFILE", "AMBIGUOUS", "", None])
def test_only_a_selected_exact_profile_maps(outcome):
    document = case()
    document["selection"]["outcome"] = outcome
    with Raises(ValueError, match="profile_not_selected"):
        project(document)


@parametrize("profile_id", ["huawei.sun2000.map0.monitoring-readonly@1.0.0", "huawei.sun2000.m1.monitoring-readonly@1.0.1", "huawei.sun2000.m1.monitoring-readonly", "huawei.sun2000.m1", ""])
def test_profile_identifier_must_be_exact(profile_id):
    document = case()
    document["selection"]["profile_id"] = profile_id
    with Raises(ValueError, match="profile_not_selected"):
        project(document)


def test_a_group_outside_the_plan_or_a_refused_group_that_answered_is_rejected():
    document = case("m3")
    document["groups"][2]["outcome"] = "ANSWERED"
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)
    document = case()
    document["groups"].append({"offset": 37000, "quantity": 30, "outcome": "ANSWERED"})
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


@parametrize("group,member,value", [(0, "outcome", "EXCEPTION"), (0, "exception_code", 3), (0, "outcome", "TIMEOUT"), (3, "exception_code", 3)])
def test_group_outcomes_and_exception_codes_must_agree(group, member, value):
    document = case()
    document["groups"][group][member] = value
    with Raises(ValueError, match="native_evidence_invalid"):
        project(document)


def test_operations_are_never_available():
    for member, value in (("request_operation", "pv.operation.set_export_limit"), ("request_operation", "pv.operation.set_active_power_limit")):
        document = case()
        document[member] = value
        with Raises(ValueError, match="unsupported_or_withheld"):
            project(document)
    assert project(case())["operations"] == []


def test_scenario_expectations_are_executed_not_trusted():
    for damage, message in (
        (lambda d: d["scenarios"][0]["expect"]["output"]["facts"][0]["value"]["quantity"]["number"].update({"coefficient": "999"}), "scenario output"),
        (lambda d: d["scenarios"][4]["expect"]["summary"]["facts"].pop(), "scenario summary"),
        (lambda d: d["scenarios"][2]["expect"]["summary"]["native_dispositions"][0].update({"outcome": "withheld"}), "scenario summary"),
        (lambda d: next(x for x in d["scenarios"] if x["id"] == "PV-NEG-018-error-precedence")["expect"].update({"error": "lifecycle_missing"}), "profile_not_selected"),
    ):
        document = contract()
        damage(document)
        with Raises(ValueError) as error:
            v.validate_scenarios(document)
        assert message in str(error.value)


def test_scenario_coverage_is_enforced():
    document = contract()
    document["scenarios"] = [s for s in document["scenarios"] if s["id"] != "PV-POS-005-unobserved-status-code"]
    with Raises(ValueError, match="cover every reason"):
        v.validate_scenarios(document)
    document = contract()
    document["scenarios"] = [s for s in document["scenarios"] if s["id"] != "PV-NEG-014-lifecycle-missing"]
    with Raises(ValueError, match="cover every error"):
        v.validate_scenarios(document)
    document = contract()
    document["scenarios"][1]["mutations"] = [{"path": "identity.asset_id", "value": "asset:demo-omega"}]
    with Raises(ValueError, match="scenario baselines"):
        v.validate_scenarios(document)


def test_a_negative_scenario_that_is_accepted_fails():
    document = contract()
    negative = next(s for s in document["scenarios"] if s["id"] == "PV-NEG-010-asset-missing")
    negative["mutations"] = []
    with Raises(ValueError, match="accepted"):
        v.validate_scenarios(document)


def page():
    return v.DOC.read_text(encoding="utf-8")


@parametrize("old,new", [
    ("| `pv_string_voltage` | I16, V, gain 10 | `pv.dc.voltage` | `unit.volt`, factor 1, offset 0 | `pv.dimension.input` | exact |", "| `pv_string_voltage` | I16, V, gain 10 | `pv.dc.voltage` | `unit.volt`, factor 1, offset 0 | `pv.dimension.input` | transformed |"),
    ("| `input_power` | I32, kW, gain 1000 | `pv.dc.aggregate_power` |", "| `input_power` | I32, kW, gain 1000 | `pv.dc.power` |"),
    ("| `pv_string_current` | I16, A, gain 100 | `pv.dc.current` | `unit.ampere`, factor 1, offset 0 | `pv.dimension.input` | exact | none | Hypothesis | Proven |", "| `pv_string_current` | I16, A, gain 100 | `pv.dc.current` | `unit.ampere`, factor 1, offset 0 | `pv.dimension.input` | exact | none | Proven | Proven |"),
    ("| `pv.energy.generated` | `unit.joule`, factor 3600000, offset 0 |", "| `pv.energy.generated` | `unit.kilowatt_hour`, factor 1, offset 0 |"),
    ("| `pv.energy.generated` | `unit.joule`, factor 3600000, offset 0 |", "| `pv.energy.generated` | `unit.joule`, factor 3600, offset 0 |"),
    ("| `pv.dc.aggregate_power` | `unit.watt`, factor 1000, offset 0 |", "| `pv.dc.aggregate_power` | `unit.watt`, factor 1000, offset 1 |"),
    ("| transformed | unit+policy | Proven | Hypothesis |", "| transformed | policy | Proven | Hypothesis |"),
    ("| `battery` | withheld | `pv.reason.battery_outside_pv_family` | capability | withheld |", "| `battery` | withheld | `pv.reason.battery_outside_pv_family` | capability | unrepresentable |"),
    ("| `PV-NEG-018-error-precedence` |", "| `PV-NEG-099-other` |"),
    ("Proven only where the monitoring contract is Proven", "Proven where a document says so"),
    ("outbound_allowed=false", "outbound allowed"),
])
def test_page_must_match_the_contract(old, new):
    pattern = re.escape(old).replace(r"\ ", r"\s+")
    mutated, count = re.subn(pattern, lambda _: new, page())
    assert count >= 1
    with Raises(ValueError):
        v.check_document(v.load(), mutated)


def test_a_dropped_table_row_is_rejected():
    lines = [line for line in page().splitlines(keepends=True) if not line.startswith("| `energy_current_day` |")]
    with Raises(ValueError, match="document tables"):
        v.check_document(v.load(), "".join(lines))


@parametrize("literal", ["a" * 40, "0123456789abcdef" * 4, "A1" * 20])
def test_publication_rule_rejects_revision_and_digest_literals(literal):
    with Raises(ValueError, match="publication"):
        v.check_document(v.load(), page() + "\n" + literal + "\n")


OWNER_LINK = "https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/graph.md"


def test_publication_accepts_approved_full_sha_owner_link():
    v.check_document(v.load(), page() + "\n[graph](" + OWNER_LINK + ")\n")
    v.check_document(v.load(), page() + "\n[readme](" + OWNER_LINK.replace("api/v1/graph.md", "api/README.md") + "#unit-rule)\n")


@parametrize("link", [
    OWNER_LINK.replace("de3a0b49535582ad51a0ec23011d3aebf9c04c76", "0123456789abcdef0123456789abcdef01234567"),
    OWNER_LINK.replace("helianthus-docs-semantic", "helianthus-modbusreg"),
    OWNER_LINK.replace("/blob/", "/commit/"),
    OWNER_LINK + "#L10",
])
def test_publication_rejects_other_owner_links(link):
    with Raises(ValueError, match="publication"):
        v.check_document(v.load(), page() + "\n[graph](" + link + ")\n")


def test_published_files_cite_no_revision_link_or_endpoint():
    for path in (v.PATH, v.DOC):
        text = path.read_text(encoding="utf-8")
        assert not v.HEX_RUN_RE.search(v.unowned_text(text))
        assert "/blob/" not in v.unowned_text(text) and "/commit/" not in text and "/tree/" not in text
        assert "#L" not in text
    forbidden = ("serial_number:", "esn:", "tcp://")
    assert not any(token in v.PATH.read_text(encoding="utf-8") for token in forbidden)


def test_baselines_are_synthetic():
    text = v.PATH.read_text(encoding="utf-8")
    document = json.loads(text)
    for baseline in document["baselines"].values():
        assert baseline["lifecycle"]["receipt_wall"] == "2026-01-01T00:00:00Z"
        assert baseline["identity"]["asset_id"].startswith("asset:demo-")
        assert baseline["source"]["source_id"].startswith("source:gateway-demo-")
        resources = baseline["identity"]["resources"]
        assert all(item.startswith("resource:demo-") for item in [resources["logger"], resources["system"], resources["inverter"]] + resources["inputs"])


def test_identity_carries_no_class_token_and_no_derivation():
    """Section 6.3: a pack class token and a profile identifier are provenance, never identity."""
    identity = contract()["identity"]
    assert set(identity) == {"asset_id", "source_id", "binding_id", "resources", "failure"}
    assert identity["asset_id"]["source"] == "gateway_allocated_asset_id" and "derivation" not in identity["asset_id"]
    assert {"class_token", "profile_id"} <= set(identity["asset_id"]["forbidden_derivations"])
    assert identity["source_id"]["source"] == "gateway_managed_instance_source_id"
    assert "class_token" not in json.dumps(identity["resources"])
    selectors = {item["selector"]: item["classification"] for item in contract()["graph"]["selectors"]}
    assert selectors == {"profile_id": "provenance", "class_token": "provenance"}


def test_source_identity_contract_cannot_return_to_per_asset_configuration():
    assert_rejected(lambda d: set_path(d, "identity.source_id.source", "gateway_configured_semantic_source_id"), "identity contract")


def test_obsolete_per_child_configured_asset_requirement_is_rejected():
    assert_rejected(lambda d: set_path(d, "identity.asset_id.source", "gateway_configured_semantic_asset_id"), "identity contract")
    assert_rejected(lambda d: set_path(d, "identity.asset_id.source", "gateway_derived_semantic_asset_id"), "identity contract")


@parametrize("family,baseline,label", [
    ("sdongle", "m1", "default"), ("sdongle", "m1", "roof"),
    ("smartlogger", "m3", "default"), ("smartlogger", "m3", "roof"),
])
def test_managed_source_identifiers_project_with_allocated_resources(family, baseline, label):
    document = case(baseline)
    suffix = "" if label == "default" else "." + label
    source = "modbus.huawei." + family + suffix + ":00000000-0000-4000-8000-0000000000d5"
    document["source"]["source_id"] = source
    out = project(document)
    assert out["facts"]
    assert all(f["source_path"]["source_id"] == source for f in out["facts"])
    energy = next(f for f in out["facts"] if f["key"]["fact_id"] == "pv.energy.generated")
    assert energy["key"]["dimensions"][0]["value"]["text"] == document["identity"]["resources"]["system"]


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


BATTERY_RELAYS = {"definition": "relation.relays", "basis_class": "native_observation", "from_kind": "resource.inverter", "to_kind": "resource.battery",
                  "evidence": ["synthetic:battery"], "validity_scope": {"profile": "m3_and_m1", "version": "1.0.0", "rule_scope": "synthetic"}, "absence_sides": []}
STRING_RESOURCE = {"kind": "resource.pv_string", "anchor_class": "component", "anchor_basis": "native_slot", "anchor_source": "native_position",
                   "identification_scheme": "none_listed", "parent_kind": "resource.inverter", "evidence": ["synthetic:string"]}
VOLTAGE_AGGREGATE = {"fact": "pv.dc.voltage", "subject_kind": "resource.dc_input", "coverage": []}

# (name, edits, expected error). Indices: resources logger, system, inverter, input, phase; relations relays, system to
# inverter, inverter to input; selectors profile identifier, class token; aggregates DC power, AC power, energy.
GRAPH_NEGATIVES = [
    # Item 5: a class token, routed unit or profile offered as an anchor source.
    ("class_token_as_anchor_source", [("set", "graph.resources.2.anchor_source", "class_token")], "graph anchor_source_not_allowed $.graph.resources[2].anchor_source"),
    ("routed_unit_as_anchor_source", [("set", "graph.resources.2.anchor_source", "routed_unit")], "graph anchor_source_not_allowed $.graph.resources[2].anchor_source"),
    ("profile_as_anchor_source", [("set", "graph.resources.0.anchor_source", "profile_id")], "graph anchor_source_not_allowed $.graph.resources[0].anchor_source"),
    ("class_token_as_identity_evidence", [("set", "graph.selectors.1.classification", "identity_evidence")], "graph_selectors"),
    ("class_token_as_resource_attribute", [("set", "graph.selectors.1.classification", "resource_attribute")], "graph_selectors"),
    ("class_token_selector_dropped", [("delete", "graph.selectors.1", None)], "graph_selectors"),
    # Item 5: one device reached through two labels or paths is joined by declaration or flagged, never two identities.
    ("false_join", [("set", "graph.joins.0.native_tokens", "differ")], "graph false_join $.graph.joins[0].native_tokens"),
    ("join_by_label", [("set", "graph.joins.0.basis", "configured_instance_label")], "graph join_basis_not_allowed $.graph.joins[0].basis"),
    ("join_by_routed_unit", [("set", "graph.joins.0.basis", "routed_unit")], "graph join_basis_not_allowed $.graph.joins[0].basis"),
    ("two_paths_two_identities", [("set", "graph.joins", [])], "graph_joins"),
    ("join_of_one_observation", [("set", "graph.joins.0.observations", ["observed_directly"])], "graph invalid_value $.graph.joins[0].observations"),
    # Item 5: a relation record whose endpoint is not a published resource.
    ("dangling_battery_endpoint", [("append", "graph.relations", BATTERY_RELAYS)], "graph dangling_endpoint $.graph.relations[3].to_kind"),
    ("dangling_logger_endpoint", [("set", "graph.resources.0.kind", "resource.meter")], "graph dangling_endpoint $.graph.relations[0].from_kind"),
    # Resources (section 15 item 1): kind, class, basis, source, scheme, parent and evidence each varied.
    ("resource_kind_not_member", [("set", "graph.resources.4.kind", "resource.not_a_catalog_kind")], "graph kind_not_member $.graph.resources[4].kind"),
    ("resource_unpublished_kind", [("append", "graph.resources", STRING_RESOURCE)], "graph_resources"),
    ("input_anchor_class", [("set", "graph.resources.3.anchor_class", "equipment")], "graph anchor_class_mismatch $.graph.resources[3].anchor_class"),
    ("input_anchored_on_label", [("set", "graph.resources.3.anchor_basis", "operator_slot"), ("set", "graph.resources.3.anchor_source", "operator_label")], "graph_anchor"),
    ("system_anchored_on_position", [("set", "graph.resources.1.anchor_basis", "native_slot"), ("set", "graph.resources.1.anchor_source", "native_position")], "graph_anchor"),
    ("inverter_basis_source_mismatch", [("set", "graph.resources.2.anchor_basis", "native_slot")], "graph anchor_basis_mismatch $.graph.resources[2].anchor_basis"),
    ("inverter_native_token_without_scheme", [("set", "graph.resources.2.anchor_basis", "native_identity"), ("set", "graph.resources.2.anchor_source", "native_token")], "graph scheme_not_listed $.graph.resources[2].identification_scheme"),
    ("inverter_scheme_unlisted", [("set", "graph.resources.2.identification_scheme", {"id": "scheme.unlisted", "issuer_registry": "none"})], "graph scheme_not_listed $.graph.resources[2].identification_scheme"),
    ("input_parent_system", [("set", "graph.resources.3.parent_kind", "resource.pv_system")], "graph_parent"),
    ("phase_parent_missing", [("set", "graph.resources.4.parent_kind", None)], "graph parent_kind_missing $.graph.resources[4].parent_kind"),
    ("inverter_token_stability_unstated", [("set", "graph.resources.2.evidence", ["native_contract:monitoring_contract"])], "graph_evidence"),
    ("inverter_future_token_unstated", [("delete", "graph.resources.2.evidence.1", None)], "graph_evidence"),
    ("resource_evidence_empty", [("set", "graph.resources.1.evidence", [])], "graph invalid_value $.graph.resources[1].evidence"),
    # Relations (section 15 item 2): definition, basis class, both endpoints, evidence, each scope member and absence sides.
    ("relation_not_member", [("set", "graph.relations.1.definition", "relation.invented")], "graph relation_not_member $.graph.relations[1].definition"),
    ("system_inverter_by_configuration", [("set", "graph.relations.1.basis_class", "configuration")], "graph_relations"),
    ("relays_by_gate_rule", [("set", "graph.relations.0.basis_class", "gate_rule")], "graph basis_class_not_admitted $.graph.relations[0].basis_class"),
    ("relays_by_configuration", [("set", "graph.relations.0.basis_class", "configuration")], "graph_relations"),
    ("relation_from_kind", [("set", "graph.relations.2.from_kind", "resource.pv_system")], "graph endpoint_kind_mismatch $.graph.relations[2].from_kind"),
    ("relation_to_kind", [("set", "graph.relations.1.to_kind", "resource.dc_input")], "graph endpoint_kind_mismatch $.graph.relations[1].to_kind"),
    ("relation_evidence_empty", [("set", "graph.relations.2.evidence", [])], "graph invalid_value $.graph.relations[2].evidence"),
    ("scope_profile", [("set", "graph.relations.1.validity_scope.profile", "m3")], "graph_relations"),
    ("scope_version", [("set", "graph.relations.2.validity_scope.version", "1.0.1")], "graph_relations"),
    ("scope_rule_blank", [("set", "graph.relations.0.validity_scope.rule_scope", "")], "graph invalid_value $.graph.relations[0].validity_scope.rule_scope"),
    ("scope_rule_widened", [("set", "graph.relations.2.validity_scope.rule_scope", "every_native_string_slot")], "graph_relations"),
    ("absence_claimed", [("set", "graph.relations.2.absence_sides", ["to"])], "graph_relations"),
    ("relation_dropped", [("delete", "graph.relations.2", None)], "graph_relations"),
    # Relayed (section 15 item 3).
    ("relayed_direct_key_address", [("set", "graph.relayed.0.direct_anchor_key_source", "network_address")], "graph anchor_source_not_allowed $.graph.relayed[0].direct_anchor_key_source"),
    ("relayed_direct_key_denied", [("set", "graph.relayed.0.direct_anchor_key_source", "none")], "graph_relayed"),
    ("relayed_dropped", [("set", "graph.relayed", [])], "graph_relayed"),
    # Aggregates (section 15 item 4, EN-5).
    ("energy_without_coverage", [("set", "graph.aggregates.2.coverage", [])], "graph_aggregates"),
    ("power_coverage_claimed", [("set", "graph.aggregates.0.coverage", ["pv.relation.inverter_input"])], "graph_aggregates"),
    ("aggregate_subject_missing", [("set", "graph.aggregates.2.subject_kind", "")], "graph aggregate_subject_missing $.graph.aggregates[2].subject_kind"),
    ("member_as_aggregate", [("append", "graph.aggregates", VOLTAGE_AGGREGATE)], "graph not_an_aggregate $.graph.aggregates[4].fact"),
    # Section 15 item 6 and PLN-1.
    ("plane_on_resource", [("set", "graph.resources.0.planes", ["physical"])], "graph plane_membership_stated $.graph.resources[0].planes"),
    ("plane_on_field_rule", [("set", "field_rules.0.planes", ["energy"])], "graph plane_membership_stated $.field_rules[0].planes"),
    # Withheld children: unknown, never absent (section 7.2).
    ("unstated_phase_dropped", [("delete", "unstated_relations.2", None)], "unstated_relations"),
    ("unstated_battery_absent", [("set", "unstated_relations.3.outcome", "absent")], "unstated_relations"),
    ("unstated_battery_from_logger", [("set", "unstated_relations.3.from_kind", "resource.equipment")], "unstated_relations"),
    ("unstated_meter_reason", [("set", "unstated_relations.4.withheld_by", "pv.reason.native_fact_unknown")], "unstated_relations"),
    # Identity (section 6.3).
    ("identity_class_token_allowed", [("delete", "identity.asset_id.forbidden_derivations.7", None)], "identity contract"),
    ("identity_profile_allowed", [("delete", "identity.asset_id.forbidden_derivations.8", None)], "identity contract"),
    ("identity_derivation_restored", [("set", "identity.asset_id.derivation", {"input_fields": ["domain", "configured_instance_label", "class_token", "routed_unit"]})], "identity contract"),
    ("identity_input_dimension", [("set", "identity.resources.inputs.dimension", "pv.dimension.string")], "identity contract"),
    ("identity_inverter_source", [("set", "identity.resources.inverter.source", "gateway_derived_semantic_asset_id")], "identity contract"),
    ("identity_logger_kind", [("set", "identity.resources.logger.kind", "resource.meter")], "identity contract"),
    ("identity_input_slots", [("set", "identity.resources.inputs.slots", 9)], "identity contract"),
    # EN-1: every mapped field rule names its subject kind and measurement point.
    ("field_rule_subject_kind", [("set", "field_rules.0.subject_kind", "resource.inverter")], "field rule subject"),
    ("field_rule_subject_missing", [("delete", "field_rules.2.subject_kind", None)], "field rule subject"),
    ("field_rule_measurement_point_blank", [("set", "field_rules.0.measurement_point", "")], "field rule subject"),
    ("field_rule_measurement_point_moved", [("set", "field_rules.3.measurement_point", "inverter_dc_side")], "field rule subject"),
    ("field_rule_dimension", [("set", "field_rules.5.dimension", "pv.dimension.inverter")], "target definition"),
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
    ("pv.dc.voltage", lambda a: a["subject"].update(kind="resource.inverter"), "field rule subject"),
    ("pv.energy.generated", lambda a: a["subject"].update(kind="resource.inverter"), "graph subject_kind_mismatch $.graph.aggregates[2].subject_kind"),
    ("pv.dimension.input", lambda a: a.update(resource_kind="resource.pv_string"), "graph_resources"),
    ("pv.relation.inverter_input", lambda a: a.update(basis_classes=["configuration"]), "graph basis_class_not_admitted $.graph.relations[2].basis_class"),
    ("pv.relation.inverter_phase", lambda a: a.update(from_kinds=["resource.pv_system"]), "graph_parent"),
    ("pv.ac.aggregate_active_power", lambda a: a.update(aggregation="member"), "graph not_an_aggregate $.graph.aggregates[1].fact"),
])
def test_catalog_facts_are_looked_up_not_repeated(element_id, mutate_attributes, expected):
    assert graph_rejection([], catalog_with(element_id, mutate_attributes)) == expected


def test_withheld_children_stay_unknown_and_never_absent():
    document = contract()
    rows = {(row["definition"], row["from_kind"], row["to_kind"]): row for row in document["unstated_relations"]}
    assert set(rows) == {("pv.relation.array_string", "resource.pv_array", "resource.pv_string"), ("pv.relation.inverter_array", "resource.inverter", "resource.pv_array"),
                         ("pv.relation.inverter_phase", "resource.inverter", "resource.phase"), ("relation.relays", "resource.inverter", "resource.battery"),
                         ("relation.relays", "resource.inverter", "resource.meter")}
    assert {row["outcome"] for row in rows.values()} == {"unknown_not_absent"}
    assert all(not relation["absence_sides"] for relation in document["graph"]["relations"])


def test_one_inverter_through_two_paths_is_joined_only_by_declaration():
    graph = contract()["graph"]
    assert graph["joins"] == [{"kind": "resource.inverter", "observations": ["relayed_by_the_gateway_path", "observed_directly"], "basis": "operator_declaration", "native_tokens": "not_exposed"}]
    assert graph["relayed"] == [{"relaying_kind": "resource.equipment", "relayed_kind": "resource.inverter", "direct_anchor_key_source": "operator_label"}]


@parametrize("source", [
    "modbus.huawei.sdongle:serial-private", "modbus.huawei.sdongle:502",
    "modbus.huawei.sdongle.bad.label:00000000-0000-4000-8000-0000000000d5",
    "modbus.huawei.unknown:00000000-0000-4000-8000-0000000000d5",
    "modbus.huawei.sdongle.serial-private:00000000-0000-4000-8000-0000000000d5",
    "modbus.huawei.sdongle." + "a" * 33 + ":00000000-0000-4000-8000-0000000000d5",
])
def test_managed_namespace_exception_does_not_allow_native_identity_text(source):
    document = case()
    document["source"]["source_id"] = source
    with Raises(ValueError, match="identity_missing_or_invalid"):
        project(document)


if __name__ == "__main__":
    raise SystemExit(support.run(globals()))
