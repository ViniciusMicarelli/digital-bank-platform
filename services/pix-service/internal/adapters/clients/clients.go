// Package clients adapts account-service and transaction-service to the Pix ports.
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
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/ports"
)

// PixSettlementAccountID is the ledger's internal Pix settlement account.
var PixSettlementAccountID = uuid.MustParse("00000000-0000-0000-0000-000000000002")

func classify(err error) error {
	if err == nil {
		return nil
	}
	st := status.Convert(err)
	switch st.Code() {
	case codes.FailedPrecondition, codes.NotFound, codes.InvalidArgument:
		reason := grpcserver.Reason(err)
		if reason == "" {
			reason = st.Code().String()
		}
		return domain.Reject(reason, st.Message())
	}
	return err
}

type Accounts struct {
	c accountv1.AccountServiceClient
}

func NewAccounts(c accountv1.AccountServiceClient) *Accounts { return &Accounts{c: c} }

var _ ports.Accounts = (*Accounts)(nil)

func (a *Accounts) Holder(ctx context.Context, id uuid.UUID) (ports.Holder, error) {
	resp, err := a.c.GetAccount(ctx, &accountv1.GetAccountRequest{Id: id.String()})
	if status.Code(err) == codes.NotFound {
		return ports.Holder{}, domain.ErrNotFound
	}
	if err != nil {
		return ports.Holder{}, err
	}
	acc := resp.GetAccount()
	return ports.Holder{
		Name: acc.GetHolderName(), Document: acc.GetDocument(),
		Active: acc.GetStatus() == accountv1.AccountStatus_ACCOUNT_STATUS_ACTIVE,
	}, nil
}

type Transactions struct{ c txv1.TransactionServiceClient }

func NewTransactions(c txv1.TransactionServiceClient) *Transactions { return &Transactions{c: c} }

var _ ports.Transactions = (*Transactions)(nil)

func moneyPB(m money.Money) *commonv1.Money {
	return &commonv1.Money{MinorUnits: m.Minor(), Currency: string(m.Currency())}
}

func result(t *txv1.Transaction, err error) (ports.TxResult, error) {
	if err != nil {
		return ports.TxResult{}, classify(err)
	}
	id, err := uuid.Parse(t.GetId())
	if err != nil {
		return ports.TxResult{}, err
	}
	st := map[txv1.TransactionStatus]string{
		txv1.TransactionStatus_TRANSACTION_STATUS_PENDING:      "PENDING",
		txv1.TransactionStatus_TRANSACTION_STATUS_RESERVED:     "RESERVED",
		txv1.TransactionStatus_TRANSACTION_STATUS_COMPLETED:    "COMPLETED",
		txv1.TransactionStatus_TRANSACTION_STATUS_COMPENSATING: "COMPENSATING",
		txv1.TransactionStatus_TRANSACTION_STATUS_FAILED:       "FAILED",
	}[t.GetStatus()]
	return ports.TxResult{ID: id, Status: st, FailureReason: t.GetFailureReason()}, nil
}

func (c *Transactions) Transfer(ctx context.Context, key string, from, to uuid.UUID, amount money.Money, description string, md map[string]string) (ports.TxResult, error) {
	resp, err := c.c.CreateTransfer(ctx, &txv1.CreateTransferRequest{
		IdempotencyKey: key, FromAccountId: from.String(), ToAccountId: to.String(),
		Amount: moneyPB(amount), Description: description, Metadata: md,
	})
	return result(resp.GetTransaction(), err)
}

func (c *Transactions) ReserveOutgoing(ctx context.Context, key string, from uuid.UUID, amount money.Money, description string, md map[string]string) (ports.TxResult, error) {
	resp, err := c.c.ReserveOutgoing(ctx, &txv1.ReserveOutgoingRequest{
		IdempotencyKey: key, Type: txv1.TransactionType_TRANSACTION_TYPE_PIX_OUT,
		FromAccountId: from.String(), ToAccountId: PixSettlementAccountID.String(),
		Amount: moneyPB(amount), Description: description, Metadata: md,
	})
	return result(resp.GetTransaction(), err)
}

func (c *Transactions) Complete(ctx context.Context, id uuid.UUID) (ports.TxResult, error) {
	resp, err := c.c.CompleteTransaction(ctx, &txv1.CompleteTransactionRequest{Id: id.String()})
	return result(resp.GetTransaction(), err)
}

func (c *Transactions) Compensate(ctx context.Context, id uuid.UUID, reason string) (ports.TxResult, error) {
	resp, err := c.c.CompensateTransaction(ctx, &txv1.CompensateTransactionRequest{Id: id.String(), Reason: reason})
	return result(resp.GetTransaction(), err)
}

func (c *Transactions) CreditIncoming(ctx context.Context, key string, to uuid.UUID, amount money.Money, description string, md map[string]string) (ports.TxResult, error) {
	resp, err := c.c.CreditIncoming(ctx, &txv1.CreditIncomingRequest{
		IdempotencyKey: key, Type: txv1.TransactionType_TRANSACTION_TYPE_PIX_IN,
		FromAccountId: PixSettlementAccountID.String(), ToAccountId: to.String(),
		Amount: moneyPB(amount), Description: description, Metadata: md,
	})
	return result(resp.GetTransaction(), err)
}
