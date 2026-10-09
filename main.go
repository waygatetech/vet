// Command vet runs the agent and deterministic checks that gate plans and
// diffs. It is unrelated to `go vet`.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/waygatetech/vet/internal/config"
	"github.com/waygatetech/vet/internal/critique"
	"github.com/waygatetech/vet/internal/findings"
	"github.com/waygatetech/vet/internal/review"
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
	cmd.AddCommand(newCritiqueCmd(&a), newReviewCmd(&a))
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
	files, err := readContexts(contexts)
	if err != nil {
		return err
	}

	a.log.Debug("running critic", "command", a.cfg.CriticCommand)
	report, err := findings.Run(cmd.Context(), a.cfg.CriticCommand, critique.Prompt(ticketText, planText, concepts, files))
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

// read returns a file's contents; failures are usage errors.
func read(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", &exitError{code: exitUsage, err: fmt.Errorf("reading input: %w", err)}
	}
	return string(b), nil
}

func readContexts(paths []string) ([]critique.File, error) {
	var files []critique.File
	for _, path := range paths {
		text, err := read(path)
		if err != nil {
			return nil, err
		}
		files = append(files, critique.File{Name: path, Content: text})
	}
	return files, nil
}

func newReviewCmd(a *app) *cobra.Command {
	var (
		plan, base string
		contexts   []string
		asJSON     bool
		tier       int
	)
	cmd := &cobra.Command{
		Use:   "review --plan <plan.md>",
		Short: "Run a fresh-context reviewer over a plan and its diff; exit 1 on blocking findings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if plan == "" {
				return &exitError{code: exitUsage, err: errors.New("--plan is required")}
			}
			planText, err := read(plan)
			if err != nil {
				return err
			}
			files, err := readContexts(contexts)
			if err != nil {
				return err
			}
			diff, err := review.Diff(cmd.Context(), base)
			if err != nil {
				return &exitError{code: exitUsage, err: err}
			}
			contracts, err := review.ContractsChanged([]byte(planText))
			if err != nil {
				return &exitError{code: exitUsage, err: err}
			}
			changed := review.Files(diff)
			var reviewers []config.Reviewer
			for _, r := range a.cfg.Reviewers {
				if review.Triggered(r, contracts, tier, changed) {
					reviewers = append(reviewers, r)
				}
			}
			if len(reviewers) == 0 {
				return &exitError{code: exitUsage, err: errors.New("no reviewer triggered; give at least one reviewer no when")}
			}

			// Every reviewer gets the same prompt; any failure fails the gate closed.
			prompt := review.Prompt(planText, diff, files)
			reports := make([]findings.Report, len(reviewers))
			errs := make([]error, len(reviewers))
			var wg sync.WaitGroup
			for i, r := range reviewers {
				a.log.Debug("running reviewer", "name", r.Name, "command", r.Command)
				wg.Go(func() {
					reports[i], errs[i] = findings.Run(cmd.Context(), r.Command, prompt)
					if errs[i] != nil {
						errs[i] = fmt.Errorf("reviewer %s: %w", r.Name, errs[i])
					}
				})
			}
			wg.Wait()
			if err := errors.Join(errs...); err != nil {
				return &exitError{code: exitAgent, err: err}
			}
			var report findings.Report
			for i, r := range reports {
				for _, f := range r.Findings {
					f.Reviewer = reviewers[i].Name
					report.Findings = append(report.Findings, f)
				}
			}
			out := cmd.OutOrStdout()
			if asJSON {
				if err := json.NewEncoder(out).Encode(report); err != nil {
					return &exitError{code: exitAgent, err: fmt.Errorf("writing report: %w", err)}
				}
			} else {
				for _, f := range report.Findings {
					loc := f.File
					if f.Line > 0 {
						loc = fmt.Sprintf("%s:%d", f.File, f.Line)
					}
					fmt.Fprintf(out, "[%s] %s %s %s (%s)\n", f.Severity, f.Check, loc, strings.TrimSpace(f.Message), f.Reviewer)
				}
			}
			if report.Blocking() {
				return &exitError{code: exitBlocking, err: errors.New("review raised blocking findings")}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&plan, "plan", "", "plan file the diff implements (required)")
	cmd.Flags().StringVar(&base, "base", "main", "ref whose merge base with HEAD the diff starts from")
	cmd.Flags().StringArrayVar(&contexts, "context", nil, "extra context file for the reviewer, e.g. human rulings (repeatable)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "write the findings JSON report to stdout")
	cmd.Flags().IntVar(&tier, "tier", 0, "the ticket's review tier, for reviewers with when.min_tier")
	return cmd
}
