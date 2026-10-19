# Fronius supplied-evidence provider port

This family-local application seam composes the merged
[offline provider cycle](fronius-offline-provider-cycle.md) and its retained
owner. Protocol facts, exact applicability and native limits remain owned by
the public [native qualification-readiness contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/fronius/qualification-readiness-v1.md)
and [SunSpec flavor contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/fronius/sunspec-float-v1.md).
The public [offline lifecycle contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/fronius-offline-provider-cycle.md)
owns the existing cycle requirements. This page adds no protocol mapping,
applicability, acquisition recipe or larger native allowance. Nothing is
stable before Helianthus 1.0.

**Merge hold:** I26, the docs-modbus Fronius provider-port companion, must merge
before or with issue #335. Its source, claims, transitions, loss states and
examples must be reconciled with this implementation. The companion is a
required public documentation update; this statement does not establish its
publication or approval.

## Configured local API

`I25FroniusProvider` is an interface implemented by `I25FroniusAdapter`:

```go
I25Run(context.Context, I25FroniusLifetime, FroniusOfflineRequest) error
I25Cancel(I25FroniusLifetime, FroniusRetainedToken) error
I25Retire(I25FroniusLifetime) error
I25View() I25FroniusView
```

Construct with `I25NewFroniusAdapter(registry, maxWords, maxOccurrences,
maxSourceViews, reader)`. The existing `SunSpecDecoderRegistry` is supplied by
the caller and copied, including decoder definitions. The three positive
`uint32` ceilings cannot exceed existing card limits. A nil reader, empty or
incompatible registry, zero or excessive ceiling returns an error and no
adapter, with zero supplier calls. Construction performs no native acquisition
or registration. Inputs must remain immutable during construction/calls;
callback observations must remain immutable until the call returns.

Use `I25View().I25Lifetime` for calls. This opaque identity belongs to one
adapter and is independent of `FroniusRetainedAttempt.Token`, which changes
with native attempts. Nil and zero-value adapters refuse work. Foreign, zero
and retired lifetimes refuse work without invoking the supplier or changing
the native owner. Do not copy the adapter after construction.

`I25Run` accepts the existing finite-deadline request. Its three bounds must
also fit the configured ceilings. Oversized requests still pass through the
native Begin/key fence and fail without a supplier call. The merged native
cycle retains ownership of all other validation, exact request echo checking,
source identity, replay, chain scanning and publication. No second scanner,
retained store, qualification result or generic Provider contract is introduced.

The adapter calls the synchronous offline supplier only through the native
cycle. It creates no goroutine, background scheduler, retry, socket or device
access. The supplier must cooperate with the original caller context and
bounds; the adapter cannot forcibly interrupt a blocking supplier. The adapter
lock is released for the supplier, permitting view, cancel, retirement and
supersession; publication resumes under that lock and the native token fence.

## Lifecycle and native loss boundaries

`I25FroniusView` contains `I25Lifetime`, `I25Retired` and the existing detached
`I25Native FroniusRetainedView`. Native current attempt/outcome and retained
receipt keep their separate meanings. A returned view can describe a newer
attempt than the call that just finished. Absence is a nil envelope, never
synthetic zero facts. Source words, spans, occurrence order, decoder revisions,
flavor, capability and provenance remain native evidence.

| Event | Result |
| --- | --- |
| Complete supplied-evidence replay | Existing native owner atomically publishes one complete exact-card receipt. Adapter lifetime stays unchanged. |
| Same-key partial, rejected or failed refresh | Current attempt fails; the entire original last-good receipt, original attempt and local commit instant remain unchanged and stale. |
| Compatibility-key change | Native Begin permanently drops incompatible history, including on invalid new requests. Returning to the old key does not resurrect history. |
| Native chain decoder revision change | Existing exact-card replay remains authoritative; changing an occurrence revision does not create a new admissible profile. |
| Cancel before publication | `I25Cancel` delegates the exact token to the native owner; late completion cannot replace retained evidence. Caller context cancellation after supplier return is also checked by the native cycle. |
| Supersession | Older callbacks may return, but their closed token cannot change the newer attempt or its result. |
| Retire before publication | Mark the adapter retired and cancel its active native attempt while serialized against publication. All later runs are refused; late callbacks cannot publish. |
| Retire after publication | Keep the completed native receipt as history with its original outcome and commit instant; retirement is separately visible. |
| Repeated retire, foreign lifetime or closed/foreign native token | Return an error; do not invalidate another adapter or newer attempt. |
| View or accepted input mutation after return | Detached copies protect retained storage; no caller-owned slice can rewrite an existing receipt. |

The exact key axes remain requested function, received function, logical table,
unit, opaque source, connection identity, transport family, transport generation,
card identity and schema. Attempt axes remain poll generation, deadline
identity, original finite deadline and native token. Neither matching values nor
a matching callback echo can repair incompatible native source evidence.

For example, complete attempt A followed by partial attempt B retains A's
receipt unchanged and stale while reporting B's failure. Retiring during B
closes B and refuses attempt C under the same adapter lifetime. A's local
commit instant remains A's; it is neither a capture timestamp nor an
operational freshness guarantee. With no successful A, absence remains absence.

## Evidence and remaining acceptance

| Class | Scope |
| --- | --- |
| Proven | Source composition delegates Run/View/Cancel to the merged native cycle. Synthetic I25 tests cover identity axes, refusal, retention, detachment, revision rejection, cancellation, retirement and stale completion. Execution evidence is recorded separately. |
| Hypothesis | Future production composition could bind this local port to a common provider owner. |
| Unknown | Live acquisition, runtime integration, operational freshness policy, persistence, restart recovery, physical acceptance and consumer parity. |

These evidence classes do not confer qualification, freshness or admission.
Documentary replay success grants no live qualification, automatic runtime
admission, semantic promotion or write authority. Existing native validation is
unchanged. This is a partial implementation advancing #201; the parent remains
open. The common production Provider interface and M02 registry are outside
this slice. eBUS MCP compatibility and later MCP/GraphQL parity remain
requirements; no consumer binding changes here. G5 D1-D5/O8 are unchanged;
eeBUS output remains in helianthus-eebusreg.

Focused acceptance: `go test . -run '^TestI25' -count=1` with nonzero test
selection. Documentation acceptance is manual source, claim, transition,
loss-state, example and public-link review. Independent review, the I26
companion, committed-head CI, publication audit, runtime integration and
physical acceptance remain separate gates. No transport or smoke PASS is
implied by offline synthetic tests.
