// Package findings defines the JSON report shared by vet's critique and
// review checks.
package findings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Severity ranks a finding. Only Blocking findings fail a gate.
type Severity string

const (
	Blocking Severity = "blocking"
	Warning  Severity = "warning"
	Info     Severity = "info"
)

// UnmarshalJSON rejects severities outside the known set, so a malformed
// reviewer response fails instead of silently passing.
func (s *Severity) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("decoding severity: %w", err)
	}
	switch Severity(v) {
	case Blocking, Warning, Info:
		*s = Severity(v)
		return nil
	}
	return fmt.Errorf("unknown severity %q", v)
}

// Finding is one issue raised by a check.
type Finding struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	// AcceptanceRef names the acceptance criterion a review finding relates to.
	AcceptanceRef string `json:"acceptance_ref,omitempty"`
	Message       string `json:"message"`
	// Reviewer names the configured reviewer that raised a review finding.
	// vet sets it; agent output cannot.
	Reviewer string `json:"reviewer,omitempty"`
}

// Report is the top-level JSON document vet emits.
type Report struct {
	Findings []Finding `json:"findings"`
}

// MarshalJSON emits an empty array rather than null when there are no findings.
func (r Report) MarshalJSON() ([]byte, error) {
	type plain Report
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
	return json.Marshal(plain(r))
}

// Blocking reports whether any finding should fail the gate.
func (r Report) Blocking() bool {
	for _, f := range r.Findings {
		if f.Severity == Blocking {
			return true
		}
	}
	return false
}

// Run executes the agent argv with prompt on stdin and parses its findings
// report. Any failure, including malformed output, is an error so the gate
// fails closed.
func Run(ctx context.Context, argv []string, prompt string) (Report, error) {
	if len(argv) == 0 {
		return Report{}, errors.New("agent command is empty")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Report{}, fmt.Errorf("running agent %q: %w: %s", argv[0], err, strings.TrimSpace(stderr.String()))
	}
	return parse(out)
}

// parse extracts the outermost JSON object, tolerating code fences or prose
// around it, and requires a findings key.
func parse(out []byte) (Report, error) {
	start, end := bytes.IndexByte(out, '{'), bytes.LastIndexByte(out, '}')
	if start < 0 || end < start {
		return Report{}, fmt.Errorf("agent output has no JSON object: %q", out)
	}
	var raw struct {
		Findings *[]Finding `json:"findings"`
	}
	dec := json.NewDecoder(bytes.NewReader(out[start : end+1]))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return Report{}, fmt.Errorf("decoding agent output: %w", err)
	}
	if raw.Findings == nil {
		return Report{}, errors.New("agent output is missing \"findings\"")
	}
	return Report{Findings: *raw.Findings}, nil
}
