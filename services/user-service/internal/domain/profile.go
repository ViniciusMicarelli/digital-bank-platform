// Package domain models the customer profile and its KYC (know your
// customer) verification.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/document"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
)

// RefusedError is a business refusal (422).
type RefusedError struct{ Reason, Msg string }

func (e *RefusedError) Error() string { return e.Reason + ": " + e.Msg }

func Refuse(reason, msg string) error { return &RefusedError{Reason: reason, Msg: msg} }

type KYCStatus string

const (
	KYCNotStarted   KYCStatus = "NOT_STARTED"
	KYCPending      KYCStatus = "PENDING"
	KYCApproved     KYCStatus = "APPROVED"
	KYCRejected     KYCStatus = "REJECTED"
	KYCManualReview KYCStatus = "MANUAL_REVIEW"
)

type Address struct {
	PostalCode string // CEP, 8 digits
	Street     string
	Number     string
	Complement string
	District   string
	City       string
	State      string // UF
}

type Profile struct {
	CustomerID string
	FullName   string
	CPF        string // 11 digits
	BirthDate  time.Time
	Phone      string
	Address    Address
	KYCStatus  KYCStatus
	KYCReason  string
	UpdatedAt  time.Time
	DecidedAt  *time.Time
	ReviewedBy string
}

const MinAge = 18

var (
	states = []string{"AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG", "PA", "PB", "PR", "PE",
		"PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO"}
	cepRE   = regexp.MustCompile(`^\d{8}$`)
	phoneRE = regexp.MustCompile(`^\+55\d{10,11}$`)
)

func invalid(msg string) error { return fmt.Errorf("%w: %s", ErrInvalidArgument, msg) }

// Normalize validates the input and puts it in canonical form.
func (p *Profile) Normalize() error {
	p.FullName = strings.Join(strings.Fields(p.FullName), " ")
	if len(strings.Fields(p.FullName)) < 2 || len(p.FullName) > 120 {
		return invalid("full name: first and last name, up to 120 characters")
	}
	cpf, err := document.NormalizeCPF(p.CPF)
	if err != nil {
		return invalid("CPF is not valid")
	}
	p.CPF = cpf
	if p.BirthDate.IsZero() || p.BirthDate.Year() < 1900 {
		return invalid("birth date is required")
	}
	p.Phone = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(p.Phone)
	if p.Phone != "" && !phoneRE.MatchString(p.Phone) {
		return invalid("phone must be +55 with area code, e.g. +5511912345678")
	}
	a := &p.Address
	a.PostalCode = strings.ReplaceAll(a.PostalCode, "-", "")
	a.State = strings.ToUpper(strings.TrimSpace(a.State))
	switch {
	case !cepRE.MatchString(a.PostalCode):
		return invalid("postal code (CEP) has 8 digits")
	case strings.TrimSpace(a.Street) == "" || strings.TrimSpace(a.Number) == "" || strings.TrimSpace(a.City) == "":
		return invalid("street, number and city are required")
	case !slices.Contains(states, a.State):
		return invalid("state must be a Brazilian UF, e.g. SP")
	}
	return nil
}

// Age in whole years on a date.
func (p Profile) Age(on time.Time) int {
	age := on.Year() - p.BirthDate.Year()
	if on.Month() < p.BirthDate.Month() || (on.Month() == p.BirthDate.Month() && on.Day() < p.BirthDate.Day()) {
		age-- // birthday not reached yet this year
	}
	return age
}

// Update replaces the profile with in. Once verified (or being verified),
// the identity (name, CPF, birth date) is locked; contact data is not.
func (p *Profile) Update(in Profile) error {
	locked := p.KYCStatus == KYCApproved || p.KYCStatus == KYCPending || p.KYCStatus == KYCManualReview
	if locked && (in.FullName != p.FullName || in.CPF != p.CPF || !in.BirthDate.Equal(p.BirthDate)) {
		return Refuse("IDENTITY_LOCKED", "name, CPF and birth date cannot change after verification starts")
	}
	p.FullName, p.CPF, p.BirthDate, p.Phone, p.Address = in.FullName, in.CPF, in.BirthDate, in.Phone, in.Address
	if p.KYCStatus == KYCRejected {
		p.KYCStatus, p.KYCReason = KYCNotStarted, "" // corrected data can be submitted again
	}
	return nil
}

// Submit starts verification.
func (p *Profile) Submit(today time.Time) error {
	switch p.KYCStatus {
	case KYCNotStarted:
	case KYCApproved, KYCPending, KYCManualReview:
		return nil // idempotent
	default:
		return Refuse("KYC_NOT_RESUBMITTABLE", "update the profile before submitting again")
	}
	p.KYCStatus, p.KYCReason = KYCPending, ""
	if p.Age(today) < MinAge {
		p.KYCStatus, p.KYCReason = KYCRejected, "UNDERAGE"
	}
	return nil
}

// BureauDecision is the credit bureau / government database answer.
type BureauDecision string

const (
	BureauApprove BureauDecision = "APPROVE"
	BureauReject  BureauDecision = "REJECT"
	BureauReview  BureauDecision = "REVIEW"
)

// Decide applies a bureau (or analyst) decision to a pending verification.
func (p *Profile) Decide(d BureauDecision, reason string, now time.Time) error {
	if p.KYCStatus != KYCPending && p.KYCStatus != KYCManualReview {
		return Refuse("KYC_NOT_PENDING", fmt.Sprintf("verification is %s", p.KYCStatus))
	}
	if p.KYCStatus == KYCManualReview && d == BureauReview {
		return invalid("a manual review ends in APPROVE or REJECT")
	}
	p.KYCStatus, p.KYCReason = map[BureauDecision]KYCStatus{
		BureauApprove: KYCApproved, BureauReject: KYCRejected, BureauReview: KYCManualReview}[d], reason
	if p.KYCStatus != KYCManualReview {
		p.DecidedAt = &now
	}
	return nil
}
