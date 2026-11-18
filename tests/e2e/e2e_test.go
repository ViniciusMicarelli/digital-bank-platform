//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
)

const (
	bankFunding   = "00000000-0000-0000-0000-000000000001"
	pixSettlement = "00000000-0000-0000-0000-000000000002"
)

type stack struct {
	ledger   ledgerv1.LedgerServiceClient
	accounts accountv1.AccountServiceClient
	tx       txv1.TransactionServiceClient
	pix      pixv1.PixServiceClient
	feed     statementv1.StatementServiceClient
	users    userv1.UserServiceClient
}

func addr(env, def string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return def
}

func connect(t *testing.T) stack {
	t.Helper()
	dial := func(target string) *grpc.ClientConn {
		conn, err := grpcserver.Dial(target)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	return stack{
		ledger:   ledgerv1.NewLedgerServiceClient(dial(addr("LEDGER_ADDR", "127.0.0.1:9001"))),
		accounts: accountv1.NewAccountServiceClient(dial(addr("ACCOUNT_ADDR", "127.0.0.1:9002"))),
		tx:       txv1.NewTransactionServiceClient(dial(addr("TRANSACTION_ADDR", "127.0.0.1:9003"))),
		pix:      pixv1.NewPixServiceClient(dial(addr("PIX_ADDR", "127.0.0.1:9004"))),
		feed:     statementv1.NewStatementServiceClient(dial(addr("STATEMENT_ADDR", "127.0.0.1:9005"))),
		users:    userv1.NewUserServiceClient(dial(addr("USER_ADDR", "127.0.0.1:9012"))),
	}
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return c
}

func brl(minor int64) *commonv1.Money { return &commonv1.Money{MinorUnits: minor, Currency: "BRL"} }

// openAccount onboards a new customer (profile + KYC in user-service, with
// a fresh CPF) and opens their account.
func openAccount(t *testing.T, s stack, name, _ string) string {
	t.Helper()
	customer := "cust-" + uuid.NewString()[:8]
	if _, err := s.users.UpsertProfile(ctx(t), &userv1.UpsertProfileRequest{CustomerId: customer, FullName: name, Cpf: randomCPF(),
		BirthDate: "1990-05-20", Address: &userv1.Address{PostalCode: "01310100", Street: "Av. Paulista", Number: "1000",
			City: "São Paulo", State: "SP"}}); err != nil {
		t.Fatalf("profile: %v", err)
	}
	if k, err := s.users.SubmitKyc(ctx(t), &userv1.SubmitKycRequest{CustomerId: customer}); err != nil || k.GetProfile().GetKycStatus() != "APPROVED" {
		t.Fatalf("kyc: %v %v", k, err)
	}
	resp, err := s.accounts.OpenAccount(ctx(t), &accountv1.OpenAccountRequest{
		IdempotencyKey: uuid.NewString(), CustomerId: customer, HolderName: name,
	})
	if err != nil {
		t.Fatalf("open account: %v", err)
	}
	return resp.GetAccount().GetId()
}

// deposit simulates money arriving at the bank (e.g. a cash deposit).
func deposit(t *testing.T, s stack, account string, minor int64) {
	t.Helper()
	_, err := s.ledger.PostEntry(ctx(t), &ledgerv1.PostEntryRequest{
		IdempotencyKey: "deposit-" + uuid.NewString(),
		Postings: []*ledgerv1.Posting{
			{AccountId: bankFunding, Amount: brl(-minor)},
			{AccountId: account, Amount: brl(minor)},
		},
		Description: "Deposit",
	})
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}
}

func available(t *testing.T, s stack, account string) int64 {
	t.Helper()
	b, err := s.accounts.GetBalance(ctx(t), &accountv1.GetBalanceRequest{AccountId: account})
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return b.GetAvailable().GetMinorUnits()
}

