# SunSpec phase one

Issue #9 adds the small public, standard-family surface for a read-only SunSpec
phase-one discovery image. It is vendor-neutral: it makes no manufacturer,
product, unit-identity, endpoint, or transport-family decision.

## API and compatibility

`NewSunSpecPhaseOneProfile` accepts only profile and codec version `1.0.0` and
returns an immutable `ProfileDescriptor`. `NewSunSpecPhaseOneDecoder` accepts
only that standard-family profile. `DiscoveryRequest` describes FC03 holding
register discovery at PDU offset `40000`, the normalized form of documentary
register `40001`. No write or control operation is exported.

`Parse` accepts a bounded raw image (at most 512 words), requires the `SunS`
signature and a zero-length end marker, and validates every declared extent.
It semantically exposes models 1, 101, 102, and 103; it structurally skips
unknown models only after their bounds validate; and it explicitly rejects the
deferred model families. `Int16`, `Acc32`, `ScaleFactor`, and `String` encode
the phase-one signedness, high-word ordering, scale-factor range/sentinel, and
first-NUL fixed-width string rules.

`Activate` does not rebuild observations. It passes the caller-provided
`ObservationSpec` into the existing immutable admission path, so sample, poll,
dependency, wire/logical view, timing, version, and coherence facts remain
validated and retained exactly.

## Evidence, limits, and rollback

The documentary source is the merged M3-01 evidence packet at
`helianthus-docs-ebus` commit
`54d9d178290be8e8cfaac99427e98c64c5ee5136`, specifically
`docs/platform/fronius-sunspec-evidence-v1.md` and its phase-one manifest.
This implementation contains only the standard contract from that packet; it
does not imply qualification of any product or live installation.

The raw-chain 512-word limit is an API validation limit, not a request-size or
scheduler policy. Parsing and activation are memory-only. They open no socket,
perform no framing, and execute no live access. To roll back phase one, remove
the profile from a caller catalog and stop invoking the decoder; retained
observations remain immutable evidence and are not rewritten.
