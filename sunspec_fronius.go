package modbusreg

import "slices"

const SunSpecFroniusObservedFlavorID = "sunspec.flavor.fronius.gen24.float.observed@1.0.0"

type SunSpecFroniusFlavorReason string

const (
	SunSpecFroniusFlavorReasonCapabilityNotAdmitted  SunSpecFroniusFlavorReason = "CAPABILITY_NOT_ADMITTED"
	SunSpecFroniusFlavorReasonCommonIdentityMismatch SunSpecFroniusFlavorReason = "COMMON_IDENTITY_MISMATCH"
	SunSpecFroniusFlavorReasonFirmwareMismatch       SunSpecFroniusFlavorReason = "FIRMWARE_MISMATCH"
	SunSpecFroniusFlavorReasonChainMismatch          SunSpecFroniusFlavorReason = "CHAIN_MISMATCH"
	SunSpecFroniusFlavorReasonAmbiguousSource        SunSpecFroniusFlavorReason = "AMBIGUOUS_SOURCE"
	SunSpecFroniusFlavorReasonMatched                SunSpecFroniusFlavorReason = "MATCHED"
)

type SunSpecFroniusFlavorDecision struct {
	matched    bool
	reason     SunSpecFroniusFlavorReason
	capability SunSpecCapabilityDecision
	chain      []SunSpecWireKey
	views      []LogicalViewSnapshot
}

func (d SunSpecFroniusFlavorDecision) Matched() bool                      { return d.matched }
func (d SunSpecFroniusFlavorDecision) Reason() SunSpecFroniusFlavorReason { return d.reason }
func (d SunSpecFroniusFlavorDecision) FlavorID() string                   { return SunSpecFroniusObservedFlavorID }
func (d SunSpecFroniusFlavorDecision) Capability() SunSpecCapabilityDecision {
	return cloneSunSpecCapabilityDecision(d.capability)
}
func (d SunSpecFroniusFlavorDecision) Chain() []SunSpecWireKey {
	return append([]SunSpecWireKey(nil), d.chain...)
}
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

func (r SunSpecDecoderRegistry) EvaluateFroniusObservedFlavor(snapshot SunSpecChainSnapshot) SunSpecFroniusFlavorDecision {
	capability := r.EvaluateThreePhaseMonitoring(snapshot)
	rejected := func(reason SunSpecFroniusFlavorReason) SunSpecFroniusFlavorDecision {
		return SunSpecFroniusFlavorDecision{reason: reason, capability: capability}
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
	if !firmwareOK || firmware != "1.41.11-1" {
		return rejected(SunSpecFroniusFlavorReasonFirmwareMismatch)
	}
	chain, valid := sunSpecSnapshotWireChain(snapshot)
	if !valid || !slices.Equal(chain, froniusObservedChainV1) {
		return rejected(SunSpecFroniusFlavorReasonChainMismatch)
	}
	return SunSpecFroniusFlavorDecision{
		matched:    true,
		reason:     SunSpecFroniusFlavorReasonMatched,
		capability: capability,
		chain:      chain,
		views:      snapshot.SourceViews(),
	}
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
