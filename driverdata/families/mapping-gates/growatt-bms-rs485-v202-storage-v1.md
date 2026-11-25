# Growatt 1xSxxP RS-485 v2.02 storage mapping gate v1

This accepted, source-specific gate maps a bounded native read-only observation
to `helianthus.pack.storage@1.1.0`. It is neither a device support claim nor a
physical qualification. Its machine-readable companion is
[the mapping vectors](growatt-bms-rs485-v202-storage-v1.json).

The executable vectors use offline synthetic RTU observations with valid frame
shape and CRC. They are deterministic mapping evidence, not observations from
hardware and not a device-compatibility claim.

## Publishable evidence pins

The native source is the Growatt RS-485 v2.02 page
`protocols/growatt/bms-rs485-1xsxxp-v202.md` of the docs-modbus repository, at
the substantive revision named in the `pins` member of the executable contract.
The lifecycle gate, the registry input, the SemReg input, the storage docs and
runtime inputs, and the gateway composition are pinned in the same member.
These pins are evidence, not a runtime dependency on a sibling checkout.

## Native admission and preservation

Admission requires the combined revision tuple `1xSxxP ESS`, `Rev2.01`,
`V2.0`, and `2.02`; one caller-selected unicast unit from 1 through 247; and
the four FC03 slices at `0x0001/7`, `0x000D/29`, `0x0100/12`, and `0x010D/2`.
Broadcast unit zero is `NO_SEND`. The immutable native path retains the request
and response ADUs with valid Modbus CRC, per-slice request IDs and exact word
counts, offsets, function, unit, tuple, observation ID and revision, wall and
monotonic receipt axes, opaque clock/source epochs, driver and transport
generations, and qualification. The four slices share the same unit and
transport generation, and request IDs are distinct. Missing or contradictory
evidence rejects the observation before mapping; missing semantic fields are
withheld or unavailable, never zero.

A gateway-configured, non-secret semantic `asset_id` and distinct `source_id`
are required for projection. Neither may be derived from unit, vendor, version,
company, or generation. A stable native binding ID is derived with a
domain-separated function over only the asset ID, source ID, and exact profile;
it never selects or changes the asset identity. Missing or invalid identity
fails closed. The revision,
selected unit, version/gauge evidence, BMS and pack company/generation,
topology, and cell-series evidence remain coherent native provenance. This
mapping may qualify its offline projection while physical qualification remains
false; it grants no device support claim or operation.

## Resources

The gate states what section 15 of the [graph contract](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/graph.md) asks of
a mapping gate, and its validator runs the shared graph rules before its own.

This is section 15 item 1. The catalog lists no identification
scheme for this low-voltage BMS family. A pack and an interface are anchored by
their native position under the configured battery, so a pack keeps its
identity from start to start and never takes it from an address or a label.
Every other published resource is anchored by an operator label. A pack serial
becomes a native token only when the native owner documents it as stable.

| Kind | Anchor class | Basis | Anchor source | Scheme | Parent kind |
| --- | --- | --- | --- | --- | --- |
| `resource.battery_pack` | equipment | `native_slot` | `native_position` | none listed | none |
| `resource.battery_cell` | component | `operator_slot` | `operator_label` | none listed | `resource.battery_module` |
| `resource.battery` | equipment | `operator_slot` | `operator_label` | none listed | none |
| `resource.storage_system` | function | `operator_slot` | `operator_label` | none listed | none |
| `resource.storage_interface` | component | `native_slot` | `native_position` | none listed | `resource.battery_pack` |

The pack is the subject of the pack facts. The cell voltage extremes are
subjects of one allocated cell. The native contract does not name which cell
holds either extreme, so both facts share that one cell and the index stays
unknown. The cell's parent kind is the module. The module itself is not
published, and the module-to-cell relation stays unknown, not absent. The
battery, the storage system and the interface are resources of the graph; no
fact is published on them, so no identifier is allocated for them yet. No
string is published. In every fact key,
the value of the subject dimension is the gateway-allocated resource
identifier of that subject (RES-5). It is never the asset, source or binding
identifier. A key that carries one of them, or no subject identifier, fails
closed as `identity_missing_or_invalid`.

## Relations

This is section 15 item 2. One unit reports the values of one pack and the
two cell-voltage extremes. The gate states the three storage relations its
evidence supports, each with evidence and the validity scope of this native
profile and revision: the battery contains its pack, the pack contains its
RS-485 interface, and the storage system the operator declares groups the
battery. It does not state which cell holds an extreme, nor any module or
string. The gate makes no absence statement. A relation record whose endpoint the gate does
not publish is rejected as a dangling endpoint (REL-8). An example is a
module-to-cell relation keyed by a cell index with no published module. An
ordinal position is never a basis (section 7.2).

Section 7.2: when neither an instance nor an absence statement exists, a
relation is unknown, not absent. The relations whose structure the native
contract does not state stay unknown here.

| Definition | Basis class | From kind | To kind | Scope of the rule |
| --- | --- | --- | --- | --- |
| `storage.relation.battery_pack` | `gate_rule` | `resource.battery` | `resource.battery_pack` | `the_pack_at_the_native_position_the_unit_reports_under_its_configured_battery` |
| `storage.relation.pack_interface` | `gate_rule` | `resource.battery_pack` | `resource.storage_interface` | `one_rs485_interface_per_pack_at_the_native_position_of_its_pack` |
| `storage.relation.system_battery` | `configuration` | `resource.storage_system` | `resource.battery` | `the_storage_system_the_operator_declares_for_each_configured_battery` |

