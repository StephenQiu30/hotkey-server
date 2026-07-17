package queue

// P0 job kinds are deliberately finite. Each handler consumes only this
// package's bounded ID/version envelope and rereads business facts in the DB.
const (
	KindCollectSource        = "collect_source"
	KindNormalizeContent     = "normalize_content"
	KindEvaluateRelevance    = "evaluate_relevance"
	KindClusterContent       = "cluster_content"
	KindRecomputeEventHeat   = "recompute_event_heat"
	KindGenerateEventSummary = "generate_event_summary"
	KindBuildReport          = "build_report"
	KindDeliverEmail         = "deliver_email"
	KindProjectKnowledge     = "project_knowledge"
	KindReconcileKnowledge   = "reconcile_knowledge"
	KindRunRetention         = "run_retention"
)

func IsKnownKind(kind string) bool {
	switch kind {
	case KindCollectSource, KindNormalizeContent, KindEvaluateRelevance, KindClusterContent,
		KindRecomputeEventHeat, KindGenerateEventSummary, KindBuildReport, KindDeliverEmail,
		KindProjectKnowledge, KindReconcileKnowledge, KindRunRetention:
		return true
	default:
		return false
	}
}
