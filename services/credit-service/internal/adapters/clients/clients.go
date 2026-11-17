// Package clients adapts account-service and transaction-service to the credit ports.
package clients

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/ports"
)

type Accounts struct {
	c accountv1.AccountServiceClient
}

func NewAccounts(c accountv1.AccountServiceClient) *Accounts { return &Accounts{c: c} }

func (a *Accounts) Active(ctx context.Context, id uuid.UUID) (bool, error) {
	resp, err := a.c.GetAccount(ctx, &accountv1.GetAccountRequest{Id: id.String()})
	if status.Code(err) == codes.NotFound {
		return false, domain.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	return resp.GetAccount().GetStatus() == accountv1.AccountStatus_ACCOUNT_STATUS_ACTIVE, nil
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
	id, err := uuid.Parse(resp.GetTransaction().GetId())
	if err != nil {
		return ports.TxResult{}, err
	}
	return ports.TxResult{ID: id, Status: resp.GetTransaction().GetStatus().String()[len("TRANSACTION_STATUS_"):],
		FailureReason: resp.GetTransaction().GetFailureReason()}, nil
}
