# Huawei gateway-meter Infrastructure mapping gate v1

This gate states what `helianthus.pack.infrastructure@1.0.0` will take from a
power meter behind a Huawei gateway once a meter is decoded, and it publishes
nothing before then. It adds no pack and no vendor-specific semantic family, and
it makes no stability promise before Helianthus 1.0. It is neither a device
support claim nor a live qualification. Its machine-readable companion is
[the mapping vectors](huawei-gateway-meter-infrastructure-v1.json); the vectors
are synthetic and are not observations from hardware.

Every meter value is Unknown today. The sibling gates are
[the SUN2000 PV gate](huawei-sun2000-pv-v1.md) and
[the LUNA2000 storage gate](huawei-luna2000-storage-v1.md). The existing
[gateway-local runtime assessment](../../../mappings/infrastructure/gateway-runtime.md)
still applies: a driver connection, runtime readiness or acquisition failure is
not an electrical connection, readiness or fault, and this gate maps none of them.

## Evidence basis

| Source | Reference | Role |
| --- | --- | --- |
| Native monitoring contract | `helianthus-docs-modbus` issue 242, pull request 253: `protocols/huawei/monitoring-readonly-v1.md` and `protocols/huawei/read-pacing-v1.md` | Owns the meter paths, the planned reads, the sign sources and every grade. |
| Native meter contract | `protocols/huawei/gateway-power-meter-v1.md` in the same repository | Owns meter identity, exposure and the rule that the role is configuration. |
| Meter observation amendment | `helianthus-docs-modbus` issue 254 | Open. It records the observed meter paths. This gate depends on none of its content, and the gate reads as Unknown whatever it records, because nothing is decoded. |
| Registry profiles | `helianthus-modbusreg` issue 348, pull request 357 | The SUN2000 profiles decode no meter word and withhold every meter. |
| Program | `helianthus-modbusreg` issues 199 and 201 | The Huawei read-only program this gate belongs to. |
| Multi-endpoint contract | `helianthus-docs-ebus` issue 746, pull request 748 | Keying per endpoint instance. Its text was not final when this gate was written. |

No revision, digest or pinned link is cited. Grades are those of the monitoring
contract, kept separate from admission, qualification and support: Proven needs a
live reading checked independently, Hypothesis is a document statement or a
plausible unchecked reading, and Unknown is unsettled. A rule of this gate that
goes beyond the contract is a Hypothesis.

## Admission state

As of the monitoring contract on its main branch, no meter register is decoded on
any path, no meter class contract exists, and every meter stays
`INSUFFICIENT_EVIDENCE` (Proven as a statement of the contract). A group that
answers is neither a value nor an identity, so the answer of a meter group
changes nothing here: vector `MTR-POS-004-groups-answer-nothing-decoded` pins
that. The inverter-exposed meter group was refused with exception 3 on the M3
series (Proven, cause a Hypothesis), and its behavior at the other children is
Unknown.

The gate accepts the registry outcomes `INSUFFICIENT_EVIDENCE`,
`NO_ADMISSIBLE_PROFILE` and `AMBIGUOUS`. A selected meter, or a meter asset
identifier, fails closed with `gate_revision_required`. All five registry flags
stay false, and `outbound_allowed=false` grants no operation.

Before any row below may be mapped, a revision of this gate must show that:

1. a meter class contract exists with an identity read;
2. the exposure path is observed;
3. type, word order, gain, unit and sign of each consumed word are stated;
4. a sign reference is graded per source and never mixed;
5. the role is declared in configuration;
6. executable vectors for that meter are added.

## Role and dimension

The role is configuration. The operator declares it, keyed to the endpoint and the
routed unit, and it is never derived from a register, from the sign or magnitude
of a power value, from the label, the unit number, the listing order, the model
or the presence of another meter. A declared role is configuration and not
evidence: it identifies no meter and changes no identity outcome. A meter without
a declared role has role Unknown, and a fact that depends on the role is not
published without a declaration. The vectors `MTR-NEG-007-role-derived-from-sign`
and `MTR-NEG-008-role-outside-set` pin these rules.

