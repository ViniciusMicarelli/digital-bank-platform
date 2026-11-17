package postgres_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/ports"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/migrations"
)

const cpf = "52998224725"

func TestDocumentIsEncryptedAtRest(t *testing.T) {
	pool := pgtest.NewDB(t, migrations.FS, ".")
	store, err := pgstore.NewStore(pool, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	insert := func(key string) domain.Account {
		a, _ := domain.NewAccount(key, "cust", "Ana Souza", cpf, "")
		var stored domain.Account
		err := store.InTx(ctx, func(tx ports.Tx) error {
			var err error
			stored, _, err = tx.InsertAccount(ctx, a, domain.DefaultLimits)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return stored
	}
	a, b := insert("k1"), insert("k2")
	if a.Document != cpf {
		t.Fatalf("document read back = %q", a.Document)
	}

	// Nothing in the row reveals the CPF.
	var dump string
	_ = pool.QueryRow(ctx, `SELECT row_to_json(a)::text FROM accounts a WHERE id = $1`, a.ID).Scan(&dump)
	if strings.Contains(dump, cpf) || strings.Contains(dump, cpf[3:9]) {
		t.Fatalf("CPF visible at rest: %s", dump)
	}
	// Equal CPFs share a blind index (searchable) but not a ciphertext.
	var sameIndex, sameCipher bool
	_ = pool.QueryRow(ctx, `SELECT x.document_index = y.document_index, x.document_enc = y.document_enc
		FROM accounts x, accounts y WHERE x.id = $1 AND y.id = $2`, a.ID, b.ID).Scan(&sameIndex, &sameCipher)
	if !sameIndex || sameCipher {
		t.Fatalf("sameIndex=%v sameCipher=%v", sameIndex, sameCipher)
	}

	// A ciphertext copied to another row does not decrypt (bound to the id).
	if _, err := pool.Exec(ctx, `UPDATE accounts SET document_enc = (SELECT document_enc FROM accounts WHERE id = $1) WHERE id = $2`, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Account(ctx, b.ID); err == nil {
		t.Fatal("a swapped ciphertext must not decrypt")
	}
}

func TestLegacyRowsAreEncrypted(t *testing.T) {
	pool := pgtest.NewDB(t, migrations.FS, ".")
	store, _ := pgstore.NewStore(pool, bytes.Repeat([]byte{3}, 32))
	ctx := context.Background()
	// A row written before migration 0002: plaintext document, no ciphertext.
	_, err := pool.Exec(ctx, `INSERT INTO accounts (id, idempotency_key, customer_id, holder_name, document, branch, number, currency, status)
		VALUES (gen_random_uuid(), 'legacy', 'cust', 'Ana Souza', $1, '0001', '00000001-0', 'BRL', 'ACTIVE')`, cpf)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := store.EncryptLegacyDocuments(ctx); err != nil || n != 1 {
		t.Fatalf("backfill = %d, %v", n, err)
	}
	if n, _ := store.EncryptLegacyDocuments(ctx); n != 0 {
		t.Fatal("backfill must be idempotent")
	}
	var plain *string
	var id string
	_ = pool.QueryRow(ctx, `SELECT id::text, document FROM accounts WHERE idempotency_key = 'legacy'`).Scan(&id, &plain)
	if plain != nil {
		t.Fatal("plaintext not cleared")
	}
	list, err := store.AccountsByCustomer(ctx, "cust")
	if err != nil || len(list) != 1 || list[0].Document != cpf {
		t.Fatalf("read after backfill = %+v, %v", list, err)
	}
}
