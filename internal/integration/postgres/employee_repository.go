package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vacation-approval/internal/core/vacation"
)

type EmployeeRepository struct {
	db *pgxpool.Pool
}

func NewEmployeeRepository(db *pgxpool.Pool) *EmployeeRepository {
	return &EmployeeRepository{
		db: db,
	}
}

func (r *EmployeeRepository) GetByKeycloakUserID(
	ctx context.Context,
	userID string,
) (vacation.Employee, error) {
	var employee vacation.Employee

	err := r.db.QueryRow(ctx, `
        SELECT id, keycloak_user_id, balance_days
        FROM employees
        WHERE keycloak_user_id = $1
    `, userID).Scan(
		&employee.ID,
		&employee.KeycloakUserID,
		&employee.BalanceDays,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return vacation.Employee{}, vacation.ErrEmployeeNotFound
	}

	if err != nil {
		return vacation.Employee{}, fmt.Errorf("get employee: %w", err)
	}

	return employee, nil
}
