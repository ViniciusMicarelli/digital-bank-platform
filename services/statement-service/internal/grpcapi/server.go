// Package grpcapi serves the feed over gRPC (bank.statement.v1).
package grpcapi

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/internal/projection"
)

type Server struct {
	statementv1.UnimplementedStatementServiceServer
	p *projection.Projector
}

func NewServer(p *projection.Projector) *Server { return &Server{p: p} }

func (s *Server) ListFeed(ctx context.Context, req *statementv1.ListFeedRequest) (*statementv1.ListFeedResponse, error) {
	account, err := uuid.Parse(req.GetAccountId())
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "account_id must be a UUID")
	}
	size := int(req.GetPageSize())
	if size <= 0 {
		size = 20
	}
	var from, to time.Time
	if req.GetFrom() != nil {
		from = req.GetFrom().AsTime()
	}
	if req.GetTo() != nil {
		to = req.GetTo().AsTime()
	}
	items, next, err := s.p.FeedBetween(ctx, account, from, to, req.GetPageToken(), min(size, 100))
	if errors.Is(err, projection.ErrInvalidCursor) {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	}
	if err != nil {
		return nil, grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
	}
	resp := &statementv1.ListFeedResponse{NextPageToken: next}
	for _, it := range items {
		resp.Items = append(resp.Items, itemPB(it))
	}
	return resp, nil
}

func itemPB(it projection.Item) *statementv1.FeedItem {
	dir := statementv1.Direction_DIRECTION_DEBIT
	if it.Direction == "CREDIT" {
		dir = statementv1.Direction_DIRECTION_CREDIT
	}
	return &statementv1.FeedItem{
		Id: it.ID.String(), AccountId: it.AccountID.String(), TransactionId: it.TransactionID.String(),
		Type: it.Type, Direction: dir,
		Amount:           &commonv1.Money{MinorUnits: it.AmountMinor, Currency: it.Currency},
		CounterpartyName: it.CounterpartyName, Description: it.Description,
		EndToEndId: it.EndToEndID, OccurredAt: timestamppb.New(it.OccurredAt),
	}
}

func receiptPB(r projection.Receipt) *statementv1.Receipt {
	return &statementv1.Receipt{Item: itemPB(r.Item), HolderName: r.HolderName, Branch: r.Branch, Number: r.Number,
		CounterpartyBranch: r.CounterpartyBranch, CounterpartyNumber: r.CounterpartyNumber, AuthenticationCode: r.Code}
}

func (s *Server) GetReceipt(ctx context.Context, req *statementv1.GetReceiptRequest) (*statementv1.GetReceiptResponse, error) {
	account, err1 := uuid.Parse(req.GetAccountId())
	item, err2 := uuid.Parse(req.GetItemId())
	if err1 != nil || err2 != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "ids must be UUIDs")
	}
	r, err := s.p.Receipt(ctx, account, item)
	if errors.Is(err, projection.ErrNotFound) {
		return nil, grpcserver.Error(codes.NotFound, "NOT_FOUND", "no such item")
	}
	if err != nil {
		return nil, grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
	}
	return &statementv1.GetReceiptResponse{Receipt: receiptPB(r)}, nil
}

// VerifyReceipt answers who paid and who received with names masked: the
// code may be checked by anyone holding the receipt.
func (s *Server) VerifyReceipt(ctx context.Context, req *statementv1.VerifyReceiptRequest) (*statementv1.VerifyReceiptResponse, error) {
	r, err := s.p.Verify(ctx, req.GetAuthenticationCode())
	if errors.Is(err, projection.ErrNotFound) {
		return nil, grpcserver.Error(codes.NotFound, "RECEIPT_NOT_FOUND", "this code does not match a receipt of this bank")
	}
	if err != nil {
		return nil, grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
	}
	r.HolderName, r.CounterpartyName = projection.MaskName(r.HolderName), projection.MaskName(r.CounterpartyName)
	r.Description, r.Number, r.CounterpartyNumber = "", "", ""
	return &statementv1.VerifyReceiptResponse{Receipt: receiptPB(r)}, nil
}

func brl(minor int64) *commonv1.Money { return &commonv1.Money{MinorUnits: minor, Currency: "BRL"} }

func (s *Server) GetSpending(ctx context.Context, req *statementv1.GetSpendingRequest) (*statementv1.GetSpendingResponse, error) {
	account, err := uuid.Parse(req.GetAccountId())
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "account_id must be a UUID")
	}
	start, err := projection.ParseMonth(req.GetMonth(), time.Now())
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_MONTH", err.Error())
	}
	sp, err := s.p.Spending(ctx, account, start)
	if err != nil {
		return nil, grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
	}
	out := &statementv1.GetSpendingResponse{Month: sp.Month, Total: brl(sp.Total), PreviousTotal: brl(sp.PreviousTotal)}
	for _, c := range sp.Categories {
		out.Categories = append(out.Categories, &statementv1.CategorySpending{Category: c.Category, Amount: brl(c.Amount), Count: int32(c.Count)}) //nolint:gosec // counts
	}
	for _, m := range sp.Merchants {
		out.Merchants = append(out.Merchants, &statementv1.MerchantSpending{Name: m.Name, Category: m.Category, Amount: brl(m.Amount), Count: int32(m.Count)}) //nolint:gosec // counts
	}
	return out, nil
}
