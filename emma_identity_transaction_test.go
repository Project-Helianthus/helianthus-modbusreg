package modbusreg

import (
	"reflect"
	"testing"
	"time"
)

func emmaTransactionFixture() (EmmaIdentityContext, EmmaIdentityInput, time.Time) {
	now := time.Unix(100, 0).UTC()
	return EmmaIdentityContext{ParentSource: "parent-P", SourceRevision: "revision-R", ConnectionGeneration: 1, Attempt: 2, Deadline: now.Add(time.Minute)}, EmmaIdentityInput{Status: EmmaFC03Complete, Offering: "synthetic offering", Model: "EMMA-A01", Firmware: "SmartHEMS V100R024C00SPC100", Request: []byte{3, 0}, Response: []byte{3, 2, 0, 1}}, now
}

func TestEmmaIdentityTransactionIndependentInventory(t *testing.T) {
	ctx, input, now := emmaTransactionFixture()
	for _, mei := range []EmmaMEIStatus{EmmaMEINotRequested, EmmaMEIFailed, EmmaMEISucceeded} {
		for _, requested := range []bool{false, true} {
			result := NewEmmaIdentityAttempt(ctx, input, mei, requested).Commit(ctx, now, EmmaIdentityResult{})
			if !result.IdentityMatch() || !result.Committed() {
				t.Fatalf("identity lost: %s %v", mei, requested)
			}
			want := EmmaInventoryNotRequested
			if requested {
				want = EmmaInventoryBlocked
			}
			if result.InventoryStatus() != want || result.InventoryComplete() {
				t.Fatal("inventory promoted")
			}
			if result.MEIStatus() != mei || !result.DefaultDenied() || result.AutomaticRuntimeAdmission() || result.LiveQualified() || result.CatalogRegistered() || result.SupportClaim() || result.WriteAuthority() || result.Qualification() != HuaweiQualificationEvidenceBlocked {
				t.Fatal("safety/evidence changed")
			}
		}
	}
	for _, status := range []EmmaFC03Status{"", EmmaFC03Missing, EmmaFC03Failed} {
		input.Status = status
		result := NewEmmaIdentityAttempt(ctx, input, EmmaMEISucceeded, true).Commit(ctx, now, EmmaIdentityResult{})
		if result.IdentityMatch() || result.Committed() {
			t.Fatal("MEI substituted for FC03")
		}
	}
	input.Status = EmmaFC03Complete
	input.Model = "unknown"
	result := NewEmmaIdentityAttempt(ctx, input, EmmaMEISucceeded, true).Commit(ctx, now, EmmaIdentityResult{})
	if result.IdentityMatch() || result.Committed() {
		t.Fatal("mismatch accepted")
	}
}

func TestEmmaIdentityTransactionFences(t *testing.T) {
	ctx, input, now := emmaTransactionFixture()
	prior := NewEmmaIdentityAttempt(ctx, input, EmmaMEIFailed, false).Commit(ctx, now, EmmaIdentityResult{})
	mutations := []struct {
		name   string
		change func(*EmmaIdentityContext)
	}{
		{"parent", func(c *EmmaIdentityContext) { c.ParentSource = "parent-Q" }},
		{"revision", func(c *EmmaIdentityContext) { c.SourceRevision = "revision-S" }},
		{"connection", func(c *EmmaIdentityContext) { c.ConnectionGeneration++ }},
		{"attempt", func(c *EmmaIdentityContext) { c.Attempt++ }},
		{"deadline", func(c *EmmaIdentityContext) { c.Deadline = c.Deadline.Add(time.Second) }},
		{"cancelled", func(c *EmmaIdentityContext) { c.Cancelled = true }},
	}
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			attempt := NewEmmaIdentityAttempt(ctx, input, EmmaMEIFailed, true)
			control := attempt.Commit(ctx, now.Add(time.Second), prior)
			if !control.Committed() {
				t.Fatal("invalid control")
			}
			current := ctx
			m.change(&current)
			result := attempt.Commit(current, now.Add(time.Second), prior)
			if result.Committed() || result.Reason() != EmmaCommitContextRejected {
				t.Fatal("fence bypass")
			}
			old, _ := prior.RetainedIdentity()
			got, ok := result.RetainedIdentity()
			if !ok || !reflect.DeepEqual(got, old) {
				t.Fatal("retained context/time changed")
			}
		})
	}
	for _, at := range []time.Time{ctx.Deadline, ctx.Deadline.Add(time.Second), {}} {
		if NewEmmaIdentityAttempt(ctx, input, EmmaMEIFailed, true).Commit(ctx, at, prior).Committed() {
			t.Fatal("expired/absent time accepted")
		}
	}
	for _, m := range []func(*EmmaIdentityContext){
		func(c *EmmaIdentityContext) { c.ParentSource = "" }, func(c *EmmaIdentityContext) { c.SourceRevision = "" }, func(c *EmmaIdentityContext) { c.ConnectionGeneration = 0 }, func(c *EmmaIdentityContext) { c.Attempt = 0 }, func(c *EmmaIdentityContext) { c.Deadline = time.Time{} }, func(c *EmmaIdentityContext) { c.Cancelled = true },
	} {
		bad := ctx
		m(&bad)
		if NewEmmaIdentityAttempt(bad, input, EmmaMEIFailed, true).Commit(bad, now, prior).Committed() {
			t.Fatal("invalid frozen context")
		}
	}
	var zero EmmaIdentityAttempt
	result := zero.Commit(ctx, now, prior)
	if result.Committed() || result.IdentityMatch() || !result.DefaultDenied() {
		t.Fatal("zero attempt allowed")
	}
	var empty EmmaIdentityResult
	if empty.Committed() || empty.IdentityMatch() || !empty.DefaultDenied() || empty.InventoryComplete() {
		t.Fatal("zero result allowed")
	}
}

