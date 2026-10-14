# Tesla FC100 EVSE catalog scope

The public Gen3 FC100 catalog is limited to EVSE interoperability for HEMS and
SmartHome consumers, including Matter and eeBUS EVSE profiles. The native owner
is `helianthus-modbusreg`; this catalog does not define universal semantics or
consumer bindings. `TeslaHSCFC100OperationCompatibilityV1` remains
`wc3_24_44_3`. An operation name or decoded body does not qualify a live device,
admit an exchange, or authorize a control.

## Retained operation set

The retained set follows the public
[TEDAPI EVSE scope](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/12bd50b95fddfdfdfb7b73771b05f972dc0ecd09/protocols/tesla/tedapi.md),
[Gen3 HSC and exact current-limit body contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/12bd50b95fddfdfdfb7b73771b05f972dc0ecd09/protocols/tesla/gen3-hsc-rtu.md),
and [current-limit operation contract V1](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/12bd50b95fddfdfdfb7b73771b05f972dc0ecd09/protocols/tesla/gen3-current-limit-operation-v1.md).
These pages define the EVSE scope and the three current-limit pairs. They do
not publish the family/tag numbers for `common.system_info` or the four WC
observation pairs in the table below. Those five pairs require the
[helianthus-docs-modbus#155, published at `fb4fee5b`](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/fb4fee5b5097a9d8edf98e24b8a2fea600998ee9/protocols/tesla/tedapi.md) publication described under "Regression and
evidence boundary"; that documentation companion is merged.
The two existing system-information pairs retain identity and capability/version
provenance separately. The retained field labels are Helianthus recovered
labels; they do not establish new field semantics or live qualification.

| Catalog operation | Family | Request → terminal tag | EVSE scope |
| --- | --- | --- | --- |
| `common.system_info` | 4 | 2 → 3 | System identity and firmware/version provenance |
| `wc.get_vitals` | 6 | 1 → 2 | Charging state and electrical observations |
| `wc.get_lifetime_stats` | 6 | 3 → 4 | Lifetime and energy observations |
| `wc.get_system_info` | 6 | 9 → 10 | Hardware identity and capability provenance |
| `wc.get_load_sharing_state` | 6 | 11 → 12 | Native current capability and limits |
| `wc.configure_settings` | 6 | 7 → 8 | Persistent current limit only; t8 is returned settings state |
| `wc.set_provisional` | 6 | 25 → 26 | Provisional current-limit tuple; t26 is ACK only |
| `wc.get_provisional` | 6 | 27 → 28 | Provisional readback and distinct configured current |

The six observation/readback requests have exact empty bodies. The two mutation
requests retain structurally validated opaque bodies in the request matrix.
That validation and the generic request builder are offline codec primitives,
not safe current-only setter admission. Unrelated settings and unknown request
members acquire no control authority. The full admission, interlock, phase,
confirmation, immutable evidence, deadline, quarantine, and duplicate-guard
requirements remain a separate acquisition-lifecycle task (registry issue #200).
This change adds no sender, transport dependency, or lifecycle implementation.

Persistent, provisional, configured, and effective/pilot current stay distinct.
The existing exact-version typed current-limit records and retained-operation
replay decoders are unchanged. Unknown terminal members remain in the complete
bounded native body, without new names or qualification claims.

## Exclusions and native retention

Network provisioning, service/update, reset, registration, access control,
vehicle history, PPU authorization/settings, OCPP, charging schedules/commands,
fleet/site configuration, country configuration, meter configuration, and
load-sharing mutations are outside this catalog. `wc.get_config` is also outside
the retained set; it is not the scoped current-limit readback. Their exported
operation constants, request-matrix entries, recovered field-name entries, and
positive tests are removed. The Common system-information `system_update`
field label is also removed; its native bytes remain opaque in the retained
terminal. The standalone named PPU replay decoder and its
profile configuration/version hook are removed as well.

No retained EVSE decoder requires an excluded operation's terminal body, so no
opaque terminal exception to the retained set is introduced. There is no
additional native-owner review exemption. Bounded generic native framing and
unknown bytes inside a retained operation remain available; they do not create
a named excluded operation or send authority.

FC101 and FC102 have no EVSE operation mapping. Existing profile-scoped opaque
framing/native-record support remains unnamed and grants no EVSE admission.
The [legacy Wall Connector contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/12bd50b95fddfdfdfb7b73771b05f972dc0ecd09/protocols/tesla/legacy-wall-connector-rs485.md)
is separate: its retained frames and FBE0/FDE0 dynamic-current records are
unchanged. Legacy `FC` command bytes are not Modbus function codes, and this
catalog does not claim support for every Gen2 unit.

## Regression and evidence boundary

`TestTeslaFC100CatalogStaysWithinEVSEScope` pins the eight allowed operations,
family/tag pairs, and empty-request classification independently of the
implementation maps. Its source guard walks every non-test Go file in the
package and checks operation-typed string values in constants, conversion
expressions, variables, and composite literals. Mutation tests exercise the
three reported declaration escapes and slice, map, and struct literal variants.
It checks both the request catalog and recovered-name catalog.
`TestTeslaFC100RejectsOperationsOutsideEVSEScope` exercises rejection
through request construction, request-matrix validation, sequence decoding,
recovered names, and named native records. Existing retained Gen3 and legacy
replay tests continue to verify offline behavior with self-contained synthetic
vectors. No private artifact, live capture, network access, or hardware is
needed by these checks.

This is a public API contraction: callers of removed operation constants,
`TeslaFC100DeterministicApplicationStatus`, or PPU replay APIs must migrate to
an in-scope contract. The retained field labels are Helianthus recovered labels,
not field metadata published by the linked documentation contracts.

The public documentation/evidence gate applies. The family/tag numbers for
`common.system_info` (4:2 → 3), `wc.get_vitals` (6:1 → 2),
`wc.get_lifetime_stats` (6:3 → 4), `wc.get_system_info` (6:9 → 10), and
`wc.get_load_sharing_state` (6:11 → 12) are published by the documentation
companion [helianthus-docs-modbus#155, published at `fb4fee5b`](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/fb4fee5b5097a9d8edf98e24b8a2fea600998ee9/protocols/tesla/tedapi.md).
The linked docs-modbus revision `12bd50b9` does not publish those five numeric
pairs; that documentation companion is merged.

The three current-limit pairs, `wc.configure_settings` (6:7 → 8),
`wc.set_provisional` (6:25 → 26), and `wc.get_provisional` (6:27 → 28), remain
cited to the linked `gen3-hsc-rtu.md` exact body contract and
`gen3-current-limit-operation-v1.md` operation contract. No new firmware
evidence, control qualification, or acquisition lifecycle is asserted here.
