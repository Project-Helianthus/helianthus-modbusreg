package outback

import "errors"

import "github.com/Project-Helianthus/helianthus-modbusreg/sunspec"

const (
	// AXSModelInterface is the vendor SunSpec model ID of the AXS interface
	// block; no layout for it is admitted. See
	// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-source-profile-matrix-v1.md.
	AXSModelInterface uint16 = 64110
	// AXSModelChargeControl is the vendor SunSpec model ID of the AXS charge
	// control block, which stays opaque.
	AXSModelChargeControl uint16 = 64111

	// AXSOwnerManualRevCInterfaceCandidateWords is the declared length
	// in the owner manual revision C, not an admitted decoder layout.
	AXSOwnerManualRevCInterfaceCandidateWords uint16 = 282
	// AXSApplicationNoteRev5InterfaceCandidateWords conflicts with the
	// owner manual length. Geometry alone cannot select a qualified source.
	AXSApplicationNoteRev5InterfaceCandidateWords uint16 = 420

	// AXSInterfaceModelWords retains the historical revision-C candidate.
	// Deprecated: use AXSOwnerManualRevCInterfaceCandidateWords for evidence
	// only. No OutBack interface layout is currently admitted.
	AXSInterfaceModelWords uint16 = AXSOwnerManualRevCInterfaceCandidateWords
	// AXSChargeModelWords is the historical revision-C candidate length.
	// Deprecated: evidence only; revision 5 declares 26 and 64111 remains opaque.
	AXSChargeModelWords uint16 = 23
)

// ErrOutBackAXSSourceNotQualified denotes the docs-modbus #145 NO_GO selector.
// Neither pinned source proves product/firmware applicability or an admitted,
// bounded read operation. Payload length is not qualification evidence.
var ErrOutBackAXSSourceNotQualified = errors.New("OutBack AXS NO_GO: source selection and read operation are not qualified")

// AXSReadOnlyDecoder retains the explicit caller API while source
// qualification is NO_GO. It contains no admitted definitions, discovery,
// acquisition request, transport, or send surface. Its zero value also rejects.
type AXSReadOnlyDecoder struct{}

// NewAXSReadOnlyDecoder creates an inert decoder, not a qualified profile.
func NewAXSReadOnlyDecoder() (AXSReadOnlyDecoder, error) {
	return AXSReadOnlyDecoder{}, nil
}

// Decode rejects every caller-supplied block without partial typed output.
// Raw evidence remains with the caller and is never modified. A future public
// compatibility source and bounded operation contract are needed for admission.
func (AXSReadOnlyDecoder) Decode(_ []uint16) (sunspec.DecodedModel, error) {
	return sunspec.DecodedModel{}, ErrOutBackAXSSourceNotQualified
}
