package huawei

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// The shared mechanics of the Huawei read-only production providers. One
// provider exists per gateway path whose disposition admits one: SmartLogger,
// EMMA and S-Dongle. A provider drives a supplied
// read-only session through the gateway identity card, child routing, the
// child's admission (its identity words read at the routed unit and the SUN2000
// capability selection over the catalog decision) and the monitoring cycle
// of the docs-modbus read pacing and acquisition contract. It opens nothing:
// the caller supplies the session, the clock and the wait, and owns the
// cadence.

// ProviderPath names a Huawei gateway path.
type ProviderPath string

// The Huawei gateway paths.
const (
	ProviderPathSmartLogger ProviderPath = "SmartLogger"
	ProviderPathEMMA        ProviderPath = "EMMA"
	ProviderPathSDongle     ProviderPath = "S-Dongle"
)

// The errors of the provider lifecycle and of a refresh. Construction returns
// ErrHuaweiProviderNotAdmitted when the path's disposition admits no provider;
// ErrHuaweiProviderConfig reports an invalid configuration or argument.
var (
	ErrHuaweiProviderConfig      = errors.New("huawei provider: invalid configuration")
	ErrHuaweiProviderNotAdmitted = errors.New("huawei provider: the path's disposition admits no provider")
	ErrHuaweiProviderNotStarted  = errors.New("huawei provider: no session is bound")
	ErrHuaweiProviderStarted     = errors.New("huawei provider: a session is already bound")
	ErrHuaweiProviderBusy        = errors.New("huawei provider: a refresh is in progress")
	ErrHuaweiProviderClosed      = errors.New("huawei provider: closed")
	ErrHuaweiProviderClosing     = errors.New("huawei provider: a session close is in progress")
	ErrHuaweiProviderEarly       = errors.New("huawei provider: the cycle interval has not elapsed")
	ErrHuaweiProviderFenced      = errors.New("huawei provider: the cycle was fenced by a stop or a reconnect")
	ErrHuaweiProviderPacing      = errors.New("huawei provider: the caller's wait returned before the contract gap")
)

// IdentificationRequest is one FC2B / MEI 0x0e Read Device
// Identification request: read code 1 (basic) or 3 (extended) and the object
// identifier the read starts at.
type IdentificationRequest struct {
	ReadDeviceIDCode uint8
	ObjectID         uint8
}

// IdentificationPage is one already-decoded Read Device Identification
// response with the wire facts that the observed dialects tolerate: the
// response length, the declared-object field and the continuation fields.
type IdentificationPage struct {
	ReadDeviceIDCode uint8
	Conformity       uint8
	ResponseBytes    int
	DeclaredObjects  int
	More             bool
	NextObjectID     uint8
	Objects          []GatewayInventoryObject
}

func (page IdentificationPage) clone() IdentificationPage {
	objects := make([]GatewayInventoryObject, len(page.Objects))
	for index, object := range page.Objects {
		objects[index] = GatewayInventoryObject{ObjectID: object.ObjectID, Value: append([]byte(nil), object.Value...)}
	}
	page.Objects = objects
	return page
}

// ProviderSession is one supplied, already connected read-only session:
// one connection generation of one endpoint. It exposes the two reads the
// contract allows and the close of its own connection. An exception reply is a
// *modbus.ProtocolError of kind modbus.ErrorExceptionResponse; a malformed reply
// is one of kind modbus.ErrorMalformedResponse; a timeout is an error that
// wraps context.DeadlineExceeded. Any other error is a transport failure.
type ProviderSession interface {
	ReadHolding(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
	ReadDeviceIdentification(context.Context, byte, IdentificationRequest) (IdentificationPage, error)
	Close() error
}

// ProviderWait blocks until the duration has passed or the context ends.
// It is the caller's cadence: the provider asks for the gap of its path and
// checks the gap against its clock before every request.
type ProviderWait func(context.Context, time.Duration) error

// ProviderPacing is the read pacing envelope of one path.
type ProviderPacing struct {
	// Gap is the least time from the end of one exchange to the next request.
	Gap time.Duration
	// Cycle is the recommended cycle. A cycle whose read time exceeds three
	// quarters of it lengthens the interval to the next cycle; the gap is never
	// shortened.
	Cycle time.Duration
	// AttemptDeadline bounds one wire attempt.
	AttemptDeadline time.Duration
	// MaxAttempts bounds the wire attempts of one read.
	MaxAttempts int
	// Backoff is the wait before each repeat of a failed attempt.
	Backoff []time.Duration
	// FirstRequestExtraAttempt allows the first request of a connection
	// generation one extra attempt: its first timeout is recorded and not
	// counted as a failure.
	FirstRequestExtraAttempt bool
	// PromptBound separates a prompt reply from a slower one in the elapsed
	// class of an exchange.
	PromptBound time.Duration
}

func (p ProviderPacing) clone() ProviderPacing {
	p.Backoff = append([]time.Duration(nil), p.Backoff...)
	return p
}

// ProviderPacingFor returns the pacing envelope of a path with a provider.
func ProviderPacingFor(path ProviderPath) (ProviderPacing, bool) {
	switch path {
	case ProviderPathSmartLogger, ProviderPathEMMA:
		return ProviderPacing{
			Gap: 750 * time.Millisecond, Cycle: 60 * time.Second, AttemptDeadline: 5 * time.Second, MaxAttempts: 4,
			Backoff: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, PromptBound: 100 * time.Millisecond,
		}, true
	case ProviderPathSDongle:
		return ProviderPacing{
			Gap: 5 * time.Second, Cycle: 60 * time.Second, AttemptDeadline: 5 * time.Second, MaxAttempts: 4,
			Backoff:                  []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second},
			FirstRequestExtraAttempt: true, PromptBound: 100 * time.Millisecond,
		}, true
	}
	return ProviderPacing{}, false
}

