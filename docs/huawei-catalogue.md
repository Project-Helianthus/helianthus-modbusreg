# Huawei SUN2000 catalogue

`profiles/vendor/huawei/catalogue` holds the Huawei SUN2000 inverter catalogue
as generated registry data: every documented model row with its documented
minimum software, every read-only register row of the sixteen profile cards,
the dictionaries, and the quarantined contradictions. It is generated from the
consulted CC0 inverter reference by `tools/huaweicatalogue` and is checked by a
byte-for-byte round-trip test. The facts come from a reference whose original
tables, keys and labels are dedicated under CC0-1.0; vendor publications keep
their own rights, and the catalogue copies no vendor text, link, locator or
digest (see [vendor documentation identity](huawei-vendor-documentation.md)).

This change adds data only. No code reads the catalogue: it registers no
profile, selects nothing, admits nothing and grants no write. The two SUN2000
monitoring profiles and every disposition are unchanged (see
[SUN2000 monitoring profiles](huawei-sun2000-monitoring-profiles.md)). The
selection predicate (token, identifier and floor), the monitoring profiles built
on the catalogue and any public page text are follow-ups.

## Files

| File | Content |
| --- | --- |
| `editions.json` | one record per consulted edition named by a model row, a card or a quarantine |
| `models.json` | one row per model-row candidate of the reference, plus the 23 model rows of the MB02 card |
| `registers-<card>.json` | the read-only rows of one card and its readable configuration fields |
| `dictionaries.json` | status enumerations, value lists, the storage selector and the primary alarm bits |
| `quarantines.json` | contradictions and anomalies, with the rows that carry each flag |

Every file has a `schema` key (`helianthus-modbusreg-huawei-catalogue-<file>/v1`).
The canonical form puts one top-level key per line and each row of a list on
its own line; decoding and re-encoding a file reproduces its bytes.

## Editions

`id`, `title`, `issue` (`Issue NN` or `not stated`), `date` (`YYYY-MM-DD` or
`not stated`), `family` and `branch` when the title states them,
`source_class` (`vendor_edition`, or `mirrored_edition` for the MB02 edition,
which came from third-party mirrors), the reference `alias` where one exists,
and `card` for the sixteen card editions. Distinct editions that share a title,
an issue and a date carry `copy` (1, 2, ...); only the sidecar tells them apart.
`attribution` carries the attribution note.

## Model rows

