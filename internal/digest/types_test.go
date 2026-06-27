package digest

import "testing"

func TestExportStatusIsValid(t *testing.T) {
	tests := []struct {
		status ExportStatus
		want   bool
	}{
		{StatusPending, true},
		{StatusPublished, true},
		{StatusFailed, true},
		{ExportStatus("archived"), false},
		{ExportStatus(""), false},
	}

	for _, tt := range tests {
		if got := tt.status.IsValid(); got != tt.want {
			t.Fatalf("ExportStatus(%q).IsValid() = %v, want %v", tt.status, got, tt.want)
		}
	}
}
