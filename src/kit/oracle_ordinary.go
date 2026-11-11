package kit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	jsonv1 "encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"strings"
)

const (
	OracleOrdinaryRecordSchema   = "tsgk-oracle-record/r5"
	OracleOrdinaryManifestSchema = "tsgk-oracle-manifest/r8"
	OracleOrdinaryBindingSchema  = "tsgk-query-semantics-run-binding/r2"
)

// OrdinaryRunBatch points to one captured driver process. Its owner is shared by all
// process-level members in the batch, rather than copied onto each case.
type OrdinaryRunBatch struct {
	Index   int          `json:"index"`
	Owner   string       `json:"owner"`
	Capture ReplayMember `json:"capture"`
	Cases   []string     `json:"cases"`
}

// OrdinaryRunBinding records actual producer members in caller-owned output.
type OrdinaryRunBinding struct {
	Schema            string             `json:"schema"`
	Context           string             `json:"context"`
	Profile           OrdinaryPin        `json:"profile_pin"`
	PlanningProfile   OrdinaryPin        `json:"planning_profile_pin"`
	Registration      OrdinaryPin        `json:"registration_pin"`
	Assets            OrdinaryPin        `json:"asset_registration_pin"`
	Plan              OrdinaryPin        `json:"shard_plan_pin"`
	Candidate         string             `json:"candidate"`
	SemanticCohort    string             `json:"semantic_cohort"`
	RunID             string             `json:"run_id"`
	ProducerOwner     string             `json:"producer_owner"`
	ShardIDs          []string           `json:"selected_shard_ids"`
	FullScopeComplete bool               `json:"full_scope_complete"`
	Members           []OrdinaryMember   `json:"members"`
	Batches           []OrdinaryRunBatch `json:"batches"`
}

// OrdinaryOracleRecord keeps the producer-local pending judgment beside a separately pinned
// immutable base projection. This is storage shape only; it does not approve the judgment.
type OrdinaryOracleRecord struct {
	Schema         string                `json:"schema"`
	Case           string                `json:"case"`
	Workload       string                `json:"workload"`
	BaseProjection ReplayMember          `json:"base_projection"`
	QuerySemantics *OracleQuerySemantics `json:"query_semantics"`
	Transport      jsonv1.RawMessage     `json:"transport"`
	StepFacts      jsonv1.RawMessage     `json:"step_facts"`
	Complete       bool                  `json:"complete"`
}

// OrdinaryOracleManifest is the r8 completeness inventory for an ordinary batch run.
type OrdinaryOracleManifest struct {
	Schema            string             `json:"schema"`
	Context           string             `json:"context"`
	ClosureEncoding   string             `json:"closure_encoding"`
	Profile           OrdinaryPin        `json:"profile_pin"`
	PlanningProfile   OrdinaryPin        `json:"planning_profile_pin"`
	Registration      OrdinaryPin        `json:"registration_pin"`
	Assets            OrdinaryPin        `json:"asset_registration_pin"`
	Plan              OrdinaryPin        `json:"shard_plan_pin"`
	Candidate         string             `json:"candidate"`
	SemanticCohort    string             `json:"semantic_cohort"`
	RunID             string             `json:"run_id"`
	ProducerOwner     string             `json:"producer_owner"`
	ShardIDs          []string           `json:"selected_shard_ids"`
	FullScopeComplete bool               `json:"full_scope_complete"`
	Policy            IdentityRef        `json:"policy"`
	Operation         NativeOperation    `json:"operation"`
	Producer          map[string]string  `json:"producer"`
	ExecutionStatus   string             `json:"execution_status"`
	Assessment        string             `json:"assessment"`
	Cases             []string           `json:"cases"`
	Members           []OrdinaryMember   `json:"members"`
	Batches           []OrdinaryRunBatch `json:"batches"`
	Complete          bool               `json:"complete"`
}