// ---- enablement --------------------------------------------------------------

// ProviderChildDisposition is one child monitoring profile row of a path.
type ProviderChildDisposition struct {
	Family    string
	ProfileID string
	// Model is the catalogue model token of the child. An empty model uses
	// the model of the named hand profile.
	Model string
	// Unit is the configured child unit. Zero means the row names none.
	// A unit through 247 is a configured child on every path, including a dongle.
	Unit    uint8
	Outcome string
	Reason  string
}

// ProviderDisposition is the part of a path's recorded disposition that
// the enablement gate reads. The recorded values mirror the disposition files
// under profiles/vendors/huawei; a test keeps them equal.
type ProviderDisposition struct {
	Path                   ProviderPath
	IdentityCard           string
	IdentityStatus         string
	IdentityGateRegistered bool
	IdentityScope          string
	// RecordedStop is what an earlier recorded stop is superseded for, or
	// empty when the path has none.
	RecordedStop string
	// FurtherLiveIO is the recorded rule for live reads after the stop, or
	// empty when the path has none.
	FurtherLiveIO             string
	ChildProfiles             []ProviderChildDisposition
	CatalogRegistered         bool
	AutomaticRuntimeAdmission bool
	SupportClaim              bool
	WriteAuthority            bool
	// Qualified is the declared runtime binding. HardwareTest records a
	// hardware test beside it and never changes Qualified.
	Qualified    bool
	HardwareTest string
}

func (d ProviderDisposition) clone() ProviderDisposition {
	d.ChildProfiles = append([]ProviderChildDisposition(nil), d.ChildProfiles...)
	return d
}

const (
	huaweiIdentityAdmitted     = "OFFLINE_IDENTITY_ADMITTED"
	huaweiIdentityObservedOnly = "OBSERVED_TUPLE_ONLY"
	huaweiProfileAdmitted      = "OFFLINE_PROFILE_ADMITTED"
	huaweiHardwareTested       = "TESTED_ON_HARDWARE"
)

// RecordedProviderDisposition returns the recorded disposition of a path.
func RecordedProviderDisposition(path ProviderPath) (ProviderDisposition, bool) {
	switch path {
	case ProviderPathSmartLogger:
		return ProviderDisposition{
			Path: path, IdentityCard: SmartLoggerObservedQualificationCardID, IdentityStatus: huaweiIdentityAdmitted,
			IdentityGateRegistered: true, IdentityScope: huaweiIdentityObservedOnly,
			ChildProfiles: []ProviderChildDisposition{
				{Family: huaweiSUN2000M3Family, ProfileID: SUN2000M3MonitoringProfileID, Model: huaweiSUN2000M3Model, Outcome: huaweiProfileAdmitted},
			},
			Qualified: true, HardwareTest: huaweiHardwareTested,
		}, true
	case ProviderPathSDongle:
		return ProviderDisposition{
			Path: path, IdentityCard: SDongleObservedQualificationCardID, IdentityStatus: huaweiIdentityAdmitted,
			IdentityGateRegistered: true, IdentityScope: huaweiIdentityObservedOnly,
			ChildProfiles: []ProviderChildDisposition{
				{Family: huaweiSUN2000M1Family, ProfileID: SUN2000M1MonitoringProfileID, Model: huaweiSDongleObservedChildModel, Outcome: huaweiProfileAdmitted},
			},
			Qualified: true, HardwareTest: huaweiHardwareTested,
		}, true
	case ProviderPathEMMA:
		return ProviderDisposition{
			Path: path, IdentityCard: EMMAObservedQualificationCardID, IdentityStatus: huaweiIdentityAdmitted,
			IdentityGateRegistered: true, IdentityScope: huaweiIdentityObservedOnly,
			ChildProfiles: []ProviderChildDisposition{
				{Family: "SUN2000 MAP0 series", Model: "SUN2000_12K_MAP0", Outcome: huaweiProfileAdmitted},
			},
			Qualified: true, HardwareTest: huaweiHardwareTested,
		}, true
	}
	return ProviderDisposition{}, false
}

