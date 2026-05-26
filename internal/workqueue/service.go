package workqueue

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	JobTypeCollect = "collect"
	JobTypeAnalyze = "analyze"
	JobTypeReport  = "report"

	PriorityLow    = 10
	PriorityNormal = 50
	PriorityHigh   = 90

	StatusPending     = "pending"
	StatusSucceeded   = "succeeded"
	StatusRetrying    = "retrying"
	StatusCompensated = "compensated"
)

type JobInput struct {
	Type        string            `json:"type"`
	Priority    int               `json:"priority"`
	MaxAttempts int               `json:"maxAttempts"`
	Payload     map[string]string `json:"payload"`
}

type Job struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Priority    int               `json:"priority"`
	MaxAttempts int               `json:"maxAttempts"`
	Attempts    int               `json:"attempts"`
	Status      string            `json:"status"`
	Payload     map[string]string `json:"payload"`
	CreatedAt   time.Time         `json:"createdAt"`
}

type Compensation struct {
	ID        string    `json:"id"`
	JobID     string    `json:"jobId"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

type WorkerPoolConfig struct {
	Workers int `json:"workers"`
	MaxJobs int `json:"maxJobs"`
}

type WorkerPoolResult struct {
	Workers   int            `json:"workers"`
	Processed int            `json:"processed"`
	ByType    map[string]int `json:"byType"`
}

type Service struct {
	mu            sync.Mutex
	nextJob       int
	nextComp      int
	pending       []Job
	jobs          map[string]Job
	compensations []Compensation
}

func NewService() *Service {
	return &Service{
		nextJob:  1,
		nextComp: 1,
		jobs:     make(map[string]Job),
	}
}

func (s *Service) Enqueue(input JobInput) Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	job := normalizeJob(input, fmt.Sprintf("job_%d", s.nextJob))
	s.nextJob++
	s.jobs[job.ID] = job
	s.pending = append(s.pending, job)
	s.sortPendingLocked()
	return cloneJob(job)
}

func (s *Service) Dequeue() (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dequeueLocked()
}

func (s *Service) FailJob(id string, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return
	}
	s.removePendingLocked(id)
	job.Attempts++
	if job.Attempts < job.MaxAttempts {
		job.Status = StatusRetrying
		s.jobs[id] = job
		s.pending = append(s.pending, job)
		s.sortPendingLocked()
		return
	}
	job.Status = StatusCompensated
	s.jobs[id] = job
	compensation := Compensation{
		ID:        fmt.Sprintf("comp_%d", s.nextComp),
		JobID:     id,
		Reason:    strings.TrimSpace(reason),
		CreatedAt: time.Now().UTC(),
	}
	s.nextComp++
	s.compensations = append(s.compensations, compensation)
}

func (s *Service) RunWorkerPool(config WorkerPoolConfig) WorkerPoolResult {
	workers := config.Workers
	if workers <= 0 {
		workers = 1
	}
	maxJobs := config.MaxJobs
	if maxJobs <= 0 {
		maxJobs = len(s.ListPendingJobs())
	}
	result := WorkerPoolResult{
		Workers: workers,
		ByType:  make(map[string]int),
	}
	for result.Processed < maxJobs {
		job, ok := s.Dequeue()
		if !ok {
			break
		}
		s.completeJob(job.ID)
		result.Processed++
		result.ByType[job.Type]++
	}
	return result
}

func (s *Service) ListPendingJobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs := make([]Job, 0, len(s.pending))
	for _, job := range s.pending {
		jobs = append(jobs, cloneJob(job))
	}
	return jobs
}

func (s *Service) ListCompensations() []Compensation {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := append([]Compensation(nil), s.compensations...)
	return result
}

func (s *Service) dequeueLocked() (Job, bool) {
	if len(s.pending) == 0 {
		return Job{}, false
	}
	job := s.pending[0]
	s.pending = s.pending[1:]
	return cloneJob(job), true
}

func (s *Service) completeJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return
	}
	job.Status = StatusSucceeded
	s.jobs[id] = job
}

func (s *Service) sortPendingLocked() {
	sort.SliceStable(s.pending, func(i, j int) bool {
		if s.pending[i].Priority == s.pending[j].Priority {
			return s.pending[i].CreatedAt.Before(s.pending[j].CreatedAt)
		}
		return s.pending[i].Priority > s.pending[j].Priority
	})
}

func (s *Service) removePendingLocked(id string) {
	filtered := s.pending[:0]
	for _, job := range s.pending {
		if job.ID == id {
			continue
		}
		filtered = append(filtered, job)
	}
	s.pending = filtered
}

func normalizeJob(input JobInput, id string) Job {
	maxAttempts := input.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	priority := input.Priority
	if priority <= 0 {
		priority = PriorityNormal
	}
	return Job{
		ID:          id,
		Type:        normalizeJobType(input.Type),
		Priority:    priority,
		MaxAttempts: maxAttempts,
		Status:      StatusPending,
		Payload:     cloneStringMap(input.Payload),
		CreatedAt:   time.Now().UTC(),
	}
}

func normalizeJobType(jobType string) string {
	switch strings.TrimSpace(jobType) {
	case JobTypeAnalyze:
		return JobTypeAnalyze
	case JobTypeReport:
		return JobTypeReport
	default:
		return JobTypeCollect
	}
}

func cloneJob(job Job) Job {
	job.Payload = cloneStringMap(job.Payload)
	return job
}

func cloneStringMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
