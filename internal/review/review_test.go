package review

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/waygatetech/vet/internal/critique"
)

func TestPromptIncludesInputs(t *testing.T) {
	p := Prompt("PLN", "DIF", []critique.File{{Name: "why.txt", Content: "CTX"}})
	for _, s := range []string{"PLN", "DIF", "CONTEXT: why.txt", "CTX"} {
		if !strings.Contains(p, s) {
			t.Errorf("prompt missing %q", s)
		}
	}
}

func TestDiff(t *testing.T) {
	t.Chdir(t.TempDir())
	sh := func(script string) {
		t.Helper()
		cmd := exec.Command("sh", "-c", script)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v: %s", script, err, out)
		}
	}
	sh(`git init -q -b main && echo base > a.txt && git add . && git commit -qm base &&
		git checkout -qb feature && echo committed > b.txt && git add . && git commit -qm b &&
		echo edited >> a.txt && echo new > c.txt && echo ignored > d.log && echo '*.log' > .gitignore`)

	tests := []struct {
		name    string
		base    string
		want    []string
		notWant []string
		wantErr bool
	}{
		{name: "branch work, edits, untracked", base: "main", want: []string{"+committed", "+edited", "+new", "c.txt"}, notWant: []string{"ignored"}},
		{name: "head only", base: "HEAD", want: []string{"+edited", "+new"}, notWant: []string{"+committed"}},
		{name: "bad base", base: "nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Diff(context.Background(), tt.base)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Diff() error = %v, wantErr %v", err, tt.wantErr)
			}
			for _, s := range tt.want {
				if !strings.Contains(got, s) {
					t.Errorf("diff missing %q:\n%s", s, got)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(got, s) {
					t.Errorf("diff contains %q:\n%s", s, got)
				}
			}
		})
	}
}
