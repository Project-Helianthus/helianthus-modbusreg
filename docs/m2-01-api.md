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
6. Convert a successful runtime `LogicalReadView` into an opaque single-use
   token with `CaptureLogicalView`. Synthetic `LogicalViewRecord` values belong
   only to the fixture/replay lane.
7. Create or restore issuer/profile/dependency-set-bound `SampleLedgerState`
   with a trusted minimum revision.
8. Bind an `ObservationFactory` to the ledger and a consumer-supplied
   `SampleStateCAS`.
9. Start an `ObservationAttempt` with the declared poll generation and retry
   ordinal. For runtime input, call `CaptureDependency` with the declared
   dependency ID, opaque runtime token, and non-view source facts. For fixture
   input, emit deterministic bytes with `MarshalFixtureSpec` and bind them only
   through `DecodeSpec`. Call `Publish`; it returns an `Observation` only after
   the exact ledger-state compare-and-swap succeeds.
10. Use `Replay` and `LogicalViewRecord` for complete immutable raw words and
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
overflow. HTTPS locators require a parsed host and non-root identifier path;
`urn:helianthus:evidence:` locators require a nonempty valid identity suffix.

## Logical View Snapshot

`CaptureLogicalView` consumes `helianthus-modbus.LogicalReadView` and emits an
opaque token whose value copies share one private single-use claim. The token
retains:

- logical, wire-response, and physical-request identities;
- endpoint, unit, transport, connection, and transport generation;
- requested and received function, table, physical offset, and physical count;
- authorization scope, poll generation, and deadline identity;
- logical offset/count and physical slice offset/count; and
- an independent copy of exact wire-order words.

`ObservationAttempt.CaptureDependency` consumes that claim, derives
`poll_generation_id` from immutable provenance, derives `retry_ordinal` from
the attempt, and derives dependency/codec/normalization versions from the
profile declaration. A retained token copy cannot be relabelled into another
retry or attempt. The factory also retains each stable logical-view source
identity for the current poll generation. Calling `CaptureLogicalView` again
cannot remint the same M1 source into a later retry after a failed attempt.
Distinct logical views remain distinct even when their raw words are equal.

`NewLogicalViewSnapshot` validates operation identity and checked range/slice
arithmetic for synthetic fixture/replay records. `NewDependencyResult` likewise
constructs fixture records only; `BindDependency` rejects them on the direct
runtime lane. Synthetic records become attempt-owned only after
`MarshalFixtureSpec` and `DecodeSpec` validate the complete serialized attempt
identity. A malformed or exceptional wire response cannot provide a successful
runtime logical view. TCP snapshots require connection identity. RTU snapshots
require `ConnectionID=0`, equal physical/logical ranges, and `SliceOffset=0`,
matching the pinned runtime contract.

## Observation And Replay

`ObservationAttempt.Publish` requires every contract version, one
`poll_generation_id`, the exact `dependency_set_id`, explicit source validity,
either a real source time or the explicit unavailable state, local receipt
time, endpoint, and unit. Caller-selected `sample_id` values are rejected. The
factory derives them from the ledger's immutable issuer domain and monotonic
high-water only after the external state transition succeeds.

Dependencies must appear in declaration order and every result must be
successful, complete, version-matched, and provenance-consistent. Missing,
torn, malformed, exceptional, mixed-generation, mixed-endpoint, or incomplete
inputs fail closed and produce no observation.

Runtime capture and `DecodeSpec` retain a private complete snapshot for every
attempt-owned dependency. The returned `DependencyResult` is a caller-facing
handle, not mutable publication storage. Replacing its view or changing its
source facts after admission cannot change validation, replay, or serialized
observation output; publication uses the retained snapshot.

