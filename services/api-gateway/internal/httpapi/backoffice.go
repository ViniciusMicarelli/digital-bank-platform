package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	backofficev1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/backoffice/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
)

// me tells the apps who is signed in, with which role, and whether the
// session has a second factor (the back office requires one).
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	p := principal(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"customer_id": p.Subject, "role": p.Role, "mfa": p.MFA})
}

// audit writes a staff action to the audit trail before it runs. It fails
// closed: when the trail cannot be written the action is refused (503).
func (a *API) audit(w http.ResponseWriter, r *http.Request, action, targetType, targetID string, details map[string]string) bool {
	_, err := a.c.BackOffice.RecordAudit(r.Context(), &backofficev1.RecordAuditRequest{
		ActorId: customerID(r.Context()), Action: action, TargetType: targetType, TargetId: targetID, Details: details})
	if err != nil {
		a.log.ErrorContext(r.Context(), "audit trail unavailable, action refused", "action", action, "error", err)
		writeError(w, r.Context(), http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "the audit trail is unavailable; try again")
		return false
	}
	return true
}

// auditFailure records that an audited action did not go through
// (best effort: the action already failed and the caller gets its error).
func (a *API) auditFailure(ctx context.Context, action, targetType, targetID string, err error) {
	reason := grpcserver.Reason(err)
	if reason == "" {
		reason = status.Code(err).String()
	}
	if _, aerr := a.c.BackOffice.RecordAudit(context.WithoutCancel(ctx), &backofficev1.RecordAuditRequest{
		ActorId: customerID(ctx), Action: action + "_FAILED", TargetType: targetType, TargetId: targetID,
		Details: map[string]string{"error": reason}}); aerr != nil {
		a.log.ErrorContext(ctx, "audit failure event", "action", action, "error", aerr)
	}
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
	customer, action := r.PathValue("customerID"), "KYC_APPROVE"
	if !req.Approve {
		action = "KYC_REJECT"
	}
	if !a.audit(w, r, action, "customer", customer, map[string]string{"reason": req.Reason}) {
		return
	}
	resp, err := a.c.Users.ReviewKyc(r.Context(), &userv1.ReviewKycRequest{
		CustomerId: customer, Approve: req.Approve, Reason: req.Reason, ReviewerId: customerID(r.Context())})
	if err != nil {
		a.auditFailure(r.Context(), action, "customer", customer, err)
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, reviewJSON(resp.GetProfile()))
}

// customerFile shows an analyst a customer's profile and accounts.
func (a *API) customerFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("customerID")
	// Personal data: every look at a customer's file is in the trail (LGPD).
	if !a.audit(w, r, "CUSTOMER_FILE_VIEW", "customer", id, nil) {
		return
	}
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
	id := r.PathValue("id")
	if !a.audit(w, r, "ACCOUNT_BLOCK", "account", id, map[string]string{"reason": req.Reason}) {
		return
	}
	resp, err := a.c.Accounts.BlockAccount(r.Context(), &accountv1.BlockAccountRequest{
		Id: id, Reason: req.Reason + " (analista " + customerID(r.Context()) + ")"})
	if err != nil {
		a.auditFailure(r.Context(), "ACCOUNT_BLOCK", "account", id, err)
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, accountJSON(resp.GetAccount()))
}

// unblockAccount opens a four-eyes request: the account is only unblocked
// when a different analyst approves it (202 with the approval).
func (a *API) unblockAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		writeError(w, r.Context(), http.StatusBadRequest, "REASON_REQUIRED", "unblocking an account needs a reason")
		return
	}
	id := r.PathValue("id")
	acc, err := a.c.Accounts.GetAccount(r.Context(), &accountv1.GetAccountRequest{Id: id})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	if acc.GetAccount().GetStatus() != accountv1.AccountStatus_ACCOUNT_STATUS_BLOCKED {
		writeError(w, r.Context(), http.StatusUnprocessableEntity, "ACCOUNT_NOT_BLOCKED", "only a blocked account can be unblocked")
		return
	}
	resp, err := a.c.BackOffice.RequestApproval(r.Context(), &backofficev1.RequestApprovalRequest{
		Kind: backofficev1.ApprovalKind_APPROVAL_KIND_UNBLOCK_ACCOUNT, TargetId: id, Reason: req.Reason, RequestedBy: customerID(r.Context())})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusAccepted, approvalJSON(resp.GetApproval()))
}

func enumName(s, prefix string) string { return strings.TrimPrefix(s, prefix) }

