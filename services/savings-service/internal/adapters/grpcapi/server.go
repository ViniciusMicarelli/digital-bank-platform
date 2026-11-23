// Package grpcapi exposes the savings service over gRPC.
package grpcapi

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	savingsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/savings/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/domain"
)

type Server struct {
	savingsv1.UnimplementedSavingsServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

func brl(minor int64) *commonv1.Money { return &commonv1.Money{MinorUnits: minor, Currency: "BRL"} }

func (s *Server) boxPB(v application.View) *savingsv1.Box {
	var sched *savingsv1.Schedule
	if v.Schedule.Active() {
		sched = &savingsv1.Schedule{Amount: brl(v.Schedule.Amount), Day: int32(v.Schedule.Day), //nolint:gosec // 1..28
			NextRun: v.Schedule.Next.Format(time.DateOnly), LastFailure: v.Schedule.LastFailure}
	}
	return &savingsv1.Box{Schedule: sched, Id: v.ID.String(), AccountId: v.AccountID.String(), Name: v.Name, Goal: brl(v.Goal),
		Balance: brl(v.Balance), Yielded: brl(v.Yielded), Status: string(v.Status), CreatedAt: timestamppb.New(v.CreatedAt),
		AnnualRateBp: int32(s.app.AnnualRateBP())} //nolint:gosec // basis points
}

func movementPB(m domain.Movement) *savingsv1.Movement {
	return &savingsv1.Movement{Id: m.ID.String(), BoxId: m.BoxID.String(), Kind: string(m.Kind), Amount: brl(m.Amount),
		Status: string(m.Status), FailureReason: m.FailureReason, CreatedAt: timestamppb.New(m.CreatedAt)}
}

func parseID(v string) (uuid.UUID, error) {
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "ids must be UUIDs")
	}
	return id, nil
}

func amount(m *commonv1.Money, zeroOK bool) (int64, error) {
	if m == nil && zeroOK {
		return 0, nil
	}
	if m.GetCurrency() != "BRL" || m.GetMinorUnits() < 0 || (m.GetMinorUnits() == 0 && !zeroOK) {
		return 0, grpcserver.Error(codes.InvalidArgument, "INVALID_AMOUNT", "amount must be a positive BRL value")
	}
	return m.GetMinorUnits(), nil
}

func toStatus(err error) error {
	if r, ok := domain.AsRefused(err); ok {
		return grpcserver.Error(codes.FailedPrecondition, r.Reason, r.Msg)
	}
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "BOX_NOT_FOUND", "no such box")
	case errors.Is(err, domain.ErrConflict):
		return grpcserver.Error(codes.AlreadyExists, "IDEMPOTENCY_CONFLICT", err.Error())
	}
	return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "try again")
}

func (s *Server) CreateBox(ctx context.Context, req *savingsv1.CreateBoxRequest) (*savingsv1.CreateBoxResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	goal, err := amount(req.GetGoal(), true)
	if err != nil {
		return nil, err
	}
	v, err := s.app.Create(ctx, req.GetIdempotencyKey(), account, req.GetName(), goal)
	if err != nil {
		return nil, toStatus(err)
	}
	return &savingsv1.CreateBoxResponse{Box: s.boxPB(v)}, nil
}

