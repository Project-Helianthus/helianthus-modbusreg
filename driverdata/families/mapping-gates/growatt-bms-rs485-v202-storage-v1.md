# Growatt 1xSxxP RS-485 v2.02 storage mapping gate v1

This accepted, source-specific gate maps a bounded native read-only observation
to `helianthus.pack.storage@1.1.0`. It is neither a device support claim nor a
physical qualification. Its machine-readable companion is
[the mapping vectors](growatt-bms-rs485-v202-storage-v1.json).

The executable vectors use offline synthetic RTU observations with valid frame
shape and CRC. They are deterministic mapping evidence, not observations from
hardware and not a device-compatibility claim.

## Publishable evidence pins

The native source is [Growatt RS-485 v2.02 documentation at substantive revision
`6c08d4d`](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/6c08d4d2acf70bea622da333f6d75e26d2d92621/protocols/growatt/bms-rs485-1xsxxp-v202.md).
The [lifecycle gate](https://github.com/Project-Helianthus/helianthus-docs-modbus/commit/35151979c8561d5dc4215030899277aec36d2f9f),
[registry input](https://github.com/Project-Helianthus/helianthus-modbusreg/commit/7853d903970a4fdded35abaef01fe30f7e93be6a),
and [SemReg input](https://github.com/Project-Helianthus/helianthus-semreg/commit/f3f761bc67e10d6a65eba6c13cb4dc51002d6955)
are pinned. Storage docs/runtime inputs are `c20fdef000fe95df95fca60c55d651a0614c4efd`
and `6556bf4b3fffd645b6c14ebfd194d8570b7c5207`.

Gateway composition is accepted only at [main
`32244901c4c8337266cd348bdad90fb9e8eb0a61`](https://github.com/Project-Helianthus/helianthus-ebusgateway/commit/32244901c4c8337266cd348bdad90fb9e8eb0a61), tree
`0a850d5646d46f5782b1396d72a3d93bffa6974e`, from reviewed source
[`15ab7f0f4197485c83c04f8fe24000ebe058cb7a`](https://github.com/Project-Helianthus/helianthus-ebusgateway/commit/15ab7f0f4197485c83c04f8fe24000ebe058cb7a).
These pins are evidence, not a
runtime dependency on a sibling checkout.

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

The gate states what section 15 of the [graph contract](../graph.md) asks of
a mapping gate, and its validator runs the shared graph rules before its own.

This is section 15 item 1. The catalog lists no identification
scheme for this low-voltage BMS family, so an operator label anchors each
published resource. No pack token is read, so none is recorded as a future
native token. A pack serial becomes one only when the native owner documents
it as stable; until then its stability is Unknown.

| Kind | Anchor class | Basis | Anchor source | Scheme | Parent kind |
| --- | --- | --- | --- | --- | --- |
| `resource.battery_pack` | equipment | `operator_slot` | `operator_label` | none listed | none |
| `resource.battery_cell` | component | `operator_slot` | `operator_label` | none listed | `resource.battery_module` |

The pack is the subject of the pack facts. The cell voltage extremes are
subjects of one allocated cell. The native contract does not name which cell
holds either extreme, so both facts share that one cell and the index stays
unknown. The cell's parent kind is the module. The module itself is not
published, and the module-to-cell relation stays unknown, not absent. No
battery, string, interface or storage system is published. In every fact key,
the value of the subject dimension is the gateway-allocated resource
identifier of that subject (RES-5). It is never the asset, source or binding
identifier. A key that carries one of them, or no subject identifier, fails
closed as `identity_missing_or_invalid`.

## Relations

This is section 15 item 2. One unit reports the values of one pack and the
two cell-voltage extremes. It states neither the battery the pack belongs to
nor its modules, strings or interfaces, and it does not state which cell
holds an extreme, so the gate's evidence supports no relation. The gate states none
and makes no absence statement. A relation record whose endpoint the gate does
not publish is rejected as a dangling endpoint (REL-8). An example is a
module-to-cell relation keyed by a cell index with no published module. An
ordinal position is never a basis (section 7.2).

Section 7.2: when neither an instance nor an absence statement exists, a
relation is unknown, not absent. Every Storage relation stays unknown here.

| Definition | From kind | To kind | Withheld by |
| --- | --- | --- | --- |
| `storage.relation.battery_pack` | `resource.battery` | `resource.battery_pack` | `unsupported:repeated_pack_identity` |
| `storage.relation.module_cell` | `resource.battery_module` | `resource.battery_cell` | `unsupported:pack_module_cell_string_topology` |
| `storage.relation.pack_interface` | `resource.battery_pack` | `resource.storage_interface` | `unsupported:charge_discharge_limits` |
| `storage.relation.pack_module` | `resource.battery_pack` | `resource.battery_module` | `unsupported:pack_module_cell_string_topology` |
| `storage.relation.pack_string` | `resource.battery_pack` | `resource.battery_string` | `unsupported:pack_module_cell_string_topology` |
| `storage.relation.system_battery` | `resource.storage_system` | `resource.battery` | `configuration:no_storage_system_declared_by_this_gate` |

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
under the [unit rule](../../README.md#unit-rule), and the exact factor and
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
