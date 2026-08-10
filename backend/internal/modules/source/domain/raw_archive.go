package domain

import (
	"errors"
	"fmt"
	"strings"
)

const MaxEvidenceReferences = 8

const (
	WholePayloadSelectorVersion = "whole-payload-sha256-v1"
	ByteRangeSelectorVersion    = "byte-range-sha256-v1"
)

var (
	ErrRawArchiveNotAuthorized = errors.New("raw archive is not authorized")
	ErrRawEvidenceConflict     = errors.New("raw evidence identity conflict")
	ErrEvidenceSelection       = errors.New("evidence selection could not be verified")
)

type EvidenceLocatorType string

const (
	EvidenceLocatorJSONPointer  EvidenceLocatorType = "json_pointer"
	EvidenceLocatorXMLPath      EvidenceLocatorType = "xml_path"
	EvidenceLocatorByteRange    EvidenceLocatorType = "byte_range"
	EvidenceLocatorWholePayload EvidenceLocatorType = "whole_payload"
)

func (locator EvidenceLocatorType) Valid() bool {
	return locator == EvidenceLocatorJSONPointer || locator == EvidenceLocatorXMLPath || locator == EvidenceLocatorByteRange || locator == EvidenceLocatorWholePayload
}

// EvidenceUsage states whether a locator is eligible to create a document or
// only preserves provider context such as a ranked-list membership response.
// Context evidence remains fully auditable but must never enqueue a body
// extraction job.
type EvidenceUsage string

const (
	EvidenceUsageDocumentSource EvidenceUsage = "document_source"
	EvidenceUsageContext        EvidenceUsage = "context"
)

func (usage EvidenceUsage) Valid() bool {
	return usage == EvidenceUsageDocumentSource || usage == EvidenceUsageContext
}

// EvidenceReference is an immutable locator from a normalized SourceItem to a
// selected portion of an EvidenceSnapshot.
type EvidenceReference struct {
	SnapshotKey           string
	Usage                 EvidenceUsage
	LocatorType           EvidenceLocatorType
	LocatorValue          string
	ByteStart             *int64
	ByteEnd               *int64
	SelectedPayloadSHA256 string
	SelectorVersion       string
}

func (reference EvidenceReference) Validate() error {
	if !validSHA256(reference.SnapshotKey) || !reference.Usage.Valid() || !reference.LocatorType.Valid() || !validSHA256(reference.SelectedPayloadSHA256) {
		return fmt.Errorf("evidence reference identity is invalid")
	}
	if reference.LocatorValue == "" || reference.LocatorValue != strings.TrimSpace(reference.LocatorValue) || len(reference.LocatorValue) > 2048 || strings.ContainsAny(reference.LocatorValue, "\x00\r\n") {
		return fmt.Errorf("evidence locator is invalid")
	}
	if reference.SelectorVersion == "" || reference.SelectorVersion != strings.TrimSpace(reference.SelectorVersion) || len(reference.SelectorVersion) > 128 || strings.ContainsAny(reference.SelectorVersion, "\x00\r\n") {
		return fmt.Errorf("evidence selector version is invalid")
	}
	if reference.LocatorType == EvidenceLocatorByteRange {
		if reference.ByteStart == nil || reference.ByteEnd == nil || *reference.ByteStart < 0 || *reference.ByteEnd <= *reference.ByteStart {
			return fmt.Errorf("evidence byte range is invalid")
		}
	} else if reference.ByteStart != nil || reference.ByteEnd != nil {
		return fmt.Errorf("evidence byte range is invalid")
	}
	return nil
}

// EvidenceLifecycleState is the lifecycle value persisted for raw evidence.
type EvidenceLifecycleState string

const (
	EvidenceLifecyclePending          EvidenceLifecycleState = "raw_pending"
	EvidenceLifecycleAvailable        EvidenceLifecycleState = "raw_available"
	EvidenceLifecycleFailed           EvidenceLifecycleState = "raw_failed"
	EvidenceLifecyclePolicyBlocked    EvidenceLifecycleState = "policy_blocked"
	EvidenceLifecycleRetentionBlocked EvidenceLifecycleState = "retention_blocked"
	EvidenceLifecycleQuarantined      EvidenceLifecycleState = "quarantined"
	EvidenceLifecycleTombstoned       EvidenceLifecycleState = "tombstoned"
)

func (state EvidenceLifecycleState) Valid() bool {
	switch state {
	case EvidenceLifecyclePending, EvidenceLifecycleAvailable, EvidenceLifecycleFailed,
		EvidenceLifecyclePolicyBlocked, EvidenceLifecycleRetentionBlocked,
		EvidenceLifecycleQuarantined, EvidenceLifecycleTombstoned:
		return true
	default:
		return false
	}
}
