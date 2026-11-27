package storage

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// Conversion is the arithmetic of one row: the registry value is the native
// value times Mul over Div, plus Add. A quotient that does not terminate in
// eighteen decimal places is rounded half to even to Places decimal places,
// the places that return the native value (rule 21, F3-049).
type Conversion struct {
	Mul, Div int64
	Add      string
	Places   int32
}

// maxExactPlaces is the largest exponent magnitude a decimal states exactly.
const maxExactPlaces = 18

func ratOf(text string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(text))
	if !ok {
		return nil, fmt.Errorf("storage: %q is not a decimal number", text)
	}
	return r, nil
}

// floatRat is the exact decimal of the shortest text a float64 prints as, the
// value the native decoder states.
func floatRat(v float64) (*big.Rat, error) {
	return ratOf(strconv.FormatFloat(v, 'f', -1, 64))
}

// Apply converts one native value.
func (c Conversion) Apply(native *big.Rat) (semreg.Decimal, error) {
	mul, div := c.Mul, c.Div
	if mul == 0 {
		mul = 1
	}
	if div == 0 {
		div = 1
	}
	r := new(big.Rat).Mul(native, big.NewRat(mul, div))
	if c.Add != "" {
		add, err := ratOf(c.Add)
		if err != nil {
			return semreg.Decimal{}, err
		}
		r.Add(r, add)
	}
	return decimalOf(r, c.Places), nil
}

func pow10(n int32) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

func decimalOf(r *big.Rat, places int32) semreg.Decimal {
	for p := int32(0); p <= maxExactPlaces; p++ {
		scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(pow10(p)))
		if scaled.IsInt() {
			return trimmed(new(big.Int).Set(scaled.Num()), -p)
		}
	}
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(pow10(places)))
	return trimmed(roundHalfEven(scaled), -places)
}

func roundHalfEven(v *big.Rat) *big.Int {
	q, rem := new(big.Int).QuoRem(v.Num(), v.Denom(), new(big.Int))
	twice := new(big.Int).Abs(rem)
	twice.Lsh(twice, 1)
	switch cmp := twice.Cmp(v.Denom()); {
	case cmp > 0, cmp == 0 && q.Bit(0) == 1:
		q.Add(q, big.NewInt(int64(v.Sign())))
	}
	return q
}

func trimmed(n *big.Int, exponent int32) semreg.Decimal {
	if n.Sign() == 0 {
		return semreg.Decimal{Coefficient: "0"}
	}
	ten := big.NewInt(10)
	for exponent < maxExactPlaces {
		q, rem := new(big.Int).QuoRem(n, ten, new(big.Int))
		if rem.Sign() != 0 {
			break
		}
		n, exponent = q, exponent+1
	}
	return semreg.Decimal{Coefficient: n.String(), Exponent10: exponent}
}

func decimalRat(d semreg.Decimal) *big.Rat {
	n, _ := new(big.Int).SetString(d.Coefficient, 10)
	r := new(big.Rat).SetInt(n)
	switch {
	case d.Exponent10 > 0:
		r.Mul(r, new(big.Rat).SetInt(pow10(d.Exponent10)))
	case d.Exponent10 < 0:
		r.Quo(r, new(big.Rat).SetInt(pow10(-d.Exponent10)))
	}
	return r
}

func quantity(n semreg.Decimal, unit semreg.DefinitionID) semreg.Value {
	return semreg.Value{Kind: semreg.ValueQuantity, Quantity: &semreg.Quantity{Number: n, Unit: unit}}
}

func symbol(namespace semreg.DefinitionID, token string) semreg.Value {
	return semreg.Value{Kind: semreg.ValueSymbol, Symbol: &semreg.Symbol{Namespace: namespace, Token: token, Known: true}}
}
