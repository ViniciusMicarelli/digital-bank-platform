// Package grpcapi exposes the credit service over gRPC.
package grpcapi

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/domain"
)

type Server struct {
	creditv1.UnimplementedCreditServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

func parseID(v string) (uuid.UUID, error) {
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "ids must be UUIDs")
	}
	return id, nil
}

func brl(minor int64) *commonv1.Money { return &commonv1.Money{MinorUnits: minor, Currency: "BRL"} }

func amount(m *commonv1.Money) (int64, error) {
	if m.GetCurrency() != "BRL" || m.GetMinorUnits() <= 0 {
		return 0, grpcserver.Error(codes.InvalidArgument, "INVALID_AMOUNT", "amount must be a positive BRL value")
	}
	return m.GetMinorUnits(), nil
}

func linePB(l domain.Line) *creditv1.CreditLine {
	return &creditv1.CreditLine{Id: l.ID.String(), AccountId: l.AccountID.String(), Status: string(l.Status),
		Limit: brl(l.Limit), Used: brl(l.Used), Available: brl(l.Available()), ClosingDay: int32(l.ClosingDay),
		InterestRateBp: int32(l.InterestRateBP), CreatedAt: timestamppb.New(l.CreatedAt)}
}

func invoicePB(i domain.Invoice) *creditv1.Invoice {
	return &creditv1.Invoice{Id: i.ID.String(), LineId: i.LineID.String(), Status: string(i.Status),
		ClosingDate: i.ClosingDate.Format(time.DateOnly), DueDate: i.DueDate.Format(time.DateOnly),
		Total: brl(i.Total), Paid: brl(i.Paid), Remaining: brl(max(i.Remaining(), 0))}
}

func holdPB(h domain.Hold) *creditv1.Hold {
	return &creditv1.Hold{Id: h.ID.String(), Status: string(h.Status), FailureReason: h.FailureReason,
		Amount: brl(h.Amount), Captured: brl(h.Captured), Installments: int32(max(h.Installments, 1))} //nolint:gosec // 1..12
}

func paymentPB(p domain.Payment) *creditv1.Payment {
	return &creditv1.Payment{Id: p.ID.String(), InvoiceId: p.InvoiceID.String(), Status: string(p.Status),
		FailureReason: p.FailureReason, Amount: brl(p.Amount), CreatedAt: timestamppb.New(p.CreatedAt)}
}

