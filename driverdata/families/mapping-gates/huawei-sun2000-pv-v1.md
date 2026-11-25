# Huawei SUN2000 PV mapping gate v1

This source-specific, read-only gate maps the facts that two offline SUN2000
monitoring profiles admit to `helianthus.pack.pv@1.0.0`. It adds no pack and no
vendor-specific semantic family, and it makes no stability promise before
Helianthus 1.0. It is neither a device-support claim nor a live qualification.
Its machine-readable companion is
[the mapping vectors](huawei-sun2000-pv-v1.json). The vectors are synthetic and
derive from the published native contract; they are not observations from
hardware.

The same family set has two sibling gates that map nothing today:
[the LUNA2000 storage gate](huawei-luna2000-storage-v1.md) and
[the gateway-meter infrastructure gate](huawei-gateway-meter-infrastructure-v1.md).

## Evidence basis

| Source | Reference | Role |
| --- | --- | --- |
| Native monitoring contract | `helianthus-docs-modbus` issue 242, pull request 253: `protocols/huawei/monitoring-readonly-v1.md` and `protocols/huawei/read-pacing-v1.md` | Owns every register group, word form, gain, unit, sign and grade. This gate restates none of them as new protocol fact. |
| Registry profiles | `helianthus-modbusreg` issue 348, pull request 357 | Two inert monitoring profiles, the typed facts they decode, the refusal map and the dispositions. |
| Program | `helianthus-modbusreg` issues 199 and 201 | The Huawei read-only program this gate belongs to. |
| Multi-endpoint contract | `helianthus-docs-ebus` issue 746, pull request 748 | Finalized source keying per endpoint instance. Its class token is provenance here, never identity. |

No revision, digest or pinned link is cited: the references above are issue and
pull request numbers. Grades are those of the monitoring contract. Proven means
a live reading was decoded with the stated form and agreed with an independent
check at the observed units; Hypothesis means a document states it or a reading
was plausible without an independent check; Unknown means neither settles it.
Grades are separate from admission, qualification and support. The grade of a
published fact is inherited and never raised: Proven only where the monitoring
contract is Proven. A rule of this gate that goes beyond the contract is a
Hypothesis.

## Admitted profiles and selection

| Profile | Family and path | Edition binding | Facts |
| --- | --- | --- | --- |
| `huawei.sun2000.m3.monitoring-readonly@1.0.0` | SUN2000 M3 series behind the SmartLogger | The consulted edition's supported-model table names the model (Proven). | 19: eight string voltages, eight string currents and three energy counters. The power group is refused with exception 3 and the cause is Unknown. |
| `huawei.sun2000.m1.monitoring-readonly@1.0.0` | SUN2000 M1 series behind the S-Dongle | The edition does not list the model; the contract checked its layout against the child's reading (Proven, one reading). | 22: the same 19, input power, active power and the device status code. |

Both profiles are pinned to one consulted edition of the vendor's SUN2000
interface definition: `SUN2000MC V200R023C00 Modbus Interface Definitions`,
Issue 01, dated 2023-03-17. The edition is cited by identity only and is
inspection-only; nothing of it is reproduced here. Its information is attributed
to Huawei Technologies Co., Ltd., which retains the copyright. The M3 binding is
Proven because the edition's supported-model table names the model. The M1
binding is a live check of the edition's layout against one reading, so the
family-wide statement is a Hypothesis.

The gate runs only on a registry selection of `PROFILE_SELECTED` for one of these
two exact profile identifiers. `INSUFFICIENT_EVIDENCE`, `NO_ADMISSIBLE_PROFILE`
and `AMBIGUOUS` publish nothing. Firmware is neither a predicate nor an identity
input. The five registry flags `catalog_registered`,
`automatic_runtime_admission`, `live_qualified`, `support_claim` and
`write_authority` are all false and the gate requires them false; a live
qualification needs a revision of this gate. `mapping_qualified=true` describes
this offline projection only, and `outbound_allowed=false` grants no operation.

