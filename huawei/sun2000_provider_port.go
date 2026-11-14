package huawei

import (
	"context"
	"errors"
	"sync"
)

// I13Identity binds a caller-local documentary stream, never a device address.
type I13Identity struct {
	I13Subject, I13Candidate, I13Evidence, I13Source string
	I13Generation                                    uint64
}

// I13Request is what one cycle asks the supplier for: the provider identity
// at the time of the call and a receipt number that increases by one per
// started cycle.
type I13Request struct {
	I13Identity I13Identity
	I13Receipt  uint64
}

// I13Evidence is the supplier's answer. I13Request must echo the request
// exactly; I13Bytes is a Sun2000EvidenceInput document that the provider
// copies before parsing.
type I13Evidence struct {
	I13Request I13Request
	I13Bytes   []byte
}

// I13Supplier supplies one already-held record synchronously. It must cooperate
// with cancellation and return within caller bounds; it performs no acquisition.
// Returned buffers must remain unchanged until I13Cycle returns.
type I13Supplier func(context.Context, I13Request) (I13Evidence, error)

// I13Record is one accepted documentary record: its request, the copied raw
// bytes kept as native evidence, and the parsed input. Records returned by the
// provider are deep copies.
type I13Record struct {
	I13Request I13Request
	I13Bytes   []byte
	I13Input   Sun2000EvidenceInput
}

// I13Result separates this attempt from prior structurally valid documentary
// input. Receipts are local sequence numbers, not observation times or freshness.
type I13Result struct {
	I13Outcome              string
	I13Current, I13Retained *I13Record
	I13Rejected             *I13Request
}

// I13Port is family-local; it is not a production Provider or admission API.
type I13Port interface {
	I13Cycle(context.Context) (I13Result, error)
	I13Replace(I13Identity) error
	I13Retire()
}

// I13Provider must not be copied. Its zero value refuses all work.
type I13Provider struct {
	i13Mu               sync.Mutex
	i13Identity         I13Identity
	i13Limits           Sun2000EvidenceInputLimits
	i13Supplier         I13Supplier
	i13Receipt          uint64
	i13Busy, i13Retired bool
	i13Last             *I13Record
}

func i13Valid(id I13Identity) bool {
	return id.I13Subject != "" && id.I13Candidate != "" && id.I13Evidence != "" && id.I13Source != "" && id.I13Generation > 0
}

// I13NewProvider returns a provider for one documentary stream. It refuses an
// identity with an empty field or a zero generation, limits without a positive
// byte and record budget, and a nil supplier.
func I13NewProvider(id I13Identity, limits Sun2000EvidenceInputLimits, supplier I13Supplier) (*I13Provider, error) {
	if !i13Valid(id) || limits.MaxBytes <= 0 || limits.MaxRecords <= 0 || supplier == nil {
		return nil, errors.New("I13 invalid configuration")
	}
	return &I13Provider{i13Identity: id, i13Limits: limits, i13Supplier: supplier}, nil
}

func i13Clone(record *I13Record) *I13Record {
	if record == nil {
		return nil
	}
	out := *record
	out.I13Bytes = append([]byte(nil), record.I13Bytes...)
	out.I13Input.evidence = record.I13Input.Evidence()
	out.I13Input.candidates = record.I13Input.Candidates()
	return &out
}

func (p *I13Provider) i13Result(request *I13Request) I13Result {
	return I13Result{I13Outcome: "NO_ADMISSIBLE_PROFILE", I13Retained: i13Clone(p.i13Last), I13Rejected: request}
}

