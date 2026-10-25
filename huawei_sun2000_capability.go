package modbusreg

import (
	"bytes"
	"context"
	"errors"
	"strconv"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// HuaweiSUN2000CapabilityOutcome is the outcome of SUN2000 monitoring profile
// selection.
type HuaweiSUN2000CapabilityOutcome string

const (
	HuaweiSUN2000ProfileSelected           HuaweiSUN2000CapabilityOutcome = "PROFILE_SELECTED"
	HuaweiSUN2000NoAdmissibleProfile       HuaweiSUN2000CapabilityOutcome = "NO_ADMISSIBLE_PROFILE"
	HuaweiSUN2000InsufficientEvidence      HuaweiSUN2000CapabilityOutcome = "INSUFFICIENT_EVIDENCE"
	HuaweiSUN2000AmbiguousMultipleProfiles HuaweiSUN2000CapabilityOutcome = "AMBIGUOUS"
)

const huaweiSUN2000ModelBlockBytes = 2 * huaweiSUN2000ModelQuantity

// HuaweiSUN2000CapabilityReason names why one profile was or was not selected.
type HuaweiSUN2000CapabilityReason string

const (
	HuaweiSUN2000ReasonNoGatewayIdentity        HuaweiSUN2000CapabilityReason = "NO_GATEWAY_IDENTITY"
	HuaweiSUN2000ReasonGatewayIdentityRejected  HuaweiSUN2000CapabilityReason = "GATEWAY_IDENTITY_NOT_MATCHED"
	HuaweiSUN2000ReasonUnitNotRouted            HuaweiSUN2000CapabilityReason = "UNIT_NOT_ROUTED"
	HuaweiSUN2000ReasonModelBlockInvalid        HuaweiSUN2000CapabilityReason = "MODEL_BLOCK_INVALID"
	HuaweiSUN2000ReasonListingModelConflict     HuaweiSUN2000CapabilityReason = "LISTING_MODEL_CONFLICT"
	HuaweiSUN2000ReasonModelNotAdmitted         HuaweiSUN2000CapabilityReason = "MODEL_NOT_ADMITTED"
	HuaweiSUN2000ReasonDocumentNotAdmitted      HuaweiSUN2000CapabilityReason = "DOCUMENT_NOT_ADMITTED"
	HuaweiSUN2000ReasonMatched                  HuaweiSUN2000CapabilityReason = "MATCHED"
	HuaweiSUN2000ReasonMultipleProfilesSelected HuaweiSUN2000CapabilityReason = "MULTIPLE_PROFILES_SELECTED"
)

// HuaweiSUN2000CapabilityInput is detached, already-read evidence for one
// endpoint and one unit. It carries no endpoint, session or transport state.
type HuaweiSUN2000CapabilityInput struct {
	// SmartLogger is the observed SmartLogger gateway evidence, if any.
	SmartLogger *HuaweiSmartLoggerObservedQualificationInput
	// SDongle is the observed S-Dongle gateway evidence, if any. Its child
	// reads carry the routed model read of the child unit.
	SDongle *HuaweiSDongleObservedQualificationInput
	// UnitID is the routed unit the profile would read.
	UnitID uint8
	// RoutedModelBlock is the raw extent of the FC03 offset 30000 quantity 15
	// read at UnitID behind the SmartLogger.
	RoutedModelBlock []byte
	// Document is the consulted document edition that the caller pins.
	Document HuaweiVendorDocumentIdentity
}

// HuaweiSUN2000ProfileEvaluation is the result of one profile against the input.
type HuaweiSUN2000ProfileEvaluation struct {
	profileID string
	outcome   HuaweiSUN2000CapabilityOutcome
	reason    HuaweiSUN2000CapabilityReason
}

func (e HuaweiSUN2000ProfileEvaluation) ProfileID() string { return e.profileID }
func (e HuaweiSUN2000ProfileEvaluation) Outcome() HuaweiSUN2000CapabilityOutcome {
	return e.outcome
}
func (e HuaweiSUN2000ProfileEvaluation) Reason() HuaweiSUN2000CapabilityReason { return e.reason }

// HuaweiSUN2000CapabilitySelection is the result of capability detection. A
// profile is selected only when exactly one exact pair matches; order,
// priority and vendor name never choose a winner.
type HuaweiSUN2000CapabilitySelection struct {
	outcome     HuaweiSUN2000CapabilityOutcome
	reason      HuaweiSUN2000CapabilityReason
	unitID      uint8
	profile     *HuaweiSUN2000MonitoringProfile
	evaluations []HuaweiSUN2000ProfileEvaluation
}

func (s HuaweiSUN2000CapabilitySelection) Outcome() HuaweiSUN2000CapabilityOutcome {
	return s.outcome
}
func (s HuaweiSUN2000CapabilitySelection) Reason() HuaweiSUN2000CapabilityReason { return s.reason }
func (s HuaweiSUN2000CapabilitySelection) UnitID() uint8                         { return s.unitID }
func (s HuaweiSUN2000CapabilitySelection) Profile() (HuaweiSUN2000MonitoringProfile, bool) {
	if s.profile == nil {
		return HuaweiSUN2000MonitoringProfile{}, false
	}
	return *s.profile, true
}
func (s HuaweiSUN2000CapabilitySelection) Evaluations() []HuaweiSUN2000ProfileEvaluation {
	return append([]HuaweiSUN2000ProfileEvaluation(nil), s.evaluations...)
}

// Plan is the bounded read plan of the selected profile, or nothing.
func (s HuaweiSUN2000CapabilitySelection) Plan() []HuaweiSUN2000ReadStep {
	if s.profile == nil {
		return nil
	}
	return s.profile.Plan(s.unitID)
}

// BatteryProfileSelected is false: no LUNA2000 profile is admitted, whether
// or not the parent inverter is identified.
func (HuaweiSUN2000CapabilitySelection) BatteryProfileSelected() bool    { return false }
func (HuaweiSUN2000CapabilitySelection) CatalogRegistered() bool         { return false }
func (HuaweiSUN2000CapabilitySelection) AutomaticRuntimeAdmission() bool { return false }
func (HuaweiSUN2000CapabilitySelection) LiveQualified() bool             { return false }
func (HuaweiSUN2000CapabilitySelection) SupportClaim() bool              { return false }
func (HuaweiSUN2000CapabilitySelection) WriteAuthority() bool            { return false }
func (HuaweiSUN2000CapabilitySelection) AutomaticRequestCount() int      { return 0 }

// SelectHuaweiSUN2000MonitoringProfile evaluates every exact profile against
// the input. Identity comes from the observed gateway card, the routed unit and
// the routed model string; the document gate requires the exact consulted
// edition of the profile. A missing piece of evidence is insufficient
// evidence; identity established at a routed unit that no exact pair admits is
// no admissible profile.
func SelectHuaweiSUN2000MonitoringProfile(input HuaweiSUN2000CapabilityInput) HuaweiSUN2000CapabilitySelection {
	profiles := HuaweiSUN2000MonitoringProfiles()
	selection := HuaweiSUN2000CapabilitySelection{
		outcome:     HuaweiSUN2000InsufficientEvidence,
		reason:      HuaweiSUN2000ReasonNoGatewayIdentity,
		unitID:      input.UnitID,
		evaluations: make([]HuaweiSUN2000ProfileEvaluation, 0, len(profiles)),
	}
	var matched []int
	noAdmissible := -1
	firstInsufficient := -1
	for index, profile := range profiles {
		evaluation := evaluateHuaweiSUN2000Profile(profile, input)
		selection.evaluations = append(selection.evaluations, evaluation)
		switch evaluation.outcome {
		case HuaweiSUN2000ProfileSelected:
			matched = append(matched, index)
		case HuaweiSUN2000NoAdmissibleProfile:
			if noAdmissible < 0 {
				noAdmissible = index
			}
		default:
			if firstInsufficient < 0 && evaluation.reason != HuaweiSUN2000ReasonNoGatewayIdentity {
				firstInsufficient = index
			}
		}
	}
	switch {
	case len(matched) > 1:
		selection.outcome, selection.reason = HuaweiSUN2000AmbiguousMultipleProfiles, HuaweiSUN2000ReasonMultipleProfilesSelected
	case len(matched) == 1:
		profile := profiles[matched[0]]
		selection.outcome, selection.reason, selection.profile = HuaweiSUN2000ProfileSelected, HuaweiSUN2000ReasonMatched, &profile
	case noAdmissible >= 0:
		selection.outcome, selection.reason = HuaweiSUN2000NoAdmissibleProfile, selection.evaluations[noAdmissible].reason
	case firstInsufficient >= 0:
		selection.reason = selection.evaluations[firstInsufficient].reason
	}
	return selection
}

func evaluateHuaweiSUN2000Profile(profile HuaweiSUN2000MonitoringProfile, input HuaweiSUN2000CapabilityInput) HuaweiSUN2000ProfileEvaluation {
	result := func(outcome HuaweiSUN2000CapabilityOutcome, reason HuaweiSUN2000CapabilityReason) HuaweiSUN2000ProfileEvaluation {
		return HuaweiSUN2000ProfileEvaluation{profileID: profile.id, outcome: outcome, reason: reason}
	}
	insufficient := func(reason HuaweiSUN2000CapabilityReason) HuaweiSUN2000ProfileEvaluation {
		return result(HuaweiSUN2000InsufficientEvidence, reason)
	}
	var model string
	switch profile.identityCard {
	case HuaweiSmartLoggerObservedQualificationCardID:
		if input.SmartLogger == nil {
			return insufficient(HuaweiSUN2000ReasonNoGatewayIdentity)
		}
		gateway := NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(*input.SmartLogger)
		inventory, ok := gateway.Inventory()
		if !gateway.IdentityMatched() || !gateway.QualificationTestReady() || !ok {
			return insufficient(HuaweiSUN2000ReasonGatewayIdentityRejected)
		}
		listed, found := huaweiSUN2000ListedChild(inventory, input.UnitID)
		if !found {
			return insufficient(HuaweiSUN2000ReasonUnitNotRouted)
		}
		text, valid := parseHuaweiSUN2000RoutedModelBlock(input.RoutedModelBlock)
		if !valid {
			return insufficient(HuaweiSUN2000ReasonModelBlockInvalid)
		}
		if listed != "" && listed != text {
			return insufficient(HuaweiSUN2000ReasonListingModelConflict)
		}
		model = text
	case HuaweiSDongleObservedQualificationCardID:
		if input.SDongle == nil {
			return insufficient(HuaweiSUN2000ReasonNoGatewayIdentity)
		}
		card := NewHuaweiSDongleObservedQualificationCard()
		gateway := card.Evaluate(*input.SDongle)
		if !gateway.IdentityMatched() || !gateway.QualificationTestReady() {
			return insufficient(HuaweiSUN2000ReasonGatewayIdentityRejected)
		}
		if input.UnitID != card.ChildUnit() || input.SDongle.Child.UnitID != card.ChildUnit() {
			return insufficient(HuaweiSUN2000ReasonUnitNotRouted)
		}
		model = card.ChildModel()
	default:
		return insufficient(HuaweiSUN2000ReasonNoGatewayIdentity)
	}
	if model != profile.model {
		return result(HuaweiSUN2000NoAdmissibleProfile, HuaweiSUN2000ReasonModelNotAdmitted)
	}
	if input.Document != profile.document {
		return result(HuaweiSUN2000NoAdmissibleProfile, HuaweiSUN2000ReasonDocumentNotAdmitted)
	}
	return result(HuaweiSUN2000ProfileSelected, HuaweiSUN2000ReasonMatched)
}

// huaweiSUN2000ListedChild finds the child that the validated listing names at
// the unit. The listing routes; it never supplies identity.
func huaweiSUN2000ListedChild(inventory HuaweiGatewayInventory, unit uint8) (string, bool) {
	if unit == 0 || unit > huaweiSUN2000MaxUnit {
		return "", false
	}
	address := strconv.Itoa(int(unit))
	for _, child := range inventory.Children() {
		if child.Address() == address {
			return child.Attribute("model"), true
		}
	}
	return "", false
}

// parseHuaweiSUN2000RoutedModelBlock decodes the routed model read behind the
// SmartLogger: printable text ended by a NUL, then NUL padding only, as
// observed for the M3 series units.
func parseHuaweiSUN2000RoutedModelBlock(block []byte) (string, bool) {
	if len(block) != huaweiSUN2000ModelBlockBytes {
		return "", false
	}
	end := bytes.IndexByte(block, 0)
	if end <= 0 || !isHuaweiSmartLoggerInventoryASCII(block[:end]) {
		return "", false
	}
	for _, padding := range block[end:] {
		if padding != 0 {
			return "", false
		}
	}
	return string(block[:end]), true
}

// HuaweiSUN2000MonitoringSession is an already configured read boundary. It
// exposes one holding-register read and nothing else.
type HuaweiSUN2000MonitoringSession interface {
	ReadHolding(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// ErrHuaweiSUN2000MonitoringObserverInvalid reports an observer that cannot run.
var ErrHuaweiSUN2000MonitoringObserverInvalid = errors.New("huawei SUN2000 monitoring observer is invalid")

// HuaweiSUN2000MonitoringObserver reads the plan of one selected profile at
// one unit and decodes it. It owns neither discovery, retries, pacing nor
// publication.
type HuaweiSUN2000MonitoringObserver struct {
	profile HuaweiSUN2000MonitoringProfile
	unitID  uint8
	session HuaweiSUN2000MonitoringSession
}

// NewHuaweiSUN2000MonitoringObserver binds a selected profile to a session.
// Any selection other than one selected profile is refused.
func NewHuaweiSUN2000MonitoringObserver(selection HuaweiSUN2000CapabilitySelection, session HuaweiSUN2000MonitoringSession) (*HuaweiSUN2000MonitoringObserver, error) {
	profile, ok := selection.Profile()
	if session == nil || !ok || selection.Outcome() != HuaweiSUN2000ProfileSelected || selection.UnitID() == 0 {
		return nil, ErrHuaweiSUN2000MonitoringObserverInvalid
	}
	return &HuaweiSUN2000MonitoringObserver{profile: profile, unitID: selection.UnitID(), session: session}, nil
}

// Observe reads each group of the plan once, in order. An exception reply is
// recorded with its code and the next group is read. A timeout, a transport
// error or a reply that does not match the request fails the group and ends
// the unit's cycle: the remaining groups are not read. A cancelled context
// returns the groups already read together with the context error.
func (o *HuaweiSUN2000MonitoringObserver) Observe(ctx context.Context, cycle uint64) (HuaweiSUN2000MonitoringReading, error) {
	if o == nil || o.session == nil || ctx == nil {
		return HuaweiSUN2000MonitoringReading{}, ErrHuaweiSUN2000MonitoringObserverInvalid
	}
	plan := o.profile.Plan(o.unitID)
	readings := make([]HuaweiSUN2000GroupReading, 0, len(plan))
	for _, step := range plan {
		if err := ctx.Err(); err != nil {
			partial, decodeErr := o.profile.Decode(o.unitID, readings)
			if decodeErr != nil {
				return HuaweiSUN2000MonitoringReading{}, err
			}
			return partial, err
		}
		reading := HuaweiSUN2000GroupReading{Offset: step.Offset, Quantity: step.Quantity, Cycle: cycle}
		request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadHoldingRegisters, step.Offset, step.Quantity)
		if err != nil || step.Quantity > huaweiSUN2000MaxReadWords {
			return HuaweiSUN2000MonitoringReading{}, ErrHuaweiSUN2000MonitoringObserverInvalid
		}
		response, err := o.session.ReadHolding(ctx, o.unitID, request)
		var protocol *modbus.ProtocolError
		switch {
		case err == nil && response.Provenance == (modbus.ReadProvenance{
			Function: modbus.FunctionReadHoldingRegisters, Table: modbus.HoldingRegisters, Offset: step.Offset, Quantity: step.Quantity,
		}) && len(response.Words) == int(step.Quantity):
			reading.Outcome, reading.Words = HuaweiSUN2000GroupAnswered, append([]uint16(nil), response.Words...)
		case errors.As(err, &protocol) && protocol.Kind == modbus.ErrorExceptionResponse && protocol.ExceptionCode != 0:
			reading.Outcome, reading.ExceptionCode, reading.Cycle = HuaweiSUN2000GroupException, protocol.ExceptionCode, 0
		default:
			reading.Outcome, reading.Cycle = HuaweiSUN2000GroupFailed, 0
		}
		readings = append(readings, reading)
		if reading.Outcome == HuaweiSUN2000GroupFailed {
			break
		}
	}
	decoded, decodeErr := o.profile.Decode(o.unitID, readings)
	if ctxErr := ctx.Err(); ctxErr != nil && len(readings) > 0 && readings[len(readings)-1].Outcome == HuaweiSUN2000GroupFailed {
		if decodeErr != nil {
			return HuaweiSUN2000MonitoringReading{}, ctxErr
		}
		return decoded, ctxErr
	}
	return decoded, decodeErr
}
