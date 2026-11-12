package kit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// ordinaryBinaryPagedRebind copies a producer directory (hard-linking the large members), applies a
// consistent mutation to the r8 manifest and the run binding, and returns the new producer directory.
// Every pin the mutation touches is recomputed in both documents, so only a semantic check of the paged
// closure can reject the copy.
func ordinaryBinaryPagedRebind(t *testing.T, producerDir string, mutate func(m *OrdinaryOracleManifest, b *OrdinaryRunBinding, write func(rel string, data []byte), remove func(rel string))) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(producerDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(producerDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if err := os.Link(path, target); err == nil {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	r8 := filepath.Join(dst, "r8")
	var manifest OrdinaryOracleManifest
	manifestBytes, err := os.ReadFile(filepath.Join(r8, "manifest.json"))
	if err != nil || json.Unmarshal(manifestBytes, &manifest) != nil {
		t.Fatalf("manifest: %v", err)
	}
	var meta struct {
		Profile    []byte `json:"profile"`
		RunBinding []byte `json:"run_binding"`
		RunID      string `json:"run_id"`
	}
	metaBytes, err := os.ReadFile(filepath.Join(dst, "producer.json"))
	if err != nil || json.Unmarshal(metaBytes, &meta) != nil {
		t.Fatalf("producer metadata: %v", err)
	}
	var binding OrdinaryRunBinding
	if err := json.Unmarshal(meta.RunBinding, &binding); err != nil {
		t.Fatal(err)
	}
	write := func(rel string, data []byte) {
		t.Helper()
		target := filepath.Join(r8, filepath.FromSlash(rel))
		// The member may be a hard link shared with the original; replace the name, never the bytes.
		_ = os.Remove(target)
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	remove := func(rel string) {
		t.Helper()
		if err := os.Remove(filepath.Join(r8, filepath.FromSlash(rel))); err != nil {
			t.Fatal(err)
		}
	}
	mutate(&manifest, &binding, write, remove)
	// encoding/json (v1) keeps the producer's wire shape, including nanosecond durations.
	marshal := func(v any) []byte {
		t.Helper()
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	meta.RunBinding = marshal(binding)
	if err := os.WriteFile(filepath.Join(dst, "producer.json"), marshal(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(r8, "manifest.json"))
	if err := os.WriteFile(filepath.Join(r8, "manifest.json"), marshal(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

// ordinaryBinaryPagedRepin sets the pin of one member path in both documents.
func ordinaryBinaryPagedRepin(m *OrdinaryOracleManifest, b *OrdinaryRunBinding, path string, data []byte) {
	for _, list := range []*[]OrdinaryMember{&m.Members, &b.Members} {
		for i := range *list {
			if (*list)[i].Path == path {
				(*list)[i].Pin.Bytes, (*list)[i].Pin.SHA256 = uint64(len(data)), digestHex(data)
			}
		}
	}
}

func ordinaryBinaryPagedMembers(m OrdinaryOracleManifest, role string) []OrdinaryMember {
	var out []OrdinaryMember
	for _, member := range m.Members {
		if member.Role == role {
			out = append(out, member)
		}
	}
	return out
}

// TestOrdinaryBinaryPagedOwnedTSQLPublicReplayAndPageCounterexamples runs the real owned native producer for the
// smallest TSQL P0 case. The TSQL grammar parser (68 MB) and driver (35 MB) are past the 16 MiB member limit,
// so the closure is binary-paged: paged grammar parser and driver pages in the r8. The public API, the r9
// replay and `tsgk replay` must agree. TSQL has no independent expected body for this case, so all eight
// tuples stay BLOCKED; that is the evidence the execution path runs to its end, not a semantic verdict. Then
// page omission, a stale descriptor and a changed page order must each change the result.
//
// The prepared TSQL grammar is 68 MB and cannot be tracked; the test needs TSGK_ORDINARY_SQL_E2E=1.
func TestOrdinaryBinaryPagedOwnedTSQLPublicReplayAndPageCounterexamples(t *testing.T) {
	if os.Getenv("TSGK_ORDINARY_SQL_E2E") != "1" {
		t.Skip("set TSGK_ORDINARY_SQL_E2E=1 to run the binary-paged TSQL native regression (the prepared 68 MB grammar is not tracked)")
	}
	ordinaryFullPosNativeSetup(t)
	const caseID = "alt-tsql-B04-r144"
	req, producer, references, bodies := ordinaryBinaryPagedTSQLInput(t, caseID)
	inputPath, producerDir := ordinaryE2EWriteProducerInput(t, producer)
	ordinaryE2ERunOwnedProducer(t, inputPath, producerDir)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	assembled := ordinaryE2EAssemble(t, req, producerDir, references, bodies)

	var manifest OrdinaryOracleManifest
	data, err := os.ReadFile(filepath.Join(producerDir, "r8", "manifest.json"))
	if err != nil || json.Unmarshal(data, &manifest) != nil {
		t.Fatalf("manifest: %v", err)
	}
	if manifest.ClosureEncoding != "binary-paged" || len(ordinaryBinaryPagedMembers(manifest, "driver-paged-descriptor")) != 1 ||
		len(ordinaryBinaryPagedMembers(manifest, "driver-paged-part")) < 2 || len(ordinaryBinaryPagedMembers(manifest, "grammar-paged-descriptor")) != 1 ||
		len(ordinaryBinaryPagedMembers(manifest, "grammar-paged-part")) < 2 {
		t.Fatalf("the r8 is not a binary-paged closure: encoding=%s driver pages=%d grammar pages=%d", manifest.ClosureEncoding,
			len(ordinaryBinaryPagedMembers(manifest, "driver-paged-part")), len(ordinaryBinaryPagedMembers(manifest, "grammar-paged-part")))
	}
	t.Logf("binary-paged r8: driver pages=%d grammar pages=%d members=%d", len(ordinaryBinaryPagedMembers(manifest, "driver-paged-part")),
		len(ordinaryBinaryPagedMembers(manifest, "grammar-paged-part")), len(manifest.Members))

	api, err := ordinaryFullPosVerify(t, ctx, assembled)
	if err != nil || !api.BindingValid || api.Semantic != AssessBlocked || api.Tuple.Required != 8 || api.Tuple.Blocked != 8 || api.Tuple.Verified != 0 || api.Tuple.Failed != 0 || api.Tuple.Unprocessed != 0 ||
		!api.ProcessingComplete || api.FullScopeComplete || api.BaseVerified.Assessment != AssessPass || api.Eligibility == nil || api.Eligibility.Assessment != AssessPass {
		t.Fatalf("binary-paged public API did not run to the end: err=%v result=%+v", err, api)
	}
	env := ordinaryFullPosBuildCLI(t)
	replay, cli := ordinaryFullPosParity(t, ctx, env, assembled, api)
	t.Logf("binary-paged TSQL %s: tuples=%+v code=%s coverage=%s", caseID, api.Tuple, api.Code, api.CoverageSHA256)
	t.Logf("binary-paged budget read/work/logical API=%d/%d/%d replay=%d/%d/%d cli=%d/%d/%d", api.BytesRead, api.Work, api.LogicalBytes,
		replay.BytesRead, replay.Work, replay.LogicalBytes, cli.BytesRead, cli.Work, cli.LogicalBytes)

	// A page counterexample is rejected when the result no longer matches the unmodified run: an error, a broken
	// binding, a different code or an eligibility or base verdict that is no longer PASS.
	differs := func(name string, dir string) {
		t.Helper()
		request := ordinaryE2EAssemble(t, req, dir, references, bodies)
		result, err := ordinaryFullPosVerify(t, ctx, request)
		code := result.Code
		if err != nil {
			code = asError(err).Code
		}
		same := err == nil && result.BindingValid && result.Code == api.Code && result.BaseVerified.Assessment == AssessPass && result.Eligibility != nil && result.Eligibility.Assessment == AssessPass
		if same || result.Semantic == AssessPass || result.Tuple.Verified != 0 {
			t.Fatalf("%s: the paged-closure counterexample was accepted like the unmodified run: err=%v result=%+v", name, err, result)
		}
		t.Logf("page counterexample %-38s -> code=%s semantic=%s binding=%t eligibility=%v verified=%d", name, code, result.Semantic, result.BindingValid, result.Eligibility != nil && result.Eligibility.Assessment == AssessPass, result.Tuple.Verified)
	}
	descriptorPath := func(role string, m OrdinaryOracleManifest) string {
		list := ordinaryBinaryPagedMembers(m, role)
		return list[0].Path
	}

	differs("driver page omitted", ordinaryBinaryPagedRebind(t, producerDir, func(m *OrdinaryOracleManifest, b *OrdinaryRunBinding, _ func(string, []byte), remove func(string)) {
		part := ordinaryBinaryPagedMembers(*m, "driver-paged-part")[1].Path
		drop := func(list []OrdinaryMember) []OrdinaryMember {
			return slices.DeleteFunc(list, func(member OrdinaryMember) bool { return member.Path == part })
		}
		m.Members, b.Members = drop(m.Members), drop(b.Members)
		remove(part)
	}))
	differs("grammar page omitted", ordinaryBinaryPagedRebind(t, producerDir, func(m *OrdinaryOracleManifest, b *OrdinaryRunBinding, _ func(string, []byte), remove func(string)) {
		part := ordinaryBinaryPagedMembers(*m, "grammar-paged-part")[1].Path
		drop := func(list []OrdinaryMember) []OrdinaryMember {
			return slices.DeleteFunc(list, func(member OrdinaryMember) bool { return member.Path == part })
		}
		m.Members, b.Members = drop(m.Members), drop(b.Members)
		remove(part)
	}))
	// The descriptor keeps its indexes and offsets but its first two parts exchange files: the page order changed,
	// every shape check passes, and only the hash of the reassembled logical file can reject it.
	swapParts := func(role string) func(m *OrdinaryOracleManifest, b *OrdinaryRunBinding, write func(string, []byte), _ func(string)) {
		return func(m *OrdinaryOracleManifest, b *OrdinaryRunBinding, write func(string, []byte), _ func(string)) {
			path := descriptorPath(role, *m)
			original, err := os.ReadFile(filepath.Join(producerDir, "r8", filepath.FromSlash(path)))
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]json.RawMessage
			var parts []map[string]json.RawMessage
			if err := json.Unmarshal(original, &document); err != nil || json.Unmarshal(document["parts"], &parts) != nil || len(parts) < 2 {
				t.Fatalf("descriptor %s: %v", path, err)
			}
			parts[0]["path"], parts[1]["path"] = parts[1]["path"], parts[0]["path"]
			parts[0]["sha256"], parts[1]["sha256"] = parts[1]["sha256"], parts[0]["sha256"]
			raw, err := json.Marshal(parts)
			if err != nil {
				t.Fatal(err)
			}
			document["parts"] = raw
			changed, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			write(path, changed)
			ordinaryBinaryPagedRepin(m, b, path, changed)
		}
	}
	differs("driver pages reordered in the descriptor", ordinaryBinaryPagedRebind(t, producerDir, swapParts("driver-paged-descriptor")))
	differs("grammar pages reordered in the descriptor", ordinaryBinaryPagedRebind(t, producerDir, swapParts("grammar-paged-descriptor")))
	// Page contents exchanged with re-pinned members and a stale descriptor.
	differs("driver page contents exchanged", ordinaryBinaryPagedRebind(t, producerDir, func(m *OrdinaryOracleManifest, b *OrdinaryRunBinding, write func(string, []byte), _ func(string)) {
		parts := ordinaryBinaryPagedMembers(*m, "driver-paged-part")
		first, second := parts[0].Path, parts[1].Path
		a, err := os.ReadFile(filepath.Join(producerDir, "r8", filepath.FromSlash(first)))
		if err != nil {
			t.Fatal(err)
		}
		c, err := os.ReadFile(filepath.Join(producerDir, "r8", filepath.FromSlash(second)))
		if err != nil {
			t.Fatal(err)
		}
		write(first, c)
		write(second, a)
		ordinaryBinaryPagedRepin(m, b, first, c)
		ordinaryBinaryPagedRepin(m, b, second, a)
	}))
}

func ordinaryBinaryPagedTSQLInput(t *testing.T, caseID string) (OrdinaryQuerySemanticsRequest, ordinaryE2EProducerInput, []ordinaryE2EReference, map[string][]byte) {
	t.Helper()
	const route = "tsql"
	_, inventory := ordinaryP0ForPlanTest(t)
	workload := inventory.Routes[ordinaryRouteIndex(inventory, route)].Workload
	index := slices.IndexFunc(workload.Cases, func(c QualCase) bool { return c.ID == caseID })
	if index < 0 {
		t.Fatalf("P0 TSQL case %s is missing", caseID)
	}
	qual := workload.Cases[index]
	source, ok := ordinaryRouteSources(t, route)[caseID]
	if !ok || uint64(len(source)) != qual.Input.Bytes || digestHex([]byte(source)) != qual.Input.SHA256 || len(qual.Edits) != 0 {
		t.Fatalf("selected P0 source pin mismatch for %s", caseID)
	}
	querySources := ordinaryRouteQueries(t, route)
	queries := make([]OracleQuery, 0, len(workload.Queries))
	for _, query := range workload.Queries {
		text, ok := querySources[query.ID]
		if !ok || digestHex([]byte(text)) != query.SHA256 {
			t.Fatalf("P0 query source pin mismatch for %s/%s", route, query.ID)
		}
		queries = append(queries, OracleQuery{ID: query.ID, Source: text})
	}
	packBytes, err := os.ReadFile("../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	factMapping, err := os.ReadFile("../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := os.ReadFile(os.Getenv("TSGK_NATIVE_CC"))
	if err != nil {
		t.Fatal(err)
	}
	compilerHash := sha256.Sum256(compiler)
	profile := struct {
		Schema       string        `json:"schema"`
		ID           string        `json:"id"`
		Route        string        `json:"route"`
		Operation    string        `json:"operation"`
		Symbol       string        `json:"symbol"`
		Encoding     string        `json:"encoding"`
		Output       string        `json:"output"`
		Compiler     ToolIdentity  `json:"compiler"`
		Grammar      []NativeInput `json:"grammar"`
		Declarations *Declarations `json:"declarations"`
		Queries      []OracleQuery `json:"queries"`
		FactPack     *FactPackRef  `json:"fact_pack"`
		API          bool          `json:"api"`
		Cases        []OracleCase  `json:"cases"`
	}{Schema: OracleSchema, ID: "ordinary-binary-paged-tsql", Route: route, Operation: workload.Operation, Symbol: workload.Symbol,
		Encoding: EncodingUTF8, Output: OutputTree, Compiler: ToolIdentity{Name: "cc", Version: "configured-pinned-tool", SHA256: hex.EncodeToString(compilerHash[:]), Bytes: uint64(len(compiler))},
		Grammar: workload.Grammar, Declarations: workload.Declarations, Queries: queries, FactPack: &FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: route}, API: true,
		Cases: []OracleCase{{IncrementalCase: IncrementalCase{ID: qual.ID, Input: NativeInput{Path: "cases/" + qual.ID + ".sql", Role: "case", Bytes: qual.Input.Bytes, SHA256: qual.Input.SHA256},
			Encoding: EncodingUTF8, Edits: qual.Edits, Points: append([]NativePoint{}, qual.Points...), Expect: qual.Expect}, QueryExpect: []QueryExpectation{}}}}
	profileBytes := ordinaryJSON(t, profile)
	refLibrary := []byte("synthetic reference library; never loaded")
	refPin := OrdinaryPin{Path: "build/reference-library.bin", Bytes: uint64(len(refLibrary)), SHA256: digestHex(refLibrary)}
	planned := OrdinaryMember{Owner: "D_build", Path: refPin.Path, Role: "reference-library", CaseOrBatch: "build", Pin: refPin}
	req, registration, plan, _, selectedSource, _, _ := ordinarySelectedInput(t, route, caseID, OrdinaryAssetRegistration{}, "binary-paged", true, profileBytes, planned)
	producer := ordinaryE2EProducerInput{Profile: profileBytes, PlanningProfile: req.PlanningProfile, AssetRegistration: req.AssetRegistration, ShardPlan: ordinaryJSON(t, plan),
		RegistrationBytes: req.Registration, Registration: registration, ShardIDs: req.Selection.ShardIDs, Sources: [][]byte{selectedSource}, FactPack: packBytes, FactMapping: factMapping,
		GrammarRoot: ordinaryPreparedRoute(t, route)}
	references := ordinarySQLSyntheticReferences(t, profileBytes, selectedSource, registration, ordinarySQLExpectedSet{}, nil)
	if !strings.HasPrefix(source, "dbo.") {
		t.Fatalf("unexpected P0 TSQL source %q", source)
	}
	return req, producer, references, map[string][]byte{}
}
