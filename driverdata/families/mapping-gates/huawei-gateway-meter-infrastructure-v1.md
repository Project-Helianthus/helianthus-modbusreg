# Huawei gateway-meter Infrastructure mapping gate v1

This gate states what `helianthus.pack.infrastructure@1.0.0` will take from a
power meter behind a Huawei gateway once a meter is decoded, and it publishes
nothing before then. It adds no pack and no vendor-specific semantic family, and
it makes no stability promise before Helianthus 1.0. A live qualification observed on a record is a recorded
level and does not reject its mapping. Its machine-readable companion is
[the mapping vectors](huawei-gateway-meter-infrastructure-v1.json); the vectors
are synthetic and are not observations from hardware.

Every meter value is Unknown today. The sibling gates are
[the SUN2000 PV gate](huawei-sun2000-pv-v1.md) and
[the LUNA2000 storage gate](huawei-luna2000-storage-v1.md). The existing
[gateway-local runtime assessment](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/mappings/infrastructure/gateway-runtime.md)
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

The documented meter exposures are declared by this gate. They are distinct, and an exposure observed on one path is no claim about another:

- a SmartLogger meter is listed as a child and answers at its own routed unit
  (`smartlogger_listed_meter`, Proven as an observation);
- an S-Dongle meter is not listed as a child and answers inverter-side at the
  inverter child's routed unit (`sdongle_inverter_side`, Proven as an
  observation). The gateway's own plant route at unit 100 is not that unit.

An EMMA serves its built-in sensor groups at the gateway's own routed unit 0
(`emma_gateway_level`, Proven as an answer; the meter behind the groups is
Unknown). That is a known gateway sensor route, listed apart from the meter
exposures: it is no meter exposure, it cannot be declared as a configured
meter, and it establishes no meter identity.

The plant grid exchange is a separate statement. The S-Dongle plant group at
routed unit 100 states the grid power of the plant (Proven as an observation,
with a Hypothesis value form). It establishes no meter exposure and no meter
identity, and whether it comes from the same meter as the inverter-side
exposure is Unknown. A published plant fact names the grid connection resource
and carries the plant route in its provenance, and no meter. The registry
outcome `DECLARED_FROM_LISTING` applies the mapping; `INSUFFICIENT_EVIDENCE`,
`NO_ADMISSIBLE_PROFILE` and `AMBIGUOUS` are unresolved inputs and withhold every
mapped item, never zero. A meter selected by a registry profile, or a meter
asset identifier, fails closed with `gate_revision_required`: no meter class
contract exists. The listing establishes no meter at a plant route, so the meter
identity is Unknown.

A configured meter exposure names one of the two documented exposures and the
routed unit of its own route. The route read at runtime is still checked against
that declaration, and the reading must come from the routed unit the declaration
names. A read that differs, a group of another exposure, or a reading from
another routed unit is reported with `infrastructure.reason.identity_mismatch`:
nothing is guessed and the declared meter is not replaced by the meter that was
read. A route that was not read is `infrastructure.reason.identity_not_read`.
The plant group is read only from its own routed unit; from another unit it is
`infrastructure.reason.group_not_read`, and no configured meter exposure
publishes or withholds a plant fact. All registry flags stay false, and
`outbound_allowed=false` grants no operation.

Each remaining precondition is decided from the documents, with its grade:

1. the meter identity is Unknown: the documented listing establishes no meter
   at the route (Unknown);
2. the meter exposures are the SmartLogger listed meter and the S-Dongle inverter-side route, each checked at its own routed unit (Proven); the plant group at its unit is a grid exchange distinct from both (Proven);
3. type, word order, gain and unit of the consumed word are stated (Hypothesis);
4. the sign reference is graded per source and never mixed: the plant group
   states positive import (Hypothesis);
5. the role is declared in configuration and never derived (Proven);
6. the executable vectors are the scenarios of this gate (Proven).

A group whose word positions the documents do not state answers without a
decoded value, as `infrastructure.reason.answered_not_decoded`. The inverter-
exposed meter group was refused with exception 3 on the M3 series (Proven, cause
a Hypothesis), and its behavior at the other children is Unknown.

## Role and dimension

