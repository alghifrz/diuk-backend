// Command seed inserts the Kasiko Coffee development tenant.
//
// It does not create a Supabase Auth user. Set SEED_AUTH_USER_ID to an
// existing auth.users.id and SEED_USER_EMAIL to that user's email.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alghifrz/diuk-backend/internal/database"
	"github.com/alghifrz/diuk-backend/internal/seed"
	"github.com/alghifrz/diuk-backend/pkg/config"
	"github.com/alghifrz/diuk-backend/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	in, err := seed.LoadInput()
	if err != nil {
		return err
	}

	log := logger.New(cfg.AppEnv)
	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	fmt.Println("Seeding development data...")
	log.Info("seeding development data")

	result, err := seed.Run(ctx, tx, in)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	fmt.Printf("Business: %s (%s)\n", result.Business.Name, result.Business.ID)
	fmt.Printf("User: %s (%s)\n", result.User.FullName, result.User.ID)
	fmt.Printf("Role: %s (%s)\n", result.Role.Name, result.Role.ID)
	fmt.Printf("Membership: %s (%s)\n", result.Membership.Status, result.Membership.ID)
	fmt.Println("Development seed completed successfully.")
	return nil
}