| Field | Rule |
| --- | --- |
| `edition`, `page` | the edition and the physical page of the row |
| `form` | `supported_model_row`, or `revision_history_line` for a change-history line among the candidates |
| `token` | the exact source token, punctuation kept |
| `identifier` | the model identifier where the line carries one |
| `minimum_software` | the documented minimum software, a floor, or `not stated` |
| `floor_basis` | `row` (the row's own cell), `reviewed` (a reviewed card or appendix statement completes a cell wrapped beyond the locator line), or `not stated` |
| `wire_token`, `wire_token_grade` | the hyphenated wire form of the MAP0 underscore tokens, graded `HYPOTHESIS` |
| `source_set` | `model_row_locator` (the reference's 2113 candidates) or `card` (the MB02 table) |
| `quarantine` | the identifiers of the quarantines that apply |

The floor is the only software bound. A row states no maximum and no range:
what an edition documents applies from the floor onward, to the builds that
follow, until a recorded Helianthus decision narrows it. Tokens repeat with
different identifiers, so a token alone never identifies a model. One MAP0
series unit answered offset 30000 with hyphens where the source token has
underscores; the wire form is carried for every MAP0 row as a hypothesis.

## Register rows

`key` (the reference's functional key), `endpoint` (`inverter`, `emma`,
`emma-associated-device`; EMMA rows never enter an inverter read map),
`offset`, `quantity`, `type` (the literal source type), `access`, `class`,
`functional_class`, `gates` (named gate identifiers defined in the file's
`gates` list), `condition` (the row-specific condition), `core` (the card's
core key and use gate for the 429 core rows), `page`, `occurrence` (for a row
the source lists twice) and `quarantine`.

| Class | Fields |
| --- | --- |
| `reviewed`, `conditional` | `decode` with the literal `gain` and `unit` cells |
| `raw-only`, `conflict` | `literal` with the same cells, kept as evidence; never decoded |

A blank source cell reads `not stated`; `N/A` and other literals stay as
printed. Readable configuration fields of the MC, MA-V200 and MB02 control
catalogues (local epoch 40000, output mode 42001, time-zone offset 43006,
storage selector 47000) sit in `readable_configuration` with their literal
`RW` access and the same class rules; they are not counted as read-only rows.

## Dictionaries

`id`, `kind` (`status_enum`, `value_enum`, `selector`, `alarm_bits`), `card`,
`edition`, `endpoint`, `offsets`, `pages`, `mode_predicate`, `values` (code and
meaning), `modules` and `bits` for alarm words, and `unlisted` (what an
unlisted code is). Each dictionary is bound to its card edition and is never
transplanted to another edition. The MA-C00 operating status carries only its
off-grid mode predicate: the reference keeps those codes raw.

## Quarantines

`id`, `origin` (`reference` or `generator`), `subject`, `evidence`, `meaning`,
`editions` and `affected` (register rows by card, offset, key and occurrence;
model rows by edition, token, identifier and page). The generator flags rows and
never resolves a quarantine.

| Identifiers | Content |
| --- | --- |
| Q01 to Q07 | the quarantined contradictions of the reference, meaning verbatim |
| A01 to A08 | the source-specific anomalies of the reference (HA and HD string formulas, MB02 35010, PSC110 and optimizer table, MA-V200 32133 to 32137, MC and MA-V200 optimizer table), meaning verbatim |
| M01 to M04 | model rows whose token or floor cell wraps beyond the locator line or is outside the branch form, change-history lines, and tokens without an identifier |
| X01 | register template rows without an address |

## Counts

| Count | Value |
| --- | --- |
| cards | 16 |
| read-only rows | 3115 (372 reviewed, 2127 conditional, 596 raw-only, 20 conflict) |
| core rows | 429 |
| model rows | 2113 candidates and 23 MB02 rows |

The generator also checks each card against the reference's own accounting:
the per-card row counts, the totals of its validation record, the class counts
of the MC, MA-V200 and MB02 reviews, the core rows per card, and the Markdown
tables of MC and MA-V200 cell by cell against their machine-readable twins.

## Regenerating

```bash
go run ./tools/huaweicatalogue -reference <reference root> -sidecar <directory outside the repository>
go run ./tools/huaweicatalogue -reference <reference root> -check
```

`-model-rows` names the model-row locator file (by default next to the
reference). The run is deterministic. It stops when a locator line matches no
known form, when a reviewed statement contradicts a complete floor cell, when a
string fails the publication guard, or when the counts differ from 3115, 429,
2113 and 16; a reviewed reference change is accepted with
`-expect ro=N,core=N,models=N,cards=N`. Locators beyond title, issue, date and
page (digests, table and row positions, native model lines, census lines) go
only to the `-sidecar` directory, which must lie outside the repository.

## Tests

`tools/huaweicatalogue` tests the committed files on every run: canonical
round-trip, counts, class invariants, the floor form, token and identifier
uniqueness per edition, the qualification families (identifier 471 resolves to
MC, 428 to MA-C00 and every MAP0 identifier to MA-V200), flag and affected-row
agreement, and the publication rules. With `HELIANTHUS_HUAWEI_REFERENCE` set to
the reference root, a further test regenerates the catalogue twice and compares
both runs with the committed bytes.

## Rollback

Removing the catalogue directory, `tools/huaweicatalogue`, this page and the
README paragraph, and restoring the vendor directory listing in
`huawei_inverter_battery_disposition_test.go`, restores the registry as it was.
No product code changes.
