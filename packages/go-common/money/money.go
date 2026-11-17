// Package money provides an immutable, currency-aware monetary value.
//
// Amounts are stored as int64 minor units (e.g. cents). Floating point is
// never used: values are parsed from and formatted to decimal strings.
// All arithmetic checks for currency mismatch and int64 overflow.
package money

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Currency is an ISO 4217 alphabetic code.
type Currency string

const (
	BRL Currency = "BRL"
	USD Currency = "USD"
	EUR Currency = "EUR"
)

// decimals holds the number of minor-unit digits for each supported currency.
var decimals = map[Currency]int{
	BRL: 2,
	USD: 2,
	EUR: 2,
}

var (
	ErrUnsupportedCurrency = errors.New("money: unsupported currency")
	ErrCurrencyMismatch    = errors.New("money: currency mismatch")
	ErrOverflow            = errors.New("money: amount overflows int64")
	ErrInvalidAmount       = errors.New("money: invalid amount")
)

// Decimals returns the number of minor-unit digits for c.
func (c Currency) Decimals() (int, error) {
	d, ok := decimals[c]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnsupportedCurrency, string(c))
	}
	return d, nil
}

// Money is an amount in minor units of a given currency.
// The zero value is not valid; build values with New or Parse.
type Money struct {
	amount   int64
	currency Currency
}

// New returns minor units of currency c (New(1050, BRL) is R$ 10,50).
// Negative amounts are allowed: ledger postings are signed.
func New(minor int64, c Currency) (Money, error) {
	if _, err := c.Decimals(); err != nil {
		return Money{}, err
	}
	return Money{amount: minor, currency: c}, nil
}

// Zero returns a zero amount of currency c.
func Zero(c Currency) (Money, error) { return New(0, c) }

// Parse reads a decimal string such as "10.5", "-0.01" or "1234".
// It rejects more fractional digits than the currency allows.
func Parse(s string, c Currency) (Money, error) {
	d, err := c.Decimals()
	if err != nil {
		return Money{}, err
	}
	invalid := fmt.Errorf("%w: %q", ErrInvalidAmount, s)

	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	intPart, fracPart, hasDot := strings.Cut(s, ".")
	if intPart == "" || (hasDot && fracPart == "") || len(fracPart) > d ||
		!isDigits(intPart) || !isDigits(fracPart) {
		return Money{}, invalid
	}
	fracPart += strings.Repeat("0", d-len(fracPart))

	minor, err := strconv.ParseInt(intPart+fracPart, 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("%w: %q", ErrOverflow, s)
	}
	if neg {
		minor = -minor
	}
	return Money{amount: minor, currency: c}, nil
}

// MustParse is like Parse but panics on error. Intended for tests and constants.
func MustParse(s string, c Currency) Money {
	m, err := Parse(s, c)
	if err != nil {
		panic(err)
	}
	return m
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Minor returns the amount in minor units.
func (m Money) Minor() int64 { return m.amount }

// Currency returns the currency.
func (m Money) Currency() Currency { return m.currency }

func (m Money) IsZero() bool     { return m.amount == 0 }
func (m Money) IsNegative() bool { return m.amount < 0 }
func (m Money) IsPositive() bool { return m.amount > 0 }

// Neg returns -m.
func (m Money) Neg() (Money, error) {
	if m.amount == math.MinInt64 {
		return Money{}, ErrOverflow
	}
	return Money{amount: -m.amount, currency: m.currency}, nil
}

// Add returns m + o.
func (m Money) Add(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, m.currency, o.currency)
	}
	sum := m.amount + o.amount
	// Overflow happened if both operands share a sign that the result lacks.
	if (m.amount > 0 && o.amount > 0 && sum < 0) || (m.amount < 0 && o.amount < 0 && sum >= 0) {
		return Money{}, ErrOverflow
	}
	return Money{amount: sum, currency: m.currency}, nil
}

// Sub returns m - o.
func (m Money) Sub(o Money) (Money, error) {
	neg, err := o.Neg()
	if err != nil {
		return Money{}, err
	}
	return m.Add(neg)
}

// Cmp compares m and o: -1 if m < o, 0 if equal, +1 if m > o.
func (m Money) Cmp(o Money) (int, error) {
	if m.currency != o.currency {
		return 0, fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, m.currency, o.currency)
	}
	switch {
	case m.amount < o.amount:
		return -1, nil
	case m.amount > o.amount:
		return 1, nil
	}
	return 0, nil
}

// Sum adds all values. They must share currency c; an empty list returns zero.
func Sum(c Currency, values ...Money) (Money, error) {
	total, err := Zero(c)
	if err != nil {
		return Money{}, err
	}
	for _, v := range values {
		if total, err = total.Add(v); err != nil {
			return Money{}, err
		}
	}
	return total, nil
}

// Split divides m into n parts whose sum is exactly m. Leftover minor units
// go to the first parts: R$ 10,00 / 3 = [3,34 3,33 3,33].
func (m Money) Split(n int) ([]Money, error) {
	if n <= 0 {
		return nil, fmt.Errorf("money: split into %d parts", n)
	}
	q, r := m.amount/int64(n), m.amount%int64(n)
	step := int64(1)
	if r < 0 {
		step, r = -1, -r
	}
	parts := make([]Money, n)
	for i := range parts {
		parts[i] = Money{amount: q, currency: m.currency}
		if int64(i) < r {
			parts[i].amount += step
		}
	}
	return parts, nil
}

// Decimal formats the amount as a plain decimal string, e.g. "-10.50".
func (m Money) Decimal() string {
	d := decimals[m.currency]
	if d == 0 {
		return strconv.FormatInt(m.amount, 10)
	}
	// Work on the unsigned magnitude so math.MinInt64 is handled.
	mag := uint64(m.amount)
	sign := ""
	if m.amount < 0 {
		sign, mag = "-", -mag
	}
	digits := strconv.FormatUint(mag, 10)
	if len(digits) <= d {
		digits = strings.Repeat("0", d-len(digits)+1) + digits
	}
	cut := len(digits) - d
	return sign + digits[:cut] + "." + digits[cut:]
}

// String implements fmt.Stringer, e.g. "BRL 10.50".
func (m Money) String() string { return string(m.currency) + " " + m.Decimal() }

type jsonMoney struct {
	Amount   string   `json:"amount"`
	Currency Currency `json:"currency"`
}

// MarshalJSON encodes as {"amount":"10.50","currency":"BRL"}. The amount is a
// string so that clients never parse it as a float.
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonMoney{Amount: m.Decimal(), Currency: m.currency})
}

// UnmarshalJSON decodes the format produced by MarshalJSON.
func (m *Money) UnmarshalJSON(b []byte) error {
	var j jsonMoney
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	parsed, err := Parse(j.Amount, j.Currency)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}
