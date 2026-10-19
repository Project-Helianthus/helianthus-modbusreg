# SmartLogger supplied-evidence provider port

`I17SmartloggerProvider` is a family-local application port over the existing
[offline provider cycle](smartlogger-offline-provider-cycle.md) and
[inventory commit owner](smartlogger-inventory-commit.md). The public
[native lifecycle contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/smartlogger-provider-lifecycle-v1.md)
and its linked native sources remain authoritative for protocol facts. This
adapter adds no encoding, parser, pagination, qualification or acquisition rule.
Nothing in this API is stable before Helianthus 1.0.

## API and ownership

`I17NewSmartloggerProvider(identity SmartloggerInventoryContext,
limits SmartloggerOfflineInventoryLimits, clock func() time.Time,
reader SmartloggerOfflineInventoryReader)
(*I17SmartloggerProviderAdapter, error)` validates the limits and delegates
context and reader validation to `NewSmartloggerOfflineProvider`. Invalid
construction invokes no supplier. Context and limits are stored by value; the
selected clock and reader functions are fixed at construction. A nil clock
uses the native default. Clock closures can still observe advancing caller
state: freezing the function does not freeze time or detach a closure's state.
The clock must remain monotonic, nonblocking and safe for the native owner.
Do not copy the adapter. Its zero value and nil receiver refuse refresh.

The interface exposes only existing native result and snapshot types:

| Method | Behavior |
| --- | --- |
| `I17Refresh(ctx context.Context, deadline time.Time) (SmartloggerInventoryResult, error)` | One synchronous `Cycle` with the frozen limits. Concurrent refresh is refused with native busy error; there is no queue, retry or poller. |
| `I17NativeView() (SmartloggerInventorySnapshot, bool)` | Delegates to `LastComplete`; returns detached native history or false for Unknown inventory. |
| `I17Retire(next SmartloggerInventoryContext) error` | Delegates validation of a strictly newer context to native `Retire`, then permanently closes this port. Invalid retirement leaves it usable. A successor needs a new port. |
| `I17Close()` | Idempotent permanent native closure; no wait for supplier completion. |

The adapter reserves one refresh slot without holding its lifecycle mutex across
supplier code. Close and retirement can fence an active cycle. Successful
retirement preserves an already active native attempt's `retired` terminal result
before closing the owner; it does not expose a refresh under the successor
context. A refresh racing retirement before native begin may instead be refused
or closed. Calls after either terminal operation invoke no supplier. An invalid
retirement neither discards history nor cancels an accepted refresh.

The supplier receives the native frozen plan by value. It consumes supplied
records only and must obey the native synchronous, finite work and allocation
contract. It must not mutate returned buffers during validation. Neither the
adapter nor a deadline can forcibly interrupt arbitrary supplier code. There
is no background task, socket, device read, active acquisition or registration.

## Native evidence, transitions and loss

The cycle and commit owner remain the only inventory validation and retention
owners. They retain the exact caller context (OwnerToken, ConnectionGeneration,
RoutingContext, UnitID, ParentProfile, SourceRevision, SelfModel and Firmware),
attempt token, absolute deadline, successful native counter/page evidence and
original observation/completion markers. The port does not construct a second
parser, cycle or last-known-good store, or weaken native input validation.

Only a complete valid cycle replaces history. Incomplete pages, supplier errors,
cancellation, expired deadlines and invalid native evidence retain the entire
last complete snapshot, without re-dating it. Before the first complete cycle,
inventory is Unknown. A retained view remains detached at the page, object,
byte, child and plan levels. `Current=false` after successful retirement or
closure means historical local context; `Current=true` establishes only an
open local context match. Neither value establishes connected identity or age
qualification. Error categories and failed attempt details remain as coarse as
the native cycle; the adapter adds no failed-response diagnostic store.

For example, a synthetic complete bracket at counter 24 survives a later
incomplete bracket, a callback returning valid-looking data together with an
error, and a callback cancelled before return. All retain the original
`ObservedAt` and `CompletedAt`. If retirement wins while a callback is blocked,
its late data cannot replace that history, and this port refuses further refresh.
These examples demonstrate local supplied-evidence mechanics only.

Every native result and inventory stays `DefaultDenied() == true`. Inventory
completion is not qualification, admission, child capability promotion or write
authority. The port retains native evidence without semantic projection; later
consumers must state any omitted evidence and loss separately.

## Evidence and delivery

| Evidence label | Claim and limit |
| --- | --- |
| Proven | Focused synthetic tests exercise the typed port, frozen context/limits/clock selection, independent context fields, native validation, retained markers, detached input/output, busy refusal and terminal lifecycle fences. |
| Hypothesis | Future production composition can supply native records under independently established connected identity and acquisition provenance. |
| Unknown | Runtime integration, transport isolation, live qualification, persistence, physical acceptance and end-to-end consumer replay. |

Evidence labels are separate from qualification, freshness and admission. This
family-specific port is not the common production Provider interface or M02
gateway registry. Existing eBUS MCP compatibility, GraphQL parity, deterministic
consumer envelopes and G5 D1-D5/O8 remain unchanged; eeBUS output remains owned
by helianthus-eebusreg. No new authorization gate is introduced.

This partial slice addresses [issue #331](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/331)
and advances [#206](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/206)
and [#201](https://github.com/Project-Helianthus/helianthus-modbusreg/issues/201)
without closing the parents. **MERGE HOLD:** I18, the public docs-modbus
SmartLogger provider-port companion, must merge before or with this code.
Its reviewed signature, terminal retirement policy and evidence claims must
agree with this implementation. Companion publication and independent review
are not established by these local tests.
