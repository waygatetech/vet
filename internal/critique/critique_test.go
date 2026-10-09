package critique

import (
	"slices"
	"strings"
	"testing"

	"github.com/waygatetech/vet/internal/findings"
)

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
