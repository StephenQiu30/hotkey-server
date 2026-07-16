package postgresfixture

import "testing"

func TestDatabaseNameSeparatesConcurrentTestProcesses(t *testing.T) {
	first := databaseName(101, 123456789, 1)
	second := databaseName(102, 123456789, 1)
	if first == second {
		t.Fatalf("database names collide across processes: %q", first)
	}
}
