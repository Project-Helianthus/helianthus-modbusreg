# Source of these files

The six `sequence*` files are byte copies of the gateway's characterization of its first
Wall Connector Gen 3 incarnation. They are compared, never regenerated.

- Repository: gateway (`helianthus-ebusgateway`).
- Source directory: `cmd/gateway/testdata/characterization/tesla_owner_first_incarnation/`.
- Files: `sequence2.ids.json`, `sequence3.ids.json`, `sequence2.canonical.json`,
  `sequence3.canonical.json`, `sequence2.public.json`, `sequence3.public.json`.
- Revision: the gateway revision named by the `source` entry of the docs-semantic index
  `acceptance/vectors/current/gateway-goldens.json`, which is release v0.6.32 plus 496
  commits, committed on 2026-11-18. The index lists the byte digest of each file; the copies
  here equal the indexed bytes.
- Producing test: `TestTeslaCharacterizationOwnerFirstIncarnationLegacyIdentityAndPublicBytes`
  in `cmd/gateway/tesla_hsc_retained_owner_test.go`.

`donor_session.json` is the replay input of the same characterization.

- Repository: docs-semantic.
- Path: `acceptance/vectors/current/sessions/modbus/gen3-managed-first-incarnation/session.json`.
- Revision: the head of docs-semantic main when the copy was taken; the file is the index's
  `input` entry for the case `modbus/gen3-managed-first-incarnation` and is synthetic.

The gateway's MCP and GraphQL renderings of these cuts (`mcp_sequence3.json`, `m2m/evse.json`)
and the Prometheus text are rendered by the gateway's projection code, which this repository
does not contain; they are not copied.
