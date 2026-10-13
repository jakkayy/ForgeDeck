// Command runner runs a ForgeDeck build Runner on a dedicated build host.
//
// Usage:
//
//	forgedeck-runner run --url https://forgedeck.example.com --credential-file /etc/forgedeck/runner.credential
//	    (or set FORGEDECK_RUNNER_CREDENTIAL instead of --credential-file)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jakkayy/ForgeDeck/internal/logging"
	"github.com/jakkayy/ForgeDeck/internal/runner"
	"github.com/jakkayy/ForgeDeck/internal/runners"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "forgedeck-runner:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 || args[0] != "run" {
		return errors.New("usage: forgedeck-runner run --url URL [--credential-file FILE] [flags]")
	}
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	controlPlaneURL := flags.String("url", os.Getenv("FORGEDECK_URL"), "Control Plane URL, e.g. https://forgedeck.example.com")
	credentialFile := flags.String("credential-file", "", "file holding the Runner credential (default: $FORGEDECK_RUNNER_CREDENTIAL)")
	allowInsecure := flags.Bool("allow-insecure", false, "allow a non-HTTPS Control Plane URL")
	logLevel := flags.String("log-level", "info", "debug, info, warn or error")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if err := worker.ValidateControlPlaneURL(*controlPlaneURL, *allowInsecure); err != nil {
		return err
	}
	credential, err := loadCredential(*credentialFile)
	if err != nil {
		return err
	}
	logger, err := logging.New(os.Stderr, *logLevel, "text")
	if err != nil {
		return err
	}

	cfg := runner.Config{ControlPlaneURL: *controlPlaneURL, Credential: credential, Version: version}
	return runner.Run(ctx, cfg, runner.Handlers(), runner.DockerStatus, logger)
}

// loadCredential reads the credential from a file or the environment, never
// from arguments, so it stays out of process listings.
func loadCredential(path string) (string, error) {
	raw := os.Getenv("FORGEDECK_RUNNER_CREDENTIAL")
	if path != "" {
		data, err := os.ReadFile(path) //nolint:gosec // operator-supplied path
		if err != nil {
			return "", fmt.Errorf("read credential: %w", err)
		}
		raw = string(data)
	}
	credential := strings.TrimSpace(raw)
	if !strings.HasPrefix(credential, runners.CredentialPrefix) {
		return "", fmt.Errorf("runner credential missing or malformed: pass --credential-file or set FORGEDECK_RUNNER_CREDENTIAL to the %s… value printed by `forgedeck-api create-runner`", runners.CredentialPrefix)
	}
	return credential, nil
}