The SUN2000 MAP0 series behind the EMMA has no consulted edition. Its
disposition is `NO_ADMISSIBLE_PROFILE` with the reason `no_consulted_edition`.
This gate has no field for it: its monitoring groups answered, but their words
stay native evidence and no PV fact is published from them (the layout is a
cross-family Hypothesis). Vector `PV-NEG-001-emma-map0-not-admitted` pins that.

## Identity and keying

Identity follows sections 6.3 and 15 of the [graph contract](../graph.md). The
gateway allocates every identifier; this gate states what each one names and
what never derives it.

- The home asset is gateway-allocated, one per admitted inverter child within
  an endpoint instance. A per-asset configuration field is not required. The
  asset is kernel lineage only: no fact key and no relation endpoint carries
  it.
- The source identifier is the existing managed instance identifier, derived
  from its DriverID and configured owner identity, with exactly one source per
  endpoint instance under the finalized multi-endpoint contract of
  `helianthus-docs-ebus` issue 746 (merged pull request 748). Its fixed family
  namespace is configuration, not observed native identity. A binding
  identifier is derived by the gateway over the asset, the source and the
  profile only; it never selects or changes the asset or a resource.
- Every resource dimension value and every relation endpoint is a
  gateway-allocated resource identifier (RES-5, REL-8): one for the gateway
  path that relays the child, one for the PV system, one for the inverter and
  one for each of the eight string slots. No phase identifier is allocated,
  because no profile admits a per-phase word.

The class token of the PV pack and the registry profile identifier are native
interpretation selectors, and the gate classifies both as provenance (section
6.3). Neither is an anchor source, an input of the asset or part of any
identity. A hybrid inverter whose PV and storage facts both publish therefore
stays one inverter and one home asset: the class token no longer splits it in
two. Neither the asset, the source nor a resource identifier derives from a
serial number, an ESN, observed vendor or model text, firmware, an endpoint
address, a routed unit alone, a class token or a profile identifier.

Missing, invalid, reused or ambiguous identity fails closed. One identifier
never names two resources, and a resource identifier never equals the asset,
the source or the binding. The validator permits the managed Huawei source
namespace with an owner GUID while rejecting native identity text; this is a
syntax lint, not proof of how an identifier was derived. The gate publishes
nothing at endpoint or gateway level. The projection vectors use short
synthetic opaque identifiers to isolate mapping behavior; they are not
evidence of gateway identity construction.

## Resources

This is section 15 item 1. A component is anchored by its native position and
everything else by an operator label, because the element catalog lists no
identification scheme for this family. An address, a routed unit, a
credential, a class token or a profile identifier is never an anchor source.

| Kind | Anchor class | Basis | Anchor source | Parent kind |
| --- | --- | --- | --- | --- |
| `resource.equipment` | equipment | `operator_slot` | `operator_label` | none |
| `resource.pv_system` | function | `operator_slot` | `operator_label` | none |
| `resource.inverter` | equipment | `operator_slot` | `operator_label` | none |
| `resource.dc_input` | component | `native_slot` | `native_position` | `resource.inverter` |
| `resource.phase` | component | `native_slot` | `native_position` | `resource.inverter` |

- The gateway path of the profile, the SmartLogger or the S-Dongle, is generic
  equipment. It has no token on the admitted read plans.
- The inverter is anchored by an operator label until an identification
  scheme is listed for its family. The child key of the native monitoring
  contract is recorded as its future native token. Its stability is Unknown
  until the native owner states it, and the admitted read plans do not read
  it. Once a scheme is listed, the anchor is upgraded by supersession (section
  6.5) and the inverter keeps its identifier.
- The PV system is a function anchored by an operator label. This gate gives
  each admitted inverter child its own system.
- A DC input is a component of its inverter at the native string index 1 to 8.
- A phase is a component of its inverter at its native position. The kind is
  stated so that its anchor is fixed, but no phase is allocated or published
  until a profile admits a per-phase word.

## Relations

This is section 15 item 2. Each relation record of the projection names its
definition, the identifiers of two published resources and its basis class.
The validity scope of every row is the two admitted profiles at version 1.0.0.

