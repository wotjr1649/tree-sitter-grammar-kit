package kit

import (
	"bytes"
	"encoding/json"
	"testing"
	"testing/fstest"
)

// oracleSet builds a valid one-case record set: a record, its raw response and the manifest.
func oracleSet(t *testing.T) (fstest.MapFS, OracleManifest) {
	t.Helper()
	in := digestHex([]byte("a = 1;"))
	rec := []byte(`{"schema":"tsgk-oracle-record/r1","case":"c1","workload":"w","id":"c1","input":{"path":"c1.txt","role":"case","sha256":"` + in +
		`","bytes":6},"execution_status":"COMPLETED","steps":[{"step":0,"source_bytes":6,"source_sha256":"` + in + `"}],"complete":true}` + "\n")
	raw := []byte(`{"protocol":"tsgk-native/r2"}`)
	m := OracleManifest{Schema: OracleManifestSchema, Workload: map[string]string{"profile_id": "w"}, Producer: map[string]string{}, Protocol: "tsgk-native/r2",
		Comparators: []string{}, Queries: []IdentityRef{}, ExecutionStatus: StatusCompleted, Assessment: AssessPass, Records: 1, Complete: true,
		Members: []OracleMember{
			{Path: "raw/00000-c1.json", Role: "raw", Case: "c1", Bytes: uint64(len(raw)), SHA256: digestHex(raw), InputBytes: 6, InputSHA256: in, ExecutionStatus: StatusCompleted},
			{Path: "records/00000-c1.json", Role: "record", Case: "c1", Bytes: uint64(len(rec)), SHA256: digestHex(rec), InputBytes: 6, InputSHA256: in, ExecutionStatus: StatusCompleted},
		}}
	fsys := fstest.MapFS{"raw/00000-c1.json": {Data: raw}, "records/00000-c1.json": {Data: rec}}
	fsys["manifest.json"] = &fstest.MapFile{Data: manifestBytes(t, m)}
	return fsys, m
}

func manifestBytes(t *testing.T, m OracleManifest) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// S06-A09: VerifyOracleSet accepts only a complete set and rejects a truncated member, a
// manifest without its final completeness member, a missing or unlisted member, a record
// count mismatch and a record that is not complete or not bound to its member's input.
func TestVerifyOracleSet(t *testing.T) {
	good, m := oracleSet(t)
	if r := VerifyOracleSet(good); !r.Valid || r.Records != 1 {
		t.Fatalf("valid set rejected: %+v", r)
	}
	clone := func() fstest.MapFS {
		c := fstest.MapFS{}
		for k, v := range good {
			c[k] = &fstest.MapFile{Data: bytes.Clone(v.Data)}
		}
		return c
	}
	// a record rewritten consistently into the manifest, so only the record check can fail
	rebind := func(rec []byte) fstest.MapFS {
		c := clone()
		c["records/00000-c1.json"] = &fstest.MapFile{Data: rec}
		mm := m
		mm.Members = append([]OracleMember(nil), m.Members...)
		mm.Members[1].Bytes, mm.Members[1].SHA256 = uint64(len(rec)), digestHex(rec)
		c["manifest.json"] = &fstest.MapFile{Data: manifestBytes(t, mm)}
		return c
	}
	recData := good["records/00000-c1.json"].Data
	for name, tc := range map[string]struct {
		fsys fstest.MapFS
		code string
	}{
		"truncated-record": {func() fstest.MapFS {
			c := clone()
			c["records/00000-c1.json"].Data = recData[:len(recData)/2]
			return c
		}(), "MEMBER_MISMATCH"},
		"manifest-without-footer": {func() fstest.MapFS {
			c := clone()
			d := c["manifest.json"].Data
			c["manifest.json"].Data = append(bytes.TrimSuffix(d, []byte(`,"complete":true}`)), '}')
			return c
		}(), "JSON_MISSING_FIELD"},
		"manifest-truncated": {func() fstest.MapFS {
			c := clone()
			c["manifest.json"].Data = c["manifest.json"].Data[:40]
			return c
		}(), ""},
		"manifest-missing": {func() fstest.MapFS { c := clone(); delete(c, "manifest.json"); return c }(), "MANIFEST_MISSING"},
		"member-missing":   {func() fstest.MapFS { c := clone(); delete(c, "raw/00000-c1.json"); return c }(), "MEMBER_MISSING"},
		"member-unlisted": {func() fstest.MapFS {
			c := clone()
			c["records/00001-extra.json"] = &fstest.MapFile{Data: []byte("{}")}
			return c
		}(), "MEMBER_UNLISTED"},
		"record-count": {func() fstest.MapFS {
			c := clone()
			mm := m
			mm.Records = 2
			c["manifest.json"].Data = manifestBytes(t, mm)
			return c
		}(), "RECORD_COUNT_MISMATCH"},
		"record-incomplete": {rebind(bytes.Replace(recData, []byte(`,"complete":true}`), []byte(`}`), 1)), "RECORD_INCOMPLETE"},
		"record-input":      {rebind(bytes.Replace(recData, []byte(`"bytes":6`), []byte(`"bytes":7`), 1)), "RECORD_INPUT_MISMATCH"},
		"record-step":       {rebind(bytes.Replace(recData, []byte(`"source_bytes":6`), []byte(`"source_bytes":5`), 1)), "RECORD_STEP_MISMATCH"},
		"record-case":       {rebind(bytes.Replace(recData, []byte(`"case":"c1"`), []byte(`"case":"c2"`), 1)), "RECORD_IDENTITY_MISMATCH"},
	} {
		t.Run(name, func(t *testing.T) {
			r := VerifyOracleSet(tc.fsys)
			if r.Valid || len(r.Findings) == 0 || (tc.code != "" && r.Findings[0].Code != tc.code) {
				t.Fatalf("%+v", r)
			}
		})
	}
}

// CompareCaptures is the single ordered capture comparator: a swapped tie, a dropped
// duplicate or a changed node index is a difference; equal streams are nil.
func TestCompareCaptures(t *testing.T) {
	a := []Capture{{Match: 0, Name: "x", Node: 1, Type: "id", StartByte: 0, EndByte: 1}, {Match: 1, Name: "y", Node: 1, Type: "id", StartByte: 0, EndByte: 1}}
	if d := CompareCaptures(a, append([]Capture(nil), a...)); d != nil {
		t.Fatalf("equal streams differ: %+v", d)
	}
	if d := CompareCaptures(a, []Capture{a[1], a[0]}); d == nil || d.Index != 0 || d.Field != "match" {
		t.Fatalf("swapped tie not detected: %+v", d)
	}
	if d := CompareCaptures(a, a[:1]); d == nil || d.Field != "capture_count" {
		t.Fatalf("dropped duplicate not detected: %+v", d)
	}
	b := append([]Capture(nil), a...)
	b[1].Node = 2
	if d := CompareCaptures(a, b); d == nil || d.Field != "node" {
		t.Fatalf("node index change not detected: %+v", d)
	}
}
