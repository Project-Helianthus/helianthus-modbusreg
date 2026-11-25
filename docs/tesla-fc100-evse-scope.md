# Tesla FC100 EVSE catalog scope

This catalog retains Modbus-native evidence for the Wall Connector Gen3 family.
It does not define semantic bindings, applied charging current, or send
admission. `HSCFC100OperationCompatibilityV1` identifies the driver codec; it
is not a firmware range or a qualification claim.

| Catalog operation | Family | Request → terminal tag | Scope |
| --- | --- | --- | --- |
| `common.system_info` | 4 | 2 → 3 | System identity and firmware provenance |
| `wc.get_vitals` | 6 | 1 → 2 | Station vitals on applicable firmware; native terminal diagnostics otherwise |
| `wc.get_lifetime_stats` | 6 | 3 → 4 | Lifetime observations |
| `wc.get_system_info` | 6 | 9 → 10 | Hardware identity and capability provenance |
| `wc.get_load_sharing_state` | 6 | 11 → 12 | Native current capability and limits |
| `wc.get_provisional` | 6 | 27 → 28 | Provisional parameters and separate configured-current scalar |
| `wc.set_provisional` | 6 | 25 → 25 (26.34.0, 26.42.0); 25 → 26 (qualified 24.44.3 compatibility only) | Provisional parameters; a separate 27 → 28 read establishes returned state |
| `wc.configure_settings` | 6 | 7 → 8 | Current-only persistent configuration; t8 is returned settings |

## Vitals

The TEDAPI Specification §§6.4.1, 6.6.1, 8 and 9.3 specifies the RS-485
vitals reply on firmware 26.34.0 and 26.42.0. The driver projects station
uptime and session duration in seconds, contactor and vehicle booleans,
temperatures in °C, voltages in V (RMS where named), ground resistance in Ω,
vehicle identification, vehicle-side RMS voltage, power in W, and line
current in amperes (**Provisional** unit). The projection preserves scalar
and wrapper presence. It reports telemetry, not a command outcome or an
independent measurement of charging current.

The driver keeps the numeric OCPP code even when no enumeration name is
assigned. Fault and scheduled-charging codes have no specified enumeration
association. Powershare session state (projected on 26.34.0), dry-contact
control state and smart-charging reason retain their numeric codes; their
enumeration associations are **Provisional**. Session energy, grid AC,
latched alerts, and vehicle AC members other than voltage remain native bytes;
this projection does not ascribe behavior to members marked Not specified.

`DecodeWCVitalsReplayForFirmware` and `ExecuteForFirmware` retain the complete
terminal body with its length and digest. Unversioned replay and dispatch
continue to return length and digest only. Firmware selection adds neither
another exchange nor operation admission. The tests exercise the selected
operations offline.

## Selected native mutations and readback

The TEDAPI Specification §§6.4.10–6.4.11 and 7.1 and the RS-485 Site
Controller Protocol §5.4 define the RS-485 exchange on 26.34.0 and 26.42.0.
The t25 terminal can equal the request or omit default-valued members; it is
not a readback. A correlated status error is a rejection, not an ACK.
The specification does not define this exchange on 24.44.3; its explicitly
qualified t26 compatibility path is unchanged. The catalog codec still
represents that compatibility contract, not the firmware-selected reply.

On the documented path the setter queries installation settings (6/5 → 6/6)
under the same reservation and deadline. It uses the returned installation
maximum as its current ceiling, not the t28 configured-current scalar, whose
meaning is not specified (TEDAPI Specification §§6.4.3, 6.4.11 and 7.8). The
query is private to the setter, not a public catalog operation. After a
successful t25 reply, the setter reads t27 → t28. It preserves presence
independently of value. On 26.34.0 the timeout reports remaining time and
zero current or zero request timeout removes the limit. This driver adds no
release command. The effect of a current limit on 26.42.0 is not specified.

The TEDAPI Specification §§6.4.4, 6.6.2 and 7.8 defines the settings
pair on RS-485 for 26.34.0 and 26.42.0. The specification does not say
whether omitted settings are retained or cleared. The driver therefore
requires independent qualification of the exact t7/t8 schema and current-only
scope before it sends a t7. Its encoder writes only signed int32
`settings.maxOutputCurrentAmps`. A correlated t8 can disagree with the
request; the result reports both values and retains the native body including
other settings members. Persistent, provisional, configured and effective
current are distinct.

The [managed lifecycle](tesla-gen3-read-acquisition.md) exposes
`SetProvisionalCurrentLimit` and `ConfigurePersistentCurrent` as native
mutations. The latter accepts a current, operation identity and deadline and
returns the original receipt with a new-execution or historical-duplicate
disposition. It is not the semantic allocated-current setter.

## Explicit site-controller and charging mutations

| Lifecycle method | Native request → reply | RS-485 firmware | Specification |
| --- | --- | --- | --- |
| `ExchangeControlStatus` | Function 101 `HscControlStatusRequest` → `HscControlStatusResponse` | MID 25.2.2, 26.18.0, 26.34.0 | RS-485 Site Controller Protocol §6 and Appendix A |
| `ExchangeStateSync` | Function 102 `HscStateSyncRequest` → `HscStateSyncResponse` | MID 25.2.2, 26.18.0, 26.34.0 | RS-485 Site Controller Protocol §7 and Appendix A |
| `EnableCharging` | Function 100, 6/39 (command 1) → 6/40 | 26.34.0, 26.42.0 | TEDAPI Specification §§6.4.17 and 7.2 |
| `DisableCharging` | Function 100, 6/39 (command 2) → 6/40 | 26.34.0, 26.42.0 | TEDAPI Specification §§6.4.17 and 7.2 |

These calls share the ready lifecycle and its single-flight reservation. They
are not polled or retried, require no additional configuration switch, and
accept no raw function, address or payload. They return correlated native
evidence. A Modbus exception is not a normal reply; functions 101 and 102
have no application status field. The charging commands preserve the TEDAPI
application status on a correlated error (RS-485 Site Controller Protocol
§5.4). A successful 6/40 reply does not confirm a change in charging state.
Start and stop are charge commands, not operating-mode changes (TEDAPI
Specification §7.5) or persistent enable/disable settings.

Every decodable function-101 request replaces the stored control request,
including the provisional status-poll form. MID 25.2.2 has only update field
2.1 and snapshot fields 1–7. On the other two builds the mode hint is
**Provisional**; behavior above 1 and the effect of the external-power
recheck field are **Not specified**. Function 102 replaces the stored report
when the report is present, including an empty report. The returned vehicle
snapshot age is station storage age (nominally milliseconds), not sample
time. Only 26.34.0 projects vehicle snapshot field 5. Unknown fields remain
in the native payload, without a typed interpretation.

Although functions 101 and 102 are supported on 24.28.3 and 24.44.3, the
complete layouts are **Not specified** (RS-485 Site Controller Protocol
§§4.2, 6.8 and 7.7). Their applicability on 26.42.0 is **Not specified**.
The driver does not infer coverage on unlisted firmware. No function 103–105
operation is added. The legacy Wall Connector RS-485 profile remains
separate and never acts as a fallback.
