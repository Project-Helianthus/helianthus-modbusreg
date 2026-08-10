# M2-01 Profile And Observation API

M2-01 is a contracts and observation-ingestion layer. It consumes successful
opaque runtime values from `helianthus-modbus` M1-06 and does not perform I/O,
detection, qualification, vendor selection, canonical projection, or gateway
composition.

## Construction Order

1. Parse strict `major.minor.patch` contract versions.
2. Construct complete immutable codecs with `NewCodec`.
3. Normalize documentary register addresses with `NewAddressNormalization`.
4. Construct dependencies and an ordered `DependencySet`.
5. Construct a standard-family `ProfileDescriptor` and deterministic `Catalog`.
6. Create or restore profile-bound `SampleLedgerState` and bounded
   `LedgerRestartState`.
7. Create `SampleLedger` with finite `LedgerLimits`.
8. Implement `PublicationCommitter` so durable state, irreversible external
   effect, and terminal decision share one transaction.
9. Construct `ObservationFactory` from the profile, ledger, and committer.
10. Begin, issue, admit, claim, seal, and publish one runtime attempt, or cancel
    it. Fixture replay uses the separate offline path described below.

## Production Runtime API

```go
factory, err := modbusreg.NewObservationFactory(profile, ledger, committer)

attempt, err := factory.BeginRuntimeAttempt(modbusreg.RuntimeAttemptRequest{
    Source:       source,
    AttemptKey:   attemptKey,
    Identity:     identity,
    Observation:  observationFacts,
    Dependencies: dependencyFacts,
    Diagnostics:  diagnostics,
})

err = attempt.Issue(ordinal, logicalView, normalizationRecord)
err = attempt.Admit()
outcome, err := attempt.Claim(ordinal)
err = attempt.Seal()
observation, err := attempt.Publish(ctx)

// Any open, admitted, sealed, or publishing attempt can be cancelled.
result, err := attempt.Cancel()
```

`BeginRuntimeAttempt` calls `Source.BeginAttempt(AttemptKey)` itself and keeps
both the exact source and `*modbus.RuntimeAttempt` private in one shared attempt
state. It reserves the attempt and claim terminal sequences as one batch,
rejects duplicate retained attempt keys, and checks every configured bound
before producer authority is retained.

`Issue` accepts only an ordinal, one successful M1 `LogicalReadView`, and its
producer-parsed `RuntimeNormalizationRecord`. It calls
`Source.Issue(retainedAttempt, ...)` internally and stores the returned
`RuntimeAcquisition` privately at that ordinal. A copied attempt shares the same
state, so the same ordinal or source view cannot be issued twice.

`Admit` accepts no caller-supplied source, attempt, instance, or acquisition.
It requires every dependency ordinal exactly once, closes the retained producer
attempt over the private ordered set, and stores the exact returned
`RuntimeAttemptInstance`. Acquisitions from another source or attempt cannot be
injected into admission.

`Claim` performs exactly one producer
`acquisition.Capability().Claim(retainedInstance)` operation. Later calls see
the immutable terminal claim outcome and cannot publish the acquisition again
through copied attempt values or another factory view.

`Seal` succeeds only when every dependency claim terminally succeeded. It
freezes the successful runtime set before publication arbitration.

No exported method returns an M1 attempt, attempt instance, runtime
acquisition, or capability.

## Normalization And Provenance

The registry retains the exact bytes returned by
`RuntimeNormalizationRecord.Bytes()`, including unknown producer extensions.
It reads `RuntimeNormalizationRecord.Fields()` only to match the declared
dependency identity, table, documentary address, function, normalized offset,
and word count. Replay returns an independent byte-identical copy.

Each admitted dependency also retains exact M1 provenance: logical, wire, and
physical identities; endpoint and connection facts; transport generation;
unit and function; physical and logical ranges; authorization scope; poll and
deadline identities; raw words; and exact wire-response bytes.

The publication dependency-set digest does not hash a profile JSON document or
`DependencySet.ID`. Its normative v1 encoding is:

1. `helianthus.modbusreg.runtime-dependent-identities/v1\x00`;
2. a big-endian `uint64` dependency count; and
3. each ordered dependency ID and version as a big-endian `uint64` byte length
   followed by the exact UTF-8 bytes.

Changing dependency order or identity changes the digest. Metadata-only profile
changes do not.

## Attempt Lifecycle

The closed lifecycle is:

```text
open -> sealed -> publishing -> published
  |        |           `-----> publish_failed
  `--------+-> cancelling ----> cancelled
                         `----> cancel_failed
