// Package grpcapi exposes the saga orchestrator over gRPC (bank.transaction.v1).
package grpcapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/domain"
)

type Server struct {
	txv1.UnimplementedTransactionServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

var types = map[txv1.TransactionType]domain.Type{
	txv1.TransactionType_TRANSACTION_TYPE_INTERNAL_TRANSFER:      domain.TypeInternalTransfer,
	txv1.TransactionType_TRANSACTION_TYPE_PIX_OUT:                domain.TypePixOut,
	txv1.TransactionType_TRANSACTION_TYPE_PIX_IN:                 domain.TypePixIn,
	txv1.TransactionType_TRANSACTION_TYPE_DEBIT_CARD_PURCHASE:    domain.TypeDebitCard,
	txv1.TransactionType_TRANSACTION_TYPE_CREDIT_CARD_PURCHASE:   domain.TypeCreditPurchase,
	txv1.TransactionType_TRANSACTION_TYPE_CREDIT_INVOICE_PAYMENT: domain.TypeCreditInvoicePayment,
	txv1.TransactionType_TRANSACTION_TYPE_CREDIT_CHARGES:         domain.TypeCreditCharges,
	txv1.TransactionType_TRANSACTION_TYPE_BOLETO_PAYMENT:         domain.TypeBoletoPayment,
	txv1.TransactionType_TRANSACTION_TYPE_CARD_REFUND:            domain.TypeCardRefund,
	txv1.TransactionType_TRANSACTION_TYPE_CREDIT_CARD_REFUND:     domain.TypeCreditCardRefund,
}

func (s *Server) CreateTransfer(ctx context.Context, req *txv1.CreateTransferRequest) (*txv1.CreateTransferResponse, error) {
	from, to, amount, err := parse(req.GetFromAccountId(), req.GetToAccountId(), req.GetAmount())
	if err != nil {
		return nil, err
	}
	typ := domain.TypeInternalTransfer
	if req.GetType() != txv1.TransactionType_TRANSACTION_TYPE_UNSPECIFIED {
		typ = types[req.GetType()]
	}
	t, err := s.app.Post(ctx, req.GetIdempotencyKey(), typ, from, to, amount, req.GetDescription(), req.GetMetadata())
	return respond(t, err, func(p *txv1.Transaction) *txv1.CreateTransferResponse {
		return &txv1.CreateTransferResponse{Transaction: p}
	})
}

func (s *Server) ReserveOutgoing(ctx context.Context, req *txv1.ReserveOutgoingRequest) (*txv1.ReserveOutgoingResponse, error) {
	from, to, amount, err := parse(req.GetFromAccountId(), req.GetToAccountId(), req.GetAmount())
	if err != nil {
		return nil, err
	}
	t, err := s.app.ReserveOutgoing(ctx, req.GetIdempotencyKey(), types[req.GetType()], from, to, amount, req.GetDescription(), req.GetMetadata())
	return respond(t, err, func(p *txv1.Transaction) *txv1.ReserveOutgoingResponse {
		return &txv1.ReserveOutgoingResponse{Transaction: p}
	})
}

func (s *Server) CompleteTransaction(ctx context.Context, req *txv1.CompleteTransactionRequest) (*txv1.CompleteTransactionResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	var amount *money.Money
	if req.GetAmount() != nil {
		m, err := money.New(req.GetAmount().GetMinorUnits(), money.Currency(req.GetAmount().GetCurrency()))
		if err != nil {
			return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
		}
		amount = &m
	}
	t, err := s.app.Complete(ctx, id, amount)
	return respond(t, err, func(p *txv1.Transaction) *txv1.CompleteTransactionResponse {
		return &txv1.CompleteTransactionResponse{Transaction: p}
	})
}

func (s *Server) CompensateTransaction(ctx context.Context, req *txv1.CompensateTransactionRequest) (*txv1.CompensateTransactionResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	t, err := s.app.Compensate(ctx, id, req.GetReason())
	return respond(t, err, func(p *txv1.Transaction) *txv1.CompensateTransactionResponse {
		return &txv1.CompensateTransactionResponse{Transaction: p}
	})
}

func (s *Server) CreditIncoming(ctx context.Context, req *txv1.CreditIncomingRequest) (*txv1.CreditIncomingResponse, error) {
	from, to, amount, err := parse(req.GetFromAccountId(), req.GetToAccountId(), req.GetAmount())
	if err != nil {
		return nil, err
	}
	t, err := s.app.CreditIncoming(ctx, req.GetIdempotencyKey(), types[req.GetType()], from, to, amount, req.GetDescription(), req.GetMetadata())
	return respond(t, err, func(p *txv1.Transaction) *txv1.CreditIncomingResponse {
		return &txv1.CreditIncomingResponse{Transaction: p}
	})
}

func (s *Server) GetTransaction(ctx context.Context, req *txv1.GetTransactionRequest) (*txv1.GetTransactionResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	t, err := s.app.Get(ctx, id)
	return respond(t, err, func(p *txv1.Transaction) *txv1.GetTransactionResponse {
		return &txv1.GetTransactionResponse{Transaction: p}
	})
}

// respond returns the transaction even when the saga ended FAILED: a business
// refusal is a valid outcome, visible in status and failure_reason.
func respond[R any](t domain.Transaction, err error, wrap func(*txv1.Transaction) R) (R, error) {
	var zero R
	if err != nil {
		return zero, toStatus(err)
	}
	return wrap(transactionPB(t)), nil
}

func parse(fromID, toID string, m *commonv1.Money) (uuid.UUID, uuid.UUID, money.Money, error) {
	from, err := parseID(fromID)
	if err != nil {
		return uuid.Nil, uuid.Nil, money.Money{}, err
	}
	to, err := parseID(toID)
	if err != nil {
		return uuid.Nil, uuid.Nil, money.Money{}, err
	}
	amount, err := money.New(m.GetMinorUnits(), money.Currency(m.GetCurrency()))
	if err != nil {
		return uuid.Nil, uuid.Nil, money.Money{}, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	}
	return from, to, amount, nil
}

func parseID(v string) (uuid.UUID, error) {
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "ids must be UUIDs")
	}
	return id, nil
}

