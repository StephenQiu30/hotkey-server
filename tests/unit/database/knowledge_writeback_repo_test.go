package database_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/tests/testutil"
)

func TestKnowledgeWritebackRepo_RecordAttempt(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := database.NewKnowledgeWritebackRepo(db)

	err := repo.RecordAttempt(context.Background(), database.RecordAttemptInput{
		ObjectType: "theme",
		ObjectID:   201,
		FieldName:  "manual_tags",
		Status:     "validated",
	})
	if err != nil {
		t.Fatalf("record attempt: %v", err)
	}
}
