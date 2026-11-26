package write

import "testing"

func TestTheRowDeclaresTheAmperesCaseAndTheTypedRelease(t *testing.T) {
	row := Row()
	set := row.Binding.Write
	if set == nil || set.Operation.ID != OperationID || set.Argument != ArgumentID || set.TargetRow != TargetRow || set.ReadbackRow != ReadbackRow {
		t.Fatalf("%+v", set)
	}
	if row.Binding.Capability.ID != CapabilityID || row.Binding.Service.ID != ServiceID {
		t.Fatalf("%+v", row.Binding)
	}
	if set.NativeBounds.Min == nil || set.NativeBounds.Min.Coefficient != "6" || set.NativeBounds.Step == nil || set.NativeBounds.Step.Coefficient != "1" {
		t.Fatalf("the bounds are not the pilot floor in whole amperes: %+v", set.NativeBounds)
	}
	if set.NativeBounds.Max != nil {
		t.Fatal("the row states a highest value of its own")
	}
	if row.Release.Operation != set.Operation || row.Release.Argument != set.Argument || row.Release.TargetRow != set.TargetRow || row.Release.ReadbackRow != set.ReadbackRow {
		t.Fatalf("the release is not under the same operation: %+v", row.Release)
	}
}

func TestWholeAmpsIsExact(t *testing.T) {
	rows := []struct {
		coefficient string
		exponent    int32
		want        uint32
		reason      string
	}{
		{"16", 0, 16, ""}, {"160", -1, 16, ""}, {"1", 1, 10, ""}, {"0", 0, 0, ""}, {"0", -5, 0, ""},
		{"65", -1, 0, ReasonNotWhole}, {"1", -1, 0, ReasonNotWhole}, {"1", -30, 0, ReasonNotWhole},
		{"-3", 0, 0, ReasonNegative}, {"4294967296", 0, 0, ReasonOutOfRange}, {"1", 11, 0, ReasonOutOfRange}, {"x", 0, 0, ReasonNotWhole},
	}
	for _, row := range rows {
		got, reason := wholeAmps(amperes(row.coefficient, row.exponent))
		if got != row.want || reason != row.reason {
			t.Errorf("%s e%d: %d %q, want %d %q", row.coefficient, row.exponent, got, reason, row.want, row.reason)
		}
	}
}
