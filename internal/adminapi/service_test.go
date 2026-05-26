package adminapi

import "testing"

func TestTaskRunRecordsIncludeFailures(t *testing.T) {
	service := NewService()

	service.RecordTaskRun(TaskRunInput{
		TaskName: "daily-report",
		Status:   TaskStatusSucceeded,
		Message:  "platform report generated",
	})
	service.RecordTaskRun(TaskRunInput{
		TaskName: "source-refresh",
		Status:   TaskStatusFailed,
		Message:  "source timeout",
	})

	runs := service.ListTaskRuns(ListTaskRunsOptions{})
	if len(runs) != 2 {
		t.Fatalf("runs len = %d, want 2", len(runs))
	}

	failures := service.ListTaskRuns(ListTaskRunsOptions{Status: TaskStatusFailed})
	if len(failures) != 1 {
		t.Fatalf("failures len = %d, want 1", len(failures))
	}
	if failures[0].TaskName != "source-refresh" || failures[0].Message != "source timeout" {
		t.Fatalf("failure = %#v", failures[0])
	}
}

func TestTriggerDailyReportRecordsTaskRun(t *testing.T) {
	service := NewService()

	run := service.RecordTaskRun(TaskRunInput{
		TaskName: "daily-report",
		Status:   TaskStatusSucceeded,
		Message:  "manual trigger accepted",
	})

	if run.ID == "" {
		t.Fatalf("run id is empty")
	}
	if run.TaskName != "daily-report" {
		t.Fatalf("task name = %q, want daily-report", run.TaskName)
	}
	if run.Status != TaskStatusSucceeded {
		t.Fatalf("status = %q, want %q", run.Status, TaskStatusSucceeded)
	}
}
