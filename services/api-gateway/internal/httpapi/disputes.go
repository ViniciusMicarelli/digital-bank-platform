package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	cardv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/card/v1"
)

func disputeJSON(d *cardv1.Dispute) map[string]any {
	out := map[string]any{
		"id": d.GetId(), "authorization_id": d.GetAuthorizationId(), "card_id": d.GetCardId(), "account_id": d.GetAccountId(),
		"reason": strings.TrimPrefix(d.GetReason().String(), "DISPUTE_REASON_"), "amount": moneyJSON(d.GetAmount()),
		"description": d.GetDescription(), "status": strings.TrimPrefix(d.GetStatus().String(), "DISPUTE_STATUS_"),
		"decided_by": d.GetDecidedBy(), "decision_reason": d.GetDecisionReason(), "created_at": ts(d.GetCreatedAt()),
		"merchant_name": d.GetMerchantName(), "purchased_at": ts(d.GetPurchasedAt()),
	}
	if d.GetDecidedAt() != nil {
		out["decided_at"] = ts(d.GetDecidedAt())
	}
	return out
}

// purchaseJSON is a card purchase as the customer sees it, with what was
// refunded and the latest dispute.
func purchaseJSON(a *cardv1.Authorization) map[string]any {
	out := authorizationJSON(a)
	out["refunded_amount"] = moneyJSON(a.GetRefundedAmount())
	out["created_at"] = ts(a.GetCreatedAt())
	if a.GetDispute() != nil {
		out["dispute"] = disputeJSON(a.GetDispute())
	}
	return out
}

// cardPurchases lists a card's purchases, newest first.
func (a *API) cardPurchases(w http.ResponseWriter, r *http.Request, c *cardv1.Card) {
	resp, err := a.c.Cards.ListAuthorizations(r.Context(), &cardv1.ListAuthorizationsRequest{CardId: c.GetId(), Limit: 50})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, p := range resp.GetAuthorizations() {
		out = append(out, purchaseJSON(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"purchases": out})
}

// openDispute disputes a purchase of the card: {reason, amount?, description?}.
func (a *API) openDispute(w http.ResponseWriter, r *http.Request, c *cardv1.Card) {
	var req struct {
		Reason      string `json:"reason"`
		Amount      string `json:"amount"`
		Description string `json:"description"`
	}
	if !decode(w, r, &req) {
		return
	}
	reason, ok := cardv1.DisputeReason_value["DISPUTE_REASON_"+strings.ToUpper(req.Reason)]
	if !ok || reason == 0 {
		writeError(w, r.Context(), http.StatusBadRequest, "INVALID_REASON",
			"reason is NOT_RECOGNIZED, NOT_RECEIVED, DUPLICATE, WRONG_AMOUNT or CANCELLED")
		return
	}
	amount, ok := optionalBRL(w, r, req.Amount)
	if !ok {
		return
	}
	if _, err := uuid.Parse(r.PathValue("authID")); err != nil {
		writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "purchase not found")
		return
	}
	resp, err := a.c.Cards.OpenDispute(r.Context(), &cardv1.OpenDisputeRequest{AuthorizationId: r.PathValue("authID"), CardId: c.GetId(),
		Reason: cardv1.DisputeReason(reason), Amount: amount, Description: req.Description})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, disputeJSON(resp.GetDispute()))
}

// accountDisputes lists the account's disputes, newest first.
func (a *API) accountDisputes(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	a.writeDisputes(w, r, &cardv1.ListDisputesRequest{AccountId: acc.GetId(), Limit: 100}, false)
}

// writeDisputes answers a list of disputes; for analysts (staff) each one
// carries the account holder and customer, to open the customer's file.
func (a *API) writeDisputes(w http.ResponseWriter, r *http.Request, req *cardv1.ListDisputesRequest, staff bool) {
	resp, err := a.c.Cards.ListDisputes(r.Context(), req)
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, d := range resp.GetDisputes() {
		item := disputeJSON(d)
		if staff {
			if acc, err := a.c.Accounts.GetAccount(r.Context(), &accountv1.GetAccountRequest{Id: d.GetAccountId()}); err == nil {
				item["customer_id"], item["holder_name"] = acc.GetAccount().GetCustomerId(), acc.GetAccount().GetHolderName()
			}
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"disputes": out})
}

// networkRefund (sandbox) is a merchant refund sent by the card network.
func (a *API) networkRefund(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Amount          string `json:"amount"`
		RefundReference string `json:"refund_reference"`
	}
	if !decode(w, r, &req) {
		return
	}
	amount, ok := parseBRL(w, r, req.Amount)
	if !ok {
		return
	}
	if req.RefundReference == "" {
		req.RefundReference = uuid.NewString()
	}
	resp, err := a.c.Cards.Refund(r.Context(), &cardv1.RefundRequest{AuthorizationId: r.PathValue("authID"),
		RefundReference: req.RefundReference, Amount: amount})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	f := resp.GetRefund()
	writeJSON(w, http.StatusOK, map[string]any{"id": f.GetId(), "authorization_id": f.GetAuthorizationId(), "amount": moneyJSON(f.GetAmount()),
		"source": f.GetSource(), "status": f.GetStatus(), "failure_reason": f.GetFailureReason()})
}

// backofficeDisputes lists disputes for analysts (?status=OPEN by default, or WON, LOST, ALL).
func (a *API) backofficeDisputes(w http.ResponseWriter, r *http.Request) {
	want := strings.ToUpper(r.URL.Query().Get("status"))
	if want == "" {
		want = "OPEN"
	}
	req := &cardv1.ListDisputesRequest{Limit: 100}
	if want != "ALL" {
		v, ok := cardv1.DisputeStatus_value["DISPUTE_STATUS_"+want]
		if !ok {
			writeError(w, r.Context(), http.StatusBadRequest, "INVALID_STATUS", "unknown dispute status")
			return
		}
		req.Status = cardv1.DisputeStatus(v)
	}
	a.writeDisputes(w, r, req, true)
}

// decideDispute is the analyst's decision {won, reason}; audited on the account.
func (a *API) decideDispute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Won    bool   `json:"won"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !req.Won && strings.TrimSpace(req.Reason) == "" {
		writeError(w, r.Context(), http.StatusBadRequest, "REASON_REQUIRED", "a lost dispute needs a reason")
		return
	}
	id := r.PathValue("id")
	d, err := a.c.Cards.GetDispute(r.Context(), &cardv1.GetDisputeRequest{Id: id})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	action := "DISPUTE_ACCEPT"
	if !req.Won {
		action = "DISPUTE_REJECT"
	}
	account := d.GetDispute().GetAccountId()
	if !a.audit(w, r, action, "account", account, map[string]string{"dispute_id": id, "reason": req.Reason,
		"amount": moneyJSON(d.GetDispute().GetAmount()).Amount, "merchant": d.GetDispute().GetMerchantName()}) {
		return
	}
	resp, err := a.c.Cards.DecideDispute(r.Context(), &cardv1.DecideDisputeRequest{Id: id, AnalystId: customerID(r.Context()),
		Won: req.Won, Reason: req.Reason})
	if err != nil {
		a.auditFailure(r.Context(), action, "account", account, err)
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, disputeJSON(resp.GetDispute()))
}
