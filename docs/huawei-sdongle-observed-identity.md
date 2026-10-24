# Huawei S-Dongle observed identity

This registry slice turns one read-only observation of an S-Dongle fronting one
inverter into synthetic fixtures, one inert qualification card and a revised
disposition. It admits an identity tuple offline. It registers no catalog entry,
grants no runtime admission, support claim or write authority, sends no request,
and leaves the profile and monitoring outcomes where they were.

The protocol facts, their Proven, Hypothesis and Unknown grades and the
exact-tuple semantics are owned by the public documentation, not by this page:

- [gateway contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md),
- [SmartLogger observation](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/smartlogger-observed-inventory-v1.md),
- [SUN2000 identity contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/sun2000-observed-identity-v1.md) and
- [qualification readiness](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/qualification-readiness-v1.md).

They are published by the companion documentation pull request
(helianthus-docs-modbus issue 241, pull request 243), which is merged. Those
pages still record the S-Dongle as unobserved. The observation behind this slice
is therefore a registry-side candidate until a contract update describes it, and
the card, the fixtures and the disposition rows carry no claim beyond the
offline tuple below.

## The observation in one paragraph

One gateway unit (100) refuses basic device identification with exception 1
(illegal function) and answers its holding-register status reads. One child
unit (1) answers the model block, the serial-bearing block, the version-style
block, the protocol words, basic device identification (vendor and the SUN2000
family) and a one-page extended inventory with one unpadded entry. The
neighbouring units 2 and 3 answer exception 4 and unit 0 stays silent. The
dongle's own search and capacity registers also answer at the child, and the
logger counter, the EMMA model block and the SunSpec marker offsets do not
identify anything there. The first request on a fresh connection timed out once
in both runs and the retry answered.

## The card

`HuaweiSDongleObservedQualificationCard`
(`huawei.qualification.sdongle.observed-readonly@1.0.0`) is separate from the
retained evidence-blocked S-Dongle card, which keeps its identity, its
hard-stop result and its six missing-evidence names. The new card evaluates
already-read evidence and returns `QUALIFICATION_TEST_READY` only for this exact
tuple:

| Element | Exact value |
| --- | --- |
| Gateway unit the gateway reads were addressed to | `100` |
| Basic device identification at the gateway unit | exception `1`; any reply, any other code and an absent read are outside the tuple |
| Search status (FC03 offset 37411, quantity 1) | `0` |
| Protocol words (FC03 offset 30068, quantity 2) | `50`, `0`, compared as words and never decoded |
| Search sequence (FC03 offset 37410, quantity 3) | type `4`, state `0`, and the same three words before and after the child reads |
| Capacity (FC03 offset 37429, quantity 1) | at least `1`, so that it covers the one child |
| Child unit | `1` |
| Child model block (FC03 offset 30000, quantity 15) | the text `SUN2000-8KTL-M1`, a NUL, optional printable residue, NUL padding, in the full 30-byte extent |
| Child basic identification | vendor `Huawei`, product `SUN2000`, a version object that is present; each object once |
| Child extended inventory | one page, read code 3, conformity `0x03`, no continuation, count object `1`, one entry object `0x88` |
| Entry keys | exactly 1 through 6, in any order, each once |
| Entry model (key 1) | equal to the model block text |
| Entry unit (key 5) | `1`, the unit that was read |

Every text is compared byte for byte and case-sensitively after only terminal
NUL or space padding is removed. A one-character change of any element, a
prefix, a range or a case or whitespace variant produces `INSUFFICIENT_EVIDENCE`,
an empty selection and no retained child list. The selected class is `S-Dongle`
and the model variant is empty: the gateway refuses to name its product, and the
child's model never becomes the gateway's.

Never predicates: serial numbers and ESN-class values (the serial-bearing block
is not even an input), register readability (the dongle's own search and
capacity registers answer at the child), the child's firmware text and protocol
words, entry keys 2, 3, 4 and 6, the basic version object and the objects beyond
the basic category (counted as opaque), a child listing, and the unit identifier
of a request. A positive result next to another positive Huawei result is
`AMBIGUOUS` through the shared resolution.

The card consumes answered replies only. The input has no field for a timeout,
an attempt or an elapsed time, so the unanswered first attempt can neither be
required nor forged; the fixture `sdongle_first_request_timing.json` records it.

