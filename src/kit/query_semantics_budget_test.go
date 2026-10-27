package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestSemanticCommonPreflightReservesBeforeAST(t *testing.T) {
	data := append([]byte(`{"x":[`), bytes.Repeat([]byte("0,"), 500000)...)
	data[len(data)-1] = ']'
	data = append(data, '}')
	for _, typedDecode := range []bool{false, true} {
		b := semanticTestBudget(t)
		buffers := 33*uint64(len(data)) + 32768
		if typedDecode {
			buffers += 2 * uint64(len(data))
		}
		b.logical = eligibilityLogicalLimit - (buffers + 448*8)
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		var e *Error
		if typedDecode {
			var dst map[string][]int
			e = b.decode("bounded-preflight", data, &dst, true)
		} else {
			e = b.reserveJSON("bounded-preflight", data, 3)
		}
		runtime.ReadMemStats(&after)
		if e == nil || e.Kind != KindResourceLimit || e.Code != "ELIGIBILITY_WORK_LIMIT" {
			t.Fatalf("container budget not enforced before AST (typed=%v): %v", typedDecode, e)
		}
		if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
			t.Fatalf("AST allocated before container reservation: %d", allocated)
		}
	}
	b := semanticTestBudget(t)
	sparse := append([]byte(`{"x":1}`), bytes.Repeat([]byte(" "), 8<<20)...)
	if e := b.reserveJSON("sparse-preflight", sparse, 3); e != nil {
		t.Fatalf("sparse valid input acquired a worst-case value count: %v", e)
	}
	if _, e := b.semanticAST("sparse-ast", sparse); e != nil {
		t.Fatalf("sparse AST retained a byte/2 value estimate: %v", e)
	}
}

func TestSemanticManifestSelectionBeforeDecode(t *testing.T) {
	t.Run("wide-before-schema", func(t *testing.T) {
		data := []byte(`{"extra":{`)
		for i := 0; i < 900000; i++ {
			data = append(data, '"', 'k')
			data = strconv.AppendInt(data, int64(i), 10)
			data = append(data, []byte(`":null,`)...)
		}
		data[len(data)-1] = '}'
		data = append(data, []byte(`,"schema":"tsgk-oracle-manifest/r5"}`)...)
		if uint64(len(data)) > MaxDocumentBytes {
			t.Fatal("fixture exceeds document cap")
		}
		expired, stop := context.WithCancel(context.Background())
		stop()
		testHookWall = func() (context.Context, context.CancelFunc) { return expired, func() {} }
		t.Cleanup(func() { testHookWall = nil })
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		result := VerifyOracleSet(fstest.MapFS{"manifest.json": &fstest.MapFile{Data: data}})
		runtime.ReadMemStats(&after)
		found := false
		for _, f := range result.Findings {
			found = found || f.Code == "WALL_LIMIT"
		}
		if !found {
			t.Fatalf("manifest decoded before bounded reservation: %+v", result.Findings)
		}
		// io.ReadAll copies the input; the selector must not build the wide object's name index.
		if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8*uint64(len(data))+8<<20 {
			t.Fatalf("manifest header allocated nested names: %d", allocated)
		}
	})
	t.Run("depth-before-late-syntax", func(t *testing.T) {
		data := []byte(`{"schema":"tsgk-oracle-manifest/r5","extra":` + strings.Repeat("[", 33) + "?")
		result := VerifyOracleSet(fstest.MapFS{"manifest.json": &fstest.MapFile{Data: data}})
		if len(result.Findings) != 1 || result.Findings[0].Code != "JSON_DEPTH_LIMIT" {
			t.Fatalf("unbounded header replaced first depth error: %+v", result.Findings)
		}
	})
}

func TestSemanticReplayProfileBudgetAndCancellation(t *testing.T) {
	t.Run("cancel-before-malformed-body", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		cancel()
		result, err := Replay(ctx, ReplayRequest{Profile: []byte(`{"reducer":"oracle-set-r4","extra":[?`)})
		if result.ResultSchema != ReplayResultSchema || result.QuerySemantics != nil {
			t.Fatal("unparsed profile claimed a semantic result revision")
		}
		if e := asError(err); e == nil || e.Kind != KindCancelled {
			t.Fatalf("profile decoded before caller cancellation: %v", err)
		}
	})
	t.Run("values-before-AST", func(t *testing.T) {
		data := append([]byte(`{"reducer":"oracle-set-r4","extra":[`), bytes.Repeat([]byte("0,"), 2000001)...)
		data[len(data)-1] = ']'
		data = append(data, '}')
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := Replay(ctx, ReplayRequest{Profile: data})
		runtime.ReadMemStats(&after)
		if e := asError(err); e == nil || e.Kind != KindResourceLimit {
			t.Fatalf("profile AST not bounded: %v", err)
		}
		if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 16*uint64(len(data))+8<<20 {
			t.Fatalf("profile AST allocated before ledger: %d", allocated)
		}
	})
}

