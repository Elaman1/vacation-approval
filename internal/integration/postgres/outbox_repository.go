package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vacation-approval/internal/core/vacation"
)

type OutboxRepository struct {
	db *pgxpool.Pool
}

func NewOutboxRepository(db *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// Pending selects events in creation order. The MVP runs a single publisher.
// With multiple publishers this method would need a lease/claim mechanism.
func (r *OutboxRepository) Pending(ctx context.Context, limit int) ([]vacation.DecisionEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT o.id, o.request_id, v.employee_id, v.days, o.decision, o.created_at
		FROM vacation_outbox o
		JOIN vacation_requests v ON v.id = o.request_id
		WHERE o.published_at IS NULL
		ORDER BY o.id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("select outbox events: %w", err)
	}
	defer rows.Close()

	result := make([]vacation.DecisionEvent, 0)
	for rows.Next() {
		var event vacation.DecisionEvent
		var decision string
		if err := rows.Scan(&event.EventID, &event.RequestID, &event.EmployeeID, &event.Days, &decision, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		event.Decision = vacation.Status(decision)
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox events: %w", err)
	}
	return result, nil
}

// MarkPublished happens only AFTER Kafka has acknowledged the publish.
// A failure here may cause a duplicate Kafka message; that is intentional
// at-least-once delivery, not data loss.
func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command, err := r.db.Exec(ctx, `
		UPDATE vacation_outbox
		SET published_at = NOW()
		WHERE id = $1 AND published_at IS NULL
	`, eventID)
	if err != nil {
		return fmt.Errorf("mark outbox event %d published: %w", eventID, err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("outbox event %d not found or already published", eventID)
	}
	return nil
}