```

Claims linearize against sealing and cancellation. Cancellation waits for an
in-progress producer claim, terminalizes unresolved M2 claims, and drains the
exact producer instance through the retained source.

`modbus.ErrRuntimeAttemptClosed` during drain is the known benign disposition
when producer restart export already retired an otherwise terminal exact
instance. Any unknown drain error still reaches an immutable terminal state,
wakes waiters, emits an audit tombstone, and reclaims retained resources. It
never leaves an attempt in `cancelling`.

Publication and cancellation share one arbitration. A publishing attempt has a
private cancel signal and derived context. Commit-slot acquisition selects on
the slot, the caller context, and that signal, then rechecks cancellation before
committer admission. A queued publication therefore cannot wait indefinitely
behind another blocked committer after its own cancellation wins.

## Transactional Publication

```go
type PublicationCommitter interface {
    CommitPublication(
        context.Context,
        PublicationCommitRequest,
    ) (PublicationCommitDecision, error)
}
```

`PublicationCommitRequest` contains the complete expected ledger state, the
next published state, and the only externally publishable attempt projection.
The committer must atomically commit the published state, irreversible external
effect, and `committed` decision. An error or `cancelled` decision guarantees
that no irreversible effect occurred.

The ledger serializes committer admission locally without holding its mutex
during the callback. A committed decision advances revision/high-water and
produces the production sample ID. Any other decision produces no observation
and terminalizes the attempt as `publish_failed`.

## Bounded Ledger And Restart

`LedgerLimits` requires finite positive limits for:

- retained attempts;
- claim entries per attempt and in aggregate;
- canonical dependency bytes;
- attempt-key UTF-8 bytes;
- normalization record bytes;
- retained diagnostic count and bytes; and
- audit tombstone count and encoded bytes.

Terminal sequence reservation is all-or-nothing for one attempt and all of its
claims. Sequence exhaustion is explicit. Terminal attempts are reclaimed
synchronously, freeing attempt and claim capacity for deterministic reuse.

`LedgerAuditTombstone` is a strict bounded snake_case JSON variant. Attempt and
claim records contain only terminal sequence links, ordinal where applicable,
and closed outcomes. They do not reconstruct attempt keys, capabilities,
normalization bytes, diagnostics, or producer authority.

`ExportRestartState` succeeds only when no live attempt remains. Restart state
contains bounded tombstones plus terminal-sequence state; live attempts and
capabilities are intentionally nonserializable.

## Offline Fixture Replay

Fixture replay is not a production attempt:

```go
replayer, err := modbusreg.NewFixtureReplayer(profile)
fixture, err := replayer.Replay(encodedFixture)
```

`FixtureReplay` exposes only `FixtureID`, `Spec`, and immutable dependency
`Replay`. It has no runtime capability, claim, seal, publish, committer, ledger
transition, or production `SampleID`. Empty, mixed, malformed, or
production-identified fixture bytes fail closed.

Fixture identity is a domain-separated content digest. Replaying fixture bytes
does not advance production revision or high-water state.

## Codec, Dependency, And Coherence Contracts

Every codec declares raw width, word permutation, intra-word byte order,
representation, scale source and order, sentinels, string dimensions, output
type, and validity behavior. Inapplicable dimensions are explicit. Constructors
reject missing, contradictory, or width-mismatched declarations; no value is
guessed, clamped, trimmed, replaced, or reinterpreted.

`NewDependencySet` preserves declaration order and derives its documentary set
identity from complete ordered dependency records. Documentary notation is
never treated as a PDU offset. Normalization retains source locator, notation,
base, address-space label, transformation, documentary address, and resolved
zero-based offset.

Every repeated wire-response identity binds one exact physical request,
endpoint, connection, transport generation, unit, function/table, range,
authorization scope, poll generation, and deadline identity. Overlapping words
must agree exactly.

`single_wire_response` requires one physical/wire group. The bounded mode
requires explicit acquisition order, source/receipt skew, generation equality,
same transport family/endpoint/connection/unit, whole-set retry behavior, and a
documentary consistency marker. RTU and TCP logical-view invariants remain those
enforced by the pinned M1 runtime contract.

## Serialization

Profile, observation, ledger, restart, and audit records use deterministic
bounded JSON. Recursive preflight rejects oversized input, excessive nesting or
collections, duplicate and case-folded keys, invalid UTF-8, unpaired surrogate
escapes, `null`, missing required members, unknown fields, and incompatible
schemas before activation.

Required timestamp presence is structural. UTC year one is valid when
explicitly present; absent, unmarked, or out-of-range values fail closed.

## Repository Boundary

This is one multi-vendor registry. M2-01 contains contracts and producer-backed
observation ingestion only. Concrete detector, probe, qualification, standard
profile, vendor profile, gateway, private binding, and canonical semantic
ownership are out of scope.

The offline structural policy scans the current tree and product imports. It
does not maintain file inventories, source hashes, documentation locks, Git
history checks, GitHub calls, or network checks. Review follows the workspace
blocker-driven process against the current head.

M2-01 completion is a hard stop. Another milestone or gateway work begins only
after a separate operator request.
