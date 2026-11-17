//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"

	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
)

// waitForFeed polls until the account's feed has n items (eventual consistency).
func waitForFeed(t *testing.T, s stack, account string, n int) []*statementv1.FeedItem {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		resp, err := s.feed.ListFeed(ctx(t), &statementv1.ListFeedRequest{AccountId: account})
		if err == nil && len(resp.GetItems()) >= n {
			return resp.GetItems()
		}
		if time.Now().After(deadline) {
			t.Fatalf("feed for %s: %v, %v", account, resp.GetItems(), err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestFeed(t *testing.T) {
	s := connect(t)
	alice := openAccount(t, s, "Alice Souza", "529.982.247-25")
	bob := openAccount(t, s, "Bob Lima", "111.444.777-35")
	deposit(t, s, alice, 100_00)

	if _, err := s.tx.CreateTransfer(ctx(t), &txv1.CreateTransferRequest{
		IdempotencyKey: uuid.NewString(), FromAccountId: alice, ToAccountId: bob, Amount: brl(12_34), Description: "Livros",
	}); err != nil {
		t.Fatal(err)
	}
	sendPix(t, s, alice, "maria@outrobanco.com.br", 5_00)

	items := waitForFeed(t, s, alice, 2)
	if items[0].GetType() != "PIX_OUT" || items[0].GetCounterpartyName() != "Maria Oliveira" || items[0].GetEndToEndId() == "" {
		t.Errorf("newest = %v", items[0])
	}
	if items[1].GetType() != "INTERNAL_TRANSFER" || items[1].GetCounterpartyName() != "Bob Lima" ||
		items[1].GetDirection() != statementv1.Direction_DIRECTION_DEBIT || items[1].GetAmount().GetMinorUnits() != 12_34 {
		t.Errorf("transfer = %v", items[1])
	}
	bobItems := waitForFeed(t, s, bob, 1)
	if bobItems[0].GetDirection() != statementv1.Direction_DIRECTION_CREDIT || bobItems[0].GetCounterpartyName() != "Alice Souza" {
		t.Errorf("bob = %v", bobItems[0])
	}
}
