# Changelog

## Growatt package (#377)

The Growatt profiles move from the module root to package `github.com/Project-Helianthus/helianthus-modbusreg/growatt`. Exported names that began with `Growatt` lose that prefix and constructors follow their types; methods, fields, wire values and behavior are unchanged. No alias or forwarding declaration remains in the root package. Consumers replace each previous name with the current one:

| Previous name | Current name |
| --- | --- |
| `modbusreg.GrowattAggregateAttempt` | `growatt.AggregateAttempt` |
| `modbusreg.GrowattAggregateContext` | `growatt.AggregateContext` |
| `modbusreg.GrowattAggregateHistoryLimit` | `growatt.AggregateHistoryLimit` |
| `modbusreg.GrowattAggregateIdentity` | `growatt.AggregateIdentity` |
| `modbusreg.GrowattAggregateOfflineCycle` | `growatt.AggregateOfflineCycle` |
| `modbusreg.GrowattAggregateOfflineProvider` | `growatt.AggregateOfflineProvider` |
| `modbusreg.GrowattAggregateOfflineReader` | `growatt.AggregateOfflineReader` |
| `modbusreg.GrowattAggregateRetention` | `growatt.AggregateRetention` |
| `modbusreg.GrowattAggregateSnapshot` | `growatt.AggregateSnapshot` |
| `modbusreg.GrowattBMSCellVoltageGroup` | `growatt.BMSCellVoltageGroup` |
| `modbusreg.GrowattBMSDiagnosticRawSlice` | `growatt.BMSDiagnosticRawSlice` |
| `modbusreg.GrowattBMSMultiGroupDiagnostic` | `growatt.BMSMultiGroupDiagnostic` |
| `modbusreg.GrowattBMSMultiGroupDiagnosticInput` | `growatt.BMSMultiGroupDiagnosticInput` |
| `modbusreg.GrowattBMSNativeCellState` | `growatt.BMSNativeCellState` |
| `modbusreg.GrowattBMSNativeCellStateOff` | `growatt.BMSNativeCellStateOff` |
| `modbusreg.GrowattBMSNativeCellStateOn` | `growatt.BMSNativeCellStateOn` |
| `modbusreg.GrowattBMSOperatingState` | `growatt.BMSOperatingState` |
| `modbusreg.GrowattBMSRS485RTUObserver` | `growatt.BMSRS485RTUObserver` |
| `modbusreg.GrowattBMSRS485RTUObserverSession` | `growatt.BMSRS485RTUObserverSession` |
| `modbusreg.GrowattBMSReadOnlyInput` | `growatt.BMSReadOnlyInput` |
| `modbusreg.GrowattBMSReadOnlyObservation` | `growatt.BMSReadOnlyObservation` |
| `modbusreg.GrowattBMSReadOnlySlice` | `growatt.BMSReadOnlySlice` |
| `modbusreg.GrowattBMSRevisionTuple` | `growatt.BMSRevisionTuple` |
| `modbusreg.GrowattBMSStateCharging` | `growatt.BMSStateCharging` |
| `modbusreg.GrowattBMSStateDischarging` | `growatt.BMSStateDischarging` |
| `modbusreg.GrowattBMSStateSoftStarting` | `growatt.BMSStateSoftStarting` |
| `modbusreg.GrowattBMSStateStandby` | `growatt.BMSStateStandby` |
| `modbusreg.GrowattBMSTypedReadOnlyStatus` | `growatt.BMSTypedReadOnlyStatus` |
| `modbusreg.GrowattDiagnosticCommitBase` | `growatt.DiagnosticCommitBase` |
| `modbusreg.GrowattDiagnosticCommitCandidate` | `growatt.DiagnosticCommitCandidate` |
| `modbusreg.GrowattDiagnosticCommitContext` | `growatt.DiagnosticCommitContext` |
| `modbusreg.GrowattDiagnosticCommitContextMismatch` | `growatt.DiagnosticCommitContextMismatch` |
| `modbusreg.GrowattDiagnosticCommitIdentity` | `growatt.DiagnosticCommitIdentity` |
| `modbusreg.GrowattDiagnosticCommitInvalidBase` | `growatt.DiagnosticCommitInvalidBase` |
| `modbusreg.GrowattDiagnosticCommitInvalidContext` | `growatt.DiagnosticCommitInvalidContext` |
| `modbusreg.GrowattDiagnosticCommitInvalidDiagnostic` | `growatt.DiagnosticCommitInvalidDiagnostic` |
| `modbusreg.GrowattDiagnosticCommitLineageChanged` | `growatt.DiagnosticCommitLineageChanged` |
| `modbusreg.GrowattDiagnosticCommitNativeIdentity` | `growatt.DiagnosticCommitNativeIdentity` |
| `modbusreg.GrowattDiagnosticCommitRejection` | `growatt.DiagnosticCommitRejection` |
| `modbusreg.GrowattDiagnosticCommitTopologyChanged` | `growatt.DiagnosticCommitTopologyChanged` |
| `modbusreg.GrowattFC04FieldExtent` | `growatt.FC04FieldExtent` |
| `modbusreg.GrowattFC04Record` | `growatt.FC04Record` |
| `modbusreg.GrowattFC04SymbolicSchema` | `growatt.FC04SymbolicSchema` |
| `modbusreg.GrowattProtocolIIFC04Applicability` | `growatt.ProtocolIIFC04Applicability` |
| `modbusreg.GrowattProtocolIIFC04RTUObserver` | `growatt.ProtocolIIFC04RTUObserver` |
| `modbusreg.GrowattProtocolIIFC04RTUObserverSession` | `growatt.ProtocolIIFC04RTUObserverSession` |
| `modbusreg.GrowattProtocolIIFC04Slice` | `growatt.ProtocolIIFC04Slice` |
| `modbusreg.GrowattProtocolIIFC04Telemetry` | `growatt.ProtocolIIFC04Telemetry` |
| `modbusreg.GrowattProtocolIIIdentityInput` | `growatt.ProtocolIIIdentityInput` |
| `modbusreg.GrowattProtocolIIIdentityObservation` | `growatt.ProtocolIIIdentityObservation` |
| `modbusreg.GrowattProtocolIIIdentityProfile` | `growatt.ProtocolIIIdentityProfile` |
| `modbusreg.GrowattProtocolIIIdentityRTUObserver` | `growatt.ProtocolIIIdentityRTUObserver` |
| `modbusreg.GrowattProtocolIIIdentityRTUObserverSession` | `growatt.ProtocolIIIdentityRTUObserverSession` |
| `modbusreg.GrowattProtocolIIIdentitySlice` | `growatt.ProtocolIIIdentitySlice` |
| `modbusreg.GrowattProtocolIIInverterState` | `growatt.ProtocolIIInverterState` |
| `modbusreg.GrowattProtocolIIStateFault` | `growatt.ProtocolIIStateFault` |
| `modbusreg.GrowattProtocolIIStateNormal` | `growatt.ProtocolIIStateNormal` |
| `modbusreg.GrowattProtocolIIStateWaiting` | `growatt.ProtocolIIStateWaiting` |
| `modbusreg.NewGrowattAggregateOfflineProvider` | `growatt.NewAggregateOfflineProvider` |
| `modbusreg.NewGrowattAggregateRetention` | `growatt.NewAggregateRetention` |
| `modbusreg.NewGrowattBMSRS485RTUObserver` | `growatt.NewBMSRS485RTUObserver` |
| `modbusreg.NewGrowattDiagnosticCommitBase` | `growatt.NewDiagnosticCommitBase` |
| `modbusreg.NewGrowattProtocolIIFC04RTUObserver` | `growatt.NewProtocolIIFC04RTUObserver` |
| `modbusreg.NewGrowattProtocolIIIdentityRTUObserver` | `growatt.NewProtocolIIIdentityRTUObserver` |

