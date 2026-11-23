package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	savingsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/savings/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/document"
)

var brasilia = time.FixedZone("BRT", -3*60*60)

// taxReport is the annual income report ("informe de rendimentos") of an
// account (?year=YYYY, the previous year by default): the checking balance
// and each savings box's balance on 31/12 of the year before and of the
// year, and what the boxes yielded. Closed accounts keep theirs.
func (a *API) taxReport(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	now := time.Now().In(brasilia)
	year := now.Year() - 1
	if v := r.URL.Query().Get("year"); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil || y < 2000 || y > now.Year() {
			writeError(w, r.Context(), http.StatusBadRequest, "INVALID_YEAR", "year must be a past or the current year")
			return
		}
		year = y
	}
	from := time.Date(year, 1, 1, 0, 0, 0, 0, brasilia)
	to := from.AddDate(1, 0, 0)
	balanceAt := func(at time.Time) (*accountv1.GetBalanceAtResponse, error) {
		return a.c.Accounts.GetBalanceAt(r.Context(), &accountv1.GetBalanceAtRequest{AccountId: acc.GetId(), At: timestamppb.New(at)})
	}
	start, err := balanceAt(from)
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	end, err := balanceAt(to)
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	boxes, err := a.c.Savings.GetYearReport(r.Context(), &savingsv1.GetYearReportRequest{AccountId: acc.GetId(), Year: int32(year)}) //nolint:gosec // 2000..now
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	list := []map[string]any{}
	var yielded, savedStart, savedEnd int64
	for _, b := range boxes.GetBoxes() {
		list = append(list, map[string]any{"name": b.GetName(), "status": b.GetStatus(), "balance_start": moneyJSON(b.GetBalanceStart()),
			"balance_end": moneyJSON(b.GetBalanceEnd()), "yielded": moneyJSON(b.GetYielded())})
		yielded += b.GetYielded().GetMinorUnits()
		savedStart += b.GetBalanceStart().GetMinorUnits()
		savedEnd += b.GetBalanceEnd().GetMinorUnits()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"year": year, "complete": now.After(to), "generated_at": now.UTC().Format(time.RFC3339),
		"holder":   map[string]string{"name": acc.GetHolderName(), "document": document.MaskCPF(acc.GetDocument())},
		"account":  map[string]string{"branch": acc.GetBranch(), "number": acc.GetNumber()},
		"checking": map[string]any{"balance_start": moneyJSON(start.GetPosted()), "balance_end": moneyJSON(end.GetPosted())},
		"savings": map[string]any{"boxes": list, "balance_start": moneyJSON(brlMoney(savedStart)), "balance_end": moneyJSON(brlMoney(savedEnd)),
			"yielded": moneyJSON(brlMoney(yielded))},
	})
}