The role is configuration. The operator declares it, keyed to the endpoint and the
routed unit, and it is never derived from a register, from the sign or magnitude
of a power value, from the label, the unit number, the listing order, the model
or the presence of another meter. A declared role is configuration and not
evidence: it identifies no meter and changes no identity outcome. A meter without
a declared role has role Unknown, and a fact that depends on the role is not
published without a declaration. The vectors `MTR-NEG-007-role-derived-from-sign`
and `MTR-NEG-008-role-outside-set` pin these rules.

Under EN-8 of the [graph contract](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/graph.md), a meter's role is an instance
of a `measures` relation definition between the meter and the resource it
measures. The grid connection's home asset holds it, and its basis class is
`configuration` because the native meter contract states that the role is
configuration. The role never changes a fact definition. The validator takes
each role's relation from the [element catalog](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/element-catalog-v1.json): it
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
make, model or firmware of any meter is established. The meter identity is
Unknown, and the grid connection resource that
a published fact names is gateway-allocated. The routed unit and the
source key the role. The keying follows the multi-endpoint contract of
`helianthus-docs-ebus` issue 746 (pull request 748, open): one source per
endpoint instance. A published fact carries the grid connection resource.

## Resources

This is section 15 item 1 of the [graph contract](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/graph.md). The element
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

This is section 15 item 2. The validity scope of every row is the documented
gateway listing at the gateway power meter contract `gateway_power_meter_v1`.

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

Each native item has exactly one disposition. An item listed here is withheld
whatever is read, unless a supplied read selects the answered-not-decoded or
exception reason; the mapped plant grid power is in the intake table below.
Absent optional groups are unknown, never zero.

| Native item | Gateway path | Grade | Reason | Loss kind |
| --- | --- | --- | --- | --- |
| `smartlogger_meter_electrical_group` | smartlogger | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `smartlogger_meter_energy_totals` | smartlogger | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `emma_built_in_sensor_group` | emma | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `emma_external_sensor_group` | emma | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `sdongle_inverter_exposed_meter_group` | sdongle | Unknown | `infrastructure.reason.no_meter_class_contract` | policy |
| `sun2000_inverter_exposed_meter_group_refused` | sun2000_m3 | Proven | `infrastructure.reason.refused_on_family` | provenance |
| `profile_meter_groups` | sun2000_profiles | Unknown | `infrastructure.reason.profile_withholds_meter` | policy |

The SmartLogger rows are reads at the listed meter units whose word positions
the contract does not state. The EMMA rows are two sensor groups at the EMMA's
own unit, and which of them carries the backup box's meter is Unknown. The
S-Dongle rows are an inverter-exposed group at the child, whose word positions
are not stated, and the dongle's plant grid power, which is mapped.
Runtime reasons complete the set:
`infrastructure.reason.answered_not_decoded`,
`infrastructure.reason.read_exception_no_negation` (an exception negates neither
the gateway nor an identified inverter), `infrastructure.reason.group_not_read`,
`infrastructure.reason.identity_not_read`, `infrastructure.reason.identity_mismatch`,
`infrastructure.reason.role_not_declared`, `infrastructure.reason.opposite_direction`
and `infrastructure.reason.value_outside_pack_range`.

## Prospective intake

