# Detached opaque OutBack observations

This offline component implements one bounded immutable diagnostic record for
caller-supplied OutBack AXS Port family evidence. Nothing here is stable before
Helianthus 1.0. This is a partial implementation of
[issue #201](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/201),
tracked by [issue #303](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/303).

## Authority and evidence boundary

The canonical application contract is
[OutBack observation lifecycle](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/outback-observation-lifecycle.md).
Its native authorities are the
[source/profile matrix](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-source-profile-matrix-v1.md)
and [native read-only boundary](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-sunspec-readonly-v1.md).

**Proven by local synthetic tests:** successful construction retains detached raw
words, ordered occurrences (including repeated identifiers), explicit present and
missing spans, and the original source association. Accessors return detached
values. Failed construction returns no record. All evaluations return `NO_GO`,
zero typed facts and zero acquisition requests, including complete 64110/282,
64110/420, 64111/23 and 64111/26 geometry. The native authorities establish the
conflicting declarations; geometry does not select one. Identifier 64111 remains
opaque.

**Hypothesis:** later composition could use these records as diagnostic inputs
for a retention lifecycle. This component does not establish that lifecycle.

**Unknown:** unique public applicability evidence and a qualified read operation.
Source metadata is supplied by the caller, without verification or admission.
Qualification, acquisition and runtime admission remain separate from structural
completeness and retention. Synthetic tests establish no device support.

## API and bound

`NewOutBackDetachedRecord(input, maxBytes)` accepts a positive finite `uint64`
byte bound for this single record. It checks the whole input before copying;
zero or insufficient bounds return `nil, ErrOutBackDetachedRecordBound`.
There is no partial receipt, replacement, padding, deduplication or layout
selection. No store or previous record is touched, including on failure.

The bound accounts deterministic evidence bytes, rather than exact Go heap
usage or a durable serialization format:

| Retained item | Accounted bytes |
| --- | --- |
| Record extent and four collection lengths | 40 |
| Source availability and seven string lengths | 57 |
| Each source string | UTF-8 bytes as supplied, without normalization |
| Each raw or nested payload word | 2 |
| Each present or missing span | 16 |
| Each occurrence's ordinal, wire key, offset and three collection lengths | 40 |

`AccountedBytes()` exposes this total. Every nested word, span, occurrence and
source string participates in the bound; subtraction-based checks prevent
arithmetic wraparound. Go allocation overhead is additional and proportional to
the bounded evidence. Callers must choose a practical bound; this is no promise
of accepting all representable bounds under available memory.

`OutBackDetachedRecordInput` contains raw `Words`, `DeclaredWords`, `Present`,
`Missing`, `Occurrences` and `Source`. Raw words are exactly the supplied
sequence. Occurrence `Words` are payload words without a synthesized header;
`WireKey` preserves the supplied native identifier and declared payload length.
`Ordinal` and slice order are retained independently without renumbering.
`Offset` is a logical payload offset into the record, never a transport address.
Span coordinates are relative to the enclosing raw record or occurrence payload.
Malformed spans are retained diagnostically and do not trigger reads.

`RawWords()`, `PresentSpans()`, `MissingSpans()` and `Occurrences()` copy all
mutable buffers, including nested occurrence buffers. `DeclaredWords()` and
`SourceAssociation()` return values. `Complete()` is conservative structural
metadata: nonempty, ordered contiguous present coverage must cover exactly the
supplied and declared extent, with no missing spans. Every occurrence must also
have matching payload extent and fit inside the record. Gaps, overlaps,
out-of-range extents and empty records are incomplete. It does not verify wire
provenance, source contents, headers, source layout or applicability. Missing
words are never allocated. Unavailable string metadata remains empty, not guessed.

## Association and evaluation

`OutBackSourceAssociation` freezes source key, revision, public locator, content
identity, declared layout, source-set revision, selector inputs and availability.
Strings are opaque exact values. `Association(current)` is a pure comparison:
all fields must match and both descriptors must be available for `current`.
Any independent field change returns `historical`; zero records and unavailable
sources also return `historical`. It never rewrites the original association.
Same payload words associated with two different descriptors remain distinct
records; there is no payload identity index or source substitution.

For example, retain a synthetic incomplete prefix under source `A`, revision
`R1`, layout `candidate-A` and source-set revision `set-A`. Evaluating against
`R2` returns `historical` while the stored association remains `R1`. Changing only
availability to unavailable has the same historical result. An unchanged
available descriptor is `current`, but evaluation is still `NO_GO`. No transition
publishes interpreted facts or performs I/O.

`Evaluate()` returns the existing inert OutBack flavor decision. `TypedFacts()`
and `Requests()` always return empty slices, including on nil and zero records.
No decoder, profile, registry registration, scheduler, transport, provider,
transaction fencing, capacity eviction or durable persistence is added. There
is no authentication or authorization gate. Existing admission and trust checks,
eBUS MCP compatibility, GraphQL parity and driver-owned eeBUS binding are
unchanged; no consumer integration or parity acceptance is claimed.

## Delivery and remaining acceptance

**Merge hold:** the documentation companion owner must publish
`architecture/outback-detached-record-implementation.md` in
[helianthus-docs-modbus](https://github.com/Project-Helianthus/helianthus-docs-modbus),
describing this reviewed API and implementation boundary and citing the canonical
observation lifecycle, before or with merge. This application API material
belongs outside `protocols/` and `types/`.

The focused test command is
`go test . -run '^TestOutbackDetachedRecord' -count=1`.
It proves local synthetic behavior only. Provider lifecycle, concurrent source-set
replacement and publication fencing, retention policy, public qualification,
transport conformance, runtime integration, consumer validation and hardware
acceptance remain outside this slice. The parent issue stays open.
