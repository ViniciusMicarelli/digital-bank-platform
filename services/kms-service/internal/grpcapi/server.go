// Package grpcapi exposes the keyring over gRPC (bank.kms.v1), authorizing
// each call by the caller's mTLS identity.
package grpcapi

import (
	"context"
	"errors"
	"os"

	"google.golang.org/grpc/codes"

	kmsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/kms/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/mtls"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/kms-service/internal/keyring"
)

type Server struct {
	kmsv1.UnimplementedKmsServiceServer
	k *keyring.Keyring
	// Operators may rotate any key; services only use their own.
	operators map[string]bool
}

func NewServer(k *keyring.Keyring, operators ...string) *Server {
	ops := map[string]bool{}
	for _, o := range operators {
		ops[o] = true
	}
	return &Server{k: k, operators: ops}
}

func caller(ctx context.Context) string {
	if id, ok := mtls.PeerIdentity(ctx); ok {
		return id
	}
	// Plaintext development mode (no TLS configured) only.
	return os.Getenv("KMS_INSECURE_CALLER")
}

func (s *Server) authorize(ctx context.Context, name string) error {
	return toStatus(keyring.Authorize(caller(ctx), name))
}

func (s *Server) GenerateDataKey(ctx context.Context, req *kmsv1.GenerateDataKeyRequest) (*kmsv1.GenerateDataKeyResponse, error) {
	if err := s.authorize(ctx, req.GetKeyName()); err != nil {
		return nil, err
	}
	key, wrapped, v, err := s.k.GenerateDataKey(ctx, req.GetKeyName())
	if err != nil {
		return nil, toStatus(err)
	}
	return &kmsv1.GenerateDataKeyResponse{Plaintext: key, Wrapped: wrapped, KeyVersion: int32(v)}, nil
}

func (s *Server) Encrypt(ctx context.Context, req *kmsv1.EncryptRequest) (*kmsv1.EncryptResponse, error) {
	if err := s.authorize(ctx, req.GetKeyName()); err != nil {
		return nil, err
	}
	wrapped, v, err := s.k.Wrap(ctx, req.GetKeyName(), req.GetPlaintext())
	if err != nil {
		return nil, toStatus(err)
	}
	return &kmsv1.EncryptResponse{Wrapped: wrapped, KeyVersion: int32(v)}, nil
}

func (s *Server) Decrypt(ctx context.Context, req *kmsv1.DecryptRequest) (*kmsv1.DecryptResponse, error) {
	if err := s.authorize(ctx, req.GetKeyName()); err != nil {
		return nil, err
	}
	key, _, err := s.k.Unwrap(ctx, req.GetKeyName(), req.GetWrapped())
	if err != nil {
		return nil, toStatus(err)
	}
	return &kmsv1.DecryptResponse{Plaintext: key}, nil
}

func (s *Server) ReEncrypt(ctx context.Context, req *kmsv1.ReEncryptRequest) (*kmsv1.ReEncryptResponse, error) {
	if err := s.authorize(ctx, req.GetKeyName()); err != nil {
		return nil, err
	}
	wrapped, v, changed, err := s.k.ReEncrypt(ctx, req.GetKeyName(), req.GetWrapped())
	if err != nil {
		return nil, toStatus(err)
	}
	return &kmsv1.ReEncryptResponse{Wrapped: wrapped, KeyVersion: int32(v), Changed: changed}, nil
}

func (s *Server) RotateKey(ctx context.Context, req *kmsv1.RotateKeyRequest) (*kmsv1.RotateKeyResponse, error) {
	if !s.operators[caller(ctx)] {
		if err := s.authorize(ctx, req.GetKeyName()); err != nil {
			return nil, err
		}
	}
	v, err := s.k.Rotate(ctx, req.GetKeyName())
	if err != nil {
		return nil, toStatus(err)
	}
	return &kmsv1.RotateKeyResponse{KeyVersion: int32(v)}, nil
}

func toStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, keyring.ErrDenied):
		return grpcserver.Error(codes.PermissionDenied, "KEY_ACCESS_DENIED", err.Error())
	case errors.Is(err, keyring.ErrDecrypt):
		return grpcserver.Error(codes.InvalidArgument, "CANNOT_DECRYPT", err.Error())
	}
	return grpcserver.Error(codes.Internal, "INTERNAL", "internal error")
}
