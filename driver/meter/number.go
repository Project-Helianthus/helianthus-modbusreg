package meter

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

const maxExponent = 18

type factor struct{ num, den int64 }

var (
	one        = factor{1, 1}
	perHundred = factor{1, 100}
	perHour    = factor{3600, 1}
	kiloToBase = factor{1000, 1}
	kwhToJoule = factor{3600000, 1}
)

func ratOf(d semreg.Decimal) (*big.Rat, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	n, ok := new(big.Int).SetString(d.Coefficient, 10)
	if !ok {
		return nil, fmt.Errorf("coefficient %q", d.Coefficient)
	}
	return scaled(new(big.Rat).SetInt(n), int64(d.Exponent10)), nil
}

func scaled(r *big.Rat, exponent int64) *big.Rat {
	magnitude := exponent
	if magnitude < 0 {
		magnitude = -magnitude
	}
	p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(magnitude), nil))
	if exponent < 0 {
		return r.Quo(r, p)
	}
	return r.Mul(r, p)
}

// decimalOf returns the exact decimal of r. A value that is not exact in the
// decimal range is an error; nothing is rounded here.
func decimalOf(r *big.Rat) (semreg.Decimal, error) {
	s := new(big.Rat).Set(r)
	scaled(s, maxExponent)
	if !s.IsInt() {
		return semreg.Decimal{}, fmt.Errorf("%s is not exact within %d decimal places", r.RatString(), maxExponent)
	}
	n := new(big.Int).Set(s.Num())
	if n.Sign() == 0 {
		return semreg.Decimal{Coefficient: "0"}, nil
	}
	exp, ten := int32(-maxExponent), big.NewInt(10)
	for exp < maxExponent {
		q, rem := new(big.Int).QuoRem(n, ten, new(big.Int))
		if rem.Sign() != 0 {
			break
		}
		n, exp = q, exp+1
	}
	d := semreg.Decimal{Coefficient: n.String(), Exponent10: exp}
	return d, d.Validate()
}

func (f factor) apply(r *big.Rat) *big.Rat {
	return new(big.Rat).Mul(r, big.NewRat(f.num, f.den))
}

func quantity(r *big.Rat, unit semreg.DefinitionID) (semreg.Value, error) {
	d, err := decimalOf(r)
	if err != nil {
		return semreg.Value{}, err
	}
	return semreg.Value{Kind: semreg.ValueQuantity, Quantity: &semreg.Quantity{Number: d, Unit: unit}}, nil
}

// parseNumber reads a plain decimal text such as "-12.5".
func parseNumber(text string) (*big.Rat, error) {
	if strings.ContainsAny(text, "eE") {
		return nil, fmt.Errorf("%q is not a plain decimal", text)
	}
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, fmt.Errorf("%q is not a number", text)
	}
	return r, nil
}
