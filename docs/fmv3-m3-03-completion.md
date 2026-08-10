# FMV3-M3-03 completion decision

FMV3-M3-03 records the phase-one Fronius disposition as `STANDARD_ONLY`. The
immutable public API is `CompletionRecord`; the canonical checked-in JSON is
[`testdata/fmv3-m3-03-completion-v2.json`](../testdata/fmv3-m3-03-completion-v2.json).
Its schema ID is `helianthus.fmv3-m3-03-completion.v2` and its version is `2`.

## Evidence and decision

The record binds the M3-01 documentary evidence SHA
`59218d21163acb868687ed3d8196f0aa1496aab7`, the M3-02 merge SHA
`867c8275c090d3c703a9638548b48ea6846e8c56`, and the official SunSpec-model
SHA `7abdf8982d5364f8ae916deee18aac86c11be36d`.

Those sources support the existing vendor-neutral `sunspec.phase1` profile at
version `1.0.0`. They do not prove a Fronius-specific behavioral delta or a
safe product detector. Accordingly, the canonical record is read-only and
transport-neutral, has no overlay, has no write capability, and cannot perform
automatic product qualification. `OVERLAY_REQUIRED` is retained only as the
second named value in the closed schema vocabulary. It is unrepresentable by
the current v2 constructor and JSON decoder because this contract admits no
overlay or overlay evidence. It remains reserved until a separately versioned,
evidence-qualified overlay contract exists.

## Applicability and limits

Applicability is limited to the qualified documentary GEN24 Primo/Symo ROW
int+SF boundary, and runtime SunSpec chain discovery remains required. The
following remain `UNKNOWN`: Verto, Tauro, older Datamanager, SnapINverter, and
live installations. The record is an evidence conclusion, not a runtime
authorization, transport selector, or product detector.

## Invalidation and rollback

An evidence change invalidates this decision. Rollback retains standard
SunSpec/raw access and has no automatic side effect. A different disposition
requires a separately evidence-qualified record and normal issue/PR review;
this record itself creates no overlay or device action. Evidence SHAs are
provenance references only; they do not authorize a transition or activate a
hash-based workflow.

## Hard stop

This completes FMV3-M3-03. Stop before FMV3-M4-01: no gateway, MCP, add-on,
live-device, credential, Modbus-write, canonical-semantic, or private-binding
work is part of this change.
