# Tesla Gen3 WC3 24.44.3 EVSE current-limit mapping gate v1

This accepted source-specific, read-only gate maps qualified Tesla Wall Connector
Gen3 `wc3_24_44_3` current-limit evidence into
`helianthus.pack.evse@1.0.0`. The executable contract is
[tesla-gen3-wc3-24443-evse-current-limit-v1.json](tesla-gen3-wc3-24443-evse-current-limit-v1.json).
It is an offline mapping qualification, not a device-support or physical
qualification claim.

## Publishable inputs and evidence boundary

The native current-limit contract is pinned to
[docs-modbus `6611e20`](https://github.com/Project-Helianthus/helianthus-docs-modbus/commit/6611e20ac8b2c3e8b953934757346f8a847b30bf),
the read-only native-projection revision is
[`1127c333`](https://github.com/Project-Helianthus/helianthus-docs-modbus/commit/1127c333de1f1d02305952a58f8745ee05c42e12),
the typed registry is
[`57f7eb84`](https://github.com/Project-Helianthus/helianthus-modbusreg/commit/57f7eb84f7d4e1173621711bf64624726c71bc75),
the semantic package is
[`f3f761bc`](https://github.com/Project-Helianthus/helianthus-semreg/commit/f3f761bc67e10d6a65eba6c13cb4dc51002d6955),
and gateway composition is
[`32244901`](https://github.com/Project-Helianthus/helianthus-ebusgateway/commit/32244901c4c8337266cd348bdad90fb9e8eb0a61).
The accepted generic EVSE input is `helianthus.pack.evse@1.0.0` at this
repository's base `f830ace6c2b9dd1af0e87ce808fa545662578418`.

Only FC100 family-6 evidence for this exact profile is admissible. The
provisional readback `t27` to `t28` is the source of both limits.
`ConfiguredCurrentAmps` maps exactly, in integer amperes, to
`evse.limit.configured_current`. `LimitCurrentMaxAmps` maps exactly to
`evse.limit.allocated_current` only when `LimitTimeoutSeconds` is finite
`1..86399` and `InhibitCharging` is false. The zero-timeout form withholds
allocated current; timeout and inhibit themselves have no EVSE fact. The
ingested persistent `t7` to `t8` exchange is not the source of either limit
and is never substituted for the readback.

The four provisional request/ack/readback payloads, persistent request/terminal
payloads, FC100 decoding, and their immutable digests remain native evidence.
The semantic gate accepts their immutable references only. A source path,
receipt timestamp, source epoch, driver generation, qualification, semantic
revision, lifecycle generation, and field-scoped immutable evidence are
required. Persistent request/terminal evidence stays required and publishes
no limit. Both limits require the provisional readback. Its absence, or an
invalid or mismatched provisional evidence set, withholds both. Inhibition
or a zero timeout withholds only allocated current.
Evidence from observations, epochs, generations, or revisions cannot be
combined within either fact path. Permitted last-known-good retention remains
the native owner's lifecycle decision.

## Resources, relations, and identity

The gate states its resources and relations in the member `graph`, in the
form that section 15 of the [graph contract](../graph.md) requires. Its
validator runs the [shared checker](../../../scripts/mapping_gate_graph_rules.py)
first and then its own rules. Kinds, anchor classes, dimensions, relation
definitions and admitted basis classes are looked up in the
[element catalog](../element-catalog-v1.json); the gate repeats none of them as
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
invalid, reused, or ambiguous identifiers fail closed.

The station-to-EVSE and EVSE-to-connector relations are stated with basis
class `configuration`, scoped to this profile and its version, and with no
absence statement (REL-3, REL-8). The connector-to-phase, connector-to-session
and EVSE-to-meter relations are listed in `unstated_relations` as
unknown, not absent: the gate publishes no phase, session or meter, and it
never claims that a meter is missing. Native connector topology stays unsupported.

Under EN-1 each field rule names its subject kind and measurement point. The
configured current limit's subject is the EVSE it applies to, defined at the
EVSE output; the allocated current's subject is the connector, defined at the
connector output. Both subject kinds and dimensions come from the catalog. The
profile is an interpretation selector and stays provenance until the native
owner states otherwise. No plane membership appears in the gate.

## Loss and operations

Power, energy, phase, session, readiness, SOC, thermal, fault, interlock,
meter, and all unsupported native facts remain withheld or native-only. There
is no sender, route, authority, acknowledgement/readback authority, retry, or
live control. Every operation is unavailable, including
`evse.operation.set_allocated_current`; `outbound_allowed=false` is not an
operation admission. Every operation is unavailable.

## Later consumer cutover

A later gateway change may use one SemReg projection to feed versioned semantic
MCP, mTLS GraphQL and Portal with parity, then applicable Home Assistant
adoption. It must remove fallback, comparator, compatibility adapter, shadow
authority, dual publication, caller-supplied provenance, and write routes.
Until that implementation is accepted, this mapping exposes no consumer binding.

Run `python3 scripts/validate_tesla_gen3_wc3_24443_evse_current_limit_mapping.py`,
`python3 scripts/test_validate_tesla_gen3_wc3_24443_evse_current_limit_mapping.py`,
`python3 scripts/mapping_gate_graph_rules.py api/v1/mappings/tesla-gen3-wc3-24443-evse-current-limit-v1.json`,
and `./scripts/check_docs.sh`.
