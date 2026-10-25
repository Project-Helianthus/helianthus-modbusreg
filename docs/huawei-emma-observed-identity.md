# Huawei EMMA observed identity

This registry slice turns one read-only observation of an EMMA gateway fronting
two SUN2000-family inverters into synthetic fixtures, one inert qualification
card and a revised disposition. It admits an identity tuple offline. It
registers no catalog entry, grants no runtime admission, support claim or write
authority, sends no request, and leaves the profile, monitoring and battery
outcomes where they were.

The protocol facts, their Proven, Hypothesis and Unknown grades and the
exact-tuple semantics are owned by the public documentation, not by this page:

- [gateway contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md),
- [SmartLogger observation](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/smartlogger-observed-inventory-v1.md),
- [EMMA observation](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/emma-observed-identity-v1.md),
- [S-Dongle observation](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/sdongle-observed-identity-v1.md),
- [SUN2000 identity contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/sun2000-observed-identity-v1.md) and
- [qualification readiness](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/qualification-readiness-v1.md).

The contract text for the EMMA observation is the merged companion
documentation change (helianthus-docs-modbus issue 249). That page names
families and withholds model tokens, version texts, serials and residue, and it
leaves the exact comparison to the fixtures of this slice. The card, the
fixtures and the disposition rows carry no claim beyond the offline tuple
below.

## The observation in one paragraph

The gateway unit (0) answers four holding-register identity blocks: a family
label, a model string, a software label (the family label, a space and a
version-style text) and a serial-bearing block. It answers
basic device identification with 14 objects and extended identification in five
responses of one object each, listing four entries: itself at unit 0, an entry
of another device family at unit 2, and two SUN2000-family inverter entries at
units 3 and 4. The two inverter units answer their own model, firmware,
protocol and serial-bearing blocks. Unit 100 answers basic identification with
the gateway's own reply and refuses the S-Dongle registers with exception 4. The
SmartLogger counter register gives exception 3 at the gateway and at an inverter
unit, and the SunSpec marker offset answers words that are not the marker. The
battery group is not listed in the inventory, and no battery unit was addressed.
All 30 requests were answered at the first attempt, the first one included.
Four runs before this observation received no Modbus response; why is unknown.

## The card

`HuaweiEMMAObservedQualificationCard`
(`huawei.qualification.emma.observed-readonly@1.0.0`) is separate from the
documentary EMMA card, which keeps its identity, its steps and its missing
capability evidence. The card evaluates already-read evidence and returns
`QUALIFICATION_TEST_READY` only for this exact tuple:

| Element | Exact value |
| --- | --- |
| Gateway unit the gateway reads were addressed to | `0`; an inverter unit and unit 100 are outside the tuple |
| Family label (FC03 offset 30000, quantity 15) | the text `SmartHEMS`, then NUL padding to the 30-byte extent |
| Model (FC03 offset 30222, quantity 20) | the text `EMMA-A02`, then NUL padding to the 40-byte extent; the other documented alias is outside the tuple |
| Software label (FC03 offset 30035, quantity 15) | the family label, one space and a version-style token, then NUL padding; the three blocks satisfy the documentary EMMA identity gate, so the token meets the branch rule and floor of that gate |
| Basic identification | vendor `Huawei`, product `HEMS`, a version object equal to the token of the software label; each object once; objects beyond the basic category are counted as opaque |
| Extended inventory | read code 3, conformity `0x03`, at most five responses of at most 253 bytes, count object `4`, then four entries |
| Self entry (object `0x88`) | unit `0`, model equal to the model block, product type `HEMS`, version equal to the token |
| Backup-box entry (`0x89`) | unit `2`, product type `BackupBox`; the model is recorded and never compared, and the unit is never read |
| Inverter entries (`0x8a`, `0x8b`) | units `3` and `4`, product type `SUN2000`, a model of the `SUN2000` family |
| Inverter reads (FC03 offset 30000, quantity 15) | one read each at units `3` and `4`: a `SUN2000`-family model string equal to the model of the entry of that unit, then NUL bytes and printable ASCII to the end of the extent |

Every text is compared byte for byte and case-sensitively after only terminal
NUL or space padding is removed. A one-character change of any element, a
prefix, a wildcard, a range or a case or whitespace variant produces
`INSUFFICIENT_EVIDENCE`, an empty selection and no retained child list. The
selected class is `EMMA` and the model variant is the model alias that the
gateway names. The inverter model is compared by its family token and by
equality with its own entry; the rest of the string must be a plain token and
is a placeholder in the fixtures. The card pins the alias and the software
branch that were observed, where the contract admits two aliases and two
branches; the other alias and the other branch are outside the tuple and stay
Unknown.

Never predicates: serial numbers and ESN-class values (neither serial-bearing
block is an input), register readability, the firmware blocks and protocol words
of the inverters, entry keys 3, 4, 6, 9 and 10, key 2 of the entries of units 2
to 4, the model of the unit 2 entry, the objects beyond the basic category, the
page layout, a child listing, and the unit identifier of a request. Basic identification is answered at units
0, 3 and 100 with the same reply, so it never separates the paths by unit. A
positive result next to another positive Huawei result of a different class is
`AMBIGUOUS` through the shared resolution.

The card consumes answered replies only. The input has no field for a timeout,
an attempt or an elapsed time; the fixture `emma_request_timing.json` records
the timing classes.

The replay procedure is `Steps()`: the three identity-block reads and the basic
read at unit 0, the extended inventory from object `0x87`, and the model read at
units 3 and 4. Every step is an FC03 or FC2B / MEI `0x0e` read. The two inverter
units are the ones that the inventory entries name; no step scans, reads the
serial-bearing block or addresses unit 2, unit 100 or any other unit. FC0x41 and
FC0x17 stay no-send. The expected result of replaying the fixtures is
`testdata/huawei/qualification/emma_observed_replay_expected.json`, and the
shared status row is in `testdata/huawei/qualification/readiness_expected.json`.

