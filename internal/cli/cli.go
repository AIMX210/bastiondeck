package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"bastiondeck/internal/apiclient"
)

// App is the CLI application.
type App struct {
	Out io.Writer
	Err io.Writer
}

// New constructs an App writing to stdout/stderr.
func New() *App { return &App{Out: os.Stdout, Err: os.Stderr} }

// command describes one subcommand.
type command struct {
	name  string
	usage string
	desc  string
	run   func(ctx context.Context, a *App, args []string) error
}

var commands = map[string]command{}

func register(c command) { commands[c.name] = c }

// Run dispatches argv (excluding program name).
func (a *App) Run(ctx context.Context, argv []string) int {
	if len(argv) == 0 {
		a.usage()
		return 2
	}
	name, rest := argv[0], argv[1:]
	if name == "-h" || name == "--help" || name == "help" {
		a.usage()
		return 0
	}
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(a.Err, "unknown command %q\n\n", name)
		a.usage()
		return 2
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if err := cmd.run(ctx, a, rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		var ae *apiclient.APIError
		if errors.As(err, &ae) {
			fmt.Fprintf(a.Err, "error [%s]: %s\n", ae.Code, ae.Message)
		} else {
			fmt.Fprintf(a.Err, "error: %v\n", err)
		}
		return 1
	}
	return 0
}

func (a *App) usage() {
	fmt.Fprintln(a.Out, "bdk — BastionDeck command line client")
	fmt.Fprintln(a.Out, "")
	fmt.Fprintln(a.Out, "Usage: bdk <command> [flags]")
	fmt.Fprintln(a.Out, "")
	fmt.Fprintln(a.Out, "Commands:")
	order := []string{"login", "logout", "status", "doctor", "tui", "hosts", "host",
		"creds", "run", "runs", "runx", "snippets", "audit", "tunnels", "agents", "backup"}
	for _, n := range order {
		if c, ok := commands[n]; ok {
			fmt.Fprintf(a.Out, "  %-10s %s\n", n, c.desc)
		}
	}
}

// client builds an authenticated client from stored config, with optional
// per-invocation overrides via flags.
func (a *App) client(rest []string) (*apiclient.Client, []string, error) {
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	urlFlag := fs.String("url", "", "API base URL override")
	tokenFlag := fs.String("token", "", "bearer token override")
	if err := fs.Parse(rest); err != nil {
		return nil, nil, err
	}
	cfg, err := LoadConfig()
	if err != nil && *urlFlag == "" {
		return nil, nil, fmt.Errorf("not logged in: run `bdk login` or pass --url/--token")
	}
	if cfg == nil {
		cfg = &Config{}
	}
	base := *urlFlag
	if base == "" {
		base = cfg.BaseURL
	}
	token := *tokenFlag
	if token == "" {
		token = cfg.Token
	}
	if base == "" {
		return nil, nil, errors.New("no API URL configured")
	}
	return apiclient.New(base, token), fs.Args(), nil
}
