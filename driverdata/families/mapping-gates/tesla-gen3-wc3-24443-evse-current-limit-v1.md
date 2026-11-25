# Tesla Gen3 WC3 24.44.3 EVSE current-limit mapping gate v1

This accepted source-specific gate maps qualified Tesla Wall Connector
Gen3 `wc3_24_44_3` current-limit evidence into
`helianthus.pack.evse@1.0.0`. The executable contract is
[tesla-gen3-wc3-24443-evse-current-limit-v1.json](tesla-gen3-wc3-24443-evse-current-limit-v1.json).
It is an offline mapping qualification, not a device-support or physical
qualification claim. The native current-limit getter is read-only and sends
nothing; the operations the driver offers for this charger are declared
separately below.

## Publishable inputs and evidence boundary

The pinned inputs are listed in the `pins` member of the executable contract:
the native current-limit contract, the read-only native projection, the typed
registry, the semantic package, and gateway composition. The accepted generic
EVSE input is `helianthus.pack.evse@1.0.0`.

Only FC100 family-6 evidence for this exact profile is admissible.
`ConfiguredCurrentAmps` of the provisional readback `t27` to `t28` maps exactly,
in integer amperes, to `evse.limit.configured_current`. The persistent `t7` to
`t8` `MaxOutputCurrentAmps` is a second candidate for the same fact: when both
carry it in one epoch, the later receipt is the candidate, and an explicit zero
is unknown. `LimitCurrentMaxAmps` maps exactly, in integer amperes, to
`evse.limit.allocated_current` when `InhibitCharging` is false and the current
is nonzero, whatever `LimitTimeoutSeconds` is. A zero or unresolved timeout
leaves only the expiry unknown. A managed set with a known nonzero duration ends
at its acknowledgement plus that duration; a readback that names that set and
acknowledgement and is received at or after that cutoff withholds the nonzero allocation as `allocation_expired` and keeps the
native timeout and the known expiry, the cutoff itself. The timeout and the expiry are served in the modbusreg
namespace as native values (`LimitTimeoutSeconds` and `AllocationExpiry`), because no catalog element yet gives
either one; neither is published as an EVSE fact. An unresolved or zero timeout gives no expiry, and none is derived from it. An inhibited readback maps to 0 A at the
connector, not to the 6 A a pause carries natively. A zero current with inhibit
false leaves the allocated current unknown. Timeout and inhibit have no EVSE
fact of their own.

The four provisional request/ack/readback payloads, persistent request/terminal
payloads, FC100 decoding, and their immutable digests remain native evidence.
The semantic gate accepts their immutable references only. A source path,
receipt timestamp, source epoch, driver generation, qualification, semantic
revision, lifecycle generation, and field-scoped immutable evidence with its own
receipt timestamp are required. Persistent request/terminal evidence stays
required. The allocated current requires the provisional readback: its absence,
or an invalid or mismatched provisional evidence set, withholds it, while the
configured current may still come from the persistent candidate. Evidence from
observations, epochs, generations, or revisions cannot be combined within either
fact path. Permitted last-known-good retention remains the native owner's
lifecycle decision.

## Resources, relations, and identity

