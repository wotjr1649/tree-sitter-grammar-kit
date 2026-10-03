package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// S04-A11: generator limits accept the adopted ceiling and reject one over; only the
// postgresql-sql route has the 6 GiB memory exception. No value is doubled or defaulted.
func TestReproduceLimitCeilings(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "contracts", "examples", "reproduce-r1.json"))
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(route, key string, v uint64) *Error {
		var doc map[string]any
		json.Unmarshal(data, &doc)
		doc["route"] = route
		doc["limits"].(map[string]any)[key] = v
		b, _ := json.Marshal(doc)
		_, e := parseReproduce(b)
		return e
	}
	ceilings := map[string]uint64{"wall_seconds": GenWallSeconds, "output_bytes": GenOutputBytes, "storage_bytes": GenStorageBytes,
		"memory_bytes": GenMemoryBytes, "input_files": GenInputFiles, "input_bytes": GenInputBytes, "file_bytes": GenFileBytes}
	for key, ceil := range ceilings {
		if e := mutate("swift", key, ceil); e != nil {
			t.Fatalf("%s at ceiling rejected: %v", key, e)
		}
		if e := mutate("swift", key, ceil+1); e == nil || e.Code != "PROFILE_LIMIT_INVALID" {
			t.Fatalf("%s one over accepted: %v", key, e)
		}
		if e := mutate("swift", key, 0); e == nil || e.Code != "PROFILE_LIMIT_INVALID" {
			t.Fatalf("%s zero accepted: %v", key, e)
		}
	}
	if e := mutate("postgresql-sql", "memory_bytes", GenPGMemoryBytes); e != nil {
		t.Fatalf("PG exception rejected: %v", e)
	}
	if e := mutate("postgresql-sql", "memory_bytes", GenPGMemoryBytes+1); e == nil {
		t.Fatal("PG one over accepted")
	}
	if e := mutate("tsql", "memory_bytes", GenPGMemoryBytes); e == nil {
		t.Fatal("6 GiB accepted outside postgresql-sql")
	}
}
