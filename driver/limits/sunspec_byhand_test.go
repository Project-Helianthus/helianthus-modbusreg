package limits

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/life"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/tree"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// Chain layout counted from the model point order: signature, model 121,
// two model 123 blocks, model 124, model 125, then the end marker.
const (
	ratingHeader       = 2
	ratingWords        = 30
	controlsHeader     = ratingHeader + 2 + ratingWords
	controlsWords      = 24
	controlsHeaderNext = controlsHeader + 2 + controlsWords
	storageHeader      = controlsHeaderNext + 2 + controlsWords
	storageWords       = 24
	gridHeader         = storageHeader + 2 + storageWords
	gridWords          = 8
	endHeader          = gridHeader + 2 + gridWords
)

const (
	ordinalControls     = 2
	ordinalControlsNext = 3
	ordinalStorage      = 4
)

type handChain struct {
	label string
	unit  uint8
	base  uint16
}

type handSpec struct {
	model  uint16
	name   string
	offset uint16
	shape  string
}

type handCase struct {
	local string
	unit  uint8
	addr  uint16
	arg   string
	word  uint16
}

type handRecording struct {
	*SunSpecByHand
	startErr error
}

func (r *handRecording) Start(ctx context.Context, s driver.Session) error {
	r.startErr = r.SunSpecByHand.Start(ctx, s)
	return r.startErr
}

type handFactory struct{ family *handRecording }

func (handFactory) Descriptor() driver.Descriptor { return driver.Descriptor{} }
func (handFactory) Plan(c driver.Config) ([]driver.InstancePlan, error) {
	return []driver.InstancePlan{{Driver: limitsKind, Instance: driver.InstanceKey(c.Instance)}}, nil
}
func (f handFactory) Construct(context.Context, driver.InstancePlan) (driver.Instance, error) {
	return f.family, nil
}

func handChains() []handChain {
	return []handChain{{label: "chain-a", unit: 1, base: 40000}, {label: "chain-b", unit: 2, base: 41000}}
}

func handSpecs() []handSpec {
	return []handSpec{
		{123, "Conn_WinTms", 2, "word"}, {123, "Conn_RvrtTms", 3, "word"}, {123, "Conn", 4, "enum"},
		{123, "WMaxLimPct", 5, "percent"}, {123, "WMaxLimPct_WinTms", 6, "word"}, {123, "WMaxLimPct_RvrtTms", 7, "word"},
		{123, "WMaxLimPct_RmpTms", 8, "word"}, {123, "WMaxLim_Ena", 9, "enum"}, {123, "OutPFSet_WinTms", 11, "word"},
		{123, "OutPFSet_RvrtTms", 12, "word"}, {123, "OutPFSet_RmpTms", 13, "word"}, {123, "VArPct_WinTms", 18, "word"},
		{123, "VArPct_RvrtTms", 19, "word"}, {123, "VArPct_RmpTms", 20, "word"}, {123, "VArPct_Mod", 21, "mode"},
		{124, "WChaGra", 3, "gradient"}, {124, "WDisChaGra", 4, "gradient"}, {124, "StorCtl_Mod", 5, "word"},
		{124, "OutWRte", 12, "percent"}, {124, "InWRte", 13, "percent"}, {124, "InOutWRte_WinTms", 14, "word"},
		{124, "InOutWRte_RvrtTms", 15, "word"}, {124, "InOutWRte_RmpTms", 16, "word"}, {124, "ChaGriSet", 17, "enum"},
	}
}

func (s handSpec) value(n int) (string, uint16) {
	switch s.shape {
	case "enum":
		word := uint16(n % 2)
		return fmt.Sprintf(`{"word":%d}`, word), word
	case "mode":
		word := uint16(n % 4)
		return fmt.Sprintf(`{"word":%d}`, word), word
	case "percent":
		word := uint16(300 + n*20 + int(s.offset))
		return fmt.Sprintf(`{"coefficient":%d,"exponent":-1}`, word), word
	case "gradient":
		word := uint16(12 + n*3)
		return fmt.Sprintf(`{"coefficient":%d,"exponent":0}`, word), word
	default:
		word := uint16(80 + n*17 + int(s.offset))
		return fmt.Sprintf(`{"word":%d}`, word), word
	}
}

