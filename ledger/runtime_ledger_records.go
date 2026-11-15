package ledger

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// ClaimOutcome is the closed immutable M2 claim terminal enum.
type ClaimOutcome string

// The claim outcomes. ClaimSucceeded is the only success; every other outcome
// is a terminal non-success.
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

// The attempt phases. Open, sealed, cancelling and publishing are live phases;
// published, publish-failed, cancel-failed and cancelled are terminal.
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

// The cancellation results Cancel reports.
const (
	CancellationCompleted        CancellationResult = "cancelled"
	CancellationFailed           CancellationResult = "cancel_failed"
	CancellationPublishFailed    CancellationResult = "publish_failed"
	CancellationAlreadyPublished CancellationResult = "already_published"
)

// AuditObjectKind is the closed tombstone variant discriminator.
type AuditObjectKind string

// The audit tombstone kinds: one attempt tombstone per terminal attempt and one
// claim tombstone per terminal claim of that attempt.
const (
	AuditAttempt AuditObjectKind = "attempt"
	AuditClaim   AuditObjectKind = "claim"
)

// AuditTombstone is a bounded, immutable, non-reconstructing audit
// record. Claim-only fields are absent from attempt JSON encodings.
type AuditTombstone struct {
	SchemaVersion           int             `json:"schema_version"`
	ObjectKind              AuditObjectKind `json:"object_kind"`
	TerminalSequence        uint64          `json:"terminal_sequence"`
	ClaimCount              uint64          `json:"claim_count,omitempty"`
	AttemptTerminalSequence uint64          `json:"attempt_terminal_sequence,omitempty"`
	ClaimOrdinal            uint64          `json:"claim_ordinal,omitempty"`
	TerminalOutcome         string          `json:"terminal_outcome"`
}

const maxLedgerAuditTombstoneJSONBytes = 256

type attemptAuditTombstoneDTO struct {
	SchemaVersion    int             `json:"schema_version"`
	ObjectKind       AuditObjectKind `json:"object_kind"`
	TerminalSequence uint64          `json:"terminal_sequence"`
	ClaimCount       uint64          `json:"claim_count"`
	TerminalOutcome  string          `json:"terminal_outcome"`
}

type claimAuditTombstoneDTO struct {
	SchemaVersion           int             `json:"schema_version"`
	ObjectKind              AuditObjectKind `json:"object_kind"`
	TerminalSequence        uint64          `json:"terminal_sequence"`
	AttemptTerminalSequence uint64          `json:"attempt_terminal_sequence"`
	ClaimOrdinal            uint64          `json:"claim_ordinal"`
	TerminalOutcome         string          `json:"terminal_outcome"`
}

func validateLedgerAuditTombstone(tombstone AuditTombstone) error {
	switch tombstone.ObjectKind {
	case AuditAttempt:
		if !validAttemptTerminalOutcome(AttemptPhase(tombstone.TerminalOutcome)) ||
			tombstone.SchemaVersion != 1 || tombstone.TerminalSequence == 0 ||
			tombstone.ClaimCount == 0 ||
			tombstone.AttemptTerminalSequence != 0 || tombstone.ClaimOrdinal != 0 {
			return fmt.Errorf("attempt audit tombstone is invalid")
		}
	case AuditClaim:
		if !validClaimOutcome(ClaimOutcome(tombstone.TerminalOutcome)) ||
			tombstone.SchemaVersion != 1 || tombstone.TerminalSequence == 0 ||
			tombstone.AttemptTerminalSequence == 0 || tombstone.ClaimCount != 0 {
			return fmt.Errorf("claim audit tombstone is invalid")
		}
	default:
		return fmt.Errorf("ledger audit tombstone kind is invalid")
	}
	return nil
}

