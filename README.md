# helianthus-modbusreg

`helianthus-modbusreg` is the single public multi-vendor Modbus profile
registry for Helianthus.

## Status

M2-01 defines immutable profile and observation contracts and consumes the
public M1-06 opaque runtime-acquisition API. M2-02 adds bounded, deterministic,
read-only profile detection over the immutable catalog. M2-03 adds an immutable
transport-neutral corpus for sanitized synthetic fixture replay and cross-profile
conformance. M3-02 adds the bounded, read-only SunSpec phase-one standard-family
profile and parser. M3-03 records its historical evidence-qualified
`STANDARD_ONLY` disposition. The generic SunSpec R1-R3 wave now adds the
bounded model chain and decoder registry, standard Models
1/101-103/111-113/120-124/160/201-204/211-214/302-308, the encoding-neutral three-phase monitoring
capability, and one exact experimental Fronius observed flavor. That flavor is
post-capability classification only: it adds no product qualification,
acquisition activation, support statement, live result, or write authority.
The Fronius qualification card now turns that exact observed flavor into a
bounded FC03 replay procedure with a sanitized expected result. It keeps live
qualification, automatic runtime admission, and write authority false.

## One Registry, Multiple Vendors

Helianthus does not create one repository per inverter vendor. This repository
owns one versioned catalog for standard families and evidence-backed Fronius,
Growatt, Huawei, and future vendor overlays.

A standard family contains only vendor-neutral behavior established by its
public standard. A vendor overlay contains only proven applicability,
normalization, or behavior differences that cannot be represented by the
standard family. Vendor names alone do not justify an overlay.

SmartLogger, EMMA, S-Dongle, model, and firmware distinctions are profile
applicability and qualification dimensions, not separate repositories.

## Ownership

This repository owns:

- versioned profile, codec, dependency-set, normalization, and coherence
  contracts;
- exact replayable source facts and wire-order provenance;
- production ingestion from one exact M1-06 runtime attempt;
- bounded retained attempts, claim entries, diagnostics, terminal sequences,
  and non-reconstructing audit tombstones;
- deterministic restart state for terminal sequences and audit records;
- a transactional publication interface joining the irreversible external
  effect and terminal publication decision;
- strict bounded serialization for contracts, observations, and ledger state;
- bounded read-only probe plans and deterministic qualification-aware profile
  detection;
- strict bounded synthetic fixture corpus serialization, replay, mutation
  classification, and deterministic conformance reports; and
- evidence-gated, versioned vendor flavors that cannot override standard model
  semantics or create acquisition authority.

This repository does not own:

- sockets, serial ports, endpoint scheduling, retries, or transport recovery;
- Modbus TCP/RTU framing or arbitrary protocol operations;
- transport I/O behind the caller-supplied read-only probe interface or
  hardware qualification execution;
- canonical energy/PV publication policy;
- gateway composition or private output bindings.

Transport and runtime authority come from the public
[`helianthus-modbus`](https://github.com/Project-Helianthus/helianthus-modbus)
module, pinned as
`v0.0.0-20260810083147-eab30aed9eb6`. `ObservationFactory` begins and retains
the exact producer attempt from the supplied source and attempt key.
`ObservationAttempt.Issue` passes a successful `LogicalReadView` and its exact
`RuntimeNormalizationRecord` into that retained attempt. `Admit` closes the
private ordered acquisition set; no M1 attempt, instance, acquisition, or
capability is exposed by the registry API. Claims use the retained exact
instance, and cancellation drains it through the retained source.

Fixture bytes use `FixtureReplayer.Replay`. `FixtureReplay` has a fixture
content identity and immutable replay facts, but no runtime capability,
production sample ID, seal, or publish method.

The detailed contracts are in [`docs/m2-01-api.md`](docs/m2-01-api.md),
[`docs/m2-02-detection.md`](docs/m2-02-detection.md), and
[`docs/m2-03-fixture-conformance.md`](docs/m2-03-fixture-conformance.md). The
phase-one API, evidence boundary, limits, compatibility gates, and rollback
are documented in [`docs/sunspec-phase-one.md`](docs/sunspec-phase-one.md). The
M3-03 completion decision, checked-in golden JSON, and hard stop are documented
in [`docs/fmv3-m3-03-completion.md`](docs/fmv3-m3-03-completion.md). The
cross-repository architecture remains documented in the public Helianthus
protocol documentation repository.

## Development

Prerequisites:

- Go 1.22 or newer;
- Python 3; and
- `golangci-lint` v2.11.4 on `PATH` for the complete local gate.

Run:

```bash
./scripts/ci_local.sh
```

The offline gate validates
[`policy/registry-boundary.json`](policy/registry-boundary.json) against the
current tree. It enforces the one-registry public semantic boundary,
the public Modbus dependency, product import restrictions, forbidden ownership
directories, vendor neutrality for future standard-family source, and public
evidence for future vendor overlays. Python mutation tests prove that forbidden
imports and scope leakage fail while ordinary new domain files require no
inventory or hash update.

The policy does not inventory repository files, hash product code, lock a
documentation commit, inspect Git history, call GitHub, or access the network.
Local CI also runs terminology, formatting, vet, build, race tests, and lint.

Work remains one issue and one pull request at a time, with squash merge,
proportionate test-first evidence, applicable public documentation/evidence,
and blocker-driven review of the current head.

Gateway composition, registry dispatch, and live qualification remain separate
milestones outside this repository.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md).

## License

Implementation code and repository documentation are licensed under
[AGPL-3.0](LICENSE). Independently authored implementation-neutral protocol
facts belong in the public CC0-1.0 documentation lane, not in private fixtures
or bindings. See the
[Helianthus licensing model](https://github.com/Project-Helianthus/.github/blob/main/LICENSING.md).
