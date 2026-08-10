package postgres

import (
	"encoding/json"
	"fmt"
	"strings"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
)

type storylineRecord struct {
	id, version                          int64
	storylineKey, title, summary, status string
	relationProfileVersion               string
}

func (record storylineRecord) dto() eventapplication.StorylineDTO {
	return eventapplication.StorylineDTO{ID: record.id, Version: record.version,
		StorylineKey: strings.TrimSpace(record.storylineKey), Title: record.title, Summary: record.summary,
		Status: record.status, RelationProfileVersion: record.relationProfileVersion}
}

type storylineEventRecord struct {
	id, storylineID, storylineVersion, microEventID, microEventVersion int64
	relationType, relationProfileVersion, decisionOrigin               string
	relationScore                                                      float64
	reasonCodesJSON                                                    []byte
	commandFingerprint                                                 string
}

func (record storylineEventRecord) dto() (eventapplication.StorylineEventDTO, error) {
	reasons := []string{}
	if err := json.Unmarshal(record.reasonCodesJSON, &reasons); err != nil || len(reasons) == 0 {
		return eventapplication.StorylineEventDTO{}, fmt.Errorf("invalid storyline reason codes")
	}
	return eventapplication.StorylineEventDTO{ID: record.id, StorylineID: record.storylineID,
		StorylineVersion: record.storylineVersion, MicroEventID: record.microEventID,
		MicroEventVersion: record.microEventVersion, RelationType: record.relationType,
		RelationScore: record.relationScore, RelationProfileVersion: record.relationProfileVersion,
		ReasonCodes: reasons, DecisionOrigin: record.decisionOrigin}, nil
}
