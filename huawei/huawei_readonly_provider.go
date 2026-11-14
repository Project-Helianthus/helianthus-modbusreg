package huawei

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// The shared mechanics of the Huawei read-only production providers. One
// provider exists per gateway path whose disposition admits one: the observed
// SmartLogger path and the observed S-Dongle path. A provider drives a supplied
// read-only session through the gateway identity card, child routing, the
// child's admission (its identity words read at the routed unit and the SUN2000
// capability selection over the catalog decision) and the monitoring cycle
// of the docs-modbus read pacing and acquisition contract. It opens nothing:
// the caller supplies the session, the clock and the wait, and owns the
// cadence.

// ProviderPath names a Huawei gateway path.
type ProviderPath string

const (
	ProviderPathSmartLogger ProviderPath = "SmartLogger"
	ProviderPathEMMA        ProviderPath = "EMMA"
	ProviderPathSDongle     ProviderPath = "S-Dongle"
)

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
	case ProviderPathSmartLogger:
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
	Outcome   string
	Reason    string
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
}

func (d ProviderDisposition) clone() ProviderDisposition {
	d.ChildProfiles = append([]ProviderChildDisposition(nil), d.ChildProfiles...)
	return d
}

const (
	huaweiIdentityAdmitted     = "OFFLINE_IDENTITY_ADMITTED"
	huaweiIdentityObservedOnly = "OBSERVED_TUPLE_ONLY"
	huaweiProfileAdmitted      = "OFFLINE_PROFILE_ADMITTED"
	huaweiStopObservedIdentity = "IDENTITY_OF_OBSERVED_TUPLE_ONLY"
	huaweiLiveIOOperatorAtTime = "OPERATOR_APPROVAL_AT_ACTION_TIME"
)

// RecordedProviderDisposition returns the recorded disposition of a path.
func RecordedProviderDisposition(path ProviderPath) (ProviderDisposition, bool) {
	switch path {
	case ProviderPathSmartLogger:
		return ProviderDisposition{
			Path: path, IdentityCard: SmartLoggerObservedQualificationCardID, IdentityStatus: huaweiIdentityAdmitted,
			IdentityGateRegistered: true, IdentityScope: huaweiIdentityObservedOnly,
			ChildProfiles: []ProviderChildDisposition{
				{Family: huaweiSUN2000M3Family, ProfileID: SUN2000M3MonitoringProfileID, Outcome: huaweiProfileAdmitted},
			},
		}, true
	case ProviderPathSDongle:
		return ProviderDisposition{
			Path: path, IdentityCard: SDongleObservedQualificationCardID, IdentityStatus: huaweiIdentityAdmitted,
			IdentityGateRegistered: true, IdentityScope: huaweiIdentityObservedOnly,
			RecordedStop: huaweiStopObservedIdentity, FurtherLiveIO: huaweiLiveIOOperatorAtTime,
			ChildProfiles: []ProviderChildDisposition{
				{Family: huaweiSUN2000M1Family, ProfileID: SUN2000M1MonitoringProfileID, Outcome: huaweiProfileAdmitted},
			},
		}, true
	case ProviderPathEMMA:
		return ProviderDisposition{
			Path: path, IdentityCard: EMMAObservedQualificationCardID, IdentityStatus: huaweiIdentityAdmitted,
			IdentityGateRegistered: true, IdentityScope: huaweiIdentityObservedOnly,
			ChildProfiles: []ProviderChildDisposition{
				{Family: "SUN2000 MAP0 series", Outcome: "NO_ADMISSIBLE_PROFILE", Reason: "catalog_card_without_monitoring_profile"},
			},
		}, true
	}
	return ProviderDisposition{}, false
}

// ProviderEnablementReason names why the gate admits or refuses a path.
type ProviderEnablementReason string

