package huawei

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestEMMACycleRetriesUnconfirmedInventory(t *testing.T) {
	for _, name := range []string{"matching retry", "conflicting retry", "confirmed empty retry"} {
		t.Run(name, func(t *testing.T) {
			clock := newH06Clock()
			identity := emmaSiteIdentityScript()
			badListing := *identity[3].answer.page
			badListing.Conformity = 2
			session := newH06Session("emma", clock, &h06Recorder{}, identity[:3:3]...)
			session.Append(h06FC2B(0, 3, 0x87, h06Answer{page: &badListing}))
			session.Append(emmaSiteScript(0, 1, 0, 0, 0, 2)...)
			provider := startEMMASite(t, clock, session, []uint8{3, 4})
			first := refreshEMMASite(t, provider)
			child, ok := first.Child(3)
			if !ok || child.Evidence != ProviderEvidenceWithheld || child.Reason != ProviderReasonListingUnconfirmed ||
				first.EMMASite[0].Outcome != SUN2000GroupAnswered {
				t.Fatalf("failed inventory should withhold only the child: %+v, site %+v", child, first.EMMASite)
			}
			listing := identity[3].answer.page
			conflict, empty := name == "conflicting retry", name == "confirmed empty retry"
			switch {
			case conflict:
				self := []byte("1=Family-1;5=0;8=HEMS")
				listing.ResponseBytes += len(self) - len(listing.Objects[1].Value)
				listing.Objects[1].Value = self
			case empty:
				listing.ResponseBytes -= 2 + len(listing.Objects[2].Value)
				listing.DeclaredObjects = 2
				listing.Objects = listing.Objects[:2]
				listing.Objects[0].Value = []byte{1}
			}
			session.Append(identity[3])
			if !conflict {
				session.Append(emmaSiteScript(0, 3, 0, 0, 0, 4)...)
			}
			if !conflict && !empty {
				session.Append(emmaUncoveredChildIdentityScript()...)
				session.Append(emmaUncoveredChildGroups()...)
			}
			clock.Advance(time.Minute)
			second, err := provider.Refresh(context.Background())
			if err != nil || session.Remaining() != 0 || len(session.Unexpected()) != 0 {
				t.Fatalf("inventory retry did not follow the script: %v, %d left, %v", err, session.Remaining(), session.Unexpected())
			}
			if conflict {
				if second.Outcome != ProviderCycleInsufficient || second.Reason != ProviderReasonGatewayMismatch || len(second.EMMASite[0].Words) != 0 {
					t.Fatalf("conflicting inventory did not fence monitoring: %+v", second)
				}
				return
			}
			if second.Outcome != ProviderCycleComplete || second.Gateway != first.Gateway || second.EMMASite[0].Cycle != second.Cycle {
				t.Fatalf("matching inventory did not resume monitoring with the admitted gateway: %+v", second)
			}
			if empty {
				session.Append(emmaSiteScript(0, 5, 0, 0, 0, 6)...)
				clock.Advance(time.Minute)
				third := refreshEMMASite(t, provider)
				for _, cycle := range []ProviderCycle{second, third} {
					child, ok := cycle.Child(3)
					if !ok || child.Evidence != ProviderEvidenceWithheld || child.Reason != ProviderReasonNotListed {
						t.Fatalf("confirmed empty inventory did not persist: %+v", child)
					}
				}
				if third.PlannedReads != 3 || session.Remaining() != 0 || len(session.Unexpected()) != 0 {
					t.Fatalf("confirmed empty inventory was retried: %d requests, %v", third.PlannedReads, session.Unexpected())
				}
				return
			}
			requireEMMANativeChild(t, second)
		})
	}
}

