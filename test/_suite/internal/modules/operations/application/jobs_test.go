package application_test

import (
	"context"
	"testing"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
)

type jobStoreFake struct {
	job operationsdomain.JobSummary
}

func (fake *jobStoreFake) ListJobs(context.Context, operationsdomain.JobListQuery) (operationsdomain.JobPage, error) {
	return operationsdomain.JobPage{Items: []operationsdomain.JobSummary{fake.job}}, nil
}
func (fake *jobStoreFake) CancelJob(context.Context, int64) (operationsdomain.JobSummary, error) {
	return fake.job, nil
}
func (fake *jobStoreFake) RetryJob(context.Context, int64) (operationsdomain.JobSummary, error) {
	return fake.job, nil
}

func TestJobServiceValidatesQueriesAndMutationActors(t *testing.T) {
	fake := &jobStoreFake{job: operationsdomain.JobSummary{ID: 1, Kind: "collect_source", State: operationsdomain.JobAvailable, MaxAttempts: 3, Priority: 1, ScheduledAt: time.Now().UTC(), CreatedAt: time.Now().UTC()}}
	service, err := operationsapplication.NewJobService(fake, nil)
	if err != nil {
		t.Fatalf("NewJobService() error = %v", err)
	}
	if _, err := service.List(context.Background(), operationsdomain.JobListQuery{Limit: 101}); err == nil {
		t.Fatal("invalid list query was accepted")
	}
	if _, err := service.Retry(context.Background(), operationsdomain.JobMutationInput{ActorID: 0, JobID: 1}); err == nil {
		t.Fatal("missing actor was accepted")
	}
}
