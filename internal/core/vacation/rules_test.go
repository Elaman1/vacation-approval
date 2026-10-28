package vacation

import (
	"testing"
	"time"
)

func TestCountDays(t *testing.T) {
	tests := []struct {
		name    string
		start   string
		end     string
		want    int
		wantErr bool
	}{
		{
			name:  "three days",
			start: "2026-11-10",
			end:   "2026-11-12",
			want:  3,
		},
		{
			name:  "cross month",
			start: "2026-11-30",
			end:   "2026-12-02",
			want:  3,
		},
		{
			name:    "invalid range",
			start:   "2026-11-15",
			end:     "2026-11-10",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, err := time.Parse("2006-01-02", tt.start)
			if err != nil {
				t.Fatal(err)
			}

			end, err := time.Parse("2006-01-02", tt.end)
			if err != nil {
				t.Fatal(err)
			}

			got, err := CountDays(start, end)

			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("got %d days, want %d", got, tt.want)
			}
		})
	}
}

func TestHasEnoughBalance(t *testing.T) {
	tests := []struct {
		balance   int
		requested int
		want      bool
	}{
		{20, 7, true},
		{5, 7, false},
		{7, 7, true},
		{20, 0, false},
		{20, -2, false},
	}

	for _, tt := range tests {
		got := HasEnoughBalance(tt.balance, tt.requested)

		if got != tt.want {
			t.Errorf(
				"balance=%d requested=%d: got %v, want %v",
				tt.balance, tt.requested, got, tt.want,
			)
		}
	}
}
