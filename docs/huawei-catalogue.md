# Huawei SUN2000 catalogue

`profiles/vendor/huawei/catalogue` holds the Huawei SUN2000 inverter catalogue
as generated registry data: every documented model row with its floor, every
read-only and control row of the sixteen profile cards, the value-aligned read
extents, the dictionaries, and the quarantined contradictions. It is generated
by `tools/huaweicatalogue` from reference baseline `2026-10-26.1` of the
consulted CC0 inverter reference, and a byte-for-byte round-trip test checks
it. The facts come from a reference whose original tables, keys and labels are
dedicated under CC0-1.0; vendor publications keep their own rights, and the
catalogue copies no vendor text, link, locator or digest (see
[vendor documentation identity](huawei-vendor-documentation.md)).

Applicability: `evidence.json` lists the consulted source as `huawei-pv-inverter-reference-2026-10-26.1`, the Huawei PV inverter Modbus reference at baseline 2026-10-26.1 of 2026-10-26 (CC0-1.0), cited by title, baseline and date; source pages are carried per row in the catalogue, and no digest is published.

This change adds data only. No code reads the catalogue: it registers no
profile, selects nothing, admits nothing and grants no write. The two SUN2000
monitoring profiles and every disposition are unchanged (see
[SUN2000 monitoring profiles](huawei-sun2000-monitoring-profiles.md)). The
selection predicate (token or qualified alias, identifier and floor), the
monitoring profiles built on the catalogue and any public page text are
follow-ups.

## Inputs

The generator reads the deterministic JSON twin of each of the sixteen cards
and the model table of each of the 49 source editions. Every card has a twin,
so no card falls back to Markdown; a card without a twin stops the generator.
Markdown is read only where the baseline keeps a fact nowhere else: the
edition identity of the source appendix, the card list, core counts and
selection notes of the profile directory, the per-card row counts of the
catalogue navigation, the per-edition model counts, the contradiction
appendix, and the off-grid mode statement of the status chapter.

## Files

| File | Content |
| --- | --- |
| `editions.json` | the reference baseline, the attribution note, and one record per source edition |
| `models.json` | the document-wide requirements and one row per model-table row |
| `registers-<card>.json` | the read-only rows, control rows, read extents, gates and classes of one card |
| `dictionaries.json` | value and status enumerations, bitmaps, sign conventions, sentinels, storage selectors and alarm bits |
| `quarantines.json` | contradictions and anomalies, with the rows that carry each flag |

Every file has a `schema` key (`helianthus-modbusreg-huawei-catalogue-<file>/v1`).
The canonical form puts one top-level key per line and each row of a list on
its own line; decoding and re-encoding a file reproduces its bytes.

## Editions

`reference_baseline` names the baseline (`2026-10-26.1`). Each edition has
`id`, `title`, `issue` (`Issue NN` or `not stated`), `date` (`YYYY-MM-DD` or
`not stated`), `family` and `branch` when the title states them,
`source_class` (`vendor_edition`, or `mirrored_edition` for the MB02 edition,
which came from third-party mirrors), the reference `alias` where one exists,
`card` for the sixteen card editions, and `model_rows`. Distinct editions that
share a title, an issue and a date carry `copy`; distinct titles whose
identifier words coincide carry an `alt` suffix. Only the sidecar tells such
editions apart.

## Model rows

| Field | Rule |
| --- | --- |
| `edition`, `page` | the edition and the physical page of the row |
| `token` | the exact source token, punctuation kept |
| `identifier` | the model identifier where the table labels one |
| `sequence_number` | a table ordinal that the edition does not label as a model identifier; such a row has no identifier and carries M01 |
| `row_minimum` | the minimum the row states, or `not stated` |
| `document_requirement` | the document-wide requirement that applies when the row states no minimum |
| `minimum_software`, `floor_basis` | the floor: the row minimum (`row`) or, when the row states none, the linked requirement (`document`) |
| `wire_aliases` | an alias token with its evidence and scope, only where the reference records one |
| `quarantine` | the identifiers of the quarantines that apply |

`document_requirements` lists each requirement with its edition, page, section,
inclusive minimum, component and model tokens. The floor is the only software
bound. A row states no maximum and no range: what an edition documents applies
from the floor onward, to the builds that follow, until a recorded Helianthus
decision narrows it. Tokens repeat with different identifiers, and a wire
alias holds only within its recorded scope; no punctuation is normalized
globally.

## Register and control rows

