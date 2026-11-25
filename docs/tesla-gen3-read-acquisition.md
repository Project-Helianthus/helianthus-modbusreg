# Tesla Gen3 read-path acquisition lifecycle

`Gen3ReadAcquisition` owns the configured Wall Connector Gen3 endpoint and
admits five baseline FC100 observations: Common system information, vitals,
lifetime statistics, Wall Connector system information, and load-sharing state.
See the [catalog](tesla-fc100-evse-scope.md) for the family/tag pairs.

The driver's read floor is `wc3_24_44_3`. It compares identified firmware
versions numerically, admits later builds with compatible activation,
private-function and operation capabilities, and retains the incoming label
and disposition. The baseline has grade `Proven` and the other admitted
builds have grade `Hypothesis`. A below-floor, unidentified, or
capability-incompatible build has a named reason and sends nothing; a legacy
identity is not a Gen3 candidate. This is product admission, not a statement
of protocol applicability. The RS-485 Site Controller Protocol §§1.5 and 4.2
states applicability per firmware; behavior for an unnamed build is not
specified. MID is a separate firmware line.

## State machine

```text
stopped --Start--> activating --driver confirmation or qualified probe--> ready
ready --Acquire/PollOnce or an admitted native call--> ready
ready --observation fencing failure--> recovering --Recover ok--> ready
ready --fenced mutation exchange or rejected terminal decoding--> unavailable
recovering|activating --failure--> unavailable
unavailable --Restart with an unchanged activation epoch--> ready
unavailable --Restart before observed activation--> activating
any --Close--> closed
```

The host supplies the activation driver. The RS-485 Site Controller Protocol
§§3.1–3.6 defines conditions, reevaluation, and firmware-specific attempts.
For 24.44.3 and 26.34.0 the start-up tokens have no RTU envelope. The peer
returns the six probe bytes unchanged without added delay; evaluation waits
at most 2 ms nominally when no echo byte has arrived. The station attempts ten times on 24.44.3, MID 25.2.2, 26.18.0 and 26.34.0;
24.28.3 keeps probing. Latest accepted arrival and the interval between
attempts are not specified. The local lifecycle
reuses activation across transport recovery or local Restart only while the
station's activation epoch is unchanged. After observed activation, local
`Restart` opens a successor endpoint in the same epoch, enters `ready`, and
sends nothing. It neither resets the station nor detects a station reset.
A station reset or a change that invalidates activation requires a new
lifecycle and activation decision. The optional readiness probe is a single
planned observation when the driver sees no activation event.

Admission binds the owned endpoint instance, node, endpoint label, exact
planned payload and transport generation. Modbus owns framing, integrity,
single-attempt exchanges and bounded recovery. The lifecycle owns admission,
decoding and retained evidence. Concurrent acquisitions, restart and native
mutations contend for the same single-flight reservation. Observation retries
stay within `MaxAttempts` and `AcquisitionDeadline`. Mutation exchanges and
their reserved reads do not retry or cross into a successor generation.

## Vitals

`Acquire` and `PollOnce` decode a correlated family-6 tag-2 terminal on
identified firmware 26.34.0 or 26.42.0. The TEDAPI Specification
§§6.4.1, 6.6.1, 8 and 9.3 defines the `vitals` member and its layout.
This is a decoding decision, not an extension of read admission. Other
builds keep the native terminal without assigning typed values. The
optional activation probe establishes readiness, not a typed vitals read.

`Gen3ReadRecord.Vitals()` gives a copy of the decoded record for an accepted
read containing that member. Scalar defaults do not erase wire presence:
explicit zero or false differs from an omitted scalar. An omitted wrapper
reports no value; an empty wrapper reports the default scalar and records
that the scalar itself was omitted. See the [vitals catalog entry](tesla-fc100-evse-scope.md)
for units and interpretation.

`TerminalBody()` and `ResponseADUs()` return detached copies. Additional
members remain in the native body. A malformed typed response, including a
wrong documented singular wire type or a duplicate singular member, retains
the receipt as the last attempt without replacing `LastGood`. An out-of-date
receipt or a receipt from the wrong transport generation likewise cannot
publish a new typed value. Retained historical values still carry the
existing `Fresh` and `Current` flags. Poll selection, cadence and write
admission are unchanged.