Under EN-8 of the [graph contract](../graph.md), a meter's role is an instance
of a `measures` relation definition between the meter and the resource it
measures. The grid connection's home asset holds it, and its basis class is
`configuration` because the native meter contract states that the role is
configuration. The role never changes a fact definition. The validator takes
each role's relation from the [element catalog](../element-catalog-v1.json): it
is the catalog's `measures` definition between the measured resource and the
meter, and it is unknown when the catalog has none. A stated role relation
needs its instance in the gate's graph. A role relation with a
`native_observation` basis is rejected, even if the catalog admitted one.

Where each quantity lands follows EN-1, not the role. The catalog places the
direction-separated power and the power factor on the grid connection and the
energy counters on the meter. A role whose relation is unknown therefore takes
no power (Hypothesis, because no meter is decoded):

| Role | Power | Energy counters | Relation |
| --- | --- | --- | --- |
| `grid_connection` | `infrastructure.dimension.grid_connection` | `infrastructure.dimension.meter` | `infrastructure.relation.grid_connection_meter` |
| `feeder` | withheld | `infrastructure.dimension.meter` | unknown |
| `circuit` | withheld | `infrastructure.dimension.meter` | unknown |
| undeclared | withheld | withheld | none |

The catalog has no `measures` definition between a feeder or a circuit and a
meter. For those roles the gate states neither an instance nor an absence
statement, so the relation is unknown. It is not absent. The gate never invents
a feeder, circuit, phase or meter member.

## Identity and keying

The source identifier is gateway-configured, non-secret and opaque to this gate,
and it must not be derived from a serial number, an ESN, the vendor, the model,
firmware or any version text, a routed unit alone, or an endpoint address; a
missing or invalid one fails closed. No meter asset identifier exists, because no
make, model or firmware of any meter is established. The routed unit and the
source key the role. The keying follows the multi-endpoint contract of
`helianthus-docs-ebus` issue 746 (pull request 748, open): one source per
endpoint instance. Nothing is published today.

## Resources

This is section 15 item 1 of the [graph contract](../graph.md). The element
catalog lists no identification scheme for this family, so every resource below
is anchored by an operator label. An address, a routed unit, a credential, a
gateway path or a profile identifier is never an anchor source. The routed unit
keys a read and never an anchor.

| Kind | Anchor class | Basis | Anchor source | Parent kind |
| --- | --- | --- | --- | --- |
| `resource.site` | function | `operator_slot` | `operator_label` | none |
| `resource.grid_connection` | function | `operator_slot` | `operator_label` | none |
| `resource.meter` | equipment | `operator_slot` | `operator_label` | none |

- The site and the grid connection are functions that the operator declares.
  No native token exists for them.
- The meter is equipment. The native meter contract gives it no identity read
  until a meter class contract exists, so the stability of any meter token is
  Unknown. The meter's operator label is not a meter asset identifier, and the
  identity outcome stays as above.

## Relations

This is section 15 item 2. The validity scope of every row is "no meter class
contract" with no admitted profile, so no record is emitted today.

| Definition | Basis class | From kind | To kind | Scope of the rule |
| --- | --- | --- | --- | --- |
| `infrastructure.relation.site_grid_connection` | `configuration` | `resource.site` | `resource.grid_connection` | `each_grid_connection_the_operator_declares_in_a_site` |
| `infrastructure.relation.grid_connection_meter` | `configuration` | `resource.grid_connection` | `resource.meter` | `each_meter_the_operator_declares_with_role_grid_connection` |

- The grid-connection-to-meter row is the meter role of EN-8. A declared
  `grid_connection` role without this instance is rejected.
- The gate makes no absence statement. A withheld meter group is not an absent
  meter.

Relayed values follow section 15 item 3. Whether a meter's values sit at its
own unit or at the gateway unit is Unknown, so the gate names no relaying
resource and states no relay until the exposure path is observed. One meter may
be seen at a gateway meter unit and through an inverter child. Without a meter
identity read those two views are one resource only by an operator declaration
that gives both one slot (section 6.4). A join by a name, a routed unit or a
position is rejected, and so is a join of views whose native tokens differ (a
false join). A feeder or circuit relation is rejected as a dangling endpoint,
because the gate publishes neither.

Aggregates follow section 15 item 4 and EN-5. Every prospective target is a
member fact in the element catalog, so the gate states no aggregate. Selectors
follow section 15 item 5: the gateway path and the registry profile identifier
are the only interpretation selectors, and both are provenance. The gate states
no plane membership (section 15 item 6, PLN-1).

## Unknown, not absent

