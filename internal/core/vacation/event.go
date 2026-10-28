package vacation

import "time"

// DecisionEvent is the event contract shared by the Kafka publisher and consumer.
// The outbox id is stable across publication retries.
type DecisionEvent struct {
	EventID    int64     `json:"event_id"`
	RequestID  int64     `json:"request_id"`
	EmployeeID int64     `json:"employee_id"`
	Days       int       `json:"days"`
	Decision   Status    `json:"decision"`
	OccurredAt time.Time `json:"occurred_at"`
}