func TestEMMACycleKeepsChildHistoryOnReadmissionFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		step int
		err  error
	}{
		{"model timeout", 0, context.DeadlineExceeded},
		{"identifier timeout", 1, context.DeadlineExceeded},
		{"model exception", 0, h06Exception(3, 2)},
		{"firmware exception", 3, h06Exception(3, 2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := newH06Clock()
			first := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
			first.Append(emmaSiteScript(0, 1, 0, 0, 0, 2)...)
			first.Append(emmaUncoveredChildIdentityScript()...)
			first.Append(emmaUncoveredChildGroups()...)
			provider := startEMMASite(t, clock, first, []uint8{3, 4})
			initial := refreshEMMASite(t, provider)
			requireEMMANativeChild(t, initial)
			want, _ := initial.Child(3)

			next := newH06Session("emma-reconnected", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
			next.Append(emmaSiteScript(0, 3, 0, 0, 0, 4)...)
			identity := emmaUncoveredChildIdentityScript()
			next.Append(identity[:tc.step]...)
			failed := identity[tc.step]
			failed.answer = h06Answer{err: tc.err}
			next.Append(failed)
			timedOut := errors.Is(tc.err, context.DeadlineExceeded)
			if timedOut {
				next.Append(failed, failed, failed)
			}
			if err := provider.Reconnect(next); err != nil {
				t.Fatal(err)
			}
			clock.Advance(time.Minute)
			cycle := refreshEMMASite(t, provider)
			got, ok := cycle.Child(3)
			if !ok || got.IdentifiedGeneration != 0 || next.Remaining() != 0 || len(next.Unexpected()) != 0 {
				t.Fatalf("readmission: %+v, %d requests left, unexpected %v", got, next.Remaining(), next.Unexpected())
			}
			if timedOut {
				if got.Evidence != ProviderEvidenceStale || got.Reason != ProviderReasonChildTimeout || got.Model != want.Model || got.Fenced != nil {
					t.Fatalf("timeout did not leave stale history: %+v", got)
				}
				wantReading := want.Reading.Groups()
				for i := range want.Groups {
					want.Groups[i].Outcome, want.Groups[i].Retained = SUN2000GroupNotRead, true
					wantReading[i].Retained = true
				}
				if !reflect.DeepEqual(got.Groups, want.Groups) || !reflect.DeepEqual(got.Reading.Groups(), wantReading) {
					t.Fatalf("stale history lost original words or epochs: %+v, %+v", got.Groups, got.Reading.Groups())
				}
			} else if got.Evidence != ProviderEvidenceWithheld || got.Reason != ProviderReasonIdentityInsuff || len(got.Groups) != 0 ||
				len(got.Reading.Groups()) != 0 || got.Fenced == nil || got.Fenced.Model != want.Model ||
				!reflect.DeepEqual(got.Fenced.Groups, want.Groups) || !reflect.DeepEqual(got.Fenced.Reading.Groups(), want.Reading.Groups()) {
				t.Fatalf("exception did not fence original words and epochs: %+v", got)
			}
		})
	}
}

func TestEMMACycleKeepsInitialRefusalsAfterCancellation(t *testing.T) {
	clock := newH06Clock()
	session := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
	site := emmaSiteScript(0, 1, 0, 0, 0, 2)
	site[0].answer = h06Answer{err: h06Exception(3, 3)}
	session.Append(site...)
	session.Append(emmaUncoveredChildIdentityScript()...)
	groups := emmaUncoveredChildGroups()
	entered, release := make(chan struct{}), make(chan struct{})
	groups[0].answer = h06Answer{err: h06Exception(3, 3), entered: entered, release: release}
	session.Append(groups[0])
	provider := startEMMASite(t, clock, session, []uint8{3})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := provider.Refresh(ctx)
		done <- err
	}()
	select {
	case <-entered:
		cancel()
		close(release)
	case err := <-done:
		t.Fatalf("cycle ended before the scripted refusal: %v", err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled refresh: %v", err)
	}
	if _, ok := provider.LastComplete(); ok {
		t.Fatal("cancelled refresh committed a cycle")
	}

	session.Append(emmaSiteIdentityScript()...)
	session.Append(site[1:]...)
	session.Append(emmaUncoveredChildIdentityScript()...)
	session.Append(groups[1:]...)
	clock.Advance(time.Minute)
	cycle := refreshEMMASite(t, provider)
	child, ok := cycle.Child(3)
	if !cycle.EMMASite[0].Refused || cycle.EMMASite[0].Outcome != SUN2000GroupNotRead || !ok || len(child.Groups) != 3 ||
		!child.Groups[0].Refused || child.Groups[0].Outcome != SUN2000GroupNotRead || child.Groups[1].Outcome != SUN2000GroupAnswered ||
		session.Remaining() != 0 || len(session.Unexpected()) != 0 {
		t.Fatalf("carried refusals were not honored: site %+v, child %+v, %d requests left, unexpected %v",
			cycle.EMMASite[0], child.Groups, session.Remaining(), session.Unexpected())
	}
}
