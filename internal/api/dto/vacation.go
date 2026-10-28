package dto

type CreateVacationRequest struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type VacationResponse struct {
	ID        int64  `json:"id"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Days      int    `json:"days"`
	Status    string `json:"status"`
}