What stays open for this tuple is listed in the card and in the disposition:
the capability fixtures of the monitoring profile and the addressing of the
battery group.

## Tolerances

The gateway answers in a shape that a strict reader rejects. The card tolerates
exactly these differences and compares nothing about them:

- The declared-object field of every extended response is 5 although each
  response holds one object. It is retained and counted, never compared.
- The page layout is not part of the tuple. The same objects in fewer responses
  are the same inventory, as long as each response stays within the classic
  253-byte frame and the inventory within five responses.
- The count object includes the gateway's own entry.
- The gateway has no change counter: the counter register is refused with
  exception 3, so the inventory is not bracketed by one.
- An inverter model block holds the model text, a NUL, and then NUL bytes and
  printable ASCII to the end of the extent, with no trailing padding required.
  The printable bytes are the residue, retained by length and never compared.
  The block of the S-Dongle path, whose residue follows the first NUL directly,
  is read by the same rule, and a block with no NUL inside the extent may be
  truncated and is not matched. The gateway's own blocks allow no residue.
- Entries are not NUL padded. Padding is accepted and not required, and the key
  order is free. Keys 9 and 10 are known opaque keys; an unknown or repeated key
  rejects the inventory.

## Fixtures

`testdata/huawei/identity/` holds seven synthetic re-creations of the sanitized
observation:

| File | Content |
| --- | --- |
| `emma_gateway_identity_blocks.json` | the family label, the model, the software label and the serial-bearing block at unit 0 |
| `emma_gateway_basic_reply.json` | the 14-object basic reply, answered at units 0, 3 and 100 |
| `emma_extended_inventory.json` | the five extended responses: the count object and four unpadded entries |
| `emma_child_identity_blocks.json` | the model block with its NUL run and residue, the serial-bearing, firmware and protocol blocks, at units 3 and 4 |
| `emma_unit100_behaviour.json` | the three S-Dongle register reads at unit 100, each refused with exception 4 |
| `emma_negative_overlap.json` | the SmartLogger, S-Dongle, EMMA and SunSpec probes read at unit 0 and at an inverter unit |
| `emma_request_timing.json` | the timing classes of the 30 requests, recorded and not required |

The fixtures use the Huawei-local schema of the SmartLogger and S-Dongle slices
(`helianthus-huawei-identity-fixture/v1`). The schema gains one optional outcome
field, `residue_at`, for a residue that starts after a NUL run, and one fixture
kind, `request_timing`. Every earlier fixture decodes unchanged. The tests
rebuild the response bytes from the fixtures and decode them with a parser that
shares no code with the registry; digests are computed and never stored.

Sanitization replaces every digit with `0`, every upper-case letter with `X` and
every lower-case letter with `x`, keeping punctuation and length, in the
serial-bearing blocks, the firmware blocks of the inverters, the residue after
the inverter model, the objects of the basic reply beyond the vendor, product
and version, and entry keys 3, 4, 6 and 10, and key 2 of every entry but the
gateway's. An inverter model keeps its family token and replaces the rest; the
model of the backup-box entry is replaced whole. The gateway's model alias, its
own version token (the one place that the documentary gate needs literally, as
the software label, the version object of the basic reply and key 2 of its
entry), the family labels, the product types, the units, the object
identifiers, the counts, the lengths and the exception codes are exact. The words of the SunSpec and EMMA-model offset reads are synthetic
stand-ins, flagged as such. A test scans every committed file under
`testdata/huawei` for serial-, ESN-, address- and time-shaped values.

## Dispositions

The EMMA disposition was already at schema v2 and moves to schema v3, because
its shape changes: the pointer row that recorded the absence of a response gives
way to `observed_identity`, which carries the exact tuple, the tolerances, the
forbidden identities, the negative-overlap rows, the battery status and the
fixtures. The recorded absence of a response stays on record under
`evidence.earlier_observation` and is superseded for the identity of the
observed tuple only. The profile outcome stays `OFFLINE_IDENTITY_ADMITTED`,
monitoring and batteries are not admitted, the battery group is a LUNA2000
group whose own disposition keeps `NO_ADMISSIBLE_PROFILE`, and its addressing
stays Unknown. The overlap gap with the SmartLogger path closes for the observed
tuple, because both directions are now covered by fixtures; the capability gaps
stay, and the battery addressing joins them. The unit 2 entry is recorded by its
product type, with its model withheld.

The negative-overlap rows name their fixture, and a test reads each claimed
outcome back out of that fixture. The SmartLogger, S-Dongle, SUN2000 and LUNA2000
dispositions are unchanged. The SmartLogger and S-Dongle dispositions still list
their overlap with EMMA as open, although the fixtures of this slice cover the
reverse direction; closing them changes two other cards and is left to a
separate change.

## Resolution

The documentary EMMA card reads the three identity blocks of the fixtures and
reports the identity as known with the capabilities missing; the replay file
records that result next to the observed one. The shared resolution drops that
result when a result of the same class matched the observed tuple, because both
describe one device and the observed one is complete. It drops nothing else: a SmartLogger or S-Dongle match next to
an EMMA match is still `AMBIGUOUS`, two complete EMMA results are still
`AMBIGUOUS`, and the persistent S-Dongle non-response record is still dropped
only by a match of its own class.

## Rollback

The card, the fixtures, the shared-resolution rule and the disposition rows are
referenced only by the tests of this slice. Removing them restores the earlier
disposition and the earlier resolution.
