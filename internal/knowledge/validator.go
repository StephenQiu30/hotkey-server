package knowledge

import (
	"errors"
)

// ErrFieldNotAllowed is returned when a writeback field is not in the whitelist.
var ErrFieldNotAllowed = errors.New("writeback field not allowed")

// allowedWritebackFields defines the set of fields that can be written back
// from Obsidian to HotKey.
var allowedWritebackFields = map[string]bool{
	"manual_tags":        true,
	"analyst_conclusion": true,
	"theme_ref":          true,
	"material_status":    true,
}

// WritebackChange represents a single writeback change to be validated and applied.
type WritebackChange struct {
	ObjectType string
	ObjectID   int64
	FieldName  string
	Value      interface{}
	SourcePath string
}

// ValidateWriteback validates that a writeback change targets an allowed field.
func ValidateWriteback(change WritebackChange) error {
	if !allowedWritebackFields[change.FieldName] {
		return ErrFieldNotAllowed
	}
	return nil
}