func TestSemanticSelectionPreservesTypesAndUTF8(t *testing.T) {
	for _, field := range []struct{ doc, name, target string }{{"manifest", "schema", OracleSemanticsManifestSchema}, {"replay", "reducer", "oracle-set-r4"}} {
		for _, value := range []string{"0", "false", "{\"nested\":0}", "[0,1]", "{}", "[]", "null"} {
			_, e := selectJSONRevision([]byte(`{"`+field.name+`":`+value+`}`), nil, field.doc, field.name, field.target, nil)
			code := "JSON_TYPE"
			if value == "null" {
				code = "JSON_NULL"
			}
			if e == nil || e.Code != code {
				t.Fatalf("%s nonstring %s: %v", field.name, value, e)
			}
		}
	}
	for _, typedDecode := range []bool{false, true} {
		b := semanticTestBudget(t)
		data := []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}
		var e *Error
		if typedDecode {
			var dst map[string]string
			e = b.decode("invalid-utf8", data, &dst, true)
		} else {
			e = b.reserveJSON("invalid-utf8", data, 3)
		}
		if e == nil || e.Kind != KindInvalidInput || e.Code != "JSON_INVALID_UTF8" {
			t.Fatalf("UTF8 code changed (typed=%v): %v", typedDecode, e)
		}
	}
}

func TestSemanticPreflightPreservesStandaloneSparseApproval(t *testing.T) {
	for _, inside := range []bool{false, true} {
		t.Run(strconv.FormatBool(inside), func(t *testing.T) {
			f := newEligibilityFixture(t)
			req := f.request()
			pad := func(data []byte) []byte {
				if len(data) >= 15<<20 || data[0] != '{' {
					t.Fatal("invalid fixture")
				}
				spaces := bytes.Repeat([]byte(" "), (15<<20)-len(data))
				if inside {
					out := make([]byte, 0, 15<<20)
					out = append(out, '{')
					out = append(out, spaces...)
					return append(out, data[1:]...)
				}
				return append(data, spaces...)
			}
			req.Registration = pad(req.Registration)
			f.approval.RegistrationSHA256 = digestHex(req.Registration)
			req.Approval = pad(eligibilityJSON(t, f.approval))
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := VerifyQueryReferenceEligibility(ctx, req)
			if err != nil || !result.Complete || result.Assessment != AssessPass {
				t.Fatalf("legacy15MiB pair lost budget (inside=%v): %v %+v", inside, err, result)
			}
			t.Logf("two15MiB documents accepted inside=%v: logical=%d work=%d", inside, result.LogicalBytes, result.Work)
		})
	}
}

func TestSemanticSelectorMalformedLiterals(t *testing.T) {
	for _, field := range []struct{ doc, name, target string }{{"manifest", "schema", OracleSemanticsManifestSchema}, {"replay", "reducer", "oracle-set-r4"}} {
		for _, tc := range []struct{ tail, code string }{{"n}", "JSON_SYNTAX"}, {"n", "JSON_TRUNCATED"}, {"tr}", "JSON_SYNTAX"}, {"tr", "JSON_TRUNCATED"}, {"truex}", "JSON_SYNTAX"}, {"0e}", "JSON_SYNTAX"}, {"0e", "JSON_TRUNCATED"}, {"01}", "JSON_SYNTAX"}} {
			_, e := selectJSONRevision([]byte(`{"`+field.name+`":`+tc.tail), nil, field.doc, field.name, field.target, nil)
			if e == nil || e.Code != tc.code {
				t.Fatalf("%s %s: want %s got %v", field.name, tc.tail, tc.code, e)
			}
		}
	}
}

func TestSemanticUTF8BudgetBoundaryPriority(t *testing.T) {
	data := []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}
	for _, typedDecode := range []bool{false, true} {
		initial := uint64(len(data))
		if typedDecode {
			initial *= 2
		}
		for _, enough := range []bool{false, true} {
			b := semanticTestBudget(t)
			remaining := initial - 1
			if enough {
				remaining = initial + 1
			}
			b.logical = eligibilityLogicalLimit - remaining
			var e *Error
			if typedDecode {
				var dst map[string]string
				e = b.decode("utf8-boundary", data, &dst, true)
			} else {
				e = b.reserveJSON("utf8-boundary", data, 3)
			}
			code := "ELIGIBILITY_WORK_LIMIT"
			if enough {
				code = "JSON_INVALID_UTF8"
			}
			if e == nil || e.Code != code {
				t.Fatalf("typed=%v enough=%v: want %s got %v", typedDecode, enough, code, e)
			}
		}
	}
}

