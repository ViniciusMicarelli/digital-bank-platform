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
		"closing_day": l.GetClosingDay(), "monthly_interest_rate": float64(l.GetInterestRateBp()) / 100,
		"upcoming_installments": moneyJSON(l.GetUpcomingInstallments())}
}

// installmentPlans lists purchases with installments still to come.
func (a *API) installmentPlans(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Credit.ListInstallmentPlans(r.Context(), &creditv1.ListInstallmentPlansRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, p := range resp.GetPlans() {
		out = append(out, map[string]any{"description": p.GetDescription(), "total": moneyJSON(p.GetTotal()),
			"installments": p.GetInstallments(), "billed": p.GetBilled(), "installment_amount": moneyJSON(p.GetInstallmentAmount()),
			"remaining": moneyJSON(p.GetRemaining()), "purchased_at": ts(p.GetPurchasedAt())})
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": out})
}

func invoiceJSON(i *creditv1.Invoice) map[string]any {
	return map[string]any{"id": i.GetId(), "status": i.GetStatus(), "closing_date": i.GetClosingDate(),
		"due_date": i.GetDueDate(), "total": moneyJSON(i.GetTotal()), "paid": moneyJSON(i.GetPaid()),
		"remaining": moneyJSON(i.GetRemaining()), "minimum_payment": moneyJSON(i.GetMinimumPayment())}
}

// planQuotes lists the ways a closed invoice can be split.
func (a *API) planQuotes(w http.ResponseWriter, r *http.Request, inv *creditv1.GetInvoiceResponse) {
	resp, err := a.c.Credit.QuoteInvoicePlan(r.Context(), &creditv1.QuoteInvoicePlanRequest{InvoiceId: inv.GetInvoice().GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, q := range resp.GetQuotes() {
		out = append(out, map[string]any{"installments": q.GetInstallments(), "installment": moneyJSON(q.GetInstallment()),
			"total": moneyJSON(q.GetTotal()), "interest": moneyJSON(q.GetInterest()),
			"monthly_rate": float64(q.GetMonthlyRateBp()) / 100, "annual_cost": float64(q.GetAnnualCostBp()) / 100})
	}
	writeJSON(w, http.StatusOK, map[string]any{"quotes": out})
}

// planInvoice splits a closed invoice {installments} (Idempotency-Key).
func (a *API) planInvoice(w http.ResponseWriter, r *http.Request, inv *creditv1.GetInvoiceResponse) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		Installments int32 `json:"installments"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Credit.PlanInvoice(r.Context(), &creditv1.PlanInvoiceRequest{IdempotencyKey: key, InvoiceId: inv.GetInvoice().GetId(), Installments: req.Installments})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	p := resp.GetPlan()
	writeJSON(w, http.StatusCreated, map[string]any{"id": p.GetId(), "invoice_id": p.GetInvoiceId(), "principal": moneyJSON(p.GetPrincipal()),
		"installments": p.GetInstallments(), "installment": moneyJSON(p.GetInstallment()), "interest": moneyJSON(p.GetInterest()),
		"monthly_rate": float64(p.GetMonthlyRateBp()) / 100})
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
