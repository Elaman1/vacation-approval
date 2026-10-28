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

func TestCreateVacationRequest(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "vacation_test" {
		t.Fatalf("unsafe test database %q: expected vacation_test", config.ConnConfig.Database)
	}
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Создаём сотрудника в тестовой БД.
	userID := fmt.Sprintf("test-%d", time.Now().UnixNano())

	var employeeID int64
	err = db.QueryRow(ctx, `
        INSERT INTO employees (keycloak_user_id, balance_days)
        VALUES ($1, $2)
        RETURNING id
    `, userID, 20).Scan(&employeeID)

	if err != nil {
		t.Fatal(err)
	}

	// Удаляем созданные тестовые данные.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cancel()

		_, err := db.Exec(cleanupCtx,
			"DELETE FROM vacation_requests WHERE employee_id = $1",
			employeeID,
		)
		if err != nil {
			t.Errorf("cleanup requests: %v", err)
		}

		_, err = db.Exec(cleanupCtx,
			"DELETE FROM employees WHERE id = $1",
			employeeID,
		)
		if err != nil {
			t.Errorf("cleanup employee: %v", err)
		}
	}()

	// Собираем сервис из реальных Repository.
	employeeRepo := NewEmployeeRepository(db)
	vacationRepo := NewVacationRepository(db)

	service := vacation.NewService(
		employeeRepo,
		vacationRepo,
	)

	start, _ := time.Parse("2006-01-02", "2026-11-10")
	end, _ := time.Parse("2006-01-02", "2026-11-16")

	request, err := service.CreateRequest(
		ctx,
		vacation.CreateRequestInput{
			UserID:    userID,
			StartDate: start,
			EndDate:   end,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if request.ID == 0 {
		t.Error("expected generated request ID")
	}

	if request.Days != 7 {
		t.Errorf("expected 7 days, got %d", request.Days)
	}

	if request.Status != vacation.StatusPendingBalance {
		t.Errorf("unexpected status: %s", request.Status)
	}

	// Проверяем, что запись реально существует в БД.
	var savedDays int
	var savedStatus string

	err = db.QueryRow(ctx, `
        SELECT days, status
        FROM vacation_requests
        WHERE id = $1
    `, request.ID).Scan(&savedDays, &savedStatus)

	if err != nil {
		t.Fatal(err)
	}

	if savedDays != 7 || savedStatus != "pending_balance" {
		t.Errorf("unexpected saved request: %d, %s",
			savedDays, savedStatus)
	}

	listed, err := service.ListRequests(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	if len(listed) != 1 {
		t.Fatalf("expected 1 vacation request, got %d", len(listed))
	}

	if listed[0].ID != request.ID {
		t.Errorf(
			"expected request ID %d, got %d",
			request.ID,
			listed[0].ID,
		)
	}

}