Read-only rows carry `key` (the reference's functional key), `endpoint`
(`inverter`, `emma`, `emma-associated-device`; EMMA rows never enter an
inverter read map), `offset` or `address_status` (`unspecified_in_source` for
the two rows whose source address is an ellipsis; they are never requested),
`quantity`, `type`, `access`, `class`, `functional_class`, `gates` (defined in
the file's `gates` list), `condition`, `core` (the card's core key and use gate
for the 429 core rows), `page`, `occurrence` (for a row the source lists twice)
and `quarantine`.

| Rows | Class | Fields |
| --- | --- | --- |
| read-only | `reviewed`, `conditional` | `decode` with the literal `gain` and `unit` cells |
| read-only | `raw-only`, `conflict` | `literal` with the same cells, kept as evidence; never decoded |
| control | `readable-configuration` | `literal` only; an RW row with a documented FC03 read and a documented status, readable as raw evidence |
| control | `no-send` | `literal` only; WO, restricted, conflicting or without a documented read; never requested |

A blank source cell reads `not stated`; `N/A` and other literals stay as
printed. `read_extents` lists the documented value-aligned extents: start, end,
quantity, the quantities that end on a field boundary, the field offsets, the
following source hole and its classification. An extent is no snapshot or
response guarantee, and every row contract still applies. `additive_support`
(MA-C00) carries the identity rows that another edition documents for one
listed model.

## Dictionaries

`id`, `kind` (`status_enum`, `value_enum`, `bitmap`, `sign`, `sentinels`,
`selector`, `alarm_bits`), `card`, `edition`, `endpoint`, `offsets`, `pages`,
`mode_predicate`, `values` (code and meaning), `bits` (alarm words by offset or
by word where the source gives no offset), `modules` where the twin states
them, and `unlisted` (what an unlisted code is). Each dictionary is bound to its
card edition and is never transplanted to another edition.

## Quarantines

`id`, `origin` (`reference` or `generator`), `subject`, `evidence`, `meaning`,
`editions` and `affected` (rows by card, offset, key and occurrence; model rows
by edition, token, identifier and page). The generator flags rows and never
resolves a quarantine.

| Identifiers | Content |
| --- | --- |
| Q01 to Q07 | the quarantined contradictions of the reference, meaning verbatim; Q07 flags only the row the reference names as inconsistent |
| A01 to A08 | the source-specific anomalies (HA and HD string formulas, MB02 35010, PSC110 and optimizer table, MA-V200 32133 to 32137, MC and MA-V200 optimizer table), meaning verbatim |
| M01 | model rows numbered by a sequence ordinal, not a model identifier |
| M02 | row minimums that are merged cells or literals outside the branch form |

## Counts

| Count | Value |
| --- | --- |
| cards | 16 |
| read-only rows | 3115 (372 reviewed, 2127 conditional, 596 raw-only, 20 conflict) |
| control rows | 2135 (1910 readable configuration, 225 no-send) |
| core rows | 429 |
| model rows | 2401 in 49 editions (2282 row floors, 119 document floors) |

The generator also checks each card against the baseline's own accounting: the
validation totals, the per-card row counts, the status counts of every twin,
the core rows per card, the per-edition model counts, the equality of each
card's model table with its edition table, and that every row a twin
quarantines is a conflict or no-send row.

## Regenerating

```bash
go run ./tools/huaweicatalogue -reference <reference root> -sidecar <directory outside the repository>
go run ./tools/huaweicatalogue -reference <reference root> -check
```

The run is deterministic. It stops when a twin or a model table is missing or
inconsistent with the stated accounting, when a string fails the publication
guard, or when the counts differ from 3115, 429, 2135, 2401 and 16; a reviewed
reference change is accepted with
`-expect ro=N,core=N,controls=N,models=N,cards=N`. Locators beyond title,
issue, date and page (source identifiers, digests, table and row positions,
census lines) go only to the `-sidecar` directory, which must lie outside the
repository.

## Tests

`tools/huaweicatalogue` tests the committed files on every run: canonical
round-trip, counts, the class rules (no read-only row decodes beyond its class
and no control row decodes), floors only (row or document floor, never a
maximum), sequence ordinals kept apart from identifiers, token and identifier
uniqueness per edition, the qualification families (identifier 471 resolves to
MC, 428 to MA-C00 and every MAP0 identifier to MA-V200, with the hyphenated
wire alias where the reference records it), unspecified addresses, value-whole
read extents, flag and affected-row agreement, and the publication rules. With
`HELIANTHUS_HUAWEI_REFERENCE` set to the reference root, a further test
regenerates the catalogue twice and compares both runs with the committed
bytes.

## Rollback

Removing the catalogue directory, `tools/huaweicatalogue`, this page, the
README paragraph and the catalogue source and applicability line of
`evidence.json`, and restoring the vendor directory listing in
`huawei_inverter_battery_disposition_test.go`, restores the registry as it was.
No product code changes.