Section 7.2: when neither an instance nor an absence statement exists, a
relation is unknown, not absent. These Infrastructure relations stay unknown
here.

| Definition | From kind | To kind | Withheld by |
| --- | --- | --- | --- |
| `infrastructure.relation.circuit_phase` | `resource.electrical_circuit` | `resource.phase` | `native_contract:phase_assignment_not_stated` |
| `infrastructure.relation.feeder_circuit` | `resource.feeder` | `resource.electrical_circuit` | `configuration:no_feeder_or_circuit_declared_by_this_gate` |
| `infrastructure.relation.grid_connection_feeder` | `resource.grid_connection` | `resource.feeder` | `configuration:no_feeder_or_circuit_declared_by_this_gate` |

## Subjects and measurement points

Under EN-1 each prospective row names its subject kind and its measurement
point. The subject is the resource the quantity is measured at, never the
device that reports it: the plant grid power of a dongle and the power of an
inverter-exposed group belong to the grid connection, not to the dongle, the
inverter or the meter. The subject kind and the dimension are looked up in the
element catalog, not repeated by the validator. The measurement points are a
Hypothesis of this gate. A withheld item without a prospective row names no
target, so it has no subject.

| Native item | Infrastructure fact | Subject kind | Measurement point |
| --- | --- | --- | --- |
| `smartlogger_meter_electrical_group` | `infrastructure.power.import_active` | `resource.grid_connection` | `grid_connection_point` |
| `smartlogger_meter_electrical_group` | `infrastructure.power.export_active` | `resource.grid_connection` | `grid_connection_point` |
| `smartlogger_meter_electrical_group` | `infrastructure.ac.power_factor` | `resource.grid_connection` | `grid_connection_point` |
| `smartlogger_meter_energy_totals` | `infrastructure.energy.import` | `resource.meter` | `meter_register` |
| `smartlogger_meter_energy_totals` | `infrastructure.energy.export` | `resource.meter` | `meter_register` |
| `sdongle_inverter_exposed_meter_group` | `infrastructure.power.import_active` | `resource.grid_connection` | `grid_connection_point` |
| `sdongle_inverter_exposed_meter_group` | `infrastructure.power.export_active` | `resource.grid_connection` | `grid_connection_point` |
| `sdongle_inverter_exposed_meter_group` | `infrastructure.energy.import` | `resource.meter` | `meter_register` |
| `sdongle_inverter_exposed_meter_group` | `infrastructure.energy.export` | `resource.meter` | `meter_register` |
| `sdongle_plant_grid_power` | `infrastructure.power.import_active` | `resource.grid_connection` | `grid_connection_point` |
| `sdongle_plant_grid_power` | `infrastructure.power.export_active` | `resource.grid_connection` | `grid_connection_point` |

## Withheld items

Each native item has exactly one disposition, `withheld`. The reason shown is the
one that applies when no read is supplied; a supplied read selects the
answered-not-decoded or exception reason. Absent optional groups are unknown,
never zero.

| Native item | Gateway path | Grade | Reason | Loss kind |
| --- | --- | --- | --- | --- |
| `smartlogger_meter_electrical_group` | smartlogger | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `smartlogger_meter_energy_totals` | smartlogger | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `emma_built_in_sensor_group` | emma | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `emma_external_sensor_group` | emma | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `sdongle_inverter_exposed_meter_group` | sdongle | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `sdongle_plant_grid_power` | sdongle | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `sun2000_inverter_exposed_meter_group_refused` | sun2000_m3 | Proven | `infrastructure.reason.refused_on_family` | provenance |
| `profile_meter_groups` | sun2000_profiles | Unknown | `infrastructure.reason.profile_withholds_meter` | policy |

The SmartLogger rows are planned reads at the listed meter units, named from an
older logger document. The EMMA rows are two sensor groups at the EMMA's own
unit, and which of them carries the backup box's meter is Unknown. The S-Dongle
rows are an inverter-exposed group at the child and the dongle's plant grid
power. The monitoring contract does not send any of them until a meter class
contract exists, the operator approves the observation and the contract is
revised. The two remaining reasons,
`infrastructure.reason.answered_not_decoded` and
`infrastructure.reason.read_exception_no_negation`, apply to a supplied read, and
an exception negates neither the gateway nor an identified inverter.

## Prospective intake

