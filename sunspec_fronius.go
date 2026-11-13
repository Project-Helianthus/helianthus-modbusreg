package modbusreg

import "slices"

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
	capability SunSpecCapabilityDecision
	chain      []SunSpecWireKey
	views      []LogicalViewSnapshot
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
func (d SunSpecFroniusFlavorDecision) Capability() SunSpecCapabilityDecision {
	return cloneSunSpecCapabilityDecision(d.capability)
}

// Chain returns a copy of the matched wire chain, end marker included; it is
// empty unless matched.
func (d SunSpecFroniusFlavorDecision) Chain() []SunSpecWireKey {
	return append([]SunSpecWireKey(nil), d.chain...)
}

// SourceViews returns copies of the snapshot's source views; it is empty
// unless matched.
func (d SunSpecFroniusFlavorDecision) SourceViews() []LogicalViewSnapshot {
	return cloneSunSpecLogicalViewSnapshots(d.views)
}

var froniusObservedChainV1 = []SunSpecWireKey{
	{ModelID: 1, ModelLength: 65},
	{ModelID: 113, ModelLength: 60},
	{ModelID: 120, ModelLength: 26},
	{ModelID: 121, ModelLength: 30},
	{ModelID: 122, ModelLength: 44},
	{ModelID: 160, ModelLength: 88},
	{ModelID: 124, ModelLength: 24},
	{ModelID: sunSpecEndModel, ModelLength: 0},
}

var froniusObservedChainV11 = []SunSpecWireKey{
	{ModelID: 1, ModelLength: 65},
	{ModelID: 113, ModelLength: 60},
	{ModelID: 120, ModelLength: 26},
	{ModelID: 121, ModelLength: 30},
	{ModelID: 122, ModelLength: 44},
	{ModelID: 123, ModelLength: 24},
	{ModelID: 160, ModelLength: 88},
	{ModelID: 124, ModelLength: 24},
	{ModelID: sunSpecEndModel, ModelLength: 0},
}

var froniusObservedChainV12 = []SunSpecWireKey{
	{ModelID: 1, ModelLength: 65},
	{ModelID: 113, ModelLength: 60},
	{ModelID: 120, ModelLength: 26},
	{ModelID: 121, ModelLength: 30},
	{ModelID: 122, ModelLength: 44},
	{ModelID: 123, ModelLength: 24},
	{ModelID: 160, ModelLength: 88},
	{ModelID: 124, ModelLength: 24},
	{ModelID: sunSpecEndModel, ModelLength: 0},
}

