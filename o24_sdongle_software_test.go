package modbusreg

import (
	"bytes"
	"testing"
)

func o24Text(o24Value string) []uint16 {
	o24Block := make([]byte, 30)
	copy(o24Block, o24Value)
	return h06BlockWords(o24Block)
}

// o24Script varies the synthetic gateway software in the public scripted session.
// Negative cases stop at the closing guard, before any child admission.
func o24Script(o24T testing.TB, o24Rig *h06Rig, o24Answer h06Answer, o24Admit bool) []h06Expected {
	o24T.Helper()
	_, o24Start := h06FixtureCycle(o24T, "sdongle_provider_cycles.json", "start_identification")
	o24Base := h06Script(o24T, o24Rig.sources, "S-Dongle", o24Start)
	// The public script starts with a tolerated search timeout, then its answer,
	// basic refusal, status, protocol and capacity. Software follows capacity.
	o24Out := append([]h06Expected(nil), o24Base...)
	if o24Out[6].function != 3 || o24Out[6].unit != 100 || o24Out[6].offset != 30050 || o24Out[6].quantity != 15 {
		o24T.Fatal("script must read gateway software after capacity")
	}
	o24Out[6].answer = o24Answer
	if !o24Admit {
		for o24Index := 7; o24Index < len(o24Out); o24Index++ {
			if o24Out[o24Index].unit == 100 && o24Out[o24Index].offset == 37410 {
				return o24Out[:o24Index+1]
			}
		}
		o24T.Fatal("no closing guard")
	}
	return o24Out
}

func o24Rig(o24T testing.TB) *h06Rig {
	o24T.Helper()
	o24Rig := newH06Rig(o24T, "S-Dongle", []uint8{1})
	o24T.Cleanup(func() {
		if o24Err := o24Rig.provider.Close(); o24Err != nil {
			o24T.Error(o24Err)
		}
	})
	return o24Rig
}

func TestO24SoftwareReadAndGuardedRefresh(o24T *testing.T) {
	o24Rig := o24Rig(o24T)
	o24Words := o24Text("V200R022C10SPC001")
	o24First := o24Rig.refresh(o24T, o24Script(o24T, o24Rig, h06Answer{words: o24Words}, true)...)
	if o24First.Gateway.State != HuaweiProviderGatewayIdentified || o24First.Reason != HuaweiProviderReasonIdentified {
		o24T.Fatalf("valid software did not identify: %+v", o24First)
	}
	o24Child, o24OK := o24First.Child(1)
	if !o24OK || o24Child.Evidence != HuaweiProviderEvidenceCurrent {
		o24T.Fatalf("valid control did not monitor: %+v", o24Child)
	}
	o24Provider := o24Rig.provider.(*HuaweiSDongleReadOnlyProvider)
	if o24Provider.core.state.sdongle == nil || !bytes.Equal(o24Provider.core.state.sdongle.Gateway.Software, huaweiWordsToBytes(o24Words)) {
		o24T.Fatal("gateway software bytes were not retained exactly")
	}
	_, o24Held := h06FixtureCycle(o24T, "sdongle_provider_cycles.json", "guard_held")
	o24Second := o24Rig.refresh(o24T, h06Script(o24T, o24Rig.sources, "S-Dongle", o24Held)...)
	if o24Second.Reason != HuaweiProviderReasonGuardHeld {
		o24T.Fatalf("guarded refresh: %s", o24Second.Reason)
	}
	for o24Index, o24Call := range o24Rig.recorder.Calls() {
		if o24Call.function != 3 && o24Call.function != 0x2b {
			o24T.Fatalf("non-read-only request %d: %+v", o24Index, o24Call)
		}
		if o24Index > 0 {
			o24Previous := o24Rig.recorder.Calls()[o24Index-1]
			if o24Call.sentAt.Sub(o24Previous.endAt) < o24Rig.provider.Pacing().Gap {
				o24T.Fatalf("request %d violated pacing", o24Index)
			}
		}
	}
}

func TestO24SoftwareAdmission(o24T *testing.T) {
	for _, o24Case := range []struct {
		o24Name  string
		o24Value string
		o24Admit bool
	}{
		{"o24_floor_control", "V200R022C10SPC001", true},
		{"o24_other_floor_control", "V100R001C00SPC100", true},
		{"o24_below_line_floor", "V200R022C09SPC001", false},
		{"o24_below_build_floor", "V100R001C00SPC099", false},
		{"o24_missing", "", false},
		{"o24_malformed", "invalid", false},
		{"o24_unmatched", "SmartHEMS V200R022C10SPC001", false},
	} {
		o24T.Run(o24Case.o24Name, func(o24T *testing.T) {
			o24Rig := o24Rig(o24T)
			o24Cycle := o24Rig.refresh(o24T, o24Script(o24T, o24Rig, h06Answer{words: o24Text(o24Case.o24Value)}, o24Case.o24Admit)...)
			if (o24Cycle.Gateway.State == HuaweiProviderGatewayIdentified) != o24Case.o24Admit {
				o24T.Fatalf("software admission: %+v", o24Cycle)
			}
			if !o24Case.o24Admit && o24Cycle.Outcome != HuaweiProviderCycleInsufficient {
				o24T.Fatalf("software refusal: %+v", o24Cycle)
			}
		})
	}
}

