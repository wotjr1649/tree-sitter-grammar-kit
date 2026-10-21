package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// oracleProfile writes the plain-grammar oracle profile for the given case sources into a
// new case root and returns the profile bytes and the root.
func oracleProfile(t *testing.T, queries []kit.OracleQuery, sources ...string) ([]byte, string) {
	t.Helper()
	_, cc := nativeTools(t)
	root := t.TempDir()
	sum, n := digestOf(t, cc)
	var cases []map[string]any
	for i, src := range sources {
		name := "case" + string(rune('a'+i)) + ".txt"
		if err := os.WriteFile(filepath.Join(root, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		s, b := digestOf(t, filepath.Join(root, name))
		cases = append(cases, map[string]any{"id": "c" + string(rune('a'+i)), "input": map[string]any{"path": name, "role": "case", "sha256": s, "bytes": b},
			"edits": []any{}, "points": []any{}, "expect": []any{}, "query_expect": []any{}, "dynamic_sql_expect": nil})
	}
	if queries == nil {
		queries = []kit.OracleQuery{}
	}
	p := map[string]any{"schema": kit.OracleSchema, "id": "owned-oracle", "route": "owned", "operation": "native-query", "symbol": "tree_sitter_tsgk_plain",
		"encoding": "UTF-8", "output": "tree", "compiler": map[string]any{"name": "cc", "version": "test", "sha256": sum, "bytes": n},
		"grammar": fixtureGrammar(t, "plain"), "declarations": nil, "queries": queries, "fact_pack": nil, "api": true, "cases": cases}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return data, root
}

func oracleRequest(t *testing.T, profile []byte, root, out string) OracleRequest {
	rt, cc := nativeTools(t)
	work := t.TempDir()
	return OracleRequest{Root: root, GrammarRoot: fixtureRoot(t, "plain"), Profile: profile, Runtime: rt, Compiler: cc, Work: work, Out: out,
		Allow: []string{AllowBuild, AllowExec}, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")}
}

func runOracle(t *testing.T, req OracleRequest) (OracleResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return Oracle(ctx, req)
}

// snapshot reads every file below dir.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			out[p] = string(b)
		}
		return nil
	})
	return out
}

// #217: an observed record set is r4 with its run id, and every case carries its claim: a
// parsed case PASS, one whose input is gone before the run (refused before any parse)
// NOT_CLAIMED, never empty.
func TestOracleRecordObservedSet(t *testing.T) {
	t.Parallel()
	profile, root := oracleProfile(t, nil, "a = f(1);\n", "b = 2;\n")
	if err := os.Remove(filepath.Join(root, "caseb.txt")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "set")
	req := oracleRequest(t, profile, root, out)
	req.ReuseObservation = true
	res, err := runOracle(t, req)
	if err != nil || res.Set == nil || !res.Set.Valid || len(res.Cases) != 2 || res.ObservationRunID == "" {
		t.Fatalf("%v %s %+v %+v", err, res.ExecutionStatus, res.Set, res.Findings)
	}
	if got := [2]string{res.Cases[0].ReuseObservationClaim, res.Cases[1].ReuseObservationClaim}; got != [2]string{ClaimPass, ClaimNotClaimed} || res.Cases[1].ExecutionStatus != kit.StatusNotRun {
		t.Fatalf("claims %v %+v", got, res.Cases)
	}
	data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	var manifest struct {
		Protocol string            `json:"protocol"`
		Producer map[string]string `json:"producer"`
	}
	if err != nil || json.Unmarshal(data, &manifest) != nil || manifest.Protocol != ProtocolR4 || manifest.Producer["observation_run_id"] != res.ObservationRunID {
		t.Fatalf("manifest %v %+v", err, manifest)
	}
}

