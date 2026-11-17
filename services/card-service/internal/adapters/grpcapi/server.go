// Package grpcapi exposes cards over gRPC (bank.card.v1).
package grpcapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	cardv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/card/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/domain"
)

type Server struct {
	cardv1.UnimplementedCardServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

var kinds = map[cardv1.CardKind]domain.Kind{cardv1.CardKind_CARD_KIND_PHYSICAL: domain.Physical, cardv1.CardKind_CARD_KIND_VIRTUAL: domain.Virtual}

var channels = map[cardv1.Channel]domain.Channel{
	cardv1.Channel_CHANNEL_ONLINE: domain.Online, cardv1.Channel_CHANNEL_CHIP: domain.Chip, cardv1.Channel_CHANNEL_CONTACTLESS: domain.Contactless,
}

func parseID(v string) (uuid.UUID, error) {
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "ids must be UUIDs")
	}
	return id, nil
}

func cardPB(c domain.Card) *cardv1.Card {
	out := &cardv1.Card{
		Id: c.ID.String(), AccountId: c.AccountID.String(), StatusReason: c.StatusReason, Last4: c.Last4,
		ExpiryMonth: int32(c.ExpiryMonth), ExpiryYear: int32(c.ExpiryYear), HolderName: c.HolderName,
		Controls:  &cardv1.Controls{Online: c.Controls.Online, International: c.Controls.International, Contactless: c.Controls.Contactless},
		CreatedAt: timestamppb.New(c.CreatedAt),
		Status: map[domain.Status]cardv1.CardStatus{
			domain.Active: cardv1.CardStatus_CARD_STATUS_ACTIVE, domain.Blocked: cardv1.CardStatus_CARD_STATUS_BLOCKED,
			domain.Cancelled: cardv1.CardStatus_CARD_STATUS_CANCELLED,
		}[c.Status],
	}
	for k, v := range kinds {
		if v == c.Kind {
			out.Kind = k
		}
	}
	return out
}

func authPB(a domain.Authorization) *cardv1.Authorization {
	st := map[domain.AuthStatus]cardv1.AuthorizationStatus{
		domain.Approved: cardv1.AuthorizationStatus_AUTHORIZATION_STATUS_APPROVED,
		domain.Declined: cardv1.AuthorizationStatus_AUTHORIZATION_STATUS_DECLINED,
		// Still being cleaned up: the network has already been told "declined".
		domain.PendingReversal: cardv1.AuthorizationStatus_AUTHORIZATION_STATUS_DECLINED,
		domain.Cleared:         cardv1.AuthorizationStatus_AUTHORIZATION_STATUS_CLEARED,
		domain.Reversed:        cardv1.AuthorizationStatus_AUTHORIZATION_STATUS_REVERSED,
	}[a.Status]
	out := &cardv1.Authorization{
		Id: a.ID.String(), NetworkReference: a.NetworkReference, Status: st, ResponseCode: a.Code, Reason: a.Reason,
		Amount:        &commonv1.Money{MinorUnits: a.Purchase.Amount.Minor(), Currency: string(a.Purchase.Amount.Currency())},
		ClearedAmount: &commonv1.Money{MinorUnits: a.Cleared, Currency: string(a.Purchase.Amount.Currency())},
		MerchantName:  a.Purchase.MerchantName, Mcc: a.Purchase.MCC, Country: a.Purchase.Country,
		CreatedAt: timestamppb.New(a.CreatedAt), Function: cardv1.Function_FUNCTION_DEBIT,
	}
	if a.Purchase.Function == domain.Credit {
		out.Function = cardv1.Function_FUNCTION_CREDIT
	}
	if a.CardID != uuid.Nil {
		out.CardId = a.CardID.String()
	}
	if a.TokenID != uuid.Nil {
		out.TokenId = a.TokenID.String()
	}
	for k, v := range channels {
		if v == a.Purchase.Channel {
			out.Channel = k
		}
	}
	return out
}

func (s *Server) IssueCard(ctx context.Context, req *cardv1.IssueCardRequest) (*cardv1.IssueCardResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	c, err := s.app.Issue(ctx, req.GetIdempotencyKey(), account, kinds[req.GetKind()])
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.IssueCardResponse{Card: cardPB(c)}, nil
}

