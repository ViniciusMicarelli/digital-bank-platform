// Package accounts runs approved actions against account-service.
package accounts

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/ports"
)

type Executor struct {
	c accountv1.AccountServiceClient
}

func New(c accountv1.AccountServiceClient) *Executor { return &Executor{c: c} }

var _ ports.Executor = (*Executor)(nil)

// Execute is idempotent: an account that is already active counts as
// unblocked, so a retry after a crash does not fail the approval.
func (e *Executor) Execute(ctx context.Context, a domain.Approval) error {
	switch a.Kind {
	case domain.KindUnblockAccount:
		acc, err := e.c.GetAccount(ctx, &accountv1.GetAccountRequest{Id: a.TargetID})
		if err != nil {
			return permanent(err)
		}
		switch acc.GetAccount().GetStatus() {
		case accountv1.AccountStatus_ACCOUNT_STATUS_ACTIVE:
			return nil
		case accountv1.AccountStatus_ACCOUNT_STATUS_BLOCKED:
			_, err := e.c.UnblockAccount(ctx, &accountv1.UnblockAccountRequest{Id: a.TargetID})
			return permanent(err)
		}
		return domain.Refuse("ACCOUNT_NOT_BLOCKED", "the account is "+acc.GetAccount().GetStatus().String())
	}
	return domain.Refuse("UNKNOWN_ACTION", string(a.Kind))
}

// permanent turns the errors that a retry cannot fix into refusals.
func permanent(err error) error {
	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.NotFound, codes.InvalidArgument, codes.FailedPrecondition, codes.PermissionDenied:
		reason := grpcserver.Reason(err)
		if reason == "" {
			reason = status.Code(err).String()
		}
		return domain.Refuse(reason, status.Convert(err).Message())
	}
	return err
}
