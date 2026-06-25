package jobs_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/scoring"
)

func TestRunRepoImplementsRunRepository(t *testing.T) {
	var _ jobs.RunRepository = (*database.RunRepo)(nil)
}

func TestPollPostRepoImplementsPostRepository(t *testing.T) {
	var _ jobs.PostRepository = (*database.PollPostRepo)(nil)
}

func TestPollHitRepoImplementsHitRepository(t *testing.T) {
	var _ jobs.HitRepository = (*database.PollHitRepo)(nil)
}

func TestHitScoreRepoImplementsScoringHitRepository(t *testing.T) {
	var _ scoring.HitRepository = (*database.HitScoreRepo)(nil)
}

func TestDeliveryRepoImplementsDeliveryRepository(t *testing.T) {
	var _ jobs.DeliveryRepository = (*database.DeliveryRepo)(nil)
}

func TestDeliveryRepoImplementsUserEmailLookup(t *testing.T) {
	var _ jobs.UserEmailLookup = (*database.DeliveryRepo)(nil)
}

func TestJobQueryRepoImplementsPostCandidateProvider(t *testing.T) {
	var _ jobs.PostCandidateProvider = (*database.JobQueryRepo)(nil)
}

func TestJobQueryRepoImplementsTopicProvider(t *testing.T) {
	var _ jobs.TopicProvider = (*database.JobQueryRepo)(nil)
}

func TestXConnectorAdapterImplementsPlatformConnector(t *testing.T) {
	var _ jobs.PlatformConnector = (*jobs.XConnectorAdapter)(nil)
}

func TestScorerAdapterImplementsHitScorer(t *testing.T) {
	var _ jobs.HitScorer = (*jobs.ScorerAdapter)(nil)
}

func TestTopicRepoImplementsTopicPersister(t *testing.T) {
	var _ jobs.TopicPersister = (*database.TopicRepo)(nil)
}

func TestMonitorRepoImplementsMonitorLister(t *testing.T) {
	var _ jobs.MonitorLister = (*database.MonitorRepo)(nil)
}