func TestSemanticReplayInvalidRootIsBounded(t *testing.T) {
	data := append([]byte("["), bytes.Repeat([]byte("0,"), 2000001)...)
	data[len(data)-1] = ']'
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		if canceled {
			cancel()
		} else {
			defer cancel()
		}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := Replay(ctx, ReplayRequest{Profile: data})
		runtime.ReadMemStats(&after)
		kind := KindResourceLimit
		if canceled {
			kind = KindCancelled
		}
		if e := asError(err); e == nil || e.Kind != kind {
			t.Fatalf("invalidroot cancel=%v: %v", canceled, err)
		}
		if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 16*uint64(len(data))+8<<20 {
			t.Fatalf("invalidroot built AST: %d", allocated)
		}
	}
	for _, tc := range []struct{ data, code string }{{"[0]", "JSON_TYPE"}, {"[?]", "JSON_SYNTAX"}, {"[0", "JSON_TRUNCATED"}, {"null", "JSON_NULL"}, {"[] 1", "JSON_TRAILING_VALUE"}} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		_, err := Replay(ctx, ReplayRequest{Profile: []byte(tc.data)})
		cancel()
		if e := asError(err); e == nil || e.Code != tc.code {
			t.Fatalf("root %q: want%s got%v", tc.data, tc.code, err)
		}
	}
}

func TestSemanticReplayUTF8CancellationPriority(t *testing.T) {
	for _, semantic := range []bool{false, true} {
		for _, canceled := range []bool{false, true} {
			reducer := "oracle-set-r2"
			if semantic {
				reducer = "oracle-set-r4"
			}
			data := append([]byte(`{"reducer":"`+reducer+`","extra":"`), 0xff)
			data = append(data, []byte(`"}`)...)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			if canceled {
				cancel()
			} else {
				defer cancel()
			}
			_, err := Replay(ctx, ReplayRequest{Profile: data})
			code := "JSON_INVALID_UTF8"
			if semantic && canceled {
				code = "CANCELLED"
			}
			if e := asError(err); e == nil || e.Code != code {
				t.Fatalf("semantic=%v cancel=%v: want%s got%v", semantic, canceled, code, err)
			}
		}
	}
}

func TestSemanticSelectorPreservesUTF8OnErrors(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		if canceled {
			cancel()
		} else {
			defer cancel()
		}
		for _, prefix := range []string{`{"reducer":"oracle-set-r2`, `{"reducer":"oracle-set-r4`, `{"reducer"0,"extra":"`} {
			data := append([]byte(prefix), 0xff)
			data = append(data, []byte(`"}`)...)
			_, err := Replay(ctx, ReplayRequest{Profile: data})
			if e := asError(err); e == nil || e.Code != "JSON_INVALID_UTF8" {
				t.Fatalf("early selector error lost UTF8 priority cancel=%v: %v", canceled, err)
			}
		}
	}
	for _, field := range []struct{ doc, name, target string }{{"manifest", "schema", OracleSemanticsManifestSchema}, {"replay", "reducer", "oracle-set-r4"}} {
		for _, value := range []string{`"bad\q"`, `"\ud800"`, `"\udc00"`} {
			_, e := selectJSONRevision([]byte(`{"`+field.name+`":`+value+`}`), nil, field.doc, field.name, field.target, nil)
			if e == nil || e.Code != "JSON_SYNTAX" {
				t.Fatalf("invalid scalar escape changed error: %v", e)
			}
		}
	}
}

func TestSemanticProjectionShapeMapReservation(t *testing.T) {
	b := semanticTestBudget(t)
	b.logical = eligibilityLogicalLimit - 1
	budget := &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, "projection-shape") }, reserve: func(n uint64) *Error { return b.charge(0, n, "projection-shape") }}
	v := &jv{kind: '{'}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	e := witnessShape(typed{doc: "projection"}, v, reflect.TypeFor[witnessProjection](), false, false, budget)
	runtime.ReadMemStats(&after)
	if e == nil || e.Kind != KindResourceLimit || e.Code != "ELIGIBILITY_WORK_LIMIT" {
		t.Fatalf("projection shape allocated reflection map before budget: %v", e)
	}
	if after.TotalAlloc-before.TotalAlloc > 1<<20 {
		t.Fatal("projection reflection map allocated before reservation")
	}
}

type semanticRawReadFS struct {
	fs.FS
	path      string
	afterRead func()
}
type semanticRawReadFile struct {
	fs.File
	afterRead func()
}

func (f semanticRawReadFS) Open(path string) (fs.File, error) {
	file, err := f.FS.Open(path)
	if err != nil || path != f.path {
		return file, err
	}
	return semanticRawReadFile{file, f.afterRead}, nil
}
func (f semanticRawReadFile) Close() error { err := f.File.Close(); f.afterRead(); return err }

