package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	cardv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/card/v1"
)

func cardJSON(c *cardv1.Card) map[string]any {
	return map[string]any{
		"id": c.GetId(), "kind": c.GetKind().String()[len("CARD_KIND_"):], "status": c.GetStatus().String()[len("CARD_STATUS_"):],
		"status_reason": c.GetStatusReason(), "last4": c.GetLast4(), "holder_name": c.GetHolderName(),
		"expiry_month": c.GetExpiryMonth(), "expiry_year": c.GetExpiryYear(),
		"controls": map[string]bool{"online": c.GetControls().GetOnline(), "international": c.GetControls().GetInternational(), "contactless": c.GetControls().GetContactless()},
	}
}

func (a *API) issueCard(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		Kind string `json:"kind"`
	}
	if !decode(w, r, &req) {
		return
	}
	kind := map[string]cardv1.CardKind{"PHYSICAL": cardv1.CardKind_CARD_KIND_PHYSICAL, "VIRTUAL": cardv1.CardKind_CARD_KIND_VIRTUAL}[req.Kind]
	resp, err := a.c.Cards.IssueCard(r.Context(), &cardv1.IssueCardRequest{IdempotencyKey: key, AccountId: acc.GetId(), Kind: kind})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, cardJSON(resp.GetCard()))
}

func (a *API) listCards(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Cards.ListCards(r.Context(), &cardv1.ListCardsRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, c := range resp.GetCards() {
		out = append(out, cardJSON(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"cards": out})
}

// ownedCard checks {cardID} belongs to the {id} account (404 otherwise).
func (a *API) ownedCard(next func(http.ResponseWriter, *http.Request, *cardv1.Card)) func(http.ResponseWriter, *http.Request, *accountv1.Account) {
	return func(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
		id := r.PathValue("cardID")
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "card not found")
			return
		}
		resp, err := a.c.Cards.GetCard(r.Context(), &cardv1.GetCardRequest{Id: id})
		if err != nil || resp.GetCard().GetAccountId() != acc.GetId() {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "card not found")
			return
		}
		next(w, r, resp.GetCard())
	}
}

func (a *API) blockCard(w http.ResponseWriter, r *http.Request, c *cardv1.Card) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Cards.BlockCard(r.Context(), &cardv1.BlockCardRequest{Id: c.GetId(), Reason: req.Reason})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, cardJSON(resp.GetCard()))
}

