package postgres

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
)

type microEventRecord struct {
	id, version                                             int64
	eventKey, status, subjectKey, actionKey, profileVersion string
	locationKeys, identifierKeys                            []string
	eventStartedAt                                          time.Time
}

func (record microEventRecord) dto() eventapplication.MicroEventDTO {
	return eventapplication.MicroEventDTO{ID: record.id, Version: record.version, EventKey: strings.TrimSpace(record.eventKey),
		Status: record.status, PrimarySubjectKey: record.subjectKey, PrimaryActionKey: record.actionKey,
		LocationKeys: append([]string(nil), record.locationKeys...), IdentifierKeys: append([]string(nil), record.identifierKeys...),
		EventStartedAt: record.eventStartedAt.UTC(), ClusteringProfileVersion: record.profileVersion}
}

type microEventDecisionRecord struct {
	id, contentFamilyID, documentMatchDecisionID, microEventID, eventVersion int64
	action, profileVersion, commandFingerprint                               string
	sameEventScore, leadingMargin                                            float64
	features                                                                 eventapplication.MicroEventFeaturesDTO
	reasonCodesJSON                                                          []byte
}

type microEventStringArrayScan struct{ destination *[]string }

func (scan microEventStringArrayScan) Scan(value any) error {
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return fmt.Errorf("scan micro-event string array: %T", value)
	}
	if err := json.Unmarshal(raw, scan.destination); err != nil {
		return fmt.Errorf("scan micro-event string array: %w", err)
	}
	return nil
}

func (record microEventDecisionRecord) dto() (eventapplication.MicroEventMembershipDecisionDTO, error) {
	reasons := []string{}
	if err := json.Unmarshal(record.reasonCodesJSON, &reasons); err != nil || len(reasons) == 0 {
		return eventapplication.MicroEventMembershipDecisionDTO{}, fmt.Errorf("invalid micro-event reason codes")
	}
	return eventapplication.MicroEventMembershipDecisionDTO{ID: record.id, ContentFamilyID: record.contentFamilyID,
		DocumentMatchDecisionID: record.documentMatchDecisionID, MicroEventID: record.microEventID,
		EventVersion: record.eventVersion, Action: record.action, SameEventScore: record.sameEventScore,
		LeadingMargin: record.leadingMargin, Features: record.features, ClusteringProfileVersion: record.profileVersion,
		ReasonCodes: reasons}, nil
}
