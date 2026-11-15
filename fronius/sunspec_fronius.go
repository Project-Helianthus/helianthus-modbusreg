package fronius

import "slices"

import modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"

// Fronius observed flavor identifiers, one per immutable flavor version. Each
// version pins one Common identity, one exact firmware string and one exact
// model chain; a different firmware or chain needs a new version. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/fronius/sunspec-float-v1.md.
const (
	SunSpecFroniusObservedFlavorID    = "sunspec.flavor.fronius.gen24.float.observed@1.0.0"
	SunSpecFroniusObservedFlavorV11ID = "sunspec.flavor.fronius.gen24.float.observed@1.1.0"
	SunSpecFroniusObservedFlavorV12ID = "sunspec.flavor.fronius.gen24.float.observed@1.2.0"
)

// SunSpecFroniusFlavorReason is the stable reason code of a
// SunSpecFroniusFlavorDecision.
type SunSpecFroniusFlavorReason string

// Fronius flavor reasons, checked in this order: AmbiguousSource (the
// capability found more than one source), CapabilityNotAdmitted,
// CommonIdentityMismatch, FirmwareMismatch and ChainMismatch. Matched means
// every check held.
const (
	SunSpecFroniusFlavorReasonCapabilityNotAdmitted  SunSpecFroniusFlavorReason = "CAPABILITY_NOT_ADMITTED"
	SunSpecFroniusFlavorReasonCommonIdentityMismatch SunSpecFroniusFlavorReason = "COMMON_IDENTITY_MISMATCH"
	SunSpecFroniusFlavorReasonFirmwareMismatch       SunSpecFroniusFlavorReason = "FIRMWARE_MISMATCH"
	SunSpecFroniusFlavorReasonChainMismatch          SunSpecFroniusFlavorReason = "CHAIN_MISMATCH"
	SunSpecFroniusFlavorReasonAmbiguousSource        SunSpecFroniusFlavorReason = "AMBIGUOUS_SOURCE"
	SunSpecFroniusFlavorReasonMatched                SunSpecFroniusFlavorReason = "MATCHED"
)

// SunSpecFroniusFlavorDecision is the immutable result of evaluating one
// Fronius flavor version. It always carries the capability decision it was
// based on; the chain and source views are present only when matched. A match
// is structural evidence for one exact flavor, not a live qualification.
type SunSpecFroniusFlavorDecision struct {
	flavorID   string
	matched    bool
	reason     SunSpecFroniusFlavorReason
	capability modbusreg.SunSpecCapabilityDecision
	chain      []modbusreg.SunSpecWireKey
	views      []modbusreg.LogicalViewSnapshot
}

// Matched reports whether every check of the flavor held.
func (d SunSpecFroniusFlavorDecision) Matched() bool { return d.matched }

// Reason returns the first failed check, or the matched reason.
func (d SunSpecFroniusFlavorDecision) Reason() SunSpecFroniusFlavorReason { return d.reason }

// FlavorID returns the evaluated flavor version; the zero value reports
// SunSpecFroniusObservedFlavorID.
func (d SunSpecFroniusFlavorDecision) FlavorID() string {
	if d.flavorID == "" {
		return SunSpecFroniusObservedFlavorID
	}
	return d.flavorID
}

// Capability returns a copy of the three-phase monitoring decision the
// evaluation was based on.
func (d SunSpecFroniusFlavorDecision) Capability() modbusreg.SunSpecCapabilityDecision {
	return d.capability.Clone()
}

// Chain returns a copy of the matched wire chain, end marker included; it is
// empty unless matched.
func (d SunSpecFroniusFlavorDecision) Chain() []modbusreg.SunSpecWireKey {
	return append([]modbusreg.SunSpecWireKey(nil), d.chain...)
}

// SourceViews returns copies of the snapshot's source views; it is empty
// unless matched.
func (d SunSpecFroniusFlavorDecision) SourceViews() []modbusreg.LogicalViewSnapshot {
	return modbusreg.SunSpecCloneSourceViews(d.views)
}

var froniusObservedChainV1 = []modbusreg.SunSpecWireKey{
	{ModelID: 1, ModelLength: 65},
	{ModelID: 113, ModelLength: 60},
	{ModelID: 120, ModelLength: 26},
	{ModelID: 121, ModelLength: 30},
	{ModelID: 122, ModelLength: 44},
	{ModelID: 160, ModelLength: 88},
	{ModelID: 124, ModelLength: 24},
	{ModelID: modbusreg.SunSpecEndModelID, ModelLength: 0},
}

var froniusObservedChainV11 = []modbusreg.SunSpecWireKey{
	{ModelID: 1, ModelLength: 65},
	{ModelID: 113, ModelLength: 60},
	{ModelID: 120, ModelLength: 26},
	{ModelID: 121, ModelLength: 30},
	{ModelID: 122, ModelLength: 44},
	{ModelID: 123, ModelLength: 24},
	{ModelID: 160, ModelLength: 88},
	{ModelID: 124, ModelLength: 24},
	{ModelID: modbusreg.SunSpecEndModelID, ModelLength: 0},
}

