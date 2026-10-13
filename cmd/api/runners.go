package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/runners"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// createRunner registers a build Runner and prints its credential once.
func createRunner(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) error {
	name, err := runnerName("create-runner", args)
	if err != nil {
		return err
	}
	svc, closeDB, err := runnerService(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer closeDB()

	runner, credential, err := svc.Create(ctx, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "created runner %s (%s)\n", runner.Name, runner.ID)
	fmt.Fprintln(os.Stderr, "Credential (shown once; give it to the build host as FORGEDECK_RUNNER_CREDENTIAL):")
	fmt.Println(credential)
	return nil
}

// deleteRunner removes a Runner and revokes its credential.
func deleteRunner(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) error {
	name, err := runnerName("delete-runner", args)
	if err != nil {
		return err
	}
	svc, closeDB, err := runnerService(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer closeDB()
	if err := svc.Delete(ctx, name); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "deleted runner %s; its credential no longer works\n", name)
	return nil
}

func runnerName(command string, args []string) (string, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	name := flags.String("name", "", "runner name, e.g. build-01")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	if *name == "" {
		return "", errors.New("usage: forgedeck-api " + command + " --name <name>")
	}
	return *name, nil
}

func runnerService(ctx context.Context, cfg config.Config, logger *slog.Logger) (*runners.Service, func(), error) {
	if err := migrateUp(ctx, cfg, logger); err != nil {
		return nil, nil, err
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	return runners.NewService(store.New(pool), time.Now), pool.Close, nil
}
