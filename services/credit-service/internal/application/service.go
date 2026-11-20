// Package application runs credit lines: purchase holds for card-service,
// invoice cycles (closing, overdue charges) and invoice payments.
//
// Money moves in the ledger through transaction-service:
//   - clearing a credit purchase: credit-receivables -> card clearing
//   - paying an invoice: customer account -> credit-receivables
//   - fine and interest: credit-receivables -> credit-revenue
//
// so credit-receivables is always minus what customers owe.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/ports"
)

const (
	source = "credit-service"
	topic  = "bank.credit.events"
)

// Ledger accounts (see ledger-service migrations).
var (
	CardClearingAccountID      = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	CreditReceivablesAccountID = uuid.MustParse("00000000-0000-0000-0000-000000000004")
	CreditRevenueAccountID     = uuid.MustParse("00000000-0000-0000-0000-000000000005")
)

type Service struct {
	store        ports.Store
	accounts     ports.Accounts
	txs          ports.Transactions
	log          *slog.Logger
	now          func() time.Time
	defaultLimit int64
}

func New(store ports.Store, accounts ports.Accounts, txs ports.Transactions, log *slog.Logger, defaultLimit int64) *Service {
	if defaultLimit <= 0 {
		defaultLimit = domain.DefaultLimit
	}
	return &Service{store: store, accounts: accounts, txs: txs, log: log, now: time.Now, defaultLimit: defaultLimit}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) today() time.Time { return domain.Today(s.now()) }

func event(ctx context.Context, eventType, key string, payload any) (outbox.Message, error) {
	ev, err := outbox.NewEvent(ctx, source, eventType, payload)
	if err != nil {
		return outbox.Message{}, err
	}
	return outbox.Message{Topic: topic, Key: key, Event: ev}, nil
}

func publish(ctx context.Context, tx ports.Tx, eventType, key string, payload any) error {
	m, err := event(ctx, eventType, key, payload)
	if err != nil {
		return err
	}
	return tx.Publish(m)
}

// ---- lines ----

// OpenLine opens the credit line of an active account, with its first
// invoice. Same key: the same line; another key: CREDIT_LINE_EXISTS.
func (s *Service) OpenLine(ctx context.Context, key string, account uuid.UUID, closingDay int) (domain.Line, error) {
	if closingDay == 0 {
		closingDay = 10
	}
	l, err := domain.NewLine(key, account, closingDay, s.defaultLimit)
	if err != nil {
		return domain.Line{}, err
	}
	l.CreatedAt = s.now()
	if existing, err := s.store.LineByAccount(ctx, account); err == nil {
		return sameLine(existing, key)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Line{}, err
	}
	active, err := s.accounts.Active(ctx, account)
	if err != nil {
		return domain.Line{}, err
	}
	if !active {
		return domain.Line{}, domain.ErrAccountInactive
	}
	inserted := false
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		if inserted, err = tx.InsertLine(l); err != nil || !inserted {
			return err
		}
		if err := tx.InsertInvoice(domain.NewInvoice(l.ID, domain.FirstClosing(closingDay, s.today()))); err != nil {
			return err
		}
		return publish(ctx, tx, "credit.line.opened", l.ID.String(), map[string]any{
			"line_id": l.ID, "account_id": account, "limit": l.Limit, "closing_day": closingDay})
	})
	if err != nil {
		return domain.Line{}, err
	}
	if !inserted { // lost a race with another request
		existing, err := s.store.LineByAccount(ctx, account)
		if err != nil {
			return domain.Line{}, err
		}
		return sameLine(existing, key)
	}
	return s.store.LineByAccount(ctx, account)
}

func sameLine(l domain.Line, key string) (domain.Line, error) {
	if l.IdempotencyKey != key {
		return domain.Line{}, domain.Refuse("CREDIT_LINE_EXISTS", "the account already has a credit line")
	}
	return l, nil
}

func (s *Service) Line(ctx context.Context, account uuid.UUID) (domain.Line, error) {
	return s.store.LineByAccount(ctx, account)
}

func (s *Service) Invoices(ctx context.Context, account uuid.UUID) ([]domain.Invoice, error) {
	l, err := s.store.LineByAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return s.store.Invoices(ctx, l.ID)
}

// InvoiceDetail is an invoice, its items and the account that pays it.
type InvoiceDetail struct {
	Invoice   domain.Invoice
	AccountID uuid.UUID
	Items     []domain.Item
}

func (s *Service) Invoice(ctx context.Context, id uuid.UUID) (InvoiceDetail, error) {
	inv, err := s.store.Invoice(ctx, id)
	if err != nil {
		return InvoiceDetail{}, err
	}
	l, err := s.store.Line(ctx, inv.LineID)
	if err != nil {
		return InvoiceDetail{}, err
	}
	items, err := s.store.Items(ctx, id)
	return InvoiceDetail{Invoice: inv, AccountID: l.AccountID, Items: items}, err
}