func TestO24ChildModelIndependentOfSoftware(o24T *testing.T) {
	o24Rig := o24Rig(o24T)
	o24Script := o24Script(o24T, o24Rig, h06Answer{words: o24Text("V200R022C10SPC001")}, false)
	o24Script[7].answer.words = o24Text("SUN2000-OTHER")
	o24Cycle := o24Rig.refresh(o24T, o24Script...)
	if o24Cycle.Gateway.State == HuaweiProviderGatewayIdentified || o24Cycle.Outcome != HuaweiProviderCycleInsufficient {
		o24T.Fatalf("child mismatch admitted: %+v", o24Cycle)
	}
}

func TestO24ReconnectDoesNotReuseSoftware(o24T *testing.T) {
	o24Rig := o24Rig(o24T)
	o24First := o24Rig.refresh(o24T, o24Script(o24T, o24Rig, h06Answer{words: o24Text("V200R022C10SPC001")}, true)...)
	o24Next := newH06Session("o24_generation_next", o24Rig.clock, o24Rig.recorder)
	if o24Err := o24Rig.provider.Reconnect(o24Next); o24Err != nil {
		o24T.Fatal(o24Err)
	}
	o24Rig.session = o24Next
	o24Control := o24Rig.refresh(o24T, o24Script(o24T, o24Rig, h06Answer{words: o24Text("V200R022C10SPC001")}, true)...)
	if o24Control.Generation == o24First.Generation || o24Control.Gateway.State != HuaweiProviderGatewayIdentified {
		o24T.Fatalf("same identity on a new generation: %+v", o24Control)
	}
	o24Next = newH06Session("o24_generation_bad_software", o24Rig.clock, o24Rig.recorder)
	if o24Err := o24Rig.provider.Reconnect(o24Next); o24Err != nil {
		o24T.Fatal(o24Err)
	}
	o24Rig.session = o24Next
	o24Second := o24Rig.refresh(o24T, o24Script(o24T, o24Rig, h06Answer{words: o24Text("V200R022C09SPC001")}, false)...)
	if o24Second.Generation == o24Control.Generation || o24Second.Gateway.State == HuaweiProviderGatewayIdentified || o24Second.Outcome != HuaweiProviderCycleInsufficient {
		o24T.Fatalf("predecessor software survived reconnect: %+v", o24Second)
	}
}

func TestO24SoftwareReadFailurePolicy(o24T *testing.T) {
	o24T.Run("o24_definitive_exception", func(o24T *testing.T) {
		o24Rig := o24Rig(o24T)
		o24Script := o24Script(o24T, o24Rig, h06Answer{err: h06Exception(3, 2)}, false)
		o24Cycle := o24Rig.refresh(o24T, o24Script[:7]...)
		if o24Cycle.Outcome != HuaweiProviderCycleInsufficient || o24Cycle.Gateway.State == HuaweiProviderGatewayIdentified {
			o24T.Fatalf("exception admitted: %+v", o24Cycle)
		}
	})
	o24T.Run("o24_bounded_timeout_and_recovery", func(o24T *testing.T) {
		o24Rig := o24Rig(o24T)
		o24Script := o24Script(o24T, o24Rig, h06Answer{err: h06Timeout()}, false)
		o24Failed := append([]h06Expected(nil), o24Script[:6]...)
		for o24Attempt := 0; o24Attempt < 4; o24Attempt++ {
			o24Failed = append(o24Failed, o24Script[6])
		}
		o24Cycle := o24Rig.refresh(o24T, o24Failed...)
		if o24Cycle.Outcome != HuaweiProviderCycleInsufficient || o24Cycle.Reason != HuaweiProviderReasonGatewayReadFail {
			o24T.Fatalf("failed read admitted: %+v", o24Cycle)
		}
		// The rig advances to NextRefreshAt, including the endpoint backoff.
		o24Recovered := o24Rig.refresh(o24T, o24ScriptForRecovery(o24T, o24Rig)...)
		if o24Recovered.Gateway.State != HuaweiProviderGatewayIdentified {
			o24T.Fatalf("recovery did not reidentify: %+v", o24Recovered)
		}
	})
}

func o24ScriptForRecovery(o24T testing.TB, o24Rig *h06Rig) []h06Expected {
	o24T.Helper()
	o24Script := o24Script(o24T, o24Rig, h06Answer{words: o24Text("V200R022C10SPC001")}, true)
	// Only the first request of a connection generation gets a timeout allowance.
	return o24Script[1:]
}
