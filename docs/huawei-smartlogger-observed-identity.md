# Huawei SmartLogger observed identity

This registry slice turns one read-only observation of a SmartLogger-family
gateway into synthetic fixtures, one inert qualification card and revised
dispositions. It admits an identity tuple offline. It registers no catalog
entry, grants no runtime admission, support claim or write authority, sends no
request, and leaves the profile and monitoring outcomes where they were.

The protocol facts, their Proven, Hypothesis and Unknown grades and the
exact-tuple semantics are owned by the public documentation, not by this page:

- [gateway contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md),
- [SmartLogger observation](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/smartlogger-observed-inventory-v1.md),
- [qualification readiness](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/qualification-readiness-v1.md),
- [SUN2000 identity contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/sun2000-observed-identity-v1.md) and
- [power-meter contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-power-meter-v1.md).

They are published by the companion documentation pull request
(helianthus-docs-modbus issue 241, pull request 243), which is merged. Five
points that the contract leaves to this registry are tracked in
helianthus-docs-modbus issue 246: the card identity and status row, resolution
among cards of one class, count-object semantics for the documentary branches,
the finite response cap, and the entry padding, unit and ASCII rules.

## The card

`HuaweiSmartLoggerObservedQualificationCard`
(`huawei.qualification.smartlogger.observed-readonly@1.0.0`) is separate from
the documentary SmartLogger card. The documentary card keeps its identity, its
two documentary firmware values and its behavior: applied to the observed
gateway it returns `INSUFFICIENT_EVIDENCE`, as the documentation states.

The card evaluates already-read evidence and returns `QUALIFICATION_TEST_READY`
only for this exact tuple:

| Field | Exact value |
| --- | --- |
| Unit the reads were addressed to | `0` |
| Change counter (FC03 offset 65521, quantity 1) | equal before and after one complete inventory |
| Self-entry model (attribute 1) | `Smart Logger` (one space) |
| Self-entry firmware (attribute 2) | `V300R023C10SPC620` |
| Self-entry unit (attribute 5) | `0`, equal to the unit the reads were addressed to |

Every field is compared byte for byte and case-sensitively. A one-character
change of any field, another family's self entry, the documentary tuples,
a prefix, a range or an ordering produce `INSUFFICIENT_EVIDENCE`, an empty
selection and no retained child list. Serial numbers, ESNs, register
readability, a child listing and the unit identifier of a basic read are never
predicates. A positive result next to another positive Huawei result is
`AMBIGUOUS` through the shared resolution, and the retained S-Dongle hard stop
still dominates every combination.

A decoded basic identification reply is optional corroboration. When supplied,
vendor `HUAWEI`, product `Smart Logger` and a version equal to the self-entry
firmware must all agree; any disagreement, repeated object or missing identity
object is `INSUFFICIENT_EVIDENCE` with the reason `BASIC_IDENTITY_CONFLICT`.
Objects beyond the three basic ones are counted as opaque and never read. An
absent or undecodable reply changes nothing.

The replay procedure is `Steps()`: the counter read, the extended inventory read
from object 0x87, the counter read again and the optional basic read. All four
are FC03 or FC2B / MEI 0x0e reads. FC0x41 and FC0x17 stay no-send. The expected
result of replaying the fixtures is
`testdata/huawei/qualification/smartlogger_observed_replay_expected.json`, and
the shared status row is in `testdata/huawei/qualification/readiness_expected.json`.

## Inventory dialect

`parseHuaweiSmartLoggerObservedInventory` follows the tolerance requirements of
the observation. It differs from the documentary parser only where the
observation fixed a behavior:

- A response may exceed the classic 253-byte PDU limit. The response length is
  carried in the input, must equal the length implied by the objects (a header
  of seven bytes plus two bytes and the value for each object), and must not
  exceed the registry cap. The observation fixes only the lower bound of 263
  bytes. This registry chooses a cap of 1024 bytes: it holds three maximal
  entries and stays far below the aggregate ceiling of 65536 bytes.
- The declared-object field is retained and counted as an anomaly when it is not
  the number of objects present. It is never compared.
- The count object counts the self entry, so the child count is the count minus
  one.
- Attribute keys 9 and 10 are known opaque keys. Key order is free. Any other
  key and any repeated key reject the inventory.
- An entry is at most 254 bytes of printable ASCII followed by terminal NUL or
  space padding. A unit is a canonical decimal number: the self entry names the
  unit the reads were addressed to, every other entry names a distinct unit from
  1 through 247.

The ceilings of 15 seconds, 248 pages, 248 objects, 65536 bytes and 247
children are unchanged, and a failure never leaves a partial child list.

## Fixtures

`testdata/huawei/identity/` holds four synthetic re-creations of the sanitized
observation:

| File | Content |
| --- | --- |
| `smartlogger_extended_inventory.json` | the 13-response paged inventory, including twelve oversize responses and the declared-object field on every response |
| `smartlogger_basic_reply.json` | the basic reply with twelve objects, answered for units 0, 1 and 100 |
| `sun2000_routed_identity.json` | the FC03 identity reads of the eight routed inverter units and the answers of listed and unlisted units |
| `smartlogger_negative_overlap.json` | the EMMA, S-Dongle, SunSpec and counter probes read at the gateway and at a routed unit |

The generic fixture corpus carries FC03 and FC04 register words only, so it
cannot represent a long FC2B / MEI response. These files use a small
Huawei-local schema (`helianthus-huawei-identity-fixture/v1`) that carries the
wire facts; the tests rebuild the response bytes from it and decode them with a
parser that shares no code with the registry. Sanitization replaces every
digit with `0`, every upper-case letter with `X` and every lower-case letter
with `x`, keeping punctuation and length, in attributes 3, 4, 6 and 10 of every
entry, in attribute 2 of the PLC and meter entries, and in the serial-like and
opaque objects of the basic reply. Placeholders are therefore not distinct
between entries. Structure, object identifiers, counts, lengths, units, labels
and the published firmware strings are exact. Digests are computed by the
tests and are never stored.

A test scans every committed file under `testdata/huawei` for serial-shaped,
ESN-shaped, address-shaped and time-shaped values.

## Dispositions

The SmartLogger disposition moves to schema v2. Its identity status is
superseded for the observed tuple (`observed_identity`). Its profile outcome
stays `NO_ADMISSIBLE_PROFILE`, because only part of the overlap gap closes: the
EMMA probes read at the SmartLogger endpoint are rejected, while the SmartLogger
probes at an EMMA endpoint have no evidence. The two gaps that the fixtures
close leave `missing_evidence` and `reopen_requires`; the partly closed gap
stays in both. The three exact tuples are independent entries.

The EMMA, S-Dongle, SUN2000 and LUNA2000 dispositions gain only an
`observation_pointer` row. It names the documentation pages and the fixtures
that bear on them, records the observation state and states that neither the
outcome nor the missing-evidence list changed. No EMMA tuple exists because
the EMMA endpoint returned no Modbus response, and no S-Dongle was reached, so
the persistent non-response hard stop is unchanged.

## Rollback

The card, the parser, the fixtures and the added disposition rows are
referenced only by the tests of this slice. Removing them restores the earlier
dispositions.