The replay procedure is `Steps()`: the basic read at the gateway unit, the four
gateway FC03 reads, the child model read, the child basic read, the child
extended inventory from object 0x87 and the sequence read again. Every step is
an FC03 or FC2B / MEI 0x0e read, extended inventory is addressed to the child
only, and no step addresses a unit other than 100 and 1. FC0x41 and FC0x17 stay
no-send. The expected result of replaying the fixtures is
`testdata/huawei/qualification/sdongle_observed_replay_expected.json`, and the
shared status row is in `testdata/huawei/qualification/readiness_expected.json`.

## Tolerances

The child answers in a shape that a strict reader of the logger dialect rejects.
The card tolerates exactly these differences and compares nothing about them:

- The model block holds the model text, a NUL, printable residue and NUL padding.
  The text ends at the first NUL, the residue must be printable ASCII followed by
  NUL padding only, and its length is retained. A block without a terminating
  NUL may be truncated and is not matched.
- The next object identifier is set although no continuation follows; it is
  counted as an anomaly and never compared.
- The declared-object field is retained and counted when it differs from the
  objects present; it is never compared.
- The entry is not NUL padded. Padding is accepted and not required.
- The response stays within the classic 253-byte limit; this child gave 87 bytes.

## Fixtures

`testdata/huawei/identity/` holds seven synthetic re-creations of the sanitized
observation:

| File | Content |
| --- | --- |
| `sdongle_gateway_basic_exception.json` | the illegal-function reply to basic identification at unit 100 |
| `sdongle_gateway_fc03_replies.json` | the status, protocol, sequence (before and after) and capacity reads at unit 100 |
| `sdongle_child_identity_blocks.json` | the child model block with residue, the serial-bearing block, the version-style block, the protocol words, units 2 and 3 (exception 4) and unit 0 (silent) |
| `sdongle_child_basic_reply.json` | the six-object basic reply of the child |
| `sdongle_child_extended_inventory.json` | the one-page inventory with one 75-byte unpadded entry |
| `sdongle_negative_overlap.json` | the SmartLogger, EMMA, S-Dongle and SunSpec probes read at the child and at unit 0 |
| `sdongle_first_request_timing.json` | the timing class of the unanswered first attempt, recorded and not required |

The fixtures use the Huawei-local schema of the SmartLogger slice
(`helianthus-huawei-identity-fixture/v1`) with three optional outcome fields
(`residue`, `sanitized`, `attempts`), two outcome types (`text_with_residue` and
`no_response`) and three fixture kinds (`mei_exception`, `first_request_timing`
and an inventory addressed to one unit). Every earlier fixture decodes
unchanged. The tests rebuild the response bytes from the fixtures and decode them
with a parser that shares no code with the registry; digests are computed and
never stored.

Sanitization replaces every digit with `0`, every upper-case letter with `X` and
every lower-case letter with `x`, keeping punctuation and length, in the
serial-bearing block, the version-style block, the residue after the model text,
the version object of the basic reply, and entry keys 2, 3, 4 and 6. The
sequence word of the search read and the words of the model-offset and
marker-offset reads are synthetic stand-ins, flagged as such. Structure, unit
identifiers, object identifiers, counts, lengths, exception codes, the published
labels and the child model are exact. A test scans every committed file under
`testdata/huawei` for serial-, ESN-, address- and time-shaped values.

## Dispositions

The S-Dongle disposition moves to schema v2. The recorded persistent
non-response stays on record, and its status is superseded for the identity of
the observed tuple only: `observed_identity` carries the exact tuple, the
tolerances, the forbidden identities and the negative-overlap rows, and the
further live I/O rule becomes operator approval at action time. The profile
outcome stays `PRE_LIVE_INSUFFICIENT_EVIDENCE` and monitoring is not admitted.
The child inventory gap closes for the observed tuple. The product string, the
encoding of the protocol words and the EMMA overlap in its reverse direction stay
open, and the first two are closed only in part.

The shared resolution drops a persistent non-response record when a result of
its own class matched, because an exact tuple of that class contradicts it. No
other result drops it: a SmartLogger or EMMA match, a failed S-Dongle card and
an absent S-Dongle result all leave the hard stop in force.

The negative-overlap rows name their fixture, and a test reads each claimed
outcome back out of that fixture. The EMMA, SmartLogger, SUN2000 and LUNA2000
dispositions are unchanged.

## Rollback

The card, the fixtures, the shared-resolution rule and the disposition rows are
referenced only by the tests of this slice. Removing them restores the earlier
disposition and the earlier resolution.
