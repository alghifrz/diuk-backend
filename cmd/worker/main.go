// Command worker runs AI and delivery workers without the HTTP API.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alghifrz/diuk-backend/internal/app"
	"github.com/alghifrz/diuk-backend/pkg/config"
	"github.com/alghifrz/diuk-backend/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("APP_MODE") == "" {
		_ = os.Setenv("APP_MODE", "worker")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AppMode = "worker"
	log := logger.New(cfg.AppEnv)
	log.Info("starting workers", "service", cfg.AppName, "env", cfg.AppEnv)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rt, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer rt.Close()

	return rt.Run(ctx)
}
