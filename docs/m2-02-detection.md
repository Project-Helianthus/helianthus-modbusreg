# M2-02 Deterministic Profile Detection

Status: implemented registry contract. This package supplies deterministic
detection declarations, execution, decisions, and strict decision
serialization. It does not supply a profile corpus or perform qualification.

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

## Contract

The implementation exposes these constructor-owned immutable values:

- `ProbePlan` from `ProbePlanSpec` and ordered `ProbeDeclarationSpec` values;
- `ProbeReadResult` from `ProbeReadResultSpec`;
- `DetectionCandidate` from `DetectionCandidateSpec`;
- `ProfileDetector` from `ProfileDetectorSpec`; and
- `DetectionDecision`, serialized through `MarshalDetectionDecision` and
  `UnmarshalDetectionDecision`.

Constructors reject zero values and malformed or infeasible declarations.
Slice inputs and outputs are defensively copied. A `ProfileDetector` retains an
independent plan, exact catalog profiles, exact candidate profile versions, and
the detector and qualification contract versions carried by those profiles.
One detector can be used concurrently because each `Detect` call owns its read
and evaluation state.

Probe declarations associate bounded ASCII register words with exactly one of
manufacturer, model, or firmware identity. Duplicate declaration identities,
duplicate identity producers, unsupported functions, empty reads, and values
outside configured limits are invalid.

Each candidate binds one exact catalog profile ID and version to exact
case-sensitive manufacturer/model gates, a half-open semantic firmware range,
a positive score, explicit runtime enablement, and an optional fixture-only
disposition. The plan version and detector version must equal every bound
profile's detector contract version. Decision evidence retains each bound
profile's exact detector and qualification versions.

Firmware comparison uses numeric semantic-version components without integer
conversion, so component size cannot overflow a machine integer. Aliases,
pre-release forms, leading-zero aliases, and non-numeric components are not
accepted.

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
missing results, malformed firmware, duplicate evidence identities, and
duplicate or contradictory identity declarations fail closed without selecting
a profile. Context cancellation is checked before and after every caller read;
it stops the remaining plan and returns a bounded no-match decision together
with the context error.

Decision evidence is immutable and includes exact profile ID/version, score,
reason, matched gates, ordered probe evidence IDs, detector version, and
qualification version. Accessors return independent copies. Strict JSON rejects
unknown, missing, duplicate, case-folded, malformed, non-canonical-key,
unreachable, and oversized input. Marshal output is deterministic and bounded;
unmarshal reconstructs an immutable decision and validates outcome, ranking,
gate, and selected-profile consistency.

## Bounds

`DetectionLimits` closes plan declarations, executed reads, words per read,
aggregate words, decoded identity bytes, evidence-ID bytes, and serialized
decision bytes. Every limit is finite and positive. Plan feasibility and a
conservative worst-case decision size are checked before any read. Runtime
results are checked before identity decoding, and decision serialization cannot
exceed either its configured limit or `MaxSerializedContractBytes`.

This implementation is not qualification evidence. Standard-family
declarations, vendor overlays, and the M2-03 evidence corpus require separate
scoped work backed by public evidence.
