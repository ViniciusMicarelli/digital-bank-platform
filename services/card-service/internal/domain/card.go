// Package domain holds the card rules: card numbers, CVV derivation, card
// lifecycle and the authorization decision. It does no I/O.
package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

var (
	ErrInvalidArgument   = errors.New("invalid argument")
	ErrNotFound          = errors.New("not found")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrAccountNotActive  = errors.New("account is not active")
	ErrInvalidState      = errors.New("invalid authorization state")
)

// BIN is this issuer's 6-digit bank identification number. "999999" is not
// assigned to any real issuer.
const BIN = "999999"

// TokenBIN is the range of device / merchant token numbers (never physical cards).
const TokenBIN = "999998"

// NewPAN returns a random 16-digit card number under BIN with a valid Luhn
// check digit.
func NewPAN() string { return newPAN(BIN) }

func newPAN(bin string) string {
	digits := []byte(bin)
	for len(digits) < 15 {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		digits = append(digits, byte('0'+n.Int64()))
	}
	return string(digits) + string(byte('0'+luhnDigit(string(digits))))
}

// luhnDigit computes the check digit for a number without it.
func luhnDigit(s string) int {
	sum := 0
	for i := len(s) - 1; i >= 0; i-- {
		d := int(s[i] - '0')
		if (len(s)-1-i)%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return (10 - sum%10) % 10
}

// ValidPAN checks length, digits and the Luhn checksum.
func ValidPAN(pan string) bool {
	if len(pan) < 13 || len(pan) > 19 {
		return false
	}
	for _, r := range pan {
		if r < '0' || r > '9' {
			return false
		}
	}
	return luhnDigit(pan[:len(pan)-1]) == int(pan[len(pan)-1]-'0')
}

// DeriveCVV computes the card verification value from the issuer's key, the
// card number and expiry, the way an HSM does with its CVK. Nothing needs to
// be stored, and the value cannot be recomputed without the key.
func DeriveCVV(cvk []byte, pan string, month, year int) string {
	m := hmac.New(sha256.New, cvk)
	fmt.Fprintf(m, "%s|%02d%04d", pan, month, year)
	n := binary.BigEndian.Uint32(m.Sum(nil)[:4]) % 1000
	return fmt.Sprintf("%03d", n)
}

type Kind string

const (
	Physical Kind = "PHYSICAL"
	Virtual  Kind = "VIRTUAL"
	// Temporary is a single-use virtual card for one online purchase.
	Temporary Kind = "TEMPORARY"
)

// Temporary cards.
const (
	TemporaryLifetime = 24 * time.Hour
	MaxTemporaryCards = 3
	// ReasonUsed cancels a temporary card after its purchase.
	ReasonUsed = "USED"
)

type Status string

const (
	Active    Status = "ACTIVE"
	Blocked   Status = "BLOCKED"
	Cancelled Status = "CANCELLED"
)

// Reasons used when blocking because of the account.
const ReasonAccountBlocked = "ACCOUNT_BLOCKED"

// Controls are the customer's switches for where the card may be used.
type Controls struct {
	Online, International, Contactless bool
}

// MaxMonthlyLimit bounds the per-card monthly limit (R$ 50.000,00).
const MaxMonthlyLimit = 50_000_00

// Brasilia is the bank's calendar (no daylight saving since 2019).
var Brasilia = time.FixedZone("BRT", -3*60*60)

// MonthStart is the start of now's calendar month in Brasília.
func MonthStart(now time.Time) time.Time {
	y, m, _ := now.In(Brasilia).Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, Brasilia)
}

// WithinMonthlyLimit reports whether a purchase of amount fits the card's
// monthly limit, given what the card already used this month.
func (c Card) WithinMonthlyLimit(used, amount int64) bool {
	return c.MonthlyLimit == 0 || used+amount <= c.MonthlyLimit
}

// SetMonthlyLimit sets (or, with 0, removes) the card's monthly limit.
func (c *Card) SetMonthlyLimit(amount int64) error {
	switch {
	case c.Status == Cancelled:
		return fmt.Errorf("%w: card is cancelled", ErrInvalidTransition)
	case amount < 0 || amount > MaxMonthlyLimit || amount%100 != 0:
		return fmt.Errorf("%w: the monthly limit is in whole reais, up to R$ 50.000,00", ErrInvalidArgument)
	}
	c.MonthlyLimit = amount
	return nil
}

// DefaultControls: international purchases must be enabled explicitly.
var DefaultControls = Controls{Online: true, International: false, Contactless: true}

