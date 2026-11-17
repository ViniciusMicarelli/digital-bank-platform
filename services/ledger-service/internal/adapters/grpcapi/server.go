// Package grpcapi exposes the ledger application over gRPC (bank.ledger.v1).
package grpcapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/ports"
)

// Server implements ledgerv1.LedgerServiceServer.
type Server struct {
	ledgerv1.UnimplementedLedgerServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

func (s *Server) CreateLedgerAccount(ctx context.Context, req *ledgerv1.CreateLedgerAccountRequest) (*ledgerv1.CreateLedgerAccountResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	kind := map[ledgerv1.AccountKind]domain.AccountKind{
		ledgerv1.AccountKind_ACCOUNT_KIND_CUSTOMER: domain.KindCustomer,
		ledgerv1.AccountKind_ACCOUNT_KIND_INTERNAL: domain.KindInternal,
	}[req.GetKind()]
	acc, err := s.app.CreateAccount(ctx, domain.Account{
		ID: id, Kind: kind, Currency: money.Currency(req.GetCurrency()),
		OverdraftLimit: req.GetOverdraftLimitMinorUnits(), Code: req.GetCode(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &ledgerv1.CreateLedgerAccountResponse{Account: accountPB(acc)}, nil
}

func (s *Server) GetLedgerAccount(ctx context.Context, req *ledgerv1.GetLedgerAccountRequest) (*ledgerv1.GetLedgerAccountResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	acc, err := s.app.Account(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &ledgerv1.GetLedgerAccountResponse{Account: accountPB(acc)}, nil
}

func (s *Server) PostEntry(ctx context.Context, req *ledgerv1.PostEntryRequest) (*ledgerv1.PostEntryResponse, error) {
	postings := make([]domain.Posting, len(req.GetPostings()))
	for i, p := range req.GetPostings() {
		id, err := parseID(fmt.Sprintf("postings[%d].account_id", i), p.GetAccountId())
		if err != nil {
			return nil, err
		}
		amount, err := moneyFromPB(p.GetAmount())
		if err != nil {
			return nil, err
		}
		postings[i] = domain.Posting{AccountID: id, Amount: amount}
	}
	e, err := s.app.PostEntry(ctx, req.GetIdempotencyKey(), postings, req.GetDescription(), req.GetMetadata())
	if err != nil {
		return nil, toStatus(err)
	}
	return &ledgerv1.PostEntryResponse{Entry: entryPB(e)}, nil
}

func (s *Server) GetEntry(ctx context.Context, req *ledgerv1.GetEntryRequest) (*ledgerv1.GetEntryResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	e, err := s.app.Entry(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &ledgerv1.GetEntryResponse{Entry: entryPB(e)}, nil
}

func (s *Server) ReverseEntry(ctx context.Context, req *ledgerv1.ReverseEntryRequest) (*ledgerv1.ReverseEntryResponse, error) {
	id, err := parseID("entry_id", req.GetEntryId())
	if err != nil {
		return nil, err
	}
	e, err := s.app.ReverseEntry(ctx, req.GetIdempotencyKey(), id, req.GetDescription())
	if err != nil {
		return nil, toStatus(err)
	}
	return &ledgerv1.ReverseEntryResponse{Entry: entryPB(e)}, nil
}

func (s *Server) Hold(ctx context.Context, req *ledgerv1.HoldRequest) (*ledgerv1.HoldResponse, error) {
	account, err := parseID("account_id", req.GetAccountId())
	if err != nil {
		return nil, err
	}
	counter, err := parseID("counter_account_id", req.GetCounterAccountId())
	if err != nil {
		return nil, err
	}
	amount, err := moneyFromPB(req.GetAmount())
	if err != nil {
		return nil, err
	}
	h, err := s.app.Hold(ctx, req.GetIdempotencyKey(), account, counter, amount, req.GetDescription())
	if err != nil {
		return nil, toStatus(err)
	}
	return &ledgerv1.HoldResponse{Hold: holdPB(h)}, nil
}

func (s *Server) Capture(ctx context.Context, req *ledgerv1.CaptureRequest) (*ledgerv1.CaptureResponse, error) {
	id, err := parseID("hold_id", req.GetHoldId())
	if err != nil {
		return nil, err
	}
	var amount *money.Money
	if req.GetAmount() != nil {
		m, err := moneyFromPB(req.GetAmount())
		if err != nil {
			return nil, err
		}
		amount = &m
	}
	h, e, err := s.app.Capture(ctx, id, amount)
	if err != nil {
		return nil, toStatus(err)
	}
	return &ledgerv1.CaptureResponse{Hold: holdPB(h), Entry: entryPB(e)}, nil
}

func (s *Server) Release(ctx context.Context, req *ledgerv1.ReleaseRequest) (*ledgerv1.ReleaseResponse, error) {
	id, err := parseID("hold_id", req.GetHoldId())
	if err != nil {
		return nil, err
	}
	h, err := s.app.Release(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &ledgerv1.ReleaseResponse{Hold: holdPB(h)}, nil
}

func (s *Server) GetBalance(ctx context.Context, req *ledgerv1.GetBalanceRequest) (*ledgerv1.GetBalanceResponse, error) {
	id, err := parseID("account_id", req.GetAccountId())
	if err != nil {
		return nil, err
	}
	acc, b, err := s.app.Balance(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	c := string(acc.Currency)
	return &ledgerv1.GetBalanceResponse{Balance: &ledgerv1.Balance{
		AccountId: id.String(),
		Posted:    &commonv1.Money{MinorUnits: b.Posted, Currency: c},
		Pending:   &commonv1.Money{MinorUnits: b.Pending, Currency: c},
		Available: &commonv1.Money{MinorUnits: b.Available(), Currency: c},
	}}, nil
}

func (s *Server) GetBalanceAt(ctx context.Context, req *ledgerv1.GetBalanceAtRequest) (*ledgerv1.GetBalanceAtResponse, error) {
	id, err := parseID("account_id", req.GetAccountId())
	if err != nil {
		return nil, err
	}
	if req.GetAt() == nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "at is required")
	}
	acc, posted, err := s.app.BalanceAt(ctx, id, req.GetAt().AsTime())
	if err != nil {
		return nil, toStatus(err)
	}
	return &ledgerv1.GetBalanceAtResponse{Posted: &commonv1.Money{MinorUnits: posted, Currency: string(acc.Currency)}}, nil
}

func (s *Server) ListPostings(ctx context.Context, req *ledgerv1.ListPostingsRequest) (*ledgerv1.ListPostingsResponse, error) {
	id, err := parseID("account_id", req.GetAccountId())
	if err != nil {
		return nil, err
	}
	size := int(req.GetPageSize())
	if size <= 0 {
		size = 50
	}
	size = min(size, 200)
	var before int64
	if tok := req.GetPageToken(); tok != "" {
		raw, err := base64.RawURLEncoding.DecodeString(tok)
		if err == nil {
			before, err = strconv.ParseInt(string(raw), 10, 64)
		}
		if err != nil || before <= 0 {
			return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "invalid page_token")
		}
	}
	// Fetch one extra row to know whether there is a next page.
	rows, err := s.app.Postings(ctx, id, before, size+1)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &ledgerv1.ListPostingsResponse{}
	if len(rows) > size {
		rows = rows[:size]
		resp.NextPageToken = base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(rows[size-1].ID, 10)))
	}
	for _, p := range rows {
		resp.Postings = append(resp.Postings, postingPB(p))
	}
	return resp, nil
}

func parseID(field, v string) (uuid.UUID, error) {
	id, err := uuid.Parse(v)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", field+" must be a UUID")
	}
	return id, nil
}

func moneyFromPB(m *commonv1.Money) (money.Money, error) {
	if m == nil {
		return money.Money{}, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "amount is required")
	}
	out, err := money.New(m.GetMinorUnits(), money.Currency(m.GetCurrency()))
	if err != nil {
		return money.Money{}, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	}
	return out, nil
}

func moneyPB(m money.Money) *commonv1.Money {
	return &commonv1.Money{MinorUnits: m.Minor(), Currency: string(m.Currency())}
}

func accountPB(a domain.Account) *ledgerv1.LedgerAccount {
	kind := ledgerv1.AccountKind_ACCOUNT_KIND_CUSTOMER
	if a.Kind == domain.KindInternal {
		kind = ledgerv1.AccountKind_ACCOUNT_KIND_INTERNAL
	}
	return &ledgerv1.LedgerAccount{
		Id: a.ID.String(), Kind: kind, Currency: string(a.Currency),
		OverdraftLimitMinorUnits: a.OverdraftLimit, Code: a.Code, CreatedAt: timestamppb.New(a.CreatedAt),
	}
}

func entryPB(e domain.Entry) *ledgerv1.JournalEntry {
	out := &ledgerv1.JournalEntry{
		Id: e.ID.String(), IdempotencyKey: e.IdempotencyKey, Description: e.Description,
		Metadata: e.Metadata, CreatedAt: timestamppb.New(e.CreatedAt),
	}
	if e.ReversesEntryID != uuid.Nil {
		out.ReversesEntryId = e.ReversesEntryID.String()
	}
	for _, p := range e.Postings {
		out.Postings = append(out.Postings, &ledgerv1.Posting{AccountId: p.AccountID.String(), Amount: moneyPB(p.Amount)})
	}
	return out
}

func holdPB(h domain.Hold) *ledgerv1.HoldInfo {
	out := &ledgerv1.HoldInfo{
		Id: h.ID.String(), IdempotencyKey: h.IdempotencyKey, AccountId: h.AccountID.String(),
		CounterAccountId: h.CounterAccountID.String(), Amount: moneyPB(h.Amount),
		Status: map[domain.HoldStatus]ledgerv1.HoldStatus{
			domain.HoldActive:   ledgerv1.HoldStatus_HOLD_STATUS_ACTIVE,
			domain.HoldCaptured: ledgerv1.HoldStatus_HOLD_STATUS_CAPTURED,
			domain.HoldReleased: ledgerv1.HoldStatus_HOLD_STATUS_RELEASED,
		}[h.Status],
		Description: h.Description, CreatedAt: timestamppb.New(h.CreatedAt),
	}
	if h.CaptureEntryID != uuid.Nil {
		out.CaptureEntryId = h.CaptureEntryID.String()
	}
	return out
}

func postingPB(p ports.AccountPosting) *ledgerv1.AccountPosting {
	return &ledgerv1.AccountPosting{
		Id: p.ID, EntryId: p.EntryID.String(),
		Amount:      &commonv1.Money{MinorUnits: p.Amount, Currency: p.Currency},
		Description: p.Description, CreatedAt: timestamppb.New(p.CreatedAt),
	}
}

// Reasons attached to errors, so callers can branch without parsing messages.
const (
	ReasonInsufficientFunds   = "INSUFFICIENT_FUNDS"
	ReasonIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
	ReasonAlreadyReversed     = "ALREADY_REVERSED"
	ReasonHoldNotActive       = "HOLD_NOT_ACTIVE"
	ReasonAccountConflict     = "ACCOUNT_CONFLICT"
)

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrInsufficientFunds):
		return grpcserver.Error(codes.FailedPrecondition, ReasonInsufficientFunds, err.Error())
	case errors.Is(err, domain.ErrHoldNotActive):
		return grpcserver.Error(codes.FailedPrecondition, ReasonHoldNotActive, err.Error())
	case errors.Is(err, domain.ErrAlreadyReversed):
		return grpcserver.Error(codes.FailedPrecondition, ReasonAlreadyReversed, err.Error())
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return grpcserver.Error(codes.AlreadyExists, ReasonIdempotencyConflict, err.Error())
	case errors.Is(err, domain.ErrAccountConflict):
		return grpcserver.Error(codes.AlreadyExists, ReasonAccountConflict, err.Error())
	case errors.Is(err, context.Canceled):
		return grpcserver.Error(codes.Canceled, "", err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return grpcserver.Error(codes.DeadlineExceeded, "", err.Error())
	}
	return grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
}
