package modbusreg

import (
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// ByteOrder declares the byte order inside each 16-bit word.
type ByteOrder string

const (
	ByteOrderModbus  ByteOrder = "modbus"
	ByteOrderSwapped ByteOrder = "swapped"
)

// Representation declares the raw numeric or string representation.
type Representation string

const (
	RepresentationUnsignedInteger Representation = "unsigned_integer"
	RepresentationSignedInteger   Representation = "signed_integer"
	RepresentationIEEE754         Representation = "ieee754"
	RepresentationString          Representation = "string"
)

// ScaleSource identifies where scaling comes from.
type ScaleSource string

const (
	ScaleNotApplicable ScaleSource = "not_applicable"
	ScaleConstant      ScaleSource = "constant"
	ScaleDependency    ScaleSource = "dependency"
)

// ScaleApplicationOrder prevents implicit pre/post representation scaling.
type ScaleApplicationOrder string

const (
	ScaleOrderNotApplicable   ScaleApplicationOrder = "not_applicable"
	ScaleBeforeRepresentation ScaleApplicationOrder = "before_representation"
	ScaleAfterRepresentation  ScaleApplicationOrder = "after_representation"
)

// ScaleSpec is the complete immutable scaling declaration.
type ScaleSpec struct {
	Source           ScaleSource
	ApplicationOrder ScaleApplicationOrder
	Numerator        int64
	Denominator      int64
	DependencyID     string
}

// SentinelKind classifies an exact raw sentinel without coercion.
type SentinelKind string

const (
	SentinelInvalid        SentinelKind = "invalid"
	SentinelNotImplemented SentinelKind = "not_implemented"
	SentinelReserved       SentinelKind = "reserved"
)

// RawSentinel is an exact wire-order raw value and disposition.
type RawSentinel struct {
	Kind  SentinelKind
	Words []uint16
}

// StringApplicability makes the string dimension explicit for every codec.
type StringApplicability string

const (
	StringNotApplicable StringApplicability = "not_applicable"
	StringApplicable    StringApplicability = "applicable"
)

// StringWordPacking declares byte placement across words.
type StringWordPacking string

const (
	StringHighByteFirst StringWordPacking = "high_byte_first"
	StringLowByteFirst  StringWordPacking = "low_byte_first"
)

// StringTermination declares how a documentary string ends.
type StringTermination string

const (
	StringFixedLength   StringTermination = "fixed_length"
	StringNULTerminated StringTermination = "nul_terminated"
)

// StringSpec declares every string-specific codec dimension.
type StringSpec struct {
	Applicability                  StringApplicability
	WordPacking                    StringWordPacking
	ByteOrder                      ByteOrder
	PaddingByte                    *byte
	Termination                    StringTermination
	RetainedRawLength              uint32
	DocumentaryCharacterRepertoire string
}

// ValidityBehavior declares the non-coercing disposition of invalid raw data.
type ValidityBehavior string

const (
	ValidityRejectSentinel ValidityBehavior = "reject_sentinel"
	ValidityPreserveRaw    ValidityBehavior = "preserve_raw_invalid"
)

// CodecSpec is the input form for one complete immutable codec declaration.
type CodecSpec struct {
	ID                 string
	Version            Version
	RawWordCount       uint16
	WordPermutation    []uint16
	IntraWordByteOrder ByteOrder
	Representation     Representation
	Scale              ScaleSpec
	Sentinels          []RawSentinel
	String             StringSpec
	OutputProfileType  string
	ValidityBehavior   ValidityBehavior
}

// Codec is an immutable validated codec declaration. Decoding implementations
// may consume it, but M2-01 does not reinterpret or canonicalize values.
type Codec struct {
	spec CodecSpec
}

// NewCodec validates every dimension and retains independent copies.
func NewCodec(spec CodecSpec) (Codec, error) {
	if err := preflightCodecSpec(spec); err != nil {
		return Codec{}, err
	}
	spec = cloneCodecSpec(spec)
	if !validIdentity(spec.ID) || !spec.Version.valid() ||
		spec.RawWordCount == 0 ||
		spec.RawWordCount > modbus.MaxReadRegisters ||
		len(spec.WordPermutation) != int(spec.RawWordCount) ||
		spec.OutputProfileType == "" {
		return Codec{}, fmt.Errorf("incomplete codec identity or dimensions")
	}
	seenPermutation := make([]bool, spec.RawWordCount)
	for _, position := range spec.WordPermutation {
		if position >= spec.RawWordCount || seenPermutation[position] {
			return Codec{}, fmt.Errorf("word permutation is not exact")
		}
		seenPermutation[position] = true
	}
	if spec.IntraWordByteOrder != ByteOrderModbus &&
		spec.IntraWordByteOrder != ByteOrderSwapped {
		return Codec{}, fmt.Errorf("byte order is not declared")
	}
	switch spec.Representation {
	case RepresentationUnsignedInteger, RepresentationSignedInteger:
		if err := validateScale(spec.Scale); err != nil {
			return Codec{}, err
		}
		if spec.String.Applicability != StringNotApplicable ||
			!emptyStringDimensions(spec.String) {
			return Codec{}, fmt.Errorf("numeric codec has string dimensions")
		}
	case RepresentationIEEE754:
		if spec.RawWordCount != 2 && spec.RawWordCount != 4 {
			return Codec{}, fmt.Errorf("IEEE754 width is unsupported")
		}
		if err := validateScale(spec.Scale); err != nil {
			return Codec{}, err
		}
		if spec.String.Applicability != StringNotApplicable ||
			!emptyStringDimensions(spec.String) {
			return Codec{}, fmt.Errorf("floating codec has string dimensions")
		}
	case RepresentationString:
		if spec.Scale.Source != ScaleNotApplicable ||
			spec.Scale.ApplicationOrder != ScaleOrderNotApplicable ||
			spec.Scale.Numerator != 0 || spec.Scale.Denominator != 0 ||
			spec.Scale.DependencyID != "" {
			return Codec{}, fmt.Errorf("string codec has numeric scale")
		}
		if err := validateStringSpec(spec.String, spec.RawWordCount); err != nil {
			return Codec{}, err
		}
	default:
		return Codec{}, fmt.Errorf("representation is not declared")
	}
	if spec.ValidityBehavior != ValidityRejectSentinel &&
		spec.ValidityBehavior != ValidityPreserveRaw {
		return Codec{}, fmt.Errorf("validity behavior is not declared")
	}
	seenSentinels := make(map[string]struct{}, len(spec.Sentinels))
	for _, sentinel := range spec.Sentinels {
		if sentinel.Kind != SentinelInvalid &&
			sentinel.Kind != SentinelNotImplemented &&
			sentinel.Kind != SentinelReserved {
			return Codec{}, fmt.Errorf("unknown sentinel kind")
		}
		if len(sentinel.Words) != int(spec.RawWordCount) {
			return Codec{}, fmt.Errorf("sentinel width differs from codec")
		}
		key := fmt.Sprint(sentinel.Words)
		if _, exists := seenSentinels[key]; exists {
			return Codec{}, fmt.Errorf("raw sentinel has conflicting dispositions")
		}
		seenSentinels[key] = struct{}{}
	}
	return Codec{spec: spec}, nil
}

func preflightCodecSpec(spec CodecSpec) error {
	if spec.RawWordCount == 0 || spec.RawWordCount > modbus.MaxReadRegisters {
		return fmt.Errorf("codec raw-word width exceeds the runtime boundary")
	}
	if err := validateBoundedString("codec ID", spec.ID, true); err != nil {
		return err
	}
	if err := validateBoundedString(
		"codec output profile type",
		spec.OutputProfileType,
		true,
	); err != nil {
		return err
	}
	if err := validateBoundedString(
		"codec scale dependency",
		spec.Scale.DependencyID,
		false,
	); err != nil {
		return err
	}
	if err := validateBoundedString(
		"codec character repertoire",
		spec.String.DocumentaryCharacterRepertoire,
		false,
	); err != nil {
		return err
	}
	if len(spec.WordPermutation) > MaxRawWords ||
		len(spec.Sentinels) > MaxCodecSentinels {
		return fmt.Errorf("codec input exceeds the contract collection boundary")
	}
	for _, sentinel := range spec.Sentinels {
		if len(sentinel.Words) > MaxRawWords {
			return fmt.Errorf("codec sentinel exceeds the raw-word boundary")
		}
	}
	return nil
}

func validateScale(scale ScaleSpec) error {
	switch scale.Source {
	case ScaleConstant:
		if scale.ApplicationOrder != ScaleBeforeRepresentation &&
			scale.ApplicationOrder != ScaleAfterRepresentation {
			return fmt.Errorf("constant scale order is not declared")
		}
		if scale.Denominator == 0 || scale.DependencyID != "" {
			return fmt.Errorf("constant scale is incomplete")
		}
	case ScaleDependency:
		if scale.ApplicationOrder != ScaleBeforeRepresentation &&
			scale.ApplicationOrder != ScaleAfterRepresentation {
			return fmt.Errorf("dependency scale order is not declared")
		}
		if !validIdentity(scale.DependencyID) ||
			scale.Numerator != 0 || scale.Denominator != 0 {
			return fmt.Errorf("dependency scale is incomplete")
		}
	case ScaleNotApplicable:
		if scale.ApplicationOrder != ScaleOrderNotApplicable ||
			scale.Numerator != 0 || scale.Denominator != 0 ||
			scale.DependencyID != "" {
			return fmt.Errorf("inapplicable scale carries values")
		}
	default:
		return fmt.Errorf("scale source is not declared")
	}
	return nil
}

func emptyStringDimensions(spec StringSpec) bool {
	return spec.WordPacking == "" && spec.ByteOrder == "" &&
		spec.PaddingByte == nil && spec.Termination == "" &&
		spec.RetainedRawLength == 0 &&
		spec.DocumentaryCharacterRepertoire == ""
}

func validateStringSpec(spec StringSpec, words uint16) error {
	if spec.Applicability != StringApplicable ||
		(spec.WordPacking != StringHighByteFirst &&
			spec.WordPacking != StringLowByteFirst) ||
		(spec.ByteOrder != ByteOrderModbus &&
			spec.ByteOrder != ByteOrderSwapped) ||
		(spec.Termination != StringFixedLength &&
			spec.Termination != StringNULTerminated) ||
		spec.PaddingByte == nil ||
		spec.RetainedRawLength != uint32(words)*2 ||
		spec.DocumentaryCharacterRepertoire == "" {
		return fmt.Errorf("string dimensions are incomplete")
	}
	return nil
}

func cloneCodecSpec(spec CodecSpec) CodecSpec {
	spec.WordPermutation = append([]uint16(nil), spec.WordPermutation...)
	if spec.String.PaddingByte != nil {
		padding := *spec.String.PaddingByte
		spec.String.PaddingByte = &padding
	}
	spec.Sentinels = append([]RawSentinel(nil), spec.Sentinels...)
	for index := range spec.Sentinels {
		spec.Sentinels[index].Words = append(
			[]uint16(nil),
			spec.Sentinels[index].Words...,
		)
	}
	return spec
}

// Spec returns an independent complete declaration.
func (codec Codec) Spec() CodecSpec {
	return cloneCodecSpec(codec.spec)
}

// ID returns the stable codec identifier.
func (codec Codec) ID() string {
	return codec.spec.ID
}

// Version returns the immutable codec version.
func (codec Codec) Version() Version {
	return codec.spec.Version
}

// RawWordCount returns the exact input width.
func (codec Codec) RawWordCount() uint16 {
	return codec.spec.RawWordCount
}

// WordPermutation returns an independent permutation copy.
func (codec Codec) WordPermutation() []uint16 {
	return append([]uint16(nil), codec.spec.WordPermutation...)
}

// Sentinels returns independent exact raw sentinels.
func (codec Codec) Sentinels() []RawSentinel {
	return cloneCodecSpec(codec.spec).Sentinels
}
