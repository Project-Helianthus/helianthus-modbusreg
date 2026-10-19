# OutBack supplied-evidence provider port

This I27 family-local port composes the existing
[offline provider](outback-offline-provider-cycle.md) over caller-supplied opaque
OutBack AXS Port family diagnostics. It addresses
[issue #336](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/336)
and advances [parent #201](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/201)
as a partial implementation. Nothing here is stable before Helianthus 1.0.

## Native authority and evidence

The public native [source/profile matrix](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-source-profile-matrix-v1.md)
and [read-only boundary](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-sunspec-readonly-v1.md)
own protocol claims. The supplied
[offline-cycle contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/outback-offline-provider-cycle.md)
provides the composition boundaries. This adapter adds no register mapping,
polling plan, hard-coded read span, decoder or transport request.

**Proven by focused synthetic component tests:** construction/refusal without
supplier calls, one synchronous callback at a time, all eight association axes,
replacement/retirement/cancellation fencing, detached input/output and retained
history without renewed receipt time. Complete evidence still evaluates to
`NO_GO`, with empty typed facts and requests; no-send remains unconditional.

**Hypothesis:** future production composition could consume this typed local
port. It is neither the absent common production Provider interface nor the M02
gateway registry and has no registration or native qualification behavior.

**Unknown:** physical applicability, qualified reads, production integration,
persistence and live behavior. Structural completeness, association, evidence
status, receipt time, freshness, qualification and admission remain independent.

**Merge hold:** I28, the public docs-modbus OutBack provider-port companion, must
merge before or with this code. The local page does not release that hold.

## Typed configuration and lifecycle

Implementation: [port and adapter](../outback_provider_port.go),
[synthetic tests](../outback_provider_port_test.go).

`I27OutBackProvider` exposes `I27Replace`, `I27Retire`, `I27Cycle` and
`I27Snapshot` using existing native source, context, receipt and snapshot types.
`I27NewOutBackAdapter(I27OutBackConfig)` returns the concrete
`*I27OutBackAdapter`, implementing that interface. Configuration requires
`I27Source`, positive `I27MaxBytes`, positive `I27MaxWork` and nonnil
`I27Supplier` of native type `OutBackDiagnosticSupplier`. Construction delegates
to `NewOutBackOfflineProvider`; native validation and errors are preserved.
Nil and zero adapters refuse cycles/replacements and expose empty historical
snapshots. Do not copy an adapter after use.

Each explicit cycle delegates once to native `Cycle`, passing the caller context
and nonzero receipt unchanged. Nil/already canceled contexts, absent receipts
and retired providers refuse before supplier invocation. A busy call returns
`ErrI27OutBackBusy` without changing latest native status or invoking a supplier.
This takes precedence over other refusal categories while a callback is active.
There is no queue, retry, background scheduler, socket or active acquisition.

A callback occupies capacity until it returns, even after replacement,
retirement or cancellation. Lifecycle and snapshot operations remain available
during the callback. The cooperative supplier must honor its native deadline
and bound its own allocations; the adapter cannot preempt arbitrary callback
code. Supplied buffers remain exclusively handed off until the cycle returns.
The native byte bound checks evidence before detachment, not callback heap use.

`I27Replace` delegates to native `Replace` and advances the local association
generation only on success. `I27Retire` delegates permanent closure to `Retire`.
Native `Snapshot` supplies original evidence and comparison, with one additional
restriction: the retained attempt generation must equal the adapter generation
to present a current association. Equal metadata after replacement, or changing
away and back, cannot revive an old publication. Generation/attempt counters
are local publication context, never native identity or qualification.

## History, loss and examples

All native identity axes remain exact: `Key`, `Revision`, `PublicLocator`,
`ContentIdentity`, `DeclaredLayout`, `SourceSetRevision`, `SelectorInputs` and
`Available`. The adapter never stamps expected metadata onto supplied words.
Unavailable original/current sources remain historical; missing current source
metadata also fails closed. Immutable record accessors return independent copies.

| Transition | Record, receipt and outcome |
| --- | --- |
| Complete matching import | Retain detached opaque record and caller receipt; association may be current; admission remains denied. |
| Same-source supplier error, oversize or source mismatch | Preserve previous record, identity and receipt; latest status failed, without retry. |
| Bounded partial/malformed input after a complete record | Preserve complete record and receipt; status incomplete. Discard the candidate fragment rather than accumulate another slot. |
| Partial/malformed input with no complete record | Retain exact diagnostic extents and ordered duplicate occurrences without padding, decoding or repair. |
| Replace source A with B, including equal metadata | Preserve A as historical at its original receipt; status idle. Late A callback cannot publish. |
| Import complete B after replacement | Replace the slot with B and its new receipt. Previously returned A snapshots remain unchanged. |
| Cancel before publication / retire during callback | Discard late result; preserve record and receipt; native canceled / retired status remains. Retirement permanently refuses new cycles. |

For example, import complete synthetic A at logical receipt 1, then fail at 2.
A remains current with receipt 1. Replace A with B: A becomes historical, still
at receipt 1. An incomplete B import preserves A and reports incomplete; only a
complete matching B import replaces it. Replacing B with A cannot make the old
A receipt current again. Receipt values are synthetic caller metadata, not
capture times, freshness verdicts or physical observations.

## Remaining acceptance

Focused discovery: `go test . -list '^TestI27'`.
Focused verification: `go test . -run '^TestI27' -count=1`.
These are offline component checks. Independent review, committed-head full
gates, publication audit and the I28 companion comparison remain required.

No eBUS MCP or GraphQL surface changes; compatibility and parity obligations
remain unchanged. eeBUS output remains owned by helianthus-eebusreg.
G5 D1-D5/O8 are unchanged. No authorization gate is introduced. There is no
runtime, transport, smoke, passive-device or hardware acceptance claim, and
parent #201 remains open.
