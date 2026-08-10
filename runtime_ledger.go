package modbusreg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// ClaimOutcome is the closed immutable M2 claim terminal enum.
type ClaimOutcome string

const (
	ClaimSucceeded        ClaimOutcome = "claim_succeeded"
	CapabilityCancelled   ClaimOutcome = "capability_cancelled"
	CapabilityFailed      ClaimOutcome = "capability_failed"
	CapabilityExpired     ClaimOutcome = "capability_expired"
	ClaimRejectedTerminal ClaimOutcome = "claim_rejected_terminal"
	ClaimAttemptCancelled ClaimOutcome = "attempt_cancelled"
)

// AttemptPhase is the closed ledger attempt lifecycle.
type AttemptPhase string

const (
	AttemptOpen          AttemptPhase = "open"
	AttemptSealed        AttemptPhase = "sealed"
	AttemptCancelling    AttemptPhase = "cancelling"
	AttemptPublishing    AttemptPhase = "publishing"
	AttemptPublished     AttemptPhase = "published"
	AttemptPublishFailed AttemptPhase = "publish_failed"
	AttemptCancelFailed  AttemptPhase = "cancel_failed"
	AttemptCancelled     AttemptPhase = "cancelled"
)

// CancellationResult distinguishes a completed cancellation from publication
// winning the one-shot commit arbitration.
type CancellationResult string

const (
	CancellationCompleted        CancellationResult = "cancelled"
	CancellationFailed           CancellationResult = "cancel_failed"
	CancellationPublishFailed    CancellationResult = "publish_failed"
	CancellationAlreadyPublished CancellationResult = "already_published"
)

// LedgerAuditObjectKind is the closed tombstone variant discriminator.
type LedgerAuditObjectKind string

const (
	LedgerAuditAttempt LedgerAuditObjectKind = "attempt"
	LedgerAuditClaim   LedgerAuditObjectKind = "claim"
)

// LedgerAuditTombstone is a bounded, immutable, non-reconstructing audit
// record. Claim-only fields are absent from attempt JSON encodings.
type LedgerAuditTombstone struct {
	SchemaVersion           int                   `json:"schema_version"`
	ObjectKind              LedgerAuditObjectKind `json:"object_kind"`
	TerminalSequence        uint64                `json:"terminal_sequence"`
	AttemptTerminalSequence uint64                `json:"attempt_terminal_sequence,omitempty"`
	ClaimOrdinal            uint64                `json:"claim_ordinal,omitempty"`
	TerminalOutcome         string                `json:"terminal_outcome"`
}

const maxLedgerAuditTombstoneJSONBytes = 256

type attemptAuditTombstoneDTO struct {
	SchemaVersion    int                   `json:"schema_version"`
	ObjectKind       LedgerAuditObjectKind `json:"object_kind"`
	TerminalSequence uint64                `json:"terminal_sequence"`
	TerminalOutcome  string                `json:"terminal_outcome"`
}

type claimAuditTombstoneDTO struct {
	SchemaVersion           int                   `json:"schema_version"`
	ObjectKind              LedgerAuditObjectKind `json:"object_kind"`
	TerminalSequence        uint64                `json:"terminal_sequence"`
	AttemptTerminalSequence uint64                `json:"attempt_terminal_sequence"`
	ClaimOrdinal            uint64                `json:"claim_ordinal"`
	TerminalOutcome         string                `json:"terminal_outcome"`
}

func validateLedgerAuditTombstone(tombstone LedgerAuditTombstone) error {
	switch tombstone.ObjectKind {
	case LedgerAuditAttempt:
		if !validAttemptTerminalOutcome(AttemptPhase(tombstone.TerminalOutcome)) ||
			tombstone.SchemaVersion != 1 || tombstone.TerminalSequence == 0 ||
			tombstone.AttemptTerminalSequence != 0 || tombstone.ClaimOrdinal != 0 {
			return fmt.Errorf("attempt audit tombstone is invalid")
		}
	case LedgerAuditClaim:
		if !validClaimOutcome(ClaimOutcome(tombstone.TerminalOutcome)) ||
			tombstone.SchemaVersion != 1 || tombstone.TerminalSequence == 0 ||
			tombstone.AttemptTerminalSequence == 0 {
			return fmt.Errorf("claim audit tombstone is invalid")
		}
	default:
		return fmt.Errorf("ledger audit tombstone kind is invalid")
	}
	return nil
}

func (tombstone LedgerAuditTombstone) MarshalJSON() ([]byte, error) {
	if err := validateLedgerAuditTombstone(tombstone); err != nil {
		return nil, err
	}
	switch tombstone.ObjectKind {
	case LedgerAuditAttempt:
		return json.Marshal(attemptAuditTombstoneDTO{
			SchemaVersion:    1,
			ObjectKind:       LedgerAuditAttempt,
			TerminalSequence: tombstone.TerminalSequence,
			TerminalOutcome:  tombstone.TerminalOutcome,
		})
	case LedgerAuditClaim:
		return json.Marshal(claimAuditTombstoneDTO{
			SchemaVersion:           1,
			ObjectKind:              LedgerAuditClaim,
			TerminalSequence:        tombstone.TerminalSequence,
			AttemptTerminalSequence: tombstone.AttemptTerminalSequence,
			ClaimOrdinal:            tombstone.ClaimOrdinal,
			TerminalOutcome:         tombstone.TerminalOutcome,
		})
	default:
		return nil, fmt.Errorf("ledger audit tombstone kind is invalid")
	}
}

