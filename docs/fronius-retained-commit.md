# Fronius retained observation boundary

This application API implements the bounded in-memory portion of the
[canonical provider commit contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/fronius-provider-commit.md).
Native facts and exact applicability remain owned by the linked native contracts
and the existing qualification card. Nothing is stable before Helianthus 1.0.

## API and lifecycle

`FroniusRetainedCommit` has a usable zero value and must not be copied after use.
It owns one eligible complete envelope and one latest-attempt diagnostic enum.
It has no persistent history. `View` atomically returns a detached envelope,
latest attempt, outcome and stale flag. Absence is a nil envelope, never zero
measurements. Callers must keep supplied inputs immutable during a method call.

`Begin(ctx, key, pollGeneration, deadlineIdentity)` supersedes the previous
attempt and returns an opaque, store-local token. The context must have a future
finite deadline; poll and deadline identities must be nonzero. The key carries
requested and received function, table, unit, opaque source identity, connection
identity, transport family and generation, card identity and schema revision.
Unknown source, connection, generation or card/schema identity fails closed.
A key change drops the prior envelope immediately, even if the attempted Begin
is invalid. Returning to an earlier key cannot reactivate it. Unsupported keys
may describe a new attempt, but can never bypass the exact card at commit.

`Commit(token, registry, observation)` accepts an existing
`SunSpecQualificationObservation`, never a caller success flag. It obtains the
retained replay and calls the existing card's `Qualify`. Replay must produce the
same complete observation; every source view must agree with the active key
and poll/deadline identity. Existing card bounds and exact applicability stay
unchanged. Validation and publication hold the same mutex as Begin, Fail,
Cancel and View. The context is checked again after replay, before publication.
A successful commit closes the token and atomically replaces the envelope.

`Fail(token)` closes an active attempt with the fixed `failed` diagnostic.
`Cancel(token)` closes it with `cancelled`. Invalid active candidates also close
as failed. Foreign, superseded or already closed tokens return errors without
changing a newer attempt. Context cancellation or expiry is observed at the
next operation; no background task is created. A cancelled or expired active
attempt cannot commit. Explicit Cancel and Commit serialize: if Cancel wins,
publication is rejected; if Commit wins, its completed envelope remains history.

Starting another attempt under the same key marks the previous envelope stale.
Failure or cancellation keeps that envelope, its original attempt, commit age,
raw words, ordered occurrences, source spans, capability and flavor unchanged.
A successful envelope's `CommittedAt` is a local commit instant, not a capture
time or a freshness policy. The caller may apply a stricter age policy. Fresh
here means only latest-attempt success, never operational freshness.

The latest attempt may differ from `Envelope.Attempt`. For example, commit
cycle A, begin cycle B with the same key, then Fail B: View reports `failed`,
`Stale=true`, and B as the latest attempt, while the envelope still carries A.
Before the first complete success, the same sequence returns no envelope.
Changing the key during B makes A permanently unavailable to this store.

## Evidence and integration boundary

| Evidence class | Scope | Qualification and admission |
| --- | --- | --- |
| Proven | Synthetic offline tests exercise the retained boundary and existing exact-card replay; source review establishes no transport calls | Offline replay only; no new live qualification |
| Hypothesis | Later composition can explicitly call this API and project identical envelopes through MCP and GraphQL | No integration or consumer parity acceptance yet |
| Unknown | Device behavior, operational age policy, restart recovery and downstream projection behavior | No runtime or hardware acceptance |

This component performs no acquisition, planning, flavor selection, polling,
transport registration, discovery, semantic promotion or gateway integration.
It preserves existing native authorization-scope provenance without adding an
authentication, authorization, principal or permission check. The existing card
still grants no automatic runtime admission or write authority. Existing eBUS
MCP behavior is untouched; later consumer integration must preserve MCP and
GraphQL parity and make any projection loss explicit. eeBUS binding stays
with its driver owner.

The docs-modbus companion
`architecture/fronius-retained-commit-implementation.md` must describe the
reviewed API and evidence boundary and cite the canonical provider contract
before or with merge. This is application lifecycle material outside
`protocols/` and `types/`. Issue #302 advances #201 as a partial slice; hardware,
transport, integration, independent review and committed-head gates remain open.

## Focused evidence

`go test . -run '^TestFroniusRetainedCommit' -count=1` selects the local suite.
It independently varies each compatibility-key field, cycle token, poll and
deadline identity. Controls cover complete replacement, partial and zero-value
rejection, stale age preservation, detached inputs/views, mixed native source
views, finite deadlines, foreign tokens and cancellation/commit ordering.
These synthetic cases establish the offline boundary only. Documentation is
reviewed manually against source, transitions, examples and public links.