const (
	ProviderEnabled                   ProviderEnablementReason = "PROVIDER_ADMITTED_FOR_OBSERVED_TUPLE"
	ProviderUnknownPath               ProviderEnablementReason = "UNKNOWN_PATH"
	ProviderAuthorityFieldSet         ProviderEnablementReason = "AUTHORITY_FIELD_SET"
	ProviderNoAdmittedChildProfile    ProviderEnablementReason = "NO_ADMITTED_CHILD_PROFILE"
	ProviderIdentityNotAdmitted       ProviderEnablementReason = "GATEWAY_IDENTITY_NOT_ADMITTED"
	ProviderRecordedStopInForce       ProviderEnablementReason = "RECORDED_STOP_IN_FORCE"
	ProviderChildProfileNotRecognised ProviderEnablementReason = "CHILD_PROFILE_NOT_RECOGNISED"
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

// EvaluateHuaweiProviderEnablement admits a path only when its gateway
// identity is admitted for the observed tuple, at least one child monitoring
// profile of that gateway card is admitted, any recorded stop is superseded for
// the observed identity, and every authority field is false. Anything else
// refuses.
func EvaluateHuaweiProviderEnablement(d ProviderDisposition) ProviderEnablement {
	refuse := func(reason ProviderEnablementReason, detail ...string) ProviderEnablement {
		return ProviderEnablement{Path: d.Path, Reason: reason, Detail: detail}
	}
	var card string
	switch d.Path {
	case ProviderPathSmartLogger:
		card = SmartLoggerObservedQualificationCardID
	case ProviderPathSDongle:
		card = SDongleObservedQualificationCardID
	case ProviderPathEMMA:
	default:
		return refuse(ProviderUnknownPath)
	}
	if d.CatalogRegistered || d.AutomaticRuntimeAdmission || d.SupportClaim || d.WriteAuthority {
		return refuse(ProviderAuthorityFieldSet)
	}
	var admitted, detail []string
	for _, row := range d.ChildProfiles {
		if row.Outcome != huaweiProfileAdmitted {
			detail = append(detail, row.Family+": "+row.Reason)
			continue
		}
		known := false
		for _, profile := range SUN2000MonitoringProfiles() {
			if profile.ID() == row.ProfileID && profile.Family() == row.Family && profile.IdentityCard() == card && card != "" {
				known = true
			}
		}
		if !known {
			return refuse(ProviderChildProfileNotRecognised, row.Family+": "+row.ProfileID)
		}
		admitted = append(admitted, row.ProfileID)
	}
	if len(admitted) == 0 {
		return refuse(ProviderNoAdmittedChildProfile, detail...)
	}
	if d.IdentityCard != card || d.IdentityStatus != huaweiIdentityAdmitted || !d.IdentityGateRegistered || d.IdentityScope != huaweiIdentityObservedOnly {
		return refuse(ProviderIdentityNotAdmitted)
	}
	switch {
	case d.RecordedStop == "" && d.FurtherLiveIO == "":
	case d.RecordedStop == huaweiStopObservedIdentity && d.FurtherLiveIO == huaweiLiveIOOperatorAtTime:
	default:
		return refuse(ProviderRecordedStopInForce)
	}
	return ProviderEnablement{Path: d.Path, Admitted: true, Reason: ProviderEnabled, Profiles: admitted}
}

// ---- output ----------------------------------------------------------------

// ProviderCycleOutcome is the outcome of one refresh.
type ProviderCycleOutcome string

const (
	// ProviderCycleComplete: the guard held or the identification
	// succeeded, and every child of the effective set was handled.
	ProviderCycleComplete ProviderCycleOutcome = "COMPLETE"
	// ProviderCycleIncomplete: the cycle ended before its children were
	// read; the previous complete cycle stays the last complete one.
	ProviderCycleIncomplete ProviderCycleOutcome = "INCOMPLETE"
	// ProviderCycleInsufficient: the gateway identity was not established.
	ProviderCycleInsufficient ProviderCycleOutcome = "INSUFFICIENT_EVIDENCE"
)

// ProviderReason is a descriptive label, not a public interface. The
// child labels follow the decision table of the docs-modbus gateway contract.
type ProviderReason string

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
	// ProviderEvidenceCurrent: every group sent this cycle answered.
	ProviderEvidenceCurrent ProviderEvidenceState = "CURRENT"
	// ProviderEvidenceStale: the child was not fully read this cycle.
	// Facts carried by an earlier reading keep their original cycle and age
	// and are marked retained; the others are Unknown.
	ProviderEvidenceStale ProviderEvidenceState = "STALE"
	// ProviderEvidenceWithheld: the child emits no fact this cycle.
	ProviderEvidenceWithheld ProviderEvidenceState = "WITHHELD"
)

// ProviderGatewayState is the state of the gateway identity.
type ProviderGatewayState string

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

// ProviderGroupEvidence is the provenance of one monitoring group of a
// child: the outcome of this cycle and the read that carried the words.
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

const (
	ProviderExchangeAnswered  ProviderExchangeOutcome = "ANSWERED"
	ProviderExchangeException ProviderExchangeOutcome = "EXCEPTION"
	ProviderExchangeTimeout   ProviderExchangeOutcome = "TIMEOUT"
	ProviderExchangeMalformed ProviderExchangeOutcome = "MALFORMED"
	ProviderExchangeTransport ProviderExchangeOutcome = "TRANSPORT_FAILURE"
)

// ProviderElapsedClass is the timing class of one wire attempt.
type ProviderElapsedClass string

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

const (
	ProviderLossPlannedGroup    ProviderLossKind = "PLANNED_GROUP_NOT_READ"
	ProviderLossRefusedGroup    ProviderLossKind = "REFUSED_GROUP_NOT_READ"
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
	Children     []ProviderChildObservation
	Exchanges    []ProviderExchange
	Loss         []ProviderLoss
	PlannedReads int
	WireAttempts int
}

func (c ProviderCycle) clone() ProviderCycle {
	children := make([]ProviderChildObservation, len(c.Children))
	for index, child := range c.Children {
		children[index] = child.clone()
	}
	c.Children = children
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

func (ProviderCycle) CatalogRegistered() bool         { return false }
func (ProviderCycle) AutomaticRuntimeAdmission() bool { return false }
func (ProviderCycle) LiveQualified() bool             { return false }
func (ProviderCycle) SupportClaim() bool              { return false }
func (ProviderCycle) WriteAuthority() bool            { return false }

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
	catalog *SUN2000CatalogDecision
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
	// carriedRefusals keeps exception 3 refusals learned in a cycle that was
	// cancelled before it could commit, for children not yet in children.
	carriedRefusals map[uint8]map[string]bool
}

func (s *huaweiProviderState) clone() *huaweiProviderState {
	copied := *s
	copied.guard = append([]uint16(nil), s.guard...)
	copied.listing = append([]GatewayInventoryChild(nil), s.listing...)
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
	delete(s.carriedRefusals, unit)
	s.children[unit] = c
	return c
}

