package huawei

import (
	"bytes"
	"context"
	"errors"
	"strconv"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// SUN2000CapabilityOutcome is the outcome of SUN2000 monitoring profile
// selection.
// The selection contract is
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/monitoring-readonly-v1.md.
type SUN2000CapabilityOutcome string

// Capability outcomes. Only SUN2000ProfileSelected carries a profile.
const (
	// SUN2000ProfileSelected means exactly one profile was selected.
	SUN2000ProfileSelected SUN2000CapabilityOutcome = "PROFILE_SELECTED"
	// SUN2000NoAdmissibleProfile means identity was established but no
	// profile serves the selected catalog row or the catalog selected none.
	SUN2000NoAdmissibleProfile SUN2000CapabilityOutcome = "NO_ADMISSIBLE_PROFILE"
	// SUN2000InsufficientEvidence means gateway, routing or identity
	// evidence was missing or invalid; it is the fail-closed default.
	SUN2000InsufficientEvidence SUN2000CapabilityOutcome = "INSUFFICIENT_EVIDENCE"
	// SUN2000AmbiguousMultipleProfiles means more than one profile or
	// catalog row fit; nothing is chosen.
	SUN2000AmbiguousMultipleProfiles SUN2000CapabilityOutcome = "AMBIGUOUS"
)

const huaweiSUN2000ModelBlockBytes = 2 * huaweiSUN2000ModelQuantity

// SUN2000CapabilityReason names why one profile was or was not selected.
type SUN2000CapabilityReason string

// Capability reasons, in the order a profile evaluation can stop.
const (
	// SUN2000ReasonNoGatewayIdentity means the input carries no gateway
	// evidence for the profile's gateway card.
	SUN2000ReasonNoGatewayIdentity SUN2000CapabilityReason = "NO_GATEWAY_IDENTITY"
	// SUN2000ReasonGatewayIdentityRejected means the observed gateway
	// card did not return a test-ready identity match.
	SUN2000ReasonGatewayIdentityRejected SUN2000CapabilityReason = "GATEWAY_IDENTITY_NOT_MATCHED"
	// SUN2000ReasonUnitNotRouted means the gateway evidence does not
	// route to the requested unit.
	SUN2000ReasonUnitNotRouted SUN2000CapabilityReason = "UNIT_NOT_ROUTED"
	// SUN2000ReasonModelBlockInvalid means the routed model read does
	// not have its exact extent and NUL-terminated printable form.
	SUN2000ReasonModelBlockInvalid SUN2000CapabilityReason = "MODEL_BLOCK_INVALID"
	// SUN2000ReasonListingModelConflict means the gateway listing names
	// a model at the unit that differs from the routed model text.
	SUN2000ReasonListingModelConflict SUN2000CapabilityReason = "LISTING_MODEL_CONFLICT"
	// SUN2000ReasonIdentityWordsInvalid means a routed identity part
	// was read but does not have the shape of its read.
	SUN2000ReasonIdentityWordsInvalid SUN2000CapabilityReason = "IDENTITY_WORDS_INVALID"
	// SUN2000ReasonCatalogNotSelected means the catalog decision
	// selected no row.
	SUN2000ReasonCatalogNotSelected SUN2000CapabilityReason = "CATALOG_NOT_SELECTED"
	// SUN2000ReasonCatalogAmbiguous means the catalog decision could not
	// tell rows apart.
	SUN2000ReasonCatalogAmbiguous SUN2000CapabilityReason = "CATALOG_AMBIGUOUS"
	// SUN2000ReasonCatalogRowNoProfile means the selected row is not the
	// model of the evaluated profile.
	SUN2000ReasonCatalogRowNoProfile SUN2000CapabilityReason = "CATALOG_ROW_WITHOUT_PROFILE"
	// SUN2000ReasonMatched accompanies a selected profile.
	SUN2000ReasonMatched SUN2000CapabilityReason = "MATCHED"
	// SUN2000ReasonMultipleProfilesSelected means more than one profile
	// evaluation selected its profile.
	SUN2000ReasonMultipleProfilesSelected SUN2000CapabilityReason = "MULTIPLE_PROFILES_SELECTED"
)

const (
	huaweiSUN2000TextBlockBytes = 2 * 15
	huaweiSUN2000ProtocolWords  = 2
)

// SUN2000RoutedIdentity is the identity evidence read at the routed unit
// besides the model text, each part as read. A nil part was not read.
type SUN2000RoutedIdentity struct {
	// IdentifierWords are the words of the FC03 offset 30070 quantity 1 read.
	IdentifierWords []uint16
	// SoftwareBlock is the raw extent of the FC03 offset 30050 quantity 15 read.
	SoftwareBlock []byte
	// FirmwareBlock is the raw extent of the FC03 offset 30035 quantity 15 read.
	FirmwareBlock []byte
	// ProtocolWords are the words of the FC03 offset 30068 quantity 2 read;
	// they are recorded and never compared.
	ProtocolWords []uint16
}

// SUN2000CapabilityInput is detached, already-read evidence for one
// endpoint and one unit. It carries no endpoint, session or transport state.
type SUN2000CapabilityInput struct {
	// SmartLogger is the observed SmartLogger gateway evidence, if any.
	SmartLogger *SmartLoggerObservedQualificationInput
	// SDongle is the observed S-Dongle gateway evidence, if any. Its child
	// reads carry the routed model read of the child unit.
	SDongle *SDongleObservedQualificationInput
	// UnitID is the routed unit the profile would read.
	UnitID uint8
	// RoutedModelBlock is the raw extent of the FC03 offset 30000 quantity 15
	// read at UnitID behind the SmartLogger.
	RoutedModelBlock []byte
	// Identity is the rest of the identity evidence read at UnitID. The
	// catalog decision selects a row only when the model identifier was read.
	Identity SUN2000RoutedIdentity
}

// SUN2000ProfileEvaluation is the result of one profile against the input.
type SUN2000ProfileEvaluation struct {
	profileID string
	outcome   SUN2000CapabilityOutcome
	reason    SUN2000CapabilityReason
	catalog   *SUN2000CatalogDecision
}

// ProfileID returns the identifier of the evaluated monitoring profile.
func (e SUN2000ProfileEvaluation) ProfileID() string { return e.profileID }

// Outcome returns the outcome of this profile alone, before the selection
// combines all evaluations.
func (e SUN2000ProfileEvaluation) Outcome() SUN2000CapabilityOutcome {
	return e.outcome
}

// Reason returns the step at which this profile's evaluation stopped.
func (e SUN2000ProfileEvaluation) Reason() SUN2000CapabilityReason { return e.reason }

// Catalog is the catalog decision taken for the profile's path, if the
// gateway evidence reached it.
func (e SUN2000ProfileEvaluation) Catalog() (SUN2000CatalogDecision, bool) {
	if e.catalog == nil {
		return SUN2000CatalogDecision{}, false
	}
	return *e.catalog, true
}

// SUN2000CapabilitySelection is the result of capability detection. A
// profile is selected only when exactly one profile evaluation selects it;
// order, priority and vendor name never choose a winner.
type SUN2000CapabilitySelection struct {
	outcome     SUN2000CapabilityOutcome
	reason      SUN2000CapabilityReason
	unitID      uint8
	profile     *SUN2000MonitoringProfile
	catalog     *SUN2000CatalogDecision
	evaluations []SUN2000ProfileEvaluation
}

// Catalog is the catalog decision that the outcome rests on: the one of
// the selected profile, or of the evaluation that decided the outcome.
func (s SUN2000CapabilitySelection) Catalog() (SUN2000CatalogDecision, bool) {
	if s.catalog == nil {
		return SUN2000CatalogDecision{}, false
	}
	return *s.catalog, true
}

// Outcome returns the combined outcome over all profile evaluations.
func (s SUN2000CapabilitySelection) Outcome() SUN2000CapabilityOutcome {
	return s.outcome
}

// Reason returns the reason of the evaluation that decided the outcome.
func (s SUN2000CapabilitySelection) Reason() SUN2000CapabilityReason { return s.reason }

// UnitID returns the routed unit of the input, whatever the outcome.
func (s SUN2000CapabilitySelection) UnitID() uint8 { return s.unitID }

// Profile returns the selected profile. The second result is true only for
// SUN2000ProfileSelected.
func (s SUN2000CapabilitySelection) Profile() (SUN2000MonitoringProfile, bool) {
	if s.profile == nil {
		return SUN2000MonitoringProfile{}, false
	}
	return *s.profile, true
}

// Evaluations returns a copy of the per-profile evaluations in profile order.
func (s SUN2000CapabilitySelection) Evaluations() []SUN2000ProfileEvaluation {
	return append([]SUN2000ProfileEvaluation(nil), s.evaluations...)
}

// Plan is the bounded read plan of the selected profile, or nothing.
func (s SUN2000CapabilitySelection) Plan() []SUN2000ReadStep {
	if s.profile == nil {
		return nil
	}
	return s.profile.Plan(s.unitID)
}

// BatteryProfileSelected is false: no LUNA2000 profile is admitted, whether
// or not the parent inverter is identified.
func (SUN2000CapabilitySelection) BatteryProfileSelected() bool { return false }

// CatalogRegistered is always false: a selection registers nothing.
func (SUN2000CapabilitySelection) CatalogRegistered() bool { return false }

// AutomaticRuntimeAdmission is always false: a selection admits no runtime
// provider.
func (SUN2000CapabilitySelection) AutomaticRuntimeAdmission() bool { return false }

// LiveQualified is always false: selection evaluates detached evidence only.
func (SUN2000CapabilitySelection) LiveQualified() bool { return false }

// SupportClaim is always false: a selected profile is not a support claim.
func (SUN2000CapabilitySelection) SupportClaim() bool { return false }

// WriteAuthority is always false: profiles are read-only.
func (SUN2000CapabilitySelection) WriteAuthority() bool { return false }

// AutomaticRequestCount is always zero: selection issues no requests.
func (SUN2000CapabilitySelection) AutomaticRequestCount() int { return 0 }

// SelectHuaweiSUN2000MonitoringProfile evaluates every monitoring profile
// against the input. The gateway identity comes from the observed gateway card,
// the routed unit and the routed model text; the model row comes from the
// catalog decision over the routed identity words in the inverter map scope,
// and a profile is routed by the selected row of its family. A missing piece
// of gateway evidence is insufficient evidence; identity established at a
// routed unit that no profile serves is no admissible profile; a catalog
// decision that cannot tell two rows apart is ambiguous.
//
// The catalog decision alone routes: without the model identifier it selects
// no row, so no profile is selected on the model text alone. The read-only
// providers read the identifier, the software text and the firmware text at
// child admission and pass them here.
func SelectHuaweiSUN2000MonitoringProfile(input SUN2000CapabilityInput) SUN2000CapabilitySelection {
	profiles := SUN2000MonitoringProfiles()
	selection := SUN2000CapabilitySelection{
		outcome:     SUN2000InsufficientEvidence,
		reason:      SUN2000ReasonNoGatewayIdentity,
		unitID:      input.UnitID,
		evaluations: make([]SUN2000ProfileEvaluation, 0, len(profiles)),
	}
	var matched []int
	ambiguous, noAdmissible, firstInsufficient := -1, -1, -1
	for index, profile := range profiles {
		evaluation := evaluateHuaweiSUN2000Profile(profile, input)
		selection.evaluations = append(selection.evaluations, evaluation)
		switch evaluation.outcome {
		case SUN2000ProfileSelected:
			matched = append(matched, index)
		case SUN2000AmbiguousMultipleProfiles:
			if ambiguous < 0 {
				ambiguous = index
			}
		case SUN2000NoAdmissibleProfile:
			if noAdmissible < 0 {
				noAdmissible = index
			}
		default:
			if firstInsufficient < 0 && evaluation.reason != SUN2000ReasonNoGatewayIdentity {
				firstInsufficient = index
			}
		}
	}
	decided := -1
	switch {
	case len(matched) > 1:
		selection.outcome, selection.reason = SUN2000AmbiguousMultipleProfiles, SUN2000ReasonMultipleProfilesSelected
	case len(matched) == 1:
		profile := profiles[matched[0]]
		selection.outcome, selection.reason, selection.profile = SUN2000ProfileSelected, selection.evaluations[matched[0]].reason, &profile
		decided = matched[0]
	case ambiguous >= 0:
		selection.outcome, selection.reason = SUN2000AmbiguousMultipleProfiles, SUN2000ReasonCatalogAmbiguous
		decided = ambiguous
	case noAdmissible >= 0:
		selection.outcome, selection.reason = SUN2000NoAdmissibleProfile, selection.evaluations[noAdmissible].reason
		decided = noAdmissible
	case firstInsufficient >= 0:
		selection.reason = selection.evaluations[firstInsufficient].reason
		decided = firstInsufficient
	}
	if decided >= 0 {
		selection.catalog = selection.evaluations[decided].catalog
	}
	return selection
}

func evaluateHuaweiSUN2000Profile(profile SUN2000MonitoringProfile, input SUN2000CapabilityInput) SUN2000ProfileEvaluation {
	result := func(outcome SUN2000CapabilityOutcome, reason SUN2000CapabilityReason) SUN2000ProfileEvaluation {
		return SUN2000ProfileEvaluation{profileID: profile.id, outcome: outcome, reason: reason}
	}
	insufficient := func(reason SUN2000CapabilityReason) SUN2000ProfileEvaluation {
		return result(SUN2000InsufficientEvidence, reason)
	}
	var model string
	switch profile.identityCard {
	case SmartLoggerObservedQualificationCardID:
		if input.SmartLogger == nil {
			return insufficient(SUN2000ReasonNoGatewayIdentity)
		}
		gateway := NewSmartLoggerObservedQualificationCard().Evaluate(*input.SmartLogger)
		inventory, ok := gateway.Inventory()
		if !gateway.IdentityMatched() || !gateway.QualificationTestReady() || !ok {
			return insufficient(SUN2000ReasonGatewayIdentityRejected)
		}
		listed, found := huaweiSUN2000ListedChild(inventory, input.UnitID)
		if !found {
			return insufficient(SUN2000ReasonUnitNotRouted)
		}
		text, valid := parseHuaweiSUN2000RoutedModelBlock(input.RoutedModelBlock)
		if !valid {
			return insufficient(SUN2000ReasonModelBlockInvalid)
		}
		if listed != "" && listed != text {
			return insufficient(SUN2000ReasonListingModelConflict)
		}
		model = text
	case SDongleObservedQualificationCardID:
		if input.SDongle == nil {
			return insufficient(SUN2000ReasonNoGatewayIdentity)
		}
		card := NewSDongleObservedQualificationCard()
		gateway := card.Evaluate(*input.SDongle)
		if !gateway.IdentityMatched() || !gateway.QualificationTestReady() {
			return insufficient(SUN2000ReasonGatewayIdentityRejected)
		}
		if input.UnitID != card.ChildUnit() || input.SDongle.Child.UnitID != card.ChildUnit() {
			return insufficient(SUN2000ReasonUnitNotRouted)
		}
		model = card.ChildModel()
	default:
		return insufficient(SUN2000ReasonNoGatewayIdentity)
	}
	catalogInput, valid := huaweiSUN2000CatalogInputOf(model, input.Identity)
	if !valid {
		return insufficient(SUN2000ReasonIdentityWordsInvalid)
	}
	decision := DecideHuaweiSUN2000CatalogSelection(catalogInput)
	evaluated := func(outcome SUN2000CapabilityOutcome, reason SUN2000CapabilityReason) SUN2000ProfileEvaluation {
		evaluation := result(outcome, reason)
		evaluation.catalog = &decision
		return evaluation
	}
	if row, selected := decision.Row(); selected {
		if row.Token != profile.model {
			return evaluated(SUN2000NoAdmissibleProfile, SUN2000ReasonCatalogRowNoProfile)
		}
		return evaluated(SUN2000ProfileSelected, SUN2000ReasonMatched)
	}
	if decision.Outcome() == SUN2000CatalogAmbiguous {
		return evaluated(SUN2000AmbiguousMultipleProfiles, SUN2000ReasonCatalogAmbiguous)
	}
	return evaluated(SUN2000NoAdmissibleProfile, SUN2000ReasonCatalogNotSelected)
}

// huaweiSUN2000CatalogInputOf decodes the routed identity words. A part that
// was read but does not have the shape of its read is invalid evidence.
func huaweiSUN2000CatalogInputOf(model string, identity SUN2000RoutedIdentity) (SUN2000CatalogInput, bool) {
	input := SUN2000CatalogInput{Scope: SUN2000ScopeInverterMap, ModelText: model}
	if identity.IdentifierWords != nil {
		if len(identity.IdentifierWords) != 1 {
			return SUN2000CatalogInput{}, false
		}
		identifier := identity.IdentifierWords[0]
		input.ModelIdentifier = &identifier
	}
	for _, part := range []struct {
		block  []byte
		target *string
	}{{identity.SoftwareBlock, &input.SoftwareText}, {identity.FirmwareBlock, &input.FirmwareText}} {
		if part.block == nil {
			continue
		}
		text, valid := parseHuaweiSUN2000TextBlock(part.block, huaweiSUN2000TextBlockBytes)
		if !valid {
			return SUN2000CatalogInput{}, false
		}
		*part.target = text
	}
	if identity.ProtocolWords != nil {
		if len(identity.ProtocolWords) != huaweiSUN2000ProtocolWords {
			return SUN2000CatalogInput{}, false
		}
		input.ProtocolWords = append([]uint16(nil), identity.ProtocolWords...)
	}
	return input, true
}

// huaweiSUN2000ListedChild finds the child that the validated listing names at
// the unit. The listing routes; it never supplies identity.
func huaweiSUN2000ListedChild(inventory GatewayInventory, unit uint8) (string, bool) {
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
	return parseHuaweiSUN2000TextBlock(block, huaweiSUN2000ModelBlockBytes)
}

// parseHuaweiSUN2000TextBlock decodes a text read of the given extent:
// printable text ended by a NUL, then NUL padding only.
func parseHuaweiSUN2000TextBlock(block []byte, extent int) (string, bool) {
	if len(block) != extent {
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

// SUN2000MonitoringSession is an already configured read boundary. It
// exposes one holding-register read and nothing else.
type SUN2000MonitoringSession interface {
	// ReadHolding reads holding registers at the given unit. A Modbus
	// exception is returned as a *modbus.ProtocolError of kind
	// modbus.ErrorExceptionResponse; any other error fails the group.
	ReadHolding(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// ErrHuaweiSUN2000MonitoringObserverInvalid reports an observer that cannot run.
var ErrHuaweiSUN2000MonitoringObserverInvalid = errors.New("huawei SUN2000 monitoring observer is invalid")

// SUN2000MonitoringObserver reads the plan of one selected profile at
// one unit and decodes it. It owns neither discovery, retries, pacing nor
// publication.
type SUN2000MonitoringObserver struct {
	profile SUN2000MonitoringProfile
	unitID  uint8
	session SUN2000MonitoringSession
}

// NewSUN2000MonitoringObserver binds a selected profile to a session.
// Any selection other than one selected profile is refused.
func NewSUN2000MonitoringObserver(selection SUN2000CapabilitySelection, session SUN2000MonitoringSession) (*SUN2000MonitoringObserver, error) {
	profile, ok := selection.Profile()
	if session == nil || !ok || selection.Outcome() != SUN2000ProfileSelected || selection.UnitID() == 0 {
		return nil, ErrHuaweiSUN2000MonitoringObserverInvalid
	}
	return &SUN2000MonitoringObserver{profile: profile, unitID: selection.UnitID(), session: session}, nil
}

// Observe reads each group of the plan once, in order. An exception reply is
// recorded with its code and the next group is read. A timeout, a transport
// error or a reply that does not match the request fails the group and ends
// the unit's cycle: the remaining groups are not read. A cancelled context
// returns the groups already read together with the context error.
func (o *SUN2000MonitoringObserver) Observe(ctx context.Context, cycle uint64) (SUN2000MonitoringReading, error) {
	if o == nil || o.session == nil || ctx == nil {
		return SUN2000MonitoringReading{}, ErrHuaweiSUN2000MonitoringObserverInvalid
	}
	plan := o.profile.Plan(o.unitID)
	readings := make([]SUN2000GroupReading, 0, len(plan))
	for _, step := range plan {
		if err := ctx.Err(); err != nil {
			partial, decodeErr := o.profile.Decode(o.unitID, readings)
			if decodeErr != nil {
				return SUN2000MonitoringReading{}, err
			}
			return partial, err
		}
		reading := SUN2000GroupReading{Offset: step.Offset, Quantity: step.Quantity, Cycle: cycle}
		request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadHoldingRegisters, step.Offset, step.Quantity)
		if err != nil || step.Quantity > huaweiSUN2000MaxReadWords {
			return SUN2000MonitoringReading{}, ErrHuaweiSUN2000MonitoringObserverInvalid
		}
		response, err := o.session.ReadHolding(ctx, o.unitID, request)
		var protocol *modbus.ProtocolError
		switch {
		case err == nil && response.Provenance == (modbus.ReadProvenance{
			Function: modbus.FunctionReadHoldingRegisters, Table: modbus.HoldingRegisters, Offset: step.Offset, Quantity: step.Quantity,
		}) && len(response.Words) == int(step.Quantity):
			reading.Outcome, reading.Words = SUN2000GroupAnswered, append([]uint16(nil), response.Words...)
		case errors.As(err, &protocol) && protocol.Kind == modbus.ErrorExceptionResponse && protocol.ExceptionCode != 0:
			reading.Outcome, reading.ExceptionCode, reading.Cycle = SUN2000GroupException, protocol.ExceptionCode, 0
		default:
			reading.Outcome, reading.Cycle = SUN2000GroupFailed, 0
		}
		readings = append(readings, reading)
		if reading.Outcome == SUN2000GroupFailed {
			break
		}
	}
	decoded, decodeErr := o.profile.Decode(o.unitID, readings)
	if ctxErr := ctx.Err(); ctxErr != nil && len(readings) > 0 && readings[len(readings)-1].Outcome == SUN2000GroupFailed {
		if decodeErr != nil {
			return SUN2000MonitoringReading{}, ctxErr
		}
		return decoded, ctxErr
	}
	return decoded, decodeErr
}