// I13Cycle asks the supplier for one record and, when it is valid, makes it
// the retained record. The outcome is always NO_ADMISSIBLE_PROFILE. A busy,
// retired, zero-value or receipt-exhausted provider and a cancelled context
// are refused without calling the supplier. The supplier is called without
// the lock held. A supplier error, an echoed request that differs, bytes over
// the budget, a parse failure, or a record whose subject, evidence and
// candidate do not join the identity returns an error with the request in
// I13Rejected and the prior record in I13Retained. A result that arrives after
// cancellation, retirement or replacement is never committed.
func (p *I13Provider) I13Cycle(ctx context.Context) (I13Result, error) {
	denied := I13Result{I13Outcome: "NO_ADMISSIBLE_PROFILE"}
	if p == nil || ctx == nil {
		return denied, errors.New("I13 invalid cycle")
	}
	p.i13Mu.Lock()
	if p.i13Supplier == nil || p.i13Retired || p.i13Busy || p.i13Receipt == ^uint64(0) {
		result := p.i13Result(nil)
		p.i13Mu.Unlock()
		return result, errors.New("I13 unavailable cycle")
	}
	if err := ctx.Err(); err != nil {
		result := p.i13Result(nil)
		p.i13Mu.Unlock()
		return result, err
	}
	p.i13Receipt++
	request := I13Request{p.i13Identity, p.i13Receipt}
	limits, supplier := p.i13Limits, p.i13Supplier
	p.i13Busy = true
	p.i13Mu.Unlock()
	// Release the refusal fence even if caller code panics; the panic propagates.
	defer func() { p.i13Mu.Lock(); p.i13Busy = false; p.i13Mu.Unlock() }()
	evidence, supplyErr := supplier(ctx, request)
	var record *I13Record
	var err error
	if supplyErr != nil {
		err = errors.New("I13 supplier failed")
	} else if evidence.I13Request != request {
		err = errors.New("I13 receipt identity mismatch")
	} else if len(evidence.I13Bytes) > limits.MaxBytes {
		err = errors.New("I13 byte budget exceeded")
	} else {
		data := append([]byte(nil), evidence.I13Bytes...)
		var input Sun2000EvidenceInput
		input, err = ParseSun2000EvidenceInput(data, limits)
		if err == nil && !i13Joined(input, request.I13Identity) {
			err = errors.New("I13 documentary join mismatch")
		}
		if err == nil {
			record = &I13Record{request, data, input}
		}
	}
	p.i13Mu.Lock()
	defer p.i13Mu.Unlock()
	if ctx.Err() != nil {
		return p.i13Result(&request), ctx.Err()
	}
	if p.i13Retired || p.i13Identity != request.I13Identity {
		return p.i13Result(&request), errors.New("I13 retired receipt")
	}
	if err != nil {
		return p.i13Result(&request), err
	}
	p.i13Last = record
	return I13Result{I13Outcome: "NO_ADMISSIBLE_PROFILE", I13Current: i13Clone(record)}, nil
}

func i13Joined(input Sun2000EvidenceInput, id I13Identity) bool {
	if input.Subject() != id.I13Subject {
		return false
	}
	found := false
	for _, ref := range input.Evidence() {
		if ref.ID() == id.I13Evidence {
			found = true
		}
	}
	if !found {
		return false
	}
	for _, candidate := range input.Candidates() {
		if candidate.ID() != id.I13Candidate {
			continue
		}
		for _, refs := range [][]string{candidate.Family().Refs(), candidate.Model().Refs(), candidate.Firmware().Refs(), candidate.Unit().Refs(), candidate.Map().Refs()} {
			for _, ref := range refs {
				if ref == id.I13Evidence {
					return true
				}
			}
		}
	}
	return false
}

// I13Replace invalidates prior-source evidence and pending callbacks. Generations
// strictly increase even when only source or subject changes; receipt order stays.
func (p *I13Provider) I13Replace(id I13Identity) error {
	if p == nil || !i13Valid(id) {
		return errors.New("I13 invalid replacement")
	}
	p.i13Mu.Lock()
	defer p.i13Mu.Unlock()
	if p.i13Supplier == nil || p.i13Retired || id.I13Generation <= p.i13Identity.I13Generation {
		return errors.New("I13 invalid replacement generation")
	}
	p.i13Identity = id
	p.i13Last = nil
	return nil
}

// I13Retire permanently refuses further cycles and replacements and drops the
// retained record. It is idempotent.
func (p *I13Provider) I13Retire() {
	if p == nil {
		return
	}
	p.i13Mu.Lock()
	defer p.i13Mu.Unlock()
	p.i13Retired = true
	p.i13Last = nil
}
