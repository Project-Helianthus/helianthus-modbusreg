# Fronius offline provider cycle

This family-local application API composes the existing
[retained observation owner](fronius-retained-commit.md). The
[public provider commit contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/fronius-provider-commit.md)
owns the lifecycle contract and links the public native contracts that own
protocol facts and exact applicability. No native card limit or decoder rule
changes here. Nothing is stable before Helianthus 1.0.

The required public companion is
[the offline provider cycle contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/fronius-offline-provider-cycle.md)
in docs-modbus, issue A27. **Merge hold:** that independently authored contract
must merge before or with issue #320. Review must compare its actual API,
results and transition claims with this implementation before releasing the
hold; this link does not assert that the companion is already published.

## Local API

`FroniusOfflineProvider` has a usable zero value and must not be copied after
use. It contains one `FroniusRetainedCommit`, with no second store or parser.

`Run(ctx, registry, request, reader) error` synchronously runs one attempt.
`FroniusOfflineRequest` contains `Key FroniusRetainedKey`, `PollGeneration` and
`DeadlineIdentity` (`uint64`), and `MaxWords`, `MaxOccurrences` and
`MaxSourceViews` (`uint32`). The context must have a future finite deadline.
The bounds must be positive and no greater than the existing card limits:
512 words, 64 occurrences and 512 source views. A smaller bound is permitted
and rejects a larger result. These are input/work limits, not read authority.

`FroniusOfflineReader` has signature
`func(context.Context, FroniusOfflineReadRequest) (FroniusOfflineReadResult, error)`.
It receives the original context, one `Attempt FroniusRetainedAttempt`, and
the three caller bounds. It is invoked exactly once for valid input and must
cooperate with cancellation and the finite bounds. It supplies an existing
`SunSpecQualificationObservation` from offline native records; it performs
no live I/O. The provider starts no goroutine, retry or timeout worker and
cannot forcibly stop a callback that ignores its context.

`FroniusOfflineReadResult` contains `Attempt FroniusRetainedAttempt` and
`Observation SunSpecQualificationObservation`. The callback must echo the
attempt exactly, including its opaque token and original deadline. An error
discards even a valid returned observation; caller error text is not retained.
The post-reader context fence wraps the caller context error for `errors.Is`.
Inputs and returned observations must remain immutable during `Run`. Retained
results and subsequent views are detached from caller slices.

`Cancel(token FroniusRetainedToken) error` delegates to the retained owner.
The callback's supplied attempt exposes the token to a caller coordinating
cancellation. `View() FroniusRetainedView` delegates to the owner's detached,
atomic envelope/latest-attempt view. There is no global provider interface.

## Identity and transitions

The native compatibility key includes requested function, received function,
table, unit, opaque source, connection, transport family, transport generation,
card identity and schema. Poll generation, deadline identity, deadline and
opaque cycle token describe the local attempt. Native source views must still
carry the matching poll and deadline identities; echoing the callback attempt
does not rewrite or validate mismatched native evidence by itself.

| Step | Behavior and loss boundary |
| --- | --- |
| Begin | The existing owner supersedes any old token and immediately drops incompatible history, even if input validation then fails. |
| Validate | Require a finite active context, nonzero identities, exact card key, supported transport, compatible nonempty registry, positive bounded input and nonnil reader. Invalid input makes no reader call. |
| Read | Invoke the injected offline callback once, outside the retained owner's lock, with exact context and attempt. Pending same-key refresh leaves the original envelope stale. |
| Fence | Check caller cancellation after the callback, before considering its result. Errors, mismatched echoed attempt or excessive result sizes fail the active token. |
| Commit | The existing owner replays and qualifies through the supplied decoder registry, checks every native source identity and rechecks token/context before atomic publication. No partial observation can commit. |
| Cancel or supersede | If invalidation wins, late completion returns an error and cannot change a newer attempt or envelope. If commit wins first, its completed envelope remains historical evidence. |
| View | Absence is a nil envelope, never zero facts. Same-key failure retains the entire original compatible envelope, original attempt and local commit instant, with stale status. Key changes drop it permanently; equal values or returning to an earlier key do not resurrect it. |

For example, a complete cycle A followed by same-key cycle B returning a
partial observation reports B as failed and A as stale. A's words, provenance,
flavor, capability, attempt and `CommittedAt` remain unchanged. Without A,
failure leaves absence. A changed connection identity removes A even if B's
words equal A's. `CommittedAt` is local commit age, not capture time or an
operational freshness policy.

## Evidence and remaining acceptance

| Evidence status | Scope |
| --- | --- |
| Proven | Synthetic focused tests exercise exact-card offline success, every echoed identity field independently, native attempt mismatch, per-key invalidation, bounded inputs/results, partial/rejected/error refresh, detached slices and barrier-controlled cancellation/supersession. |
| Hypothesis | Future gateway composition can register and project this family-local seam through a provider registry. |
| Unknown | Live behavior, physical qualification, operational freshness, persistence, restart recovery and consumer parity. |

Qualification, freshness and admission remain separate from these evidence
classes. An offline replay match grants no live qualification, automatic
runtime admission, semantic promotion or write authority. All native evidence
and unavailable/unknown states remain owned by the existing observation/card.
No discovery, transport adapter, acquisition owner, socket or device is added.

This is a partial implementation for Project #2; future M02 provider registration
is outside this slice. Existing eBUS MCP behavior is untouched. Later bindings
must preserve MCP/GraphQL parity and explicit projection loss; eeBUS output
remains driver-owned. No runtime or hardware acceptance is established here.

Focused command: `go test . -run '^TestFroniusOfflineProvider' -count=1`.
Documentation acceptance is manual source, claim, transition, loss-state,
example and public-link review. Companion reconciliation, independent review
and full committed-head gates remain required before merge.
