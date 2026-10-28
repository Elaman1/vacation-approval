package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"vacation-approval/internal/api/dto"
	"vacation-approval/internal/api/middleware"
	"vacation-approval/internal/core/vacation"
)

type VacationHandler struct {
	service *vacation.Service
	starter vacation.ProcessStarter
}

func NewVacationHandler(service *vacation.Service, starter vacation.ProcessStarter) *VacationHandler {
	return &VacationHandler{service: service, starter: starter}
}

func (h *VacationHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input dto.CreateVacationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expected one JSON object"})
		return
	}

	start, err := time.Parse("2006-01-02", input.StartDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid start_date"})
		return
	}
	end, err := time.Parse("2006-01-02", input.EndDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid end_date"})
		return
	}

	request, err := h.service.CreateAndStart(r.Context(), vacation.CreateRequestInput{
		UserID: userID, StartDate: start, EndDate: end,
	}, h.starter)
	if err != nil {
		switch {
		case errors.Is(err, vacation.ErrInvalidDateRange):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid date range"})
		case errors.Is(err, vacation.ErrEmployeeNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
		case errors.Is(err, vacation.ErrProcessStartUnconfirmed):
			log.Printf("camunda start unconfirmed for request %d: %v", request.ID, err)
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error": "Camunda start not confirmed; check request before retrying", "request_id": request.ID,
			})
		default:
			log.Printf("create vacation: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": "internal server error", "request_id": request.ID,
			})
		}
		return
	}
	writeJSON(w, http.StatusCreated, toVacationResponse(request))
}

func (h *VacationHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	requests, err := h.service.ListRequests(r.Context(), userID)
	if err != nil {
		if errors.Is(err, vacation.ErrEmployeeNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return
		}
		log.Printf("list vacations: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	response := make([]dto.VacationResponse, 0, len(requests))
	for _, request := range requests {
		response = append(response, toVacationResponse(request))
	}
	writeJSON(w, http.StatusOK, response)
}

func toVacationResponse(request vacation.Request) dto.VacationResponse {
	return dto.VacationResponse{
		ID:        request.ID,
		StartDate: request.StartDate.Format("2006-01-02"),
		EndDate:   request.EndDate.Format("2006-01-02"),
		Days:      request.Days,
		Status:    string(request.Status),
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}
