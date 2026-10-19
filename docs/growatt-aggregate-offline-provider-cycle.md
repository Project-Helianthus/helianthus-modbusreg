# Growatt aggregate offline provider cycle

This family-local seam composes the existing aggregate retention component over
caller-supplied offline native records. It is a partial implementation for
[issue #322](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/322)
and Project #2. Nothing is stable before Helianthus 1.0.

The public [native family contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/growatt/bms-rs485-1xsxxp-v202.md)
remains protocol authority. The [retention contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/growatt-battery-group-retention.md)
and [retention implementation contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/growatt-aggregate-cycle-retention-implementation.md)
bound this composition. No protocol fact, native read extent or admission rule
is added here.

The independently authored [A31 public companion](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/growatt-aggregate-offline-provider-cycle.md)
must merge before or with this code. Merge remains on hold until review compares
its actual API signatures and results with the implementation below. This link
names the required destination; its availability has not been verified offline.

| Evidence status | Boundary |
| --- | --- |
| Proven | Source and synthetic tests demonstrate one reader invocation per accepted cycle, context fencing, bounded input acceptance, cancellation fencing, atomic base replacement and retained last-success metadata. |
| Hypothesis | A future composition owner can supply coherent identity, a cooperative reader and a consistent logical clock. |
| Unknown | Hardware continuity, physical values, measurement simultaneity, wall-clock freshness, concrete persistence and gateway integration. |

Qualification, freshness and admission remain separate from these evidence
statuses. A successful offline cycle grants no acquisition or outbound authority.

## Local API

Implementation: [provider](../growatt_aggregate_offline_provider.go),
[tests](../growatt_aggregate_offline_provider_test.go),
[retention owner](../growatt_aggregate_retention.go).

```go
type GrowattAggregateOfflineCycle struct {
    Context GrowattAggregateContext
    Cycle, Deadline uint64
}
type GrowattAggregateOfflineReader func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error)
func NewGrowattAggregateOfflineProvider(maxCycleTicks, maxSlices, maxWords uint64, reader GrowattAggregateOfflineReader) (*GrowattAggregateOfflineProvider, error)
func (p *GrowattAggregateOfflineProvider) Cycle(ctx context.Context, cycle GrowattAggregateOfflineCycle, clock func() uint64) error
func (p *GrowattAggregateOfflineProvider) Advance(now uint64) error
func (p *GrowattAggregateOfflineProvider) Snapshot() GrowattAggregateSnapshot
```

Construction requires positive explicit bounds and a nonnil reader. `maxWords`
bounds the total words across all returned slices, using subtraction without
overflow. Bounds limit accepted input and processing; they cannot prevent an
arbitrary callback allocating or blocking internally. The synchronous reader and
clock must themselves honor caller work bounds. The reader receives cancellation
and the complete cycle/deadline by value. The wrapper creates no goroutine or
timer to interrupt it. Logical deadlines require the caller to drive `Advance`
while a callback is blocked or allow `Cycle` to observe expiry on return.

`Cycle` requires a nonnil context and clock, then delegates `Begin` validation
to retention before reader invocation. Unsupported unit/function/revision and
unknown identity close the newly begun attempt through `Fail`, without calling
the reader. Malformed local cycle/deadline or bounded-context input rejects at
`Begin`. Context cancellation observed before the reader prevents invocation.
This distinction preserves invalidation on changed or unknown native continuity.

The reader runs once. After return, the wrapper reads logical time, checks
cancellation, handles reader error through `Fail`, checks input bounds, and
calls `Complete` only for a bounded error-free candidate. Reader error accompanied
by plausible records never commits those records. Joined errors preserve both
the reader/cancellation error and any retention rejection. Decode failures retain
the existing retention component's native contradiction and atomicity behavior.

`Context` includes the opaque source token, owner epoch, selected unit, function,
every revision component, `IdentityKnown`, MCU software version, BMS and pack
company/generation, topology and cell-series count. Known zero is distinct from
unknown. These are continuity claims supplied by the caller, checked against
complete native evidence by retention. Local cycle and deadline correlate an
attempt; they are not native identity or evidence of a physical capture time.

There is one last-good aggregate and one latest attempt, both inside retention.
The provider stores no second value, ledger, diagnostic bank or error history.
Snapshots use the existing detached deterministic envelope. Callers must not
mutate returned reader slices until `Cycle` returns; later mutations cannot
change accepted storage. Concurrent cycles can supersede each other. Readers
and clocks must support any concurrency their caller permits.

## Synthetic transitions and loss

| Supplied event | Result |
| --- | --- |
| First complete coherent base | Current aggregate, replacement revision advances, original native slices retained. |
| Partial base or reader error after success | Earlier aggregate retained with unchanged success tick/revision; latest attempt failed. No field merge. |
| Valid zero in a complete base | Replaces the previous nonzero value atomically. |
| Completion at or after deadline | Incomplete attempt; cannot renew last-success age. |
| Cancellation observed after reader return | Failure closes the active attempt; supplied records cannot commit. |
| Newer cycle or epoch supersedes blocked work | Old callback rejected; newer success cannot be overwritten. |
| Changed context or unknown identity | Prior applicability invalidated; only a new complete coherent success restores it. |
| FC04, diagnostic ranges, empty or malformed records | No aggregate admission widening; no partial replacement. |

`current` means success in the latest cycle, not fresh physical telemetry.
`retained` carries the original success metadata; `absent` has no successful
value, and `invalidated` exposes no applicable value. Failed reads cannot
refresh success time or present retained data as a new native-success response.
Opaque words and native provenance remain available through the retained native
observation without deriving diagnostic or universal facts.

## Integration boundary and remaining acceptance

This is not the future gateway Provider registry M02. It creates no global
provider API, transport adapter, RTU session, acquisition owner, registration,
retry, diagnostic schedule, FC04 inverter admission or automatic admission.
There are no MCP or GraphQL bindings here. Future consumers must preserve parity
for presence, versions, attempt outcomes and native evidence, with explicit
projection loss. Existing eBUS MCP compatibility is unchanged; eeBUS output
remains driver-owned.

Manual documentation acceptance covers source signatures, claims, transition and
loss tables, synthetic examples and link destinations. Full committed-head gates,
independent review and the A31 comparison remain required. Offline tests establish
no transport, passive-capture, runtime, smoke or hardware acceptance. Registration,
persistence, live qualification and the remainder of Project #2 remain outside
this partial slice.
