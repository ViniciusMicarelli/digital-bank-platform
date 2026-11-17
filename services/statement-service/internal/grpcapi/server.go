// Package grpcapi serves the feed over gRPC (bank.statement.v1).
package grpcapi

import (
	"context"
	"errors"

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
