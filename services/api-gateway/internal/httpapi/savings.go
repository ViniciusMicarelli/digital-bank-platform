package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	savingsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/savings/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
)

func boxJSON(b *savingsv1.Box) map[string]any {
	out := map[string]any{
		"id": b.GetId(), "name": b.GetName(), "balance": moneyJSON(b.GetBalance()), "yielded": moneyJSON(b.GetYielded()),
		"status": b.GetStatus(), "created_at": ts(b.GetCreatedAt()), "annual_rate_bp": b.GetAnnualRateBp(),
	}
	if b.GetGoal().GetMinorUnits() > 0 {
		out["goal"] = moneyJSON(b.GetGoal())
	}
	return out
}

func movementJSON(m *savingsv1.Movement) map[string]any {
	return map[string]any{"id": m.GetId(), "kind": m.GetKind(), "amount": moneyJSON(m.GetAmount()), "status": m.GetStatus(),
		"failure_reason": m.GetFailureReason(), "created_at": ts(m.GetCreatedAt())}
}

// ownedBox checks {boxID} belongs to the {id} account (404 otherwise).
func (a *API) ownedBox(next func(http.ResponseWriter, *http.Request, *savingsv1.Box)) func(http.ResponseWriter, *http.Request, *accountv1.Account) {
	return func(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
		id := r.PathValue("boxID")
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "box not found")
			return
		}
		resp, err := a.c.Savings.GetBox(r.Context(), &savingsv1.GetBoxRequest{Id: id})
		if err != nil || resp.GetBox().GetAccountId() != acc.GetId() {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "box not found")
			return
		}
		next(w, r, resp.GetBox())
	}
}

// listBoxes answers the account's active boxes and their totals.
func (a *API) listBoxes(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Savings.ListBoxes(r.Context(), &savingsv1.ListBoxesRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	var total, yielded int64
	for _, b := range resp.GetBoxes() {
		out = append(out, boxJSON(b))
		total += b.GetBalance().GetMinorUnits()
		yielded += b.GetYielded().GetMinorUnits()
	}
	writeJSON(w, http.StatusOK, map[string]any{"boxes": out,
		"total": moneyJSON(&commonv1.Money{MinorUnits: total, Currency: "BRL"}), "yielded": moneyJSON(&commonv1.Money{MinorUnits: yielded, Currency: "BRL"})})
}

// createBox opens a box {name, goal?} (Idempotency-Key).
func (a *API) createBox(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
		Goal string `json:"goal"`
	}
	if !decode(w, r, &req) {
		return
	}
	goal, ok := optionalBRL(w, r, req.Goal)
	if !ok {
		return
	}
	resp, err := a.c.Savings.CreateBox(r.Context(), &savingsv1.CreateBoxRequest{IdempotencyKey: key, AccountId: acc.GetId(), Name: req.Name, Goal: goal})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, boxJSON(resp.GetBox()))
}

// getBox answers the box and its latest movements.
func (a *API) getBox(w http.ResponseWriter, r *http.Request, b *savingsv1.Box) {
	resp, err := a.c.Savings.ListMovements(r.Context(), &savingsv1.ListMovementsRequest{BoxId: b.GetId(), Limit: 50})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := boxJSON(b)
	list := []map[string]any{}
	for _, m := range resp.GetMovements() {
		if m.GetStatus() == "FAILED" {
			continue // a refused attempt moved nothing: not part of the history
		}
		list = append(list, movementJSON(m))
	}
	out["movements"] = list
	writeJSON(w, http.StatusOK, out)
}

// updateBox renames a box and sets its goal {name, goal?} (no goal removes it).
func (a *API) updateBox(w http.ResponseWriter, r *http.Request, b *savingsv1.Box) {
	var req struct {
		Name string `json:"name"`
		Goal string `json:"goal"`
	}
	if !decode(w, r, &req) {
		return
	}
	goal, ok := optionalBRL(w, r, req.Goal)
	if !ok {
		return
	}
	resp, err := a.c.Savings.UpdateBox(r.Context(), &savingsv1.UpdateBoxRequest{Id: b.GetId(), Name: req.Name, Goal: goal})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, boxJSON(resp.GetBox()))
}

// closeBox moves everything back to the checking account and closes the box.
func (a *API) closeBox(w http.ResponseWriter, r *http.Request, b *savingsv1.Box) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	resp, err := a.c.Savings.CloseBox(r.Context(), &savingsv1.CloseBoxRequest{Id: b.GetId(), IdempotencyKey: key})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, boxJSON(resp.GetBox()))
}

// boxMove is a deposit or a withdrawal {amount} (Idempotency-Key). A refusal
// (INSUFFICIENT_FUNDS) answers 422 with the movement.
func (a *API) boxMove(deposit bool) func(http.ResponseWriter, *http.Request, *savingsv1.Box) {
	return func(w http.ResponseWriter, r *http.Request, b *savingsv1.Box) {
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
		var (
			m   *savingsv1.Movement
			box *savingsv1.Box
			err error
		)
		if deposit {
			resp, e := a.c.Savings.Deposit(r.Context(), &savingsv1.DepositRequest{IdempotencyKey: key, BoxId: b.GetId(), Amount: amount})
			m, box, err = resp.GetMovement(), resp.GetBox(), e
		} else {
			resp, e := a.c.Savings.Withdraw(r.Context(), &savingsv1.WithdrawRequest{IdempotencyKey: key, BoxId: b.GetId(), Amount: amount})
			m, box, err = resp.GetMovement(), resp.GetBox(), e
		}
		if err != nil {
			writeGRPCError(w, r.Context(), err)
			return
		}
		if m.GetStatus() == "FAILED" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]string{"code": m.GetFailureReason(),
				"message": "the money was not moved"}, "movement": movementJSON(m)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"movement": movementJSON(m), "box": boxJSON(box)})
	}
}

// simulateYield (sandbox) pays {days} of yield at once.
func (a *API) simulateYield(w http.ResponseWriter, r *http.Request, b *savingsv1.Box) {
	var req struct {
		Days int32 `json:"days"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Savings.SimulateYield(r.Context(), &savingsv1.SimulateYieldRequest{BoxId: b.GetId(), Days: req.Days})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"movement": movementJSON(resp.GetMovement()), "box": boxJSON(resp.GetBox())})
}

// spending summarizes where the money went in a month (?month=YYYY-MM).
func (a *API) spending(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Statement.GetSpending(r.Context(), &statementv1.GetSpendingRequest{AccountId: acc.GetId(), Month: r.URL.Query().Get("month")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	cats := []map[string]any{}
	for _, c := range resp.GetCategories() {
		cats = append(cats, map[string]any{"category": c.GetCategory(), "amount": moneyJSON(c.GetAmount()), "count": c.GetCount()})
	}
	merchants := []map[string]any{}
	for _, m := range resp.GetMerchants() {
		merchants = append(merchants, map[string]any{"name": m.GetName(), "category": m.GetCategory(), "amount": moneyJSON(m.GetAmount()), "count": m.GetCount()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"month": resp.GetMonth(), "total": moneyJSON(resp.GetTotal()),
		"previous_total": moneyJSON(resp.GetPreviousTotal()), "categories": cats, "merchants": merchants})
}