// ProviderEnablementReason names why the gate admits or refuses a path.
type ProviderEnablementReason string

// The enablement reasons. ProviderEnabled is the only reason of an admitted
// path; every other reason refuses it.
const (
	ProviderEnabled                   ProviderEnablementReason = "PROVIDER_ADMITTED_FOR_DECLARED_IDENTITY"
	ProviderUnknownPath               ProviderEnablementReason = "UNKNOWN_PATH"
	ProviderAuthorityFieldSet         ProviderEnablementReason = "AUTHORITY_FIELD_SET"
	ProviderNoAdmittedChildProfile    ProviderEnablementReason = "NO_ADMITTED_CHILD_PROFILE"
	ProviderIdentityNotAdmitted       ProviderEnablementReason = "GATEWAY_IDENTITY_NOT_ADMITTED"
	ProviderRecordedStopInForce       ProviderEnablementReason = "RECORDED_STOP_IN_FORCE"
	ProviderChildProfileNotRecognised ProviderEnablementReason = "CHILD_PROFILE_NOT_RECOGNISED"
	ProviderDeclaredBindingAbsent     ProviderEnablementReason = "DECLARED_BINDING_ABSENT"
)

// ProviderEnablement is the gate's decision for one disposition.
type ProviderEnablement struct {
	Path     ProviderPath
	Admitted bool
	Reason   ProviderEnablementReason
	// Detail carries the reasons of the child rows that admit nothing.
	Detail   []string
	Profiles []string
}

// EvaluateHuaweiProviderEnablement admits a path when the catalogue declares
// its gateway identity and at least one routed child has a declared monitoring
// profile. The identity read at runtime stays a separate check. WriteAuthority
// and the hardware-test level do not decide enablement. Qualified is the
// declared runtime binding.
func EvaluateHuaweiProviderEnablement(d ProviderDisposition) ProviderEnablement {
	refuse := func(reason ProviderEnablementReason, detail ...string) ProviderEnablement {
		return ProviderEnablement{Path: d.Path, Reason: reason, Detail: detail}
	}
	card, known := huaweiDeclaredGatewayCard(d.Path)
	if !known || !huaweiCatalogDeclaresGateway(d.Path) {
		return refuse(ProviderUnknownPath)
	}
	if d.CatalogRegistered || d.AutomaticRuntimeAdmission || d.SupportClaim {
		return refuse(ProviderAuthorityFieldSet)
	}
	if !d.Qualified {
		return refuse(ProviderDeclaredBindingAbsent)
	}
	if d.IdentityCard != card || d.IdentityStatus != huaweiIdentityAdmitted || !d.IdentityGateRegistered {
		return refuse(ProviderIdentityNotAdmitted)
	}
	var admitted, detail []string
	for _, row := range d.ChildProfiles {
		if row.Outcome != huaweiProfileAdmitted {
			detail = append(detail, row.Family+": "+row.Reason)
			continue
		}
		name, ok := huaweiDeclaredChildName(row)
		if !ok {
			return refuse(ProviderChildProfileNotRecognised, row.Family+": "+row.ProfileID)
		}
		admitted = append(admitted, name)
	}
	if len(admitted) == 0 {
		return refuse(ProviderNoAdmittedChildProfile, detail...)
	}
	if d.RecordedStop != "" || d.FurtherLiveIO != "" {
		return refuse(ProviderRecordedStopInForce)
	}
	return ProviderEnablement{Path: d.Path, Admitted: true, Reason: ProviderEnabled, Profiles: admitted}
}

func huaweiDeclaredGatewayCard(path ProviderPath) (string, bool) {
	switch path {
	case ProviderPathSmartLogger:
		return SmartLoggerObservedQualificationCardID, true
	case ProviderPathSDongle:
		return SDongleObservedQualificationCardID, true
	case ProviderPathEMMA:
		return EMMAObservedQualificationCardID, true
	default:
		return "", false
	}
}

