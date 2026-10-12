package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// createUser adds a user account. The password is read from the terminal
// without echo, or from the first line of stdin when piped, never from args.
func createUser(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) error {
	flags := flag.NewFlagSet("create-user", flag.ContinueOnError)
	email := flags.String("email", "", "email address of the new user")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *email == "" {
		return errors.New("usage: forgedeck-api create-user --email <address>")
	}

	password, err := readPassword(os.Stdin, os.Stderr)
	if err != nil {
		return err
	}

	if err := migrateUp(ctx, cfg, logger); err != nil {
		return err
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	user, err := auth.NewService(store.New(pool)).CreateUser(ctx, *email, password)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "created user %s (%s)\n", user.Email, user.ID)
	return nil
}

func readPassword(stdin *os.File, prompt io.Writer) (string, error) {
	fd := int(stdin.Fd()) //nolint:gosec // file descriptors fit in int
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	first, err := promptHidden(fd, prompt, "Password: ")
	if err != nil {
		return "", err
	}
	second, err := promptHidden(fd, prompt, "Repeat password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("passwords do not match")
	}
	return first, nil
}

func promptHidden(fd int, prompt io.Writer, label string) (string, error) {
	_, _ = fmt.Fprint(prompt, label)
	b, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(prompt)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(b), nil
}