// Query-free API records still publish an empty identity array in the manifest.
func TestOracleRecordSetWithoutQueries(t *testing.T) {
	t.Parallel() // Independent build and output, like TestOracleRecordSet.
	profile, root := oracleProfile(t, nil, "a = f(1);\n")
	out := filepath.Join(t.TempDir(), "set")
	res, err := runOracle(t, oracleRequest(t, profile, root, out))
	if err != nil || res.ExecutionStatus != kit.StatusCompleted || res.Set == nil || !res.Set.Valid || res.Set.Records != 1 {
		t.Fatalf("%v %s %+v %+v", err, res.ExecutionStatus, res.Set, res.Findings)
	}
	if len(res.Cases) != 1 || res.Cases[0].Oracle == nil || res.Cases[0].Oracle.API != ClaimPass || res.Cases[0].Oracle.QueryEquality != ClaimNotClaimed {
		t.Fatalf("query-free API claims: %+v", res.Cases)
	}
	data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	var manifest map[string]json.RawMessage
	if err != nil || json.Unmarshal(data, &manifest) != nil || string(manifest["queries"]) != "[]" {
		t.Fatalf("empty query identities: %v %s", err, manifest["queries"])
	}
	if string(manifest["schema"]) != `"`+kit.OracleManifestSchema+`"` {
		t.Fatalf("default manifest revision changed: %s", manifest["schema"])
	}
	if _, err := os.Stat(filepath.Join(out, "source")); !os.IsNotExist(err) {
		t.Fatalf("default retained source: %v", err)
	}
}

