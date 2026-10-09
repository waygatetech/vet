package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		contents *string // nil means the file does not exist
		explicit bool
		want     slog.Level
		wantErr  bool
	}{
		{name: "missing default file", want: slog.LevelInfo},
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
		})
	}
}

func ptr(s string) *string { return &s }
