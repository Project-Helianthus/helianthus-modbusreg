package modbusreg

import (
	"context"
	"fmt"
	"math"
	"unicode/utf8"
)

// ProbeFunction is the closed read-only Modbus function set used by detection.
type ProbeFunction string

const (
	ProbeReadHoldingRegisters ProbeFunction = "fc03"
	ProbeReadInputRegisters   ProbeFunction = "fc04"
)

// ProbeIdentityField identifies one authoritative device identity value.
type ProbeIdentityField string

const (
	ProbeIdentityManufacturer ProbeIdentityField = "manufacturer"
	ProbeIdentityModel        ProbeIdentityField = "model"
	ProbeIdentityFirmware     ProbeIdentityField = "firmware"
)

// ProbeIdentityEncoding is the closed register-to-identity decoding set.
type ProbeIdentityEncoding string

const (
	// ProbeIdentityASCII decodes big-endian word bytes and trims trailing NULs.
	ProbeIdentityASCII ProbeIdentityEncoding = "ascii"
)

// DetectionLimits closes every detector-owned collection and byte dimension.
type DetectionLimits struct {
	MaxPlanDeclarations int
	MaxReads            int
	MaxWordsPerRead     int
	MaxTotalWords       int
	MaxIdentityBytes    int
	MaxEvidenceIDBytes  int
	MaxDecisionBytes    int
}

// DefaultDetectionLimits returns finite bounds for one manufacturer/model/
// firmware detection plan.
func DefaultDetectionLimits() DetectionLimits {
	return DetectionLimits{
		MaxPlanDeclarations: 3,
		MaxReads:            3,
		MaxWordsPerRead:     MaxRawWords,
		MaxTotalWords:       3 * MaxRawWords,
		MaxIdentityBytes:    256,
		MaxEvidenceIDBytes:  256,
		MaxDecisionBytes:    MaxSerializedContractBytes,
	}
}

func validateDetectionLimits(limits DetectionLimits) error {
	positive := []int{
		limits.MaxPlanDeclarations,
		limits.MaxReads,
		limits.MaxWordsPerRead,
		limits.MaxTotalWords,
		limits.MaxIdentityBytes,
		limits.MaxEvidenceIDBytes,
		limits.MaxDecisionBytes,
	}
	for _, value := range positive {
		if value <= 0 {
			return fmt.Errorf("detection limits must be finite and positive")
		}
	}
	if limits.MaxPlanDeclarations > MaxProfileDependencies ||
		limits.MaxReads > MaxProfileDependencies ||
		limits.MaxWordsPerRead > MaxRawWords ||
		limits.MaxIdentityBytes > MaxContractStringBytes ||
		limits.MaxEvidenceIDBytes > MaxContractStringBytes ||
		limits.MaxDecisionBytes > MaxSerializedContractBytes {
		return fmt.Errorf("detection limits exceed the contract maximum")
	}
	if limits.MaxPlanDeclarations > math.MaxInt/limits.MaxWordsPerRead ||
		limits.MaxTotalWords > limits.MaxPlanDeclarations*limits.MaxWordsPerRead {
		return fmt.Errorf("detection word limits are inconsistent")
	}
	return nil
}

// ProbeDeclarationSpec declares one bounded read and its identity projection.
type ProbeDeclarationSpec struct {
	ID            string
	Function      ProbeFunction
	Address       uint16
	WordCount     uint16
	IdentityField ProbeIdentityField
	Encoding      ProbeIdentityEncoding
}

// ProbePlanSpec is the complete ordered detector read declaration.
type ProbePlanSpec struct {
	Version      Version
	Declarations []ProbeDeclarationSpec
}

// ProbePlan is an immutable ordered read-only plan.
type ProbePlan struct {
	spec ProbePlanSpec
}

func cloneProbePlanSpec(spec ProbePlanSpec) ProbePlanSpec {
	spec.Declarations = append([]ProbeDeclarationSpec(nil), spec.Declarations...)
	return spec
}

func validProbeFunction(function ProbeFunction) bool {
	return function == ProbeReadHoldingRegisters ||
		function == ProbeReadInputRegisters
}