## Catalog spelling (#379)

Direct replacement of external values and schema identifiers; no aliases or migration acceptance. Native evidence, qualification and admission remain unchanged.

| Previous string | Current string |
| --- | --- |
| `%d product files read the catalogue: %v` | `%d product files read the catalog: %v` |
| `%s names a profile identifier; the catalogue registers nothing` | `%s names a profile identifier; the catalog registers nothing` |
| `%s reads the catalogue; only the two selections do` | `%s reads the catalog; only the two selections do` |
| `%s: a refused catalogue selected %+v` | `%s: a refused catalog selected %+v` |
| `%s: catalogue accepted` | `%s: catalog accepted` |
| `%s: catalogue_selection=%v want=%v` | `%s: catalog_selection=%v want=%v` |
| `%s: unit %d catalogue decision %q, want %q` | `%s: unit %d catalog decision %q, want %q` |
| `CATALOGUE_AMBIGUOUS` | `CATALOG_AMBIGUOUS` |
| `CATALOGUE_MODEL_ROWS` | `CATALOG_MODEL_ROWS` |
| `CATALOGUE_NOT_SELECTED` | `CATALOG_NOT_SELECTED` |
| `CATALOGUE_ROW_WITHOUT_PROFILE` | `CATALOG_ROW_WITHOUT_PROFILE` |
| `Catalogue did not round-trip` | `Catalog did not round-trip` |
| `Catalogue=%s want=%s` | `Catalog=%s want=%s` |
| `EMMA_FAMILY_BY_CATALOGUE_MODEL_ROWS` | `EMMA_FAMILY_BY_CATALOG_MODEL_ROWS` |
| `NOT_SELECTED no_catalogue_row` | `NOT_SELECTED no_catalog_row` |
| `SUN2000 MAP0 series: catalogue_card_without_monitoring_profile` | `SUN2000 MAP0 series: catalog_card_without_monitoring_profile` |
| `a catalogue decision grants authority` | `a catalog decision grants authority` |
| `an ambiguous catalogue decision built an observer` | `an ambiguous catalog decision built an observer` |
| `an invalid catalogue decided %s` | `an invalid catalog decided %s` |
| `below the catalogue floor: %#v` | `below the catalog floor: %#v` |
| `catalogue decision policy: %#v` | `catalog decision policy: %#v` |
| `catalogue differs from the reference: %v` | `catalog differs from the reference: %v` |
| `catalogue directory (default ` | `catalog directory (default ` |
| `catalogue files not found: %v` | `catalog files not found: %v` |
| `catalogue matches the reference: ` | `catalog matches the reference: ` |
| `catalogue to generate: inverter (SUN2000 reference) or gateway (SmartLogger, S-Dongle and EMMA reference)` | `catalog to generate: inverter (SUN2000 reference) or gateway (SmartLogger, S-Dongle and EMMA reference)` |
| `catalogue written: ` | `catalog written: ` |
| `catalogue` | `catalog` |
| `catalogue_ambiguous` | `catalog_ambiguous` |
| `catalogue_ambiguous: MA-C00 449, MA-C20 449` | `catalog_ambiguous: MA-C00 449, MA-C20 449` |
| `catalogue_card_without_monitoring_profile` | `catalog_card_without_monitoring_profile` |
| `catalogue_decision` | `catalog_decision` |
| `catalogue_decision_table` | `catalog_decision_table` |
| `catalogue_files` | `catalog_files` |
| `catalogue_identifier_mismatch` | `catalog_identifier_mismatch` |
| `catalogue_invalid` | `catalog_invalid` |
| `catalogue_m1_row_behind_sdongle` | `catalog_m1_row_behind_sdongle` |
| `catalogue_m3_row_behind_smartlogger` | `catalog_m3_row_behind_smartlogger` |
| `catalogue_model_row` | `catalog_model_row` |
| `catalogue_monitoring_profile` | `catalog_monitoring_profile` |
| `catalogue_not_selected` | `catalog_not_selected` |
| `catalogue_not_selected: below_floor` | `catalog_not_selected: below_floor` |
| `catalogue_not_selected: identifier_mismatch` | `catalog_not_selected: identifier_mismatch` |
| `catalogue_outcome` | `catalog_outcome` |
| `catalogue_reason` | `catalog_reason` |
| `catalogue_row_without_profile_on_path` | `catalog_row_without_profile_on_path` |
| `catalogue_rows_not_told_apart` | `catalog_rows_not_told_apart` |
| `catalogue_selection` | `catalog_selection` |
| `catalogue_software_malformed_or_absent` | `catalog_software_malformed_or_absent` |
| `committed catalogue: %v` | `committed catalog: %v` |
| `compare the generated files with the catalogue directory and write nothing` | `compare the generated files with the catalog directory and write nothing` |
| `cycle %d unit %d carries no catalogue decision: %+v` | `cycle %d unit %d carries no catalog decision: %+v` |
| `family_without_catalogue_row` | `family_without_catalog_row` |
| `gateway_catalogue_decision_table` | `gateway_catalog_decision_table` |
| `gateway_fc03_30050_q15_software_admitted_by_catalogue_floors_when_read` | `gateway_fc03_30050_q15_software_admitted_by_catalog_floors_when_read` |
| `helianthus-huawei-catalogue-selection-fixture/v1` | `helianthus-huawei-catalog-selection-fixture/v1` |
| `helianthus-modbusreg-huawei-catalogue-dictionaries/v1` | `helianthus-modbusreg-huawei-catalog-dictionaries/v1` |
| `helianthus-modbusreg-huawei-catalogue-editions/v1` | `helianthus-modbusreg-huawei-catalog-editions/v1` |
| `helianthus-modbusreg-huawei-catalogue-models/v1` | `helianthus-modbusreg-huawei-catalog-models/v1` |
| `helianthus-modbusreg-huawei-catalogue-quarantines/v1` | `helianthus-modbusreg-huawei-catalog-quarantines/v1` |
| `helianthus-modbusreg-huawei-catalogue-registers/v1` | `helianthus-modbusreg-huawei-catalog-registers/v1` |
| `helianthus-modbusreg-huawei-catalogue-services/v1` | `helianthus-modbusreg-huawei-catalog-services/v1` |
| `huaweicatalogue` | `huaweicatalog` |
| `huaweicatalogue: ` | `huaweicatalog: ` |
| `invalid SUN2000 catalogue` | `invalid SUN2000 catalog` |
| `invalid SmartLogger, S-Dongle and EMMA catalogue` | `invalid SmartLogger, S-Dongle and EMMA catalog` |
| `missing Catalogue JSON key` | `missing Catalog JSON key` |
| `no duplicate pair in the catalogue` | `no duplicate pair in the catalog` |
| `no_catalogue_row` | `no_catalog_row` |
| `row %+v disagrees with the catalogue row %+v` | `row %+v disagrees with the catalog row %+v` |
| `self units do not follow the catalogue` | `self units do not follow the catalog` |
| `self_entry_software_admitted_by_catalogue_floors` | `self_entry_software_admitted_by_catalog_floors` |
| `self_entry_software_admitted_by_catalogue_floors_and_at_or_above_observed_floor` | `self_entry_software_admitted_by_catalog_floors_and_at_or_above_observed_floor` |
| `software_admitted_by_catalogue_floors` | `software_admitted_by_catalog_floors` |
| `the catalogue directory holds %s, which the generator does not produce` | `the catalog directory holds %s, which the generator does not produce` |
| `the sidecar directory must lie outside the repository and the catalogue directory` | `the sidecar directory must lie outside the repository and the catalog directory` |
| `without_catalogue_row` | `without_catalog_row` |
| `Catalogue` | `catalog` |
| `json:"catalogue,omitempty` | `json:"catalog,omitempty` |


## Unreleased

- Doc comments now cover every exported declaration in the Tesla, Growatt,
  OutBack and Fronius family files (first slice of #375; the six declarations
  in `tesla_gen3_read_acquisition.go` follow in a later slice). Comments state
  the implemented contract: units and scaling, errors and fail-closed behavior,
  ownership, concurrency and lifecycle, with protocol meaning left to the
  public helianthus-docs-modbus pages. The matching `revive/exported` rows are
  removed from `scripts/go-lint-baseline.json`. No code, name or behavior
  changes; added comment lines shift the line table, so the change is
  artifact-changing for consumers that re-pin.
- Huawei monitoring emits `status` for the M1 family state field. The previous
  field name has no runtime alias or fallback. The public naming contract
  requires coordinated consumer adoption; native identities, grades, units,
  read plans, partial retention and fail-closed admission are preserved.
- Accumulated energy keeps its existing provisional key pending public evidence
  of total accumulation across both families. Other provisional names and native
  SunSpec and TEDAPI identifiers remain unchanged.
- This is an artifact-changing contract: renew affected security, hardware and
  candidate evidence before release. Synthetic replay grants no qualification.
