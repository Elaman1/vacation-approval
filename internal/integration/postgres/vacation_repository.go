package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vacation-approval/internal/core/vacation"
)

type VacationRepository struct {
	db *pgxpool.Pool
}

func NewVacationRepository(db *pgxpool.Pool) *VacationRepository {
	return &VacationRepository{db: db}
}

func (r *VacationRepository) Create(ctx context.Context, request vacation.Request) (vacation.Request, error) {
	err := r.db.QueryRow(ctx, `
        INSERT INTO vacation_requests (employee_id, start_date, end_date, days, status)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id
    `, request.EmployeeID, request.StartDate, request.EndDate, request.Days, string(request.Status)).Scan(&request.ID)
	if err != nil {
		return vacation.Request{}, fmt.Errorf("create vacation request: %w", err)
	}
	return request, nil
}

func (r *VacationRepository) ListByEmployeeID(ctx context.Context, employeeID int64) ([]vacation.Request, error) {
	rows, err := r.db.Query(ctx, `
        SELECT id, employee_id, start_date, end_date, days, status
        FROM vacation_requests WHERE employee_id = $1
        ORDER BY created_at DESC, id DESC
    `, employeeID)
	if err != nil {
		return nil, fmt.Errorf("list vacation requests: %w", err)
	}
	defer rows.Close()

	requests := make([]vacation.Request, 0)
	for rows.Next() {
		var request vacation.Request
		var status string
		if err := rows.Scan(&request.ID, &request.EmployeeID, &request.StartDate, &request.EndDate, &request.Days, &status); err != nil {
			return nil, fmt.Errorf("scan vacation request: %w", err)
		}
		request.Status = vacation.Status(status)
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vacation requests: %w", err)
	}
	return requests, nil
}

func (r *VacationRepository) GetBalanceForRequest(ctx context.Context, requestID int64) (int, int, error) {
	var balance, requested int
	err := r.db.QueryRow(ctx, `
        SELECT e.balance_days, v.days FROM vacation_requests v
        JOIN employees e ON e.id = v.employee_id WHERE v.id = $1
    `, requestID).Scan(&balance, &requested)
	if err != nil {
		return 0, 0, fmt.Errorf("get balance for request %d: %w", requestID, err)
	}
	return balance, requested, nil
}

func (r *VacationRepository) AttachProcess(ctx context.Context, requestID int64, processID string) error {
	tag, err := r.db.Exec(ctx, `
        UPDATE vacation_requests SET camunda_process_id = $2 WHERE id = $1
    `, requestID, processID)
	if err != nil {
		return fmt.Errorf("attach camunda process: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("attach camunda process: request %d missing", requestID)
	}
	return nil
}

func (r *VacationRepository) MarkStartUnconfirmed(ctx context.Context, requestID int64) error {
	_, err := r.db.Exec(ctx, `
        UPDATE vacation_requests SET status = 'start_unconfirmed'
        WHERE id = $1 AND status = 'pending_balance' AND camunda_process_id IS NULL
    `, requestID)
	return err
}

// MarkPendingManager is safe if the same external task is retried.
func (r *VacationRepository) MarkPendingManager(ctx context.Context, requestID int64) error {
	tag, err := r.db.Exec(ctx, `
        UPDATE vacation_requests SET status = 'pending_manager'
        WHERE id = $1 AND status IN ('pending_balance', 'pending_manager', 'start_unconfirmed')
    `, requestID)
	if err != nil {
		return fmt.Errorf("mark pending manager: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("cannot mark request %d pending manager", requestID)
	}
	return nil
}

// Finalize atomically persists the DMN result and, when necessary, subtracts days.
// A repeated invocation never subtracts days twice.
func (r *VacationRepository) Finalize(ctx context.Context, requestID int64, decision vacation.Status) (vacation.Status, error) {
	if !vacation.IsFinalDecision(decision) {
		return "", fmt.Errorf("invalid DMN decision %q", decision)
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", fmt.Errorf("begin finalization: %w", err)
	}
	defer tx.Rollback(ctx)

	var employeeID int64
	var days int
	var current string
	err = tx.QueryRow(ctx, `
        SELECT employee_id, days, status FROM vacation_requests
        WHERE id = $1 FOR UPDATE
    `, requestID).Scan(&employeeID, &days, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("request %d does not exist", requestID)
	}
	if err != nil {
		return "", fmt.Errorf("lock vacation request: %w", err)
	}

	// A prior worker already committed the final result; do nothing.
	if vacation.IsFinalDecision(vacation.Status(current)) {
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return vacation.Status(current), nil
	}

	actual := decision
	if vacation.ConsumesBalance(decision) {
		// Conditional UPDATE is atomic and prevents a negative balance even
		// when several different requests are being finalized concurrently.
		tag, err := tx.Exec(ctx, `
            UPDATE employees SET balance_days = balance_days - $1
            WHERE id = $2 AND balance_days >= $1
        `, days, employeeID)
		if err != nil {
			return "", fmt.Errorf("deduct vacation balance: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// The balance changed since the initial BPMN check.
			actual = vacation.StatusRejected
		}
	}

	_, err = tx.Exec(ctx, `UPDATE vacation_requests SET status = $2 WHERE id = $1`, requestID, string(actual))
	if err != nil {
		return "", fmt.Errorf("save vacation decision: %w", err)
	}

	// Outbox is part of the SAME transaction as balance deduction and status.
	// A retry hitting a final status returns above without another event.
	_, err = tx.Exec(ctx, `
        INSERT INTO vacation_outbox (request_id, decision)
        VALUES ($1, $2)
        ON CONFLICT (request_id) DO NOTHING
    `, requestID, string(actual))
	if err != nil {
		return "", fmt.Errorf("store vacation decision event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit vacation decision: %w", err)
	}
	return actual, nil
}

// ListPendingManager is a read-only listing for manager-role API users.
func (r *VacationRepository) ListPendingManager(ctx context.Context) ([]vacation.Request, error) {
	rows, err := r.db.Query(ctx, `
        SELECT id, employee_id, start_date, end_date, days, status
        FROM vacation_requests
        WHERE status = 'pending_manager'
        ORDER BY created_at ASC, id ASC
    `)
	if err != nil {
		return nil, fmt.Errorf("list manager requests: %w", err)
	}
	defer rows.Close()

	requests := make([]vacation.Request, 0)
	for rows.Next() {
		var req vacation.Request
		var status string
		if err := rows.Scan(&req.ID, &req.EmployeeID, &req.StartDate, &req.EndDate, &req.Days, &status); err != nil {
			return nil, fmt.Errorf("scan manager request: %w", err)
		}
		req.Status = vacation.Status(status)
		requests = append(requests, req)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read manager requests: %w", err)
	}
	return requests, nil
}
