# SmartLogger offline inventory commit owner

This application component implements the supplied-page ownership slice of
[the native provider lifecycle contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/smartlogger-provider-lifecycle-v1.md).
That contract and its linked public native sources remain authoritative for
encoding, exact self firmware tuples, counter bracketing and parser ceilings.
This API has no stability guarantee before Helianthus 1.0.

## API and publication boundary

`NewSmartloggerInventoryCommitOwner(context, clock)` creates an explicitly called,
offline in-memory owner. A nil clock uses `time.Now`; an injected clock must be
monotonic and nonblocking. It is called while the publication lock is held.
Construction validates nonempty opaque owner/routing/profile/source context,
nonzero connection generation, logical unit zero, the exact SmartLogger family
and one of the native contract's two exact firmware tuples. These caller-supplied
values correlate evidence; they are not independent proof of a connected parent.
No constructor grants successful production admission.

`Begin(deadline)` freezes the context, a unique owner-local attempt token and the
absolute deadline. The deadline must be strictly after the begin observation and
no later than fifteen seconds after it. A second begin returns
`ErrSmartloggerInventoryBusy` without replacing the attempt or queuing work.
The zero owner refuses attempts. Do not copy owners or attempt handles.

`attempt.Plan()` returns a value copy. `attempt.Commit(plan, input)` accepts only
already-collected counter-before, pages and counter-after from that exact plan.
The caller must begin before collection and must not mutate input during commit.
This component cannot attest acquisition timing or transport provenance.
It compares every frozen field: owner token, attempt token, connection generation,
routing context, logical unit, parent profile, source revision, self family,
firmware and deadline. An altered completion fails the attempt.

Commit calls `ParseHuaweiSmartLoggerOfflineInventory` without changing its native
limits or rejection rules. It separately validates the self entry's exact family
and firmware against the frozen identity. It checks the deadline again after
validation and detachment; reaching the deadline is failure. Only then does it
replace the complete snapshot. Counter changes, malformed late pages, invalid
self identity and exhaustion cannot publish partial children or renew age.

`Cancel`, `Close`, `Retire` and commit share the publication mutex. `Cancel` fixes
one cancelled outcome and releases that attempt's slot. `Close` retires the active
attempt and permanently closes the owner. `Retire(newContext)` requires a valid,
strictly greater connection generation and retires the active attempt before
installing the successor context. A late callback returns its original terminal
result and cannot release a successor's slot. A successful commit remains a
historical success when cancellation, closure or retirement follows it.

Each accepted attempt has one immutable terminal result: `complete`, `failed`,
`cancelled`, `closed` or `retired`. Results are returned by value with the original
plan and completion marker. Failure is deliberately coarse; this slice does not
provide an acquisition-stage or failed-response diagnostic store.

`LastComplete()` returns a detached inventory, native counter/page evidence,
frozen plan, original begin observation and successful completion marker. Native
bytes, object slices and child slices have no mutable aliases to stored state.
Failures retain the entire previous snapshot and both markers. Age is measured
from `ObservedAt`, never from the latest unsuccessful attempt.
`Current` means only that the retained context still matches the open owner.
After generation retirement the old snapshot remains historical with
`Current=false`; only a new complete bracket can replace it. History alone is
not current identity proof. Native evidence can contain sensitive attributes;
runtime retention is separate from public redaction and redistribution.

## Evidence and remaining acceptance

| Evidence label | Claim and limit |
| --- | --- |
| Proven | Synthetic unit sequences exercise exact context fences, atomic replacement, detached buffers, terminal immutability and retained observation history. Public native documentation defines the accepted tuples and ceilings. |
| Hypothesis | Later composition can supply one coherent bracket under independently established transport generation and acquisition provenance. This component does not establish those properties. |
| Unknown | Connected identity, live qualification, production acquisition, transport isolation and end-to-end consumer replay remain unestablished. |

These evidence labels are independent of qualification, acquisition and admission.
Every result and parsed inventory remains default denied. There is no catalog
registration, profile admission, child capability promotion, scheduler, transport
retry or gateway integration. No new authentication or authorization gate is
introduced. Existing trust/admission checks, eBUS MCP compatibility, GraphQL
parity, G5 D1-D5/O8 and driver ownership of the eeBUS output binding are unchanged.

For example, a synthetic complete snapshot at counter 12 survives a later bracket
whose counters are 12 and 13. Its children and original observation marker remain
unchanged. A cancelled attempt's delayed completion cannot release the next
attempt's slot. Neither example establishes device behavior or qualification.

The public docs-modbus companion
[architecture/smartlogger-inventory-commit-implementation.md](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/smartlogger-inventory-commit-implementation.md)
must describe this reviewed API and evidence boundary and cite the native
lifecycle authority before or with merge. That companion is a required merge
dependency, not an assertion that publication has occurred. Application lifecycle
prose belongs outside `protocols/` and `types/`.
This partial slice advances registry issues #298, #206 and #201 without closing
the parents or establishing hardware acceptance.