// ---- holds (card-facing) ----

// Reserve holds amount of the account's limit for a card authorization.
// Idempotent on key; refusals are FAILED holds with a reason.
func (s *Service) Reserve(ctx context.Context, key string, account uuid.UUID, amount int64, installments int, description string, md map[string]string) (domain.Hold, error) {
	if key == "" || amount <= 0 {
		return domain.Hold{}, fmt.Errorf("%w: key and a positive amount are required", domain.ErrInvalidArgument)
	}
	if installments == 0 {
		installments = 1
	}
	if err := domain.ValidInstallments(amount, installments); err != nil {
		return domain.Hold{}, err
	}
	var h domain.Hold
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		existing, err := tx.HoldByKey(key)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.AccountID != account || existing.Amount != amount || existing.Installments != installments {
				return fmt.Errorf("%w: %q", domain.ErrConflict, key)
			}
			h = *existing
			return nil
		}
		h = domain.Hold{ID: uuid.Must(uuid.NewV7()), IdempotencyKey: key, AccountID: account, Status: domain.HoldReserved,
			Amount: amount, Description: description, Metadata: md, CreatedAt: s.now(), Installments: installments}
		l, err := tx.LockLineByAccount(account)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			h.Status, h.FailureReason = domain.HoldFailed, domain.ReasonNoCreditLine
		case err != nil:
			return err
		default:
			h.LineID = l.ID
			if reason := l.Reserve(amount); reason != "" {
				h.Status, h.FailureReason = domain.HoldFailed, reason
			} else if err := tx.SaveLine(l); err != nil {
				return err
			}
		}
		return tx.InsertHold(h)
	})
	return h, err
}

// Capture settles a held purchase (for its amount or less): the bank pays the
// card network and the purchase goes into the open invoice.
func (s *Service) Capture(ctx context.Context, id uuid.UUID, amount *int64) (domain.Hold, error) {
	h, err := s.store.Hold(ctx, id)
	if err != nil {
		return domain.Hold{}, err
	}
	switch h.Status {
	case domain.HoldCaptured:
		return h, nil
	case domain.HoldReserved:
	default:
		return domain.Hold{}, fmt.Errorf("%w: cannot capture a %s hold", domain.ErrInvalidState, h.Status)
	}
	captured := h.Amount
	if amount != nil {
		if *amount <= 0 || *amount > h.Amount {
			return domain.Hold{}, fmt.Errorf("%w: capture must be positive and at most the held amount", domain.ErrInvalidArgument)
		}
		captured = *amount
	}
	md := map[string]string{"hold_id": h.ID.String(), "account_id": h.AccountID.String()}
	res, err := s.txs.Post(ctx, h.CaptureKey(), "CREDIT_CARD_PURCHASE", CreditReceivablesAccountID, CardClearingAccountID, captured, h.Description, md)
	if err != nil {
		return domain.Hold{}, err
	}
	if res.Status != "COMPLETED" {
		return domain.Hold{}, fmt.Errorf("ledger posting of the purchase: %s %s", res.Status, res.FailureReason)
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		l, err := tx.LockLine(h.LineID)
		if err != nil {
			return err
		}
		h, err = tx.LockHold(id)
		if err != nil || h.Status != domain.HoldReserved {
			return err
		}
		inv, err := s.openInvoice(ctx, tx, l)
		if err != nil {
			return err
		}
		h.Status, h.Captured = domain.HoldCaptured, captured
		l.Free(h.Amount - captured) // the debt stays; only the difference is freed
		// Installments: the first goes on this invoice, the others wait for
		// the next ones (the whole debt is on the limit already).
		parts := domain.Split(captured, max(h.Installments, 1))
		inv.Total += parts[0]
		desc := h.Description
		if len(parts) > 1 {
			plan := make([]domain.Installment, len(parts))
			for k, amt := range parts {
				plan[k] = domain.Installment{ID: uuid.Must(uuid.NewV7()), LineID: l.ID, HoldID: h.ID, Number: k + 1, Total: len(parts),
					Amount: amt, Description: h.Description, CreatedAt: s.now()}
			}
			plan[0].InvoiceID = &inv.ID
			if err := tx.InsertInstallments(plan); err != nil {
				return err
			}
			desc = plan[0].Label()
		}
		if err := tx.AddItem(domain.NewItem(inv.ID, domain.ItemPurchase, parts[0], desc, h.ID.String(), s.now())); err != nil {
			return err
		}
		if err := errors.Join(tx.SaveHold(h), tx.SaveLine(l), tx.SaveInvoice(inv)); err != nil {
			return err
		}
		return publish(ctx, tx, "credit.purchase.captured", h.ID.String(), map[string]any{
			"hold_id": h.ID, "account_id": h.AccountID, "invoice_id": inv.ID, "amount": captured,
			"description": h.Description, "metadata": h.Metadata})
	})
	return h, err
}

