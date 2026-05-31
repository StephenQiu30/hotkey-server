package report

import "context"

func (s *Service) ListReportsByDate(ctx context.Context, date string) ([]DailyReport, error) {
	if date == "" {
		return nil, ErrInvalidInput
	}
	return s.reports.ListReportsByDate(ctx, date)
}

func (s *Service) FindReportByID(ctx context.Context, id string) (DailyReport, error) {
	if id == "" {
		return DailyReport{}, ErrInvalidInput
	}
	return s.reports.FindReportByID(ctx, id)
}
