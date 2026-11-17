package grpcapi

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
)

var frequencies = map[pixv1.Frequency]domain.Frequency{
	pixv1.Frequency_FREQUENCY_ONCE: domain.Once, pixv1.Frequency_FREQUENCY_WEEKLY: domain.Weekly, pixv1.Frequency_FREQUENCY_MONTHLY: domain.Monthly,
}

func frequencyPB(f domain.Frequency) pixv1.Frequency {
	for k, v := range frequencies {
		if v == f {
			return k
		}
	}
	return pixv1.Frequency_FREQUENCY_UNSPECIFIED
}

func parseDate(field, v string) (time.Time, error) {
	d, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, grpcserver.Error(codes.InvalidArgument, "INVALID_ARGUMENT", field+" must be YYYY-MM-DD")
	}
	return d, nil
}

func schedulePB(s domain.Schedule) *pixv1.Schedule {
	out := &pixv1.Schedule{
		Id: s.ID.String(), AccountId: s.AccountID.String(), Key: s.Key,
		Amount:      &commonv1.Money{MinorUnits: s.Amount.Minor(), Currency: string(s.Amount.Currency())},
		Description: s.Description, StartDate: s.StartDate.Format("2006-01-02"), Frequency: frequencyPB(s.Frequency),
		Occurrences: int32(s.Occurrences), Executed: int32(s.Executed), NextDate: s.NextDate.Format("2006-01-02"),
		Status: string(s.Status),
	}
	if s.MandateID != uuid.Nil {
		out.MandateId = s.MandateID.String()
	}
	return out
}

func mandatePB(m domain.Mandate) *pixv1.Mandate {
	return &pixv1.Mandate{
		Id: m.ID.String(), ReceiverAccountId: m.ReceiverAccount.String(), ReceiverKey: m.ReceiverKey,
		ReceiverName: m.ReceiverName, PayerAccountId: m.PayerAccount.String(),
		MaxAmount: &commonv1.Money{MinorUnits: m.MaxAmount.Minor(), Currency: "BRL"},
		Frequency: frequencyPB(m.Frequency), Description: m.Description, Status: string(m.Status),
	}
}

func (s *Server) GetDynamicQrPayload(ctx context.Context, req *pixv1.GetDynamicQrPayloadRequest) (*pixv1.GetDynamicQrPayloadResponse, error) {
	jws, err := s.app.DynamicPayload(ctx, req.GetTxid())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.GetDynamicQrPayloadResponse{Jws: jws}, nil
}

func (s *Server) GetQrJwks(context.Context, *pixv1.GetQrJwksRequest) (*pixv1.GetQrJwksResponse, error) {
	resp := &pixv1.GetQrJwksResponse{}
	for _, k := range s.app.QRJWKS() {
		resp.Keys = append(resp.Keys, &pixv1.Jwk{Kid: k.Kid, Kty: k.Kty, Crv: k.Crv, X: k.X, Alg: k.Alg})
	}
	return resp, nil
}

func (s *Server) CreateSchedule(ctx context.Context, req *pixv1.CreateScheduleRequest) (*pixv1.CreateScheduleResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	amount, err := moneyFromPB(req.GetAmount())
	if err != nil {
		return nil, err
	}
	start, err := parseDate("start_date", req.GetStartDate())
	if err != nil {
		return nil, err
	}
	sc, err := s.app.Schedule(ctx, req.GetIdempotencyKey(), account, req.GetKey(), amount, req.GetDescription(),
		start, frequencies[req.GetFrequency()], int(req.GetOccurrences()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.CreateScheduleResponse{Schedule: schedulePB(sc)}, nil
}

func (s *Server) ListSchedules(ctx context.Context, req *pixv1.ListSchedulesRequest) (*pixv1.ListSchedulesResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	list, err := s.app.Schedules(ctx, account)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &pixv1.ListSchedulesResponse{}
	for _, sc := range list {
		resp.Schedules = append(resp.Schedules, schedulePB(sc))
	}
	return resp, nil
}

func (s *Server) CancelSchedule(ctx context.Context, req *pixv1.CancelScheduleRequest) (*pixv1.CancelScheduleResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetScheduleId())
	if err != nil {
		return nil, err
	}
	sc, err := s.app.CancelSchedule(ctx, account, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.CancelScheduleResponse{Schedule: schedulePB(sc)}, nil
}

func (s *Server) RunDueSchedules(ctx context.Context, req *pixv1.RunDueSchedulesRequest) (*pixv1.RunDueSchedulesResponse, error) {
	asOf := time.Now()
	if req.GetAsOf() != "" {
		d, err := parseDate("as_of", req.GetAsOf())
		if err != nil {
			return nil, err
		}
		asOf = time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, domain.Location)
	}
	n, err := s.app.RunDueSchedules(ctx, asOf)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.RunDueSchedulesResponse{Executed: int32(n)}, nil
}

func (s *Server) RequestMandate(ctx context.Context, req *pixv1.RequestMandateRequest) (*pixv1.RequestMandateResponse, error) {
	receiver, err := parseID(req.GetReceiverAccountId())
	if err != nil {
		return nil, err
	}
	max, err := moneyFromPB(req.GetMaxAmount())
	if err != nil {
		return nil, err
	}
	m, err := s.app.RequestMandate(ctx, receiver, req.GetReceiverKey(), req.GetPayerKey(), max, frequencies[req.GetFrequency()], req.GetDescription())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.RequestMandateResponse{Mandate: mandatePB(m)}, nil
}

func (s *Server) DecideMandate(ctx context.Context, req *pixv1.DecideMandateRequest) (*pixv1.DecideMandateResponse, error) {
	payer, err := parseID(req.GetPayerAccountId())
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetMandateId())
	if err != nil {
		return nil, err
	}
	m, err := s.app.DecideMandate(ctx, payer, id, req.GetApprove())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.DecideMandateResponse{Mandate: mandatePB(m)}, nil
}

func (s *Server) RevokeMandate(ctx context.Context, req *pixv1.RevokeMandateRequest) (*pixv1.RevokeMandateResponse, error) {
	by, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetMandateId())
	if err != nil {
		return nil, err
	}
	m, err := s.app.RevokeMandate(ctx, by, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.RevokeMandateResponse{Mandate: mandatePB(m)}, nil
}

func (s *Server) ListMandates(ctx context.Context, req *pixv1.ListMandatesRequest) (*pixv1.ListMandatesResponse, error) {
	account, err := parseID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	list, err := s.app.Mandates(ctx, account)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &pixv1.ListMandatesResponse{}
	for _, m := range list {
		resp.Mandates = append(resp.Mandates, mandatePB(m))
	}
	return resp, nil
}

func (s *Server) ChargeMandate(ctx context.Context, req *pixv1.ChargeMandateRequest) (*pixv1.ChargeMandateResponse, error) {
	receiver, err := parseID(req.GetReceiverAccountId())
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetMandateId())
	if err != nil {
		return nil, err
	}
	amount, err := moneyFromPB(req.GetAmount())
	if err != nil {
		return nil, err
	}
	due, err := parseDate("due_date", req.GetDueDate())
	if err != nil {
		return nil, err
	}
	sc, err := s.app.ChargeMandate(ctx, req.GetIdempotencyKey(), receiver, id, amount, due)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pixv1.ChargeMandateResponse{Schedule: schedulePB(sc)}, nil
}
