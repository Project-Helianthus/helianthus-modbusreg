# M2-02 Deterministic Profile Detection

Status: RED-phase contract skeleton. The API and behavior described here are
acceptance targets exercised by compile-failing tests. No detector is
implemented yet.

## Boundary

M2-02 owns bounded, deterministic, read-only profile detection over the
existing immutable `Catalog`. It does not own a transport endpoint, Modbus
framing, writes, device qualification execution, vendor facts, profile corpus,
gateway composition, or publication.

The caller supplies the only runtime dependency:

```go
type ProbeReader interface {
    ReadProbe(context.Context, ProbeReadRequest) (ProbeReadResult, error)
}
```

`ProbeReadRequest` can represent only FC03 or FC04, a declaration identity,
an address, and a positive bounded word count. It cannot represent an endpoint,
connection, write operation, retry policy, or framing choice. Declarations run
once, serially, and in immutable plan order.

## Proposed Contract

The RED tests target these constructor-owned values:

- `ProbePlan` from `ProbePlanSpec` and ordered `ProbeDeclarationSpec` values;
- `ProbeReadResult` from `ProbeReadResultSpec`;
- `DetectionCandidate` from `DetectionCandidateSpec`;
- `ProfileDetector` from `ProfileDetectorSpec`; and
- `DetectionDecision` with bounded strict JSON marshal and unmarshal helpers.

Probe declarations associate bounded ASCII register words with exactly one of
manufacturer, model, or firmware identity. Duplicate declaration identities,
duplicate identity producers, unsupported functions, empty reads, and values
outside configured limits are invalid.

Each candidate binds one exact catalog profile ID and version to exact
manufacturer/model gates, a semantic firmware range, a finite score, explicit
runtime enablement, and an optional fixture-only disposition. Candidate and
plan versions must agree with the profile's detector and qualification contract
versions.

## Decisions

The closed outcomes are matched, no-match, and ambiguous. Every outcome has a
closed reason. Equal highest eligible scores are ambiguous; lexical profile
identity is used only to order evidence and never to choose a winner. Catalog
or candidate input permutation cannot change the decision.

Active selection excludes revoked and superseded profiles, unqualified
profiles, profiles that default off, explicitly disabled candidates, and
fixture-only candidates without explicit caller opt-in. Ineligible candidates
may appear in bounded decision evidence with their rejection reason, but cannot
win regardless of score.

Read errors, Modbus exceptions, incomplete word counts, invalid encodings,
missing results, malformed firmware, and duplicate or contradictory identity
sources fail closed without selecting a profile. Context cancellation stops the
remaining plan.

Decision evidence is immutable and includes exact profile ID/version, score,
reason, matched gates, ordered probe evidence IDs, detector version, and
qualification version. Accessors return independent copies. Strict JSON rejects
unknown, missing, malformed, and oversized input.

## Bounds

`DetectionLimits` closes plan declarations, executed reads, words per read,
aggregate words, decoded identity bytes, evidence-ID bytes, and serialized
decision bytes. Every limit is finite and positive, and decision serialization
cannot exceed `MaxSerializedContractBytes`.

This document must not be read as implementation or qualification evidence.
GREEN implementation, standard-family declarations, vendor overlays, and the
M2-03 evidence corpus require their own scoped work.