func huaweiCatalogDeclaresGateway(path ProviderPath) bool {
	var family string
	switch path {
	case ProviderPathSmartLogger:
		family = SmartLoggerCanonicalClass
	case ProviderPathSDongle:
		family = SDongleCanonicalClass
	case ProviderPathEMMA:
		family = EMMACanonicalClass
	default:
		return false
	}
	catalog := huaweiGatewayLoadedCatalog()
	if catalog.err != nil {
		return false
	}
	_, ok := catalog.families[family]
	return ok
}

func huaweiCatalogDeclaresInverterModel(model string) bool {
	if model == "" {
		return false
	}
	catalog := huaweiSUN2000LoadedCatalog()
	if catalog.err != nil {
		return false
	}
	for _, entry := range catalog.entries {
		if entry.row.Token == model {
			return true
		}
		for _, alias := range entry.aliases {
			if alias.token == model {
				return true
			}
		}
	}
	return false
}

func huaweiDeclaredChildName(row ProviderChildDisposition) (string, bool) {
	if row.Unit > huaweiSUN2000MaxUnit {
		return "", false
	}
	if row.ProfileID != "" {
		for _, profile := range SUN2000MonitoringProfiles() {
			if profile.ID() != row.ProfileID || profile.Family() != row.Family {
				continue
			}
			model := row.Model
			if model == "" {
				model = profile.Model()
			}
			if model != profile.Model() || !huaweiCatalogDeclaresInverterModel(model) {
				return "", false
			}
			return profile.ID(), true
		}
		return "", false
	}
	if huaweiCatalogDeclaresInverterModel(row.Model) {
		return row.Model, true
	}
	return "", false
}

// ---- output ----------------------------------------------------------------

// ProviderCycleOutcome is the outcome of one refresh.
type ProviderCycleOutcome string

const (
	// ProviderCycleComplete means the guard held or the identification
	// succeeded, and every child of the effective set was handled.
	ProviderCycleComplete ProviderCycleOutcome = "COMPLETE"
	// ProviderCycleIncomplete means the cycle ended before its children were
	// read; the previous complete cycle stays the last complete one.
	ProviderCycleIncomplete ProviderCycleOutcome = "INCOMPLETE"
	// ProviderCycleInsufficient means the gateway identity was not established.
	ProviderCycleInsufficient ProviderCycleOutcome = "INSUFFICIENT_EVIDENCE"
)

// ProviderReason is a descriptive label, not a public interface. The
// child labels follow the decision table of the docs-modbus gateway contract.
type ProviderReason string

// The cycle and child reasons. A gateway reason labels the cycle; a child
// reason labels one child observation.
const (
	ProviderReasonIdentified         ProviderReason = "identified"
	ProviderReasonGuardHeld          ProviderReason = "guard_held"
	ProviderReasonGuardChanged       ProviderReason = "guard_changed"
	ProviderReasonGuardFailed        ProviderReason = "guard_failed"
	ProviderReasonListingUnstable    ProviderReason = "listing_unstable"
	ProviderReasonListingBackoff     ProviderReason = "listing_unstable_backoff"
	ProviderReasonGatewayMismatch    ProviderReason = "gateway_identity_mismatch"
	ProviderReasonGatewayReadFail    ProviderReason = "gateway_read_failed"
	ProviderReasonRead               ProviderReason = "read"
	ProviderReasonNotAllowlisted     ProviderReason = "not_allowlisted"
	ProviderReasonNotListed          ProviderReason = "not_listed"
	ProviderReasonNoClassContract    ProviderReason = "no_class_contract"
	ProviderReasonIdentityInsuff     ProviderReason = "identity_insufficient"
	ProviderReasonIdentityMismatch   ProviderReason = "identity_mismatch"
	ProviderReasonNoProfile          ProviderReason = "no_admissible_profile"
	ProviderReasonChildTimeout       ProviderReason = "child_timeout"
	ProviderReasonChildReadFailed    ProviderReason = "child_read_failed"
	ProviderReasonChildException     ProviderReason = "child_exception"
	ProviderReasonChildBackoff       ProviderReason = "child_backoff"
	ProviderReasonListingUnconfirmed ProviderReason = "listing_unconfirmed"

	// ProviderReasonCatalogNotSelected the catalog decision over
	// the child's identity words selected no row; the observation carries the
	// decision and its closed reason.
	ProviderReasonCatalogNotSelected ProviderReason = "catalog_not_selected"
	// ProviderReasonCatalogAmbiguous the catalog decision could not
	// tell two or more rows apart; the observation names every candidate.
	ProviderReasonCatalogAmbiguous ProviderReason = "catalog_ambiguous"
)

// ProviderEvidenceState is the evidence state of a child in one cycle.
type ProviderEvidenceState string

