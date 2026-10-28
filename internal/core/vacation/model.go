package vacation

import "time"

type Status string

const (
	StatusPendingBalance   Status = "pending_balance"
	StatusPendingManager   Status = "pending_manager"
	StatusAutoApproved     Status = "auto_approved"
	StatusApproved         Status = "approved"
	StatusRejected         Status = "rejected"
	StatusNeedsHRReview    Status = "needs_hr_review"
	StatusStartUnconfirmed Status = "start_unconfirmed"
)

type Employee struct {
	ID             int64
	KeycloakUserID string
	BalanceDays    int
}

type Request struct {
	ID         int64
	EmployeeID int64
	StartDate  time.Time
	EndDate    time.Time
	Days       int
	Status     Status
}

func IsFinalDecision(status Status) bool {
	switch status {
	case StatusAutoApproved, StatusApproved, StatusRejected, StatusNeedsHRReview:
		return true
	default:
		return false
	}
}

func ConsumesBalance(status Status) bool {
	return status == StatusApproved || status == StatusAutoApproved
}
