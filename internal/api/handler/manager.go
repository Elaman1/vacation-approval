package handler

import (
	"context"
	"log"
	"net/http"
	"time"

	"vacation-approval/internal/core/vacation"
)

type PendingLister interface {
	ListPendingManager(ctx context.Context) ([]vacation.Request, error)
}

type ManagerHandler struct {
	requests PendingLister
}

func NewManagerHandler(requests PendingLister) *ManagerHandler {
	return &ManagerHandler{requests: requests}
}

func (h *ManagerHandler) Pending(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	requests, err := h.requests.ListPendingManager(ctx)
	if err != nil {
		log.Printf("list pending manager requests: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	result := make([]any, 0, len(requests))
	for _, req := range requests {
		result = append(result, map[string]any{
			"id":          req.ID,
			"employee_id": req.EmployeeID,
			"start_date":  req.StartDate.Format("2006-01-02"),
			"end_date":    req.EndDate.Format("2006-01-02"),
			"days":        req.Days,
			"status":      req.Status,
		})
	}
	writeJSON(w, http.StatusOK, result)
}