| Definition | Basis class | From kind | To kind | Scope of the rule |
| --- | --- | --- | --- | --- |
| `relation.relays` | `native_observation` | `resource.equipment` | `resource.inverter` | `each_admitted_child_is_read_through_the_gateway_path_its_profile_names` |
| `pv.relation.system_inverter` | `gate_rule` | `resource.pv_system` | `resource.inverter` | `one_pv_system_per_admitted_inverter_child` |
| `pv.relation.inverter_input` | `gate_rule` | `resource.inverter` | `resource.dc_input` | `native_string_index_1_to_8_with_a_nonzero_voltage_or_current_word` |

- Each profile names the gateway path its reads go through, so the binding that
  reads a child observes the relay. The basis is a native observation, a
  Hypothesis until the native owner's review confirms its scope. One record is
  emitted whenever the inverter is published.
- The gate emits one system-to-inverter record when a system or inverter fact
  is published, and one inverter-to-input record for each published input.
- The gate makes no absence statement. A zero pair does not show that an input
  is absent, and a withheld child is not absent.

Relayed values follow section 15 item 3. If the same inverter is also observed
directly, its anchor source there is again an operator label. The gate
therefore states one join: the relayed and the direct observation are one
resource only by an operator declaration that gives both one slot. The cut
then flags the pair `same_unit`, or one asset holds both paths with an
identity link (section 6.4). Without a shared key the pair is not detectable
(section 6.3). A join on a label, a routed unit or an ordinal is rejected, and
so is a join of observations whose native tokens differ (a false join).

The gate states no plane membership (section 15 item 6, PLN-1).

## Unknown, not absent

Section 7.2: when neither an instance nor an absence statement exists, a
relation is unknown, not absent. These relations stay unknown here.

| Definition | From kind | To kind | Withheld by |
| --- | --- | --- | --- |
| `pv.relation.array_string` | `resource.pv_array` | `resource.pv_string` | `unsupported:array_or_string_topology` |
| `pv.relation.inverter_array` | `resource.inverter` | `resource.pv_array` | `unsupported:array_or_string_topology` |
| `pv.relation.inverter_phase` | `resource.inverter` | `resource.phase` | `unsupported:per_phase_values` |
| `relation.relays` | `resource.inverter` | `resource.battery` | `pv.reason.battery_outside_pv_family` |
| `relation.relays` | `resource.inverter` | `resource.meter` | `pv.reason.meter_outside_pv_family` |

The battery and meter groups are fields of the inverter child's profile. They
are withheld here and governed by the storage and gateway-meter gates, so the
inverter's relay of a battery or a meter is neither stated nor denied.

## Subjects and measurement points

Under EN-1 each mapped field rule names its subject kind and its measurement
point. The subject kind and the dimension are looked up in the
[element catalog](../element-catalog-v1.json), not repeated by the validator.

| Native fact | Target fact | Subject kind | Measurement point |
| --- | --- | --- | --- |
| `pv_string_voltage` | `pv.dc.voltage` | `resource.dc_input` | `dc_input_terminals` |
| `pv_string_current` | `pv.dc.current` | `resource.dc_input` | `dc_input_terminals` |
| `input_power` | `pv.dc.aggregate_power` | `resource.inverter` | `inverter_dc_side` |
| `active_power` | `pv.ac.aggregate_active_power` | `resource.inverter` | `inverter_ac_output` |
| `device_status` | `pv.status.operating` | `resource.inverter` | `inverter_as_a_whole` |
| `energy_accumulated` | `pv.energy.generated` | `resource.pv_system` | `inverter_ac_output_lifetime_counter` |
| `energy_current_day` | `pv.energy.yield_day` | `resource.pv_system` | `inverter_ac_output_day_counter` |