These rows are not in force. They record only where a fact will land if the
preconditions above are met and the role is `grid_connection` where the pack puts
the fact on the grid connection. Each target, dimension and unit is checked
against the Infrastructure pack catalog. Each row names the SI unit of its pack
field under the [unit rule](../../README.md#unit-rule) and the exact factor and
offset of its conversion, so that the SI value is the native value times the
factor plus the offset. Kilowatt at gain 1000 is published as watt by the factor
1000 and kilowatt-hour as joule by the exact factor 3600000. Every conversion
that is not the identity declares a reversible `unit` loss, the native value and
unit stay in the evidence, and every counter keeps a `policy` loss: reset,
rollover and continuity are Unknown and never inferred. Every factor here
terminates in decimal, so no conversion of this gate rounds. Quantities use the
kernel coefficient and exponent pair. There is no binary floating point, and
the validator rejects a float literal.

| Infrastructure fact | Dimension | Native candidate | Native form | SI unit and conversion | Loss | Sign, direction or counter rule | Role |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `infrastructure.power.import_active` | `infrastructure.dimension.grid_connection` | `smartlogger_meter_electrical_group` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `not_stated` | `grid_connection` |
| `infrastructure.power.export_active` | `infrastructure.dimension.grid_connection` | `smartlogger_meter_electrical_group` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `not_stated` | `grid_connection` |
| `infrastructure.ac.power_factor` | `infrastructure.dimension.grid_connection` | `smartlogger_meter_electrical_group` | I16, ratio, gain 1000 | `unit.ratio`, factor 1, offset 0 | none | none | `grid_connection` |
| `infrastructure.energy.import` | `infrastructure.dimension.meter` | `smartlogger_meter_energy_totals` | I64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | direction not_stated; `reset_wrap_and_continuity_never_inferred` | any role |
| `infrastructure.energy.export` | `infrastructure.dimension.meter` | `smartlogger_meter_energy_totals` | I64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | direction not_stated; `reset_wrap_and_continuity_never_inferred` | any role |
| `infrastructure.power.import_active` | `infrastructure.dimension.grid_connection` | `sdongle_inverter_exposed_meter_group` | I32, W, gain 1 | `unit.watt`, factor 1, offset 0 | none | `positive_export` | `grid_connection` |
| `infrastructure.power.export_active` | `infrastructure.dimension.grid_connection` | `sdongle_inverter_exposed_meter_group` | I32, W, gain 1 | `unit.watt`, factor 1, offset 0 | none | `positive_export` | `grid_connection` |
| `infrastructure.energy.import` | `infrastructure.dimension.meter` | `sdongle_inverter_exposed_meter_group` | I32, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | direction stated; `reset_wrap_and_continuity_never_inferred` | any role |
| `infrastructure.energy.export` | `infrastructure.dimension.meter` | `sdongle_inverter_exposed_meter_group` | I32, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | direction stated; `reset_wrap_and_continuity_never_inferred` | any role |
| `infrastructure.power.import_active` | `infrastructure.dimension.grid_connection` | `sdongle_plant_grid_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `positive_import` | `grid_connection` |
| `infrastructure.power.export_active` | `infrastructure.dimension.grid_connection` | `sdongle_plant_grid_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `positive_import` | `grid_connection` |

Rules for the intake, each a Hypothesis of this gate unless stated:

1. **Sign references differ by source.** The dongle's plant grid power is
   positive while importing, while the inverter-exposed group and the EMMA's
   external-meter note are positive while exporting; the logger's meter power
   states no sign (Unknown). The references are never mixed (the monitoring
   contract states the difference as a derived note).
2. **Direction split.** The pack separates import and export active power. A
   signed native value may be split only under a stated sign reference; without
   one, neither fact is published. The unused direction of a split is not a
   native zero and needs its own rule at admission.
3. **Energy direction.** The logger's three active-energy totals state no
   direction (Unknown), so none maps to an import or export counter until a
   direction is graded.
4. **Voltage, current and active power per phase.** The planned electrical group
   carries them, but their phase assignment is not stated by the contract
   (Unknown), so `infrastructure.ac.voltage`, `infrastructure.ac.current` and
   `infrastructure.ac.active_power` are requested and withheld with no
   prospective row.

## Explicit non-mappings

| Subject | Statement | Grade |
| --- | --- | --- |
| Meter identity | No make, model or firmware of any meter is established, and the listing label, an operator declaration or a readable register is not identity. | Unknown |
| Exposure | Whether a meter serves its own unit or its values sit at the gateway unit is Unknown. | Unknown |
| Gateway-local state | Driver connection, readiness and acquisition state are not electrical connection, readiness or fault. | Proven as a statement of the Infrastructure pack |
| Operations | No import or export limit, enable, write, acknowledgement, readback, retry or route exists, and the pack indexes no operation. | Proven as a statement of the pack |

## Lifecycle

A case carries the observation identifier and revision, the receipt on a wall and
a monotonic axis, the clock epoch, the source epoch, the driver generation, the
connection generation, the registry outcome, the role and any reads. The first
error of this order is reported: gate revision required, native evidence invalid,
identity missing or invalid, role invalid, lifecycle missing, mapping
unqualified, unsupported or withheld. Withdrawal is explicit and
generation-fenced.

## Acceptance vectors

| Vector | What it proves |
| --- | --- |
| `MTR-POS-001-grid-role-no-meter-read` | A declared grid-connection role keys nothing today: every meter item is withheld and Unknown. |
| `MTR-POS-002-role-undeclared` | A meter without a declared role has role Unknown and no role-dependent fact. |
| `MTR-POS-003-feeder-role` | A feeder role takes no direction-separated power. |
| `MTR-POS-004-groups-answer-nothing-decoded` | A group that answers is neither a value nor an identity. |
| `MTR-POS-005-meter-read-exception` | An exception or a failed read withholds the item and negates nothing else. |
| `MTR-POS-006-selection-ambiguous` | Every allowed registry outcome withholds the same items. |
| `MTR-NEG-001-meter-selected` | A selected meter before this gate is revised fails closed. |
| `MTR-NEG-002-meter-asset-claimed` | No meter identity exists without a class contract. |
| `MTR-NEG-003-decoded-words-supplied` | No word of a meter group is decoded, so words cannot be supplied. |
| `MTR-NEG-004-refused-group-fed-as-read` | A refused group is not a readable item. |
| `MTR-NEG-005-unknown-read` | A read of an item the gate does not list is rejected. |
| `MTR-NEG-006-source-missing` | A blank source identifier fails closed. |
| `MTR-NEG-007-role-derived-from-sign` | A role is configuration and never derived from a value. |
| `MTR-NEG-008-role-outside-set` | A role outside the configured set is rejected. |
| `MTR-NEG-009-lifecycle-missing` | The observation identifier is required. |
| `MTR-NEG-010-live-qualified` | A live qualification requires a revision of this gate. |
| `MTR-NEG-011-operation` | The gate has no operation, route or authority. |
| `MTR-NEG-012-direction-split` | A signed value is not split without a stated sign reference. |
| `MTR-NEG-013-mixed-sign-references` | Sign references differ by source and are never mixed. |
| `MTR-NEG-014-error-precedence` | The first error of the precedence list is reported. |

## Unresolved and falsifiers

| Question | Falsifier | Grade |
| --- | --- | --- |
| What are the make, model and firmware of each meter, and which identity read does a meter serve? | A meter class contract with an identity read. | Unknown |
| Is a meter exposed at its routed unit, at the gateway unit, or both? | An observation of the exposure path. | Unknown |
| What sign does each source use, and which direction does each energy total carry? | A reading checked against an independent measurement at import and at export. | Unknown |
| Which EMMA sensor group carries the backup box's meter? | An observation that pairs a group with the box. | Unknown |
| Does the final multi-endpoint contract key one source per endpoint instance? | The contract of `helianthus-docs-ebus` issue 746. | Hypothesis |

## Later consumer cutover

Consumers read these facts only through the shared semantic snapshot. This gate
publishes no consumer binding. Any implementation must bind consumers in one
atomic cutover and keep `outbound_allowed=false`.

## Validation

Run `python3 scripts/validate_huawei_gateway_meter_infrastructure_mapping.py`,
`python3 scripts/test_validate_huawei_gateway_meter_infrastructure_mapping.py`
(also collected by `python3 -m pytest`), and `./scripts/check_docs.sh`, which runs
both. The validator first runs the shared graph rules of
`scripts/mapping_gate_graph_rules.py` and then its own rules. It executes every
vector, checks each prospective target against the Infrastructure pack catalog
and the element catalog, takes each role's relation and each subject kind from
the element catalog, checks that every native item has exactly one disposition,
and compares the tables of this page with the contract.
