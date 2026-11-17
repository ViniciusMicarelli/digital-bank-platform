// Package grpcapi exposes the notification service over gRPC.
package grpcapi

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	notificationv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/notification/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/domain"
)

type Server struct {
	notificationv1.UnimplementedNotificationServiceServer
	app *application.Service
	// poll re-reads streams even without a NOTIFY (safety net).
	poll time.Duration
}

func NewServer(app *application.Service, poll time.Duration) *Server {
	return &Server{app: app, poll: poll}
}

func customer(id string) error {
	if id == "" {
		return grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "customer_id is required")
	}
	return nil
}

func optionalID(v string) (uuid.UUID, error) {
	if v == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", "ids must be UUIDs")
	}
	return id, nil
}

func pb(n domain.Notification) *notificationv1.Notification {
	out := &notificationv1.Notification{Id: n.ID.String(), AccountId: n.AccountID.String(), Category: string(n.Category),
		Title: n.Title, Body: n.Body, OccurredAt: timestamppb.New(n.OccurredAt), Read: n.ReadAt != nil}
	for _, c := range n.Channels {
		out.Channels = append(out.Channels, string(c))
	}
	return out
}

func internal(err error) error {
	return grpcserver.Error(codes.Unavailable, "RETRY_LATER", err.Error())
}

func (s *Server) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	if err := customer(req.GetCustomerId()); err != nil {
		return nil, err
	}
	before, err := optionalID(req.GetBeforeId())
	if err != nil {
		return nil, err
	}
	list, unread, err := s.app.List(ctx, req.GetCustomerId(), req.GetUnreadOnly(), before, int(req.GetLimit()))
	if err != nil {
		return nil, internal(err)
	}
	resp := &notificationv1.ListNotificationsResponse{UnreadCount: int32(unread)}
	for _, n := range list {
		resp.Notifications = append(resp.Notifications, pb(n))
	}
	return resp, nil
}

func (s *Server) MarkRead(ctx context.Context, req *notificationv1.MarkReadRequest) (*notificationv1.MarkReadResponse, error) {
	if err := customer(req.GetCustomerId()); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(req.GetIds()))
	for _, v := range req.GetIds() {
		id, err := optionalID(v)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	n, err := s.app.MarkRead(ctx, req.GetCustomerId(), ids)
	if err != nil {
		return nil, internal(err)
	}
	return &notificationv1.MarkReadResponse{Marked: int32(n)}, nil
}

func (s *Server) GetPreferences(ctx context.Context, req *notificationv1.GetPreferencesRequest) (*notificationv1.GetPreferencesResponse, error) {
	if err := customer(req.GetCustomerId()); err != nil {
		return nil, err
	}
	p, err := s.app.Preferences(ctx, req.GetCustomerId())
	if err != nil {
		return nil, internal(err)
	}
	return &notificationv1.GetPreferencesResponse{Preferences: &notificationv1.Preferences{Push: p.Push, Email: p.Email}}, nil
}

func (s *Server) UpdatePreferences(ctx context.Context, req *notificationv1.UpdatePreferencesRequest) (*notificationv1.UpdatePreferencesResponse, error) {
	if err := customer(req.GetCustomerId()); err != nil {
		return nil, err
	}
	p := domain.Preferences{Push: req.GetPreferences().GetPush(), Email: req.GetPreferences().GetEmail()}
	if err := s.app.SetPreferences(ctx, req.GetCustomerId(), p); err != nil {
		return nil, internal(err)
	}
	return &notificationv1.UpdatePreferencesResponse{Preferences: req.GetPreferences()}, nil
}

func (s *Server) Subscribe(req *notificationv1.SubscribeRequest, stream notificationv1.NotificationService_SubscribeServer) error {
	if err := customer(req.GetCustomerId()); err != nil {
		return err
	}
	after, err := optionalID(req.GetAfterId())
	if err != nil {
		return err
	}
	return s.app.Stream(stream.Context(), req.GetCustomerId(), after, s.poll, func(n domain.Notification) error {
		return stream.Send(&notificationv1.SubscribeResponse{Notification: pb(n)})
	})
}
