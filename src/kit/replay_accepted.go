package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func checkAcceptedResultShape(t typed, v *jv) *Error {
	if v == nil {
		return fail(KindInvalidInput, "JSON_MISSING_FIELD", t.doc+"#/reuse_accepted", nil)
	}
	m, e := t.object(v, []string{"schema", "history_sha256", "result", "code", "steps"})
	if e != nil {
		return e
	}
	schema, e := t.str(m["schema"])
	if e != nil {
		return e
	}
	if schema != ReuseAcceptedSchema {
		return t.bad("SCHEMA_UNSUPPORTED", m["schema"])
	}
	if _, e := hexDigest(t, m["history_sha256"]); e != nil {
		return e
	}
	result, e := t.str(m["result"])
	if e != nil {
		return e
	}
	code, e := t.str(m["code"])
	if e != nil {
		return e
	}
	if (result != claimPass && result != claimFail && result != claimBlocked) || (result == claimPass) != (code == "") {
		return t.bad("REUSE_ACCEPTED_SHAPE_INVALID", v)
	}
	steps, e := t.array(m["steps"])
	if e != nil {
		return e
	}
	if len(steps) == 0 || len(steps) > 4 {
		return t.bad("REUSE_ACCEPTED_SHAPE_INVALID", m["steps"])
	}
	previous := uint64(0)
	for _, s := range steps {
		sm, e := t.object(s, []string{"step", "result", "code"})
		if e != nil {
			return e
		}
		k, e := t.uint(sm["step"])
		if e != nil {
			return e
		}
		if k <= previous || k > 4 {
			return t.bad("REUSE_ACCEPTED_SHAPE_INVALID", s)
		}
		previous = k
		result, e := t.str(sm["result"])
		if e != nil {
			return e
		}
		code, e := t.str(sm["code"])
		if e != nil {
			return e
		}
		if (result != claimPass && result != claimFail && result != claimBlocked) || (result == claimPass) != (code == "") {
			return t.bad("REUSE_ACCEPTED_SHAPE_INVALID", s)
		}
	}
	return nil
}

func (x *replayEnv) readAcceptedLedger(prof IncrementalProfile) *Error {
	m, e := x.oneMember("reuse-ledger")
	if e != nil {
		return e
	}
	b, e := x.member(m.Path)
	if e != nil {
		return e
	}
	ledger, err := ParseReuseAcceptedLedger(b)
	if err != nil {
		return fail(KindInvalidInput, "REUSE_ACCEPTED_LEDGER_INVALID", m.Path, err)
	}
	if prof.Preprocess != nil || prof.SvcContext != nil || prof.Declarations != nil || prof.Format != "" || prof.Output != OutputTree || prof.Encoding != EncodingUTF8 ||
		(prof.Operation != "native-parse-edit" && prof.Operation != "native-query") || ledger.CheckProfile(prof) != nil {
		return fail(KindUnsupported, "REUSE_ACCEPTED_PROFILE_MISMATCH", m.Path, nil)
	}
	x.acceptedLedger = ledger
	return nil
}

func (x *replayEnv) acceptedMember(path, role string) ([]byte, *Error) {
	m, ok := x.members[path]
	if !ok {
		return nil, nil
	}
	if m.Role != role {
		return nil, fail(KindInvalidInput, "REUSE_ACCEPTED_MEMBER_ROLE", path, nil)
	}
	b, e := x.member(path)
	if e == nil && b == nil {
		b = []byte{}
	}
	return b, e
}