Every repeated `wire_response_id`, in either coherence mode, binds one exact
physical request, endpoint, TCP connection, transport/generation, unit,
function/table, physical range, authorization scope, poll generation, and
deadline identity. Shared physical positions are reconstructed and overlapping
words must agree exactly. The mapping is bidirectional: one physical identity
maps to one wire-response ID and one wire-response ID maps back to one physical
identity. The exact deadline equality matches `helianthus-modbus` V1
`sameCoalescingIdentity`.

`single_wire_response` requires one such physical/wire group. Logical-view IDs
are unique inside that physical group, acquisition ordinals are absent, and
retry ordinal, dependency source times, local dependency receipt times, and
acquisition ordinals use their explicit not-applicable representation. Numeric
logical-view IDs may be reused by distinct physical response groups because the
pinned runtime scopes them to one coalesced read.

An RTU physical response produces exactly one logical view in every coherence
mode. TCP may retain multiple views from one physical response only when their
declared logical ranges have one nonempty common intersection:
`max(logical_start) < min(logical_end)`. The same M1 invariant is enforced when
constructing a `single_wire_response` profile. Equal boundaries and disjoint
ranges fail closed even when their physical union fits within the runtime
register limit.

`bounded_multi_response` requires explicit acquisition ordering, declared
source/receipt skew, transport-generation equality, same transport family,
endpoint, TCP connection and unit, whole-set retry behavior, and any documentary
consistency marker. Acquisition ordinals identify unique physical responses,
not dependency views. Multiple logical views from one physical response must
carry the same ordinal, source-time state/value, local receipt time, and marker;
contradictory chronology fails closed. Physical ordinals remain contiguous and
ordered under the declared policy.

An `ObservationAttempt` owns one deterministic identity formed by the declared
`poll_generation_id` and nonzero `retry_ordinal`. Every dependency carries the
same pair. Serialization retains those fixture-stable facts, so an equivalent
fresh-process attempt can validate and rebind exactly one serialized capture
without random state. `DependencyResult` capture handles have a private
single-use claim and an attempt-owned pointer; copied DTOs cannot be rebound or
mixed across attempts. Value copies of `ObservationAttempt` share one private
lifecycle and cannot publish twice.

When every physical acquisition has an observed source time, source skew and
the declared source-time order are checked and the envelope carries the latest
source time. If any acquisition explicitly reports `SourceTimeUnavailable`,
the whole envelope reports unavailable and coherence falls back to the required
local receipt times and receipt skew. Source-time ordering cannot be claimed
when a source time is unavailable. `SourceTimeObserved(time.Time{})` remains an
explicit observed UTC year-one value because presence is determined by
`SourceTime.State`, never `time.Time.IsZero`. Declared skew is
capped by `MaxDeclaredCoherenceSkew`; checked seconds/nanoseconds comparison
avoids `time.Duration` saturation across years 1 through 9999.

`SampleLedger` is O(1): state contains schema, issuer domain, exact profile
ID/version, exact `dependency_set_id`, monotonic revision, high-water, and the
last committed `(poll_generation_id, retry_ordinal)`, with no per-sample map or
record list. Retries within one poll may be attempted only before a successful
commit. After success, that poll is terminal: every attempt whose
`poll_generation_id` is equal to or below the committed poll is rejected,
regardless of retry ordinal. Because the committed attempt is part of the
serialized `expected` and `next` states, restart cannot replay the same
serialized attempt under a new sample ID. At factory construction, committed
single-wire state requires retry ordinal zero, while committed bounded state
requires a nonzero retry ordinal. Restore requires a trusted minimum revision.
`SampleStateCAS.CompareAndSwap(expected, next)` is called without holding the
ledger or attempt-state mutex. A separate local commit serializer snapshots the
expected and next states, releases internal locks, invokes the consumer, and
then finalizes local state. Reentrant callbacks may call `ExportState` without
deadlock. Only a true result advances local state and permits the observation to
escape; false or error returns a zero observation and terminally closes that
attempt. Two processes restored from the same state therefore publish at most
one sample when the consumer implements exact atomic compare-and-swap.

