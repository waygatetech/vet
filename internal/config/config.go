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

	"gopkg.in/yaml.v3"
)

// DefaultPath is the config file vet reads when --config is not given.
const DefaultPath = ".vet.yaml"

// Config is the contents of .vet.yaml. Unknown keys are rejected.
type Config struct {
	LogLevel slog.Level `yaml:"log_level"`
}

// Load reads the config at path. A missing file yields defaults unless the
// path was given explicitly, in which case it is an error.
func Load(path string, explicit bool) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !explicit {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return cfg, nil
}
