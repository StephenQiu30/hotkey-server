package knowledge

import "errors"

// ErrWritebackConflict is returned when the incoming revision does not match
// the current revision, indicating a writeback conflict.
var ErrWritebackConflict = errors.New("writeback revision conflict")

// ConflictInput carries revision information for writeback conflict detection.
type ConflictInput struct {
	CurrentRevision string
	IncomingRevision string
}

// DetectConflict checks whether the incoming revision matches the current revision.
// If both revisions are empty, no conflict is detected (backward compatibility).
// If both revisions are non-empty and differ, a conflict is returned.
func DetectConflict(in ConflictInput) error {
	// Empty revisions are allowed (backward compatibility with pre-revision data).
	if in.CurrentRevision == "" && in.IncomingRevision == "" {
		return nil
	}
	// If either side has no revision set, allow (data being upgraded).
	if in.CurrentRevision == "" || in.IncomingRevision == "" {
		return nil
	}
	if in.CurrentRevision != in.IncomingRevision {
		return ErrWritebackConflict
	}
	return nil
}