func (a *API) unblockCard(w http.ResponseWriter, r *http.Request, c *cardv1.Card) {
	resp, err := a.c.Cards.UnblockCard(r.Context(), &cardv1.UnblockCardRequest{Id: c.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, cardJSON(resp.GetCard()))
}

func (a *API) cardControls(w http.ResponseWriter, r *http.Request, c *cardv1.Card) {
	var req struct {
		Online        bool `json:"online"`
		International bool `json:"international"`
		Contactless   bool `json:"contactless"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Cards.UpdateControls(r.Context(), &cardv1.UpdateControlsRequest{Id: c.GetId(),
		Controls: &cardv1.Controls{Online: req.Online, International: req.International, Contactless: req.Contactless}})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, cardJSON(resp.GetCard()))
}

// revealCard returns the full card data to its holder. It sits behind
// step-up authentication and responses are not cacheable.
func (a *API) revealCard(w http.ResponseWriter, r *http.Request, c *cardv1.Card) {
	resp, err := a.c.Cards.RevealCardDetails(r.Context(), &cardv1.RevealCardDetailsRequest{Id: c.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"pan": resp.GetPan(), "expiry_month": resp.GetExpiryMonth(),
		"expiry_year": resp.GetExpiryYear(), "cvv": resp.GetCvv()})
}

// ---- tokens (wallets) ----

func tokenJSON(t *cardv1.CardToken) map[string]any {
	return map[string]any{"id": t.GetId(), "card_id": t.GetCardId(), "wallet": t.GetWallet(),
		"device_name": t.GetDeviceName(), "status": t.GetStatus(), "last4": t.GetLast4(), "created_at": ts(t.GetCreatedAt())}
}

// tokenizeCard provisions a wallet token. The token number goes to the
// wallet once (in the sandbox, to the caller) and is never shown again.
func (a *API) tokenizeCard(w http.ResponseWriter, r *http.Request, c *cardv1.Card) {
	var req struct {
		Wallet     string `json:"wallet"`
		DeviceName string `json:"device_name"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Cards.TokenizeCard(r.Context(), &cardv1.TokenizeCardRequest{CardId: c.GetId(), Wallet: req.Wallet, DeviceName: req.DeviceName})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := tokenJSON(resp.GetToken())
	out["token_pan"] = resp.GetTokenPan()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, out)
}

func (a *API) listTokens(w http.ResponseWriter, r *http.Request, c *cardv1.Card) {
	resp, err := a.c.Cards.ListTokens(r.Context(), &cardv1.ListTokensRequest{CardId: c.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, t := range resp.GetTokens() {
		out = append(out, tokenJSON(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (a *API) updateToken(action string) func(http.ResponseWriter, *http.Request, *cardv1.Card) {
	return func(w http.ResponseWriter, r *http.Request, c *cardv1.Card) {
		id := r.PathValue("tokenID")
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "token not found")
			return
		}
		resp, err := a.c.Cards.UpdateToken(r.Context(), &cardv1.UpdateTokenRequest{CardId: c.GetId(), TokenId: id, Action: action})
		if err != nil {
			writeGRPCError(w, r.Context(), err)
			return
		}
		writeJSON(w, http.StatusOK, tokenJSON(resp.GetToken()))
	}
}

// ---- simulated card network (sandbox) ----

func authorizationJSON(a *cardv1.Authorization) map[string]any {
	return map[string]any{
		"id": a.GetId(), "network_reference": a.GetNetworkReference(),
		"status": a.GetStatus().String()[len("AUTHORIZATION_STATUS_"):], "response_code": a.GetResponseCode(),
		"reason": a.GetReason(), "amount": moneyJSON(a.GetAmount()), "cleared_amount": moneyJSON(a.GetClearedAmount()),
		"merchant_name": a.GetMerchantName(), "card_id": a.GetCardId(), "token_id": a.GetTokenId(),
		"function": strings.TrimPrefix(a.GetFunction().String(), "FUNCTION_"),
	}
}

func (a *API) networkAuthorize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NetworkReference string `json:"network_reference"`
		PAN              string `json:"pan"`
		ExpiryMonth      int32  `json:"expiry_month"`
		ExpiryYear       int32  `json:"expiry_year"`
		CVV              string `json:"cvv"`
		Amount           string `json:"amount"`
		MerchantName     string `json:"merchant_name"`
		MCC              string `json:"mcc"`
		Country          string `json:"country"`
		Channel          string `json:"channel"`
		// DEBIT (default) or CREDIT.
		Function string `json:"function"`
	}
	if !decode(w, r, &req) {
		return
	}
	amount, ok := parseBRL(w, r, req.Amount)
	if !ok {
		return
	}
	if req.NetworkReference == "" {
		req.NetworkReference = uuid.NewString()
	}
	channel := map[string]cardv1.Channel{"ONLINE": cardv1.Channel_CHANNEL_ONLINE, "CHIP": cardv1.Channel_CHANNEL_CHIP,
		"CONTACTLESS": cardv1.Channel_CHANNEL_CONTACTLESS}[req.Channel]
	resp, err := a.c.Cards.Authorize(r.Context(), &cardv1.AuthorizeRequest{
		NetworkReference: req.NetworkReference, Pan: req.PAN, ExpiryMonth: req.ExpiryMonth, ExpiryYear: req.ExpiryYear,
		Cvv: req.CVV, Amount: amount, MerchantName: req.MerchantName, Mcc: req.MCC, Country: req.Country, Channel: channel,
		Function: map[string]cardv1.Function{"CREDIT": cardv1.Function_FUNCTION_CREDIT}[req.Function],
	})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	// Declines are normal answers for a network, not HTTP errors.
	writeJSON(w, http.StatusOK, authorizationJSON(resp.GetAuthorization()))
}

func (a *API) networkClear(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Amount string `json:"amount"`
	}
	if !decode(w, r, &req) {
		return
	}
	amount, ok := optionalBRL(w, r, req.Amount)
	if !ok {
		return
	}
	resp, err := a.c.Cards.Clear(r.Context(), &cardv1.ClearRequest{AuthorizationId: r.PathValue("authID"), Amount: amount})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, authorizationJSON(resp.GetAuthorization()))
}

func (a *API) networkReverse(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Cards.Reverse(r.Context(), &cardv1.ReverseRequest{AuthorizationId: r.PathValue("authID")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, authorizationJSON(resp.GetAuthorization()))
}
