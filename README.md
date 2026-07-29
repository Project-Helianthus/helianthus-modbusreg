# helianthus-modbusreg

`helianthus-modbusreg` is the single public multi-vendor Modbus profile registry
for Helianthus.

## Status

M2-01 provides the vendor-neutral profile and source-observation contract. The
repository still contains no detector, qualification record, standard-family
profile, vendor overlay, or vendor fixture. Those surfaces remain locked to
later authorized issues.

## One Registry, Multiple Vendors

Helianthus does not create one repository per inverter vendor. This repository
owns one versioned catalog for SunSpec standard families and evidence-backed
Fronius, Growatt, Huawei, and future vendor overlays.

A standard family contains only behavior established by the applicable public
standard. A vendor overlay contains only evidence-backed detection,
applicability, address-normalization, or behavior differences that cannot be
represented by the standard family. Vendor names alone do not justify an
overlay.

SmartLogger, EMMA, S-Dongle, model, and firmware distinctions are profile
applicability and qualification dimensions, not separate repositories.

## Ownership

This repository owns:

- versioned profile declarations and catalog lookup;
- complete, versioned codec declarations without silent coercion;
- exact ordered dependency-set and documentary address normalization records;
- immutable source-observation snapshots and exact raw-word replay;
- fail-closed coherence, deterministic retry-attempt, and complete physical
  and wire-group validation;
- O(1) factory-issued samples with persisted monotonic attempt identity,
  published only after a consumer-supplied CAS;
- bounded deterministic profile/observation serialization.

The following ownership is planned but is not implemented by M2-01:

- bounded detection plans and fail-closed qualification;
- sanitized fixtures, mutation, and cross-profile conformance;
- standard-family definitions and minimal vendor overlays.

This repository does not own:

- sockets, serial ports, endpoint scheduling, retries, or recovery;
- Modbus TCP/RTU framing or arbitrary protocol operations;
- canonical energy/PV publication policy;
- gateway composition or private output bindings.

Protocol/runtime operations are supplied by
[`helianthus-modbus`](https://github.com/Project-Helianthus/helianthus-modbus)
through its public read interface. M2-01 pins commit
`4f81cbeb6321e64fa51676ed6e375ce36b60d16d` as an immutable Go pseudo-version
and copies successful `LogicalReadView` facts through
`CaptureLogicalView`. The registry never retains a transport owner and cannot
frame its own PDU.

The normative cross-repository boundary is
[`modbus-multivendor-boundaries.md`](https://github.com/Project-Helianthus/helianthus-docs-ebus/blob/main/docs/platform/modbus-multivendor-boundaries.md).
The M2-01 API contract and examples are documented in
[`docs/m2-01-api.md`](docs/m2-01-api.md).

## Development

Prerequisites:

- Go 1.22 or newer;
- `golangci-lint` available on `PATH`.

Run the complete local gate:

```bash
./scripts/ci_local.sh
```

The gate validates the exact machine-readable
[`policy/registry-boundary.json`](policy/registry-boundary.json), rejects
unauthorized project/transport dependencies, prevents named vendor leakage into
standard-family source, requires an evidence manifest for future vendor artifacts,
and runs mutation tests for those boundaries. The M2-01 policy checks the Git
index for regular tracked files, inventories every admitted repository file,
anchors and scans code-bearing non-Go files, and separately hashes all product
Go files. Go import inspection includes product, internal-test, and
external-test imports. This hash gate is an enforced scope/change detector, not
a malicious-committer security boundary. Exact-head adversarial status and
protected-branch enforcement are the external trust boundary. Detector,
qualification, standard/vendor profile, non-Go probe, socket artifact, and
fixture source remain rejected.

The same gate verifies
[`modbus-runtime-consumer-lock-v1.json`](policy/modbus-runtime-consumer-lock-v1.json)
against the downloaded module's VCS origin and validates
[`modbus-companion-consumer-lock-v1.json`](policy/modbus-companion-consumer-lock-v1.json)
against the exact documentation commit
`711a556fee344c6fe7f1ecf3253fcdb3f5f22d06` and manifest digest.

The repository follows one issue and one pull request at a time, squash merge,
strict test-first implementation, and applicable documentation/evidence gates.
GitHub protects `main` with required `checks` and `lint` jobs, linear history,
conversation resolution, and disabled merge/rebase commit methods. A separate
required `adversarial-review` status is emitted only for an exact head that has
a fresh OpenAI-only `NO_FINDINGS` verdict. All protections apply to
administrators.
See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md).

## License

Implementation code and repository documentation are licensed under
[AGPL-3.0](LICENSE). Implementation-neutral register maps and value semantics
belong in the Helianthus public `CC0-1.0` protocol-documentation lane, not in
private fixtures or bindings. See the
[Helianthus licensing model](https://github.com/Project-Helianthus/.github/blob/main/LICENSING.md).
