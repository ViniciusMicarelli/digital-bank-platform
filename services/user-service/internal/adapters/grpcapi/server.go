// Package grpcapi exposes the user service over gRPC.
package grpcapi

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/document"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/domain"
)

type Server struct {
	userv1.UnimplementedUserServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

func pb(p domain.Profile) *userv1.Profile {
	a := p.Address
	return &userv1.Profile{CustomerId: p.CustomerID, FullName: p.FullName, CpfMasked: document.MaskCPF(p.CPF),
		BirthDate: p.BirthDate.Format(time.DateOnly), Phone: p.Phone, KycStatus: string(p.KYCStatus), KycReason: p.KYCReason,
		ReviewedBy: p.ReviewedBy, UpdatedAt: timestamppb.New(p.UpdatedAt),
		Address: &userv1.Address{PostalCode: a.PostalCode, Street: a.Street, Number: a.Number, Complement: a.Complement,
			District: a.District, City: a.City, State: a.State}}
}

func need(customer string) error {
	if customer == "" {
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "customer_id is required")
	}
	return nil
}

func (s *Server) GetProfile(ctx context.Context, req *userv1.GetProfileRequest) (*userv1.GetProfileResponse, error) {
	if err := need(req.GetCustomerId()); err != nil {
		return nil, err
	}
	p, err := s.app.Profile(ctx, req.GetCustomerId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &userv1.GetProfileResponse{Profile: pb(p)}, nil
}

func (s *Server) UpsertProfile(ctx context.Context, req *userv1.UpsertProfileRequest) (*userv1.UpsertProfileResponse, error) {
	if err := need(req.GetCustomerId()); err != nil {
		return nil, err
	}
	birth, err := time.Parse(time.DateOnly, req.GetBirthDate())
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "birth date is YYYY-MM-DD")
	}
	a := req.GetAddress()
	p, err := s.app.PutProfile(ctx, req.GetCustomerId(), domain.Profile{FullName: req.GetFullName(), CPF: req.GetCpf(),
		BirthDate: birth, Phone: req.GetPhone(), Address: domain.Address{PostalCode: a.GetPostalCode(), Street: a.GetStreet(),
			Number: a.GetNumber(), Complement: a.GetComplement(), District: a.GetDistrict(), City: a.GetCity(), State: a.GetState()}})
	if err != nil {
		return nil, toStatus(err)
	}
	return &userv1.UpsertProfileResponse{Profile: pb(p)}, nil
}

func (s *Server) SubmitKyc(ctx context.Context, req *userv1.SubmitKycRequest) (*userv1.SubmitKycResponse, error) {
	if err := need(req.GetCustomerId()); err != nil {
		return nil, err
	}
	p, err := s.app.SubmitKYC(ctx, req.GetCustomerId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &userv1.SubmitKycResponse{Profile: pb(p)}, nil
}

func (s *Server) ReviewKyc(ctx context.Context, req *userv1.ReviewKycRequest) (*userv1.ReviewKycResponse, error) {
	if err := need(req.GetCustomerId()); err != nil {
		return nil, err
	}
	p, err := s.app.Review(ctx, req.GetCustomerId(), req.GetApprove(), req.GetReason(), req.GetReviewerId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &userv1.ReviewKycResponse{Profile: pb(p)}, nil
}

func (s *Server) ListKycReviews(ctx context.Context, req *userv1.ListKycReviewsRequest) (*userv1.ListKycReviewsResponse, error) {
	list, err := s.app.Reviews(ctx, int(req.GetLimit()))
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &userv1.ListKycReviewsResponse{}
	for _, p := range list {
		resp.Profiles = append(resp.Profiles, pb(p))
	}
	return resp, nil
}

func (s *Server) GetIdentity(ctx context.Context, req *userv1.GetIdentityRequest) (*userv1.GetIdentityResponse, error) {
	if err := need(req.GetCustomerId()); err != nil {
		return nil, err
	}
	p, err := s.app.Profile(ctx, req.GetCustomerId())
	if errors.Is(err, domain.ErrNotFound) {
		return &userv1.GetIdentityResponse{KycStatus: string(domain.KYCNotStarted)}, nil
	}
	if err != nil {
		return nil, toStatus(err)
	}
	return &userv1.GetIdentityResponse{KycStatus: string(p.KYCStatus), FullName: p.FullName, Cpf: p.CPF}, nil
}

func toStatus(err error) error {
	var r *domain.RefusedError
	switch {
	case errors.As(err, &r):
		return grpcserver.Error(codes.FailedPrecondition, r.Reason, r.Msg)
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_PROFILE", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "PROFILE_NOT_FOUND", "create your profile first")
	}
	return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "try again")
}
