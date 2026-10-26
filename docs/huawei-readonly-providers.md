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
provider-port pages. The contracts own every protocol fact; this page adds
none. Nothing here is stable before Helianthus 1.0.

## One provider per admitted gateway path

`EvaluateHuaweiProviderEnablement` reads the part of a path's disposition that
matters for acquisition, as `HuaweiRecordedProviderDisposition` records it. A
test keeps the recorded values equal to the disposition files. A path is
admitted only when all of the following hold, and refused otherwise:

- the gateway identity is `OFFLINE_IDENTITY_ADMITTED` for the observed tuple
  only, through the path's observed qualification card, and its gate is
  registered;
- at least one child monitoring profile bound to that card is
  `OFFLINE_PROFILE_ADMITTED`;
- a recorded stop, where the path has one, is superseded for the observed
  identity and further live reads need operator approval at action time;
- catalog registration, automatic runtime admission, the support claim and
  write authority are all false.

| Path | Gateway identity | Child profile | Decision | Provider |
| --- | --- | --- | --- | --- |
| SmartLogger | observed card admitted | SUN2000 M3 series admitted | admitted | `HuaweiSmartLoggerReadOnlyProvider` |
| S-Dongle | observed card admitted; recorded stop superseded for that identity | SUN2000 M1 series admitted | admitted | `HuaweiSDongleReadOnlyProvider` |
| EMMA | observed card admitted | SUN2000 MAP0 series `NO_ADMISSIBLE_PROFILE`, reason `catalogue_card_without_monitoring_profile` | refused, `NO_ADMITTED_CHILD_PROFILE` | none |

The SmartLogger and S-Dongle profile outcomes of the gateways themselves stay
`NO_ADMISSIBLE_PROFILE` and `PRE_LIVE_INSUFFICIENT_EVIDENCE`: they describe the
documentary candidates and the gateways' own groups, which no provider reads.
Each admitted path records its provider in a `production_provider` row of its
disposition.

**Why the EMMA path has no provider.** Issue 201 gives no provider to a profile
whose disposition is identity only, and the EMMA path is identity only: its
inverters belong to the MAP0 series, which has a catalogue row and no
monitoring profile, so the monitoring contract consumes none of their words,
and the EMMA's own site groups are planned reads that no production cycle
sends. A scaffold that only identified the gateway would issue live reads that
yield no fact. The gate records the refusal with the MAP0 reason instead, and a
provider follows when a monitoring profile admits the family.

**What the providers build on.** Gateway identity is the observed qualification
card of the path, evaluated over the replies the provider read, with the card's
own inventory parser. Child identity is
`SelectHuaweiSUN2000MonitoringProfile`. The providers read the model text
only, so a child is admitted on its observed path while its identifier is not
read, and the selection records the catalogue decision; reading the identity
words is a change of the provider contract. The read plan, the decoder and the one
last-known-good store are the SUN2000 profile's `Plan`, `Decode` and
`RetainHuaweiSUN2000Monitoring`. The documentary SmartLogger commit owner, the
documentary offline cycles and the supplied-evidence ports validate the
documentary tuples only and refuse the observed tuple, so they stay unchanged
and are not reused.

## Session and lifecycle

The caller supplies a `HuaweiProviderSession` per connection generation: an FC03
holding-register read, an FC2B / MEI 0x0e Read Device Identification read that
returns one decoded page with its response length, declared-object field and
continuation fields, and the close of its own connection. The provider opens
nothing, starts no goroutine and owns no socket. The caller also supplies the
clock and `HuaweiProviderWait`, its cadence.

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
| SmartLogger | the counter, the extended inventory from object 0x87 page by page, the counter again and the optional basic identification, all at unit 0; then, in ascending unit order, the routed model read and the M3 series groups at each listed and allowed unit labelled SUN2000 | the counter at unit 0, then the M3 series groups of each identified child in ascending unit order |
| S-Dongle | the search-sequence read at unit 100, basic identification at unit 100, which must be refused with exception 1, the search status, protocol and capacity reads, the child's model, basic and extended identification at unit 1 and the search-sequence read again; then the M1 series groups at unit 1, including 32064/32 | the search-sequence read at unit 100, then the M1 series groups at unit 1 when the state word is 0 and the change sequence word equals the bound value |

A changed or failed guard sends no child read in that cycle and identifies
again in the next. On the SmartLogger path a bracket whose counters differ, a
malformed listing, or an enumeration longer than the card's deadline is
`listing_unstable`; after three such cycles in a row the endpoint waits ten
cycles. A gateway tuple outside the observed card withholds the whole instance:
no child is read and no child emits a fact. A gateway that stops answering backs
off for one cycle, doubling up to ten.

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
| listed and allowed, label without a class contract (PLC, meter, any label other than SUN2000) | withheld, no read | `no_class_contract` |
| routed model disagrees with the listing | withheld until the next identification | `identity_mismatch` |
| routed read refused, or the read is not a model block | withheld until the next identification | `identity_insufficient` |
| model of a family without an admitted pair | withheld until the next identification | `no_admissible_profile` |
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

`HuaweiProviderCycle` carries the provider identifier and version, the path,
the caller's opaque source label, the cycle number and connection generation,
the start and completion times, the outcome (`COMPLETE`, `INCOMPLETE` or
`INSUFFICIENT_EVIDENCE`) and its reason, the native gateway identity (card,
class, unit, model, firmware or bound child model, state, generation and time
of identification), the children with their routed unit, listed class, family,
profile identifier and version, typed reading and group provenance, every wire
attempt, and the named loss. Its authority methods all return false.

The loss names what is never published: the planned gateway groups, the groups
refused by the family, the status group and the other unconsumed words, every
battery offset, every meter value, each withheld child and each group refused
at runtime. A consumer sees each of them as Unknown, never as zero, empty or
false.

## Read allowlist

The providers send FC03 reads of the listed identity extents and of the profile
read plans, and FC2B / MEI 0x0e reads with read code 1 from object 0 or read code
3 from object 0x87 and its continuations. They never send a write, FC0x41 or
FC0x17. The SmartLogger provider never reads the power group or the
inverter-exposed meter group of the M3 series, a battery offset, or a PLC or
meter unit. The S-Dongle provider addresses units 100 and 1 only.

## Fixtures and tests

`testdata/huawei/provider` holds one scripted session per path. Each step is one
request in the order the provider must send it; its reply is taken from the
sanitized identity and monitoring fixtures, or is a timeout, an exception or a
placeholder marked synthetic, so the fixtures inherit their sources'
sanitization and are covered by the committed shape scan. The `TestH06` tests
replay them and cover acquisition order, pacing against the clock, the read
allowlist, refusal handling, identity-gate failures, stop, reconnect and close,
the retained readings and the enablement gate, whose mutation test varies every
field of the recorded disposition.

## Authority

`catalog_registered`, `automatic_runtime_admission`, `live_qualified`,
`support_claim` and `write_authority` stay false. A provider is acquisition over
a session that the caller has been allowed to open; the operator approval at
action time that the S-Dongle record requires belongs to that caller.

## Rollback

Removing the three provider files, their tests, `testdata/huawei/provider`,
this page, the two `production_provider` rows and the provider lines of
`evidence.json` and the README restores the previous state. No other product
code changes.
