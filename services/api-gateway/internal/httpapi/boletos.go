package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	paymentv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/payment/v1"
)

func payeeJSON(p *paymentv1.Payee) map[string]string {
	return map[string]string{"name": p.GetName(), "document": p.GetDocumentMasked(), "bank": p.GetBankName()}
}

func boletoPaymentJSON(p *paymentv1.BoletoPayment) map[string]any {
	return map[string]any{"id": p.GetId(), "status": p.GetStatus(), "failure_reason": p.GetFailureReason(),
		"line": p.GetLine(), "payee": payeeJSON(p.GetPayee()), "due_date": p.GetDueDate(),
		"nominal": moneyJSON(p.GetNominal()), "fine": moneyJSON(p.GetFine()), "interest": moneyJSON(p.GetInterest()),
		"amount": moneyJSON(p.GetAmount()), "scheduled_for": p.GetScheduledFor(), "created_at": ts(p.GetCreatedAt())}
}

// lookupBoleto shows what a boleto is and costs today, before paying it.
func (a *API) lookupBoleto(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Line string `json:"line"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Payments.LookupBoleto(r.Context(), &paymentv1.LookupBoletoRequest{Line: req.Line})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	q := resp.GetQuote()
	writeJSON(w, http.StatusOK, map[string]any{"line": q.GetLine(), "formatted_line": q.GetFormattedLine(),
		"payee": payeeJSON(q.GetPayee()), "due_date": q.GetDueDate(), "nominal": moneyJSON(q.GetNominal()),
		"fine": moneyJSON(q.GetFine()), "interest": moneyJSON(q.GetInterest()), "total": moneyJSON(q.GetTotal())})
}

func (a *API) payBoleto(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		Line         string `json:"line"`
		Amount       string `json:"amount"`
		ScheduledFor string `json:"scheduled_for"`
	}
	if !decode(w, r, &req) {
		return
	}
	in := &paymentv1.PayBoletoRequest{IdempotencyKey: key, AccountId: acc.GetId(), Line: req.Line, ScheduledFor: req.ScheduledFor}
	if req.Amount != "" {
		if in.Amount, ok = parseBRL(w, r, req.Amount); !ok {
			return
		}
	}
	resp, err := a.c.Payments.PayBoleto(r.Context(), in)
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	p := resp.GetPayment()
	if p.GetStatus() == "REJECTED" || p.GetStatus() == "FAILED" {
		writeError(w, r.Context(), http.StatusUnprocessableEntity, p.GetFailureReason(), "the boleto was not paid")
		return
	}
	writeJSON(w, http.StatusCreated, boletoPaymentJSON(p))
}

func (a *API) listBoletoPayments(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Payments.ListBoletoPayments(r.Context(), &paymentv1.ListBoletoPaymentsRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, p := range resp.GetPayments() {
		out = append(out, boletoPaymentJSON(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"payments": out})
}

// ownedBoletoPayment checks {paymentID} was made from the {id} account.
func (a *API) ownedBoletoPayment(next func(http.ResponseWriter, *http.Request, *paymentv1.BoletoPayment)) func(http.ResponseWriter, *http.Request, *accountv1.Account) {
	return func(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
		id := r.PathValue("paymentID")
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "payment not found")
			return
		}
		resp, err := a.c.Payments.GetBoletoPayment(r.Context(), &paymentv1.GetBoletoPaymentRequest{Id: id})
		if err != nil || resp.GetPayment().GetAccountId() != acc.GetId() {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "payment not found")
			return
		}
		next(w, r, resp.GetPayment())
	}
}

func (a *API) getBoletoPayment(w http.ResponseWriter, _ *http.Request, p *paymentv1.BoletoPayment) {
	writeJSON(w, http.StatusOK, boletoPaymentJSON(p))
}

func (a *API) cancelBoletoPayment(w http.ResponseWriter, r *http.Request, p *paymentv1.BoletoPayment) {
	resp, err := a.c.Payments.CancelBoletoPayment(r.Context(), &paymentv1.CancelBoletoPaymentRequest{Id: p.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, boletoPaymentJSON(resp.GetPayment()))
}

// issueSandboxBoleto (sandbox) creates a valid boleto to pay.
func (a *API) issueSandboxBoleto(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Amount    string `json:"amount"`
		DueDate   string `json:"due_date"`
		PayeeName string `json:"payee_name"`
		Behavior  string `json:"behavior"`
	}
	if !decode(w, r, &req) {
		return
	}
	in := &paymentv1.IssueSandboxBoletoRequest{DueDate: req.DueDate, PayeeName: req.PayeeName, Behavior: req.Behavior}
	if req.Amount != "" {
		var ok bool
		if in.Amount, ok = parseBRL(w, r, req.Amount); !ok {
			return
		}
	}
	resp, err := a.c.Payments.IssueSandboxBoleto(r.Context(), in)
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"line": resp.GetLine(), "formatted_line": resp.GetFormattedLine()})
}