// Release gives a held amount back (merchant reversal, timeout, expiry).
func (s *Service) Release(ctx context.Context, id uuid.UUID) (domain.Hold, error) {
	h, err := s.store.Hold(ctx, id)
	if err != nil {
		return domain.Hold{}, err
	}
	if h.Status != domain.HoldReserved {
		if h.Status == domain.HoldCaptured {
			return domain.Hold{}, fmt.Errorf("%w: the purchase was already captured", domain.ErrInvalidState)
		}
		return h, nil
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		l, err := tx.LockLine(h.LineID)
		if err != nil {
			return err
		}
		if h, err = tx.LockHold(id); err != nil || h.Status != domain.HoldReserved {
			return err
		}
		h.Status = domain.HoldReleased
		l.Free(h.Amount)
		return errors.Join(tx.SaveHold(h), tx.SaveLine(l))
	})
	return h, err
}

// Refund credits a card refund to the open invoice and frees the limit; the
// money comes back from the card network (card clearing -> receivables).
// Idempotent on key: the ledger posting and the credit happen once.
func (s *Service) Refund(ctx context.Context, key string, account uuid.UUID, amount int64, description string, md map[string]string) (domain.Refund, error) {
	if key == "" || amount <= 0 {
		return domain.Refund{}, fmt.Errorf("%w: key and a positive amount are required", domain.ErrInvalidArgument)
	}
	if r, err := s.store.RefundByKey(ctx, key); err != nil || r != nil {
		if r != nil && (r.AccountID != account || r.Amount != amount) {
			return domain.Refund{}, fmt.Errorf("%w: %q", domain.ErrConflict, key)
		}
		return derefRefund(r), err
	}
	line, err := s.store.LineByAccount(ctx, account)
	if err != nil {
		return domain.Refund{}, err
	}
	if md == nil {
		md = map[string]string{}
	}
	md["account_id"] = account.String()
	res, err := s.txs.Post(ctx, "credit-refund:"+key, "CREDIT_CARD_REFUND", CardClearingAccountID, CreditReceivablesAccountID, amount, description, md)
	if err != nil {
		return domain.Refund{}, err
	}
	if res.Status != "COMPLETED" {
		return domain.Refund{}, fmt.Errorf("ledger posting of the refund: %s %s", res.Status, res.FailureReason)
	}
	r := domain.Refund{ID: uuid.Must(uuid.NewV7()), IdempotencyKey: key, LineID: line.ID, AccountID: account, Amount: amount,
		Description: description, CreatedAt: s.now()}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		l, err := tx.LockLine(line.ID)
		if err != nil {
			return err
		}
		inv, err := s.openInvoice(ctx, tx, l)
		if err != nil {
			return err
		}
		r.InvoiceID = inv.ID
		inserted, err := tx.InsertRefund(r)
		if err != nil || !inserted {
			return err // a concurrent retry did it
		}
		inv.Total -= amount
		l.Free(amount)
		if err := tx.AddItem(domain.NewItem(inv.ID, domain.ItemRefund, amount, description, r.ID.String(), s.now())); err != nil {
			return err
		}
		if err := errors.Join(tx.SaveInvoice(inv), tx.SaveLine(l)); err != nil {
			return err
		}
		return publish(ctx, tx, "credit.refund.completed", r.ID.String(), map[string]any{
			"refund_id": r.ID, "account_id": account, "invoice_id": inv.ID, "amount": amount, "description": description, "metadata": md})
	})
	if err != nil {
		return domain.Refund{}, err
	}
	if stored, err := s.store.RefundByKey(ctx, key); err == nil && stored != nil {
		return *stored, nil
	}
	return r, nil
}

func derefRefund(r *domain.Refund) domain.Refund {
	if r == nil {
		return domain.Refund{}
	}
	return *r
}

// UpcomingInstallments is what installment purchases will still bill.
func (s *Service) UpcomingInstallments(ctx context.Context, line domain.Line) (int64, error) {
	return s.store.UpcomingInstallments(ctx, line.ID)
}

// InstallmentPlans lists the account's purchases with installments to come.
func (s *Service) InstallmentPlans(ctx context.Context, account uuid.UUID) ([]domain.InstallmentPlan, error) {
	l, err := s.store.LineByAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return s.store.InstallmentPlans(ctx, l.ID)
}

