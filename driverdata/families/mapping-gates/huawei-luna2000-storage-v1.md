# Huawei LUNA2000 storage mapping gate v1

This gate states what `helianthus.pack.storage@1.1.0`, the generic Storage
family that the Growatt storage mapping also uses, takes from Huawei
LUNA2000 observations. It admits the facts of the native monitoring contract
for the exact family and publishes them. It adds no pack and no vendor-specific semantic family, and
it makes no stability promise before Helianthus 1.0. A live qualification observed on a record is a recorded
level and does not reject its mapping. Its machine-readable companion is
[the mapping vectors](huawei-luna2000-storage-v1.json); the vectors are
synthetic and are not observations from hardware.

The site state of charge, the plant battery power and the site lifetime energy
counters are mapped and published; the site day counters and the registry groups are served
natively; every other item is withheld with an explicit reason and loss. The sibling gates are
[the SUN2000 PV gate](huawei-sun2000-pv-v1.md), which maps the admitted inverter
facts, and [the gateway-meter gate](huawei-gateway-meter-infrastructure-v1.md).

## Evidence basis

| Source | Reference | Role |
| --- | --- | --- |
| Native monitoring contract | `helianthus-docs-modbus` issue 242, pull request 253: `protocols/huawei/monitoring-readonly-v1.md` and `protocols/huawei/read-pacing-v1.md` | Owns the battery rule, the planned site and plant groups, and every grade. |
| Registry dispositions | `helianthus-modbusreg` issue 348, pull request 357 | The LUNA2000 disposition stays `NO_ADMISSIBLE_PROFILE` and records the three battery groups as present but not admitted. |
| Program | `helianthus-modbusreg` issues 199 and 201 | The Huawei read-only program this gate belongs to. |
| Multi-endpoint contract | `helianthus-docs-ebus` issue 746, pull request 748 | Asset and source keying per endpoint instance. Its text was not final when this gate was written. |

No revision, digest or pinned link is cited. Grades are those of the monitoring
contract, kept separate from admission, qualification and support: Proven needs a
live reading checked independently, Hypothesis is a document statement or a
plausible unchecked reading, and Unknown is unsettled. A rule of this gate that
goes beyond the contract is a Hypothesis.

## Decision: the generic Storage family

The operator decision on the storage publication family is to publish Huawei
battery facts into the generic Storage family and to create no new family. This
gate therefore keys every prospective fact to a `storage.*` definition of
`helianthus.pack.storage@1.1.0`. Those facts are read only through the shared
semantic snapshot. The Growatt-specific single-asset storage projection and its
machine-to-machine root are not reused or cloned.

## Admission state

The gate declares its native profile source: the hybrid-inverter battery and
monitoring contract of `helianthus-docs-modbus`
(`protocols/huawei/monitoring-readonly-v1.md` and
`protocols/huawei/inverter-battery-readonly-v1.md`), and it admits facts from
it. The registry outcome `PROFILE_SELECTED` applies the mapping. The outcomes
`INSUFFICIENT_EVIDENCE`, `NO_ADMISSIBLE_PROFILE` and `AMBIGUOUS` are unresolved
inputs: every mapped item is then withheld with
`storage.reason.no_admitted_battery_profile` and nothing is zero. A selection
outside that set is `native_evidence_invalid`. The registry flags
(`catalog_registered`, `automatic_runtime_admission`, `support_claim`) stay
false, a live qualification (`live_qualified`) is a recorded level and not a
rejection, and `outbound_allowed=false` grants no operation.

Each remaining precondition is decided from the documents, with its grade:

1. the battery profile is declared from the profile source for the exact family
   (Hypothesis);
2. type, word order, gain and unit of each consumed word are stated per row
   (Hypothesis);
3. the battery power sign: the plant group states positive while charging
   (Hypothesis) and the site group states none, so site power is withheld;
4. the aggregation basis, site group or plant group, is recorded as provenance
   on each fact (Proven);
5. the asset granularity is one storage system with its battery packs as the
   pack defines (Proven);
