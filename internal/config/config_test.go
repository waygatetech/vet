package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name         string
		contents     *string // nil means the file does not exist
		explicit     bool
		want         slog.Level
		wantCmd      []string // nil means the default
		wantReviewer []string // nil means the default
		wantErr      bool
	}{
		{name: "missing default file", want: slog.LevelInfo},
		{name: "critic command", contents: ptr("critic_command: [codex, exec]\n"), wantCmd: []string{"codex", "exec"}},
		{name: "reviewer command", contents: ptr("reviewer_command: [codex, exec]\n"), wantReviewer: []string{"codex", "exec"}},
		{name: "missing explicit file", explicit: true, wantErr: true},
		{name: "empty file", contents: ptr(""), want: slog.LevelInfo},
		{name: "log level", contents: ptr("log_level: debug\n"), want: slog.LevelDebug},
		{name: "bad log level", contents: ptr("log_level: loud\n"), wantErr: true},
		{name: "unknown key", contents: ptr("log_levle: debug\n"), wantErr: true},
		{name: "invalid yaml", contents: ptr("log_level: [\n"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), DefaultPath)
			if tt.contents != nil {
				if err := os.WriteFile(path, []byte(*tt.contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load(path, tt.explicit)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && cfg.LogLevel != tt.want {
				t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, tt.want)
			}
			wantCmd := tt.wantCmd
			if wantCmd == nil {
				wantCmd = []string{"claude", "-p"}
			}
			if err == nil && !slices.Equal(cfg.CriticCommand, wantCmd) {
				t.Errorf("CriticCommand = %v, want %v", cfg.CriticCommand, wantCmd)
			}
			wantReviewer := tt.wantReviewer
			if wantReviewer == nil {
				wantReviewer = []string{"claude", "-p"}
			}
			if err == nil && !slices.Equal(cfg.ReviewerCommand, wantReviewer) {
				t.Errorf("ReviewerCommand = %v, want %v", cfg.ReviewerCommand, wantReviewer)
			}
		})
	}
}

func ptr(s string) *string { return &s }
