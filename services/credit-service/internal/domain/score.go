package domain

import "time"

// ---- credit score ----
//
// The bank's own behaviour score, 0..1000, from how the customer uses the
// card here: paying invoices on time, how much of the limit is used, how
// long the relationship is, and whether invoices had to be split. It is
// explainable: every factor reports the points it adds or takes.

const (
	ScoreBase = 500
	ScoreMax  = 1000
	// ScoreWindow is how far back invoices and plans count.
	ScoreWindow = 12 // months
)

// ScoreInputs is what the score is computed from.
type ScoreInputs struct {
	Line Line
	// Invoices due in the window: paid with no charges, and that were late
	// (charges were applied).
	OnTime, Late int
	// OverdueNow: an invoice is overdue right now.
	OverdueNow bool
	// Plans: invoices split into installments in the window.
	Plans int
}

type ScoreFactor string

const (
	FactorPaymentHistory ScoreFactor = "PAYMENT_HISTORY"
	FactorLatePayments   ScoreFactor = "LATE_PAYMENTS"
	FactorOverdueNow     ScoreFactor = "OVERDUE_NOW"
	FactorUtilization    ScoreFactor = "UTILIZATION"
	FactorRelationship   ScoreFactor = "RELATIONSHIP"
	FactorInvoicePlans   ScoreFactor = "INVOICE_PLANS"
)

// Factor is one reason for the score: Points added (or, negative, taken)
// and the Value behind them (a count, a percentage or months).
type Factor struct {
	Code   ScoreFactor
	Points int
	Value  int
}

type ScoreBand string

const (
	BandLow       ScoreBand = "LOW"       // 0..299
	BandFair      ScoreBand = "FAIR"      // 300..499
	BandGood      ScoreBand = "GOOD"      // 500..749
	BandExcellent ScoreBand = "EXCELLENT" // 750..1000
)

type Score struct {
	Value   int
	Band    ScoreBand
	Factors []Factor
}

// Factor is the factor with that code.
func (s Score) Factor(code ScoreFactor) Factor {
	for _, f := range s.Factors {
		if f.Code == code {
			return f
		}
	}
	return Factor{Code: code}
}

func Band(score int) ScoreBand {
	switch {
	case score >= 750:
		return BandExcellent
	case score >= 500:
		return BandGood
	case score >= 300:
		return BandFair
	}
	return BandLow
}

// MonthsBetween counts the full months from a to b.
func MonthsBetween(a, b time.Time) int {
	m := (b.Year()-a.Year())*12 + int(b.Month()-a.Month())
	if b.Day() < a.Day() {
		m--
	}
	return max(m, 0)
}

// ComputeScore explains and totals the score. Every factor is always
// listed, so the customer sees what helps even when it adds nothing yet.
func ComputeScore(in ScoreInputs, now time.Time) Score {
	usage := 0
	if in.Line.Limit > 0 {
		usage = int((in.Line.Used*100 + in.Line.Limit - 1) / in.Line.Limit) // rounded up
	}
	utilization := map[bool]int{true: 80}[usage <= 30]
	switch {
	case usage > 90:
		utilization = -100
	case usage > 60:
		utilization = -40
	case usage > 30:
		utilization = 30
	}
	months := MonthsBetween(in.Line.CreatedAt, now)
	overdue := 0
	if in.OverdueNow {
		overdue = -200
	}
	factors := []Factor{
		{FactorPaymentHistory, min(in.OnTime*30, 240), in.OnTime},
		{FactorLatePayments, -120 * in.Late, in.Late},
		{FactorOverdueNow, overdue, map[bool]int{true: 1}[in.OverdueNow]},
		{FactorUtilization, utilization, usage},
		{FactorRelationship, min(months*5, 120), months},
		{FactorInvoicePlans, -60 * in.Plans, in.Plans},
	}
	total := ScoreBase
	for _, f := range factors {
		total += f.Points
	}
	total = min(max(total, 0), ScoreMax)
	return Score{Value: total, Band: Band(total), Factors: factors}
}

// AutoIncreaseFor is the highest limit the automatic analysis may grant for
// a score, in basis points of the current limit (15000 = 150%; 0: an analyst
// decides any increase).
func AutoIncreaseFor(score int) int {
	switch {
	case score >= 750:
		return 20_000 // +100%
	case score >= 450:
		return AutoIncreaseBP // +50%
	}
	return 0
}

// MinScore is the score below which an increase is refused outright.
const MinScore = 250

// AutoIncreaseBy is how much more than the current limit the automatic
// analysis may grant, in basis points (5000 = +50%).
func AutoIncreaseBy(score int) int { return max(AutoIncreaseFor(score)-10_000, 0) }
