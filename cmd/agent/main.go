// Command agent runs the ForgeDeck Agent on a Docker host.
//
// Usage:
//
//	forgedeck-agent enroll --url https://forgedeck.example.com
//	    (token from FORGEDECK_ENROLLMENT_TOKEN or the first line of stdin)
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jakkayy/ForgeDeck/internal/agent"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "forgedeck-agent:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		return errors.New("usage: forgedeck-agent enroll [flags]")
	}
	switch args[0] {
	case "enroll":
		return enroll(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q (want enroll)", args[0])
	}
}

func enroll(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
	controlPlaneURL := flags.String("url", "", "Control Plane URL, e.g. https://forgedeck.example.com")
	stateDir := flags.String("state-dir", agent.DefaultStateDir, "directory for the Agent identity")
	allowInsecure := flags.Bool("allow-insecure", false, "allow a non-HTTPS Control Plane URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := agent.ValidateControlPlaneURL(*controlPlaneURL, *allowInsecure); err != nil {
		return err
	}
	token, err := enrollmentToken(os.Stdin)
	if err != nil {
		return err
	}

	state, err := agent.NewState(*stateDir)
	if err != nil {
		return err
	}
	identity, err := agent.Enroll(ctx, state, *controlPlaneURL, token, version)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "enrolled server %s; start the Agent with `forgedeck-agent run`\n", identity.ServerID)
	return nil
}

// enrollmentToken reads the token from the environment or stdin, never from
// arguments, so it does not end up in shell history or process listings.
func enrollmentToken(stdin io.Reader) (string, error) {
	if token := strings.TrimSpace(os.Getenv("FORGEDECK_ENROLLMENT_TOKEN")); token != "" {
		return token, nil
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read enrollment token: %w", err)
	}
	if token := strings.TrimSpace(line); token != "" {
		return token, nil
	}
	return "", errors.New("enrollment token missing: set FORGEDECK_ENROLLMENT_TOKEN or pipe it on stdin")
}