func handCases() []handCase {
	var out []handCase
	for chainIndex, chain := range handChains() {
		for _, spec := range handSpecs() {
			blocks := []struct {
				ordinal uint32
				header  uint16
			}{{ordinalStorage, storageHeader}}
			if spec.model == 123 {
				blocks = []struct {
					ordinal uint32
					header  uint16
				}{{ordinalControls, controlsHeader}, {ordinalControlsNext, controlsHeaderNext}}
			}
			for dup, block := range blocks {
				arg, word := spec.value(chainIndex*2 + dup)
				out = append(out, handCase{
					local: fmt.Sprintf("%s.m%d.o%d.%s", chain.label, spec.model, block.ordinal, spec.name),
					unit:  chain.unit, addr: chain.base + block.header + spec.offset, arg: arg, word: word,
				})
			}
		}
	}
	return out
}

func handImage(base uint16) []uint16 {
	cells := make([]uint16, int(base)+200)
	cells[base], cells[base+1] = 0x5375, 0x6e53
	cells[base+ratingHeader], cells[base+ratingHeader+1] = 121, ratingWords
	cells[base+ratingHeader+2] = 8000
	for _, header := range []uint16{controlsHeader, controlsHeaderNext} {
		cells[base+header], cells[base+header+1] = 123, controlsWords
		cells[base+header+23] = 0xffff
	}
	cells[base+storageHeader], cells[base+storageHeader+1] = 124, storageWords
	cells[base+storageHeader+19] = 0
	cells[base+storageHeader+25] = 0xffff
	cells[base+gridHeader], cells[base+gridHeader+1] = 125, gridWords
	cells[base+gridHeader+9] = 0x8000
	cells[base+endHeader], cells[base+endHeader+1] = 0xffff, 0
	return cells
}

func listenHand(t *testing.T) *scriptedLink {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	peer := &scriptedPeer{units: map[byte][]uint16{1: handImage(40000), 2: handImage(41000)}}
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- conn
		_ = peer.serve(conn)
	}()
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var server net.Conn
	select {
	case server = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("synthetic peer did not accept")
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return &scriptedLink{conn: client, peer: peer}
}

