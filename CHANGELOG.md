# Changelog

## Huawei package (#377)

The Huawei profiles (SUN2000, LUNA2000, SmartLogger, S-Dongle and EMMA) move from the module root to package `github.com/Project-Helianthus/helianthus-modbusreg/huawei`. Exported names that began with `Huawei` lose that prefix and constructors follow their types; other exported names change only their qualifier; methods, fields, wire values and behavior are unchanged. No alias or forwarding declaration remains in the root package. The two read-only parts of the generated Huawei catalog are embedded by the new data package `github.com/Project-Helianthus/helianthus-modbusreg/profiles` (`profiles.HuaweiInverterCatalog` and `profiles.HuaweiGatewayCatalog`), because Go embed cannot reach a parent directory; the embedded files are unchanged and only the two catalog selections read them. Consumers replace each previous name with the current one:

| Previous name | Current name |
| --- | --- |
| `modbusreg.HuaweiDecimal` | `huawei.Decimal` |
| `modbusreg.HuaweiEMMACanonicalClass` | `huawei.EMMACanonicalClass` |
| `modbusreg.HuaweiEMMAObservedBasicReply` | `huawei.EMMAObservedBasicReply` |
| `modbusreg.HuaweiEMMAObservedChildReads` | `huawei.EMMAObservedChildReads` |
| `modbusreg.HuaweiEMMAObservedGatewayReads` | `huawei.EMMAObservedGatewayReads` |
| `modbusreg.HuaweiEMMAObservedInventoryPage` | `huawei.EMMAObservedInventoryPage` |
| `modbusreg.HuaweiEMMAObservedQualificationCard` | `huawei.EMMAObservedQualificationCard` |
| `modbusreg.HuaweiEMMAObservedQualificationCardID` | `huawei.EMMAObservedQualificationCardID` |
| `modbusreg.HuaweiEMMAObservedQualificationInput` | `huawei.EMMAObservedQualificationInput` |
| `modbusreg.HuaweiEMMAOfflineDecision` | `huawei.EMMAOfflineDecision` |
| `modbusreg.HuaweiEMMAOfflineFirmwareMismatch` | `huawei.EMMAOfflineFirmwareMismatch` |
| `modbusreg.HuaweiEMMAOfflineInventory` | `huawei.EMMAOfflineInventory` |
| `modbusreg.HuaweiEMMAOfflineInventoryInput` | `huawei.EMMAOfflineInventoryInput` |
| `modbusreg.HuaweiEMMAOfflineMatched` | `huawei.EMMAOfflineMatched` |
| `modbusreg.HuaweiEMMAOfflineModelMismatch` | `huawei.EMMAOfflineModelMismatch` |
| `modbusreg.HuaweiEMMAOfflineOfferingMismatch` | `huawei.EMMAOfflineOfferingMismatch` |
| `modbusreg.HuaweiEMMAOfflineReason` | `huawei.EMMAOfflineReason` |
| `modbusreg.HuaweiEMMAQualificationCard` | `huawei.EMMAQualificationCard` |
| `modbusreg.HuaweiEMMAQualificationCardID` | `huawei.EMMAQualificationCardID` |
| `modbusreg.HuaweiEMMAQualificationInput` | `huawei.EMMAQualificationInput` |
| `modbusreg.HuaweiEMMAReadOnlyProfileID` | `huawei.EMMAReadOnlyProfileID` |
| `modbusreg.HuaweiEditionBinding` | `huawei.EditionBinding` |
| `modbusreg.HuaweiEditionBindingLiveCheckedLayout` | `huawei.EditionBindingLiveCheckedLayout` |
| `modbusreg.HuaweiEditionBindingSupportedModelTable` | `huawei.EditionBindingSupportedModelTable` |
| `modbusreg.HuaweiEvidenceGrade` | `huawei.EvidenceGrade` |
| `modbusreg.HuaweiFactAvailability` | `huawei.FactAvailability` |
| `modbusreg.HuaweiFactUnknown` | `huawei.FactUnknown` |
| `modbusreg.HuaweiFactValue` | `huawei.FactValue` |
| `modbusreg.HuaweiGatewayAmbiguous` | `huawei.GatewayAmbiguous` |
| `modbusreg.HuaweiGatewayAtOrAboveFloor` | `huawei.GatewayAtOrAboveFloor` |
| `modbusreg.HuaweiGatewayBelowFloor` | `huawei.GatewayBelowFloor` |
| `modbusreg.HuaweiGatewayCatalogInvalid` | `huawei.GatewayCatalogInvalid` |
| `modbusreg.HuaweiGatewayEdition` | `huawei.GatewayEdition` |
| `modbusreg.HuaweiGatewayEndpoint` | `huawei.GatewayEndpoint` |
| `modbusreg.HuaweiGatewayEndpointSelf` | `huawei.GatewayEndpointSelf` |
| `modbusreg.HuaweiGatewayFamilyMismatch` | `huawei.GatewayFamilyMismatch` |
| `modbusreg.HuaweiGatewayFloor` | `huawei.GatewayFloor` |
| `modbusreg.HuaweiGatewayFloorBranch` | `huawei.GatewayFloorBranch` |
| `modbusreg.HuaweiGatewayFloorDomain` | `huawei.GatewayFloorDomain` |
| `modbusreg.HuaweiGatewayFloorLine` | `huawei.GatewayFloorLine` |
| `modbusreg.HuaweiGatewayFloorRelation` | `huawei.GatewayFloorRelation` |
| `modbusreg.HuaweiGatewayInventory` | `huawei.GatewayInventory` |
| `modbusreg.HuaweiGatewayInventoryChild` | `huawei.GatewayInventoryChild` |
| `modbusreg.HuaweiGatewayInventoryObject` | `huawei.GatewayInventoryObject` |
| `modbusreg.HuaweiGatewayInventoryPage` | `huawei.GatewayInventoryPage` |
| `modbusreg.HuaweiGatewayMultipleFloors` | `huawei.GatewayMultipleFloors` |
| `modbusreg.HuaweiGatewayNoDocumentedRelation` | `huawei.GatewayNoDocumentedRelation` |
| `modbusreg.HuaweiGatewayNotSelected` | `huawei.GatewayNotSelected` |
| `modbusreg.HuaweiGatewayOpaqueAttribute` | `huawei.GatewayOpaqueAttribute` |
| `modbusreg.HuaweiGatewayRelationAtOrAbove` | `huawei.GatewayRelationAtOrAbove` |
| `modbusreg.HuaweiGatewayRelationBelow` | `huawei.GatewayRelationBelow` |
| `modbusreg.HuaweiGatewayRelationNoDocumentedRelation` | `huawei.GatewayRelationNoDocumentedRelation` |
| `modbusreg.HuaweiGatewayRelationNotEvaluated` | `huawei.GatewayRelationNotEvaluated` |
| `modbusreg.HuaweiGatewayScope` | `huawei.GatewayScope` |
| `modbusreg.HuaweiGatewaySelected` | `huawei.GatewaySelected` |
| `modbusreg.HuaweiGatewaySelection` | `huawei.GatewaySelection` |
| `modbusreg.HuaweiGatewaySelectionInput` | `huawei.GatewaySelectionInput` |
| `modbusreg.HuaweiGatewaySelectionOutcome` | `huawei.GatewaySelectionOutcome` |
| `modbusreg.HuaweiGatewaySelectionReason` | `huawei.GatewaySelectionReason` |
| `modbusreg.HuaweiGatewaySoftwareMalformed` | `huawei.GatewaySoftwareMalformed` |
| `modbusreg.HuaweiGatewaySoftwareNotRead` | `huawei.GatewaySoftwareNotRead` |
| `modbusreg.HuaweiGradeHypothesis` | `huawei.GradeHypothesis` |
| `modbusreg.HuaweiGradeProven` | `huawei.GradeProven` |
| `modbusreg.HuaweiGradeUnknown` | `huawei.GradeUnknown` |
| `modbusreg.HuaweiIdentificationPage` | `huawei.IdentificationPage` |
| `modbusreg.HuaweiIdentificationRequest` | `huawei.IdentificationRequest` |
| `modbusreg.HuaweiInventoryPageAnomalies` | `huawei.InventoryPageAnomalies` |
| `modbusreg.HuaweiProviderAtDeadline` | `huawei.ProviderAtDeadline` |
| `modbusreg.HuaweiProviderAuthorityFieldSet` | `huawei.ProviderAuthorityFieldSet` |
| `modbusreg.HuaweiProviderChildDisposition` | `huawei.ProviderChildDisposition` |
| `modbusreg.HuaweiProviderChildObservation` | `huawei.ProviderChildObservation` |
| `modbusreg.HuaweiProviderChildProfileNotRecognised` | `huawei.ProviderChildProfileNotRecognised` |
| `modbusreg.HuaweiProviderConfig` | `huawei.ProviderConfig` |
| `modbusreg.HuaweiProviderCycle` | `huawei.ProviderCycle` |
| `modbusreg.HuaweiProviderCycleComplete` | `huawei.ProviderCycleComplete` |
| `modbusreg.HuaweiProviderCycleIncomplete` | `huawei.ProviderCycleIncomplete` |
| `modbusreg.HuaweiProviderCycleInsufficient` | `huawei.ProviderCycleInsufficient` |
| `modbusreg.HuaweiProviderCycleOutcome` | `huawei.ProviderCycleOutcome` |
| `modbusreg.HuaweiProviderDisposition` | `huawei.ProviderDisposition` |
| `modbusreg.HuaweiProviderElapsedClass` | `huawei.ProviderElapsedClass` |
| `modbusreg.HuaweiProviderEnabled` | `huawei.ProviderEnabled` |
| `modbusreg.HuaweiProviderEnablement` | `huawei.ProviderEnablement` |
| `modbusreg.HuaweiProviderEnablementReason` | `huawei.ProviderEnablementReason` |
| `modbusreg.HuaweiProviderEvidenceCurrent` | `huawei.ProviderEvidenceCurrent` |
| `modbusreg.HuaweiProviderEvidenceStale` | `huawei.ProviderEvidenceStale` |
| `modbusreg.HuaweiProviderEvidenceState` | `huawei.ProviderEvidenceState` |
| `modbusreg.HuaweiProviderEvidenceWithheld` | `huawei.ProviderEvidenceWithheld` |
| `modbusreg.HuaweiProviderExchange` | `huawei.ProviderExchange` |
| `modbusreg.HuaweiProviderExchangeAnswered` | `huawei.ProviderExchangeAnswered` |
| `modbusreg.HuaweiProviderExchangeException` | `huawei.ProviderExchangeException` |
| `modbusreg.HuaweiProviderExchangeMalformed` | `huawei.ProviderExchangeMalformed` |
| `modbusreg.HuaweiProviderExchangeOutcome` | `huawei.ProviderExchangeOutcome` |
| `modbusreg.HuaweiProviderExchangeTimeout` | `huawei.ProviderExchangeTimeout` |
| `modbusreg.HuaweiProviderExchangeTransport` | `huawei.ProviderExchangeTransport` |
| `modbusreg.HuaweiProviderFencedReading` | `huawei.ProviderFencedReading` |
| `modbusreg.HuaweiProviderFirstRequestTolerated` | `huawei.ProviderFirstRequestTolerated` |
| `modbusreg.HuaweiProviderGatewayIdentified` | `huawei.ProviderGatewayIdentified` |
| `modbusreg.HuaweiProviderGatewayIdentity` | `huawei.ProviderGatewayIdentity` |
| `modbusreg.HuaweiProviderGatewayInsufficient` | `huawei.ProviderGatewayInsufficient` |
| `modbusreg.HuaweiProviderGatewayNotIdentified` | `huawei.ProviderGatewayNotIdentified` |
| `modbusreg.HuaweiProviderGatewayState` | `huawei.ProviderGatewayState` |
| `modbusreg.HuaweiProviderGatewayUnconfirmed` | `huawei.ProviderGatewayUnconfirmed` |
| `modbusreg.HuaweiProviderGroupEvidence` | `huawei.ProviderGroupEvidence` |
| `modbusreg.HuaweiProviderIdentityNotAdmitted` | `huawei.ProviderIdentityNotAdmitted` |
| `modbusreg.HuaweiProviderLoss` | `huawei.ProviderLoss` |
| `modbusreg.HuaweiProviderLossBattery` | `huawei.ProviderLossBattery` |
| `modbusreg.HuaweiProviderLossKind` | `huawei.ProviderLossKind` |
| `modbusreg.HuaweiProviderLossMeter` | `huawei.ProviderLossMeter` |
| `modbusreg.HuaweiProviderLossPlannedGroup` | `huawei.ProviderLossPlannedGroup` |
| `modbusreg.HuaweiProviderLossRefusedGroup` | `huawei.ProviderLossRefusedGroup` |
| `modbusreg.HuaweiProviderLossUnconsumedWords` | `huawei.ProviderLossUnconsumedWords` |
| `modbusreg.HuaweiProviderLossWithheldChild` | `huawei.ProviderLossWithheldChild` |
| `modbusreg.HuaweiProviderNoAdmittedChildProfile` | `huawei.ProviderNoAdmittedChildProfile` |
| `modbusreg.HuaweiProviderPacing` | `huawei.ProviderPacing` |
| `modbusreg.HuaweiProviderPacingFor` | `huawei.ProviderPacingFor` |
| `modbusreg.HuaweiProviderPath` | `huawei.ProviderPath` |
| `modbusreg.HuaweiProviderPathEMMA` | `huawei.ProviderPathEMMA` |
| `modbusreg.HuaweiProviderPathSDongle` | `huawei.ProviderPathSDongle` |
| `modbusreg.HuaweiProviderPathSmartLogger` | `huawei.ProviderPathSmartLogger` |
| `modbusreg.HuaweiProviderReason` | `huawei.ProviderReason` |
| `modbusreg.HuaweiProviderReasonCatalogAmbiguous` | `huawei.ProviderReasonCatalogAmbiguous` |
| `modbusreg.HuaweiProviderReasonCatalogNotSelected` | `huawei.ProviderReasonCatalogNotSelected` |
| `modbusreg.HuaweiProviderReasonChildBackoff` | `huawei.ProviderReasonChildBackoff` |
| `modbusreg.HuaweiProviderReasonChildException` | `huawei.ProviderReasonChildException` |
| `modbusreg.HuaweiProviderReasonChildReadFailed` | `huawei.ProviderReasonChildReadFailed` |
| `modbusreg.HuaweiProviderReasonChildTimeout` | `huawei.ProviderReasonChildTimeout` |
| `modbusreg.HuaweiProviderReasonGatewayMismatch` | `huawei.ProviderReasonGatewayMismatch` |
| `modbusreg.HuaweiProviderReasonGatewayReadFail` | `huawei.ProviderReasonGatewayReadFail` |
| `modbusreg.HuaweiProviderReasonGuardChanged` | `huawei.ProviderReasonGuardChanged` |
| `modbusreg.HuaweiProviderReasonGuardFailed` | `huawei.ProviderReasonGuardFailed` |
| `modbusreg.HuaweiProviderReasonGuardHeld` | `huawei.ProviderReasonGuardHeld` |
| `modbusreg.HuaweiProviderReasonIdentified` | `huawei.ProviderReasonIdentified` |
| `modbusreg.HuaweiProviderReasonIdentityInsuff` | `huawei.ProviderReasonIdentityInsuff` |
| `modbusreg.HuaweiProviderReasonIdentityMismatch` | `huawei.ProviderReasonIdentityMismatch` |
| `modbusreg.HuaweiProviderReasonListingBackoff` | `huawei.ProviderReasonListingBackoff` |
| `modbusreg.HuaweiProviderReasonListingUnconfirmed` | `huawei.ProviderReasonListingUnconfirmed` |
| `modbusreg.HuaweiProviderReasonListingUnstable` | `huawei.ProviderReasonListingUnstable` |
| `modbusreg.HuaweiProviderReasonNoClassContract` | `huawei.ProviderReasonNoClassContract` |
| `modbusreg.HuaweiProviderReasonNoProfile` | `huawei.ProviderReasonNoProfile` |
| `modbusreg.HuaweiProviderReasonNotAllowlisted` | `huawei.ProviderReasonNotAllowlisted` |
| `modbusreg.HuaweiProviderReasonNotListed` | `huawei.ProviderReasonNotListed` |
| `modbusreg.HuaweiProviderReasonRead` | `huawei.ProviderReasonRead` |
| `modbusreg.HuaweiProviderRecordedStopInForce` | `huawei.ProviderRecordedStopInForce` |
| `modbusreg.HuaweiProviderSession` | `huawei.ProviderSession` |
| `modbusreg.HuaweiProviderUnknownPath` | `huawei.ProviderUnknownPath` |
| `modbusreg.HuaweiProviderWait` | `huawei.ProviderWait` |
| `modbusreg.HuaweiProviderWithinDeadline` | `huawei.ProviderWithinDeadline` |
| `modbusreg.HuaweiProviderWithinPromptBound` | `huawei.ProviderWithinPromptBound` |
| `modbusreg.HuaweiQualificationAmbiguous` | `huawei.QualificationAmbiguous` |
| `modbusreg.HuaweiQualificationBasicIdentityConflict` | `huawei.QualificationBasicIdentityConflict` |
| `modbusreg.HuaweiQualificationEvidenceBlocked` | `huawei.QualificationEvidenceBlocked` |
| `modbusreg.HuaweiQualificationExpectedResult` | `huawei.QualificationExpectedResult` |
| `modbusreg.HuaweiQualificationGatewayBehaviourMismatch` | `huawei.QualificationGatewayBehaviourMismatch` |
| `modbusreg.HuaweiQualificationIdentityInvalid` | `huawei.QualificationIdentityInvalid` |
| `modbusreg.HuaweiQualificationIdentityMatchedCapabilitiesMissing` | `huawei.QualificationIdentityMatchedCapabilitiesMissing` |
| `modbusreg.HuaweiQualificationInsufficientEvidence` | `huawei.QualificationInsufficientEvidence` |
| `modbusreg.HuaweiQualificationInventoryInvalid` | `huawei.QualificationInventoryInvalid` |
| `modbusreg.HuaweiQualificationInventoryLimits` | `huawei.QualificationInventoryLimits` |
| `modbusreg.HuaweiQualificationMatchedIdentityInventory` | `huawei.QualificationMatchedIdentityInventory` |
| `modbusreg.HuaweiQualificationMultipleClasses` | `huawei.QualificationMultipleClasses` |
| `modbusreg.HuaweiQualificationPackage` | `huawei.QualificationPackage` |
| `modbusreg.HuaweiQualificationPersistentNonResponse` | `huawei.QualificationPersistentNonResponse` |
| `modbusreg.HuaweiQualificationReadiness` | `huawei.QualificationReadiness` |
| `modbusreg.HuaweiQualificationReason` | `huawei.QualificationReason` |
| `modbusreg.HuaweiQualificationResult` | `huawei.QualificationResult` |
| `modbusreg.HuaweiQualificationStep` | `huawei.QualificationStep` |
| `modbusreg.HuaweiQualificationTestReady` | `huawei.QualificationTestReady` |
| `modbusreg.HuaweiRecordedProviderDisposition` | `huawei.RecordedProviderDisposition` |
| `modbusreg.HuaweiSDongleCanonicalClass` | `huawei.SDongleCanonicalClass` |
| `modbusreg.HuaweiSDongleObservedBasicOutcome` | `huawei.SDongleObservedBasicOutcome` |
| `modbusreg.HuaweiSDongleObservedBasicReply` | `huawei.SDongleObservedBasicReply` |
| `modbusreg.HuaweiSDongleObservedChildInventoryPage` | `huawei.SDongleObservedChildInventoryPage` |
| `modbusreg.HuaweiSDongleObservedChildReads` | `huawei.SDongleObservedChildReads` |
| `modbusreg.HuaweiSDongleObservedGatewayReads` | `huawei.SDongleObservedGatewayReads` |
| `modbusreg.HuaweiSDongleObservedQualificationCard` | `huawei.SDongleObservedQualificationCard` |
| `modbusreg.HuaweiSDongleObservedQualificationCardID` | `huawei.SDongleObservedQualificationCardID` |
| `modbusreg.HuaweiSDongleObservedQualificationInput` | `huawei.SDongleObservedQualificationInput` |
| `modbusreg.HuaweiSDongleOfflineCapacityMismatch` | `huawei.SDongleOfflineCapacityMismatch` |
| `modbusreg.HuaweiSDongleOfflineDecision` | `huawei.SDongleOfflineDecision` |
| `modbusreg.HuaweiSDongleOfflineFirmwareMismatch` | `huawei.SDongleOfflineFirmwareMismatch` |
| `modbusreg.HuaweiSDongleOfflineMatched` | `huawei.SDongleOfflineMatched` |
| `modbusreg.HuaweiSDongleOfflineModelMismatch` | `huawei.SDongleOfflineModelMismatch` |
| `modbusreg.HuaweiSDongleOfflineObservation` | `huawei.SDongleOfflineObservation` |
| `modbusreg.HuaweiSDongleOfflineProtocolMismatch` | `huawei.SDongleOfflineProtocolMismatch` |
| `modbusreg.HuaweiSDongleOfflineReason` | `huawei.SDongleOfflineReason` |
| `modbusreg.HuaweiSDongleOfflineSearchIncomplete` | `huawei.SDongleOfflineSearchIncomplete` |
| `modbusreg.HuaweiSDongleOfflineSequenceMismatch` | `huawei.SDongleOfflineSequenceMismatch` |
| `modbusreg.HuaweiSDongleOfflineUnitMismatch` | `huawei.SDongleOfflineUnitMismatch` |
| `modbusreg.HuaweiSDongleQualificationCard` | `huawei.SDongleQualificationCard` |
| `modbusreg.HuaweiSDongleQualificationCardID` | `huawei.SDongleQualificationCardID` |
| `modbusreg.HuaweiSDongleReadOnlyProfileID` | `huawei.SDongleReadOnlyProfileID` |
| `modbusreg.HuaweiSDongleReadOnlyProvider` | `huawei.SDongleReadOnlyProvider` |
| `modbusreg.HuaweiSDongleReadOnlyProviderID` | `huawei.SDongleReadOnlyProviderID` |
| `modbusreg.HuaweiSUN2000AmbiguousMultipleProfiles` | `huawei.SUN2000AmbiguousMultipleProfiles` |
| `modbusreg.HuaweiSUN2000BuildFirmwareText` | `huawei.SUN2000BuildFirmwareText` |
| `modbusreg.HuaweiSUN2000BuildNone` | `huawei.SUN2000BuildNone` |
| `modbusreg.HuaweiSUN2000BuildSoftwareText` | `huawei.SUN2000BuildSoftwareText` |
| `modbusreg.HuaweiSUN2000BuildSource` | `huawei.SUN2000BuildSource` |
| `modbusreg.HuaweiSUN2000CapabilityInput` | `huawei.SUN2000CapabilityInput` |
| `modbusreg.HuaweiSUN2000CapabilityOutcome` | `huawei.SUN2000CapabilityOutcome` |
| `modbusreg.HuaweiSUN2000CapabilityReason` | `huawei.SUN2000CapabilityReason` |
| `modbusreg.HuaweiSUN2000CapabilitySelection` | `huawei.SUN2000CapabilitySelection` |
| `modbusreg.HuaweiSUN2000CatalogAmbiguous` | `huawei.SUN2000CatalogAmbiguous` |
| `modbusreg.HuaweiSUN2000CatalogAtOrAboveFloor` | `huawei.SUN2000CatalogAtOrAboveFloor` |
| `modbusreg.HuaweiSUN2000CatalogBelowFloor` | `huawei.SUN2000CatalogBelowFloor` |
| `modbusreg.HuaweiSUN2000CatalogDecision` | `huawei.SUN2000CatalogDecision` |
| `modbusreg.HuaweiSUN2000CatalogEdition` | `huawei.SUN2000CatalogEdition` |
| `modbusreg.HuaweiSUN2000CatalogEqualOnCarriedParts` | `huawei.SUN2000CatalogEqualOnCarriedParts` |
| `modbusreg.HuaweiSUN2000CatalogFloorOutsideBranchForm` | `huawei.SUN2000CatalogFloorOutsideBranchForm` |
| `modbusreg.HuaweiSUN2000CatalogIdentifierAbsent` | `huawei.SUN2000CatalogIdentifierAbsent` |
| `modbusreg.HuaweiSUN2000CatalogIdentifierMismatch` | `huawei.SUN2000CatalogIdentifierMismatch` |
| `modbusreg.HuaweiSUN2000CatalogInput` | `huawei.SUN2000CatalogInput` |
| `modbusreg.HuaweiSUN2000CatalogInvalid` | `huawei.SUN2000CatalogInvalid` |
| `modbusreg.HuaweiSUN2000CatalogMatch` | `huawei.SUN2000CatalogMatch` |
| `modbusreg.HuaweiSUN2000CatalogMatchToken` | `huawei.SUN2000CatalogMatchToken` |
| `modbusreg.HuaweiSUN2000CatalogMatchWireAlias` | `huawei.SUN2000CatalogMatchWireAlias` |
| `modbusreg.HuaweiSUN2000CatalogModelTextInvalid` | `huawei.SUN2000CatalogModelTextInvalid` |
| `modbusreg.HuaweiSUN2000CatalogMultipleCandidateRows` | `huawei.SUN2000CatalogMultipleCandidateRows` |
| `modbusreg.HuaweiSUN2000CatalogNoCatalogRow` | `huawei.SUN2000CatalogNoCatalogRow` |
| `modbusreg.HuaweiSUN2000CatalogNoDocumentedRelation` | `huawei.SUN2000CatalogNoDocumentedRelation` |
| `modbusreg.HuaweiSUN2000CatalogNotSelected` | `huawei.SUN2000CatalogNotSelected` |
| `modbusreg.HuaweiSUN2000CatalogOutcome` | `huawei.SUN2000CatalogOutcome` |
| `modbusreg.HuaweiSUN2000CatalogReason` | `huawei.SUN2000CatalogReason` |
| `modbusreg.HuaweiSUN2000CatalogRow` | `huawei.SUN2000CatalogRow` |
| `modbusreg.HuaweiSUN2000CatalogScopeInvalid` | `huawei.SUN2000CatalogScopeInvalid` |
| `modbusreg.HuaweiSUN2000CatalogScopeMismatch` | `huawei.SUN2000CatalogScopeMismatch` |
| `modbusreg.HuaweiSUN2000CatalogSelected` | `huawei.SUN2000CatalogSelected` |
| `modbusreg.HuaweiSUN2000CatalogSoftwareAbsent` | `huawei.SUN2000CatalogSoftwareAbsent` |
| `modbusreg.HuaweiSUN2000CatalogSoftwareMalformed` | `huawei.SUN2000CatalogSoftwareMalformed` |
| `modbusreg.HuaweiSUN2000EndpointScope` | `huawei.SUN2000EndpointScope` |
| `modbusreg.HuaweiSUN2000Fact` | `huawei.SUN2000Fact` |
| `modbusreg.HuaweiSUN2000FloorAtOrAbove` | `huawei.SUN2000FloorAtOrAbove` |
| `modbusreg.HuaweiSUN2000FloorBelow` | `huawei.SUN2000FloorBelow` |
| `modbusreg.HuaweiSUN2000FloorEqualOnCarriedParts` | `huawei.SUN2000FloorEqualOnCarriedParts` |
| `modbusreg.HuaweiSUN2000FloorNoDocumentedRelation` | `huawei.SUN2000FloorNoDocumentedRelation` |
| `modbusreg.HuaweiSUN2000FloorNotEvaluated` | `huawei.SUN2000FloorNotEvaluated` |
| `modbusreg.HuaweiSUN2000FloorOutsideBranchForm` | `huawei.SUN2000FloorOutsideBranchForm` |
| `modbusreg.HuaweiSUN2000FloorRelation` | `huawei.SUN2000FloorRelation` |
| `modbusreg.HuaweiSUN2000GroupAnswered` | `huawei.SUN2000GroupAnswered` |
| `modbusreg.HuaweiSUN2000GroupException` | `huawei.SUN2000GroupException` |
| `modbusreg.HuaweiSUN2000GroupFailed` | `huawei.SUN2000GroupFailed` |
| `modbusreg.HuaweiSUN2000GroupNotRead` | `huawei.SUN2000GroupNotRead` |
| `modbusreg.HuaweiSUN2000GroupOutcome` | `huawei.SUN2000GroupOutcome` |
| `modbusreg.HuaweiSUN2000GroupReading` | `huawei.SUN2000GroupReading` |
| `modbusreg.HuaweiSUN2000InsufficientEvidence` | `huawei.SUN2000InsufficientEvidence` |
| `modbusreg.HuaweiSUN2000M1MonitoringProfileID` | `huawei.SUN2000M1MonitoringProfileID` |
| `modbusreg.HuaweiSUN2000M3MonitoringProfileID` | `huawei.SUN2000M3MonitoringProfileID` |
| `modbusreg.HuaweiSUN2000MonitoringObserver` | `huawei.SUN2000MonitoringObserver` |
| `modbusreg.HuaweiSUN2000MonitoringProfile` | `huawei.SUN2000MonitoringProfile` |
| `modbusreg.HuaweiSUN2000MonitoringProfiles` | `huawei.SUN2000MonitoringProfiles` |
| `modbusreg.HuaweiSUN2000MonitoringReading` | `huawei.SUN2000MonitoringReading` |
| `modbusreg.HuaweiSUN2000MonitoringSession` | `huawei.SUN2000MonitoringSession` |
| `modbusreg.HuaweiSUN2000NoAdmissibleProfile` | `huawei.SUN2000NoAdmissibleProfile` |
| `modbusreg.HuaweiSUN2000ProfileEvaluation` | `huawei.SUN2000ProfileEvaluation` |
| `modbusreg.HuaweiSUN2000ProfileSelected` | `huawei.SUN2000ProfileSelected` |
| `modbusreg.HuaweiSUN2000ReadStep` | `huawei.SUN2000ReadStep` |
| `modbusreg.HuaweiSUN2000ReasonCatalogAmbiguous` | `huawei.SUN2000ReasonCatalogAmbiguous` |
| `modbusreg.HuaweiSUN2000ReasonCatalogNotSelected` | `huawei.SUN2000ReasonCatalogNotSelected` |
| `modbusreg.HuaweiSUN2000ReasonCatalogRowNoProfile` | `huawei.SUN2000ReasonCatalogRowNoProfile` |
| `modbusreg.HuaweiSUN2000ReasonGatewayIdentityRejected` | `huawei.SUN2000ReasonGatewayIdentityRejected` |
| `modbusreg.HuaweiSUN2000ReasonIdentityWordsInvalid` | `huawei.SUN2000ReasonIdentityWordsInvalid` |
| `modbusreg.HuaweiSUN2000ReasonListingModelConflict` | `huawei.SUN2000ReasonListingModelConflict` |
| `modbusreg.HuaweiSUN2000ReasonMatched` | `huawei.SUN2000ReasonMatched` |
| `modbusreg.HuaweiSUN2000ReasonModelBlockInvalid` | `huawei.SUN2000ReasonModelBlockInvalid` |
| `modbusreg.HuaweiSUN2000ReasonMultipleProfilesSelected` | `huawei.SUN2000ReasonMultipleProfilesSelected` |
| `modbusreg.HuaweiSUN2000ReasonNoGatewayIdentity` | `huawei.SUN2000ReasonNoGatewayIdentity` |
| `modbusreg.HuaweiSUN2000ReasonUnitNotRouted` | `huawei.SUN2000ReasonUnitNotRouted` |
| `modbusreg.HuaweiSUN2000RefusedGroup` | `huawei.SUN2000RefusedGroup` |
| `modbusreg.HuaweiSUN2000RoutedIdentity` | `huawei.SUN2000RoutedIdentity` |
| `modbusreg.HuaweiSUN2000ScopeC81Map` | `huawei.SUN2000ScopeC81Map` |
| `modbusreg.HuaweiSUN2000ScopeGatewayNamespace` | `huawei.SUN2000ScopeGatewayNamespace` |
| `modbusreg.HuaweiSUN2000ScopeInverterMap` | `huawei.SUN2000ScopeInverterMap` |
| `modbusreg.HuaweiSmartLoggerCanonicalClass` | `huawei.SmartLoggerCanonicalClass` |
| `modbusreg.HuaweiSmartLoggerInventory` | `huawei.SmartLoggerInventory` |
| `modbusreg.HuaweiSmartLoggerInventoryChild` | `huawei.SmartLoggerInventoryChild` |
| `modbusreg.HuaweiSmartLoggerInventoryObject` | `huawei.SmartLoggerInventoryObject` |
| `modbusreg.HuaweiSmartLoggerInventoryPage` | `huawei.SmartLoggerInventoryPage` |
| `modbusreg.HuaweiSmartLoggerObservedBasicReply` | `huawei.SmartLoggerObservedBasicReply` |
| `modbusreg.HuaweiSmartLoggerObservedInventoryInput` | `huawei.SmartLoggerObservedInventoryInput` |
| `modbusreg.HuaweiSmartLoggerObservedInventoryPage` | `huawei.SmartLoggerObservedInventoryPage` |
| `modbusreg.HuaweiSmartLoggerObservedQualificationCard` | `huawei.SmartLoggerObservedQualificationCard` |
| `modbusreg.HuaweiSmartLoggerObservedQualificationCardID` | `huawei.SmartLoggerObservedQualificationCardID` |
| `modbusreg.HuaweiSmartLoggerObservedQualificationInput` | `huawei.SmartLoggerObservedQualificationInput` |
| `modbusreg.HuaweiSmartLoggerOfflineInventoryInput` | `huawei.SmartLoggerOfflineInventoryInput` |
| `modbusreg.HuaweiSmartLoggerQualificationCard` | `huawei.SmartLoggerQualificationCard` |
| `modbusreg.HuaweiSmartLoggerQualificationCardID` | `huawei.SmartLoggerQualificationCardID` |
| `modbusreg.HuaweiSmartLoggerQualificationInput` | `huawei.SmartLoggerQualificationInput` |
| `modbusreg.HuaweiSmartLoggerReadOnlyProvider` | `huawei.SmartLoggerReadOnlyProvider` |
| `modbusreg.HuaweiSmartLoggerReadOnlyProviderID` | `huawei.SmartLoggerReadOnlyProviderID` |
| `modbusreg.HuaweiVendorDocumentIdentity` | `huawei.VendorDocumentIdentity` |
| `modbusreg.NewHuaweiEMMAObservedQualificationCard` | `huawei.NewEMMAObservedQualificationCard` |
| `modbusreg.NewHuaweiEMMAQualificationCard` | `huawei.NewEMMAQualificationCard` |
| `modbusreg.NewHuaweiQualificationPackage` | `huawei.NewQualificationPackage` |
| `modbusreg.NewHuaweiSDongleObservedQualificationCard` | `huawei.NewSDongleObservedQualificationCard` |
| `modbusreg.NewHuaweiSDongleQualificationCard` | `huawei.NewSDongleQualificationCard` |
| `modbusreg.NewHuaweiSDongleReadOnlyProvider` | `huawei.NewSDongleReadOnlyProvider` |
| `modbusreg.NewHuaweiSUN2000MonitoringObserver` | `huawei.NewSUN2000MonitoringObserver` |
| `modbusreg.NewHuaweiSmartLoggerObservedQualificationCard` | `huawei.NewSmartLoggerObservedQualificationCard` |
| `modbusreg.NewHuaweiSmartLoggerQualificationCard` | `huawei.NewSmartLoggerQualificationCard` |
| `modbusreg.NewHuaweiSmartLoggerReadOnlyProvider` | `huawei.NewSmartLoggerReadOnlyProvider` |

