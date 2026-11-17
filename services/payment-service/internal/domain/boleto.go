// Package domain models boletos (Brazilian bank payment slips) and their
// payments. Amounts are BRL minor units (centavos).
package domain

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
	ErrInvalidState    = errors.New("invalid state")
	ErrConflict        = errors.New("idempotency key reused with a different request")
	ErrBusy            = errors.New("payment is being processed")
)

// RefusedError is a business refusal (422 to the customer; compensation in
// the saga).
type RefusedError struct{ Reason, Msg string }

func (e *RefusedError) Error() string { return e.Reason + ": " + e.Msg }

func Refuse(reason, msg string) error { return &RefusedError{Reason: reason, Msg: msg} }

func AsRefused(err error) (*RefusedError, bool) {
	var r *RefusedError
	ok := errors.As(err, &r)
	return r, ok
}

// Boleto is a bank boleto (FEBRABAN layout): a 44-digit barcode, shown to
// people as a 47-digit "linha digitável" with check digits per field.
type Boleto struct {
	Barcode  string
	Line     string
	BankCode string
	// DueDate is zero when the boleto has no due date (factor 0000).
	DueDate time.Time
	// Amount is 0 for boletos the payer fills in (open amount).
	Amount int64
	// FreeField is the issuing bank's 25 digits (agreement, our number...).
	FreeField string
}

// ParseBoleto accepts a linha digitável (47 digits) or a barcode (44), with
// or without punctuation, and checks every check digit.
func ParseBoleto(s string, today time.Time) (Boleto, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '.' || r == ' ' || r == '-' {
			return -1
		}
		return 'x'
	}, s)
	if strings.ContainsRune(digits, 'x') {
		return Boleto{}, fmt.Errorf("%w: a boleto has only digits", ErrInvalidArgument)
	}
	var barcode string
	switch len(digits) {
	case 47:
		fields := []struct{ data, dv string }{{digits[0:9], digits[9:10]}, {digits[10:20], digits[20:21]}, {digits[21:31], digits[31:32]}}
		for i, f := range fields {
			if mod10(f.data) != f.dv {
				return Boleto{}, fmt.Errorf("%w: wrong check digit in field %d", ErrInvalidArgument, i+1)
			}
		}
		barcode = digits[0:4] + digits[32:33] + digits[33:47] + digits[4:9] + digits[10:20] + digits[21:31]
	case 44:
		barcode = digits
	case 48:
		return Boleto{}, Refuse("UNSUPPORTED_BOLETO", "utility and tax bills (arrecadação) are not supported yet")
	default:
		return Boleto{}, fmt.Errorf("%w: a boleto has 47 digits (or a 44-digit barcode)", ErrInvalidArgument)
	}
	if barcode[3] != '9' {
		return Boleto{}, fmt.Errorf("%w: only BRL boletos (currency 9)", ErrInvalidArgument)
	}
	if barcodeDV(barcode[:4]+barcode[5:]) != barcode[4:5] {
		return Boleto{}, fmt.Errorf("%w: wrong general check digit", ErrInvalidArgument)
	}
	factor, _ := strconv.Atoi(barcode[5:9])
	amount, _ := strconv.ParseInt(barcode[9:19], 10, 64)
	b := Boleto{Barcode: barcode, Line: lineOf(barcode), BankCode: barcode[0:3], Amount: amount, FreeField: barcode[19:44]}
	if factor != 0 {
		b.DueDate = dateOfFactor(factor, today)
	}
	return b, nil
}

// NewBoleto builds a valid boleto (sandbox issuing, tests).
func NewBoleto(bank string, due time.Time, amount int64, freeField string) (Boleto, error) {
	if len(bank) != 3 || len(freeField) != 25 || amount < 0 || amount > 99_999_999_99 {
		return Boleto{}, fmt.Errorf("%w: bank has 3 digits, free field 25, amount up to 10 digits", ErrInvalidArgument)
	}
	factor := 0
	if !due.IsZero() {
		factor = factorOf(due)
	}
	body := bank + "9" + fmt.Sprintf("%04d%010d", factor, amount) + freeField
	barcode := body[:4] + barcodeDV(body) + body[4:]
	return ParseBoleto(barcode, due)
}