type ordinaryBatchCapture struct {
	Schema  string       `json:"schema"`
	Owner   string       `json:"owner"`
	Batch   int          `json:"batch"`
	Process ReplayMember `json:"process"`
	Stderr  ReplayMember `json:"stderr"`
	Frames  []struct {
		Index  int                  `json:"index"`
		Case   string               `json:"case_id"`
		State  string               `json:"state"`
		Stdin  *ordinaryCaptureSpan `json:"stdin"`
		Stdout *ordinaryCaptureSpan `json:"stdout"`
	} `json:"frames"`
}

type ordinaryCaptureSpan struct {
	Offset   uint64         `json:"offset"`
	Bytes    uint64         `json:"bytes"`
	SHA256   string         `json:"sha256"`
	Complete bool           `json:"complete"`
	Members  []ReplayMember `json:"members"`
}

func ordinaryFinalComplete(data []byte) *Error {
	v, e := decodeStrict("ordinary", data, MaxDocumentBytes)
	if e != nil {
		return e
	}
	if v.kind != '{' || len(v.keys) == 0 || v.keys[len(v.keys)-1] != "complete" || v.vals[len(v.vals)-1].kind != 't' {
		return fail(KindInvalidInput, "SET_INCOMPLETE", "ordinary", nil)
	}
	return nil
}

func parseOrdinaryRecord(data []byte) (OrdinaryOracleRecord, error) {
	var rec OrdinaryOracleRecord
	if e := ordinaryFinalComplete(data); e != nil {
		return rec, e
	}
	if err := ordinaryDecode(data, &rec); err != nil {
		return rec, err
	}
	if rec.Schema != OracleOrdinaryRecordSchema || rec.Case == "" || rec.Workload == "" || !rec.Complete || rec.BaseProjection.Role != "projection" || !ordinaryReplayPinValid(rec.BaseProjection) || rec.QuerySemantics == nil || rec.QuerySemantics.Phase != "PRODUCER_LOCAL" || rec.QuerySemantics.Claim != "BLOCKED" || rec.QuerySemantics.Code != "EXTERNAL_APPROVAL_PENDING" {
		return rec, ordinaryBad("ORDINARY_RECORD")
	}
	if rec.Transport == nil || rec.StepFacts == nil || !jsonv1.Valid(rec.Transport) || !jsonv1.Valid(rec.StepFacts) {
		return rec, ordinaryBad("ORDINARY_RECORD")
	}
	return rec, nil
}

func parseOrdinaryRunBinding(data []byte) (OrdinaryRunBinding, error) {
	var b OrdinaryRunBinding
	if err := ordinaryDecode(data, &b); err != nil {
		return b, err
	}
	if b.Schema != OracleOrdinaryBindingSchema || b.Context != OrdinaryContext || b.ProducerOwner != "O" || b.RunID == "" || b.Candidate == "" || b.SemanticCohort == "" || b.ShardIDs == nil || len(b.ShardIDs) == 0 || b.FullScopeComplete || b.Members == nil || b.Batches == nil {
		return b, ordinaryBad("ORDINARY_BINDING")
	}
	seenShard := map[string]bool{}
	for _, id := range b.ShardIDs {
		if id == "" || seenShard[id] {
			return b, ordinaryBad("ORDINARY_BINDING_SHARDS")
		}
		seenShard[id] = true
	}
	for _, p := range []OrdinaryPin{b.Profile, b.PlanningProfile, b.Registration, b.Assets, b.Plan} {
		if !ordinaryPin(p) {
			return b, ordinaryBad("ORDINARY_BINDING_PIN")
		}
	}
	return b, nil
}

