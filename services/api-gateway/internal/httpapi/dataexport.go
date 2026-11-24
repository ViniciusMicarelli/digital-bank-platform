package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	cardv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/card/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	identityv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/identity/v1"
	notificationv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/notification/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	savingsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/savings/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
)

// maxExportItems bounds the statement in an export (the CSV export covers
// any period in full).
const maxExportItems = 2000

// dataExport answers everything the bank keeps about the caller, as a JSON
// file (LGPD, art. 18: access and portability). It holds the full CPF, so
// it needs a step-up. Card numbers never leave the vault: only the last 4.
func (a *API) dataExport(w http.ResponseWriter, r *http.Request) {
	ctx, customer := r.Context(), customerID(r.Context())
	out := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"customer_id":  customer,
		"about": "Seus dados no Digital Bank (LGPD, art. 18). Registros de conta e de transações são guardados pelo prazo legal " +
			"(prevenção à lavagem de dinheiro) mesmo depois do encerramento da conta; número completo e CVV de cartões nunca são exportados.",
	}
	if p, err := a.c.Users.GetProfile(ctx, &userv1.GetProfileRequest{CustomerId: customer}); err == nil {
		profile := profileJSON(p.GetProfile())
		if id, err := a.c.Users.GetIdentity(ctx, &userv1.GetIdentityRequest{CustomerId: customer}); err == nil && id.GetCpf() != "" {
			profile["cpf"] = id.GetCpf()
		}
		out["profile"] = profile
	} else if status.Code(err) != codes.NotFound {
		writeGRPCError(w, ctx, err)
		return
	}
	sessions, err := a.c.Identity.ListSessions(ctx, &identityv1.ListSessionsRequest{CustomerId: customer})
	if err != nil {
		writeGRPCError(w, ctx, err)
		return
	}
	list := []map[string]any{}
	for _, s := range sessions.GetSessions() {
		list = append(list, map[string]any{"device": s.GetDevice(), "ip": s.GetIp(), "created_at": ts(s.GetCreatedAt()), "last_used_at": ts(s.GetLastUsedAt())})
	}
	out["sessions"] = list
	if prefs, err := a.c.Notifications.GetPreferences(ctx, &notificationv1.GetPreferencesRequest{CustomerId: customer}); err == nil {
		out["notification_preferences"] = map[string]bool{"push": prefs.GetPreferences().GetPush(), "email": prefs.GetPreferences().GetEmail()}
	}
	accs, err := a.c.Accounts.ListAccounts(ctx, &accountv1.ListAccountsRequest{CustomerId: customer})
	if err != nil {
		writeGRPCError(w, ctx, err)
		return
	}
	accounts := []map[string]any{}
	for _, acc := range accs.GetAccounts() {
		item, err := a.exportAccount(ctx, acc)
		if err != nil {
			writeGRPCError(w, ctx, err)
			return
		}
		accounts = append(accounts, item)
	}
	out["accounts"] = accounts

	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		writeError(w, ctx, http.StatusInternalServerError, "INTERNAL", "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="meus-dados-`+time.Now().Format("2006-01-02")+`.json"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// exportAccount gathers one account: balance, Pix keys, cards (last 4 only),
// savings boxes, credit line, budgets and the statement.
func (a *API) exportAccount(ctx context.Context, acc *accountv1.Account) (map[string]any, error) {
	id := acc.GetId()
	out := map[string]any{"account": accountJSON(acc)}
	if bal, err := a.c.Accounts.GetBalance(ctx, &accountv1.GetBalanceRequest{AccountId: id}); err == nil {
		out["balance"] = moneyJSON(bal.GetAvailable())
	}
	keys, err := a.c.Pix.ListKeys(ctx, &pixv1.ListKeysRequest{AccountId: id})
	if err != nil {
		return nil, err
	}
	pix := []map[string]string{}
	for _, k := range keys.GetKeys() {
		pix = append(pix, keyJSON(k))
	}
	out["pix_keys"] = pix
	cards, err := a.c.Cards.ListCards(ctx, &cardv1.ListCardsRequest{AccountId: id})
	if err != nil {
		return nil, err
	}
	cl := []map[string]any{}
	for _, c := range cards.GetCards() {
		cl = append(cl, cardJSON(c))
	}
	out["cards"] = cl
	boxes, err := a.c.Savings.ListBoxes(ctx, &savingsv1.ListBoxesRequest{AccountId: id})
	if err != nil {
		return nil, err
	}
	bl := []map[string]any{}
	for _, b := range boxes.GetBoxes() {
		bl = append(bl, boxJSON(b))
	}
	out["savings_boxes"] = bl
	switch line, err := a.c.Credit.GetCreditLine(ctx, &creditv1.GetCreditLineRequest{AccountId: id}); {
	case status.Code(err) == codes.NotFound:
	case err != nil:
		return nil, err
	default:
		out["credit_line"] = map[string]any{"limit": moneyJSON(line.GetLine().GetLimit()), "used": moneyJSON(line.GetLine().GetUsed()),
			"closing_day": line.GetLine().GetClosingDay()}
	}
	if budgets, err := a.c.Statement.ListBudgets(ctx, &statementv1.ListBudgetsRequest{AccountId: id}); err == nil {
		bs := []map[string]any{}
		for _, b := range budgets.GetBudgets() {
			bs = append(bs, map[string]any{"category": b.GetCategory(), "amount": moneyJSON(b.GetAmount())})
		}
		out["budgets"] = bs
	}
	items := []map[string]any{}
	token := ""
	for len(items) < maxExportItems {
		page, err := a.c.Statement.ListFeed(ctx, &statementv1.ListFeedRequest{AccountId: id, PageSize: 100, PageToken: token})
		if err != nil {
			return nil, err
		}
		for _, it := range page.GetItems() {
			items = append(items, feedItemJSON(it))
		}
		if token = page.GetNextPageToken(); token == "" {
			break
		}
	}
	out["statement"] = items
	return out, nil
}
