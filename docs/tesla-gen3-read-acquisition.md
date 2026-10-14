# Tesla Gen3 read-path acquisition lifecycle

`TeslaGen3ReadAcquisition` is the driver-owned acquisition lifecycle for the
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
  device-originated activation sequence (`TESLA` NUL, `PASS` NUL), activation
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
stopped --Start--> activating --activation ok--> ready
ready --Acquire/PollOnce--> ready                      (ack, rejected, no_send)
ready --fencing failure--> recovering --Recover ok--> activating --ok--> ready
recovering|activating --failure--> unavailable --Restart--> activating
any --Close--> closed
```

Every activated endpoint generation opens a new lifecycle epoch. Admission is
bound to the owned endpoint instance, the planned exact request payload, the
configured node and endpoint label, and the transport generation pinned for
the epoch; a retired endpoint's admission denies every candidate. The FC03/FC04
read path of the endpoint admits nothing.

## Scheduling, retries, and retention

One lifecycle call owns the single in-flight slot; concurrent `Acquire`,
`PollOnce`, and `Restart` calls fail fast with
`ErrTeslaGen3ReadAcquisitionBusy`. Each transport exchange has exactly one send
attempt. `MaxAttempts` (1 through 3) bounds exchanges per acquisition, all
inside `AcquisitionDeadline`. A timeout or indeterminate outcome after possible
transmission is followed by the transport's bounded recovery and a fresh
activation before any retry; both run inside the remaining acquisition budget.
An expired or cancelled acquisition does not start recovery: the endpoint stays
fenced, the lifecycle becomes unavailable, and `Restart` is required. The
response classifier accepts as terminal only the request operation's exact
family/terminal-tag container or a TEDAPI application error; any other frame is
a classifier fault that fences the transport generation, so a delayed terminal
can never satisfy a later attempt. A correlated exception or application error
is not retried. `PollOnce` acquires each configured operation whose
last-known-good record is not fresh and current; `Run` repeats it every
`PollInterval` and restarts an unavailable lifecycle at most once per interval.

Each operation has its own freshness and retention window. A failed attempt
never replaces the last acknowledged record. Records older than the retention
window are discarded. `Current` is true only for a record from the active
ready epoch; records from a recovered, restarted, or closed endpoint remain
historical.

## Records and evidence states

A record carries the endpoint label, node, operation and family/tag pair,
profile and operation versions, the Gen3 version label and disposition, the
source, lifecycle epoch, transport generation and request identifier, attempt
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

The lifecycle compares the bytes reported by a configured
`TeslaGen3HSCActivationWitness` with the documented sequence and fails closed
without one. A report returned at or after the `ActivationTimeout` deadline (measured
on the lifecycle clock), or after cancellation, is rejected even when its
bytes match; cancellation is checked again inside the readiness commit.
The final ownership check, ACK promotion, and retention run in one critical
section with `Close`, so a record whose endpoint was closed or retired before
publication is `indeterminate` with its frames kept and no decoded value. The helianthus-modbus production endpoint does not expose
pre-exchange activation bytes, so this repository ships no production witness;
until one exists the lifecycle cannot be constructed for live use.
