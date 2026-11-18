//go:build e2e

package e2e

import (
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
)

func sendPix(t *testing.T, s stack, from, key string, minor int64) *pixv1.Transfer {
	t.Helper()
	resp, err := s.pix.CreateTransfer(ctx(t), &pixv1.CreateTransferRequest{
		IdempotencyKey: uuid.NewString(), FromAccountId: from, Key: key, Amount: brl(minor), Description: "e2e",
	})
	if err != nil {
		t.Fatalf("pix to %s: %v", key, err)
	}
	return resp.GetTransfer()
}

func TestPix(t *testing.T) {
	s := connect(t)
	alice := openAccount(t, s, "Alice Souza", "529.982.247-25")
	bob := openAccount(t, s, "Bob Lima", "111.444.777-35")
	deposit(t, s, alice, 200_00)

	bobKey := "bob." + uuid.NewString()[:8] + "@example.com"
	if _, err := s.pix.RegisterKey(ctx(t), &pixv1.RegisterKeyRequest{AccountId: bob, Type: pixv1.KeyType_KEY_TYPE_EMAIL, Value: bobKey}); err != nil {
		t.Fatal(err)
	}
	look, err := s.pix.LookupKey(ctx(t), &pixv1.LookupKeyRequest{Key: bobKey})
	if err != nil || look.GetOwner().GetName() != "Bob Lima" || !regexp.MustCompile(`^\*\*\*\.\d{3}\.\d{3}-\*\*$`).MatchString(look.GetOwner().GetDocumentMasked()) {
		t.Fatalf("lookup = %v, %v", look, err)
	}

	t.Run("to a customer of this bank", func(t *testing.T) {
		tr := sendPix(t, s, alice, bobKey, 50_00)
		if tr.GetStatus() != pixv1.TransferStatus_TRANSFER_STATUS_COMPLETED {
			t.Fatalf("status = %s (%s)", tr.GetStatus(), tr.GetFailureReason())
		}
		if got := available(t, s, bob); got != 50_00 {
			t.Fatalf("bob = %d", got)
		}
	})

	t.Run("to another bank", func(t *testing.T) {
		tr := sendPix(t, s, alice, "maria@outrobanco.com.br", 30_00)
		if tr.GetStatus() != pixv1.TransferStatus_TRANSFER_STATUS_COMPLETED || len(tr.GetEndToEndId()) != 32 {
			t.Fatalf("transfer = %v", tr)
		}
	})

	t.Run("refused by the receiving bank gives the money back", func(t *testing.T) {
		before := available(t, s, alice)
		tr := sendPix(t, s, alice, "recusa@outrobanco.com.br", 10_00)
		if tr.GetStatus() != pixv1.TransferStatus_TRANSFER_STATUS_FAILED || tr.GetFailureReason() != "RECEIVER_BANK_REJECTED" {
			t.Fatalf("transfer = %v", tr)
		}
		if after := available(t, s, alice); after != before {
			t.Fatalf("alice %d -> %d", before, after)
		}
	})

	t.Run("not enough money is rejected", func(t *testing.T) {
		tr := sendPix(t, s, alice, "maria@outrobanco.com.br", 1_000_00)
		if tr.GetStatus() != pixv1.TransferStatus_TRANSFER_STATUS_REJECTED {
			t.Fatalf("transfer = %v", tr)
		}
	})

	t.Run("incoming from another bank", func(t *testing.T) {
		e2e := "E12345678" + time.Now().UTC().Format("200601021504") + uuid.NewString()[:8] + "abc"
		in, err := s.pix.SimulateIncoming(ctx(t), &pixv1.SimulateIncomingRequest{
			EndToEndId: e2e, Key: bobKey, Amount: brl(7_00),
			Payer: &pixv1.Party{Name: "Maria Oliveira", Ispb: "12345678", BankName: "Banco Exemplo"},
		})
		if err != nil || in.GetTransfer().GetStatus() != pixv1.TransferStatus_TRANSFER_STATUS_COMPLETED {
			t.Fatalf("incoming = %v, %v", in, err)
		}
		if got := available(t, s, bob); got != 57_00 {
			t.Fatalf("bob = %d", got)
		}
	})

	if got := available(t, s, alice); got != 120_00 {
		t.Fatalf("alice final = %d, want 200 - 50 - 30", got)
	}

	t.Run("closing an account removes its keys (Kafka consumer)", func(t *testing.T) {
		carol := openAccount(t, s, "Carol Dias", "529.982.247-25")
		key := "carol." + uuid.NewString()[:8] + "@example.com"
		if _, err := s.pix.RegisterKey(ctx(t), &pixv1.RegisterKeyRequest{AccountId: carol, Type: pixv1.KeyType_KEY_TYPE_EMAIL, Value: key}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.accounts.CloseAccount(ctx(t), &accountv1.CloseAccountRequest{Id: carol}); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(15 * time.Second)
		for {
			keys, err := s.pix.ListKeys(ctx(t), &pixv1.ListKeysRequest{AccountId: carol})
			if err == nil && len(keys.GetKeys()) == 0 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("keys still registered: %v %v", keys, err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	})
}
