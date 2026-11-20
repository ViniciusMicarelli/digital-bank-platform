// Package grpcapi exposes identity over gRPC (bank.identity.v1).
package grpcapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/identity/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/domain"
)

type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

func pairPB(p application.TokenPair) *identityv1.TokenPair {
	return &identityv1.TokenPair{
		AccessToken: p.AccessToken, AccessExpiresAt: timestamppb.New(p.AccessExpiresAt),
		RefreshToken: p.RefreshToken, RefreshExpiresAt: timestamppb.New(p.RefreshExpiresAt),
	}
}

func (s *Server) Register(ctx context.Context, req *identityv1.RegisterRequest) (*identityv1.RegisterResponse, error) {
	id, err := s.app.Register(ctx, req.GetEmail(), req.GetPassword(), req.GetFullName())
	if err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.RegisterResponse{CustomerId: id.String()}, nil
}

func (s *Server) Login(ctx context.Context, req *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
	id, pair, err := s.app.Login(ctx, req.GetEmail(), req.GetPassword(), req.GetTotpCode(),
		domain.Device{UserAgent: req.GetUserAgent(), IP: req.GetIp()})
	if err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.LoginResponse{CustomerId: id.String(), Tokens: pairPB(pair)}, nil
}

func (s *Server) Refresh(ctx context.Context, req *identityv1.RefreshRequest) (*identityv1.RefreshResponse, error) {
	pair, err := s.app.Refresh(ctx, req.GetRefreshToken(), req.GetIp())
	if err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.RefreshResponse{Tokens: pairPB(pair)}, nil
}

func (s *Server) Logout(ctx context.Context, req *identityv1.LogoutRequest) (*identityv1.LogoutResponse, error) {
	if err := s.app.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.LogoutResponse{}, nil
}

func (s *Server) StepUp(ctx context.Context, req *identityv1.StepUpRequest) (*identityv1.StepUpResponse, error) {
	id, err := uuid.Parse(req.GetCustomerId())
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "customer_id must be a UUID")
	}
	tok, exp, err := s.app.StepUp(ctx, id, req.GetPassword(), req.GetTotpCode())
	if err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.StepUpResponse{StepUpToken: tok, ExpiresAt: timestamppb.New(exp)}, nil
}

func (s *Server) EnrollTotp(ctx context.Context, req *identityv1.EnrollTotpRequest) (*identityv1.EnrollTotpResponse, error) {
	id, err := uuid.Parse(req.GetCustomerId())
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "customer_id must be a UUID")
	}
	secret, uri, err := s.app.EnrollTOTP(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.EnrollTotpResponse{Secret: secret, OtpauthUri: uri}, nil
}

func (s *Server) ConfirmTotp(ctx context.Context, req *identityv1.ConfirmTotpRequest) (*identityv1.ConfirmTotpResponse, error) {
	id, err := uuid.Parse(req.GetCustomerId())
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "customer_id must be a UUID")
	}
	if err := s.app.ConfirmTOTP(ctx, id, req.GetCode()); err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.ConfirmTotpResponse{}, nil
}

func (s *Server) GetJwks(ctx context.Context, _ *identityv1.GetJwksRequest) (*identityv1.GetJwksResponse, error) {
	keys, err := s.app.JWKS(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &identityv1.GetJwksResponse{}
	for _, k := range keys {
		resp.Keys = append(resp.Keys, &identityv1.Jwk{Kid: k.Kid, Kty: k.Kty, Crv: k.Crv, X: k.X, Alg: k.Alg, Use: k.Use})
	}
	return resp, nil
}

func (s *Server) RotateSigningKey(ctx context.Context, _ *identityv1.RotateSigningKeyRequest) (*identityv1.RotateSigningKeyResponse, error) {
	kid, err := s.app.RotateSigningKey(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.RotateSigningKeyResponse{Kid: kid}, nil
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrEmailTaken):
		return grpcserver.Error(codes.AlreadyExists, "EMAIL_TAKEN", err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		return grpcserver.Error(codes.Unauthenticated, "INVALID_CREDENTIALS", err.Error())
	case errors.Is(err, domain.ErrMFARequired):
		return grpcserver.Error(codes.Unauthenticated, "MFA_REQUIRED", err.Error())
	case errors.Is(err, domain.ErrInvalidMFACode):
		return grpcserver.Error(codes.Unauthenticated, "INVALID_MFA_CODE", err.Error())
	case errors.Is(err, domain.ErrLocked):
		return grpcserver.Error(codes.Unauthenticated, "ACCOUNT_LOCKED", err.Error())
	case errors.Is(err, domain.ErrInvalidRefresh):
		return grpcserver.Error(codes.Unauthenticated, "INVALID_REFRESH_TOKEN", err.Error())
	case errors.Is(err, domain.ErrRefreshReused):
		return grpcserver.Error(codes.Unauthenticated, "REFRESH_TOKEN_REUSED", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, application.ErrTOTPAlreadyEnabled):
		return grpcserver.Error(codes.FailedPrecondition, "TOTP_ALREADY_ENABLED", err.Error())
	}
	return grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
}

func ids(values ...string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, len(values))
	for i, v := range values {
		id, err := uuid.Parse(v)
		if err != nil {
			return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "ids must be UUIDs")
		}
		out[i] = id
	}
	return out, nil
}

func (s *Server) ListSessions(ctx context.Context, req *identityv1.ListSessionsRequest) (*identityv1.ListSessionsResponse, error) {
	id, err := ids(req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	list, err := s.app.Sessions(ctx, id[0])
	if err != nil {
		return nil, toStatus(err)
	}
	out := &identityv1.ListSessionsResponse{}
	for _, x := range list {
		out.Sessions = append(out.Sessions, &identityv1.Session{Id: x.ID.String(), Device: x.Device, Ip: x.IP,
			CreatedAt: timestamppb.New(x.CreatedAt), LastUsedAt: timestamppb.New(x.LastUsedAt)})
	}
	return out, nil
}

func (s *Server) RevokeSession(ctx context.Context, req *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error) {
	id, err := ids(req.GetCustomerId(), req.GetSessionId())
	if err != nil {
		return nil, err
	}
	if err := s.app.RevokeSession(ctx, id[0], id[1]); err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.RevokeSessionResponse{}, nil
}

func (s *Server) RevokeOtherSessions(ctx context.Context, req *identityv1.RevokeOtherSessionsRequest) (*identityv1.RevokeOtherSessionsResponse, error) {
	id, err := ids(req.GetCustomerId(), req.GetKeepSessionId())
	if err != nil {
		return nil, err
	}
	n, err := s.app.RevokeOtherSessions(ctx, id[0], id[1])
	if err != nil {
		return nil, toStatus(err)
	}
	return &identityv1.RevokeOtherSessionsResponse{Revoked: int32(n)}, nil //nolint:gosec // at most a few hundred
}
