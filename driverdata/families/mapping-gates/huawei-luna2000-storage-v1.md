# Huawei LUNA2000 storage mapping gate v1

This gate states what `helianthus.pack.storage@1.1.0`, the generic Storage
family that the Growatt storage mapping also uses, will take from Huawei
LUNA2000 observations once a battery profile is admitted, and it publishes
nothing before then. It adds no pack and no vendor-specific semantic family, and
it makes no stability promise before Helianthus 1.0. It is neither a device
support claim nor a live qualification. Its machine-readable companion is
[the mapping vectors](huawei-luna2000-storage-v1.json); the vectors are
synthetic and are not observations from hardware.

No battery fact is admitted today, so every item below is withheld with an
explicit reason and loss. The sibling gates are
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

The registry's LUNA2000 disposition is `NO_ADMISSIBLE_PROFILE`; whether or not
the parent inverter is identified, no battery profile is selected, and the
decoder key count is zero (Proven as a statement of the registry). The battery
groups answered with data on the SUN2000 MAP0 series only (Proven), and their
meanings, gains and signs are Unknown. On the M3 and M1 series the single reads
at the battery offsets answered one zero word (Proven), and the rule that a
battery offset answers zero at an inverter without a battery is a Hypothesis.
No battery offset is in any read plan.

The gate accepts the registry outcomes `INSUFFICIENT_EVIDENCE`,
`NO_ADMISSIBLE_PROFILE` and `AMBIGUOUS`. A selected battery profile fails closed
with `gate_revision_required`: the registry cannot admit a profile before this
gate is revised. All five registry flags (`catalog_registered`,
`automatic_runtime_admission`, `live_qualified`, `support_claim`,
`write_authority`) stay false, and `outbound_allowed=false` grants no operation.

Before any row below may be mapped, a revision of this gate must show that:

1. a battery profile for the exact family is admitted with a consulted edition;
2. type, word order, gain, unit and sign of each consumed word are stated;
3. a sign is graded before any power is published;
4. the aggregation basis is recorded as provenance;
5. the storage asset granularity is settled;
6. executable vectors for that profile are added.

The resource kinds and anchors of item 5 are fixed below. Item 5 stays open
because no read path states how many packs a battery holds.

## Identity and keying

Identity follows sections 6.3 and 15 of the [graph contract](../graph.md), the
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
  gateway-allocated resource identifier (RES-5, REL-8): one for the inverter
  and one for the battery. No pack identifier is allocated, because no profile
  states the packs of a battery.

The class token of the Storage pack and the registry profile identifier are
native interpretation selectors, and the gate classifies both as provenance
(section 6.3). A hybrid inverter whose PV and storage facts both publish
therefore stays one inverter and one home asset. Neither the asset, the source
nor a resource identifier derives from a serial number, an ESN, the vendor, the
model, firmware or any version text, a routed unit alone, an endpoint address, a
class token or a profile identifier. Missing, invalid, reused or ambiguous
identity fails closed. Nothing is keyed today because nothing is published.

## Resources

This is section 15 item 1. The element catalog lists no identification scheme
for this family, so every resource below is anchored by an operator label. An
address, a routed unit, a credential, a class token or a profile identifier is
never an anchor source.

| Kind | Anchor class | Basis | Anchor source | Parent kind |
| --- | --- | --- | --- | --- |
| `resource.inverter` | equipment | `operator_slot` | `operator_label` | none |
| `resource.battery` | equipment | `operator_slot` | `operator_label` | none |
| `resource.battery_pack` | equipment | `operator_slot` | `operator_label` | none |

- The inverter is the same resource as in the PV gate, with the same anchor.
  The child key of the native monitoring contract is its future native token,
  and its stability is Unknown until the native owner states it.
- The battery is equipment of its own, relayed by the inverter. The batteries
  are registers of the inverter units (Proven), and the relaying path exposes
  no anchor key source for the battery: no battery token is on it.
- The native evidence separates the battery from its packs only in kind: the
  planned site and plant values are stated over packs whose number is Unknown.
  No pack count, position or token is on the relaying path. The pack kind is
  stated so that its anchor is fixed, but no pack is allocated or published
  until a battery profile states the packs.

## Relations

