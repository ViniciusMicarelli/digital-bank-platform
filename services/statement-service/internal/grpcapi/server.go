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
	items, next, err := s.p.Feed(ctx, account, req.GetPageToken(), min(size, 100))
	if errors.Is(err, projection.ErrInvalidCursor) {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	}
	if err != nil {
		return nil, grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
	}
	resp := &statementv1.ListFeedResponse{NextPageToken: next}
	for _, it := range items {
		dir := statementv1.Direction_DIRECTION_DEBIT
		if it.Direction == "CREDIT" {
			dir = statementv1.Direction_DIRECTION_CREDIT
		}
		resp.Items = append(resp.Items, &statementv1.FeedItem{
			Id: it.ID.String(), AccountId: it.AccountID.String(), TransactionId: it.TransactionID.String(),
			Type: it.Type, Direction: dir,
			Amount:           &commonv1.Money{MinorUnits: it.AmountMinor, Currency: it.Currency},
			CounterpartyName: it.CounterpartyName, Description: it.Description,
			EndToEndId: it.EndToEndID, OccurredAt: timestamppb.New(it.OccurredAt),
		})
	}
	return resp, nil
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
