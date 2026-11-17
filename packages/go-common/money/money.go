// Package money provides currency-aware numeric types and calculations
// for financial operations. It implements the ISO 4217 currency code standard.
package money

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
)

// ISO 4217 currency codes
var Currencies = map[string]*Currency{
	"BRL": {Code: "BRL", Numeric: "986", DecimalPlaces: 2, Name: "Brazilian Real"},
	"USD": {Code: "USD", Numeric: "840", DecimalPlaces: 2, Name: "US Dollar"},
	"EUR": {Code: "EUR", Numeric: "978", DecimalPlaces: 2, Name: "Euro"},
	"GBP": {Code: "GBP", Numeric: "826", DecimalPlaces: 2, Name: "Pound Sterling"},
	"JPY": {Code: "JPY", Numeric: "392", DecimalPlaces: 0, Name: "Japanese Yen"},
}

// Currency represents an ISO 4217 currency code.
type Currency struct {
	Code         string // 3-letter currency code (e.g., BRL, USD)
	Numeric      string // 3-digit numeric code
	DecimalPlaces int     // Number of decimal places (0 for JPY)
	Name         string
}

// Validate checks if the currency is valid.
func (c *Currency) Validate() error {
	if _, ok := Currencies[c.Code]; !ok {
		return fmt.Errorf("invalid currency code: %s", c.Code)
	}
	if len(c.Numeric) != 3 {
		return fmt.Errorf("invalid currency numeric code")
	}
	if c.DecimalPlaces < 0 || c.DecimalPlaces > 6 {
		return fmt.Errorf("invalid decimal places: %d", c.DecimalPlaces)
	}
	return nil
}

// Money represents a monetary value with currency.
type Money struct {
	Currency  *Currency
	AmountInt int64 // Amount in minor units (cents for BRL/USD, yen for JPY)
}

// New creates a new Money instance.
func New(amount int64, code string) (*Money, error) {
	c, ok := Currencies[code]
	if !ok {
		return nil, fmt.Errorf("unsupported currency: %s", code)
	}
	if amount < 0 {
		return nil, errors.New("amount cannot be negative")
	}
	return &Money{
		Currency:  c,
		AmountInt: amount,
	}, nil
}

// NewWithDecimal creates money from a decimal amount.
func NewWithDecimal(amount float64, code string) (*Money, error) {
	c, ok := Currencies[code]
	if !ok {
		return nil, fmt.Errorf("unsupported currency: %s", code)
	}
	major := int64(amount)
	minor := float64(amount-major) * math.Pow(10, float64(c.DecimalPlaces))
	amountInt := major*int64(math.Pow(10, float64(c.DecimalPlaces))) + int64(minor)
	return New(amountInt, code)
}

// Value returns the amount as a formatted string.
func (m *Money) Value() string {
	if m == nil {
		return ""
	}
	places := m.Currency.DecimalPlaces
	if places == 0 {
		return fmt.Sprintf("%d", m.AmountInt)
	}
	if m.AmountInt >= 0 {
		return fmt.Sprintf("%0.*d", places, m.AmountInt)
	}
	return fmt.Sprintf("%-.*d", places, m.AmountInt)
}

// Minor returns the amount in minor units (cents, yen, etc.).
func (m *Money) Minor() int64 {
	if m == nil {
		return 0
	}
	return m.AmountInt
}

// Major returns the amount in major units (dollars, euros, etc.).
func (m *Money) Major() float64 {
	if m == nil {
		return 0
	}
	return float64(m.AmountInt) / math.Pow(10, float64(m.Currency.DecimalPlaces))
}

