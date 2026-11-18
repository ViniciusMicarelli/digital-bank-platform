// Package users adapts user-service to the account ports.
package users

import (
	"context"

	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/ports"
)

type Client struct{ c userv1.UserServiceClient }

func New(c userv1.UserServiceClient) *Client { return &Client{c: c} }

func (u *Client) Identity(ctx context.Context, customer string) (ports.Identity, error) {
	resp, err := u.c.GetIdentity(ctx, &userv1.GetIdentityRequest{CustomerId: customer})
	if err != nil {
		return ports.Identity{}, err
	}
	return ports.Identity{KYCStatus: resp.GetKycStatus(), FullName: resp.GetFullName(), CPF: resp.GetCpf()}, nil
}
