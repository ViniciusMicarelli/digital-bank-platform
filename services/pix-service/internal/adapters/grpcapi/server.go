// Package grpcapi exposes Pix over gRPC (bank.pix.v1).
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
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
)

type Server struct {
	pixv1.UnimplementedPixServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

var keyTypes = map[pixv1.KeyType]domain.KeyType{
	pixv1.KeyType_KEY_TYPE_CPF:   domain.KeyCPF,
	pixv1.KeyType_KEY_TYPE_EMAIL: domain.KeyEmail,
	pixv1.KeyType_KEY_TYPE_PHONE: domain.KeyPhone,
	pixv1.KeyType_KEY_TYPE_EVP:   domain.KeyEVP,
}

func keyTypePB(t domain.KeyType) pixv1.KeyType {
	for k, v := range keyTypes {
		if v == t {
			return k
		}
	}
	return pixv1.KeyType_KEY_TYPE_UNSPECIFIED
}

func (s *Server) RegisterKey(ctx context.Context, req *pixv1.RegisterKeyRequest) (*pixv1.RegisterKeyResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	k, err := s.app.RegisterKey(ctx, account, keyTypes[req.GetType()], req.GetValue())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.RegisterKeyResponse{Key: keyPB(k)}, nil
}

func (s *Server) DeleteKey(ctx context.Context, req *pixv1.DeleteKeyRequest) (*pixv1.DeleteKeyResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	keyID, err := parseID(req.GetKeyId())
	if err != nil {
		return nil, err
	}
	if err := s.app.DeleteKey(ctx, account, keyID); err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.DeleteKeyResponse{}, nil
}

func (s *Server) ListKeys(ctx context.Context, req *pixv1.ListKeysRequest) (*pixv1.ListKeysResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	keys, err := s.app.Keys(ctx, account)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &pixv1.ListKeysResponse{}
	for _, k := range keys {
		resp.Keys = append(resp.Keys, keyPB(k))
	}
	return resp, nil
}

func (s *Server) LookupKey(ctx context.Context, req *pixv1.LookupKeyRequest) (*pixv1.LookupKeyResponse, error) {
	t, _, owner, _, err := s.app.Lookup(ctx, req.GetKey())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.LookupKeyResponse{Type: keyTypePB(t), Owner: partyPB(owner)}, nil
}

func (s *Server) CreateTransfer(ctx context.Context, req *pixv1.CreateTransferRequest) (*pixv1.CreateTransferResponse, error) {
	from, err := parseID(req.GetFromAccountId())
	if err != nil {
		return nil, err
	}
	amount, err := moneyFromPB(req.GetAmount())
	if err != nil {
		return nil, err
	}
	t, err := s.app.Send(ctx, req.GetIdempotencyKey(), from, req.GetKey(), amount, req.GetDescription())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.CreateTransferResponse{Transfer: transferPB(t)}, nil
}

func (s *Server) GetTransfer(ctx context.Context, req *pixv1.GetTransferRequest) (*pixv1.GetTransferResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	t, err := s.app.Transfer(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.GetTransferResponse{Transfer: transferPB(t)}, nil
}

func (s *Server) SimulateIncoming(ctx context.Context, req *pixv1.SimulateIncomingRequest) (*pixv1.SimulateIncomingResponse, error) {
	amount, err := moneyFromPB(req.GetAmount())
	if err != nil {
		return nil, err
	}
	p := req.GetPayer()
	t, err := s.app.Receive(ctx, req.GetEndToEndId(), req.GetKey(), amount, domain.Party{
		Name: p.GetName(), DocumentMasked: p.GetDocumentMasked(), ISPB: p.GetIspb(), BankName: p.GetBankName(),
	}, req.GetDescription())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.SimulateIncomingResponse{Transfer: transferPB(t)}, nil
}

func optionalMoney(m *commonv1.Money) (*money.Money, error) {
	if m == nil {
		return nil, nil
	}
	v, err := moneyFromPB(m)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Server) CreateQrCode(ctx context.Context, req *pixv1.CreateQrCodeRequest) (*pixv1.CreateQrCodeResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	amount, err := optionalMoney(req.GetAmount())
	if err != nil {
		return nil, err
	}
	q, payload, err := s.app.CreateQRCode(ctx, account, req.GetKey(), amount, req.GetDescription(), req.GetOneTime(),
		time.Duration(req.GetExpiresInSeconds())*time.Second)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &pixv1.CreateQrCodeResponse{Txid: q.TxID, Payload: payload}
	if q.ExpiresAt != nil {
		resp.ExpiresAt = timestamppb.New(*q.ExpiresAt)
	}
	return resp, nil
}

func (s *Server) DecodeQrCode(ctx context.Context, req *pixv1.DecodeQrCodeRequest) (*pixv1.DecodeQrCodeResponse, error) {
	d, err := s.app.DecodeQRCode(ctx, req.GetPayload())
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &pixv1.DecodeQrCodeResponse{
		Key: d.Code.Key, Receiver: partyPB(d.Receiver), Txid: d.Code.TxID,
		Description: d.Code.Description, OneTime: d.Code.OneTime, IssuedHere: d.Charge != nil,
	}
	if d.Code.Amount != nil {
		resp.Amount = &commonv1.Money{MinorUnits: d.Code.Amount.Minor(), Currency: string(d.Code.Amount.Currency())}
	}
	return resp, nil
}

func (s *Server) PayQrCode(ctx context.Context, req *pixv1.PayQrCodeRequest) (*pixv1.PayQrCodeResponse, error) {
	from, err := parseID(req.GetFromAccountId())
	if err != nil {
		return nil, err
	}
	amount, err := optionalMoney(req.GetAmount())
	if err != nil {
		return nil, err
	}
	t, err := s.app.PayQRCode(ctx, req.GetIdempotencyKey(), from, req.GetPayload(), amount, req.GetDescription())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.PayQrCodeResponse{Transfer: transferPB(t)}, nil
}

func (s *Server) RefundTransfer(ctx context.Context, req *pixv1.RefundTransferRequest) (*pixv1.RefundTransferResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	original, err := parseID(req.GetTransferId())
	if err != nil {
		return nil, err
	}
	amount, err := moneyFromPB(req.GetAmount())
	if err != nil {
		return nil, err
	}
	t, err := s.app.Refund(ctx, req.GetIdempotencyKey(), account, original, amount, req.GetReason())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.RefundTransferResponse{Transfer: transferPB(t)}, nil
}

func parseID(v string) (uuid.UUID, error) {
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "ids must be UUIDs")
	}
	return id, nil
}

