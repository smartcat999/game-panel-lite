package globalproduct

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/commerce"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

func TestPostgresCheckoutAndActivationTransactions(t *testing.T) {
	database := openPostgresTestDatabase(t)
	product := NewPostgres(database)
	ctx := context.Background()
	now := testNow()

	if _, err := database.ExecContext(ctx, `ALTER TABLE orders ADD CONSTRAINT reject_test_workspace CHECK (workspace_id <> 'ws_reject')`); err != nil {
		t.Fatal(err)
	}
	rejected := validCommand("idem_database_reject")
	rejected.WorkspaceID = "ws_reject"
	if _, err := product.CreateCheckout(ctx, rejected, now); err == nil {
		t.Fatal("CreateCheckout succeeded despite the late order constraint")
	}
	assertTableCount(t, database, "logical_instances", 0)
	assertTableCount(t, database, "orders", 0)

	command := validCommand("idem_database_repeat")
	first, err := product.CreateCheckout(ctx, command, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := product.CreateCheckout(ctx, command, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.Instance.ID != second.Instance.ID || first.Order.ID != second.Order.ID {
		t.Fatalf("idempotent result changed: first=%#v second=%#v", first, second)
	}
	assertTableCount(t, database, "logical_instances", 1)
	assertTableCount(t, database, "orders", 1)

	if err := product.RequestDeployment(ctx, first.Instance.ID, "deploy_without_entitlement", now); !errors.Is(err, ErrActiveEntitlementNeeded) {
		t.Fatalf("RequestDeployment error = %v, want active entitlement error", err)
	}
	assertTableCount(t, database, "global_outbox", 0)

	if _, err := product.ActivateVerifiedPayment(ctx, first.Order.ID, "notice_unverified", false, now); !errors.Is(err, commerce.ErrPaymentUnverified) {
		t.Fatalf("unverified payment error = %v", err)
	}
	activation, err := product.ActivateVerifiedPayment(ctx, first.Order.ID, "notice_verified", true, now)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := product.ActivateVerifiedPayment(ctx, first.Order.ID, "notice_verified", true, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if activation.Payment.ID != repeated.Payment.ID || activation.Entitlement.ID != repeated.Entitlement.ID {
		t.Fatalf("payment redelivery changed result: first=%#v second=%#v", activation, repeated)
	}
	assertTableCount(t, database, "payments", 1)
	assertTableCount(t, database, "entitlements", 1)
	assertTableCount(t, database, "global_outbox", 2)

	if err := product.RequestDeployment(ctx, first.Instance.ID, "deploy_authorized", now); err != nil {
		t.Fatal(err)
	}
	if err := product.RequestDeployment(ctx, first.Instance.ID, "deploy_authorized", now); err != nil {
		t.Fatal(err)
	}
	assertTableCount(t, database, "global_outbox", 3)

	handled := 0
	inbox := messaging.NewPostgres(database)
	wasHandled, err := inbox.HandleOnce(ctx, contract.EventID("evt_inbox_once"), "payment.verified.v1", now, func(query persistence.DBTX) error {
		handled++
		_, err := query.ExecContext(ctx, `UPDATE orders SET status = status WHERE id = $1`, first.Order.ID)
		return err
	})
	if err != nil || !wasHandled {
		t.Fatalf("first inbox delivery handled=%v error=%v", wasHandled, err)
	}
	wasHandled, err = inbox.HandleOnce(ctx, contract.EventID("evt_inbox_once"), "payment.verified.v1", now, func(query persistence.DBTX) error {
		handled++
		return nil
	})
	if err != nil || wasHandled || handled != 1 {
		t.Fatalf("inbox redelivery handled=%v calls=%d error=%v", wasHandled, handled, err)
	}
}

func openPostgresTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GAMEPANEL_GLOBAL_TEST_DSN")
	if dsn == "" {
		t.Skip("GAMEPANEL_GLOBAL_TEST_DSN is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("phase3_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	if _, err := database.Exec(`SET search_path TO "` + schema + `"`); err != nil {
		database.Close()
		admin.Close()
		t.Fatal(err)
	}
	_, filename, _, _ := runtime.Caller(0)
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "migrations", "global", "0002_product_instance_messaging.sql"))
	if err != nil {
		t.Fatal(err)
	}
	execStatements(t, database, string(migration))
	seed := `
		INSERT INTO regions (id, code, name, available, created_at) VALUES ('reg_test', 'test', 'Test', true, $1);
		INSERT INTO plans (id, name, created_at) VALUES ('pln_standard', 'Standard', $1);
		INSERT INTO plan_versions (id, plan_id, version, name, price_minor, currency, billing_period, memory_megabytes, cpu_units, created_at) VALUES ('plv_standard_1', 'pln_standard', 1, 'Standard', 1200, 'USD', 'month', 2048, 1000, $1);
		INSERT INTO plan_version_regions (plan_version_id, region_id) VALUES ('plv_standard_1', 'reg_test');`
	for _, statement := range strings.Split(seed, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		var err error
		if strings.Contains(statement, "$1") {
			_, err = database.Exec(statement, testNow())
		} else {
			_, err = database.Exec(statement)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		database.Close()
		_, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
		admin.Close()
	})
	return database
}

func execStatements(t *testing.T, database *sql.DB, source string) {
	t.Helper()
	for _, statement := range strings.Split(source, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func assertTableCount(t *testing.T, database *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT count(*) FROM ` + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