// huaweiLinkState is the pacing state of the bound connection generation.
type huaweiLinkState struct {
	lastEnd time.Time
	sent    bool
}

type huaweiCycleRunner func(*huaweiCycleRun)

type huaweiProviderCore struct {
	path       ProviderPath
	providerID string
	source     string
	allowed    []uint8
	pacing     ProviderPacing
	clock      func() time.Time
	wait       ProviderWait
	enablement ProviderEnablement
	run        huaweiCycleRunner
	staticLoss []ProviderLoss

	mu         sync.Mutex
	session    ProviderSession
	generation uint64
	started    bool
	closed     bool
	busy       bool
	closing    bool
	link       huaweiLinkState
	cycle      uint64
	nextAt     time.Time
	state      *huaweiProviderState
	last       *ProviderCycle
}

func newHuaweiProviderCore(path ProviderPath, providerID string, config ProviderConfig, disposition ProviderDisposition, run huaweiCycleRunner, loss []ProviderLoss) (*huaweiProviderCore, error) {
	enablement := EvaluateHuaweiProviderEnablement(disposition.clone())
	if !enablement.Admitted || disposition.Path != path {
		return nil, ErrHuaweiProviderNotAdmitted
	}
	pacing, ok := ProviderPacingFor(path)
	if !ok || config.Wait == nil || !validHuaweiProviderSource(config.Source) {
		return nil, ErrHuaweiProviderConfig
	}
	allowed := append([]uint8(nil), config.AllowedUnits...)
	sort.Slice(allowed, func(i, j int) bool { return allowed[i] < allowed[j] })
	for index, unit := range allowed {
		if unit == 0 || unit > huaweiSUN2000MaxUnit || (index > 0 && allowed[index-1] == unit) {
			return nil, ErrHuaweiProviderConfig
		}
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &huaweiProviderCore{
		path: path, providerID: providerID, source: config.Source, allowed: allowed, pacing: pacing, clock: clock,
		wait: config.Wait, enablement: enablement, run: run, staticLoss: append([]ProviderLoss(nil), loss...),
		state: &huaweiProviderState{children: map[uint8]*huaweiChildState{}, gateway: ProviderGatewayIdentity{State: ProviderGatewayNotIdentified}},
	}, nil
}

func (p *huaweiProviderCore) start(session ProviderSession) error {
	if p == nil || session == nil {
		return ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrHuaweiProviderClosed
	}
	if p.closing {
		return ErrHuaweiProviderClosing
	}
	if p.started {
		return ErrHuaweiProviderStarted
	}
	p.bindLocked(session)
	return nil
}

// bindLocked starts a new connection generation. The gateway identity must be
// read again on it before any child reading is taken.
func (p *huaweiProviderCore) bindLocked(session ProviderSession) {
	p.session, p.started = session, true
	p.generation++
	p.link = huaweiLinkState{}
	p.state.identified = false
	p.state.reidentify = false
}

func (p *huaweiProviderCore) stop() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrHuaweiProviderClosed
	}
	if p.closing {
		p.mu.Unlock()
		return ErrHuaweiProviderClosing
	}
	if !p.started {
		p.mu.Unlock()
		return ErrHuaweiProviderNotStarted
	}
	session := p.unbindLocked()
	p.closing = true
	p.mu.Unlock()
	err := closeHuaweiProviderSession(session)
	p.mu.Lock()
	p.closing = false
	p.mu.Unlock()
	return err
}

// unbindLocked detaches the current connection and fences any running cycle.
// The caller closes the returned session after releasing the lock, so a Close
// that calls back into the provider cannot deadlock.
func (p *huaweiProviderCore) unbindLocked() ProviderSession {
	session := p.session
	p.session, p.started = nil, false
	p.generation++
	p.state.identified = false
	return session
}

func closeHuaweiProviderSession(session ProviderSession) error {
	if session == nil {
		return nil
	}
	return session.Close()
}

// reconnect closes the old session before the new one is bound: while the old
// Close runs the provider is stopped, so no cycle can read through the new
// session until the old connection is released. The new session is bound
// whether or not the old Close failed; that error is returned.
func (p *huaweiProviderCore) reconnect(session ProviderSession) error {
	if p == nil || session == nil {
		return ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrHuaweiProviderClosed
	}
	if p.closing {
		p.mu.Unlock()
		return ErrHuaweiProviderClosing
	}
	var old ProviderSession
	if p.started {
		old = p.unbindLocked()
	}
	p.closing = true
	p.mu.Unlock()
	closeErr := closeHuaweiProviderSession(old)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closing = false
	if p.closed {
		return ErrHuaweiProviderClosed
	}
	if p.started {
		return ErrHuaweiProviderStarted
	}
	p.bindLocked(session)
	return closeErr
}

func (p *huaweiProviderCore) close() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	var old ProviderSession
	if p.started {
		old = p.unbindLocked()
	} else {
		p.generation++
	}
	p.mu.Unlock()
	return closeHuaweiProviderSession(old)
}

