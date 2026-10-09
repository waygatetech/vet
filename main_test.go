package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("nope: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		args       []string
		want       int
		wantStdout string
	}{
		{name: "version", args: []string{"--version"}, want: exitPass, wantStdout: "dev"},
		{name: "no args shows help", args: nil, want: exitPass, wantStdout: "not go vet"},
		{name: "unknown flag", args: []string{"--bogus"}, want: exitUsage},
		{name: "unexpected arg", args: []string{"bogus"}, want: exitUsage},
		{name: "missing explicit config", args: []string{"--config", filepath.Join(dir, "missing.yaml")}, want: exitUsage},
		{name: "invalid config", args: []string{"--config", bad}, want: exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tt.args, &stdout, &stderr); got != tt.want {
				t.Fatalf("run(%v) = %d, want %d; stderr: %s", tt.args, got, tt.want, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}
