// Package config loads vet's .vet.yaml configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	pathpkg "path"

	"gopkg.in/yaml.v3"
)

// DefaultPath is the config file vet reads when --config is not given.
const DefaultPath = ".vet.yaml"

// Config is the contents of .vet.yaml. Unknown keys are rejected.
type Config struct {
	LogLevel slog.Level `yaml:"log_level"`
	// CriticCommand is the argv vet critique runs; the prompt goes on stdin.
	// Defaults to claude -p.
	CriticCommand []string `yaml:"critic_command"`
	// Reviewers are the agents vet review runs. Defaults to one reviewer,
	// claude, running claude -p.
	Reviewers []Reviewer `yaml:"reviewers"`
	// TestCommand is the argv vet lock --check runs to prove the locked tests
	// pass, e.g. [go, test, ./...]. Unset disables vet lock.
	TestCommand []string `yaml:"test_command"`
}

// Reviewer is one agent vet review may run; the prompt goes on stdin.
type Reviewer struct {
	Name    string   `yaml:"name"`
	Command []string `yaml:"command"`
	// When limits the reviewer to some changes. Nil means it always runs.
	When *Trigger `yaml:"when"`
}

// Trigger fires a reviewer when any of its set conditions holds.
type Trigger struct {
	ContractsChanged bool     `yaml:"contracts_changed"` // plan declares contracts_changed
	MinTier          int      `yaml:"min_tier"`          // --tier is at least this; 0 disables
	Paths            []string `yaml:"paths"`             // diff touches a matching file; ** spans directories
}

// Load reads the config at path. A missing file yields defaults unless the
// path was given explicitly, in which case it is an error.
func Load(path string, explicit bool) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !explicit {
		data, err = nil, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("parsing config %s: %w", path, err)
	}
	if len(cfg.CriticCommand) == 0 {
		cfg.CriticCommand = []string{"claude", "-p"}
	}
	if len(cfg.Reviewers) == 0 {
		cfg.Reviewers = []Reviewer{{Name: "claude", Command: []string{"claude", "-p"}}}
	}
	seen := map[string]bool{}
	for i, r := range cfg.Reviewers {
		switch {
		case r.Name == "":
			return cfg, fmt.Errorf("config %s: reviewer %d has no name", path, i+1)
		case seen[r.Name]:
			return cfg, fmt.Errorf("config %s: duplicate reviewer %q", path, r.Name)
		case len(r.Command) == 0:
			return cfg, fmt.Errorf("config %s: reviewer %q has no command", path, r.Name)
		case r.When != nil && !r.When.ContractsChanged && r.When.MinTier <= 0 && len(r.When.Paths) == 0:
			return cfg, fmt.Errorf("config %s: reviewer %q has an empty when; omit it to always run", path, r.Name)
		}
		if r.When != nil {
			for _, p := range r.When.Paths {
				if _, err := pathpkg.Match(p, ""); err != nil {
					return cfg, fmt.Errorf("config %s: reviewer %q path %q: %w", path, r.Name, p, err)
				}
			}
		}
		seen[r.Name] = true
	}
	return cfg, nil
}
