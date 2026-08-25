# AGENTS.md

## Purpose and ownership

`helianthus-modbusreg` is the single public multi-vendor Modbus profile
registry. It owns isolated vendor profiles, shared profile primitives,
detectors, qualification, native decoding, capability provenance, and explicit
projection loss. It does not own sockets, serial ports, Modbus TCP/RTU framing,
retries, canonical or universal semantics, gateway composition, or private
consumer bindings.

The universal cross-protocol semantic owner is planned `helianthus-semreg`.
That repository is not an active dependency or authority until it exists with a
merged scope. Until then, this registry must keep Modbus-native evidence and
profiles explicit and must not absorb universal semantic ownership.

Do not create vendor-specific repositories. Keep standard families
vendor-neutral and limit overlays to proven vendor differences. A detection
result with multiple eligible matches is `ambiguous/multiple_matches`; score,
priority, registration order, detector order, or vendor name must not choose a
winner. Preserve raw native evidence and distinguish observed, inferred,
qualified, promoted, unsupported, and unknown states. Partial read failures
must retain last-known-good fields where the profile contract permits it rather
than wholesale-replacing state.

## Workflow

1. Reconcile `origin/main`, the working tree, related issues, branches, pull
   requests, reviews, and checks before editing.
2. Use one scoped issue and an `issue/<number>-<slug>` branch created from
   current `origin/main`. Keep unrelated work out of the branch.
3. Add focused RED tests first for profile, codec, detector, qualification, or
   exported-behavior changes where TDD adds evidence; record the observed
   failure before implementation.
4. Run `./scripts/ci_local.sh` and every applicable unit, race, lint, and
   conformance check before pushing. Include exact commands and results in the
   pull request.
5. Open a linked pull request stating scope, native evidence, tests,
   documentation-gate impact, any required companion documentation PR, and
   residual risk.
6. Resolve valid P0-P2 findings, then obtain a fresh exact-HEAD
   `NO_BLOCKING_FINDINGS` review verdict. P3/P4 findings are triaged as fix,
   backlog, or by design.
7. Squash merge only after all applicable checks and gates are green and the
   exact-HEAD blocker review is clear. Verify remote `main`, issue, PR, and
   branch state, then stop at the requested boundary.

## Documentation and evidence gate

Profile, codec, detector, qualification, native-decoding, capability,
projection-loss, or exported-behavior changes require a public documentation
and evidence update in
https://github.com/Project-Helianthus/helianthus-docs-modbus. Open the companion
documentation PR in the same delivery cycle and merge it first or together with
the code PR. A missing, red, or blocking companion gate prevents this repository
from merging.

Documentation-only instruction changes that establish no protocol or profile
claim may record the gate as not applicable. Publish only redacted,
redistributable evidence and keep hypotheses distinct from qualified behavior.

## Safety and public boundaries

Discovery and qualification are explicit read-only allowlists with bounded
retries, version-aware evidence, and fail-closed outcomes. Public builds,
tests, fixtures, and documentation must be self-contained and must not require
private repositories, artifacts, credentials, local network access, or personal
laboratory equipment. Any real installation, credential use, destructive or
irreversible action, safety-relevant control, or live-device write requires
explicit operator confirmation at action time.