## Tesla package (#377)

The Tesla profiles move from the module root to package `github.com/Project-Helianthus/helianthus-modbusreg/tesla`. Exported names that began with `Tesla` lose that prefix and constructors follow their types; other exported names change only their qualifier; methods, fields, wire values and behavior are unchanged. No alias or forwarding declaration remains in the root package. The root contract identity rule the Tesla code shares is exported as `modbusreg.ValidIdentity`. Consumers replace each previous name with the current one:

| Previous name | Current name |
| --- | --- |
| `modbusreg.NewTeslaFC100WCVitalsExecutor` | `tesla.NewFC100WCVitalsExecutor` |
| `modbusreg.NewTeslaFC100WCVitalsRTUSessionAdapter` | `tesla.NewFC100WCVitalsRTUSessionAdapter` |
| `modbusreg.NewTeslaGen3HSCNativeOperationRecord` | `tesla.NewGen3HSCNativeOperationRecord` |
| `modbusreg.NewTeslaGen3HSCProfile` | `tesla.NewGen3HSCProfile` |
| `modbusreg.NewTeslaGen3InteroperableCurrentLimit` | `tesla.NewGen3InteroperableCurrentLimit` |
| `modbusreg.NewTeslaGen3PersistentCurrentLimit` | `tesla.NewGen3PersistentCurrentLimit` |
| `modbusreg.NewTeslaGen3ProvisionalCurrentLimit` | `tesla.NewGen3ProvisionalCurrentLimit` |
| `modbusreg.NewTeslaGen3ReadAcquisition` | `tesla.NewGen3ReadAcquisition` |
| `modbusreg.NewTeslaHSCProfile` | `tesla.NewHSCProfile` |
| `modbusreg.NewTeslaHSCProvenance` | `tesla.NewHSCProvenance` |
| `modbusreg.NewTeslaHSCSyntacticRequest` | `tesla.NewHSCSyntacticRequest` |
| `modbusreg.NewTeslaLegacyFBE0Observation` | `tesla.NewLegacyFBE0Observation` |
| `modbusreg.NewTeslaLegacyFDE0Observation` | `tesla.NewLegacyFDE0Observation` |
| `modbusreg.NewTeslaLegacyWallConnectorProfile` | `tesla.NewLegacyWallConnectorProfile` |
| `modbusreg.NewTeslaTEDAPISemanticRegistry` | `tesla.NewTEDAPISemanticRegistry` |
| `modbusreg.TeslaFC100CommonSystemInfoIntermediate` | `tesla.FC100CommonSystemInfoIntermediate` |
| `modbusreg.TeslaFC100CommonSystemInfoReplay` | `tesla.FC100CommonSystemInfoReplay` |
| `modbusreg.TeslaFC100CommonSystemInfoReplayKind` | `tesla.FC100CommonSystemInfoReplayKind` |
| `modbusreg.TeslaFC100CommonSystemInfoTerminal` | `tesla.FC100CommonSystemInfoTerminal` |
| `modbusreg.TeslaFC100Operation` | `tesla.FC100Operation` |
| `modbusreg.TeslaFC100OperationApplicationError` | `tesla.FC100OperationApplicationError` |
| `modbusreg.TeslaFC100OperationCommonSystemInfo` | `tesla.FC100OperationCommonSystemInfo` |
| `modbusreg.TeslaFC100OperationIntermediate` | `tesla.FC100OperationIntermediate` |
| `modbusreg.TeslaFC100OperationNames` | `tesla.FC100OperationNames` |
| `modbusreg.TeslaFC100OperationResult` | `tesla.FC100OperationResult` |
| `modbusreg.TeslaFC100OperationResultKind` | `tesla.FC100OperationResultKind` |
| `modbusreg.TeslaFC100OperationTerminal` | `tesla.FC100OperationTerminal` |
| `modbusreg.TeslaFC100OperationWCConfigureSettings` | `tesla.FC100OperationWCConfigureSettings` |
| `modbusreg.TeslaFC100OperationWCGetLifetimeStats` | `tesla.FC100OperationWCGetLifetimeStats` |
| `modbusreg.TeslaFC100OperationWCGetLoadSharingState` | `tesla.FC100OperationWCGetLoadSharingState` |
| `modbusreg.TeslaFC100OperationWCGetProvisional` | `tesla.FC100OperationWCGetProvisional` |
| `modbusreg.TeslaFC100OperationWCGetSystemInfo` | `tesla.FC100OperationWCGetSystemInfo` |
| `modbusreg.TeslaFC100OperationWCGetVitals` | `tesla.FC100OperationWCGetVitals` |
| `modbusreg.TeslaFC100OperationWCSetProvisional` | `tesla.FC100OperationWCSetProvisional` |
| `modbusreg.TeslaFC100RecoveredNames` | `tesla.FC100RecoveredNames` |
| `modbusreg.TeslaFC100RequestMatrix` | `tesla.FC100RequestMatrix` |
| `modbusreg.TeslaFC100TEDAPI` | `tesla.FC100TEDAPI` |
| `modbusreg.TeslaFC100WCLifetimeIntermediate` | `tesla.FC100WCLifetimeIntermediate` |
| `modbusreg.TeslaFC100WCLifetimeReplay` | `tesla.FC100WCLifetimeReplay` |
| `modbusreg.TeslaFC100WCLifetimeReplayKind` | `tesla.FC100WCLifetimeReplayKind` |
| `modbusreg.TeslaFC100WCLifetimeTerminal` | `tesla.FC100WCLifetimeTerminal` |
| `modbusreg.TeslaFC100WCLoadSharingStateIntermediate` | `tesla.FC100WCLoadSharingStateIntermediate` |
| `modbusreg.TeslaFC100WCLoadSharingStateReplay` | `tesla.FC100WCLoadSharingStateReplay` |
| `modbusreg.TeslaFC100WCLoadSharingStateReplayKind` | `tesla.FC100WCLoadSharingStateReplayKind` |
| `modbusreg.TeslaFC100WCLoadSharingStateTerminal` | `tesla.FC100WCLoadSharingStateTerminal` |
| `modbusreg.TeslaFC100WCSystemInfoIntermediate` | `tesla.FC100WCSystemInfoIntermediate` |
| `modbusreg.TeslaFC100WCSystemInfoReplay` | `tesla.FC100WCSystemInfoReplay` |
| `modbusreg.TeslaFC100WCSystemInfoReplayKind` | `tesla.FC100WCSystemInfoReplayKind` |
| `modbusreg.TeslaFC100WCSystemInfoTerminal` | `tesla.FC100WCSystemInfoTerminal` |
| `modbusreg.TeslaFC100WCVitalsExecutor` | `tesla.FC100WCVitalsExecutor` |
| `modbusreg.TeslaFC100WCVitalsIntermediate` | `tesla.FC100WCVitalsIntermediate` |
| `modbusreg.TeslaFC100WCVitalsRTUSession` | `tesla.FC100WCVitalsRTUSession` |
| `modbusreg.TeslaFC100WCVitalsRTUSessionAdapter` | `tesla.FC100WCVitalsRTUSessionAdapter` |
| `modbusreg.TeslaFC100WCVitalsReplay` | `tesla.FC100WCVitalsReplay` |
| `modbusreg.TeslaFC100WCVitalsReplayKind` | `tesla.FC100WCVitalsReplayKind` |
| `modbusreg.TeslaFC100WCVitalsTerminal` | `tesla.FC100WCVitalsTerminal` |
| `modbusreg.TeslaFC100WireEntry` | `tesla.FC100WireEntry` |
| `modbusreg.TeslaGen3CurrentLimitOperationVersion24443` | `tesla.Gen3CurrentLimitOperationVersion24443` |
| `modbusreg.TeslaGen3HSCActivationRequest` | `tesla.Gen3HSCActivationRequest` |
| `modbusreg.TeslaGen3HSCActivationWitness` | `tesla.Gen3HSCActivationWitness` |
| `modbusreg.TeslaGen3HSCExceptionOutcome` | `tesla.Gen3HSCExceptionOutcome` |
| `modbusreg.TeslaGen3HSCNativeOperationDirection` | `tesla.Gen3HSCNativeOperationDirection` |
| `modbusreg.TeslaGen3HSCNativeOperationOutcome` | `tesla.Gen3HSCNativeOperationOutcome` |
| `modbusreg.TeslaGen3HSCNativeOperationRecord` | `tesla.Gen3HSCNativeOperationRecord` |
| `modbusreg.TeslaGen3HSCNativeOperationRecordConfig` | `tesla.Gen3HSCNativeOperationRecordConfig` |
| `modbusreg.TeslaGen3HSCNormalOutcome` | `tesla.Gen3HSCNormalOutcome` |
| `modbusreg.TeslaGen3HSCProfile` | `tesla.Gen3HSCProfile` |
| `modbusreg.TeslaGen3HSCProfileConfig` | `tesla.Gen3HSCProfileConfig` |
| `modbusreg.TeslaGen3HSCRequestDirection` | `tesla.Gen3HSCRequestDirection` |
| `modbusreg.TeslaGen3HSCResponseDirection` | `tesla.Gen3HSCResponseDirection` |
| `modbusreg.TeslaGen3HSCVersionCompatibleCandidate` | `tesla.Gen3HSCVersionCompatibleCandidate` |
| `modbusreg.TeslaGen3HSCVersionDisposition` | `tesla.Gen3HSCVersionDisposition` |
| `modbusreg.TeslaGen3HSCVersionKnownObservation` | `tesla.Gen3HSCVersionKnownObservation` |
| `modbusreg.TeslaGen3HSCVersionUnknown` | `tesla.Gen3HSCVersionUnknown` |
| `modbusreg.TeslaGen3InteroperableCurrentLimit` | `tesla.Gen3InteroperableCurrentLimit` |
| `modbusreg.TeslaGen3PersistentCurrentLimit` | `tesla.Gen3PersistentCurrentLimit` |
| `modbusreg.TeslaGen3PersistentCurrentLimitSpec` | `tesla.Gen3PersistentCurrentLimitSpec` |
| `modbusreg.TeslaGen3ProvisionalCurrentLimit` | `tesla.Gen3ProvisionalCurrentLimit` |
| `modbusreg.TeslaGen3ProvisionalCurrentLimitSpec` | `tesla.Gen3ProvisionalCurrentLimitSpec` |
| `modbusreg.TeslaGen3ReadAcquisition` | `tesla.Gen3ReadAcquisition` |
| `modbusreg.TeslaGen3ReadAcquisitionConfig` | `tesla.Gen3ReadAcquisitionConfig` |
| `modbusreg.TeslaGen3ReadEndpoint` | `tesla.Gen3ReadEndpoint` |
| `modbusreg.TeslaGen3ReadEndpointOpener` | `tesla.Gen3ReadEndpointOpener` |
| `modbusreg.TeslaGen3ReadEvidenceAck` | `tesla.Gen3ReadEvidenceAck` |
| `modbusreg.TeslaGen3ReadEvidenceIndeterminate` | `tesla.Gen3ReadEvidenceIndeterminate` |
| `modbusreg.TeslaGen3ReadEvidenceNoSend` | `tesla.Gen3ReadEvidenceNoSend` |
| `modbusreg.TeslaGen3ReadEvidenceRejected` | `tesla.Gen3ReadEvidenceRejected` |
| `modbusreg.TeslaGen3ReadEvidenceState` | `tesla.Gen3ReadEvidenceState` |
| `modbusreg.TeslaGen3ReadEvidenceTimeout` | `tesla.Gen3ReadEvidenceTimeout` |
| `modbusreg.TeslaGen3ReadLifecycleActivating` | `tesla.Gen3ReadLifecycleActivating` |
| `modbusreg.TeslaGen3ReadLifecycleClosed` | `tesla.Gen3ReadLifecycleClosed` |
| `modbusreg.TeslaGen3ReadLifecycleReady` | `tesla.Gen3ReadLifecycleReady` |
| `modbusreg.TeslaGen3ReadLifecycleRecovering` | `tesla.Gen3ReadLifecycleRecovering` |
| `modbusreg.TeslaGen3ReadLifecycleState` | `tesla.Gen3ReadLifecycleState` |
| `modbusreg.TeslaGen3ReadLifecycleStopped` | `tesla.Gen3ReadLifecycleStopped` |
| `modbusreg.TeslaGen3ReadLifecycleUnavailable` | `tesla.Gen3ReadLifecycleUnavailable` |
| `modbusreg.TeslaGen3ReadRecord` | `tesla.Gen3ReadRecord` |
| `modbusreg.TeslaGen3ReadRetained` | `tesla.Gen3ReadRetained` |
| `modbusreg.TeslaGen3ReadSourceRTUPrivateFunction` | `tesla.Gen3ReadSourceRTUPrivateFunction` |
| `modbusreg.TeslaGen3ReadWindow` | `tesla.Gen3ReadWindow` |
| `modbusreg.TeslaHSCCompatibilityV1` | `tesla.HSCCompatibilityV1` |
| `modbusreg.TeslaHSCDisabled` | `tesla.HSCDisabled` |
| `modbusreg.TeslaHSCDisposition` | `tesla.HSCDisposition` |
| `modbusreg.TeslaHSCEnvelope` | `tesla.HSCEnvelope` |
| `modbusreg.TeslaHSCException` | `tesla.HSCException` |
| `modbusreg.TeslaHSCExceptionCodecFailure` | `tesla.HSCExceptionCodecFailure` |
| `modbusreg.TeslaHSCExceptionUnknown` | `tesla.HSCExceptionUnknown` |
| `modbusreg.TeslaHSCExceptionUnknownFunction` | `tesla.HSCExceptionUnknownFunction` |
| `modbusreg.TeslaHSCFC100OperationCompatibilityV1` | `tesla.HSCFC100OperationCompatibilityV1` |
| `modbusreg.TeslaHSCFramingOnly` | `tesla.HSCFramingOnly` |
| `modbusreg.TeslaHSCNativeRecord` | `tesla.HSCNativeRecord` |
| `modbusreg.TeslaHSCOpaqueTerminalProvenance` | `tesla.HSCOpaqueTerminalProvenance` |
| `modbusreg.TeslaHSCProfile` | `tesla.HSCProfile` |
| `modbusreg.TeslaHSCProfileConfig` | `tesla.HSCProfileConfig` |
| `modbusreg.TeslaHSCProfileName` | `tesla.HSCProfileName` |
| `modbusreg.TeslaHSCProvenance` | `tesla.HSCProvenance` |
| `modbusreg.TeslaHSCQualifiedReadOnly` | `tesla.HSCQualifiedReadOnly` |
| `modbusreg.TeslaHSCResponse` | `tesla.HSCResponse` |
| `modbusreg.TeslaHSCSyntacticRequest` | `tesla.HSCSyntacticRequest` |
| `modbusreg.TeslaHSCSystemInfoCompatibilityV1` | `tesla.HSCSystemInfoCompatibilityV1` |
| `modbusreg.TeslaHSCWCLifetimeCompatibilityV1` | `tesla.HSCWCLifetimeCompatibilityV1` |
| `modbusreg.TeslaHSCWCLoadSharingStateCompatibilityV1` | `tesla.HSCWCLoadSharingStateCompatibilityV1` |
| `modbusreg.TeslaHSCWCSystemInfoCompatibilityV1` | `tesla.HSCWCSystemInfoCompatibilityV1` |
| `modbusreg.TeslaHSCWCVitalsCompatibilityV1` | `tesla.HSCWCVitalsCompatibilityV1` |
| `modbusreg.TeslaLegacyBuildConfirmed` | `tesla.LegacyBuildConfirmed` |
| `modbusreg.TeslaLegacyCommand` | `tesla.LegacyCommand` |
| `modbusreg.TeslaLegacyDirection` | `tesla.LegacyDirection` |
| `modbusreg.TeslaLegacyFBE0Observation` | `tesla.LegacyFBE0Observation` |
| `modbusreg.TeslaLegacyFBE0ObservationSpec` | `tesla.LegacyFBE0ObservationSpec` |
| `modbusreg.TeslaLegacyFDE0Observation` | `tesla.LegacyFDE0Observation` |
| `modbusreg.TeslaLegacyFDE0ObservationSpec` | `tesla.LegacyFDE0ObservationSpec` |
| `modbusreg.TeslaLegacyFDE0State` | `tesla.LegacyFDE0State` |
| `modbusreg.TeslaLegacyFDE0StateCharging` | `tesla.LegacyFDE0StateCharging` |
| `modbusreg.TeslaLegacyFDE0StatePreCharge` | `tesla.LegacyFDE0StatePreCharge` |
| `modbusreg.TeslaLegacyFamilyCompatible` | `tesla.LegacyFamilyCompatible` |
| `modbusreg.TeslaLegacyOperation` | `tesla.LegacyOperation` |
| `modbusreg.TeslaLegacyOperationPrimaryDiscovery` | `tesla.LegacyOperationPrimaryDiscovery` |
| `modbusreg.TeslaLegacyOperationPrimaryHeartbeat` | `tesla.LegacyOperationPrimaryHeartbeat` |
| `modbusreg.TeslaLegacyOperationSecondaryDiscovery` | `tesla.LegacyOperationSecondaryDiscovery` |
| `modbusreg.TeslaLegacyOperationSecondaryHeartbeat` | `tesla.LegacyOperationSecondaryHeartbeat` |
| `modbusreg.TeslaLegacyOperationUnknown` | `tesla.LegacyOperationUnknown` |
| `modbusreg.TeslaLegacyQualification` | `tesla.LegacyQualification` |
| `modbusreg.TeslaLegacyWallConnectorBuildConfirmed` | `tesla.LegacyWallConnectorBuildConfirmed` |
| `modbusreg.TeslaLegacyWallConnectorCompatibility` | `tesla.LegacyWallConnectorCompatibility` |
| `modbusreg.TeslaLegacyWallConnectorFamilyCompatible` | `tesla.LegacyWallConnectorFamilyCompatible` |
| `modbusreg.TeslaLegacyWallConnectorFrame` | `tesla.LegacyWallConnectorFrame` |
| `modbusreg.TeslaLegacyWallConnectorGeneration` | `tesla.LegacyWallConnectorGeneration` |
| `modbusreg.TeslaLegacyWallConnectorGenerationCandidate` | `tesla.LegacyWallConnectorGenerationCandidate` |
| `modbusreg.TeslaLegacyWallConnectorProfile` | `tesla.LegacyWallConnectorProfile` |
| `modbusreg.TeslaLegacyWallConnectorProfileConfig` | `tesla.LegacyWallConnectorProfileConfig` |
| `modbusreg.TeslaLegacyWallConnectorRecord` | `tesla.LegacyWallConnectorRecord` |
| `modbusreg.TeslaLegacyWallConnectorRequest` | `tesla.LegacyWallConnectorRequest` |
| `modbusreg.TeslaLegacyWallConnectorResponse` | `tesla.LegacyWallConnectorResponse` |
| `modbusreg.TeslaLegacyWallConnectorUnknown` | `tesla.LegacyWallConnectorUnknown` |
| `modbusreg.TeslaTEDAPIAdmissionAllowedCommonSystemInfo` | `tesla.TEDAPIAdmissionAllowedCommonSystemInfo` |
| `modbusreg.TeslaTEDAPIAdmissionAllowedWCLifetime` | `tesla.TEDAPIAdmissionAllowedWCLifetime` |
| `modbusreg.TeslaTEDAPIAdmissionAllowedWCLoadSharingState` | `tesla.TEDAPIAdmissionAllowedWCLoadSharingState` |
| `modbusreg.TeslaTEDAPIAdmissionAllowedWCSystemInfo` | `tesla.TEDAPIAdmissionAllowedWCSystemInfo` |
| `modbusreg.TeslaTEDAPIAdmissionAllowedWCVitals` | `tesla.TEDAPIAdmissionAllowedWCVitals` |
| `modbusreg.TeslaTEDAPIAdmissionBlockedNoAdmissibleOperation` | `tesla.TEDAPIAdmissionBlockedNoAdmissibleOperation` |
| `modbusreg.TeslaTEDAPIAdmissionBlockedProfile` | `tesla.TEDAPIAdmissionBlockedProfile` |
| `modbusreg.TeslaTEDAPIAdmissionState` | `tesla.TEDAPIAdmissionState` |
| `modbusreg.TeslaTEDAPIDirection` | `tesla.TEDAPIDirection` |
| `modbusreg.TeslaTEDAPIFramingOnly` | `tesla.TEDAPIFramingOnly` |
| `modbusreg.TeslaTEDAPINativeRecord` | `tesla.TEDAPINativeRecord` |
| `modbusreg.TeslaTEDAPIObservationSpec` | `tesla.TEDAPIObservationSpec` |
| `modbusreg.TeslaTEDAPIOpaqueQualified` | `tesla.TEDAPIOpaqueQualified` |
| `modbusreg.TeslaTEDAPIOperationAdmission` | `tesla.TEDAPIOperationAdmission` |
| `modbusreg.TeslaTEDAPIOperationCommonSystemInfoV1` | `tesla.TEDAPIOperationCommonSystemInfoV1` |
| `modbusreg.TeslaTEDAPIOperationWCLifetimeV1` | `tesla.TEDAPIOperationWCLifetimeV1` |
| `modbusreg.TeslaTEDAPIOperationWCLoadSharingStateV1` | `tesla.TEDAPIOperationWCLoadSharingStateV1` |
| `modbusreg.TeslaTEDAPIOperationWCSystemInfoV1` | `tesla.TEDAPIOperationWCSystemInfoV1` |
| `modbusreg.TeslaTEDAPIOperationWCVitalsV1` | `tesla.TEDAPIOperationWCVitalsV1` |
| `modbusreg.TeslaTEDAPIRequest` | `tesla.TEDAPIRequest` |
| `modbusreg.TeslaTEDAPIResponse` | `tesla.TEDAPIResponse` |
| `modbusreg.TeslaTEDAPISemanticRegistry` | `tesla.TEDAPISemanticRegistry` |
| `modbusreg.TeslaTEDAPISemanticState` | `tesla.TEDAPISemanticState` |

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
