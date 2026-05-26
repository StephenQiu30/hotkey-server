package adminapi

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	TaskStatusSucceeded = "succeeded"
	TaskStatusFailed    = "failed"
	TaskStatusRunning   = "running"
)

type TaskRun struct {
	ID         string    `json:"id"`
	TaskName   string    `json:"taskName"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
}

type TaskRunInput struct {
	TaskName string
	Status   string
	Message  string
}

type ListTaskRunsOptions struct {
	Status string
}

type Service struct {
	mu      sync.Mutex
	nextID  int
	taskRun []TaskRun
}

func NewService() *Service {
	return &Service{
		nextID: 1,
	}
}

func (s *Service) RecordTaskRun(input TaskRunInput) TaskRun {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	run := TaskRun{
		ID:         "task_run_" + strconv.Itoa(s.nextID),
		TaskName:   strings.TrimSpace(input.TaskName),
		Status:     normalizeTaskStatus(input.Status),
		Message:    strings.TrimSpace(input.Message),
		StartedAt:  now,
		FinishedAt: now,
	}
	s.nextID++
	s.taskRun = append(s.taskRun, run)
	return run
}

func (s *Service) ListTaskRuns(options ListTaskRunsOptions) []TaskRun {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := strings.TrimSpace(options.Status)
	runs := make([]TaskRun, 0, len(s.taskRun))
	for i := len(s.taskRun) - 1; i >= 0; i-- {
		run := s.taskRun[i]
		if status != "" && run.Status != status {
			continue
		}
		runs = append(runs, run)
	}
	return runs
}

func normalizeTaskStatus(status string) string {
	switch strings.TrimSpace(status) {
	case TaskStatusFailed:
		return TaskStatusFailed
	case TaskStatusRunning:
		return TaskStatusRunning
	default:
		return TaskStatusSucceeded
	}
}