func TestInternalTransfer(t *testing.T) {
	s := connect(t)
	alice := openAccount(t, s, "Alice Souza", "529.982.247-25")
	bob := openAccount(t, s, "Bob Lima", "111.444.777-35")
	deposit(t, s, alice, 100_00)

	key := uuid.NewString()
	resp, err := s.tx.CreateTransfer(ctx(t), &txv1.CreateTransferRequest{
		IdempotencyKey: key, FromAccountId: alice, ToAccountId: bob, Amount: brl(30_00), Description: "Dinner",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetTransaction().GetStatus(); got != txv1.TransactionStatus_TRANSACTION_STATUS_COMPLETED {
		t.Fatalf("status = %s (%s)", got, resp.GetTransaction().GetFailureReason())
	}
	// Retrying the same request must not move money again.
	if _, err := s.tx.CreateTransfer(ctx(t), &txv1.CreateTransferRequest{
		IdempotencyKey: key, FromAccountId: alice, ToAccountId: bob, Amount: brl(30_00), Description: "Dinner",
	}); err != nil {
		t.Fatal(err)
	}
	if a, b := available(t, s, alice), available(t, s, bob); a != 70_00 || b != 30_00 {
		t.Fatalf("alice=%d bob=%d", a, b)
	}

	// Not enough money: the saga fails and gives the limit back.
	resp, err = s.tx.CreateTransfer(ctx(t), &txv1.CreateTransferRequest{
		IdempotencyKey: uuid.NewString(), FromAccountId: alice, ToAccountId: bob, Amount: brl(70_01),
	})
	if err != nil {
		t.Fatal(err)
	}
	if tx := resp.GetTransaction(); tx.GetStatus() != txv1.TransactionStatus_TRANSACTION_STATUS_FAILED || tx.GetFailureReason() != "INSUFFICIENT_FUNDS" {
		t.Fatalf("overdraw = %s / %s", tx.GetStatus(), tx.GetFailureReason())
	}
	limits, err := s.accounts.GetLimits(ctx(t), &accountv1.GetLimitsRequest{AccountId: alice})
	if err != nil || limits.GetUsage().GetDailyUsedMinorUnits() != 30_00 {
		t.Fatalf("daily usage = %v, %v (the failed transfer must be restored)", limits.GetUsage(), err)
	}
}

func TestTwoPhaseOutgoing(t *testing.T) {
	s := connect(t)
	alice := openAccount(t, s, "Alice Souza", "529.982.247-25")
	deposit(t, s, alice, 50_00)

	reserve := func() *txv1.Transaction {
		resp, err := s.tx.ReserveOutgoing(ctx(t), &txv1.ReserveOutgoingRequest{
			IdempotencyKey: uuid.NewString(), Type: txv1.TransactionType_TRANSACTION_TYPE_PIX_OUT,
			FromAccountId: alice, ToAccountId: pixSettlement, Amount: brl(20_00),
		})
		if err != nil || resp.GetTransaction().GetStatus() != txv1.TransactionStatus_TRANSACTION_STATUS_RESERVED {
			t.Fatalf("reserve = %v, %v", resp, err)
		}
		return resp.GetTransaction()
	}

	first := reserve()
	if got := available(t, s, alice); got != 30_00 {
		t.Fatalf("available after hold = %d", got)
	}
	if _, err := s.tx.CompensateTransaction(ctx(t), &txv1.CompensateTransactionRequest{Id: first.GetId(), Reason: "SPI_REJECTED"}); err != nil {
		t.Fatal(err)
	}
	if got := available(t, s, alice); got != 50_00 {
		t.Fatalf("available after compensation = %d", got)
	}

	second := reserve()
	done, err := s.tx.CompleteTransaction(ctx(t), &txv1.CompleteTransactionRequest{Id: second.GetId()})
	if err != nil || done.GetTransaction().GetStatus() != txv1.TransactionStatus_TRANSACTION_STATUS_COMPLETED {
		t.Fatalf("complete = %v, %v", done, err)
	}
	b, _ := s.accounts.GetBalance(ctx(t), &accountv1.GetBalanceRequest{AccountId: alice})
	if b.GetPosted().GetMinorUnits() != 30_00 || b.GetPending().GetMinorUnits() != 0 {
		t.Fatalf("balance = %v", b)
	}
}