6. the executable vectors are the scenarios of this gate (Proven).

A zero word at a registry battery offset is never a state of charge, power or
energy of zero, and battery presence is never decided from it. The one admitted
exception is the routed site state of charge row, whose answered zero word is a
valid 0% reading (see the routes and zero words section). A raw group read that has
no storage element in the pack is served in the modbusreg namespace until a
catalog row gives it an element, with `storage.reason.group_native_only_no_catalog_row`.

## Identity and keying

Identity follows sections 6.3 and 15 of the [graph contract](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/graph.md), the
rule the PV gate of the same hybrid inverter family follows. The gateway
allocates every identifier; this gate states what each one names and what never
derives it.

- The home asset is gateway-allocated: it is the home asset of the inverter
  child that relays the battery. The asset is kernel lineage only, and no fact
  key and no relation endpoint carries it.
- The source identifier is the managed instance identifier of the endpoint
  instance, one source per endpoint instance. A binding identifier is derived
  by the gateway over the asset, the source and the profile only.
- Every resource dimension value and every relation endpoint is a
  gateway-allocated resource identifier (RES-5, REL-8): one for the inverter,
  one for the battery and one for each pack at its native position 1 to 3
  under the configured battery. A fact of a site or plant group is published on
  the first pack, the pack as a whole, with its aggregation basis as provenance.

The class token of the Storage pack and the registry profile identifier are
native interpretation selectors, and the gate classifies both as provenance
(section 6.3). A hybrid inverter whose PV and storage facts both publish
therefore stays one inverter and one home asset. Neither the asset, the source
nor a resource identifier derives from a serial number, an ESN, the vendor, the
model, firmware or any version text, a routed unit alone, an endpoint address, a
class token or a profile identifier. Missing, invalid, reused or ambiguous
identity fails closed. A published fact carries the resource identifier of its pack.

## Resources

This is section 15 item 1. The element catalog lists no identification scheme
for this family, so a resource is anchored by an operator label, except a pack
and an interface, which are anchored by their native position under the
configured battery. An address, a routed unit, a credential, a class token or
a profile identifier is never an anchor source.

| Kind | Anchor class | Basis | Anchor source | Parent kind |
| --- | --- | --- | --- | --- |
| `resource.inverter` | equipment | `operator_slot` | `operator_label` | none |
| `resource.battery` | equipment | `operator_slot` | `operator_label` | none |
| `resource.battery_pack` | equipment | `native_slot` | `native_position` | none |
| `resource.storage_system` | function | `operator_slot` | `operator_label` | none |
| `resource.storage_interface` | component | `native_slot` | `native_position` | `resource.battery_pack` |

- The inverter is the same resource as in the PV gate, with the same anchor.
  The child key of the native monitoring contract is its future native token,
  and its stability is Unknown until the native owner states it.
- The battery is equipment of its own, relayed by the inverter. The batteries
  are registers of the inverter units (Proven), and the relaying path exposes
  no anchor key source for the battery: no battery token is on it.
- A pack is identified by its native position under the configured battery:
  pack 1, pack 2 and pack 3 stay the same start after start. Its identity is
  never an address and never an operator label.
- The storage system is a function the operator declares, one for each
  configured battery. The batteries and their packs are its asset granularity.
- An interface is a component of its pack at the native position of the pack:
  the storage unit's bus voltage and current words state it.

## Relations

This is section 15 item 2. The validity scope of every row is the registry
profile `huawei.luna2000` at the monitoring contract version `monitoring_contract_v1`.

| Definition | Basis class | From kind | To kind | Scope of the rule |
| --- | --- | --- | --- | --- |
| `relation.relays` | `configuration` | `resource.inverter` | `resource.battery` | `each_battery_the_operator_attaches_to_an_inverter_child` |
| `storage.relation.battery_pack` | `gate_rule` | `resource.battery` | `resource.battery_pack` | `one_record_per_pack_at_its_native_position` |
| `storage.relation.pack_interface` | `gate_rule` | `resource.battery_pack` | `resource.storage_interface` | `one_interface_per_pack_at_the_native_position_of_its_pack` |
| `storage.relation.system_battery` | `configuration` | `resource.storage_system` | `resource.battery` | `the_storage_system_the_operator_declares_for_each_configured_battery` |

