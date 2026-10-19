# Growatt aggregate provider port

This is the I23 family-local lifecycle port for supplied Growatt `1xSxxP ESS`
RS-485 battery evidence. It composes the merged
[aggregate offline provider](../growatt_aggregate_offline_provider.go) and
[same-cycle diagnostic checker](../growatt_diagnostic_commit.go).
Nothing is stable before Helianthus 1.0.

The public [native family contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/growatt/bms-rs485-1xsxxp-v202.md)
owns all register, identity and revision meanings. The
[aggregate cycle contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/growatt-aggregate-offline-provider-cycle.md)
and [battery retention contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/growatt-battery-group-retention.md)
remain the native lifecycle authorities. This page adds application composition,
with no new mapping, read extent, qualification or admission rule.

| Evidence status | Boundary |
| --- | --- |
| Proven by source and focused synthetic tests | Aggregate refresh precedes optional diagnostic validation. Exact linkage, independent native context checks, base retention, detached storage, logical expiry, cancellation and retirement fences are exercised offline. |
| Hypothesis | A future owner supplies truthful same-cycle provenance, a cooperative bounded supplier and a consistent monotonic logical clock. |
| Unknown | Acquisition simultaneity, hardware continuity, physical values, wall-clock freshness, persistence and production integration. |

These statuses are separate from qualification, promotion, freshness, runtime
admission and physical acceptance. A synthetic documentary success grants none
of those properties. Exact inverter FC04 automatic admission remains denied.

## Local API and ownership

[Implementation](../growatt_aggregate_provider_port.go) and
[focused tests](../growatt_aggregate_provider_port_test.go) define the concrete
signature. `I23NewGrowattProvider(maxTicks, maxSlices, maxWords, reader)` returns
an `I23GrowattAdapter` implementing `I23GrowattProvider`:

- `I23Refresh(context, request, clock)` returns `I23GrowattResult`.
- `I23Snapshot()` returns a detached `I23GrowattSnapshot` without advancing time.
- `I23Advance(tick)` advances the native deadline owner and expires diagnostics.
- `I23Retire()` permanently fences new work and cancels any in-flight callback.

Construction requires positive bounded ticks, a nonnil supplier and capacity
for the complete native base. Capacity is an upper bound, never permission to
widen native input. The merged decoders remain authoritative. All work is
synchronous; callbacks must cooperate with cancellation and declared bounds.
The port starts no goroutine, timer, retry, scheduler, socket or device access.
Caller input must not be mutated concurrently with entry or copying. Optional
diagnostic storage is frozen before supplier invocation; after refresh returns,
input and returned snapshots may be changed without affecting stored evidence.

A request carries the existing `GrowattAggregateOfflineCycle`, a positive,
strictly increasing local generation, a nonempty bounded opaque receipt, and an
optional `I23GrowattDiagnostic`. Receipts name successful base records; a new
refresh cannot reuse the last successful receipt. Generations are consumed when
refresh work starts, including a later native failure. A generation is local
correlation, distinct from owner epoch, native identity and success revision.
Only one refresh runs at a time; overlapping refreshes are refused with zero
supplier calls. Invalid construction, lifecycle preconditions, native context,
already-observed cancellation and retired ownership also invoke no supplier.

## Exact base association and separate outcomes

The adapter first calls the merged aggregate cycle. Its snapshot preserves
source, owner epoch, unit, function, every native revision and continuity member,
original success cycle, success tick, replacement revision and raw input.
A failed or partial base keeps last-known-good data only where the existing
native continuity contract permits it. Unknown or changed continuity invalidates
applicability. There is no field merge, SOC calculation, rack/module synthesis,
cell aggregation or inference of missing evidence.

Only a successful base from this exact cycle can be used for diagnostics. The
adapter creates `I23GrowattLink` from that success and the request's generation
and receipt. The supplied diagnostic link must match every field independently:
whole aggregate context, cycle, generation, receipt and actual success revision.
The diagnostic context must separately match the base-record receipt, source,
epoch, cycle, unit, function, all four revision fields and every raw native
continuity word. The adapter derives the checker's frozen base context from the
accepted native evidence, rather than trusting the diagnostic's claimed context.
The existing checker also validates the entire diagnostic bundle and explicit
expected reported-group word. The group word is not a selector.

