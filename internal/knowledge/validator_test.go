package knowledge

import (
	"errors"
	"testing"
)

func TestValidateWriteback_AllowsWhitelistedField(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		value     interface{}
	}{
		{"manual_tags", "manual_tags", []string{"ai监管"}},
		{"analyst_conclusion", "analyst_conclusion", "持续关注监管升级"},
		{"theme_ref", "theme_ref", "热点专题-01"},
		{"material_status", "material_status", "draft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWriteback(WritebackChange{
				FieldName: tt.fieldName,
				Value:     tt.value,
			})
			if err != nil {
				t.Fatalf("expected nil for whitelisted field %q, got %v", tt.fieldName, err)
			}
		})
	}
}

func TestValidateWriteback_RejectsMachineField(t *testing.T) {
	err := ValidateWriteback(WritebackChange{
		FieldName: "heat",
		Value:     99.9,
	})
	if !errors.Is(err, ErrFieldNotAllowed) {
		t.Fatalf("expected ErrFieldNotAllowed, got %v", err)
	}
}
