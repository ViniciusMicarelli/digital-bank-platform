package httpapi

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	paymentv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/payment/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	savingsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/savings/v1"
)

// closure is what stands between an account and its closing, and what
// closing it will undo on the way.
type closure struct {
	// Blockers the customer must clear first: money left (BALANCE), money in
	// savings boxes (SAVINGS), card debt (CREDIT_DEBT), boleto payments
	// on their way (PAYMENTS_IN_PROGRESS).
	Blockers []map[string]any
	// Cancelled on closing: scheduled Pix, scheduled boletos, Pix
	// Automático authorizations, empty savings boxes.
	pixSchedules, boletos, mandates, emptyBoxes []string
}

func brlMoney(minor int64) *commonv1.Money {
	return &commonv1.Money{MinorUnits: minor, Currency: "BRL"}
}

// closureOf gathers the account's blockers and loose ends from every
// service that keeps something of it. Any service it cannot ask makes the
// whole check fail: closing on partial information is not safe.
func (a *API) closureOf(ctx context.Context, acc *accountv1.Account) (closure, error) {
	var c closure
	id := acc.GetId()
	bal, err := a.c.Accounts.GetBalance(ctx, &accountv1.GetBalanceRequest{AccountId: id})
	if err != nil {
		return c, err
	}
	if p := bal.GetPosted().GetMinorUnits(); p != 0 || bal.GetPending().GetMinorUnits() != 0 {
		c.Blockers = append(c.Blockers, map[string]any{"code": "BALANCE", "amount": moneyJSON(bal.GetAvailable())})
	}
	boxes, err := a.c.Savings.ListBoxes(ctx, &savingsv1.ListBoxesRequest{AccountId: id})
	if err != nil {
		return c, err
	}
	var saved int64
	var full int
	for _, b := range boxes.GetBoxes() {
		if m := b.GetBalance().GetMinorUnits(); m > 0 {
			saved += m
			full++
		} else {
			c.emptyBoxes = append(c.emptyBoxes, b.GetId())
		}
	}
	if full > 0 {
		c.Blockers = append(c.Blockers, map[string]any{"code": "SAVINGS", "count": full, "amount": moneyJSON(brlMoney(saved))})
	}
	line, err := a.c.Credit.GetCreditLine(ctx, &creditv1.GetCreditLineRequest{AccountId: id})
	switch {
	case status.Code(err) == codes.NotFound:
	case err != nil:
		return c, err
	case line.GetLine().GetUsed().GetMinorUnits() > 0:
		c.Blockers = append(c.Blockers, map[string]any{"code": "CREDIT_DEBT", "amount": moneyJSON(line.GetLine().GetUsed())})
	}
	pays, err := a.c.Payments.ListBoletoPayments(ctx, &paymentv1.ListBoletoPaymentsRequest{AccountId: id})
	if err != nil {
		return c, err
	}
	inFlight := 0
	for _, p := range pays.GetPayments() {
		switch p.GetStatus() {
		case "SCHEDULED":
			c.boletos = append(c.boletos, p.GetId())
		case "CREATED", "FUNDS_RESERVED", "SENT_TO_CLEARING":
			inFlight++
		}
	}
	if inFlight > 0 {
		c.Blockers = append(c.Blockers, map[string]any{"code": "PAYMENTS_IN_PROGRESS", "count": inFlight})
	}
	scheds, err := a.c.Pix.ListSchedules(ctx, &pixv1.ListSchedulesRequest{AccountId: id})
	if err != nil {
		return c, err
	}
	for _, s := range scheds.GetSchedules() {
		if s.GetStatus() == "ACTIVE" {
			c.pixSchedules = append(c.pixSchedules, s.GetId())
		}
	}
	mandates, err := a.c.Pix.ListMandates(ctx, &pixv1.ListMandatesRequest{AccountId: id})
	if err != nil {
		return c, err
	}
	for _, m := range mandates.GetMandates() {
		if m.GetPayerAccountId() == id && (m.GetStatus() == "ACTIVE" || m.GetStatus() == "PENDING") {
			c.mandates = append(c.mandates, m.GetId())
		}
	}
	return c, nil
}

func (c closure) json(acc *accountv1.Account) map[string]any {
	blockers := c.Blockers
	if blockers == nil {
		blockers = []map[string]any{}
	}
	return map[string]any{"account_id": acc.GetId(), "status": enumName(acc.GetStatus().String(), "ACCOUNT_STATUS_"),
		"can_close": len(c.Blockers) == 0 && acc.GetStatus() == accountv1.AccountStatus_ACCOUNT_STATUS_ACTIVE, "blockers": blockers,
		"will_cancel": map[string]int{"pix_schedules": len(c.pixSchedules), "boleto_schedules": len(c.boletos),
			"pix_automatico": len(c.mandates), "empty_savings_boxes": len(c.emptyBoxes)}}
}

// closureCheck says whether the account can be closed now, and what is in the way.
func (a *API) closureCheck(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	c, err := a.closureOf(r.Context(), acc)
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, c.json(acc))
}

// closeAccount closes the account at the customer's request {reason?}
// (step-up). With blockers it answers 409 CLOSURE_BLOCKED and the check.
// Otherwise it cancels what would run on a closed account (scheduled Pix
// and boletos, Pix Automático, empty boxes), then closes it: account-service
// re-checks the balance, and the account.closed event removes Pix keys and
// cancels cards.
func (a *API) closeAccount(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if acc.GetStatus() != accountv1.AccountStatus_ACCOUNT_STATUS_ACTIVE {
		writeError(w, r.Context(), http.StatusConflict, "ACCOUNT_NOT_ACTIVE", "only an active account can be closed by its holder")
		return
	}
	ctx := r.Context()
	c, err := a.closureOf(ctx, acc)
	if err != nil {
		writeGRPCError(w, ctx, err)
		return
	}
	if len(c.Blockers) > 0 {
		body := c.json(acc)
		body["error"] = map[string]string{"code": "CLOSURE_BLOCKED", "message": "clear the blockers before closing the account"}
		writeJSON(w, http.StatusConflict, body)
		return
	}
	id := acc.GetId()
	for _, s := range c.pixSchedules {
		if _, err := a.c.Pix.CancelSchedule(ctx, &pixv1.CancelScheduleRequest{AccountId: id, ScheduleId: s}); err != nil {
			writeGRPCError(w, ctx, err)
			return
		}
	}
	for _, p := range c.boletos {
		if _, err := a.c.Payments.CancelBoletoPayment(ctx, &paymentv1.CancelBoletoPaymentRequest{Id: p}); err != nil {
			writeGRPCError(w, ctx, err)
			return
		}
	}
	for _, m := range c.mandates {
		if _, err := a.c.Pix.RevokeMandate(ctx, &pixv1.RevokeMandateRequest{AccountId: id, MandateId: m}); err != nil {
			writeGRPCError(w, ctx, err)
			return
		}
	}
	for _, b := range c.emptyBoxes {
		if _, err := a.c.Savings.CloseBox(ctx, &savingsv1.CloseBoxRequest{Id: b, IdempotencyKey: "account-closure:" + b}); err != nil {
			writeGRPCError(w, ctx, err)
			return
		}
	}
	reason := "Pedido do cliente"
	if t := strings.TrimSpace(req.Reason); t != "" {
		reason += ": " + t
	}
	resp, err := a.c.Accounts.CloseAccount(ctx, &accountv1.CloseAccountRequest{Id: id, Reason: reason})
	if err != nil {
		writeGRPCError(w, ctx, err)
		return
	}
	writeJSON(w, http.StatusOK, accountJSON(resp.GetAccount()))
}