// ---- invoice cycle ----

// openInvoice returns the locked OPEN invoice of l, closing any whose
// closing date has passed first (the line is locked by the caller).
func (s *Service) openInvoice(ctx context.Context, tx ports.Tx, l domain.Line) (domain.Invoice, error) {
	inv, err := tx.LockOpenInvoice(l.ID)
	if err != nil {
		return domain.Invoice{}, err
	}
	for today := s.today(); !inv.ClosingDate.After(today); {
		if err := inv.Close(); err != nil {
			return domain.Invoice{}, err
		}
		// Refunds larger than what was owed: the credit moves to the next invoice.
		credit := max(-inv.Remaining(), 0)
		inv.Total += credit
		if err := tx.SaveInvoice(inv); err != nil {
			return domain.Invoice{}, err
		}
		if err := publish(ctx, tx, "credit.invoice.closed", inv.ID.String(), map[string]any{
			"invoice_id": inv.ID, "account_id": l.AccountID, "total": inv.Total, "remaining": inv.Remaining(),
			"due_date": inv.DueDate.Format(time.DateOnly), "status": inv.Status}); err != nil {
			return domain.Invoice{}, err
		}
		next := inv.Next()
		next.Total -= credit
		if err := tx.InsertInvoice(next); err != nil {
			return domain.Invoice{}, err
		}
		// The next installment of every installment purchase goes on the new invoice.
		due, err := tx.NextInstallments(l.ID)
		if err != nil {
			return domain.Invoice{}, err
		}
		for _, it := range due {
			if err := errors.Join(
				tx.AddItem(domain.NewItem(next.ID, domain.ItemPurchase, it.Amount, it.Label(), "installment:"+it.ID.String(), s.now())),
				tx.BillInstallment(it.ID, next.ID),
			); err != nil {
				return domain.Invoice{}, err
			}
			next.Total += it.Amount
		}
		if len(due) > 0 {
			if err := tx.SaveInvoice(next); err != nil {
				return domain.Invoice{}, err
			}
		}
		if credit > 0 {
			if err := errors.Join(
				tx.AddItem(domain.NewItem(inv.ID, domain.ItemCreditCarriedOut, credit, "Crédito transferido para a próxima fatura", next.ID.String(), s.now())),
				tx.AddItem(domain.NewItem(next.ID, domain.ItemCreditCarriedIn, credit, "Crédito da fatura anterior", inv.ID.String(), s.now())),
			); err != nil {
				return domain.Invoice{}, err
			}
		}
		inv = next
	}
	return inv, nil
}

// cycle closes the open invoice of a line when due and moves unpaid closed
// invoices past their due date to the next one, with a fine and interest.
func (s *Service) cycle(ctx context.Context, lineID uuid.UUID) error {
	return s.store.InTx(ctx, func(tx ports.Tx) error {
		l, err := tx.LockLine(lineID)
		if err != nil {
			return err
		}
		next, err := s.openInvoice(ctx, tx, l)
		if err != nil {
			return err
		}
		closed, err := tx.LockClosedInvoices(l.ID)
		if err != nil {
			return err
		}
		today := s.today()
		for _, inv := range closed {
			if !inv.Overdue(today) {
				continue
			}
			remaining := inv.Remaining()
			fine, interest := domain.Charges(remaining, l.InterestRateBP)
			inv.Status = domain.InvoiceOverdue
			ref := inv.ID.String()
			due := inv.DueDate.Format("02/01")
			for _, it := range []domain.Item{
				domain.NewItem(next.ID, domain.ItemBalanceCarried, remaining, "Saldo da fatura vencida em "+due, ref, s.now()),
				domain.NewItem(next.ID, domain.ItemFine, fine, "Multa por atraso (2%)", ref, s.now()),
				domain.NewItem(next.ID, domain.ItemInterest, interest, "Juros do rotativo", ref, s.now()),
			} {
				if it.Amount == 0 {
					continue
				}
				if err := tx.AddItem(it); err != nil {
					return err
				}
			}
			next.Total += remaining + fine + interest
			l.Used += fine + interest // the debt grows; it may pass the limit
			if err := errors.Join(tx.SaveInvoice(inv), tx.SetCharges(inv.ID, fine+interest)); err != nil {
				return err
			}
			if err := publish(ctx, tx, "credit.invoice.overdue", inv.ID.String(), map[string]any{
				"invoice_id": inv.ID, "account_id": l.AccountID, "unpaid": remaining, "fine": fine, "interest": interest,
				"carried_to": next.ID}); err != nil {
				return err
			}
		}
		return errors.Join(tx.SaveInvoice(next), tx.SaveLine(l))
	})
}

