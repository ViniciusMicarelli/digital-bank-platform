package domain

import (
	"errors"
	"testing"
	"time"
)

func valid() Profile {
	return Profile{CustomerID: "c1", FullName: "  Ana   Souza ", CPF: "529.982.247-25", BirthDate: time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC),
		Phone: "+55 (11) 91234-5678", Address: Address{PostalCode: "01310-100", Street: "Av. Paulista", Number: "1000", City: "São Paulo", State: "sp"},
		KYCStatus: KYCNotStarted}
}

func TestNormalize(t *testing.T) {
	p := valid()
	if err := p.Normalize(); err != nil || p.FullName != "Ana Souza" || p.CPF != "52998224725" || p.Phone != "+5511912345678" ||
		p.Address.PostalCode != "01310100" || p.Address.State != "SP" {
		t.Fatalf("normalized = %+v %v", p, err)
	}
	for name, mod := range map[string]func(*Profile){
		"one name":  func(p *Profile) { p.FullName = "Ana" },
		"bad CPF":   func(p *Profile) { p.CPF = "529.982.247-26" },
		"bad CEP":   func(p *Profile) { p.Address.PostalCode = "0131" },
		"bad UF":    func(p *Profile) { p.Address.State = "XX" },
		"bad phone": func(p *Profile) { p.Phone = "912345678" },
		"no birth":  func(p *Profile) { p.BirthDate = time.Time{} },
	} {
		p := valid()
		mod(&p)
		if err := p.Normalize(); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestKYC(t *testing.T) {
	today := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	p := valid()
	_ = p.Normalize()
	if p.Age(today) != 36 || p.Age(time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)) != 35 {
		t.Fatalf("age = %d", p.Age(today))
	}
	_ = p.Submit(today)
	if p.KYCStatus != KYCPending {
		t.Fatal(p.KYCStatus)
	}
	var r *RefusedError
	other := p
	other.CPF = "11144477735"
	if err := p.Update(other); !errors.As(err, &r) || r.Reason != "IDENTITY_LOCKED" {
		t.Fatalf("changing the CPF during verification: %v", err)
	}
	_ = p.Decide(BureauReview, "PEP", today)
	if p.KYCStatus != KYCManualReview || p.DecidedAt != nil {
		t.Fatalf("review = %+v", p)
	}
	_ = p.Decide(BureauApprove, "", today)
	if p.KYCStatus != KYCApproved || p.DecidedAt == nil {
		t.Fatalf("approved = %+v", p)
	}
	contact := p
	contact.Phone = "+5521987654321"
	if err := p.Update(contact); err != nil {
		t.Fatalf("contact data stays editable: %v", err)
	}

	minor := valid()
	minor.BirthDate = today.AddDate(-17, 0, 0)
	_ = minor.Submit(today)
	if minor.KYCStatus != KYCRejected || minor.KYCReason != "UNDERAGE" {
		t.Fatalf("minor = %+v", minor)
	}
	if err := minor.Submit(today); !errors.As(err, &r) {
		t.Fatal("rejected profiles must be corrected first")
	}
	fixed := minor
	fixed.BirthDate = today.AddDate(-30, 0, 0)
	_ = minor.Update(fixed)
	if minor.KYCStatus != KYCNotStarted {
		t.Fatalf("after correction = %s", minor.KYCStatus)
	}
}