- The battery's attachment to an inverter is configuration. No native owner
  documents it as observable, and a zero word at a battery offset never decides
  presence. It becomes a native observation only when the native owner
  documents it as observable for a stated scope.
- The battery-to-pack and pack-to-interface relations are gate rules. The
  catalog admits no other basis for them. The system-to-battery relation is
  configuration, the only basis the catalog admits for it.
- The gate makes no absence statement. A withheld battery group is not an
  absent battery.

Relayed values follow section 15 item 3. The inverter relays the battery, and
no direct path to the same battery is documented for this family, so the gate
states that no anchor key source exists for a direct view. A second view of the
same battery, for example through its own management bus, is therefore one
resource only by an operator declaration that gives both views one slot
(section 6.4). Without a shared key the pair is not detectable (section 6.3). A
join by a name, a label text, a position or an ordinal is rejected, and so is a
join of views whose native tokens differ (a false join). A relation record
whose endpoint is a module or a cell is rejected as a dangling endpoint,
because the gate publishes neither.

Aggregates follow section 15 item 4 and EN-5. Every requested Storage fact is a
member fact of the pack in the element catalog, so the gate states no
aggregate. The planned site and plant values remain withheld (rule 4 below).
Selectors follow section 15 item 5: the registry profile identifier and the
pack class token are the only interpretation selectors, and both are
provenance. The gate states no plane membership (section 15 item 6, PLN-1).

## Unknown, not absent

Section 7.2: when neither an instance nor an absence statement exists, a
relation is unknown, not absent. These Storage relations stay unknown here.

| Definition | From kind | To kind | Withheld by |
| --- | --- | --- | --- |
| `storage.relation.module_cell` | `resource.battery_module` | `resource.battery_cell` | `native_contract:no_module_or_cell_structure_stated_for_a_pack` |
| `storage.relation.pack_module` | `resource.battery_pack` | `resource.battery_module` | `native_contract:no_module_or_cell_structure_stated_for_a_pack` |
| `storage.relation.pack_string` | `resource.battery_pack` | `resource.battery_string` | `native_contract:no_string_structure_stated_for_a_pack` |

## Subjects and measurement points

Under EN-1 each prospective row names its subject kind and its measurement
point. The subject kind and the dimension are looked up in the
[element catalog](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/element-catalog-v1.json), not repeated by the validator,
and the measurement points are a Hypothesis of this gate.

| Native item | Storage fact | Subject kind | Measurement point |
| --- | --- | --- | --- |
| `battery_group_37000_30` | `storage.status.operating` | `resource.battery_pack` | `pack_as_a_whole` |
| `site_battery_soc` | `storage.state.soc` | `resource.battery_pack` | `pack_as_a_whole` |
| `site_battery_power` | `storage.pack.power` | `resource.battery_pack` | `pack_terminals` |
| `plant_battery_power` | `storage.pack.power` | `resource.battery_pack` | `pack_terminals` |
| `site_battery_energy_charged_total` | `storage.energy.charged` | `resource.battery_pack` | `pack_terminals_lifetime_counter` |
| `site_battery_energy_discharged_total` | `storage.energy.discharged` | `resource.battery_pack` | `pack_terminals_lifetime_counter` |
| `site_rated_storage_capacity` | `storage.energy.nominal_capacity` | `resource.battery_pack` | `pack_as_a_whole` |

## Routes and zero words

