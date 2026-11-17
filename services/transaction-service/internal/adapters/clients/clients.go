// Package clients adapts ledger-service and account-service to the saga ports.
package clients

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/ports"
)

// classify turns definitive refusals into *domain.RejectedError. Anything
// else (UNAVAILABLE, DEADLINE_EXCEEDED, INTERNAL...) stays a technical error
// so the saga is retried instead of compensated.
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

func moneyPB(m money.Money) *commonv1.Money {
	return &commonv1.Money{MinorUnits: m.Minor(), Currency: string(m.Currency())}
}

type Ledger struct{ c ledgerv1.LedgerServiceClient }

func NewLedger(c ledgerv1.LedgerServiceClient) *Ledger { return &Ledger{c: c} }

var _ ports.Ledger = (*Ledger)(nil)

func (l *Ledger) Transfer(ctx context.Context, key string, from, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (uuid.UUID, error) {
	debit, err := amount.Neg()
	if err != nil {
		return uuid.Nil, err
	}
	resp, err := l.c.PostEntry(ctx, &ledgerv1.PostEntryRequest{
		IdempotencyKey: key,
		Postings: []*ledgerv1.Posting{
			{AccountId: from.String(), Amount: moneyPB(debit)},
			{AccountId: to.String(), Amount: moneyPB(amount)},
		},
		Description: description,
		Metadata:    metadata,
	})
	if err != nil {
		return uuid.Nil, classify(err)
	}
	return uuid.Parse(resp.GetEntry().GetId())
}

func (l *Ledger) Hold(ctx context.Context, key string, from, to uuid.UUID, amount money.Money, description string) (uuid.UUID, error) {
	resp, err := l.c.Hold(ctx, &ledgerv1.HoldRequest{
		IdempotencyKey: key, AccountId: from.String(), CounterAccountId: to.String(),
		Amount: moneyPB(amount), Description: description,
	})
	if err != nil {
		return uuid.Nil, classify(err)
	}
	return uuid.Parse(resp.GetHold().GetId())
}

func (l *Ledger) Capture(ctx context.Context, holdID uuid.UUID, amount *money.Money) (uuid.UUID, error) {
	req := &ledgerv1.CaptureRequest{HoldId: holdID.String()}
	if amount != nil {
		req.Amount = moneyPB(*amount)
	}
	resp, err := l.c.Capture(ctx, req)
	if err != nil {
		return uuid.Nil, classify(err)
	}
	return uuid.Parse(resp.GetEntry().GetId())
}

func (l *Ledger) Release(ctx context.Context, holdID uuid.UUID) error {
	_, err := l.c.Release(ctx, &ledgerv1.ReleaseRequest{HoldId: holdID.String()})
	return classify(err)
}

type Accounts struct {
	c accountv1.AccountServiceClient
}

func NewAccounts(c accountv1.AccountServiceClient) *Accounts { return &Accounts{c: c} }

var _ ports.Accounts = (*Accounts)(nil)

func (a *Accounts) ConsumeLimit(ctx context.Context, account uuid.UUID, txID string, amount money.Money) error {
	_, err := a.c.ConsumeLimit(ctx, &accountv1.ConsumeLimitRequest{
		AccountId: account.String(), TransactionId: txID, Amount: moneyPB(amount),
	})
	return classify(err)
}

func (a *Accounts) RestoreLimit(ctx context.Context, account uuid.UUID, txID string) error {
	_, err := a.c.RestoreLimit(ctx, &accountv1.RestoreLimitRequest{AccountId: account.String(), TransactionId: txID})
	return classify(err)
}

func (a *Accounts) EnsureActive(ctx context.Context, account uuid.UUID) error {
	resp, err := a.c.GetAccount(ctx, &accountv1.GetAccountRequest{Id: account.String()})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return domain.Reject("DESTINATION_NOT_FOUND", "destination account does not exist")
		}
		return classify(err)
	}
	if resp.GetAccount().GetStatus() != accountv1.AccountStatus_ACCOUNT_STATUS_ACTIVE {
		return domain.Reject("DESTINATION_NOT_ACTIVE", "destination account is not active")
	}
	return nil
}
