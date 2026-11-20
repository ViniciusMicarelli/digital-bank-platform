package httpapi

import (
	"net/http"

	identityv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/identity/v1"
)

// sessions lists where the customer is signed in; "current" marks the one
// making the request.
func (a *API) sessions(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Identity.ListSessions(r.Context(), &identityv1.ListSessionsRequest{CustomerId: customerID(r.Context())})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	current := principal(r.Context()).Session
	list := []map[string]any{}
	for _, s := range resp.GetSessions() {
		list = append(list, map[string]any{"id": s.GetId(), "device": s.GetDevice(), "ip": s.GetIp(),
			"created_at": ts(s.GetCreatedAt()), "last_used_at": ts(s.GetLastUsedAt()), "current": s.GetId() == current})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
}

// endSession signs one of the customer's sessions out (204).
func (a *API) endSession(w http.ResponseWriter, r *http.Request) {
	_, err := a.c.Identity.RevokeSession(r.Context(), &identityv1.RevokeSessionRequest{CustomerId: customerID(r.Context()),
		SessionId: r.PathValue("sessionID")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// endOtherSessions signs out everywhere but here.
func (a *API) endOtherSessions(w http.ResponseWriter, r *http.Request) {
	current := principal(r.Context()).Session
	if current == "" {
		writeError(w, r.Context(), http.StatusConflict, "SESSION_UNKNOWN", "sign in again to manage your sessions")
		return
	}
	resp, err := a.c.Identity.RevokeOtherSessions(r.Context(), &identityv1.RevokeOtherSessionsRequest{CustomerId: customerID(r.Context()),
		KeepSessionId: current})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ended": resp.GetRevoked()})
}
