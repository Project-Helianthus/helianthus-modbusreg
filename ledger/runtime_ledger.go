package ledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
	"unicode/utf8"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// PublishedAttemptV1 is the exact closed external publication projection.
type PublishedAttemptV1 struct {
	SchemaVersion           int    `json:"schema_version"`
	AttemptTerminalSequence uint64 `json:"attempt_terminal_sequence"`
	DependencySetDigest     string `json:"dependency_set_digest"`
	RuntimeDependencyCount  uint64 `json:"runtime_dependency_count"`
	ClaimOutcomeDigest      string `json:"claim_outcome_digest"`
}

// PublicationCommitRequest stages the exact observation, durable sample state,
// terminal watermark, and externally publishable projection in one transaction.
type PublicationCommitRequest struct {
	ExpectedState         SampleLedgerState
	PublishedState        SampleLedgerState
	ExpectedRestartState  RestartState
	PublishedRestartState RestartState
	Attempt               PublishedAttemptV1
	Observation           modbusreg.Observation
	ObservationBytes      []byte
}

// TerminalStateCommitRequest durably records a nonpublished terminal outcome
// and its monotonic restart watermark without creating an external sample.
type TerminalStateCommitRequest struct {
	ExpectedRestartState RestartState
	TerminalRestartState RestartState
	Attempt              PublishedAttemptV1
	Outcome              AttemptPhase
}

// PublicationCommitDecision is the transaction's atomic outcome.
type PublicationCommitDecision string

// The publication commit decisions a PublicationCommitter returns.
const (
	PublicationCommitCommitted PublicationCommitDecision = "committed"
	PublicationCommitCancelled PublicationCommitDecision = "cancelled"
)

// PublicationCommitter atomically persists publication or a nonpublished
// terminal watermark. Publication errors/cancellation guarantee no external
// effect. Terminal targets are idempotent monotonic joins; an error guarantees
// the durable restart state was not changed.
type PublicationCommitter interface {
	CommitPublication(
		context.Context,
		PublicationCommitRequest,
	) (PublicationCommitDecision, error)
	CommitTerminalState(
		context.Context,
		TerminalStateCommitRequest,
	) error
}

// RuntimeObservationFacts are immutable envelope facts not supplied by M1.
type RuntimeObservationFacts struct {
	SourceValidity          modbusreg.SourceValidity
	SourceTime              modbusreg.SourceTimeSpec
	LocalReceiptTime        time.Time
	LocalReceiptTimePresent bool
}

// RuntimeAttemptRequest asks the factory to create and own one exact producer
// attempt before any acquisition is issued.
type RuntimeAttemptRequest struct {
	Source       *modbus.RuntimeAcquisitionSource
	AttemptKey   string
	Identity     modbusreg.AttemptIdentity
	Observation  RuntimeObservationFacts
	Dependencies []modbusreg.RuntimeDependencyFacts
	Diagnostics  []string
}

// ObservationFactory binds production attempts to one profile, ledger, and
// transactional publication boundary.
type ObservationFactory struct {
	profile               modbusreg.ProfileDescriptor
	ledger                *SampleLedger
	committer             PublicationCommitter
	dependencySetDigest   string
	dependencyBytes       []byte
	terminalCommitTimeout time.Duration
}

// DefaultTerminalCommitTimeout bounds every nonpublication terminal commit.
const DefaultTerminalCommitTimeout = 5 * time.Second

// ObservationFactoryOptions controls bounded transactional behavior without
// changing the public attempt lifecycle.
type ObservationFactoryOptions struct {
	TerminalCommitTimeout time.Duration
}

// NewObservationFactory activates production ingestion. Fixture replay uses
// the separate FixtureReplayer and never needs a committer.
func NewObservationFactory(
	profile modbusreg.ProfileDescriptor,
	ledger *SampleLedger,
	committer PublicationCommitter,
) (*ObservationFactory, error) {
	return NewObservationFactoryWithOptions(
		profile,
		ledger,
		committer,
		ObservationFactoryOptions{TerminalCommitTimeout: DefaultTerminalCommitTimeout},
	)
}

// NewObservationFactoryWithOptions activates production ingestion with an
// explicit finite nonpublication terminal-commit deadline.
func NewObservationFactoryWithOptions(
	profile modbusreg.ProfileDescriptor,
	ledger *SampleLedger,
	committer PublicationCommitter,
	options ObservationFactoryOptions,
) (*ObservationFactory, error) {
	if ledger == nil || committer == nil {
		return nil, fmt.Errorf("observation factory requires a ledger and publication committer")
	}
	if options.TerminalCommitTimeout <= 0 || options.TerminalCommitTimeout > time.Minute {
		return nil, fmt.Errorf("terminal commit timeout must be finite and positive")
	}
	copy, err := modbusreg.NewProfileDescriptor(profile.Spec())
	if err != nil {
		return nil, fmt.Errorf("observation factory profile: %w", err)
	}
	if copy.Kind() != modbusreg.ProfileStandardFamily {
		return nil, fmt.Errorf("vendor overlay requires M3 resolution")
	}
	state := ledger.ExportState()
	if state.ProfileID != copy.ID() || state.ProfileVersion != copy.Version() ||
		state.DependencySetID != copy.Dependencies().ID() {
		return nil, fmt.Errorf("observation factory persistence domain disagrees")
	}
	if state.Revision > 0 {
		switch copy.Coherence().Mode {
		case modbusreg.CoherenceSingleWireResponse:
			if state.LastCommittedAttempt.RetryOrdinal != 0 {
				return nil, fmt.Errorf("persisted attempt disagrees with profile mode")
			}
		case modbusreg.CoherenceBoundedMultiResponse:
			if state.LastCommittedAttempt.RetryOrdinal == 0 {
				return nil, fmt.Errorf("persisted attempt disagrees with profile mode")
			}
		default:
			return nil, fmt.Errorf("observation coherence mode is invalid")
		}
	}
	encoded, digest, err := encodeRuntimeDependencySet(
		copy.Dependencies(),
		ledger.limits.MaxDependencySetEncodedBytes,
	)
	if err != nil {
		return nil, err
	}
	return &ObservationFactory{
		profile:               copy,
		ledger:                ledger,
		committer:             committer,
		dependencySetDigest:   digest,
		dependencyBytes:       encoded,
		terminalCommitTimeout: options.TerminalCommitTimeout,
	}, nil
}

