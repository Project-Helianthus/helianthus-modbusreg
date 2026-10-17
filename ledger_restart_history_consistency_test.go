package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestQ28RestartHistoryConsistency(t *testing.T) {
	initial := emptyLedgerState(t, profileFixture(t))
	limits := reg.DefaultLedgerLimits()
	attempt := func(sequence uint64) reg.LedgerAuditTombstone {
		return reg.LedgerAuditTombstone{
			SchemaVersion: 1, ObjectKind: reg.LedgerAuditAttempt,
			TerminalSequence: sequence, ClaimCount: 1,
			TerminalOutcome: string(reg.AttemptCancelled),
		}
	}
	claim := func(sequence, parent uint64) reg.LedgerAuditTombstone {
		return reg.LedgerAuditTombstone{
			SchemaVersion: 1, ObjectKind: reg.LedgerAuditClaim,
			TerminalSequence: sequence, AttemptTerminalSequence: parent,
			ClaimOrdinal: 0, TerminalOutcome: string(reg.ClaimAttemptCancelled),
		}
	}
	full := reg.LedgerRestartState{
		SchemaVersion: 1, NextTerminalSequence: 7,
		AuditTombstones: []reg.LedgerAuditTombstone{
			attempt(1), claim(2, 1), attempt(3), claim(4, 3), attempt(5), claim(6, 5),
		},
	}
	clone := func(state reg.LedgerRestartState) reg.LedgerRestartState {
		state.AuditTombstones = append([]reg.LedgerAuditTombstone(nil), state.AuditTombstones...)
		return state
	}
	prefixTruncated := clone(full)
	prefixTruncated.TruncatedThroughSequence = 2
	prefixTruncated.AuditTombstones = prefixTruncated.AuditTombstones[2:]
	claimTail := clone(full)
	claimTail.TruncatedThroughSequence = 5
	claimTail.AuditTombstones = claimTail.AuditTombstones[5:]

	// This wire type bypasses only the top-level MarshalJSON validation so that
	// malformed histories reach the public UnmarshalJSON boundary independently.
	type restartWire reg.LedgerRestartState
	check := func(t *testing.T, state reg.LedgerRestartState, wantValid bool) {
		t.Helper()
		t.Run("restore", func(t *testing.T) {
			ledger, err := reg.NewSampleLedgerFromRestart(initial, 0, limits, state)
			if !wantValid {
				if err == nil || ledger != nil {
					t.Fatal("invalid history was restored")
				}
				return
			}
			if err != nil || ledger == nil {
				t.Fatalf("valid history was rejected: %v", err)
			}
			got, err := ledger.ExportRestartState()
			if err != nil || !reflect.DeepEqual(got, state) {
				t.Fatalf("restart round trip = %+v, %v; want %+v", got, err, state)
			}
		})
		wire, err := json.Marshal(restartWire(state))
		if err != nil {
			t.Fatalf("encode history wire: %v", err)
		}
		t.Run("marshal", func(t *testing.T) {
			encoded, err := json.Marshal(state)
			if !wantValid {
				if err == nil {
					t.Fatal("invalid history was marshaled")
				}
				return
			}
			if err != nil || !bytes.Equal(encoded, wire) {
				t.Fatalf("valid history encoding = %s, %v; want %s", encoded, err, wire)
			}
		})
		t.Run("unmarshal", func(t *testing.T) {
			var decoded reg.LedgerRestartState
			err := json.Unmarshal(wire, &decoded)
			if !wantValid {
				if err == nil {
					t.Fatal("invalid history was unmarshaled")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(decoded, state) {
				t.Fatalf("JSON round trip = %+v, %v; want %+v", decoded, err, state)
			}
			ledger, err := reg.NewSampleLedgerFromRestart(initial, 0, limits, decoded)
			if err != nil || ledger == nil {
				t.Fatalf("decoded valid history was rejected: %v", err)
			}
		})
	}

	tests := []struct {
		name   string
		mutate func(*reg.LedgerRestartState)
	}{
		{"missing_middle_tombstone", func(s *reg.LedgerRestartState) {
			s.AuditTombstones = append(s.AuditTombstones[:3], s.AuditTombstones[4:]...)
		}},
		// Removing a complete reservation leaves all remaining claim links valid;
		// only the unexplained gap violates the contiguous-history contract.
		{"missing_middle_reservation", func(s *reg.LedgerRestartState) {
			s.AuditTombstones = append(s.AuditTombstones[:2], s.AuditTombstones[4:]...)
		}},
		{"reordered_reservations", func(s *reg.LedgerRestartState) {
			s.AuditTombstones[0], s.AuditTombstones[2] = s.AuditTombstones[2], s.AuditTombstones[0]
			s.AuditTombstones[1], s.AuditTombstones[3] = s.AuditTombstones[3], s.AuditTombstones[1]
		}},
		{"duplicate_sequence", func(s *reg.LedgerRestartState) {
			s.AuditTombstones[3].TerminalSequence = 3
		}},
		{"prefix_removed_without_boundary", func(s *reg.LedgerRestartState) {
			s.AuditTombstones = s.AuditTombstones[2:]
		}},
		{"boundary_overlaps_retained_history", func(s *reg.LedgerRestartState) {
			s.TruncatedThroughSequence = 2
		}},
		{"next_sequence_skips_retained_tail", func(s *reg.LedgerRestartState) {
			s.NextTerminalSequence = 8
		}},
		{"next_sequence_reuses_retained_tail", func(s *reg.LedgerRestartState) {
			// A legal leading claim suffix isolates the tail watermark from the
			// separate completeness checks for retained attempt reservations.
			*s = clone(claimTail)
			s.NextTerminalSequence = 6
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("valid_full", func(t *testing.T) { check(t, clone(full), true) })
			t.Run("valid_truncated_prefix", func(t *testing.T) { check(t, clone(prefixTruncated), true) })
			t.Run("valid_truncated_claim_tail", func(t *testing.T) { check(t, clone(claimTail), true) })
			t.Run("invalid", func(t *testing.T) {
				state := clone(full)
				test.mutate(&state)
				if len(state.AuditTombstones) == 0 || reflect.DeepEqual(state, full) {
					t.Fatal("history mutation has no nonempty discriminator")
				}
				check(t, state, false)
			})
		})
	}
}