The gate states its resources and relations in the member `graph`, in the
form that section 15 of the [graph contract](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/graph.md) requires. Its
validator runs the [shared checker](../../../scripts/mapping/mapping_gate_graph_rules.py)
first and then its own rules. Kinds, anchor classes, dimensions, relation
definitions and admitted basis classes are looked up in the
[element catalog](https://github.com/Project-Helianthus/helianthus-docs-semantic/blob/de3a0b49535582ad51a0ec23011d3aebf9c04c76/api/v1/element-catalog-v1.json); the gate repeats none of them as
a constant.

- The charging station is equipment. No identification scheme is listed for
  this family and the native current-limit path carries no unit token, so its
  anchor is an operator label (RES-2, section 6.3).
- The EVSE is a component of the station, and the connector a component of the
  EVSE. The native current-limit evidence gives neither a position, so both
  are anchored by an operator label. A native slot would need a native
  position.
- A charging session is an episode hosted by the connector. The native
  current-limit evidence carries no session token, so no session resource is
  published.
- A network address, a unit number, firmware text, an ordinal of sorted
  identifiers or the asset identifier never anchors or names a resource. A
  connector numbered by sort order, a join by that order, and a station keyed
  by an address are rejected. The gate states no relayed value, join or
  aggregate.

The asset, EVSE and connector identity values are the resource identifiers the
gateway allocates for these three resources. Each names its pack dimension,
whose resource kind comes from the catalog; equal text with the asset
identifier never makes two identities one. The identifiers must be stable,
non-secret and distinct, and cannot be derived from unit ID, firmware text,
observation values, payload bytes, request order, a network address, an
ordinal of sorted identifiers, or the asset identifier. Missing, blank,
invalid, reused, or ambiguous identifiers fail closed. The meter identifier is
configuration: it is checked, and must be distinct from the other three, only
when the operator configures the charger's energy counter. A case with no meter
identifier is valid, publishes the same limits, and states no meter resource.

The station-to-EVSE and EVSE-to-connector relations are stated with basis
class `configuration`, scoped to this profile and its version, and with no
absence statement (REL-3, REL-8). The EVSE-to-meter relation is stated with basis class `configuration`: the
meter is the charger's own energy counter, declared by configuration, and it is
a meter resource in the graph with a configured identity of its own. The
connector-to-phase and connector-to-session relations are listed in
`unstated_relations` as unknown, not absent: the gate publishes no phase or
session, and it never claims that a phase or session is missing. Native
connector topology stays unsupported.

Under EN-1 each field rule names its subject kind and measurement point. The
configured current limit's subject is the EVSE it applies to, defined at the
EVSE output; the allocated current's subject is the connector, defined at the
connector output. Both subject kinds and dimensions come from the catalog. The
profile is an interpretation selector and stays provenance until the native
owner states otherwise. No plane membership appears in the gate.

## Loss and operations

Power, energy, phase, session, readiness, SOC, thermal, fault, interlock,
meter, and all unsupported native facts remain withheld or native-only. The
gate states no sender, route, authority or live control for the native getter,
and it denies no operation.

`projection.operations` lists every operation the driver offers for this
charger, and the injected MCP projection shows them as SemReg shows them:

- `evse.operation.set_allocated_current` (capability
  `evse.capability.set_allocated_current` on `evse.service.connector`, effect
  `evse.effect.set_allocated_current` on the connector): route t25 to t26 with
  readback t27 to t28, authority the setting owner through the SemReg call path.
  The same operation takes boolean `false` for `evse.limit.allocated_current_enabled`
  with no amperage, effect `evse.effect.release_allocated_current` on the same
  connector; a release needs a valid nonzero configured bound, and a missing,
  zero or invalid bound sends nothing.
- `ConfigurePersistentCurrent` in the modbusreg namespace: the persistent
  `t7` to `t8` current write.

The allocated-current set accepts whole amperes only. The configured-current
bound is a non-negative integer read from the device; a missing, malformed or
non-integer bound sends nothing. Its upper bound is the
configured current of the latest t28 in the same lifecycle generation. Where that
generation has no t28, the prerequisite t27 to t28 read may run first, and a
value above the configured current then read sends no t25. A pause at 0 A
encodes 6 A with inhibit and needs a configured bound of at least 6 A. 1 A to
5 A is refused and nothing is sent. A timeout of zero is not an interoperable
request value. The operation acts only on its own connector. The 6 A floor
follows IEC 61851-1 Annex A, which the ND02 page names as the lowest charging
current a charger can signal.

The native retained-record getter `modbus.v1.tesla.gen3.evse.current_limit.get`
is the only read-only surface: `read_only` and `outbound_allowed=false` describe
that getter alone. The operations above are separate from it.

## Later consumer cutover

A later gateway change may use one SemReg projection to feed versioned semantic
MCP, mTLS GraphQL and Portal with parity, then applicable Home Assistant
adoption. It must remove fallback, comparator, compatibility adapter, shadow
authority, dual publication, caller-supplied provenance, and write routes.
Until that implementation is accepted, this mapping exposes no consumer binding.

Run `python3 scripts/mapping/validate_tesla_gen3_wc3_24443_evse_current_limit_mapping.py`,
`python3 tests/test_mapping_validate_tesla_gen3_wc3_24443_evse_current_limit.py`,
and `python3 scripts/mapping/mapping_gate_graph_rules.py --self-test`.