func encodeRuntimeDependencySet(
	set modbusreg.DependencySet,
	maximum int,
) ([]byte, string, error) {
	dependencies := set.Dependencies()
	if set.ID() == "" || !set.Version().Valid() || len(dependencies) == 0 || maximum <= 0 {
		return nil, "", fmt.Errorf("runtime dependency set is empty")
	}
	// Canonical v1 encoding is the domain, a big-endian uint64 count, then each
	// ordered dependency ID and version as a big-endian uint64 length plus bytes.
	const domain = "helianthus.modbusreg.runtime-dependent-identities/v1\x00"
	size := len(domain) + 8
	identities := make([][2]string, len(dependencies))
	for index, dependency := range dependencies {
		identities[index] = [2]string{
			dependency.ID(),
			dependency.Version().String(),
		}
		for _, value := range identities[index] {
			if value == "" || size > maximum-8 || len(value) > maximum-size-8 {
				return nil, "", fmt.Errorf("runtime dependency set exceeds encoded byte bound")
			}
			size += 8 + len(value)
		}
	}
	if size > maximum {
		return nil, "", fmt.Errorf("runtime dependency set exceeds encoded byte bound")
	}
	var buffer bytes.Buffer
	buffer.Grow(size)
	buffer.WriteString(domain)
	_ = binary.Write(&buffer, binary.BigEndian, uint64(len(identities)))
	for _, identity := range identities {
		for _, value := range identity {
			_ = binary.Write(&buffer, binary.BigEndian, uint64(len(value)))
			buffer.WriteString(value)
		}
	}
	encoded := buffer.Bytes()
	digest := sha256.Sum256(encoded)
	digestID := "sha256:" + hex.EncodeToString(digest[:])
	return append([]byte(nil), encoded...), digestID, nil
}

type runtimeClaimPhase uint8

const (
	runtimeClaimUnresolved runtimeClaimPhase = iota + 1
	runtimeClaimInProgress
	runtimeClaimTerminal
)

type runtimeClaimEntry struct {
	phase            runtimeClaimPhase
	outcome          ClaimOutcome
	terminalSequence uint64
}

type runtimeAttemptState struct {
	mu                       sync.Mutex
	cond                     *sync.Cond
	factory                  *ObservationFactory
	key                      string
	identity                 modbusreg.AttemptIdentity
	phase                    AttemptPhase
	terminalSequence         uint64
	entries                  []runtimeClaimEntry
	inProgressClaims         int
	source                   *modbus.RuntimeAcquisitionSource
	producerAttempt          *modbus.RuntimeAttempt
	instance                 modbus.RuntimeAttemptInstance
	admitted                 bool
	acquisitions             []modbus.RuntimeAcquisition
	issued                   []bool
	observationFacts         RuntimeObservationFacts
	dependencyFacts          []modbusreg.RuntimeDependencyFacts
	diagnostics              []string
	template                 modbusreg.Observation
	publishCancel            context.CancelFunc
	publishAbort             chan struct{}
	publishAbortClosed       bool
	restartCommitted         bool
	terminalOutcome          AttemptPhase
	terminalCommitInProgress bool
	terminalPersistenceErr   error
	terminalWaiters          int
	sourceDrainMu            sync.Mutex
	sourceDrainComplete      bool
	sourceDrainErr           error
	cancelOpen               func() error
}

// ObservationAttempt is a shared pointer view of one bounded ledger attempt.
type ObservationAttempt struct {
	state *runtimeAttemptState
}

// BeginRuntimeAttempt validates every ledger-owned byte and bound, then begins
// and retains the exact M1 producer attempt before returning its shared M2
// owner. No capability is issued or claimed by this operation.
func (factory *ObservationFactory) BeginRuntimeAttempt(
	request RuntimeAttemptRequest,
) (*ObservationAttempt, error) {
	if factory == nil || factory.ledger == nil || factory.committer == nil ||
		request.Source == nil {
		return nil, fmt.Errorf("runtime attempt request is invalid")
	}
	if err := factory.validateAttemptIdentity(request.Identity); err != nil {
		return nil, err
	}
	if request.AttemptKey == "" || !utf8.ValidString(request.AttemptKey) ||
		len(request.AttemptKey) > factory.ledger.limits.AttemptKeyMaxUTF8Bytes {
		return nil, fmt.Errorf("runtime attempt key exceeds the ledger bound")
	}
	declarations := factory.profile.Dependencies().Dependencies()
	if len(request.Dependencies) != len(declarations) ||
		len(declarations) > factory.ledger.limits.MaxClaimEntriesPerAttempt {
		return nil, fmt.Errorf("runtime dependency cardinality disagrees")
	}
	if err := validateRetainedDiagnostics(request.Diagnostics, factory.ledger.limits); err != nil {
		return nil, err
	}
	if err := validateRuntimeAttemptFacts(request); err != nil {
		return nil, err
	}
	ledger := factory.ledger
	ledger.mu.Lock()
	if committed := ledger.lastCommittedAttempt; committed.PollGenerationID != 0 &&
		request.Identity.PollGenerationID <= committed.PollGenerationID {
		ledger.mu.Unlock()
		return nil, fmt.Errorf("attempt poll generation is already terminal")
	}
	if _, duplicate := ledger.attempts[request.AttemptKey]; duplicate {
		ledger.mu.Unlock()
		return nil, fmt.Errorf("runtime attempt key is already retained")
	}
	if len(ledger.attempts) >= ledger.limits.MaxRetainedAttempts ||
		ledger.retainedClaims+len(declarations) > ledger.limits.MaxRetainedClaimEntries {
		ledger.mu.Unlock()
		return nil, fmt.Errorf("runtime attempt ledger capacity is exhausted")
	}
	if !ledger.terminalBatchAvailableLocked(len(declarations)) {
		ledger.mu.Unlock()
		return nil, fmt.Errorf("ledger terminal sequence is exhausted")
	}
	producerAttempt, err := request.Source.BeginAttempt(request.AttemptKey)
	if err != nil {
		ledger.mu.Unlock()
		return nil, fmt.Errorf("begin producer runtime attempt: %w", err)
	}
	attemptSequence, claimSequences, err := ledger.reserveTerminalBatchLocked(
		len(declarations),
	)
	if err != nil {
		_, _ = producerAttempt.Close(nil)
		ledger.mu.Unlock()
		return nil, err
	}
	state := &runtimeAttemptState{
		factory:          factory,
		key:              request.AttemptKey,
		identity:         request.Identity,
		phase:            AttemptOpen,
		terminalSequence: attemptSequence,
		entries:          make([]runtimeClaimEntry, len(declarations)),
		source:           request.Source,
		producerAttempt:  producerAttempt,
		acquisitions:     make([]modbus.RuntimeAcquisition, len(declarations)),
		issued:           make([]bool, len(declarations)),
		observationFacts: request.Observation,
		dependencyFacts: append(
			[]modbusreg.RuntimeDependencyFacts(nil),
			request.Dependencies...,
		),
		diagnostics: append([]string(nil), request.Diagnostics...),
	}
	state.cancelOpen = func() error {
		return request.Source.CancelOpen(state.instance)
	}
	state.cond = sync.NewCond(&state.mu)
	for index, sequence := range claimSequences {
		state.entries[index] = runtimeClaimEntry{
			phase:            runtimeClaimUnresolved,
			terminalSequence: sequence,
		}
	}
	ledger.attempts[request.AttemptKey] = state
	ledger.retainedClaims += len(declarations)
	ledger.mu.Unlock()
	return &ObservationAttempt{state: state}, nil
}