var froniusObservedChainV12 = []modbusreg.SunSpecWireKey{
	{ModelID: 1, ModelLength: 65},
	{ModelID: 113, ModelLength: 60},
	{ModelID: 120, ModelLength: 26},
	{ModelID: 121, ModelLength: 30},
	{ModelID: 122, ModelLength: 44},
	{ModelID: 123, ModelLength: 24},
	{ModelID: 160, ModelLength: 88},
	{ModelID: 124, ModelLength: 24},
	{ModelID: modbusreg.SunSpecEndModelID, ModelLength: 0},
}

// sunSpecFroniusFlavorDefinition is one immutable observed flavor. Each version
// carries its own exact Common firmware string and ordered model chain; a new
// firmware or chain requires a new flavor version, never a range or an edit.
type sunSpecFroniusFlavorDefinition struct {
	id       string
	firmware string
	chain    []modbusreg.SunSpecWireKey
}

var (
	froniusObservedFlavorV1 = sunSpecFroniusFlavorDefinition{
		id: SunSpecFroniusObservedFlavorID, firmware: "1.41.11-1", chain: froniusObservedChainV1,
	}
	froniusObservedFlavorV11 = sunSpecFroniusFlavorDefinition{
		id: SunSpecFroniusObservedFlavorV11ID, firmware: "1.41.11-1", chain: froniusObservedChainV11,
	}
	froniusObservedFlavorV12 = sunSpecFroniusFlavorDefinition{
		id: SunSpecFroniusObservedFlavorV12ID, firmware: "1.42.9-1", chain: froniusObservedChainV12,
	}
)

// SunSpecFroniusFlavorSelectionReason is the stable reason code of a
// SunSpecFroniusFlavorSelection.
type SunSpecFroniusFlavorSelectionReason string

// Selection reasons: NoMatch when no flavor version matched, AmbiguousMatch
// when more than one did, Matched when exactly one did.
const (
	SunSpecFroniusFlavorSelectionReasonNoMatch        SunSpecFroniusFlavorSelectionReason = "NO_MATCH"
	SunSpecFroniusFlavorSelectionReasonAmbiguousMatch SunSpecFroniusFlavorSelectionReason = "AMBIGUOUS_MATCH"
	SunSpecFroniusFlavorSelectionReasonMatched        SunSpecFroniusFlavorSelectionReason = "MATCHED"
)

// SunSpecFroniusFlavorSelection is the result of SelectObservedFlavor:
// every per-version evaluation and, when exactly one version matched, that
// decision. More than one match is ambiguous and selects nothing; version
// order never breaks a tie.
type SunSpecFroniusFlavorSelection struct {
	matched     bool
	reason      SunSpecFroniusFlavorSelectionReason
	decision    *SunSpecFroniusFlavorDecision
	evaluations []SunSpecFroniusFlavorDecision
}

// Matched reports whether exactly one flavor version matched.
func (s SunSpecFroniusFlavorSelection) Matched() bool { return s.matched }

// Reason returns the selection reason.
func (s SunSpecFroniusFlavorSelection) Reason() SunSpecFroniusFlavorSelectionReason {
	return s.reason
}

// Decision returns a copy of the selected decision and true only when Matched.
func (s SunSpecFroniusFlavorSelection) Decision() (SunSpecFroniusFlavorDecision, bool) {
	if s.decision == nil {
		return SunSpecFroniusFlavorDecision{}, false
	}
	return cloneSunSpecFroniusFlavorDecision(*s.decision), true
}

// Evaluations returns copies of every per-version evaluation, in version
// order.
func (s SunSpecFroniusFlavorSelection) Evaluations() []SunSpecFroniusFlavorDecision {
	return cloneSunSpecFroniusFlavorDecisions(s.evaluations)
}

// EvaluateObservedFlavor evaluates flavor version 1.0.0
// (SunSpecFroniusObservedFlavorID) against snapshot with registry. It requires
// an admitted three-phase monitoring capability, then the flavor's exact Common
// manufacturer and model, its exact firmware string and its exact wire chain.
// It never returns an error; the reason names the first failed check.
func EvaluateObservedFlavor(registry modbusreg.SunSpecDecoderRegistry, snapshot modbusreg.SunSpecChainSnapshot) SunSpecFroniusFlavorDecision {
	return evaluateFroniusObservedFlavor(registry, snapshot, froniusObservedFlavorV1)
}

// EvaluateObservedFlavorV11 applies the checks of
// EvaluateObservedFlavor for flavor version 1.1.0
// (SunSpecFroniusObservedFlavorV11ID).
func EvaluateObservedFlavorV11(registry modbusreg.SunSpecDecoderRegistry, snapshot modbusreg.SunSpecChainSnapshot) SunSpecFroniusFlavorDecision {
	return evaluateFroniusObservedFlavor(registry, snapshot, froniusObservedFlavorV11)
}

