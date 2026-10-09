// Command vet runs the agent and deterministic checks that gate plans and
// diffs. It is unrelated to `go vet`.
package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/waygatetech/vet/internal/config"
)

// Set by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Exit codes are a contract with the tix hooks that call vet.
const (
	exitPass     = 0 // no blocking findings
	exitBlocking = 1 // at least one blocking finding
	exitUsage    = 2 // bad flags, args, or config
	exitAgent    = 3 // reviewer/agent failed; the gate fails closed
)

// exitError carries a specific exit code. Errors without one (for example
// Cobra's flag and argument errors) exit with exitUsage.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// app holds state shared by subcommands once config is loaded.
type app struct {
	cfg config.Config
	log *slog.Logger
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cmd := newRootCmd(stderr)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	err := cmd.Execute()
	if err == nil {
		return exitPass
	}
	fmt.Fprintln(stderr, "vet:", err)
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return exitUsage
}

func newRootCmd(stderr io.Writer) *cobra.Command {
	var (
		a          app
		configPath string
	)
	cmd := &cobra.Command{
		Use:           "vet",
		Short:         "Run the checks that gate plans and diffs (not go vet)",
		Version:       fmt.Sprintf("%s (commit %s, built %s)", version, commit, date),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath, cmd.Flags().Changed("config"))
			if err != nil {
				return &exitError{code: exitUsage, err: err}
			}
			a.cfg = cfg
			a.log = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&configPath, "config", config.DefaultPath, "path to config file")
	return cmd
}
