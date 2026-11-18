package httpapi

import (
	"net/http"

	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
)

type addressJSON struct {
	PostalCode string `json:"postal_code"`
	Street     string `json:"street"`
	Number     string `json:"number"`
	Complement string `json:"complement"`
	District   string `json:"district"`
	City       string `json:"city"`
	State      string `json:"state"`
}

func profileJSON(p *userv1.Profile) map[string]any {
	a := p.GetAddress()
	return map[string]any{"full_name": p.GetFullName(), "cpf": p.GetCpfMasked(), "birth_date": p.GetBirthDate(),
		"phone": p.GetPhone(), "kyc_status": p.GetKycStatus(), "kyc_reason": p.GetKycReason(),
		"address": addressJSON{PostalCode: a.GetPostalCode(), Street: a.GetStreet(), Number: a.GetNumber(),
			Complement: a.GetComplement(), District: a.GetDistrict(), City: a.GetCity(), State: a.GetState()}}
}

func (a *API) getProfile(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Users.GetProfile(r.Context(), &userv1.GetProfileRequest{CustomerId: customerID(r.Context())})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, profileJSON(resp.GetProfile()))
}

// putProfile creates or updates the caller's profile (onboarding step 1).
func (a *API) putProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FullName  string      `json:"full_name"`
		CPF       string      `json:"cpf"`
		BirthDate string      `json:"birth_date"`
		Phone     string      `json:"phone"`
		Address   addressJSON `json:"address"`
	}
	if !decode(w, r, &req) {
		return
	}
	ad := req.Address
	resp, err := a.c.Users.UpsertProfile(r.Context(), &userv1.UpsertProfileRequest{CustomerId: customerID(r.Context()),
		FullName: req.FullName, Cpf: req.CPF, BirthDate: req.BirthDate, Phone: req.Phone,
		Address: &userv1.Address{PostalCode: ad.PostalCode, Street: ad.Street, Number: ad.Number, Complement: ad.Complement,
			District: ad.District, City: ad.City, State: ad.State}})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, profileJSON(resp.GetProfile()))
}

// submitKYC starts identity verification (onboarding step 2).
func (a *API) submitKYC(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Users.SubmitKyc(r.Context(), &userv1.SubmitKycRequest{CustomerId: customerID(r.Context())})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, profileJSON(resp.GetProfile()))
}

// reviewKYC (sandbox) plays the analyst for a verification in manual review.
func (a *API) reviewKYC(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Users.ReviewKyc(r.Context(), &userv1.ReviewKycRequest{CustomerId: customerID(r.Context()), Approve: req.Approve, Reason: req.Reason})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, profileJSON(resp.GetProfile()))
}
