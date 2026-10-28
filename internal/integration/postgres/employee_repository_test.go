//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vacation-approval/internal/core/vacation"
)

func TestEmployeeRepository_Integration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is not set")
	}

	// Проверяем конфигурацию подключения.
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse database config: %v", err)
	}

	// Защита от случайного запуска на основной БД.
	if config.ConnConfig.Database != "vacation_test" {
		t.Fatalf(
			"unsafe database %q: expected vacation_test",
			config.ConnConfig.Database,
		)
	}

	db, err := pgxpool.NewWithConfig(
		context.Background(),
		config,
	)
	if err != nil {
		t.Fatalf("create connection pool: %v", err)
	}
	t.Cleanup(db.Close)

	// Проверяем реальное подключение к PostgreSQL.
	pingCtx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := db.Ping(pingCtx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	// Создаём уникального тестового сотрудника.
	userID := fmt.Sprintf(
		"test-user-%d",
		time.Now().UnixNano(),
	)

	insertCtx, insertCancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)

	_, err = db.Exec(insertCtx, `
		INSERT INTO employees (
			keycloak_user_id,
			balance_days
		)
		VALUES ($1, $2)
	`, userID, 20)

	insertCancel()

	if err != nil {
		t.Fatalf("insert test employee: %v", err)
	}

	// Очистка выполняется даже при падении теста.
	// Регистрируем после создания пула:
	// cleanup сотрудника выполнится раньше db.Close().
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		_, err := db.Exec(ctx,
			"DELETE FROM employees WHERE keycloak_user_id = $1",
			userID,
		)
		if err != nil {
			t.Errorf("cleanup test employee: %v", err)
		}
	})

	repo := NewEmployeeRepository(db)

	t.Run("existing employee", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		employee, err := repo.GetByKeycloakUserID(
			ctx,
			userID,
		)
		if err != nil {
			t.Fatalf("get employee: %v", err)
		}

		if employee.BalanceDays != 20 {
			t.Errorf(
				"expected balance 20, got %d",
				employee.BalanceDays,
			)
		}

		if employee.KeycloakUserID != userID {
			t.Errorf(
				"expected user ID %q, got %q",
				userID,
				employee.KeycloakUserID,
			)
		}
	})

	t.Run("employee not found", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		_, err := repo.GetByKeycloakUserID(
			ctx,
			userID+"-missing",
		)

		if !errors.Is(err, vacation.ErrEmployeeNotFound) {
			t.Errorf(
				"expected ErrEmployeeNotFound, got %v",
				err,
			)
		}
	})
}