func moneyFromPB(m *commonv1.Money) (money.Money, error) {
	out, err := money.New(m.GetMinorUnits(), money.Currency(m.GetCurrency()))
	if err != nil {
		return money.Money{}, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	}
	return out, nil
}

func keyPB(k domain.Key) *pixv1.PixKey {
	return &pixv1.PixKey{Id: k.ID.String(), AccountId: k.AccountID.String(), Type: keyTypePB(k.Type),
		Value: k.Value, CreatedAt: timestamppb.New(k.CreatedAt)}
}

func partyPB(p domain.Party) *pixv1.Party {
	return &pixv1.Party{Name: p.Name, DocumentMasked: p.DocumentMasked, Ispb: p.ISPB, BankName: p.BankName}
}

func transferPB(t domain.Transfer) *pixv1.Transfer {
	out := &pixv1.Transfer{
		Id: t.ID.String(), EndToEndId: t.EndToEndID, AccountId: t.AccountID.String(), Key: t.Key,
		Counterparty: partyPB(t.Counterparty),
		Amount:       &commonv1.Money{MinorUnits: t.Amount.Minor(), Currency: string(t.Amount.Currency())},
		Description:  t.Description, FailureReason: t.FailureReason,
		CreatedAt: timestamppb.New(t.CreatedAt), UpdatedAt: timestamppb.New(t.UpdatedAt),
		Direction: map[domain.Direction]pixv1.TransferDirection{
			domain.Outgoing: pixv1.TransferDirection_TRANSFER_DIRECTION_OUTGOING,
			domain.Incoming: pixv1.TransferDirection_TRANSFER_DIRECTION_INCOMING,
		}[t.Direction],
		Status: map[domain.Status]pixv1.TransferStatus{
			domain.StatusCreated:       pixv1.TransferStatus_TRANSFER_STATUS_CREATED,
			domain.StatusFundsReserved: pixv1.TransferStatus_TRANSFER_STATUS_FUNDS_RESERVED,
			domain.StatusSentToSPI:     pixv1.TransferStatus_TRANSFER_STATUS_SENT_TO_SPI,
			domain.StatusCompleted:     pixv1.TransferStatus_TRANSFER_STATUS_COMPLETED,
			domain.StatusFailed:        pixv1.TransferStatus_TRANSFER_STATUS_FAILED,
			domain.StatusRejected:      pixv1.TransferStatus_TRANSFER_STATUS_REJECTED,
		}[t.Status],
	}
	if t.TransactionID != uuid.Nil {
		out.TransactionId = t.TransactionID.String()
	}
	if t.RefundOf != uuid.Nil {
		out.RefundOf = t.RefundOf.String()
	}
	if t.Direction == domain.Incoming {
		out.Refunded = &commonv1.Money{MinorUnits: t.Refunded, Currency: string(t.Amount.Currency())}
	}
	out.QrTxid = t.QRTxID
	return out
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrKeyTaken):
		return grpcserver.Error(codes.AlreadyExists, "KEY_TAKEN", err.Error())
	case errors.Is(err, domain.ErrTooManyKeys):
		return grpcserver.Error(codes.FailedPrecondition, "TOO_MANY_KEYS", err.Error())
	case errors.Is(err, application.ErrAccountNotActive):
		return grpcserver.Error(codes.FailedPrecondition, "ACCOUNT_NOT_ACTIVE", err.Error())
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return grpcserver.Error(codes.AlreadyExists, "IDEMPOTENCY_CONFLICT", err.Error())
	case errors.Is(err, domain.ErrQRUnavailable):
		return grpcserver.Error(codes.FailedPrecondition, "QR_CODE_UNAVAILABLE", err.Error())
	case errors.Is(err, domain.ErrNotRefundable):
		return grpcserver.Error(codes.FailedPrecondition, "NOT_REFUNDABLE", err.Error())
	case errors.Is(err, domain.ErrRefundTooLarge):
		return grpcserver.Error(codes.FailedPrecondition, "REFUND_TOO_LARGE", err.Error())
	case errors.Is(err, domain.ErrBusy):
		return grpcserver.Error(codes.Aborted, "IN_PROGRESS", err.Error())
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "downstream unavailable: "+st.Message())
	}
	return grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
}