func parseOrdinaryManifest(data []byte) (OrdinaryOracleManifest, error) {
	var m OrdinaryOracleManifest
	if e := ordinaryFinalComplete(data); e != nil {
		return m, e
	}
	if err := ordinaryDecode(data, &m, jsonv1.FormatDurationAsNano(true)); err != nil {
		return m, err
	}
	if m.Schema != OracleOrdinaryManifestSchema || m.Context != OrdinaryContext || !ordinaryClosure(m.ClosureEncoding) || m.ProducerOwner != "O" || m.RunID == "" || m.Candidate == "" || m.SemanticCohort == "" || m.Cases == nil || m.ShardIDs == nil || len(m.ShardIDs) == 0 || len(m.ShardIDs) != len(m.Cases) || m.FullScopeComplete || m.Members == nil || m.Batches == nil || m.Producer == nil || !m.Complete {
		return m, ordinaryBad("ORDINARY_MANIFEST")
	}
	seenShard := map[string]bool{}
	for i, id := range m.ShardIDs {
		if id == "" || seenShard[id] || m.Cases[i] == "" {
			return m, ordinaryBad("ORDINARY_MANIFEST_SHARDS")
		}
		seenShard[id] = true
	}
	for _, p := range []OrdinaryPin{m.Profile, m.PlanningProfile, m.Registration, m.Assets, m.Plan} {
		if !ordinaryPin(p) {
			return m, ordinaryBad("ORDINARY_MANIFEST_PIN")
		}
	}
	if !reflect.DeepEqual(m.Operation, NativeOperations()["native-query-batch"]) || m.Assessment == AssessPass || m.Policy != NativePolicy(NativeOperations()["native-query-batch"], OutputTree, nil) ||
		len(m.Cases) > m.Operation.MaxCases || len(m.Batches) > m.Operation.MaxCases ||
		!slices.Contains([]string{"COMPLETED", "FAILED", "CANCELLED", "RESOURCE_LIMIT"}, m.ExecutionStatus) ||
		!slices.Contains([]string{AssessPass, AssessFail, AssessBlocked, AssessNotAssessed}, m.Assessment) {
		return m, ordinaryBad("ORDINARY_MANIFEST_CONTEXT")
	}
	return m, nil
}