A row in force publishes its fact; a row not in force records only where a fact
will land. A fact lands on the grid connection only when the role is
`grid_connection`. Each target, dimension and unit is checked
against the Infrastructure pack catalog. Each row names the SI unit of its pack
field under the [unit rule](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/README.md#unit-rule) and the exact factor and
offset of its conversion, so that the SI value is the native value times the
factor plus the offset. Kilowatt at gain 1000 is published as watt by the factor
1000 and kilowatt-hour as joule by the exact factor 3600000. Every conversion
that is not the identity declares a reversible `unit` loss, the native value and
unit stay in the evidence, and every counter keeps a `policy` loss: reset,
rollover and continuity are Unknown and never inferred. Every factor here
terminates in decimal, so no conversion of this gate rounds. Quantities use the
kernel coefficient and exponent pair. There is no binary floating point, and
the validator rejects a float literal.

| Infrastructure fact | Dimension | Native candidate | Native form | SI unit and conversion | Loss | Sign, direction or counter rule | Role | In force |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `infrastructure.power.import_active` | `infrastructure.dimension.grid_connection` | `smartlogger_meter_electrical_group` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `not_stated` | `grid_connection` | no |
| `infrastructure.power.export_active` | `infrastructure.dimension.grid_connection` | `smartlogger_meter_electrical_group` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `not_stated` | `grid_connection` | no |
| `infrastructure.ac.power_factor` | `infrastructure.dimension.grid_connection` | `smartlogger_meter_electrical_group` | I16, ratio, gain 1000 | `unit.ratio`, factor 1, offset 0 | none | none | `grid_connection` | no |
| `infrastructure.energy.import` | `infrastructure.dimension.meter` | `smartlogger_meter_energy_totals` | I64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | direction not_stated; `reset_wrap_and_continuity_never_inferred` | any role | no |
| `infrastructure.energy.export` | `infrastructure.dimension.meter` | `smartlogger_meter_energy_totals` | I64, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | direction not_stated; `reset_wrap_and_continuity_never_inferred` | any role | no |
| `infrastructure.power.import_active` | `infrastructure.dimension.grid_connection` | `sdongle_inverter_exposed_meter_group` | I32, W, gain 1 | `unit.watt`, factor 1, offset 0 | none | `positive_export` | `grid_connection` | no |
| `infrastructure.power.export_active` | `infrastructure.dimension.grid_connection` | `sdongle_inverter_exposed_meter_group` | I32, W, gain 1 | `unit.watt`, factor 1, offset 0 | none | `positive_export` | `grid_connection` | no |
| `infrastructure.energy.import` | `infrastructure.dimension.meter` | `sdongle_inverter_exposed_meter_group` | I32, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | direction stated; `reset_wrap_and_continuity_never_inferred` | any role | no |
| `infrastructure.energy.export` | `infrastructure.dimension.meter` | `sdongle_inverter_exposed_meter_group` | I32, kWh, gain 100 | `unit.joule`, factor 3600000, offset 0 | unit+policy | direction stated; `reset_wrap_and_continuity_never_inferred` | any role | no |
| `infrastructure.power.import_active` | `infrastructure.dimension.grid_connection` | `sdongle_plant_grid_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `positive_import` | `grid_connection` | yes |
| `infrastructure.power.export_active` | `infrastructure.dimension.grid_connection` | `sdongle_plant_grid_power` | I32, kW, gain 1000 | `unit.watt`, factor 1000, offset 0 | unit | `positive_import` | `grid_connection` | yes |

Rules for the intake, each a Hypothesis of this gate unless stated:

1. **Sign references differ by source.** The dongle's plant grid power is
   positive while importing, while the inverter-exposed group and the EMMA's
   external-meter note are positive while exporting; the logger's meter power
   states no sign (Unknown). The references are never mixed (the monitoring
   contract states the difference as a derived note).
2. **Direction split.** The pack separates import and export active power. A
   signed native value is split only under a stated sign reference; without one,
   neither fact is published. The plant group states positive import, so a
   positive value publishes the import fact and a negative value publishes the
   export fact as its magnitude. The other direction is Unknown for that
   observation, never a native zero (`infrastructure.reason.opposite_direction`).
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
connection generation, the registry outcome, the role and any reads. The configured meter exposure, if any, and its route read are part of the case. The first
error of this order is reported: gate revision required, native evidence invalid,
identity missing or invalid, role invalid, lifecycle missing, mapping
unqualified, unsupported or withheld. Withdrawal is explicit and
generation-fenced.

## Acceptance vectors

| Vector | What it proves |
| --- | --- |
| `MTR-POS-001-declared-meter-no-read` | The role with no plant group read: the active item is withheld as not read, nothing is zero. |
| `MTR-POS-002-plant-grid-import` | A documented plant grid power of 1.5 kW, positive import, publishes import 1500 W for the declared grid connection; the export item is Unknown. |
| `MTR-POS-003-plant-grid-export` | A plant grid power of -2 kW publishes export 2000 W and leaves the import item Unknown. |
| `MTR-POS-004-identity-mismatch` | A configured SmartLogger meter whose route read at runtime names another routed unit is reported as a mismatch and replaces nothing. |
| `MTR-POS-005-identity-not-read` | A configured meter exposure whose route was not read at runtime is neither confirmed nor replaced. |
| `MTR-POS-006-role-undeclared` | A meter without a declared role has role Unknown and no role-dependent fact. |
| `MTR-POS-007-feeder-role` | A feeder role takes no direction-separated grid power. |
| `MTR-POS-008-groups-answer-nothing-decoded` | A group whose word positions are not documented answers without a decoded value. |
| `MTR-POS-009-meter-read-exception` | An exception or a failed read withholds the meter item and negates nothing else. |
| `MTR-POS-010-selection-unresolved` | Every unresolved outcome withholds the same items whatever is read. |
| `MTR-POS-011-grid-power-outside-range` | A converted power above the pack bound is withheld and never clamped. |
| `MTR-POS-012-listed-meter-route-agrees` | A configured SmartLogger meter whose runtime route agrees with its listed routed unit is not an identity mismatch; the answered group decodes nothing. |
| `MTR-POS-013-inverter-side-route-agrees` | The S-Dongle inverter-side exposure is checked at the inverter child's own routed unit and is not a plant route. |
| `MTR-POS-014-plant-read-beside-a-listed-meter` | A plant group read from the wrong routed unit is not read at its route, whatever meter exposure is configured. |
| `MTR-NEG-017-plant-group-declared-as-meter-exposure` | The plant group at its unit is a grid exchange and not a documented meter exposure. |
| `MTR-NEG-001-meter-selected` | A meter selected by a registry profile is refused: no meter class contract exists. |
| `MTR-NEG-002-meter-asset-claimed` | No meter asset identity exists without a class contract. |
| `MTR-NEG-003-word-count-mismatch` | An answered plant group carries exactly its quantity of words. |
| `MTR-NEG-004-refused-group-fed-as-read` | A refused group is not a readable item. |
| `MTR-NEG-005-unknown-read` | A read of an item the gate does not list is rejected. |
| `MTR-NEG-006-meter-declaration-missing` | A configured meter exposure without its routed unit is not a declaration and fails closed. |
| `MTR-NEG-007-grid-resource-missing` | A grid connection resource is required to publish. |
| `MTR-NEG-008-source-missing` | A blank source identifier fails closed. |
| `MTR-NEG-009-role-derived-from-sign` | A role is configuration and never derived from a value. |
| `MTR-NEG-010-role-outside-set` | A role outside the configured set is rejected. |
| `MTR-NEG-011-lifecycle-missing` | The observation identifier is required. |
| `MTR-NEG-012-mapping-unqualified` | A mapping that is not declared qualified is rejected. |
| `MTR-NEG-013-operation` | The gate has no operation, route or authority. |
| `MTR-NEG-014-direction-split` | A signed value of a source with no stated sign reference is not split. |
| `MTR-NEG-015-mixed-sign-references` | Sign references differ by source and are never mixed. |
| `MTR-NEG-016-error-precedence` | The first error of the precedence list is reported. |

## Unresolved and falsifiers

| Question | Falsifier | Grade |
| --- | --- | --- |
| What are the make, model and firmware of each meter, and which identity read does a meter serve? | A meter class contract with an identity read. | Unknown |
| Where do the active power and energy words sit in the SmartLogger meter groups? | A native page that states the word positions. | Unknown |
| Is a meter exposed at its routed unit, at the gateway unit, or both? | An observation of the exposure path. | Unknown |
| What sign does each source use, and which direction does each energy total carry? | A reading checked against an independent measurement at import and at export. | Unknown |
| Which EMMA sensor group carries the backup box's meter? | An observation that pairs a group with the box. | Unknown |
| Does the final multi-endpoint contract key one source per endpoint instance? | The contract of `helianthus-docs-ebus` issue 746. | Hypothesis |

## Later consumer cutover

Consumers read these facts only through the shared semantic snapshot. This gate
publishes no consumer binding. Any implementation must bind consumers in one
atomic cutover and keep `outbound_allowed=false`.

## Validation

Run `python3 scripts/mapping/validate_huawei_gateway_meter_infrastructure_mapping.py`,
`python3 tests/test_mapping_validate_huawei_gateway_meter_infrastructure.py`
(also collected by `python3 -m pytest`), and `./scripts/check_docs.sh`, which runs
both. The validator first runs the shared graph rules of
`scripts/mapping_gate_graph_rules.py` and then its own rules. It executes every
vector, checks each prospective target against the Infrastructure pack catalog
and the element catalog, takes each role's relation and each subject kind from
the element catalog, checks that every native item has exactly one disposition,
and compares the tables of this page with the contract.
