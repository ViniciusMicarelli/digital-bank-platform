//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc/metadata"

	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/correlation"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
)

// waitForEvent reads topic from the beginning until match returns true.
func waitForEvent(t *testing.T, topic string, match func(outbox.Event) bool) outbox.Event {
	t.Helper()
	client, err := kgo.NewClient(
		kgo.SeedBrokers(addr("KAFKA_ADDR", "127.0.0.1:9094")),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	deadline, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for {
		fetches := client.PollFetches(deadline)
		if deadline.Err() != nil {
			t.Fatalf("no matching event on %s", topic)
		}
		var found *outbox.Event
		fetches.EachRecord(func(r *kgo.Record) {
			var ev outbox.Event
			if json.Unmarshal(r.Value, &ev) == nil && found == nil && match(ev) {
				found = &ev
			}
		})
		if found != nil {
			return *found
		}
	}
}

func TestTransferPublishesEvents(t *testing.T) {
	s := connect(t)
	alice := openAccount(t, s, "Alice Souza", "529.982.247-25")
	bob := openAccount(t, s, "Bob Lima", "111.444.777-35")
	deposit(t, s, alice, 10_00)

	corr := "e2e-" + uuid.NewString()
	c := metadata.AppendToOutgoingContext(ctx(t), correlation.MetadataKey, corr)
	resp, err := s.tx.CreateTransfer(c, &txv1.CreateTransferRequest{
		IdempotencyKey: uuid.NewString(), FromAccountId: alice, ToAccountId: bob, Amount: brl(1_00),
	})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.GetTransaction().GetId()

	ev := waitForEvent(t, "bank.transaction.events", func(ev outbox.Event) bool {
		var p struct {
			TransactionID string `json:"transaction_id"`
		}
		return ev.Type == "transaction.completed" && json.Unmarshal(ev.Payload, &p) == nil && p.TransactionID == id
	})
	if ev.CorrelationID != corr || ev.Source != "transaction-service" {
		t.Fatalf("event = %+v", ev)
	}

	// The ledger entry carries the same correlation ID across the service hop.
	waitForEvent(t, "bank.ledger.events", func(ev outbox.Event) bool {
		return ev.Type == "ledger.entry.posted" && ev.CorrelationID == corr
	})
}
