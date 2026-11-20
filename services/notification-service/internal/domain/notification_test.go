package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func ev(t *testing.T, typ string, p map[string]any) Event {
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return Event{ID: uuid.NewString(), Type: typ, Payload: raw}
}

func stale(e Event) Event {
	e.OccurredAt = time.Now().Add(-MaxAge - time.Hour)
	return e
}

func TestBRL(t *testing.T) {
	for minor, want := range map[int64]string{5: "R$ 0,05", 1_234_56: "R$ 1.234,56", 1_000_000_00: "R$ 1.000.000,00", -15_00: "-R$ 15,00"} {
		if got := BRL(minor); got != want {
			t.Errorf("BRL(%d) = %s, want %s", minor, got, want)
		}
	}
}

func TestFromEvent(t *testing.T) {
	acc := uuid.NewString()
	cases := []struct {
		typ     string
		payload map[string]any
		title   string
		body    string
	}{
		{"pix.transfer.received", map[string]any{"account_id": acc, "amount_minor": 15050, "counterparty_name": "Bruno"},
			"Pix recebido", "Você recebeu R$ 150,50 de Bruno."},
		{"card.authorization.approved", map[string]any{"account_id": acc, "amount_minor": 4200, "merchant_name": "PADARIA", "function": "CREDIT"},
			"Compra aprovada", "Compra de R$ 42,00 em PADARIA no crédito."},
		{"card.dispute.opened", map[string]any{"account_id": acc, "amount_minor": 8000, "merchant_name": "LOJA X", "reason": "NOT_RECOGNIZED"},
			"Contestação aberta", "Recebemos sua contestação de R$ 80,00 em LOJA X. Avisamos quando ela for analisada. Por segurança, bloqueamos o cartão."},
		{"card.dispute.decided", map[string]any{"account_id": acc, "amount_minor": 8000, "merchant_name": "LOJA X", "status": "LOST", "decision_reason": "comprovante de entrega"},
			"Contestação recusada", "Sua contestação de R$ 80,00 em LOJA X foi recusada: comprovante de entrega."},
		{"card.purchase.refunded", map[string]any{"account_id": acc, "amount_minor": 2500, "merchant_name": "LOJA X", "function": "CREDIT"},
			"Estorno recebido", "R$ 25,00 de LOJA X voltaram na sua fatura."},
		{"statement.budget.threshold", map[string]any{"account_id": acc, "category": "FOOD", "threshold": 80, "budget": 50000, "spent": 41000},
			"80% do orçamento: Alimentação", "Você já gastou R$ 410,00 dos R$ 500,00 planejados para o mês."},
		{"statement.budget.threshold", map[string]any{"account_id": acc, "category": "TOTAL", "threshold": 100, "budget": 300000, "spent": 312050},
			"Orçamento estourado: Gastos do mês", "Você gastou R$ 3.120,50 este mês, acima do orçamento de R$ 3.000,00."},
		{"credit.limit.changed", map[string]any{"account_id": acc, "previous_limit": 100000, "granted": 300000},
			"Limite aumentado", "Seu limite passou de R$ 1.000,00 para R$ 3.000,00."},
		{"credit.invoice.planned", map[string]any{"account_id": acc, "installments": 6, "installment": 11807},
			"Fatura parcelada", "Sua fatura foi parcelada em 6x de R$ 118,07. A primeira parcela já está na fatura atual."},
		{"credit.invoice.closed", map[string]any{"account_id": acc, "remaining": 25000, "due_date": "2026-12-20"},
			"Sua fatura fechou", "Fatura de R$ 250,00 com vencimento em 20/12/2026."},
		{"user.kyc.approved", map[string]any{"customer_id": "c-1"}, "Cadastro aprovado",
			"Sua identidade foi verificada. Você já pode abrir sua conta."},
		{"transaction.completed", map[string]any{"type": "INTERNAL_TRANSFER", "to_account_id": acc, "amount_minor": 1000, "metadata": map[string]string{}},
			"Transferência recebida", "Você recebeu uma transferência de R$ 10,00."},
	}
	for _, c := range cases {
		ds, err := FromEvent(ev(t, c.typ, c.payload), time.Now())
		wantAccount := acc
		if c.typ == "user.kyc.approved" {
			wantAccount = uuid.Nil.String()
		}
		if err != nil || len(ds) != 1 || ds[0].Title != c.title || ds[0].Body != c.body || ds[0].AccountID.String() != wantAccount {
			t.Errorf("%s: %+v %v", c.typ, ds, err)
		}
	}
	for _, skip := range []Event{
		ev(t, "pix.transfer.completed", map[string]any{"account_id": acc, "direction": "INCOMING"}),
		ev(t, "transaction.completed", map[string]any{"type": "INTERNAL_TRANSFER", "to_account_id": acc, "metadata": map[string]string{"end_to_end_id": "E1"}}),
		ev(t, "transaction.completed", map[string]any{"type": "PIX_OUT", "from_account_id": acc}),
		ev(t, "credit.invoice.closed", map[string]any{"account_id": acc, "remaining": 0}),
		ev(t, "ledger.entry.posted", map[string]any{}),
		ev(t, "card.authorization.approved", map[string]any{"amount_minor": 100}), // no account in old events
		stale(ev(t, "pix.transfer.received", map[string]any{"account_id": acc})),
	} {
		if ds, err := FromEvent(skip, time.Now()); err != nil || len(ds) != 0 {
			t.Errorf("%s should not notify: %+v %v", skip.Type, ds, err)
		}
	}
}

func TestChannels(t *testing.T) {
	d := Draft{Important: true}
	if ch := DefaultPreferences.Channels(d); len(ch) != 2 || ch[1] != Push {
		t.Fatalf("default = %v", ch)
	}
	if ch := (Preferences{Push: false, Email: true}).Channels(d); len(ch) != 2 || ch[1] != Email {
		t.Fatalf("email only = %v", ch)
	}
	if ch := (Preferences{Email: true}).Channels(Draft{}); len(ch) != 1 {
		t.Fatalf("not important, no e-mail = %v", ch)
	}
}
