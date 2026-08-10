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
signature, Common model 1 first and exactly once, and a zero-length end marker
that consumes all input. It validates every declared extent.
It semantically exposes models 1, 101, 102, and 103; it structurally skips
unknown models only after their bounds validate; and it explicitly rejects the
deferred model families. `Int16`, `Acc32`, `ScaleFactor`, and `String` encode
the phase-one signedness, high-word ordering, scale-factor range/sentinel, and
first-NUL fixed-width string rules.

The semantic coordinates are pinned to official SunSpec models revision
`7abdf8982d5364f8ae916deee18aac86c11be36d`. Common model 1 uses payload
coordinates `Mn 0:16`, `Md 16:32`, `Opt 32:40`, `Vr 40:48`, `SN 48:64`, and
`DA 64`. The exact accepted Common length set is `{65,66}`: the qualified
vendor package uses length 65, while the pinned official model uses length 66
with a non-semantic Pad at payload offset 65. The Pad remains present in raw
words and model extent but is not published as a typed value. Inverter models
101, 102, and 103 use `W 12`, `W_SF 13`, `WH 22:24`
with the high word first, and `WH_SF 24`. No legacy offset fallback is applied.

`Activate` is stateless across polls. Each call takes a
`SunSpecPhaseOneCapture` containing ordered `LogicalViewSnapshot` source views,
derives the exact raw chain from contiguous FC03 views beginning at offset
`40000`, and requires the observation dependency to be the exact captured base
view. It then passes the caller-provided `ObservationSpec` into the existing
immutable admission path, so sample, poll, dependency, wire/logical view,
timing, version, and coherence facts remain validated and retained exactly.

## Evidence, limits, and rollback

The documentary source is the merged M3-01 evidence packet at
`helianthus-docs-ebus` commit
`59218d21163acb868687ed3d8196f0aa1496aab7`, specifically
`docs/platform/fronius-sunspec-evidence-v1.md` and its phase-one manifest.
This implementation contains only the standard contract from that packet; it
does not imply qualification of any product or live installation.

The raw-chain 512-word limit is an API validation limit, not a request-size or
scheduler policy. Parsing and activation are memory-only. They open no socket,
perform no framing, and execute no live access. To roll back phase one, remove
the profile from a caller catalog and stop invoking the decoder; retained
observations remain immutable evidence and are not rewritten.
