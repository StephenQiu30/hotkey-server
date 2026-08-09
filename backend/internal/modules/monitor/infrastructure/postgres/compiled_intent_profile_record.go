package postgres

import monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"

type compiledIntentProfileRecord struct {
	ID                                int64
	MonitorID                         int64
	ConfigVersionID                   int64
	PreviewRunID                      int64
	DraftID                           int64
	DraftResourceVersion              int64
	IntentRevisionID                  int64
	CompilerVersion                   string
	MatchingAlgorithmVersion          string
	LexicalAlgorithmVersion           string
	SemanticAlgorithmVersion          string
	StructuredAlgorithmVersion        string
	SearchNormalizationProfileVersion string
	SemanticState                     string
	SemanticUnavailableReason         string
	Status                            string
	ProfileHash                       string
}

type compiledIntentClauseRecord struct {
	Operator        string
	Field           string
	Value           string
	NormalizedValue string
	Origin          string
}

type compiledIntentEntityRecord struct {
	ID                int64
	CanonicalID       string
	Aliases           []string
	NormalizedAliases []string
}

func compiledIntentClauseDTO(record compiledIntentClauseRecord) monitorapplication.CompiledIntentClauseDTO {
	return monitorapplication.CompiledIntentClauseDTO{
		Operator: record.Operator, Field: record.Field, Value: record.Value,
		NormalizedValue: record.NormalizedValue, Origin: record.Origin,
	}
}

func compiledIntentEntityDTO(record compiledIntentEntityRecord) monitorapplication.CompiledIntentEntityDTO {
	return monitorapplication.CompiledIntentEntityDTO{
		CanonicalID: record.CanonicalID, Aliases: append([]string(nil), record.Aliases...),
		NormalizedAliases: append([]string(nil), record.NormalizedAliases...),
	}
}
