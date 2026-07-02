package knowledge

import (
	"errors"
	"testing"
)

func TestValidateWriteback_RejectsMachineField(t *testing.T) {
	err := ValidateWriteback(WritebackChange{
		FieldName: "heat",
		Value:     99.9,
	})
	if !errors.Is(err, ErrFieldNotAllowed) {
		t.Fatalf("expected ErrFieldNotAllowed, got %v", err)
	}
}
