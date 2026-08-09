package postgres_test

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
)

type rightsFixtureQueryRower interface {
	QueryRow(string, ...any) *sql.Row
}

func insertRightsFixtureActor(t testingT, runtime rightsFixtureQueryRower, seed string) int64 {
	t.Helper()
	digest := rightsFixtureDigest("actor", seed)
	var actorID int64
	if err := runtime.QueryRow(`
INSERT INTO users (email,password_hash,display_name,role)
VALUES ($1,'fixture-not-a-credential','Rights fixture operator','admin')
RETURNING id`, "rights-fixture-"+digest[:24]+"@example.test").Scan(&actorID); err != nil {
		t.Fatalf("insert rights fixture actor: %v", err)
	}
	return actorID
}

func rightsFixtureReceipt(kind string, values ...any) (string, string) {
	fingerprint := rightsFixtureDigest(kind, fmt.Sprint(values...))
	return "fixture." + kind + "." + fingerprint[:32], fingerprint
}

func rightsFixtureDigest(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%x", digest[:])
}

// testingT is the narrow test dependency needed by shared fixture helpers.
type testingT interface {
	Helper()
	Fatalf(string, ...any)
}
