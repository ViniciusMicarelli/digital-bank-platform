package postgres_test

import (
	"bytes"
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
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

// The contract migration must refuse to drop the plaintext column while a
// row would lose its only copy of the CPF.
func TestContractMigrationIsGuarded(t *testing.T) {
	upTo0002 := fstest.MapFS{}
	for _, name := range []string{"0001_init.up.sql", "0002_encrypt_document.up.sql"} {
		data, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			t.Fatal(err)
		}
		upTo0002[name] = &fstest.MapFile{Data: data}
	}
	pool := pgtest.NewDB(t, upTo0002, ".")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO accounts (id, idempotency_key, customer_id, holder_name, document, branch, number, currency, status)
		VALUES (gen_random_uuid(), 'legacy', 'cust', 'Ana Souza', $1, '0001', '00000001-0', 'BRL', 'ACTIVE')`, cpf); err != nil {
		t.Fatal(err)
	}
	if err := pg.Migrate(ctx, pool, migrations.FS, "."); err == nil {
		t.Fatal("0003 dropped a column holding the only copy of a CPF")
	}
	var exists bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'accounts' AND column_name = 'document')`).Scan(&exists)
	if !exists {
		t.Fatal("the failed migration must leave the column in place")
	}
}
