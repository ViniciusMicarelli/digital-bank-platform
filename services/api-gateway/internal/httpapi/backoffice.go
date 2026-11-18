package httpapi

import (
	"net/http"
	"strings"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
)

// me tells the apps who is signed in and with which role.
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"customer_id": customerID(r.Context()), "role": role(r.Context())})
}

func reviewJSON(p *userv1.Profile) map[string]any {
	out := profileJSON(p)
	out["customer_id"] = p.GetCustomerId()
	out["reviewed_by"] = p.GetReviewedBy()
	if p.GetUpdatedAt() != nil {
		out["updated_at"] = ts(p.GetUpdatedAt())
	}
	return out
}

// kycQueue lists the verifications waiting for an analyst, oldest first.
func (a *API) kycQueue(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Users.ListKycReviews(r.Context(), &userv1.ListKycReviewsRequest{Limit: 100})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, p := range resp.GetProfiles() {
		out = append(out, reviewJSON(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": out})
}

// decideKYC records the analyst's decision; the analyst id goes with it.
func (a *API) decideKYC(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !req.Approve && strings.TrimSpace(req.Reason) == "" {
		writeError(w, r.Context(), http.StatusBadRequest, "REASON_REQUIRED", "a rejection needs a reason")
		return
	}
	resp, err := a.c.Users.ReviewKyc(r.Context(), &userv1.ReviewKycRequest{
		CustomerId: r.PathValue("customerID"), Approve: req.Approve, Reason: req.Reason, ReviewerId: customerID(r.Context())})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	a.log.InfoContext(r.Context(), "kyc decided", "analyst_id", customerID(r.Context()), "customer_id", r.PathValue("customerID"), "approve", req.Approve)
	writeJSON(w, http.StatusOK, reviewJSON(resp.GetProfile()))
}

// customerFile shows an analyst a customer's profile and accounts.
func (a *API) customerFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("customerID")
	out := map[string]any{"customer_id": id}
	if p, err := a.c.Users.GetProfile(r.Context(), &userv1.GetProfileRequest{CustomerId: id}); err == nil {
		out["profile"] = reviewJSON(p.GetProfile())
	}
	accs, err := a.c.Accounts.ListAccounts(r.Context(), &accountv1.ListAccountsRequest{CustomerId: id})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	list := []map[string]any{}
	for _, acc := range accs.GetAccounts() {
		item := map[string]any{"account": accountJSON(acc)}
		if b, err := a.c.Accounts.GetBalance(r.Context(), &accountv1.GetBalanceRequest{AccountId: acc.GetId()}); err == nil {
			item["available"] = moneyJSON(b.GetAvailable())
		}
		list = append(list, item)
	}
	out["accounts"] = list
	writeJSON(w, http.StatusOK, out)
}

func (a *API) blockAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		writeError(w, r.Context(), http.StatusBadRequest, "REASON_REQUIRED", "blocking an account needs a reason")
		return
	}
	resp, err := a.c.Accounts.BlockAccount(r.Context(), &accountv1.BlockAccountRequest{
		Id: r.PathValue("id"), Reason: req.Reason + " (analista " + customerID(r.Context()) + ")"})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	a.log.InfoContext(r.Context(), "account blocked by analyst", "analyst_id", customerID(r.Context()), "account_id", r.PathValue("id"))
	writeJSON(w, http.StatusOK, accountJSON(resp.GetAccount()))
}

func (a *API) unblockAccount(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Accounts.UnblockAccount(r.Context(), &accountv1.UnblockAccountRequest{Id: r.PathValue("id")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	a.log.InfoContext(r.Context(), "account unblocked by analyst", "analyst_id", customerID(r.Context()), "account_id", r.PathValue("id"))
	writeJSON(w, http.StatusOK, accountJSON(resp.GetAccount()))
}
