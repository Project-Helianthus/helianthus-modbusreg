package modbus

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/roweval"
)

const connectorPolicy semreg.PolicyID = "evse.telemetry.synthetic.v1"

type legacyFixture struct {
	request  tesla.LegacyFBE0ObservationSpec
	response tesla.LegacyFDE0ObservationSpec
	length   uint16
}

func legacyFrames(t *testing.T, length uint16, state tesla.LegacyFDE0State, requestCA, reportedCA, actualCA uint16) legacyFixture {
	t.Helper()
	profile, err := tesla.NewLegacyWallConnectorProfile(tesla.LegacyWallConnectorProfileConfig{Enabled: true, Compatibility: tesla.LegacyWallConnectorFamilyCompatible})
	if err != nil {
		t.Fatal(err)
	}
	req := []byte{0, 1, 0, 2, byte(state), byte(requestCA >> 8), byte(requestCA), 0xaa, 0xbb, 0xcc, 0xdd}
	rsp := []byte{0, 2, 0, 1, byte(state), byte(reportedCA >> 8), byte(reportedCA), byte(actualCA >> 8), byte(actualCA), 0xaa, 0xbb}
	if length == 13 {
		req = append(req, 0xee, 0xff)
		rsp = append(rsp, 0xcc, 0xdd)
	}
	request := legacyFrame(t, tesla.LegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, req)
	response := legacyFrame(t, tesla.LegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, rsp)
	return legacyFixture{
		request:  tesla.LegacyFBE0ObservationSpec{Profile: profile, Direction: tesla.LegacyWallConnectorRequest, Qualification: tesla.LegacyFamilyCompatible, State: state, AllocatedMaxCA: requestCA, Raw: request},
		response: tesla.LegacyFDE0ObservationSpec{Profile: profile, Direction: tesla.LegacyWallConnectorResponse, Qualification: tesla.LegacyFamilyCompatible, State: state, AllocatedMaxCA: reportedCA, ActualCurrentCA: actualCA, Raw: response}, length: length}
}

