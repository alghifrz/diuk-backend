// Command api starts the DIUK HTTP API and, when APP_MODE=all, the workers.
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
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.AppEnv)
	log.Info("starting service", "service", cfg.AppName, "env", cfg.AppEnv, "mode", cfg.AppMode)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rt, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer rt.Close()

	return rt.Run(ctx)
}
