package modbusreg_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestS07RestartStateBoundaries(t *testing.T) {
	// This synthetic domain has no committed publications or external fixtures.
	digest := sha256.Sum256([]byte("S07 synthetic dependency set"))
	initial := reg.SampleLedgerState{
		SchemaVersion: reg.CurrentSchemaVersion(), IssuerDomain: "s07-synthetic",
		ProfileID: "s07-family", ProfileVersion: reg.MustParseVersion("0.1.0"),
		DependencySetID: fmt.Sprintf("sha256:%x", digest),
	}
	attempt := func(sequence, count uint64) reg.LedgerAuditTombstone {
		return reg.LedgerAuditTombstone{
			SchemaVersion: 1, ObjectKind: reg.LedgerAuditAttempt,
			TerminalSequence: sequence, ClaimCount: count,
			TerminalOutcome: string(reg.AttemptCancelled),
		}
	}
	claim := func(sequence, parent, ordinal uint64) reg.LedgerAuditTombstone {
		return reg.LedgerAuditTombstone{
			SchemaVersion: 1, ObjectKind: reg.LedgerAuditClaim,
			TerminalSequence: sequence, AttemptTerminalSequence: parent,
			ClaimOrdinal: ordinal, TerminalOutcome: string(reg.ClaimAttemptCancelled),
		}
	}
	state := func(next, truncated uint64, exhausted bool, records ...reg.LedgerAuditTombstone) reg.LedgerRestartState {
		return reg.LedgerRestartState{
			SchemaVersion: 1, NextTerminalSequence: next, SequenceExhausted: exhausted,
			TruncatedThroughSequence: truncated,
			AuditTombstones:          append([]reg.LedgerAuditTombstone{}, records...),
		}
	}
	sameState := func(got, want reg.LedgerRestartState) bool {
		return got.SchemaVersion == want.SchemaVersion &&
			got.NextTerminalSequence == want.NextTerminalSequence &&
			got.SequenceExhausted == want.SequenceExhausted &&
			got.TruncatedThroughSequence == want.TruncatedThroughSequence &&
			slices.Equal(got.AuditTombstones, want.AuditTombstones)
	}
	tests := []struct {
		name    string
		restart reg.LedgerRestartState
		wantErr string
	}{
		// A single retained claim has a valid truncated parent, ordinal and end
		// watermark. The negative omits the preceding claim without advancing
		// truncation, so only the first-record relation can reject it. Zero-claim
		// attempts cannot isolate this relation: their tombstone schema is invalid.
		{"first_record_claim_tail_control", state(3, 1, false, claim(2, 1, 0)), ""},
		{"first_record_claim_tail_gap", state(4, 1, false, claim(3, 1, 1)), "ledger restart tombstone is invalid"},
		{"retained_reservation_control", state(3, 0, false, attempt(1, 1), claim(2, 1, 0)), ""},
		{"claim_wrong_parent", state(3, 0, false, attempt(1, 1), claim(2, 2, 0)), "ledger restart attempt claim interval is invalid"},
		{"claim_wrong_ordinal", state(3, 0, false, attempt(1, 1), claim(2, 1, 1)), "ledger restart attempt claim interval is invalid"},
		{"claim_exceeds_reservation", state(4, 0, false, attempt(1, 1), claim(2, 1, 0), claim(3, 1, 1)), "ledger restart claim exceeds its reservation"},
		{"claim_targets_claim", state(4, 1, false, claim(2, 1, 0), claim(3, 2, 0)), "ledger restart claim target is not an attempt"},
		{"truncated_claim_tail_control", state(4, 2, false, claim(3, 1, 1)), ""},
		{"truncated_claim_wrong_sequence", state(4, 2, false, claim(3, 1, 0)), "ledger restart claim sequence is invalid"},
		{"initial_empty_control", state(1, 0, false), ""},
		{"truncated_empty_control", state(4, 3, false), ""},
		{"max_next_empty_control", state(math.MaxUint64, math.MaxUint64-1, false), ""},
		{"exhausted_empty_control", state(0, math.MaxUint64, true), ""},
		{"zero_next_without_exhaustion", state(0, math.MaxUint64, false), "ledger restart state is invalid"},
		{"exhausted_with_nonzero_next", state(1, math.MaxUint64, true), "ledger restart state is invalid"},
		{"exhausted_reservation_control", state(0, math.MaxUint64-2, true, attempt(math.MaxUint64-1, 1), claim(math.MaxUint64, math.MaxUint64-1, 0)), ""},
		{"exhausted_claim_tail_control", state(0, math.MaxUint64-1, true, claim(math.MaxUint64, math.MaxUint64-1, 0)), ""},
		{"exhausted_claim_link_overflow", state(0, math.MaxUint64-1, true, claim(math.MaxUint64, math.MaxUint64, 0)), "ledger restart claim sequence is invalid"},
		{"exhausted_truncation_with_history", state(0, math.MaxUint64, true, attempt(math.MaxUint64, 1)), "ledger restart history follows an exhausted truncation watermark"},
		{"claim_link_wraps_uint64", state(2, 0, false, claim(1, math.MaxUint64, 1)), "ledger restart claim sequence is invalid"},
		{"exhausted_attempt_reservation_overflow", state(0, math.MaxUint64-1, true, attempt(math.MaxUint64, 1)), "ledger restart attempt claim count is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Bypass both custom marshalers only to feed independently malformed
			// objects into the public JSON decoder, preserving every schema field.
			wireRecords := make([]map[string]any, len(test.restart.AuditTombstones))
			for i, record := range test.restart.AuditTombstones {
				fields := map[string]any{
					"schema_version": record.SchemaVersion, "object_kind": record.ObjectKind,
					"terminal_sequence": record.TerminalSequence, "terminal_outcome": record.TerminalOutcome,
				}
				if record.ObjectKind == reg.LedgerAuditAttempt {
					fields["claim_count"] = record.ClaimCount
				} else {
					fields["attempt_terminal_sequence"] = record.AttemptTerminalSequence
					fields["claim_ordinal"] = record.ClaimOrdinal
				}
				wireRecords[i] = fields
			}
			wire, err := json.Marshal(struct {
				SchemaVersion int              `json:"schema_version"`
				Next          uint64           `json:"next_terminal_sequence"`
				Exhausted     bool             `json:"sequence_exhausted"`
				Truncated     uint64           `json:"truncated_through_sequence"`
				Records       []map[string]any `json:"audit_tombstones"`
			}{1, test.restart.NextTerminalSequence, test.restart.SequenceExhausted, test.restart.TruncatedThroughSequence, wireRecords})
			if err != nil {
				t.Fatalf("encode independent wire: %v", err)
			}
			checkError := func(t *testing.T, err error) {
				t.Helper()
				if test.wantErr == "" {
					if err != nil {
						t.Fatalf("legal neighboring control rejected: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v; want category %q", err, test.wantErr)
				}
			}
			t.Run("tombstone_schema", func(t *testing.T) {
				for i, record := range test.restart.AuditTombstones {
					if _, err := json.Marshal(record); err != nil {
						t.Fatalf("record %d fails before restart relation: %v", i, err)
					}
				}
			})
			t.Run("restore", func(t *testing.T) {
				ledger, err := reg.NewSampleLedgerFromRestart(initial, 0, reg.DefaultLedgerLimits(), test.restart)
				checkError(t, err)
				if test.wantErr != "" {
					if ledger != nil {
						t.Fatal("invalid restart returned a ledger")
					}
					return
				}
				got, err := ledger.ExportRestartState()
				if err != nil || !sameState(got, test.restart) {
					t.Fatalf("restored restart = %+v, %v; want %+v", got, err, test.restart)
				}
			})
			t.Run("marshal", func(t *testing.T) {
				encoded, err := json.Marshal(test.restart)
				checkError(t, err)
				if test.wantErr == "" {
					var got reg.LedgerRestartState
					if err := json.Unmarshal(encoded, &got); err != nil || !sameState(got, test.restart) {
						t.Fatalf("marshal round trip = %+v, %v; want %+v", got, err, test.restart)
					}
				}
			})
			t.Run("unmarshal", func(t *testing.T) {
				var got reg.LedgerRestartState
				err := json.Unmarshal(wire, &got)
				checkError(t, err)
				if test.wantErr == "" && !sameState(got, test.restart) {
					t.Fatalf("decoded restart = %+v; want %+v", got, test.restart)
				}
			})
		})
	}
}