// verifyOrdinaryOracleSet checks the r8 storage graph and physical pins. It does not run the
// ordinary semantic reader or turn producer-local pending status into an approved result.
func verifyOrdinaryOracleSet(fsys fs.FS, manifestBytes []byte) OracleSetReport {
	r := OracleSetReport{ExecutionStatus: StatusNotRun, Assessment: AssessNotAssessed, Findings: []Finding{}}
	add := func(code, p string) {
		r.Findings = append(r.Findings, Finding{Code: code, Severity: "error", Path: p, Message: "ordinary record set 구조가 일치하지 않는다"})
	}
	addDecode := func(err error, p string) {
		var ke *Error
		if errors.As(err, &ke) {
			add(ke.Code, p)
			return
		}
		add("ORDINARY_DECODE_FAILED", p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	defer cancel()
	run, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		add(e.Code, "manifest.json")
		return r
	}
	defer run.cancel()
	b := &eligibilityBudget{r: run}
	lim := ReplayOperations()["evidence-replay"]
	run.readCeiling = lim.TotalBytes
	if e := run.chargeRead(uint64(len(manifestBytes)), lim.TotalBytes, "ordinary-manifest"); e != nil {
		add(e.Code, "manifest.json")
		return r
	}
	if e := b.charge(1, uint64(len(manifestBytes)), "ordinary-manifest-read"); e != nil {
		add(e.Code, "manifest.json")
		return r
	}
	if e := b.reserveJSON("ordinary-manifest", manifestBytes, 4); e != nil {
		add(e.Code, "manifest.json")
		return r
	}
	manifest, err := parseOrdinaryManifest(manifestBytes)
	if err != nil {
		addDecode(err, "manifest.json")
		return r
	}
	r.ExecutionStatus, r.Assessment = manifest.ExecutionStatus, manifest.Assessment
	if len(manifest.Members)+1 > int(lim.Files) {
		add("ORDINARY_SET_LIMIT", "manifest.json")
		return r
	}
	listed := map[string]bool{"manifest.json": true}
	memberByPath := map[string]OrdinaryMember{}
	var total uint64 = uint64(len(manifestBytes))
	ownerRole := func(m OrdinaryMember) bool {
		switch m.Role {
		case "native-batch-capture", "process", "stderr", "stdin-frame", "stdout-frame", "native-request", "native-response":
			return strings.HasPrefix(m.Owner, "D_batch_") && m.CaseOrBatch == m.Owner
		case "driver", "native-driver", "grammar", "reference-library", "build-control", "runtime", "build", "build-manifest", "build-input", "grammar-parser", "grammar-scanner", "grammar-header", "grammar-paged-descriptor", "grammar-paged-part", "driver-paged-descriptor", "driver-paged-part", "library-paged-descriptor", "library-paged-part":
			return m.Owner == "D_build" && m.CaseOrBatch == "build"
		default:
			return m.Owner == "O" && m.CaseOrBatch != ""
		}
	}
	for _, mem := range manifest.Members {
		key := strings.ToLower(mem.Path)
		if !ordinaryPath(mem.Path) || mem.Pin.Path != mem.Path || !ordinaryPin(mem.Pin) || listed[key] || !ownerRole(mem) || !ordinaryMemberRole(mem.Role, manifest.ClosureEncoding) || mem.Pin.Bytes > lim.FileBytes || total > lim.TotalBytes-mem.Pin.Bytes {
			add("ORDINARY_MEMBER_INVALID", mem.Path)
			continue
		}
		listed[key] = true
		memberByPath[mem.Path] = mem
		if readErr := verifyOrdinaryMember(fsys, mem.Pin, b, lim); readErr != nil {
			add("ORDINARY_MEMBER_MISMATCH", mem.Path)
			continue
		}
		total += mem.Pin.Bytes
	}
	if total > lim.TotalBytes {
		add("ORDINARY_SET_LIMIT", "manifest.json")
	}
	caseSet := map[string]bool{}
	for _, c := range manifest.Cases {
		if c == "" || caseSet[c] {
			add("ORDINARY_CASE_DUPLICATE", "manifest.json")
		}
		caseSet[c] = true
	}
	if e := b.charge(uint64(len(manifest.Members))+1, 0, "ordinary-member-index"); e != nil {
		add(e.Code, "manifest.json")
		return r
	}
	rawByCase := map[string][]OrdinaryMember{}
	recordCountByCase := map[string]int{}
	for _, mem := range manifest.Members {
		if mem.Role == "raw" {
			rawByCase[mem.CaseOrBatch] = append(rawByCase[mem.CaseOrBatch], mem)
		}
		if mem.Role == "record" {
			recordCountByCase[mem.CaseOrBatch]++
		}
	}
	caseBatches := map[string]string{}
	type frameBinding struct {
		owner string
		index int
		state string
	}
	frameByCase := map[string]frameBinding{}
	batchOwners := map[string]bool{}
	for i, batch := range manifest.Batches {
		wantOwner := fmt.Sprintf("D_batch_%05d", i)
		if batch.Index != i || batch.Owner != wantOwner || batchOwners[batch.Owner] || batch.Cases == nil || len(batch.Cases) > manifest.Operation.BatchFiles || batch.Capture.Role != "native-batch-capture" || !ordinaryReplayPinValid(batch.Capture) {
			add("ORDINARY_BATCH_INVALID", batch.Capture.Path)
			continue
		}
		batchOwners[batch.Owner] = true
		for _, c := range batch.Cases {
			if !caseSet[c] || caseBatches[c] != "" {
				add("ORDINARY_BATCH_CASE", batch.Capture.Path)
			}
			caseBatches[c] = batch.Owner
		}
		capMember, found := memberByPath[batch.Capture.Path]
		if !found || capMember.Owner != batch.Owner || capMember.CaseOrBatch != batch.Owner || capMember.Role != batch.Capture.Role || !ordinaryReplayPinMatches(capMember, batch.Capture) {
			add("ORDINARY_BATCH_CAPTURE", batch.Capture.Path)
			continue
		}
		capBytes, readErr := readOrdinaryMemberBudget(fsys, capMember.Pin, b)
		if readErr != nil {
			add("ORDINARY_BATCH_CAPTURE", batch.Capture.Path)
			continue
		}
		if e := b.reserveJSON("ordinary-batch-capture", capBytes, 2); e != nil {
			add(e.Code, batch.Capture.Path)
			return r
		}
		var capture ordinaryBatchCapture
		if err := decodeOrdinaryCapture(capBytes, &capture); err != nil || capture.Schema != "tsgk-native-batch-capture/r1" || capture.Owner != batch.Owner || capture.Batch != i || capture.Process.Path == "" || capture.Stderr.Path == "" || len(capture.Frames) != len(batch.Cases) {
			add("ORDINARY_BATCH_CAPTURE", batch.Capture.Path)
			continue
		}
		if !ordinaryCaptureMember(capture.Process, batch.Owner, memberByPath) || !ordinaryCaptureMember(capture.Stderr, batch.Owner, memberByPath) {
			add("ORDINARY_BATCH_OWNER", batch.Capture.Path)
		}
		for j, frame := range capture.Frames {
			frameByCase[frame.Case] = frameBinding{owner: batch.Owner, index: j, state: frame.State}
			if frame.Index != j || frame.Case != batch.Cases[j] || frame.State != "ANSWERED" && frame.State != "SENT" && frame.State != "NOT_SENT" {
				add("ORDINARY_BATCH_FRAME", batch.Capture.Path)
			}
			if frame.State == "ANSWERED" && (frame.Stdout == nil || !frame.Stdout.Complete) || frame.State == "NOT_SENT" && (frame.Stdin != nil || frame.Stdout != nil) {
				add("ORDINARY_BATCH_FRAME", batch.Capture.Path)
			}
			for _, item := range []struct {
				span   *ordinaryCaptureSpan
				stdout bool
			}{{frame.Stdin, false}, {frame.Stdout, true}} {
				span := item.span
				if span == nil {
					continue
				}
				if span.Bytes > lim.FileBytes {
					add("ORDINARY_BATCH_FRAME_SPAN", batch.Capture.Path)
					continue
				}
				var partBytes uint64
				validParts := true
				for _, part := range span.Members {
					if part.Bytes > lim.FileBytes || part.Bytes > span.Bytes || partBytes > span.Bytes-part.Bytes {
						validParts = false
						break
					}
					partBytes += part.Bytes
				}
				if !validParts || partBytes != span.Bytes {
					add("ORDINARY_BATCH_FRAME_SPAN", batch.Capture.Path)
					continue
				}
				if e := b.charge(span.Bytes+1, span.Bytes, "ordinary-frame-span"); e != nil {
					add(e.Code, batch.Capture.Path)
					return r
				}
				joined := make([]byte, 0, int(span.Bytes))
				for _, part := range span.Members {
					if !ordinaryCaptureMember(part, batch.Owner, memberByPath) {
						add("ORDINARY_BATCH_FRAME_MEMBER", batch.Capture.Path)
						continue
					}
					partBytes, partErr := readOrdinaryMemberBudget(fsys, OrdinaryPin{Path: part.Path, Bytes: part.Bytes, SHA256: part.SHA256}, b)
					if partErr != nil {
						add("ORDINARY_BATCH_FRAME_MEMBER", part.Path)
						continue
					}
					joined = append(joined, partBytes...)
				}
				if uint64(len(joined)) != span.Bytes || digestHex(joined) != span.SHA256 {
					add("ORDINARY_BATCH_FRAME_SPAN", batch.Capture.Path)
				}
				if item.stdout && span.Complete && len(joined) >= 4 {
					want := uint64(joined[0])<<24 | uint64(joined[1])<<16 | uint64(joined[2])<<8 | uint64(joined[3])
					if uint64(len(joined)-4) != want {
						add("ORDINARY_BATCH_RESPONSE_FRAME", batch.Capture.Path)
					}
					var raw *OrdinaryMember
					raws := rawByCase[frame.Case]
					if len(raws) == 1 {
						raw = &raws[0]
					}
					rawBytes, rawErr := []byte(nil), error(nil)
					if raw != nil {
						rawBytes, rawErr = readOrdinaryMemberBudget(fsys, raw.Pin, b)
					}
					if raw == nil || rawErr != nil || !slices.Equal(rawBytes, joined[4:]) {
						add("ORDINARY_BATCH_RAW_BINDING", batch.Capture.Path)
					}
				} else if item.stdout && span.Complete {
					add("ORDINARY_BATCH_RESPONSE_FRAME", batch.Capture.Path)
				}
			}
		}
	}
	for _, mem := range manifest.Members {
		if mem.Role == "record" {
			recordBytes, readErr := readOrdinaryMemberBudget(fsys, mem.Pin, b)
			if readErr != nil {
				add("ORDINARY_RECORD_INVALID", mem.Path)
				continue
			}
			if e := b.reserveJSON("ordinary-record", recordBytes, 4); e != nil {
				add(e.Code, mem.Path)
				return r
			}
			rec, recErr := parseOrdinaryRecord(recordBytes)
			if recErr != nil || rec.Case != mem.CaseOrBatch || !caseSet[rec.Case] || mem.Owner != "O" {
				add("ORDINARY_RECORD_INVALID", mem.Path)
				continue
			}
			projection, exists := memberByPath[rec.BaseProjection.Path]
			if !exists || projection.Owner != "O" || projection.CaseOrBatch != rec.Case || projection.Role != "projection" || !ordinaryReplayPinMatches(projection, rec.BaseProjection) {
				add("ORDINARY_RECORD_PROJECTION", mem.Path)
			}
			if rec.QuerySemantics.RegistrationSHA256 != manifest.Registration.SHA256 || rec.QuerySemantics.Projection != rec.BaseProjection {
				add("ORDINARY_RECORD_BINDING", mem.Path)
			}
			transportNull := string(rec.Transport) == "null"
			if transportNull != (rec.QuerySemantics.Capture == nil) {
				add("ORDINARY_RECORD_TRANSPORT", mem.Path)
			}
			if !transportNull {
				if e := b.reserveJSON("ordinary-record-transport", rec.Transport, 2); e != nil {
					add(e.Code, mem.Path)
					return r
				}
				var transport struct {
					Batch int    `json:"batch"`
					Frame int    `json:"frame"`
					State string `json:"state"`
				}
				if jsonv1.Unmarshal(rec.Transport, &transport) != nil {
					add("ORDINARY_RECORD_TRANSPORT", mem.Path)
					continue
				}
				fb, exists := frameByCase[rec.Case]
				if !exists || caseBatches[rec.Case] != fb.owner || transport.Batch < 0 || transport.Batch >= len(manifest.Batches) || manifest.Batches[transport.Batch].Owner != fb.owner || transport.Frame != fb.index || transport.State != fb.state || transport.State == "NOT_SENT" {
					add("ORDINARY_RECORD_TRANSPORT", mem.Path)
				}
				owner := caseBatches[rec.Case]
				captureMember, ok := memberByPath[rec.QuerySemantics.Capture.Path]
				if !ok || owner == "" || captureMember.Owner != owner || captureMember.Role != "native-batch-capture" || !ordinaryReplayPinMatches(captureMember, *rec.QuerySemantics.Capture) {
					add("ORDINARY_RECORD_CAPTURE", mem.Path)
				}
			}
			if len(manifest.Producer["profile_id"]) != 0 && rec.Workload != manifest.Producer["profile_id"] {
				add("ORDINARY_RECORD_WORKLOAD", mem.Path)
			}
		}
	}
	for c := range caseSet {
		if recordCountByCase[c] != 1 {
			add("ORDINARY_RECORD_COUNT", "manifest.json")
		}
	}
	ownerRoles := map[string]map[string]int{}
	caseRoles := map[string]map[string]int{}
	if e := b.charge(uint64(len(manifest.Members))+1, 0, "ordinary-owner-summary"); e != nil {
		add(e.Code, "manifest.json")
		return r
	}
	for _, mem := range manifest.Members {
		if strings.HasPrefix(mem.Owner, "D_batch_") {
			if !batchOwners[mem.Owner] {
				add("ORDINARY_BATCH_OWNER_ORPHAN", mem.Path)
			}
			if ownerRoles[mem.Owner] == nil {
				ownerRoles[mem.Owner] = map[string]int{}
			}
			ownerRoles[mem.Owner][mem.Role]++
		}
		if mem.Owner == "O" && mem.CaseOrBatch != "run" {
			if !caseSet[mem.CaseOrBatch] {
				add("ORDINARY_CASE_MEMBER_OWNER", mem.Path)
			}
			if caseRoles[mem.CaseOrBatch] == nil {
				caseRoles[mem.CaseOrBatch] = map[string]int{}
			}
			caseRoles[mem.CaseOrBatch][mem.Role]++
		}
		if mem.Owner == "O" && mem.CaseOrBatch == "run" && mem.Role != "fact-pack" && mem.Role != "fact-mapping" {
			add("ORDINARY_RUN_MEMBER_ROLE", mem.Path)
		}
	}
	for owner := range batchOwners {
		roles := ownerRoles[owner]
		if roles["native-batch-capture"] != 1 || roles["process"] != 1 || roles["stderr"] != 1 {
			add("ORDINARY_BATCH_OWNER_CARDINALITY", owner)
		}
	}
	for c := range caseSet {
		roles := caseRoles[c]
		if roles["source"]+roles["source-witness"] != 1 || roles["projection"] != 1 {
			add("ORDINARY_CASE_MEMBER_CARDINALITY", c)
		}
	}
	if walkErr := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			add("SET_UNREADABLE", p)
			return nil
		}
		if !d.IsDir() && !listed[strings.ToLower(p)] {
			add("MEMBER_UNLISTED", p)
		}
		return nil
	}); walkErr != nil {
		add("SET_UNREADABLE", ".")
	}
	r.Records = len(manifest.Cases)
	r.Valid = len(r.Findings) == 0
	return r
}

