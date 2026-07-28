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
7. Import explicit `SampleLedgerState`, bind an `ObservationFactory`, and
   atomically construct/admit an all-or-nothing `Observation`.
8. Use `Replay` and `LogicalViewRecord` for complete immutable raw words and
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

`ObservationFactory.NewObservation` requires every contract version, a
unique non-empty
`sample_id`, one `poll_generation_id`, the exact `dependency_set_id`, explicit
source validity, either a real source time or the explicit unavailable state,
local receipt time, endpoint, and unit.

Dependencies must appear in declaration order and every result must be
successful, complete, version-matched, and provenance-consistent. Missing,
torn, malformed, exceptional, mixed-generation, mixed-endpoint, or incomplete
inputs fail closed and produce no observation.

`single_wire_response` requires one physical/wire response, connection,
authorization scope, transport generation, function, table, endpoint, and unit.
Logical-view IDs are unique. Shared physical positions are reconstructed and
overlapping words must agree exactly. Per-dependent deadline identities are
retained losslessly and are not collapsed into one inferred value.

`bounded_multi_response` requires explicit acquisition ordering, declared
source/receipt skew, transport-generation equality, same transport family,
endpoint and unit, whole-set retry behavior, and any documentary consistency
marker. Its envelope times are the latest source and receipt times in the
validated set.

`SampleLedger` state is mandatory factory input. Admission is atomic under
concurrency. `ExportState`, `MarshalSampleLedgerState`, and
`UnmarshalSampleLedgerState` define the restart boundary so a restart cannot
silently create a fresh identity domain. M2-01 does not own durable storage:
the consumer must persist exported state before publishing or otherwise using
the returned observation externally.

## Serialization

`ProfileDescriptorSpec`, `ProfileDescriptor`, `ObservationSpec`, and admitted
`Observation` use validated deterministic JSON contracts. Profile and
observation round trips retain exact versions, dependency order, raw words, and
the complete `LogicalViewRecord`. Unknown fields and incompatible schemas fail
closed. Optional relationship versions remain absent; zero values are never
rewritten as current schema versions.

`Catalog` validates relationship graphs. Vendor overlays require their exact
active, qualified standard-family base in the same catalog. Superseded profiles
require a distinct active, version-matched, kind-compatible replacement.

## Ownership Boundary

Source validity and source time remain source facts. This package does not
define downstream units, freshness, availability, publication, aggregation, or
device semantics. Those belong to later composition layers.
