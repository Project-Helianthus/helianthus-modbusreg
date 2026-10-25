# Huawei vendor documentation identity

The Huawei dispositions under `profiles/vendor/huawei` may rest on official
vendor documentation. The public
[applicability and licensing policy](https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/applicability-and-licensing.md)
owns the rule. This page records how the registry keeps it. It adds no profile,
card, fixture, detector, decoder, or admission.

## Policy as the registry keeps it

- Official vendor documentation is documentation that the vendor published
  officially and publicly. A copy from a third party, or a document obtained
  under an agreement or behind an account, is outside the policy.
- A monitoring profile may rest on a named document, together with observation
  fixtures from operator-owned sites. Both are required: the document says what
  to read and how to interpret it, and the fixtures show what a connected unit
  returned. A fact that only a document states is documentary and is never
  graded Proven.
- The vendor keeps the copyright. The registry never copies the document, its
  register table, labels, or prose, and it records no excerpt, link, locator, or
  digest. The public page that describes a device carries the attribution note
  and the Documents consulted section; the registry holds identity only.
- A pin changes no outcome. Exact tuples, executable negative overlap, and
  fail-closed admission stay required, and every disposition stays default
  denied.

## The field set

Each Huawei disposition has an optional `evidence.vendor_documentation` block.
All five dispositions carry it. The SUN2000 disposition pins one consulted
edition, `SUN2000MC V200R023C00 Modbus Interface Definitions`, `Issue 01`,
`2023-03-17`, which binds its two monitoring profiles (see
[SUN2000 monitoring profiles](huawei-sun2000-monitoring-profiles.md)); the
other four keep an empty `consulted_documents` list.

| Key | Content |
| --- | --- |
| `source_class` | `OFFICIAL_VENDOR_DOCUMENTATION_ONLY` |
| `monitoring_profile_basis` | `pinned_vendor_document_identity` and `operator_site_observation_fixtures`, both required |
| `consulted_documents` | zero or more pins; the empty list is allowed and means no consulted document |
| `changes_outcome` | `false` |

A pin is one exact document version with three string fields, each as the
document states it and each equal to the row on the public page:

| Field | Rule |
| --- | --- |
| `title` | non-empty, at most 200 characters, no surrounding space, no control or invisible character, not a placeholder |
| `version` | non-empty, at most 64 characters, same text rules, not `not stated`, no wildcard or comparison character |
| `date` | `YYYY-MM-DD`, `YYYY-MM`, or `YYYY` as a real calendar value in 1979 or after, or the exact text `not stated` |

Title and version also reject anything that reads as a location (a scheme, a
`www.` host, a host with a path, a mail address) and anything that reads as a
digest (a hexadecimal run of 32 or more digits, a long base64 run, or a digest
algorithm name). A pin has no other key. Two pins that are equal in all three
fields repeat a row and are refused; pins that differ in any one field are two
document versions, with no range, prefix, ordering, or inheritance between them.

A pin that lacks the title, the version, or the date, or that fails a rule,
leaves the record without a consulted document. The validation lives in
`huawei_vendor_documentation_test.go`, because no production code reads the
dispositions; it runs over the five records and is exercised on its own with
accepted and refused field sets.

## Dispositions

- The SmartLogger, S-Dongle, and EMMA dispositions gain only the block. Their
  schema, outcomes, missing evidence, reopen requirements, observed identities,
  and the supersession of the recorded S-Dongle stop by a matched card of the
  same class are unchanged.
- The SUN2000 and LUNA2000 dispositions move to schema v2. `source_document`
  keeps the candidate document name and its unverified classification, drops
  the version marker, the pinned flag, and the locator, and states the citation
  rule in place of the earlier statement that forbade using the document. The
  reopen item that asked for a pinned document version now asks for a pinned
  document identity (title, version, and date). Their outcome stays
  `NO_ADMISSIBLE_PROFILE` and their reopen list keeps the four other items.
- `evidence.json` lists the policy page as a public source and states the rule
  in its applicability lines.
- The SUN2000 monitoring profiles (helianthus-modbusreg issue 348) move the
  SUN2000 and LUNA2000 dispositions to schema v3. The SUN2000 pin supplies the
  pinned document identity for the M3 and M1 series, so that reopen item stays
  only for the MAP0 series, which has no consulted edition. The pin by itself
  still changes no outcome: the outcome rests on the monitoring contract, the
  observation fixtures, the exact pairs and the executable decision table.

## Rollback

Removing the block, restoring the SUN2000 and LUNA2000 `source_document` and
reopen item, and removing the test file restores the earlier dispositions. No
product code, fixture, or card changes.