func verifyOrdinaryMember(fsys fs.FS, pin OrdinaryPin, b *eligibilityBudget, lim ReplayLimits) error {
	if !ordinaryPath(pin.Path) || pin.Bytes > lim.FileBytes {
		return fmt.Errorf("invalid ordinary member pin")
	}
	f, err := fsys.Open(path.Clean(pin.Path))
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || uint64(info.Size()) != pin.Bytes {
		return fmt.Errorf("ordinary member size mismatch")
	}
	if _, e := b.r.remainingRead(lim.TotalBytes, "ordinary-member"); e != nil {
		return e
	}
	if e := b.charge(1, 32768+256, "ordinary-member-hash-buffer"); e != nil {
		return e
	}
	if e := b.charge(1, pin.Bytes, "ordinary-member-read"); e != nil {
		return e
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, int64(pin.Bytes)+1))
	if err != nil {
		return err
	}
	if e := b.r.chargeRead(uint64(n), lim.TotalBytes, "ordinary-member"); e != nil {
		return e
	}
	if uint64(n) != pin.Bytes || hex.EncodeToString(h.Sum(nil)) != pin.SHA256 {
		return fmt.Errorf("ordinary member pin mismatch")
	}
	return nil
}

func readOrdinaryMemberBudget(fsys fs.FS, pin OrdinaryPin, b *eligibilityBudget) ([]byte, error) {
	lim := ReplayOperations()["evidence-replay"]
	if !ordinaryPath(pin.Path) || pin.Bytes > lim.FileBytes {
		return nil, fmt.Errorf("invalid ordinary member pin")
	}
	f, err := fsys.Open(path.Clean(pin.Path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || uint64(info.Size()) != pin.Bytes {
		return nil, fmt.Errorf("ordinary member size mismatch")
	}
	if _, e := b.r.remainingRead(lim.TotalBytes, "ordinary-member"); e != nil {
		return nil, e
	}
	if e := b.charge(1, pin.Bytes, "ordinary-member-decode"); e != nil {
		return nil, e
	}
	buffer, e := eligibilityProduct("ordinary-member-buffer", 4, pin.Bytes+1)
	if e != nil {
		return nil, e
	}
	if e := b.charge(0, buffer+32768, "ordinary-member-buffer"); e != nil {
		return nil, e
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(pin.Bytes)+1))
	if err != nil {
		return nil, err
	}
	if e := b.r.chargeRead(uint64(len(data)), lim.TotalBytes, "ordinary-member"); e != nil {
		return nil, e
	}
	if uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
		return nil, fmt.Errorf("ordinary member pin mismatch")
	}
	return data, nil
}

