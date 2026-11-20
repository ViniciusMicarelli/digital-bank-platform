package httpapi

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
)

// typeNames are the statement types as a customer reads them (CSV export).
var typeNames = map[string]string{
	"INTERNAL_TRANSFER": "Transferência", "PIX": "Pix", "PIX_OUT": "Pix enviado", "PIX_IN": "Pix recebido",
	"PIX_REFUND": "Devolução Pix", "DEBIT_CARD_PURCHASE": "Compra no débito", "CREDIT_INVOICE_PAYMENT": "Pagamento de fatura",
	"BOLETO_PAYMENT": "Boleto", "CARD_REFUND": "Estorno no cartão", "SAVINGS_DEPOSIT": "Guardado na caixinha",
	"SAVINGS_WITHDRAWAL": "Resgate da caixinha",
}

func feedItemJSON(it *statementv1.FeedItem) map[string]any {
	dir := "DEBIT"
	if it.GetDirection() == statementv1.Direction_DIRECTION_CREDIT {
		dir = "CREDIT"
	}
	return map[string]any{
		"id": it.GetId(), "transaction_id": it.GetTransactionId(), "type": it.GetType(), "direction": dir,
		"amount": moneyJSON(it.GetAmount()), "counterparty_name": it.GetCounterpartyName(),
		"description": it.GetDescription(), "end_to_end_id": it.GetEndToEndId(), "occurred_at": ts(it.GetOccurredAt()),
	}
}

func receiptJSON(rc *statementv1.Receipt) map[string]any {
	return map[string]any{"item": feedItemJSON(rc.GetItem()), "holder_name": rc.GetHolderName(), "branch": rc.GetBranch(),
		"number": rc.GetNumber(), "counterparty_branch": rc.GetCounterpartyBranch(), "counterparty_number": rc.GetCounterpartyNumber(),
		"authentication_code": rc.GetAuthenticationCode()}
}

// receipt is the receipt of one statement item, with its authentication code.
func (a *API) receipt(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	if _, err := uuid.Parse(r.PathValue("itemID")); err != nil {
		writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "statement item not found")
		return
	}
	resp, err := a.c.Statement.GetReceipt(r.Context(), &statementv1.GetReceiptRequest{AccountId: acc.GetId(), ItemId: r.PathValue("itemID")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, receiptJSON(resp.GetReceipt()))
}

// verifyReceipt checks an authentication code (public: anyone holding a
// receipt may check it; names come back masked).
func (a *API) verifyReceipt(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Statement.VerifyReceipt(r.Context(), &statementv1.VerifyReceiptRequest{AuthenticationCode: r.PathValue("code")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, receiptJSON(resp.GetReceipt()))
}

// statementCSV exports a period (?from=YYYY-MM-DD&to=YYYY-MM-DD, inclusive,
// Brasília days; the current month by default) as CSV for spreadsheets in
// Portuguese: ";" separated, decimal comma, UTF-8 with a BOM.
func (a *API) statementCSV(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	brt := time.FixedZone("BRT", -3*60*60)
	now := time.Now().In(brt)
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, brt)
	to := from.AddDate(0, 1, 0)
	for _, p := range []struct {
		name string
		dst  *time.Time
		add  int
	}{{"from", &from, 0}, {"to", &to, 1}} {
		if v := r.URL.Query().Get(p.name); v != "" {
			d, err := time.ParseInLocation(time.DateOnly, v, brt)
			if err != nil {
				writeError(w, r.Context(), http.StatusBadRequest, "INVALID_DATE", p.name+" must be YYYY-MM-DD")
				return
			}
			*p.dst = d.AddDate(0, 0, p.add)
		}
	}
	if !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		writeError(w, r.Context(), http.StatusBadRequest, "INVALID_PERIOD", "the period is from 1 day to 1 year")
		return
	}
	var items []*statementv1.FeedItem
	token := ""
	for page := 0; page < 100; page++ {
		resp, err := a.c.Statement.ListFeed(r.Context(), &statementv1.ListFeedRequest{AccountId: acc.GetId(), PageSize: 100,
			PageToken: token, From: timestamppb.New(from), To: timestamppb.New(to)})
		if err != nil {
			writeGRPCError(w, r.Context(), err)
			return
		}
		items = append(items, resp.GetItems()...)
		if token = resp.GetNextPageToken(); token == "" {
			break
		}
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="extrato-%s-a-%s.csv"`,
		from.Format(time.DateOnly), to.AddDate(0, 0, -1).Format(time.DateOnly)))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("\ufeff")) // BOM: spreadsheets read UTF-8
	out := csv.NewWriter(w)
	out.Comma = ';'
	_ = out.Write([]string{"Data", "Hora", "Tipo", "Descrição", "Contraparte", "Valor (R$)", "ID Pix"})
	for i := len(items) - 1; i >= 0; i-- { // oldest first, as statements read
		it := items[i]
		at := it.GetOccurredAt().AsTime().In(brt)
		amount := strings.Replace(moneyJSON(it.GetAmount()).Amount, ".", ",", 1)
		if it.GetDirection() != statementv1.Direction_DIRECTION_CREDIT {
			amount = "-" + amount
		}
		name := typeNames[it.GetType()]
		if name == "" {
			name = it.GetType()
		}
		_ = out.Write([]string{at.Format("02/01/2006"), at.Format("15:04"), name, it.GetDescription(), it.GetCounterpartyName(), amount, it.GetEndToEndId()})
	}
	out.Flush()
}