func openHand(t *testing.T, access Registers) (*runtime.Runtime, *SunSpecByHand) {
	t.Helper()
	chains := handChains()
	inverters := make([]Inverter, len(chains))
	for i, chain := range chains {
		inverters[i] = Inverter{Unit: chain.unit, Label: chain.label, Base: chain.base}
	}
	rec := &handRecording{SunSpecByHand: &SunSpecByHand{cfg: Config{Access: access, Inverters: inverters}}}
	ctx := context.Background()
	clock := &limitClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	cfg := life.InstanceConfig{Driver: limitsKind, Instance: "limits.hand", Enabled: true, Config: driver.Config{Instance: "limits.hand", Enabled: true}}
	rt, err := runtime.OpenHost(ctx, life.Installation{Key: "installation.sunspec-hand", Bridge: "bridge.test", StateDir: t.TempDir(), Clock: clock, Instances: []life.InstanceConfig{cfg}}, runtime.HostOptions{}, handFactory{rec})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if err = rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = rt.WaitInitial(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.startErr != nil {
		status, snapErr := rt.Snapshot(life.Ref{Driver: limitsKind, Instance: "limits.hand"})
		t.Fatalf("hand start: %v; snapshot %s %v (%v)", rec.startErr, status.ObservedState, status.Reason, snapErr)
	}
	return rt, rec.SunSpecByHand
}

func handList(t *testing.T, rt *runtime.Runtime, root driver.NodeRef) (*tree.TreeCut, map[string]tree.Ref) {
	t.Helper()
	ctx := context.Background()
	cut, err := rt.Tree().Capture(ctx, tree.CutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]tree.Ref{}
	token := ""
	for {
		page, err := cut.List(ctx, root, tree.Page{Size: 50, Token: token, Filter: tree.Filter{Kind: tree.EntryMethod}})
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range page.Entries {
			if entry.Domain == tree.KeywordNative && !entry.Withdrawn {
				found[entry.Local] = entry.Ref
			}
		}
		if page.Next == "" {
			break
		}
		token = page.Next
	}
	return cut, found
}

func handOutsideRef(t *testing.T, root driver.NodeRef, local string) tree.Ref {
	t.Helper()
	path, err := tree.ParseNodePath("/" + string(root.ID))
	if err != nil {
		t.Fatal(err)
	}
	segment, err := tree.NativeSegment(driver.NamespaceID(limitsKind), driver.MemberID(local))
	if err != nil {
		t.Fatal(err)
	}
	return tree.Ref(tree.Key{Node: path, Facet: tree.KeywordMethods, Domain: tree.KeywordNative, Segment: segment}.String())
}

func handInvoke(t *testing.T, rt *runtime.Runtime, cut *tree.TreeCut, link *scriptedLink, ref tree.Ref, arg string) (handResult, string, int) {
	t.Helper()
	ctx := context.Background()
	before := len(link.peer.writtenFrames())
	prepared, err := rt.Prepare(ctx, cut.ID(), ref, tree.Input{Encoded: []byte(arg)})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if got := len(link.peer.writtenFrames()); got != before {
		t.Fatalf("prepare wrote %d frames", got-before)
	}
	receipt, err := rt.Invoke(ctx, prepared)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := rt.Outcome(ctx, receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	var result handResult
	if outcome.Error == "" {
		if err = json.Unmarshal(outcome.Native, &result); err != nil {
			t.Fatal(err)
		}
	}
	return result, outcome.Error, len(link.peer.writtenFrames()) - before
}

type handFlip struct {
	Registers
	unit uint8
	addr uint16
	next uint16
	arm  bool
}

func (f *handFlip) WriteMultiple(ctx context.Context, unit uint8, address uint16, registers []uint16, admission modbus.RegisterWriteAdmission) (modbus.WriteMultipleRegistersAcknowledgement, error) {
	ack, err := f.Registers.WriteMultiple(ctx, unit, address, registers, admission)
	if err == nil && unit == f.unit && address == f.addr && len(registers) == 1 {
		f.arm = true
	}
	return ack, err
}

func (f *handFlip) ReadHolding(ctx context.Context, unit uint8, address, quantity uint16) ([]uint16, error) {
	words, err := f.Registers.ReadHolding(ctx, unit, address, quantity)
	if err == nil && f.arm && unit == f.unit && address == f.addr && quantity == 1 && len(words) == 1 {
		f.arm = false
		words = []uint16{f.next}
	}
	return words, err
}

func TestSunSpecByHandEachPointWritesOnceAndRereads(t *testing.T) {
	link := listenHand(t)
	rt, family := openHand(t, link)
	if len(link.peer.writtenFrames()) != 0 {
		t.Fatal("starting the writer sent a write")
	}
	cut, found := handList(t, rt, family.session.Native().Root())
	cases := handCases()
	want := map[string]handCase{}
	for _, item := range cases {
		want[item.local] = item
	}
	if len(found) != len(want) {
		t.Fatalf("methods %d, want %d: %s", len(found), len(want), handSetDiff(found, want))
	}
	for local := range found {
		if _, ok := want[local]; !ok {
			t.Fatalf("unexpected method %s", local)
		}
		for _, banned := range []string{".m125.", ".OutPFSet", ".WChaMax", ".ChaState"} {
			if len(local) >= len(banned) && (banned == ".m125." && contains(local, banned) || hasSuffix(local, banned) && banned != ".m125.") {
				t.Fatalf("method %s is outside the read-write points", local)
			}
		}
	}
	ctx := context.Background()
	before := len(link.peer.writtenFrames())
	if _, err := rt.Prepare(ctx, cut.ID(), handOutsideRef(t, family.session.Native().Root(), "chain-a.m125.o5.RvtTms"), tree.Input{Encoded: []byte(`{"word":1}`)}); err == nil {
		t.Fatal("a model 125 point was callable")
	}
	if _, err := rt.Prepare(ctx, cut.ID(), handOutsideRef(t, family.session.Native().Root(), "chain-a.m123.o2.OutPFSet"), tree.Input{Encoded: []byte(`{"word":1}`)}); err == nil {
		t.Fatal("a read-only model 123 point was callable")
	}
	if len(link.peer.writtenFrames()) != before {
		t.Fatal("a refused point sent bytes")
	}
	sample := want["chain-a.m123.o2.WMaxLimPct"]
	for _, arg := range []string{
		`{"coefficient":50}`,
		`{"word":50}`,
		`{"coefficient":1,"exponent":-2}`,
		`{"coefficient":50,"exponent":-1,"word":1}`,
		`{"word":9}`,
		``,
	} {
		_, callErr, frames := handInvoke(t, rt, cut, link, found[sample.local], arg)
		if callErr == "" || frames != 0 {
			t.Fatalf("argument %s error %q frames %d", arg, callErr, frames)
		}
	}
	for _, item := range cases {
		result, callErr, frames := handInvoke(t, rt, cut, link, found[item.local], item.arg)
		if callErr != "" || frames != 1 {
			t.Fatalf("%s error %q frames %d", item.local, callErr, frames)
		}
		got := link.peer.writtenFrames()
		frame := got[len(got)-1]
		addr, qty, words := wordsOf(t, frame)
		if frame[6] != item.unit || frame[7] != 16 || addr != item.addr || qty != 1 || len(words) != 1 || words[0] != item.word {
			t.Fatalf("%s frame unit %d function %d addr %d qty %d words %v, want unit %d addr %d word %d", item.local, frame[6], frame[7], addr, qty, words, item.unit, item.addr, item.word)
		}
		if result.Readback != "match" || result.Unit != item.unit || result.Address != item.addr || result.Quantity != 1 || len(result.Written) != 1 || result.Written[0] != item.word || len(result.Reread) != 1 || result.Reread[0] != item.word {
			t.Fatalf("%s result %+v", item.local, result)
		}
	}
	if report := family.Stop(context.Background()); !report.Confirmed {
		t.Fatal("stop was not confirmed")
	}
	_, callErr, frames := handInvoke(t, rt, cut, link, found[sample.local], sample.arg)
	if callErr == "" || frames != 0 {
		t.Fatalf("stopped call error %q frames %d", callErr, frames)
	}
}

func TestSunSpecByHandReadbackCanMismatchOrStayUnknown(t *testing.T) {
	link := listenHand(t)
	mismatchAddr := uint16(40000 + controlsHeader + 4)
	unknownAddr := uint16(40000 + controlsHeader + 2)
	flip := &handFlip{Registers: link, unit: 1, addr: mismatchAddr, next: 1}
	rt, family := openHand(t, flip)
	cut, found := handList(t, rt, family.session.Native().Root())
	conn := found["chain-a.m123.o2.Conn"]
	result, callErr, frames := handInvoke(t, rt, cut, link, conn, `{"word":0}`)
	if callErr != "" || frames != 1 || result.Readback != "mismatch" || len(result.Reread) != 1 || result.Reread[0] != 1 || result.Written[0] != 0 {
		t.Fatalf("mismatch result %+v error %q frames %d", result, callErr, frames)
	}
	frame := link.peer.writtenFrames()[len(link.peer.writtenFrames())-1]
	addr, qty, words := wordsOf(t, frame)
	if addr != mismatchAddr || qty != 1 || words[0] != 0 {
		t.Fatalf("mismatch frame %d x %d %v", addr, qty, words)
	}
	flip.addr, flip.next = unknownAddr, 0xffff
	window := found["chain-a.m123.o2.Conn_WinTms"]
	result, callErr, frames = handInvoke(t, rt, cut, link, window, `{"word":82}`)
	if callErr != "" || frames != 1 || result.Readback != "unknown" || len(result.Reread) != 1 || result.Reread[0] != 0xffff || result.Written[0] != 82 {
		t.Fatalf("unknown result %+v error %q frames %d", result, callErr, frames)
	}
}

func handSetDiff(found map[string]tree.Ref, want map[string]handCase) string {
	var missing, extra []string
	for local := range want {
		if _, ok := found[local]; !ok {
			missing = append(missing, local)
		}
	}
	for local := range found {
		if _, ok := want[local]; !ok {
			extra = append(extra, local)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return fmt.Sprintf("missing %v extra %v", missing, extra)
}

func contains(s, part string) bool {
	return len(s) >= len(part) && (s == part || len(part) == 0 || indexOf(s, part) >= 0)
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
