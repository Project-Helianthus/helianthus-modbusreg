# FMV3-M2-03 fixture conformance

Status: implemented as an immutable, transport-neutral offline corpus and
replay harness.

## Scope

M2-03 adds a small immutable, transport-neutral conformance corpus over the
existing profile, observation, fixture replay, codec, coherence, provenance,
serialization, limits, and detector contracts. It does not create transport
connections, accept credentials, introduce vendor facts, publish production
samples, or reimplement M2-01/M2-02 behavior.

Fixtures are generic and synthetic. Metadata is exactly a corpus identity,
`CC0-1.0` public license expression, and public synthetic provenance.

That restriction describes the corpus delivered by M2-03, not a permanent
profile-kind allowlist. The harness remains profile-kind-neutral and relies on
the existing catalog, evidence, and qualification gates. Schema v1 does not
claim provenance for sanitized live captures; admitting those requires a
versioned provenance extension and the public evidence work assigned to M3-01.

## Implemented bounded API

- `FixtureConformanceCorpusSpec`, `SanitizedFixtureMetadata`, immutable
  `FixtureConformanceCorpus`, `Spec`, and bounded corpus limits;
- `MarshalFixtureConformanceCorpusSpec` and
  `UnmarshalFixtureConformanceCorpus` as the strict bounded JSON boundary;
- records bind an existing profile and observation to a serializable detector
  declaration (plan, candidates, limits) plus recorded probe results, exact expected detector
  outcome/reason/profile/version/evidence, qualification disposition, and an
  expected replay outcome/reason. Accepted replay expectations also carry exact
  raw words; rejected expectations may omit raw words;
- `Replay`, immutable report/result accessors, exact logical slices/provenance,
  deterministic versioned `MarshalBoundedReport`, and immutable per-dependency
  facts (`DependencyFacts`) retaining FC03/FC04 function/table, normalized
  address, raw words, logical slice, source time, and wire/logical/sample
  identities. The existing record-level accessors remain compatibility views of
  the first dependency;
- closed `FixtureMutationReason` values and `IsFixtureMutationReason`.

Construction validates schema, bounds, sanitization, immutable declarations,
profile/detector binding, and expected-outcome consistency. A negative record
whose `ExpectedReplay` is rejected is admissible. Replay composes
`FixtureReplayer` and a `ProfileDetector` reconstructed from the serializable
declaration and corpus catalog. It may adapt recorded probe results to
`ProbeReader`, but cannot introduce a parallel codec, detector, qualification,
or observation-validation engine: canonical `FixtureReplayer` admission is
authoritative, including bounded-multi coherence. An incompatible or torn record declared
accepted is contradictory and rejected at construction.

## Acceptance criteria

1. Strict raw JSON decoding rejects missing, unknown, duplicate, case-folded,
   malformed, oversized, and contradictory input with stable reason codes.
2. Construction rejects credential-like endpoints and non-fixture/live source
   identities. Corpus specs and report accessors are defensively copied.
3. Every FC03/FC04 dependency fact retains function/table, unit, normalized
   address, raw words, logical slice, wire and logical identities,
   sample/generation identity, and source time; mixed bounded-multi records do
   not collapse provenance to their first dependency.
4. Compatible unequal overlaps preserve exact logical slices. Negative records
   for unit, table/access, generation, source, normalization, deadline, and
   coherence mismatches replay to exact closed reasons without contaminating
   unaffected records.
5. Concrete codec, detector, qualification, normalization, generation, and
   torn-read observations are actually replayed against their expected outcomes.
6. Two distinct generic profiles and two records, reversed independently, and
   concurrent repeated runs produce byte-identical bounded sorted reports. A
   valid marshal/unmarshal round trip reconstructs replayable detector state
   and produces the same bytes, including rejected records without expected raw
   words.

## Validation

The M2-03 tests cover strict decode mutation reasons at record and nested
M2-03-owned object boundaries, construction-time
sanitization and contradiction rejection, exact FC03/FC04 provenance and
slices, deterministic sorted/concurrent replay, executable marshal/unmarshal
round trips, detector evidence, canonical bounded-multi coherence, nested
raw-word cardinality limits, and expected negative replay reasons. The
corpus never opens an endpoint or creates a production sample path.
