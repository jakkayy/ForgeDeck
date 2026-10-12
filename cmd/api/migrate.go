package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/database"
)

func migrate(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) (err error) {
	if len(args) != 1 {
		return errors.New("usage: forgedeck-api migrate up|down|status")
	}
	migrator, err := database.NewMigrator(cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, migrator.Close()) }()

	switch args[0] {
	case "up":
		return migrator.Up(ctx)
	case "down":
		return migrator.Down(ctx)
	case "status":
		return migrator.PrintStatus(ctx, os.Stdout)
	default:
		return fmt.Errorf("unknown migrate action %q (want up, down or status)", args[0])
	}
}
