package limits

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	handSchema driver.SchemaRef = "modbus.sunspec.byhand/v1"
	handScope  driver.ScopeID   = "control"
)

// SunSpecByHand registers one native method for each read-write point of SunSpec
// models 123 and 124. Calling a method writes that point's span once, through
// the same admitted register write used by the limit executors, and re-reads it.
// Registration and discovery send nothing.
type SunSpecByHand struct {
	cfg     Config
	mu      sync.Mutex
	fenced  atomic.Bool
	running bool
	session driver.Session
	reg     sunspec.DecoderRegistry
	ready   bool
}

type handID struct {
	inv     Inverter
	model   uint16
	ordinal uint32
	name    string
	field   string
}

type handBody struct {
	Word        *uint16 `json:"word"`
	Coefficient *int64  `json:"coefficient"`
	Exponent    *int16  `json:"exponent"`
}

type handResult struct {
	Readback string   `json:"readback"`
	Unit     uint8    `json:"unit"`
	Address  uint16   `json:"address"`
	Quantity uint16   `json:"quantity"`
	Written  []uint16 `json:"written"`
	Reread   []uint16 `json:"reread,omitempty"`
}

// FenceWithdrawal ends later sends without blocking.
func (h *SunSpecByHand) FenceWithdrawal() { h.fenced.Store(true) }

// Stop marks the writer stopped. A later call sends nothing.
func (h *SunSpecByHand) Stop(context.Context) driver.CloseReport {
	h.FenceWithdrawal()
	h.mu.Lock()
	h.running = false
	h.mu.Unlock()
	return driver.CloseReport{Confirmed: true}
}

// Start registers one method per read-write point on the instance node. It sends nothing.
func (h *SunSpecByHand) Start(ctx context.Context, session driver.Session) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ctx == nil || session == nil || h.cfg.Access == nil {
		return errors.New("sunspec hand write requires a context, a session and register access")
	}
	if h.running {
		return errors.New("sunspec hand write is already running")
	}
	h.fenced.Store(false)
	points, err := h.collect(ctx)
	if err != nil {
		return err
	}
	if err = h.register(session, points); err != nil {
		return err
	}
	h.session, h.running = session, true
	return nil
}

func (h *SunSpecByHand) collect(ctx context.Context) ([]handID, error) {
	reg, err := h.registry()
	if err != nil {
		return nil, err
	}
	var out []handID
	for _, inv := range h.cfg.Inverters {
		snap, err := h.readChain(ctx, inv)
		if err != nil {
			return nil, err
		}
		for _, occ := range snap.Occurrences() {
			if occ.ModelID() != 123 && occ.ModelID() != 124 {
				continue
			}
			found, err := handPoints(reg, snap, inv, occ)
			if err != nil {
				return nil, err
			}
			out = append(out, found...)
		}
	}
	return out, nil
}

func handPoints(reg sunspec.DecoderRegistry, snap sunspec.ChainSnapshot, inv Inverter, occ sunspec.Occurrence) ([]handID, error) {
	writes, err := sunspec.PointWrites(reg, snap, occ.Ordinal)
	if err != nil {
		return nil, err
	}
	decoded, err := reg.DecodeOccurrence(occ)
	if err != nil {
		return nil, err
	}
	out := make([]handID, 0, len(writes))
	for _, write := range writes {
		field, _, _ := write.Point()
		fact, ok := decoded.Fact(field)
		if !ok || fact.PointName == "" {
			return nil, errors.New("sunspec hand write point has no name")
		}
		out = append(out, handID{inv: inv, model: occ.ModelID(), ordinal: occ.Ordinal, name: fact.PointName, field: field})
	}
	return out, nil
}

func (id handID) local() driver.MemberID {
	return driver.MemberID(id.inv.Label + ".m" + strconv.Itoa(int(id.model)) + ".o" + strconv.FormatUint(uint64(id.ordinal), 10) + "." + id.name)
}