func TestOracleSourceWitness(t *testing.T) {
	for _, mode := range []string{"exact-buffer", "missing-source", "write-failure", "build-failure"} {
		t.Run(mode, func(t *testing.T) {
			profile, root := oracleProfile(t, nil, "a = f(1);\n", "")
			out := filepath.Join(t.TempDir(), "set")
			req := oracleRequest(t, profile, root, out)
			req.SourceWitness = true
			if mode == "build-failure" {
				var p map[string]any
				if err := json.Unmarshal(req.Profile, &p); err != nil {
					t.Fatal(err)
				}
				p["compiler"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
				req.Profile, _ = json.Marshal(p)
			}
			if mode == "missing-source" {
				if err := os.Remove(filepath.Join(root, "casea.txt")); err != nil {
					t.Fatal(err)
				}
			}
			original := createExclusive
			defer func() { createExclusive = original }()
			createExclusive = func(dir *os.Root, path string, data []byte) error {
				if strings.HasSuffix(path, ".bin") {
					if mode == "write-failure" {
						return errors.New("injected source write failure")
					}
					if mode == "exact-buffer" {
						if err := os.WriteFile(filepath.Join(root, "casea.txt"), []byte("changed after execution"), 0600); err != nil {
							return err
						}
					}
				}
				return original(dir, path, data)
			}
			res, err := runOracle(t, req)
			if err != nil {
				t.Fatal(err)
			}
			if mode != "exact-buffer" {
				if res.ExecutionStatus != kit.StatusFailed || res.Set != nil {
					t.Fatalf("incomplete witness set accepted: %+v", res)
				}
				if _, err := os.Stat(filepath.Join(out, "manifest.json")); !os.IsNotExist(err) {
					t.Fatalf("failure published manifest: %v", err)
				}
				return
			}
			if res.Set == nil || !res.Set.Valid || res.ExecutionStatus != kit.StatusCompleted {
				t.Fatalf("witness set: %+v", res)
			}
			for index, want := range []string{"a = f(1);\n", ""} {
				name := fmt.Sprintf("%05d-c%c", index, 'a'+index)
				source, err := os.ReadFile(filepath.Join(out, "source", name+".bin"))
				if err != nil || string(source) != want {
					t.Fatalf("source buffer changed: %v", err)
				}
				var rec OracleRecord
				data, err := os.ReadFile(filepath.Join(out, "records", name+".json"))
				if err != nil || json.Unmarshal(data, &rec) != nil || rec.Input.SHA256 != sha(source) || rec.Steps[0].SourceSHA256 != sha(source) {
					t.Fatalf("source/record/driver binding: %v", err)
				}
			}
			data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
			var m kit.OracleManifest
			if err != nil || json.Unmarshal(data, &m) != nil || m.Schema != kit.OracleSourceManifestSchema {
				t.Fatalf("witness revision: %v", err)
			}
		})
	}
}

func TestOracleSourceWitnessSvcRefusedBeforeOutput(t *testing.T) {
	profile, root := oracleProfile(t, nil, "a = 1;\n")
	var p map[string]any
	if err := json.Unmarshal(profile, &p); err != nil {
		t.Fatal(err)
	}
	p["symbol"], p["format"] = "tree_sitter_c_sharp", kit.SvcLegacyFormat
	profile, _ = json.Marshal(p)
	out := filepath.Join(t.TempDir(), "set")
	req := oracleRequest(t, profile, root, out)
	req.SourceWitness = true
	res, err := runOracle(t, req)
	var ne *Error
	if !errors.As(err, &ne) || ne.Code != "SOURCE_WITNESS_SVC_UNSUPPORTED" || res.Build != nil || res.Assessment != kit.AssessBlocked {
		t.Fatalf("SVC witness refusal: %+v %v", res, err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("SVC refusal created output: %v", err)
	}
}

// S06-A02/A09/A10: a record run publishes a verified set bound to the build and the inputs;
// an existing output or a concurrent run on the same output is refused without touching
// it; a member write failure leaves no manifest and no completed set.
func TestOracleRecordSet(t *testing.T) {
	t.Parallel() // independent builds and outputs: run after the sequential (timing) tests
	profile, root := oracleProfile(t, []kit.OracleQuery{{ID: "ids", Source: "(identifier) @id"}}, "a = f(1);\n", "{ b = 2; }\n")
	base := t.TempDir()
	out := filepath.Join(base, "set")
	res, err := runOracle(t, oracleRequest(t, profile, root, out))
	if err != nil || res.ExecutionStatus != kit.StatusCompleted || res.Set == nil || !res.Set.Valid || res.Set.Records != 2 {
		t.Fatalf("%v %s %+v %+v", err, res.ExecutionStatus, res.Set, res.Findings)
	}
	var rec OracleRecord
	data, err := os.ReadFile(filepath.Join(out, "records", "00000-ca.json"))
	if err != nil || json.Unmarshal(data, &rec) != nil {
		t.Fatalf("record %v", err)
	}
	ids := rec.Steps[0].Incremental.Tree.Identities
	if rec.Schema != kit.OracleRecordSchema || !rec.Complete || len(ids) != 3 || ids[0].SHA256 != res.Build.Identity || ids[1].SHA256 != rec.Input.SHA256 ||
		rec.Steps[0].SourceSHA256 != rec.Input.SHA256 || len(rec.Steps[0].Incremental.Queries) != 1 {
		t.Fatalf("record binding %+v %+v", rec.Input, ids)
	}
	before := snapshot(t, out)
	if _, err := runOracle(t, oracleRequest(t, profile, root, out)); err == nil || !strings.Contains(err.Error(), "OUTPUT_EXISTS") {
		t.Fatalf("existing output accepted: %v", err)
	}
	after := snapshot(t, out)
	if len(after) != len(before) {
		t.Fatalf("existing set changed")
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("existing member %s changed", k)
		}
	}
	t.Run("concurrent", func(t *testing.T) {
		out := filepath.Join(base, "race")
		var wg sync.WaitGroup
		errs := make([]error, 2)
		results := make([]OracleResult, 2)
		for i := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i], errs[i] = runOracle(t, oracleRequest(t, profile, root, out))
			}()
		}
		wg.Wait()
		ok := 0
		for i := range 2 {
			if errs[i] == nil && results[i].Set != nil && results[i].Set.Valid {
				ok++
			} else if errs[i] == nil || !strings.Contains(errs[i].Error(), "OUTPUT_EXISTS") {
				t.Fatalf("loser %d: %v", i, errs[i])
			}
		}
		if ok != 1 {
			t.Fatalf("%d winners", ok)
		}
	})
}

