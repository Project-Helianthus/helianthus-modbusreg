package modbusreg

const OutBackAXSReadOnlyFlavorID = "sunspec.flavor.outback.axs.readonly@1.0.0"

type OutBackAXSReadOnlyFlavorReason string

const (
	OutBackAXSReadOnlyFlavorReasonInvalidChain         OutBackAXSReadOnlyFlavorReason = "INVALID_CHAIN"
	OutBackAXSReadOnlyFlavorReasonCommonNotAdmitted    OutBackAXSReadOnlyFlavorReason = "COMMON_NOT_ADMITTED"
	OutBackAXSReadOnlyFlavorReasonInterfaceAbsent      OutBackAXSReadOnlyFlavorReason = "INTERFACE_ABSENT"
	OutBackAXSReadOnlyFlavorReasonInterfaceWrongLength OutBackAXSReadOnlyFlavorReason = "INTERFACE_WRONG_LENGTH"
	OutBackAXSReadOnlyFlavorReasonInterfaceAmbiguous   OutBackAXSReadOnlyFlavorReason = "INTERFACE_AMBIGUOUS"
	OutBackAXSReadOnlyFlavorReasonMatched              OutBackAXSReadOnlyFlavorReason = "MATCHED"
)

// OutBackAXSReadOnlyFlavorDecision is an immutable result of an explicitly
// requested OutBack selection. It neither alters the standard registry nor
// authorizes acquisition, runtime activation, or control.
type OutBackAXSReadOnlyFlavorDecision struct {
	matched    bool
	reason     OutBackAXSReadOnlyFlavorReason
	chain      []SunSpecWireKey
	occurrence *SunSpecOccurrence
	model      *SunSpecDecodedModel
}

func (d OutBackAXSReadOnlyFlavorDecision) Matched() bool { return d.matched }
func (d OutBackAXSReadOnlyFlavorDecision) Reason() OutBackAXSReadOnlyFlavorReason {
	return d.reason
}
func (d OutBackAXSReadOnlyFlavorDecision) FlavorID() string { return OutBackAXSReadOnlyFlavorID }
func (d OutBackAXSReadOnlyFlavorDecision) Chain() []SunSpecWireKey {
	return append([]SunSpecWireKey(nil), d.chain...)
}
func (d OutBackAXSReadOnlyFlavorDecision) InterfaceOccurrence() (SunSpecOccurrence, bool) {
	if d.occurrence == nil {
		return SunSpecOccurrence{}, false
	}
	return cloneOccurrence(*d.occurrence), true
}
func (d OutBackAXSReadOnlyFlavorDecision) InterfaceModel() (SunSpecDecodedModel, bool) {
	if d.model == nil {
		return SunSpecDecodedModel{}, false
	}
	return cloneOutBackAXSDecodedModel(*d.model), true
}

// EvaluateSnapshot validates a terminal, source-backed V1 snapshot supplied
// by the caller. The caller chooses when to invoke it; it has no acquisition
// or profile-discovery side effect.
func (d OutBackAXSReadOnlyDecoder) EvaluateSnapshot(snapshot SunSpecChainSnapshot) OutBackAXSReadOnlyFlavorDecision {
	rejected := func(reason OutBackAXSReadOnlyFlavorReason) OutBackAXSReadOnlyFlavorDecision {
		return OutBackAXSReadOnlyFlavorDecision{reason: reason}
	}
	chain, valid := sunSpecSnapshotWireChain(snapshot)
	if !valid {
		return rejected(OutBackAXSReadOnlyFlavorReasonInvalidChain)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) == 0 {
		return rejected(OutBackAXSReadOnlyFlavorReasonInvalidChain)
	}
	standard, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV1)
	if err != nil {
		return rejected(OutBackAXSReadOnlyFlavorReasonCommonNotAdmitted)
	}
	common, err := standard.DecodeOccurrence(occurrences[0])
	if err != nil || !common.GeometryValid() || !common.Qualifies() {
		return rejected(OutBackAXSReadOnlyFlavorReasonCommonNotAdmitted)
	}

	var selected *SunSpecOccurrence
	for index := range occurrences {
		occurrence := occurrences[index]
		if occurrence.ModelID() != OutBackAXSModelInterface {
			continue
		}
		if occurrence.ModelLength() != OutBackAXSInterfaceModelWords {
			return rejected(OutBackAXSReadOnlyFlavorReasonInterfaceWrongLength)
		}
		if selected != nil {
			return rejected(OutBackAXSReadOnlyFlavorReasonInterfaceAmbiguous)
		}
		copy := cloneOccurrence(occurrence)
		selected = &copy
	}
	if selected == nil {
		return rejected(OutBackAXSReadOnlyFlavorReasonInterfaceAbsent)
	}
	model, err := d.Decode(selected.Words())
	if err != nil {
		return rejected(OutBackAXSReadOnlyFlavorReasonInterfaceWrongLength)
	}
	modelCopy := cloneOutBackAXSDecodedModel(model)
	return OutBackAXSReadOnlyFlavorDecision{
		matched:    true,
		reason:     OutBackAXSReadOnlyFlavorReasonMatched,
		chain:      append([]SunSpecWireKey(nil), chain...),
		occurrence: selected,
		model:      &modelCopy,
	}
}

func cloneOutBackAXSDecodedModel(model SunSpecDecodedModel) SunSpecDecodedModel {
	model.raw = append([]uint16(nil), model.raw...)
	model.spans = append([]SunSpecSourceSpan(nil), model.spans...)
	model.facts = model.Facts()
	return model
}
