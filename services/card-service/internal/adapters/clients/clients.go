// Package clients adapts account-service and transaction-service to the card ports.
package clients

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
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

// Credit reserves credit purchases on the credit line (credit-service),
// with the transaction-service reserve / complete / compensate contract.
type Credit struct{ c creditv1.CreditServiceClient }

func NewCredit(c creditv1.CreditServiceClient) *Credit { return &Credit{c: c} }

func hold(h *creditv1.Hold, err error) (ports.TxResult, error) {
	if err != nil {
		return ports.TxResult{}, err
	}
	id, err := uuid.Parse(h.GetId())
	if err != nil {
		return ports.TxResult{}, err
	}
	st := map[string]string{"RESERVED": "RESERVED", "CAPTURED": "COMPLETED", "RELEASED": "FAILED", "FAILED": "FAILED"}[h.GetStatus()]
	return ports.TxResult{ID: id, Status: st, FailureReason: h.GetFailureReason()}, nil
}

func (c *Credit) Reserve(ctx context.Context, key string, account uuid.UUID, amount money.Money, description string, md map[string]string) (ports.TxResult, error) {
	resp, err := c.c.ReserveCredit(ctx, &creditv1.ReserveCreditRequest{IdempotencyKey: key, AccountId: account.String(),
		Amount: moneyPB(amount), Description: description, Metadata: md})
	return hold(resp.GetHold(), err)
}

func (c *Credit) Complete(ctx context.Context, id uuid.UUID, amount *money.Money) (ports.TxResult, error) {
	req := &creditv1.CaptureCreditRequest{HoldId: id.String()}
	if amount != nil {
		req.Amount = moneyPB(*amount)
	}
	resp, err := c.c.CaptureCredit(ctx, req)
	return hold(resp.GetHold(), err)
}

func (c *Credit) Compensate(ctx context.Context, id uuid.UUID, reason string) (ports.TxResult, error) {
	resp, err := c.c.ReleaseCredit(ctx, &creditv1.ReleaseCreditRequest{HoldId: id.String(), Reason: reason})
	return hold(resp.GetHold(), err)
}

func (c *Transactions) Compensate(ctx context.Context, id uuid.UUID, reason string) (ports.TxResult, error) {
	resp, err := c.c.CompensateTransaction(ctx, &txv1.CompensateTransactionRequest{Id: id.String(), Reason: reason})
	return result(resp.GetTransaction(), err)
}

// Refund credits a debit purchase back to the checking account, from card
// clearing (CARD_REFUND).
func (c *Transactions) Refund(ctx context.Context, key string, account uuid.UUID, amount money.Money, description string, md map[string]string) (ports.TxResult, error) {
	resp, err := c.c.CreditIncoming(ctx, &txv1.CreditIncomingRequest{
		IdempotencyKey: key, Type: txv1.TransactionType_TRANSACTION_TYPE_CARD_REFUND,
		FromAccountId: CardClearingAccountID.String(), ToAccountId: account.String(),
		Amount: moneyPB(amount), Description: description, Metadata: md,
	})
	return result(resp.GetTransaction(), err)
}

// Refund credits a credit purchase back on the invoice (credit-service).
// A refusal credit-service would repeat (no line, bad request) is FAILED.
func (c *Credit) Refund(ctx context.Context, key string, account uuid.UUID, amount money.Money, description string, md map[string]string) (ports.TxResult, error) {
	resp, err := c.c.RefundCredit(ctx, &creditv1.RefundCreditRequest{IdempotencyKey: key, AccountId: account.String(),
		Amount: moneyPB(amount), Description: description, Metadata: md})
	switch status.Code(err) {
	case codes.OK:
	case codes.NotFound, codes.InvalidArgument, codes.FailedPrecondition, codes.AlreadyExists:
		return ports.TxResult{Status: "FAILED", FailureReason: status.Code(err).String()}, nil
	default:
		return ports.TxResult{}, err
	}
	id, err := uuid.Parse(resp.GetRefund().GetId())
	if err != nil {
		return ports.TxResult{}, err
	}
	return ports.TxResult{ID: id, Status: "COMPLETED"}, nil
}
