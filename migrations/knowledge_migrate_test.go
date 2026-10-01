package migrations

import (
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	pgxdriver "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestKnowledgeMigrationCycle(t *testing.T) {
	if os.Getenv("TEST_KNOWLEDGE_MIGRATE") != "1" {
		t.Skip("TEST_KNOWLEDGE_MIGRATE is not 1; skipping 000011↔000012 cycle to avoid racing shared TEST_DATABASE_URL")
	}
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping knowledge migration cycle")
	}

	source, err := iofs.New(FS, ".")
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	driver, err := pgxdriver.WithInstance(db, &pgxdriver.Config{})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		t.Fatalf("version: %v", err)
	}
	if dirty {
		t.Fatal("database is already dirty")
	}
	if version < 11 {
		t.Skipf("schema version %d is below 11; skipping 11↔12 cycle", version)
	}

	mustStep := func(steps int) {
		t.Helper()
		if err := m.Steps(steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			t.Fatalf("steps %d: %v", steps, err)
		}
		v, dirty, err := m.Version()
		if err != nil {
			t.Fatalf("read version: %v", err)
		}
		if dirty {
			t.Fatalf("dirty=true after steps %d at version %d", steps, v)
		}
	}

	if version == 12 {
		mustStep(-1)
		if v, _, _ := m.Version(); v != 11 {
			t.Fatalf("after down = %d, want 11", v)
		}
		mustStep(1)
		if v, _, _ := m.Version(); v != 12 {
			t.Fatalf("after up = %d, want 12", v)
		}
		mustStep(-1)
		mustStep(1)
	} else if version == 11 {
		mustStep(1)
		mustStep(-1)
		mustStep(1)
	} else {
		t.Skipf("schema version %d is not 11 or 12", version)
	}

	v, dirty, err := m.Version()
	if err != nil || dirty || v != 12 {
		t.Fatalf("final version=%d dirty=%t err=%v", v, dirty, err)
	}
}
