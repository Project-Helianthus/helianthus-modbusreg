# Huawei SUN2000 monitoring profiles

This page records how the registry implements the SUN2000 part of the public
[Huawei read-only monitoring contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/monitoring-readonly-v1.md)
and its
[read pacing and acquisition contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/read-pacing-v1.md)
(helianthus-docs-modbus issue 242, pull request 253). The contract owns every
protocol fact; this page adds none. Nothing here is stable before Helianthus
1.0.

## Profiles

Each profile serves one family behind one observed gateway identity. It is
routed by the catalogue row of its family that the selection decides (see
[capability detection](#capability-detection)), and it reads the groups of the
monitoring contract with the layout that the contract checked against one
consulted edition. That edition is pinned in the SUN2000 disposition as
identity only (see [vendor documentation identity](huawei-vendor-documentation.md)):
title `SUN2000MC V200R023C00 Modbus Interface Definitions`, version `Issue 01`,
date `2023-03-17`.

| Profile | Family and path | Identity | Edition binding | Read plan (FC03) |
| --- | --- | --- | --- | --- |
| `huawei.sun2000.m3.monitoring-readonly@1.0.0` | SUN2000 M3 series behind the SmartLogger | the observed SmartLogger card matches, the unit is a listed child of the validated listing, the routed model read at offset 30000 is NUL padded text, and the catalogue selects the M3 series row of the MC card | the edition's supported-model table names the model | 32000/16, 32016/24, 32106/14 |
| `huawei.sun2000.m1.monitoring-readonly@1.0.0` | SUN2000 M1 series behind the S-Dongle | the observed S-Dongle card matches, which binds the child model, the unit is the observed child unit 1, and the catalogue selects the M1 series row of the MA-C00 card | the edition does not list the model; the contract checked the edition's layout against the child's reading | 32000/16, 32016/24, 32064/32, 32106/14 |

The catalogue places the M1 series on the MA-C00 card, whose edition lists its
model and identifier; the observed hand profile keeps the groups and layout
that the monitoring contract checked against the SUN2000MC edition, and the
selection record names the MA-C00 row. The MA-C00 card documents four string
channels. The catalogue profile of that family, below, keeps the hand
identifier and reads the card's own string count.

The SUN2000 MAP0 series behind the EMMA has a catalogue row: its identifier,
reported with the recorded hyphenated wire alias of its underscored source
token, selects the MA-V200 card. That card has a catalogue profile. No EMMA
path input reaches the observed-path selector, which still routes only the two
hand profiles. The disposition row of that observed path records the catalogue
selection, the outcome `NO_ADMISSIBLE_PROFILE` and the reason
`catalogue_card_without_monitoring_profile`; that selector has no hand fixture,
hand decoder key or hand read plan for the card.

## Capability detection

`SelectHuaweiSUN2000MonitoringProfile` evaluates every profile against one
detached input: the observed gateway evidence, the routed unit, the routed model
read and the other routed identity words (the model identifier at 30070, the
software text at 30050, the firmware text at 30035 and the protocol words at
30068), each as read and absent when not read. The gateway card is evaluated by
the registry, never trusted as a result. The listing routes and never
identifies: it must name the unit, and a model it names must agree with the
routed read.

The model row comes from the catalogue decision,
`DecideHuaweiSUN2000CatalogueSelection`, which reads the generated
[catalogue](huawei-catalogue.md) and follows the public
[catalogue page](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/sun2000-inverter-catalogue-v1.md)
and the
[software applicability rule](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/monitoring-readonly-v1.md#software-applicability):

1. **Candidates.** A candidate is a model row of one of the card editions whose
   exact source token, or a wire alias recorded for the row within its scope,
   equals the model text, whose identifier equals the identifier read, and
   whose card map is the scope of the read. The map scope of a card comes from
   its identity rows: the inverter map documents the model text at 30000 and
   the identifier at 30070; the C81 map documents its identifier elsewhere. A
   gateway's own namespace holds no model row. No decision crosses scopes, and
   no punctuation is normalized: the one recorded alias, the hyphenated token
   of a MAP0 row, holds only with that row's identifier in the inverter map.
2. **Floor.** The software text, or the firmware text when no software text was
   read, is compared with the floor of each candidate: the row minimum, or the
   document-wide requirement when the row states none (recorded as a document
   floor). Texts are compared within one branch only (line, release and branch
   equal) and only on the parts both carry; builds of one branch are ordered by
   their SPC number. A text below the floor within its branch removes the row.
   A text of a branch with no documented relation to the floor keeps the row
   with the reason `no_documented_branch_relation`, and a floor outside the
   branch form keeps it with `floor_outside_branch_form`: no text is refused for
   being unseen, and no row has a maximum.
3. **Outcome.** Exactly one remaining row is `SELECTED`, graded Hypothesis until
   an observation grades it, with its card, edition, page, floor, floor basis
   and match kind. Two or more remaining rows are `AMBIGUOUS` and every one is
   named; none is chosen by order, card or position. Anything else is
   `NOT_SELECTED` with a closed reason: `model_text_invalid`, `scope_invalid`,
   `software_malformed`, `no_catalogue_row`, `identifier_mismatch`,
   `scope_mismatch`, `software_absent`, `identifier_absent`, `below_floor` or
   `catalogue_invalid`. Without an identifier no row is selected, because a
   token does not identify a model on its own: the M3 series token is listed
   with identifier 471 on the MC card and with identifier 449 on the MA-C00 and
   MA-C20 cards.

A profile is routed by the selected row of its family on its own gateway path.
A selected row whose family has no profile on that path is
`NO_ADMISSIBLE_PROFILE` with the selection recorded; a path routes and never
decides applicability.

The catalogue decision alone routes. The read-only providers read the model
identifier, the software text and the firmware text at the child's admission on
the SmartLogger and S-Dongle paths (see the
[providers page](huawei-readonly-providers.md#child-admission)), so the
decision over the words read at the routed unit admits or withholds the child.
Without an identifier the catalogue selects nothing and no profile is selected
on the model text alone: the M3 series token is `AMBIGUOUS` across its three
rows, and the M1 series token is `NOT_SELECTED` with `identifier_absent`. The
disposition records the catalogue selection of each family and, as its observed
path, the decision that the provider records when it admits the family's child
in its scripted session.

| Evidence | Outcome |
| --- | --- |
| a missing or non-matching gateway tuple, a unit outside the listing or other than the observed child, a missing or malformed model read, a listing that disagrees with the routed read, an identity word that does not have the shape of its read | `INSUFFICIENT_EVIDENCE` |
| identity established at a routed unit, but the catalogue selects no row (no row, another identifier, no identifier for a token of one row, a malformed or missing software text with the identifier read, a text below the floor), or selects a row of a family without a profile on that path | `NO_ADMISSIBLE_PROFILE` |
| two catalogue rows that the decision cannot tell apart, including a token of several rows without its identifier, or two profiles for one endpoint and unit | `AMBIGUOUS`, no winner |
| exactly one profile routed by a selected row | `PROFILE_SELECTED` |

The decision table of `testdata/huawei/selection/sun2000_catalog_decisions.json`
covers the three qualification families and the floor, alias, identifier and
scope rules, with synthetic software texts; `huawei_sun2000_catalog_selection_test.go`
runs it and the rules on the committed catalogue.

The version gate of the refused groups is a fact per family, not a software
predicate, and it stays fail closed. The power group (32064/32) was refused with
exception 3 on the M3 series and answered on the M1 series, so it is in the M1
hand plan only. The inverter-exposed meter group (37100/38) was refused on the
M3 series and is in no hand plan. The observed hand plans do not adopt the
candidate extents 32064/31 and 32064/33. The catalogue profiles below do read
offset 32064 with quantity 33 where the card documents that block. Every row of
the disposition's decision table is executed by
`TestH05CapabilityDecisionTableRowsAreExecutable`.

## Facts

Each fact carries its decoder key, the string index where it has one, the unit,
an exact decimal (coefficient and exponent), its availability and its grade.
An Unknown fact carries no value, never a zero.

| Field | Offset | Type and unit | M3 series grade | M1 series grade |
| --- | --- | --- | --- | --- |
| `pv_string_voltage`, strings 1 to 8 | 32016 + 2(n − 1) | I16, V, gain 10 | Proven at units 1 and 2 | Proven |
| `pv_string_current`, strings 1 to 8 | 32017 + 2(n − 1) | I16, A, gain 100 | Hypothesis | Proven |
| `input_power` | 32064 | I32, kW, gain 1000 | not read | Proven |
| `active_power` | 32080 | I32, kW, gain 1000; a negative value stays Unknown | not read | Proven |
| `status` | 32089 | U16; only the observed code is published, as `grid_connected`, and every other code stays Unknown | not read | Hypothesis |
| `energy_accumulated` | 32106 | U32, kWh, gain 100 | Proven at units 1 and 2 | Hypothesis |
| `energy_current_hour` | 32112 | U32, kWh, gain 100 | Hypothesis | Hypothesis |
| `energy_current_day` | 32114 | U32, kWh, gain 100 | Hypothesis | Hypothesis |
| `energy_current_month` | 32116 | U32, kWh, gain 100 | Hypothesis | Hypothesis |
| `energy_current_year` | 32118 | U32, kWh, gain 100 | Hypothesis | Hypothesis |

A Proven field is graded Hypothesis at an M3 series unit other than units 1 and
2, which answered identity reads but were not read for monitoring. On the
observed hand profiles the status group yields no fact; its words are retained
as native evidence, as are the string words past string 8, the four words at
32108 where the hand decoder names no numeric source field, and the anchor
words of the power group. The commercial card and the HA and HD cards name that
interval as a hole and name the month and year yields above. Both yields stay
Hypothesis on both hand profiles. The catalogue profiles in the next section
consume the words their cards document, including 32108 where that section
says so.

The supported field name is `status`; the former name is not accepted by
`Fact` or emitted by `DecoderKeys`. Profile namespaces and string indexes stay
part of the exact lookup identity. See the public
[naming contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/naming-contract.md).
The accumulated-energy row remains provisional: the contract does not establish
total accumulation across both families, so its existing key is unchanged.
Proven, Hypothesis and Unknown remain evidence grades, separate from qualification.
This artifact-changing contract requires renewed affected security, hardware and
candidate evidence before release; synthetic replay establishes no qualification.

## Catalogue card profiles

`CatalogMonitoringProfiles` returns one profile for every catalogue card. The
groups are the string, power, energy, alarm, tracker and rating rows the card
documents at the inverter. A row whose class is raw-only or conflict is not
decoded. Aggregate PV voltages that are not string channels are not counted as
strings. The string count is the number of documented string voltage channels.

The MC card keeps the identifier
`huawei.sun2000.m3.monitoring-readonly@1.0.0` and the family SUN2000 M3 series.
The MA-C00 card keeps
`huawei.sun2000.m1.monitoring-readonly@1.0.0` and the family SUN2000 M1 series.
Every other card uses
`huawei.sun2000.<card>.monitoring-readonly@1.0.0`, with the card key in lower
case, and the family name `SUN2000` followed by the card key. Catalogue facts
are documentary, so their grade is Hypothesis.

Where the card documents the power block at offset 32064, that group is read
with quantity 33. It decodes input power, reactive power, the line and phase
voltages, the phase currents, frequency, power factor, efficiency, internal
temperature, insulation resistance and the status code, with the card's types
and gains. A card that does not document offset 32064 keeps the power words it
does document and does not gain an invented read at 32064. Words inside the
read that the card does not define stay undecoded.

The status code uses that card's dictionary. The status names and alarm
labels are compiled into the module, so a built binary does not read the
source tree. The register rows are the card documents the SUN2000 selection
retains from the embedded inverter catalogue. This builder does not open
that catalogue. `CatalogMonitoringProfiles` returns an error when the label
document is missing or does not decode, and it does not return profiles
with empty names. A code the dictionary lists is
published under the dictionary's meaning. A code it does not list stays
Unknown, with no value. One card's dictionary is not applied to another card.
A raw-only status row is not given a meaning.

Active power is the documented signed value. A negative reading stays negative.
The same holds for the other documented signed measurements in the group.

The direct-current input counter at 32108, U32, kWh, gain 100, is a field on
the two hand-family profiles and on every card that documents it. The M1 hand
profile is bound to the SUN2000MC edition, which documents that counter, so the
MA-C00 catalogue profile decodes it inside the energy read at offset 32106,
quantity 14. A card that names the interval as a hole does not gain the field.
On the two hand-family profiles the energy read is offset 32106, quantity 14,
so a refused or unread power group still leaves the counter in the energy group.

Alarm words are read where the card's alarm-bit dictionary defines the word
and the row is reviewed or conditional. A set bit with a dictionary label is
published under that label. A set bit with no label keeps a native name and
grade Unknown. A clear bit is not a fact.

Tracker power and tracker energy are separate groups where the card documents
them, below offset 40000, and predicted tracker power is not included. Rating
rows at the documented nameplate and limit offsets are a group when the card
gives a power unit. A card without one of these classes has no invented group.

A group the unit answers with an exception is a loss for that unit, naming the
unit, the offset and the quantity. The other groups of the same reading still
decode. Battery and meter facts stay unadmitted: those losses remain, and the
holes the hand decoder still leaves unconsumed are unchanged on the hand
profiles.

## Battery and meters

No battery or meter fact is admitted. The LUNA2000 disposition records the
battery groups (37000/30, 37066/4, 47000/1) as present but not admitted: they
answered with data on the MAP0 series (Proven), their meanings are Unknown, and
the rule that a battery offset answers zero at an inverter without a battery is
a Hypothesis. The single reads at 37000 and 47000 answered one zero word on the
M3 and M1 series; such a zero is never published as a state of charge, power or
energy, and a battery offset is outside every read plan. No LUNA2000 profile is
selected, whether or not the parent inverter is identified. Every meter value
stays Unknown.

## Observer and retention

`SUN2000MonitoringSession` exposes one holding-register read and nothing
else. `NewSUN2000MonitoringObserver` accepts only a selection with one
profile. `Observe` reads the plan once, in order: an exception reply is recorded
with its code and the next group is read; a timeout, a transport error or a
reply that does not match the request fails the group and ends the unit's cycle.
`Decode` refuses a group outside the plan, a repeated group and a group whose
words do not fill it. A group that did not answer contributes no fact.

`RetainHuaweiSUN2000Monitoring` keeps, per group, the last complete reading
with its original cycle when a later read of that group did not answer, and
marks the facts it carries as retained. A reading of another profile or unit is
fenced: nothing migrates from it. Pacing, retries, connections and publication
belong to the providers (helianthus-modbusreg issue 349).

## Fixtures

`testdata/huawei/monitoring` holds one fixture per profile, re-created from one
sanitized read-only observation at one daylight operating point: units 1 and 2
behind the SmartLogger, and the child unit behind the S-Dongle. Words are as
read except the replaced ranges: the serial-derived status word and every time
word are zero, and each lifetime energy total is a synthetic value that keeps
the observed ordering of the counters and a conversion ratio inside the
efficiency band. Each reading lists its refused groups and battery zero words
next to the planned groups, and the exact decimal expected for every fact. No
second operating point was observed, so none is invented. The fixtures are
covered by the committed shape scan of `testdata/huawei`.

## Authority

`catalog_registered`, `automatic_runtime_admission`, `live_qualified`,
`support_claim` and `write_authority` stay false. FC0x41 and FC0x17 stay
no-send, and writes stay default denied.

## Rollback

Removing the two monitoring files, their test, the fixtures and this page, and
restoring the SUN2000 and LUNA2000 dispositions, `evidence.json` and the README
paragraph, restores the `NO_ADMISSIBLE_PROFILE` dispositions. Reverting the
catalogue selection (helianthus-modbusreg issue 362) restores the exact routed
model string and the caller-pinned edition as the selection predicate.