// sunSpecFroniusFlavorDefinition is one immutable observed flavor. Each version
// carries its own exact Common firmware string and ordered model chain; a new
// firmware or chain requires a new flavor version, never a range or an edit.
type sunSpecFroniusFlavorDefinition struct {
	id       string
	firmware string
	chain    []SunSpecWireKey
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

// SunSpecFroniusFlavorSelection is the result of SelectFroniusObservedFlavor:
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

// EvaluateFroniusObservedFlavor evaluates flavor version 1.0.0
// (SunSpecFroniusObservedFlavorID) against snapshot. It requires an admitted
// three-phase monitoring capability, then the flavor's exact Common
// manufacturer and model, its exact firmware string and its exact wire chain.
// It never returns an error; the reason names the first failed check.
func (r SunSpecDecoderRegistry) EvaluateFroniusObservedFlavor(snapshot SunSpecChainSnapshot) SunSpecFroniusFlavorDecision {
	return r.evaluateFroniusObservedFlavor(snapshot, froniusObservedFlavorV1)
}

// EvaluateFroniusObservedFlavorV11 applies the checks of
// EvaluateFroniusObservedFlavor for flavor version 1.1.0
// (SunSpecFroniusObservedFlavorV11ID).
func (r SunSpecDecoderRegistry) EvaluateFroniusObservedFlavorV11(snapshot SunSpecChainSnapshot) SunSpecFroniusFlavorDecision {
	return r.evaluateFroniusObservedFlavor(snapshot, froniusObservedFlavorV11)
}

// EvaluateFroniusObservedFlavorV12 applies the checks of
// EvaluateFroniusObservedFlavor for flavor version 1.2.0
// (SunSpecFroniusObservedFlavorV12ID).
func (r SunSpecDecoderRegistry) EvaluateFroniusObservedFlavorV12(snapshot SunSpecChainSnapshot) SunSpecFroniusFlavorDecision {
	return r.evaluateFroniusObservedFlavor(snapshot, froniusObservedFlavorV12)
}

// SelectFroniusObservedFlavor evaluates every Fronius flavor version against
// snapshot and selects one only when exactly one matches.
func (r SunSpecDecoderRegistry) SelectFroniusObservedFlavor(snapshot SunSpecChainSnapshot) SunSpecFroniusFlavorSelection {
	return selectSunSpecFroniusFlavor([]SunSpecFroniusFlavorDecision{
		r.EvaluateFroniusObservedFlavor(snapshot),
		r.EvaluateFroniusObservedFlavorV11(snapshot),
		r.EvaluateFroniusObservedFlavorV12(snapshot),
	})
}

func (r SunSpecDecoderRegistry) evaluateFroniusObservedFlavor(snapshot SunSpecChainSnapshot, definition sunSpecFroniusFlavorDefinition) SunSpecFroniusFlavorDecision {
	capability := r.EvaluateThreePhaseMonitoring(snapshot)
	rejected := func(reason SunSpecFroniusFlavorReason) SunSpecFroniusFlavorDecision {
		return SunSpecFroniusFlavorDecision{flavorID: definition.id, reason: reason, capability: capability}
	}
	if capability.Reason() == SunSpecCapabilityReasonAmbiguousSource {
		return rejected(SunSpecFroniusFlavorReasonAmbiguousSource)
	}
	if !capability.Admitted() {
		return rejected(SunSpecFroniusFlavorReasonCapabilityNotAdmitted)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) == 0 {
		return rejected(SunSpecFroniusFlavorReasonChainMismatch)
	}
	common, err := r.DecodeOccurrence(occurrences[0])
	if err != nil {
		return rejected(SunSpecFroniusFlavorReasonCommonIdentityMismatch)
	}
	manufacturer, manufacturerOK := sunSpecTextFact(common, "device.manufacturer")
	model, modelOK := sunSpecTextFact(common, "device.model")
	if !manufacturerOK || !modelOK || manufacturer != "Fronius" || model != "Symo GEN24 10.0" {
		return rejected(SunSpecFroniusFlavorReasonCommonIdentityMismatch)
	}
	firmware, firmwareOK := sunSpecTextFact(common, "device.version")
	if !firmwareOK || definition.firmware == "" || firmware != definition.firmware {
		return rejected(SunSpecFroniusFlavorReasonFirmwareMismatch)
	}
	chain, valid := sunSpecSnapshotWireChain(snapshot)
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
	decision.capability = cloneSunSpecCapabilityDecision(decision.capability)
	decision.chain = append([]SunSpecWireKey(nil), decision.chain...)
	decision.views = cloneSunSpecLogicalViewSnapshots(decision.views)
	return decision
}

func cloneSunSpecFroniusFlavorDecisions(decisions []SunSpecFroniusFlavorDecision) []SunSpecFroniusFlavorDecision {
	out := make([]SunSpecFroniusFlavorDecision, len(decisions))
	for index := range decisions {
		out[index] = cloneSunSpecFroniusFlavorDecision(decisions[index])
	}
	return out
}

func sunSpecTextFact(model SunSpecDecodedModel, fieldID string) (string, bool) {
	fact, ok := model.Fact(fieldID)
	if !ok || fact.Value.State() != SunSpecValueValid {
		return "", false
	}
	return fact.Value.Text()
}

func sunSpecSnapshotWireChain(snapshot SunSpecChainSnapshot) ([]SunSpecWireKey, bool) {
	raw := snapshot.RawWords()
	if len(raw) < 4 || raw[0] != sunSpecSignatureFirst || raw[1] != sunSpecSignatureSecond {
		return nil, false
	}
	occurrences := snapshot.Occurrences()
	views := snapshot.SourceViews()
	viewByID := make(map[uint64]LogicalViewRecord, len(views))
	for _, view := range views {
		record := view.Record()
		if record.LogicalViewID == 0 {
			return nil, false
		}
		if _, duplicate := viewByID[record.LogicalViewID]; duplicate {
			return nil, false
		}
		viewByID[record.LogicalViewID] = record
	}
	keys := make([]SunSpecWireKey, 0, len(occurrences)+1)
	offset := 2
	var nextHeader uint32
	for index, occurrence := range occurrences {
		if occurrence.Ordinal != uint32(index+1) || offset+2 > len(raw) {
			return nil, false
		}
		key := SunSpecWireKey{ModelID: raw[offset], ModelLength: raw[offset+1]}
		end := offset + 2 + int(key.ModelLength)
		if key.ModelID == sunSpecEndModel || key.ModelLength == 0 || end > len(raw) || key != occurrence.WireKey || !slices.Equal(raw[offset:end], occurrence.Words()) {
			return nil, false
		}
		if index == 0 {
			if occurrence.HeaderOffset < 2 {
				return nil, false
			}
			nextHeader = uint32(occurrence.HeaderOffset)
		}
		if uint32(occurrence.HeaderOffset) != nextHeader || uint32(occurrence.PayloadOffset) != nextHeader+2 {
			return nil, false
		}
		wordCursor := 0
		for _, span := range occurrence.SourceSpans() {
			record, ok := viewByID[span.LogicalViewID]
			spanEnd := wordCursor + int(span.WordCount)
			if !ok || span.WordCount == 0 || spanEnd > len(occurrence.words) || span.PDUOffset != record.SliceOffset || span.WordCount != record.SliceWordCount || len(record.Words) != int(span.WordCount) || !slices.Equal(record.Words, occurrence.words[wordCursor:spanEnd]) {
				return nil, false
			}
			wordCursor = spanEnd
		}
		if wordCursor != len(occurrence.words) {
			return nil, false
		}
		keys = append(keys, key)
		offset = end
		nextHeader += uint32(len(occurrence.words))
		if nextHeader > 65535 {
			return nil, false
		}
	}
	if offset+2 != len(raw) || raw[offset] != sunSpecEndModel || raw[offset+1] != 0 {
		return nil, false
	}
	return append(keys, SunSpecWireKey{ModelID: sunSpecEndModel, ModelLength: 0}), true
}

func cloneSunSpecCapabilityDecision(decision SunSpecCapabilityDecision) SunSpecCapabilityDecision {
	out := decision
	if decision.source != nil {
		source := cloneOccurrence(*decision.source)
		out.source = &source
	}
	out.facts = cloneSunSpecCapabilityFacts(decision.facts)
	out.views = cloneSunSpecLogicalViewSnapshots(decision.views)
	return out
}
