package sunspec

import (
	"slices"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// Model 124 points, by decoded field ID.
const (
	fieldWChaMax      = "storage.control.max_charge_power"
	fieldStorCtlMod   = "storage.control.mode"
	fieldOutWRte      = "storage.control.discharge_rate"
	fieldInWRte       = "storage.control.charge_rate"
	fieldInOutRvrtTms = "storage.control.rate_revert"
)

// StorCtl_Mod bit 0 is CHARGE and bit 1 is DiSCHARGE (SunSpec model 124).
const (
	storageChargeBit    uint16 = 1 << 0
	storageDischargeBit uint16 = 1 << 1
)

// StorageDirection selects the charge or the discharge limit of model 124.
type StorageDirection uint8

// The two directions of model 124.
const (
	StorageCharge StorageDirection = iota + 1
	StorageDischarge
)

func (d StorageDirection) bit() (uint16, bool) {
	switch d {
	case StorageCharge:
		return storageChargeBit, true
	case StorageDischarge:
		return storageDischargeBit, true
	}
	return 0, false
}

// StorageLimitRequest asks for a limit of Watts in one direction on the model
// 124 instance at Ordinal. The charge bound is WChaMax. The discharge bound is
// MaxDischarge where the device reports one, else WChaMax. A nil Duration
// writes no InOutWRte_RvrtTms.
type StorageLimitRequest struct {
	Ordinal      uint32
	Direction    StorageDirection
	Watts        Decimal
	MaxDischarge *Decimal
	Duration     *uint16
}

// StorageModeWrite is the final StorCtl_Mod write. Its words depend on the
// mode the owner reads inside its sequence, so they are built by Request and
// only the requested direction's bit changes.
type StorageModeWrite struct {
	point   PointWrite
	bit     uint16
	release bool
}

// StorageLimitWrite is the ordered plan of one limit: the rate and the
// optional revert time, each its own request, then the mode write last.
type StorageLimitWrite struct {
	Steps []ControlWriteStep
	Mode  StorageModeWrite
}

// EncodeStorageLimit builds the rate request (InWRte or OutWRte, percent of
// the bound rounded down in InOutWRte_SF), the optional InOutWRte_RvrtTms
// request and the mode write that sets the direction's bit. It never builds a
// request for the opposite rate, a status word or a span across the gap.
func EncodeStorageLimit(r DecoderRegistry, snapshot ChainSnapshot, q StorageLimitRequest) (StorageLimitWrite, error) {
	bit, ok := q.Direction.bit()
	if !ok {
		return StorageLimitWrite{}, controlRefusal(ControlReasonUnencodable, "unknown storage direction")
	}
	occurrence, definitions, err := controlsOccurrence(r, snapshot, q.Ordinal, 124)
	if err != nil {
		return StorageLimitWrite{}, err
	}
	bound, ok := fixedDecimal(occurrence, definitions, fieldWChaMax)
	if q.Direction == StorageDischarge && q.MaxDischarge != nil {
		bound, ok = *q.MaxDischarge, q.MaxDischarge.Coefficient > 0 && q.MaxDischarge.Exponent >= -10 && q.MaxDischarge.Exponent <= 10
	}
	if !ok {
		return StorageLimitWrite{}, controlRefusal(ControlReasonBoundAbsent, "model 124 has no bound for this direction")
	}
	writes, err := PointWrites(r, snapshot, q.Ordinal)
	if err != nil {
		return StorageLimitWrite{}, controlRefusal(ControlReasonOccurrenceInvalid, "%v", err)
	}
	field := fieldInWRte
	if q.Direction == StorageDischarge {
		field = fieldOutWRte
	}
	rate, ok := writeByField(writes, field)
	if !ok || rate.scale == nil {
		return StorageLimitWrite{}, controlRefusal(ControlReasonPointUnavailable, "model 124 offers no write of %s", field)
	}
	mode, ok := writeByField(writes, fieldStorCtlMod)
	if !ok {
		return StorageLimitWrite{}, controlRefusal(ControlReasonPointUnavailable, "model 124 offers no write of %s", fieldStorCtlMod)
	}
	percent, err := percentOfBound(q.Watts, bound, int16(rate.scale.signed))
	if err != nil {
		return StorageLimitWrite{}, err
	}
	steps := make([]ControlWriteStep, 0, 2)
	rateStep, err := singlePointStep(rate, percent)
	if err != nil {
		return StorageLimitWrite{}, err
	}
	steps = append(steps, rateStep)
	if q.Duration != nil {
		revert, ok := writeByField(writes, fieldInOutRvrtTms)
		if !ok {
			return StorageLimitWrite{}, controlRefusal(ControlReasonPointUnavailable, "model 124 offers no write of %s", fieldInOutRvrtTms)
		}
		revertStep, err := singlePointStep(revert, *q.Duration)
		if err != nil {
			return StorageLimitWrite{}, err
		}
		steps = append(steps, revertStep)
	}
	return StorageLimitWrite{Steps: steps, Mode: StorageModeWrite{point: mode, bit: bit}}, nil
}

// EncodeStorageRelease builds the mode write that clears the direction's bit
// and nothing else.
func EncodeStorageRelease(r DecoderRegistry, snapshot ChainSnapshot, ordinal uint32, direction StorageDirection) (StorageModeWrite, error) {
	bit, ok := direction.bit()
	if !ok {
		return StorageModeWrite{}, controlRefusal(ControlReasonUnencodable, "unknown storage direction")
	}
	if _, _, err := controlsOccurrence(r, snapshot, ordinal, 124); err != nil {
		return StorageModeWrite{}, err
	}
	writes, err := PointWrites(r, snapshot, ordinal)
	if err != nil {
		return StorageModeWrite{}, controlRefusal(ControlReasonOccurrenceInvalid, "%v", err)
	}
	mode, ok := writeByField(writes, fieldStorCtlMod)
	if !ok {
		return StorageModeWrite{}, controlRefusal(ControlReasonPointUnavailable, "model 124 offers no write of %s", fieldStorCtlMod)
	}
	return StorageModeWrite{point: mode, bit: bit, release: true}, nil
}

// Span is the request span of the mode write without words, for admission.
func (m StorageModeWrite) Span() PointWriteRequest {
	return PointWriteRequest{Function: 16, Address: m.point.address, WordCount: m.point.point.size}
}

// Admission admits exactly the StorCtl_Mod word.
func (m StorageModeWrite) Admission() modbus.RegisterWriteAdmission { return m.Span().Admission() }

// Request builds the mode write from the mode read inside the owner's
// sequence. Only the direction's bit changes; every other bit, known or not,
// stays as read. A current mode that is not an established value is refused.
func (m StorageModeWrite) Request(current uint16) (ControlWriteStep, error) {
	if m.bit == 0 || m.point.point.size != 1 {
		return ControlWriteStep{}, controlRefusal(ControlReasonUnencodable, "the mode write is the zero value")
	}
	if decodeSunSpecValue(m.point.point, []uint16{current}, nil).State() != ValueValid {
		return ControlWriteStep{}, controlRefusal(ControlReasonUnencodable, "the mode read is not an established value")
	}
	next := current | m.bit
	if m.release {
		next = current &^ m.bit
	}
	return singlePointStep(m.point, next)
}

// CompareReadback compares one re-read per planned step, in send order and the
// mode write last, with what each step wrote. A step without a re-read, or a
// re-read beyond the plan, leaves the command unknown, so the echo or re-read
// of the first step alone never confirms the command.
func (p StorageLimitWrite) CompareReadback(mode ControlWriteStep, reads ...[]uint16) ReadbackState {
	steps := append(slices.Clone(p.Steps), mode)
	if len(reads) != len(steps) {
		return ReadbackUnknown
	}
	states := make([]ReadbackState, len(steps))
	for i, step := range steps {
		states[i] = step.CompareReadback(reads[i])
	}
	return CombineReadback(states...)
}

func singlePointStep(point PointWrite, native any) (ControlWriteStep, error) {
	request, err := point.Encode(native)
	if err != nil {
		return ControlWriteStep{}, controlRefusal(ControlReasonUnencodable, "%v", err)
	}
	return ControlWriteStep{
		Fields:  []string{point.point.fieldID},
		Request: request,
		points:  []controlStepPoint{{def: point.point, scale: point.scale}},
	}, nil
}
