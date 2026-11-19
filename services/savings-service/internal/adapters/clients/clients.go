// Package clients adapts ledger-service and transaction-service to the
// savings ports.
package clients

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/ports"
)

type Ledger struct{ c ledgerv1.LedgerServiceClient }

func NewLedger(c ledgerv1.LedgerServiceClient) *Ledger { return &Ledger{c: c} }

var _ ports.Ledger = (*Ledger)(nil)

// CreateBoxAccount is idempotent: the box id is the ledger account id.
func (l *Ledger) CreateBoxAccount(ctx context.Context, id uuid.UUID) error {
	_, err := l.c.CreateLedgerAccount(ctx, &ledgerv1.CreateLedgerAccountRequest{
		Id: id.String(), Kind: ledgerv1.AccountKind_ACCOUNT_KIND_CUSTOMER, Currency: "BRL"})
	return err
}

func (l *Ledger) Balance(ctx context.Context, id uuid.UUID) (int64, error) {
	resp, err := l.c.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: id.String()})
	return resp.GetBalance().GetPosted().GetMinorUnits(), err
}

func (l *Ledger) BalanceAt(ctx context.Context, id uuid.UUID, at time.Time) (int64, error) {
	resp, err := l.c.GetBalanceAt(ctx, &ledgerv1.GetBalanceAtRequest{AccountId: id.String(), At: timestamppb.New(at)})
	return resp.GetPosted().GetMinorUnits(), err
}

type Transactions struct{ c txv1.TransactionServiceClient }

func NewTransactions(c txv1.TransactionServiceClient) *Transactions { return &Transactions{c: c} }

func (t *Transactions) Post(ctx context.Context, key, typ string, from, to uuid.UUID, amount int64, description string, md map[string]string) (ports.TxResult, error) {
	resp, err := t.c.CreateTransfer(ctx, &txv1.CreateTransferRequest{
		IdempotencyKey: key, Type: txv1.TransactionType(txv1.TransactionType_value["TRANSACTION_TYPE_"+typ]),
		FromAccountId: from.String(), ToAccountId: to.String(),
		Amount: &commonv1.Money{MinorUnits: amount, Currency: "BRL"}, Description: description, Metadata: md,
	})
	if err != nil {
		return ports.TxResult{}, err
	}
	tx := resp.GetTransaction()
	st := tx.GetStatus().String()[len("TRANSACTION_STATUS_"):]
	if st != "COMPLETED" && st != "FAILED" {
		return ports.TxResult{}, errStillRunning
	}
	return ports.TxResult{Status: st, FailureReason: tx.GetFailureReason()}, nil
}

type stillRunning struct{}

func (stillRunning) Error() string { return "the transaction is still running" }

var errStillRunning error = stillRunning{}
