// Package grpcapi exposes the payment service over gRPC.
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
	paymentv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/payment/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/domain"
)

type Server struct {
	paymentv1.UnimplementedPaymentServiceServer
	app    *application.Service
	issuer application.SandboxIssuer
}

func NewServer(app *application.Service, issuer application.SandboxIssuer) *Server {
	return &Server{app: app, issuer: issuer}
}

func brl(v int64) *commonv1.Money { return &commonv1.Money{MinorUnits: v, Currency: "BRL"} }

func dateString(d time.Time) string {
	if d.IsZero() {
		return ""
	}
	return d.Format(time.DateOnly)
}

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return d, grpcserver.Error(codes.InvalidArgument, "INVALID_DATE", "dates are YYYY-MM-DD")
	}
	return d, nil
}

func parseID(v string) (uuid.UUID, error) {
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "ids must be UUIDs")
	}
	return id, nil
}

func payeePB(p domain.Payee) *paymentv1.Payee {
	return &paymentv1.Payee{Name: p.Name, DocumentMasked: p.DocumentMasked, BankName: p.BankName}
}

func paymentPB(p domain.Payment) *paymentv1.BoletoPayment {
	return &paymentv1.BoletoPayment{Id: p.ID.String(), AccountId: p.AccountID.String(), Status: string(p.Status),
		FailureReason: p.FailureReason, Line: p.Line, Payee: payeePB(p.Payee), DueDate: dateString(p.DueDate),
		Nominal: brl(p.Nominal), Fine: brl(p.Fine), Interest: brl(p.Interest), Amount: brl(p.Amount()),
		ScheduledFor: dateString(p.ScheduledFor), CreatedAt: timestamppb.New(p.CreatedAt)}
}

func (s *Server) LookupBoleto(ctx context.Context, req *paymentv1.LookupBoletoRequest) (*paymentv1.LookupBoletoResponse, error) {
	q, err := s.app.Lookup(ctx, req.GetLine())
	if err != nil {
		return nil, toStatus(err)
	}
	return &paymentv1.LookupBoletoResponse{Quote: &paymentv1.BoletoQuote{Line: q.Boleto.Line, FormattedLine: domain.FormatLine(q.Boleto.Line),
		Barcode: q.Boleto.Barcode, Payee: payeePB(q.Payee), DueDate: dateString(q.Boleto.DueDate), Nominal: brl(q.Nominal),
		Fine: brl(q.Fine), Interest: brl(q.Interest), Total: brl(q.Total())}}, nil
}

func (s *Server) PayBoleto(ctx context.Context, req *paymentv1.PayBoletoRequest) (*paymentv1.PayBoletoResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	on, err := parseDate(req.GetScheduledFor())
	if err != nil {
		return nil, err
	}
	if req.GetAmount() != nil && req.GetAmount().GetCurrency() != "BRL" {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_AMOUNT", "amount must be BRL")
	}
	p, err := s.app.Pay(ctx, req.GetIdempotencyKey(), account, req.GetLine(), req.GetAmount().GetMinorUnits(), on)
	if err != nil {
		return nil, toStatus(err)
	}
	return &paymentv1.PayBoletoResponse{Payment: paymentPB(p)}, nil
}

func (s *Server) GetBoletoPayment(ctx context.Context, req *paymentv1.GetBoletoPaymentRequest) (*paymentv1.GetBoletoPaymentResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	p, err := s.app.Payment(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &paymentv1.GetBoletoPaymentResponse{Payment: paymentPB(p)}, nil
}

func (s *Server) ListBoletoPayments(ctx context.Context, req *paymentv1.ListBoletoPaymentsRequest) (*paymentv1.ListBoletoPaymentsResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	list, err := s.app.Payments(ctx, account)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &paymentv1.ListBoletoPaymentsResponse{}
	for _, p := range list {
		resp.Payments = append(resp.Payments, paymentPB(p))
	}
	return resp, nil
}

func (s *Server) CancelBoletoPayment(ctx context.Context, req *paymentv1.CancelBoletoPaymentRequest) (*paymentv1.CancelBoletoPaymentResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	p, err := s.app.Cancel(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &paymentv1.CancelBoletoPaymentResponse{Payment: paymentPB(p)}, nil
}

func (s *Server) IssueSandboxBoleto(ctx context.Context, req *paymentv1.IssueSandboxBoletoRequest) (*paymentv1.IssueSandboxBoletoResponse, error) {
	due, err := parseDate(req.GetDueDate())
	if err != nil {
		return nil, err
	}
	b, err := application.IssueSandbox(ctx, s.issuer, req.GetAmount().GetMinorUnits(), due, req.GetPayeeName(), req.GetBehavior())
	if err != nil {
		return nil, toStatus(err)
	}
	return &paymentv1.IssueSandboxBoletoResponse{Line: b.Line, FormattedLine: domain.FormatLine(b.Line)}, nil
}

func toStatus(err error) error {
	if r, ok := domain.AsRefused(err); ok {
		return grpcserver.Error(codes.FailedPrecondition, r.Reason, r.Msg)
	}
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_BOLETO", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrConflict):
		return grpcserver.Error(codes.AlreadyExists, "IDEMPOTENCY_CONFLICT", err.Error())
	case errors.Is(err, domain.ErrInvalidState):
		return grpcserver.Error(codes.FailedPrecondition, "INVALID_STATE", err.Error())
	case errors.Is(err, domain.ErrBusy):
		return grpcserver.Error(codes.Aborted, "IN_PROGRESS", err.Error())
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "downstream unavailable: "+st.Message())
	}
	return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "try again with the same idempotency key")
}