// MarshalJSON validates the tombstone and encodes its closed variant: attempt
// tombstones carry the claim count, claim tombstones the attempt sequence and
// claim ordinal. An invalid tombstone fails without output.
func (tombstone AuditTombstone) MarshalJSON() ([]byte, error) {
	if err := validateLedgerAuditTombstone(tombstone); err != nil {
		return nil, err
	}
	switch tombstone.ObjectKind {
	case AuditAttempt:
		return json.Marshal(attemptAuditTombstoneDTO{
			SchemaVersion:    1,
			ObjectKind:       AuditAttempt,
			TerminalSequence: tombstone.TerminalSequence,
			ClaimCount:       tombstone.ClaimCount,
			TerminalOutcome:  tombstone.TerminalOutcome,
		})
	case AuditClaim:
		return json.Marshal(claimAuditTombstoneDTO{
			SchemaVersion:           1,
			ObjectKind:              AuditClaim,
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
func (tombstone *AuditTombstone) UnmarshalJSON(data []byte) error {
	if tombstone == nil || len(data) == 0 || len(data) > maxLedgerAuditTombstoneJSONBytes {
		return fmt.Errorf("ledger audit tombstone encoding exceeds its bound")
	}
	var discriminator struct {
		ObjectKind AuditObjectKind `json:"object_kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	var decoded AuditTombstone
	switch discriminator.ObjectKind {
	case AuditAttempt:
		var record attemptAuditTombstoneDTO
		if err := modbusreg.DecodeStrict(data, &record); err != nil {
			return err
		}
		decoded = AuditTombstone{
			SchemaVersion:    record.SchemaVersion,
			ObjectKind:       record.ObjectKind,
			TerminalSequence: record.TerminalSequence,
			ClaimCount:       record.ClaimCount,
			TerminalOutcome:  record.TerminalOutcome,
		}
	case AuditClaim:
		var record claimAuditTombstoneDTO
		if err := modbusreg.DecodeStrict(data, &record); err != nil {
			return err
		}
		decoded = AuditTombstone{
			SchemaVersion:           record.SchemaVersion,
			ObjectKind:              record.ObjectKind,
			TerminalSequence:        record.TerminalSequence,
			AttemptTerminalSequence: record.AttemptTerminalSequence,
			ClaimOrdinal:            record.ClaimOrdinal,
			TerminalOutcome:         record.TerminalOutcome,
		}
	default:
		return fmt.Errorf("ledger audit tombstone kind is invalid")
	}
	if err := validateLedgerAuditTombstone(decoded); err != nil {
		return err
	}
	*tombstone = decoded
	return nil
}

// RestartState is the bounded deterministic audit/sequence restart
// state. Live attempts and capabilities are intentionally nonserializable.
type RestartState struct {
	SchemaVersion            int              `json:"schema_version"`
	NextTerminalSequence     uint64           `json:"next_terminal_sequence"`
	SequenceExhausted        bool             `json:"sequence_exhausted"`
	TruncatedThroughSequence uint64           `json:"truncated_through_sequence"`
	AuditTombstones          []AuditTombstone `json:"audit_tombstones"`
}

// CoversTerminalWatermark reports whether restart prevents reuse of every
// terminal sequence already covered by prior without contradicting any
// overlapping retained history. History fully below restart's truncation
// watermark is intentionally non-reconstructing and needs no comparison.
func (restart RestartState) CoversTerminalWatermark(prior RestartState) bool {
	limits := maximumLedgerRestartValidationLimits()
	if validateLedgerRestartState(restart, limits) != nil ||
		validateLedgerRestartState(prior, limits) != nil {
		return false
	}
	if prior.SequenceExhausted && !restart.SequenceExhausted {
		return false
	}
	if !restart.SequenceExhausted && !prior.SequenceExhausted &&
		restart.NextTerminalSequence < prior.NextTerminalSequence {
		return false
	}
	current := make(map[uint64]AuditTombstone, len(restart.AuditTombstones))
	for _, tombstone := range restart.AuditTombstones {
		current[tombstone.TerminalSequence] = tombstone
	}
	for _, tombstone := range prior.AuditTombstones {
		if tombstone.TerminalSequence <= restart.TruncatedThroughSequence {
			continue
		}
		candidate, exists := current[tombstone.TerminalSequence]
		if !exists || candidate != tombstone {
			return false
		}
	}
	return true
}

type ledgerRestartStateDTO struct {
	SchemaVersion            int              `json:"schema_version"`
	NextTerminalSequence     uint64           `json:"next_terminal_sequence"`
	SequenceExhausted        bool             `json:"sequence_exhausted"`
	TruncatedThroughSequence uint64           `json:"truncated_through_sequence"`
	AuditTombstones          []AuditTombstone `json:"audit_tombstones"`
}

func maximumLedgerRestartValidationLimits() modbusreg.LedgerLimits {
	limits := modbusreg.DefaultLedgerLimits()
	limits.MaxClaimEntriesPerAttempt = modbusreg.MaxProfileDependencies
	limits.AuditTombstoneLimit = modbusreg.MaxLedgerRestartAuditTombstones
	limits.AuditTombstoneMaxEncodedBytes = modbusreg.MaxSerializedContractBytes
	return limits
}

// MarshalJSON emits the exact bounded snake_case restart contract.
func (restart RestartState) MarshalJSON() ([]byte, error) {
	if err := validateLedgerRestartState(
		restart,
		maximumLedgerRestartValidationLimits(),
	); err != nil {
		return nil, err
	}
	return modbusreg.MarshalBounded(ledgerRestartStateDTO{
		SchemaVersion:            restart.SchemaVersion,
		NextTerminalSequence:     restart.NextTerminalSequence,
		SequenceExhausted:        restart.SequenceExhausted,
		TruncatedThroughSequence: restart.TruncatedThroughSequence,
		AuditTombstones:          nonNilLedgerAuditTombstones(restart.AuditTombstones),
	})
}

// UnmarshalJSON accepts only the exact closed top-level restart object.
func (restart *RestartState) UnmarshalJSON(data []byte) error {
	if restart == nil {
		return fmt.Errorf("ledger restart state target is nil")
	}
	var record ledgerRestartStateDTO
	if err := modbusreg.DecodeStrict(data, &record); err != nil {
		return err
	}
	decoded := RestartState{
		SchemaVersion:            record.SchemaVersion,
		NextTerminalSequence:     record.NextTerminalSequence,
		SequenceExhausted:        record.SequenceExhausted,
		TruncatedThroughSequence: record.TruncatedThroughSequence,
		AuditTombstones:          append([]AuditTombstone(nil), record.AuditTombstones...),
	}
	if err := validateLedgerRestartState(
		decoded,
		maximumLedgerRestartValidationLimits(),
	); err != nil {
		return err
	}
	*restart = decoded
	return nil
}

func nonNilLedgerAuditTombstones(
	values []AuditTombstone,
) []AuditTombstone {
	if values == nil {
		return []AuditTombstone{}
	}
	return values
}

// Snapshot reports bounded resource use without exposing attempt keys,
// capabilities, normalization records, or diagnostics.
type Snapshot struct {
	RetainedAttempts     int
	RetainedClaimEntries int
	NextTerminalSequence uint64
	SequenceExhausted    bool
	AuditTombstones      []AuditTombstone
}

// SampleLedger owns sample sequencing plus the bounded M2 attempt ledger.
type SampleLedger struct {
	mu                          sync.Mutex
	commitSerial                chan struct{}
	limits                      modbusreg.LedgerLimits
	issuerDomain                string
	profileID                   string
	profileVersion              modbusreg.Version
	dependencySetID             string
	revision                    uint64
	highWater                   uint64
	lastCommittedAttempt        modbusreg.AttemptIdentity
	attempts                    map[string]*runtimeAttemptState
	retainedClaims              int
	nextTerminalSequence        uint64
	sequenceExhausted           bool
	durableNextTerminalSequence uint64
	durableSequenceExhausted    bool
	truncatedThroughSequence    uint64
	auditTombstones             []AuditTombstone
}

// NewSampleLedger activates a new bounded attempt ledger. Omitting limits is
// retained for source compatibility and selects DefaultLedgerLimits.
func NewSampleLedger(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
	configuredLimits ...modbusreg.LedgerLimits,
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
	limits modbusreg.LedgerLimits,
	restart RestartState,
) (*SampleLedger, error) {
	return newSampleLedger(state, trustedMinimumRevision, limits, &restart)
}

func selectLedgerLimits(configured []modbusreg.LedgerLimits) (modbusreg.LedgerLimits, error) {
	if len(configured) > 1 {
		return modbusreg.LedgerLimits{}, fmt.Errorf("sample ledger accepts one limits value")
	}
	limits := modbusreg.DefaultLedgerLimits()
	if len(configured) == 1 {
		limits = configured[0]
	}
	if err := modbusreg.ValidateLedgerLimits(limits); err != nil {
		return modbusreg.LedgerLimits{}, err
	}
	return limits, nil
}

func newSampleLedger(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
	limits modbusreg.LedgerLimits,
	restart *RestartState,
) (*SampleLedger, error) {
	if err := modbusreg.ValidateLedgerLimits(limits); err != nil {
		return nil, err
	}
	const restartEnvelopeBudget = 512
	perTombstoneBudget := limits.AuditTombstoneMaxEncodedBytes + 1
	if limits.AuditTombstoneLimit >
		(modbusreg.MaxSerializedContractBytes-restartEnvelopeBudget)/perTombstoneBudget {
		return nil, fmt.Errorf("ledger audit limits exceed the aggregate serialization bound")
	}
	if err := validateLargestLedgerTombstones(limits); err != nil {
		return nil, err
	}
	if err := validateSampleLedgerState(state, trustedMinimumRevision); err != nil {
		return nil, err
	}
	if state.Revision != 0 && restart == nil {
		return nil, fmt.Errorf("committed sample state requires terminal restart metadata")
	}
	nextSequence := uint64(1)
	durableNextSequence := uint64(1)
	var tombstones []AuditTombstone
	sequenceExhausted := false
	durableSequenceExhausted := false
	var truncatedThrough uint64
	if restart != nil {
		if err := validateLedgerRestartState(*restart, limits); err != nil {
			return nil, err
		}
		if err := validateSampleRestartRelationship(state, *restart); err != nil {
			return nil, err
		}
		nextSequence = restart.NextTerminalSequence
		durableNextSequence = restart.NextTerminalSequence
		sequenceExhausted = restart.SequenceExhausted
		durableSequenceExhausted = restart.SequenceExhausted
		truncatedThrough = restart.TruncatedThroughSequence
		tombstones = append([]AuditTombstone(nil), restart.AuditTombstones...)
	}
	ledger := &SampleLedger{
		commitSerial:                make(chan struct{}, 1),
		limits:                      limits,
		issuerDomain:                state.IssuerDomain,
		profileID:                   state.ProfileID,
		profileVersion:              state.ProfileVersion,
		dependencySetID:             state.DependencySetID,
		revision:                    state.Revision,
		highWater:                   state.HighWater,
		lastCommittedAttempt:        state.LastCommittedAttempt,
		attempts:                    make(map[string]*runtimeAttemptState),
		nextTerminalSequence:        nextSequence,
		sequenceExhausted:           sequenceExhausted,
		durableNextTerminalSequence: durableNextSequence,
		durableSequenceExhausted:    durableSequenceExhausted,
		truncatedThroughSequence:    truncatedThrough,
		auditTombstones:             tombstones,
	}
	ledger.commitSerial <- struct{}{}
	return ledger, nil
}

func validateSampleRestartRelationship(
	state SampleLedgerState,
	restart RestartState,
) error {
	var retainedPublications uint64
	for _, tombstone := range restart.AuditTombstones {
		if tombstone.ObjectKind == AuditAttempt &&
			tombstone.TerminalOutcome == string(AttemptPublished) {
			if retainedPublications == math.MaxUint64 {
				return fmt.Errorf("retained publication history exceeds sample revision capacity")
			}
			retainedPublications++
		}
	}
	if restart.TruncatedThroughSequence == 0 {
		if retainedPublications != state.Revision {
			return fmt.Errorf("complete terminal history disagrees with sample revision")
		}
	} else if retainedPublications > state.Revision {
		return fmt.Errorf("retained publication history exceeds sample revision")
	}
	if state.Revision == 0 {
		return nil
	}
	// Every committed publication consumes one attempt sequence and at least
	// one claim sequence. Exact claim cardinality is profile-specific and is
	// intentionally not reconstructed from SampleLedgerState.
	if state.Revision > math.MaxUint64/2 {
		return fmt.Errorf("committed sample history exceeds terminal sequence capacity")
	}
	if restart.SequenceExhausted {
		return nil
	}
	minimumConsumed := state.Revision * 2
	if restart.NextTerminalSequence-1 < minimumConsumed {
		return fmt.Errorf("terminal restart watermark predates committed publications")
	}
	return nil
}

// SampleLedgerState is the O(1) explicit restart and CAS boundary of one
// issuer domain, profile and dependency set: the CAS revision, the high-water
// mark of issued sample IDs and the last committed attempt.
type SampleLedgerState struct {
	SchemaVersion        modbusreg.Version
	IssuerDomain         string
	ProfileID            string
	ProfileVersion       modbusreg.Version
	DependencySetID      string
	Revision             uint64
	HighWater            uint64
	LastCommittedAttempt modbusreg.AttemptIdentity
}

// EmptySampleLedgerState creates a new issuer/profile/dependency-set domain.
func EmptySampleLedgerState(
	issuerDomain string,
	profile modbusreg.ProfileDescriptor,
) (SampleLedgerState, error) {
	if profile.ID() == "" {
		return SampleLedgerState{}, fmt.Errorf("sample ledger profile is invalid")
	}
	state := SampleLedgerState{
		SchemaVersion:   modbusreg.CurrentSchemaVersion(),
		IssuerDomain:    issuerDomain,
		ProfileID:       profile.ID(),
		ProfileVersion:  profile.Version(),
		DependencySetID: profile.Dependencies().ID(),
	}
	if err := validateSampleLedgerState(state, 0); err != nil {
		return SampleLedgerState{}, err
	}
	return state, nil
}

func validateSampleLedgerState(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
) error {
	initialAttemptState := state.LastCommittedAttempt == (modbusreg.AttemptIdentity{})
	if state.SchemaVersion != modbusreg.CurrentSchemaVersion() ||
		!validIssuerDomain(state.IssuerDomain) ||
		!modbusreg.ValidIdentity(state.ProfileID) ||
		!state.ProfileVersion.Valid() ||
		!modbusreg.ValidDependencySetID(state.DependencySetID) ||
		state.Revision != state.HighWater ||
		state.Revision < trustedMinimumRevision ||
		(state.Revision == 0 && !initialAttemptState) ||
		(state.Revision != 0 && state.LastCommittedAttempt.PollGenerationID == 0) ||
		(state.Revision != 0 &&
			state.LastCommittedAttempt.PollGenerationID < state.Revision) ||
		(state.LastCommittedAttempt.PollGenerationID == 0 &&
			state.LastCommittedAttempt.RetryOrdinal != 0) {
		return fmt.Errorf("sample ledger state is incomplete, stale, or incompatible")
	}
	return nil
}

func validIssuerDomain(value string) bool {
	if len(value) > modbusreg.MaxSampleIssuerDomainBytes || !modbusreg.ValidIdentity(value) {
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
	restart RestartState,
	limits modbusreg.LedgerLimits,
) error {
	if restart.SchemaVersion != 1 ||
		len(restart.AuditTombstones) > limits.AuditTombstoneLimit ||
		(restart.SequenceExhausted && restart.NextTerminalSequence != 0) ||
		(!restart.SequenceExhausted && restart.NextTerminalSequence == 0) {
		return fmt.Errorf("ledger restart state is invalid")
	}
	var endSequence uint64
	if restart.SequenceExhausted {
		endSequence = math.MaxUint64
	} else {
		endSequence = restart.NextTerminalSequence - 1
	}
	if restart.TruncatedThroughSequence > endSequence {
		return fmt.Errorf("ledger restart truncation watermark is invalid")
	}
	if len(restart.AuditTombstones) == 0 {
		if restart.TruncatedThroughSequence != endSequence {
			return fmt.Errorf("ledger restart empty history lacks a truncation watermark")
		}
		return nil
	}
	if restart.TruncatedThroughSequence == math.MaxUint64 {
		return fmt.Errorf("ledger restart history follows an exhausted truncation watermark")
	}
	var previous uint64
	tombstoneBySequence := make(
		map[uint64]AuditTombstone,
		len(restart.AuditTombstones),
	)
	for index, tombstone := range restart.AuditTombstones {
		encoded, err := json.Marshal(tombstone)
		if err != nil || len(encoded) > limits.AuditTombstoneMaxEncodedBytes ||
			(index == 0 && tombstone.TerminalSequence !=
				restart.TruncatedThroughSequence+1) ||
			(index > 0 && (previous == math.MaxUint64 ||
				tombstone.TerminalSequence != previous+1)) {
			return fmt.Errorf("ledger restart tombstone is invalid")
		}
		tombstoneBySequence[tombstone.TerminalSequence] = tombstone
		previous = tombstone.TerminalSequence
	}
	if previous != endSequence {
		return fmt.Errorf("ledger restart history does not end at the watermark")
	}
	var leadingAttemptSequence uint64
	var leadingClaimOrdinal uint64
	retainedAttemptSeen := false
	for _, tombstone := range restart.AuditTombstones {
		if tombstone.ObjectKind == AuditAttempt {
			retainedAttemptSeen = true
			continue
		}
		if tombstone.ObjectKind != AuditClaim ||
			tombstone.AttemptTerminalSequence > restart.TruncatedThroughSequence {
			continue
		}
		if retainedAttemptSeen {
			return fmt.Errorf("ledger restart truncated claim follows retained attempts")
		}
		if leadingAttemptSequence == 0 {
			leadingAttemptSequence = tombstone.AttemptTerminalSequence
			leadingClaimOrdinal = tombstone.ClaimOrdinal
			continue
		}
		if tombstone.AttemptTerminalSequence != leadingAttemptSequence ||
			leadingClaimOrdinal == math.MaxUint64 ||
			tombstone.ClaimOrdinal != leadingClaimOrdinal+1 {
			return fmt.Errorf("ledger restart truncated claim suffix is incoherent")
		}
		leadingClaimOrdinal = tombstone.ClaimOrdinal
	}
	lastClaimOrdinal := make(map[uint64]uint64)
	for index, tombstone := range restart.AuditTombstones {
		switch tombstone.ObjectKind {
		case AuditClaim:
			if tombstone.ClaimOrdinal >= uint64(limits.MaxClaimEntriesPerAttempt) ||
				tombstone.ClaimOrdinal == math.MaxUint64 {
				return fmt.Errorf("ledger restart claim ordinal is invalid")
			}
			offset := tombstone.ClaimOrdinal + 1
			if tombstone.AttemptTerminalSequence > math.MaxUint64-offset ||
				tombstone.TerminalSequence != tombstone.AttemptTerminalSequence+offset {
				return fmt.Errorf("ledger restart claim sequence is invalid")
			}
			if target, exists := tombstoneBySequence[tombstone.AttemptTerminalSequence]; tombstone.AttemptTerminalSequence > restart.TruncatedThroughSequence &&
				(!exists || target.ObjectKind != AuditAttempt) {
				return fmt.Errorf("ledger restart claim target is not an attempt")
			}
			if prior, exists := lastClaimOrdinal[tombstone.AttemptTerminalSequence]; exists {
				if prior == math.MaxUint64 || tombstone.ClaimOrdinal != prior+1 {
					return fmt.Errorf("ledger restart claim ordinals are not contiguous")
				}
			} else if tombstone.AttemptTerminalSequence >
				restart.TruncatedThroughSequence && tombstone.ClaimOrdinal != 0 {
				return fmt.Errorf("ledger restart retained attempt lacks its first claim")
			}
			lastClaimOrdinal[tombstone.AttemptTerminalSequence] = tombstone.ClaimOrdinal
		case AuditAttempt:
			if tombstone.ClaimCount > uint64(limits.MaxClaimEntriesPerAttempt) ||
				tombstone.ClaimCount > math.MaxUint64-tombstone.TerminalSequence {
				return fmt.Errorf("ledger restart attempt claim count is invalid")
			}
			if tombstone.TerminalSequence+tombstone.ClaimCount > endSequence ||
				index+int(tombstone.ClaimCount) >= len(restart.AuditTombstones) {
				return fmt.Errorf("ledger restart attempt claim interval is incomplete")
			}
			for ordinal := uint64(0); ordinal < tombstone.ClaimCount; ordinal++ {
				claim := restart.AuditTombstones[index+int(ordinal)+1]
				if claim.ObjectKind != AuditClaim ||
					claim.AttemptTerminalSequence != tombstone.TerminalSequence ||
					claim.ClaimOrdinal != ordinal ||
					claim.TerminalSequence != tombstone.TerminalSequence+ordinal+1 {
					return fmt.Errorf("ledger restart attempt claim interval is invalid")
				}
				if (tombstone.TerminalOutcome == string(AttemptPublished) ||
					tombstone.TerminalOutcome == string(AttemptPublishFailed)) &&
					claim.TerminalOutcome != string(ClaimSucceeded) {
					return fmt.Errorf("ledger restart publication has an impossible claim outcome")
				}
			}
		default:
			return fmt.Errorf("ledger restart tombstone kind is invalid")
		}
	}
	for _, tombstone := range restart.AuditTombstones {
		if tombstone.ObjectKind != AuditClaim {
			continue
		}
		if target, exists := tombstoneBySequence[tombstone.AttemptTerminalSequence]; exists &&
			target.ObjectKind == AuditAttempt &&
			tombstone.ClaimOrdinal >= target.ClaimCount {
			return fmt.Errorf("ledger restart claim exceeds its reservation")
		}
	}
	if _, err := modbusreg.MarshalBounded(ledgerRestartStateDTO{
		SchemaVersion:            restart.SchemaVersion,
		NextTerminalSequence:     restart.NextTerminalSequence,
		SequenceExhausted:        restart.SequenceExhausted,
		TruncatedThroughSequence: restart.TruncatedThroughSequence,
		AuditTombstones:          nonNilLedgerAuditTombstones(restart.AuditTombstones),
	}); err != nil {
		return fmt.Errorf("ledger restart aggregate: %w", err)
	}
	return nil
}

func validateLargestLedgerTombstones(limits modbusreg.LedgerLimits) error {
	candidates := []AuditTombstone{
		{
			SchemaVersion:    1,
			ObjectKind:       AuditAttempt,
			TerminalSequence: math.MaxUint64,
			ClaimCount:       uint64(limits.MaxClaimEntriesPerAttempt),
			TerminalOutcome:  string(AttemptPublishFailed),
		},
		{
			SchemaVersion:           1,
			ObjectKind:              AuditClaim,
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
func (ledger *SampleLedger) Limits() modbusreg.LedgerLimits {
	if ledger == nil {
		return modbusreg.LedgerLimits{}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.limits
}

func (ledger *SampleLedger) stateLocked() SampleLedgerState {
	return SampleLedgerState{
		SchemaVersion:        modbusreg.CurrentSchemaVersion(),
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
func (ledger *SampleLedger) ExportRestartState() (RestartState, error) {
	if ledger == nil {
		return RestartState{}, fmt.Errorf("sample ledger is invalid")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if len(ledger.attempts) != 0 || ledger.retainedClaims != 0 {
		return RestartState{}, fmt.Errorf("live ledger state is not restartable")
	}
	if ledger.nextTerminalSequence != ledger.durableNextTerminalSequence ||
		ledger.sequenceExhausted != ledger.durableSequenceExhausted {
		return RestartState{}, fmt.Errorf("terminal watermark is not durable")
	}
	return ledger.durableRestartStateLocked(), nil
}

func (ledger *SampleLedger) durableRestartStateLocked() RestartState {
	return RestartState{
		SchemaVersion:            1,
		NextTerminalSequence:     ledger.durableNextTerminalSequence,
		SequenceExhausted:        ledger.durableSequenceExhausted,
		TruncatedThroughSequence: ledger.truncatedThroughSequence,
		AuditTombstones: append(
			[]AuditTombstone(nil),
			ledger.auditTombstones...,
		),
	}
}

func ledgerRestartStatesEqual(first, second RestartState) bool {
	if first.SchemaVersion != second.SchemaVersion ||
		first.NextTerminalSequence != second.NextTerminalSequence ||
		first.SequenceExhausted != second.SequenceExhausted ||
		first.TruncatedThroughSequence != second.TruncatedThroughSequence ||
		len(first.AuditTombstones) != len(second.AuditTombstones) {
		return false
	}
	for index := range first.AuditTombstones {
		if first.AuditTombstones[index] != second.AuditTombstones[index] {
			return false
		}
	}
	return true
}

// Snapshot reports bounded resource counts and non-reconstructing tombstones.
func (ledger *SampleLedger) Snapshot() Snapshot {
	if ledger == nil {
		return Snapshot{}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return Snapshot{
		RetainedAttempts:     len(ledger.attempts),
		RetainedClaimEntries: ledger.retainedClaims,
		NextTerminalSequence: ledger.nextTerminalSequence,
		SequenceExhausted:    ledger.sequenceExhausted,
		AuditTombstones: append(
			[]AuditTombstone(nil),
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

func (ledger *SampleLedger) terminalRestartStateLocked(
	records []AuditTombstone,
) (RestartState, error) {
	combined := append([]AuditTombstone(nil), ledger.auditTombstones...)
	combined = append(combined, records...)
	sort.Slice(combined, func(first, second int) bool {
		return combined[first].TerminalSequence < combined[second].TerminalSequence
	})
	unique := combined[:0]
	for _, tombstone := range combined {
		if len(unique) != 0 &&
			unique[len(unique)-1].TerminalSequence == tombstone.TerminalSequence {
			if unique[len(unique)-1] != tombstone {
				return RestartState{}, fmt.Errorf("terminal sequence has conflicting outcomes")
			}
			continue
		}
		unique = append(unique, tombstone)
	}
	endSequence := uint64(math.MaxUint64)
	if !ledger.sequenceExhausted {
		endSequence = ledger.nextTerminalSequence - 1
	}
	last := sort.Search(len(unique), func(index int) bool {
		return unique[index].TerminalSequence >= endSequence
	})
	retained := []AuditTombstone(nil)
	truncatedThrough := endSequence
	if last < len(unique) && unique[last].TerminalSequence == endSequence {
		first := last
		for first > 0 && unique[first-1].TerminalSequence != math.MaxUint64 &&
			unique[first-1].TerminalSequence+1 == unique[first].TerminalSequence {
			first--
		}
		if ledger.truncatedThroughSequence != math.MaxUint64 &&
			unique[first].TerminalSequence == ledger.truncatedThroughSequence+1 {
			retained = append(retained, unique[first:last+1]...)
			if len(retained) > ledger.limits.AuditTombstoneLimit {
				retained = retained[len(retained)-ledger.limits.AuditTombstoneLimit:]
			}
			truncatedThrough = retained[0].TerminalSequence - 1
		}
	}
	restart := RestartState{
		SchemaVersion:            1,
		NextTerminalSequence:     ledger.nextTerminalSequence,
		SequenceExhausted:        ledger.sequenceExhausted,
		TruncatedThroughSequence: truncatedThrough,
		AuditTombstones:          retained,
	}
	if err := validateLedgerRestartState(restart, ledger.limits); err != nil {
		return RestartState{}, err
	}
	return restart, nil
}

func (ledger *SampleLedger) applyDurableRestartStateLocked(
	restart RestartState,
) {
	ledger.durableNextTerminalSequence = restart.NextTerminalSequence
	ledger.durableSequenceExhausted = restart.SequenceExhausted
	ledger.truncatedThroughSequence = restart.TruncatedThroughSequence
	ledger.auditTombstones = append(
		[]AuditTombstone(nil),
		restart.AuditTombstones...,
	)
}

func (ledger *SampleLedger) insertAuditTombstoneLocked(
	tombstone AuditTombstone,
) {
	index := sort.Search(len(ledger.auditTombstones), func(index int) bool {
		return ledger.auditTombstones[index].TerminalSequence >
			tombstone.TerminalSequence
	})
	ledger.auditTombstones = append(
		ledger.auditTombstones,
		AuditTombstone{},
	)
	copy(ledger.auditTombstones[index+1:], ledger.auditTombstones[index:])
	ledger.auditTombstones[index] = tombstone
	if len(ledger.auditTombstones) > ledger.limits.AuditTombstoneLimit {
		copy(ledger.auditTombstones, ledger.auditTombstones[1:])
		ledger.auditTombstones =
			ledger.auditTombstones[:len(ledger.auditTombstones)-1]
	}
}