// postCharges books fines and interest in the ledger (after the commit that
// recorded them; idempotent per invoice, retried until it succeeds).
func (s *Service) postCharges(ctx context.Context, inv domain.Invoice) error {
	if inv.Charges > 0 {
		res, err := s.txs.Post(ctx, "credit-charges:"+inv.ID.String(), "CREDIT_CHARGES", CreditReceivablesAccountID,
			CreditRevenueAccountID, inv.Charges, "Encargos da fatura", map[string]string{"invoice_id": inv.ID.String()})
		if err != nil {
			return err
		}
		if res.Status != "COMPLETED" {
			return fmt.Errorf("ledger posting of charges: %s %s", res.Status, res.FailureReason)
		}
	}
	return s.store.MarkChargesPosted(ctx, inv.ID)
}

// AdvanceCycle (sandbox) closes the open invoice today (CLOSE) or moves the
// due date of closed invoices to yesterday (EXPIRE), then runs the cycle.
func (s *Service) AdvanceCycle(ctx context.Context, account uuid.UUID, action string) error {
	l, err := s.store.LineByAccount(ctx, account)
	if err != nil {
		return err
	}
	today := s.today()
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		if _, err := tx.LockLine(l.ID); err != nil {
			return err
		}
		switch action {
		case "CLOSE":
			inv, err := tx.LockOpenInvoice(l.ID)
			if err != nil {
				return err
			}
			inv.ClosingDate, inv.DueDate = today, today.AddDate(0, 0, domain.DueAfterClosingDays)
			return tx.SaveInvoice(inv)
		case "EXPIRE":
			closed, err := tx.LockClosedInvoices(l.ID)
			if err != nil {
				return err
			}
			for _, inv := range closed {
				inv.DueDate = today.AddDate(0, 0, -1)
				if err := tx.SaveInvoice(inv); err != nil {
					return err
				}
			}
			return nil
		}
		return fmt.Errorf("%w: action must be CLOSE or EXPIRE", domain.ErrInvalidArgument)
	})
	if err != nil {
		return err
	}
	if err := s.cycle(ctx, l.ID); err != nil {
		return err
	}
	return s.postAllCharges(ctx)
}

func (s *Service) postAllCharges(ctx context.Context) error {
	invs, err := s.store.UnpostedCharges(ctx, 100)
	if err != nil {
		return err
	}
	var errs []error
	for _, inv := range invs {
		errs = append(errs, s.postCharges(ctx, inv))
	}
	return errors.Join(errs...)
}

// ---- limit changes ----

// RequestLimit asks for a new limit (idempotent on key): decreases and
// increases the automatic analysis can grant apply at once; refusals and
// requests in review are recorded too.
// Score is the account's behaviour score, explained factor by factor.
func (s *Service) Score(ctx context.Context, account uuid.UUID) (domain.Score, error) {
	line, err := s.store.LineByAccount(ctx, account)
	if err != nil {
		return domain.Score{}, err
	}
	return s.scoreLine(ctx, line)
}

func (s *Service) scoreLine(ctx context.Context, line domain.Line) (domain.Score, error) {
	now := s.now()
	in, err := s.store.ScoreInputs(ctx, line, now.AddDate(0, -domain.ScoreWindow, 0))
	if err != nil {
		return domain.Score{}, err
	}
	return domain.ComputeScore(in, now), nil
}

func (s *Service) RequestLimit(ctx context.Context, key string, account uuid.UUID, amount int64) (domain.LimitRequest, error) {
	if key == "" {
		return domain.LimitRequest{}, fmt.Errorf("%w: idempotency key is required", domain.ErrInvalidArgument)
	}
	if existing, err := s.store.LimitRequestByKey(ctx, key); err != nil || existing != nil {
		if existing != nil && (existing.AccountID != account || existing.Requested != amount) {
			return domain.LimitRequest{}, fmt.Errorf("%w: %q", domain.ErrConflict, key)
		}
		if existing == nil {
			return domain.LimitRequest{}, err
		}
		return *existing, nil
	}
	line, err := s.store.LineByAccount(ctx, account)
	if err != nil {
		return domain.LimitRequest{}, err
	}
	score, err := s.scoreLine(ctx, line)
	if err != nil {
		return domain.LimitRequest{}, err
	}
	overdue := score.Factor(domain.FactorOverdueNow).Value > 0
	var r domain.LimitRequest
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		l, err := tx.LockLine(line.ID)
		if err != nil {
			return err
		}
		status, reason, err := domain.AnalyzeLimit(l, amount, overdue, score.Value)
		if err != nil {
			return err
		}
		now := s.now()
		r = domain.LimitRequest{ID: uuid.Must(uuid.NewV7()), IdempotencyKey: key, LineID: l.ID, AccountID: l.AccountID,
			CurrentLimit: l.Limit, Requested: amount, Status: status, Reason: reason, Score: score.Value, CreatedAt: now}
		if status != domain.LimitInReview {
			r.DecidedBy, r.DecidedAt = "automatic", &now
		}
		if status == domain.LimitApproved {
			r.Granted = amount
		}
		if err := tx.InsertLimitRequest(r); err != nil {
			return err
		}
		return s.applyLimit(ctx, tx, l, r)
	})
	return r, err
}

