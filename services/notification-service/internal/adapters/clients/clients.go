// Package clients adapts account-service to the notification ports.
package clients

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/domain"
)

type Accounts struct {
	c accountv1.AccountServiceClient
}

func NewAccounts(c accountv1.AccountServiceClient) *Accounts { return &Accounts{c: c} }

func (a *Accounts) Customer(ctx context.Context, id uuid.UUID) (string, error) {
	resp, err := a.c.GetAccount(ctx, &accountv1.GetAccountRequest{Id: id.String()})
	if status.Code(err) == codes.NotFound {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return resp.GetAccount().GetCustomerId(), nil
}