// Card is everything about a card except its number, which lives in the vault.
type Card struct {
	ID             uuid.UUID
	IdempotencyKey string
	AccountID      uuid.UUID
	Kind           Kind
	Status         Status
	StatusReason   string
	Last4          string
	ExpiryMonth    int
	ExpiryYear     int
	HolderName     string
	Controls       Controls
	// MonthlyLimit caps what the card spends per calendar month (0: none).
	MonthlyLimit int64
	CreatedAt    time.Time
	// ValidUntil is set for temporary cards.
	ValidUntil *time.Time
}

// TemporaryExpired reports whether a temporary card ran out of time.
func (c Card) TemporaryExpired(now time.Time) bool {
	return c.Kind == Temporary && c.ValidUntil != nil && !now.Before(*c.ValidUntil)
}

// NewCard issues a card for an account: physical cards last 5 years,
// virtual ones 1 year. It returns the card and its number (for the vault).
func NewCard(key string, account uuid.UUID, kind Kind, holder string, now time.Time) (Card, string, error) {
	switch {
	case key == "":
		return Card{}, "", fmt.Errorf("%w: idempotency key is required", ErrInvalidArgument)
	case kind != Physical && kind != Virtual && kind != Temporary:
		return Card{}, "", fmt.Errorf("%w: kind must be PHYSICAL, VIRTUAL or TEMPORARY", ErrInvalidArgument)
	}
	exp, controls := now.AddDate(5, 0, 0), DefaultControls
	var validUntil *time.Time
	switch kind {
	case Virtual:
		exp = now.AddDate(1, 0, 0)
	case Temporary: // online only, for one purchase within a day
		exp = now.AddDate(0, 1, 0)
		until := now.Add(TemporaryLifetime)
		validUntil = &until
		controls = Controls{Online: true}
	}
	pan := NewPAN()
	return Card{
		ID: uuid.Must(uuid.NewV7()), IdempotencyKey: key, AccountID: account, Kind: kind, Status: Active,
		Last4: pan[len(pan)-4:], ExpiryMonth: int(exp.Month()), ExpiryYear: exp.Year(),
		HolderName: holder, Controls: controls, ValidUntil: validUntil,
	}, pan, nil
}

// Expired reports whether the card is past the last day of its expiry month.
func (c Card) Expired(now time.Time) bool {
	end := time.Date(c.ExpiryYear, time.Month(c.ExpiryMonth)+1, 1, 0, 0, 0, 0, time.UTC)
	return !now.Before(end)
}

func (c *Card) Block(reason string) error {
	if c.Status != Active {
		return fmt.Errorf("%w: card is %s", ErrInvalidTransition, c.Status)
	}
	c.Status, c.StatusReason = Blocked, reason
	return nil
}

// Unblock reactivates a blocked card, except one blocked because its holder
// reported fraud: that number is compromised, a new card is the way out.
func (c *Card) Unblock() error {
	if c.Status != Blocked {
		return fmt.Errorf("%w: card is %s", ErrInvalidTransition, c.Status)
	}
	if c.StatusReason == ReasonFraudSuspected {
		return Refuse("CARD_COMPROMISED", "a card blocked for fraud cannot be unblocked; ask for a new one")
	}
	c.Status, c.StatusReason = Active, ""
	return nil
}

// Cancel is final (account closed, card lost).
func (c *Card) Cancel(reason string) {
	c.Status, c.StatusReason = Cancelled, reason
}

type Channel string

const (
	Online      Channel = "ONLINE"
	Chip        Channel = "CHIP"
	Contactless Channel = "CONTACTLESS"
)

// ISO 8583-style response codes returned to the network.
const (
	CodeApproved           = "00"
	CodeDoNotHonor         = "05"
	CodeInvalidCard        = "14"
	CodeInsufficientFunds  = "51"
	CodeExpiredCard        = "54"
	CodeNotPermitted       = "57"
	CodeExceedsLimit       = "61"
	CodeRestrictedCard     = "62"
	CodeWrongCVV           = "82"
	CodeIssuerUnavailable  = "91"
	MaxSingleAuthorization = 10_000_00 // R$ 10.000,00: above it, decline (simple fraud rule)
)

// Blocked merchant categories (gambling).
var blockedMCC = map[string]bool{"7995": true, "7801": true, "7802": true}

