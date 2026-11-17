package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
)

// ---- auth ----

func (a *API) devToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomerID string `json:"customer_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.CustomerID == "" || len(req.CustomerID) > 64 {
		writeError(w, r.Context(), http.StatusBadRequest, "INVALID_ARGUMENT", "customer_id is required (max 64 characters)")
		return
	}
	token, exp, err := a.auth.Issue(req.CustomerID)
	if err != nil {
		writeError(w, r.Context(), http.StatusInternalServerError, "INTERNAL", "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token, "token_type": "Bearer", "expires_at": exp.UTC().Format(time.RFC3339),
	})
}

// ---- accounts ----

type accountResp struct {
	ID         string `json:"id"`
	HolderName string `json:"holder_name"`
	Branch     string `json:"branch"`
	Number     string `json:"number"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
	OpenedAt   string `json:"opened_at"`
}

func accountJSON(acc *accountv1.Account) accountResp {
	return accountResp{
		ID: acc.GetId(), HolderName: acc.GetHolderName(), Branch: acc.GetBranch(), Number: acc.GetNumber(),
		Currency: acc.GetCurrency(), Status: map[accountv1.AccountStatus]string{
			accountv1.AccountStatus_ACCOUNT_STATUS_ACTIVE:  "ACTIVE",
			accountv1.AccountStatus_ACCOUNT_STATUS_BLOCKED: "BLOCKED",
			accountv1.AccountStatus_ACCOUNT_STATUS_CLOSED:  "CLOSED",
		}[acc.GetStatus()],
		OpenedAt: ts(acc.GetOpenedAt()),
	}
}

