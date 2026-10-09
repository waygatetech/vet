package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("nope: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		args       []string
		want       int
		wantStdout string
	}{
		{name: "version", args: []string{"--version"}, want: exitPass, wantStdout: "dev"},
		{name: "no args shows help", args: nil, want: exitPass, wantStdout: "not go vet"},
		{name: "unknown flag", args: []string{"--bogus"}, want: exitUsage},
		{name: "unexpected arg", args: []string{"bogus"}, want: exitUsage},
		{name: "missing explicit config", args: []string{"--config", filepath.Join(dir, "missing.yaml")}, want: exitUsage},
		{name: "invalid config", args: []string{"--config", bad}, want: exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tt.args, &stdout, &stderr); got != tt.want {
				t.Fatalf("run(%v) = %d, want %d; stderr: %s", tt.args, got, tt.want, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}

func TestCritiqueExitCodes(t *testing.T) {
	dir := t.TempDir()
	write := func(name, contents string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	report := write("report.json", `{"findings":[{"check":"acceptance","severity":"blocking","message":"no tests"}]}`)
	good := write("good.yaml", "critic_command: [cat, "+report+"]\n")
	failing := write("fail.yaml", "critic_command: [sh, -c, \"exit 1\"]\n")
	ticket := write("ticket.txt", "do the thing\n")
	unanswered := write("plan.md", "---\nticket: x\n---\nbody\n")
	answered := write("answered.md", "---\ncritique_responses:\n  C1: added tests\n---\n")
	write("answered.critique.md", "## C1 [blocking] acceptance\n")

	// Steps run in order: the critique file written by "writes critique" feeds the --check steps.
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "check before critique", args: []string{"critique", "--check", unanswered}, want: exitUsage},
		{name: "missing ticket flag", args: []string{"critique", unanswered}, want: exitUsage},
		{name: "missing ticket file", args: []string{"--config", good, "critique", "--ticket", filepath.Join(dir, "nope"), unanswered}, want: exitUsage},
		{name: "critic fails", args: []string{"--config", failing, "critique", "--ticket", ticket, unanswered}, want: exitAgent},
		{name: "writes critique", args: []string{"--config", good, "critique", "--ticket", ticket, "--context", ticket, unanswered}, want: exitPass},
		{name: "check unanswered", args: []string{"critique", "--check", unanswered}, want: exitBlocking},
		{name: "check answered", args: []string{"critique", "--check", answered}, want: exitPass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tt.args, &stdout, &stderr); got != tt.want {
				t.Fatalf("run(%v) = %d, want %d; stderr: %s", tt.args, got, tt.want, stderr.String())
			}
		})
	}
}

func TestReviewExitCodes(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cmd := exec.Command("sh", "-c", "git init -q -b main && git -c user.name=t -c user.email=t@t commit -q --allow-empty -m base && echo new > new.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("setting up repo: %v: %s", err, out)
	}
	write := func(name, contents string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	plan := write("plan.md", "---\nticket: x\n---\nbody\n")
	reviewer := func(name, script string) string {
		return write(name, "reviewer_command: [sh, -c, "+strconv.Quote(script)+"]\n")
	}
	clean := reviewer("clean.yaml", `grep -q '+new' && echo '{"findings":[{"check":"scope","severity":"warning","file":"new.go","line":1,"message":"extra"}]}'`)
	blocking := reviewer("blocking.yaml", `echo '{"findings":[{"check":"acceptance","severity":"blocking","acceptance_ref":"AC1","message":"untested"}]}'`)
	malformed := reviewer("malformed.yaml", `echo LGTM`)

	tests := []struct {
		name       string
		args       []string
		want       int
		wantStdout string
	}{
		{name: "missing plan flag", args: []string{"review"}, want: exitUsage},
		{name: "missing plan file", args: []string{"--config", clean, "review", "--plan", filepath.Join(dir, "nope.md")}, want: exitUsage},
		{name: "bad base", args: []string{"--config", clean, "review", "--plan", plan, "--base", "nope"}, want: exitUsage},
		{name: "warnings pass", args: []string{"--config", clean, "review", "--plan", plan, "--context", plan}, want: exitPass, wantStdout: "[warning] scope new.go:1 extra"},
		{name: "blocking", args: []string{"--config", blocking, "review", "--plan", plan}, want: exitBlocking, wantStdout: "[blocking] acceptance"},
		{name: "json", args: []string{"--config", blocking, "review", "--plan", plan, "--json"}, want: exitBlocking, wantStdout: `"acceptance_ref":"AC1"`},
		{name: "malformed fails closed", args: []string{"--config", malformed, "review", "--plan", plan}, want: exitAgent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tt.args, &stdout, &stderr); got != tt.want {
				t.Fatalf("run(%v) = %d, want %d; stderr: %s", tt.args, got, tt.want, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}
