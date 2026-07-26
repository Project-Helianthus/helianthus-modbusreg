# AGENTS

This repository is part of the Helianthus multi-protocol gateway platform.
Workspace orchestration is governed by the workspace-root `AGENTS.md` and the
cruise-control skills referenced there.

## Repository Rules

1. Work one issue at a time and keep at most one open pull request.
2. Use `issue/<id>-<slug>` branches and squash merge only.
3. Run `./scripts/ci_local.sh` before pushing.
4. Product implementation requires a test-only RED commit observed by CI before
   the implementation commit.
5. Profile, codec, detector, qualification, or exported behavior changes
   require their merged or companion public documentation and evidence gate.
6. React and reply to every review comment; resolve findings with evidence.

## Ownership Invariants

- This is one multi-vendor registry; do not create repositories per vendor.
- Standard families remain vendor-neutral. Overlays contain only proven,
  minimal vendor differences.
- This repository does not own sockets, serial ports, Modbus framing, canonical
  publication policy, gateway composition, or private bindings.
- Profile detection is bounded, read-only, version-aware, and fail-closed.
- Public builds, tests, fixtures, and docs must never require private
  repositories or artifacts.
- Stop before gateway work unless a later execution authorization explicitly
  permits it.