func (s *Server) GetCard(ctx context.Context, req *cardv1.GetCardRequest) (*cardv1.GetCardResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	c, err := s.app.Card(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.GetCardResponse{Card: cardPB(c)}, nil
}

func (s *Server) ListCards(ctx context.Context, req *cardv1.ListCardsRequest) (*cardv1.ListCardsResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	list, err := s.app.Cards(ctx, account)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &cardv1.ListCardsResponse{}
	for _, c := range list {
		resp.Cards = append(resp.Cards, cardPB(c))
	}
	return resp, nil
}

func (s *Server) BlockCard(ctx context.Context, req *cardv1.BlockCardRequest) (*cardv1.BlockCardResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	c, err := s.app.Block(ctx, id, req.GetReason())
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.BlockCardResponse{Card: cardPB(c)}, nil
}

func (s *Server) UnblockCard(ctx context.Context, req *cardv1.UnblockCardRequest) (*cardv1.UnblockCardResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	c, err := s.app.Unblock(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.UnblockCardResponse{Card: cardPB(c)}, nil
}

func (s *Server) UpdateControls(ctx context.Context, req *cardv1.UpdateControlsRequest) (*cardv1.UpdateControlsResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	ctl := req.GetControls()
	c, err := s.app.UpdateControls(ctx, id, domain.Controls{Online: ctl.GetOnline(), International: ctl.GetInternational(), Contactless: ctl.GetContactless()})
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.UpdateControlsResponse{Card: cardPB(c)}, nil
}

func (s *Server) RevealCardDetails(ctx context.Context, req *cardv1.RevealCardDetailsRequest) (*cardv1.RevealCardDetailsResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	d, err := s.app.Reveal(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.RevealCardDetailsResponse{Pan: d.PAN, ExpiryMonth: int32(d.ExpiryMonth), ExpiryYear: int32(d.ExpiryYear), Cvv: d.CVV}, nil
}

func tokenPB(t domain.Token) *cardv1.CardToken {
	return &cardv1.CardToken{Id: t.ID.String(), CardId: t.CardID.String(), Wallet: string(t.Wallet),
		DeviceName: t.DeviceName, Status: string(t.Status), Last4: t.Last4, CreatedAt: timestamppb.New(t.CreatedAt)}
}

func (s *Server) TokenizeCard(ctx context.Context, req *cardv1.TokenizeCardRequest) (*cardv1.TokenizeCardResponse, error) {
	id, err := parseID(req.GetCardId())
	if err != nil {
		return nil, err
	}
	t, pan, err := s.app.Tokenize(ctx, id, domain.Wallet(req.GetWallet()), req.GetDeviceName())
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.TokenizeCardResponse{Token: tokenPB(t), TokenPan: pan}, nil
}

func (s *Server) ListTokens(ctx context.Context, req *cardv1.ListTokensRequest) (*cardv1.ListTokensResponse, error) {
	id, err := parseID(req.GetCardId())
	if err != nil {
		return nil, err
	}
	list, err := s.app.Tokens(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &cardv1.ListTokensResponse{}
	for _, t := range list {
		resp.Tokens = append(resp.Tokens, tokenPB(t))
	}
	return resp, nil
}

func (s *Server) UpdateToken(ctx context.Context, req *cardv1.UpdateTokenRequest) (*cardv1.UpdateTokenResponse, error) {
	card, err := parseID(req.GetCardId())
	if err != nil {
		return nil, err
	}
	token, err := parseID(req.GetTokenId())
	if err != nil {
		return nil, err
	}
	t, err := s.app.UpdateToken(ctx, card, token, req.GetAction())
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.UpdateTokenResponse{Token: tokenPB(t)}, nil
}

func (s *Server) Authorize(ctx context.Context, req *cardv1.AuthorizeRequest) (*cardv1.AuthorizeResponse, error) {
	amount, err := money.New(req.GetAmount().GetMinorUnits(), money.Currency(req.GetAmount().GetCurrency()))
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	}
	a, err := s.app.Authorize(ctx, req.GetPan(), domain.Purchase{
		NetworkReference: req.GetNetworkReference(), ExpiryMonth: int(req.GetExpiryMonth()), ExpiryYear: int(req.GetExpiryYear()),
		CVV: req.GetCvv(), Amount: amount, MerchantName: req.GetMerchantName(), MCC: req.GetMcc(),
		Country: req.GetCountry(), Channel: channels[req.GetChannel()],
		Function: map[cardv1.Function]domain.Function{cardv1.Function_FUNCTION_CREDIT: domain.Credit}[req.GetFunction()],
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.AuthorizeResponse{Authorization: authPB(a)}, nil
}

func (s *Server) Clear(ctx context.Context, req *cardv1.ClearRequest) (*cardv1.ClearResponse, error) {
	id, err := parseID(req.GetAuthorizationId())
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
	a, err := s.app.Clear(ctx, id, amount)
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.ClearResponse{Authorization: authPB(a)}, nil
}

func (s *Server) Reverse(ctx context.Context, req *cardv1.ReverseRequest) (*cardv1.ReverseResponse, error) {
	id, err := parseID(req.GetAuthorizationId())
	if err != nil {
		return nil, err
	}
	a, err := s.app.Reverse(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &cardv1.ReverseResponse{Authorization: authPB(a)}, nil
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		return grpcserver.Error(codes.FailedPrecondition, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, domain.ErrAccountNotActive):
		return grpcserver.Error(codes.FailedPrecondition, "ACCOUNT_NOT_ACTIVE", err.Error())
	case errors.Is(err, domain.ErrInvalidState):
		return grpcserver.Error(codes.FailedPrecondition, "INVALID_STATE", err.Error())
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "downstream unavailable: "+st.Message())
	}
	return grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
}