// applyLimit changes the line when a request is approved, and tells the
// customer how it ended.
func (s *Service) applyLimit(ctx context.Context, tx ports.Tx, l domain.Line, r domain.LimitRequest) error {
	if r.Status == domain.LimitApproved {
		l.Limit = r.Granted
		if err := tx.SaveLine(l); err != nil {
			return err
		}
	}
	return publish(ctx, tx, "credit.limit."+map[domain.LimitStatus]string{
		domain.LimitApproved: "changed", domain.LimitRejected: "rejected", domain.LimitInReview: "in_review"}[r.Status],
		r.ID.String(), map[string]any{"request_id": r.ID, "account_id": r.AccountID, "previous_limit": r.CurrentLimit,
			"requested": r.Requested, "granted": r.Granted, "reason": r.Reason, "status": r.Status})
}

// LimitRequests lists requests of an account (or, with uuid.Nil, all of
// them, for the back office) filtered by status.
func (s *Service) LimitRequests(ctx context.Context, account uuid.UUID, status domain.LimitStatus) ([]domain.LimitRequest, error) {
	var line uuid.UUID
	if account != uuid.Nil {
		l, err := s.store.LineByAccount(ctx, account)
		if err != nil {
			return nil, err
		}
		line = l.ID
	}
	return s.store.LimitRequests(ctx, line, status, 100)
}

// DecideLimit is an analyst's decision on a request in review.
func (s *Service) DecideLimit(ctx context.Context, id uuid.UUID, analyst string, approve bool, granted int64, reason string) (domain.LimitRequest, error) {
	r, err := s.store.LimitRequest(ctx, id)
	if err != nil {
		return r, err
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		l, err := tx.LockLine(r.LineID)
		if err != nil {
			return err
		}
		if r, err = tx.LockLimitRequest(id); err != nil {
			return err
		}
		if err := r.Decide(analyst, approve, granted, reason, s.now()); err != nil {
			return err
		}
		if err := tx.SaveLimitRequest(r); err != nil {
			return err
		}
		return s.applyLimit(ctx, tx, l, r)
	})
	return r, err
}

// ---- invoice plans ----

// PlanQuotes are the ways a closed invoice can be split (2 to 12 installments).
func (s *Service) PlanQuotes(ctx context.Context, invoiceID uuid.UUID) ([]domain.PlanQuote, error) {
	inv, err := s.store.Invoice(ctx, invoiceID)
	if err != nil {
		return nil, err
	}
	if err := inv.Plannable(); err != nil {
		return nil, err
	}
	out := make([]domain.PlanQuote, 0, domain.MaxPlanInstallments-1)
	for n := domain.MinPlanInstallments; n <= domain.MaxPlanInstallments; n++ {
		out = append(out, domain.QuotePlan(inv.Remaining(), n, domain.InvoicePlanRateBP))
	}
	return out, nil
}

