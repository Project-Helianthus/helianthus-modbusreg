# EMMA offline identity transaction

This native component advances #299 as a partial slice of #201 and #199.
Nothing is stable before Helianthus 1.0.

The [canonical provider transaction contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/emma-provider-transactions.md)
defines the evidence boundary. Native predicates remain owned by the linked
[Huawei read-only contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md)
and [qualification contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/qualification-readiness-v1.md).
This page adds application API and lifecycle guidance, no native protocol facts.

## API and ownership

`NewEmmaIdentityAttempt(context, input, mei, inventoryRequested)` detaches the
candidate. `EmmaIdentityContext` freezes ParentSource, SourceRevision,
ConnectionGeneration, Attempt, Deadline and cancellation. Source keys are opaque
correlation metadata. The caller supplies one complete, already validated FC03
tuple, its native request/response evidence and an explicit acquisition status.
It must validate native target, extents and encoding and must never assemble a
tuple from multiple attempts. The component evaluates the decoded fields with
`EvaluateHuaweiEMMAOfflineIdentity`; it is not a wire decoder or acquisition
validator. Missing and failed input cannot be rescued by successful MEI.

`attempt.Commit(current, now, previous)` is a pure value transition. The owning
coordinator must serialize obtaining `current`, calling Commit and publishing
its result against changes to that parent's context. This API owns no lock or
shared store and cannot independently guarantee publication atomicity. A zero
context/attempt, cancellation, absent time, deadline expiry (including equality)
or difference in any of the five context axes rejects replacement. Deadlines
are compared as instants. The caller supplies time; there is no clock read.

`IdentityMatch` describes a validated match eligible in the current fence.
`Committed` says whether this call replaced retained identity. `Reason`
distinguishes context rejection, unavailable FC03, a complete mismatch and
identity commit. An unavailable or rejected result is not proof of a native
mismatch. `AttemptInput` and `AttemptContext` preserve diagnostic evidence even
on failure. `RetainedIdentity` separately returns history with its original
context, raw evidence and `ObservedAt` commit time. Returned byte slices are
copies. A later failure cannot refresh history or turn it into a current match.
The caller must distinguish retained history from current availability.

`MEIStatus` records optional diagnostic success/failure independently. Inventory
is `not_requested` when omitted and `blocked` when requested; it is never
complete. No guard-qualified flag, page input, guard read or inventory parser
is accepted here. Successful MEI does not establish a qualified guard procedure.
`InventoryAssociationInvalidated` signals retirement of an earlier association
when offering, family variant, firmware, parent source, source revision or
connection generation changes on a successful replacement. Attempt/deadline
changes alone do not change identity. No inventory association or child data
is stored or invented by this slice; later composition must consume the signal.

For example, synthetic parent P/revision R/connection 1/attempt 2 can commit a
complete matching tuple while optional MEI fails and requested inventory stays
blocked. A missing tuple on attempt 3 retains attempt 2's original context and
time but reports no current match. A late attempt 2 completion against attempt
3 is rejected. A successful changed tuple retires the prior association.

## Evidence, qualification and remaining acceptance

| Axis | Boundary |
| --- | --- |
| Proven | Synthetic focused tests establish detached values, independent optional status, context fencing and last valid identity retention. This is offline implementation evidence. |
| Hypothesis | A later coordinator can use this pure transition within its atomic per-parent publication boundary. Integration is not implemented or tested here. |
| Unknown | Qualified inventory-guard acquisition, live correlation, capabilities and hardware behavior. |
| Qualification | Always evidence blocked, independently of a local identity match. |
| Admission | Default denied; catalog registration, automatic admission, live qualification, support claim and write authority remain false. |

No registration, transport, scheduling, retry, gateway integration, consumer
binding or authorization gate is added. MCP and GraphQL are unchanged; later
composition must expose the same committed envelope and freshness. eBUS MCP
compatibility remains unchanged and eeBUS output binding stays driver-owned.

Merge is held until the docs-modbus companion
`architecture/emma-identity-transaction-implementation.md` describes the reviewed
API and boundary, cites the canonical provider transaction contract, and merges
before or with this change. That companion belongs outside `protocols/` and
`types/`. Remaining parent acceptance includes coordinator serialization,
lifecycle/race validation, bounded acquisition/retries, qualified inventory
acquisition, capability/negative-overlap evidence, transport replay, consumer
parity, independent review and separate hardware acceptance. Neither parent
closes with this offline slice.
