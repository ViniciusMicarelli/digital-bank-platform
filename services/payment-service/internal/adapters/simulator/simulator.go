// Package simulator stands in for the boleto registry (CIP) and the
// clearing network (COMPE) in development and the sandbox.
package simulator

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/ports"
)

var bankNames = map[string]string{
	"001": "Banco do Brasil S.A.", "033": "Banco Santander (Brasil) S.A.", "077": "Banco Inter S.A.",
	"104": "Caixa Econômica Federal", "237": "Banco Bradesco S.A.", "260": "Nu Pagamentos S.A.", "341": "Itaú Unibanco S.A.",
}

// SandboxPayees finds the payee of boletos issued in the sandbox.
type SandboxPayees interface {
	SandboxPayee(ctx context.Context, barcode string) (name, document string, ok bool, err error)
}

// Registry answers with the sandbox payee (or a generic one) and reports
// boletos whose free field starts with 00000 as already paid.
type Registry struct{ payees SandboxPayees }

func NewRegistry(payees SandboxPayees) *Registry { return &Registry{payees: payees} }

var _ ports.Registry = (*Registry)(nil)

func (r *Registry) Lookup(ctx context.Context, b domain.Boleto) (domain.Payee, error) {
	if b.FreeField[:5] == "00000" {
		return domain.Payee{}, domain.Refuse("BOLETO_ALREADY_PAID", "the registry shows this boleto as paid")
	}
	bank := bankNames[b.BankCode]
	if bank == "" {
		bank = "Banco " + b.BankCode
	}
	name, doc, ok, err := r.payees.SandboxPayee(ctx, b.Barcode)
	if err != nil {
		return domain.Payee{}, err
	}
	if !ok {
		name, doc = "BENEFICIARIO "+b.FreeField[:6], "**.***.***/0001-**"
	}
	return domain.Payee{Name: name, DocumentMasked: doc, BankName: bank}, nil
}

// Clearing settles at once, idempotently per payment, and refuses boletos
// whose free field starts with 99999 (the payee's bank rejects them).
type Clearing struct {
	mu      sync.Mutex
	settled map[uuid.UUID]error
}

func NewClearing() *Clearing { return &Clearing{settled: map[uuid.UUID]error{}} }

var _ ports.Clearing = (*Clearing)(nil)

func (c *Clearing) Settle(_ context.Context, payment uuid.UUID, b domain.Boleto, _ int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if res, ok := c.settled[payment]; ok {
		return res
	}
	var res error
	if b.FreeField[:5] == "99999" {
		res = domain.Refuse("SETTLEMENT_REJECTED", "the payee's bank refused the payment")
	}
	c.settled[payment] = res
	return res
}
