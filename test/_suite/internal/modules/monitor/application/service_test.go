package application

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

func TestQuerySignatureIsCanonicalAndExcludesPendingAI(t *testing.T) {
	connection := sourcedomain.MonitorSourceConnection{ID: 9, SourceType: sourcedomain.SourceTypeRSS, Endpoint: "https://feeds.example.test/rss", Enabled: true, Config: sourcedomain.DefaultSourceConfig()}
	config := domain.MonitorConfig{Timezone: "UTC", Languages: []string{"en"}, Regions: []string{"US"}, CollectionIntervalSeconds: 300, RelevanceThreshold: 60, EventThreshold: 0, RetentionDays: 30}
	source := domain.MonitorSource{ID: 1, SourceConnectionID: connection.ID, Priority: 1, Enabled: true}
	approved := domain.MonitorRule{ID: 3, RuleType: domain.RuleTypeKeyword, Operator: domain.RuleOperatorContains, Value: "AI", Weight: 100, Priority: 1, Origin: domain.RuleOriginUser, ApprovalStatus: domain.RuleApprovalApproved, Enabled: true}
	pending := domain.MonitorRule{ID: 4, RuleType: domain.RuleTypeKeyword, Operator: domain.RuleOperatorContains, Value: "unreviewed", Weight: 10, Priority: 2, Origin: domain.RuleOriginAI, ApprovalStatus: domain.RuleApprovalPending, Enabled: true}
	first, err := querySignature(source, connection, config, []domain.MonitorRule{pending, approved})
	if err != nil {
		t.Fatalf("querySignature(first): %v", err)
	}
	second, err := querySignature(source, connection, config, []domain.MonitorRule{approved, pending})
	if err != nil {
		t.Fatalf("querySignature(second): %v", err)
	}
	if first != second {
		t.Fatalf("rule ordering changed signature: first=%s second=%s", first, second)
	}
	differentID := approved
	differentID.ID = 999
	withDifferentID, err := querySignature(source, connection, config, []domain.MonitorRule{differentID})
	if err != nil {
		t.Fatalf("querySignature(different ID): %v", err)
	}
	withoutPending, err := querySignature(source, connection, config, []domain.MonitorRule{approved})
	if err != nil {
		t.Fatalf("querySignature(without pending): %v", err)
	}
	if withDifferentID != withoutPending {
		t.Fatalf("rule ID changed signature: original=%s different=%s", withoutPending, withDifferentID)
	}
	exclude := domain.MonitorRule{ID: 11, RuleType: domain.RuleTypeExcludeKeyword, Operator: domain.RuleOperatorContains, Value: "noise", Weight: 0, Priority: 1, Origin: domain.RuleOriginUser, ApprovalStatus: domain.RuleApprovalApproved, Enabled: true}
	excludeChanged := exclude
	excludeChanged.ID, excludeChanged.Weight, excludeChanged.Priority = 12, 42, 99
	withExclude, err := querySignature(source, connection, config, []domain.MonitorRule{approved, exclude})
	if err != nil {
		t.Fatalf("querySignature(exclude): %v", err)
	}
	withChangedExclude, err := querySignature(source, connection, config, []domain.MonitorRule{approved, excludeChanged})
	if err != nil {
		t.Fatalf("querySignature(changed exclude): %v", err)
	}
	if withExclude != withChangedExclude {
		t.Fatalf("exclude ID/weight/priority changed signature: first=%s second=%s", withExclude, withChangedExclude)
	}
	approvedPending := pending
	approvedPending.ApprovalStatus = domain.RuleApprovalApproved
	withApprovedAI, err := querySignature(source, connection, config, []domain.MonitorRule{approved, approvedPending})
	if err != nil {
		t.Fatalf("querySignature(approved AI): %v", err)
	}
	if withApprovedAI == first {
		t.Fatal("approved AI did not change query signature")
	}
	changedOverride := source
	changedOverride.QueryOverride = "specific query"
	withOverride, err := querySignature(changedOverride, connection, config, []domain.MonitorRule{approved})
	if err != nil {
		t.Fatalf("querySignature(override): %v", err)
	}
	withoutOverride, err := querySignature(source, connection, config, []domain.MonitorRule{approved})
	if err != nil {
		t.Fatalf("querySignature(no override): %v", err)
	}
	if withOverride == withoutOverride {
		t.Fatal("query override did not change query signature")
	}
}
