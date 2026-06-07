package mail

import (
	"context"
	"sync"
	"time"
)

type memoryRepository struct {
	mu         sync.Mutex
	now        time.Time
	reports    map[string]DailyReport
	recipients map[string]Recipient
	deliveries []Delivery
}

func newMemoryRepository(now time.Time) *memoryRepository {
	return &memoryRepository{
		now:        now,
		reports:    make(map[string]DailyReport),
		recipients: make(map[string]Recipient),
	}
}

func (r *memoryRepository) DailyReportByID(_ context.Context, reportID string) (DailyReport, error) {
	report, ok := r.reports[reportID]
	if !ok {
		return DailyReport{}, ErrNotFound
	}
	return report, nil
}

func (r *memoryRepository) RecipientByUserID(_ context.Context, userID string) (Recipient, error) {
	recipient, ok := r.recipients[userID]
	if !ok {
		return Recipient{}, ErrNotFound
	}
	return recipient, nil
}

func (r *memoryRepository) CreateDelivery(_ context.Context, delivery Delivery) (Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delivery.ID = "delivery-1"
	delivery.CreatedAt = r.now
	delivery.UpdatedAt = r.now
	r.deliveries = append(r.deliveries, delivery)
	return delivery, nil
}

func (r *memoryRepository) UpdateDelivery(_ context.Context, delivery Delivery) (Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.deliveries) == 0 {
		return Delivery{}, ErrNotFound
	}
	delivery.UpdatedAt = r.now
	r.deliveries[len(r.deliveries)-1] = delivery
	return delivery, nil
}

func (r *memoryRepository) FindDeliveryByReportAndUser(_ context.Context, reportID, userID string) (Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.deliveries {
		if d.ReportID == reportID && d.RecipientUserID == userID {
			return d, nil
		}
	}
	return Delivery{}, ErrNotFound
}
