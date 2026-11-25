# Tesla FC100 EVSE catalog scope

This catalog retains Modbus-native evidence for the Wall Connector Gen3 family.
It does not define semantic bindings, applied charging current, or send
admission. `HSCFC100OperationCompatibilityV1` identifies the driver codec; it
is not a firmware range or a qualification claim.

| Catalog operation | Family | Request → terminal tag | Scope |
| --- | --- | --- | --- |
| `common.system_info` | 4 | 2 → 3 | System identity and firmware provenance |
| `wc.get_vitals` | 6 | 1 → 2 | Native terminal length and digest; typed disposition `unknown` |
| `wc.get_lifetime_stats` | 6 | 3 → 4 | Lifetime observations |
| `wc.get_system_info` | 6 | 9 → 10 | Hardware identity and capability provenance |
| `wc.get_load_sharing_state` | 6 | 11 → 12 | Native current capability and limits |
| `wc.get_provisional` | 6 | 27 → 28 | Provisional parameters and separate configured-current scalar |
| `wc.set_provisional` | 6 | 25 → 26 | Explicitly qualified t26 compatibility path only |
| `wc.configure_settings` | 6 | 7 → 8 | Current-only persistent configuration; t8 is returned settings |

The TEDAPI Specification §§6.4.10–6.4.11 and 7.1 defines the
provisional pair. On RS-485, 26.34.0 and 26.42.0 return t25, not
a specified t26; neither version is admitted to this driver's t26 path.
The typed readback preserves presence independently of the decoded value.
On 26.34.0 the timeout is remaining time, and zero current or zero
request timeout removes the limit. This driver does not add a release command
or infer behavior on another build. A status-bearing error is not an ACK,
and an ACK is not readback.

The TEDAPI Specification §§6.4.4, 6.6.2 and 7.8 defines the settings
pair on RS-485 for 26.34.0 and 26.42.0. The specification does not say
whether omitted settings are retained or cleared. The driver therefore
requires independent qualification of the exact t7/t8 schema and current-only
scope before it sends a t7. Its encoder writes only signed int32
`settings.maxOutputCurrentAmps`. A correlated t8 can disagree with the
request; the result reports both values and retains the native body including
other settings members. Persistent, provisional, configured and effective
current are distinct.

The [managed lifecycle](tesla-gen3-read-acquisition.md) exposes only
`SetProvisionalCurrentLimit` and `ConfigurePersistentCurrent` as native
mutations. The latter accepts a current, operation identity and deadline and
returns the original receipt with a new-execution or historical-duplicate
disposition. The host can register it by hand; it is not the semantic
allocated-current setter and does not accept raw function, address, or bytes.

The RS-485 Site Controller Protocol §§4.1–4.2 and 6–10 defines functions
100–105 with firmware-specific applicability. This lifecycle does not send
FC101–105 or expose separate enable/disable commands. Native opaque retention
for FC101/102 does not imply opaque wire schemas, or grant admission. The
legacy Wall Connector RS-485 profile remains separate and never acts as a
fallback.