// Purchase is what the network asks to authorize.
type Purchase struct {
	NetworkReference string
	ExpiryMonth      int
	ExpiryYear       int
	CVV              string
	Amount           money.Money
	MerchantName     string
	MCC              string
	Country          string
	Channel          Channel
	Function         Function
	// Installments: 1 (or 0) paid at once; 2 to 12 on credit, interest free.
	Installments int
}

// Function is what the purchase is charged to.
type Function string

const (
	Debit  Function = "DEBIT"
	Credit Function = "CREDIT"
)

// Validate checks the request is well formed.
func (p Purchase) Validate() error {
	switch {
	case p.NetworkReference == "" || len(p.NetworkReference) > 64:
		return fmt.Errorf("%w: network reference is required", ErrInvalidArgument)
	case !p.Amount.IsPositive() || p.Amount.Currency() != money.BRL:
		return fmt.Errorf("%w: amount must be positive BRL", ErrInvalidArgument)
	case p.MerchantName == "" || len(p.MCC) != 4 || len(p.Country) != 2:
		return fmt.Errorf("%w: merchant name, 4-digit MCC and 2-letter country are required", ErrInvalidArgument)
	case p.Channel != Online && p.Channel != Chip && p.Channel != Contactless:
		return fmt.Errorf("%w: unknown channel", ErrInvalidArgument)
	case p.Function != Debit && p.Function != Credit:
		return fmt.Errorf("%w: function must be DEBIT or CREDIT", ErrInvalidArgument)
	case p.Installments < 0 || p.Installments > 12:
		return fmt.Errorf("%w: installments must be between 1 and 12", ErrInvalidArgument)
	case p.Installments > 1 && p.Function != Credit:
		return fmt.Errorf("%w: only credit purchases can be paid in installments", ErrInvalidArgument)
	case p.Installments > 1 && p.Amount.Minor()/int64(p.Installments) < 5_00:
		return fmt.Errorf("%w: each installment must be at least R$ 5,00", ErrInvalidArgument)
	}
	return nil
}

// Decision is the card-level verdict, before funds are reserved.
type Decision struct {
	Approve bool
	Code    string
	Reason  string
}

func decline(code, reason string) Decision { return Decision{Code: code, Reason: reason} }

// Decide applies the card checks in the order a network expects. The CVV is
// compared with the derived value (constant time); it is required online.
func (c Card) Decide(p Purchase, expectedCVV string, now time.Time) Decision {
	return c.decide(p, expectedCVV, now, nil)
}

// DecideToken authorizes a purchase made with a token: the token's own
// status and expiry apply, and no CVV is needed (wallets send a cryptogram
// that the network validates).
func (c Card) DecideToken(p Purchase, t Token, now time.Time) Decision {
	return c.decide(p, "", now, &t)
}

func (c Card) decide(p Purchase, expectedCVV string, now time.Time, tok *Token) Decision {
	month, year := c.ExpiryMonth, c.ExpiryYear
	if tok != nil {
		if tok.Status != TokenActive {
			return decline(CodeRestrictedCard, "token "+strings.ToLower(string(tok.Status)))
		}
		month, year = tok.ExpiryMonth, tok.ExpiryYear
	}
	switch {
	case c.Status == Cancelled:
		return decline(CodeRestrictedCard, "card cancelled")
	case c.Status == Blocked:
		return decline(CodeRestrictedCard, "card blocked")
	case p.ExpiryMonth != month || p.ExpiryYear != year:
		return decline(CodeExpiredCard, "expiry date does not match")
	case c.Expired(now):
		return decline(CodeExpiredCard, "card expired")
	case c.TemporaryExpired(now):
		return decline(CodeExpiredCard, "temporary card expired")
	case tok == nil && p.Channel == Online && p.CVV == "":
		return decline(CodeWrongCVV, "CVV required for card-not-present purchases")
	case tok == nil && p.CVV != "" && !hmac.Equal([]byte(p.CVV), []byte(expectedCVV)):
		return decline(CodeWrongCVV, "wrong CVV")
	case p.Channel == Online && !c.Controls.Online:
		return decline(CodeNotPermitted, "online purchases disabled")
	case p.Channel == Contactless && !c.Controls.Contactless:
		return decline(CodeNotPermitted, "contactless disabled")
	case p.Country != "BR" && !c.Controls.International:
		return decline(CodeNotPermitted, "international purchases disabled")
	case blockedMCC[p.MCC]:
		return decline(CodeNotPermitted, "merchant category not allowed")
	case p.Amount.Minor() > MaxSingleAuthorization:
		return decline(CodeExceedsLimit, "amount above the single purchase limit")
	}
	return Decision{Approve: true, Code: CodeApproved}
}

