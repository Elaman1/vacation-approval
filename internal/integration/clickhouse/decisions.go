package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vacation-approval/internal/core/vacation"
)

const insertSQL = "INSERT INTO vacation.decisions (event_id, request_id, employee_id, days, decision, occurred_at) FORMAT JSONEachRow"

type Client struct {
	baseURL  string
	user     string
	password string
	http     *http.Client
}

func NewClient(baseURL, user, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		user:     user,
		password: password,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

// Insert uses JSONEachRow; duplicate events may arrive after consumer crashes.
// ClickHouse ReplacingMergeTree stores one logical row per event_id; use FINAL
// when querying immediately, since background deduplication is asynchronous.
func (c *Client) Insert(ctx context.Context, event vacation.DecisionEvent) error {
	if event.EventID <= 0 || !vacation.IsFinalDecision(event.Decision) {
		return fmt.Errorf("invalid decision event %d (%s)", event.EventID, event.Decision)
	}
	row := struct {
		EventID    int64  `json:"event_id"`
		RequestID  int64  `json:"request_id"`
		EmployeeID int64  `json:"employee_id"`
		Days       int    `json:"days"`
		Decision   string `json:"decision"`
		OccurredAt string `json:"occurred_at"`
	}{
		EventID:    event.EventID,
		RequestID:  event.RequestID,
		EmployeeID: event.EmployeeID,
		Days:       event.Days,
		Decision:   string(event.Decision),
		OccurredAt: event.OccurredAt.UTC().Format("2006-01-02 15:04:05.000"),
	}
	payload, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("encode clickhouse row: %w", err)
	}
	payload = append(payload, '\n')

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/?query="+url.QueryEscape(insertSQL), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.user, c.password)
	req.Header.Set("Content-Type", "application/x-ndjson")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("insert event %d into ClickHouse: %w", event.EventID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("clickhouse HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}
