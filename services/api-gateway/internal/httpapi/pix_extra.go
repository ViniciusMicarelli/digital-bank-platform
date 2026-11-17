package httpapi

import (
	"net/http"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
)

// ---- dynamic QR (public) ----

func (a *API) qrPayload(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Pix.GetDynamicQrPayload(r.Context(), &pixv1.GetDynamicQrPayloadRequest{Txid: r.PathValue("txid")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	w.Header().Set("Content-Type", "application/jose")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(resp.GetJws()))
}

func (a *API) qrJWKS(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Pix.GetQrJwks(r.Context(), &pixv1.GetQrJwksRequest{})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	keys := []map[string]string{}
	for _, k := range resp.GetKeys() {
		keys = append(keys, map[string]string{"kid": k.GetKid(), "kty": k.GetKty(), "crv": k.GetCrv(), "x": k.GetX(), "alg": k.GetAlg(), "use": "sig"})
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

// ---- schedules ----

var frequencies = map[string]pixv1.Frequency{
	"ONCE": pixv1.Frequency_FREQUENCY_ONCE, "WEEKLY": pixv1.Frequency_FREQUENCY_WEEKLY, "MONTHLY": pixv1.Frequency_FREQUENCY_MONTHLY,
}

func scheduleJSON(s *pixv1.Schedule) map[string]any {
	out := map[string]any{
		"id": s.GetId(), "key": s.GetKey(), "amount": moneyJSON(s.GetAmount()), "description": s.GetDescription(),
		"start_date": s.GetStartDate(), "frequency": s.GetFrequency().String()[len("FREQUENCY_"):],
		"occurrences": s.GetOccurrences(), "executed": s.GetExecuted(), "next_date": s.GetNextDate(), "status": s.GetStatus(),
	}
	if s.GetMandateId() != "" {
		out["mandate_id"] = s.GetMandateId()
	}
	return out
}

func (a *API) createSchedule(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		Key         string `json:"key"`
		Amount      string `json:"amount"`
		Description string `json:"description"`
		StartDate   string `json:"start_date"`
		Frequency   string `json:"frequency"`
		Occurrences int32  `json:"occurrences"`
	}
	if !decode(w, r, &req) {
		return
	}
	amount, ok := parseBRL(w, r, req.Amount)
	if !ok {
		return
	}
	if req.Frequency == "" {
		req.Frequency = "ONCE"
	}
	resp, err := a.c.Pix.CreateSchedule(r.Context(), &pixv1.CreateScheduleRequest{
		IdempotencyKey: key, AccountId: acc.GetId(), Key: req.Key, Amount: amount, Description: req.Description,
		StartDate: req.StartDate, Frequency: frequencies[req.Frequency], Occurrences: req.Occurrences,
	})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, scheduleJSON(resp.GetSchedule()))
}

func (a *API) listSchedules(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Pix.ListSchedules(r.Context(), &pixv1.ListSchedulesRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, s := range resp.GetSchedules() {
		out = append(out, scheduleJSON(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": out})
}

func (a *API) cancelSchedule(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Pix.CancelSchedule(r.Context(), &pixv1.CancelScheduleRequest{AccountId: acc.GetId(), ScheduleId: r.PathValue("scheduleID")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, scheduleJSON(resp.GetSchedule()))
}

func (a *API) sandboxRunSchedules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AsOf string `json:"as_of"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Pix.RunDueSchedules(r.Context(), &pixv1.RunDueSchedulesRequest{AsOf: req.AsOf})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"executed": resp.GetExecuted()})
}

// ---- Pix Automático ----

func mandateJSON(m *pixv1.Mandate) map[string]any {
	return map[string]any{
		"id": m.GetId(), "receiver_account_id": m.GetReceiverAccountId(), "receiver_key": m.GetReceiverKey(),
		"receiver_name": m.GetReceiverName(), "payer_account_id": m.GetPayerAccountId(),
		"max_amount": moneyJSON(m.GetMaxAmount()), "frequency": m.GetFrequency().String()[len("FREQUENCY_"):],
		"description": m.GetDescription(), "status": m.GetStatus(),
	}
}

func (a *API) requestMandate(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	var req struct {
		ReceiverKey string `json:"receiver_key"`
		PayerKey    string `json:"payer_key"`
		MaxAmount   string `json:"max_amount"`
		Frequency   string `json:"frequency"`
		Description string `json:"description"`
	}
	if !decode(w, r, &req) {
		return
	}
	max, ok := parseBRL(w, r, req.MaxAmount)
	if !ok {
		return
	}
	resp, err := a.c.Pix.RequestMandate(r.Context(), &pixv1.RequestMandateRequest{
		ReceiverAccountId: acc.GetId(), ReceiverKey: req.ReceiverKey, PayerKey: req.PayerKey, MaxAmount: max,
		Frequency: frequencies[req.Frequency], Description: req.Description,
	})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, mandateJSON(resp.GetMandate()))
}

func (a *API) listMandates(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Pix.ListMandates(r.Context(), &pixv1.ListMandatesRequest{AccountId: acc.GetId()})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, m := range resp.GetMandates() {
		out = append(out, mandateJSON(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"mandates": out})
}

func (a *API) decideMandate(approve bool) func(http.ResponseWriter, *http.Request, *accountv1.Account) {
	return func(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
		resp, err := a.c.Pix.DecideMandate(r.Context(), &pixv1.DecideMandateRequest{PayerAccountId: acc.GetId(), MandateId: r.PathValue("mandateID"), Approve: approve})
		if err != nil {
			writeGRPCError(w, r.Context(), err)
			return
		}
		writeJSON(w, http.StatusOK, mandateJSON(resp.GetMandate()))
	}
}

func (a *API) revokeMandate(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	resp, err := a.c.Pix.RevokeMandate(r.Context(), &pixv1.RevokeMandateRequest{AccountId: acc.GetId(), MandateId: r.PathValue("mandateID")})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, mandateJSON(resp.GetMandate()))
}

func (a *API) chargeMandate(w http.ResponseWriter, r *http.Request, acc *accountv1.Account) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		Amount  string `json:"amount"`
		DueDate string `json:"due_date"`
	}
	if !decode(w, r, &req) {
		return
	}
	amount, ok := parseBRL(w, r, req.Amount)
	if !ok {
		return
	}
	resp, err := a.c.Pix.ChargeMandate(r.Context(), &pixv1.ChargeMandateRequest{
		IdempotencyKey: key, ReceiverAccountId: acc.GetId(), MandateId: r.PathValue("mandateID"), Amount: amount, DueDate: req.DueDate,
	})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusCreated, scheduleJSON(resp.GetSchedule()))
}
