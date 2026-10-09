// Package review runs a fresh-context reviewer over a plan and the diff that
// implements it.
package review

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/waygatetech/vet/internal/config"
	"github.com/waygatetech/vet/internal/critique"
)

const instructions = `You are reviewing a code change against the plan it implements. You have not
seen the implementation conversation; judge only what is written below.

Find:
- acceptance: acceptance criteria or planned behavior the diff does not satisfy.
- correctness: bugs, regressions, or unhandled failures introduced by the diff.
- scope: changes the plan does not call for.
- ruling: changes that contradict a human ruling in the context files without
  a supersedes entry in the plan.

Raise only concrete, specific issues, with file and line where possible. Set
acceptance_ref to the criterion a finding relates to, if any. Use severity
"blocking" for issues that must be fixed before the change is done, "warning"
for real risks, and "info" for minor notes. An empty findings list is a valid
answer.

Reply with only this JSON and nothing else:
{"findings": [{"check": "acceptance|correctness|scope|ruling", "severity": "blocking|warning|info", "file": "...", "line": 0, "acceptance_ref": "...", "message": "..."}]}
`

// Prompt builds the reviewer's entire input. The reviewer sees nothing else.
func Prompt(plan, diff string, context []critique.File) string {
	var b strings.Builder
	b.WriteString(instructions)
	section := func(name, body string) {
		fmt.Fprintf(&b, "\n===== %s =====\n%s\n", name, strings.TrimRight(body, "\n"))
	}
	section("PLAN", plan)
	section("DIFF", diff)
	for _, f := range context {
		section("CONTEXT: "+f.Name, f.Content)
	}
	return b.String()
}

// Diff returns the changes since the merge base of base and HEAD, including
// uncommitted edits and untracked files, which plain git diff omits.
func Diff(ctx context.Context, base string) (string, error) {
	mb, err := git(ctx, "merge-base", base, "HEAD")
	if err != nil {
		return "", err
	}
	diff, err := git(ctx, "diff", strings.TrimSpace(mb))
	if err != nil {
		return "", err
	}
	untracked, err := git(ctx, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(diff)
	for f := range strings.SplitSeq(strings.TrimRight(untracked, "\x00"), "\x00") {
		if f == "" {
			continue
		}
		// --no-index exits 1 when the files differ, which they always do here.
		out, err := exec.CommandContext(ctx, "git", "diff", "--no-index", "--", "/dev/null", f).Output()
		var ee *exec.ExitError
		if err != nil && !(errors.As(err, &ee) && ee.ExitCode() == 1) {
			return "", fmt.Errorf("diffing untracked file %s: %w", f, err)
		}
		b.Write(out)
	}
	return b.String(), nil
}

// ContractsChanged reports whether the plan's frontmatter lists any
// contracts_changed entries.
func ContractsChanged(plan []byte) (bool, error) {
	fm, err := critique.Frontmatter(plan)
	if err != nil {
		return false, err
	}
	var meta struct {
		ContractsChanged []string `yaml:"contracts_changed"`
	}
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		return false, fmt.Errorf("parsing plan frontmatter: %w", err)
	}
	return len(meta.ContractsChanged) > 0, nil
}

// Files returns the paths a diff touches, read from its "diff --git" headers.
// For renames it returns the new path.
func Files(diff string) []string {
	var files []string
	for line := range strings.Lines(diff) {
		line = strings.TrimRight(line, "\n")
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		// git quotes paths with unusual characters: diff --git "a/x y" "b/x y"
		if i := strings.LastIndex(line, ` "b/`); i >= 0 && strings.HasSuffix(line, `"`) {
			if f, err := strconv.Unquote(line[i+1:]); err == nil {
				files = append(files, strings.TrimPrefix(f, "b/"))
			}
			continue
		}
		if i := strings.LastIndex(line, " b/"); i >= 0 {
			files = append(files, line[i+len(" b/"):])
		}
	}
	return files
}

// Triggered reports whether reviewer r should run for this change. A
// reviewer without a when always runs; otherwise any matching condition
// triggers it.
func Triggered(r config.Reviewer, contractsChanged bool, tier int, files []string) bool {
	w := r.When
	if w == nil {
		return true
	}
	if w.ContractsChanged && contractsChanged {
		return true
	}
	if w.MinTier > 0 && tier >= w.MinTier {
		return true
	}
	for _, f := range files {
		for _, p := range w.Paths {
			if match(strings.Split(p, "/"), strings.Split(f, "/")) {
				return true
			}
		}
	}
	return false
}

// match is path.Match per segment, plus "**" matching zero or more segments.
func match(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		for i := range len(name) + 1 {
			if match(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	ok, _ := path.Match(pattern[0], name[0])
	return ok && match(pattern[1:], name[1:])
}

func git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}