func paramsJSON(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func approvalJSON(p *backofficev1.Approval) map[string]any {
	out := map[string]any{
		"id": p.GetId(), "kind": enumName(p.GetKind().String(), "APPROVAL_KIND_"), "target_id": p.GetTargetId(),
		"reason": p.GetReason(), "requested_by": p.GetRequestedBy(), "status": enumName(p.GetStatus().String(), "APPROVAL_STATUS_"),
		"decided_by": p.GetDecidedBy(), "decision_reason": p.GetDecisionReason(), "failure_reason": p.GetFailureReason(),
		"created_at": ts(p.GetCreatedAt()), "expires_at": ts(p.GetExpiresAt()), "params": paramsJSON(p.GetParams()),
	}
	if p.GetDecidedAt() != nil {
		out["decided_at"] = ts(p.GetDecidedAt())
	}
	return out
}

// approvals lists four-eyes requests (?status=PENDING by default; "all" for every status).
func (a *API) approvals(w http.ResponseWriter, r *http.Request) {
	want := strings.ToUpper(r.URL.Query().Get("status"))
	if want == "" {
		want = "PENDING"
	}
	st := backofficev1.ApprovalStatus_APPROVAL_STATUS_UNSPECIFIED
	if want != "ALL" {
		v, ok := backofficev1.ApprovalStatus_value["APPROVAL_STATUS_"+want]
		if !ok {
			writeError(w, r.Context(), http.StatusBadRequest, "INVALID_STATUS", "unknown approval status")
			return
		}
		st = backofficev1.ApprovalStatus(v)
	}
	resp, err := a.c.BackOffice.ListApprovals(r.Context(), &backofficev1.ListApprovalsRequest{Status: st, Limit: 100})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, p := range resp.GetApprovals() {
		item := approvalJSON(p)
		// What the second analyst needs to judge the request: whose account it is.
		accountID := p.GetTargetId()
		if p.GetKind() == backofficev1.ApprovalKind_APPROVAL_KIND_LIMIT_INCREASE {
			accountID = p.GetParams()["account_id"]
			if line, err := a.c.Credit.GetCreditLine(r.Context(), &creditv1.GetCreditLineRequest{AccountId: accountID}); err == nil {
				item["current_limit"], item["used"] = moneyJSON(line.GetLine().GetLimit()), moneyJSON(line.GetLine().GetUsed())
			}
			if sc, err := a.c.Credit.GetCreditScore(r.Context(), &creditv1.GetCreditScoreRequest{AccountId: accountID}); err == nil {
				item["score"] = sc.GetScore()
			}
		}
		if acc, err := a.c.Accounts.GetAccount(r.Context(), &accountv1.GetAccountRequest{Id: accountID}); err == nil {
			item["customer_id"] = acc.GetAccount().GetCustomerId()
			item["account"] = accountJSON(acc.GetAccount())
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": out})
}

// decideApproval is the second analyst's decision; approving runs the action.
func (a *API) decideApproval(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &req) {
			return
		}
		if !approve && strings.TrimSpace(req.Reason) == "" {
			writeError(w, r.Context(), http.StatusBadRequest, "REASON_REQUIRED", "a rejection needs a reason")
			return
		}
		resp, err := a.c.BackOffice.DecideApproval(r.Context(), &backofficev1.DecideApprovalRequest{
			Id: r.PathValue("id"), ApproverId: customerID(r.Context()), Approve: approve, Reason: req.Reason})
		if err != nil {
			writeGRPCError(w, r.Context(), err)
			return
		}
		writeJSON(w, http.StatusOK, approvalJSON(resp.GetApproval()))
	}
}

// auditTrail lists audit events, newest first (?target_id=, ?actor_id=, ?limit=).
func (a *API) auditTrail(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	resp, err := a.c.BackOffice.ListAudit(r.Context(), &backofficev1.ListAuditRequest{
		TargetId: q.Get("target_id"), ActorId: q.Get("actor_id"), Limit: int32(min(max(limit, 0), 500))}) //nolint:gosec // bounded
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, e := range resp.GetEvents() {
		details := e.GetDetails()
		if details == nil {
			details = map[string]string{}
		}
		out = append(out, map[string]any{"seq": e.GetSeq(), "actor_id": e.GetActorId(), "action": e.GetAction(),
			"target_type": e.GetTargetType(), "target_id": e.GetTargetId(), "details": details, "at": ts(e.GetAt()), "hash": e.GetHash()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}

// verifyAudit recomputes the audit chain.
func (a *API) verifyAudit(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.BackOffice.VerifyAudit(r.Context(), &backofficev1.VerifyAuditRequest{})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"intact": resp.GetIntact(), "events": resp.GetEvents(), "broken_at": resp.GetBrokenAt()})
}