// PlanInvoice splits a closed invoice into n installments with interest:
// the invoice is settled by the plan, the first installment goes on the open
// invoice and the others on the next ones; the interest is added to the debt
// (and booked in the ledger after the commit). Idempotent on key.
func (s *Service) PlanInvoice(ctx context.Context, key string, invoiceID uuid.UUID, n int) (domain.InvoicePlan, error) {
	if key == "" {
		return domain.InvoicePlan{}, fmt.Errorf("%w: idempotency key is required", domain.ErrInvalidArgument)
	}
	if n < domain.MinPlanInstallments || n > domain.MaxPlanInstallments {
		return domain.InvoicePlan{}, fmt.Errorf("%w: from 2 to 12 installments", domain.ErrInvalidArgument)
	}
	if existing, err := s.store.InvoicePlanByKey(ctx, key); err != nil || existing != nil {
		if existing != nil && (existing.InvoiceID != invoiceID || existing.Installments != n) {
			return domain.InvoicePlan{}, fmt.Errorf("%w: %q", domain.ErrConflict, key)
		}
		if existing == nil {
			return domain.InvoicePlan{}, err
		}
		return *existing, nil
	}
	inv, err := s.store.Invoice(ctx, invoiceID)
	if err != nil {
		return domain.InvoicePlan{}, err
	}
	var plan domain.InvoicePlan
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		l, err := tx.LockLine(inv.LineID)
		if err != nil {
			return err
		}
		closed, err := tx.LockInvoice(invoiceID)
		if err != nil {
			return err
		}
		if err := closed.Plannable(); err != nil {
			return err
		}
		q := domain.QuotePlan(closed.Remaining(), n, domain.InvoicePlanRateBP)
		plan = domain.InvoicePlan{ID: uuid.Must(uuid.NewV7()), IdempotencyKey: key, InvoiceID: closed.ID, LineID: l.ID,
			Principal: closed.Remaining(), Installments: n, Installment: q.Installment, MonthlyBP: q.MonthlyBP, CreatedAt: s.now()}
		if err := tx.InsertInvoicePlan(plan); err != nil {
			return err
		}
		// The closed invoice is settled by the plan.
		if err := tx.AddItem(domain.NewItem(closed.ID, domain.ItemInvoicePlan, plan.Principal,
			fmt.Sprintf("Fatura parcelada em %dx", n), plan.ID.String(), s.now())); err != nil {
			return err
		}
		closed.Paid += plan.Principal
		closed.Status = domain.InvoicePaid
		// The installments: the first on the open invoice, the rest to come.
		open, err := s.openInvoice(ctx, tx, l)
		if err != nil {
			return err
		}
		list := make([]domain.Installment, n)
		for k := range list {
			list[k] = domain.Installment{ID: uuid.Must(uuid.NewV7()), LineID: l.ID, PlanID: plan.ID, Number: k + 1, Total: n,
				Amount: plan.Installment, Description: "Parcelamento da fatura", CreatedAt: s.now()}
		}
		list[0].InvoiceID = &open.ID
		if err := tx.InsertInstallments(list); err != nil {
			return err
		}
		if err := tx.AddItem(domain.NewItem(open.ID, domain.ItemPurchase, list[0].Amount, list[0].Label(), "installment:"+list[0].ID.String(), s.now())); err != nil {
			return err
		}
		open.Total += list[0].Amount
		l.Used += plan.Interest() // the debt grows by the interest
		if err := errors.Join(tx.SaveInvoice(closed), tx.SaveInvoice(open), tx.SaveLine(l)); err != nil {
			return err
		}
		return publish(ctx, tx, "credit.invoice.planned", plan.ID.String(), map[string]any{
			"plan_id": plan.ID, "invoice_id": closed.ID, "account_id": l.AccountID, "principal": plan.Principal,
			"installments": n, "installment": plan.Installment, "interest": plan.Interest()})
	})
	if err != nil {
		return domain.InvoicePlan{}, err
	}
	if err := s.postPlanInterest(ctx, plan); err != nil {
		s.log.WarnContext(ctx, "plan interest will be posted later", "plan_id", plan.ID, "error", err)
	}
	return plan, nil
}

// postPlanInterest books a plan's interest as revenue (idempotent per plan).
func (s *Service) postPlanInterest(ctx context.Context, p domain.InvoicePlan) error {
	if p.Interest() > 0 {
		res, err := s.txs.Post(ctx, p.InterestKey(), "CREDIT_CHARGES", CreditReceivablesAccountID, CreditRevenueAccountID, p.Interest(),
			"Juros do parcelamento da fatura", map[string]string{"plan_id": p.ID.String()})
		if err != nil {
			return err
		}
		if res.Status != "COMPLETED" {
			return fmt.Errorf("ledger posting of plan interest: %s %s", res.Status, res.FailureReason)
		}
	}
	return s.store.MarkPlanInterestPosted(ctx, p.ID)
}

func (s *Service) postAllPlanInterest(ctx context.Context) error {
	plans, err := s.store.UnpostedPlanInterest(ctx, 100)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range plans {
		errs = append(errs, s.postPlanInterest(ctx, p))
	}
	return errors.Join(errs...)
}

// ---- payments ----

