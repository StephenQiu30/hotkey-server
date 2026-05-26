package workqueue

import "testing"

func TestQueueDispatchesHigherPriorityJobsFirst(t *testing.T) {
	service := NewService()
	low := service.Enqueue(JobInput{Type: JobTypeReport, Priority: PriorityLow, Payload: map[string]string{"date": "2026-05-25"}})
	high := service.Enqueue(JobInput{Type: JobTypeCollect, Priority: PriorityHigh, Payload: map[string]string{"source": "arxiv-ai"}})

	first, ok := service.Dequeue()
	if !ok {
		t.Fatalf("expected first job")
	}
	if first.ID != high.ID {
		t.Fatalf("first job = %s, want high priority %s; low=%s", first.ID, high.ID, low.ID)
	}
}

func TestFailedJobsRetryAndThenCompensate(t *testing.T) {
	service := NewService()
	job := service.Enqueue(JobInput{
		Type:        JobTypeAnalyze,
		Priority:    PriorityNormal,
		MaxAttempts: 2,
		Payload:     map[string]string{"event": "cluster_openai"},
	})

	service.FailJob(job.ID, "model timeout")
	retry, ok := service.Dequeue()
	if !ok {
		t.Fatalf("expected retry job")
	}
	if retry.ID != job.ID || retry.Attempts != 1 {
		t.Fatalf("retry job = %#v", retry)
	}

	service.FailJob(job.ID, "model timeout")
	compensations := service.ListCompensations()
	if len(compensations) != 1 {
		t.Fatalf("compensations len = %d, want 1", len(compensations))
	}
	if compensations[0].JobID != job.ID || compensations[0].Reason == "" {
		t.Fatalf("compensation = %#v", compensations[0])
	}
}

func TestWorkerPoolProcessesCollectionAnalysisAndReportJobs(t *testing.T) {
	service := NewService()
	service.Enqueue(JobInput{Type: JobTypeReport, Priority: PriorityLow})
	service.Enqueue(JobInput{Type: JobTypeAnalyze, Priority: PriorityNormal})
	service.Enqueue(JobInput{Type: JobTypeCollect, Priority: PriorityHigh})

	result := service.RunWorkerPool(WorkerPoolConfig{Workers: 2, MaxJobs: 3})
	if result.Processed != 3 {
		t.Fatalf("processed = %d, want 3", result.Processed)
	}
	if result.ByType[JobTypeCollect] != 1 || result.ByType[JobTypeAnalyze] != 1 || result.ByType[JobTypeReport] != 1 {
		t.Fatalf("by type = %#v", result.ByType)
	}
	if len(service.ListPendingJobs()) != 0 {
		t.Fatalf("pending jobs should be empty")
	}
}
