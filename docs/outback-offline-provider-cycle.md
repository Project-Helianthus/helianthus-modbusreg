# OutBack offline provider cycle

This family-local holder imports caller-supplied opaque OutBack AXS Port family
records. It is a partial contribution to [issue #201](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/201),
tracked by [issue #321](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/321).
Nothing here is stable before Helianthus 1.0.

## Native authority and public companion

The public [observation lifecycle](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/outback-observation-lifecycle.md)
and [detached-record implementation boundary](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/outback-detached-record-implementation.md)
precede this implementation. Native protocol claims remain owned by the
[source/profile matrix](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-source-profile-matrix-v1.md)
and [native read-only boundary](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-sunspec-readonly-v1.md).
This holder adds no protocol facts or read-operation contract.

**Merge hold:** the independently authored A29
[public offline-cycle companion](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/outback-offline-provider-cycle.md)
must merge before or with this code. That proposed page is a delivery dependency,
not evidence of an already published contract. Reviewed actual signatures,
results and transitions must be compared with it before releasing the hold.

**Proven by local synthetic tests:** bounded record import, detached evidence,
original association, one retained slot, independent source-axis rejection,
local generation/attempt fencing, cancellation fencing and last-complete-record
preservation. Complete and incomplete records remain `NO_GO`, with no typed
facts or acquisition requests. These are offline component claims.

**Hypothesis:** a future composition could use this family-local seam as a
diagnostic input. It does not implement the future gateway Provider registry M02.

**Unknown:** unique public applicability, a qualified read operation, physical
behavior, integration and persistence. Qualification, freshness and admission
are independent of structural completeness, association and receipt time.

## API and finite bounds

Implementation: [provider](../outback_offline_provider.go),
[synthetic tests](../outback_offline_provider_test.go), and
[detached-record contract](outback-detached-record.md).

```go
type OutBackDiagnosticSupplier func(context.Context, OutBackOfflineAttempt) (OutBackDetachedRecordInput, error)
func NewOutBackOfflineProvider(source OutBackSourceAssociation, maxBytes uint64, maxWork time.Duration, supplier OutBackDiagnosticSupplier) (*OutBackOfflineProvider, error)
func (*OutBackOfflineProvider) Cycle(ctx context.Context, receivedAt time.Time) error
func (*OutBackOfflineProvider) Replace(source OutBackSourceAssociation) error
func (*OutBackOfflineProvider) Retire()
func (*OutBackOfflineProvider) Snapshot(current OutBackSourceAssociation) OutBackOfflineSnapshot
```

Construction requires positive practical `maxBytes`, positive finite `maxWork`
and a nonnil supplier. Source metadata is checked using the existing
`NewOutBackDetachedRecord` accounting and detached before storage. Zero work,
negative work, missing callback or unbounded/oversize metadata is rejected
without invoking the supplier. Record/source byte failures return
`ErrOutBackDetachedRecordBound`; invalid configuration returns
`ErrOutBackOfflineInput`. There is no default word count or inferred read plan.

`Cycle` rejects nil context, zero receipt time, already canceled context, a zero
holder, exhausted attempt counter or retired holder before invoking a callback.
A valid call synchronously invokes the supplier once with a deadline bounded by
`maxWork` and any earlier parent deadline. There are no retries or background
workers. The supplier must honor that context and bound its own work and
allocations; the holder cannot preempt arbitrary callback code or bound memory
allocated inside it. Callers must serialize or bound concurrent invocations and
must not mutate returned buffers until `Cycle` returns. The byte bound checks
returned evidence before detachment; it is deterministic evidence accounting,
not exact heap accounting. During replacement a candidate and retained record
can coexist transiently, each subject to the same bound.

`OutBackOfflineAttempt` contains `Generation`, `Number` and `Source`. Local
counters start with generation one and attempt zero; valid cycles increment the
attempt number. These counters are publication context, not native identity,
source revisions or qualification. Source key, revision, public locator, content
identity, declared layout, source-set revision, selector inputs and availability
are frozen as supplied. A supplier result must match every frozen source field,
including availability; mismatch returns `ErrOutBackOfflineAssociation`.
Unavailable declarations can be retained diagnostically but are never current.

`Replace` byte-checks a new source before any change, advances generation even
for equal native metadata, cancels outstanding work and sets latest status to
`idle`. The last actual `LatestAttempt` remains frozen until a new cycle
starts; replacement does not invent or relabel an attempt. Exhausted generation returns `ErrOutBackOfflineInput` without a change.
`Retire` permanently closes the holder and cancels outstanding work. New cycles
and replacements then return `ErrOutBackOfflineRetired`. Counters never wrap.
No mutation changes the retained record's original association.

## Publication, retention and loss

`OutBackOfflineSnapshot` exposes immutable `Record`, caller-supplied `RetainedAt`,
`RetainedAttempt`, `LatestAttempt`, `Status` and `Association`. Mutable record
accessors return detached copies. Receipt time is opaque caller metadata; no
age threshold, ordering, TTL or freshness verdict is inferred from it.

| Transition | Retained slot and latest status |
| --- | --- |
| Valid cycle begins | Freeze local attempt/source; status `running`; cancel the preceding attempt. |
| Complete bounded result, matching source | Replace slot atomically; set original receipt time/attempt; status `imported`. |
| Incomplete or malformed bounded result with no complete slot | Retain exact diagnostic words, duplicate occurrences and extents without repair, padding or deduplication; status `incomplete`. |
| Incomplete or malformed bounded result with a complete slot | Preserve the complete slot, its original receipt time and retained attempt; status `incomplete`. The candidate diagnostic fragment is not retained in a second slot. |
| Supplier error, oversize or source mismatch | Return error, status `failed`; preserve retained record, receipt time and retained attempt. No partial result commits. |
| Cancellation/deadline before publication | Return context error, status `canceled`; preserve retained evidence and age. Recheck cancellation after the callback and after copying. |
| Newer attempt, replacement or retirement overtakes a callback | Return `ErrOutBackOfflineSuperseded`; discard its result without touching the newer status or retained slot. |
| Replacement / retirement | Preserve original record and receipt; latest status becomes `idle` / `retired`. Retirement is irreversible. |

`Snapshot(current)` reports `current` only if the caller descriptor matches the
holder's current source, the retained original descriptor matches every field,
both are available and the holder is not retired. Changed, missing, conflicting
or unavailable association yields `historical` (including unassociated evidence).
A caller cannot restore current association by passing the old descriptor after
replacement. Equal source metadata across local generations can still describe
the retained native source; generation equality separately fences publication.

For example, import a complete synthetic record under revision `one`, then
replace the source with revision `two`. The old record keeps revision `one` and
its original receipt time. A late revision-one callback is discarded. An
incomplete revision-two candidate updates status but preserves that old complete
record as historical. Only a later complete matching candidate replaces it.
A supplier error never renews its age.

## Admission and remaining acceptance

Inspect `Snapshot(...).Record` using the existing nil-safe `Evaluate()`,
`TypedFacts()` and `Requests()`. Every path remains `NO_GO` with empty typed facts
and requests. Structural completeness does not choose a layout or admit a
profile. This holder is no Modbus reader, transport adapter, acquisition owner,
global provider API, scheduler, registration mechanism or automatic admission.
There is no concrete persistence, eviction policy or interpreted view.

No MCP or GraphQL fields change; eBUS MCP compatibility and GraphQL parity
remain existing consumer obligations. eeBUS output stays driver-owned. No
consumer parity, G5 integration, transport conformance, runtime, passive device
observation or hardware acceptance is claimed. No authentication or authorization
gate is introduced. The parent issue stays open.

Focused verification is `go test . -run '^TestOutbackOfflineProvider' -count=1`.
Tests use synthetic records, barriers, invocation spies and logical receipt
or expiry values. Offline evidence is not physical qualification. Independent
review, the public companion comparison and complete committed-head gates remain
required before merge.