// The failure hook is a package variable, so this test stays sequential: run in parallel it
// failed another test's records/00001-cb.json (TestOracleRecordObservedSet).
func TestOracleRecordSetWriteFailure(t *testing.T) {
	profile, root := oracleProfile(t, []kit.OracleQuery{{ID: "ids", Source: "(identifier) @id"}}, "a = f(1);\n", "{ b = 2; }\n")
	out := filepath.Join(t.TempDir(), "fail")
	orig := createExclusive
	defer func() { createExclusive = orig }()
	createExclusive = func(root *os.Root, path string, data []byte) error {
		if strings.Contains(path, "00001-cb") {
			return errors.New("injected write failure")
		}
		return orig(root, path, data)
	}
	res, err := runOracle(t, oracleRequest(t, profile, root, out))
	if err != nil || res.ExecutionStatus != kit.StatusFailed || res.Set != nil {
		t.Fatalf("%v %s %+v", err, res.ExecutionStatus, res.Set)
	}
	if _, err := os.Stat(filepath.Join(out, "manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest written after a failed member: %v", err)
	}
	if set := kit.VerifyOracleSet(os.DirFS(out)); set.Valid || set.Findings[0].Code != "MANIFEST_MISSING" {
		t.Fatalf("incomplete set verified: %+v", set)
	}
}

// C1-REAL-WORLD-SOURCE-WINDOWS-R3: on a host outside the operation's platform scope both
// commands refuse before any build or output; no native tool is needed to see it.
func TestPlatformScope(t *testing.T) {
	orig := hostPlatform
	hostPlatform = "linux/amd64"
	defer func() { hostPlatform = orig }()
	zero := strings.Repeat("0", 64)
	common := `"route":"owned","symbol":"tree_sitter_tsgk_plain","encoding":"UTF-8","output":"auto","compiler":{"name":"cc","version":"x","sha256":"` + zero +
		`","bytes":1},"grammar":[{"path":"src/parser.c","role":"parser","sha256":"` + zero + `","bytes":1}],"declarations":null`
	caseBase := `{"id":"c","input":{"path":"c.txt","role":"case","sha256":"` + zero + `","bytes":1},"edits":[],"points":[],"expect":[]`
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	inc := []byte(`{"schema":"tsgk-incremental/r2","id":"p","operation":"real-world-source-r3",` + common + `,"cases":[` + caseBase + `}]}`)
	ores, err := Incremental(context.Background(), IncrementalRequest{Root: dir, Profile: inc, Out: out, Allow: []string{AllowBuild, AllowExec}})
	if err == nil || !strings.Contains(err.Error(), "OPERATION_PLATFORM_SCOPE") || ores.Assessment != kit.AssessBlocked || ores.Build != nil {
		t.Fatalf("incremental r3 on linux: %v %+v", err, ores.Findings)
	}
	orc := []byte(`{"schema":"tsgk-oracle/r2","id":"p","operation":"native-query-large",` + common + `,"queries":[],"fact_pack":null,"api":false,"cases":[` + caseBase +
		`,"query_expect":[],"dynamic_sql_expect":null}]}`)
	res, err := Oracle(context.Background(), OracleRequest{Root: dir, Profile: orc, Out: out, Allow: []string{AllowBuild, AllowExec}})
	if err == nil || !strings.Contains(err.Error(), "OPERATION_PLATFORM_SCOPE") || res.Assessment != kit.AssessBlocked || res.Build != nil {
		t.Fatalf("oracle native-query-large on linux: %v %+v", err, res.Findings)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output created by a refused run: %v", err)
	}
}
