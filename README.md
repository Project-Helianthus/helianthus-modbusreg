# helianthus-modbusreg

`helianthus-modbusreg` is the single public multi-vendor Modbus profile registry
for Helianthus.

## Status

The repository is bootstrapped but contains no profile, detector, codec, or
vendor implementation yet. APIs and profiles arrive through separately
authorized, test-first issues after their public companion contracts and
evidence packets are merged.

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
- codecs and raw-word interpretation;
- bounded detection plans and fail-closed qualification;
- sanitized fixtures, replay, mutation, and cross-profile conformance;
- standard-family definitions and minimal vendor overlays.

This repository does not own:

- sockets, serial ports, endpoint scheduling, retries, or recovery;
- Modbus TCP/RTU framing or arbitrary protocol operations;
- canonical energy/PV publication policy;
- gateway composition or private output bindings.

Protocol/runtime operations are supplied by
[`helianthus-modbus`](https://github.com/Project-Helianthus/helianthus-modbus)
through a public read interface. A detector may use only operations supported
by that runtime. It may not frame its own PDU.

The normative cross-repository boundary is
[`modbus-multivendor-boundaries.md`](https://github.com/Project-Helianthus/helianthus-docs-ebus/blob/main/docs/platform/modbus-multivendor-boundaries.md).

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
standard-family source, requires an evidence manifest for vendor code, and runs
mutation tests for those boundaries.

The repository follows one issue and one pull request at a time, squash merge,
strict test-first implementation, and applicable documentation/evidence gates.
GitHub protects `main` with required `checks` and `lint` jobs, linear history,
conversation resolution, and disabled merge/rebase commit methods.
See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md).

## License

Implementation code and repository documentation are licensed under
[AGPL-3.0](LICENSE). Implementation-neutral register maps and value semantics
belong in the Helianthus public `CC0-1.0` protocol-documentation lane, not in
private fixtures or bindings. See the
[Helianthus licensing model](https://github.com/Project-Helianthus/.github/blob/main/LICENSING.md).