func (p *huaweiProviderCore) lastComplete() (ProviderCycle, bool) {
	if p == nil {
		return ProviderCycle{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last == nil {
		return ProviderCycle{}, false
	}
	return p.last.clone(), true
}

func (p *huaweiProviderCore) nextRefreshAt() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nextAt
}

func (p *huaweiProviderCore) fenced(generation uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed || p.generation != generation
}

// refresh runs one cycle. A concurrent refresh is refused without a request.
// No lock is held while the session is called. Stop, reconnect and close fence
// the running cycle: it sends nothing further and commits nothing.
func (p *huaweiProviderCore) refresh(ctx context.Context) (ProviderCycle, error) {
	if p == nil || ctx == nil {
		return ProviderCycle{}, ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	switch {
	case p.closed:
		p.mu.Unlock()
		return ProviderCycle{}, ErrHuaweiProviderClosed
	case p.busy:
		p.mu.Unlock()
		return ProviderCycle{}, ErrHuaweiProviderBusy
	case !p.started:
		p.mu.Unlock()
		return ProviderCycle{}, ErrHuaweiProviderNotStarted
	}
	now := p.clock()
	if !p.nextAt.IsZero() && now.Before(p.nextAt) {
		p.mu.Unlock()
		return ProviderCycle{}, ErrHuaweiProviderEarly
	}
	if err := ctx.Err(); err != nil {
		p.mu.Unlock()
		return ProviderCycle{}, err
	}
	p.busy = true
	p.cycle++
	run := &huaweiCycleRun{
		core: p, ctx: ctx, session: p.session, generation: p.generation, cycle: p.cycle, startedAt: now,
		link: p.link, state: p.state.clone(),
		out: ProviderCycle{ProviderID: p.providerID, Path: p.path, Source: p.source, Cycle: p.cycle, Generation: p.generation, StartedAt: now},
	}
	p.mu.Unlock()

	p.run(run)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.busy = false
	if p.closed {
		return ProviderCycle{}, ErrHuaweiProviderClosed
	}
	if p.generation != run.generation {
		// A fenced cycle commits no reading, but a group refused with
		// exception 3 stays refused for the next generation.
		p.carryRefusals(run.state)
		return ProviderCycle{}, ErrHuaweiProviderFenced
	}
	p.link = run.link
	end := p.clock()
	interval := p.pacing.Cycle
	if elapsed := end.Sub(now); elapsed*4 > interval*3 {
		interval = (elapsed*4 + 2) / 3
	}
	interval += time.Duration(run.waitCycles) * p.pacing.Cycle
	p.nextAt = now.Add(interval)
	if run.err != nil {
		// A cancelled cycle or a pacing violation commits no reading, but a
		// group refused with exception 3 stays refused.
		p.carryRefusals(run.state)
		return ProviderCycle{}, run.err
	}
	run.out.CompletedAt = end
	run.out.Loss = append(append([]ProviderLoss(nil), p.staticLoss...), run.out.Loss...)
	p.state = run.state
	result := run.out.clone()
	if result.Outcome == ProviderCycleComplete {
		kept := result.clone()
		p.last = &kept
	}
	return result, nil
}

// carryRefusals keeps the exception 3 refusals of an uncommitted cycle.
func (p *huaweiProviderCore) carryRefusals(from *huaweiProviderState) {
	for unit, child := range from.children {
		for key := range child.refused {
			if kept, ok := p.state.children[unit]; ok {
				kept.refused[key] = true
				continue
			}
			if p.state.carriedRefusals == nil {
				p.state.carriedRefusals = map[uint8]map[string]bool{}
			}
			if p.state.carriedRefusals[unit] == nil {
				p.state.carriedRefusals[unit] = map[string]bool{}
			}
			p.state.carriedRefusals[unit][key] = true
		}
	}
}

// ---- one cycle ---------------------------------------------------------------

type huaweiReadStatus uint8

const (
	huaweiReadAnswered huaweiReadStatus = iota + 1
	huaweiReadException
	huaweiReadFailed
	huaweiReadStopped
)

type huaweiReadResult struct {
	status        huaweiReadStatus
	exceptionCode uint8
	// decodedException reports that the reply was decoded as a definitive
	// exception before a stop or cancellation ended the read.
	decodedException bool
	// last is the outcome of the last attempt of a failed read.
	last ProviderExchangeOutcome
	// overDeadline reports that the read gave up because the budget set on the
	// cycle run would have been exceeded.
	overDeadline bool
}

type huaweiCycleRun struct {
	core       *huaweiProviderCore
	ctx        context.Context
	session    ProviderSession
	generation uint64
	cycle      uint64
	startedAt  time.Time
	link       huaweiLinkState
	state      *huaweiProviderState
	out        ProviderCycle
	// waitCycles is the number of further cycles to skip before the next one.
	waitCycles int
	// budgetEnd, when set, bounds every wait and attempt of the reads made
	// while it is set; no request starts or runs past it.
	budgetEnd time.Time
	err       error
}

func (r *huaweiCycleRun) stopped() bool { return r.err != nil }

// exchange performs one read under the pacing contract: the gap or backoff is
// checked against the clock before every attempt, each attempt has the
// contract deadline, a failed attempt is repeated at most MaxAttempts-1 times
// and an exception other than 5, 6 or 11 is a definitive reply.
func (r *huaweiCycleRun) exchange(record ProviderExchange, send func(context.Context) (ProviderExchangeOutcome, uint8)) huaweiReadResult {
	if r.stopped() {
		return huaweiReadResult{status: huaweiReadStopped}
	}
	pacing := r.core.pacing
	r.out.PlannedReads++
	first := !r.link.sent
	tolerated := false
	failed := 0
	for {
		if r.core.fenced(r.generation) {
			r.err = ErrHuaweiProviderFenced
			return huaweiReadResult{status: huaweiReadStopped}
		}
		spacing := pacing.Gap
		if failed > 0 && failed-1 < len(pacing.Backoff) && pacing.Backoff[failed-1] > spacing {
			spacing = pacing.Backoff[failed-1]
		}
		if r.link.sent {
			required := r.link.lastEnd.Add(spacing)
			if !r.budgetEnd.IsZero() && !required.Before(r.budgetEnd) {
				return huaweiReadResult{status: huaweiReadFailed, last: ProviderExchangeTimeout, overDeadline: true}
			}
			if now := r.core.clock(); now.Before(required) {
				if err := r.core.wait(r.ctx, required.Sub(now)); err != nil {
					r.err = err
					return huaweiReadResult{status: huaweiReadStopped}
				}
				if r.core.clock().Before(required) {
					r.err = ErrHuaweiProviderPacing
					return huaweiReadResult{status: huaweiReadStopped}
				}
			}
		}
		if err := r.ctx.Err(); err != nil {
			r.err = err
			return huaweiReadResult{status: huaweiReadStopped}
		}
		if r.core.fenced(r.generation) {
			r.err = ErrHuaweiProviderFenced
			return huaweiReadResult{status: huaweiReadStopped}
		}
		attemptDeadline := pacing.AttemptDeadline
		if !r.budgetEnd.IsZero() {
			remaining := r.budgetEnd.Sub(r.core.clock())
			if remaining <= 0 {
				return huaweiReadResult{status: huaweiReadFailed, last: ProviderExchangeTimeout, overDeadline: true}
			}
			if remaining < attemptDeadline {
				attemptDeadline = remaining
			}
		}
		attemptCtx, cancel := context.WithTimeout(r.ctx, attemptDeadline)
		sent := r.core.clock()
		outcome, code := send(attemptCtx)
		end := r.core.clock()
		// A reply that arrives after the attempt deadline is not current: the
		// attempt context is read before it is cancelled.
		if (outcome == ProviderExchangeAnswered || outcome == ProviderExchangeException) && (errors.Is(attemptCtx.Err(), context.DeadlineExceeded) || end.Sub(sent) > attemptDeadline) {
			outcome, code = ProviderExchangeTimeout, 0
		}
		cancel()
		r.link.sent, r.link.lastEnd = true, end
		r.out.WireAttempts++
		attempt := record
		attempt.Attempt, attempt.Outcome, attempt.ExceptionCode, attempt.SentAt = failed+1, outcome, code, sent
		switch {
		case outcome == ProviderExchangeTimeout:
			attempt.Elapsed = ProviderAtDeadline
		case end.Sub(sent) <= pacing.PromptBound:
			attempt.Elapsed = ProviderWithinPromptBound
		default:
			attempt.Elapsed = ProviderWithinDeadline
		}
		if first && !tolerated && pacing.FirstRequestExtraAttempt && outcome == ProviderExchangeTimeout && failed == 0 {
			tolerated = true
			attempt.Elapsed = ProviderFirstRequestTolerated
			r.out.Exchanges = append(r.out.Exchanges, attempt)
			if err := r.ctx.Err(); err != nil {
				r.err = err
				return huaweiReadResult{status: huaweiReadStopped}
			}
			continue
		}
		r.out.Exchanges = append(r.out.Exchanges, attempt)
		if err := r.ctx.Err(); err != nil {
			r.err = err
			// The reply was decoded before the cancellation: a definitive
			// exception is handed to the caller so it is recorded.
			stopped := huaweiReadResult{status: huaweiReadStopped}
			if outcome == ProviderExchangeException && code != 5 && code != 6 && code != 11 {
				stopped.exceptionCode, stopped.decodedException = code, true
			}
			return stopped
		}
		switch outcome {
		case ProviderExchangeAnswered:
			return huaweiReadResult{status: huaweiReadAnswered}
		case ProviderExchangeException:
			if code != 5 && code != 6 && code != 11 {
				return huaweiReadResult{status: huaweiReadException, exceptionCode: code}
			}
		}
		failed++
		if failed >= pacing.MaxAttempts {
			return huaweiReadResult{status: huaweiReadFailed, last: outcome}
		}
	}
}

func classifyHuaweiProviderError(attemptCtx context.Context, err error) (ProviderExchangeOutcome, uint8) {
	var protocol *modbus.ProtocolError
	if errors.As(err, &protocol) {
		if protocol.Kind == modbus.ErrorExceptionResponse && protocol.ExceptionCode != 0 {
			return ProviderExchangeException, protocol.ExceptionCode
		}
		return ProviderExchangeMalformed, 0
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		return ProviderExchangeTimeout, 0
	}
	return ProviderExchangeTransport, 0
}

// readHolding reads one FC03 extent.
func (r *huaweiCycleRun) readHolding(unit uint8, offset, quantity uint16) ([]uint16, huaweiReadResult) {
	request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadHoldingRegisters, offset, quantity)
	if err != nil {
		r.err = ErrHuaweiProviderConfig
		return nil, huaweiReadResult{status: huaweiReadStopped}
	}
	want := modbus.ReadProvenance{Function: modbus.FunctionReadHoldingRegisters, Table: modbus.HoldingRegisters, Offset: offset, Quantity: quantity}
	var words []uint16
	result := r.exchange(ProviderExchange{Function: uint8(modbus.FunctionReadHoldingRegisters), UnitID: unit, Offset: offset, Quantity: quantity},
		func(attemptCtx context.Context) (ProviderExchangeOutcome, uint8) {
			response, err := r.session.ReadHolding(attemptCtx, unit, request)
			if err != nil {
				return classifyHuaweiProviderError(attemptCtx, err)
			}
			if response.Provenance != want || len(response.Words) != int(quantity) {
				return ProviderExchangeMalformed, 0
			}
			words = append([]uint16(nil), response.Words...)
			return ProviderExchangeAnswered, 0
		})
	if result.status != huaweiReadAnswered {
		return nil, result
	}
	return words, result
}

// readIdentification reads one Read Device Identification page.
func (r *huaweiCycleRun) readIdentification(unit uint8, request IdentificationRequest) (IdentificationPage, huaweiReadResult) {
	var page IdentificationPage
	result := r.exchange(ProviderExchange{Function: uint8(modbus.FunctionEncapsulatedInterface), UnitID: unit, ReadDeviceIDCode: request.ReadDeviceIDCode, ObjectID: request.ObjectID},
		func(attemptCtx context.Context) (ProviderExchangeOutcome, uint8) {
			reply, err := r.session.ReadDeviceIdentification(attemptCtx, unit, request)
			if err != nil {
				return classifyHuaweiProviderError(attemptCtx, err)
			}
			if reply.ReadDeviceIDCode != request.ReadDeviceIDCode || len(reply.Objects) == 0 {
				return ProviderExchangeMalformed, 0
			}
			page = reply.clone()
			return ProviderExchangeAnswered, 0
		})
	if result.status != huaweiReadAnswered {
		return IdentificationPage{}, result
	}
	return page, result
}

func huaweiWordsToBytes(words []uint16) []byte {
	out := make([]byte, 0, 2*len(words))
	for _, word := range words {
		out = append(out, byte(word>>8), byte(word))
	}
	return out
}

// ---- child admission -----------------------------------------------------------

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

// childAdmissionRead handles one admission read of a child and reports
// whether it answered. A read that failed leaves the child unadmitted: a
// reading of an earlier identity stays as stale history, nothing is guessed,
// and the admission runs again on a later cycle after the child back-off. An
// exception withholds the child until the next identification event. A stop
// adds nothing; the caller checks it.
func (r *huaweiCycleRun) childAdmissionRead(child *huaweiChildState, result huaweiReadResult) bool {
	switch result.status {
	case huaweiReadAnswered:
		return true
	case huaweiReadFailed:
		child.failures++
		child.skipUntil = huaweiChildSkipUntil(r.cycle, child.failures)
		reason := ProviderReasonChildReadFailed
		if result.last == ProviderExchangeTimeout {
			reason = ProviderReasonChildTimeout
		}
		r.out.Children = append(r.out.Children, r.childObservation(child, huaweiStaleOrWithheld(child), reason, nil))
	case huaweiReadException:
		child.withhold(ProviderReasonIdentityInsuff, nil)
		r.out.Children = append(r.out.Children, r.childObservation(child, ProviderEvidenceWithheld, child.definitive, nil))
	}
	return false
}

// readChildIdentityWords reads the model identifier, the software text and the
// firmware text at the child's routed unit, one request at a time under the
// path's pacing. It reports false when a read did not answer; the child's
// observation is then already added, or the cycle was stopped.
func (r *huaweiCycleRun) readChildIdentityWords(child *huaweiChildState) (SUN2000RoutedIdentity, bool) {
	identifier, result := r.readHolding(child.unit, huaweiSUN2000IdentifierOffset, 1)
	if !r.childAdmissionRead(child, result) {
		return SUN2000RoutedIdentity{}, false
	}
	software, result := r.readHolding(child.unit, huaweiSUN2000SoftwareOffset, huaweiSUN2000TextQuantity)
	if !r.childAdmissionRead(child, result) {
		return SUN2000RoutedIdentity{}, false
	}
	firmware, result := r.readHolding(child.unit, huaweiSUN2000FirmwareOffset, huaweiSUN2000TextQuantity)
	if !r.childAdmissionRead(child, result) {
		return SUN2000RoutedIdentity{}, false
	}
	return SUN2000RoutedIdentity{
		IdentifierWords: identifier, SoftwareBlock: huaweiWordsToBytes(software), FirmwareBlock: huaweiWordsToBytes(firmware),
	}, true
}

// bindChildSelection admits the child on the profile of a selected catalog
// row, recording the decision. A changed identity fences the earlier reading.
// Anything else withholds the child until the next identification event: a
// decision that selected no row with its closed reason, an ambiguous decision
// with every candidate, a selected row whose family has no profile on this
// path, a model that disagrees with the listing, or insufficient evidence.
func (r *huaweiCycleRun) bindChildSelection(child *huaweiChildState, selection SUN2000CapabilitySelection) bool {
	var decision *SUN2000CatalogDecision
	if taken, ok := selection.Catalog(); ok {
		decision = &taken
	}
	profile, ok := selection.Profile()
	if selection.Outcome() == SUN2000ProfileSelected && ok && decision != nil && decision.Outcome() == SUN2000CatalogSelected {
		identity := &huaweiChildIdentity{profile: profile, model: profile.Model()}
		if child.identity != nil && child.identity.key() != identity.key() {
			child.fence()
		}
		child.identity, child.identifiedGeneration, child.definitive, child.catalog = identity, r.generation, "", decision
		return true
	}
	reason := ProviderReasonIdentityInsuff
	switch {
	case selection.Reason() == SUN2000ReasonCatalogAmbiguous:
		reason = ProviderReasonCatalogAmbiguous
	case selection.Reason() == SUN2000ReasonCatalogNotSelected:
		reason = ProviderReasonCatalogNotSelected
	case selection.Outcome() == SUN2000NoAdmissibleProfile:
		reason = ProviderReasonNoProfile
	case selection.Reason() == SUN2000ReasonListingModelConflict:
		reason = ProviderReasonIdentityMismatch
	}
	child.withhold(reason, decision)
	r.out.Children = append(r.out.Children, r.childObservation(child, ProviderEvidenceWithheld, reason, nil))
	return false
}

// huaweiChildBackoffCycles is the registry's back-off of one child after
// consecutive failures: none after the first, then doubling up to ten cycles.
// huaweiChildSkipUntil returns the first cycle at which a failed child is read
// again; the skip check is strict, so the backoff length is skipped in full.
func huaweiChildSkipUntil(cycle uint64, failures int) uint64 {
	wait := huaweiChildBackoffCycles(failures)
	if wait == 0 {
		return cycle
	}
	return cycle + wait + 1
}

func huaweiChildBackoffCycles(failures int) uint64 {
	if failures < 2 {
		return 0
	}
	wait := uint64(1)
	for step := 2; step < failures && wait < 10; step++ {
		wait *= 2
	}
	if wait > 10 {
		wait = 10
	}
	return wait
}

// huaweiEndpointBackoffCycles doubles from one cycle up to ten cycles.
func huaweiEndpointBackoffCycles(failures int) int {
	wait := 1
	for step := 1; step < failures && wait < 10; step++ {
		wait *= 2
	}
	if wait > 10 {
		wait = 10
	}
	return wait
}

// monitorChild reads the plan of an identified child's profile, skipping the
// groups refused at runtime, decodes it with the profile and retains the last
// complete reading of each group through RetainHuaweiSUN2000Monitoring. An
// exception reply is recorded and the next group is read; a failed read ends
// the child's cycle.
func (r *huaweiCycleRun) monitorChild(child *huaweiChildState) (ProviderEvidenceState, ProviderReason, map[uint16]SUN2000GroupReading) {
	profile := child.identity.profile
	current := map[uint16]SUN2000GroupReading{}
	var readings []SUN2000GroupReading
	state, reason := ProviderEvidenceCurrent, ProviderReasonRead
	readAt := map[uint16]time.Time{}
	for _, step := range profile.Plan(child.unit) {
		if child.refused[huaweiRefusalKey(profile.ID(), step.Offset, step.Quantity)] {
			continue
		}
		words, result := r.readHolding(child.unit, step.Offset, step.Quantity)
		reading := SUN2000GroupReading{Offset: step.Offset, Quantity: step.Quantity}
		switch result.status {
		case huaweiReadAnswered:
			reading.Outcome, reading.Words, reading.Cycle = SUN2000GroupAnswered, words, r.cycle
			readAt[step.Offset] = r.link.lastEnd
		case huaweiReadException:
			reading.Outcome, reading.ExceptionCode = SUN2000GroupException, result.exceptionCode
			if result.exceptionCode == huaweiSUN2000RefusalCode {
				child.refused[huaweiRefusalKey(profile.ID(), step.Offset, step.Quantity)] = true
			}
			state, reason = ProviderEvidenceStale, ProviderReasonChildException
		case huaweiReadFailed:
			reading.Outcome = SUN2000GroupFailed
			state, reason = ProviderEvidenceStale, ProviderReasonChildReadFailed
			if result.last == ProviderExchangeTimeout {
				reason = ProviderReasonChildTimeout
			}
		default:
			// Stopped: a refusal decoded before the stop is still recorded.
			if result.decodedException && result.exceptionCode == huaweiSUN2000RefusalCode {
				child.refused[huaweiRefusalKey(profile.ID(), step.Offset, step.Quantity)] = true
			}
			return "", "", nil
		}
		current[step.Offset] = reading
		readings = append(readings, reading)
		if result.status == huaweiReadFailed {
			break
		}
	}
	decoded, err := profile.Decode(child.unit, readings)
	if err != nil {
		return ProviderEvidenceStale, ProviderReasonChildReadFailed, current
	}
	retained := RetainHuaweiSUN2000Monitoring(child.reading, decoded)
	for offset, at := range readAt {
		child.groups[offset] = huaweiGroupMeta{cycle: r.cycle, readAt: at, generation: r.generation}
	}
	child.reading = retained
	if state == ProviderEvidenceCurrent {
		child.failures, child.skipUntil = 0, 0
	} else if reason != ProviderReasonChildException {
		child.failures++
		child.skipUntil = huaweiChildSkipUntil(r.cycle, child.failures)
	}
	return state, reason, current
}

// childObservation renders one child for the cycle output.
func (r *huaweiCycleRun) childObservation(child *huaweiChildState, state ProviderEvidenceState, reason ProviderReason, current map[uint16]SUN2000GroupReading) ProviderChildObservation {
	out := ProviderChildObservation{
		UnitID: child.unit, ListedClass: child.listedClass, Evidence: state, Reason: reason, Fenced: child.fenced.clone(),
		Catalog: huaweiCloneCatalogDecision(child.catalog),
	}
	if child.identity != nil {
		out.Family, out.ProfileID, out.Model = child.identity.profile.Family(), child.identity.profile.ID(), child.identity.model
		out.IdentifiedGeneration = child.identifiedGeneration
	}
	if state != ProviderEvidenceWithheld && child.reading.ProfileID() != "" && child.identity != nil {
		if current == nil {
			// Not read this cycle: every kept group is retained with its
			// original cycle, through the same retention as a failed read.
			current = map[uint16]SUN2000GroupReading{}
			if empty, err := child.identity.profile.Decode(child.unit, nil); err == nil {
				view := *child
				view.reading = RetainHuaweiSUN2000Monitoring(child.reading, empty)
				out.Reading, out.Groups = view.reading, view.groupEvidence(current)
				return r.recordChildLoss(out, state, reason)
			}
		}
		out.Reading = child.reading
		out.Groups = child.groupEvidence(current)
	}
	return r.recordChildLoss(out, state, reason)
}

func (r *huaweiCycleRun) recordChildLoss(out ProviderChildObservation, state ProviderEvidenceState, reason ProviderReason) ProviderChildObservation {
	if state == ProviderEvidenceWithheld {
		r.out.Loss = append(r.out.Loss, ProviderLoss{Kind: ProviderLossWithheldChild, HasUnit: true, UnitID: out.UnitID, Detail: huaweiWithheldDetail(reason, out.Catalog)})
	}
	for _, group := range out.Groups {
		if group.Refused {
			r.out.Loss = append(r.out.Loss, ProviderLoss{Kind: ProviderLossRefusedGroup, HasUnit: true, UnitID: out.UnitID,
				Offset: group.Offset, Quantity: group.Quantity, Detail: "refused with exception 3 at runtime; not sent again"})
		}
	}
	return out
}

// huaweiWithheldDetail names why a child is withheld. A catalog withhold
// adds the closed reason, and an ambiguous one names every candidate row.
func huaweiWithheldDetail(reason ProviderReason, decision *SUN2000CatalogDecision) string {
	detail := string(reason)
	if decision == nil {
		return detail
	}
	switch reason {
	case ProviderReasonCatalogNotSelected:
		detail += ": " + string(decision.Reason())
	case ProviderReasonCatalogAmbiguous:
		detail += ":"
		for index, row := range decision.Candidates() {
			if index > 0 {
				detail += ","
			}
			detail += " " + row.Card + " " + strconv.Itoa(row.Identifier)
		}
	}
	return detail
}

// staleChildren renders every identified child as stale for a cycle whose
// children were not read, keeping their retained readings with original age.
func (r *huaweiCycleRun) staleChildren(reason ProviderReason, withheld bool) {
	units := make([]int, 0, len(r.state.children))
	for unit := range r.state.children {
		units = append(units, int(unit))
	}
	sort.Ints(units)
	for _, unit := range units {
		child := r.state.children[uint8(unit)]
		state, why := ProviderEvidenceStale, reason
		if r.state.gatewayFenced {
			// A gateway mismatch holds until the identity is read again and
			// matches; a later read failure never releases the readings.
			withheld, why = true, ProviderReasonGatewayMismatch
		}
		if withheld || child.identity == nil || child.reading.ProfileID() == "" || child.ineligible != "" {
			state = ProviderEvidenceWithheld
		}
		if child.ineligible != "" && !withheld && child.reading.ProfileID() != "" {
			why = child.ineligible
		}
		r.out.Children = append(r.out.Children, r.childObservation(child, state, why, nil))
	}
}

// huaweiChildProfileLoss names what a child profile never publishes.
func huaweiChildProfileLoss(profile SUN2000MonitoringProfile) []ProviderLoss {
	var loss []ProviderLoss
	for _, refused := range profile.RefusedGroups() {
		loss = append(loss, ProviderLoss{Kind: ProviderLossRefusedGroup, Offset: refused.Offset, Quantity: refused.Quantity,
			Detail: profile.Family() + ": refused with exception " + strconv.Itoa(int(refused.ExceptionCode)) + "; never read"})
	}
	loss = append(loss,
		ProviderLoss{Kind: ProviderLossUnconsumedWords, Offset: huaweiSUN2000StatusGroup.offset, Quantity: huaweiSUN2000StatusGroup.quantity,
			Detail: "status group retained as native evidence; no state is published from it"},
		ProviderLoss{Kind: ProviderLossUnconsumedWords, Offset: 32032, Quantity: 8, Detail: "string words past string 8"},
		ProviderLoss{Kind: ProviderLossUnconsumedWords, Offset: 32108, Quantity: 4, Detail: "DC input energy counter and statistics time"},
		ProviderLoss{Kind: ProviderLossBattery, Detail: "battery offsets are never read; a zero word is never a battery value"},
		ProviderLoss{Kind: ProviderLossMeter, Detail: "every meter value stays Unknown; no meter class contract"},
	)
	if profile.ID() == SUN2000M1MonitoringProfileID {
		loss = append(loss, ProviderLoss{Kind: ProviderLossUnconsumedWords, Offset: huaweiSUN2000PowerGroup.offset, Quantity: huaweiSUN2000PowerGroup.quantity,
			Detail: "power group words other than input power, active power and the observed status code"})
	}
	return loss
}

func huaweiProfileByID(id string) (SUN2000MonitoringProfile, bool) {
	for _, profile := range SUN2000MonitoringProfiles() {
		if profile.ID() == id {
			return profile, true
		}
	}
	return SUN2000MonitoringProfile{}, false
}