This is section 15 item 2. The validity scope of every row is the registry
profile `huawei.luna2000` with no admitted version, so no record is emitted
today.

| Definition | Basis class | From kind | To kind | Scope of the rule |
| --- | --- | --- | --- | --- |
| `relation.relays` | `configuration` | `resource.inverter` | `resource.battery` | `each_battery_the_operator_attaches_to_an_inverter_child` |
| `storage.relation.battery_pack` | `gate_rule` | `resource.battery` | `resource.battery_pack` | `one_record_per_pack_an_admitted_battery_profile_reads_none_before_admission` |

- The battery's attachment to an inverter is configuration. No native owner
  documents it as observable, and a zero word at a battery offset never decides
  presence. It becomes a native observation only when the native owner
  documents it as observable for a stated scope.
- The battery-to-pack relation is a gate rule. The catalog admits no other
  basis, and its records wait for a profile that reads the packs.
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
| `storage.relation.module_cell` | `resource.battery_module` | `resource.battery_cell` | `unsupported:cell_and_module_values` |
| `storage.relation.pack_interface` | `resource.battery_pack` | `resource.storage_interface` | `unsupported:charge_and_discharge_power_limits` |
| `storage.relation.pack_module` | `resource.battery_pack` | `resource.battery_module` | `unsupported:cell_and_module_values` |
| `storage.relation.pack_string` | `resource.battery_pack` | `resource.battery_string` | `registry_disposition:no_string_structure_admitted` |
| `storage.relation.system_battery` | `resource.storage_system` | `resource.battery` | `configuration:no_storage_system_declared_by_this_gate` |

## Subjects and measurement points

Under EN-1 each prospective row names its subject kind and its measurement
point. The subject kind and the dimension are looked up in the
[element catalog](../element-catalog-v1.json), not repeated by the validator,
and the measurement points are a Hypothesis of this gate.

| Native item | Storage fact | Subject kind | Measurement point |
| --- | --- | --- | --- |
| `battery_group_37000_30` | `storage.status.operating` | `resource.battery_pack` | `pack_as_a_whole` |
| `site_battery_soc` | `storage.state.soc` | `resource.battery_pack` | `pack_as_a_whole` |
| `site_battery_power` | `storage.pack.power` | `resource.battery_pack` | `pack_terminals` |
| `plant_battery_power` | `storage.pack.power` | `resource.battery_pack` | `pack_terminals` |
| `site_battery_energy_charged_total` | `storage.energy.charged` | `resource.battery_pack` | `pack_terminals_lifetime_counter` |
| `site_battery_energy_discharged_total` | `storage.energy.discharged` | `resource.battery_pack` | `pack_terminals_lifetime_counter` |

## Withheld items

Each native item has exactly one disposition, `withheld`. The reason shown for a
battery group read is the one that applies when no read is supplied; a supplied
read selects the zero-word, exception or no-profile reason, and a planned group
cannot be supplied at all because it was never read. Absent optional groups are
unknown, never zero.

| Native item | Source | Grade | Reason | Loss kind |
| --- | --- | --- | --- | --- |
| `battery_group_37000_30` | registry_disposition | Unknown | `storage.reason.battery_group_not_read` | provenance |
| `battery_single_read_37000_1` | registry_disposition | Proven | `storage.reason.battery_group_not_read` | provenance |
| `battery_group_37066_4` | registry_disposition | Unknown | `storage.reason.battery_group_not_read` | provenance |
| `battery_group_47000_1` | registry_disposition | Unknown | `storage.reason.battery_group_not_read` | provenance |
| `site_battery_soc` | monitoring_contract_planned | Hypothesis | `storage.reason.planned_group_not_observed` | provenance |
| `site_battery_power` | monitoring_contract_planned | Hypothesis | `storage.reason.planned_group_not_observed` | provenance |
| `plant_battery_power` | monitoring_contract_planned | Hypothesis | `storage.reason.planned_group_not_observed` | provenance |
| `site_battery_energy_charged_total` | monitoring_contract_planned | Hypothesis | `storage.reason.planned_group_not_observed` | provenance |
| `site_battery_energy_discharged_total` | monitoring_contract_planned | Hypothesis | `storage.reason.planned_group_not_observed` | provenance |
| `site_battery_energy_charged_day` | monitoring_contract_planned | Hypothesis | `storage.reason.period_counter_time_domain_not_represented` | time |
| `site_battery_energy_discharged_day` | monitoring_contract_planned | Hypothesis | `storage.reason.period_counter_time_domain_not_represented` | time |
| `site_rated_storage_capacity` | monitoring_contract_planned | Hypothesis | `storage.reason.no_energy_capacity_fact_in_pack` | capability |

