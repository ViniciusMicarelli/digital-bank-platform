// Package grpcapi exposes the back-office service over gRPC.
package grpcapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	backofficev1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/backoffice/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/ports"
)

type Server struct {
	backofficev1.UnimplementedBackOfficeServiceServer
	app *application.Service
}

func NewServer(app *application.Service) *Server { return &Server{app: app} }

func eventPB(e domain.Event) *backofficev1.AuditEvent {
	return &backofficev1.AuditEvent{Seq: e.Seq, ActorId: e.ActorID, Action: e.Action, TargetType: e.TargetType,
		TargetId: e.TargetID, Details: e.Details, At: timestamppb.New(e.At), Hash: e.Hash, PrevHash: e.PrevHash}
}

var (
	kindToPB = map[domain.Kind]backofficev1.ApprovalKind{
		domain.KindUnblockAccount: backofficev1.ApprovalKind_APPROVAL_KIND_UNBLOCK_ACCOUNT,
		domain.KindLimitIncrease:  backofficev1.ApprovalKind_APPROVAL_KIND_LIMIT_INCREASE,
	}
	statusToPB = map[domain.Status]backofficev1.ApprovalStatus{
		domain.StatusPending:  backofficev1.ApprovalStatus_APPROVAL_STATUS_PENDING,
		domain.StatusApproved: backofficev1.ApprovalStatus_APPROVAL_STATUS_APPROVED,
		domain.StatusExecuted: backofficev1.ApprovalStatus_APPROVAL_STATUS_EXECUTED,
		domain.StatusRejected: backofficev1.ApprovalStatus_APPROVAL_STATUS_REJECTED,
		domain.StatusFailed:   backofficev1.ApprovalStatus_APPROVAL_STATUS_FAILED,
		domain.StatusExpired:  backofficev1.ApprovalStatus_APPROVAL_STATUS_EXPIRED,
	}
)

func approvalPB(a domain.Approval) *backofficev1.Approval {
	out := &backofficev1.Approval{Id: a.ID.String(), Kind: kindToPB[a.Kind], TargetId: a.TargetID, Reason: a.Reason,
		RequestedBy: a.RequestedBy, Params: a.Params, Status: statusToPB[a.Status], DecidedBy: a.DecidedBy, DecisionReason: a.DecisionReason,
		FailureReason: a.FailureReason, CreatedAt: timestamppb.New(a.CreatedAt), ExpiresAt: timestamppb.New(a.ExpiresAt)}
	if a.DecidedAt != nil {
		out.DecidedAt = timestamppb.New(*a.DecidedAt)
	}
	return out
}

func toStatus(err error) error {
	if r, ok := domain.AsRefused(err); ok {
		return grpcserver.Error(codes.FailedPrecondition, r.Reason, r.Msg)
	}
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return grpcserver.Error(codes.NotFound, "APPROVAL_NOT_FOUND", "no such approval")
	}
	return grpcserver.Error(codes.Unavailable, "RETRY_LATER", "try again")
}

func (s *Server) RecordAudit(ctx context.Context, req *backofficev1.RecordAuditRequest) (*backofficev1.RecordAuditResponse, error) {
	e, err := s.app.Record(ctx, domain.Event{ActorID: req.GetActorId(), Action: req.GetAction(), TargetType: req.GetTargetType(),
		TargetID: req.GetTargetId(), Details: req.GetDetails()})
	if err != nil {
		return nil, toStatus(err)
	}
	return &backofficev1.RecordAuditResponse{Event: eventPB(e)}, nil
}

func (s *Server) ListAudit(ctx context.Context, req *backofficev1.ListAuditRequest) (*backofficev1.ListAuditResponse, error) {
	events, err := s.app.Audit(ctx, ports.AuditFilter{TargetID: req.GetTargetId(), ActorID: req.GetActorId(), Limit: int(req.GetLimit())})
	if err != nil {
		return nil, toStatus(err)
	}
	out := &backofficev1.ListAuditResponse{}
	for _, e := range events {
		out.Events = append(out.Events, eventPB(e))
	}
	return out, nil
}

func (s *Server) VerifyAudit(ctx context.Context, _ *backofficev1.VerifyAuditRequest) (*backofficev1.VerifyAuditResponse, error) {
	v, err := s.app.Verify(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &backofficev1.VerifyAuditResponse{Intact: v.Intact, Events: v.Events, BrokenAt: v.BrokenAt}, nil
}

func (s *Server) RequestApproval(ctx context.Context, req *backofficev1.RequestApprovalRequest) (*backofficev1.RequestApprovalResponse, error) {
	var kind domain.Kind
	for k, v := range kindToPB {
		if v == req.GetKind() {
			kind = k
		}
	}
	a, err := s.app.Request(ctx, kind, req.GetTargetId(), req.GetReason(), req.GetRequestedBy(), req.GetParams())
	if err != nil {
		return nil, toStatus(err)
	}
	return &backofficev1.RequestApprovalResponse{Approval: approvalPB(a)}, nil
}

func (s *Server) ListApprovals(ctx context.Context, req *backofficev1.ListApprovalsRequest) (*backofficev1.ListApprovalsResponse, error) {
	var status domain.Status
	for k, v := range statusToPB {
		if v == req.GetStatus() {
			status = k
		}
	}
	list, err := s.app.Approvals(ctx, status, int(req.GetLimit()))
	if err != nil {
		return nil, toStatus(err)
	}
	out := &backofficev1.ListApprovalsResponse{}
	for _, a := range list {
		out.Approvals = append(out.Approvals, approvalPB(a))
	}
	return out, nil
}

func (s *Server) DecideApproval(ctx context.Context, req *backofficev1.DecideApprovalRequest) (*backofficev1.DecideApprovalResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "id is not a UUID")
	}
	a, err := s.app.Decide(ctx, id, req.GetApproverId(), req.GetApprove(), req.GetReason())
	if err != nil {
		return nil, toStatus(err)
	}
	return &backofficev1.DecideApprovalResponse{Approval: approvalPB(a)}, nil
}