const (
	// ProviderEvidenceCurrent means every group sent this cycle answered.
	ProviderEvidenceCurrent ProviderEvidenceState = "CURRENT"
	// ProviderEvidenceStale means the child was not fully read this cycle.
	// Facts carried by an earlier reading keep their original cycle and age
	// and are marked retained; the others are Unknown.
	ProviderEvidenceStale ProviderEvidenceState = "STALE"
	// ProviderEvidenceWithheld means the child emits no fact this cycle.
	ProviderEvidenceWithheld ProviderEvidenceState = "WITHHELD"
)

// ProviderGatewayState is the state of the gateway identity.
type ProviderGatewayState string

// The gateway identity states of a cycle. The identity's Generation and
// IdentifiedAt describe the identification run the state rests on.
const (
	ProviderGatewayNotIdentified ProviderGatewayState = "NOT_IDENTIFIED"
	ProviderGatewayIdentified    ProviderGatewayState = "IDENTIFIED"
	ProviderGatewayUnconfirmed   ProviderGatewayState = "UNCONFIRMED"
	ProviderGatewayInsufficient  ProviderGatewayState = "INSUFFICIENT_EVIDENCE"
)

// ProviderGatewayIdentity is the native gateway identity of a cycle.
type ProviderGatewayIdentity struct {
	CardID     string
	Class      string
	UnitID     uint8
	Model      string
	Firmware   string
	ChildModel string
	State      ProviderGatewayState
	CardReason QualificationReason
	// Generation and IdentifiedAt describe the identification run that the
	// state rests on.
	Generation   uint64
	IdentifiedAt time.Time
}

// ProviderGroupEvidence is the provenance of one monitoring group:
// the outcome of this cycle and the read that carried the words.
type ProviderGroupEvidence struct {
	Offset        uint16
	Quantity      uint16
	Outcome       SUN2000GroupOutcome
	ExceptionCode uint8
	// Refused is true for a group refused with exception 3, which is never
	// sent again.
	Refused bool
	// Retained, Cycle, ReadAt and Generation describe the reading whose words
	// the facts of the group carry.
	Retained   bool
	Cycle      uint64
	ReadAt     time.Time
	Generation uint64
}

// ProviderFencedReading is the last reading of a child under an earlier
// identity. It is history with its original age; nothing migrates from it.
type ProviderFencedReading struct {
	ProfileID string
	Model     string
	Reading   SUN2000MonitoringReading
	Groups    []ProviderGroupEvidence
}

func (f *ProviderFencedReading) clone() *ProviderFencedReading {
	if f == nil {
		return nil
	}
	copied := *f
	copied.Groups = append([]ProviderGroupEvidence(nil), f.Groups...)
	return &copied
}

// ProviderChildObservation is one child of a cycle at its routed unit.
type ProviderChildObservation struct {
	UnitID uint8
	// ListedClass is the listing label that routed the identity read. It is
	// never identity.
	ListedClass          string
	Family               string
	ProfileID            string
	Model                string
	Evidence             ProviderEvidenceState
	Reason               ProviderReason
	IdentifiedGeneration uint64
	// Catalog is the catalog decision of the child's last completed
	// admission: the selected row with its card, edition, page, floor and
	// grade for an admitted child, and the closed reason or every candidate
	// for a child it withholds. It is nil when no admission completed.
	Catalog *SUN2000CatalogDecision `json:"catalog"`
	// Reading is the typed monitoring state. It is empty for a withheld child.
	Reading SUN2000MonitoringReading
	Groups  []ProviderGroupEvidence
	Fenced  *ProviderFencedReading
}

func (c ProviderChildObservation) clone() ProviderChildObservation {
	c.Groups = append([]ProviderGroupEvidence(nil), c.Groups...)
	c.Fenced = c.Fenced.clone()
	c.Catalog = huaweiCloneCatalogDecision(c.Catalog)
	return c
}

// huaweiCloneCatalogDecision copies a decision. A decision is never changed
// after it is taken and its accessors return copies, so the copy is shallow.
func huaweiCloneCatalogDecision(decision *SUN2000CatalogDecision) *SUN2000CatalogDecision {
	if decision == nil {
		return nil
	}
	copied := *decision
	return &copied
}

// ProviderExchangeOutcome is the outcome of one wire attempt.
type ProviderExchangeOutcome string

// The wire attempt outcomes: an answer, an exception reply with its code, a
// timeout, a malformed reply, or any other transport failure.
const (
	ProviderExchangeAnswered  ProviderExchangeOutcome = "ANSWERED"
	ProviderExchangeException ProviderExchangeOutcome = "EXCEPTION"
	ProviderExchangeTimeout   ProviderExchangeOutcome = "TIMEOUT"
	ProviderExchangeMalformed ProviderExchangeOutcome = "MALFORMED"
	ProviderExchangeTransport ProviderExchangeOutcome = "TRANSPORT_FAILURE"
)