Aggregates follow section 15 item 4 and EN-5. The input power and the active
power are aggregates of the inverter. The native words state no aggregation
basis, so both stay on the inverter without coverage. The lifetime energy
counter is the inverter's own, measured at its AC output. The pack's only
generated-energy fact has the PV system as subject, so the counter is published
on the system this gate gives the inverter, whose coverage is that one
system-to-inverter relation. The current-day yield is the same kind of
aggregate, on the same system, with the same coverage. Neither is summed
across inverters. Placing either on the inverter itself needs an
inverter-subject energy definition, which the pack does not have (see the
unresolved questions). The monitoring contract has no month or year yield
word, so those pack yields stay prospective on the hour counter and are not
published.

Selectors follow section 15 item 5. The registry profile identifier and the
pack class token are the only interpretation selectors this gate uses, and
both are provenance.

## Field mapping

Native word forms are the contract's: big-endian with the most significant word
first (Proven at all three inverter families), two's complement, and a physical
value equal to the raw integer over the gain (graded per field below). The SI
unit column names the pack field's unit and the exact factor and offset of the
conversion, so that the SI value is the native value times the factor plus the
offset.

| Native fact | Native form | Target fact | SI unit and conversion | Dimension | Disposition | Loss | M3 grade | M1 grade |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `pv_string_voltage` | I16, V, gain 10 | `pv.dc.voltage` | `unit.volt`, factor 1, offset 0 | `pv.dimension.input` | exact | none | Proven | Proven |
| `pv_string_current` | I16, A, gain 100 | `pv.dc.current` | `unit.ampere`, factor 1, offset 0 | `pv.dimension.input` | exact | none | Hypothesis | Proven |
| `input_power` | I32, kW, gain 1000 | `pv.dc.aggregate_power` | `unit.watt`, factor 1000, offset 0 | `pv.dimension.inverter` | transformed | unit+provenance | Unknown | Proven |
| `active_power` | I32, kW, gain 1000 | `pv.ac.aggregate_active_power` | `unit.watt`, factor 1000, offset 0 | `pv.dimension.inverter` | transformed | unit+provenance | Unknown | Proven |
| `device_status` | U16, code | `pv.status.operating` | none | `pv.dimension.inverter` | transformed | symbol | Unknown | Hypothesis |
| `energy_accumulated` | U32, kWh, gain 100 | `pv.energy.generated` | `unit.joule`, factor 3600000, offset 0 | `pv.dimension.system` | transformed | unit+policy | Proven | Hypothesis |
| `energy_current_day` | U32, kWh, gain 100 | `pv.energy.yield_day` | `unit.joule`, factor 3600000, offset 0 | `pv.dimension.system` | transformed | unit+policy | Hypothesis | Hypothesis |

The grade columns are ceilings. A Proven fact is already graded Hypothesis by the
registry at a unit that was not read for monitoring, and this gate takes the
lower of the fact's grade and the ceiling, so it never raises a grade. The M3
power rows are Unknown because the family refused the power group; the fact then
has no value and the item is withheld with `pv.reason.power_group_refused`.

Rules, each with the grade of the contract row it restates:

1. **Exact decimal.** A quantity is the kernel pair of an integer coefficient
   string and a base-ten exponent. A trailing zero of the coefficient moves into
   the exponent and zero is `0` with exponent 0. There is no binary floating
   point: the validator rejects a float literal in the contract and in a case
   (Proven as a statement of the kernel). A coefficient is never read from text
   with a decimal point.
