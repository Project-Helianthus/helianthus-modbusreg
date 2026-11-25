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
station's activation epoch is unchanged. A station reset or a change that
invalidates activation requires a new lifecycle and activation decision;
local Restart does not detect a station reset. The optional readiness probe
is a single planned observation when the driver sees no activation event.

Admission binds the owned endpoint instance, node, endpoint label, exact
planned payload and transport generation. Modbus owns framing, integrity,
single-attempt exchanges and bounded recovery. The lifecycle owns admission,
decoding and retained evidence. Concurrent acquisitions, restart and native
mutations contend for the same single-flight reservation. Observation retries
stay within `MaxAttempts` and `AcquisitionDeadline`. Mutation exchanges and
their reserved reads do not retry or cross into a successor generation.

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

`SetProvisionalCurrentLimit` requires ready state, explicit qualification of
the exact t25/t26 compatibility path, independently admitted t27/t28 and a
valid t28 window. The compatibility path is restricted to the baseline
firmware: the TEDAPI Specification §§6.4.10 and 7.1.2 specifies a different
RS-485 terminal on 26.34.0 and 26.42.0. The driver makes no t26 support claim
for those versions. A correlated status error does not acknowledge the set.

One deadline covers the initial t27 if needed, the t25/t26 exchange, and the
confirmation t27/t28. The current upper bound is taken from the latest usable
t28 in the same generation, even after it stops being fresh, until the
retention window expires. If that generation has seen a t28, loss of its
reusable payload through expiry does not authorize a new prerequisite read.
An absent or explicit-zero configured current, or a bound below the encoded
current, denies the set. The positive path sends at least 6 A, 900 seconds,
and inhibit false; a pause is encoded as 6 A, 900 seconds and inhibit true.
These are product rules, not device-wide range or expiry claims. On 26.34.0,
t28 reports remaining time, not an echo, and expiry restores the default
(TEDAPI Specification §7.1.1).

The setter keeps intent, the encoded triple, the bound, each correlated native
receipt, and the returned values separately. `match`, `mismatch` and `unknown`
compare the observed tuple with what was sent; wire absence does not become a
zero or false observation. A t26 alone says nothing about applied current.
Only a same-generation confirmation that yields `match` or `mismatch` makes
the effect determinate. A failed confirmation retains the ACK. A deadline
that expires before any request is sent is a no-send rejection. A timeout
during the transmitted prerequisite t27 or t25, or expiry while awaiting
confirmation after t26, is reported as `timeout` with possible transmission
and indeterminate effect.

`ConfigurePersistentCurrent` is a separate native call; it accepts only signed
current, a caller operation identity, and a deadline. It requires an explicit
current-only capability, exact schema, a valid settings retention window and
caller-supplied `PersistentIdentityCapacity`. A matching duplicate returns
historical disposition rather than sending or promoting freshness. A changed
intent for an existing identity is rejected. The receipt expires according to
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
