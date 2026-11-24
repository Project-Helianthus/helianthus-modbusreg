# Huawei read-only providers

This page records how the registry implements the production acquisition
cycle of the public
[Huawei read pacing and acquisition contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/read-pacing-v1.md)
over the groups of the
[Huawei read-only monitoring contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/monitoring-readonly-v1.md)
(helianthus-docs-modbus issue 242, pull request 253), and the production
acquisition sections of the
[SmartLogger](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/smartlogger-provider-port.md),
[EMMA](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/emma-provider-port.md)
and
[S-Dongle](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/sdongle-provider-port.md)
provider-port pages, owned by `helianthus-docs-modbus`. This registry owns
the implementation. The contracts own every protocol fact; this page adds
none. Nothing here is stable before Helianthus 1.0.

## One provider per admitted gateway path

`EvaluateHuaweiProviderEnablement` reads the part of a path's disposition that
matters for acquisition, as `RecordedProviderDisposition` records it. A
test keeps the recorded values equal to the disposition files. A path is
admitted only when all of the following hold, and refused otherwise:

- the gateway identity is `OFFLINE_IDENTITY_ADMITTED` for the observed tuple
  only, through the path's observed qualification card, and its gate is
  registered;
- at least one child monitoring profile bound to that card is
  `OFFLINE_PROFILE_ADMITTED`;
- no recorded stop is in force. The S-Dongle path has none: after a persistent
  non-response its values are reported stale (last value with its age) or
  unknown, never substituted, further reads follow the modbus transport's
  backoff, and no operator approval is asked at action time;
- catalog registration, automatic runtime admission, the support claim and
  write authority are all false.

| Path | Gateway identity | Child profile | Decision | Provider |
| --- | --- | --- | --- | --- |
| SmartLogger | observed card admitted | SUN2000 M3 series admitted | admitted | `SmartLoggerReadOnlyProvider` |
| S-Dongle | observed card admitted; recorded stop superseded for that identity | SUN2000 M1 series admitted | admitted | `SDongleReadOnlyProvider` |
| EMMA | declared gateway identity checked at unit 0 | catalogue profiles for covered children; uncovered children stay native | admitted | `EMMAReadOnlyProvider` |