func (s *Server) GetBox(ctx context.Context, req *savingsv1.GetBoxRequest) (*savingsv1.GetBoxResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	v, err := s.app.Box(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &savingsv1.GetBoxResponse{Box: s.boxPB(v)}, nil
}

func (s *Server) ListBoxes(ctx context.Context, req *savingsv1.ListBoxesRequest) (*savingsv1.ListBoxesResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	list, err := s.app.Boxes(ctx, account)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &savingsv1.ListBoxesResponse{}
	for _, v := range list {
		out.Boxes = append(out.Boxes, s.boxPB(v))
	}
	return out, nil
}

func (s *Server) UpdateBox(ctx context.Context, req *savingsv1.UpdateBoxRequest) (*savingsv1.UpdateBoxResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	goal, err := amount(req.GetGoal(), true)
	if err != nil {
		return nil, err
	}
	v, err := s.app.Update(ctx, id, req.GetName(), goal)
	if err != nil {
		return nil, toStatus(err)
	}
	return &savingsv1.UpdateBoxResponse{Box: s.boxPB(v)}, nil
}

func (s *Server) CloseBox(ctx context.Context, req *savingsv1.CloseBoxRequest) (*savingsv1.CloseBoxResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	v, err := s.app.Close(ctx, req.GetIdempotencyKey(), id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &savingsv1.CloseBoxResponse{Box: s.boxPB(v)}, nil
}

func (s *Server) move(ctx context.Context, key, box string, m *commonv1.Money, deposit bool) (*savingsv1.Movement, *savingsv1.Box, error) {
	id, err := parseID(box)
	if err != nil {
		return nil, nil, err
	}
	amt, err := amount(m, false)
	if err != nil {
		return nil, nil, err
	}
	run := s.app.Withdraw
	if deposit {
		run = s.app.Deposit
	}
	mv, v, err := run(ctx, key, id, amt)
	if err != nil {
		return nil, nil, toStatus(err)
	}
	return movementPB(mv), s.boxPB(v), nil
}

func (s *Server) Deposit(ctx context.Context, req *savingsv1.DepositRequest) (*savingsv1.DepositResponse, error) {
	m, b, err := s.move(ctx, req.GetIdempotencyKey(), req.GetBoxId(), req.GetAmount(), true)
	if err != nil {
		return nil, err
	}
	return &savingsv1.DepositResponse{Movement: m, Box: b}, nil
}

func (s *Server) Withdraw(ctx context.Context, req *savingsv1.WithdrawRequest) (*savingsv1.WithdrawResponse, error) {
	m, b, err := s.move(ctx, req.GetIdempotencyKey(), req.GetBoxId(), req.GetAmount(), false)
	if err != nil {
		return nil, err
	}
	return &savingsv1.WithdrawResponse{Movement: m, Box: b}, nil
}

func (s *Server) ListMovements(ctx context.Context, req *savingsv1.ListMovementsRequest) (*savingsv1.ListMovementsResponse, error) {
	id, err := parseID(req.GetBoxId())
	if err != nil {
		return nil, err
	}
	list, err := s.app.Movements(ctx, id, int(req.GetLimit()))
	if err != nil {
		return nil, toStatus(err)
	}
	out := &savingsv1.ListMovementsResponse{}
	for _, m := range list {
		out.Movements = append(out.Movements, movementPB(m))
	}
	return out, nil
}

func (s *Server) SimulateYield(ctx context.Context, req *savingsv1.SimulateYieldRequest) (*savingsv1.SimulateYieldResponse, error) {
	id, err := parseID(req.GetBoxId())
	if err != nil {
		return nil, err
	}
	m, v, err := s.app.SimulateYield(ctx, id, int(req.GetDays()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &savingsv1.SimulateYieldResponse{Movement: movementPB(m), Box: s.boxPB(v)}, nil
}

func (s *Server) SetSchedule(ctx context.Context, req *savingsv1.SetScheduleRequest) (*savingsv1.SetScheduleResponse, error) {
	id, err := parseID(req.GetBoxId())
	if err != nil {
		return nil, err
	}
	amt, err := amount(req.GetAmount(), true)
	if err != nil {
		return nil, err
	}
	v, err := s.app.SetSchedule(ctx, id, amt, int(req.GetDay()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &savingsv1.SetScheduleResponse{Box: s.boxPB(v)}, nil
}

func (s *Server) RunSchedule(ctx context.Context, req *savingsv1.RunScheduleRequest) (*savingsv1.RunScheduleResponse, error) {
	id, err := parseID(req.GetBoxId())
	if err != nil {
		return nil, err
	}
	m, v, err := s.app.RunSchedule(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &savingsv1.RunScheduleResponse{Movement: movementPB(m), Box: s.boxPB(v)}, nil
}

func (s *Server) GetYearReport(ctx context.Context, req *savingsv1.GetYearReportRequest) (*savingsv1.GetYearReportResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	if y := req.GetYear(); y < 2000 || y > 2100 {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_YEAR", "year must be between 2000 and 2100")
	}
	list, err := s.app.YearReport(ctx, account, int(req.GetYear()))
	if err != nil {
		return nil, toStatus(err)
	}
	out := &savingsv1.GetYearReportResponse{Year: req.GetYear()}
	for _, b := range list {
		out.Boxes = append(out.Boxes, &savingsv1.BoxYear{BoxId: b.Box.ID.String(), Name: b.Box.Name, Status: string(b.Box.Status),
			BalanceStart: brl(b.Start), BalanceEnd: brl(b.End), Yielded: brl(b.Yield)})
	}
	return out, nil
}
