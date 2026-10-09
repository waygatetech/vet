package review

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/waygatetech/vet/internal/config"
	"github.com/waygatetech/vet/internal/critique"
)

func TestFiles(t *testing.T) {
	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n" +
		"diff --git a/old.go b/dir/new.go\nrename from old.go\n" +
		"diff --git \"a/sp ace.go\" \"b/sp ace.go\"\n" +
		"diff --git a/new.txt b/new.txt\nnew file mode 100644\n+diff --git a/fake b/fake\n"
	want := []string{"a.go", "dir/new.go", "sp ace.go", "new.txt"}
	if got := Files(diff); !slices.Equal(got, want) {
		t.Errorf("Files() = %q, want %q", got, want)
	}
}

func TestContractsChanged(t *testing.T) {
	tests := []struct {
		name    string
		plan    string
		want    bool
		wantErr bool
	}{
		{name: "listed", plan: "---\ncontracts_changed:\n  - api\n---\n", want: true},
		{name: "empty list", plan: "---\ncontracts_changed: []\n---\n"},
		{name: "no frontmatter", plan: "body\n"},
		{name: "unclosed", plan: "---\ncontracts_changed: [x]\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ContractsChanged([]byte(tt.plan))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ContractsChanged() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ContractsChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTriggered(t *testing.T) {
	when := &config.Trigger{ContractsChanged: true, MinTier: 2, Paths: []string{"migrations/**", "**/*.sql", "go.mod"}}
	tests := []struct {
		name      string
		when      *config.Trigger
		contracts bool
		tier      int
		files     []string
		want      bool
	}{
		{name: "no when always runs", want: true},
		{name: "nothing matches", when: when, tier: 1, files: []string{"main.go", "sub/go.mod"}},
		{name: "contracts changed", when: when, contracts: true, want: true},
		{name: "tier at minimum", when: when, tier: 2, want: true},
		{name: "glob prefix", when: when, files: []string{"migrations/2026/01.up"}, want: true},
		{name: "doublestar matches zero segments", when: when, files: []string{"schema.sql"}, want: true},
		{name: "doublestar matches deep", when: when, files: []string{"a/b/c.sql"}, want: true},
		{name: "exact path", when: when, files: []string{"go.mod"}, want: true},
		{name: "min_tier 0 is off", when: &config.Trigger{Paths: []string{"x"}}, tier: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := config.Reviewer{Name: "r", Command: []string{"x"}, When: tt.when}
			if got := Triggered(r, tt.contracts, tt.tier, tt.files); got != tt.want {
				t.Errorf("Triggered() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