func (s *Server) OpenCreditLine(ctx context.Context, req *creditv1.OpenCreditLineRequest) (*creditv1.OpenCreditLineResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	l, err := s.app.OpenLine(ctx, req.GetIdempotencyKey(), account, int(req.GetClosingDay()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &creditv1.OpenCreditLineResponse{Line: linePB(l)}, nil
}

func (s *Server) GetCreditLine(ctx context.Context, req *creditv1.GetCreditLineRequest) (*creditv1.GetCreditLineResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	l, err := s.app.Line(ctx, account)
	if err != nil {
		return nil, toStatus(err)
	}
	out := linePB(l)
	upcoming, err := s.app.UpcomingInstallments(ctx, l)
	if err != nil {
		return nil, toStatus(err)
	}
	out.UpcomingInstallments = brl(upcoming)
	return &creditv1.GetCreditLineResponse{Line: out}, nil
}

func (s *Server) ListInstallmentPlans(ctx context.Context, req *creditv1.ListInstallmentPlansRequest) (*creditv1.ListInstallmentPlansResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	plans, err := s.app.InstallmentPlans(ctx, account)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &creditv1.ListInstallmentPlansResponse{}
	for _, p := range plans {
		out.Plans = append(out.Plans, &creditv1.InstallmentPlan{HoldId: p.HoldID.String(), Description: p.Description, Total: brl(p.Total),
			Installments: int32(p.Count), Billed: int32(p.Billed), InstallmentAmount: brl(p.Installment), //nolint:gosec // 1..12
			Remaining: brl(p.Remaining), PurchasedAt: timestamppb.New(p.PurchasedAt)})
	}
	return out, nil
}

func (s *Server) ListInvoices(ctx context.Context, req *creditv1.ListInvoicesRequest) (*creditv1.ListInvoicesResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	list, err := s.app.Invoices(ctx, account)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &creditv1.ListInvoicesResponse{}
	for _, i := range list {
		resp.Invoices = append(resp.Invoices, invoicePB(i))
	}
	return resp, nil
}

func (s *Server) GetInvoice(ctx context.Context, req *creditv1.GetInvoiceRequest) (*creditv1.GetInvoiceResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	d, err := s.app.Invoice(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &creditv1.GetInvoiceResponse{Invoice: invoicePB(d.Invoice), AccountId: d.AccountID.String()}
	for _, it := range d.Items {
		resp.Items = append(resp.Items, &creditv1.InvoiceItem{Id: it.ID.String(), Kind: string(it.Kind), Amount: brl(it.Amount),
			Description: it.Description, OccurredAt: timestamppb.New(it.OccurredAt)})
	}
	return resp, nil
}

func (s *Server) PayInvoice(ctx context.Context, req *creditv1.PayInvoiceRequest) (*creditv1.PayInvoiceResponse, error) {
	id, err := parseID(req.GetInvoiceId())
	if err != nil {
		return nil, err
	}
	amt, err := amount(req.GetAmount())
	if err != nil {
		return nil, err
	}
	p, err := s.app.Pay(ctx, req.GetIdempotencyKey(), id, amt)
	if err != nil {
		return nil, toStatus(err)
	}
	return &creditv1.PayInvoiceResponse{Payment: paymentPB(p)}, nil
}

func (s *Server) ReserveCredit(ctx context.Context, req *creditv1.ReserveCreditRequest) (*creditv1.ReserveCreditResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	amt, err := amount(req.GetAmount())
	if err != nil {
		return nil, err
	}
	h, err := s.app.Reserve(ctx, req.GetIdempotencyKey(), account, amt, int(req.GetInstallments()), req.GetDescription(), req.GetMetadata())
	if err != nil {
		return nil, toStatus(err)
	}
	return &creditv1.ReserveCreditResponse{Hold: holdPB(h)}, nil
}

func (s *Server) CaptureCredit(ctx context.Context, req *creditv1.CaptureCreditRequest) (*creditv1.CaptureCreditResponse, error) {
	id, err := parseID(req.GetHoldId())
	if err != nil {
		return nil, err
	}
	var amt *int64
	if req.GetAmount() != nil {
		v, err := amount(req.GetAmount())
		if err != nil {
			return nil, err
		}
		amt = &v
	}
	h, err := s.app.Capture(ctx, id, amt)
	if err != nil {
		return nil, toStatus(err)
	}
	return &creditv1.CaptureCreditResponse{Hold: holdPB(h)}, nil
}

func (s *Server) RefundCredit(ctx context.Context, req *creditv1.RefundCreditRequest) (*creditv1.RefundCreditResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	amt, err := amount(req.GetAmount())
	if err != nil {
		return nil, err
	}
	r, err := s.app.Refund(ctx, req.GetIdempotencyKey(), account, amt, req.GetDescription(), req.GetMetadata())
	if err != nil {
		return nil, toStatus(err)
	}
	return &creditv1.RefundCreditResponse{Refund: &creditv1.CreditRefund{Id: r.ID.String(), AccountId: r.AccountID.String(),
		InvoiceId: r.InvoiceID.String(), Amount: brl(r.Amount), Status: "COMPLETED"}}, nil
}

func (s *Server) ReleaseCredit(ctx context.Context, req *creditv1.ReleaseCreditRequest) (*creditv1.ReleaseCreditResponse, error) {
	id, err := parseID(req.GetHoldId())
	if err != nil {
		return nil, err
	}
	h, err := s.app.Release(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &creditv1.ReleaseCreditResponse{Hold: holdPB(h)}, nil
}

func (s *Server) AdvanceCycle(ctx context.Context, req *creditv1.AdvanceCycleRequest) (*creditv1.AdvanceCycleResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	if err := s.app.AdvanceCycle(ctx, account, req.GetAction()); err != nil {
		return nil, toStatus(err)
	}
	return &creditv1.AdvanceCycleResponse{}, nil
}

func toStatus(err error) error {
	var refused *domain.RefusedError
	switch {
	case errors.As(err, &refused):
		return grpcserver.Error(codes.FailedPrecondition, refused.Reason, refused.Msg)
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrConflict):
		return grpcserver.Error(codes.AlreadyExists, "IDEMPOTENCY_CONFLICT", err.Error())
	case errors.Is(err, domain.ErrAccountInactive):
		return grpcserver.Error(codes.FailedPrecondition, "ACCOUNT_NOT_ACTIVE", err.Error())
	case errors.Is(err, domain.ErrInvalidState):
		return grpcserver.Error(codes.FailedPrecondition, "INVALID_STATE", err.Error())
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "downstream unavailable: "+st.Message())
	}
	return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "try again with the same idempotency key")
}