// The enclosing reader binds files and the actual registered operation before the
// same pure checker used by live execution recomputes the case's proof.
func (x *replayEnv) replayAccepted(c *rcCase, raw []byte, index int, registered *IncrementalCase, top nativeResult) *Error {
	x.currentAccepted = nil
	g := x.gate("reuse-accepted")
	if registered == nil {
		g.fail(c.ID, "REUSE_ACCEPTED_HISTORY_MISMATCH", "등록된 original history가 없다")
		return nil
	}
	if top.ReuseAcceptedPolicy != ReuseAcceptedSchema || top.ReuseAcceptedLedgerSHA256 != ReuseAcceptedLedgerSHA256 {
		g.fail(c.ID, "REUSE_ACCEPTED_POLICY_MISMATCH", "새 policy·ledger 결속이 없다")
	}
	var op NativeOperation
	if err := json.Unmarshal(top.operation, &op); err != nil {
		return fail(KindInvalidInput, "REUSE_ACCEPTED_OPERATION_INVALID", c.ID, err)
	}
	canon, err := json.Marshal(op)
	reg := NativeOperations()[op.Name]
	narrowed := op
	narrowed.RunWall = reg.RunWall
	if err != nil || !bytes.Equal(canon, top.operation) || !reflect.DeepEqual(narrowed, reg) || op.RunWall <= 0 || op.RunWall > reg.RunWall ||
		!slices.Contains([]string{"native-parse-edit", "native-query"}, op.Name) {
		g.fail(c.ID, "REUSE_ACCEPTED_OPERATION_INVALID", "기록된 operation이 등록된 유한 예산과 다르다")
	}
	policy := NativePolicy(op, OutputTree, nil)
	var policies []IdentityRef
	for _, id := range top.Identities {
		if id.Role == "policy" {
			policies = append(policies, id)
		}
	}
	g.check(len(policies) == 1 && policies[0] == policy, c.ID, "REUSE_ACCEPTED_POLICY_MISMATCH", "operation에서 재계산한 policy가 다르다")
	sourceDir, responseRole := "sources/", "response"
	if x.observationWire == nativewire.ProtocolR4 {
		sourceDir, responseRole = "source/", "raw"
	}
	source, e := x.acceptedMember(fmt.Sprintf("%s%05d-%s.bin", sourceDir, index, c.ID), "case-source")
	if e != nil {
		return e
	}
	responsePath := fmt.Sprintf("%s%05d-%s.json", x.responseDir, index, c.ID)
	if x.responseDir == "" {
		responsePath = fmt.Sprintf("responses/%05d-%s.json", index, c.ID)
	}
	response, e := x.acceptedMember(responsePath, responseRole)
	if e != nil {
		return e
	}
	if response != nil {
		x.observationResponses[responsePath] = true
	}
	expected := nativewire.Expect{Revision: nativewire.ProtocolR3, ID: c.ID, Nodes: reg.Nodes, SourceBytes: registered.Input.Bytes, Check: func() error {
		if e := x.r.check(); e != nil {
			return e
		}
		return nil
	}}
	if x.observationWire != "" {
		expected.Revision = x.observationWire
	}
	if top.Build != nil {
		expected.Observation = &nativewire.Binding{Manifest: reuseobs.RuntimeManifestSHA256, RunID: top.ObservationRunID, ProducerSHA256: top.Build.Identity}
	}
	if source != nil {
		versions, points, err := ApplyEdits(registered.Encoding, source, registered.Edits, reg.InputBytes)
		if err != nil {
			return fail(KindInvalidInput, "REUSE_ACCEPTED_SOURCE_INVALID", c.ID, err)
		}
		expected.EditPoints = points
		for _, b := range versions {
			expected.Sources = append(expected.Sources, nativewire.SourceID{Bytes: uint64(len(b)), SHA256: digestHex(b)})
		}
		for _, ed := range registered.Edits {
			expected.Edits = append(expected.Edits, nativewire.EditSpan{StartByte: ed.StartByte, OldEndByte: ed.OldEndByte, NewEndByte: ed.NewEndByte})
		}
	}
	var process *struct {
		ExitCode int `json:"exit_code"`
	}
	if json.Unmarshal(c.Process, &process) == nil && process != nil {
		expected.Exit = &process.ExitCode
	}
	req := ReuseAcceptedRequest{Ledger: x.acceptedLedger, Route: top.Route, Case: *registered, Source: source, Raw: response, Projection: raw, Expected: expected, Policy: policy, Platform: x.prof.Subject.Platform, RecordedPlatform: top.Platform}
	proof, err := JudgeReuseAccepted(req)
	x.currentAccepted = &proof
	if x.acceptedConsumer != nil {
		consumerErr := x.acceptedConsumer(req, proof, err)
		if err == nil && consumerErr != nil {
			return asError(consumerErr)
		}
	}
	if err != nil {
		return asError(err)
	}
	var stored ReuseAcceptedResult
	v, e := decodeBounded("reuse-accepted-case", c.ReuseAccepted, MaxDocumentBytes, &valueBudget{max: 2000000, check: x.r.check})
	if e != nil {
		return e
	}
	if e := checkAcceptedResultShape(typed{doc: "reuse-accepted-case"}, v); e != nil {
		return e
	}
	if err := json.Unmarshal(c.ReuseAccepted, &stored); err != nil {
		return fail(KindInvalidInput, "REUSE_ACCEPTED_PROJECTION_INVALID", c.ID, err)
	}
	g.check(reflect.DeepEqual(stored, proof), c.ID, "REUSE_ACCEPTED_CLAIM_MISMATCH", "저장된 accepted 판정이 재계산과 다르다")
	claim, _ := mandatoryObservations(c)
	var storedClaim string
	g.check(json.Unmarshal(c.ReuseObservationClaim, &storedClaim) == nil && claim == storedClaim, c.ID, "OBSERVATION_CLAIM_MISMATCH", "기존 mandatory 관측 판정이 다르다")
	return nil
}