// ProviderElapsedClass is the timing class of one wire attempt.
type ProviderElapsedClass string

// The elapsed classes: an attempt that ended within the path's prompt bound,
// one that ended later within the attempt deadline, and one that timed out.
const (
	ProviderWithinPromptBound ProviderElapsedClass = "WITHIN_PROMPT_BOUND"
	ProviderWithinDeadline    ProviderElapsedClass = "WITHIN_DEADLINE"
	ProviderAtDeadline        ProviderElapsedClass = "AT_DEADLINE"
	// ProviderFirstRequestTolerated is the first timeout of the first
	// request of a connection generation on a path that tolerates it. It is
	// recorded and not counted as a failed attempt.
	ProviderFirstRequestTolerated ProviderElapsedClass = "FIRST_REQUEST_TIMEOUT_TOLERATED"
)

// ProviderExchange is the native evidence of one wire attempt.
type ProviderExchange struct {
	Function         uint8
	UnitID           uint8
	Offset           uint16
	Quantity         uint16
	ReadDeviceIDCode uint8
	ObjectID         uint8
	Attempt          int
	Outcome          ProviderExchangeOutcome
	ExceptionCode    uint8
	Elapsed          ProviderElapsedClass
	SentAt           time.Time
}

// ProviderLossKind names what a cycle does not publish.
type ProviderLossKind string

// The loss kinds: a planned, refused or failed group, words no field
// consumes, a battery or meter that is not admitted, and a withheld child.
const (
	ProviderLossPlannedGroup    ProviderLossKind = "PLANNED_GROUP_NOT_READ"
	ProviderLossRefusedGroup    ProviderLossKind = "REFUSED_GROUP_NOT_READ"
	ProviderLossFailedGroup     ProviderLossKind = "GROUP_READ_FAILED"
	ProviderLossUnconsumedWords ProviderLossKind = "UNCONSUMED_WORDS"
	ProviderLossBattery         ProviderLossKind = "BATTERY_NOT_ADMITTED"
	ProviderLossMeter           ProviderLossKind = "METER_NOT_ADMITTED"
	ProviderLossWithheldChild   ProviderLossKind = "CHILD_WITHHELD"
)

// ProviderLoss is one named loss. A consumer sees each as Unknown, never
// as zero, empty or false. UnitID is meaningful when HasUnit is true.
type ProviderLoss struct {
	Kind     ProviderLossKind
	HasUnit  bool
	UnitID   uint8
	Offset   uint16
	Quantity uint16
	Detail   string
}

// ProviderCycle is the output of one refresh.
type ProviderCycle struct {
	ProviderID   string
	Path         ProviderPath
	Source       string
	Cycle        uint64
	Generation   uint64
	StartedAt    time.Time
	CompletedAt  time.Time
	Outcome      ProviderCycleOutcome
	Reason       ProviderReason
	Gateway      ProviderGatewayIdentity
	Plant        []ProviderPlantGroup `json:",omitempty"`
	EMMASite     []EMMASiteGroup      `json:",omitempty"`
	Meter        []ProviderMeterGroup `json:",omitempty"`
	Children     []ProviderChildObservation
	Exchanges    []ProviderExchange
	Loss         []ProviderLoss
	PlannedReads int
	WireAttempts int

	// EMMASitePower keeps inferred site directions separate from native facts.
	EMMASitePower []EMMASitePowerReading `json:",omitempty"`
}

func (c ProviderCycle) clone() ProviderCycle {
	children := make([]ProviderChildObservation, len(c.Children))
	for index, child := range c.Children {
		children[index] = child.clone()
	}
	c.Children = children
	if c.Plant != nil {
		plant := make([]ProviderPlantGroup, len(c.Plant))
		for index, group := range c.Plant {
			plant[index] = group.clone()
		}
		c.Plant = plant
	}
	if c.EMMASite != nil {
		site := make([]EMMASiteGroup, len(c.EMMASite))
		for index, group := range c.EMMASite {
			site[index] = group.clone()
		}
		c.EMMASite = site
	}
	c.EMMASitePower = slices.Clone(c.EMMASitePower)
	c.Meter = slices.Clone(c.Meter)
	for index := range c.Meter {
		c.Meter[index] = c.Meter[index].clone()
	}
	c.Exchanges = append([]ProviderExchange(nil), c.Exchanges...)
	c.Loss = append([]ProviderLoss(nil), c.Loss...)
	return c
}