// DeclineCodeFor maps a refusal reason from transaction-service to a code.
func DeclineCodeFor(reason string) string {
	switch reason {
	case "INSUFFICIENT_FUNDS":
		return CodeInsufficientFunds
	case "LIMIT_EXCEEDED":
		return CodeExceedsLimit
	case "ACCOUNT_NOT_ACTIVE", "CREDIT_LINE_BLOCKED":
		return CodeRestrictedCard
	case "INSUFFICIENT_LIMIT":
		return CodeInsufficientFunds
	case "NO_CREDIT_LINE":
		return CodeNotPermitted
	}
	return CodeDoNotHonor
}

type AuthStatus string

const (
	Approved AuthStatus = "APPROVED"
	Declined AuthStatus = "DECLINED"
	Cleared  AuthStatus = "CLEARED"
	Reversed AuthStatus = "REVERSED"
	// PendingReversal: we declined to the network after a technical error,
	// but a reservation may exist downstream; a worker cancels it.
	PendingReversal AuthStatus = "PENDING_REVERSAL"
)

// Authorization is the record of one network request.
type Authorization struct {
	ID               uuid.UUID
	NetworkReference string
	CardID           uuid.UUID
	TokenID          uuid.UUID // set when paid with a wallet / card-on-file token
	AccountID        uuid.UUID // not stored; filled from the card when needed
	Status           AuthStatus
	Code             string
	Reason           string
	Purchase         Purchase
	Cleared          int64
	Refunded         int64 // merchant refunds and won disputes
	TransactionID    uuid.UUID
	CreatedAt        time.Time
}

// TransactionKey is the idempotency key of the reservation in transaction-service.
func (a Authorization) TransactionKey() string { return "card:" + a.ID.String() }

type Wallet string

const (
	ApplePay   Wallet = "APPLE_PAY"
	GooglePay  Wallet = "GOOGLE_PAY"
	SamsungPay Wallet = "SAMSUNG_PAY"
	Merchant   Wallet = "MERCHANT" // card on file at a merchant
)

type TokenStatus string

const (
	TokenActive    TokenStatus = "ACTIVE"
	TokenSuspended TokenStatus = "SUSPENDED"
	TokenDeleted   TokenStatus = "DELETED"
)

// Token is a stand-in card number for one wallet or merchant.
type Token struct {
	ID          uuid.UUID
	CardID      uuid.UUID
	Wallet      Wallet
	DeviceName  string
	Status      TokenStatus
	Last4       string
	ExpiryMonth int
	ExpiryYear  int
	CreatedAt   time.Time
}

// NewToken provisions a token for an active card; it shares the card's expiry.
func NewToken(c Card, wallet Wallet, device string) (Token, string, error) {
	if c.Kind == Temporary {
		return Token{}, "", Refuse("TEMPORARY_CARD", "a temporary card cannot go into a wallet")
	}
	switch {
	case c.Status != Active:
		return Token{}, "", fmt.Errorf("%w: card is %s", ErrInvalidTransition, c.Status)
	case wallet != ApplePay && wallet != GooglePay && wallet != SamsungPay && wallet != Merchant:
		return Token{}, "", fmt.Errorf("%w: unknown wallet", ErrInvalidArgument)
	case device == "" || len(device) > 64:
		return Token{}, "", fmt.Errorf("%w: device name is required", ErrInvalidArgument)
	}
	pan := newPAN(TokenBIN)
	return Token{
		ID: uuid.Must(uuid.NewV7()), CardID: c.ID, Wallet: wallet, DeviceName: device, Status: TokenActive,
		Last4: pan[len(pan)-4:], ExpiryMonth: c.ExpiryMonth, ExpiryYear: c.ExpiryYear,
	}, pan, nil
}

// Transition applies SUSPEND, RESUME or DELETE.
func (t *Token) Transition(action string) error {
	switch {
	case t.Status == TokenDeleted:
		return fmt.Errorf("%w: token is deleted", ErrInvalidTransition)
	case action == "SUSPEND" && t.Status == TokenActive:
		t.Status = TokenSuspended
	case action == "RESUME" && t.Status == TokenSuspended:
		t.Status = TokenActive
	case action == "DELETE":
		t.Status = TokenDeleted
	default:
		return fmt.Errorf("%w: cannot %s a %s token", ErrInvalidTransition, strings.ToLower(action), t.Status)
	}
	return nil
}
