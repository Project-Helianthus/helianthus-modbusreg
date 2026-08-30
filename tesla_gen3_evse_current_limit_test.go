package modbusreg

import "testing"

func TestTeslaGen3EVSECurrentLimitRetainsPersistentAndProvisionalRecords(t *testing.T) {
	persistent, err := NewTeslaGen3PersistentCurrentLimit(TeslaGen3PersistentCurrentLimitSpec{
		OperationVersion:     TeslaGen3CurrentLimitOperationVersion24443,
		MaxOutputCurrentAmps: 16,
		Raw:                  []byte{0x32, 0x02, 0x3a, 0x00},
	})
	if err != nil || persistent.MaxOutputCurrentAmps() != 16 || persistent.OperationVersion() != TeslaGen3CurrentLimitOperationVersion24443 {
		t.Fatalf("persistent record = %#v, %v", persistent, err)
	}

	provisional, err := NewTeslaGen3ProvisionalCurrentLimit(TeslaGen3ProvisionalCurrentLimitSpec{
		OperationVersion:    TeslaGen3CurrentLimitOperationVersion24443,
		LimitCurrentMaxAmps: 16,
		LimitTimeoutSeconds: 600,
		InhibitCharging:     false,
		ReadbackRaw:         []byte{0x32, 0x03, 0xda, 0x01, 0x00},
	})
	if err != nil || provisional.LimitCurrentMaxAmps() != 16 || provisional.LimitTimeoutSeconds() != 600 || provisional.InhibitCharging() {
		t.Fatalf("provisional record = %#v, %v", provisional, err)
	}
	if provisional.OperationVersion() != TeslaGen3CurrentLimitOperationVersion24443 || provisional.ReadbackRaw()[0] != 0x32 {
		t.Fatalf("provisional evidence is not retrievable")
	}
	readback := provisional.ReadbackRaw()
	readback[0] = 0
	if provisional.ReadbackRaw()[0] != 0x32 {
		t.Fatal("ReadbackRaw() did not defensively copy")
	}
}

func TestTeslaGen3EVSECurrentLimitInteroperableProjectionIsBounded(t *testing.T) {
	for _, tc := range []struct {
		amps, timeout uint32
		valid         bool
	}{{6, 1, true}, {5, 1, false}, {6, 0, false}, {6, 86400, false}} {
		_, err := NewTeslaGen3InteroperableCurrentLimit(tc.amps, tc.timeout)
		if (err == nil) != tc.valid {
			t.Fatalf("amps=%d timeout=%d err=%v", tc.amps, tc.timeout, err)
		}
	}
	limit, err := NewTeslaGen3InteroperableCurrentLimit(6, 1)
	if err != nil || limit.MaxAmps() != 6 || limit.TimeoutSeconds() != 1 {
		t.Fatalf("bounded projection = %#v, %v", limit, err)
	}
}