The external store must key transitions by issuer domain, compare the complete
profile/dependency-set-bound state, perform the replacement atomically, and
make a true result durable before returning.
`EmptySampleLedgerState` is only for a newly allocated issuer/profile domain;
external persistence must never recreate an old domain at high-water zero.
M2-01 owns no file, database, socket, or durable store and cannot prove external
durability beyond the `SampleStateCAS` result.

All admitted times are first normalized to UTC, then checked for the supported
year range, stripped of monotonic/location metadata, and checked for exact
RFC3339Nano round-trip. The explicit `observed` source-time state can represent
`0001-01-01T00:00:00Z`. A decoded required `local_receipt_time` key can also
represent that instant: decode retains key presence separately from the Go zero
value. An absent key, JSON `null`, an empty timestamp, or an unmarked zero value
constructed directly in Go remains invalid. Offset-bearing year-boundary
values that cross into UTC year 0 or 10000 fail before sample issuance and
before CAS.

## Vendor Overlay Delta

A `vendor_overlay` descriptor carries no copied standard applicability, codec
catalog, dependency set, or coherence policy. It references one exact active,
qualified standard-family base and contains only typed
`VendorOverlayDeltaSpec` records. Each delta has its own version and nonempty
overlay-evidence references. The catalog rejects copied base evidence,
duplicate targets, absent targets, and add/remove/replace operations that are
no-ops against the base. Every semantic codec, dependency, or coherence
replacement must advance that declaration's version. The catalog materializes
the complete base-plus-delta graph and applies the same codec, dependency,
coherence, evidence, and cycle validation as a standard profile. Runtime
selection and canonical M3 registry resolution remain out of scope.

## Serialization

`ProfileDescriptorSpec`, `ProfileDescriptor`, `ObservationSpec`, and admitted
`Observation` use validated deterministic JSON contracts. Profile and
observation round trips retain exact versions, dependency order, raw words, and
the complete `LogicalViewRecord`, retry attempt, overlay delta, and O(1) ledger
state. A recursive bounded token preflight rejects oversized bytes, excessive
depth/collections/strings, duplicate keys, non-exact field aliases, invalid
UTF-8, unpaired UTF-16 surrogate escapes, explicit JSON `null`, and every
missing required member before strict decoding. Unknown fields and incompatible
schemas fail closed. Optional relationship versions remain absent; zero values
are never rewritten as current schema versions.

Required timestamp presence is structural. In particular,
`local_receipt_time:"0001-01-01T00:00:00Z"` is present and valid, while an
absent or `null` `local_receipt_time` remains a deterministic decode error.

Missing required JSON members are reported in DTO declaration order, so the
same malformed bytes produce the same error across runs. The fixture lane is a
deterministic replay trust boundary, not a runtime capture substitute:
serialized attempt identity is validated now, and M2-03 will additionally bind
fixture bytes to public evidence hashes.

Direct constructors apply one cumulative aggregate budget before cloning caller
slices. Serialization uses a conservative size preflight and bounded writer, so
encoding cannot first allocate an unbounded result.
`MaxProfileDependencies` equals the pinned V1 runtime absolute coalesced
dependent cap, `4096`, from runtime commit
`4f81cbeb6321e64fa51676ed6e375ce36b60d16d`; it is not inferred from mutable
scheduler configuration.

`PinnedRuntimeContractVersion()` is the immutable `1.0.0` M1-04 authority.
Profiles carrying another syntactically valid runtime version are rejected.

`Catalog` validates relationship graphs. Superseded profiles require a distinct
active, version-matched, kind-compatible replacement. Revoked profiles cannot
carry successor fields.

## Ownership Boundary

Source validity and source time remain source facts. This package does not
define downstream units, freshness, availability, publication, aggregation, or
device semantics. Those belong to later composition layers.

The repository scope hash is a change detector enforced by CI. It is not a
malicious-committer security mechanism. Trust in an accepted change comes from
the exact-head adversarial review status and protected-branch policy outside
this package.
