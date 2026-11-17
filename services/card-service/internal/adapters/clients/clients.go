// Package clients adapts account-service and transaction-service to the card ports.
package clients

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/ports"
)

// CardClearingAccountID is the ledger's internal card clearing account.
var CardClearingAccountID = uuid.MustParse("00000000-0000-0000-0000-000000000003")

type Accounts struct {
	c accountv1.AccountServiceClient
}

func NewAccounts(c accountv1.AccountServiceClient) *Accounts { return &Accounts{c: c} }

func (a *Accounts) Holder(ctx context.Context, id uuid.UUID) (ports.Holder, error) {
	resp, err := a.c.GetAccount(ctx, &accountv1.GetAccountRequest{Id: id.String()})
	if status.Code(err) == codes.NotFound {
		return ports.Holder{}, domain.ErrNotFound
	}
	if err != nil {
		return ports.Holder{}, err
	}
	return ports.Holder{Name: resp.GetAccount().GetHolderName(),
		Active: resp.GetAccount().GetStatus() == accountv1.AccountStatus_ACCOUNT_STATUS_ACTIVE}, nil
}

type Transactions struct{ c txv1.TransactionServiceClient }

func NewTransactions(c txv1.TransactionServiceClient) *Transactions { return &Transactions{c: c} }

func moneyPB(m money.Money) *commonv1.Money {
	return &commonv1.Money{MinorUnits: m.Minor(), Currency: string(m.Currency())}
}

func result(t *txv1.Transaction, err error) (ports.TxResult, error) {
	if err != nil {
		return ports.TxResult{}, err
	}
	id, err := uuid.Parse(t.GetId())
	if err != nil {
		return ports.TxResult{}, err
	}
	return ports.TxResult{ID: id, Status: t.GetStatus().String()[len("TRANSACTION_STATUS_"):], FailureReason: t.GetFailureReason()}, nil
}

func (c *Transactions) Reserve(ctx context.Context, key string, account uuid.UUID, amount money.Money, description string, md map[string]string) (ports.TxResult, error) {
	resp, err := c.c.ReserveOutgoing(ctx, &txv1.ReserveOutgoingRequest{
		IdempotencyKey: key, Type: txv1.TransactionType_TRANSACTION_TYPE_DEBIT_CARD_PURCHASE,
		FromAccountId: account.String(), ToAccountId: CardClearingAccountID.String(),
		Amount: moneyPB(amount), Description: description, Metadata: md,
	})
	return result(resp.GetTransaction(), err)
}

func (c *Transactions) Complete(ctx context.Context, id uuid.UUID, amount *money.Money) (ports.TxResult, error) {
	req := &txv1.CompleteTransactionRequest{Id: id.String()}
	if amount != nil {
		req.Amount = moneyPB(*amount)
	}
	resp, err := c.c.CompleteTransaction(ctx, req)
	return result(resp.GetTransaction(), err)
}

func (c *Transactions) Compensate(ctx context.Context, id uuid.UUID, reason string) (ports.TxResult, error) {
	resp, err := c.c.CompensateTransaction(ctx, &txv1.CompensateTransactionRequest{Id: id.String(), Reason: reason})
	return result(resp.GetTransaction(), err)
}