A planned read is evidence only from the routed unit its group is documented at:
the site groups answer at unit 0 of the EMMA and the plant group at unit 100 of
the S-Dongle. A read of a routed group that names another unit, or names none, is
`native_evidence_invalid`, so a site reading is never admitted from the plant
route and a plant reading never from the site route. The registry battery groups
state no routed unit and are identified by offset and quantity. A zero word is
judged on the field's own words and never on the whole read: a state of charge
word of zero is a value, distinct from an unknown value; a battery power or rated
capacity word of zero is withheld with
`storage.reason.zero_word_is_not_a_battery_value` whatever its neighbours hold;
a lifetime counter is withheld with the same reason when its whole read answers
zero words only (`read_withheld`), as before. A registry group answered with zero
words only stays withheld.

| Native item | Offset | Quantity | Routed unit | Zero word |
| --- | --- | --- | --- | --- |
| `battery_group_37000_30` | 37000 | 30 | not stated | not applicable |
| `battery_single_read_37000_1` | 37000 | 1 | not stated | not applicable |
| `battery_group_37066_4` | 37066 | 4 | not stated | not applicable |
| `battery_group_47000_1` | 47000 | 1 | not stated | not applicable |
| `site_battery_soc` | 30368 | 1 | 0 | value |
| `site_battery_power` | 30354 | 12 | 0 | not applicable |
| `plant_battery_power` | 37498 | 8 | 100 | withheld |
| `site_battery_energy_charged_total` | 30306 | 46 | 0 | read_withheld |
| `site_battery_energy_discharged_total` | 30306 | 46 | 0 | read_withheld |
| `site_battery_energy_charged_day` | 30306 | 46 | 0 | not applicable |
| `site_battery_energy_discharged_day` | 30306 | 46 | 0 | not applicable |
| `site_rated_storage_capacity` | 30322 | 2 | 0 | withheld |

## Withheld items

Each native item has exactly one disposition. An item listed here is withheld or
native-only; the mapped items are in the intake table below. The reason shown
for a battery group read is the one that applies when its group is served
natively; a supplied read selects the not-read, zero-word or exception reason.
Absent optional groups are unknown, never zero.

| Native item | Source | Grade | Reason | Loss kind |
| --- | --- | --- | --- | --- |
| `battery_group_37000_30` | registry_disposition | Unknown | `storage.reason.group_native_only_no_catalog_row` | capability |
| `battery_single_read_37000_1` | registry_disposition | Proven | `storage.reason.group_native_only_no_catalog_row` | capability |
| `battery_group_37066_4` | registry_disposition | Unknown | `storage.reason.group_native_only_no_catalog_row` | capability |
| `battery_group_47000_1` | registry_disposition | Unknown | `storage.reason.group_native_only_no_catalog_row` | capability |
| `site_battery_power` | monitoring_contract_planned | Hypothesis | `storage.reason.sign_not_graded` | provenance |
| `site_battery_energy_charged_day` | monitoring_contract_planned | Hypothesis | `storage.reason.period_counter_time_domain_not_represented` | time |
| `site_battery_energy_discharged_day` | monitoring_contract_planned | Hypothesis | `storage.reason.period_counter_time_domain_not_represented` | time |

The grade is that of the native item. The battery single read at the first
battery offset answered a zero word (Proven), and its meaning is not a value.
The mapped groups are site values of the EMMA and a plant value of the S-Dongle;
whether a gateway answers is Unknown, and a group that does not answer is
withheld as not read. The running-state word of the storage unit has no symbol
table, so the operating state is withheld with
`storage.reason.status_symbols_not_stated`.

The site power row states no sign for the battery power at offset 30360, so
`site_battery_power` is withheld with `storage.reason.sign_not_graded`; the
monitoring contract's plant group states the sign and is mapped. The two day
counters are published by the site energy group as U32 values of kilowatt-hours
and the pack's catalog elements are lifetime counters, so a day counter has no
element: it is served natively with its routed unit, offset and native form,
carries no fact, and the reason below records why.

