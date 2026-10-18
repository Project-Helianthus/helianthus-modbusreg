# SUN2000 evidence input structure validator

The native authority is the public
[SUN2000 detection input contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/sun2000-detection-input-v1.md),
subject to the [native disposition](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/inverter-battery-readonly-v1.md).
The historical contract name conveys no stability before Helianthus 1.0.

`ParseSun2000EvidenceInput(data, Sun2000EvidenceInputLimits{MaxBytes: ..., MaxRecords: ...})`
is an explicitly called offline native component. Both caller limits must be
positive; there are no device-derived defaults. MaxBytes bounds the supplied
JSON bytes. MaxRecords counts every JSON object, including the envelope,
evidence, candidates, wrappers, alternatives, unit and map values. Arrays do
not consume records but remain byte-bounded. Nesting beyond 64 levels is
rejected as parser resource protection. Errors contain a single bounded static
message and never echo caller fields, values, IDs or malformed JSON.

A successful parse returns `Sun2000EvidenceInput`, with `Subject`, `Evidence`
and `Candidates` accessors. Evidence records expose ID, source, locator, context
and claim level. Candidates expose ID and typed `Family`, `Model`, `Firmware`,
`Unit` and `Map` wrappers. Wrappers expose State, optional Detail, Refs,
Alternatives and `(value, present)` through Value. Unit values expose Context
and Binding; map values expose Document, Revision and Applicability.
All record fields are private. Slices and nested accessor views are detached;
strings and typed scalar records are immutable values. No input buffer is
retained. A zero snapshot is not validated input. On error the returned snapshot
is empty; the caller must check the error and retain its prior valid snapshot
where its own lifecycle contract permits it.

The parser enforces closed, case-sensitive keys, required fields and exact JSON
types, rejects repeated JSON keys and IDs, resolves every reference ID, and
checks wrapper state/value/reference/alternative coherence. Contradictions
require distinct typed alternatives and the exact union of their references;
reference ordering carries no selection meaning. Unit contexts, including
contradictory alternatives, must equal the envelope subject. Present family is
exactly SUN2000; other nonempty native family strings are accepted only as
contradictory alternatives. Duplicate candidate structures with different IDs
are rejected: the tuple key includes all five states, values and sorted
alternative values, excluding detail and reference IDs. This identity comparison
is structural and performs no complete-candidate evaluation.

For a synthetic example, an envelope with contract
`sun2000-detection-input-v1`, subject `synthetic-subject`, evidence `[]` and
candidates `[]` parses with explicit sufficient budgets. Each candidate may
instead contain five `missing` wrappers with null values and empty refs and
alternatives. A structure-valid synthetic candidate with all five fields
present remains an unverified claim. The tests use self-contained synthetic
family, firmware, binding and map labels and exercise missing, unavailable,
present and contradictory wrappers independently.

Proven, Hypothesis and Unknown describe referenced claims only. This component
does not resolve a locator, verify retention, inspect private evidence, qualify
provenance, assess map applicability, evaluate completeness, rank candidates,
or return the detector result envelope. Those checks belong to a future explicit
consumer. In particular, structural validity never means documentary completeness,
qualification, acquisition, support, write authority or profile admission.
The native outcome remains NO_ADMISSIBLE_PROFILE and existing admission flags
remain false. No detector, catalog entry, register map, decoder, transport,
read schedule, gateway integration, MCP or GraphQL projection is added. Later
projections must preserve the native denial and evidence meaning on both
surfaces, including explicit projection loss.

Merge remains held until the public docs-modbus companion
[API and evidence implementation boundary](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/sun2000-evidence-input-validator-implementation.md)
is published before or with this change. That application material must cite
the native contract above; it belongs outside protocols and types. The
companion link names required work and does not assert that it already exists.
Parent integration, evidence qualification, acquisition and physical acceptance
remain separate work.