The SmartLogger and S-Dongle profile outcomes of the gateways themselves stay
`NO_ADMISSIBLE_PROFILE` and `PRE_LIVE_INSUFFICIENT_EVIDENCE`: they describe the
documentary candidates, not the plant readings in the production cycles.
The [EMMA site cycle](emma-offline-provider-cycle.md#read-only-site-cycle)
retains its separate site groups beside the meter groups.

**What the providers build on.** Gateway identity is the observed qualification
card of the path, evaluated over the replies the provider read, with the card's
own inventory parser. Child identity is
`SelectHuaweiSUN2000MonitoringProfile` over the identity words that the child's
admission reads at its routed unit, so the catalogue decision decides the live
path (see [Child admission](#child-admission)). The read plan, the decoder and the one
last-known-good store are the SUN2000 profile's `Plan`, `Decode` and
`RetainHuaweiSUN2000Monitoring`. The documentary SmartLogger commit owner, the
documentary offline cycles and the supplied-evidence ports validate the
documentary tuples only and refuse the observed tuple, so they stay unchanged
and are not reused.

## Session and lifecycle

The caller supplies a `ProviderSession` per connection generation: an FC03
holding-register read, an FC2B / MEI 0x0e Read Device Identification read that
returns one decoded page with its response length, declared-object field and
continuation fields, and the close of its own connection. The provider opens
nothing, starts no goroutine and owns no socket. The caller also supplies the
clock and `ProviderWait`, its cadence.

| Call | Effect |
| --- | --- |
| construct | refuses unless the gate admits the path; refuses a missing wait, a source label that is not short printable text, and an allowlist with unit 0, a unit above 247 or a repeated unit; the S-Dongle allowlist must be exactly the observed child unit |
| `Start` | binds the first connection generation |
| `Refresh` | runs one cycle; refuses without a request a concurrent refresh, a refresh before `NextRefreshAt` and a refresh without a session |
| `Stop` | closes the connection and fences a running cycle; retained readings stay as history |
| `Reconnect` | closes the bound connection first, then binds the new one as a new generation |
| `Close` | terminal; closes the connection; a late result of a running cycle is discarded |
| `LastComplete` | a detached copy of the last cycle whose outcome is complete |

No lock is held while the session is called. A stop, a reconnect or a close
makes a running cycle send nothing further and commit nothing. A cancelled
cycle and a cycle stopped by a pacing violation commit no reading either.

## The cycle

| Path | First cycle of a generation, and after a guard change or failure | Cycle with the guard held |
| --- | --- | --- |
| SmartLogger | the counter, the extended inventory from object 0x87 page by page, the counter again and the optional basic identification, all at unit 0; then the unit 0 plant groups, followed in ascending unit order by the admission of each listed and allowed unit labelled SUN2000 (the routed model read, the model identifier, the software text and the firmware text) and the M3 series groups of each child it admits | the counter at unit 0, then the unit 0 plant groups, followed by the M3 series groups of each identified child in ascending unit order; a child that a failed read left unadmitted runs its admission again first |
| S-Dongle | the search-sequence read at unit 100, basic identification at unit 100, which must be refused with exception 1, the search status, protocol and capacity reads, the gateway software read, the child's model, basic and extended identification at unit 1 and the search-sequence read again; then the unit 100 plant groups, followed by the child's admission at unit 1 (the model identifier, the software text and the firmware text) and, once it admits the child, the M1 series groups at unit 1, including 32064/32 | the search-sequence read at unit 100; when the state word is 0 and the change sequence word equals the bound value, the unit 100 plant groups, followed by the M1 series groups at unit 1; a child that a failed read left unadmitted runs its admission again first |

The S-Dongle provider supplies the raw bytes of one gateway software read
(FC03 offset 30050, quantity 15, at gateway unit 100) to the existing
[observed identity card](huawei-sdongle-observed-identity.md) and
[gateway catalogue selection](huawei-catalogue.md#gateway-selection), inside
the same generation and search bracket. It uses the existing pacing and bounded
attempt policy. A failed or refused read, missing or invalid text, or a
catalogue refusal withholds admission; child software never substitutes for
gateway software. The read runs at identification, including reidentification
after a guard change or failure and after reconnect, and is absent from an
ordinary refresh with the guard held. This supplies existing native evidence;
it establishes no firmware mapping, new profile or physical qualification.
The catalogue's Proven/Hypothesis/Unknown grades remain separate from
qualification and freshness; physical acceptance of this provider change is
Unknown.

This acquisition and admission change requires a companion public documentation
and evidence update in
[helianthus-docs-modbus](https://github.com/Project-Helianthus/helianthus-docs-modbus).
The existing gateway contract alone does not complete that gate. Registry pull
request [#369](https://github.com/Project-Helianthus/helianthus-modbusreg/pull/369)
remains blocked from merging until its companion PR is linked, its applicable
checks and blocker review are clear, and it merges first or together with the
registry change.

A changed or failed guard sends no child read in that cycle and identifies
again in the next. On the SmartLogger path a bracket whose counters differ, a
malformed listing, or an enumeration longer than the card's deadline is
`listing_unstable`; after three such cycles in a row the endpoint waits ten
cycles. A gateway tuple outside the observed card withholds the whole instance:
no child is read and no child emits a fact. A gateway that stops answering backs
off for one cycle, doubling up to ten.

## Gateway plant groups

Plant reads are FC03 at the gateway unit, not at a routed child. They run after
successful gateway identification or a held guard and before child monitoring.
`ProviderCycle.Plant` exposes the two groups for the path, in read order. Each
group contains its unit, offset and quantity, read outcome, native words, and
facts carrying their register offset, exact decimal value and physical unit.

| Path and unit | Group | Facts (key, offset, type, unit, gain) |
| --- | --- | --- |
| SmartLogger, 0 | 40521/6 | `plant_dc_input_power` (40521, U32, kW, 1000); `plant_ac_power` (40525, I32, kW, 1000) |
| SmartLogger, 0 | 40560/4 | `plant_total_energy` (40560, U32, kWh, 10); `plant_daily_energy` (40562, U32, kWh, 10) |
| S-Dongle, 100 | 37498/8 | `plant_dc_input_power` (37498, U32); `plant_consumption_power` (37500, U32); `plant_grid_exchange_power` (37502, I32); `plant_battery_power` (37504, I32); all kW, gain 1000 |
| S-Dongle, 100 | 37516/2 | `plant_ac_power` (37516, I32, kW, 1000) |

Logger 40523 is aggregate CO₂ reduction. Its two words remain in the native
power reply but publish no fact. Neither logger group supplies grid exchange
or battery power. Dongle grid exchange is import-positive and export-negative;
dongle battery power is charging-positive and discharging-negative. These are
native signs, not directions inferred from counters. Plant value forms are
Hypothesis, not hardware-qualified; live response coverage remains Unknown.

A plant group exception is a loss scoped to its gateway unit and exact extent.
An exception 3 marks that group refused and it is not sent again under the
bound identity. The other plant group and child groups are still processed.
A group without an answered read is Unknown unless it has a last-known-good
reading. A retained group keeps the original words, cycle, read time and
connection generation; the outcome describes this cycle, not the retained
read. A gateway identity mismatch fences its old plant values.

## Child admission

A child is admitted at its routed unit by the identity words that the
[catalogue decision](huawei-sun2000-monitoring-profiles.md#capability-detection)
needs: the model text (FC03 30000/15), the model identifier (30070/1), the
software text (30050/15) and the firmware text (30035/15), which is recorded
with the decision and compared with the floor only when no software text is
read. On the SmartLogger path the provider reads all four; on the S-Dongle path
the model text is the child model that the card tuple binds, read before the
bracket closes, and the admission reads the other three. Each read is one
request under the path's gap, deadline and attempt bounds, with one request in
flight. An admission runs on each identification event (the first cycle of a
connection generation and the cycle after a guard change or failure), and on a
later cycle only for a child that a failed read left unadmitted. A cycle with
the guard held reads no identity word of an admitted child, so the admission
adds three reads per child, besides the model read, at admission only.

| Admission | Child | Recorded |
| --- | --- | --- |
| the decision selects the row of the path's family | admitted: its groups are read | `Catalogue` names the row with its card, edition, page, token, identifier, floor, floor basis and floor relation, the grade (Hypothesis), the identifier, the software text it compared and the firmware text |
| the decision selects a row of a family without a profile on the path | withheld until the next identification, `no_admissible_profile` | the selected row |
| the decision selects no row | withheld until the next identification, `catalogue_not_selected` | the closed reason (for example `identifier_mismatch` or `below_floor`) and the rows it considered; the withheld-child loss names the reason |
| the decision cannot tell rows apart | withheld until the next identification, `catalogue_ambiguous` | every candidate row; the withheld-child loss names them |
| a model that disagrees with the listing, a word that does not have the shape of its read, or an exception at any admission read | withheld until the next identification, `identity_mismatch` or `identity_insufficient` | no decision |
| an admission read that times out or fails | unadmitted: a reading of an earlier identity stays stale, otherwise withheld, `child_timeout` or `child_read_failed`; the admission runs again after the child back-off and no identity is guessed | no decision |

The floors of the two owned rows, the MC row of the M3 series and the MA-C00 row
of the M1 series, name a branch without a build number, so no build of their
branch is below them; a build below a floor is exercised with a catalogue row
that states a build floor. The EMMA path has no provider, so nothing is admitted
there.

## Pacing

| Path | Gap | Cycle | Deadline per attempt | Attempts per read | Backoff before a repeat | First request of a generation |
| --- | --- | --- | --- | --- | --- | --- |
| SmartLogger | 750 ms | 60 s | 5 s | 4 | 1, 2, 4 s | no allowance |
| S-Dongle | 5 s | 60 s | 5 s | 4 | 5, 10, 15 s | its first timeout is recorded as `FIRST_REQUEST_TIMEOUT_TOLERATED` and not counted |

Before every attempt the provider computes when the gap (or the backoff before
a repeat) ends, measured from the end of the previous exchange on the same
connection, asks the caller's wait for the remainder and checks the clock
again. A wait that returns early stops the cycle with `ErrHuaweiProviderPacing`
before the next request. A timeout, a transport failure, a malformed reply and
the exception codes 5, 6 and 11 are failed attempts; any other exception is a
definitive reply. A cycle whose read time exceeds three quarters of the cycle
lengthens the interval to the next one; the gap is never shortened. Every
attempt is recorded with its elapsed class: `WITHIN_PROMPT_BOUND` up to 100 ms,
`WITHIN_DEADLINE`, `AT_DEADLINE` for a timeout, or the tolerated first-request
class.

## Children

Routing follows the decision table of the
[gateway contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md).
The listing only limits which identity reads are sent; identity is the routed
read.

| Case | Evidence | Reason |
| --- | --- | --- |
| allowed but not listed | withheld, no read | `not_listed` |
| listed but not allowed | withheld, no read | `not_allowlisted` |
| listed and allowed SmartLogger meter | meter groups at the routed unit; no inverter admission | `read` on success |
| listed and allowed, unhandled class (including PLC) | withheld, no read | `no_class_contract` |
| routed model disagrees with the listing | withheld until the next identification | `identity_mismatch` |
| an admission read refused, or a word that does not have the shape of its read | withheld until the next identification | `identity_insufficient` |
| the catalogue decision selects no row, or cannot tell rows apart | withheld until the next identification | `catalogue_not_selected`, `catalogue_ambiguous` |
| the catalogue selects a row of a family without a profile on the path | withheld until the next identification | `no_admissible_profile` |
| an admission read timed out or failed | unadmitted; admission runs again after the child back-off | `child_timeout`, `child_read_failed` |
| every group sent this cycle answered | current | `read` |
| a group timed out or failed | stale; the remaining groups of the child are not read | `child_timeout`, `child_read_failed` |
| a group answered an exception | stale; exception 3 marks the group refused and it is never sent again | `child_exception` |
| repeated failures | the child alone backs off: none after the first failure, then 1, 2, 4, 8 and at most 10 cycles | `child_backoff` |

A failure of one child never fails a sibling or the gateway and never triggers
a re-enumeration, a scan or a sibling repeat.

## Retention, staleness and fencing

Each child keeps one reading, merged by `RetainHuaweiSUN2000Monitoring`: a group
that did not answer keeps its last complete reading with its original cycle,
and its facts are marked retained. The provider adds, per kept group, the time
of the read that carried it and its connection generation. A child that was not
read in a cycle is shown stale with every kept group retained. A child whose
identity changes moves its reading to `Fenced`, history with its original age,
and starts empty; nothing migrates to the new identity. After a reconnect no
child group is read before the child is identified again on the new
generation.

## Output

`ProviderCycle` carries the provider identifier and version, the path,
the caller's opaque source label, the cycle number and connection generation,
the start and completion times, the outcome (`COMPLETE`, `INCOMPLETE` or
`INSUFFICIENT_EVIDENCE`) and its reason, the native gateway identity (card,
class, unit, model, firmware or bound child model, state, generation and time
of identification), the gateway plant groups described above, the children
with their routed unit, listed class, family, profile identifier and version,
the catalogue decision of their last completed
admission, typed reading and group provenance, every wire attempt, and the named
loss. Its authority methods all return false.

The loss names the groups refused by the family, the status group and the
other unconsumed words, withheld children, and plant, meter and battery groups
refused or failed at runtime. A child with a qualifying card carries
[storage facts](luna-optional-state-reducer.md#battery-values-on-the-provider-cycle)
in its inverter reading; without a documented decoder, EMMA records a
unit-scoped site-only battery loss.
When there is no last-known-good reading, missing facts are Unknown, never
substituted with zero, empty or false.

## Meter readings

`ProviderCycle.Meter` carries native FC03 words, typed facts and a read epoch
for each meter group. SmartLogger reads `32260/33` and `32335/30` at each listed
and allowlisted meter unit after the listing guard. It records `32293/6` and
`33000/7` as refused without requesting them; their evidence carries forward to
later builds as Hypothesis. EMMA adds the built-in sensor groups `31639/20`,
`31661/10`, `31671/4`, `31679/4` and `31687/4` at unit 0. The S-Dongle reads the
inverter-side meter area on the admitted child using `37100/1`, `37101/24`,
`37125/1` and `37126/13`. The status, type and model-check words remain native.

`MeterFactsAdmitted()` is true only for a cycle with newly decoded meter
facts. Retained groups keep their original epoch and a native-only answer does
not count as a decoded fact. Values retain native signs: a documented
export-positive convention is recorded separately; no direction is inferred
from undocumented polarity, undirected counters, or the meter's route. Meter
measurements establish neither identity nor hardware qualification.

## Read allowlist

The providers send FC03 reads of the listed identity extents, including the
model identifier, the software text and the firmware text at an admitted
child's routed unit, and of the plant groups and profile read plans. They send
FC2B / MEI 0x0e reads with read code 1 from object 0 or read code
3 from object 0x87 and its continuations. They never send a write, FC0x41 or
FC0x17. The SmartLogger provider never reads the power group or the
inverter-exposed meter group of the M3 series, an inverter battery offset, or a PLC unit.
No unlisted or non-allowlisted meter unit is read. The S-Dongle provider addresses units 100 and 1 only.

## Fixtures and tests

`testdata/huawei/provider` holds one scripted session per path and one
admission session per path. Each step is one request in the order the provider
must send it. The provider tests insert synthetic plant replies at the gateway
boundary of fixtures that predate plant reads. Each other reply is taken from the sanitized identity and monitoring
fixtures or from a case of the catalogue decision table (its vendor-published
identifiers and synthetic software texts), or is a timeout, an exception or a
placeholder marked synthetic, so the fixtures inherit their sources'
sanitization and are covered by the committed shape scan. Each child's expected
catalogue decision is part of every cycle. The plant tests assert distinct
values per offset, native signs, the CO₂ exclusion, and unit-scoped refusal
with independent retention. The `TestH06` tests replay the fixtures and
cover acquisition order, pacing against the clock, the read allowlist, refusal
handling, identity-gate failures, stop, reconnect and close, the retained
readings and the enablement gate, whose mutation test varies every field of the
recorded disposition. The `TestM89` tests cover the admission: the owned
families selected with their row facts, the identity reads at admission only,
an identity read that times out and is retried, a build of an unrelated branch
admitted as Hypothesis, an identifier of another row, an ambiguous identifier, a
build below the floor, and each identity word varied one at a time.

## Authority

`catalog_registered`, `automatic_runtime_admission`, `live_qualified`,
`support_claim` and `write_authority` stay false. A provider is acquisition over
a session that the caller has been allowed to open; the operator approval at
action time that the S-Dongle record requires belongs to that caller.

## Rollback

Removing the three provider files, their tests, `testdata/huawei/provider`,
this page, the two `production_provider` rows and the provider lines of
`evidence.json` and the README restores the state before the providers. The
child admission alone is reverted by reverting its change, which restores the
model-text-only admission of the selection.

## Catalog spelling

Provider JSON emits `catalog`; selection and provenance fields use `catalog_selection`, `catalog_decision`, `catalog_files`, `catalog_outcome` and `catalog_reason`. Reason values use `catalog_not_selected`, `catalog_ambiguous`, `catalog_invalid`, `catalog_card_without_monitoring_profile` and `no_catalog_row`. The changelog records the direct replacements. These names add no qualification or read authority.
