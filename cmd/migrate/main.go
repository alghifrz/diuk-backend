// Command migrate applies the SQL migrations in the migrations directory.
// Migrations are always explicit: the API server never changes the schema.
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	pgxdriver "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/alghifrz/diuk-backend/migrations"
	"github.com/alghifrz/diuk-backend/pkg/config"
)

const usage = `usage: migrate <command>

commands:
  up [steps]       apply pending migrations (all, or the given number of steps)
  down [steps]     roll back the given number of migrations (default 1)
  version          print the current schema version
  force <version>  set the schema version without running migrations
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("missing command")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	m, err := newMigrator(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			fmt.Fprintf(os.Stderr, "migrate: close: source=%v database=%v\n", sourceErr, dbErr)
		}
	}()

	switch args[0] {
	case "up":
		if len(args) > 1 {
			steps, err := downSteps(args[1:])
			if err != nil {
				return err
			}
			return report(m, m.Steps(steps))
		}
		return report(m, m.Up())
	case "down":
		steps, err := downSteps(args[1:])
		if err != nil {
			return err
		}
		return report(m, m.Steps(-steps))
	case "version":
		return printVersion(m)
	case "force":
		if len(args) < 2 {
			return errors.New("force requires a version")
		}
		version, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid version %q: %w", args[1], err)
		}
		return report(m, m.Force(version))
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	driver, err := pgxdriver.WithInstance(db, &pgxdriver.Config{})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create migrator: %w", err)
	}

	return m, nil
}

func downSteps(args []string) (int, error) {
	if len(args) == 0 {
		return 1, nil
	}

	steps, err := strconv.Atoi(args[0])
	if err != nil || steps < 1 {
		return 0, fmt.Errorf("invalid step count %q", args[0])
	}

	return steps, nil
}

func report(m *migrate.Migrate, err error) error {
	switch {
	case errors.Is(err, migrate.ErrNoChange):
		fmt.Println("no change")
	case err != nil:
		return err
	}

	return printVersion(m)
}

func printVersion(m *migrate.Migrate) error {
	version, dirty, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		fmt.Println("version: none")
		return nil
	}
	if err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	fmt.Printf("version: %d dirty: %t\n", version, dirty)
	return nil
}
