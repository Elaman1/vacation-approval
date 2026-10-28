package vacation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// EmployeeReader and RequestRepository are defined by what this service needs.
type EmployeeReader interface {
	GetByKeycloakUserID(ctx context.Context, userID string) (Employee, error)
}

type RequestRepository interface {
	Create(ctx context.Context, request Request) (Request, error)
	ListByEmployeeID(ctx context.Context, employeeID int64) ([]Request, error)
	AttachProcess(ctx context.Context, requestID int64, processID string) error
	MarkStartUnconfirmed(ctx context.Context, requestID int64) error
}

type ProcessStarter interface {
	StartVacation(ctx context.Context, requestID int64) (processInstanceID string, err error)
}

var ErrProcessStartUnconfirmed = errors.New("camunda process start not confirmed")

type Service struct {
	employees EmployeeReader
	requests  RequestRepository
}

func NewService(employees EmployeeReader, requests RequestRepository) *Service {
	return &Service{employees: employees, requests: requests}
}

type CreateRequestInput struct {
	UserID    string
	StartDate time.Time
	EndDate   time.Time
}

// CreateRequest only creates a database row; tests may exercise this without Camunda.
func (s *Service) CreateRequest(ctx context.Context, input CreateRequestInput) (Request, error) {
	if strings.TrimSpace(input.UserID) == "" {
		return Request{}, errors.New("user ID is required")
	}
	if input.StartDate.IsZero() || input.EndDate.IsZero() {
		return Request{}, ErrInvalidDateRange
	}
	days, err := CountDays(input.StartDate, input.EndDate)
	if err != nil {
		return Request{}, err
	}
	employee, err := s.employees.GetByKeycloakUserID(ctx, input.UserID)
	if err != nil {
		return Request{}, err
	}
	return s.requests.Create(ctx, Request{
		EmployeeID: employee.ID,
		StartDate:  input.StartDate,
		EndDate:    input.EndDate,
		Days:       days,
		Status:     StatusPendingBalance,
	})
}

// CreateAndStart is the HTTP use case: persist first, then start Camunda.
// Cross-system atomicity is not guaranteed; failures are explicitly marked for reconciliation.
func (s *Service) CreateAndStart(ctx context.Context, input CreateRequestInput, starter ProcessStarter) (Request, error) {
	request, err := s.CreateRequest(ctx, input)
	if err != nil {
		return Request{}, err
	}
	id, err := starter.StartVacation(ctx, request.ID)
	if err != nil {
		// A network timeout can mean Camunda started but the reply was lost.
		// Do NOT automatically retry this call or we could create a duplicate instance.
		markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if markErr := s.requests.MarkStartUnconfirmed(markCtx, request.ID); markErr != nil {
			return request, fmt.Errorf("start Camunda: %w; mark status: %v", ErrProcessStartUnconfirmed, markErr)
		}
		request.Status = StatusStartUnconfirmed
		return request, fmt.Errorf("%w: %v", ErrProcessStartUnconfirmed, err)
	}
	if err := s.requests.AttachProcess(ctx, request.ID, id); err != nil {
		return request, fmt.Errorf("attach process instance %s: %w", id, err)
	}
	return request, nil
}

func (s *Service) ListRequests(ctx context.Context, userID string) ([]Request, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID is required")
	}
	employee, err := s.employees.GetByKeycloakUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.requests.ListByEmployeeID(ctx, employee.ID)
}