The grade is that of the native item. The battery single read at the first
battery offset answered a zero word (Proven), and its meaning is not a value.
The planned groups are site values of the EMMA, a plant value of the S-Dongle
and a candidate presence word; the monitoring contract names them from consulted
editions and does not send them until an operator-approved observation records
their answer and the contract is revised. Whether the gateways answer is
Unknown.

Three runtime reasons and the primary reason complete the set:
`storage.reason.zero_word_is_not_a_battery_value` (a zero word is never a state
of charge, power or energy of zero, and presence is never decided from it),
`storage.reason.battery_group_exception_no_negation` (an exception negates
nothing else), `storage.reason.battery_group_not_read`, and
`storage.reason.no_admitted_battery_profile`, which is the reason of each of the
five requested items.

## Prospective intake

These rows are not in force. They record only where a fact will land if the
preconditions above are met, so that a later profile does not need a new family.
Each target and unit is checked against the Storage pack's contract tables, and
each dimension against the element catalog. Each quantity row names the SI unit of its pack field under the
[unit rule](../../README.md#unit-rule) and the exact factor and offset of its
conversion, so that the SI value is the native value times the factor plus the
offset. Percent is published as a ratio from 0 to 1 by the exact factor 0.01,
kilowatt as watt by the factor 1000, and kilowatt-hour as joule by the exact
factor 3600000. Every conversion that is not the identity declares a reversible
`unit` loss, and the native value and unit stay in the evidence. Every factor
here terminates in decimal, so no conversion of this gate rounds. The operating
symbol has no unit and no conversion.

| Storage fact | Dimension | Native candidate | Native form | SI unit and conversion | Loss | Sign or counter rule | Grade |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `storage.status.operating` | `storage.dimension.pack` | `battery_group_37000_30` | none decoded | none | none | empty table, observed states only | Unknown |
| `storage.state.soc` | `storage.dimension.pack` | `site_battery_soc` | U16, %, gain 100 | `unit.ratio`, factor 0.01, offset 0 | unit | none | Hypothesis |
| `storage.pack.power` | `storage.dimension.pack` | `site_battery_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit+provenance | `unknown_no_value_without_a_graded_sign` | Hypothesis |
| `storage.pack.power` | `storage.dimension.pack` | `plant_battery_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit+provenance | `positive_while_charging_negative_while_discharging` | Hypothesis |
| `storage.energy.charged` | `storage.dimension.pack` | `site_battery_energy_charged_total` | U64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | `reset_wrap_and_continuity_never_inferred` | Hypothesis |
| `storage.energy.discharged` | `storage.dimension.pack` | `site_battery_energy_discharged_total` | U64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | `reset_wrap_and_continuity_never_inferred` | Hypothesis |

Rules for the intake, each a Hypothesis of this gate unless stated:

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
4. **Aggregation.** The site and plant values are aggregates over an undetermined
   number of packs, while the pack's fact dimension is the pack. A revision must
   either key them to one battery asset with the aggregation basis kept as
   provenance, or keep them withheld. The pack never infers membership, a cell
   count or an arithmetic aggregate (Unknown).
5. **Decimal form.** Quantities use the kernel coefficient and exponent pair.
   There is no binary floating point, and the validator rejects a float literal.

## Capacity

Capacity in energy units is withheld. The Storage pack carries ampere-hour
counters only (`storage.capacity.charge` and `storage.capacity.discharge`, which
are cumulative throughput and not a rated capacity), and the planned rated
storage capacity is in kilowatt-hours. A rating in energy units needs a separate
SemReg and pack revision. It is never converted from ampere-hours or voltage, and
never derived from state of charge or from an energy counter; the vector
`STG-NEG-008-ampere-hour-to-kilowatt-hour` pins that the conversion is refused.
The ampere-hour counters are not mapped either, because no Huawei ampere-hour
quantity is evidenced.

## Control-adjacent items

The charge and discharge power limits, the interlock, working modes, schedules,
remote dispatch and every write are withheld or unsupported. `storage.limit.charge_power`,
`storage.limit.discharge_power`, `storage.status.interlock`,
`storage.operation.set_charge_limit` and `storage.operation.set_discharge_limit`
are never requested by this gate. No authority, acknowledgement, readback, retry
or route exists, and a request for one is rejected with `unsupported_or_withheld`.

## Lifecycle

A case carries the observation identifier and revision, the receipt on a wall and
a monotonic axis, the clock epoch, the source epoch, the driver generation, the
connection generation, the registry outcome and any battery reads. The first
error of this order is reported: gate revision required, native evidence
invalid, identity missing or invalid, lifecycle missing, mapping unqualified,
unsupported or withheld. A battery value that existed would keep its original age
when a read failed; no such value exists today. Withdrawal is explicit and
generation-fenced.

## Acceptance vectors

| Vector | What it proves |
| --- | --- |
| `STG-POS-001-no-battery-read` | No battery offset is in any read plan, so every item is withheld and nothing is zero. |
| `STG-POS-002-zero-words-without-battery` | Zero words at the battery offsets of an inverter without a battery publish no state of charge, power or energy. |
| `STG-POS-003-battery-group-answers-with-data` | A group that answers with data still publishes nothing, because no profile gives its words a meaning. |
| `STG-POS-004-battery-read-exception` | An exception or a failed read withholds the battery item and negates nothing else. |
| `STG-POS-005-selection-insufficient-evidence` | Every allowed registry outcome withholds the same items. |
| `STG-NEG-001-battery-profile-selected` | A battery profile selected before this gate is revised fails closed. |
| `STG-NEG-002-planned-group-fed-as-evidence` | A planned group that was never read cannot be supplied as evidence. |
| `STG-NEG-003-word-count-mismatch` | A read must carry exactly its quantity of words. |
| `STG-NEG-004-asset-missing` | A blank asset identifier fails closed. |
| `STG-NEG-005-lifecycle-missing` | The source epoch is required. |
| `STG-NEG-006-live-qualified` | A live qualification requires a revision of this gate. |
| `STG-NEG-007-charge-limit-operation` | The gate has no limit, interlock, route or authority. |
| `STG-NEG-008-ampere-hour-to-kilowatt-hour` | Capacity is never converted to energy units. |
| `STG-NEG-009-limit-item-requested` | A control-adjacent item is not requestable. |
| `STG-NEG-010-error-precedence` | The first error of the precedence list is reported. |

## Unresolved and falsifiers

| Question | Falsifier | Grade |
| --- | --- | --- |
| What do the battery words mean, and what are their gains and signs? | A consulted edition that covers the family, or an independent check of the layout. | Unknown |
| Which word, if any, states battery presence? | The candidate presence word checked against a site with and without a battery. | Unknown |
| Do the planned site and plant groups answer? | An operator-approved observation. | Unknown |
| What is the sign of the EMMA site battery power? | A reading at charge and at discharge with an independent check. | Unknown |
| How many packs does a battery hold, and at which positions? | A battery profile that reads the packs. | Unknown |
| Is the battery's attachment to an inverter observable natively? | The native owner documents it as observable for a stated scope. | Unknown |

## Later consumer cutover

Consumers read these facts only through the shared semantic snapshot. This gate
publishes no consumer binding. Any implementation must bind consumers in one
atomic cutover, must not reuse the single-asset Growatt projection, and must keep
`outbound_allowed=false`.

## Validation

Run `python3 scripts/validate_huawei_luna2000_storage_mapping.py`,
`python3 scripts/test_validate_huawei_luna2000_storage_mapping.py` (also collected
by `python3 -m pytest`), and `./scripts/check_docs.sh`, which runs both. The validator executes every vector, checks each
prospective target against the Storage pack tables, checks that every native item
has exactly one disposition, and compares the tables of this page with the
contract. It runs the shared graph rules of `scripts/mapping_gate_graph_rules.py`
first and then its own resource, relation, relay, join, selector and subject
rules, looking up kinds, dimensions, subjects, relation definitions, directions
and aggregations in the element catalog.