// Issue privately creates one producer acquisition at its exact dependency
// ordinal. Neither the producer attempt nor the resulting capability escapes.
func (attempt *ObservationAttempt) Issue(
	ordinal uint32,
	view modbus.LogicalReadView,
	normalization modbus.RuntimeNormalizationRecord,
) error {
	if attempt == nil || attempt.state == nil {
		return fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != AttemptOpen || state.admitted || state.source == nil ||
		state.producerAttempt == nil || ordinal >= uint32(len(state.acquisitions)) {
		return fmt.Errorf("runtime issuance is closed or outside the dependency set")
	}
	if state.issued[ordinal] {
		return fmt.Errorf("runtime dependency ordinal was already issued")
	}
	acquisition, err := state.source.Issue(
		state.producerAttempt,
		ordinal,
		view,
		normalization,
	)
	if err != nil {
		return fmt.Errorf("issue retained producer acquisition: %w", err)
	}
	state.acquisitions[ordinal] = acquisition
	state.issued[ordinal] = true
	return nil
}

// Admit closes the retained producer attempt over the exact private ordered
// set accumulated by Issue and stores the returned opaque instance privately.
func (attempt *ObservationAttempt) Admit() error {
	if attempt == nil || attempt.state == nil {
		return fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	if state.phase != AttemptOpen || state.admitted ||
		state.producerAttempt == nil || state.inProgressClaims != 0 {
		state.mu.Unlock()
		return fmt.Errorf("runtime attempt admission is closed")
	}
	for ordinal, issued := range state.issued {
		if !issued || !state.acquisitions[ordinal].Valid() {
			state.mu.Unlock()
			return fmt.Errorf("runtime dependency set is incomplete")
		}
	}
	retained := append([]modbus.RuntimeAcquisition(nil), state.acquisitions...)
	instance, err := state.producerAttempt.Close(retained)
	state.producerAttempt = nil
	if err != nil {
		state.cancelUnresolvedLocked()
		state.phase = AttemptCancelling
		state.source = nil
		state.mu.Unlock()
		cause := fmt.Errorf("close retained producer runtime attempt: %w", err)
		if terminalErr := attempt.finishCancellation(AttemptCancelFailed); terminalErr != nil {
			return fmt.Errorf("%v; terminal persistence: %w", cause, terminalErr)
		}
		return cause
	}
	state.instance = instance
	state.admitted = true
	state.issued = nil
	request := RuntimeAttemptRequest{
		Identity:     state.identity,
		Observation:  state.observationFacts,
		Dependencies: append([]modbusreg.RuntimeDependencyFacts(nil), state.dependencyFacts...),
	}
	template, validationErr := state.factory.buildRuntimeObservation(
		state.key,
		retained,
		request,
		state.factory.profile.Dependencies().Dependencies(),
	)
	if validationErr == nil {
		maximumSampleID := fmt.Sprintf(
			"%s:%d",
			state.factory.ledger.issuerDomain,
			uint64(math.MaxUint64),
		)
		_, validationErr = template.WithSampleID(maximumSampleID)
	}
	if validationErr == nil {
		state.acquisitions = retained
		state.template = template
		state.mu.Unlock()
		return nil
	}
	state.phase = AttemptCancelling
	state.acquisitions = retained
	state.mu.Unlock()
	if drainErr := state.drainSource(); drainErr != nil {
		terminalErr := attempt.finishCancellation(AttemptCancelFailed)
		if terminalErr != nil {
			return fmt.Errorf(
				"runtime admission validation failed (%v), source drain failed (%v), and terminal persistence failed: %w",
				validationErr,
				drainErr,
				terminalErr,
			)
		}
		return fmt.Errorf(
			"runtime admission validation failed (%v) and source drain failed: %w",
			validationErr,
			drainErr,
		)
	}
	if terminalErr := attempt.finishCancellation(AttemptCancelled); terminalErr != nil {
		return fmt.Errorf(
			"runtime admission validation failed (%v) and terminal persistence failed: %w",
			validationErr,
			terminalErr,
		)
	}
	return validationErr
}

func (state *runtimeAttemptState) cancelUnresolvedLocked() {
	for index := range state.entries {
		if state.entries[index].phase == runtimeClaimUnresolved {
			state.entries[index].phase = runtimeClaimTerminal
			state.entries[index].outcome = ClaimAttemptCancelled
		}
	}
}

func (factory *ObservationFactory) validateAttemptIdentity(identity modbusreg.AttemptIdentity) error {
	if identity.PollGenerationID == 0 {
		return fmt.Errorf("attempt poll generation is absent")
	}
	switch factory.profile.Coherence().Mode {
	case modbusreg.CoherenceSingleWireResponse:
		if identity.RetryOrdinal != 0 {
			return fmt.Errorf("single-wire retry identity is not applicable")
		}
	case modbusreg.CoherenceBoundedMultiResponse:
		if identity.RetryOrdinal == 0 {
			return fmt.Errorf("bounded retry identity is absent")
		}
	default:
		return fmt.Errorf("observation coherence mode is invalid")
	}
	committed := factory.ledger.ExportState().LastCommittedAttempt
	if committed.PollGenerationID != 0 &&
		identity.PollGenerationID <= committed.PollGenerationID {
		return fmt.Errorf("attempt poll generation is already terminal")
	}
	return nil
}

func validateRetainedDiagnostics(values []string, limits modbusreg.LedgerLimits) error {
	if len(values) > limits.RetainedDiagnosticCountPerObjectMax {
		return fmt.Errorf("retained diagnostics exceed count bound")
	}
	for _, value := range values {
		if value == "" || !utf8.ValidString(value) ||
			len(value) > limits.RetainedDiagnosticMaxUTF8Bytes {
			return fmt.Errorf("retained diagnostic exceeds byte bound")
		}
	}
	return nil
}

func validateRuntimeAttemptFacts(request RuntimeAttemptRequest) error {
	switch request.Observation.SourceValidity {
	case modbusreg.SourceValid, modbusreg.SourceInvalid, modbusreg.SourceNotImplemented, modbusreg.SourceReserved:
	default:
		return fmt.Errorf("runtime observation source validity is unknown")
	}
	if err := modbusreg.ValidateSourceTime(request.Observation.SourceTime); err != nil {
		return fmt.Errorf("runtime observation source time: %w", err)
	}
	if _, err := modbusreg.CanonicalRequiredTime(
		request.Observation.LocalReceiptTime,
		request.Observation.LocalReceiptTimePresent,
	); err != nil {
		return fmt.Errorf("runtime observation receipt time: %w", err)
	}
	if err := modbusreg.PreflightAggregate(request.Dependencies); err != nil {
		return fmt.Errorf("runtime dependency facts: %w", err)
	}
	for index, facts := range request.Dependencies {
		if err := modbusreg.ValidateSourceTime(facts.SourceTime); err != nil {
			return fmt.Errorf("runtime dependency %d source time: %w", index, err)
		}
		if modbusreg.HasLocalReceiptTime(
			facts.LocalReceiptTime,
			facts.LocalReceiptTimePresent,
		) {
			if _, err := modbusreg.CanonicalRequiredTime(
				facts.LocalReceiptTime,
				facts.LocalReceiptTimePresent,
			); err != nil {
				return fmt.Errorf("runtime dependency %d receipt time: %w", index, err)
			}
		}
		if err := modbusreg.ValidateBoundedString(
			"runtime dependency consistency marker",
			facts.DocumentaryConsistencyMarker,
			false,
		); err != nil {
			return err
		}
	}
	return nil
}

func (factory *ObservationFactory) buildRuntimeObservation(
	attemptKey string,
	acquisitions []modbus.RuntimeAcquisition,
	request RuntimeAttemptRequest,
	declarations []modbusreg.Dependency,
) (modbusreg.Observation, error) {
	results := make([]modbusreg.DependencyResult, len(declarations))
	runtimeBytes := make([][]byte, len(declarations))
	runtimeFields := make([]modbus.RuntimeNormalizationFields, len(declarations))
	var endpoint string
	var unitID byte
	for index, declaration := range declarations {
		acquisition := acquisitions[index]
		if !acquisition.Valid() || acquisition.AttemptKey() != attemptKey {
			return modbusreg.Observation{}, fmt.Errorf("runtime acquisition %d changed identity", index)
		}
		normalization := acquisition.Normalization()
		if !normalization.Valid() {
			return modbusreg.Observation{}, fmt.Errorf("runtime acquisition %d lacks normalization", index)
		}
		encoded := normalization.Bytes()
		if len(encoded) == 0 ||
			len(encoded) > factory.ledger.limits.NormalizationRecordMaxEncodedBytes {
			return modbusreg.Observation{}, fmt.Errorf("runtime normalization %d exceeds byte bound", index)
		}
		fields := normalization.Fields()
		if err := modbusreg.MatchRuntimeNormalization(declaration, fields); err != nil {
			return modbusreg.Observation{}, fmt.Errorf("runtime normalization %d: %w", index, err)
		}
		provenance := acquisition.Provenance()
		if provenance.SourceKind != modbus.RuntimeAcquisitionSourceRuntime ||
			provenance.SourceEvidenceID != fields.SourceEvidenceID ||
			len(provenance.WireResponseBytes) == 0 {
			return modbusreg.Observation{}, fmt.Errorf("runtime acquisition %d provenance is incomplete", index)
		}
		snapshot, err := modbusreg.NewLogicalViewSnapshot(modbusreg.LogicalViewRecord{
			LogicalViewID:       provenance.LogicalViewID,
			WireResponseID:      provenance.WireResponseID,
			PhysicalRequestID:   provenance.PhysicalRequestID,
			Endpoint:            provenance.Endpoint,
			ConnectionID:        provenance.ConnectionID,
			Transport:           provenance.Transport,
			TransportGeneration: provenance.TransportGeneration,
			UnitID:              provenance.UnitID,
			RequestedFunction:   provenance.RequestedFunction,
			ReceivedFunction:    provenance.ReceivedFunction,
			Table:               provenance.Table,
			PhysicalOffset:      provenance.PhysicalOffset,
			PhysicalWordCount:   provenance.PhysicalWordCount,
			AuthorizationScope:  provenance.AuthorizationScope,
			PollGeneration:      provenance.PollGeneration,
			DeadlineIdentity:    provenance.DeadlineIdentity,
			LogicalOffset:       provenance.LogicalOffset,
			LogicalWordCount:    provenance.LogicalWordCount,
			SliceOffset:         provenance.SliceOffset,
			SliceWordCount:      provenance.SliceWordCount,
			Words:               provenance.Words,
			WireResponseBytes:   provenance.WireResponseBytes,
		})
		if err != nil {
			return modbusreg.Observation{}, fmt.Errorf("runtime dependency %d view: %w", index, err)
		}
		if provenance.PollGeneration != request.Identity.PollGenerationID {
			return modbusreg.Observation{}, fmt.Errorf("runtime dependency %d poll generation disagrees", index)
		}
		facts := request.Dependencies[index]
		results[index] = modbusreg.DependencyResult{
			DependencyID:                 declaration.ID(),
			DependencyVersion:            declaration.Version(),
			CodecID:                      declaration.CodecID(),
			CodecVersion:                 declaration.CodecVersion(),
			NormalizationVersion:         declaration.Normalization().Spec().Version,
			Status:                       modbusreg.DependencyReadSuccessful,
			View:                         snapshot,
			SourceTime:                   facts.SourceTime,
			DocumentaryConsistencyMarker: facts.DocumentaryConsistencyMarker,
			AcquisitionOrdinal:           facts.AcquisitionOrdinal,
			RetryOrdinal:                 request.Identity.RetryOrdinal,
		}.WithLocalReceiptTime(facts.LocalReceiptTime, facts.LocalReceiptTimePresent)
		runtimeBytes[index] = append([]byte(nil), encoded...)
		runtimeFields[index] = fields
		if index == 0 {
			endpoint = provenance.Endpoint
			unitID = provenance.UnitID
		}
	}
	spec := modbusreg.ObservationSpec{
		SchemaVersion:          modbusreg.CurrentSchemaVersion(),
		RuntimeContractVersion: factory.profile.RuntimeContractVersion(),
		ProfileID:              factory.profile.ID(),
		ProfileVersion:         factory.profile.Version(),
		CodecContractVersion:   factory.profile.CodecContractVersion(),
		DetectorVersion:        factory.profile.DetectorVersion(),
		NormalizationVersion:   factory.profile.NormalizationVersion(),
		CoherenceVersion:       factory.profile.CoherenceVersion(),
		QualificationVersion:   factory.profile.QualificationVersion(),
		PollGenerationID:       request.Identity.PollGenerationID,
		RetryOrdinal:           request.Identity.RetryOrdinal,
		DependencySetID:        factory.profile.Dependencies().ID(),
		DependencySetVersion:   factory.profile.Dependencies().Version(),
		SourceValidity:         request.Observation.SourceValidity,
		SourceTime:             request.Observation.SourceTime,
		Endpoint:               endpoint,
		UnitID:                 unitID,
		Dependencies:           results,
	}.WithLocalReceiptTime(
		request.Observation.LocalReceiptTime,
		request.Observation.LocalReceiptTimePresent,
	)
	return modbusreg.NewRuntimeObservation(factory.profile, spec, runtimeBytes, runtimeFields)
}

// Phase returns the shared attempt lifecycle state.
func (attempt *ObservationAttempt) Phase() AttemptPhase {
	if attempt == nil || attempt.state == nil {
		return ""
	}
	attempt.state.mu.Lock()
	defer attempt.state.mu.Unlock()
	return attempt.state.phase
}

// Claim invokes exactly one producer Capability().Claim(instance) operation
// after ledger admission linearizes against sealing and cancellation.
func (attempt *ObservationAttempt) Claim(ordinal uint64) (ClaimOutcome, error) {
	if attempt == nil || attempt.state == nil {
		return "", fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	if ordinal >= uint64(len(state.entries)) {
		state.mu.Unlock()
		return "", fmt.Errorf("claim ordinal is outside the dependency set")
	}
	entry := &state.entries[ordinal]
	if entry.phase == runtimeClaimTerminal {
		outcome := entry.outcome
		state.mu.Unlock()
		return outcome, fmt.Errorf("claim entry is already terminal")
	}
	if state.phase != AttemptOpen || !state.admitted ||
		entry.phase != runtimeClaimUnresolved {
		state.mu.Unlock()
		return "", fmt.Errorf("claim admission is closed")
	}
	entry.phase = runtimeClaimInProgress
	state.inProgressClaims++
	capability := state.acquisitions[ordinal].Capability()
	instance := state.instance
	state.mu.Unlock()

	result, claimErr := capability.Claim(instance)
	outcome := claimOutcomeFromProducer(result, claimErr)

	state.mu.Lock()
	entry = &state.entries[ordinal]
	entry.phase = runtimeClaimTerminal
	entry.outcome = outcome
	state.inProgressClaims--
	state.cond.Broadcast()
	state.mu.Unlock()
	if claimErr != nil {
		return outcome, fmt.Errorf("producer capability claim: %w", claimErr)
	}
	return outcome, nil
}

func claimOutcomeFromProducer(
	result modbus.RuntimeCapabilityClaimResult,
	err error,
) ClaimOutcome {
	if err != nil {
		return ClaimRejectedTerminal
	}
	switch result.Outcome {
	case modbus.RuntimeCapabilityClaimed:
		if result.Won {
			return ClaimSucceeded
		}
		return ClaimRejectedTerminal
	case modbus.RuntimeCapabilityCancelled:
		return CapabilityCancelled
	case modbus.RuntimeCapabilityFailed:
		return CapabilityFailed
	case modbus.RuntimeCapabilityExpired:
		return CapabilityExpired
	default:
		return ClaimRejectedTerminal
	}
}

// Seal atomically freezes the immutable successful runtime set.
func (attempt *ObservationAttempt) Seal() error {
	if attempt == nil || attempt.state == nil {
		return fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != AttemptOpen || !state.admitted || len(state.entries) == 0 ||
		state.inProgressClaims != 0 {
		return fmt.Errorf("observation attempt cannot be sealed")
	}
	for _, entry := range state.entries {
		if entry.phase != runtimeClaimTerminal || entry.outcome != ClaimSucceeded {
			return fmt.Errorf("observation attempt has an unsuccessful runtime claim")
		}
	}
	state.phase = AttemptSealed
	return nil
}

func (state *runtimeAttemptState) drainSource() error {
	state.sourceDrainMu.Lock()
	defer state.sourceDrainMu.Unlock()
	if state.sourceDrainComplete || state.sourceDrainErr != nil {
		return state.sourceDrainErr
	}
	if state.cancelOpen == nil {
		state.sourceDrainErr = fmt.Errorf("runtime source is unavailable")
		return state.sourceDrainErr
	}
	err := state.cancelOpen()
	// A producer restart export may retire an already terminal exact instance
	// before this consumer drains its non-reconstructing producer tombstone.
	// The pinned producer reports that disposition as ErrRuntimeAttemptClosed.
	if errors.Is(err, modbus.ErrRuntimeAttemptClosed) {
		err = nil
	}
	if err != nil {
		state.sourceDrainErr = err
		return err
	}
	state.sourceDrainComplete = true
	state.acquisitions = nil
	state.source = nil
	state.cancelOpen = nil
	return nil
}

func (attempt *ObservationAttempt) finishCancellation(outcome AttemptPhase) error {
	state := attempt.state
	state.mu.Lock()
	if state.phase == AttemptCancelling {
		state.cancelUnresolvedLocked()
	}
	state.mu.Unlock()
	return attempt.persistTerminalOutcome(outcome)
}

// Cancel performs the one-shot claim drain and exact producer cancellation.
func (attempt *ObservationAttempt) Cancel() (CancellationResult, error) {
	if attempt == nil || attempt.state == nil {
		return "", fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	waitedForPersistence := false
	for {
		switch state.phase {
		case AttemptPublished:
			state.mu.Unlock()
			return CancellationAlreadyPublished, nil
		case AttemptPublishFailed:
			state.mu.Unlock()
			return CancellationPublishFailed, nil
		case AttemptCancelFailed:
			state.mu.Unlock()
			return CancellationFailed, nil
		case AttemptCancelled:
			state.mu.Unlock()
			return CancellationCompleted, nil
		case AttemptPublishing:
			if state.terminalPersistenceErr != nil {
				persistenceErr := state.terminalPersistenceErr
				outcome := state.terminalOutcome
				if waitedForPersistence ||
					(!state.terminalCommitInProgress && state.terminalWaiters != 0) {
					state.mu.Unlock()
					return CancellationFailed, persistenceErr
				}
				state.mu.Unlock()
				if err := attempt.persistTerminalOutcome(outcome); err != nil {
					return CancellationFailed, err
				}
				return CancellationPublishFailed, nil
			}
			if state.publishAbort != nil && !state.publishAbortClosed {
				close(state.publishAbort)
				state.publishAbortClosed = true
			}
			if state.publishCancel != nil {
				state.publishCancel()
			}
			state.terminalWaiters++
			state.cond.Wait()
			state.terminalWaiters--
			waitedForPersistence = true
			continue
		case AttemptCancelling:
			if state.terminalPersistenceErr != nil {
				persistenceErr := state.terminalPersistenceErr
				outcome := state.terminalOutcome
				if waitedForPersistence ||
					(!state.terminalCommitInProgress && state.terminalWaiters != 0) {
					state.mu.Unlock()
					return CancellationFailed, persistenceErr
				}
				state.mu.Unlock()
				if err := attempt.persistTerminalOutcome(outcome); err != nil {
					return CancellationFailed, err
				}
				if outcome == AttemptCancelFailed {
					return CancellationFailed, nil
				}
				return CancellationCompleted, nil
			}
			state.terminalWaiters++
			state.cond.Wait()
			state.terminalWaiters--
			waitedForPersistence = true
			continue
		case AttemptOpen, AttemptSealed:
			state.phase = AttemptCancelling
		default:
			state.mu.Unlock()
			return "", fmt.Errorf("observation attempt phase is invalid")
		}
		break
	}
	for state.inProgressClaims != 0 {
		state.cond.Wait()
	}
	admitted := state.admitted
	producerAttempt := state.producerAttempt
	state.producerAttempt = nil
	state.mu.Unlock()

	if !admitted {
		if producerAttempt == nil {
			cause := fmt.Errorf("producer runtime attempt is unavailable")
			if terminalErr := attempt.finishCancellation(AttemptCancelFailed); terminalErr != nil {
				return CancellationFailed, fmt.Errorf("%v; terminal persistence: %w", cause, terminalErr)
			}
			return CancellationFailed, cause
		}
		_, closeErr := producerAttempt.Close(nil)
		if closeErr != nil &&
			!errors.Is(closeErr, modbus.ErrRuntimeAttemptMembership) &&
			!errors.Is(closeErr, modbus.ErrRuntimeAttemptClosed) {
			cause := fmt.Errorf("close unadmitted producer attempt: %w", closeErr)
			if terminalErr := attempt.finishCancellation(AttemptCancelFailed); terminalErr != nil {
				return CancellationFailed, fmt.Errorf("%v; terminal persistence: %w", cause, terminalErr)
			}
			return CancellationFailed, cause
		}
		state.mu.Lock()
		state.source = nil
		state.mu.Unlock()
		if err := attempt.finishCancellation(AttemptCancelled); err != nil {
			return CancellationFailed, err
		}
		return CancellationCompleted, nil
	}
	if err := state.drainSource(); err != nil {
		terminalErr := attempt.finishCancellation(AttemptCancelFailed)
		if terminalErr != nil {
			return CancellationFailed, fmt.Errorf(
				"cancel producer runtime attempt: %v; %w",
				err,
				terminalErr,
			)
		}
		return CancellationFailed, fmt.Errorf("cancel producer runtime attempt: %w", err)
	}
	if err := attempt.finishCancellation(AttemptCancelled); err != nil {
		return CancellationFailed, err
	}
	return CancellationCompleted, nil
}

// Publish consumes only sealed immutable ledger state and invokes the
// transactional commit boundary exactly once.
func (attempt *ObservationAttempt) Publish(
	ctx context.Context,
) (modbusreg.Observation, error) {
	if attempt == nil || attempt.state == nil || ctx == nil {
		return modbusreg.Observation{}, fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	if state.phase != AttemptSealed {
		state.mu.Unlock()
		return modbusreg.Observation{}, fmt.Errorf("observation attempt is not sealed")
	}
	publishContext, cancel := context.WithCancel(ctx)
	publishAbort := make(chan struct{})
	state.phase = AttemptPublishing
	state.publishCancel = cancel
	state.publishAbort = publishAbort
	state.publishAbortClosed = false
	factory := state.factory
	state.mu.Unlock()
	defer cancel()

	if err := state.drainSource(); err != nil {
		return attempt.failPublication(fmt.Errorf("drain producer runtime attempt: %w", err))
	}
	ledger := factory.ledger
	select {
	case <-ctx.Done():
		return attempt.failPublication(fmt.Errorf("publication context ended before commit admission: %w", ctx.Err()))
	case <-publishAbort:
		return attempt.failPublication(fmt.Errorf("publication was cancelled before commit admission"))
	case <-ledger.commitSerial:
	}
	releaseCommitSerial := func() {
		ledger.commitSerial <- struct{}{}
	}
	state.mu.Lock()
	aborted := state.publishAbortClosed
	contextErr := ctx.Err()
	state.mu.Unlock()
	if aborted {
		releaseCommitSerial()
		return attempt.failPublication(fmt.Errorf("publication was cancelled before commit admission"))
	}
	if contextErr != nil {
		releaseCommitSerial()
		return attempt.failPublication(fmt.Errorf("publication context ended before commit admission: %w", contextErr))
	}
	ledger.mu.Lock()
	if ledger.revision == math.MaxUint64 || ledger.highWater == math.MaxUint64 ||
		!attemptIdentityAdvances(ledger.lastCommittedAttempt, state.identity) {
		ledger.mu.Unlock()
		releaseCommitSerial()
		return attempt.failPublication(fmt.Errorf("sample ledger is exhausted or attempt does not advance"))
	}
	expected := ledger.stateLocked()
	next := expected
	next.Revision++
	next.HighWater++
	next.LastCommittedAttempt = state.identity
	expectedRestart := ledger.durableRestartStateLocked()
	publishedRestart, restartErr := ledger.terminalRestartStateLocked(
		state.terminalTombstones(AttemptPublished),
	)
	ledger.mu.Unlock()
	if restartErr != nil {
		releaseCommitSerial()
		return attempt.failPublication(fmt.Errorf("publication restart state: %w", restartErr))
	}
	observation, err := state.template.WithSampleID(
		fmt.Sprintf("%s:%d", ledger.issuerDomain, next.HighWater),
	)
	var observationBytes []byte
	if err == nil {
		observationBytes, err = modbusreg.MarshalObservation(observation)
	}
	if err != nil {
		releaseCommitSerial()
		return attempt.failPublication(fmt.Errorf("final publication observation: %w", err))
	}
	projection := state.publishedProjection()
	decision, err := factory.committer.CommitPublication(
		publishContext,
		PublicationCommitRequest{
			ExpectedState:         expected,
			PublishedState:        next,
			ExpectedRestartState:  expectedRestart,
			PublishedRestartState: publishedRestart,
			Attempt:               projection,
			Observation:           observation,
			ObservationBytes:      append([]byte(nil), observationBytes...),
		},
	)
	if err != nil || decision != PublicationCommitCommitted {
		releaseCommitSerial()
		if err != nil {
			return attempt.failPublication(fmt.Errorf("transactional publication: %w", err))
		}
		return attempt.failPublication(fmt.Errorf("transactional publication was cancelled"))
	}
	ledger.mu.Lock()
	currentRestart := ledger.durableRestartStateLocked()
	if ledger.stateLocked() != expected ||
		(!ledgerRestartStatesEqual(currentRestart, expectedRestart) &&
			!currentRestart.CoversTerminalWatermark(publishedRestart)) {
		ledger.mu.Unlock()
		releaseCommitSerial()
		return attempt.failPublication(fmt.Errorf("sample ledger changed during publication commit"))
	}
	ledger.revision = next.Revision
	ledger.highWater = next.HighWater
	ledger.lastCommittedAttempt = next.LastCommittedAttempt
	if !currentRestart.CoversTerminalWatermark(publishedRestart) {
		ledger.applyDurableRestartStateLocked(publishedRestart)
	}
	ledger.mu.Unlock()
	state.mu.Lock()
	state.restartCommitted = true
	state.mu.Unlock()
	releaseCommitSerial()
	if err := attempt.finishPublication(AttemptPublished); err != nil {
		return modbusreg.Observation{}, err
	}
	return observation, nil
}

func (attempt *ObservationAttempt) failPublication(cause error) (modbusreg.Observation, error) {
	if terminalErr := attempt.finishPublication(AttemptPublishFailed); terminalErr != nil {
		return modbusreg.Observation{}, fmt.Errorf("%v; terminal persistence: %w", cause, terminalErr)
	}
	return modbusreg.Observation{}, cause
}

func (attempt *ObservationAttempt) finishPublication(outcome AttemptPhase) error {
	return attempt.persistTerminalOutcome(outcome)
}

func (attempt *ObservationAttempt) persistTerminalOutcome(outcome AttemptPhase) error {
	state := attempt.state
	state.mu.Lock()
	if !validAttemptTerminalOutcome(outcome) ||
		(state.phase != AttemptCancelling && state.phase != AttemptPublishing) {
		state.mu.Unlock()
		return fmt.Errorf("terminal persistence retry is unavailable")
	}
	if state.terminalPersistenceErr != nil && state.terminalOutcome != outcome {
		state.mu.Unlock()
		return fmt.Errorf("terminal persistence outcome is immutable")
	}
	if state.terminalCommitInProgress {
		if state.terminalOutcome != outcome {
			state.mu.Unlock()
			return fmt.Errorf("terminal persistence outcome is immutable")
		}
		state.terminalWaiters++
		for state.terminalCommitInProgress {
			state.cond.Wait()
		}
		state.terminalWaiters--
		if state.terminalPersistenceErr != nil {
			err := state.terminalPersistenceErr
			state.mu.Unlock()
			return err
		}
		if state.phase != outcome {
			state.mu.Unlock()
			return fmt.Errorf("terminal persistence retry is unavailable")
		}
		state.mu.Unlock()
		return nil
	}
	state.terminalOutcome = outcome
	state.terminalCommitInProgress = true
	alreadyCommitted := state.restartCommitted
	state.mu.Unlock()

	var commitErr error
	if !alreadyCommitted {
		commitErr = attempt.commitTerminalState(outcome)
	}
	state.mu.Lock()
	if commitErr != nil {
		state.terminalCommitInProgress = false
		state.terminalPersistenceErr = commitErr
		state.cond.Broadcast()
		state.mu.Unlock()
		return commitErr
	}
	factory := state.factory
	if factory == nil || factory.ledger == nil {
		finalizeErr := fmt.Errorf("terminal persistence factory is unavailable")
		state.terminalCommitInProgress = false
		state.terminalPersistenceErr = finalizeErr
		state.cond.Broadcast()
		state.mu.Unlock()
		return finalizeErr
	}
	state.mu.Unlock()
	return factory.ledger.finalizeTerminalAttempt(state, outcome)
}

func (state *runtimeAttemptState) publishedProjection() PublishedAttemptV1 {
	hash := sha256.New()
	hash.Write([]byte("helianthus.modbusreg.claim-outcomes/v1\x00"))
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(len(state.entries)))
	hash.Write(encoded[:])
	for ordinal, entry := range state.entries {
		binary.BigEndian.PutUint64(encoded[:], uint64(ordinal))
		hash.Write(encoded[:])
		binary.BigEndian.PutUint64(encoded[:], entry.terminalSequence)
		hash.Write(encoded[:])
		binary.BigEndian.PutUint64(encoded[:], uint64(len(entry.outcome)))
		hash.Write(encoded[:])
		hash.Write([]byte(entry.outcome))
	}
	return PublishedAttemptV1{
		SchemaVersion:           1,
		AttemptTerminalSequence: state.terminalSequence,
		DependencySetDigest:     state.factory.dependencySetDigest,
		RuntimeDependencyCount:  uint64(len(state.entries)),
		ClaimOutcomeDigest:      "sha256:" + hex.EncodeToString(hash.Sum(nil)),
	}
}

func attemptIdentityAdvances(previous, next modbusreg.AttemptIdentity) bool {
	if next.PollGenerationID == 0 {
		return false
	}
	if previous.PollGenerationID == 0 {
		return true
	}
	return next.PollGenerationID > previous.PollGenerationID
}

func validClaimOutcome(outcome ClaimOutcome) bool {
	switch outcome {
	case ClaimSucceeded, CapabilityCancelled, CapabilityFailed,
		CapabilityExpired, ClaimRejectedTerminal, ClaimAttemptCancelled:
		return true
	default:
		return false
	}
}

func validAttemptTerminalOutcome(outcome AttemptPhase) bool {
	switch outcome {
	case AttemptPublished, AttemptPublishFailed, AttemptCancelFailed, AttemptCancelled:
		return true
	default:
		return false
	}
}

func (state *runtimeAttemptState) terminalTombstones(
	outcome AttemptPhase,
) []AuditTombstone {
	state.mu.Lock()
	defer state.mu.Unlock()
	result := make([]AuditTombstone, 0, len(state.entries)+1)
	result = append(result, AuditTombstone{
		SchemaVersion:    1,
		ObjectKind:       AuditAttempt,
		TerminalSequence: state.terminalSequence,
		ClaimCount:       uint64(len(state.entries)),
		TerminalOutcome:  string(outcome),
	})
	for ordinal, entry := range state.entries {
		if entry.phase != runtimeClaimTerminal || !validClaimOutcome(entry.outcome) {
			continue
		}
		result = append(result, AuditTombstone{
			SchemaVersion:           1,
			ObjectKind:              AuditClaim,
			TerminalSequence:        entry.terminalSequence,
			AttemptTerminalSequence: state.terminalSequence,
			ClaimOrdinal:            uint64(ordinal),
			TerminalOutcome:         string(entry.outcome),
		})
	}
	return result
}

func (attempt *ObservationAttempt) commitTerminalState(outcome AttemptPhase) error {
	state := attempt.state
	state.mu.Lock()
	factory := state.factory
	state.mu.Unlock()
	if factory == nil || factory.committer == nil || factory.terminalCommitTimeout <= 0 {
		return fmt.Errorf("terminal commit factory is unavailable")
	}
	ledger := factory.ledger
	ledger.mu.Lock()
	expected := ledger.durableRestartStateLocked()
	terminal, err := ledger.terminalRestartStateLocked(state.terminalTombstones(outcome))
	ledger.mu.Unlock()
	if err != nil {
		return err
	}
	commitContext, cancel := context.WithTimeout(
		context.Background(),
		factory.terminalCommitTimeout,
	)
	defer cancel()
	if err := factory.committer.CommitTerminalState(
		commitContext,
		TerminalStateCommitRequest{
			ExpectedRestartState: expected,
			TerminalRestartState: terminal,
			Attempt:              state.publishedProjection(),
			Outcome:              outcome,
		},
	); err != nil {
		return fmt.Errorf("durable terminal state: %w", err)
	}
	ledger.mu.Lock()
	current := ledger.durableRestartStateLocked()
	if !ledgerRestartStatesEqual(current, expected) &&
		!current.CoversTerminalWatermark(terminal) {
		ledger.mu.Unlock()
		return fmt.Errorf("durable terminal state changed during commit")
	}
	if !current.CoversTerminalWatermark(terminal) {
		ledger.applyDurableRestartStateLocked(terminal)
	}
	ledger.mu.Unlock()
	state.mu.Lock()
	state.restartCommitted = true
	state.mu.Unlock()
	return nil
}

func (ledger *SampleLedger) finalizeTerminalAttempt(
	state *runtimeAttemptState,
	outcome AttemptPhase,
) error {
	if ledger == nil || state == nil || !validAttemptTerminalOutcome(outcome) {
		return fmt.Errorf("terminal attempt finalization is invalid")
	}
	// Terminal visibility and bounded reclamation share one ledger-first
	// critical section. Waiters cannot observe the terminal phase until the key
	// and retained capacity are reusable.
	ledger.mu.Lock()
	state.mu.Lock()
	if state.inProgressClaims != 0 || !state.terminalCommitInProgress ||
		state.terminalOutcome != outcome ||
		(state.phase != AttemptCancelling && state.phase != AttemptPublishing) {
		finalizeErr := fmt.Errorf("terminal attempt finalization is unavailable")
		state.terminalCommitInProgress = false
		state.terminalPersistenceErr = finalizeErr
		state.cond.Broadcast()
		state.mu.Unlock()
		ledger.mu.Unlock()
		return finalizeErr
	}
	key := state.key
	if ledger.attempts[key] != state {
		finalizeErr := fmt.Errorf("terminal attempt ledger ownership changed")
		state.terminalCommitInProgress = false
		state.terminalPersistenceErr = finalizeErr
		state.cond.Broadcast()
		state.mu.Unlock()
		ledger.mu.Unlock()
		return finalizeErr
	}
	delete(ledger.attempts, key)
	ledger.retainedClaims -= len(state.entries)
	for ordinal, entry := range state.entries {
		if state.restartCommitted {
			break
		}
		if entry.phase != runtimeClaimTerminal || !validClaimOutcome(entry.outcome) {
			continue
		}
		ledger.insertAuditTombstoneLocked(AuditTombstone{
			SchemaVersion:           1,
			ObjectKind:              AuditClaim,
			TerminalSequence:        entry.terminalSequence,
			AttemptTerminalSequence: state.terminalSequence,
			ClaimOrdinal:            uint64(ordinal),
			TerminalOutcome:         string(entry.outcome),
		})
	}
	if !state.restartCommitted {
		ledger.insertAuditTombstoneLocked(AuditTombstone{
			SchemaVersion:    1,
			ObjectKind:       AuditAttempt,
			TerminalSequence: state.terminalSequence,
			ClaimCount:       uint64(len(state.entries)),
			TerminalOutcome:  string(outcome),
		})
	}
	state.phase = outcome
	state.terminalCommitInProgress = false
	state.terminalPersistenceErr = nil
	state.factory = nil
	state.key = ""
	state.identity = modbusreg.AttemptIdentity{}
	state.entries = nil
	state.producerAttempt = nil
	state.instance = modbus.RuntimeAttemptInstance{}
	state.admitted = false
	state.acquisitions = nil
	state.issued = nil
	state.observationFacts = RuntimeObservationFacts{}
	state.dependencyFacts = nil
	state.diagnostics = nil
	state.source = nil
	state.template = modbusreg.Observation{}
	state.publishCancel = nil
	state.publishAbort = nil
	state.publishAbortClosed = false
	state.restartCommitted = false
	state.sourceDrainComplete = false
	state.sourceDrainErr = nil
	state.cancelOpen = nil
	state.cond.Broadcast()
	state.mu.Unlock()
	ledger.mu.Unlock()
	return nil
}