## Readback and native calls

The empty family-6 t27 request and t28 reply are selected separately from the
baseline reads and the setter. The TEDAPI Specification §6.4.11 specifies the
RS-485 pair on 26.34.0 and 26.42.0; otherwise this driver requires explicit
compatible t27/t28 qualification. A configured but unqualified t27 does not
suppress baseline observations. When qualified, only inclusion in
`Operations` schedules a poll; a valid t28 window can separately enable a
reserved read. `Retained` exposes the last good t28 and the latest attempt;
`ProvisionalReadback` returns a copy of the typed values and their wire
presence. Malformed, duplicate, or wrong-type documented members cannot replace
last-known-good evidence.

`SetProvisionalCurrentLimit` requires ready state, independently admitted
readback and a valid t28 window. It selects one of two paths within the same
single-flight reservation and deadline:

- **24.44.3 compatibility:** Requires explicit t25/t26 qualification. The
  specification does not define this exchange for that firmware. The driver
  uses the latest reusable t28 in the same generation as the bound, taking
  an initial t27 only if that generation has never seen a t28. Expiry does
  not authorize a new prerequisite read in a generation that already saw
  a t28. An absent or explicit-zero configured current, or a bound below
  the encoded current, denies the set.
- **26.34.0 and 26.42.0:** No separate setter declaration is required for
  these identified builds. The setter uses a reserved 6/5 → 6/6 query to
  obtain the installation maximum; a missing, nonpositive or insufficient
  value denies the mutation. The t28 configured-current scalar is not the
  installation maximum. The setter sends t25 once, accepts the correlated
  t25 reply even if it equals the request, and then reads t27 → t28. A
  status-bearing error is not an acknowledgement. These are the only builds
  admitted by this path (TEDAPI Specification §§6.4.3, 6.4.10–6.4.11, 7.1
  and 7.8; RS-485 Site Controller Protocol §5.4).

The conservative product policy sends positive limits of at least 6 A for
900 seconds with inhibit false and encodes a pause as 6 A for 900 seconds
with inhibit true. The specification bounds the request timeout at 86399
seconds and checks the current against stored capacity; it does not specify
a nonzero minimum current. The station still applies its own checks. The
product neither sends a zero-valued release nor defines renewal scheduling.
On 26.34.0 the returned timeout is remaining time, so a smaller present
value is not a mismatch; expiry returns to the hardware default. Current-limit
behavior on 26.42.0 is not specified (TEDAPI Specification §7.1).

The result keeps intent, encoded values, the bound and the native phase
receipts separately. A successful set reply does not confirm applied current.
Only correlated readback in the same generation contributes to `match`,
`mismatch` or `unknown`; absent members are not synthesized. The legacy
matching-only record is limited to the compatibility path. A failed
confirmation keeps the successful set receipt. A transmitted t25 or a
post-acknowledgement readback can time out without confirming the effect.
If only the prerequisite query ran and the mutation was denied, `NoSend`
refers to t25; the query receipt is retained in the result.

`ConfigurePersistentCurrent` takes a `PersistentSettingsRequest` with signed
current, a caller operation identity, and a deadline, and returns a
`PersistentSettingsResult` that keeps requested and returned current separate.
It requires an explicit current-only capability, exact schema, a valid settings
retention window and caller-supplied `PersistentIdentityCapacity`. A matching
duplicate returns historical disposition rather than sending or promoting
freshness. A changed intent for an existing identity is rejected. The receipt expires according to
the caller's window; the identity still occupies capacity so expiry cannot
turn an old call into a new mutation. Reaching capacity denies new identities
rather than overwriting the duplicate guard. Restart does not rebind history.

## Evidence boundary

Records retain the version, operation and family/tag pair, lifecycle epoch,
endpoint instance, transport generation and request identifier, timestamps,
exact request and response ADUs, outcome and available status. `Current`
requires the live epoch, instance, and generation; expiry discards reusable
ordinary retained payloads. Evidence is not charging-current confirmation,
qualification for a different operation, or permission to act on hardware.
