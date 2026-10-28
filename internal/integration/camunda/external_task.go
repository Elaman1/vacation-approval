package camunda

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Variable struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type ExternalTask struct {
	ID        string              `json:"id"`
	TopicName string              `json:"topicName"`
	Retries   *int                `json:"retries"`
	Variables map[string]Variable `json:"variables"`
}

func (t ExternalTask) RequestID() (int64, error) {
	v, ok := t.Variables["requestId"]
	if !ok || v.Type != "Long" {
		return 0, fmt.Errorf("missing or invalid Long requestId")
	}
	var n int64
	if err := json.Unmarshal(v.Value, &n); err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid requestId: %s", v.Value)
	}
	return n, nil
}

func (t ExternalTask) Decision() (string, error) {
	v, ok := t.Variables["decision"]
	if !ok || v.Type != "String" {
		return "", fmt.Errorf("missing or invalid String decision")
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err != nil {
		return "", fmt.Errorf("decode decision: %w", err)
	}
	return s, nil
}

type Client struct {
	baseURL  string
	workerID string
	http     *http.Client
}

func NewClient(baseURL, workerID string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		workerID: workerID,
		http:     &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) StartVacation(ctx context.Context, requestID int64) (string, error) {
	body := map[string]any{
		"businessKey": fmt.Sprintf("vacation-%d", requestID),
		"variables": map[string]any{
			"requestId":       map[string]any{"type": "Long", "value": requestID},
			"managerDecision": map[string]any{"type": "String", "value": nil},
		},
	}
	var result struct {
		ID string `json:"id"`
	}
	err := c.post(ctx, "/process-definition/key/vacation_approval/start", body, http.StatusOK, &result)
	if err != nil {
		return "", err
	}
	if result.ID == "" {
		return "", fmt.Errorf("Camunda returned no process instance id")
	}
	return result.ID, nil
}

func (c *Client) FetchTasks(ctx context.Context) ([]ExternalTask, error) {
	body := map[string]any{
		"workerId":             c.workerID,
		"maxTasks":             3,
		"asyncResponseTimeout": 10000,
		"topics": []map[string]any{
			{"topicName": "check-balance", "lockDuration": 60000, "variables": []string{"requestId"}},
			{"topicName": "mark-pending-manager", "lockDuration": 60000, "variables": []string{"requestId"}},
			{"topicName": "finalize-vacation", "lockDuration": 60000, "variables": []string{"requestId", "decision"}},
		},
	}
	var tasks []ExternalTask
	if err := c.post(ctx, "/external-task/fetchAndLock", body, http.StatusOK, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (c *Client) Complete(ctx context.Context, taskID string, variables map[string]any) error {
	body := map[string]any{"workerId": c.workerID, "variables": variables}
	return c.post(ctx, "/external-task/"+taskID+"/complete", body, http.StatusNoContent, nil)
}

func BoolVar(v bool) map[string]any {
	return map[string]any{"type": "Boolean", "value": v}
}

func IntVar(v int) map[string]any {
	return map[string]any{"type": "Integer", "value": v}
}

func StringVar(v string) map[string]any {
	return map[string]any{"type": "String", "value": v}
}

func (c *Client) Fail(ctx context.Context, task ExternalTask, cause error) error {
	retries := 2
	if task.Retries != nil {
		retries = *task.Retries - 1
		if retries < 0 {
			retries = 0
		}
	}
	body := map[string]any{
		"workerId":     c.workerID,
		"errorMessage": cause.Error(),
		"retries":      retries,
		"retryTimeout": 10000,
	}
	return c.post(ctx, "/external-task/"+task.ID+"/failure", body, http.StatusNoContent, nil)
}

func (c *Client) post(ctx context.Context, path string, body any, expected int, result any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expected {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("Camunda HTTP %d: %s", resp.StatusCode, msg)
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
