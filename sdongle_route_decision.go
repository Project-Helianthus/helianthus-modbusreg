package modbusreg

import "time"

// SDongleRouteOutcome is documentary only; none of its values authorizes I/O.
type SDongleRouteOutcome string

const (
	SDonglePlanned  SDongleRouteOutcome = "planned"
	SDongleNoSend   SDongleRouteOutcome = "no-send"
	SDongleWithheld SDongleRouteOutcome = "withheld"
)

type SDongleRouteRole string
type SDongleRequestClass string

const (
	SDongleGateway         SDongleRouteRole    = "gateway"
	SDongleChild           SDongleRouteRole    = "child"
	SDongleBasicMEI        SDongleRequestClass = "basic-mei"
	SDongleGatewayFC03     SDongleRequestClass = "gateway-fc03"
	SDongleExtendedMEI     SDongleRequestClass = "extended-mei"
	SDonglePrivateFunction SDongleRequestClass = "private-function"
)

// SDongleRouteSelection retains native discriminators, never a wire request.
// For a child, Family is the routing family; Model and Firmware are the child's
// independently evidenced identity, not inherited gateway identity.
type SDongleRouteSelection struct {
	Family        string
	Model         string
	Firmware      string
	ProtocolMajor uint8
	ProtocolMinor uint8
	Role          SDongleRouteRole
	Unit          uint8
	Request       SDongleRequestClass
	Scope         string
	Revision      string
}

// SDongleChildRoute binds an independently qualified child to its parent and
// inventory guard. A zero child context is required for gateway decisions.
type SDongleChildRoute struct {
	Parent           SDongleRouteSelection
	EvidenceRevision string
	Qualification    string
	GuardBefore      uint16
	GuardAfter       uint16
}

type SDongleRouteKey struct {
	Family        string
	Model         string
	Firmware      string
	ProtocolMajor uint8
	ProtocolMinor uint8
	Role          SDongleRouteRole
	Unit          uint8
	Request       SDongleRequestClass
	Scope         string
	Revision      string
	Child         SDongleChildRoute
}

func (k SDongleRouteKey) selection() SDongleRouteSelection {
	return SDongleRouteSelection{Family: k.Family, Model: k.Model, Firmware: k.Firmware, ProtocolMajor: k.ProtocolMajor, ProtocolMinor: k.ProtocolMinor, Role: k.Role, Unit: k.Unit, Request: k.Request, Scope: k.Scope, Revision: k.Revision}
}

// SDongleRouteReference associates an opaque source revision with native evidence.
type SDongleRouteReference struct {
	Revision  string
	Reference string
}

// SDongleRouteEvidence is supplied explicitly by offline composition. Current is
// a caller assertion, not a clock-based freshness check. Conflict includes any
// competing eligible family. Key is the original association of this evidence.
type SDongleRouteEvidence struct {
	Key        SDongleRouteKey
	Current    bool
	Conflict   bool
	Identity   SDongleRouteReference
	Protocol   SDongleRouteReference
	Allowlist  SDongleRouteReference
	Route      SDongleRouteReference
	References []string
}

// SDongleRouteBounds describes finite documentary limits, not retry authority.
// RetryBounded distinguishes an explicit zero-retry limit from absent evidence.
type SDongleRouteBounds struct {
	Deadline     time.Duration
	RetryBounded bool
	MaxRetries   uint16
	MaxPages     uint16
	MaxObjects   uint16
	MaxBytes     uint32
}

type SDongleRoutePlan struct {
	Key                  SDongleRouteKey
	Observation          HuaweiSDongleOfflineObservation
	Evidence             SDongleRouteEvidence
	Bounds               SDongleRouteBounds
	TimedOut             bool
	QualificationStopped bool
}

// SDongleRouteDecision preserves even rejected evidence as detached historical
// data. It contains no operation, encoded request, profile registration or send.
type SDongleRouteDecision struct {
	Outcome         SDongleRouteOutcome
	Reason          string
	Key             SDongleRouteKey
	Observation     HuaweiSDongleOfflineObservation
	Evidence        SDongleRouteEvidence
	Bounds          SDongleRouteBounds
	EvaluatorReason HuaweiSDongleOfflineReason
}

func (SDongleRouteDecision) DefaultDenied() bool { return true }

