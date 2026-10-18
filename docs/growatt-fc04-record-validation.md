# Growatt FC04 proposed-record validation

The native authority is the public [Protocol II FC04 admission-record
proposal](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/growatt/protocol-ii-fc04-admission-record-v1.md).
This component implements offline record parsing and symbolic comparison for
the TL3-X MAX/MID/MAC families. Nothing is stable before Helianthus 1.0.

## API and evidence boundary

`ParseGrowattFC04Record` accepts one JSON object containing exactly these eleven
mandatory scalar keys: `family`, `device_type`, `model_build_word_82`,
`model_build_word_83`, `reported_protocol_word`, `source_schema_ref`, `function`,
`offset`, `width_words`, `field_schema_ref`, and `tuple_evidence_ref`.
It rejects duplicate keys (including escaped equivalents), unknown keys,
nulls, wrong types, coercions, trailing values, malformed references and invalid
extents. Native words are unsigned 16-bit integers. Family is one of MAX, MID,
MAC; function is 4, offset is 0 and width is 59 words. References are nonempty,
exact identifiers without whitespace or control characters; parsing does not
resolve them or verify their publication or review.

`CompareGrowattFC04Record` takes a proposed record, an existing coherent native
identity observation, symbolic tuple entries, and symbolic schemas. It rechecks
the full observation through the existing identity decoder. It resolves exactly
one tuple entry by `tuple_evidence_ref` and exactly one schema by
`source_schema_ref`, then conjunctively compares every table field. Neither
candidate ordering nor a matching candidate among conflicting definitions can
select a winner. Both model words remain separate in FC03 register order 82,
83. The document edition is metadata, never the reported protocol word at 88.

`GrowattFC04SymbolicSchema` carries the exact source and field-definition
references, document edition and amendment identity, function and extent, and
symbolic field extents. Every supplied field extent must have positive width
and fit wholly within the source extent using non-wrapping arithmetic. These
geometric operands do not supply decoding, scaling, signedness, units or enum
rules. The exhaustive definitions remain identified by `FieldSchemaRef`;
caller assertions cannot establish that those definitions exist or are reviewed.
An amendment requires a distinct exact source reference even if the edition
label is reused. The diagnostic has no cached decision or lifecycle state.
Changing that reference requires a fresh comparison; conflicting amendments
under the same reference are ambiguous.

Errors support `errors.Is`: `ErrGrowattFC04Missing` identifies absent fields or
unresolved references; `ErrGrowattFC04Invalid` identifies malformed input,
failed equalities, invalid identity or field geometry;
`ErrGrowattFC04Ambiguous` identifies multiple resolutions, even identical
repetitions. A field-definition reference disagreeing with the resolved source
is invalid. No partially parsed record is returned on error.

For example, two caller-created symbolic operands may compare equal. That
result says only that their values satisfy the comparison. Supplying the same
record to `ValidateGrowattFC04RecordAdmission` still returns
`NO_ADMISSIBLE_PROFILE` and missing evidence for a well-formed identity and
record. Malformed records and incoherent identity observations also return
`NO_ADMISSIBLE_PROFILE`, with their diagnostic error. The reviewed public
catalog is empty and has no caller population or approval API. No applicability
value, typed telemetry constructor or observer constructor is added.

## Claims and transitions

| Evidence status | Claim | Separate consequence |
| --- | --- | --- |
| Proven within focused synthetic tests | Closed parsing, exact comparisons, geometry checks and public denial behave as described | Software mechanics only; no device qualification or admission |
| Hypothesis | A future independently reviewed tuple-to-schema mapping could support an owning runtime implementation | Requires separate public evidence and acceptance |
| Unknown | A current reviewed exact native tuple mapping for these families | Public admission remains `NO_ADMISSIBLE_PROFILE` |

This component retains no observations and changes no existing decoder or
retention rule. A failed comparison cannot replace partial last-known-good
native state. A later composition owner must preserve historical provenance,
handle amendments/withdrawals and revalidate current review before consumption.
This diagnostic cannot approve a cached mapping across a source transition.

There is no detector registration, transport, scheduling, acquisition, profile
admission, projection or gateway integration. Existing trust/admission checks,
eBUS MCP compatibility and GraphQL parity are untouched. Focused tests use
synthetic operands; no hardware, passive capture or live-device acceptance is
claimed.

## Documentation merge dependency

The public application companion
[architecture/growatt-fc04-record-validation-implementation.md](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/growatt-fc04-record-validation-implementation.md)
must describe the reviewed API and implementation boundary, cite the native
proposal above, and merge before or with this exported component. That companion
is required merge work; the link does not assert that it has been published.
Application API and lifecycle prose belongs in `architecture/`, outside the
native `protocols/` and `types/` contracts. The broader native-readiness parent
remains open for reviewed evidence, owning-runtime integration, bounded
acquisition/decoder acceptance and separately required real-build qualification.
