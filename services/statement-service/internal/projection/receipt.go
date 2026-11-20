package projection

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned for an unknown item or a code that is not genuine.
var ErrNotFound = errors.New("not found")

// Receipt is a feed item with the account details and its authentication code.
type Receipt struct {
	Item
	HolderName, Branch, Number             string
	CounterpartyBranch, CounterpartyNumber string
	Code                                   string
}

var codeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

const tagSize = 10 // bytes of HMAC in the code (80 bits)

// tag signs what the receipt states, so no field can change under a valid code.
func (p *Projector) tag(it Item) []byte {
	mac := hmac.New(sha256.New, p.receiptKey)
	mac.Write(it.ID[:])
	mac.Write(it.AccountID[:])
	mac.Write(it.TransactionID[:])
	mac.Write([]byte(it.Type + "|" + it.Direction + "|" + it.Currency + "|" + it.CounterpartyName + "|" + it.EndToEndID + "|"))
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(it.AmountMinor)) //nolint:gosec // positive
	binary.BigEndian.PutUint64(b[8:], uint64(it.OccurredAt.UnixMicro()))
	mac.Write(b[:])
	return mac.Sum(nil)[:tagSize]
}

// Code is the item id plus the tag, in groups of four characters.
func (p *Projector) Code(it Item) string {
	raw := codeEncoding.EncodeToString(append(it.ID[:], p.tag(it)...))
	var groups []string
	for i := 0; i < len(raw); i += 4 {
		groups = append(groups, raw[i:min(i+4, len(raw))])
	}
	return strings.Join(groups, "-")
}

// SetReceiptKey sets the key that signs receipts (from the environment).
func (p *Projector) SetReceiptKey(key []byte) { p.receiptKey = key }

const receiptQuery = `
	SELECT f.id, f.account_id, f.transaction_id, f.type, f.direction, f.amount, f.currency,
	       COALESCE(f.counterparty_name, c.holder_name, ''), f.description, f.end_to_end_id, f.occurred_at,
	       COALESCE(a.holder_name, ''), COALESCE(a.branch, ''), COALESCE(a.number, ''),
	       COALESCE(c.branch, ''), COALESCE(c.number, '')
	  FROM feed_items f
	  LEFT JOIN accounts_view a ON a.account_id = f.account_id
	  LEFT JOIN accounts_view c ON c.account_id = f.counterparty_account_id
	 WHERE f.id = $1`

func (p *Projector) receipt(ctx context.Context, id uuid.UUID) (Receipt, error) {
	var r Receipt
	err := p.pool.QueryRow(ctx, receiptQuery, id).Scan(&r.ID, &r.AccountID, &r.TransactionID, &r.Type, &r.Direction, &r.AmountMinor,
		&r.Currency, &r.CounterpartyName, &r.Description, &r.EndToEndID, &r.OccurredAt,
		&r.HolderName, &r.Branch, &r.Number, &r.CounterpartyBranch, &r.CounterpartyNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	r.OccurredAt = r.OccurredAt.UTC()
	r.Code = p.Code(r.Item)
	return r, err
}

// Receipt of an account's feed item.
func (p *Projector) Receipt(ctx context.Context, account, item uuid.UUID) (Receipt, error) {
	r, err := p.receipt(ctx, item)
	if err == nil && r.AccountID != account {
		return Receipt{}, ErrNotFound
	}
	return r, err
}

// Verify checks an authentication code: the item it names must exist and
// the code's tag must match its current fields.
func (p *Projector) Verify(ctx context.Context, code string) (Receipt, error) {
	raw, err := codeEncoding.DecodeString(strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(code)))
	if err != nil || len(raw) != 16+tagSize {
		return Receipt{}, ErrNotFound
	}
	id, _ := uuid.FromBytes(raw[:16])
	r, err := p.receipt(ctx, id)
	if err != nil {
		return Receipt{}, err
	}
	if !hmac.Equal(raw[16:], p.tag(r.Item)) {
		return Receipt{}, ErrNotFound
	}
	return r, nil
}

// MaskName keeps the first name and initials ("Ana S."), for public checks.
func MaskName(name string) string {
	parts := strings.Fields(name)
	if len(parts) <= 1 {
		return name
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += " " + strings.ToUpper(string([]rune(p)[:1])) + "."
	}
	return out
}
