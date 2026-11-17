package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
)

func creditLineJSON(l *creditv1.CreditLine) map[string]any {
	return map[string]any{"id": l.GetId(), "account_id": l.GetAccountId(), "status": l.GetStatus(),
		"limit": moneyJSON(l.GetLimit()), "used": moneyJSON(l.GetUsed()), "available": moneyJSON(l.GetAvailable()),
		"closing_day": l.GetClosingDay(), "monthly_interest_rate": float64(l.GetInterestRateBp()) / 100}
}

func invoiceJSON(i *creditv1.Invoice) map[string]any {
	return map[string]any{"id": i.GetId(), "status": i.GetStatus(), "closing_date": i.GetClosingDate(),
		"due_date": i.GetDueDate(), "total": moneyJSON(i.GetTotal()), "paid": moneyJSON(i.GetPaid()),
		"remaining": moneyJSON(i.GetRemaining())}
}

func (a *API) openCreditLine(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		ClosingDay int32 `json:"closing_day"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Credit.OpenCreditLine(r.Context(), &creditv1.OpenCreditLineRequest{IdempotencyKey: key, AccountId: acc.GetId(), ClosingDay: req.ClosingDay})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, creditLineJSON(resp.GetLine()))
}

func (a *API) getCreditLine(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Credit.GetCreditLine(r.Context(), &creditv1.GetCreditLineRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, creditLineJSON(resp.GetLine()))
}

func (a *API) listInvoices(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Credit.ListInvoices(r.Context(), &creditv1.ListInvoicesRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, i := range resp.GetInvoices() {
		out = append(out, invoiceJSON(i))
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoices": out})
}

// ownedInvoice loads {invoiceID} and checks it belongs to the {id} account.
func (a *API) ownedInvoice(next func(http.ResponseWriter, *http.Request, *creditv1.GetInvoiceResponse)) func(http.ResponseWriter, *http.Request, *accountv1.Account) {
	return func(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
		id := r.PathValue("invoiceID")
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "invoice not found")
			return
		}
		resp, err := a.c.Credit.GetInvoice(r.Context(), &creditv1.GetInvoiceRequest{Id: id})
		if err != nil || resp.GetAccountId() != acc.GetId() {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "invoice not found")
			return
		}
		next(w, r, resp)
	}
}

func (a *API) getInvoice(w http.ResponseWriter, _ *http.Request, inv *creditv1.GetInvoiceResponse) {
	out := invoiceJSON(inv.GetInvoice())
	items := []map[string]any{}
	for _, it := range inv.GetItems() {
		items = append(items, map[string]any{"kind": it.GetKind(), "amount": moneyJSON(it.GetAmount()),
			"description": it.GetDescription(), "occurred_at": ts(it.GetOccurredAt())})
	}
	out["items"] = items
	writeJSON(w, http.StatusOK, out)
}

func (a *API) payInvoice(w http.ResponseWriter, r *http.Request, inv *creditv1.GetInvoiceResponse) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		Amount string `json:"amount"`
	}
	if !decode(w, r, &req) {
		return
	}
	amount, ok := parseBRL(w, r, req.Amount)
	if !ok {
		return
	}
	resp, err := a.c.Credit.PayInvoice(r.Context(), &creditv1.PayInvoiceRequest{IdempotencyKey: key, InvoiceId: inv.GetInvoice().GetId(), Amount: amount})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	p := resp.GetPayment()
	if p.GetStatus() == "FAILED" {
		writeError(w, r.Context(), http.StatusUnprocessableEntity, p.GetFailureReason(), "the payment was not made")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": p.GetId(), "invoice_id": p.GetInvoiceId(), "status": p.GetStatus(),
		"amount": moneyJSON(p.GetAmount()), "created_at": ts(p.GetCreatedAt())})
}

// advanceCreditCycle (sandbox) closes the open invoice now (CLOSE) or makes
// the closed one overdue (EXPIRE), instead of waiting for the calendar.
func (a *API) advanceCreditCycle(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	var req struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &req) {
		return
	}
	if _, err := a.c.Credit.AdvanceCycle(r.Context(), &creditv1.AdvanceCycleRequest{AccountId: acc.GetId(), Action: req.Action}); err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