func validProbeIdentityField(field ProbeIdentityField) bool {
	switch field {
	case ProbeIdentityManufacturer, ProbeIdentityModel, ProbeIdentityFirmware:
		return true
	default:
		return false
	}
}

// NewProbePlan validates all declarations and preserves their exact order.
func NewProbePlan(spec ProbePlanSpec, limits DetectionLimits) (ProbePlan, error) {
	if err := validateDetectionLimits(limits); err != nil {
		return ProbePlan{}, err
	}
	if !spec.Version.valid() || len(spec.Declarations) == 0 ||
		len(spec.Declarations) > limits.MaxPlanDeclarations ||
		len(spec.Declarations) > limits.MaxReads {
		return ProbePlan{}, fmt.Errorf("probe plan cardinality is invalid")
	}
	if err := preflightAggregate(spec); err != nil {
		return ProbePlan{}, err
	}
	seenIDs := make(map[string]struct{}, len(spec.Declarations))
	seenFields := make(map[ProbeIdentityField]struct{}, len(spec.Declarations))
	totalWords := 0
	for _, declaration := range spec.Declarations {
		if !validIdentity(declaration.ID) ||
			!validProbeFunction(declaration.Function) ||
			!validProbeIdentityField(declaration.IdentityField) ||
			declaration.Encoding != ProbeIdentityASCII ||
			declaration.WordCount == 0 ||
			int(declaration.WordCount) > limits.MaxWordsPerRead {
			return ProbePlan{}, fmt.Errorf("probe declaration is invalid")
		}
		last := uint32(declaration.Address) + uint32(declaration.WordCount) - 1
		if last > math.MaxUint16 {
			return ProbePlan{}, fmt.Errorf("probe declaration range overflows")
		}
		if _, duplicate := seenIDs[declaration.ID]; duplicate {
			return ProbePlan{}, fmt.Errorf("probe declaration ID is duplicated")
		}
		if _, duplicate := seenFields[declaration.IdentityField]; duplicate {
			return ProbePlan{}, fmt.Errorf("probe identity producer is duplicated")
		}
		seenIDs[declaration.ID] = struct{}{}
		seenFields[declaration.IdentityField] = struct{}{}
		if totalWords > limits.MaxTotalWords-int(declaration.WordCount) {
			return ProbePlan{}, fmt.Errorf("probe plan exceeds the aggregate word bound")
		}
		totalWords += int(declaration.WordCount)
	}
	return ProbePlan{spec: cloneProbePlanSpec(spec)}, nil
}

// Spec returns an independent complete plan declaration.
func (plan ProbePlan) Spec() ProbePlanSpec {
	return cloneProbePlanSpec(plan.spec)
}

// Version returns the immutable plan contract version.
func (plan ProbePlan) Version() Version {
	return plan.spec.Version
}

// Declarations returns independent declarations in execution order.
func (plan ProbePlan) Declarations() []ProbeDeclarationSpec {
	return append([]ProbeDeclarationSpec(nil), plan.spec.Declarations...)
}

// ProbeReadRequest is the transport-neutral read request passed to a caller.
// Its unexported representation cannot carry endpoint or write authority.
type ProbeReadRequest struct {
	declarationID string
	function      ProbeFunction
	address       uint16
	wordCount     uint16
}

func probeRequest(declaration ProbeDeclarationSpec) ProbeReadRequest {
	return ProbeReadRequest{
		declarationID: declaration.ID,
		function:      declaration.Function,
		address:       declaration.Address,
		wordCount:     declaration.WordCount,
	}
}

// DeclarationID returns the immutable plan declaration identity.
func (request ProbeReadRequest) DeclarationID() string { return request.declarationID }

// Function returns FC03 or FC04.
func (request ProbeReadRequest) Function() ProbeFunction { return request.function }

// Address returns the zero-based Modbus register address.
func (request ProbeReadRequest) Address() uint16 { return request.address }

// WordCount returns the positive bounded read width.
func (request ProbeReadRequest) WordCount() uint16 { return request.wordCount }

// ProbeReader is implemented by the caller that owns transport and I/O.
type ProbeReader interface {
	ReadProbe(context.Context, ProbeReadRequest) (ProbeReadResult, error)
}

