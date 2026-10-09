// Package lock freezes a plan's acceptance tests so the implementer cannot
// redefine done by editing them.
package lock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/waygatetech/vet/internal/critique"
	"github.com/waygatetech/vet/internal/review"
)

// Lock records the test files a plan's tests globs matched when locked.
type Lock struct {
	Ticket string            `json:"ticket"`
	Head   string            `json:"head"`  // HEAD SHA at lock time
	Tests  []string          `json:"tests"` // the plan's tests globs
	Files  map[string]string `json:"files"` // path to sha256 hex
}

// Plan reads ticket and tests from a plan's frontmatter.
func Plan(plan []byte) (ticket string, tests []string, err error) {
	fm, err := critique.Frontmatter(plan)
	if err != nil {
		return "", nil, err
	}
	var meta struct {
		Ticket string   `yaml:"ticket"`
		Tests  []string `yaml:"tests"`
	}
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		return "", nil, fmt.Errorf("parsing plan frontmatter: %w", err)
	}
	if meta.Ticket == "" || meta.Ticket != filepath.Base(meta.Ticket) || meta.Ticket == ".." {
		return "", nil, fmt.Errorf("plan frontmatter: invalid ticket %q", meta.Ticket)
	}
	return meta.Ticket, meta.Tests, nil
}

// Path is where a ticket's lock lives, relative to the repo root.
func Path(ticket string) string {
	return filepath.Join(".vet", "locks", ticket+".json")
}

// Snapshot hashes the files, tracked or untracked but not ignored, that
// match the tests globs.
func Snapshot(ctx context.Context, ticket string, tests []string) (Lock, error) {
	head, err := git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return Lock{}, err
	}
	list, err := git(ctx, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Lock{}, err
	}
	l := Lock{Ticket: ticket, Head: strings.TrimSpace(head), Tests: tests, Files: map[string]string{}}
	for f := range strings.SplitSeq(strings.TrimRight(list, "\x00"), "\x00") {
		if f == "" || !slices.ContainsFunc(tests, func(p string) bool { return review.Match(p, f) }) {
			continue
		}
		data, err := os.ReadFile(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue // tracked but deleted from the worktree
		}
		if err != nil {
			return Lock{}, fmt.Errorf("hashing test file: %w", err)
		}
		sum := sha256.Sum256(data)
		l.Files[f] = hex.EncodeToString(sum[:])
	}
	return l, nil
}

// Read loads a lock file.
func Read(path string) (Lock, error) {
	var l Lock
	data, err := os.ReadFile(path)
	if err != nil {
		return l, fmt.Errorf("reading lock: %w", err)
	}
	if err := json.Unmarshal(data, &l); err != nil {
		return l, fmt.Errorf("parsing lock %s: %w", path, err)
	}
	return l, nil
}

// Committed loads the lock at path as committed at merge-base(base, HEAD).
// The bool is false if the lock is not in that commit.
func Committed(ctx context.Context, base, path string) (Lock, bool, error) {
	var l Lock
	mb, err := git(ctx, "merge-base", base, "HEAD")
	if err != nil {
		return l, false, err
	}
	spec := strings.TrimSpace(mb) + ":" + filepath.ToSlash(path)
	if ls, err := git(ctx, "ls-tree", "--name-only", strings.TrimSpace(mb), "--", filepath.ToSlash(path)); err != nil || ls == "" {
		return l, false, err
	}
	data, err := git(ctx, "show", spec)
	if err != nil {
		return l, false, err
	}
	if err := json.Unmarshal([]byte(data), &l); err != nil {
		return l, false, fmt.Errorf("parsing committed lock %s: %w", spec, err)
	}
	return l, true, nil
}

// Write saves a lock file, creating its directory.
func Write(path string, l Lock) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding lock: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing lock: %w", err)
	}
	return nil
}

// Diff lists how current differs from locked; empty means unchanged.
func Diff(locked, current Lock) []string {
	var diffs []string
	if !slices.Equal(locked.Tests, current.Tests) {
		diffs = append(diffs, fmt.Sprintf("tests globs changed: %v -> %v", locked.Tests, current.Tests))
	}
	for _, f := range slices.Sorted(maps.Keys(locked.Files)) {
		sum, ok := current.Files[f]
		switch {
		case !ok:
			diffs = append(diffs, "removed: "+f)
		case sum != locked.Files[f]:
			diffs = append(diffs, "changed: "+f)
		}
	}
	for _, f := range slices.Sorted(maps.Keys(current.Files)) {
		if _, ok := locked.Files[f]; !ok {
			diffs = append(diffs, "added: "+f)
		}
	}
	return diffs
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