2. **Units.** Each fact is published in the SI unit of its pack field under the
   [unit rule](../../README.md#unit-rule). Volt and ampere keep the native
   number. Kilowatt at gain 1000 is published as `unit.watt` with the same
   integer (factor 1000), and kilowatt-hour as `unit.joule` by the exact factor
   3600000. Every conversion that is not the identity is recorded as a
   reversible `unit` loss so that it is never silent, and the native value and
   unit stay in the evidence (Proven: the integer is the contract's raw value).
   Every factor here terminates in decimal, so no conversion of this gate
   rounds.
3. **Range.** The pack's bounds are exact decimals. A decoded value outside them
   is withheld with `pv.reason.value_outside_pack_range` and is never clamped.
4. **Sign.** Active power is published as the signed watt value, including a
   negative value. The native reference remains positive while producing, and
   the aggregation basis stays provenance. The input power has no stated sign,
   and a negative value is outside the pack's bounds.
5. **Aggregates.** The input power and the active power are single words per
   inverter. They map to the pack's aggregate fields, with the aggregation basis
   of the native words kept as provenance, because the contract does not state
   it (Unknown) and the pack assumes none. They are never split into per-input
   or per-phase values.
6. **Input presence.** The profiles decode eight string slots whether or not an
   input exists behind them. A slot whose voltage and current words are both
   zero does not show that the input exists, so neither is published and no
   membership record is created (`pv.reason.input_presence_unknown`; Hypothesis,
   an inference of this gate). The words stay native evidence. A slot with any
   non-zero word is present, and a zero current at a non-zero voltage is
   published as zero.
7. **Status symbol.** Only the observed grid-connected state has a symbol, and it
   is published as `pv.status.operating.generating` with a `symbol` loss that is
   not reversible. The contract grades its meaning for the family as a
   Hypothesis, and every other code is Unknown and withheld
   (`pv.reason.state_code_not_observed`). No idle, standby, derating or fault
   symbol is published from the status words.
8. **Unknown is never zero.** A fact without a value publishes nothing and is
   listed with its reason. The same holds for an unanswered group, a refused
   group and an absent optional group.

## Energy counter policy

`energy_accumulated` is published as read as `pv.energy.generated`, and
`energy_current_day` is published as read as `pv.energy.yield_day`. Both are
converted exactly to joule. Reset, rollover and continuity are Unknown because
none was observed, so a decrease between two observations never proves a reset
or a wrap, native evidence is required to say so, and the policy loss is not
reversible. The day yield is the counter as read. There is no derived energy: nothing
is computed from the lifetime counter, the hour counter, power, or another
inverter. The hour counter stays
unpublished: it is not a pack yield period. The monitoring contract has no
month or year yield word, so `pv.energy.yield_month` and `pv.energy.yield_year`
stay prospective and unpublished. Nothing is summed across inverters into a
plant total. The device statistics time is never used as a sample time.

## Withheld and native-only

Each native item of each profile has exactly one disposition. `native_only`
corresponds to the kernel outcome `unrepresentable`, and every withheld item
names a reason that declares its loss.

| Native item | Disposition | Reason | Loss kind | Kernel outcome |
| --- | --- | --- | --- | --- |
| `energy_current_hour` | withheld | `pv.reason.period_counter_time_domain_not_represented` | time | withheld |
| `status_group_words` | native_only | `pv.reason.state_and_alarm_meanings_unknown` | symbol | unrepresentable |
| `string_group_tail_words` | native_only | `pv.reason.no_defined_input_beyond_eighth` | capability | unrepresentable |
| `power_group_other_words` | withheld | `pv.reason.field_not_consumed_by_profile` | policy | withheld |
| `dc_input_energy_counter` | native_only | `pv.reason.no_pack_fact_for_dc_input_energy` | capability | unrepresentable |
| `statistics_time_words` | native_only | `pv.reason.statistics_time_domain_unresolved` | time | unrepresentable |
| `battery` | withheld | `pv.reason.battery_outside_pv_family` | capability | withheld |
| `meter` | withheld | `pv.reason.meter_outside_pv_family` | capability | withheld |

The status group is retained as bit fields whose meanings are Unknown, and one of
its words is derived from a serial number and is never published. The words of
the power group that are not consumed are the contract's check anchors (grid
voltages and currents, frequency, reactive power, efficiency, temperature). The
pack has fields that could carry some of them (`pv.ac.voltage`, `pv.ac.current`,
`pv.ac.frequency`, `pv.ac.power_factor`, `pv.temperature.inverter`), so they are
withheld rather than native-only; they become mappable only when a profile
admits them, and no per-phase fact is published until then.

## Explicit non-mappings

| Subject | Statement | Grade |
| --- | --- | --- |
| Storage | No battery fact is admitted by any profile. The battery groups answered with data on the MAP0 series only, and their meanings are Unknown. A zero word at a battery offset on the M3 and M1 series is never a state of charge, power or energy of zero. The [storage gate](huawei-luna2000-storage-v1.md) owns what the Storage family will take. | Proven (zero words); Hypothesis (zero rule); Unknown (meanings) |
| Meters | No meter register was read on any path and no meter class contract exists, so every meter value is Unknown. The [gateway-meter gate](huawei-gateway-meter-infrastructure-v1.md) owns that boundary. | Unknown |
| MAP0 series | No consulted edition and no profile. | Proven |
| Plant groups | The logger plant groups, the EMMA site groups and the S-Dongle plant groups are planned reads that were never read. They have no profile, and no plant or site PV fact is published from them or from a sum of inverters. | Hypothesis (documentary) |
| M3 power group | The family refused the group and the cause is Unknown; the candidate extents are Unknown. The gate never requests them. | Unknown |
| Operations | No active-power limit, export limit, enable or disable, write, acknowledgement, readback, retry or route exists. | Proven as a statement of the contract |

## Lifecycle, retention and partial publication

A case carries the observation identifier and revision, the receipt on a wall
and a monotonic axis, the clock epoch, the source epoch, the driver generation,
the connection generation, the profile selection, the group outcomes and every
fact the profile decodes. Missing or invalid evidence rejects the case; the first
error of this order is reported: profile not selected, native evidence invalid,
identity missing or invalid, lifecycle missing, mapping unqualified, unsupported
or withheld.

Publication is partial. A fact the profile did not supply is withheld with its
reason while the others publish. When a group fails in the current observation,
the registry keeps its last complete reading with its original observation; the
gate publishes those facts with that observation and its original receipt wall
time, monotonic time and clock epoch, and marks them `retained`, without raising
their grade. Each retained fact carries its original source epoch, driver
generation, connection generation, profile, routed unit, asset, binding and source
identifiers, and the gate rejects
it when any of them differs. Monotonic ticks are ordered only within one clock
epoch. A retained reading never crosses a profile, a unit
or a connection identity, and a changed identity fences earlier readings.
Withdrawal is explicit and generation-fenced.

## Acceptance vectors

The first two vectors carry the complete expected output of each admitted
profile; the others carry a summary derived from the same projection. The vector
files use synthetic identifiers and values only.

| Vector | What it proves |
| --- | --- |
| `PV-POS-001-m1-baseline` | Two populated inputs publish, six empty inputs are withheld, and the observed status symbol, both powers and the counter publish with unit conversion and canonical decimals. |
| `PV-POS-002-m3-baseline` | Eight populated inputs publish and the refused power group withholds power and status, while the accumulated counter is Proven. |
| `PV-POS-003-negative-active-power` | A negative active power is withheld and every other fact is published. |
| `PV-POS-004-retained-energy-group` | A failed energy group keeps its last complete reading with its original observation, marked retained. |
| `PV-POS-005-unobserved-status-code` | An unobserved status code stays unknown and publishes no symbol. |
| `PV-POS-006-unobserved-unit-grade-cap` | A unit that was not read for monitoring carries Hypothesis and the gate keeps it. |
| `PV-POS-007-grade-capped-at-contract-ceiling` | A grade above the contract's grade is capped at the ceiling. |
| `PV-POS-008-voltage-out-of-range` | A voltage beyond the pack bound is withheld and not clamped. |
| `PV-POS-009-all-inputs-zero` | Zero pairs publish no input and create no input relation. |
| `PV-POS-010-power-group-not-answered` | A power group that did not answer leaves the three power facts unknown and publishes the rest. |
| `PV-POS-011-canonical-decimal` | A trailing zero moves into the exponent and a zero power has exponent zero. |
| `PV-NEG-001-emma-map0-not-admitted` | The MAP0 series has no consulted edition and publishes nothing. |
| `PV-NEG-002-unknown-profile` | A profile identifier that no exact pair admits selects nothing. |
| `PV-NEG-003-ambiguous-selection` | An ambiguous selection has no winner and publishes nothing. |
| `PV-NEG-004-gain-misapplied` | A typed value that is not the words over the gain is rejected. |
| `PV-NEG-005-word-order-swapped` | The most significant word comes first. |
| `PV-NEG-006-fact-outside-profile` | A power fact on the M3 profile is outside its admitted fields. |
| `PV-NEG-007-decimal-point-in-coefficient` | A coefficient is an integer string. |
| `PV-NEG-008-missing-fact` | Every admitted fact is present exactly once. |
| `PV-NEG-009-value-from-unanswered-group` | A value needs an answered group or a retained reading. |
| `PV-NEG-010-asset-missing` | A blank asset identifier fails closed. |
| `PV-NEG-011-asset-equals-source` | The asset and the source stay distinct. |
| `PV-NEG-012-asset-from-serial` | An identifier that embeds serial text is rejected. |
| `PV-NEG-013-asset-from-unit-alone` | An identifier equal to the routed unit is rejected. |
| `PV-NEG-014-lifecycle-missing` | The monotonic receipt axis is required. |
| `PV-NEG-015-live-qualified` | A live qualification requires a revision of this gate. |
| `PV-NEG-016-operation` | The gate has no operation, route or authority. |
| `PV-NEG-017-derived-energy` | No energy is derived or summed. |
| `PV-NEG-018-error-precedence` | The first error of the precedence list is reported. |
| `PV-NEG-019-retained-from-other-connection` | A retained reading from an earlier connection generation is fenced. |
| `PV-NEG-020-retained-from-other-profile` | A retained reading never crosses a profile. |
| `PV-NEG-021-retained-without-origin` | A retained reading must carry its original receipt coordinates and lineage. |
| `PV-NEG-022-relation-endpoint-unallocated` | A relation endpoint needs an allocated resource identifier; without one the case fails closed and no record dangles. |
| `PV-NEG-023-one-identifier-two-resources` | One resource identifier never names two resources. |

## Unresolved and falsifiers

| Question | Falsifier | Grade |
| --- | --- | --- |
| Can an admitted instance distinguish its children? | Two admitted children that the gateway cannot tell apart; the ambiguous children must be withheld, and identity fails closed. | Hypothesis |
| Which native token identifies an inverter stably, and in which canonical form? | The native owner states the child key's stability and a scheme is listed for the family; the inverter anchor is then upgraded by supersession. | Unknown |
| Does the gateway path relay each child as a native observation for both profiles? | The native owner's review states another scope, or names configuration as the basis. | Hypothesis |
| Should the lifetime energy counter have the inverter as subject? | An inverter-subject energy definition in the PV pack; until then the counter stays on the one-inverter PV system. | Hypothesis |
| Do the string slots map to the pack's input one to one? | A ratings read that gives the string and tracker counts, or a disagreeing independent check. | Hypothesis |
| Is an all-zero pair an absent input? | A second operating point or a ratings read that shows a present input reading zero. | Hypothesis |
| Does the observed grid-connected state mean generating? | An observation where that state reads zero active power while producing nothing, or a consulted edition that states another meaning. | Hypothesis |
| Why does the M3 series refuse the power group? | An operator-approved read of a value-aligned extent. | Unknown |
| Do the energy counters reset or roll over? | An observation across a reset or the day boundary. | Unknown |

## Later consumer cutover

Consumers read these facts only through the shared semantic snapshot (the
semantic snapshot MCP read and the GraphQL `semanticSnapshot`). This gate
publishes no consumer binding. Any implementation must bind consumers in one
atomic cutover, must not reuse a single-asset vendor projection, and must keep
`outbound_allowed=false`.

## Validation

Run `python3 scripts/validate_huawei_sun2000_pv_mapping.py`,
`python3 scripts/test_validate_huawei_sun2000_pv_mapping.py` (also collected by
`python3 -m pytest`), and `./scripts/check_docs.sh`, which runs both. The validator runs the shared graph
rules of `scripts/mapping_gate_graph_rules.py` and then its own resource,
relation, relayed, join, aggregate and selector rules. It executes every vector,
checks each target against the PV pack catalog and the element catalog, checks
that every native field of each profile has exactly one disposition, and
compares the tables of this page with the contract.
