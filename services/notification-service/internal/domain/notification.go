// Package domain turns domain events from the other services into customer
// notifications, and decides which channels deliver them.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidArgument = errors.New("invalid argument")
)

type Category string

const (
	CategoryPix      Category = "PIX"
	CategoryTransfer Category = "TRANSFER"
	CategoryCard     Category = "CARD"
	CategoryCredit   Category = "CREDIT"
	CategoryBoleto   Category = "BOLETO"
	CategoryAccount  Category = "ACCOUNT"
)

type Channel string

const (
	InApp Channel = "IN_APP"
	Push  Channel = "PUSH"
	Email Channel = "EMAIL"
)

// Draft is a notification about one account, before we know its customer.
type Draft struct {
	EventID   string
	AccountID uuid.UUID
	Category  Category
	Title     string
	Body      string
	// Important ones (security, money that failed to move, bills) also go
	// by e-mail when the customer allows it.
	Important  bool
	OccurredAt time.Time
}

type Notification struct {
	ID         uuid.UUID
	CustomerID string
	Draft
	Channels []Channel
	ReadAt   *time.Time
	Created  time.Time
}

type Preferences struct {
	Push  bool
	Email bool
}

// DefaultPreferences: push on, e-mail off.
var DefaultPreferences = Preferences{Push: true}

// Channels decides how a notification is delivered: always in the app; by
// push if allowed; by e-mail if allowed and important.
func (p Preferences) Channels(d Draft) []Channel {
	ch := []Channel{InApp}
	if p.Push {
		ch = append(ch, Push)
	}
	if p.Email && d.Important {
		ch = append(ch, Email)
	}
	return ch
}

// BRL formats centavos the Brazilian way: R$ 1.234,56.
func BRL(minor int64) string {
	sign := ""
	if minor < 0 {
		sign, minor = "-", -minor
	}
	units := strconv.FormatInt(minor/100, 10)
	var sb strings.Builder
	for i, r := range units {
		if i > 0 && (len(units)-i)%3 == 0 {
			sb.WriteByte('.')
		}
		sb.WriteRune(r)
	}
	return fmt.Sprintf("%sR$ %s,%02d", sign, sb.String(), minor%100)
}

// Event is the envelope fields the mapping needs.
type Event struct {
	ID         string
	Type       string
	OccurredAt time.Time
	Payload    json.RawMessage
}

// payload is the union of the fields used from every event type.
type payload struct {
	AccountID       string            `json:"account_id"`
	Direction       string            `json:"direction"`
	Status          string            `json:"status"`
	CounterpartName string            `json:"counterparty_name"`
	AmountMinor     int64             `json:"amount_minor"`
	Amount          int64             `json:"amount"`
	FailureReason   string            `json:"failure_reason"`
	Reason          string            `json:"reason"`
	MerchantName    string            `json:"merchant_name"`
	Function        string            `json:"function"`
	ResponseCode    string            `json:"response_code"`
	Total           int64             `json:"total"`
	Remaining       int64             `json:"remaining"`
	DueDate         string            `json:"due_date"`
	Unpaid          int64             `json:"unpaid"`
	Fine            int64             `json:"fine"`
	Interest        int64             `json:"interest"`
	PayeeName       string            `json:"payee_name"`
	ScheduledFor    string            `json:"scheduled_for"`
	Type            string            `json:"type"`
	ToAccountID     string            `json:"to_account_id"`
	FromAccountID   string            `json:"from_account_id"`
	Metadata        map[string]string `json:"metadata"`
}

func brDate(s string) string {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return s
	}
	return d.Format("02/01/2006")
}

// MaxAge: events older than this are not notified (a consumer catching up
// after an outage should not announce yesterday's news).
const MaxAge = 24 * time.Hour

