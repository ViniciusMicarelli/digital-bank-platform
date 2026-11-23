// Package actions runs approved four-eyes actions: unblocking accounts
// (account-service), large limit increases (credit-service) and manual
// credits (transaction-service).
package actions

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/ports"
)

type Executor struct {
	c      accountv1.AccountServiceClient
	credit creditv1.CreditServiceClient
	txs    txv1.TransactionServiceClient
}

func New(c accountv1.AccountServiceClient, credit creditv1.CreditServiceClient, txs txv1.TransactionServiceClient) *Executor {
	return &Executor{c: c, credit: credit, txs: txs}
}

// ManualAdjustmentsAccountID is the ledger's internal account that pays
// manual credits.
const ManualAdjustmentsAccountID = "00000000-0000-0000-0000-000000000008"

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
	case domain.KindLimitIncrease:
		return e.grantLimit(ctx, a)
	case domain.KindManualCredit:
		return e.manualCredit(ctx, a)
	}
	return domain.Refuse("UNKNOWN_ACTION", string(a.Kind))
}

// manualCredit posts the manual credit, idempotent on the approval: a retry
// returns the same transaction. A refusal (e.g. the account is not active)
// fails the approval; a transaction still running is retried.
func (e *Executor) manualCredit(ctx context.Context, a domain.Approval) error {
	amount, err := strconv.ParseInt(a.Params["amount_minor"], 10, 64)
	if err != nil || amount <= 0 {
		return domain.Refuse("INVALID_PARAMS", "amount_minor must be a positive amount in centavos")
	}
	resp, err := e.txs.CreateTransfer(ctx, &txv1.CreateTransferRequest{IdempotencyKey: "manual-credit:" + a.ID.String(),
		Type: txv1.TransactionType_TRANSACTION_TYPE_MANUAL_CREDIT, FromAccountId: ManualAdjustmentsAccountID, ToAccountId: a.TargetID,
		Amount: &commonv1.Money{MinorUnits: amount, Currency: "BRL"}, Description: "Crédito do Digital Bank: " + a.Reason,
		Metadata: map[string]string{"approval_id": a.ID.String(), "counterparty_name": "Digital Bank", "requested_by": a.RequestedBy,
			"approved_by": a.DecidedBy}})
	if err != nil {
		return permanent(err)
	}
	switch t := resp.GetTransaction(); t.GetStatus() {
	case txv1.TransactionStatus_TRANSACTION_STATUS_COMPLETED:
		return nil
	case txv1.TransactionStatus_TRANSACTION_STATUS_FAILED:
		return domain.Refuse(t.GetFailureReason(), "the credit was refused")
	}
	return fmt.Errorf("manual credit %s still running", a.ID)
}

// grantLimit decides the limit request in credit-service with both
// analysts: the one who asked and the one who approved. Idempotent: a
// request already granted with this approval counts as done.
func (e *Executor) grantLimit(ctx context.Context, a domain.Approval) error {
	granted, err := strconv.ParseInt(a.Params["granted_minor"], 10, 64)
	if err != nil || granted <= 0 {
		return domain.Refuse("INVALID_PARAMS", "granted_minor must be a positive amount in centavos")
	}
	list, err := e.credit.ListLimitRequests(ctx, &creditv1.ListLimitRequestsRequest{AccountId: a.Params["account_id"]})
	if err != nil {
		return permanent(err)
	}
	for _, r := range list.GetRequests() {
		if r.GetId() == a.TargetID && r.GetStatus() == "APPROVED" && r.GetSecondApprover() == a.DecidedBy {
			return nil // a retry after it went through
		}
	}
	_, err = e.credit.DecideLimitRequest(ctx, &creditv1.DecideLimitRequestRequest{Id: a.TargetID, AnalystId: a.RequestedBy,
		SecondApproverId: a.DecidedBy, Approve: true, Granted: &commonv1.Money{MinorUnits: granted, Currency: "BRL"}, Reason: a.Reason})
	return permanent(err)
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
