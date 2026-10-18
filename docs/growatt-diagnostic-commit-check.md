# Growatt diagnostic commit check

This offline native component covers the Growatt `1xSxxP ESS` RS-485 battery
family and the exact revision tuple accepted by the existing decoders.
Nothing is stable before Helianthus 1.0.

The public [native contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/growatt/bms-rs485-1xsxxp-v202.md)
owns register meanings, fixed extents and revision applicability. The
[battery group retention contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/growatt-battery-group-retention.md)
owns the lifecycle proposal. This implementation adds only a pure candidate
check around the [existing diagnostic decoder](https://github.com/Project-Helianthus/helianthus-modbusreg/blob/main/growatt_bms_rs485_multigroup.go).
It implements no aggregate store or retained-state lifecycle.

## API and caller obligations

1. Decode a complete base with `DecodeGrowattBMSTypedReadOnlyStatus`.
2. Call `NewGrowattDiagnosticCommitBase(base, context)`. The context contains
   an opaque successful base-record token, source-instance token, nonzero owner
   epoch and cycle, unit, function, all four revision components and the raw
   native continuity tuple: MCU version, BMS company/generation word, pack
   company/generation word, full topology word and cell-series count. These
   words must equal the decoded base evidence. The constructor redecodes copied
   native slices and freezes them; mutable exported typed fields are not a
   success credential. An empty token, zero counter or zero base is invalid.
3. Call `CheckGrowattDiagnosticCommit(reference, completionContext,
   expectedReportedGroup, diagnosticInput)`. Supply the exact same base-record
   and provenance context asserted for this completion. The expected reported
   group is an explicit raw word, including zero; it is neither a selector nor
   an inferred pack identity. The check runs the existing diagnostic decoder
   only after matching the frozen context.
4. An empty rejection string means success. Commit the whole candidate or
   nothing. `Context()` and `Identity()` freeze the successful base reference,
   cycle, epoch, battery ID and reported-group word. `Diagnostic()` returns a
   detached typed view and copied raw records. Every rejection returns the
   zero candidate, with no good sibling, balance or identity available to commit.

The acquisition owner must truthfully associate the decoded base, unique
base-record token and completion context. This function cannot prove that
separately supplied observations were acquired together. Before the final
atomic commit the caller must serialize completion handling, recheck the
active epoch/cycle and base reference, resolve duplicate/conflicting results,
and enforce continuity and cycle closure. A successful check does not reserve
a cycle or remain valid after its context closes. Callers must not mutate inputs
concurrently with the call.

The checker does not remember a previous diagnostic identity. The owner supplies
its expected group word, and must resolve a changed lineage explicitly before
checking a complete candidate for a new lineage. Passing the received word as
the expectation without checking prior lineage establishes no continuity.

## Bounded results and transition boundaries

| Rejection | Meaning and owner action |
| --- | --- |
| `invalid_base` | Missing/zero reference or base evidence that cannot be decoded; no candidate. |
| `invalid_context` | Constructor provenance is empty, zero or disagrees with native unit/function/revision; no reference. |
| `context_mismatch` | Base-record token, source, epoch, cycle, unit, function, revision or other native continuity word differs; no candidate for this base context. |
| `topology_changed` | Full topology word differs; whole-context continuity must be resolved. This takes precedence over other context differences. |
| `invalid_diagnostic` | Existing decoder rejects the complete input, including absent/malformed/reordered slices or mismatched unit/revision; no partial admission. |
| `diagnostic_lineage_changed` | Complete decoded reported-group word differs from the explicit expectation; fence diagnostic lineage. This alone does not invalidate an otherwise coherent base. |

On success the five raw records remain ordered `0x001F`, `0x0070`, `0x0071`,
`0x0081`, `0x0109`, with their source-slice offsets, selected unit, revision,
function and words intact. Both cell blocks retain all 16 fixed positions.
Zero cells, zero reported-group word and native-balance zero remain present
values. No cell block is summed into aggregate telemetry or renumbered.

For example, if a complete base succeeds in cycle B but the second diagnostic
cell slice has 15 words, the check returns `invalid_diagnostic` and no bundle.
The first slice cannot refresh by itself. An owner with earlier applicable
diagnostics must retain that entire earlier bundle under its original context;
this component does not implement that storage. If only the reported-group
word changes in a complete bundle, the result is `diagnostic_lineage_changed`;
if topology changes, the result is `topology_changed` instead.

## Evidence and delivery boundary

| Evidence status | Boundary |
| --- | --- |
| Proven by offline synthetic tests | Same-context success; independent provenance fences; complete-bundle rejection; fixed cell positions, native zeros and detached evidence. This is software behavior only. |
| Hypothesis | A later composition owner can use this component within a serialized atomic commit and retention lifecycle. |
| Unknown | Hardware identity continuity, acquisition simultaneity, freshness budgets, restart persistence and consumer integration. |

These statuses are separate from qualification, acquisition and admission.
This component adds no polling, registration, transports, reads, profile
admission, principal/credential/permission check or outbound authority. Existing
trust and admission checks remain with their owners. No MCP or GraphQL schema
is changed; later bindings must preserve equivalent presence, provenance and
rejection meaning. eBUS MCP compatibility, G5 D1-D5/O8 and the driver-owned
eeBUS output binding are unchanged.

The focused suite is `go test . -run '^TestGrowattDiagnosticCommit' -count=1`.
Documentation review checks sources, claims, transitions, examples and link
destinations manually; it is not hardware or passive-capture evidence.

Merge is held until the docs-modbus companion
`architecture/growatt-diagnostic-commit-check-implementation.md` is published
before or with this change. That application API/evidence page must cite the
battery group retention contract as native authority and describe the reviewed
API and implementation boundary, outside `protocols/` and `types/`.
This is a partial slice for issue #305 and Project #2; downstream composition,
atomic storage, independent review, committed-head gates and hardware acceptance
remain outside this implementation.
