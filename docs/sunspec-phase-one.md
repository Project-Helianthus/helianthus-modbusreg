# SunSpec phase one

The SunSpec phase-one standard-family profile is **qualified and default-enabled**.
`NewSunSpecPhaseOneProfile` declares Common model 1 and integer inverter models
101, 102, and 103. Qualification describes the standard-family profile, not a
particular device or live installation. Detection does not require a Fronius
flavor match.

## Discovery and compatibility

`NewSunSpecPhaseOneProfile` accepts profile and codec version `1.0.0`, and
`NewPhaseOneDecoder` accepts that exact profile. Its `DiscoveryRequest` is a
read-only FC03 holding-register request at PDU offset `40000` (register `40001`
in one-based notation).

The compatibility parser accepts at most 512 words. It requires the `SunS`
signature, exactly one Common model first, valid model extents, and a terminal
zero-length end marker with no trailing words. It exposes models 1, 101, 102,
and 103, skips bounded unknown models, and rejects explicitly deferred
families. Common lengths 65 and 66 are accepted; the extra pad in length 66
stays in the raw words, not in the typed values. `Activate` binds the parsed
chain to the exact contiguous FC03 source views and the observation dependency
to the captured base view. These are rules of the compatibility API, not of the
general chain walk below.

## Vendor-neutral observation and Common segments

`sunspec.NewObservation` builds an immutable, vendor-neutral `Observation`
from a verified terminal chain with one consistent sample identity and an
admitted three-phase monitoring decision. It retains the sample identity, raw
words, ordered model occurrences, source views, wire chain, and capability
decision. The Fronius inverter-family wrapper,
`fronius.NewSunSpecQualificationObservation`, records a unique matching flavor
as hardware test evidence. A missing flavor is recorded as not observed, even
in the JSON evidence, and does not reject the admitted chain. An ambiguous
match is rejected.

`PlanAllowlistedUnits` keeps a separate chain walk for each configured unit,
including a meter on the same endpoint as an inverter. A listed 7xx or 8xx model
selects the V2 registry alongside applicable V1 definitions, including the
nameplate model 120 when listed.

`SegmentChain` starts a device segment at each Common model (model 1). A segment
includes that Common and every model after it up to the next Common or the end
marker. An inverter and a meter under different Common models are separate
devices even when read in the same chain. A source before the first Common has
no device anchor and is reported as ambiguous by `EvaluateCapabilities`.

## Capability evaluators

`EvaluateCapabilities` applies the same evaluation table to every Common
segment and keeps separate decisions for distinct source occurrences. The
three-phase compatibility evaluator retains its existing identifier and
required facts.

| Evaluator identifier | Family and facts |
| --- | --- |
| `sunspec.inverter.three_phase.monitoring@1.0.0` | `EvaluateThreePhaseMonitoring`: three-phase inverter models 103 and 113, with the required monitoring set and valid optional inverter facts. |
| `sunspec.inverter.monitoring` | Single-, split-, and three-phase inverters (101–103 and 111–113). Reports only the phases the model defines, along with decoded AC, DC, temperature, state, and event facts. |
| `sunspec.der.state` | DER models 120–123: nameplate ratings, settings, connection status, and observed controls. |
| `sunspec.mppt.monitoring` | One entry per model 160 module, with its index and available identifier, label, and measurements. The module role is unknown; a label alone does not establish it. |
| `sunspec.storage.monitoring` | Storage nameplate ratings and state or control facts (models 120 and 124), V2 storage capacity (713), and battery facts (802–805). |
| `sunspec.meter.monitoring` | Meter models 201–204 and 211–214. Retains each meter occurrence and the native sign convention of its measurements. |
| `sunspec.der.measurement` | V2 DER AC measurements (701) and DC measurements (714), retaining the type of each DC port when available. Reuses a V1 fact identifier only when the quantity and sign agree. |

Facts retain native values and units. Absent or unimplemented points, including
values whose scale factor is unimplemented, are omitted rather than reported
as zero. An invalid required fact rejects that capability decision without
invalidating otherwise valid sibling sources.

## Grid support and hand writes

Native reads cover grid-support models **125 to 145**, the V2 enter-service
model **703**, and the V2 DER trip curves **708** (high voltage) and **709**
(low frequency). Models 708 and 709 use their own field definitions and curve
layouts, not the model 707 layout. Model 707 (low voltage) remains a structural
read through `ProjectSunSpecStructuralFacts`, without a decoder key.

For manual writes, `sunspec.PointWrites` lists writable points for a selected
occurrence in a verified chain. This includes documented writable points in
models 120 to 145, 703, 708, and 709, and writable points in the validated
structural layout for 707. Read-only points and read-only curve instances
remain reads; a scaled point without a usable scale offers no write.
`PointWrite.Encode` takes a typed value in the point's native unit and returns
the register words and a single FC16 request for that point's span. If the
value cannot be encoded exactly, it returns an error and no request.

For grid-code settings with no SemReg element, the same native hand-write
operation is for use through MCP and GraphQL in the `modbusreg` namespace. This
is separate from semantic-operation dispatch and does not change the raw-bus
command policy. Discovery and monitoring never write, and the registry encodes
but does not send a write request. A live write requires an explicit call and
operator confirmation at the time of the action.

Model 123 is written.
Model 124 is written.

## Rollback

To stop using the phase-one compatibility profile, remove it from the caller's
catalog and stop invoking its decoder. Retained observations remain immutable.

### Fixed point layout

`DecoderRegistry.FixedPointLayout` looks up a field by its exact `DecoderKey` in
the registry’s fixed model definitions. It returns the point name, field ID,
type, native unit, word offset and width, plus the linked scale-factor layout
when present. Offsets include the ID and length header: ID is word 0 and
length is word 1. Dynamic layouts, unsupported keys and absent fields do not
match. Returned layouts are values; this lookup neither decodes nor admits a
model occurrence.
