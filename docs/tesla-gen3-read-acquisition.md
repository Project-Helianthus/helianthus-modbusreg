# Tesla Gen3 read-path acquisition lifecycle

`Gen3ReadAcquisition` is the driver-owned acquisition lifecycle for the
Wall Connector Gen3 HSC read path (registry issue #208, part of #200). It
acquires only the five retained observation operations: `common.system_info`
(4:2 → 3), `wc.get_vitals` (6:1 → 2), `wc.get_lifetime_stats` (6:3 → 4),
`wc.get_system_info` (6:9 → 10), and `wc.get_load_sharing_state` (6:11 → 12).
The current-limit pairs (6:7 → 8, 6:25 → 26, 6:27 → 28) have no acquisition
mapping and are rejected as `NO_SEND`. The legacy Wall Connector RS-485
profile is a separate, non-Modbus flow and is never reached from this
lifecycle; there is no fallback in either direction. The Gen3 version label
must be exactly `wc3_24_44_3`, the only operation version the retained catalog
and decoders are qualified for; any other label, including another known
observation or a compatible candidate, is `NO_SEND` at construction.

## Public contracts followed

- `protocols/tesla/gen3-hsc-rtu.md`: 115200 8N1 serial format, the
  responder greeting (`TESLA` NUL), the host echo of those bytes, and the
  confirmation (`PASS` NUL) or refusal (`FAIL` NUL), activation
  failure/timeout/reset/unexpected-frame disabling the exchange state, and
  qualification by explicit endpoint, node, exact version, activation state,
  and single-flight correlation.
- `protocols/tesla/tedapi.md`: the retained family/tag pairs for
  `wc3_24_44_3` and the `idle -> validated -> waiting -> terminal | exception |
  deadline -> idle` request state machine.
- `protocols/tesla/gen3-current-limit-operation-v1.md`: read retries only for an
  explicit read-only allowlist with a finite budget, distinct outcome classes,
  and recovery that establishes a successor generation with a valid activation
  state before fresh admission.
- `protocols/modbus/private-function-codes.md`: the production endpoint owns
  the serial stream, framing, correlation, fencing, single-attempt exchange, and
  bounded `Recover`; the registry owns admission and operation decoding.

## State machine

```text
stopped --Start--> activating --driver confirmation after host echo--> ready
activating --readiness probe acknowledged--> ready
ready --Acquire/PollOnce--> ready                      (ack, rejected, no_send)
ready --fencing failure--> recovering --Recover ok--> ready
recovering|activating --failure--> unavailable
unavailable --Restart after observed activation--> ready
unavailable --Restart before observed activation--> activating
any --Close--> closed
```

A successful activation opens one lifecycle epoch. Recovery keeps that epoch,
clears the pinned generation, and does not call the driver. The next admitted
exchange pins the successor generation of the same endpoint instance. Restart
after an observed activation opens a successor endpoint instance in the same
epoch, enters ready, and sends nothing. Restart before any observed activation
enters activating and calls the driver. Admission is bound to the owned
endpoint instance, the planned exact request payload, the configured node and
endpoint label, and the transport generation pinned for that instance; a
retired endpoint's admission denies every candidate. The FC03/FC04 read path
of the endpoint admits nothing.

## Scheduling, retries, and retention

One lifecycle call owns the single in-flight slot; concurrent `Acquire`,
`PollOnce`, and `Restart` calls fail fast with
`ErrTeslaGen3ReadAcquisitionBusy`. Each transport exchange has exactly one send
attempt. `MaxAttempts` (1 through 3) bounds exchanges per acquisition, all
inside `AcquisitionDeadline`. A timeout or indeterminate outcome after possible
transmission is followed by bounded recovery in the same epoch, without a
second activation, before any retry. Both stay inside the remaining acquisition
budget. An expired or cancelled acquisition does not start recovery: the
endpoint stays fenced, the lifecycle becomes unavailable, and `Restart` is
required.
The response classifier accepts as terminal only the request operation's exact
family/terminal-tag container or a TEDAPI application error; any other frame is
a classifier fault that fences the transport generation, so a delayed terminal
can never satisfy a later attempt. A correlated exception or application error
is not retried. `PollOnce` acquires each configured operation whose
last-known-good record is not fresh and current; `Run` repeats it every
`PollInterval` and restarts an unavailable lifecycle at most once per interval.

Each operation has its own freshness and retention window. A failed attempt
never replaces the last acknowledged record. Records older than the retention
window are discarded. `Current` is true only while the lifecycle is ready and
the record's epoch, endpoint instance, and generation are the live ones. A
fenced generation, another endpoint instance, or a closed lifecycle remains
historical.

## Records and evidence states

A record carries the endpoint label, node, operation and family/tag pair,
profile and operation versions, the Gen3 version label and disposition, the
source, lifecycle epoch, endpoint instance, transport generation and request
identifier, attempt
count, lifecycle and transport timestamps, the exact request ADU, every
received response ADU, and the evidence state:

| State | Meaning |
| --- | --- |
| `ack` | Correlated terminal accepted by the operation decoder in a receipt that is current for the owned endpoint and epoch; carries the complete native terminal body. A success receipt completed after Close or retirement is `indeterminate`. |
| `rejected` | Correlated Modbus exception or TEDAPI application error; no value. |
| `timeout` | Exchange or acquisition deadline after possible transmission; no value. |
| `indeterminate` | Other failure after possible transmission, or a correlated response the decoder rejected; no value. |
| `no_send` | Proven pre-send rejection. |

A record is native evidence only. It is not a support, hardware, or live
qualification claim, and it creates no control authority.

## Activation boundary

The lifecycle drives the activation exchange through an injected
`HSCActivationDriver` on the endpoint it owns. The responder greeting is
`TESLA` followed by NUL. The host echo is those same bytes. The responder then
sends the confirmation `PASS` followed by NUL, or the refusal `FAIL` followed
by NUL and a new greeting. Readiness requires one or more attempts of the form
greeting, optional echo, and refusal, ending with greeting, echo, and
confirmation, and each echo must equal the greeting immediately before it. A
confirmation with no echo, a final refusal or greeting, a differing echo,
unknown bytes, a wrong order, or a report over the resource ceiling fails
closed with `ErrTeslaGen3HSCActivationFailed` and leaves the lifecycle
unavailable.

The activation deadline is sampled on the lifecycle clock before the driver is
called. A report returned at or after that deadline is rejected even when its
bytes match. Cancellation is checked again inside the readiness commit, under
the same mutex `Close` takes. The final ownership check, ACK promotion, and
retention run in one critical section with `Close`, so a record whose endpoint
was closed or retired before publication is `indeterminate` with its frames
kept and no decoded value.

Recovery keeps the epoch and does not call the driver. Restart after an
observed activation opens the successor endpoint instance, enters `ready` in
the same epoch, and sends nothing. Restart before any observed activation
enters `activating` and calls the driver.

`ReadinessProbe` is off unless the configuration sets it. When it is on, and
the driver returns no event and no error for the whole activation timeout, the
lifecycle admits one exchange of the first configured operation while still
`activating`. An acknowledged record opens the epoch. Any other record leaves
the lifecycle unavailable and retires the endpoint. The probe is never used
after any event was reported, and admission allows no other exchange while
`activating`.

`ActivationOpening` returns a copy of the report that opened the current
epoch, that epoch, and the basis (`driver_exchange` or `probe`). It is nil
before an activation has been observed. A later Restart keeps that opening.

A nil driver is `NO_SEND`. This repository ships no production driver and does
not open a serial port. The host supplies the driver. Tests use a scripted
peer. The echo window, the retry count, and the host-side turnaround are not
established and must be measured.