// Child returns the child at a unit.
func (c ProviderCycle) Child(unit uint8) (ProviderChildObservation, bool) {
	for _, child := range c.Children {
		if child.UnitID == unit {
			return child.clone(), true
		}
	}
	return ProviderChildObservation{}, false
}

// CatalogRegistered is always false: a cycle registers nothing in a catalog.
func (ProviderCycle) CatalogRegistered() bool { return false }

// AutomaticRuntimeAdmission is always false: a cycle admits nothing at runtime.
func (ProviderCycle) AutomaticRuntimeAdmission() bool { return false }

// LiveQualified is always false: a cycle is evidence, not a live qualification.
func (ProviderCycle) LiveQualified() bool { return false }

// SupportClaim is always false: a cycle makes no support claim.
func (ProviderCycle) SupportClaim() bool { return false }

// WriteAuthority is always false: a provider never writes.
func (ProviderCycle) WriteAuthority() bool { return false }

// ---- configuration and core ------------------------------------------------

// ProviderConfig is the caller configuration of one provider.
type ProviderConfig struct {
	// Source is an opaque local label of the endpoint: printable ASCII of at
	// most 64 bytes. It is never an address and is never opened.
	Source string
	// AllowedUnits is the operator allowlist of routed units. The SmartLogger
	// provider reads only units in its intersection with the validated
	// listing; the S-Dongle provider requires it to name the observed child.
	AllowedUnits []uint8
	// Clock is monotonic and nonblocking; nil uses time.Now.
	Clock func() time.Time
	// Wait is the caller's cadence. It is required.
	Wait ProviderWait
}

const huaweiProviderMaxSourceBytes = 64

func validHuaweiProviderSource(source string) bool {
	return source != "" && len(source) <= huaweiProviderMaxSourceBytes && isHuaweiSmartLoggerInventoryASCII([]byte(source))
}

type huaweiGroupMeta struct {
	cycle      uint64
	readAt     time.Time
	generation uint64
}

type huaweiChildIdentity struct {
	profile SUN2000MonitoringProfile
	model   string
}

func (i *huaweiChildIdentity) key() string {
	if i == nil {
		return ""
	}
	return i.profile.IdentityCard() + "|" + i.profile.ID() + "|" + i.model
}

// huaweiChildState is the committed state of one routed child. The reading is
// the one last-known-good store: RetainHuaweiSUN2000Monitoring decides which
// group words it keeps, and groups only records when and on which connection
// generation the kept words were read.
type huaweiChildState struct {
	unit                 uint8
	listedClass          string
	identity             *huaweiChildIdentity
	identifiedGeneration uint64
	// definitive is a withhold reason that holds until the next
	// identification event; empty when a failed read may be retried.
	definitive ProviderReason
	// ineligible is the routing reason the latest enumeration withheld the
	// child for; it holds across failure cycles until the child is eligible.
	ineligible ProviderReason
	reading    SUN2000MonitoringReading
	groups     map[uint16]huaweiGroupMeta
	refused    map[string]bool
	failures   int
	skipUntil  uint64
	fenced     *ProviderFencedReading
	// catalog is the decision of the last completed admission. It describes
	// identity while one is bound, and otherwise the definitive withhold.
	catalog  *SUN2000CatalogDecision
	emmaPlan *huaweiEMMAChildPlan
	meter    huaweiMeterState
}

func (c *huaweiChildState) clone() *huaweiChildState {
	copied := *c
	if c.identity != nil {
		identity := *c.identity
		copied.identity = &identity
	}
	copied.catalog = huaweiCloneCatalogDecision(c.catalog)
	copied.groups = make(map[uint16]huaweiGroupMeta, len(c.groups))
	for key, value := range c.groups {
		copied.groups[key] = value
	}
	copied.refused = make(map[string]bool, len(c.refused))
	for key, value := range c.refused {
		copied.refused[key] = value
	}
	copied.fenced = c.fenced.clone()
	return &copied
}

func huaweiRefusalKey(profileID string, offset, quantity uint16) string {
	return profileID + "|" + strconv.Itoa(int(offset)) + "/" + strconv.Itoa(int(quantity))
}

// fence moves the child's reading to history: a changed identity keeps its
// earlier reading with its original age and nothing migrates from it.
func (c *huaweiChildState) fence() {
	if c.identity != nil && c.reading.ProfileID() != "" {
		c.fenced = &ProviderFencedReading{
			ProfileID: c.identity.profile.ID(), Model: c.identity.model, Reading: c.reading, Groups: c.groupEvidence(nil),
		}
	}
	c.reading = SUN2000MonitoringReading{}
	c.meter = huaweiMeterState{}
	c.groups = map[uint16]huaweiGroupMeta{}
	c.failures, c.skipUntil = 0, 0
}

