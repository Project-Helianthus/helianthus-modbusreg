# S-Dongle offline route decision

`DecideSDongleRoute(SDongleRoutePlan) SDongleRouteDecision` is an explicitly
called, pure native component. Nothing is stable before Helianthus 1.0.
It produces a documentary request-class decision with no executable bytes,
transport, scheduling, profile registration, admission or output binding.

The [native gateway contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md)
is the protocol authority. The [routing architecture](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/sdongle-provider-routing.md)
defines the decision boundary, and the [qualification disposition](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/sdongle-qualification-disposition-v1.md)
preserves the completed run's hard stop. This page describes the application
API; it establishes no additional protocol facts or qualified devices.

## Input and evidence association

`Key` includes routing family, native exact model and firmware, independent
protocol major/minor, explicit role, unit, request class, scope and opaque source
revision. Family is the S-Dongle routing class, including for a child; it does
not substitute for the selected device's native identity. `Observation` is the
already-decoded gateway observation passed unchanged to the existing evaluator.
For a gateway its native identity must equal the selected key. For a child it
must equal `Key.Child.Parent`, which retains a complete gateway selection.

`Evidence.Key` must equal the whole selected key, including every parent field,
child evidence revision, separate qualification disposition and before/after
inventory guards. Every identity, protocol and allowlist reference requires a
nonempty reference and the same revision as the key. `References` retains
additional native evidence references and must be nonempty with no empty entry.
Child routing additionally requires a route reference at the same revision,
distinct from the identity reference; it cannot be filled by gateway evidence
alone. Child identity, firmware, baseline, unit and explicit qualified route
are independently supplied assertions, not discovered or qualified by this API.
The parent, child and selected revision must agree. Guards must equal each
other and the evaluated gateway guard.

`Current` is an explicit caller assertion. The component has no clock or
freshness acquisition mechanism. A stale assertion, a conflicting candidate,
missing evidence or changed association withholds the decision. Consumers must
mark old evidence stale and re-evaluate when any key, source revision or guard
changes. An immutable result cannot later become current through another read.
No source revision is interpreted as firmware or as a software release.

`Bounds` requires a positive finite deadline no greater than 15 seconds,
explicit `RetryBounded`, and positive page, object and byte limits. The unsigned
`MaxRetries` supplies a finite maximum; zero is valid only with `RetryBounded`.
These are documentary limits, never permission to retry. Child inventory limits
cannot exceed the native contract's 121 pages, 121 objects and 32768 bytes.
The deadline cap is also applied conservatively to gateway decisions. Bounds
never resume a stopped qualification or authorize partial inventory success.

## Outcomes and request scopes

| Outcome | Boundary |
| --- | --- |
| `planned` | Complete coherent evidence fits a documentary request class. `DefaultDenied()` remains true. |
| `no-send` | Timeout, stopped qualification, private function, extended MEI at gateway role or unit 100, or a request outside the explicit scope mapping. |
| `withheld` | Evidence is incomplete, stale, contradictory, mismatched, documentary-only or outside required bounds. |

Known prohibitions are checked first. Otherwise evaluator rejection and evidence
validation precede scope classification; missing evidence never produces a
plan. `Reason` is the route decision reason; `EvaluatorReason` always preserves
the original evaluator reason, including `matched` on a withheld documentary
tuple. An offline match cannot become qualification or runtime admission.
The evaluator's tuple catalog is unchanged. The routing component permits only
the native contract's offline candidate firmware branch, independently checked
against the gateway observation; its documentary-only tuple remains withheld.
Exact tuple spellings remain native source evidence, not family support claims.

| Role | Request class | Scope |
| --- | --- | --- |
| gateway, logical unit 100 | `basic-mei` | `basic-identity` |
| gateway, logical unit 100 | `gateway-fc03` | `protocol-baseline`, `gateway-status`, or `capacity` |
| child, evidenced unit 1 through 247 except 100 | `extended-mei` | `extended-inventory` |

Scope labels refer to the listed native observations, not arbitrary register
ranges or byte encodings. `private-function` and unrecognized request classes
cannot plan. No fallback, inferred unit, scan or alternate family is created.
Gateway decisions require an empty child context. Extended MEI at unit 100
returns no-send even if a caller labels that unit as a child.

## Detached results and synthetic examples

The result retains the selected key, original decoded observation, evidence,
bounds and evaluator reason even on rejection. All fields are values except
`Evidence.References`, whose slice is copied. Mutating an input or returned
record cannot rewrite another result. Callers retain historical last-known-good
observations with their original key and validity; this stateless component
neither overwrites retained state nor merges partial observations into a new
qualification. Partial acquisition and retention remain caller responsibilities.

With complete synthetic evidence at revision `rA`, a gateway basic-identity
request can be planned while remaining default denied. The same evidence with
extended MEI yields no-send. A separately evidenced child at synthetic unit 7
can receive only a documentary extended-inventory decision; changing its guard,
parent or source revision withholds the old selection. A timeout produces
no-send without alternate acquisition. None of these examples asserts an
actual qualified child or successful physical observation.

| Evidence status | Claim | Qualification / acquisition / admission |
| --- | --- | --- |
| Proven | Public contracts constrain gateway and child requests; focused synthetic tests exercise the pure component. | No device qualification, acquisition or runtime admission follows. |
| Hypothesis | Later composition can consume this detached record. | Scheduling, lifecycle and transport integration remain separate work. |
| Unknown | Actual child support, physical behavior and runtime freshness. | Withhold when evidence is absent; no support claim. |

The component adds no authentication or authorization gate. Existing trust and
admission checks, eBUS MCP compatibility and GraphQL parity are unchanged;
future consumers must preserve equivalent native evidence, reasons and stale
state. No consumer integration or parity implementation is claimed here.

## Delivery boundary

Merge is held until the docs-modbus owner publishes
`architecture/sdongle-route-decision-implementation.md` before or with the code
change. That application companion must describe the reviewed API and evidence
boundary and cite `architecture/sdongle-provider-routing.md` as its native
routing authority, retaining the linked native protocol authority. It belongs
outside `protocols/` and `types/`. This is a partial contribution to provider
work; runtime providers, independent review, full committed checks and physical
acceptance remain separate. The existing evaluator discrepancy is separate
work and is not repaired by this component.
