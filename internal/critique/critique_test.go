package critique

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/waygatetech/vet/internal/findings"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		want    int // number of findings
		wantErr bool
	}{
		{name: "report", script: `echo '{"findings":[{"check":"acceptance","severity":"blocking","message":"m"}]}'`, want: 1},
		{name: "fenced", script: "printf '```json\\n{\"findings\":[]}\\n```\\n'", want: 0},
		{name: "reads prompt", script: `grep -q '===== PLAN =====' && echo '{"findings":[]}'`, want: 0},
		{name: "not json", script: `echo looks good to me`, wantErr: true},
		{name: "missing findings key", script: `echo '{}'`, wantErr: true},
		{name: "bad severity", script: `echo '{"findings":[{"check":"x","severity":"meh","message":"m"}]}'`, wantErr: true},
		{name: "nonzero exit", script: `echo '{"findings":[]}'; exit 1`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := Prompt("ticket", "plan", "cli: {}", nil)
			r, err := Run(context.Background(), []string{"sh", "-c", tt.script}, prompt)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && len(r.Findings) != tt.want {
				t.Errorf("got %d findings, want %d", len(r.Findings), tt.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	critique := Render(findings.Report{Findings: []findings.Finding{
		{Check: "failure-mode", Severity: findings.Blocking, Message: "a"},
		{Check: "contract", Severity: findings.Warning, Message: "b"},
	}})
	tests := []struct {
		name     string
		critique []byte
		plan     string
		want     []string
		wantErr  bool
	}{
		{name: "all answered", critique: critique, plan: "---\ncritique_responses:\n  C1: fixed\n  C2: accepted\n---\nbody\n"},
		{name: "one missing", critique: critique, plan: "---\ncritique_responses:\n  C1: fixed\n---\n", want: []string{"C2"}},
		{name: "empty response", critique: critique, plan: "---\ncritique_responses:\n  C1: \" \"\n  C2: ok\n---\n", want: []string{"C1"}},
		{name: "no frontmatter", critique: critique, plan: "just prose\n", want: []string{"C1", "C2"}},
		{name: "no items", critique: Render(findings.Report{}), plan: "---\nticket: x\n---\n"},
		{name: "unclosed frontmatter", critique: critique, plan: "---\nticket: x\n", wantErr: true},
		{name: "bad responses type", critique: critique, plan: "---\ncritique_responses: [C1]\n---\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Check(tt.critique, []byte(tt.plan))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Check() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPromptIncludesInputs(t *testing.T) {
	p := Prompt("TKT", "PLN", "CNC", []File{{Name: "why.txt", Content: "CTX"}})
	for _, s := range []string{"TKT", "PLN", "CNC", "CONTEXT: why.txt", "CTX"} {
		if !strings.Contains(p, s) {
			t.Errorf("prompt missing %q", s)
		}
	}
}
