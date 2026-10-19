# S-Dongle supplied-evidence provider port

`I21SDongleProvider` is a family-specific local composition port over
[`RunSDongleOfflineProvider`](sdongle-offline-provider-cycle.md), implemented by
`I21SDonglePort`. Nothing is stable before Helianthus 1.0. This partial slice
addresses [issue #333](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/333)
and references [parent #201](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/201);
it does not complete production provider acceptance.

The public [native gateway contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md)
is the protocol authority. The [offline cycle contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/sdongle-offline-provider-cycle.md)
and [native route decision](sdongle-route-decision.md) constrain this adapter.
This page establishes application ownership rules, with no new protocol facts,
supported family members or qualification. I22, the public docs-modbus provider
port companion, must merge before or with this code; the merge hold remains.

## API and bounds

```go
type I21SDongleProvider interface {
    I21Refresh(context.Context, SDongleRouteKey, time.Time) (SDongleOfflineResult, error)
    I21Snapshot() SDongleOfflineResult
    I21Retire()
}

func I21NewSDonglePort(SDongleOfflineRequest, SDongleOfflineLookup) (*I21SDonglePort, error)
```

Construction freezes the exact request key and native lookup bounds by value.
It validates the request through the merged cycle using an already canceled
context, making zero supplier calls. All native input, size and previous-record
checks remain in force. Optional `Previous` must additionally have exactly the
frozen decision key; it remains caller-supplied history, not verified provenance.
Its mutable references are copied. The initial latest result is blocked, with
only that optional detached history. The construction attempt time is required
by the native request contract and is not an evidence age.

`I21Refresh` requires the exact frozen key and a nonzero local attempt time.
Family, model, firmware, protocol major/minor, role, unit, request, scope,
revision, all ten parent selection fields, child evidence revision,
qualification and both guards participate in equality. Changing association
requires a separate owner with independently supplied evidence. A model match
cannot select a child route, establish parent provenance or qualify a child.

A valid active refresh invokes the native cycle once, which performs at most
one cooperative supplied-record lookup with its existing deadline and size
bounds. Invalid native input or history can refuse before lookup. There are no
retries, child lookups, scheduling, active acquisition or goroutines created by
the port. A concurrent refresh returns `ErrI21Busy` without a supplier call;
there is no queue. Supplier storage must remain coherent while being read and
must honor cancellation and input bounds. An uncooperative supplier cannot be
forcibly stopped; retirement does not wait for it to return.

Nil context, zero attempt time, zero/uninitialized owner, mismatched refresh
key, already canceled context, overlapping work and retirement refuse without
lookup. Key mismatch uses `ErrI21Association`; returned-plan mismatch retains
the native `ErrSDongleOfflineAssociation`. Other native and supplier errors
remain unchanged. Callers inspect the error alongside the envelope.

## Publication and retention

The owner holds one latest documentary result and at most one last complete
success, with no growing attempt log. Each snapshot, refresh result, current
decision and retained record owns independent reference slices. No projection
or native field loss is introduced. The mutex orders state publication and
retirement, and the implementation never holds it across supplier execution.

| Transition | Latest result | Retained record |
| --- | --- | --- |
| Complete exact supplied plan | Native ready/planned, still `DefaultDenied()` | Replaced with the complete decision and supplied local evaluation time |
| Native withholding or no-send | Original blocked/denied decision, reasons and evidence | Prior complete success unchanged |
| Supplier or native cycle error | Blocked, no current decision | Prior complete success unchanged |
| Invalid refresh or busy refusal | Returned blocked envelope; stored latest unchanged | Detached prior history only; no fallback readiness |
| Cancellation/deadline during refresh | Blocked, no late decision | Prior history and original evaluation time preserved |
| Retirement | Blocked, no current decision; future calls refused | Prior history unchanged; in-flight completion cannot replace it |

`I21Retire` is idempotent and cancels cooperative in-flight work. Retirement
wins at publication under the owner lock and returns `ErrI21Retired`; otherwise
parent cancellation/deadline is checked again before publication. These fences
reject a usable-looking late supplier result. Cancellation observed after an
already completed publication does not retroactively rewrite history.

A successful synthetic attempt at local logical time 20 followed by a failed
attempt at time 30 keeps the complete original record evaluated at 20. Its
native key, provenance, guards and historical `Current` assertion remain
unchanged; none asserts current freshness at 30. Partial failed observations
never overwrite last-known-good fields or form a new qualification. A later
complete success replaces the one retained record. Explicit retirement clears
the latest documentary decision without re-dating or relabeling that history.
No source capture clock is created or renewed.

## Evidence and remaining acceptance

| Status | Claim and limit |
| --- | --- |
| Proven | Focused synthetic tests exercise exact association, native validation, one cooperative call, refusal, replacement, retained age, detached copies and completion fences. This is local software evidence only. |
| Hypothesis | A future production composition can consume this family-local port while preserving native evidence and loss. |
| Unknown | Actual child support, acquired freshness, physical behavior and production integration. Absent evidence remains unknown and cannot admit a profile. |

Qualification, freshness, runtime readiness and physical acceptance remain
separate from these evidence statuses. Documentary success always remains
default denied. There is no send callback, common production Provider
interface, M02 gateway registry, automatic registration, transport lifecycle or
native qualification in this port. eeBUS output remains owned by
helianthus-eebusreg. G5 D1-D5/O8, eBUS MCP compatibility and GraphQL parity
remain unchanged; this work adds no output binding or integration claim.

Manual source, claim, transition, loss, example and link-target review supports
this application documentation only. I22 companion agreement, independent
review, committed-head CI and publication audit remain required. Transport,
smoke, passive-device, runtime and hardware acceptance are unrun and are not
implied by focused tests.