func TestEmmaIdentityTransactionRetentionAndReplacement(t *testing.T) {
	ctx, input, now := emmaTransactionFixture()
	prior := NewEmmaIdentityAttempt(ctx, input, EmmaMEISucceeded, true).Commit(ctx, now, EmmaIdentityResult{})
	original, _ := prior.RetainedIdentity()
	for _, status := range []EmmaFC03Status{EmmaFC03Missing, EmmaFC03Failed} {
		failed := input
		failed.Status = status
		next := ctx
		next.Attempt++
		result := NewEmmaIdentityAttempt(next, failed, EmmaMEISucceeded, true).Commit(next, now.Add(time.Second), prior)
		retained, _ := result.RetainedIdentity()
		if result.Committed() || result.IdentityMatch() || !reflect.DeepEqual(retained, original) {
			t.Fatal("failure refreshed or erased identity")
		}
	}
	for _, axis := range []string{"offering", "model", "firmware", "parent", "revision", "connection"} {
		t.Run(axis, func(t *testing.T) {
			changed := input
			next := ctx
			next.Attempt++
			switch axis {
			case "offering":
				changed.Offering = "another synthetic offering"
			case "model":
				changed.Model = "EMMA-A02"
			case "firmware":
				changed.Firmware = "SmartHEMS V100R024C00SPC101"
			case "parent":
				next.ParentSource = "parent-Q"
			case "revision":
				next.SourceRevision = "revision-S"
			case "connection":
				next.ConnectionGeneration++
			}
			control := NewEmmaIdentityAttempt(next, input, EmmaMEIFailed, true).Commit(next, now, EmmaIdentityResult{})
			if !control.Committed() {
				t.Fatal("invalid control")
			}
			result := NewEmmaIdentityAttempt(next, changed, EmmaMEIFailed, true).Commit(next, now.Add(time.Second), prior)
			if !result.Committed() || !result.InventoryAssociationInvalidated() {
				t.Fatal("old association survived replacement")
			}
		})
	}
	next := ctx
	next.Attempt++
	same := NewEmmaIdentityAttempt(next, input, EmmaMEIFailed, true).Commit(next, now, prior)
	if !same.Committed() || same.InventoryAssociationInvalidated() {
		t.Fatal("same identity wrongly retired")
	}
}

func TestEmmaIdentityTransactionDetached(t *testing.T) {
	assertConstructor := func(_ func(EmmaIdentityContext, EmmaIdentityInput, EmmaMEIStatus, bool) EmmaIdentityAttempt) {}
	assertConstructor(NewEmmaIdentityAttempt)
	ctx, input, now := emmaTransactionFixture()
	attempt := NewEmmaIdentityAttempt(ctx, input, EmmaMEISucceeded, true)
	input.Request[0] = 99
	input.Response[0] = 99
	input.Model = "unknown"
	ctx.ParentSource = "changed"
	frozen, _, _ := emmaTransactionFixture()
	result := attempt.Commit(frozen, now, EmmaIdentityResult{})
	first, ok := result.RetainedIdentity()
	if !ok || first.Input().Request[0] != 3 || first.Input().Response[0] != 3 || first.Context() != frozen || first.ObservedAt() != now {
		t.Fatal("caller alias")
	}
	if result.AttemptContext() != frozen || result.AttemptInput().Request[0] != 3 {
		t.Fatal("attempt evidence lost")
	}
	diagnostic := result.AttemptInput()
	diagnostic.Response[0] = 77
	if result.AttemptInput().Response[0] != 3 {
		t.Fatal("diagnostic alias")
	}
	copy := first.Input()
	copy.Request[0] = 88
	copy.Response[0] = 88
	again, _ := result.RetainedIdentity()
	if again.Input().Request[0] != 3 || again.Input().Response[0] != 3 {
		t.Fatal("getter alias")
	}
	// The public attempt accepts only a request selection and a diagnostic MEI
	// outcome. There is deliberately no caller guard-qualified input.
	typ := reflect.TypeOf(EmmaIdentityAttempt{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).IsExported() {
			t.Fatal("mutable/fabricated attempt field")
		}
	}
}
