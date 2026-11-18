// Package bureau simulates the identity bureau (Receita Federal CPF status,
// sanctions and PEP lists) for development and the sandbox.
package bureau

import (
	"context"
	"strings"

	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/domain"
)

// Simulator decides by CPF prefix: 111 → irregular CPF (rejected), 222 →
// politically exposed person (manual review), anything else → approved.
type Simulator struct{}

func (Simulator) Check(_ context.Context, p domain.Profile) (domain.BureauDecision, string, error) {
	switch {
	case strings.HasPrefix(p.CPF, "111"):
		return domain.BureauReject, "CPF_IRREGULAR", nil
	case strings.HasPrefix(p.CPF, "222"):
		return domain.BureauReview, "PEP", nil
	}
	return domain.BureauApprove, "", nil
}
