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

func TestCampaignMigrationCycle(t *testing.T) {
	if os.Getenv("TEST_CAMPAIGN_MIGRATE") != "1" {
		t.Skip("TEST_CAMPAIGN_MIGRATE is not 1; skipping 000015↔000016 cycle to avoid racing shared TEST_DATABASE_URL")
	}
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping campaign migration cycle")
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
	if version < 15 {
		t.Skipf("schema version %d is below 15; skipping 15↔16 cycle", version)
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

	if version == 16 {
		mustStep(-1)
		if v, _, _ := m.Version(); v != 15 {
			t.Fatalf("after down = %d, want 15", v)
		}
		mustStep(1)
		if v, _, _ := m.Version(); v != 16 {
			t.Fatalf("after up = %d, want 16", v)
		}
		mustStep(-1)
		mustStep(1)
	} else if version == 15 {
		mustStep(1)
		mustStep(-1)
		mustStep(1)
	} else {
		t.Skipf("schema version %d is not 15 or 16", version)
	}

	v, dirty, err := m.Version()
	if err != nil || dirty || v != 16 {
		t.Fatalf("final version=%d dirty=%t err=%v", v, dirty, err)
	}
}
