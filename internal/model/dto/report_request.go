package dto

import "time"

// CreateReportRequest is the request body for POST /api/v1/reports.
type CreateReportRequest struct {
	ReportType  string `json:"report_type" example:"weekly"`
	PeriodStart string `json:"period_start,omitempty" example:"2026-06-24"`
	PeriodEnd   string `json:"period_end,omitempty" example:"2026-06-30"`
	Send        bool   `json:"send" example:"false"`
}

// ToInput converts the request into a CreateInput for the service layer.
func (r CreateReportRequest) ToInput() (CreateInput, error) {
	var start *time.Time
	if r.PeriodStart != "" {
		parsed, err := time.Parse("2006-01-02", r.PeriodStart)
		if err != nil {
			return CreateInput{}, err
		}
		start = &parsed
	}
	var end *time.Time
	if r.PeriodEnd != "" {
		parsed, err := time.Parse("2006-01-02", r.PeriodEnd)
		if err != nil {
			return CreateInput{}, err
		}
		end = &parsed
	}
	return CreateInput{
		ReportType:  r.ReportType,
		PeriodStart: start,
		PeriodEnd:   end,
		Send:        r.Send,
	}, nil
}
