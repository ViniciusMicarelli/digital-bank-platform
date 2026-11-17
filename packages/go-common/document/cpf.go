// Package document validates and formats Brazilian taxpayer documents.
package document

import (
	"errors"
	"strings"
	"unicode"
)

// ErrInvalidCPF is returned for malformed CPFs or wrong check digits.
var ErrInvalidCPF = errors.New("invalid CPF")

// NormalizeCPF strips the usual punctuation ("529.982.247-25") and validates
// the two check digits. It returns the 11 digits.
func NormalizeCPF(s string) (string, error) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '.' || r == '-' || r == ' ':
		default:
			return "", ErrInvalidCPF
		}
	}
	digits := b.String()
	if len(digits) != 11 || strings.Count(digits, digits[:1]) == 11 {
		return "", ErrInvalidCPF
	}
	for _, n := range []int{9, 10} {
		sum := 0
		for i := range n {
			sum += int(digits[i]-'0') * (n + 1 - i)
		}
		d := sum * 10 % 11
		if d == 10 {
			d = 0
		}
		if d != int(digits[n]-'0') {
			return "", ErrInvalidCPF
		}
	}
	return digits, nil
}

// MaskCPF hides all but the middle digits, as shown in Pix confirmations:
// "52998224725" -> "***.982.247-**".
func MaskCPF(digits string) string {
	if len(digits) != 11 {
		return "***"
	}
	return "***." + digits[3:6] + "." + digits[6:9] + "-**"
}
