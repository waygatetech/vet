// Command vet runs the agent and deterministic checks that gate plans and
// diffs. It is unrelated to `go vet`.
package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/waygatetech/vet/internal/config"
	"github.com/waygatetech/vet/internal/critique"
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
	cmd.AddCommand(newCritiqueCmd(&a))
	return cmd
}

func newCritiqueCmd(a *app) *cobra.Command {
	var (
		check    bool
		ticket   string
		contexts []string
	)
	cmd := &cobra.Command{
		Use:   "critique <plan.md>",
		Short: "Run a fresh-context critic over a plan, or --check that every item is answered",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plan := args[0]
			out := strings.TrimSuffix(plan, ".md") + ".critique.md"
			if check {
				return checkCritique(cmd, plan, out)
			}
			if ticket == "" {
				return &exitError{code: exitUsage, err: errors.New("--ticket is required unless --check is set")}
			}
			return runCritique(cmd, a, plan, ticket, contexts, out)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "exit 1 unless every critique item has a critique_responses entry")
	cmd.Flags().StringVar(&ticket, "ticket", "", "file containing the ticket text")
	cmd.Flags().StringArrayVar(&contexts, "context", nil, "extra context file for the critic (repeatable)")
	return cmd
}

func runCritique(cmd *cobra.Command, a *app, plan, ticket string, contexts []string, out string) error {
	read := func(path string) (string, error) {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", &exitError{code: exitUsage, err: fmt.Errorf("reading input: %w", err)}
		}
		return string(b), nil
	}
	planText, err := read(plan)
	if err != nil {
		return err
	}
	ticketText, err := read(ticket)
	if err != nil {
		return err
	}
	concepts, err := read("concepts.yaml")
	if err != nil {
		return err
	}
	var files []critique.File
	for _, path := range contexts {
		text, err := read(path)
		if err != nil {
			return err
		}
		files = append(files, critique.File{Name: path, Content: text})
	}

	a.log.Debug("running critic", "command", a.cfg.CriticCommand)
	report, err := critique.Run(cmd.Context(), a.cfg.CriticCommand, critique.Prompt(ticketText, planText, concepts, files))
	if err != nil {
		return &exitError{code: exitAgent, err: err}
	}
	if err := os.WriteFile(out, critique.Render(report), 0o644); err != nil {
		return &exitError{code: exitAgent, err: fmt.Errorf("writing critique: %w", err)}
	}
	fmt.Fprintln(cmd.OutOrStdout(), out)
	return nil
}

func checkCritique(cmd *cobra.Command, plan, out string) error {
	critiqueText, err := os.ReadFile(out)
	if err != nil {
		return &exitError{code: exitUsage, err: fmt.Errorf("reading critique (run vet critique first): %w", err)}
	}
	planText, err := os.ReadFile(plan)
	if err != nil {
		return &exitError{code: exitUsage, err: fmt.Errorf("reading plan: %w", err)}
	}
	missing, err := critique.Check(critiqueText, planText)
	if err != nil {
		return &exitError{code: exitUsage, err: err}
	}
	if len(missing) > 0 {
		return &exitError{code: exitBlocking, err: fmt.Errorf("critique items without critique_responses: %s", strings.Join(missing, ", "))}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "all critique items answered")
	return nil
}
