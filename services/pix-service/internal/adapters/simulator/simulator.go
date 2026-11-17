// Package simulator stands in for the Central Bank's DICT and SPI, which a
// real participant reaches over the RSFN network with signed XML messages.
package simulator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/ports"
)

// Directory knows a few keys held at other banks.
type Directory struct{ keys map[string]domain.Party }

// NewDirectory returns a DICT with sample external keys:
//
//	maria@outrobanco.com.br      Maria Oliveira, Banco Exemplo (ISPB 12345678)
//	+5511987654321               João Pereira, Banco Exemplo
//	recusa@outrobanco.com.br     a receiver whose bank rejects every payment
func NewDirectory() *Directory {
	other := func(name, doc string) domain.Party {
		return domain.Party{Name: name, DocumentMasked: doc, ISPB: "12345678", BankName: "Banco Exemplo"}
	}
	return &Directory{keys: map[string]domain.Party{
		"maria@outrobanco.com.br":  other("Maria Oliveira", "***.456.789-**"),
		"+5511987654321":           other("João Pereira", "***.321.654-**"),
		"recusa@outrobanco.com.br": other("Conta Encerrada", "***.000.000-**"),
	}}
}

var _ ports.Directory = (*Directory)(nil)

func (d *Directory) Lookup(_ context.Context, key string) (domain.Party, error) {
	if p, ok := d.keys[key]; ok {
		return p, nil
	}
	return domain.Party{}, fmt.Errorf("%w: key %s", domain.ErrNotFound, key)
}

// SPI settles payments instantly. It is idempotent on the E2E id and rejects
// payments to keys containing "recusa" (the receiving bank refuses them).
type SPI struct {
	mu      sync.Mutex
	settled map[string]error
}

func NewSPI() *SPI { return &SPI{settled: map[string]error{}} }

var _ ports.SPI = (*SPI)(nil)

func (s *SPI) Send(_ context.Context, e2e string, _ domain.Party, key string, _ money.Money) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if res, ok := s.settled[e2e]; ok {
		return res
	}
	var res error
	if strings.Contains(key, "recusa") {
		res = domain.Reject("RECEIVER_BANK_REJECTED", "the receiving bank rejected the payment (AC03)")
	}
	s.settled[e2e] = res
	return res
}
