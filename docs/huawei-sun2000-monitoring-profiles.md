# Huawei SUN2000 monitoring profiles

This page records how the registry implements the SUN2000 part of the public
[Huawei read-only monitoring contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/monitoring-readonly-v1.md)
and its
[read pacing and acquisition contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/read-pacing-v1.md)
(helianthus-docs-modbus issue 242, pull request 253). The contract owns every
protocol fact; this page adds none. Nothing here is stable before Helianthus
1.0.

## Profiles

Each profile is one exact pair: a routed model string behind one observed
gateway identity, and one consulted document edition. The edition is pinned in
the SUN2000 disposition as identity only (see
[vendor documentation identity](huawei-vendor-documentation.md)): title
`SUN2000MC V200R023C00 Modbus Interface Definitions`, version `Issue 01`, date
`2023-03-17`.

| Profile | Family and path | Identity | Edition binding | Read plan (FC03) |
| --- | --- | --- | --- | --- |
| `huawei.sun2000.m3.monitoring-readonly@1.0.0` | SUN2000 M3 series behind the SmartLogger | the observed SmartLogger card matches, the unit is a listed child of the validated listing, and the routed model read at offset 30000 is the exact M3 series model string of the fixtures, with NUL padding | the edition's supported-model table names the model | 32000/16, 32016/24, 32106/10 |
| `huawei.sun2000.m1.monitoring-readonly@1.0.0` | SUN2000 M1 series behind the S-Dongle | the observed S-Dongle card matches, which binds the child model, and the unit is the observed child unit 1 | the edition does not list the model; the contract checked the edition's layout against the child's reading | 32000/16, 32016/24, 32064/32, 32106/10 |

The SUN2000 MAP0 series behind the EMMA has no consulted edition. It has a
disposition row with the outcome `NO_ADMISSIBLE_PROFILE` and the reason
`no_consulted_edition`, and nothing else: no profile, fixture, decoder key or
read plan.

## Capability detection

`SelectHuaweiSUN2000MonitoringProfile` evaluates every profile against one
detached input: the observed gateway evidence, the routed unit, the routed model
read and the edition the caller pins. The gateway card is evaluated by the
registry, never trusted as a result. The listing routes and never identifies:
it must name the unit, and a model it names must agree with the routed read.

| Evidence | Outcome |
| --- | --- |
| a missing or non-matching gateway tuple, a unit outside the listing or other than the observed child, a missing or malformed model read, a listing that disagrees with the routed read | `INSUFFICIENT_EVIDENCE` |
| identity established at a routed unit, but no exact pair admits the model string or the edition (a prefix, another family, a model behind a path it was not observed on, a different title, version or date) | `NO_ADMISSIBLE_PROFILE` |
| more than one exact pair for one endpoint and unit | `AMBIGUOUS`, no winner |
| exactly one exact pair | `PROFILE_SELECTED` |

Firmware is not a predicate: the routed firmware texts differ by family and the
cause of the version gate is Unknown, so no firmware range is claimed. The
version gate is fail closed instead. The power group (32064/32) was refused with
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
paragraph, restores the `NO_ADMISSIBLE_PROFILE` dispositions. No other product
code changes.
