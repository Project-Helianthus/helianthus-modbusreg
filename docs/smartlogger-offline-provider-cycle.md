# SmartLogger offline provider cycle

This family-local application seam composes the existing
[inventory commit owner](smartlogger-inventory-commit.md) over supplied native
records. The public [native provider lifecycle contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/smartlogger-provider-lifecycle-v1.md)
and its linked sources own protocol facts; this page adds no wire interpretation.
Nothing in this API is stable before Helianthus 1.0.

## Explicit API and caller obligations

`NewSmartloggerOfflineProvider(identity SmartloggerInventoryContext,
clock func() time.Time, reader SmartloggerOfflineInventoryReader)
(*SmartloggerOfflineProvider, error)` validates the existing native identity
context and requires a nonnil callback. A nil clock uses `time.Now`. Injected
clocks retain the commit owner's monotonic, nonblocking contract. Do not copy
providers or owner handles. The zero provider refuses work.

`SmartloggerOfflineInventoryReader` has signature
`func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error)`.
It receives a value copy of the frozen plan and returns one already-read native
counter/page bracket, or an error. It cannot return replacement correlation or
change the owner's plan. It must use only bounded offline work, finish within
the plan deadline, and obey the caller's supplied input ceilings. It must not
perform transport I/O. The caller must not mutate returned slices concurrently
with the cycle. Errors are returned unchanged and must be appropriate to retain
at the caller boundary.

`Cycle(ctx context.Context, deadline time.Time,
limits SmartloggerOfflineInventoryLimits) (SmartloggerInventoryResult, error)`
performs one explicit cycle. There is no polling, retry, queue or background
acquisition. A valid request invokes the callback once. Nil/cancelled contexts,
invalid limits, invalid deadlines, closed owners and busy attempts refuse work
without invoking it. Accepted attempts carry a nonzero attempt token and an
immutable terminal result; refusals return a zero result and error.

All three integer limits must be positive: `MaxPages`, `MaxObjects`, `MaxBytes`.
They narrow the existing parser ceilings. `MaxObjects` counts every object,
including the count object; `MaxBytes` counts object values plus two framing
bytes per object, matching parser accounting. Oversized input fails before
parsing or cloning. These checks bound validation of returned input; they cannot
prevent a callback from allocating excessive memory or blocking internally.
The caller is responsible for the callback's finite work and allocations.

The owner bounds the absolute deadline using its existing begin rules. An
earlier context deadline narrows the plan. Cancellation is checked before begin
and again after the callback returns; the latter invokes the existing attempt's
`Cancel`. Context expiration and callback errors fail without committing the
returned pages. The synchronous callback cannot be forcibly interrupted, and
cancellation does not make a blocked callback return. Close and retirement can
fence publication while that callback is blocked. There is no scheduler or
transport cancellation guarantee in this API.

After the callback, the cycle checks context cancellation/expiration and input
bounds, then calls the existing `Commit` with the original plan. Commit compares
OwnerToken, ConnectionGeneration, RoutingContext, UnitID, ParentProfile,
SourceRevision, SelfModel, Firmware, AttemptToken and Deadline and validates
native pages and the deadline at publication. Native identity fields remain
caller-supplied correlation, distinct from local owner/generation/attempt tokens
and deadline; neither category proves live identity or admission.

`Close()` delegates permanent closure to the owner. `Retire(identity
SmartloggerInventoryContext) error` requires a strictly newer valid generation.
`LastComplete() (SmartloggerInventorySnapshot, bool)` returns detached native
pages and parsed inventory, or false when no complete inventory exists.

## Results, history and projection loss

Only a complete valid bracket replaces inventory. Reader errors, invalid or
incomplete pages, wrong continuation cursors, late continuation flags, exceeded
input limits and expired deadlines retain the original complete inventory and
both its observation and completion markers. Age continues from the original
`ObservedAt`. No partial children are promoted. Cancellation produces
`cancelled`; close and retirement preserve their existing `closed` and `retired`
terminal fences against late callback completion. A late completion cannot
release a successor's slot. A completed success is not rewritten by later
closure or cancellation.

For example, a synthetic complete bracket at counter 12 survives a later
callback that returns valid-looking pages and an error. It also survives a later
page whose continuation claims more data without supplying it. Neither failure
changes its observation marker. If retirement occurs while the callback is
blocked, that callback's eventual valid pages cannot become the new generation's
inventory. Retained history then has `Current=false` until a new complete cycle.

Every result and parsed inventory remains `DefaultDenied() == true`.
`Current` is only an open-owner context match. Errors and failed outcomes are
coarse; there is no per-stage diagnostic evidence store. Successful native
counter/page evidence is retained without semantic projection. Consumers must
account for any later omission separately. Public fixtures are synthetic;
runtime native retention is a different boundary from public redistribution.

## Evidence and delivery boundary

| Evidence label | Claim and limit |
| --- | --- |
| Proven | Focused synthetic tests exercise callback invocation, bounds, detached pages, retained history, independent plan-field fences, cancellation, close and generation retirement. |
| Hypothesis | A future acquisition owner can supply records with independently established connected identity and transport provenance. |
| Unknown | Live identity, acquisition timing, transport isolation, qualification, hardware acceptance and end-to-end consumer replay. |

These evidence labels are independent of qualification, freshness and admission.
This is a family-local offline seam, not the future gateway Provider registry
M02. It adds no global provider API, transport adapter, acquisition owner,
persistence, automatic admission, child capability promotion or authorization
gate. Existing eBUS MCP compatibility, GraphQL parity and G5 D1-D5/O8 are
unchanged. eeBUS output remains driver-owned.

The independently authored public companion
[architecture/smartlogger-offline-provider-cycle.md](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/smartlogger-offline-provider-cycle.md)
(A21) must merge before or with this code. Delivery remains on **MERGE HOLD**
until reviewed actual signatures, outcome rules, input accounting, cancellation
limits and evidence claims are compared with that companion. This link is a
required publication target, not evidence that it has merged. Application
composition belongs in architecture documentation, outside native protocol pages.
This partial slice addresses issue #317 and advances #206 and #201; both parents
remain open. Future registration, concrete persistence and live qualification
remain outside this slice.