func (a *API) openAccount(w http.ResponseWriter, r *http.Request) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		HolderName string `json:"holder_name"`
		Document   string `json:"document"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Accounts.OpenAccount(r.Context(), &accountv1.OpenAccountRequest{
		IdempotencyKey: key, CustomerId: customerID(r.Context()), HolderName: req.HolderName, Document: req.Document,
	})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, accountJSON(resp.GetAccount()))
}

func (a *API) listAccounts(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Accounts.ListAccounts(r.Context(), &accountv1.ListAccountsRequest{CustomerId: customerID(r.Context())})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []accountResp{}
	for _, acc := range resp.GetAccounts() {
		out = append(out, accountJSON(acc))
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out})
}

func (a *API) getAccount(w http.ResponseWriter, _ *http.Request, acc *accountv1.Account) {
	writeJSON(w, http.StatusOK, accountJSON(acc))
}

func (a *API) getBalance(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	b, err := a.c.Accounts.GetBalance(r.Context(), &accountv1.GetBalanceRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]Money{
		"available": moneyJSON(b.GetAvailable()), "posted": moneyJSON(b.GetPosted()), "pending": moneyJSON(b.GetPending()),
	})
}

type limitsJSON struct {
	PerTransaction string `json:"per_transaction"`
	Daily          string `json:"daily"`
	Nightly        string `json:"nightly"`
}

func (a *API) getLimits(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Accounts.GetLimits(r.Context(), &accountv1.GetLimitsRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	l, u := resp.GetLimits(), resp.GetUsage()
	brl := func(minor int64) string { m, _ := money.New(minor, money.BRL); return m.Decimal() }
	writeJSON(w, http.StatusOK, map[string]any{
		"limits": limitsJSON{brl(l.GetPerTransactionMinorUnits()), brl(l.GetDailyMinorUnits()), brl(l.GetNightlyMinorUnits())},
		"used":   map[string]string{"daily": brl(u.GetDailyUsedMinorUnits()), "nightly": brl(u.GetNightlyUsedMinorUnits())},
	})
}

func (a *API) updateLimits(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	var req limitsJSON
	if !decode(w, r, &req) {
		return
	}
	per, ok1 := parseBRL(w, r, req.PerTransaction)
	if !ok1 {
		return
	}
	daily, ok2 := parseBRL(w, r, req.Daily)
	if !ok2 {
		return
	}
	nightly, ok3 := parseBRL(w, r, req.Nightly)
	if !ok3 {
		return
	}
	_, err := a.c.Accounts.UpdateLimits(r.Context(), &accountv1.UpdateLimitsRequest{AccountId: acc.GetId(), Limits: &accountv1.Limits{
		PerTransactionMinorUnits: per.GetMinorUnits(), DailyMinorUnits: daily.GetMinorUnits(), NightlyMinorUnits: nightly.GetMinorUnits(),
	}})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	a.getLimits(w, r, acc)
}

// ---- statement ----

func (a *API) statement(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	resp, err := a.c.Statement.ListFeed(r.Context(), &statementv1.ListFeedRequest{
		AccountId: acc.GetId(), PageSize: int32(min(max(limit, 0), 100)), PageToken: r.URL.Query().Get("cursor"),
	})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	items := []map[string]any{}
	for _, it := range resp.GetItems() {
		dir := "DEBIT"
		if it.GetDirection() == statementv1.Direction_DIRECTION_CREDIT {
			dir = "CREDIT"
		}
		items = append(items, map[string]any{
			"id": it.GetId(), "transaction_id": it.GetTransactionId(), "type": it.GetType(), "direction": dir,
			"amount": moneyJSON(it.GetAmount()), "counterparty_name": it.GetCounterpartyName(),
			"description": it.GetDescription(), "end_to_end_id": it.GetEndToEndId(), "occurred_at": ts(it.GetOccurredAt()),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": resp.GetNextPageToken()})
}

// ---- transfers ----

func (a *API) transfer(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		ToAccountID string `json:"to_account_id"`
		Amount      string `json:"amount"`
		Description string `json:"description"`
	}
	if !decode(w, r, &req) {
		return
	}
	amount, ok := parseBRL(w, r, req.Amount)
	if !ok {
		return
	}
	if _, err := uuid.Parse(req.ToAccountID); err != nil {
		writeError(w, r.Context(), http.StatusBadRequest, "INVALID_ARGUMENT", "to_account_id must be a UUID")
		return
	}
	resp, err := a.c.Txs.CreateTransfer(r.Context(), &txv1.CreateTransferRequest{
		IdempotencyKey: key, FromAccountId: acc.GetId(), ToAccountId: req.ToAccountID,
		Amount: amount, Description: req.Description,
	})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	t := resp.GetTransaction()
	body := map[string]any{
		"id": t.GetId(), "status": t.GetStatus().String()[len("TRANSACTION_STATUS_"):],
		"from_account_id": t.GetFromAccountId(), "to_account_id": t.GetToAccountId(),
		"amount": moneyJSON(t.GetAmount()), "description": t.GetDescription(), "created_at": ts(t.GetCreatedAt()),
	}
	if t.GetStatus() == txv1.TransactionStatus_TRANSACTION_STATUS_FAILED {
		writeError(w, r.Context(), http.StatusUnprocessableEntity, t.GetFailureReason(), "the transfer was not made")
		return
	}
	writeJSON(w, http.StatusCreated, body)
}

// ---- pix ----

var keyTypes = map[string]pixv1.KeyType{
	"CPF": pixv1.KeyType_KEY_TYPE_CPF, "EMAIL": pixv1.KeyType_KEY_TYPE_EMAIL,
	"PHONE": pixv1.KeyType_KEY_TYPE_PHONE, "EVP": pixv1.KeyType_KEY_TYPE_EVP,
}

func keyJSON(k *pixv1.PixKey) map[string]string {
	return map[string]string{"id": k.GetId(), "type": k.GetType().String()[len("KEY_TYPE_"):], "value": k.GetValue(), "created_at": ts(k.GetCreatedAt())}
}

func (a *API) listKeys(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Pix.ListKeys(r.Context(), &pixv1.ListKeysRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]string{}
	for _, k := range resp.GetKeys() {
		out = append(out, keyJSON(k))
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

func (a *API) registerKey(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	var req struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if !decode(w, r, &req) {
		return
	}
	t, ok := keyTypes[req.Type]
	if !ok {
		writeError(w, r.Context(), http.StatusBadRequest, "INVALID_ARGUMENT", "type must be CPF, EMAIL, PHONE or EVP")
		return
	}
	resp, err := a.c.Pix.RegisterKey(r.Context(), &pixv1.RegisterKeyRequest{AccountId: acc.GetId(), Type: t, Value: req.Value})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, keyJSON(resp.GetKey()))
}

func (a *API) deleteKey(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	_, err := a.c.Pix.DeleteKey(r.Context(), &pixv1.DeleteKeyRequest{AccountId: acc.GetId(), KeyId: r.PathValue("keyID")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func partyJSON(p *pixv1.Party) map[string]string {
	return map[string]string{"name": p.GetName(), "document": p.GetDocumentMasked(), "bank": p.GetBankName(), "ispb": p.GetIspb()}
}

func (a *API) lookupKey(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Pix.LookupKey(r.Context(), &pixv1.LookupKeyRequest{Key: r.PathValue("key")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"type": resp.GetType().String()[len("KEY_TYPE_"):], "owner": partyJSON(resp.GetOwner())})
}

func pixTransferJSON(t *pixv1.Transfer) map[string]any {
	return map[string]any{
		"id": t.GetId(), "end_to_end_id": t.GetEndToEndId(), "status": t.GetStatus().String()[len("TRANSFER_STATUS_"):],
		"key": t.GetKey(), "counterparty": partyJSON(t.GetCounterparty()), "amount": moneyJSON(t.GetAmount()),
		"description": t.GetDescription(), "created_at": ts(t.GetCreatedAt()),
	}
}

func (a *API) sendPix(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		Key         string `json:"key"`
		Amount      string `json:"amount"`
		Description string `json:"description"`
	}
	if !decode(w, r, &req) {
		return
	}
	amount, ok := parseBRL(w, r, req.Amount)
	if !ok {
		return
	}
	resp, err := a.c.Pix.CreateTransfer(r.Context(), &pixv1.CreateTransferRequest{
		IdempotencyKey: key, FromAccountId: acc.GetId(), Key: req.Key, Amount: amount, Description: req.Description,
	})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	t := resp.GetTransfer()
	switch t.GetStatus() {
	case pixv1.TransferStatus_TRANSFER_STATUS_FAILED, pixv1.TransferStatus_TRANSFER_STATUS_REJECTED:
		writeError(w, r.Context(), http.StatusUnprocessableEntity, t.GetFailureReason(), "the Pix was not sent")
	case pixv1.TransferStatus_TRANSFER_STATUS_COMPLETED:
		writeJSON(w, http.StatusCreated, pixTransferJSON(t))
	default: // still being processed (e.g. after an outage); the client can poll
		writeJSON(w, http.StatusAccepted, pixTransferJSON(t))
	}
}

// sandboxPixIn simulates a Pix arriving from another bank, to fund demo accounts.
func (a *API) sandboxPixIn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key       string `json:"key"`
		Amount    string `json:"amount"`
		PayerName string `json:"payer_name"`
	}
	if !decode(w, r, &req) {
		return
	}
	amount, ok := parseBRL(w, r, req.Amount)
	if !ok {
		return
	}
	if req.PayerName == "" {
		req.PayerName = "Sandbox"
	}
	e2e := "E12345678" + time.Now().UTC().Format("200601021504") + uuid.NewString()[:8] + "sbx"
	resp, err := a.c.Pix.SimulateIncoming(r.Context(), &pixv1.SimulateIncomingRequest{
		EndToEndId: e2e, Key: req.Key, Amount: amount, Description: "Sandbox deposit",
		Payer: &pixv1.Party{Name: req.PayerName, Ispb: "12345678", BankName: "Banco Exemplo"},
	})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, pixTransferJSON(resp.GetTransfer()))
}
