// Package grpcapi exposes the account application over gRPC (bank.account.v1).
package grpcapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/domain"
)

// Error reasons callers can branch on.
const (
	ReasonLimitExceeded     = "LIMIT_EXCEEDED"
	ReasonAccountNotActive  = "ACCOUNT_NOT_ACTIVE"
	ReasonInvalidTransition = "INVALID_TRANSITION"
	ReasonBalanceNotZero    = "BALANCE_NOT_ZERO"
)

type Server struct {
	accountv1.UnimplementedAccountServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

func (s *Server) OpenAccount(ctx context.Context, req *accountv1.OpenAccountRequest) (*accountv1.OpenAccountResponse, error) {
	a, err := s.app.OpenAccount(ctx, req.GetIdempotencyKey(), req.GetCustomerId(), req.GetHolderName(),
		req.GetDocument(), money.Currency(req.GetCurrency()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.OpenAccountResponse{Account: accountPB(a)}, nil
}

func (s *Server) GetAccount(ctx context.Context, req *accountv1.GetAccountRequest) (*accountv1.GetAccountResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	a, err := s.app.Account(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.GetAccountResponse{Account: accountPB(a)}, nil
}

func (s *Server) ListAccounts(ctx context.Context, req *accountv1.ListAccountsRequest) (*accountv1.ListAccountsResponse, error) {
	list, err := s.app.AccountsByCustomer(ctx, req.GetCustomerId())
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &accountv1.ListAccountsResponse{}
	for _, a := range list {
		resp.Accounts = append(resp.Accounts, accountPB(a))
	}
	return resp, nil
}

func (s *Server) GetBalance(ctx context.Context, req *accountv1.GetBalanceRequest) (*accountv1.GetBalanceResponse, error) {
	id, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	b, err := s.app.Balance(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.GetBalanceResponse{Posted: moneyPB(b.Posted), Pending: moneyPB(b.Pending), Available: moneyPB(b.Available)}, nil
}

func (s *Server) BlockAccount(ctx context.Context, req *accountv1.BlockAccountRequest) (*accountv1.BlockAccountResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	a, err := s.app.Block(ctx, id, req.GetReason())
	if err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.BlockAccountResponse{Account: accountPB(a)}, nil
}

func (s *Server) UnblockAccount(ctx context.Context, req *accountv1.UnblockAccountRequest) (*accountv1.UnblockAccountResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	a, err := s.app.Unblock(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.UnblockAccountResponse{Account: accountPB(a)}, nil
}

func (s *Server) CloseAccount(ctx context.Context, req *accountv1.CloseAccountRequest) (*accountv1.CloseAccountResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	a, err := s.app.Close(ctx, id, req.GetReason())
	if err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.CloseAccountResponse{Account: accountPB(a)}, nil
}

func (s *Server) GetLimits(ctx context.Context, req *accountv1.GetLimitsRequest) (*accountv1.GetLimitsResponse, error) {
	id, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	l, u, err := s.app.Limits(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.GetLimitsResponse{Limits: limitsPB(l), Usage: usagePB(u)}, nil
}

func (s *Server) UpdateLimits(ctx context.Context, req *accountv1.UpdateLimitsRequest) (*accountv1.UpdateLimitsResponse, error) {
	id, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	in := req.GetLimits()
	l, err := s.app.UpdateLimits(ctx, id, domain.Limits{
		PerTransaction: in.GetPerTransactionMinorUnits(), Daily: in.GetDailyMinorUnits(), Nightly: in.GetNightlyMinorUnits(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.UpdateLimitsResponse{Limits: limitsPB(l)}, nil
}

func (s *Server) ConsumeLimit(ctx context.Context, req *accountv1.ConsumeLimitRequest) (*accountv1.ConsumeLimitResponse, error) {
	id, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	amount, err := money.New(req.GetAmount().GetMinorUnits(), money.Currency(req.GetAmount().GetCurrency()))
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	}
	u, err := s.app.ConsumeLimit(ctx, id, req.GetTransactionId(), amount)
	if err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.ConsumeLimitResponse{Usage: usagePB(u)}, nil
}

func (s *Server) RestoreLimit(ctx context.Context, req *accountv1.RestoreLimitRequest) (*accountv1.RestoreLimitResponse, error) {
	id, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	if err := s.app.RestoreLimit(ctx, id, req.GetTransactionId()); err != nil {
		return nil, toStatus(err)
	}
	return &accountv1.RestoreLimitResponse{}, nil
}

func parseID(v string) (uuid.UUID, error) {
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "id must be a UUID")
	}
	return id, nil
}

func moneyPB(m money.Money) *commonv1.Money {
	return &commonv1.Money{MinorUnits: m.Minor(), Currency: string(m.Currency())}
}

func limitsPB(l domain.Limits) *accountv1.Limits {
	return &accountv1.Limits{PerTransactionMinorUnits: l.PerTransaction, DailyMinorUnits: l.Daily, NightlyMinorUnits: l.Nightly}
}

func usagePB(u domain.Usage) *accountv1.LimitUsage {
	return &accountv1.LimitUsage{DailyUsedMinorUnits: u.Daily, NightlyUsedMinorUnits: u.Nightly}
}

func accountPB(a domain.Account) *accountv1.Account {
	out := &accountv1.Account{
		Id: a.ID.String(), CustomerId: a.CustomerID, HolderName: a.HolderName, Document: a.Document,
		Branch: a.Branch, Number: a.Number, Currency: string(a.Currency), StatusReason: a.StatusReason,
		Status: map[domain.Status]accountv1.AccountStatus{
			domain.StatusActive:  accountv1.AccountStatus_ACCOUNT_STATUS_ACTIVE,
			domain.StatusBlocked: accountv1.AccountStatus_ACCOUNT_STATUS_BLOCKED,
			domain.StatusClosed:  accountv1.AccountStatus_ACCOUNT_STATUS_CLOSED,
		}[a.Status],
		OpenedAt: timestamppb.New(a.OpenedAt),
	}
	if a.ClosedAt != nil {
		out.ClosedAt = timestamppb.New(*a.ClosedAt)
	}
	return out
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrLimitExceeded):
		return grpcserver.Error(codes.FailedPrecondition, ReasonLimitExceeded, err.Error())
	case errors.Is(err, domain.ErrNotActive):
		return grpcserver.Error(codes.FailedPrecondition, ReasonAccountNotActive, err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		return grpcserver.Error(codes.FailedPrecondition, ReasonInvalidTransition, err.Error())
	case errors.Is(err, domain.ErrBalanceNotZero):
		return grpcserver.Error(codes.FailedPrecondition, ReasonBalanceNotZero, err.Error())
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return grpcserver.Error(codes.AlreadyExists, "IDEMPOTENCY_CONFLICT", err.Error())
	}
	// Errors from downstream gRPC calls keep their code (e.g. ledger UNAVAILABLE).
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return st.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return grpcserver.Error(codes.DeadlineExceeded, "", err.Error())
	}
	return grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
}