// RandomFreeField returns prefix followed by random digits, 25 in all.
func RandomFreeField(prefix string) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	for sb.Len() < 25 {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		sb.WriteByte(byte('0' + n.Int64()))
	}
	return sb.String()
}

func lineOf(barcode string) string {
	f1 := barcode[0:4] + barcode[19:24]
	f2 := barcode[24:34]
	f3 := barcode[34:44]
	return f1 + mod10(f1) + f2 + mod10(f2) + f3 + mod10(f3) + barcode[4:5] + barcode[5:19]
}

// FormatLine adds the usual punctuation: AAAAA.AAAAA BBBBB.BBBBBB CCCCC.CCCCCC D EEEEEEEEEEEEEE.
func FormatLine(l string) string {
	return l[0:5] + "." + l[5:10] + " " + l[10:15] + "." + l[15:21] + " " + l[21:26] + "." + l[26:32] + " " + l[32:33] + " " + l[33:47]
}

// mod10 is the check digit of each field of the linha digitável.
func mod10(s string) string {
	sum, weight := 0, 2
	for i := len(s) - 1; i >= 0; i-- {
		p := int(s[i]-'0') * weight
		sum += p/10 + p%10
		weight = 3 - weight
	}
	return strconv.Itoa((10 - sum%10) % 10)
}

// barcodeDV is the general check digit (modulo 11, weights 2..9) over the 43
// other digits of the barcode; 0, 10 and 11 become 1.
func barcodeDV(s string) string {
	sum, weight := 0, 2
	for i := len(s) - 1; i >= 0; i-- {
		sum += int(s[i]-'0') * weight
		if weight++; weight > 9 {
			weight = 2
		}
	}
	dv := 11 - sum%11
	if dv == 0 || dv == 10 || dv == 11 {
		dv = 1
	}
	return strconv.Itoa(dv)
}

// The due date factor counts days from 1997-10-07. It reached 9999 on
// 2025-02-21 and restarted at 1000 on 2025-02-22.
var (
	factorBase  = time.Date(1997, 10, 7, 0, 0, 0, 0, time.UTC)
	factorReset = time.Date(2025, 2, 22, 0, 0, 0, 0, time.UTC)
)

func factorOf(d time.Time) int {
	if d.Before(factorReset) {
		return int(d.Sub(factorBase).Hours() / 24)
	}
	return 1000 + int(d.Sub(factorReset).Hours()/24)%9000
}

// dateOfFactor picks, among the dates a factor can mean, the closest to today.
func dateOfFactor(f int, today time.Time) time.Time {
	best := factorBase.AddDate(0, 0, f)
	if f >= 1000 {
		for cycle := 0; cycle < 3; cycle++ {
			c := factorReset.AddDate(0, 0, f-1000+cycle*9000)
			if absDays(c, today) < absDays(best, today) {
				best = c
			}
		}
	}
	return best
}

func absDays(a, b time.Time) float64 {
	d := a.Sub(b).Hours() / 24
	if d < 0 {
		return -d
	}
	return d
}

// ---- charges ----

const (
	FineBP             = 200 // 2%
	InterestBPPerMonth = 100 // 1% a month, pro rata per day
	PaymentLimitDays   = 59  // after this, the boleto can no longer be paid
)

// EffectiveDueDate moves a due date on a weekend to the next Monday (bank
// holidays are planned).
func EffectiveDueDate(due time.Time) time.Time {
	switch due.Weekday() {
	case time.Saturday:
		return due.AddDate(0, 0, 2)
	case time.Sunday:
		return due.AddDate(0, 0, 1)
	}
	return due
}

// Charges for paying on payDate: a 2% fine and 1% a month of interest once
// past the (effective) due date; refused past the payment limit.
func Charges(nominal int64, due, payDate time.Time) (fine, interest int64, err error) {
	if due.IsZero() || !payDate.After(EffectiveDueDate(due)) {
		return 0, 0, nil
	}
	days := int64(payDate.Sub(due).Hours() / 24)
	if days > PaymentLimitDays {
		return 0, 0, Refuse("BOLETO_EXPIRED", fmt.Sprintf("the boleto could be paid until %d days after its due date", PaymentLimitDays))
	}
	fine = (nominal*FineBP + 5000) / 10000
	interest = (nominal*InterestBPPerMonth*days + 150000) / 300000 // nominal * 1% * days/30
	return fine, interest, nil
}
