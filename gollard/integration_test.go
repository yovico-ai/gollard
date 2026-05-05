package gollard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestContactDeploy ports test/Test/Integration.hs:testContactDeploy.
//
// It createdb's a fresh `contacts` database, runs the migrations under
// sql/example-contacts (with tests enabled), inserts a row, reads it back,
// and drops the database. Skips when createdb or postgres are unavailable.
func TestContactDeploy(t *testing.T) {
	if _, err := exec.LookPath("createdb"); err != nil {
		t.Skipf("createdb not found on PATH: %v", err)
	}
	if _, err := exec.LookPath("dropdb"); err != nil {
		t.Skipf("dropdb not found on PATH: %v", err)
	}

	if out, err := exec.Command("createdb", "contacts").CombinedOutput(); err != nil {
		t.Skipf("createdb contacts failed (skipping integration test): %v\n%s", err, out)
	}
	dropped := false
	defer func() {
		if dropped {
			return
		}
		if out, err := exec.Command("dropdb", "contacts").CombinedOutput(); err != nil {
			t.Logf("dropdb contacts failed: %v\n%s", err, out)
		}
	}()

	host := envOr("PGHOST", "localhost")
	user := envOr("PGUSER", "vagrant")
	password := envOr("PGPASSWORD", "password")
	connStr := fmt.Sprintf("host=%s user=%s password=%s dbname=contacts sslmode=disable", host, user, password)

	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.MaxConns = 1

	connectCtx, cancelConn := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConn()
	pool, err := pgxpool.NewWithConfig(connectCtx, cfg)
	if err != nil {
		t.Skipf("could not connect to postgres (skipping integration test): %v", err)
	}
	defer func() {
		pool.Close()
		if out, err := exec.Command("dropdb", "contacts").CombinedOutput(); err != nil {
			t.Logf("dropdb contacts failed: %v\n%s", err, out)
		}
		dropped = true
	}()

	if err := pool.Ping(connectCtx); err != nil {
		t.Skipf("postgres ping failed (skipping integration test): %v", err)
	}

	ctx := context.Background()
	db := NewDB(pool)

	t.Log("Migrate contacts database.")
	if err := Migrate(ctx, db, "../sql/example-contacts", true); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Log("Check insert of person.")
	var id int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO contact.person(name_first, name_middle, name_last) VALUES ($1, $2, $3) RETURNING id`,
		"John", "Edward", "Doe",
	).Scan(&id); err != nil {
		t.Fatalf("insert person: %v", err)
	}

	t.Log("Check recovery of person.")
	var first, mid, last string
	if err := pool.QueryRow(ctx,
		`SELECT name_first, name_middle, name_last FROM contact.person WHERE id = $1`, id,
	).Scan(&first, &mid, &last); err != nil {
		t.Fatalf("select person: %v", err)
	}
	if first != "John" {
		t.Errorf("first: got %q want %q", first, "John")
	}
	if mid != "Edward" {
		t.Errorf("middle: got %q want %q", mid, "Edward")
	}
	if last != "Doe" {
		t.Errorf("last: got %q want %q", last, "Doe")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