// FromEvent maps an event to the notifications it causes (none for most).
// now is used to drop stale events.
func FromEvent(ev Event, now time.Time) ([]Draft, error) {
	if !ev.OccurredAt.IsZero() && now.Sub(ev.OccurredAt) > MaxAge {
		return nil, nil
	}
	var p payload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return nil, fmt.Errorf("decode %s: %w", ev.Type, err)
	}
	account := p.AccountID
	d := Draft{EventID: ev.ID, OccurredAt: ev.OccurredAt}
	switch ev.Type {
	case "pix.transfer.received":
		d.Category, d.Title = CategoryPix, "Pix recebido"
		d.Body = fmt.Sprintf("Você recebeu %s de %s.", BRL(p.AmountMinor), p.CounterpartName)
	case "pix.transfer.completed":
		if p.Direction != "OUTGOING" {
			return nil, nil
		}
		d.Category, d.Title = CategoryPix, "Pix enviado"
		d.Body = fmt.Sprintf("Seu Pix de %s para %s foi concluído.", BRL(p.AmountMinor), p.CounterpartName)
	case "pix.transfer.failed":
		d.Category, d.Title, d.Important = CategoryPix, "Pix não concluído", true
		d.Body = fmt.Sprintf("Seu Pix de %s para %s não foi concluído (%s). O valor não saiu da sua conta.", BRL(p.AmountMinor), p.CounterpartName, p.FailureReason)
	case "transaction.completed":
		// Transfers between accounts here, other than Pix (pix-service notifies those).
		if p.Type != "INTERNAL_TRANSFER" || p.Metadata["end_to_end_id"] != "" {
			return nil, nil
		}
		account = p.ToAccountID
		d.Category, d.Title = CategoryTransfer, "Transferência recebida"
		d.Body = fmt.Sprintf("Você recebeu uma transferência de %s.", BRL(p.AmountMinor))
	case "card.authorization.approved":
		d.Category, d.Title = CategoryCard, "Compra aprovada"
		kind := "débito"
		if p.Function == "CREDIT" {
			kind = "crédito"
		}
		d.Body = fmt.Sprintf("Compra de %s em %s no %s.", BRL(p.AmountMinor), p.MerchantName, kind)
	case "card.authorization.declined":
		if p.AccountID == "" || p.AccountID == uuid.Nil.String() {
			return nil, nil // unknown card: nobody to tell
		}
		d.Category, d.Title, d.Important = CategoryCard, "Compra recusada", true
		d.Body = fmt.Sprintf("Uma compra de %s em %s foi recusada: %s.", BRL(p.AmountMinor), p.MerchantName, p.Reason)
	case "credit.invoice.closed":
		if p.Remaining <= 0 {
			return nil, nil
		}
		d.Category, d.Title, d.Important = CategoryCredit, "Sua fatura fechou", true
		d.Body = fmt.Sprintf("Fatura de %s com vencimento em %s.", BRL(p.Remaining), brDate(p.DueDate))
	case "credit.invoice.overdue":
		d.Category, d.Title, d.Important = CategoryCredit, "Fatura em atraso", true
		d.Body = fmt.Sprintf("%s não foram pagos e foram para a próxima fatura, com multa de %s e juros de %s.",
			BRL(p.Unpaid), BRL(p.Fine), BRL(p.Interest))
	case "credit.invoice.payment.completed":
		d.Category, d.Title = CategoryCredit, "Pagamento de fatura recebido"
		d.Body = fmt.Sprintf("Recebemos o pagamento de %s da sua fatura.", BRL(p.Amount))
	case "boleto.payment.completed":
		d.Category, d.Title = CategoryBoleto, "Boleto pago"
		d.Body = fmt.Sprintf("Pagamento de %s para %s concluído.", BRL(p.Amount), p.PayeeName)
	case "boleto.payment.scheduled":
		d.Category, d.Title = CategoryBoleto, "Boleto agendado"
		d.Body = fmt.Sprintf("Pagamento de %s para %s agendado para %s.", BRL(p.Amount), p.PayeeName, brDate(p.ScheduledFor))
	case "boleto.payment.failed", "boleto.payment.rejected":
		d.Category, d.Title, d.Important = CategoryBoleto, "Boleto não pago", true
		d.Body = fmt.Sprintf("O pagamento para %s não foi feito (%s).", p.PayeeName, p.FailureReason)
	case "account.blocked":
		d.Category, d.Title, d.Important = CategoryAccount, "Conta bloqueada", true
		d.Body = "Sua conta foi bloqueada. Fale com a gente se não reconhece isso."
	case "account.unblocked":
		d.Category, d.Title, d.Important = CategoryAccount, "Conta desbloqueada", true
		d.Body = "Sua conta voltou a funcionar normalmente."
	default:
		return nil, nil
	}
	id, err := uuid.Parse(account)
	if err != nil || id == uuid.Nil {
		return nil, nil // nobody to notify (e.g. an event from before the field existed); retrying would not help
	}
	d.AccountID = id
	return []Draft{d}, nil
}