`I23AggregateError`, `I23DiagnosticError` and `I23DiagnosticRejection` are separate
result fields. A failed diagnostic returns no candidate while leaving a valid
aggregate available with its original success metadata. No diagnostic from a
previous refresh is attached to a new attempt; absence means no candidate for
this refresh, not a zero measurement. The port retains no diagnostic history.
The snapshot's `I23Link` continues to describe its last successful base on a
failed refresh; it never relabels retained evidence as the attempted cycle.

## Logical time, closure and examples

Ticks are injected counters, not capture times. Base completion at its deadline
is late under the merged retention rules. Diagnostics must still be before that
same deadline when checked for publication. `I23Advance` can close pending native
work while a supplier is running and clears a published diagnostic at its
deadline without re-dating or erasing the aggregate. A backward advance fails.
The native aggregate's `current` state describes cycle membership, not freshness;
no age-based aggregate validity policy is introduced.

Retirement preserves the previously published base, marks the envelope retired,
clears diagnostics and rejects late publication and all subsequent refreshes.
Cancellation observed during the native cycle uses its existing failure path.
If cancellation is observed after native commit but before port publication,
the merged API cannot roll that commit back: the adapter permanently retires and
keeps its prior published base. A post-native regressing clock also retires the
adapter. Recreate a port under a new owner after either refusal; there is no
reset that can resurrect hidden native success. Retirement and publication are
serialized. Callers must still provide cooperative callbacks; logical ticks do
not forcibly interrupt arbitrary code.

All examples use invented logical counters and complete public synthetic inputs:

| Event | Aggregate | Optional diagnostic |
| --- | --- | --- |
| Cycle 1 succeeds at tick 1, revision 1; matching generation/receipt/context | Current native base | Complete detached candidate; outbound denied |
| Same base succeeds; diagnostic receipt, generation or revision differs | Base remains valid at its success tick | Association error; absent candidate |
| Same base succeeds; diagnostic raw continuity, group or slice is invalid | Base remains valid | Native rejection; absent candidate |
| Cycle 2 returns a partial base or supplier error at tick 2 | Retained cycle 1, tick 1, revision 1 under coherent continuity | No candidate for cycle 2 |
| Complete cycle 3 contains validated zero SOC | Atomic zero replacement; success revision advances | No inferred diagnostic or qualification |
| Owner advances pending cycle to its deadline before supplier returns | Late input cannot replace prior base | No late candidate |
| Retirement during a callback | Earlier published base remains; retired is explicit | Late candidate rejected |

## Projection loss and delivery boundary

This local interface is neither the absent common production Provider interface
nor the M02 gateway registry. It provides no active acquisition, native
qualification, registration, persistence, FC04 inverter provider or outbound
permission. MCP and GraphQL schemas are unchanged. Future bindings must preserve
equivalent aggregate applicability, attempt outcome, native evidence, success
revision/cycle/tick, diagnostic absence/rejection and retirement. Flattening a
retained base into an ordinary successful response would lose those distinctions.
eBUS MCP compatibility and G5 D1-D5/O8 remain unchanged; eeBUS output stays owned
by helianthus-eebusreg.

The focused command is `go test . -run '^TestI23' -count=1`. Documentation
acceptance is manual source, claim, transition, loss, example and link review.
Offline software tests establish no full CI, transport, smoke, passive-capture,
runtime integration or hardware acceptance.

This is a partial slice for [issue #334](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/334),
[issue #201](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/201)
and Project #2; the parent remains open. **MERGE HOLD:** I24, the
[docs-modbus](https://github.com/Project-Helianthus/helianthus-docs-modbus)
Growatt aggregate provider-port companion, must merge before or with this code.
Independent review, committed-head gates and remaining production acceptance
are outside this offline slice.
