# Growatt aggregate cycle retention

This offline component implements the aggregate/base slice of the public
[Growatt battery group retention contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/growatt-battery-group-retention.md).
The [native family contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/growatt/bms-rs485-1xsxxp-v202.md)
is the protocol authority. This application lifecycle page adds no protocol
fact, read extent, qualification result or admission rule. Nothing is stable
before Helianthus 1.0.

Implementation: [aggregate owner](https://github.com/Project-Helianthus/helianthus-modbusreg/blob/main/growatt_aggregate_retention.go),
[synthetic tests](https://github.com/Project-Helianthus/helianthus-modbusreg/blob/main/growatt_aggregate_retention_test.go),
[existing complete typed decoder](https://github.com/Project-Helianthus/helianthus-modbusreg/blob/main/growatt_bms_rs485_typed.go).

## API and caller responsibilities

`NewGrowattAggregateRetention(maxCycleTicks)` requires a positive finite
maximum completion span. All tick arguments are caller-supplied monotonic
logical counters in one consistent unit. They are neither capture times nor
wall-clock age. There is no default freshness policy.

| Operation | Input and effect |
| --- | --- |
| `Begin(context, cycle, now, deadline)` | Requires a nonempty opaque source token, positive owner epoch and cycle, `deadline > now`, and `deadline - now <= maxCycleTicks`. Cycles increase within an epoch; epochs cannot decrease. A higher epoch permits resetting the cycle counter. The pending predecessor is superseded as incomplete and discarded from the latest-attempt slot. |
| `Complete(context, cycle, now, input)` | Requires the exact active context and cycle. Calls `DecodeGrowattBMSTypedReadOnlyStatus` on the complete input; success atomically replaces the typed aggregate and its four native slices. Decoded native identity must equal the declared cycle identity. |
| `Fail(context, cycle, now)` | Explicit failed acquisition with no replacement content. Known coherent continuity permits retention. Repeating the same failed outcome is idempotent. |
| `Advance(now)` | Explicitly closes a pending attempt as incomplete when `now >= deadline`. Completion or failure at the deadline is too late. |
| `Snapshot()` | Returns detached values and native slices, the latest attempt, applicability state and original last-success metadata. Reading a snapshot does not advance time. |

The owner must explicitly drive time and closure using these methods; the
component has no timer, scheduler, transport or polling loop. With no caller
operation, a pending snapshot stays pending. Time cannot move backwards,
including across epoch changes on the same instance. A rejected callback for
another context or cycle cannot advance the active owner's clock.

`GrowattAggregateContext` contains `SourceInstance`, `OwnerEpoch`, `UnitID`,
`Function`, the entire `Revision`, `IdentityKnown` and `Identity`. The token
is internal correlation, not a serial number, endpoint, public installation
identifier or wire transaction identifier. Callers must start a newer epoch
on owner restart or source replacement. No credential or permission check is
added. Existing trust and profile admission boundaries remain separate.

The four revision components are family, file revision, header version and
cumulative revision. They are supplied independently of telemetry and must
match the exact existing decoder contract. A matching but unsupported context
and input still cannot succeed. A changed unsupported context invalidates the
old value before any completion can be accepted.

The native identity is compared field by field using existing decoder evidence:

| Identity field | Existing native evidence |
| --- | --- |
| MCU software version | Typed MCU version from the whole word at `0x0001` |
| BMS company and generation | Low and high bytes of `0x000D` respectively |
| Pack company and generation | Low and high bytes of `0x000E` respectively |
| Topology | Whole native word at `0x001F`, without inferring another pack |
| Cell-series count | Whole native word at `0x0029` |

`IdentityKnown` is an explicit caller assertion that **every** required identity
field has coherent evidence for that cycle. It is not inferred from nonzero
values, equal telemetry or a readable unit. The caller must declare unknown
continuity at cycle start when this evidence is unavailable. A failed or
partial acquisition cannot establish new continuity: retention relies on the
independently established known cycle context. A complete raw identity that
contradicts the context invalidates applicability even when typed telemetry
encoding also fails. Partial words never become replacement data. The
component does not decode or store diagnostic groups.

Source tokens are bounded to 128 bytes, MCU version strings to 32 bytes, and
each revision component to 64 bytes. Other context members are fixed-size.
`GrowattAggregateHistoryLimit` is one last-success record; only one latest
attempt is stored. A new success discards the previous success. There is no
historical query or persistence API, and no unbounded callback ledger.
Counters fail closed on replacement-revision exhaustion; no wrapping revision
is published. Rejected invalid starts do not replace the active cycle.

## Applicability and success evidence

| State | Value and meaning |
| --- | --- |
| `absent` | No successful value has ever been accepted. No default-filled aggregate. |
| `current` | Complete success in the latest cycle. This is cycle membership, not freshness. |
| `retained` | Applicable earlier success during a pending, failed or incomplete later cycle. |
| `invalidated` | Prior evidence exists but cannot apply. `Value` is nil. |

A changed continuity key or `IdentityKnown == false` invalidates applicability.
Contradictory complete native evidence also invalidates it. A subsequent failure
or a return to an earlier key cannot resurrect invalidated values; only a new
complete coherent success restores an applicable value. First success in a
changed coherent context may start a new lineage directly.

`SuccessRevision`, `SuccessCycle`, `SuccessTick` and `SuccessContext` always
refer to the accepted success, including while invalidated. They are not
relabeled with the latest attempt. Invalidated raw/typed content stays internal
until replacement; it is not returned as another source's value. The attempt
carries its own context, cycle, deadline and `pending`, `success`, `failed` or
`incomplete` outcome. There is no claim about historical freshness without a
separately supplied consumer policy.

Each complete success increments the replacement revision even if every value
and raw word equals the previous success. A validated zero replaces nonzero.
Failure cannot update a field, native slice, success tick or success revision.
The all-or-nothing native evidence is the existing four ordered slices with
all opaque words, exact offsets, selected unit and exact revision preserved.

Identical successful completions for the latest cycle are idempotent and do
not refresh the success tick. Any different header, offset, length or word is
a conflicting duplicate and cannot overwrite the accepted success. Failure
after success also conflicts. Failed/incomplete cycles cannot subsequently
accept success. A superseded cycle or old epoch callback is rejected even if
its values match. All public operations are serialized by an internal mutex;
callers must not concurrently mutate input slices while passing them in.

## Synthetic transition examples

These logical cycles are invented and establish no hardware qualification.
Assume the complete supported revision and coherent context unless stated.

| Transition | Result |
| --- | --- |
| First complete cycle succeeds | Current, revision 1; typed and four raw slices commit together. |
| Next cycle fails or reaches its deadline | Retained at revision 1 and original success tick; latest attempt changes only. |
| Next complete cycle reports current zero | Current zero replaces nonzero; revision 2 and new success tick. |
| Another success is byte-for-byte equal | Current at revision 3; a new success still replaces. |
| A partial input contains plausible new voltage | No field or raw slice is merged; prior coherent success remains retained. |
| Source, epoch, revision or native identity changes | Old value becomes invalidated; a coherent complete success can replace it. |
| Identity becomes unknown | Old value is hidden until a complete coherent success; a failed attempt cannot restore it. |
| Earlier cycle completes late or successful duplicate conflicts | Rejected with no replacement or success-age refresh. |

## Evidence and integration boundary

- **Proven by source inspection and synthetic tests:** aggregate atomicity,
  identity/context fencing, explicit deadlines, revision replacement, detached
  evidence, bounded storage and failure retention in this component.
- **Hypothesis:** a future composition owner can supply coherent cycle context
  and drive this API at bounded deadlines. No integration is implemented here.
- **Unknown:** hardware cross-cycle continuity, measurement simultaneity,
  freshness budgets, runtime behavior, persistence and consumer serialization.

Evidence status is separate from qualification, acquisition, admission and
profile promotion. This component registers nothing and opens no transport.
It does not merge per-field results, retain diagnostics, schedule reads or
change existing decoders. Returned native status remains observation-only
with `OutboundAllowed() == false`.

MCP and GraphQL bindings are not changed or supplied. Later composition must
preserve equivalent presence, record revisions, attempt outcomes and evidence
availability, with explicit projection loss. It must not populate an ordinary
native-success response using retained data from a failed attempt. Existing
eBUS MCP compatibility is unchanged; eeBUS output binding remains driver-owned.

This is a partial implementation for issue #304 and Project #2; the parent
remains open. Merge requires the docs-modbus companion
`architecture/growatt-aggregate-cycle-retention-implementation.md` before or
with the code change. That application API/evidence document must describe the
reviewed API and cite the canonical battery group retention contract; its
owner publishes it outside `protocols/` and `types/`.

Documentation validation consists of manual source/claim, transition, example
and link-destination review. Offline tests do not establish transport,
passive-capture, runtime or hardware acceptance. Independent review and full
committed-head gates remain separate acceptance steps.