func transactionPB(t domain.Transaction) *txv1.Transaction {
	out := &txv1.Transaction{
		Id: t.ID.String(), IdempotencyKey: t.IdempotencyKey,
		FromAccountId: t.From.String(), ToAccountId: t.To.String(),
		Amount:      &commonv1.Money{MinorUnits: t.Amount.Minor(), Currency: string(t.Amount.Currency())},
		Description: t.Description, Metadata: t.Metadata, FailureReason: t.FailureReason,
		CreatedAt: timestamppb.New(t.CreatedAt), UpdatedAt: timestamppb.New(t.UpdatedAt),
		Status: map[domain.Status]txv1.TransactionStatus{
			domain.StatusPending:      txv1.TransactionStatus_TRANSACTION_STATUS_PENDING,
			domain.StatusReserved:     txv1.TransactionStatus_TRANSACTION_STATUS_RESERVED,
			domain.StatusCompleted:    txv1.TransactionStatus_TRANSACTION_STATUS_COMPLETED,
			domain.StatusCompensating: txv1.TransactionStatus_TRANSACTION_STATUS_COMPENSATING,
			domain.StatusFailed:       txv1.TransactionStatus_TRANSACTION_STATUS_FAILED,
		}[t.Status],
	}
	for k, v := range types {
		if v == t.Type {
			out.Type = k
		}
	}
	if t.EntryID != uuid.Nil {
		out.LedgerEntryId = t.EntryID.String()
	}
	return out
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrInvalidState):
		return grpcserver.Error(codes.FailedPrecondition, "INVALID_STATE", err.Error())
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return grpcserver.Error(codes.AlreadyExists, "IDEMPOTENCY_CONFLICT", err.Error())
	case errors.Is(err, domain.ErrBusy):
		return grpcserver.Error(codes.Aborted, "IN_PROGRESS", err.Error())
	}
	// A downstream technical failure: the saga is saved and will be resumed;
	// the client may retry with the same idempotency key.
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "downstream unavailable: "+st.Message())
	}
	return grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
}
