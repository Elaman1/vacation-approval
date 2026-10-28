package clickhouse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vacation-approval/internal/core/vacation"
)

func TestInsertDecision(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Query().Get("query"), "INSERT INTO vacation.decisions") {
			t.Errorf("unexpected method or SQL: %s %s", r.Method, r.URL.RawQuery)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "vacation" || password != "vacation" {
			t.Error("basic auth credentials not supplied")
		}
		defer r.Body.Close()
		var row map[string]any
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			t.Error(err)
		}
		if row["decision"] != "approved" || row["event_id"] != float64(42) {
			t.Errorf("unexpected JSON row: %v", row)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "vacation", "vacation")
	event := vacation.DecisionEvent{
		EventID: 42, RequestID: 10, EmployeeID: 3, Days: 6,
		Decision:   vacation.StatusApproved,
		OccurredAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
	}
	if err := client.Insert(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("HTTP server was not called")
	}
}
