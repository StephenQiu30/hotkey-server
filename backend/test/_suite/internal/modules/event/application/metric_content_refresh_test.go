package application

import (
	"context"
	"testing"
	"time"
)

type contentMicroEventReaderFake struct {
	microEventIDs []int64
}

func (fake contentMicroEventReaderFake) ListMetricMicroEventIDsForContent(context.Context, int64) ([]int64, error) {
	return append([]int64(nil), fake.microEventIDs...), nil
}

type eventHeatCalculatorFake struct {
	commands []CalculateEventHeatCommand
}

func (fake *eventHeatCalculatorFake) Calculate(_ context.Context, command CalculateEventHeatCommand) (CalculateEventHeatResult, error) {
	fake.commands = append(fake.commands, command)
	return CalculateEventHeatResult{}, nil
}

func TestContentMetricRefreshRecomputesOnlyMicroEventHeatWindows(t *testing.T) {
	now := time.Date(2026, time.August, 12, 7, 43, 29, 0, time.UTC)
	calculator := &eventHeatCalculatorFake{}
	service, err := NewContentMetricRefreshServiceWithClock(
		contentMicroEventReaderFake{microEventIDs: []int64{3, 7}}, calculator,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("NewContentMetricRefreshServiceWithClock: %v", err)
	}
	if err := service.RecomputeMetricsForContent(context.Background(), 11); err != nil {
		t.Fatalf("RecomputeMetricsForContent: %v", err)
	}
	want := []CalculateEventHeatCommand{
		{MicroEventID: 3, WindowHours: 1, WindowEndedAt: now.Truncate(time.Minute)},
		{MicroEventID: 3, WindowHours: 6, WindowEndedAt: now.Truncate(time.Minute)},
		{MicroEventID: 3, WindowHours: 24, WindowEndedAt: now.Truncate(time.Minute)},
		{MicroEventID: 7, WindowHours: 1, WindowEndedAt: now.Truncate(time.Minute)},
		{MicroEventID: 7, WindowHours: 6, WindowEndedAt: now.Truncate(time.Minute)},
		{MicroEventID: 7, WindowHours: 24, WindowEndedAt: now.Truncate(time.Minute)},
	}
	if len(calculator.commands) != len(want) {
		t.Fatalf("heat commands = %#v, want %#v", calculator.commands, want)
	}
	for index := range want {
		if calculator.commands[index] != want[index] {
			t.Fatalf("heat command[%d] = %#v, want %#v", index, calculator.commands[index], want[index])
		}
	}
}
