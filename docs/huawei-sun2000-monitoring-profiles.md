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
| `huawei.sun2000.m3.monitoring-readonly@1.0.0` | SUN2000 M3 series behind the SmartLogger | the observed SmartLogger card matches, the unit is a listed child of the validated listing, the routed model read at offset 30000 is NUL padded text, and the catalogue selects the M3 series row of the MC card | the edition's supported-model table names the model | 32000/16, 32016/24, 32106/10 |
| `huawei.sun2000.m1.monitoring-readonly@1.0.0` | SUN2000 M1 series behind the S-Dongle | the observed S-Dongle card matches, which binds the child model, the unit is the observed child unit 1, and the catalogue selects the M1 series row of the MA-C00 card | the edition does not list the model; the contract checked the edition's layout against the child's reading | 32000/16, 32016/24, 32064/32, 32106/10 |

The catalogue places the M1 series on the MA-C00 card, whose edition lists its
model and identifier; the profile keeps the groups and layout that the
monitoring contract checked against the SUN2000MC edition, and the selection
record names the MA-C00 row. The MA-C00 card documents four string channels
and no current-hour energy counter where the profile decodes eight and one;
reconciling a profile with its card belongs to the monitoring profiles built on
the catalogue.

The SUN2000 MAP0 series behind the EMMA has a catalogue row: its identifier,
reported with the recorded hyphenated wire alias of its underscored source
token, selects the MA-V200 card. It has no monitoring profile, and no EMMA path
input reaches the selection. Its disposition row records the catalogue
selection, the outcome `NO_ADMISSIBLE_PROFILE` and the reason
`catalogue_card_without_monitoring_profile`; it has no profile, fixture,
decoder key or read plan.

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

The observed SmartLogger and S-Dongle paths read the model text only. While the
identifier is not read the catalogue selects nothing; a profile stays admitted
on its observed path, with the reason `OBSERVED_PATH_IDENTIFIER_NOT_READ`, when
the model text is the source token of its family and the catalogue keeps that
row among its candidates. Once an identifier is read, the catalogue decision
alone routes, and a disagreeing identifier selects nothing. The disposition
records both the catalogue selection of each family and the decision on its
observed path.

| Evidence | Outcome |
| --- | --- |
| a missing or non-matching gateway tuple, a unit outside the listing or other than the observed child, a missing or malformed model read, a listing that disagrees with the routed read, an identity word that does not have the shape of its read | `INSUFFICIENT_EVIDENCE` |
| identity established at a routed unit, but the catalogue selects no row (no row, another identifier, a malformed or missing software text with the identifier read, a text below the floor), or selects a row of a family without a profile on that path | `NO_ADMISSIBLE_PROFILE` |
| two catalogue rows that the decision cannot tell apart, or two profiles for one endpoint and unit | `AMBIGUOUS`, no winner |
| exactly one profile routed by a selected row, or admitted on its observed path without an identifier | `PROFILE_SELECTED` |

The decision table of `testdata/huawei/selection/sun2000_catalogue_decisions.json`
covers the three qualification families and the floor, alias, identifier and
scope rules, with synthetic software texts; `huawei_sun2000_catalogue_selection_test.go`
runs it and the rules on the committed catalogue.

The version gate of the refused groups is a fact per family, not a software
predicate, and it stays fail closed. The power group (32064/32) was refused with
exception 3 on the M3 series and answered on the M1 series, so it is in the M1
plan only. The inverter-exposed meter group (37100/38) was refused on the M3
series and is in no plan. The candidate extents 32064/31 and 32064/33 stay
Unknown and unplanned. Every row of the disposition's decision table is executed
by `TestH05CapabilityDecisionTableRowsAreExecutable`.

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
| `device_status` | 32089 | U16; only the observed code is published, as `grid_connected`, and every other code stays Unknown | not read | Hypothesis |
| `energy_accumulated` | 32106 | U32, kWh, gain 100 | Proven at units 1 and 2 | Hypothesis |
| `energy_current_hour` | 32112 | U32, kWh, gain 100 | Hypothesis | Hypothesis |
| `energy_current_day` | 32114 | U32, kWh, gain 100 | Hypothesis | Hypothesis |

A Proven field is graded Hypothesis at an M3 series unit other than units 1 and
2, which answered identity reads but were not read for monitoring. The status
group yields no fact; its words are retained as native evidence, as are the
string words past string 8, the DC-input energy counter, the statistics time and
the anchor words of the power group.

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

`HuaweiSUN2000MonitoringSession` exposes one holding-register read and nothing
else. `NewHuaweiSUN2000MonitoringObserver` accepts only a selection with one
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
