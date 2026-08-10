# FMV3-M2-03 fixture conformance (RED acceptance skeleton)

## Scope

M2-03 adds a small immutable, transport-neutral conformance corpus over the
existing profile, observation, fixture replay, codec, coherence, provenance,
serialization, limits, and detector contracts. It does not create transport
connections, accept credentials, introduce vendor facts, publish production
samples, or reimplement M2-01/M2-02 behavior.

Fixtures are generic and synthetic. Metadata is exactly a corpus identity,
`CC0-1.0` public license expression, and public synthetic provenance.

## Proposed bounded API

- `FixtureConformanceCorpusSpec`, `SanitizedFixtureMetadata`, immutable
  `FixtureConformanceCorpus`, `Spec`, and bounded corpus limits;
- `MarshalFixtureConformanceCorpusSpec` and
  `UnmarshalFixtureConformanceCorpus` as the strict bounded JSON boundary;
- records bind an existing profile and observation to a serializable detector
  declaration (plan, candidates, limits) plus recorded probe results, exact expected detector
  outcome/reason/profile/version/evidence, qualification disposition, and an
  expected replay outcome/reason;
- `Replay`, immutable report/result accessors, exact logical slices/provenance,
  and deterministic `MarshalBoundedReport`;
- closed `FixtureMutationReason` values and `IsFixtureMutationReason`.

Construction validates schema, bounds, sanitization, immutable declarations,
profile/detector binding, and expected-outcome consistency. A negative record
whose `ExpectedReplay` is rejected is admissible. Replay composes
`FixtureReplayer` and a `ProfileDetector` reconstructed from the serializable
declaration and corpus catalog. It may adapt recorded probe results to
`ProbeReader`, but cannot introduce a parallel codec, detector, qualification,
or observation-validation engine. An incompatible or torn record declared
accepted is contradictory and rejected at construction.

## Acceptance criteria

1. Strict raw JSON decoding rejects missing, unknown, duplicate, case-folded,
   malformed, oversized, and contradictory input with stable reason codes.
2. Construction rejects credential-like endpoints and non-fixture/live source
   identities. Corpus specs and report accessors are defensively copied.
3. FC03/FC04 evidence retains table, unit, normalized address, raw words, wire
   and logical identities, sample/generation identity, and source time.
4. Compatible unequal overlaps preserve exact logical slices. Negative records
   for unit, table/access, generation, source, normalization, deadline, and
   coherence mismatches replay to exact closed reasons without contaminating
   unaffected records.
5. Concrete codec, detector, qualification, normalization, generation, and
   torn-read observations are actually replayed against their expected outcomes.
6. Two distinct generic profiles and two records, reversed independently, and
   concurrent repeated runs produce byte-identical bounded sorted reports. A
   valid marshal/unmarshal round trip reconstructs replayable detector state
   and produces the same bytes.

## RED evidence expected

The three M2-03 test files must be formatted and fail only at type-check because
the proposed M2-03 corpus/harness API does not exist. Non-type-check gates must
remain green.