func legacyFrame(t *testing.T, c tesla.LegacyCommand, payload []byte) []byte {
	t.Helper()
	frame, err := tesla.EncodeTeslaLegacyWallConnectorFrame(c, payload)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func legacyInput(sel LegacySelector, raw []byte, resource, receipt string) (roweval.Acquired[LegacySelector], roweval.Subject) {
	sub := subject("connector", "helianthus.pack.evse", "evse.service.connector", "evse.capability.read.connector", resource)
	index := []semreg.DefinitionRef{expectRef("helianthus.pack.evse", "evse.status.charging"), expectRef("helianthus.pack.evse", "evse.limit.allocated_current"), expectRef("helianthus.pack.evse", "evse.limit.actual_current")}
	tpl := candidate(connectorPolicy, receipt, raw)
	return roweval.Acquired[LegacySelector]{Selector: sel, Template: tpl, Roles: roles(sub, index, nil), Coordinates: map[string]string{"connector-resource": resource}, Receipt: roweval.Receipt{ReceivedAt: tpl.Times.ReceivedAt, Monotonic: tpl.Times.ReceiptMonotonic}}, sub
}

func TestLegacyReceiveSelectorAndAuthority(t *testing.T) {
	for _, length := range []uint16{11, 13} {
		t.Run(fmt.Sprintf("payload-%d", length), func(t *testing.T) {
			table := legacyRows(length, connectorPolicy)
			f := legacyFrames(t, length, 0x09, 2000, 2000, 1000)
			input, _ := legacyInput(LegacySelector{}, f.response.Raw, "resource:synthetic-connector", "100")
			if err := roweval.ValidateTable(table, descriptors[LegacySelector](), input.Roles); err != nil {
				t.Fatal(err)
			}
			for _, b := range table.Bindings {
				if b.Write != nil {
					t.Fatal("passive row has an inverse")
				}
			}
			for _, responseFirst := range []bool{false, true} {
				t.Run(fmt.Sprintf("response-first-%t", responseFirst), func(t *testing.T) {
					f := legacyFrames(t, length, 0x09, 2000, 1875, 1003)
					if responseFirst {
						checkLegacyResponse(t, table, f, "101")
						checkLegacyRequest(t, table, f, "102")
					} else {
						checkLegacyRequest(t, table, f, "101")
						checkLegacyResponse(t, table, f, "102")
					}
				})
			}
			t.Run("request-only", func(t *testing.T) { checkLegacyRequest(t, table, f, "100") })
			checkLegacyResponse(t, table, f, "100")
			t.Run("selector-and-wire-rejections", func(t *testing.T) {
				checkLegacyMisses(t, table, f)
				checkLegacyRequestRejections(t, table, f)
			})
			for _, state := range []tesla.LegacyFDE0State{0x05, 0x06, 0x07, 0x08} {
				t.Run(fmt.Sprintf("state-%02x", state), func(t *testing.T) {
					checkLegacyResponse(t, table, legacyFrames(t, length, state, 600, 0x8021, 1001), "100")
				})
			}
		})
	}
}

func checkLegacyRequest(t *testing.T, table roweval.Table[LegacySelector], f legacyFixture, received string) {
	t.Helper()
	req := tesla.LegacyCommand{Prefix: 0xfb, Opcode: 0xe0}
	for _, slice := range []struct {
		off, count uint16
		raw        uint64
	}{{4, 1, uint64(f.request.State)}, {5, 2, uint64(f.request.AllocatedMaxCA)}, {7, 2, 0}} {
		a, _ := legacyInput(LegacySelector{req, tesla.LegacyWallConnectorRequest, f.length, slice.off, slice.count}, f.request.Raw, "resource:synthetic-connector", received)
		got, err := evaluateLegacyRequest(table, f.request, a)
		if err != nil || !bytes.Equal(got.Raw, f.request.Raw) || got.Candidate != nil || got.Subject != nil {
			t.Fatalf("request slice %+v: %+v (%v)", slice, got, err)
		}
		if slice.off == 7 {
			if got.Reason != roweval.ReasonNoMatch || got.Native != nil {
				t.Fatalf("request supplies nonexistent actual current: %+v", got)
			}
			continue
		}
		if got.Reason != roweval.ReasonNoBinding || got.Native == nil || got.Native.Raw != slice.raw {
			t.Fatalf("request did not keep its own native value: %+v", got)
		}
		if slice.off == 5 {
			assertNumber(t, got.Native.Value, "cA", big.NewRat(int64(slice.raw), 1))
		}
	}
}

func checkLegacyResponse(t *testing.T, table roweval.Table[LegacySelector], f legacyFixture, received string) {
	t.Helper()
	resource := "resource:synthetic-connector"
	cmd := tesla.LegacyCommand{Prefix: 0xfd, Opcode: 0xe0}
	state := f.response.State
	for _, c := range []struct {
		off, count uint16
		id         semreg.DefinitionID
		raw        uint16
		emits      bool
	}{
		{4, 1, "evse.status.charging", uint16(state), state == 0x09},
		{5, 2, "evse.limit.allocated_current", f.response.AllocatedMaxCA, state == 0x05 || state == 0x09},
		{7, 2, "evse.limit.actual_current", f.response.ActualCurrentCA, true},
	} {
		a, sub := legacyInput(LegacySelector{cmd, tesla.LegacyWallConnectorResponse, f.length, c.off, c.count}, f.response.Raw, resource, received)
		got, err := evaluateLegacyResponse(table, f.response, a)
		if err != nil || !bytes.Equal(got.Raw, f.response.Raw) || got.Native == nil || got.Native.Raw != uint64(c.raw) {
			t.Fatalf("response slice %+v: %+v, %v", c, got, err)
		}
		if c.count == 2 {
			assertNumber(t, got.Native.Value, "cA", big.NewRat(int64(c.raw), 1))
		}
		if !c.emits {
			if got.Reason != roweval.ReasonNoBinding || got.Candidate != nil || got.Subject != nil {
				t.Fatalf("unqualified state %02x published %s: %+v", state, c.id, got)
			}
			continue
		}
		want := quantity(decimal(int64(c.raw), -2), "unit.ampere")
		if c.off == 4 {
			want = semreg.Value{Kind: semreg.ValueSymbol, Symbol: &semreg.Symbol{Namespace: "evse.status.charging", Token: "evse.status.charging.charging", Known: true}}
		}
		assertCandidate(t, got.Evaluation, a.Template, sub, expectRef("helianthus.pack.evse", c.id), "evse.dimension.connector", resource, want)
	}
}

func checkLegacyMisses(t *testing.T, table roweval.Table[LegacySelector], f legacyFixture) {
	t.Helper()
	sel := LegacySelector{tesla.LegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, tesla.LegacyWallConnectorResponse, f.length, 5, 2}
	for _, c := range []struct {
		name   string
		mutate func(*LegacySelector)
	}{
		{"prefix", func(s *LegacySelector) { s.Command.Prefix = 0xfb }},
		{"opcode", func(s *LegacySelector) { s.Command.Opcode = 0xe2 }},
		{"direction", func(s *LegacySelector) { s.Direction = tesla.LegacyWallConnectorRequest }},
		{"length", func(s *LegacySelector) { s.PayloadLength = 24 - s.PayloadLength }},
		{"offset", func(s *LegacySelector) { s.ByteOffset++ }},
		{"count", func(s *LegacySelector) { s.ByteCount-- }},
	} {
		t.Run(c.name, func(t *testing.T) {
			altered := sel
			c.mutate(&altered)
			a, _ := legacyInput(altered, f.response.Raw, "resource:synthetic-connector", "100")
			got, err := evaluateLegacyResponse(table, f.response, a)
			if err != nil || got.Reason != roweval.ReasonNoMatch || got.Native != nil || got.Candidate != nil || got.Subject != nil || !bytes.Equal(got.Raw, f.response.Raw) {
				t.Fatalf("mutated selector matched: %+v %v", got, err)
			}
		})
	}
	bad := bytes.Clone(f.response.Raw)
	bad[len(bad)-2] ^= 1
	cases := map[string]tesla.LegacyFDE0ObservationSpec{}
	for _, length := range []int{7, 9, 10, 12, 14} {
		wrong := f.response
		wrong.Raw = legacyFrame(t, sel.Command, make([]byte, length))
		cases[fmt.Sprintf("invalid-length-%d", length)] = wrong
	}
	corrupted := f.response
	corrupted.Raw = bad
	cases["checksum"] = corrupted
	truncated := f.response
	truncated.Raw = truncated.Raw[:len(truncated.Raw)-1]
	cases["truncated"] = truncated
	wrongDirection := f.response
	wrongDirection.Direction = tesla.LegacyWallConnectorRequest
	cases["wrong-wire-direction"] = wrongDirection
	wrongCommand := f.response
	wrongCommand.Raw = legacyFrames(t, f.length, 0x05, 600, 2000, 1000).request.Raw
	cases["wrong-wire-command"] = wrongCommand
	for _, c := range []struct {
		name          string
		enabled       bool
		compatibility tesla.LegacyWallConnectorCompatibility
	}{
		{"disabled-profile", false, tesla.LegacyWallConnectorFamilyCompatible},
		{"unknown-profile", true, tesla.LegacyWallConnectorUnknown},
		{"qualification-mismatch", true, tesla.LegacyWallConnectorBuildConfirmed},
	} {
		spec := f.response
		var err error
		spec.Profile, err = tesla.NewLegacyWallConnectorProfile(tesla.LegacyWallConnectorProfileConfig{Enabled: c.enabled, Compatibility: c.compatibility})
		if err != nil {
			t.Fatal(err)
		}
		cases[c.name] = spec
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			a, _ := legacyInput(sel, spec.Raw, "resource:synthetic-connector", "100")
			got, err := evaluateLegacyResponse(table, spec, a)
			if err == nil || !bytes.Equal(got.Raw, spec.Raw) || got.Native != nil || got.Candidate != nil || got.Subject != nil {
				t.Fatalf("invalid frame yielded a partial fact: %+v; %v", got, err)
			}
		})
	}
}

