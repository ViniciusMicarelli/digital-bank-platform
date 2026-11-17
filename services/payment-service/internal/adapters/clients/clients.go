// Package clients adapts transaction-service to the payment ports.
package clients

import (
	"context"

	"github.com/google/uuid"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/ports"
)

// BoletoSettlementAccountID is the ledger's internal boleto settlement account.
var BoletoSettlementAccountID = uuid.MustParse("00000000-0000-0000-0000-000000000006")

type Transactions struct{ c txv1.TransactionServiceClient }

func NewTransactions(c txv1.TransactionServiceClient) *Transactions { return &Transactions{c: c} }

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

func (c *Transactions) Reserve(ctx context.Context, key string, account uuid.UUID, amount int64, description string, md map[string]string) (ports.TxResult, error) {
	resp, err := c.c.ReserveOutgoing(ctx, &txv1.ReserveOutgoingRequest{
		IdempotencyKey: key, Type: txv1.TransactionType_TRANSACTION_TYPE_BOLETO_PAYMENT,
		FromAccountId: account.String(), ToAccountId: BoletoSettlementAccountID.String(),
		Amount: &commonv1.Money{MinorUnits: amount, Currency: "BRL"}, Description: description, Metadata: md,
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