func ordinaryCaptureMember(pin ReplayMember, owner string, members map[string]OrdinaryMember) bool {
	m, ok := members[pin.Path]
	return ok && m.Owner == owner && m.CaseOrBatch == owner && m.Role == pin.Role && m.Pin.Path == pin.Path && m.Pin.Bytes == pin.Bytes && m.Pin.SHA256 == pin.SHA256
}

func ordinaryReplayPinMatches(m OrdinaryMember, p ReplayMember) bool {
	return p.Path == m.Pin.Path && p.Role == m.Role && p.Bytes == m.Pin.Bytes && p.SHA256 == m.Pin.SHA256
}

func ordinaryReplayPinValid(p ReplayMember) bool {
	return p.Role != "" && ordinaryPin(OrdinaryPin{Path: p.Path, Bytes: p.Bytes, SHA256: p.SHA256})
}

func decodeOrdinaryCapture(data []byte, out any) error {
	if _, e := decodeStrict("batch-capture", data, MaxDocumentBytes); e != nil {
		return e
	}
	return jsonv1.Unmarshal(data, out)
}

// ValidateOrdinaryRunBinding checks caller-held r2 bytes against the stored r8 member map.
func ValidateOrdinaryRunBinding(data []byte, manifest OrdinaryOracleManifest) error {
	b, err := parseOrdinaryRunBinding(data)
	if err != nil {
		return err
	}
	if b.Context != manifest.Context || b.Profile != manifest.Profile || b.PlanningProfile != manifest.PlanningProfile || b.Registration != manifest.Registration || b.Assets != manifest.Assets || b.Plan != manifest.Plan || b.Candidate != manifest.Candidate || b.SemanticCohort != manifest.SemanticCohort || b.RunID != manifest.RunID || b.ProducerOwner != manifest.ProducerOwner || b.FullScopeComplete != manifest.FullScopeComplete || !slices.Equal(b.ShardIDs, manifest.ShardIDs) ||
		!slices.EqualFunc(b.Batches, manifest.Batches, func(a, c OrdinaryRunBatch) bool {
			return a.Index == c.Index && a.Owner == c.Owner && a.Capture == c.Capture && slices.Equal(a.Cases, c.Cases)
		}) || !slices.EqualFunc(b.Members, manifest.Members, func(a, c OrdinaryMember) bool { return a == c }) {
		return ordinaryBad("ORDINARY_BINDING_MISMATCH")
	}
	return nil
}
