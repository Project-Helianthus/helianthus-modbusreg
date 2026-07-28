# M2-01 Profile And Observation API

M2-01 is an offline contract layer. It records source facts from successful
`helianthus-modbus` logical views and does not perform I/O, detection,
qualification, vendor matching, canonical projection, or gateway composition.

## Construction Order

1. Parse strict `major.minor.patch` contract versions.
2. Construct complete immutable codecs with `NewCodec`.
3. Normalize each documentary register address with
   `NewAddressNormalization`.
4. Construct dependencies and an ordered `DependencySet`.
5. Construct a `ProfileDescriptor` and deterministic `Catalog`.
6. Copy successful runtime views with `CaptureLogicalView`, or validate a
   serialized `LogicalViewRecord` with `NewLogicalViewSnapshot`.
7. Restore explicit issuer-domain/high-water `SampleLedgerState` with a trusted
   minimum revision, then bind an `ObservationFactory`.
8. Atomically issue a factory-owned sample identity and receive
   `SampleAdmission`, including the expected revision and next state for
   external compare-and-swap persistence.
9. Use `Replay` and `LogicalViewRecord` for complete immutable raw words and
   transport provenance.

## Codec Contract

Every codec declares raw width, word permutation, intra-word byte order,
representation, scale source and application order, raw sentinels, string
applicability and packing dimensions, output profile type, and validity
behavior. Inapplicable dimensions are explicit. Constructors reject missing,
contradictory, or width-mismatched declarations; no value is guessed, clamped,
trimmed, replaced, or reinterpreted.

## Dependency Identity

`NewDependencySet` preserves declaration order and derives
`dependency_set_id` from the exact set version and complete ordered dependency
records. Reordering, changing a dependency version, codec, normalization, or
word range produces a different identity.

Documentary notation is never treated as a PDU offset. Each normalization
record retains its source locator, notation, base, address-space label,
transformation, documentary address, and resolved zero-based PDU offset. The
constructor recomputes the offset and rejects ambiguity, inconsistency, and
overflow.

## Logical View Snapshot

`CaptureLogicalView` consumes `helianthus-modbus.LogicalReadView`. The immutable
snapshot retains:

- logical, wire-response, and physical-request identities;
- endpoint, unit, transport, connection, and transport generation;
- requested and received function, table, physical offset, and physical count;
- authorization scope, poll generation, and deadline identity;
- logical offset/count and physical slice offset/count; and
- an independent copy of exact wire-order words.

`NewLogicalViewSnapshot` validates operation identity and checked range/slice
arithmetic. A malformed or exceptional wire response cannot provide a
successful runtime logical view and therefore cannot become a valid snapshot.
TCP snapshots require connection identity. RTU snapshots require
`ConnectionID=0`, equal physical/logical ranges, and `SliceOffset=0`, matching
the pinned runtime contract.

## Observation And Replay

`ObservationFactory.NewObservation` requires every contract version, one
`poll_generation_id`, the exact `dependency_set_id`, explicit source validity,
either a real source time or the explicit unavailable state, local receipt
time, endpoint, and unit. Caller-selected `sample_id` values are rejected; the
factory issues them from the ledger's immutable issuer domain and monotonic
high-water.

Dependencies must appear in declaration order and every result must be
successful, complete, version-matched, and provenance-consistent. Missing,
torn, malformed, exceptional, mixed-generation, mixed-endpoint, or incomplete
inputs fail closed and produce no observation.

Every repeated `wire_response_id`, in either coherence mode, binds one exact
physical request, endpoint, TCP connection, transport/generation, unit,
function/table, physical range, authorization scope, poll generation, and
deadline identity. Shared physical positions are reconstructed and overlapping
words must agree exactly. The exact deadline equality matches
`helianthus-modbus` V1 `sameCoalescingIdentity`.

`single_wire_response` requires one such physical/wire group. Logical-view IDs
are unique, acquisition ordinals are absent, and retry-attempt identity is
explicitly not applicable.

`bounded_multi_response` requires explicit acquisition ordering, declared
source/receipt skew, transport-generation equality, same transport family,
endpoint and unit, whole-set retry behavior, and any documentary consistency
marker. The envelope and every dependency carry the same nonzero
`retry_attempt_id`, preventing retained-old/new-attempt mixtures. Its envelope
times are the latest source and receipt times in the validated set.

`SampleLedger` is O(1): state contains schema, issuer domain, exact
`dependency_set_id`, monotonic revision, and high-water, with no per-sample map
or record list. Restore requires a trusted minimum revision. Every
`SampleAdmission` exposes `ExpectedRevision` and `PersistedState`. Before
publishing, serializing for external consumption, or otherwise using its
observation, the consumer must atomically compare the durable revision with
`ExpectedRevision` and replace it with `PersistedState`; a failed CAS discards
the admission. `EmptySampleLedgerState` is only for a newly allocated issuer
domain; external persistence must never recreate an old domain at high-water
zero. M2-01 supplies the CAS anchor and validates imported state, but owns no
durable store and cannot prove that an external write reached durable media.

All admitted times are normalized to UTC, stripped of monotonic/location
metadata, and checked for exact RFC3339Nano round-trip. Out-of-range years fail
before sample issuance.

## Vendor Overlay Delta

A `vendor_overlay` descriptor carries no copied standard applicability, codec
catalog, dependency set, or coherence policy. It references one exact active,
qualified standard-family base and contains only typed
`VendorOverlayDeltaSpec` records. Each delta has its own version and nonempty
overlay-evidence references. The catalog rejects copied base evidence,
duplicate targets, absent targets, and add/remove/replace operations that are
no-ops against the base. Applying these deltas is reserved for M3.

## Serialization

`ProfileDescriptorSpec`, `ProfileDescriptor`, `ObservationSpec`, and admitted
`Observation` use validated deterministic JSON contracts. Profile and
observation round trips retain exact versions, dependency order, raw words, and
the complete `LogicalViewRecord`, retry attempt, overlay delta, and O(1) ledger
state. A recursive bounded token preflight rejects oversized bytes, excessive
depth/collections/strings, duplicate keys, and case-fold aliases at every
object level before strict decoding. Unknown fields and incompatible schemas
fail closed. Optional relationship versions remain absent; zero values are
never rewritten as current schema versions.

Direct constructors apply the same limits before cloning caller slices.
`MaxProfileDependencies` equals the pinned V1 runtime absolute coalesced
dependent cap, `4096`, from runtime commit
`4f81cbeb6321e64fa51676ed6e375ce36b60d16d`; it is not inferred from mutable
scheduler configuration.

`Catalog` validates relationship graphs. Superseded profiles require a distinct
active, version-matched, kind-compatible replacement. Revoked profiles cannot
carry successor fields.

## Ownership Boundary

Source validity and source time remain source facts. This package does not
define downstream units, freshness, availability, publication, aggregation, or
device semantics. Those belong to later composition layers.
