# EMMA family provider port

This partial slice advances [#332](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/332),
[#201](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/201) and
[#199](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/199).
The parents remain open. Nothing is stable before Helianthus 1.0.

The [public native read-only contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md)
owns native predicates, encoding, target validation and limits. Its
[qualification contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/qualification-readiness-v1.md)
owns capability qualification. The
[public cycle contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/emma-offline-provider-cycle.md)
and [local implementation boundary](emma-offline-provider-cycle.md) describe
supplied FC03 evidence and diagnostic MEI separation. This port adds lifecycle
composition, with no new native mapping or acquisition claim.

## API and ownership

[Implementation](../emma_provider_port.go) defines `I19EmmaProviderPort` and its
concrete `I19EmmaProvider`. Every operation uses already merged native types;
there is no dependency on another provider-port slice.

- `I19NewEmmaProvider(current, config)` validates the initial native context at
  the supplied logical time and refuses nil supplier/clock or nonpositive
  bounds. Construction never calls the supplier. `I19EmmaProviderConfig`
  contains `I19Bounds EmmaOfflineBounds`, `I19Reader EmmaOfflineIdentityReader`
  and `I19Now func() time.Time`. These values are copied once. Replacing fields
  in the caller's configuration cannot widen bounds or replace callbacks.
  Mutable state captured by callbacks is still caller-owned; function copying
  does not freeze that state. Callbacks must cooperate with finite work,
  deadline and cancellation, and support the caller's concurrency.
- `I19Refresh(refresh context.Context, expected EmmaIdentityContext,
  inventoryRequested bool)` calls the actual `EmmaOfflineProvider.Cycle` with
  the frozen configuration. One accepted cycle calls the supplied FC03 reader
  once. Nil refresh context, retired/uninitialized port, stale context and native
  preflight rejection call it zero times. The synchronous supplier must keep
  evidence buffers unchanged until refresh returns. There is no socket,
  goroutine, background scheduler, retry, guard acquisition or inventory sender.
- `I19SetContext(current)` serializes native context replacement. The caller
  must use new attempt and connection tokens and never restore old contexts.
  Parent replacement clears foreign history. Revision/connection changes keep
  same-parent history with its original context and observation time. Invalid
  replacement contexts are fences, not admission: native preflight still
  refuses them. `I19Retire()` is terminal, marks the native current context
  cancelled and preserves history. A retired port cannot be reactivated.
- `I19Latest()` preserves the native latest-attempt result, including reason,
  FC03 diagnostic evidence, MEI outcome, inventory intent and retained history.
  It is an attempt snapshot, so its `Committed()` flag is not a current-identity
  assertion after lifecycle changes. `I19Identity()` returns a detached tuple
  only when that committed attempt still matches every current context field
  and logical time is strictly before its deadline. It is not live freshness
  or qualification proof. `I19RetainedIdentity()` separately exposes history,
  including its originating context, bytes and unchanged observation time.
  Failed attempts never fill tuple gaps from history.

Explicit refresh cancellation is sampled after each supplied clock call,
including the native completion clock before commit. Cancellation becomes the
existing native `Cancelled` context; the native commit refuses the candidate
before matching. A refresh ticket prevents an obsolete cancellation from
cancelling a replacement context or later refresh. Cancellation occurring after
the sample is a subsequent event; this port cannot atomically coordinate an
external context cancellation with native commit or interrupt a blocked reader.
Lifecycle retirement/replacement itself uses the native serialized fence.

## Transitions and evidence loss

| Supplied event | Outcome |
| --- | --- |
| Valid complete FC03, failed MEI, inventory requested | Identity may commit; diagnostic failure remains separate; inventory stays blocked and incomplete. |
| Missing/failed FC03 or supplier error, successful MEI | Identity unavailable; MEI cannot rescue FC03. Same-parent history keeps its original observation time. In-bound error evidence remains diagnostic. |
| Complete native mismatch | Explicit mismatch, no committed identity; retained history remains separate. |
| Aggregate strings and request/response exceed frozen byte bound | Native Cycle rejects and discards the oversized candidate rather than truncating evidence. History survives. |
| Parent, revision, connection, attempt or deadline changes during supplier | Native commit rejects the old completion; a newer accepted cycle cannot be overwritten by it. |
| Cancellation, retirement or deadline equality/expiry during supplier | No match/commit; no refresh of retained age. Retirement also removes current identity availability. |
| A valid replacement changes offering, family variant, firmware, revision or connection | Native inventory-association invalidation is preserved. It does not establish an inventory. Attempt/deadline changes alone do not change native identity. |

For example, synthetic parent P can commit a supplied tuple at logical T with
failed MEI and blocked requested inventory. A later failed FC03 at T+delta
leaves that tuple historical at T. If P is retired during a third callback, its
successful return is rejected. A separate parent's valid identity is unchanged.
These are supplied-record examples, not device observations.

## Evidence status and delivery boundary

| Status | Evidence and limit |
| --- | --- |
| Proven, local synthetic tests | Actual merged Cycle with fake FC03 supply; independent context and identity axes; cancellation and retirement during callbacks; late replacement; refusal without supplier calls; frozen configuration; byte bounds; detached input/output; retained age and denial flags. |
| Hypothesis | A future acquisition owner can supply validated native records and cooperative work accounting through this local port. |
| Unknown | Physical behavior, live correlation, qualified inventory guards, acquisition boundedness and future production integration. |
| Qualification | Evidence blocked independently of an identity match or retained history. |
| Admission | Default denied; automatic runtime admission, registration, live qualification, support claim and write authority remain false. |

I20, the public docs-modbus EMMA provider-port companion, must merge before or
with this code. Its reviewed API, cancellation sampling, transitions and loss
behavior must agree with this implementation before releasing the merge hold.
Manual review covers source/claim correspondence, transition ordering, loss,
examples and links; it does not establish protocol or physical acceptance.

This family-local interface is not the common production Provider interface or
the M02 gateway registry. MCP and GraphQL are unchanged; future projections must
preserve the same native envelope, reasons, age and explicit unknowns. eBUS MCP
compatibility and G5 D1-D5/O8 remain unchanged; eeBUS output remains owned by
helianthus-eebusreg. No native qualification, registration, runtime integration,
transport, smoke, hardware or physical acceptance is claimed. Independent
review, committed-head gates, publication audit, I20 comparison/merge and the
remaining parent integration and qualification work are outstanding.