// UnmarshalJSON accepts only the exact closed attempt or claim tombstone
// projection. The fixed byte ceiling prevents whitespace or object padding
// from bypassing configured audit bounds before activation validation.
func (tombstone *LedgerAuditTombstone) UnmarshalJSON(data []byte) error {
	if tombstone == nil || len(data) == 0 || len(data) > maxLedgerAuditTombstoneJSONBytes {
		return fmt.Errorf("ledger audit tombstone encoding exceeds its bound")
	}
	var discriminator struct {
		ObjectKind LedgerAuditObjectKind `json:"object_kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	var decoded LedgerAuditTombstone
	switch discriminator.ObjectKind {
	case LedgerAuditAttempt:
		var record attemptAuditTombstoneDTO
		if err := decodeStrict(data, &record); err != nil {
			return err
		}
		decoded = LedgerAuditTombstone{
			SchemaVersion:    record.SchemaVersion,
			ObjectKind:       record.ObjectKind,
			TerminalSequence: record.TerminalSequence,
			TerminalOutcome:  record.TerminalOutcome,
		}
	case LedgerAuditClaim:
		var record claimAuditTombstoneDTO
		if err := decodeStrict(data, &record); err != nil {
			return err
		}
		decoded = LedgerAuditTombstone(record)
	default:
		return fmt.Errorf("ledger audit tombstone kind is invalid")
	}
	if err := validateLedgerAuditTombstone(decoded); err != nil {
		return err
	}
	*tombstone = decoded
	return nil
}

// LedgerRestartState is the bounded deterministic audit/sequence restart
// state. Live attempts and capabilities are intentionally nonserializable.
type LedgerRestartState struct {
	SchemaVersion        int
	NextTerminalSequence uint64
	SequenceExhausted    bool
	AuditTombstones      []LedgerAuditTombstone
}

// LedgerSnapshot reports bounded resource use without exposing attempt keys,
// capabilities, normalization records, or diagnostics.
type LedgerSnapshot struct {
	RetainedAttempts     int
	RetainedClaimEntries int
	NextTerminalSequence uint64
	SequenceExhausted    bool
	AuditTombstones      []LedgerAuditTombstone
}

// SampleLedger owns sample sequencing plus the bounded M2 attempt ledger.
type SampleLedger struct {
	mu                   sync.Mutex
	commitSerial         chan struct{}
	limits               LedgerLimits
	issuerDomain         string
	profileID            string
	profileVersion       Version
	dependencySetID      string
	revision             uint64
	highWater            uint64
	lastCommittedAttempt AttemptIdentity
	attempts             map[string]*runtimeAttemptState
	retainedClaims       int
	nextTerminalSequence uint64
	sequenceExhausted    bool
	auditTombstones      []LedgerAuditTombstone
}

// NewSampleLedger activates a new bounded attempt ledger. Omitting limits is
// retained for source compatibility and selects DefaultLedgerLimits.
func NewSampleLedger(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
	configuredLimits ...LedgerLimits,
) (*SampleLedger, error) {
	limits, err := selectLedgerLimits(configuredLimits)
	if err != nil {
		return nil, err
	}
	return newSampleLedger(state, trustedMinimumRevision, limits, nil)
}

// NewSampleLedgerFromRestart restores quiescent bounded sequence/audit state.
func NewSampleLedgerFromRestart(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
	limits LedgerLimits,
	restart LedgerRestartState,
) (*SampleLedger, error) {
	return newSampleLedger(state, trustedMinimumRevision, limits, &restart)
}

func selectLedgerLimits(configured []LedgerLimits) (LedgerLimits, error) {
	if len(configured) > 1 {
		return LedgerLimits{}, fmt.Errorf("sample ledger accepts one limits value")
	}
	limits := DefaultLedgerLimits()
	if len(configured) == 1 {
		limits = configured[0]
	}
	if err := validateLedgerLimits(limits); err != nil {
		return LedgerLimits{}, err
	}
	return limits, nil
}

func newSampleLedger(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
	limits LedgerLimits,
	restart *LedgerRestartState,
) (*SampleLedger, error) {
	if err := validateLedgerLimits(limits); err != nil {
		return nil, err
	}
	if err := validateLargestLedgerTombstones(limits); err != nil {
		return nil, err
	}
	if err := validateSampleLedgerState(state, trustedMinimumRevision); err != nil {
		return nil, err
	}
	nextSequence := uint64(1)
	var tombstones []LedgerAuditTombstone
	sequenceExhausted := false
	if restart != nil {
		if err := validateLedgerRestartState(*restart, limits); err != nil {
			return nil, err
		}
		nextSequence = restart.NextTerminalSequence
		sequenceExhausted = restart.SequenceExhausted
		tombstones = append([]LedgerAuditTombstone(nil), restart.AuditTombstones...)
	}
	ledger := &SampleLedger{
		commitSerial:         make(chan struct{}, 1),
		limits:               limits,
		issuerDomain:         state.IssuerDomain,
		profileID:            state.ProfileID,
		profileVersion:       state.ProfileVersion,
		dependencySetID:      state.DependencySetID,
		revision:             state.Revision,
		highWater:            state.HighWater,
		lastCommittedAttempt: state.LastCommittedAttempt,
		attempts:             make(map[string]*runtimeAttemptState),
		nextTerminalSequence: nextSequence,
		sequenceExhausted:    sequenceExhausted,
		auditTombstones:      tombstones,
	}
	ledger.commitSerial <- struct{}{}
	return ledger, nil
}

func validateSampleLedgerState(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
) error {
	initialAttemptState := state.LastCommittedAttempt == (AttemptIdentity{})
	if state.SchemaVersion != schemaVersionV1 ||
		!validIssuerDomain(state.IssuerDomain) ||
		!validIdentity(state.ProfileID) ||
		!state.ProfileVersion.valid() ||
		!validDependencySetID(state.DependencySetID) ||
		state.Revision != state.HighWater ||
		state.Revision < trustedMinimumRevision ||
		(state.Revision == 0 && !initialAttemptState) ||
		(state.Revision != 0 && state.LastCommittedAttempt.PollGenerationID == 0) ||
		(state.LastCommittedAttempt.PollGenerationID == 0 &&
			state.LastCommittedAttempt.RetryOrdinal != 0) {
		return fmt.Errorf("sample ledger state is incomplete, stale, or incompatible")
	}
	return nil
}

func validIssuerDomain(value string) bool {
	if len(value) > MaxSampleIssuerDomainBytes || !validIdentity(value) {
		return false
	}
	for _, character := range value {
		if character == ':' {
			return false
		}
	}
	return true
}

func validateLedgerRestartState(
	restart LedgerRestartState,
	limits LedgerLimits,
) error {
	if restart.SchemaVersion != 1 ||
		len(restart.AuditTombstones) > limits.AuditTombstoneLimit ||
		(restart.SequenceExhausted && restart.NextTerminalSequence != 0) ||
		(!restart.SequenceExhausted && restart.NextTerminalSequence == 0) {
		return fmt.Errorf("ledger restart state is invalid")
	}
	var previous uint64
	attemptSequences := make([]uint64, 0, len(restart.AuditTombstones))
	for index, tombstone := range restart.AuditTombstones {
		encoded, err := json.Marshal(tombstone)
		if err != nil || len(encoded) > limits.AuditTombstoneMaxEncodedBytes ||
			(index > 0 && tombstone.TerminalSequence <= previous) {
			return fmt.Errorf("ledger restart tombstone is invalid")
		}
		if tombstone.ObjectKind == LedgerAuditAttempt {
			attemptSequences = append(attemptSequences, tombstone.TerminalSequence)
		}
		previous = tombstone.TerminalSequence
	}
	for _, tombstone := range restart.AuditTombstones {
		if tombstone.ObjectKind != LedgerAuditClaim {
			continue
		}
		if tombstone.ClaimOrdinal >= uint64(limits.MaxClaimEntriesPerAttempt) ||
			tombstone.ClaimOrdinal == math.MaxUint64 {
			return fmt.Errorf("ledger restart claim ordinal is invalid")
		}
		offset := tombstone.ClaimOrdinal + 1
		if tombstone.AttemptTerminalSequence > math.MaxUint64-offset ||
			tombstone.TerminalSequence != tombstone.AttemptTerminalSequence+offset {
			return fmt.Errorf("ledger restart claim sequence is invalid")
		}
		laterAttempt := sort.Search(len(attemptSequences), func(index int) bool {
			return attemptSequences[index] > tombstone.AttemptTerminalSequence
		})
		if laterAttempt < len(attemptSequences) &&
			attemptSequences[laterAttempt] <= tombstone.TerminalSequence {
			return fmt.Errorf("ledger restart claim reservation overlaps a later attempt")
		}
	}
	if !restart.SequenceExhausted && previous >= restart.NextTerminalSequence {
		return fmt.Errorf("ledger restart sequence does not advance tombstones")
	}
	return nil
}

func validateLargestLedgerTombstones(limits LedgerLimits) error {
	candidates := []LedgerAuditTombstone{
		{
			SchemaVersion:    1,
			ObjectKind:       LedgerAuditAttempt,
			TerminalSequence: math.MaxUint64,
			TerminalOutcome:  string(AttemptPublishFailed),
		},
		{
			SchemaVersion:           1,
			ObjectKind:              LedgerAuditClaim,
			TerminalSequence:        math.MaxUint64,
			AttemptTerminalSequence: math.MaxUint64,
			ClaimOrdinal:            math.MaxUint64,
			TerminalOutcome:         string(ClaimRejectedTerminal),
		},
	}
	for _, candidate := range candidates {
		encoded, err := json.Marshal(candidate)
		if err != nil || len(encoded) > limits.AuditTombstoneMaxEncodedBytes {
			return fmt.Errorf("ledger tombstone encoding exceeds configured bound")
		}
	}
	return nil
}

// Limits returns the immutable activation bounds.
func (ledger *SampleLedger) Limits() LedgerLimits {
	if ledger == nil {
		return LedgerLimits{}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.limits
}

func (ledger *SampleLedger) stateLocked() SampleLedgerState {
	return SampleLedgerState{
		SchemaVersion:        schemaVersionV1,
		IssuerDomain:         ledger.issuerDomain,
		ProfileID:            ledger.profileID,
		ProfileVersion:       ledger.profileVersion,
		DependencySetID:      ledger.dependencySetID,
		Revision:             ledger.revision,
		HighWater:            ledger.highWater,
		LastCommittedAttempt: ledger.lastCommittedAttempt,
	}
}

// ExportState returns the current durable sample sequencing state.
func (ledger *SampleLedger) ExportState() SampleLedgerState {
	if ledger == nil {
		return SampleLedgerState{}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.stateLocked()
}

// ExportRestartState succeeds only after all live attempts were synchronously
// reclaimed, so no capability or attempt can be reconstructed after restart.
func (ledger *SampleLedger) ExportRestartState() (LedgerRestartState, error) {
	if ledger == nil {
		return LedgerRestartState{}, fmt.Errorf("sample ledger is invalid")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if len(ledger.attempts) != 0 || ledger.retainedClaims != 0 {
		return LedgerRestartState{}, fmt.Errorf("live ledger state is not restartable")
	}
	return LedgerRestartState{
		SchemaVersion:        1,
		NextTerminalSequence: ledger.nextTerminalSequence,
		SequenceExhausted:    ledger.sequenceExhausted,
		AuditTombstones: append(
			[]LedgerAuditTombstone(nil),
			ledger.auditTombstones...,
		),
	}, nil
}

// Snapshot reports bounded resource counts and non-reconstructing tombstones.
func (ledger *SampleLedger) Snapshot() LedgerSnapshot {
	if ledger == nil {
		return LedgerSnapshot{}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return LedgerSnapshot{
		RetainedAttempts:     len(ledger.attempts),
		RetainedClaimEntries: ledger.retainedClaims,
		NextTerminalSequence: ledger.nextTerminalSequence,
		SequenceExhausted:    ledger.sequenceExhausted,
		AuditTombstones: append(
			[]LedgerAuditTombstone(nil),
			ledger.auditTombstones...,
		),
	}
}

func (ledger *SampleLedger) reserveTerminalBatchLocked(
	claimCount int,
) (uint64, []uint64, error) {
	if !ledger.terminalBatchAvailableLocked(claimCount) {
		return 0, nil, fmt.Errorf("ledger terminal sequence is exhausted")
	}
	batch := uint64(claimCount) + 1
	first := ledger.nextTerminalSequence
	if batch-1 > math.MaxUint64-first {
		return 0, nil, fmt.Errorf("ledger terminal sequence is exhausted")
	}
	last := first + batch - 1
	claims := make([]uint64, claimCount)
	for index := range claims {
		claims[index] = first + uint64(index) + 1
	}
	if last == math.MaxUint64 {
		ledger.nextTerminalSequence = 0
		ledger.sequenceExhausted = true
	} else {
		ledger.nextTerminalSequence = last + 1
	}
	return first, claims, nil
}

func (ledger *SampleLedger) terminalBatchAvailableLocked(claimCount int) bool {
	if ledger.sequenceExhausted || ledger.nextTerminalSequence == 0 || claimCount <= 0 {
		return false
	}
	batch := uint64(claimCount) + 1
	return batch-1 <= math.MaxUint64-ledger.nextTerminalSequence
}

// PublishedAttemptV1 is the exact closed external publication projection.
type PublishedAttemptV1 struct {
	SchemaVersion           int    `json:"schema_version"`
	AttemptTerminalSequence uint64 `json:"attempt_terminal_sequence"`
	DependencySetDigest     string `json:"dependency_set_digest"`
	RuntimeDependencyCount  uint64 `json:"runtime_dependency_count"`
	ClaimOutcomeDigest      string `json:"claim_outcome_digest"`
}

// PublicationCommitRequest stages the durable sample state and the only
// externally publishable projection in one transaction.
type PublicationCommitRequest struct {
	ExpectedState  SampleLedgerState
	PublishedState SampleLedgerState
	Attempt        PublishedAttemptV1
}

// PublicationCommitDecision is the transaction's atomic outcome.
type PublicationCommitDecision string

const (
	PublicationCommitCommitted PublicationCommitDecision = "committed"
	PublicationCommitCancelled PublicationCommitDecision = "cancelled"
)

// PublicationCommitter must commit PublishedState, the external effect, and
// the published decision atomically. An error or cancelled result guarantees
// that no irreversible external effect occurred.
type PublicationCommitter interface {
	CommitPublication(
		context.Context,
		PublicationCommitRequest,
	) (PublicationCommitDecision, error)
}

// RuntimeObservationFacts are immutable envelope facts not supplied by M1.
type RuntimeObservationFacts struct {
	SourceValidity          SourceValidity
	SourceTime              SourceTimeSpec
	LocalReceiptTime        time.Time
	LocalReceiptTimePresent bool
}

// RuntimeAttemptRequest asks the factory to create and own one exact producer
// attempt before any acquisition is issued.
type RuntimeAttemptRequest struct {
	Source       *modbus.RuntimeAcquisitionSource
	AttemptKey   string
	Identity     AttemptIdentity
	Observation  RuntimeObservationFacts
	Dependencies []RuntimeDependencyFacts
	Diagnostics  []string
}

// ObservationFactory binds production attempts to one profile, ledger, and
// transactional publication boundary.
type ObservationFactory struct {
	profile             ProfileDescriptor
	ledger              *SampleLedger
	committer           PublicationCommitter
	dependencySetDigest string
	dependencyBytes     []byte
}

// NewObservationFactory activates production ingestion. Fixture replay uses
// the separate FixtureReplayer and never needs a committer.
func NewObservationFactory(
	profile ProfileDescriptor,
	ledger *SampleLedger,
	committer PublicationCommitter,
) (*ObservationFactory, error) {
	if ledger == nil || committer == nil {
		return nil, fmt.Errorf("observation factory requires a ledger and publication committer")
	}
	copy, err := NewProfileDescriptor(profile.Spec())
	if err != nil {
		return nil, fmt.Errorf("observation factory profile: %w", err)
	}
	if copy.spec.Kind != ProfileStandardFamily {
		return nil, fmt.Errorf("vendor overlay requires M3 resolution")
	}
	state := ledger.ExportState()
	if state.ProfileID != copy.ID() || state.ProfileVersion != copy.Version() ||
		state.DependencySetID != copy.Dependencies().ID() {
		return nil, fmt.Errorf("observation factory persistence domain disagrees")
	}
	if state.Revision > 0 {
		switch copy.spec.Coherence.Mode {
		case CoherenceSingleWireResponse:
			if state.LastCommittedAttempt.RetryOrdinal != 0 {
				return nil, fmt.Errorf("persisted attempt disagrees with profile mode")
			}
		case CoherenceBoundedMultiResponse:
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
		profile:             copy,
		ledger:              ledger,
		committer:           committer,
		dependencySetDigest: digest,
		dependencyBytes:     encoded,
	}, nil
}

func encodeRuntimeDependencySet(
	set DependencySet,
	maximum int,
) ([]byte, string, error) {
	dependencies := set.Dependencies()
	if set.ID() == "" || !set.Version().valid() || len(dependencies) == 0 || maximum <= 0 {
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
	mu                  sync.Mutex
	cond                *sync.Cond
	factory             *ObservationFactory
	key                 string
	identity            AttemptIdentity
	phase               AttemptPhase
	terminalSequence    uint64
	entries             []runtimeClaimEntry
	inProgressClaims    int
	source              *modbus.RuntimeAcquisitionSource
	producerAttempt     *modbus.RuntimeAttempt
	instance            modbus.RuntimeAttemptInstance
	admitted            bool
	acquisitions        []modbus.RuntimeAcquisition
	issued              []bool
	observationFacts    RuntimeObservationFacts
	dependencyFacts     []RuntimeDependencyFacts
	diagnostics         []string
	template            Observation
	publishCancel       context.CancelFunc
	publishAbort        chan struct{}
	publishAbortClosed  bool
	sourceDrainMu       sync.Mutex
	sourceDrainComplete bool
	sourceDrainErr      error
	cancelOpen          func() error
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
			[]RuntimeDependencyFacts(nil),
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
		state.phase = AttemptCancelled
		state.source = nil
		state.cond.Broadcast()
		state.mu.Unlock()
		state.factory.ledger.reclaimTerminalAttempt(state, AttemptCancelled)
		return fmt.Errorf("close retained producer runtime attempt: %w", err)
	}
	state.instance = instance
	state.admitted = true
	state.issued = nil
	request := RuntimeAttemptRequest{
		Identity:     state.identity,
		Observation:  state.observationFacts,
		Dependencies: append([]RuntimeDependencyFacts(nil), state.dependencyFacts...),
	}
	template, validationErr := state.factory.buildRuntimeObservation(
		state.key,
		retained,
		request,
		state.factory.profile.Dependencies().Dependencies(),
	)
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
		attempt.finishCancellation(AttemptCancelFailed)
		return fmt.Errorf(
			"runtime admission validation failed (%v) and source drain failed: %w",
			validationErr,
			drainErr,
		)
	}
	attempt.finishCancellation(AttemptCancelled)
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

func (factory *ObservationFactory) validateAttemptIdentity(identity AttemptIdentity) error {
	if identity.PollGenerationID == 0 {
		return fmt.Errorf("attempt poll generation is absent")
	}
	switch factory.profile.spec.Coherence.Mode {
	case CoherenceSingleWireResponse:
		if identity.RetryOrdinal != 0 {
			return fmt.Errorf("single-wire retry identity is not applicable")
		}
	case CoherenceBoundedMultiResponse:
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

func validateRetainedDiagnostics(values []string, limits LedgerLimits) error {
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
	case SourceValid, SourceInvalid, SourceNotImplemented, SourceReserved:
	default:
		return fmt.Errorf("runtime observation source validity is unknown")
	}
	if err := validateSourceTime(request.Observation.SourceTime); err != nil {
		return fmt.Errorf("runtime observation source time: %w", err)
	}
	if _, err := canonicalRequiredTime(
		request.Observation.LocalReceiptTime,
		request.Observation.LocalReceiptTimePresent,
	); err != nil {
		return fmt.Errorf("runtime observation receipt time: %w", err)
	}
	if err := preflightAggregate(request.Dependencies); err != nil {
		return fmt.Errorf("runtime dependency facts: %w", err)
	}
	for index, facts := range request.Dependencies {
		if err := validateSourceTime(facts.SourceTime); err != nil {
			return fmt.Errorf("runtime dependency %d source time: %w", index, err)
		}
		if hasLocalReceiptTime(
			facts.LocalReceiptTime,
			facts.LocalReceiptTimePresent,
		) {
			if _, err := canonicalRequiredTime(
				facts.LocalReceiptTime,
				facts.LocalReceiptTimePresent,
			); err != nil {
				return fmt.Errorf("runtime dependency %d receipt time: %w", index, err)
			}
		}
		if err := validateBoundedString(
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
	declarations []Dependency,
) (Observation, error) {
	results := make([]DependencyResult, len(declarations))
	runtimeBytes := make([][]byte, len(declarations))
	runtimeFields := make([]modbus.RuntimeNormalizationFields, len(declarations))
	var endpoint string
	var unitID byte
	for index, declaration := range declarations {
		acquisition := acquisitions[index]
		if !acquisition.Valid() || acquisition.AttemptKey() != attemptKey {
			return Observation{}, fmt.Errorf("runtime acquisition %d changed identity", index)
		}
		normalization := acquisition.Normalization()
		if !normalization.Valid() {
			return Observation{}, fmt.Errorf("runtime acquisition %d lacks normalization", index)
		}
		encoded := normalization.Bytes()
		if len(encoded) == 0 ||
			len(encoded) > factory.ledger.limits.NormalizationRecordMaxEncodedBytes {
			return Observation{}, fmt.Errorf("runtime normalization %d exceeds byte bound", index)
		}
		fields := normalization.Fields()
		if err := matchRuntimeNormalization(declaration, fields); err != nil {
			return Observation{}, fmt.Errorf("runtime normalization %d: %w", index, err)
		}
		provenance := acquisition.Provenance()
		if provenance.SourceKind != modbus.RuntimeAcquisitionSourceRuntime ||
			provenance.SourceEvidenceID != fields.SourceEvidenceID ||
			len(provenance.WireResponseBytes) == 0 {
			return Observation{}, fmt.Errorf("runtime acquisition %d provenance is incomplete", index)
		}
		snapshot, err := NewLogicalViewSnapshot(LogicalViewRecord{
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
			return Observation{}, fmt.Errorf("runtime dependency %d view: %w", index, err)
		}
		if provenance.PollGeneration != request.Identity.PollGenerationID {
			return Observation{}, fmt.Errorf("runtime dependency %d poll generation disagrees", index)
		}
		facts := request.Dependencies[index]
		results[index] = DependencyResult{
			DependencyID:                 declaration.ID(),
			DependencyVersion:            declaration.Version(),
			CodecID:                      declaration.CodecID(),
			CodecVersion:                 declaration.CodecVersion(),
			NormalizationVersion:         declaration.Normalization().Spec().Version,
			Status:                       DependencyReadSuccessful,
			View:                         snapshot,
			SourceTime:                   facts.SourceTime,
			LocalReceiptTime:             facts.LocalReceiptTime,
			DocumentaryConsistencyMarker: facts.DocumentaryConsistencyMarker,
			AcquisitionOrdinal:           facts.AcquisitionOrdinal,
			RetryOrdinal:                 request.Identity.RetryOrdinal,
			localReceiptTimePresent: facts.LocalReceiptTimePresent ||
				!facts.LocalReceiptTime.IsZero(),
		}
		runtimeBytes[index] = append([]byte(nil), encoded...)
		runtimeFields[index] = fields
		if index == 0 {
			endpoint = provenance.Endpoint
			unitID = provenance.UnitID
		}
	}
	spec := ObservationSpec{
		SchemaVersion:          schemaVersionV1,
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
		LocalReceiptTime:       request.Observation.LocalReceiptTime,
		Endpoint:               endpoint,
		UnitID:                 unitID,
		Dependencies:           results,
		localReceiptTimePresent: request.Observation.LocalReceiptTimePresent ||
			!request.Observation.LocalReceiptTime.IsZero(),
	}
	validated, err := buildObservation(factory.profile, spec)
	if err != nil {
		return Observation{}, err
	}
	for index := range validated.replayed {
		validated.replayed[index].runtimeNormalizationBytes = runtimeBytes[index]
		validated.replayed[index].runtimeNormalizationFields = runtimeFields[index]
	}
	return validated, nil
}

func matchRuntimeNormalization(
	declaration Dependency,
	fields modbus.RuntimeNormalizationFields,
) error {
	normalization := declaration.Normalization().Spec()
	expectedFunction := modbus.FunctionReadHoldingRegisters
	if declaration.Table() == InputRegisters {
		expectedFunction = modbus.FunctionReadInputRegisters
	}
	if fields.SchemaVersion != 1 ||
		fields.SourceKind != modbus.RuntimeAcquisitionSourceRuntime ||
		fields.SourceEvidenceID != normalization.SourceLocator ||
		fields.DocumentaryNotation != normalization.DocumentaryNotation ||
		fields.DocumentaryAddress != normalization.DocumentaryAddress ||
		fields.DocumentaryAddressBase != string(normalization.DocumentaryBase) ||
		fields.FunctionCode != expectedFunction ||
		fields.LogicalTable != declaration.Table() ||
		fields.NormalizedZeroBasedPDUOffset != normalization.ResolvedPDUOffset ||
		fields.WordCount != declaration.WordCount() {
		return fmt.Errorf("producer fields disagree with the declared dependency")
	}
	return nil
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

func (attempt *ObservationAttempt) finishCancellation(outcome AttemptPhase) {
	state := attempt.state
	state.mu.Lock()
	if state.phase == AttemptCancelling {
		state.cancelUnresolvedLocked()
		state.phase = outcome
	}
	factory := state.factory
	state.cond.Broadcast()
	state.mu.Unlock()
	if factory != nil {
		factory.ledger.reclaimTerminalAttempt(state, outcome)
	}
}

// Cancel performs the one-shot claim drain and exact producer cancellation.
func (attempt *ObservationAttempt) Cancel() (CancellationResult, error) {
	if attempt == nil || attempt.state == nil {
		return "", fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
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
			if state.publishAbort != nil && !state.publishAbortClosed {
				close(state.publishAbort)
				state.publishAbortClosed = true
			}
			if state.publishCancel != nil {
				state.publishCancel()
			}
			state.cond.Wait()
			continue
		case AttemptCancelling:
			state.cond.Wait()
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
			attempt.finishCancellation(AttemptCancelFailed)
			return CancellationFailed, fmt.Errorf("producer runtime attempt is unavailable")
		}
		_, closeErr := producerAttempt.Close(nil)
		if closeErr != nil &&
			!errors.Is(closeErr, modbus.ErrRuntimeAttemptMembership) &&
			!errors.Is(closeErr, modbus.ErrRuntimeAttemptClosed) {
			attempt.finishCancellation(AttemptCancelFailed)
			return CancellationFailed, fmt.Errorf("close unadmitted producer attempt: %w", closeErr)
		}
		state.mu.Lock()
		state.source = nil
		state.mu.Unlock()
		attempt.finishCancellation(AttemptCancelled)
		return CancellationCompleted, nil
	}
	if err := state.drainSource(); err != nil {
		attempt.finishCancellation(AttemptCancelFailed)
		return CancellationFailed, fmt.Errorf("cancel producer runtime attempt: %w", err)
	}
	attempt.finishCancellation(AttemptCancelled)
	return CancellationCompleted, nil
}

// Publish consumes only sealed immutable ledger state and invokes the
// transactional commit boundary exactly once.
func (attempt *ObservationAttempt) Publish(
	ctx context.Context,
) (Observation, error) {
	if attempt == nil || attempt.state == nil || ctx == nil {
		return Observation{}, fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	if state.phase != AttemptSealed {
		state.mu.Unlock()
		return Observation{}, fmt.Errorf("observation attempt is not sealed")
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
		attempt.finishPublication(AttemptPublishFailed)
		return Observation{}, fmt.Errorf("drain producer runtime attempt: %w", err)
	}
	ledger := factory.ledger
	select {
	case <-ctx.Done():
		attempt.finishPublication(AttemptPublishFailed)
		return Observation{}, fmt.Errorf("publication context ended before commit admission: %w", ctx.Err())
	case <-publishAbort:
		attempt.finishPublication(AttemptPublishFailed)
		return Observation{}, fmt.Errorf("publication was cancelled before commit admission")
	case <-ledger.commitSerial:
	}
	defer func() { ledger.commitSerial <- struct{}{} }()
	state.mu.Lock()
	aborted := state.publishAbortClosed
	contextErr := ctx.Err()
	state.mu.Unlock()
	if aborted {
		attempt.finishPublication(AttemptPublishFailed)
		return Observation{}, fmt.Errorf("publication was cancelled before commit admission")
	}
	if contextErr != nil {
		attempt.finishPublication(AttemptPublishFailed)
		return Observation{}, fmt.Errorf("publication context ended before commit admission: %w", contextErr)
	}
	ledger.mu.Lock()
	if ledger.revision == math.MaxUint64 || ledger.highWater == math.MaxUint64 ||
		!attemptIdentityAdvances(ledger.lastCommittedAttempt, state.identity) {
		ledger.mu.Unlock()
		attempt.finishPublication(AttemptPublishFailed)
		return Observation{}, fmt.Errorf("sample ledger is exhausted or attempt does not advance")
	}
	expected := ledger.stateLocked()
	next := expected
	next.Revision++
	next.HighWater++
	next.LastCommittedAttempt = state.identity
	ledger.mu.Unlock()
	projection := state.publishedProjection()
	decision, err := factory.committer.CommitPublication(
		publishContext,
		PublicationCommitRequest{
			ExpectedState:  expected,
			PublishedState: next,
			Attempt:        projection,
		},
	)
	if err != nil || decision != PublicationCommitCommitted {
		attempt.finishPublication(AttemptPublishFailed)
		if err != nil {
			return Observation{}, fmt.Errorf("transactional publication: %w", err)
		}
		return Observation{}, fmt.Errorf("transactional publication was cancelled")
	}
	ledger.mu.Lock()
	if ledger.stateLocked() != expected {
		ledger.mu.Unlock()
		attempt.finishPublication(AttemptPublishFailed)
		return Observation{}, fmt.Errorf("sample ledger changed during publication commit")
	}
	ledger.revision = next.Revision
	ledger.highWater = next.HighWater
	ledger.lastCommittedAttempt = next.LastCommittedAttempt
	ledger.mu.Unlock()
	observation := state.template
	observation.spec.SampleID = fmt.Sprintf("%s:%d", ledger.issuerDomain, next.HighWater)
	attempt.finishPublication(AttemptPublished)
	return observation, nil
}

func (attempt *ObservationAttempt) finishPublication(outcome AttemptPhase) {
	state := attempt.state
	state.mu.Lock()
	if state.phase == AttemptPublishing {
		state.phase = outcome
	}
	factory := state.factory
	state.publishCancel = nil
	state.publishAbort = nil
	state.publishAbortClosed = false
	state.cond.Broadcast()
	state.mu.Unlock()
	if factory != nil {
		factory.ledger.reclaimTerminalAttempt(state, outcome)
	}
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

func attemptIdentityAdvances(previous, next AttemptIdentity) bool {
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

func (ledger *SampleLedger) reclaimTerminalAttempt(
	state *runtimeAttemptState,
	outcome AttemptPhase,
) {
	if ledger == nil || state == nil || !validAttemptTerminalOutcome(outcome) {
		return
	}
	state.mu.Lock()
	if state.inProgressClaims != 0 || state.phase != outcome {
		state.mu.Unlock()
		return
	}
	key := state.key
	entries := append([]runtimeClaimEntry(nil), state.entries...)
	attemptSequence := state.terminalSequence
	state.mu.Unlock()

	ledger.mu.Lock()
	if ledger.attempts[key] != state {
		ledger.mu.Unlock()
		return
	}
	delete(ledger.attempts, key)
	ledger.retainedClaims -= len(entries)
	for ordinal, entry := range entries {
		if entry.phase != runtimeClaimTerminal || !validClaimOutcome(entry.outcome) {
			continue
		}
		ledger.insertAuditTombstoneLocked(LedgerAuditTombstone{
			SchemaVersion:           1,
			ObjectKind:              LedgerAuditClaim,
			TerminalSequence:        entry.terminalSequence,
			AttemptTerminalSequence: attemptSequence,
			ClaimOrdinal:            uint64(ordinal),
			TerminalOutcome:         string(entry.outcome),
		})
	}
	ledger.insertAuditTombstoneLocked(LedgerAuditTombstone{
		SchemaVersion:    1,
		ObjectKind:       LedgerAuditAttempt,
		TerminalSequence: attemptSequence,
		TerminalOutcome:  string(outcome),
	})
	ledger.mu.Unlock()

	state.mu.Lock()
	state.factory = nil
	state.key = ""
	state.identity = AttemptIdentity{}
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
	state.template = Observation{}
	state.publishCancel = nil
	state.publishAbort = nil
	state.publishAbortClosed = false
	state.sourceDrainComplete = false
	state.sourceDrainErr = nil
	state.cancelOpen = nil
	state.mu.Unlock()
}

func (ledger *SampleLedger) insertAuditTombstoneLocked(
	tombstone LedgerAuditTombstone,
) {
	index := sort.Search(len(ledger.auditTombstones), func(index int) bool {
		return ledger.auditTombstones[index].TerminalSequence >
			tombstone.TerminalSequence
	})
	ledger.auditTombstones = append(
		ledger.auditTombstones,
		LedgerAuditTombstone{},
	)
	copy(ledger.auditTombstones[index+1:], ledger.auditTombstones[index:])
	ledger.auditTombstones[index] = tombstone
	if len(ledger.auditTombstones) > ledger.limits.AuditTombstoneLimit {
		copy(ledger.auditTombstones, ledger.auditTombstones[1:])
		ledger.auditTombstones =
			ledger.auditTombstones[:len(ledger.auditTombstones)-1]
	}
}
