# SUN2000 supplied-evidence provider port

I13 adds a family-local offline composition seam over the merged
[SUN2000 structural validator](sun2000-evidence-input-validator.md).
The public native authority is the
[SUN2000 detection-input contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/sun2000-detection-input-v1.md).
It supplies the documentary schema and reference rules; this port adds no
register address, read procedure, exact map, decoder or supported firmware.
Nothing here is stable before Helianthus 1.0.

`I13Port` and `I13Provider` are local composition types, not the common
production Provider interface or a gateway registry. `I13NewProvider` freezes
positive byte/object budgets and an injected `I13Supplier`. The supplier must
return one already-held record synchronously, cooperate with caller
cancellation and finish within caller bounds. No goroutine forcibly stops
caller code. The supplier must keep returned bytes unchanged until the cycle
returns. This seam creates no socket, device access, acquisition, probe,
background scheduler, registration, native qualification or live integration.

## Identity, bounds and receipts

`I13Identity` binds five caller-local fields: subject, candidate ID, evidence
reference ID, source token and positive generation. These tokens contain no
device addresses or identifiers. Each explicit `I13Cycle` allocates a local
sequence receipt, calls the supplier once and requires the returned request to
match all five fields and that receipt exactly. Invalid construction, nil or
cancelled context, retirement and overlapping cycles refuse work before the
supplier is called. A zero provider refuses work too.

Returned bytes must fit `MaxBytes` before copying. The unchanged native parser
then enforces `MaxBytes` and `MaxRecords`, counting every JSON object including
wrappers and alternatives. The parsed subject must match the frozen subject,
the named candidate and evidence record must exist, and that candidate must
reference the named evidence in at least one presence wrapper. These are
structural joins only: documentary applicability context, exact tuple
provenance and native qualification are not established by these joins.
Additional candidates and references remain intact; none is selected.

Every accepted-size input is copied before parsing. Successful records retain
the original bytes, native snapshot and original request receipt. Returned
records have independently copied byte buffers, candidate wrappers, references
and alternatives. A supplier error is reduced to a fixed error; input text is
never interpolated into a diagnostic. Oversized and rejected bytes are not
stored as accepted evidence. The rejected attempt retains its allocated
request receipt, including when supplied bytes are malformed.

## State transitions and loss

| Transition | Current record | Retained record |
| --- | --- | --- |
| First structurally valid, joined input | Detached input with its receipt | None |
| Later valid input | New detached input and receipt | None; stored last-valid record advances |
| Malformed, over-budget, mismatched or supplier-error attempt | None; rejected receipt is separate | Previous structurally valid input, unchanged original receipt |
| Cancellation detected after supplier return | None; rejected receipt is separate | Previous structurally valid input, unchanged original receipt |
| Source/generation replacement | Pending old callback cannot publish | Cleared; old-source input cannot be inherited |
| Retirement | Pending callback cannot publish; future cycles refuse | Cleared |

`I13Replace` requires a strictly greater generation even for a source-only
change. It invalidates pending work and prior-source state. Retirement is
terminal. Receipts continue increasing across replacement and never represent
capture time, freshness or physical observation. Only one cycle may be pending.
There is no retry, fallback, partial profile or automatic refresh.

Every result, including every refusal, has `I13Outcome` equal to
`NO_ADMISSIBLE_PROFILE`. A structurally valid documentary record is not a
qualified profile. This API has no profile selection or admission field and
constructs no qualified native object. Proven, Hypothesis and Unknown claim
levels remain native documentary claims, separate from qualification,
freshness, runtime readiness and physical acceptance. Unknown optional battery
state remains Unknown; no telemetry values are generated.

The result preserves all validated native input; malformed input has no partial
native snapshot, and rejected raw bytes are omitted. A consumer that projects
away references, receipts, native detail or rejection state must declare that
loss. This seam changes no eBUS MCP or GraphQL surface; future adapters must
preserve the same evidence and denial meaning on both surfaces.

## Synthetic example and acceptance boundary

For a local synthetic subject, candidate and evidence reference, a supplied
family wrapper claiming SUN2000 may be structurally valid while all other
wrappers are missing. The cycle retains that documentary input, assigns receipt
one and returns `NO_ADMISSIBLE_PROFILE`. Supplying malformed JSON on the next
cycle returns a rejection with receipt two and the retained first record still
carrying receipt one. Cancellation cannot publish the second record, and
replacement removes the first source's retained record. Even synthetic exact
model, firmware, unit and map placeholders cannot change admission.

Focused `TestI13` checks exercise joins, bounds, refusal call counts,
cancellation, replacement, retirement, receipt retention, independent copies
and documentary denial. Synthetic structural checks provide no native
qualification, runtime integration, transport, smoke or physical acceptance.
The I14 public documentation companion must merge before or with this code.
This is a partial slice of issue #199 and closes no parent profile acceptance.
