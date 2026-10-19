# LUNA family supplied-evidence provider port

The public native authority is the
[LUNA optional-group contract](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/luna2000-optional-group-v1.md).
The [native reducer](luna-optional-state-reducer.md) owns interpretation and
retention. This page describes a family-local application port for issue #330,
a partial slice of #199. Nothing is stable before Helianthus 1.0.

Proven: the public native disposition requires `NO_ADMISSIBLE_PROFILE`.
Hypothesis: synthetic supplied cycles exercise the proposed composition.
Unknown: native presence/absence encoding, register applicability, monitoring
values and live behavior. These labels do not imply qualification, freshness,
runtime acceptance or physical acceptance.

## Explicit cycle and frozen generation

`I15NewLunaProvider(generation, plan, supplier)` constructs an
`I15LunaProvider`, implementing `I15LunaProviderPort`. The generation token must
be nonempty. The complete native binding and every native policy field are
fixed for its lifetime. The constructor validates the supplied plan but discards
its event and evidence: parent identity and constructor evidence cannot establish
optional-group presence. Initial state is unknown, with no receipt or history.

`I15Cycle(ctx, started)` invokes exactly one synchronous `I15LunaSupplier` with
a value copy of the frozen plan, empty evidence, a no-observation event and the
explicit start tick. There are no retries, goroutines, schedulers, acquisition,
sockets, transport adapters, native qualification or registration. The supplier
must return bounded data promptly and honor cancellation. Cancellation cannot
forcibly interrupt an arbitrary callback; the wrapper checks it before calling
and again before publication. The native exclusive deadline checks supplied
monotonic completion ticks, not elapsed CPU or wall-clock duration.

A nil supplier, incomplete binding, invalid policy, incoherent association,
invalid plan evidence or invalid time refuses construction without calling the
supplier. A nil/cancelled context, retired or busy generation, or invalid start
refuses a cycle without calling it. Concurrent cycles cannot overlap. Retirement
is permanent; a new binding or policy requires a separately constructed port.
A zero-value concrete port refuses cycles.

The supplier returns `LunaOptionalInput`, preserving all native fields. Parent
and group model, firmware and unit, attachment, both epochs, evidence revision,
all seven policy fields, positive parent identity, association and observation
epochs must match the frozen contract. Returned start must match the request;
completion must be at or after it and before the deadline. Evidence count,
charged bytes, references, outcomes and original observation ticks must satisfy
the native bounds. Rejected results and supplier errors leave the prior view
unchanged. Rejected cycles do not advance retention; a later valid evaluation
ages history under the native reducer. No rejected result is relabelled as a
current observation.

## Detached views and native distinctions

`I15Latest()` returns `I15LunaView`: the generation token, immutable native
state, a detached last published input receipt and `I15HasReceipt`. Supplier
input is detached from the stored plan. Supplied and returned evidence slices
are copied, and native `History()` returns independently detached records.
Owners must not concurrently mutate a supplier's returned slice while the cycle
is copying it. Latest retains one bounded receipt; historical state uses native
count, byte and age eviction. Each evidence record keeps its original receipt
time, binding, reference and native outcome. No group-wide refresh is added.

| Supplied event/outcome | Native interpretation |
| --- | --- |
| Documentary presence with valid references | present-unqualified; all admission predicates false |
| Optional failure with timeout outcome | temporarily unreadable after presence; timeout remains in receipt/history |
| Optional failure with illegal-address outcome | same state transition; distinct original outcome retained |
| No observation with unattempted outcome | unknown; never absent |
| Claimed explicit absence using an unknown event | unknown; original claim remains in latest receipt, without becoming accepted absence evidence in native history |
| Late, cancelled or retired completion | no publication; original receipt/history retained |

For synthetic presence at tick 10 followed by an optional failure at tick 12,
the successful record retains tick 10. Parent identity survives. A supplied
absence claim cannot become observed-absent because the native encoding is
unknown. Qualification, catalog registration, automatic runtime admission,
support claim and write authority remain false for every published state.

## Delivery boundary

This port is neither the common production Provider interface nor a gateway
registry. It adds no eBUS MCP or GraphQL surface; compatibility and parity remain
separate integration acceptance. There is no universal semantic projection and
no numerical substitute for unknown, absent or stale evidence. Raw evidence
payloads remain with their existing supplier; this API carries native references
and outcomes without decoding or dereferencing them.

MERGE HOLD: I16 in `helianthus-docs-modbus` must merge before or with this code.
Independent review, committed-head CI, publication audit, transport conformance,
runtime integration, native qualification, physical acceptance and remaining
parent #199 acceptance are separate and unrun by this slice.