// Pay pays amount of an invoice from the line's checking account.
// Idempotent on key; a refused transfer is a FAILED payment with a reason.
func (s *Service) Pay(ctx context.Context, key string, invoiceID uuid.UUID, amount int64) (domain.Payment, error) {
	if key == "" {
		return domain.Payment{}, fmt.Errorf("%w: idempotency key is required", domain.ErrInvalidArgument)
	}
	inv, err := s.store.Invoice(ctx, invoiceID)
	if err != nil {
		return domain.Payment{}, err
	}
	var p domain.Payment
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		existing, err := tx.PaymentByKey(key)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.InvoiceID != invoiceID || existing.Amount != amount {
				return fmt.Errorf("%w: %q", domain.ErrConflict, key)
			}
			p = *existing
			return nil
		}
		l, err := tx.LockLine(inv.LineID)
		if err != nil {
			return err
		}
		inv, err := tx.LockInvoice(invoiceID)
		if err != nil {
			return err
		}
		if err := inv.StartPayment(amount); err != nil {
			return err
		}
		p = domain.Payment{ID: uuid.Must(uuid.NewV7()), IdempotencyKey: key, InvoiceID: inv.ID, AccountID: l.AccountID,
			Status: domain.PaymentPending, Amount: amount, CreatedAt: s.now()}
		return errors.Join(tx.SaveInvoice(inv), tx.InsertPayment(p))
	})
	if err != nil || p.Status != domain.PaymentPending {
		return p, err
	}
	return s.settle(ctx, p)
}

// settle moves the money of a PENDING payment (idempotent transfer) and
// applies the outcome. On a technical error the payment stays PENDING and
// the worker retries it.
func (s *Service) settle(ctx context.Context, p domain.Payment) (domain.Payment, error) {
	res, err := s.txs.Post(ctx, p.TransferKey(), "CREDIT_INVOICE_PAYMENT", p.AccountID, CreditReceivablesAccountID, p.Amount,
		"Pagamento da fatura do cartão", map[string]string{"invoice_id": p.InvoiceID.String(), "counterparty_name": "Fatura do cartão"})
	if err != nil {
		return p, err
	}
	if res.Status != "COMPLETED" && res.Status != "FAILED" {
		return p, fmt.Errorf("unexpected transfer status %s", res.Status)
	}
	ok := res.Status == "COMPLETED"
	inv, err := s.store.Invoice(ctx, p.InvoiceID)
	if err != nil {
		return p, err
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		l, err := tx.LockLine(inv.LineID)
		if err != nil {
			return err
		}
		inv, err := tx.LockInvoice(p.InvoiceID)
		if err != nil {
			return err
		}
		if p, err = tx.LockPayment(p.ID); err != nil || p.Status != domain.PaymentPending {
			return err
		}
		inv.SettlePayment(p.Amount, ok)
		eventType := "credit.invoice.payment.failed"
		p.Status, p.FailureReason = domain.PaymentFailed, res.FailureReason
		if ok {
			eventType = "credit.invoice.payment.completed"
			p.Status, p.FailureReason = domain.PaymentCompleted, ""
			l.Free(p.Amount)
			if err := tx.AddItem(domain.NewItem(inv.ID, domain.ItemPayment, p.Amount, "Pagamento recebido", p.ID.String(), s.now())); err != nil {
				return err
			}
		}
		if err := errors.Join(tx.SavePayment(p), tx.SaveInvoice(inv), tx.SaveLine(l)); err != nil {
			return err
		}
		payload := map[string]any{"payment_id": p.ID, "invoice_id": inv.ID, "account_id": l.AccountID, "amount": p.Amount,
			"failure_reason": p.FailureReason, "invoice_status": inv.Status, "remaining": inv.Remaining()}
		return publish(ctx, tx, eventType, p.ID.String(), payload)
	})
	return p, err
}

// ---- workers ----

// RunOnce closes and expires invoices, posts pending charges, releases
// holds older than the authorization lifetime and retries stuck payments.
func (s *Service) RunOnce(ctx context.Context) {
	lines, err := s.store.LinesDue(ctx, s.today(), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "lines due", "error", err)
	}
	for _, id := range lines {
		if err := s.cycle(ctx, id); err != nil {
			s.log.WarnContext(ctx, "invoice cycle failed", "line_id", id, "error", err)
		}
	}
	if err := s.postAllCharges(ctx); err != nil {
		s.log.WarnContext(ctx, "posting charges failed", "error", err)
	}
	if err := s.postAllPlanInterest(ctx); err != nil {
		s.log.WarnContext(ctx, "posting invoice plan interest failed", "error", err)
	}
	holds, err := s.store.ExpiredHolds(ctx, s.now().Add(-domain.HoldLifetime), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "expired holds", "error", err)
	}
	for _, id := range holds {
		if _, err := s.Release(ctx, id); err != nil {
			s.log.WarnContext(ctx, "releasing an expired hold failed", "hold_id", id, "error", err)
		}
	}
	payments, err := s.store.PendingPayments(ctx, s.now().Add(-30*time.Second), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "pending payments", "error", err)
	}
	for _, p := range payments {
		if _, err := s.settle(ctx, p); err != nil {
			s.log.WarnContext(ctx, "payment still pending", "payment_id", p.ID, "error", err)
		}
	}
}

func (s *Service) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RunOnce(ctx)
		}
	}
}
