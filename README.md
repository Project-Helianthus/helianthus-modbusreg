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
capability, and exact experimental Fronius GEN24 observed flavors. Flavor
versions 1.0.0, 1.1.0, and 1.2.0 require the same exact manufacturer and model
strings, and each version carries its own exact firmware string and exact
ordered model chain; 1.2.0 shares the 1.1.0 chain and differs only in its
firmware string. Options, serial number, and device address are not compared.
Any other firmware string fails closed as `FIRMWARE_MISMATCH`: there are no
firmware ranges, prefixes, or case folding, and a new tuple needs a new flavor
version.
The flavors are post-capability classification only: they add no product
qualification, acquisition activation, support statement, live result, or
write authority. The Fronius qualification card
`sunspec.qualification.fronius.gen24.float.readonly@1.0.0` turns flavor 1.1.0
into a bounded FC03 replay procedure with a sanitized expected result and stays
bound to that flavor. It keeps live qualification, automatic runtime
admission, and write authority false.

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
The Huawei qualification package now exposes separate inert cards for bounded
SmartLogger and EMMA evidence plus the retained S-Dongle non-response hard
stop. Identity results carry no telemetry, automatic request, catalog, support,
live-qualification, or write authority.
One further SmartLogger card admits only the exact tuple that a read-only
observation established, replayed from synthetic fixtures under
`testdata/huawei/identity`; every other tuple stays insufficient evidence and
the profile outcome stays `NO_ADMISSIBLE_PROFILE`. See
[`docs/huawei-smartlogger-observed-identity.md`](docs/huawei-smartlogger-observed-identity.md).
One S-Dongle card does the same for the single-inverter tuple that a read-only
observation established: the gateway unit refuses basic identification and
answers its status reads, and one child unit answers the model, basic and
extended identification. It supersedes the recorded hard stop for that identity
only; the profile outcome stays `PRE_LIVE_INSUFFICIENT_EVIDENCE`. See
[`docs/huawei-sdongle-observed-identity.md`](docs/huawei-sdongle-observed-identity.md).
One EMMA card does the same for the gateway that a read-only observation
established: the gateway unit names its family label and model, answers basic
and extended identification, and lists two SUN2000-family inverter units that
answer their own model blocks. It supersedes the recorded absence of a response
for that identity only; monitoring and the battery group stay unadmitted. See
[`docs/huawei-emma-observed-identity.md`](docs/huawei-emma-observed-identity.md).
A Huawei monitoring profile may rest on official vendor documentation that is
cited, never copied: it needs a named document, pinned in its disposition as
identity only (title, version, and date, each as the document states it, with no
excerpt, link, or digest), together with observation fixtures from operator
sites. The identity set is optional and empty until the operator supplies it. A
pin changes no outcome, and exact tuples, negative overlap, and fail-closed
admission stay required. See
[`docs/huawei-vendor-documentation.md`](docs/huawei-vendor-documentation.md) and
the
[applicability and licensing policy](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/applicability-and-licensing.md).
Two SUN2000 monitoring profiles implement the public Huawei read-only
monitoring contract for exact pairs only: the M3 series model behind the
observed SmartLogger tuple and the M1 series model behind the observed S-Dongle
tuple, each bound to the one pinned SUN2000MC edition. Capability detection
evaluates the gateway card, the routed unit, the routed model string and the
edition; everything else stays insufficient evidence or `NO_ADMISSIBLE_PROFILE`.
The read plans are bounded FC03 groups that the family answered, a refused
group is never read, and the observer session exposes reads only. The MAP0
series has no consulted edition and no profile, no battery or meter fact is
admitted, and catalog registration, live qualification, support claims and
write authority stay false. See
[`docs/huawei-sun2000-monitoring-profiles.md`](docs/huawei-sun2000-monitoring-profiles.md).
Two read-only providers drive a supplied session through the public Huawei read
pacing and acquisition contract: one for the observed SmartLogger path and one
for the observed S-Dongle path, each admitted by an enablement gate that reads
the recorded dispositions and fails closed. Each cycle runs the observed
gateway card, routes children only to listed and allowed units with a class
contract, reads the admitted SUN2000 groups with the contract's gap, deadline
and attempt bounds, and keeps stale, retained and fenced readings explicit. The
EMMA path gets no provider while its inverter family has no consulted edition.
The providers open no socket and start no goroutine, and they send only FC03
and FC2B / MEI 0x0e reads. See
[`docs/huawei-readonly-providers.md`](docs/huawei-readonly-providers.md).

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

## OutBack AXS candidate disposition

OutBack AXS source selection is `NO_GO` under
[docs-modbus PR #145](https://github.com/Project-Helianthus/helianthus-docs-modbus/pull/145)
and its [source/profile matrix](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-source-profile-matrix-v1.md).
The owner manual revision C declares `64110/282`; application note revision 5
declares `64110/420`. Neither supplies the required public product/firmware
binding and bounded read-operation contract. The historical 282-word constant
is candidate evidence only, and no OutBack layout is registered for decoding.

`NewOutBackAXSReadOnlyDecoder` creates an inert compatibility object. `Decode`
returns `ErrOutBackAXSSourceNotQualified` and an empty model for every input;
`EvaluateSnapshot` returns `NO_GO`, no match, and no selected interface or typed
model. Caller-supplied raw blocks and snapshots remain unchanged as evidence.
Model 64111 remains opaque; its conflicting source lengths do not admit it.
The OutBack API creates no acquisition request and exposes no transport or send
method. Future admission requires the additional public evidence specified by
the matrix and a separately reviewed implementation. This change does not
establish live qualification or product support.

## Tesla FC100 EVSE catalog

The Tesla Gen3 FC100 catalog is limited to the documented
[retained EVSE operation set](docs/tesla-fc100-evse-scope.md).
The Gen3 [read-path acquisition lifecycle](docs/tesla-gen3-read-acquisition.md)
acquires only the five retained observation operations through the
helianthus-modbus production endpoint; it adds no mutation path.
