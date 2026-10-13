// Command api runs the ForgeDeck Control Plane.
//
// Usage:
//
//	forgedeck-api [serve]             run the server (applies pending migrations first)
//	forgedeck-api migrate up|down|status
//	forgedeck-api create-user --email <address>   (password from terminal or stdin)
//	forgedeck-api create-runner --name <name>     (prints the Runner credential once)
//	forgedeck-api delete-runner --name <name>
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/logging"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "forgedeck-api:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	logger, err := logging.New(os.Stdout, cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := "serve"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}
	return dispatch(ctx, command, args, cfg, logger)
}

func dispatch(ctx context.Context, command string, args []string, cfg config.Config, logger *slog.Logger) error {
	switch command {
	case "serve":
		return serve(ctx, cfg, logger)
	case "migrate":
		return migrate(ctx, args, cfg, logger)
	case "create-user":
		return createUser(ctx, args, cfg, logger)
	case "create-runner":
		return createRunner(ctx, args, cfg, logger)
	case "delete-runner":
		return deleteRunner(ctx, args, cfg, logger)
	default:
		return fmt.Errorf("unknown command %q (want serve, migrate, create-user, create-runner or delete-runner)", command)
	}
}
