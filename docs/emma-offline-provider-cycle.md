# EMMA family offline provider cycle

This is a partial implementation of [#318](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/318), advancing #201 and #199 without closing either parent. Nothing is stable before Helianthus 1.0.

The [public native transaction contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/emma-provider-transactions.md) and its linked Huawei native read-only and qualification contracts own native predicates, target validation, encoding and limits. The [identity implementation boundary](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/emma-identity-transaction-implementation.md) describes the merged component reused here. This page adds application composition only, with no new register or probing rule.

The [public offline cycle companion](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/emma-offline-provider-cycle.md) describes the detached identity seam. The read-only site cycle below uses a separate, caller-supplied session; it does not turn an offline identity snapshot into admission.

## Local API and ownership

[Implementation](../huawei/emma_offline_provider.go) composes `NewEmmaIdentityAttempt` and `Commit` from the [existing transaction](../huawei/emma_identity_transaction.go).

- `NewEmmaOfflineProvider(current EmmaIdentityContext) *EmmaOfflineProvider` creates one local publication owner, retaining one identity and the latest published attempt. It is not the future gateway Provider registry M02 or a global provider API.
- `SetContext(current EmmaIdentityContext)` serializes context replacement against publication. Caller-owned opaque parent source, revision, connection generation, attempt, deadline and cancellation identify local work, not native identity. The owner must use new generations/attempt tokens for replacement and reconnect, and must not restore an earlier context to make old work eligible. A parent change clears this provider's history; same-parent revision/connection changes keep history separately labelled.
- `Cycle(context EmmaIdentityContext, bounds EmmaOfflineBounds, inventoryRequested bool, reader EmmaOfflineIdentityReader, now func() time.Time) (EmmaIdentityResult, error)` validates the request against current context, invokes the reader once outside the lock, freezes its result and rechecks context and time during serialized commit. A later accepted cycle supersedes an earlier in-flight cycle even if the caller reuses the context. Rejected older completions return their diagnostic result but cannot replace the latest published attempt.
- `EmmaOfflineBounds` requires positive `MaxReaderWork uint32` and `MaxInputBytes int`. The byte ceiling covers offering, family variant, firmware, request and response together. Zero/negative byte limits, zero work allowance, nil reader/clock, invalid context, cancellation, missing time, expiry or a stale fence fail before the reader. Preflight failure publishes nothing.
- `EmmaOfflineIdentityReader` has signature `func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error)`. It supplies one already validated FC03 tuple, raw native request/response evidence, decoded fields, acquisition status and independent MEI diagnostics. Target, extents and encoding validation stay with the supplier. No tuple component may be borrowed from another attempt. Buffers must remain unchanged until `Cycle` returns; the component then owns detached copies.
- `now` supplies caller logical time before and after the reader; no clock is read internally. The reader must cooperatively honor the work allowance and deadline. This synchronous seam cannot interrupt an arbitrary blocked callback, enforce its internal allocations or validate how it spent its work allowance. It starts no goroutine, transport or retry. Missing finite bounds never imply defaults.
- `Latest() EmmaIdentityResult` returns the latest published attempt. Public input/history accessors return detached byte slices. These are snapshots of attempts, not fresh availability assertions after subsequent `SetContext` calls. The caller must compare originating context and observation age; this seam provides no automatic admission.

## Outcomes, native evidence and explicit loss

| Input or transition | Result and retained evidence |
| --- | --- |
| Complete validated matching FC03, MEI fails | Identity can commit; MEI failure stays diagnostic. |
| Missing/failed FC03, MEI succeeds | No match or commit; same-parent last-known-good identity retains its original context and observation time. |
| Reader returns an error with an otherwise matching tuple | Status becomes failed; in-bound supplied native bytes and decoded fields remain diagnostic. The returned error is preserved; identity cannot commit. |
| Returned aggregate bytes exceed the caller ceiling | Error and failed input; the oversized candidate is discarded entirely, never truncated or fabricated. Retained identity survives. No oversized raw evidence is published. |
| Complete nonmatching tuple | Identity mismatch, with diagnostic supplied evidence and unchanged history. |
| Cancel, deadline expiry, disconnect generation, revision or attempt replacement during reader | Post-reader commit rejects the candidate. A rejected completion cannot refresh retained age or overwrite a newer published attempt. No current match is inferred from history. |
| Successful changed offering, family variant, firmware, source revision or connection generation | The merged component signals invalidation of the prior inventory association. Attempt/deadline changes alone are local work changes, not native identity changes. No inventory association or child records are stored here. |
| Parent replacement | Separate history: the previous parent's retained identity does not migrate to the new parent. |
| Inventory omitted/requested | `not_requested`/`blocked`; never complete. There is no inventory reader, parser call, guard probe or guard-qualified input. Guard acquisition remains Unknown. |

For example, synthetic parent P at revision R can commit a complete supplied tuple while optional MEI fails and requested inventory stays blocked. Attempt B with missing FC03 retains A's original observation but has no current match. A blocked A completion after B starts cannot replace B. These examples describe local supplied records, not device acquisition.

## Read-only site cycle

[`NewEMMAReadOnlyProvider`](../huawei/huawei_emma_readonly_provider.go) uses the shared read-only session lifecycle. Each connection starts with the unit 0 identity tuple. Extended inventory routes listed and allowed inverter units only; it does not establish child identity or give the listing a guard. Failed inventory withholds children but not admitted site reads; the next cycle retries inventory without rereading the gateway tuple. A conflicting gateway identity, including a conflict found on an inventory retry, withholds monitoring. Each routed inverter must match its listed model before catalogue selection. A child without a row keeps the status, string and energy groups by their native offsets, with no typed inverter facts. An identifier conflict does not take that native fallback. After reconnect, a transient child identity read failure leaves prior groups stale at their original epochs; a definitive refusal keeps them only as fenced history.

The three site reads are FC03 at unit 0. `ProviderCycle.EMMASite` carries the words and per-group outcome, cycle, connection generation and read time; each `EMMASiteFact` carries the native key, source register, type, unit and availability. The coefficient and exponent are exact, including for a `U64` counter above the signed 64-bit range.

| Read | Native fields |
| --- | --- |
| 30306 / 46 | Day and total battery charge (30306/30308), battery discharge (30312/30314), household consumption (30324/30326), grid export (30330/30332), grid import (30336/30338) and PV energy (30346/30348). Day counters are `U32`, totals `U64`, in kWh with gain 100. The same read carries rated storage capacity at 30322 (`U32`, kWh, gain 1000). |
| 30354 / 12 | Site PV and consumption (30354/30356, `U32`), grid exchange, battery and inverter AC power (30358/30360/30364, `I32`), in kW with gain 1000. Signed values retain their native polarity; no flow direction is inferred. |
| 30368 / 1 | Battery state of charge (`U16`, %, gain 100). |

`EMMASiteFact("gateway-self.30324")` and `EMMASiteFact("gateway-self.30326")` are separate readings from the same energy block. The daily counter may restart on the operating-day boundary without changing how the total is decoded. Neither counter is synthesized from the other. A malformed or unavailable group contributes no new facts: last-known-good words retain their original epoch and are marked retained; without them the affected facts are Unknown. Other groups can still answer.

[The disposition](../profiles/vendors/huawei/emma-disposition.json) declares monitoring by contract for every declared family. Without documented inverter-side battery fields, battery readings on this path are site-level only; site readings do not establish per-inverter or per-pack values. No meter or derived-polarity operation is added. Scripted tests prove the read plan and decoding, not hardware behavior.

## Evidence and remaining acceptance of the detached seam

| Axis | Offline identity boundary |
| --- | --- |
| Proven | Synthetic focused tests exercise the local callback invocation, independent context/identity comparisons, post-reader cancellation and expiry, replacement barriers, error refusal, bounded evidence, detached slices and retained age. Mutation checks concern these offline code paths only. |
| Hypothesis | A future acquisition owner can supply validated records and cooperative work accounting through this seam. |
| Unknown | Qualified guard acquisition, live correlation, physical behavior, capability qualification and future gateway integration. |
| Qualification | Evidence blocked independently of a matched identity. |
| Admission | Default denied; automatic runtime admission, catalog registration, live qualification, support claim and write authority remain false. |

Manual documentation review covers source/API correspondence, the transition table, explicit loss on oversize input, synthetic examples and repository-relative/public links. It is not protocol qualification. Public native facts remain in the linked native contracts.

No sockets, framing, transport adapter, acquisition owner, registration, concrete persistence, scheduler, live qualification or authorization gate is added. MCP and GraphQL are unchanged; future bindings must preserve the same envelope, provenance and freshness. eBUS MCP compatibility remains unchanged, eeBUS output stays driver-owned, and gateway G5 D1-D5/O8 wiring is outside this slice.

Remaining parent acceptance includes A23 contract comparison and merge, independent review, complete committed-head gates, transport/replay and consumer parity verification, qualified inventory acquisition, live qualification and separate hardware acceptance. Offline tests do not establish physical qualification or support.