func TestSemanticManifestRawDoesNotDecodeUnusedResponseID(t *testing.T) {
	for _, wide := range []bool{false, true} {
		t.Run(strconv.FormatBool(wide), func(t *testing.T) {
			_, fixture, _ := semanticBoundFixture(t, 0)
			raw := []byte(`{"id":"c1","padding":[` + strings.Repeat("0,", 2000001) + "0]}")
			if wide {
				raw = []byte(`{"id":"c1","padding":{`)
				for i := 0; i < 450000; i++ {
					raw = append(raw, '"', 'k')
					raw = strconv.AppendInt(raw, int64(i), 10)
					raw = append(raw, []byte(`":null,`)...)
				}
				raw[len(raw)-1] = '}'
				raw = append(raw, '}')
			}
			var manifest OracleManifest
			if err := json.Unmarshal(fixture.files["oracle/manifest.json"], &manifest); err != nil {
				t.Fatal(err)
			}
			rawPath := ""
			for i := range manifest.Members {
				m := &manifest.Members[i]
				if m.Role == "raw" {
					rawPath = m.Path
					m.Bytes, m.SHA256 = uint64(len(raw)), digestHex(raw)
					break
				}
			}
			if rawPath == "" {
				t.Fatal("missing raw control")
			}
			files := fstest.MapFS{}
			for path, data := range fixture.files {
				if strings.HasPrefix(path, "oracle/") {
					files[strings.TrimPrefix(path, "oracle/")] = &fstest.MapFile{Data: data}
				}
			}
			files["manifest.json"] = &fstest.MapFile{Data: mustJSON(t, manifest)}
			files[rawPath] = &fstest.MapFile{Data: raw}
			wall, stop := context.WithCancel(context.Background())
			defer stop()
			testHookWall = func() (context.Context, context.CancelFunc) { return wall, func() {} }
			defer func() { testHookWall = nil }()
			var before, after runtime.MemStats
			reads := 0
			result := VerifyOracleSet(semanticRawReadFS{files, rawPath, func() { reads++; runtime.ReadMemStats(&before); stop() }})
			runtime.ReadMemStats(&after)
			found := false
			for _, finding := range result.Findings {
				found = found || finding.Code == "WALL_LIMIT"
			}
			if reads != 1 || !found || result.Valid {
				t.Fatalf("raw read did not stop at shared wall: reads=%d %+v", reads, result)
			}
			// Exclude io.ReadAll; no raw JSON decoder may build a name index after this read.
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
				t.Fatalf("unused raw ID decoded outside reservation: %d", allocated)
			}
		})
	}
}

func TestSemanticSelectionPreservesLegacyDecoderErrorOrder(t *testing.T) {
	for _, tc := range []struct{ name, tail, code string }{
		{"depth-before-truncation", strings.Repeat("[", 33) + "?", "JSON_DEPTH_LIMIT"},
		{"depth-before-trailing", strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33) + "}?", "JSON_DEPTH_LIMIT"},
		{"syntax-before-depth", "?" + strings.Repeat("[", 33), "JSON_SYNTAX"},
		{"duplicate-before-depth", "0,\"x\":" + strings.Repeat("[", 33) + "?", "JSON_DUPLICATE_KEY"},
		{"utf8-before-depth", strings.Repeat("[", 33) + string([]byte{0xff}), "JSON_INVALID_UTF8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			replay := []byte(`{"reducer":"oracle-set-r2","x":` + tc.tail)
			_, oldErr := ParseReplayProfile(replay)
			if oldErr == nil || asError(oldErr).Code != tc.code {
				t.Fatalf("legacy parser control: %v", oldErr)
			}
			result, err := Replay(ctxT(t), ReplayRequest{Profile: replay})
			expected := newReport("replay")
			failReport(&expected, asError(oldErr))
			if err == nil || asError(err).Code != tc.code || asError(err).Kind != asError(oldErr).Kind || result.ExecutionStatus != expected.ExecutionStatus || result.Assessment != expected.Assessment {
				t.Fatalf("selector changed legacy replay: %v %+v", err, result.Report)
			}
			manifest := []byte(`{"schema":"` + OracleSourceManifestSchema + `","x":` + tc.tail)
			var old OracleManifest
			expectedErr := decodeOracleManifestVersion(manifest, &old, OracleManifestSchema, OracleSourceManifestSchema, OracleBatchManifestSchema, OracleAcceptedManifestSchema)
			if expectedErr == nil || expectedErr.Code != tc.code {
				t.Fatalf("legacy manifest control: %v", expectedErr)
			}
			checked := VerifyOracleSet(fstest.MapFS{"manifest.json": &fstest.MapFile{Data: manifest}})
			if checked.Valid || len(checked.Findings) != 1 || checked.Findings[0].Code != expectedErr.Code {
				t.Fatalf("selector changed legacy manifest: %+v", checked)
			}
		})
	}
}
