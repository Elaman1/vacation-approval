package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vacation-approval/internal/core/vacation"
)

func TestFinalizeIdempotentAndBalance(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "vacation_test" {
		t.Fatalf("unsafe test database %q: expected vacation_test", config.ConnConfig.Database)
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	userID := fmt.Sprintf("finalization-test-%d", time.Now().UnixNano())
	var employeeID int64
	if err := db.QueryRow(ctx, `
        INSERT INTO employees(keycloak_user_id, balance_days)
        VALUES ($1, 20) RETURNING id
    `, userID).Scan(&employeeID); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM vacation_outbox WHERE request_id IN (SELECT id FROM vacation_requests WHERE employee_id=$1)`, employeeID); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM vacation_requests WHERE employee_id=$1`, employeeID); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM employees WHERE id=$1`, employeeID); err != nil {
			t.Error(err)
		}
	})

	repo := NewVacationRepository(db)
	create := func(days int) int64 {
		t.Helper()
		r, err := repo.Create(ctx, vacation.Request{
			EmployeeID: employeeID,
			StartDate:  time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC),
			EndDate:    time.Date(2026, 11, 10+days-1, 0, 0, 0, 0, time.UTC),
			Days:       days,
			Status:     vacation.StatusPendingBalance,
		})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}

	first := create(7)
	for i := 0; i < 2; i++ {
		actual, err := repo.Finalize(ctx, first, vacation.StatusApproved)
		if err != nil || actual != vacation.StatusApproved {
			t.Fatalf("iteration %d: decision=%s, err=%v", i, actual, err)
		}
	}
	second := create(15)
	actual, err := repo.Finalize(ctx, second, vacation.StatusApproved)
	if err != nil || actual != vacation.StatusRejected {
		t.Fatalf("balance depleted: decision=%s, err=%v", actual, err)
	}
	var balance int
	if err := db.QueryRow(ctx, `SELECT balance_days FROM employees WHERE id=$1`, employeeID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 13 {
		t.Fatalf("expected 13 days after exactly one debit, got %d", balance)
	}
	var total, firstCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE request_id = $1) FROM vacation_outbox WHERE request_id IN ($1, $2)`, first, second).Scan(&total, &firstCount); err != nil {
		t.Fatal(err)
	}
	if total != 2 || firstCount != 1 {
		t.Fatalf("expected 2 outbox events (one for first request), got total=%d first=%d", total, firstCount)
	}
}
