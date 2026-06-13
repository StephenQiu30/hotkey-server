package jobs_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/scoring"
)

// TestDBRunRepositoryImplementsRunRepository verifies DBRunRepository satisfies
// the RunRepository interface.
func TestDBRunRepositoryImplementsRunRepository(t *testing.T) {
	var _ jobs.RunRepository = (*jobs.DBRunRepository)(nil)
}

// TestDBPostRepositoryImplementsPostRepository verifies DBPostRepository satisfies
// the PostRepository interface.
func TestDBPostRepositoryImplementsPostRepository(t *testing.T) {
	var _ jobs.PostRepository = (*jobs.DBPostRepository)(nil)
}

// TestDBHitRepositoryImplementsHitRepository verifies DBHitRepository satisfies
// the HitRepository interface.
func TestDBHitRepositoryImplementsHitRepository(t *testing.T) {
	var _ jobs.HitRepository = (*jobs.DBHitRepository)(nil)
}

// TestDBHitScorerRepoImplementsScoringHitRepository verifies DBHitScorerRepo
// satisfies the scoring.HitRepository interface.
func TestDBHitScorerRepoImplementsScoringHitRepository(t *testing.T) {
	var _ scoring.HitRepository = (*jobs.DBHitScorerRepo)(nil)
}

// TestDBDeliveryRepositoryImplementsDeliveryRepository verifies
// DBDeliveryRepository satisfies the DeliveryRepository interface.
func TestDBDeliveryRepositoryImplementsDeliveryRepository(t *testing.T) {
	var _ jobs.DeliveryRepository = (*jobs.DBDeliveryRepository)(nil)
}

// TestDBUserEmailLookupImplementsUserEmailLookup verifies DBUserEmailLookup
// satisfies the UserEmailLookup interface.
func TestDBUserEmailLookupImplementsUserEmailLookup(t *testing.T) {
	var _ jobs.UserEmailLookup = (*jobs.DBUserEmailLookup)(nil)
}

// TestDBPostCandidateProviderImplementsPostCandidateProvider verifies
// DBPostCandidateProvider satisfies the PostCandidateProvider interface.
func TestDBPostCandidateProviderImplementsPostCandidateProvider(t *testing.T) {
	var _ jobs.PostCandidateProvider = (*jobs.DBPostCandidateProvider)(nil)
}

// TestDBTopicProviderImplementsTopicProvider verifies DBTopicProvider
// satisfies the TopicProvider interface.
func TestDBTopicProviderImplementsTopicProvider(t *testing.T) {
	var _ jobs.TopicProvider = (*jobs.DBTopicProvider)(nil)
}

// TestXConnectorAdapterImplementsPlatformConnector verifies XConnectorAdapter
// satisfies the PlatformConnector interface.
func TestXConnectorAdapterImplementsPlatformConnector(t *testing.T) {
	var _ jobs.PlatformConnector = (*jobs.XConnectorAdapter)(nil)
}

// TestScorerAdapterImplementsHitScorer verifies ScorerAdapter
// satisfies the HitScorer interface.
func TestScorerAdapterImplementsHitScorer(t *testing.T) {
	var _ jobs.HitScorer = (*jobs.ScorerAdapter)(nil)
}

// TestTopicPersisterAdapterImplementsTopicPersister verifies TopicPersisterAdapter
// satisfies the TopicPersister interface.
func TestTopicPersisterAdapterImplementsTopicPersister(t *testing.T) {
	var _ jobs.TopicPersister = (*jobs.TopicPersisterAdapter)(nil)
}

// TestDBMonitorListerImplementsMonitorLister verifies DBMonitorLister
// satisfies the MonitorLister interface.
func TestDBMonitorListerImplementsMonitorLister(t *testing.T) {
	var _ jobs.MonitorLister = (*jobs.DBMonitorLister)(nil)
}
