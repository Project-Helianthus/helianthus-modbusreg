# S-Dongle offline provider cycle

This family-local API composes supplied native records with the merged
[`DecideSDongleRoute`](sdongle-route-decision.md) component. Nothing is stable
before Helianthus 1.0. This is a partial contribution to provider issue
[#201](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/201).

The public [native gateway contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md)
is the protocol authority. The [routing architecture](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/sdongle-provider-routing.md)
and [route decision boundary](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/sdongle-route-decision-implementation.md)
constrain this application API. This page introduces no protocol facts, tuple
extensions or device qualification. The linked public
[offline cycle companion](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/sdongle-offline-provider-cycle.md)
must merge before or with this code; its publication and comparison against the
actual reviewed API remain a merge hold.

## Local API

```go
type SDongleOfflineLookup func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error)

func RunSDongleOfflineProvider(
    ctx context.Context,
    req SDongleOfflineRequest,
    lookup SDongleOfflineLookup,
) (SDongleOfflineResult, error)
```

`SDongleOfflineRequest` contains `Key`, `Bounds`, `AttemptedAt` and optional
`Previous *SDongleOfflineRecord`. The record contains `Decision` and
`EvaluatedAt`. These times describe local evaluation attempts only; neither is
native identity, a capture time, source age or a freshness assertion.

`SDongleOfflineBounds` contains `LookupTimeout time.Duration`,
`MaxInputBytes uint32` and `MaxReferences uint16`. A nonnil context and callback,
nonzero attempt time, positive timeout at most 15 seconds, positive byte limit
at most 1048576 and positive reference count limit at most 256 are required.
The selected key must have nonempty family, model, firmware, role, request,
scope and revision. Semantic validity remains the route evaluator's concern.
Input validation fails before lookup. Invalid previous records also fail before
lookup: previous outcome must be `planned`, evaluation time nonzero, and its
strings and references must fit the caller's bounds. Previous records are
caller-supplied history, not verified provenance or admission credentials.

The byte budget counts the sum of string byte lengths, independently for the
requested key, returned plan and optional previous decision. It includes both
plan/evidence keys (including parent and child strings), observation model and
firmware, all four reference pairs, additional references and, for a previous
decision, outcome and reasons. Fixed-size numeric fields need no variable-size
budget. The reference limit bounds `Evidence.References` entries before copying
or traversal. These are local memory/work limits, separate from the plan's
native documentary deadline, retry, page, object and byte limits; the latter
remain unchanged and are evaluated by `DecideSDongleRoute`.

The callback reads already-supplied offline records and is invoked once, with
the exact requested key and a derived context bounded by both the parent
deadline and `LookupTimeout`. It must cooperate with that context and honor the
caller's record limits. This synchronous seam cannot forcibly interrupt a
callback or prevent its own allocation; it creates no goroutine or background
work. Cancellation/deadline is checked before lookup, immediately after return
and before committing the evaluated result. Late success is rejected. No
retry, alternate lookup, fallback scan or live acquisition is available.

## Identity and result transitions

Before evaluation, the returned `Plan.Key` must equal every requested field:
Family, Model, Firmware, ProtocolMajor, ProtocolMinor, Role, Unit, Request,
Scope and Revision; all ten corresponding `Child.Parent` selection fields;
and Child.EvidenceRevision, Qualification, GuardBefore and GuardAfter.
`Evidence.Key` is then independently checked by the merged evaluator.
Native identity and child context are separate from local attempt time and
lookup bounds. Gateway-self and child evidence cannot substitute for each
other. No source revision is treated as a firmware version.

| Result | Meaning | History |
| --- | --- | --- |
| `ready` | Evaluator returned `planned`: documentary route readiness only. | Replace history with a detached complete decision and this local evaluation time. |
| `denied` | Evaluator returned `no-send`. | Keep the prior record and original evaluation time. |
| `blocked` | Evaluator returned `withheld`, or validation, lookup, association or cancellation failed. | Keep valid detached prior history; never merge a failed/partial attempt into it. Invalid prior input is rejected without retention. |

`SDongleOfflineResult` contains `State`, optional `Decision` and optional
`Retained`. An evaluated denial or block includes its detached native evidence
and evaluator reason. An error returns no current `Decision`.
`ErrSDongleOfflineInput` reports invalid/excessive input;
`ErrSDongleOfflineAssociation` reports a returned key mismatch. Callback errors
are preserved; an expired or canceled work context takes precedence. Callers
must inspect errors; a retained success never converts failure into readiness.
`DefaultDenied()` is always true, including on a zero result.

Retained history preserves its original key, native fields, evidence references,
caller `Current` assertion, reasons and `EvaluatedAt`. That assertion describes
the historical evaluation only; retention is no assertion that it is current
for this attempt. Source age is not renewed. Evidence acquisition and freshness
remain external responsibilities. Independent prior fields are not overwritten
by a partial failed lookup. The API retains complete records rather than
inventing field-level qualification from partial observations. Results,
retained history and caller slices are detached from each other; callers must
not mutate callback storage concurrently while the call is reading it.

For example, a synthetic gateway record evaluated at logical time 10 remains
under its original gateway key and time when a child lookup at logical time 100
fails. A matching separately evidenced child plan may be documentary `ready`;
a changed guard, stale evidence or conflicting candidate blocks it. A stopped
qualification or explicitly prohibited request is `denied`. These examples
assert no actual qualified installation or successful physical observation.

## Evidence and integration boundary

| Evidence status | Scope | Qualification, freshness and admission |
| --- | --- | --- |
| Proven | Focused synthetic tests exercise key association, one lookup, fences, detached results, controls and retained history. | Offline acceptance only; no physical qualification or acquired freshness. |
| Hypothesis | Future composition can consume the native result and history. | Concrete persistence and runtime lifecycle require separate work. |
| Unknown | Actual child support, live behavior and current evidence at an installation. | Missing evidence remains blocked; no admission follows. |

This is not the future gateway Provider registry M02. It adds no global provider
API, transport adapter, request encoding, send callback, acquisition ownership,
automatic registration or admission. It establishes no universal semantics or
projection; no native fields are intentionally discarded. Existing native trust
and admission, eBUS MCP compatibility, GraphQL parity and gateway composition
remain unchanged. No MCP/GraphQL integration, runtime, transport, smoke,
passive-device or hardware acceptance is claimed. Full committed gates and
independent review remain separate, as does the A25 public companion signature
and result comparison required before releasing the merge hold.