// ProbeReadStatus is the closed read disposition set.
type ProbeReadStatus string

const (
	ProbeReadSucceeded ProbeReadStatus = "succeeded"
	ProbeReadException ProbeReadStatus = "exception"
)

// ProbeReadResultSpec is one caller-returned bounded read result.
type ProbeReadResultSpec struct {
	Status        ProbeReadStatus
	Words         []uint16
	EvidenceID    string
	ExceptionCode uint8
}

// ProbeReadResult is an immutable read result with no endpoint provenance.
type ProbeReadResult struct {
	status        ProbeReadStatus
	words         []uint16
	evidenceID    string
	exceptionCode uint8
}

func validProbeEvidenceID(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && validIdentity(value)
}

// NewProbeReadResult validates against the absolute default result bounds.
// A detector may impose lower limits before consuming the result.
func NewProbeReadResult(spec ProbeReadResultSpec) (ProbeReadResult, error) {
	limits := DefaultDetectionLimits()
	if !validProbeEvidenceID(spec.EvidenceID, limits.MaxEvidenceIDBytes) {
		return ProbeReadResult{}, fmt.Errorf("probe evidence identity is invalid")
	}
	switch spec.Status {
	case ProbeReadSucceeded:
		if len(spec.Words) == 0 || len(spec.Words) > limits.MaxWordsPerRead ||
			spec.ExceptionCode != 0 {
			return ProbeReadResult{}, fmt.Errorf("successful probe result is invalid")
		}
	case ProbeReadException:
		if len(spec.Words) != 0 || spec.ExceptionCode == 0 {
			return ProbeReadResult{}, fmt.Errorf("exception probe result is invalid")
		}
	default:
		return ProbeReadResult{}, fmt.Errorf("probe result status is invalid")
	}
	if err := preflightAggregate(spec); err != nil {
		return ProbeReadResult{}, err
	}
	return ProbeReadResult{
		status:        spec.Status,
		words:         append([]uint16(nil), spec.Words...),
		evidenceID:    spec.EvidenceID,
		exceptionCode: spec.ExceptionCode,
	}, nil
}

// Status returns the immutable read disposition.
func (result ProbeReadResult) Status() ProbeReadStatus { return result.status }

// Words returns an independent register-word copy.
func (result ProbeReadResult) Words() []uint16 {
	return append([]uint16(nil), result.words...)
}

// EvidenceID returns the caller-supplied public probe evidence identity.
func (result ProbeReadResult) EvidenceID() string { return result.evidenceID }

// ExceptionCode returns zero for successful reads.
func (result ProbeReadResult) ExceptionCode() uint8 { return result.exceptionCode }

func (result ProbeReadResult) validWithin(limits DetectionLimits) bool {
	if !validProbeEvidenceID(result.evidenceID, limits.MaxEvidenceIDBytes) {
		return false
	}
	switch result.status {
	case ProbeReadSucceeded:
		return len(result.words) > 0 && len(result.words) <= limits.MaxWordsPerRead &&
			result.exceptionCode == 0
	case ProbeReadException:
		return len(result.words) == 0 && result.exceptionCode != 0
	default:
		return false
	}
}

func decodeProbeASCII(words []uint16, maximum int) (string, error) {
	if len(words) == 0 || len(words) > math.MaxInt/2 || maximum <= 0 {
		return "", fmt.Errorf("probe identity words are invalid")
	}
	encoded := make([]byte, len(words)*2)
	for index, word := range words {
		encoded[index*2] = byte(word >> 8)
		encoded[index*2+1] = byte(word)
	}
	end := len(encoded)
	for end > 0 && encoded[end-1] == 0 {
		end--
	}
	encoded = encoded[:end]
	if len(encoded) == 0 || len(encoded) > maximum || !utf8.Valid(encoded) {
		return "", fmt.Errorf("probe identity exceeds its byte bound")
	}
	for _, character := range encoded {
		if character < 0x20 || character > 0x7e {
			return "", fmt.Errorf("probe identity is not canonical ASCII")
		}
	}
	return string(encoded), nil
}
