// Package findings defines the JSON report shared by vet's critique and
// review checks.
package findings

import (
	"encoding/json"
	"fmt"
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
	Message  string   `json:"message"`
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