func (h *SunSpecByHand) register(session driver.Session, points []handID) error {
	root := session.Native().Root()
	for _, id := range points {
		captured := id
		spec := driver.MethodSpec{Local: id.local(), Schema: handSchema, Scope: handScope}
		if err := session.Native().Method(root, spec, func(ctx context.Context, arg []byte) ([]byte, error) {
			return h.invoke(ctx, captured, arg)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (h *SunSpecByHand) invoke(ctx context.Context, id handID, argument []byte) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.fenced.Load() || !h.running || h.cfg.Access == nil {
		return nil, errors.New("sunspec hand write is not running")
	}
	if handClosed(ctx) {
		return nil, errors.New("sunspec hand write was withdrawn")
	}
	snap, err := h.readChain(ctx, id.inv)
	if err != nil {
		return nil, err
	}
	reg, err := h.registry()
	if err != nil {
		return nil, err
	}
	point, err := matchHand(reg, snap, id)
	if err != nil {
		return nil, err
	}
	native, err := handNative(point, argument)
	if err != nil {
		return nil, err
	}
	request, err := point.Encode(native)
	if err != nil {
		return nil, err
	}
	if h.fenced.Load() || handClosed(ctx) {
		return nil, errors.New("sunspec hand write was withdrawn")
	}
	return h.finish(ctx, id, point, request)
}

func matchHand(reg sunspec.DecoderRegistry, snap sunspec.ChainSnapshot, id handID) (sunspec.PointWrite, error) {
	if id.model != 123 && id.model != 124 {
		return sunspec.PointWrite{}, errors.New("sunspec hand write point is outside models 123 and 124")
	}
	for _, occ := range snap.Occurrences() {
		if occ.Ordinal != id.ordinal {
			continue
		}
		if occ.ModelID() != id.model {
			return sunspec.PointWrite{}, errors.New("sunspec hand write occurrence changed")
		}
		writes, err := sunspec.PointWrites(reg, snap, occ.Ordinal)
		if err != nil {
			return sunspec.PointWrite{}, err
		}
		decoded, err := reg.DecodeOccurrence(occ)
		if err != nil {
			return sunspec.PointWrite{}, err
		}
		for _, write := range writes {
			field, _, _ := write.Point()
			fact, ok := decoded.Fact(field)
			if field == id.field && ok && fact.PointName == id.name {
				return write, nil
			}
		}
		return sunspec.PointWrite{}, errors.New("sunspec hand write point is not writable")
	}
	return sunspec.PointWrite{}, errors.New("sunspec hand write occurrence is absent")
}

func handNative(point sunspec.PointWrite, argument []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(argument))
	dec.DisallowUnknownFields()
	var body handBody
	if err := dec.Decode(&body); err != nil {
		return nil, errors.New("sunspec hand write argument is not a point value")
	}
	if dec.More() {
		return nil, errors.New("sunspec hand write argument is not a point value")
	}
	if handTakesDecimal(point) {
		if body.Word != nil || body.Coefficient == nil || body.Exponent == nil {
			return nil, errors.New("sunspec hand write wants a coefficient and an exponent")
		}
		return sunspec.Decimal{Coefficient: *body.Coefficient, Exponent: *body.Exponent}, nil
	}
	if body.Coefficient != nil || body.Exponent != nil || body.Word == nil {
		return nil, errors.New("sunspec hand write wants one word")
	}
	return *body.Word, nil
}

func handClosed(ctx context.Context) bool {
	return ctx == nil || ctx.Err() != nil
}

func handTakesDecimal(point sunspec.PointWrite) bool {
	_, err := point.Encode(sunspec.Decimal{})
	return err == nil
}

func (h *SunSpecByHand) finish(ctx context.Context, id handID, point sunspec.PointWrite, request sunspec.PointWriteRequest) ([]byte, error) {
	ack, err := h.cfg.Access.WriteMultiple(ctx, id.inv.Unit, request.Address, request.Words, request.Admission())
	if modbus.IsRegisterWriteUnsent(err) {
		return nil, err
	}
	result := handResult{
		Readback: string(sunspec.ReadbackUnknown), Unit: id.inv.Unit,
		Address: request.Address, Quantity: request.WordCount, Written: append([]uint16(nil), request.Words...),
	}
	if err != nil || ack.Address != request.Address || ack.Quantity != request.WordCount {
		return json.Marshal(result)
	}
	got, err := h.cfg.Access.ReadHolding(ctx, id.inv.Unit, request.Address, request.WordCount)
	if err != nil {
		return json.Marshal(result)
	}
	result.Reread = append([]uint16(nil), got...)
	result.Readback = string(handReadback(point.PointType(), request.Words, got))
	return json.Marshal(result)
}

func handReadback(kind sunspec.PointType, wrote, got []uint16) sunspec.ReadbackState {
	if len(got) != len(wrote) || len(wrote) == 0 {
		return sunspec.ReadbackUnknown
	}
	same := true
	for i := range wrote {
		if wrote[i] != got[i] {
			same = false
			break
		}
	}
	if same {
		return sunspec.ReadbackMatch
	}
	if len(got) == 1 && handUnestablished(kind, got[0]) {
		return sunspec.ReadbackUnknown
	}
	return sunspec.ReadbackMismatch
}

func handUnestablished(kind sunspec.PointType, word uint16) bool {
	switch kind {
	case sunspec.TypeUint16, sunspec.TypeEnum16, sunspec.TypeBitfield16, sunspec.TypeCount:
		return word == 0xffff
	case sunspec.TypeInt16, sunspec.TypeScaleFactor:
		return word == 0x8000
	default:
		return false
	}
}

func (h *SunSpecByHand) readChain(ctx context.Context, inv Inverter) (sunspec.ChainSnapshot, error) {
	plan, err := sunspec.NewListedChainPlan(nil, []uint16{inv.Base}, sunspec.ChainLimits{MaxTotalWords: 4096, MaxOccurrences: 16})
	if err != nil {
		return sunspec.ChainSnapshot{}, err
	}
	chain := sunspec.NewChain(plan)
	for step := 0; step < 64; step++ {
		next := chain.NextRequests()
		if len(next) == 0 {
			return sunspec.ChainSnapshot{}, errors.New("sunspec chain ended before the end marker")
		}
		req := next[0]
		words, err := h.cfg.Access.ReadHolding(ctx, inv.Unit, req.Address(), req.WordCount())
		if err != nil {
			return sunspec.ChainSnapshot{}, err
		}
		view, err := logicalView(inv, uint64(step+1), req.Address(), words)
		if err != nil {
			return sunspec.ChainSnapshot{}, err
		}
		snap, err := chain.AdmitReplay(req, view)
		if err != nil {
			return sunspec.ChainSnapshot{}, err
		}
		if len(snap.RawWords()) != 0 {
			return snap, nil
		}
	}
	return sunspec.ChainSnapshot{}, errors.New("sunspec chain walk did not finish")
}

func (h *SunSpecByHand) registry() (sunspec.DecoderRegistry, error) {
	if h.ready {
		return h.reg, nil
	}
	reg, err := sunspec.NewStandardSunSpecDecoderRegistry(sunspec.ModelsRevisionV1)
	if err != nil {
		return sunspec.DecoderRegistry{}, err
	}
	h.reg, h.ready = reg, true
	return reg, nil
}
