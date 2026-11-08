package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timotheebenchergui/arrondi/api/internal/bank"
	"github.com/timotheebenchergui/arrondi/api/internal/enablebanking"
	"github.com/timotheebenchergui/arrondi/api/internal/syncer"
)

// Test d'intégration : nécessite une base migrée.
//
//	TEST_DATABASE_URL=postgres://arrondi:arrondi-dev@127.0.0.1:5432/arrondi?sslmode=disable go test ./internal/store
//
// Il crée son propre utilisateur et le supprime à la fin (cascade).
func TestSyncStore_SynchroComplete(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL non défini")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := New(pool)

	user, err := q.CreateUser(ctx, CreateUserParams{
		Email: "test-" + uuid.NewString()[:8] + "@exemple.fr", DisplayName: "Test", Role: "member",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer q.DeleteUser(ctx, user.ID)
	if _, err := q.CreateDefaultSettings(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	conn, err := q.CreateConnection(ctx, CreateConnectionParams{
		UserID: user.ID, BankCode: "boursobank", EbApplicationID: "app", EbPrivateKeyEnc: []byte{1},
		EbKeyNonce: make([]byte, 12), AspspName: "Boursorama Banque",
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, until := "sess", time.Now().Add(180*24*time.Hour)
	conn, err = q.ActivateConnection(ctx, ActivateConnectionParams{ID: conn.ID, EbSessionID: &sess, ConsentValidUntil: &until})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := q.UpsertAccount(ctx, UpsertAccountParams{ConnectionID: conn.ID, EbAccountUid: "uid-cacc", Currency: "EUR"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.TrackAccount(ctx, TrackAccountParams{ID: acc.ID, ConnectionID: conn.ID}); err != nil {
		t.Fatal(err)
	}

	raw := func(ref, booked, amount, label string) enablebanking.Item {
		b, _ := json.Marshal(map[string]any{
			"entry_reference": ref, "status": "BOOK", "booking_date": booked, "credit_debit_indicator": "DBIT",
			"transaction_amount":     map[string]string{"currency": "EUR", "amount": amount},
			"remittance_information": []string{label},
		})
		var tx bank.RawTransaction
		_ = json.Unmarshal(b, &tx)
		return enablebanking.Item{Tx: tx, Raw: b}
	}
	today := time.Now().Format("2006-01-02")
	fb := fakeBank{items: []enablebanking.Item{
		raw("1", today, "69.37", "CARTE TOTAL"),
		raw("2", today, "35.77", "CARTE LECLERC"),
		raw("3", today, "105.42", "PRLV SEPA MACIF"),
	}}
	svc := &syncer.Service{Store: NewSyncStore(pool), Bank: fb, Profiles: []bank.Profile{bank.BoursoBank{}}, Now: time.Now}
	c := syncer.Connection{ID: conn.ID, UserID: user.ID, BankCode: "boursobank",
		ActivatedAt: *conn.ActivatedAt, AccountID: acc.ID, AccountUID: "uid-cacc"}

	for pass := 1; pass <= 2; pass++ {
		res, err := svc.Sync(ctx, c, syncer.TriggerManual)
		if err != nil {
			t.Fatalf("passage %d : %v", pass, err)
		}
		wantInserted := map[int]int{1: 3, 2: 0}[pass]
		if res.Inserted != wantInserted {
			t.Errorf("passage %d : %d insérées, attendu %d", pass, res.Inserted, wantInserted)
		}
	}

	totals, err := q.SavingsTotals(ctx, SavingsTotalsParams{UserID: user.ID, Month: dateOnly(syncer.Month(syncer.Day(time.Now())))})
	if err != nil {
		t.Fatal(err)
	}
	if totals.TotalSavedCents != 63+23 || totals.MonthSavedCents != 63+23 {
		t.Errorf("totaux %+v, attendu 86 c épargnés", totals)
	}
	runs, _ := q.LastSyncRuns(ctx, conn.ID)
	if len(runs) != 2 || runs[0].Status != "success" {
		t.Errorf("sync_runs : %d exécutions, dernière %q", len(runs), runs[0].Status)
	}
}

type fakeBank struct{ items []enablebanking.Item }

func (f fakeBank) Transactions(context.Context, enablebanking.Credentials, string, time.Time, time.Time) ([]enablebanking.Item, error) {
	return f.items, nil
}
