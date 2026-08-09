package postgres

import ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"

type documentRecallHitRecord struct {
	DocumentVersionID int64
	Rank              int
	RawScore          float64
}

func documentRecallHitDTO(record documentRecallHitRecord) ingestionapplication.RecallHitDTO {
	return ingestionapplication.RecallHitDTO{
		DocumentVersionID: record.DocumentVersionID,
		Rank:              record.Rank, RawScore: record.RawScore,
	}
}

type documentRecallFilterRecord struct {
	PositiveTerms, MustTerms, MustNotTerms           []string
	MustLanguages, MustNotLanguages                  []string
	MustSources, MustNotSources                      []string
	MustActions, ShouldActions, MustNotActions       []string
	MustLocations, ShouldLocations, MustNotLocations []string
	MustRegions, ShouldRegions, MustNotRegions       []string
	EntityKeys                                       []string
	MustTimeStarts, MustTimeEnds                     []string
	MustNotTimeStarts, MustNotTimeEnds               []string
}
