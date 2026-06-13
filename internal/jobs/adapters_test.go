package jobs

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/scoring"
)

// TestDBRunRepositoryImplementsRunRepository verifies DBRunRepository satisfies
// the RunRepository interface.
func TestDBRunRepositoryImplementsRunRepository(t *testing.T) {
	var _ RunRepository = (*DBRunRepository)(nil)
}

// TestDBPostRepositoryImplementsPostRepository verifies DBPostRepository satisfies
// the PostRepository interface.
func TestDBPostRepositoryImplementsPostRepository(t *testing.T) {
	var _ PostRepository = (*DBPostRepository)(nil)
}

// TestDBHitRepositoryImplementsHitRepository verifies DBHitRepository satisfies
// the HitRepository interface.
func TestDBHitRepositoryImplementsHitRepository(t *testing.T) {
	var _ HitRepository = (*DBHitRepository)(nil)
}

// TestDBHitScorerRepoImplementsScoringHitRepository verifies DBHitScorerRepo
// satisfies the scoring.HitRepository interface.
func TestDBHitScorerRepoImplementsScoringHitRepository(t *testing.T) {
	var _ scoring.HitRepository = (*DBHitScorerRepo)(nil)
}

// TestDBDeliveryRepositoryImplementsDeliveryRepository verifies
// DBDeliveryRepository satisfies the DeliveryRepository interface.
func TestDBDeliveryRepositoryImplementsDeliveryRepository(t *testing.T) {
	var _ DeliveryRepository = (*DBDeliveryRepository)(nil)
}

// TestDBUserEmailLookupImplementsUserEmailLookup verifies DBUserEmailLookup
// satisfies the UserEmailLookup interface.
func TestDBUserEmailLookupImplementsUserEmailLookup(t *testing.T) {
	var _ UserEmailLookup = (*DBUserEmailLookup)(nil)
}

// TestDBPostCandidateProviderImplementsPostCandidateProvider verifies
// DBPostCandidateProvider satisfies the PostCandidateProvider interface.
func TestDBPostCandidateProviderImplementsPostCandidateProvider(t *testing.T) {
	var _ PostCandidateProvider = (*DBPostCandidateProvider)(nil)
}

// TestDBTopicProviderImplementsTopicProvider verifies DBTopicProvider
// satisfies the TopicProvider interface.
func TestDBTopicProviderImplementsTopicProvider(t *testing.T) {
	var _ TopicProvider = (*DBTopicProvider)(nil)
}

// TestXConnectorAdapterImplementsPlatformConnector verifies XConnectorAdapter
// satisfies the PlatformConnector interface.
func TestXConnectorAdapterImplementsPlatformConnector(t *testing.T) {
	var _ PlatformConnector = (*XConnectorAdapter)(nil)
}

// TestScorerAdapterImplementsHitScorer verifies ScorerAdapter
// satisfies the HitScorer interface.
func TestScorerAdapterImplementsHitScorer(t *testing.T) {
	var _ HitScorer = (*ScorerAdapter)(nil)
}

// TestTopicPersisterAdapterImplementsTopicPersister verifies TopicPersisterAdapter
// satisfies the TopicPersister interface.
func TestTopicPersisterAdapterImplementsTopicPersister(t *testing.T) {
	var _ TopicPersister = (*TopicPersisterAdapter)(nil)
}

// TestDBMonitorListerImplementsMonitorLister verifies DBMonitorLister
// satisfies the MonitorLister interface.
func TestDBMonitorListerImplementsMonitorLister(t *testing.T) {
	var _ MonitorLister = (*DBMonitorLister)(nil)
}
