// Package ledger adapts the ledger-service gRPC API to the account ports.
package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/ports"
)

type Client struct{ c ledgerv1.LedgerServiceClient }

func New(c ledgerv1.LedgerServiceClient) *Client { return &Client{c: c} }

var _ ports.Ledger = (*Client)(nil)

func (l *Client) CreateCustomerAccount(ctx context.Context, id uuid.UUID, currency money.Currency) error {
	_, err := l.c.CreateLedgerAccount(ctx, &ledgerv1.CreateLedgerAccountRequest{
		Id: id.String(), Kind: ledgerv1.AccountKind_ACCOUNT_KIND_CUSTOMER, Currency: string(currency),
	})
	return err
}

func (l *Client) Balance(ctx context.Context, id uuid.UUID) (ports.Balance, error) {
	resp, err := l.c.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: id.String()})
	if err != nil {
		return ports.Balance{}, err
	}
	b := resp.GetBalance()
	var out ports.Balance
	for dst, src := range map[*money.Money]*commonv1.Money{&out.Posted: b.GetPosted(), &out.Pending: b.GetPending(), &out.Available: b.GetAvailable()} {
		m, err := money.New(src.GetMinorUnits(), money.Currency(src.GetCurrency()))
		if err != nil {
			return ports.Balance{}, fmt.Errorf("ledger balance: %w", err)
		}
		*dst = m
	}
	return out, nil
}

func (l *Client) BalanceAt(ctx context.Context, id uuid.UUID, at time.Time) (int64, error) {
	resp, err := l.c.GetBalanceAt(ctx, &ledgerv1.GetBalanceAtRequest{AccountId: id.String(), At: timestamppb.New(at)})
	if err != nil {
		return 0, err
	}
	return resp.GetPosted().GetMinorUnits(), nil
}