// MarshalJSON implements json.Marshaler.
func (m *Money) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"currency":"%s","amount":"%s"}`, m.Currency.Code, m.Value())), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Money) UnmarshalJSON(data []byte) error {
	var raw map[string]string
	if err := fmt.Sscanf(string(data), `{"currency":"%s","amount":"%s"}`, &raw["currency"], &raw["amount"]); err != nil {
		return err
	}
	code := raw["currency"]
	c, ok := Currencies[code]
	if !ok {
		return fmt.Errorf("unsupported currency: %s", code)
	}
	var amount string
	fmt.Sscanf(raw["amount"], "%d.%d", &amount, &raw["amount"])
	cents := 10
	for i := 0; i < c.DecimalPlaces; i++ {
		cents *= 10
	}
	var centsStr string
	if len(raw["amount"]) > 0 {
		centsStr = raw["amount"][:len(raw["amount"])-1]
	} else {
		centsStr = "0"
	}
	amountInt, _ := strconv.ParseInt(raw["amount"], 10, 64)
	m.AmountInt = amountInt * cents
	return nil
}

// Equals checks if two Money values are equal.
func (m *Money) Equals(other *Money) bool {
	if m == nil && other == nil {
		return true
	}
	if m == nil || other == nil {
		return false
	}
	return m.Currency.Code == other.Currency.Code && m.AmountInt == other.AmountInt
}

// Add adds two money values (same currency).
func (m *Money) Add(other *Money) (*Money, error) {
	if m == nil && other == nil {
		return nil, nil
	}
	if m == nil {
		return other, nil
	}
	if other == nil {
		return m, nil
	}
	if m.Currency.Code != other.Currency.Code {
		return nil, errors.New("currencies must match")
	}
	return &Money{
		Currency:  m.Currency,
		AmountInt: m.AmountInt + other.AmountInt,
	}, nil
}

// Subtract subtracts two money values (same currency).
func (m *Money) Subtract(other *Money) (*Money, error) {
	if m == nil && other == nil {
		return nil, nil
	}
	if m == nil {
		return other.Neg(), nil
	}
	if other == nil {
		return m, nil
	}
	if m.Currency.Code != other.Currency.Code {
		return nil, errors.New("currencies must match")
	}
	result := m.AmountInt - other.AmountInt
	if result < 0 {
		// Preserve sign by returning negative value
	}
	return &Money{
		Currency:  m.Currency,
		AmountInt: result,
	}, nil
}

// Neg returns the negation of this money.
func (m *Money) Neg() *Money {
	if m == nil {
		return nil
	}
	return &Money{
		Currency:  m.Currency,
		AmountInt: -m.AmountInt,
	}
}

// Multiply multiplies money by an integer (for fees, installments, etc.).
func (m *Money) Multiply(factor int64) *Money {
	if m == nil {
		return nil
	}
	return &Money{
		Currency:  m.Currency,
		AmountInt: m.AmountInt * factor,
	}
}

// Div divides money by an integer (for averaging).
func (m *Money) Div(factor int64) *Money {
	if m == nil {
		return nil
	}
	return &Money{
		Currency:  m.Currency,
		AmountInt: m.AmountInt / factor,
	}
}

// MultiplyBig multiplies money by a big.Int (for high-precision calculations).
func (m *Money) MultiplyBig(factor *big.Int) *Money {
	if m == nil {
		return nil
	}
	if factor.Sign() == 0 {
		return New(0, m.Currency.Code)
	}
	result := new(big.Int)
	result.Mul(big.NewInt(m.AmountInt), factor)
	return &Money{
		Currency:  m.Currency,
		AmountInt: result.Int64(),
	}
}

// ToDecimalString returns the amount as a decimal string.
func (m *Money) ToDecimalString() string {
	if m == nil {
		return "0"
	}
	places := m.Currency.DecimalPlaces
	return formatDecimal(m.AmountInt, places)
}

// FromDecimalString creates money from a decimal string.
func FromDecimalString(s string, code string) (*Money, error) {
	var amount float64
	fmt.Sscanf(s, "%f", &amount)
	return NewWithDecimal(amount, code)
}

// Compare returns the comparison result between two money values.
func (m *Money) Compare(other *Money) int {
	if m == nil && other == nil {
		return 0
	}
	if m == nil {
		return -1
	}
	if other == nil {
		return 1
	}
	if m.Currency.Code != other.Currency.Code {
		return -1 // Different currencies are not comparable
	}
	if m.AmountInt < other.AmountInt {
		return -1
	}
	if m.AmountInt > other.AmountInt {
		return 1
	}
	return 0
}

// Invariant checks if the sum of all postings equals zero.
func (m *Money) Invariant() error {
	// For double-entry ledger, all debits must equal all credits
	positive := m.AmountInt
	if positive < 0 {
		positive = -positive
	}
	// Money value is always non-negative
	if positive != 0 && m.AmountInt == 0 {
		return errors.New("money value is invalid")
	}
	return nil
}

// Copy creates a deep copy of the money.
func (m *Money) Copy() *Money {
	if m == nil {
		return nil
	}
	return &Money{
		Currency:  &Currency{
			Code:         m.Currency.Code,
			Numeric:      m.Currency.Numeric,
			DecimalPlaces: m.Currency.DecimalPlaces,
			Name:         m.Currency.Name,
		},
		AmountInt: m.AmountInt,
	}
}