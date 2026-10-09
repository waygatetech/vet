package findings

import (
	"context"
	"encoding/json"
	"testing"
)

func TestReportJSON(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		wantErr      bool
		wantBlocking bool
		wantOut      string
	}{
		{
			name:    "empty",
			in:      `{"findings":[]}`,
			wantOut: `{"findings":[]}`,
		},
		{
			name:    "missing findings encodes as empty array",
			in:      `{}`,
			wantOut: `{"findings":[]}`,
		},
		{
			name:         "blocking",
			in:           `{"findings":[{"check":"paths","severity":"blocking","file":"main.go","line":12,"message":"out of scope"}]}`,
			wantBlocking: true,
			wantOut:      `{"findings":[{"check":"paths","severity":"blocking","file":"main.go","line":12,"message":"out of scope"}]}`,
		},
		{
			name:    "warning only",
			in:      `{"findings":[{"check":"style","severity":"warning","message":"nit"}]}`,
			wantOut: `{"findings":[{"check":"style","severity":"warning","message":"nit"}]}`,
		},
		{
			name:    "unknown severity",
			in:      `{"findings":[{"check":"x","severity":"critical","message":"m"}]}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r Report
			err := json.Unmarshal([]byte(tt.in), &r)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Unmarshal error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got := r.Blocking(); got != tt.wantBlocking {
				t.Errorf("Blocking() = %v, want %v", got, tt.wantBlocking)
			}
			out, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tt.wantOut {
				t.Errorf("Marshal = %s, want %s", out, tt.wantOut)
			}
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		want    int // number of findings
		wantErr bool
	}{
		{name: "report", script: `echo '{"findings":[{"check":"acceptance","severity":"blocking","message":"m"}]}'`, want: 1},
		{name: "fenced", script: "printf '```json\\n{\"findings\":[]}\\n```\\n'", want: 0},
		{name: "reads prompt", script: `grep -q '===== PLAN =====' && echo '{"findings":[]}'`, want: 0},
		{name: "not json", script: `echo looks good to me`, wantErr: true},
		{name: "missing findings key", script: `echo '{}'`, wantErr: true},
		{name: "bad severity", script: `echo '{"findings":[{"check":"x","severity":"meh","message":"m"}]}'`, wantErr: true},
		{name: "nonzero exit", script: `echo '{"findings":[]}'; exit 1`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Run(context.Background(), []string{"sh", "-c", tt.script}, "===== PLAN =====\n")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && len(r.Findings) != tt.want {
				t.Errorf("got %d findings, want %d", len(r.Findings), tt.want)
			}
		})
	}
}
