package modbusreg

import "errors"

const (
	OutBackAXSModelInterface     uint16 = 64110
	OutBackAXSModelChargeControl uint16 = 64111

	// OutBackAXSOwnerManualRevCInterfaceCandidateWords is the declared length
	// in the owner manual revision C, not an admitted decoder layout.
	OutBackAXSOwnerManualRevCInterfaceCandidateWords uint16 = 282
	// OutBackAXSApplicationNoteRev5InterfaceCandidateWords conflicts with the
	// owner manual length. Geometry alone cannot select a qualified source.
	OutBackAXSApplicationNoteRev5InterfaceCandidateWords uint16 = 420

	// OutBackAXSInterfaceModelWords retains the historical revision-C candidate.
	// Deprecated: use OutBackAXSOwnerManualRevCInterfaceCandidateWords for evidence
	// only. No OutBack interface layout is currently admitted.
	OutBackAXSInterfaceModelWords uint16 = OutBackAXSOwnerManualRevCInterfaceCandidateWords
	// OutBackAXSChargeModelWords is the historical revision-C candidate length.
	// Deprecated: evidence only; revision 5 declares 26 and 64111 remains opaque.
	OutBackAXSChargeModelWords uint16 = 23
)

// ErrOutBackAXSSourceNotQualified denotes the docs-modbus #145 NO_GO selector.
// Neither pinned source proves product/firmware applicability or an admitted,
// bounded read operation. Payload length is not qualification evidence.
var ErrOutBackAXSSourceNotQualified = errors.New("OutBack AXS NO_GO: source selection and read operation are not qualified")

// OutBackAXSReadOnlyDecoder retains the explicit caller API while source
// qualification is NO_GO. It contains no admitted definitions, discovery,
// acquisition request, transport, or send surface. Its zero value also rejects.
type OutBackAXSReadOnlyDecoder struct{}

// NewOutBackAXSReadOnlyDecoder creates an inert decoder, not a qualified profile.
func NewOutBackAXSReadOnlyDecoder() (OutBackAXSReadOnlyDecoder, error) {
	return OutBackAXSReadOnlyDecoder{}, nil
}

// Decode rejects every caller-supplied block without partial typed output.
// Raw evidence remains with the caller and is never modified. A future public
// compatibility source and bounded operation contract are needed for admission.
func (OutBackAXSReadOnlyDecoder) Decode(_ []uint16) (SunSpecDecodedModel, error) {
	return SunSpecDecodedModel{}, ErrOutBackAXSSourceNotQualified
}