Runtime reasons complete the set:
`storage.reason.zero_word_is_not_a_battery_value` (a zero battery power,
capacity or registry group word is never a value of zero, and
presence is never decided from it; a routed site state of charge word of zero is a value, and a lifetime counter is withheld only when its whole read is zero words),
`storage.reason.battery_group_exception_no_negation` (an exception negates
nothing else), `storage.reason.battery_group_not_read`,
`storage.reason.value_outside_pack_range` (a converted value outside the pack
bounds is withheld, never clamped) and
`storage.reason.no_admitted_battery_profile`, the reason of a mapped item while
the registry selection is unresolved.

## Prospective intake

A row in force publishes its fact; a row not in force records only where a fact
will land. Each target and unit is checked against the Storage pack's contract tables, and
each dimension against the element catalog. Each quantity row names the SI unit of its pack field under the
[unit rule](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/README.md#unit-rule) and the exact factor and offset of its
conversion, so that the SI value is the native value times the factor plus the
offset. Percent is published as a ratio from 0 to 1 by the exact factor 0.01,
kilowatt as watt by the factor 1000, and kilowatt-hour as joule by the exact
factor 3600000. Every conversion that is not the identity declares a reversible
`unit` loss, and the native value and unit stay in the evidence. Every factor
here terminates in decimal, so no conversion of this gate rounds. The operating
symbol has no unit and no conversion.

| Storage fact | Dimension | Native candidate | Native form | SI unit and conversion | Loss | Sign or counter rule | Grade | In force |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `storage.status.operating` | `storage.dimension.pack` | `battery_group_37000_30` | none decoded | none | none | empty table, observed states only | Unknown | no |
| `storage.state.soc` | `storage.dimension.pack` | `site_battery_soc` | U16, %, gain 100 | `unit.ratio`, factor 0.01, offset 0 | unit | none | Hypothesis | yes |
| `storage.pack.power` | `storage.dimension.pack` | `site_battery_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit+provenance | `unknown_no_value_without_a_graded_sign` | Hypothesis | no |
| `storage.pack.power` | `storage.dimension.pack` | `plant_battery_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit+provenance | `positive_while_charging_negative_while_discharging` | Hypothesis | yes |
| `storage.energy.charged` | `storage.dimension.pack` | `site_battery_energy_charged_total` | U64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | `reset_wrap_and_continuity_never_inferred` | Hypothesis | yes |
| `storage.energy.discharged` | `storage.dimension.pack` | `site_battery_energy_discharged_total` | U64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | `reset_wrap_and_continuity_never_inferred` | Hypothesis | yes |
| `storage.energy.nominal_capacity` | `storage.dimension.pack` | `site_rated_storage_capacity` | U32, kWh, gain 1000 | `unit.joule`, factor 3600000, offset 0 | unit | none | Hypothesis | yes |

Rules for the intake, each a Hypothesis of this gate unless stated. A group is
read at its documented offset and quantity, and a word is taken at its documented
position in that read.

1. **Operating symbol.** The observed states of a battery state word are the only
   symbols that may be mapped, and none was observed with a known meaning, so the
   symbol table is empty and every code is Unknown and withheld.
2. **Pack power and sign.** The Storage pack does not normalize the sign of pack
   power, so a mapping keeps the native sign reference as provenance (a reversible
   `provenance` loss) and converts kilowatt at gain 1000 to watt by the exact
   factor 1000 with a `unit` loss. The S-Dongle plant power states positive
   while charging; the monitoring contract grades that a Hypothesis. The EMMA
   site power states no sign (Unknown), so no value is published from it until a
   sign is graded. The two sign references are never mixed.
3. **Energy charged and discharged.** The total counters map to the pack's
   joule counters by the exact factor 3600000, with a `unit` loss and a `policy`
   loss: reset, rollover and continuity are Unknown and never inferred, and no
   energy is derived or summed.
4. **Aggregation.** The site and plant values are group values, while the pack's
   fact dimension is the pack. They are keyed to the pack as a whole with the
   aggregation basis, site group or plant group, kept as provenance. The pack
   never infers membership, a cell count or an arithmetic aggregate (Unknown).
5. **Decimal form.** Quantities use the kernel coefficient and exponent pair.
   There is no binary floating point, and the validator rejects a float literal.

## Capacity

The rated storage capacity is mapped in joules. The planned rated storage
capacity word at offset 30322 is a U32 in kilowatt-hours with gain 1000, and the
Storage pack carries `storage.energy.nominal_capacity` in joules on the pack, so
the word publishes by the exact factor 3600000 with a `unit` loss; it is a
Hypothesis and a site group value recorded as provenance. A reading that is not
supplied or is zero stays Unknown and is never a capacity of zero. The ampere-hour
counters (`storage.capacity.charge` and `storage.capacity.discharge`) are
cumulative throughput and not a rated capacity. A capacity is never converted
from ampere-hours or voltage, and never derived from state of charge or from an
energy counter; the vector `STG-NEG-009-ampere-hour-to-kilowatt-hour` pins that
the conversion is refused. The ampere-hour counters are not mapped either,
because no Huawei ampere-hour quantity is evidenced.

## Control-adjacent items

The charge and discharge power limits, the interlock, working modes, schedules,
remote dispatch and every write are withheld or unsupported. `storage.limit.charge_power`,
`storage.limit.discharge_power`, `storage.status.interlock`,
`storage.operation.set_charge_limit` and `storage.operation.set_discharge_limit`
are never requested by this gate. The write authority is declared from the
vendor's published interface, in the member `write_authority`; the battery charge
and discharge executor is owned by the executor row, not by this gate, and the
gate requests no operation. No acknowledgement, readback, retry or route is
declared here, and a request for one is rejected with `unsupported_or_withheld`.

## Raw group values

A battery group that answered with data and has no catalog row is native-only. Its
raw words are served unchanged in the modbusreg namespace, in the output member
`native`, with the group identity, offset and quantity, because no catalog element
yet gives them a fact. They are never published as a fact, and a group that was
not read, raised an exception or answered only zero words carries no entry.

The two site day counters are served the same way when their group answered: each
entry carries the routed unit, the offset and quantity of its own words and its
native form (U32, kilowatt-hours, gain 100). A day counter is never labeled a
lifetime counter and is never published as a fact.

## Daily counter native form

| Native item | Native form |
| --- | --- |
| `site_battery_energy_charged_day` | U32, kWh, gain 100 |
| `site_battery_energy_discharged_day` | U32, kWh, gain 100 |

## Lifecycle

A case carries the observation identifier and revision, the receipt on a wall and
a monotonic axis, the clock epoch, the source epoch, the driver generation, the
connection generation, the registry outcome and any battery reads. The first
error of this order is reported: native evidence invalid, identity missing or invalid, lifecycle missing, mapping unqualified,
unsupported or withheld. A battery value keeps its original age
when a read fails. Withdrawal is explicit and
generation-fenced.

## Acceptance vectors

| Vector | What it proves |
| --- | --- |
| `STG-POS-001-no-battery-read` | A selected profile with no battery offset in the read plan withholds every item as not read, and nothing is zero. |
| `STG-POS-002-site-state-of-charge` | A documented site state of charge of 75.50 percent publishes 0.755 on the pack, with the site group as provenance. |
| `STG-POS-003-plant-power-charging` | Plant battery power of 2.5 kW, positive while charging, publishes 2500 W. |
| `STG-POS-004-plant-power-discharging` | Plant battery power of -1.5 kW, negative while discharging, publishes -1500 W. |
| `STG-POS-005-site-energy-totals` | The site energy group publishes the lifetime charged and discharged counters in joules, and the day counters are served natively and never published as lifetime facts. |
| `STG-POS-006-site-power-sign-not-graded` | The site power group states no sign, so the battery power it carries is withheld. |
| `STG-POS-007-zero-words-without-battery` | Zero words at the registry battery offsets publish no state of charge, power or energy. |
| `STG-POS-008-battery-group-answers-with-data` | A registry group that answers with data is served natively and publishes no storage fact. |
| `STG-POS-009-battery-read-exception` | An exception or a failed read withholds the battery item and negates nothing else. |
| `STG-POS-010-selection-unresolved` | An unresolved registry selection withholds every mapped item whatever is read. |
| `STG-POS-011-state-of-charge-outside-range` | A state of charge above 100 percent is withheld and never clamped. |
| `STG-POS-012-rated-storage-capacity` | A documented rated storage capacity of 10 kWh publishes 36000000 J as the nominal capacity of the pack, with the site group as provenance. |
| `STG-POS-013-aggregate-reading-with-several-packs` | A site state of charge of a battery with two packs belongs to no one pack and is withheld, whichever pack is listed first. |
| `STG-POS-014-site-state-of-charge-zero` | A state of charge word of zero on the configured battery with one pack publishes 0 as a ratio, distinct from an unknown value. |
| `STG-POS-015-plant-read-with-zero-battery-power` | A plant read with nonzero PV, consumption and grid words and zero battery power words publishes no battery power. |
| `STG-POS-016-site-day-counters-served-natively` | The site day counters answered with data are served in the modbusreg namespace with their routed unit and native form and publish no fact. |
| `STG-NEG-001-selection-outside-the-registry-set` | A selection outside the registry set fails closed. |
| `STG-NEG-002-read-outside-the-plan` | A group that is in no read plan cannot be supplied as evidence. |
| `STG-NEG-003-word-count-mismatch` | A read must carry exactly its quantity of words. |
| `STG-NEG-004-asset-missing` | A blank asset identifier fails closed. |
| `STG-NEG-005-pack-identity-missing` | A pack resource at its native position is required to publish. |
| `STG-NEG-006-lifecycle-missing` | The source epoch is required. |
| `STG-NEG-007-mapping-unqualified` | A mapping that is not declared qualified is rejected. |
| `STG-NEG-008-charge-limit-operation` | The gate has no limit, interlock, route or authority. |
| `STG-NEG-009-ampere-hour-to-kilowatt-hour` | Capacity is never converted to energy units. |
| `STG-NEG-010-limit-item-requested` | A control-adjacent item is not requestable. |
| `STG-NEG-011-error-precedence` | The first error of the precedence list is reported. |
| `STG-NEG-012-site-state-of-charge-at-the-plant-route` | A state of charge read tagged with the plant unit is not site evidence. |
| `STG-NEG-013-plant-power-at-the-site-route` | A plant power read tagged with the site unit is not plant evidence. |
| `STG-NEG-014-routed-read-without-its-unit` | A routed group read that names no unit is not evidence. |

## Unresolved and falsifiers

| Question | Falsifier | Grade |
| --- | --- | --- |
| What do the words of the registry battery groups mean? | A catalog row that gives them an element. | Unknown |
| Which word, if any, states battery presence? | The candidate presence word checked against a site with and without a battery. | Unknown |
| Do the site and plant groups answer? | An operator-approved observation. | Unknown |
| What is the sign of the EMMA site battery power? | A reading at charge and at discharge with an independent check. | Unknown |
| Is the battery's attachment to an inverter observable natively? | The native owner documents it as observable for a stated scope. | Unknown |

## Later consumer cutover

Consumers read these facts only through the shared semantic snapshot. This gate
publishes no consumer binding. Any implementation must bind consumers in one
atomic cutover, must not reuse the single-asset Growatt projection, and must keep
`outbound_allowed=false`.

## Validation

Run `python3 scripts/mapping/validate_huawei_luna2000_storage_mapping.py`,
`python3 tests/test_mapping_validate_huawei_luna2000_storage.py` (also collected
by `python3 -m pytest`), and `./scripts/check_docs.sh`, which runs both. The validator executes every vector, checks each
prospective target against the Storage pack tables, checks that every native item
has exactly one disposition, and compares the tables of this page with the
contract. It runs the shared graph rules of `scripts/mapping_gate_graph_rules.py`
first and then its own resource, relation, relay, join, selector and subject
rules, looking up kinds, dimensions, subjects, relation definitions, directions
and aggregations in the element catalog.