// EvaluateObservedFlavorV12 applies the checks of
// EvaluateObservedFlavor for flavor version 1.2.0
// (SunSpecFroniusObservedFlavorV12ID).
func EvaluateObservedFlavorV12(registry modbusreg.SunSpecDecoderRegistry, snapshot modbusreg.SunSpecChainSnapshot) SunSpecFroniusFlavorDecision {
	return evaluateFroniusObservedFlavor(registry, snapshot, froniusObservedFlavorV12)
}

// SelectObservedFlavor evaluates every Fronius flavor version against
// snapshot with registry and selects one only when exactly one matches.
func SelectObservedFlavor(registry modbusreg.SunSpecDecoderRegistry, snapshot modbusreg.SunSpecChainSnapshot) SunSpecFroniusFlavorSelection {
	return selectSunSpecFroniusFlavor([]SunSpecFroniusFlavorDecision{
		EvaluateObservedFlavor(registry, snapshot),
		EvaluateObservedFlavorV11(registry, snapshot),
		EvaluateObservedFlavorV12(registry, snapshot),
	})
}

func evaluateFroniusObservedFlavor(registry modbusreg.SunSpecDecoderRegistry, snapshot modbusreg.SunSpecChainSnapshot, definition sunSpecFroniusFlavorDefinition) SunSpecFroniusFlavorDecision {
	capability := registry.EvaluateThreePhaseMonitoring(snapshot)
	rejected := func(reason SunSpecFroniusFlavorReason) SunSpecFroniusFlavorDecision {
		return SunSpecFroniusFlavorDecision{flavorID: definition.id, reason: reason, capability: capability}
	}
	if capability.Reason() == modbusreg.SunSpecCapabilityReasonAmbiguousSource {
		return rejected(SunSpecFroniusFlavorReasonAmbiguousSource)
	}
	if !capability.Admitted() {
		return rejected(SunSpecFroniusFlavorReasonCapabilityNotAdmitted)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) == 0 {
		return rejected(SunSpecFroniusFlavorReasonChainMismatch)
	}
	common, err := registry.DecodeOccurrence(occurrences[0])
	if err != nil {
		return rejected(SunSpecFroniusFlavorReasonCommonIdentityMismatch)
	}
	manufacturer, manufacturerOK := common.TextFact("device.manufacturer")
	model, modelOK := common.TextFact("device.model")
	if !manufacturerOK || !modelOK || manufacturer != "Fronius" || model != "Symo GEN24 10.0" {
		return rejected(SunSpecFroniusFlavorReasonCommonIdentityMismatch)
	}
	firmware, firmwareOK := common.TextFact("device.version")
	if !firmwareOK || definition.firmware == "" || firmware != definition.firmware {
		return rejected(SunSpecFroniusFlavorReasonFirmwareMismatch)
	}
	chain, valid := snapshot.WireChain()
	if !valid || !slices.Equal(chain, definition.chain) {
		return rejected(SunSpecFroniusFlavorReasonChainMismatch)
	}
	return SunSpecFroniusFlavorDecision{
		flavorID:   definition.id,
		matched:    true,
		reason:     SunSpecFroniusFlavorReasonMatched,
		capability: capability,
		chain:      chain,
		views:      snapshot.SourceViews(),
	}
}

func selectSunSpecFroniusFlavor(evaluations []SunSpecFroniusFlavorDecision) SunSpecFroniusFlavorSelection {
	selection := SunSpecFroniusFlavorSelection{
		reason:      SunSpecFroniusFlavorSelectionReasonNoMatch,
		evaluations: cloneSunSpecFroniusFlavorDecisions(evaluations),
	}
	for index := range evaluations {
		if !evaluations[index].Matched() {
			continue
		}
		if selection.decision != nil {
			selection.decision = nil
			selection.reason = SunSpecFroniusFlavorSelectionReasonAmbiguousMatch
			return selection
		}
		decision := cloneSunSpecFroniusFlavorDecision(evaluations[index])
		selection.decision = &decision
	}
	if selection.decision != nil {
		selection.matched = true
		selection.reason = SunSpecFroniusFlavorSelectionReasonMatched
	}
	return selection
}

func cloneSunSpecFroniusFlavorDecision(decision SunSpecFroniusFlavorDecision) SunSpecFroniusFlavorDecision {
	decision.capability = decision.capability.Clone()
	decision.chain = append([]modbusreg.SunSpecWireKey(nil), decision.chain...)
	decision.views = modbusreg.SunSpecCloneSourceViews(decision.views)
	return decision
}

func cloneSunSpecFroniusFlavorDecisions(decisions []SunSpecFroniusFlavorDecision) []SunSpecFroniusFlavorDecision {
	out := make([]SunSpecFroniusFlavorDecision, len(decisions))
	for index := range decisions {
		out[index] = cloneSunSpecFroniusFlavorDecision(decisions[index])
	}
	return out
}