// DecideSDongleRoute evaluates a single offline documentary decision. Known
// prohibitions take precedence; incomplete or incoherent evidence cannot plan.
// It neither refreshes evidence nor changes the existing evaluator's tuple set.
func DecideSDongleRoute(p SDongleRoutePlan) SDongleRouteDecision {
	evaluated := EvaluateHuaweiSDongleOfflineObservation(p.Observation)
	d := SDongleRouteDecision{Outcome: SDongleWithheld, Reason: "insufficient_evidence", Key: p.Key, Observation: p.Observation, Evidence: p.Evidence, Bounds: p.Bounds, EvaluatorReason: evaluated.Reason()}
	if p.Evidence.References != nil {
		d.Evidence.References = append([]string{}, p.Evidence.References...)
	}
	k := p.Key
	if p.TimedOut || p.QualificationStopped || k.Request == SDonglePrivateFunction || (k.Request == SDongleExtendedMEI && (k.Role == SDongleGateway || k.Unit == 100)) {
		d.Outcome = SDongleNoSend
		d.Reason = "prohibited_or_stopped"
		return d
	}
	if !evaluated.Matched() {
		d.Reason = "offline_observation_rejected"
		return d
	}
	// The evaluator's broader catalog is deliberately not the routing catalog.
	if trimHuaweiTerminalPadding(p.Observation.Firmware) != "V200R025C00SPC120" {
		d.Reason = "documentary_tuple"
		return d
	}
	e := p.Evidence
	if !e.Current || e.Conflict || e.Key != k || k.Family != HuaweiSDongleCanonicalClass || k.Model == "" || k.Firmware == "" || k.ProtocolMajor != 5 || k.ProtocolMinor != 0 || k.Revision == "" || k.Scope == "" || k.Request == "" {
		return d
	}
	for _, r := range []SDongleRouteReference{e.Identity, e.Protocol, e.Allowlist} {
		if r.Reference == "" || r.Revision != k.Revision {
			return d
		}
	}
	if len(e.References) == 0 {
		return d
	}
	for _, r := range e.References {
		if r == "" {
			return d
		}
	}
	b := p.Bounds
	if b.Deadline <= 0 || b.Deadline > 15*time.Second || !b.RetryBounded || b.MaxPages == 0 || b.MaxObjects == 0 || b.MaxBytes == 0 {
		d.Reason = "missing_or_excessive_bounds"
		return d
	}
	switch k.Role {
	case SDongleGateway:
		if !sdongleGatewaySelection(k.selection(), p.Observation) || k.Child != (SDongleChildRoute{}) {
			return d
		}
	case SDongleChild:
		c := k.Child
		if k.Unit == 0 || k.Unit > 247 || k.Unit == 100 || !sdongleGatewaySelection(c.Parent, p.Observation) || !sdongleAllowedScope(c.Parent.Role, c.Parent.Request, c.Parent.Scope) || c.Parent.Revision != k.Revision || c.EvidenceRevision != k.Revision || c.Qualification != "qualified" || e.Route.Reference == "" || e.Route.Revision != k.Revision || e.Route.Reference == e.Identity.Reference || c.GuardBefore != c.GuardAfter || c.GuardBefore != p.Observation.ChangeSequenceBefore || b.MaxPages > 121 || b.MaxObjects > 121 || b.MaxBytes > 32768 {
			return d
		}
	default:
		return d
	}
	if !sdongleAllowedScope(k.Role, k.Request, k.Scope) {
		d.Outcome = SDongleNoSend
		d.Reason = "outside_explicit_allowlist"
		return d
	}
	d.Outcome = SDonglePlanned
	d.Reason = "documentary_request_class"
	return d
}

func sdongleGatewaySelection(k SDongleRouteSelection, o HuaweiSDongleOfflineObservation) bool {
	return k.Family == HuaweiSDongleCanonicalClass && k.Model == trimHuaweiTerminalPadding(o.Model) && k.Firmware == trimHuaweiTerminalPadding(o.Firmware) && k.ProtocolMajor == o.ProtocolMajor && k.ProtocolMinor == o.ProtocolMinor && k.Role == SDongleGateway && k.Unit == 100 && k.Revision != ""
}

func sdongleAllowedScope(role SDongleRouteRole, request SDongleRequestClass, scope string) bool {
	if role == SDongleChild {
		return request == SDongleExtendedMEI && scope == "extended-inventory"
	}
	if role != SDongleGateway {
		return false
	}
	switch request {
	case SDongleBasicMEI:
		return scope == "basic-identity"
	case SDongleGatewayFC03:
		return scope == "protocol-baseline" || scope == "gateway-status" || scope == "capacity"
	default:
		return false
	}
}
