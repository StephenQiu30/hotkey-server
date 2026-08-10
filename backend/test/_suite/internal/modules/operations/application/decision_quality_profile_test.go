package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type decisionQualityClock struct{ value time.Time }

func (clock decisionQualityClock) Now() time.Time { return clock.value }

type decisionQualityRepositoryFake struct {
	command PersistDecisionQualityEvaluationCommand
	result  PersistDecisionQualityEvaluationResult
	err     error
}

func (fake *decisionQualityRepositoryFake) PersistDecisionQualityEvaluation(_ context.Context, command PersistDecisionQualityEvaluationCommand) (PersistDecisionQualityEvaluationResult, error) {
	fake.command = command
	if fake.err != nil {
		return PersistDecisionQualityEvaluationResult{}, fake.err
	}
	return fake.result, nil
}

func TestDecisionQualityServicePersistsAndActivatesOnlyPassingProfiles(t *testing.T) {
	dataset := passingDecisionQualityDataset(t)
	evaluation, err := EvaluateDecisionQuality(dataset)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	repository := &decisionQualityRepositoryFake{}
	for index, metric := range evaluation.Metrics {
		actor := int64(7)
		activated := now
		repository.result.Runs = append(repository.result.Runs, DecisionQualityEvaluationRunDTO{
			ID: int64(index + 1), Version: 1, Module: metric.Module, ProfileVersion: metric.ProfileVersion,
			DatasetVersion: dataset.DatasetVersion, DatasetSHA256: dataset.DatasetSHA256,
			SampleCount: metric.SampleCount, PositiveCount: metric.PositiveCount, NegativeCount: metric.NegativeCount,
			Passed: true, EvaluatedByUserID: actor, EvaluatedAt: now,
		})
		repository.result.Profiles = append(repository.result.Profiles, DecisionQualityProfileDTO{
			ID: int64(index + 11), Version: 1, EvaluationRunID: int64(index + 1), Module: metric.Module,
			ProfileVersion: metric.ProfileVersion, Status: "active", ActivatedByUserID: &actor, ActivatedAt: &activated,
		})
	}
	service, err := NewDecisionQualityService(repository, decisionQualityClock{value: now})
	if err != nil {
		t.Fatal(err)
	}
	result, persisted, err := service.Evaluate(t.Context(), dataset, 7, true)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.AllRequiredGatesPassed || len(persisted.Profiles) != 5 || !repository.command.Activate || repository.command.ActorUserID != 7 {
		t.Fatalf("quality service result = %#v command=%#v", persisted, repository.command)
	}
}

func TestDecisionQualityServiceRejectsActivationWhenAnyGateFailsBeforePersistence(t *testing.T) {
	dataset := passingDecisionQualityDataset(t)
	for index := 200; index < 205; index++ {
		dataset.DuplicateSamples[index].RightFixtureText = dataset.DuplicateSamples[index].LeftFixtureText
	}
	repository := &decisionQualityRepositoryFake{}
	service, err := NewDecisionQualityService(repository, decisionQualityClock{value: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Evaluate(t.Context(), dataset, 7, true); !errors.Is(err, ErrInvalidDecisionQualityProfileContract) {
		t.Fatalf("failed quality profile activation error = %v", err)
	}
	if repository.command.ActorUserID != 0 {
		t.Fatal("failed quality profile reached persistence")
	}
}

func TestDecisionQualityServiceRejectsChangedPersistenceReceipt(t *testing.T) {
	repository := &decisionQualityRepositoryFake{result: PersistDecisionQualityEvaluationResult{}}
	service, err := NewDecisionQualityService(repository, decisionQualityClock{value: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Evaluate(t.Context(), passingDecisionQualityDataset(t), 7, false); !errors.Is(err, ErrInvalidDecisionQualityProfileContract) {
		t.Fatalf("changed receipt error = %v", err)
	}
}
