package modbusreg

import "testing"

func TestTeslaLegacyFDE0CandidateObservationRetainsCurrentRoles(t *testing.T) {
	raw := []byte{0xfd, 0xe0, 0x09, 0x07, 0xd0, 0x03, 0xe8}
	observation, err := NewTeslaLegacyFDE0Observation(TeslaLegacyFDE0ObservationSpec{
		Qualification: TeslaLegacyFamilyCompatible,
		State:         TeslaLegacyFDE0StateCharging,
		AllocatedMaxCA: 2000,
		ActualCurrentCA: 1000,
		Raw:           raw,
	})
	if err != nil {
		t.Fatalf("NewTeslaLegacyFDE0Observation() error = %v", err)
	}
	if observation.Qualification() != TeslaLegacyFamilyCompatible || observation.State() != TeslaLegacyFDE0StateCharging || observation.AllocatedMaxCA() != 2000 || observation.ActualCurrentCA() != 1000 {
		t.Fatalf("observation did not retain the candidate FDE0 roles: %#v", observation)
	}
	copy := observation.Raw()
	copy[0] = 0
	if observation.Raw()[0] != 0xfd {
		t.Fatal("Raw() did not defensively copy native evidence")
	}
}

func TestTeslaLegacyFDE0OnlyMapsAbsoluteOfferStates(t *testing.T) {
	for _, state := range []TeslaLegacyFDE0State{TeslaLegacyFDE0StatePreCharge, TeslaLegacyFDE0StateCharging} {
		if !state.IsAbsoluteOffer() {
			t.Fatalf("state %x must be an absolute offer", state)
		}
	}
	for _, state := range []TeslaLegacyFDE0State{0x06, 0x07, 0x08} {
		if state.IsAbsoluteOffer() {
			t.Fatalf("state %x must remain native-only or unknown", state)
		}
	}
}