func checkLegacyRequestRejections(t *testing.T, table roweval.Table[LegacySelector], f legacyFixture) {
	t.Helper()
	cmd := tesla.LegacyCommand{Prefix: 0xfb, Opcode: 0xe0}
	sel := LegacySelector{cmd, tesla.LegacyWallConnectorRequest, f.length, 5, 2}
	for _, length := range []int{7, 9, 10, 12, 14} {
		t.Run(fmt.Sprintf("request-invalid-length-%d", length), func(t *testing.T) {
			spec := f.request
			spec.Raw = legacyFrame(t, cmd, make([]byte, length))
			assertLegacyRequestRejected(t, table, sel, spec)
		})
	}
	t.Run("request-checksum", func(t *testing.T) {
		spec := f.request
		spec.Raw = bytes.Clone(spec.Raw)
		spec.Raw[len(spec.Raw)-2] ^= 1
		assertLegacyRequestRejected(t, table, sel, spec)
	})
}

func assertLegacyRequestRejected(t *testing.T, table roweval.Table[LegacySelector], sel LegacySelector, spec tesla.LegacyFBE0ObservationSpec) {
	t.Helper()
	a, _ := legacyInput(sel, spec.Raw, "resource:synthetic-connector", "100")
	got, err := evaluateLegacyRequest(table, spec, a)
	if err == nil || !bytes.Equal(got.Raw, spec.Raw) || got.Native != nil || got.Candidate != nil || got.Subject != nil {
		t.Fatalf("invalid request produced a partial fact: %+v; %v", got, err)
	}
}
