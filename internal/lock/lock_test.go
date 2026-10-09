package lock

import (
	"slices"
	"testing"
)

func TestDiff(t *testing.T) {
	locked := Lock{Tests: []string{"*_test.go"}, Files: map[string]string{"a_test.go": "1", "b_test.go": "2"}}
	tests := []struct {
		name    string
		current Lock
		want    []string
	}{
		{name: "unchanged", current: locked, want: nil},
		{name: "changed", current: Lock{Tests: locked.Tests, Files: map[string]string{"a_test.go": "x", "b_test.go": "2"}}, want: []string{"changed: a_test.go"}},
		{name: "added and removed", current: Lock{Tests: locked.Tests, Files: map[string]string{"a_test.go": "1", "c_test.go": "3"}}, want: []string{"removed: b_test.go", "added: c_test.go"}},
		{name: "globs changed", current: Lock{Tests: []string{"x_test.go"}, Files: locked.Files}, want: []string{"tests globs changed: [*_test.go] -> [x_test.go]"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Diff(locked, tt.current); !slices.Equal(got, tt.want) {
				t.Errorf("Diff = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlan(t *testing.T) {
	tests := []struct {
		name       string
		plan       string
		wantTicket string
		wantTests  []string
		wantErr    bool
	}{
		{name: "tests", plan: "---\nticket: x-1\ntests: [a_test.go]\n---\n", wantTicket: "x-1", wantTests: []string{"a_test.go"}},
		{name: "no tests", plan: "---\nticket: x-1\n---\n", wantTicket: "x-1"},
		{name: "no ticket", plan: "---\ntests: [a]\n---\n", wantErr: true},
		{name: "path ticket", plan: "---\nticket: ../x\n---\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket, got, err := Plan([]byte(tt.plan))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Plan err = %v, wantErr %v", err, tt.wantErr)
			}
			if ticket != tt.wantTicket || !slices.Equal(got, tt.wantTests) {
				t.Errorf("Plan = %q, %q; want %q, %q", ticket, got, tt.wantTicket, tt.wantTests)
			}
		})
	}
}