| Definition | From kind | To kind | Withheld by |
| --- | --- | --- | --- |
| `storage.relation.module_cell` | `resource.battery_module` | `resource.battery_cell` | `native_contract:no_module_cell_or_string_structure_stated_for_a_pack` |
| `storage.relation.pack_module` | `resource.battery_pack` | `resource.battery_module` | `native_contract:no_module_cell_or_string_structure_stated_for_a_pack` |
| `storage.relation.pack_string` | `resource.battery_pack` | `resource.battery_string` | `native_contract:no_module_cell_or_string_structure_stated_for_a_pack` |

Relayed values follow section 15 item 3. The pack is read directly over RS-485
and no unit relays it. A second view of the same pack through another path
shares no native token with this one. The two are one resource only by an
operator declaration that gives both one slot (section 6.4). A join by a label,
a position or an ordinal is rejected, and so is a join of views whose native
tokens differ.

Aggregates follow section 15 item 4 and EN-5. Every mapped fact is a member
fact or a status of the pack in the catalog, so the gate states no aggregate.
Selectors follow section 15 item 5. The interpretation selectors are the
profile, the native revision tuple and the routed unit number, and all three
are provenance. The routed unit number is an address. It is never an anchor,
never identity evidence and never a resource dimension (section 6.3). The gate
states no plane membership (section 15 item 6, PLN-1).

## Subjects and measurement points

Under EN-1 each field rule names its subject kind and its measurement point.
The validator looks the subject kind up in the element catalog and does not
repeat it. The measurement points are a Hypothesis of this gate. The catalog
keeps the native sign convention of pack current, and the rule keeps it as a
`provenance` loss (EN-4).

| Storage fact | Subject kind | Measurement point |
| --- | --- | --- |
| `storage.status.operating` | `resource.battery_pack` | `pack_as_a_whole` |
| `storage.state.soc` | `resource.battery_pack` | `pack_as_a_whole` |
| `storage.pack.voltage` | `resource.battery_pack` | `pack_terminals` |
| `storage.pack.current` | `resource.battery_pack` | `pack_terminals` |
| `storage.temperature.pack` | `resource.battery_pack` | `pack_as_a_whole` |
| `storage.capacity.charge` | `resource.battery_pack` | `pack_terminals_lifetime_counter` |
| `storage.capacity.discharge` | `resource.battery_pack` | `pack_terminals_lifetime_counter` |
| `storage.cell.voltage_minimum` | `resource.battery_cell` | `extreme_cell_unindexed` |
| `storage.cell.voltage_maximum` | `resource.battery_cell` | `extreme_cell_unindexed` |

## Mapping and loss

The JSON contract fixes every exact, transformed, native-only, and withheld
field. Each quantity rule names its native unit, the SI unit of its pack field
under the [unit rule](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/README.md#unit-rule), and the exact factor and
offset of its conversion, so that the SI value is the native value times the
factor plus the offset. Volts, amperes, and ampere-hours are published by the
identity. Cell voltage extremes in millivolts are published as volts by the
exact factor 0.001. State of charge in percent is published as a ratio from 0 to 1 by the
exact factor 0.01, and the pack temperature in degrees Celsius as kelvin by the
exact offset 273.15; each conversion that is not the identity declares a reversible `unit` loss and is
transformed. Every factor and offset here terminates in decimal, so no
conversion of this gate rounds, and the native value and unit stay in the
evidence. Capacity stays in ampere-hours, is never converted to energy units,
and energy is never derived from voltage, current, time, SOC, or a decoded
combination. `charging` and `discharging` become only the semantic `active`
state with a SemReg `symbol` loss; `soft_starting` withholds only the operating
state while the eight independently valid facts remain available. Current and
counter continuity use transformed `provenance` and `policy` loss details;
an exact disposition has empty loss. Exact quantity vectors use SemReg's
canonical `{coefficient, exponent10}` decimal form derived from the admitted
fixed-scale words, rather than binary floating-point serialization.

Pack power, SOH, cell-temperature meaning, topology, repeated
pack identity, warning/error/company status, extension words, writable ranges,
and all control-adjacent data are unsupported or opaque. The cell voltage
extremes are mapped. Remaining capacity waits for `storage.energy.remaining`.
Full-charge capacity waits for `storage.energy.usable_capacity`. Cycle count,
continuous charge time, current-cycle charge, average cell voltage and
floating pack voltage stay withheld: the catalog has no element for them, and
each row names that missing definition rather than a substituted fact. A joule
capacity is not derived from an ampere-hour capacity. Counters retain the
reset/wrap loss and never infer a reset.

There is no charge/discharge limit, interlock, authority, write, acknowledgement,
readback, retry, or control route. `outbound_allowed=false` grants no operation.

## Later consumer cutover

A later implementation may bind semantic MCP, GraphQL, Portal, and Home
Assistant only through one atomic SemReg cutover. It must replace the selected
legacy semantic path and remove fallback, comparator, compatibility-only, and
dual-publication paths. Until then this gate publishes no consumer binding.

Run `python3 scripts/validate_growatt_bms_rs485_v202_storage_mapping.py`,
`python3 scripts/test_validate_growatt_bms_rs485_v202_storage_mapping.py`,
`python3 scripts/mapping_gate_graph_rules.py api/v1/mappings/growatt-bms-rs485-v202-storage-v1.json`,
and `./scripts/check_docs.sh`.