// withhold withholds the child until the next identification event: the
// reading moves to history, no identity is bound, and the decision that
// withheld it, if one was taken, is kept to name why.
func (c *huaweiChildState) withhold(reason ProviderReason, decision *SUN2000CatalogDecision) {
	c.definitive = reason
	c.fence()
	c.identity, c.catalog = nil, decision
}

// identificationEvent makes the child's admission run again at its routed
// unit before the child counts as identified: a definitive withhold is lifted
// and a child back-off ends. A bound identity and its decision stay until the
// admission decides again.
func (c *huaweiChildState) identificationEvent() {
	c.definitive, c.identifiedGeneration, c.skipUntil = "", 0, 0
	if c.identity == nil {
		c.catalog = nil
	}
}

// groupEvidence describes every planned group of the child's reading. current
// carries this cycle's outcomes; nil describes the kept reading only.
func (c *huaweiChildState) groupEvidence(current map[uint16]SUN2000GroupReading) []ProviderGroupEvidence {
	var out []ProviderGroupEvidence
	for _, group := range c.reading.Groups() {
		evidence := ProviderGroupEvidence{Offset: group.Offset, Quantity: group.Quantity, Outcome: SUN2000GroupNotRead}
		if now, ok := current[group.Offset]; ok {
			evidence.Outcome, evidence.ExceptionCode = now.Outcome, now.ExceptionCode
		} else if current == nil {
			evidence.Outcome, evidence.ExceptionCode = group.Outcome, group.ExceptionCode
		}
		if c.identity != nil && c.refused[huaweiRefusalKey(c.identity.profile.ID(), group.Offset, group.Quantity)] {
			evidence.Refused = true
		}
		if group.Outcome == SUN2000GroupAnswered {
			meta := c.groups[group.Offset]
			evidence.Retained, evidence.Cycle, evidence.ReadAt, evidence.Generation = group.Retained, group.Cycle, meta.readAt, meta.generation
		}
		out = append(out, evidence)
	}
	return out
}

type huaweiProviderState struct {
	identified         bool
	identityGeneration uint64
	gateway            ProviderGatewayIdentity
	guard              []uint16
	reidentify         bool
	gatewayFenced      bool
	unstable           int
	endpointFailures   int
	listing            []GatewayInventoryChild
	smartLogger        *SmartLoggerObservedQualificationInput
	sdongle            *SDongleObservedQualificationInput
	children           map[uint8]*huaweiChildState
	plant              [2]huaweiPlantState
	emmaSite           [len(huaweiEMMASite)]huaweiPlantState
	emmaMeter          huaweiMeterState
	// carriedRefusals keeps exception 3 refusals learned in a cycle that was
	// cancelled before it could commit, for children not yet in children.
	carriedRefusals map[uint8]map[string]bool
}

func (s *huaweiProviderState) clone() *huaweiProviderState {
	copied := *s
	copied.guard = append([]uint16(nil), s.guard...)
	copied.listing = slices.Clone(s.listing)
	copied.carriedRefusals = make(map[uint8]map[string]bool, len(s.carriedRefusals))
	for unit, keys := range s.carriedRefusals {
		copied.carriedRefusals[unit] = make(map[string]bool, len(keys))
		for key := range keys {
			copied.carriedRefusals[unit][key] = true
		}
	}
	copied.children = make(map[uint8]*huaweiChildState, len(s.children))
	for unit, child := range s.children {
		copied.children[unit] = child.clone()
	}
	return &copied
}

func (s *huaweiProviderState) child(unit uint8) *huaweiChildState {
	if c, ok := s.children[unit]; ok {
		return c
	}
	c := &huaweiChildState{unit: unit, groups: map[uint16]huaweiGroupMeta{}, refused: map[string]bool{}}
	for key := range s.carriedRefusals[unit] {
		c.refused[key] = true
	}
	for i := range c.meter {
		c.meter[i].refused = c.refused[huaweiMeterRefusalKey(i)]
		delete(c.refused, huaweiMeterRefusalKey(i))
	}
	delete(s.carriedRefusals, unit)
	s.children[unit] = c
	return c
}

// The identity words that a child's admission reads at its routed unit besides
// the model text: the model identifier (30070, defined with the catalog
// decision), the software text and the firmware text. An admission runs on each
// identification event, and on a later cycle only for a child that a failed
// read left unadmitted; the identity words of an admitted child are never read
// in a cycle with the guard held.
const (
	huaweiSUN2000SoftwareOffset = 30050
	huaweiSUN2000FirmwareOffset = 30035
	huaweiSUN2000TextQuantity   = 15
)
