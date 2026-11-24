# LUNA optional-group documentary state reducer

The native authority is the public
[LUNA2000 optional-group contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/luna2000-optional-group-v1.md),
read together with its
[inverter/battery disposition](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/inverter-battery-readonly-v1.md).
This local page describes an application API and its evidence boundary. Nothing
is stable before Helianthus 1.0.

`ReduceLunaOptionalState(prior, input)` is an explicitly called, pure native
component. Its zero state is `unknown`. It performs no acquisition, registration,
scheduling, transport, gateway integration, decoding, or numerical battery
telemetry. The caller supplies documentary observations; this API supplies no
native presence decoder. An event named documentary presence describes the
supplied evidence, without establishing a native encoding or battery support.

Proven: scripted cycles exercise battery reads, decoding and partial retention.
The decoded register facts have the `HYPOTHESIS` evidence grade, separate from
hardware qualification. Unknown: native battery and pack presence or absence,
and live behavior. These evidence labels remain separate from observation
state, qualification, acquisition, and admission.

## Battery values on the provider cycle

An admitted inverter with a qualifying card reads that card’s numeric storage
rows in the 37000 and 37741 ranges and the documented pack rows. Each pack
keeps its register position under its storage unit; a reply does not establish
that a slot is occupied. `ProviderChildObservation.Reading` carries numeric
values and named running states as `<profile-prefix>.storage.<offset>` facts
(index 0). Native words are available in `Reading.Groups()` with read time,
cycle, generation and outcomes in `ProviderChildObservation.Groups`. The
battery fact predicate reports decoded values, not presence or qualification.

Every zero battery field stays Unknown while presence is unresolved. A zero
reply remains in native evidence; it is not a presence, absence or numeric
measurement. Running-state codes, other nonzero words, successful reads,
refusals, and site rated capacity never change the reducer’s presence state.
Failed or refused groups retain last-known-good values at their original
read time. A fresh zero response is not a failed read and does not publish a
current value. Partial retention does not refresh untouched groups.

Raw-only status and calibration words remain native. Unknown code meanings
and unresolved gains do not become typed values. Power keeps the native sign
without assigning an undocumented charge or discharge direction. Battery
identity and software text are outside the telemetry plan.

An EMMA child with a covered card gets its inverter-side readings. Without a
battery decoder the child has no typed battery values; the cycle names the
reason as a unit-scoped site-only loss. Battery measurements then appear only
in `ProviderCycle.EMMASite`, not as fabricated child or pack facts. No guessed
battery extent is requested on a child with no card.

## Inputs and immutable result

`LunaOptionalBinding` contains exact parent and group model, firmware, and unit
identity tokens, an explicit attachment relation, separate parent and group
epochs, and an evidence revision identifying the public source/version and
applicability contract. Identity tokens must come from supplied identity
evidence; inventory labels, aliases, and routing addresses cannot establish
identity. The SUN2000 parent remains independently identified. No capabilities
are inherited from another Huawei family.

`ParentIdentified` reports the caller's independent documentary identity result.
`EpochsAssociated` asserts an explicitly established coherent association; equal
epoch strings alone do not establish it. Neither boolean grants absence,
qualification, support, or admission. `Binding` is the caller's **current**
association; the observation's two epoch fields identify its originating
association. A delayed observation must retain its original epoch fields and
must not replace the caller's current binding.

`Evidence` contains bounded references and native outcome descriptions, with
original `ObservedAt` ticks. References identify supplied evidence; this component
does not dereference them. Raw native evidence remains with its supplying owner;
there is no payload decoder, schema normalization, or universal projection here.
No field supplies battery monitoring values.

The result has read-only accessors. `History()` returns a detached slice; input
slice edits cannot mutate prior results. Each historical record retains its
original binding, event, reference, native outcome, and observation time.
`Applicable` means only that the historical partition has not been fenced and
still matches the current binding. `Fresh` measures only that record's age.
Neither flag makes a record qualified or a current battery value. Fencing is
irreversible for an existing record, even if old identity tokens later recur.

## Evaluation policy and time

Every evaluation supplies a finite, positive `LunaOptionalPolicy`:

| Field | Meaning |
| --- | --- |
| MaxRecords | Maximum evidence references in this evaluation |
| MaxBytes | Aggregate charged evidence bytes, including the complete binding for each reference; also bounds a retained current binding |
| Deadline | Exclusive absolute deadline for the supplied completion/evaluation time |
| FreshnessAge | Inclusive maximum age of each historical record |
| RetentionAge | Inclusive maximum age before eviction |
| HistoryRecords | Maximum retained historical records |
| HistoryBytes | Maximum charged bytes retained in history |

All times are explicit nonnegative ticks on one caller-defined monotonic basis.
`Started <= Now < Deadline` is required. There is no wall clock or sleep; the
caller supplies `Now` as the evaluation/completion time. This does not measure
CPU time or authorize retries. There are zero I/O attempts. A backward time
cannot accept evidence; retention uses the latest accepted evaluation time so
clock regression cannot restore freshness.

Byte charging sums UTF-8 string byte lengths for the entire binding, evidence
reference and native outcome plus 64 bytes for scalar bookkeeping. It is a
logical evidence budget, not a Go heap accounting guarantee. Input records are
validated atomically before appending history. Future-dated records, empty
references/outcomes, over-budget input and expired deadlines cannot refresh
history. Missing/invalid policy retains and ages history under the last accepted
policy; it cannot extend that policy. No implicit device-derived defaults exist.

Eviction visits newest inserted records first, retaining those that fit count,
byte, and age limits, then returns them in insertion order. Oversized records are
evicted; evicted evidence is never reconstructed. A later policy cannot revive
already fenced or evicted records. Failures retain original successful-record
ages and may add separately timed failure records if those records are valid
and within bounds. No whole-group age refresh occurs.

## Transition rules and synthetic examples

Retired observation epochs are rejected first, allowing only age-based history
maintenance. A changed complete binding fences history before interpreting the
event; the resulting state is `identity changed`. A subsequent valid presence
observation can establish `present-unqualified` under the new partition. Missing
identity or incoherent association yields `unknown` and fences prior history.

| Input after binding checks | Result |
| --- | --- |
| Documentary presence with valid bounded references | present-unqualified |
| Inventory-only or missing observation | unknown; missing is never absent |
| Optional failure, malformed evidence or invalid evaluation after established presence | temporarily unreadable; parent identity survives |
| Same failure without established presence | unknown |
| Unknown event, including attempted qualified or absence events | unknown; no new evidence accepted |
| Age expiry alone | Existing observation label persists; records become stale or are evicted |

For synthetic parent P and group G, presence at tick 10 followed by an optional
exception at tick 12 leaves the tick-10 record unchanged and preserves P's
identity. With freshness age 10, that record is stale at tick 21. With retention
age 30, it is evicted at tick 41 even when the new policy is missing. Replacing
only G's firmware fences the original partition. Changing only either epoch or
the evidence revision has the same fence effect. A gateway inventory listing
alone remains unknown.

All outcomes report `NO_ADMISSIBLE_PROFILE`, with catalog registration,
automatic runtime admission, qualification, support claim and write authority
false. There are no absence or qualified event constants and no caller boolean
can enable those transitions. Existing trust/admission boundaries remain in
place; this API introduces no authentication or authorization gate.

## Reducer companion and verification

The [reducer implementation contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/luna-optional-state-reducer-implementation.md)
describes the standalone API. The child monitoring decoder does not call the
reducer or change its transitions. These checks establish offline behavior,
not hardware qualification, transport conformance or live device acceptance.
Existing eBUS MCP and GraphQL surfaces are untouched.
