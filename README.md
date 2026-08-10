# helianthus-modbusreg

`helianthus-modbusreg` is the single public multi-vendor Modbus profile
registry for Helianthus.

## Status

M2-01 defines immutable profile and observation contracts and consumes the
public M1-06 opaque runtime-acquisition API. Production observations use a
bounded attempt ledger and a caller-supplied transactional publication
boundary. Offline fixture replay is a separate nonpublishable result.

The repository contains no concrete detector, probe plan, qualification
record, standard-family profile, vendor overlay, or vendor fixture. Those
surfaces belong to later M2 milestones and require a separate operator request.

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
  and
- evidence-gated vendor overlay declarations for future milestones.

This repository does not own:

- sockets, serial ports, endpoint scheduling, retries, or transport recovery;
- Modbus TCP/RTU framing or arbitrary protocol operations;
- concrete detection, probe execution, or hardware qualification;
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

The detailed API contract is in [`docs/m2-01-api.md`](docs/m2-01-api.md). The
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
current tree. It enforces the one-registry and M2-01 contracts-only boundary,
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

Completion of M2-01 is a hard stop. Gateway work or another milestone starts
only after a separate operator request.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md).

## License

Implementation code and repository documentation are licensed under
[AGPL-3.0](LICENSE). Independently authored implementation-neutral protocol
facts belong in the public CC0-1.0 documentation lane, not in private fixtures
or bindings. See the
[Helianthus licensing model](https://github.com/Project-Helianthus/.github/blob/main/LICENSING.md).
