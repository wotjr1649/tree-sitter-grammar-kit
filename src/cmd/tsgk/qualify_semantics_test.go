package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Resolve the host temp alias before tests introduce their own path aliases.
func qualifySemanticTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestQualifySemanticCLIAdmissionAndTypedOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "src/contracts/qualification-c1.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir, host := qualifySemanticTempDir(t), qualifySemanticTempDir(t)
	plan, err := json.Marshal(map[string]any{"schema": kit.QualificationSemanticsPlanSchema, "id": "cli-probes", "inventory_sha256": fmt.Sprintf("%x", sha256.Sum256(data)), "context": "full-input-utf8/r2", "scope": "small-full-input-predicate-probes/r1", "targets": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{"inventory.json": data, "plan.json": plan, "input.json": []byte(`{"schema":"tsgk-qualification-query-semantics-input/r1","plan":"plan.json","targets":[]}`)} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	candidate := strings.Repeat("a", 40)
	args := []string{"qualify", "--inventory", filepath.Join(dir, "inventory.json"), "--candidate", candidate, "--host", "linux-amd64=" + host, "--query-semantics", filepath.Join(dir, "input.json")}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), args, &stdout, &stderr); code != exitFail {
		t.Fatalf("missing baseline must remain FAIL: exit=%d stderr=%s", code, &stderr)
	}
	api, err := kit.Qualify(ctx, kit.QualifyRequest{Inventory: data, Candidate: candidate, Hosts: []kit.QualifyHost{{Platform: "linux-amd64", Root: host}}, QuerySemantics: &kit.QualificationQuerySemanticsRequest{Plan: plan, Targets: []kit.QualificationQuerySemanticsInput{}}})
	want, marshalErr := json.Marshal(api)
	if err != nil || marshalErr != nil || strings.TrimSpace(stdout.String()) != string(want) || api.ResultSchema != kit.QualificationSemanticsResultSchema || api.QuerySemantics == nil || api.QuerySemantics.Targets[0].Tuple != nil {
		t.Fatalf("CLI/API semantic result differs: err=%v marshal=%v stderr=%s", err, marshalErr, &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	partial := kit.QualificationResult{Report: kit.Report{ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessBlocked}, ResultSchema: kit.QualificationSemanticsResultSchema}
	if code := finish(partial, nil, "", host, &stdout, &stderr); code != exitBlocked || !strings.Contains(stdout.String(), `"assessment":"BLOCKED"`) {
		t.Fatalf("partial semantic output exit=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	args[len(args)-1] = ""
	if code := run(ctx, args, &stdout, &stderr); code != exitUsage || stdout.Len() != 0 || !strings.Contains(stderr.String(), "SEMANTIC_MANIFEST_PATH_REQUIRED") {
		t.Fatalf("explicit empty opt-in fell back: exit=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}

func TestQualifySemanticFileAliasUsesIdentityBeyondLinkCount(t *testing.T) {
	path := qualifySemanticTempDir(t)
	if err := os.WriteFile(filepath.Join(path, "bundle-member.json"), []byte("approval"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := openQualifySemanticRoot(path, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	defer root.root.Close()
	file, info, err := openQualifySemanticRegular(root.root, "bundle-member.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	// A bind alias retains this opened identity and nlink=1, even at an outside path.
	docs := []*qualifySemanticDocument{{info: info}}
	if err := rejectQualifySemanticFileAliases(docs, []qualifySemanticRoot{root}, time.Now().Add(time.Minute)); err == nil {
		t.Fatal("a matching regular-file identity was accepted as an independent trust document")
	}
	if err := rejectQualifySemanticFileAliases(docs, []qualifySemanticRoot{root}, time.Now().Add(-time.Second)); !errors.Is(err, errQualifyAdmissionDeadline) {
		t.Fatal("metadata scan ignored the admission deadline", err)
	}
}

func TestQualifySemanticPublicationUsesHeldRootAfterRename(t *testing.T) {
	base := qualifySemanticTempDir(t)
	original, moved := filepath.Join(base, "bundle"), filepath.Join(base, "moved")
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(original)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	active := moved
	if err := os.Rename(original, moved); err != nil {
		if runtime.GOOS != "windows" || !(os.IsPermission(err) || errors.Is(err, syscall.Errno(32))) {
			t.Fatal(err)
		}
		t.Log("rename_succeeded=false; Windows denied namespace replacement while the root handle was held")
		active = original
	} else if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	} else {
		t.Log("rename_succeeded=true")
	}
	out := filepath.Join(active, "result.json")
	code, err := publishAgainst(out, []byte("result"), []string{original}, []*os.Root{root}, time.Now().Add(time.Minute), context.Background())
	if err == nil || code != "OUTPUT_INSIDE_INPUT" {
		t.Fatalf("publication lost the original opened root: code=%s err=%v", code, err)
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		t.Fatal("a result was written inside the consumed bundle", err)
	}
}

func TestQualifySemanticsManifestStrictShapeAndCardinality(t *testing.T) {
	valid := `{"schema":"tsgk-qualification-query-semantics-input/r1","plan":"plan.json","targets":[{"id":"linux","root":"bundle","profile":"profile.json"}]}`
	input, approvals, contexts, err := parseQualifySemanticsManifest([]byte(valid))
	if err != nil || len(*input.Targets) != 1 || approvals[0] != nil || contexts[0] != nil {
		t.Fatalf("valid manifest: input=%+v approvals=%v contexts=%v err=%v", input, approvals, contexts, err)
	}
	for name, data := range map[string]string{
		"duplicate key":   `{"schema":"tsgk-qualification-query-semantics-input/r1","schema":"tsgk-qualification-query-semantics-input/r1","plan":"p","targets":[]}`,
		"unknown key":     `{"schema":"tsgk-qualification-query-semantics-input/r1","plan":"p","targets":[],"fallback":"producer"}`,
		"three targets":   `{"schema":"tsgk-qualification-query-semantics-input/r1","plan":"p","targets":[{"id":"a","root":"a","profile":"a"},{"id":"b","root":"b","profile":"b"},{"id":"c","root":"c","profile":"c"}]}`,
		"null path":       `{"schema":"tsgk-qualification-query-semantics-input/r1","plan":"p","targets":[{"id":"a","root":"a","profile":"a","approval":null}]}`,
		"null pin opt-in": `{"schema":"tsgk-qualification-query-semantics-input/r1","plan":"p","targets":[{"id":"a","root":"a","profile":"a","pins":null}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := parseQualifySemanticsManifest([]byte(data)); err == nil {
				t.Fatal("invalid manifest was accepted")
			}
		})
	}
}

func TestQualifySemanticsPinsManifestAndForwarding(t *testing.T) {
	legacy := `{"schema":"tsgk-qualification-query-semantics-input/r1","plan":"plan.json","targets":[{"id":"linux","root":"bundle","profile":"profile.json"}]}`
	if _, _, _, err := parseQualifySemanticsManifest([]byte(legacy)); err != nil {
		t.Fatalf("legacy input schema changed: %v", err)
	}
	withPins := `{"schema":"tsgk-qualification-query-semantics-input/r2","plan":"plan.json","targets":[{"id":"linux","root":"bundle","profile":"profile.json","approval":"approval.json","pins":{"approved_approval_sha256":"a","approval_base64":"e30=","approved_pin_set_sha256":"b","pin_set_base64":"e30=","materials_base64":{"reference/catalogue.json":"e30="}}}]}`
	input, approvals, _, err := parseQualifySemanticsManifest([]byte(withPins))
	if err != nil || input.Targets == nil || (*input.Targets)[0].Pins == nil || approvals[0] == nil || *approvals[0] != "approval.json" {
		t.Fatalf("pin input schema: input=%+v approvals=%v err=%v", input, approvals, err)
	}
	for name, data := range map[string]string{
		"pins require r2":      strings.Replace(withPins, "input/r2", "input/r1", 1),
		"pins require subject": strings.Replace(withPins, `,"approval":"approval.json"`, "", 1),
		"r2 requires pins":     `{"schema":"tsgk-qualification-query-semantics-input/r2","plan":"plan.json","targets":[{"id":"linux","root":"bundle","profile":"profile.json"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := parseQualifySemanticsManifest([]byte(data)); err == nil {
				t.Fatal("invalid pin manifest was accepted")
			}
		})
	}

	approval := []byte("{ \"schema\": \"approval\" }\r\n")
	set := []byte("{\"schema\":\"set\"}\n")
	material := []byte{0, 1, 2, 0xff}
	bundle := &qualifySemanticsPins{ApprovedApprovalSHA256: strings.Repeat("a", 64),
		ApprovalBase64: base64.StdEncoding.EncodeToString(approval), ApprovedPinSetSHA256: strings.Repeat("b", 64),
		PinSetBase64: base64.StdEncoding.EncodeToString(set), MaterialsBase64: map[string]string{"reference/catalogue.json": base64.StdEncoding.EncodeToString(material)}}
	subject := []byte("eligibility approval original bytes")
	request, err := decodeQualifySemanticPins(bundle, subject)
	if err != nil || request == nil || !bytes.Equal(request.Approval, approval) || !bytes.Equal(request.PinSet, set) || !bytes.Equal(request.Subject, subject) || !bytes.Equal(request.Materials["reference/catalogue.json"], material) || request.ApprovedApprovalSHA256 != bundle.ApprovedApprovalSHA256 || request.ApprovedPinSetSHA256 != bundle.ApprovedPinSetSHA256 {
		t.Fatalf("pin request forwarding changed bytes: request=%+v err=%v", request, err)
	}

	target := qualifySemanticsManifestTarget{ID: "linux", Pins: bundle}
	profileDoc := &qualifySemanticDocument{data: []byte("profile")}
	approvalDoc := &qualifySemanticDocument{data: subject}
	entry, err := makeQualifySemanticInput(target, qualifySemanticTargetDocuments{profile: profileDoc, approval: approvalDoc}, qualifySemanticRoot{path: "bundle"})
	if err != nil || entry.Pins == nil || !bytes.Equal(entry.Pins.Subject, subject) || !bytes.Equal(entry.Pins.Approval, approval) || !bytes.Equal(entry.Replay.Profile, profileDoc.data) {
		t.Fatalf("qualification entry did not forward the pin request: entry=%+v err=%v", entry, err)
	}

	bundle.ApprovalBase64 = "not base64"
	if _, err := decodeQualifySemanticPins(bundle, subject); err == nil {
		t.Fatal("malformed inline bytes accepted")
	}
	bundle.ApprovalBase64 = base64.StdEncoding.EncodeToString(approval)
	if _, err := decodeQualifySemanticPins(bundle, nil); err == nil {
		t.Fatal("pin request without its external eligibility subject accepted")
	}
	tooMany := *bundle
	tooMany.MaterialsBase64 = make(map[string]string, kit.ReplayOperations()["evidence-replay"].Files+1)
	for i := uint64(0); i <= kit.ReplayOperations()["evidence-replay"].Files; i++ {
		tooMany.MaterialsBase64[fmt.Sprintf("material-%d", i)] = ""
	}
	if _, err := decodeQualifySemanticPins(&tooMany, subject); err == nil {
		t.Fatal("over-limit pin material map accepted")
	}
}

func TestQualifySemanticsPinsForwardingRetainsIndependentApproval(t *testing.T) {
	subject := []byte(`{"eligibility":"caller-approved"}`)
	material := []byte("historical reference catalogue")
	materialSHA := fmt.Sprintf("%x", sha256.Sum256(material))
	pinSet, err := json.Marshal(struct {
		Schema    string                    `json:"schema"`
		Reader    string                    `json:"reader"`
		Materials []kit.EvidencePinMaterial `json:"materials"`
	}{kit.EvidencePinSetSchema, kit.EvidencePinReader, []kit.EvidencePinMaterial{{Name: "reference/catalogue.json", Role: "reference-catalogue", Bytes: uint64(len(material)), SHA256: materialSHA}}})
	if err != nil {
		t.Fatal(err)
	}
	pinSetSHA := fmt.Sprintf("%x", sha256.Sum256(pinSet))
	approval, err := json.Marshal(struct {
		Schema        string `json:"schema"`
		PinSetSHA256  string `json:"pin_set_sha256"`
		SubjectSHA256 string `json:"subject_sha256"`
		Reader        string `json:"reader"`
		Scope         string `json:"scope"`
	}{kit.EvidencePinApprovalSchema, pinSetSHA, fmt.Sprintf("%x", sha256.Sum256(subject)), kit.EvidencePinReader, kit.EvidencePinScope})
	if err != nil {
		t.Fatal(err)
	}
	approvalSHA := fmt.Sprintf("%x", sha256.Sum256(approval))
	bundle := &qualifySemanticsPins{ApprovedApprovalSHA256: approvalSHA, ApprovalBase64: base64.StdEncoding.EncodeToString(approval),
		ApprovedPinSetSHA256: pinSetSHA, PinSetBase64: base64.StdEncoding.EncodeToString(pinSet),
		MaterialsBase64: map[string]string{"reference/catalogue.json": base64.StdEncoding.EncodeToString(material)}}
	target := qualifySemanticsManifestTarget{ID: "linux", Approval: json.RawMessage(`"approval.json"`), Pins: bundle}
	entry, err := makeQualifySemanticInput(target, qualifySemanticTargetDocuments{profile: &qualifySemanticDocument{data: []byte("profile")}, approval: &qualifySemanticDocument{data: subject}}, qualifySemanticRoot{path: "bundle"})
	if err != nil || entry.Pins == nil {
		t.Fatalf("pin request was not forwarded: entry=%+v err=%v", entry, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	got, err := kit.ResolveEvidencePins(ctx, *entry.Pins)
	if err != nil || got.Assessment != kit.AssessPass || !got.Complete || got.SubjectSHA256 != fmt.Sprintf("%x", sha256.Sum256(subject)) {
		t.Fatalf("CLI forwarded material without its external approval identity: result=%+v err=%v", got, err)
	}
	forged := *entry.Pins
	forged.ApprovedPinSetSHA256 = strings.Repeat("0", 64)
	got, err = kit.ResolveEvidencePins(ctx, forged)
	if err != nil || got.Assessment != kit.AssessFail {
		t.Fatalf("CLI forwarding accepted an unapproved pin-set digest: result=%+v err=%v", got, err)
	}
}

type countingQualifyReader struct {
	reader io.Reader
	calls  int
	max    int
}

func (r *countingQualifyReader) Read(p []byte) (int, error) {
	r.calls++
	r.max = max(r.max, len(p))
	return r.reader.Read(p)
}

type partialErrorQualifyReader struct{ calls int }

func (r *partialErrorQualifyReader) Read(p []byte) (int, error) {
	r.calls++
	copy(p, "xy")
	return 2, errors.New("partial read failed")
}

func TestQualifySemanticPrefixCountsPartialBytesAndDoesNotProbe(t *testing.T) {
	exact := &countingQualifyReader{reader: strings.NewReader("four")}
	data, n, err := readQualifySemanticPrefix(exact, 4, 4, 4, time.Time{})
	if err != nil || n != 4 || string(data) != "four" || exact.calls != 1 || exact.max != 4 {
		t.Fatalf("exact limit: data=%q bytes=%d calls=%d maxRequest=%d err=%v", data, n, exact.calls, exact.max, err)
	}
	partial := &partialErrorQualifyReader{}
	data, n, err = readQualifySemanticPrefix(partial, 4, 4, 4, time.Time{})
	if err == nil || n != 2 || string(data) != "xy" || partial.calls != 1 {
		t.Fatalf("partial failure: data=%q bytes=%d calls=%d err=%v", data, n, partial.calls, err)
	}
	tooLarge := &countingQualifyReader{reader: strings.NewReader("never read")}
	if _, n, err := readQualifySemanticPrefix(tooLarge, 5, 4, 5, time.Time{}); err == nil || n != 0 || tooLarge.calls != 0 {
		t.Fatalf("over-limit file was read: bytes=%d calls=%d err=%v", n, tooLarge.calls, err)
	}
}

func TestQualifySemanticCallerDocumentsUseOpenedLeafAndStayOutsideRoots(t *testing.T) {
	protectedPath := qualifySemanticTempDir(t)
	protected, err := openQualifySemanticRoot(protectedPath, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	defer protected.root.Close()
	outside := qualifySemanticTempDir(t)
	profile := filepath.Join(outside, "profile.json")
	if err := os.WriteFile(profile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := openQualifySemanticDocument(profile, []qualifySemanticRoot{protected}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("outside regular file rejected: %v", err)
	}
	doc.file.Close()
	doc.parent.Close()

	insidePath := filepath.Join(protectedPath, "inside.json")
	if err := os.WriteFile(insidePath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openQualifySemanticDocument(insidePath, []qualifySemanticRoot{protected}, time.Now().Add(time.Minute)); err == nil {
		t.Fatal("document under a protected root was accepted")
	}

	hardlink := filepath.Join(outside, "linked.json")
	if err := os.Link(profile, hardlink); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
	if _, err := openQualifySemanticDocument(hardlink, []qualifySemanticRoot{protected}, time.Now().Add(time.Minute)); err == nil {
		t.Fatal("hard-linked trust document was accepted")
	}

	leaf := filepath.Join(outside, "single-link.json")
	if err := os.WriteFile(leaf, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	leafDoc, err := openQualifySemanticDocument(leaf, []qualifySemanticRoot{protected}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("single-link regular file rejected: %v", err)
	}
	leafDoc.file.Close()
	leafDoc.parent.Close()
	symlink := filepath.Join(outside, "symlink.json")
	if err := os.Symlink(filepath.Base(leaf), symlink); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if _, err := openQualifySemanticDocument(symlink, []qualifySemanticRoot{protected}, time.Now().Add(time.Minute)); err == nil {
		t.Fatal("linked trust document leaf was accepted")
	}
}

func TestQualifySemanticOutputStaysOutsideBundleRoot(t *testing.T) {
	host, bundle := qualifySemanticTempDir(t), qualifySemanticTempDir(t)
	out := filepath.Join(bundle, "result.json")
	if code, err := publish(out, host, []byte("result"), bundle); code != "OUTPUT_INSIDE_INPUT" || err == nil {
		t.Fatalf("output inside semantic bundle was accepted: code=%s err=%v", code, err)
	}
}

func TestQualifySemanticTrustPathRejectsAliasedParent(t *testing.T) {
	base, outside := qualifySemanticTempDir(t), qualifySemanticTempDir(t)
	profile := filepath.Join(outside, "profile.json")
	if err := os.WriteFile(profile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := openQualifySemanticDocument(profile, nil, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("physical parent rejected before adding an alias: %v", err)
	}
	doc.file.Close()
	doc.parent.Close()
	alias := filepath.Join(base, "linked-parent")
	if err := os.Symlink(outside, alias); err != nil {
		t.Skipf("directory symlinks are unavailable: %v", err)
	}
	if _, err := openQualifySemanticDocument(filepath.Join(alias, "profile.json"), nil, time.Now().Add(time.Minute)); err == nil {
		t.Fatal("trust document through an aliased parent was accepted")
	}
}

func TestConvertQualifyCLIWallPreservesCallerCancellationAndKnownFailure(t *testing.T) {
	wallCtx, cancelWall := context.WithCancelCause(context.Background())
	cancelWall(errQualifyCLIWall)
	caller := context.Background()
	original := &kit.Error{Kind: kit.KindCancelled, Code: "CANCELLED", Cause: context.DeadlineExceeded}
	result := kit.QualificationResult{Report: kit.Report{ExecutionStatus: kit.StatusCancelled, Assessment: kit.AssessFail,
		Findings: []kit.Finding{{Code: "CANCELLED"}}}, QuerySemantics: &kit.QualQuerySemanticsAxis{Code: "SEMANTIC_FIRST_FAILURE"}}
	converted, err := convertQualifyCLIWall(caller, wallCtx, result, original)
	var typed *kit.Error
	if !errors.As(err, &typed) || typed.Kind != kit.KindResourceLimit || typed.Code != "WALL_LIMIT" || converted.ExecutionStatus != kit.StatusResourceLimit || converted.Assessment != kit.AssessFail || converted.QuerySemantics.Code != "SEMANTIC_FIRST_FAILURE" {
		t.Fatalf("own wall conversion lost disposition: result=%+v err=%v", converted, err)
	}

	caller, cancelCaller := context.WithCancel(context.Background())
	cancelCaller()
	unchanged, err := convertQualifyCLIWall(caller, wallCtx, result, original)
	if err != original || unchanged.ExecutionStatus != kit.StatusCancelled || unchanged.Findings[0].Code != "CANCELLED" {
		t.Fatalf("caller cancellation was converted: result=%+v err=%v", unchanged, err)
	}
	for _, interrupted := range []struct {
		result kit.QualificationResult
		err    error
	}{{converted, &kit.Error{Kind: kit.KindResourceLimit, Code: "WALL_LIMIT"}}, {unchanged, original}} {
		for _, schema := range []string{kit.QualificationSemanticsResultSchema, kit.QualificationSemanticsPinsResultSchema} {
			interrupted.result.ResultSchema = schema
			var stdout, stderr bytes.Buffer
			if code := finish(interrupted.result, interrupted.err, "", "", &stdout, &stderr); code != exitFail || !strings.Contains(stdout.String(), `"assessment":"FAIL"`) || !strings.Contains(stderr.String(), interrupted.err.(*kit.Error).Kind) {
				t.Fatalf("known semantic failure erased by typed exit: schema=%s code=%d stdout=%s stderr=%s", schema, code, &stdout, &stderr)
			}
		}
	}
	closed, err := os.CreateTemp(qualifySemanticTempDir(t), "closed-stdout")
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	for _, assessment := range []string{kit.AssessFail, kit.AssessBlocked} {
		for _, schema := range []string{kit.QualificationSemanticsResultSchema, kit.QualificationSemanticsPinsResultSchema} {
			result.ResultSchema, result.Assessment = schema, assessment
			var stderr bytes.Buffer
			if code := finish(result, original, "", "", closed, &stderr); code != exitIO || !strings.Contains(stderr.String(), "STDOUT_FAILED") {
				t.Fatalf("partial semantic report write failure: schema=%s exit=%d stderr=%s", schema, code, &stderr)
			}
		}
	}
}

func TestQualifySemanticEmissionChecksExpiredDeadline(t *testing.T) {
	for _, cancelCaller := range []bool{false, true} {
		caller, cancel := context.WithCancel(context.Background())
		if cancelCaller {
			cancel()
		}
		res := kit.QualificationResult{Report: kit.Report{ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessBlocked}, ResultSchema: kit.QualificationSemanticsResultSchema, QuerySemantics: &kit.QualQuerySemanticsAxis{Code: "ORIGINAL_CODE"}, Explanation: []string{"obsolete completed explanation"}}
		deadline := time.Now().Add(-time.Second)
		if cancelCaller {
			deadline = time.Now().Add(time.Minute)
		}
		var stdout, stderr bytes.Buffer
		code := finishWithRoots(res, nil, "", "", &stdout, &stderr, nil, nil, deadline, caller)
		cancel()
		var got kit.QualificationResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		wantCode, wantStatus := exitBlocked, kit.StatusResourceLimit
		if cancelCaller {
			wantCode, wantStatus = exitCanceled, kit.StatusCancelled
		}
		if code != wantCode || got.ExecutionStatus != wantStatus || got.QuerySemantics.Code != "ORIGINAL_CODE" || strings.Contains(stdout.String(), "obsolete completed explanation") {
			t.Fatalf("expired emission: code=%d status=%s stderr=%s", code, got.ExecutionStatus, &stderr)
		}
	}
}

func TestQualifySemanticLegacyPublicationErrorOrder(t *testing.T) {
	root := qualifySemanticTempDir(t)
	out := filepath.Join(root, "result.json")
	code, err := publish(out, root, []byte("result"), filepath.Join(root, "missing-root"))
	if err == nil || code != "OUTPUT_INSIDE_INPUT" {
		t.Fatalf("nil opt-in publication error order changed: code=%s err=%v", code, err)
	}
}

func TestQualifySemanticCancellationDuringPublication(t *testing.T) {
	old := testHookBeforeChildOpen
	defer func() { testHookBeforeChildOpen = old }()
	for _, assessment := range []string{kit.AssessBlocked, kit.AssessFail} {
		path := qualifySemanticTempDir(t)
		if err := os.Mkdir(filepath.Join(path, "child"), 0o700); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(path)
		if err != nil {
			t.Fatal(err)
		}
		caller, cancel := context.WithCancel(context.Background())
		called := false
		var removed error
		testHookBeforeChildOpen = func(_ *os.Root, name string) {
			called = true
			cancel()
			removed = os.Remove(filepath.Join(path, name))
		}
		out := filepath.Join(qualifySemanticTempDir(t), "result.json")
		res := kit.QualificationResult{Report: kit.Report{ExecutionStatus: kit.StatusCompleted, Assessment: assessment}, ResultSchema: kit.QualificationSemanticsResultSchema, QuerySemantics: &kit.QualQuerySemanticsAxis{Code: "ORIGINAL_CODE"}}
		var stdout, stderr bytes.Buffer
		code := finishWithRoots(res, nil, out, "", &stdout, &stderr, nil, []*os.Root{root}, time.Now().Add(time.Minute), caller)
		cancel()
		root.Close()
		want := exitCanceled
		if assessment == kit.AssessFail {
			want = exitFail
		}
		var got kit.QualificationResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !called || removed != nil || code != want || got.ExecutionStatus != kit.StatusCancelled || got.Assessment != assessment || got.QuerySemantics.Code != "ORIGINAL_CODE" {
			t.Fatalf("publication cancellation: called=%t code=%d err=%v stdout=%s stderr=%s", called, code, err, &stdout, &stderr)
		}
		if _, err := os.Lstat(out); !os.IsNotExist(err) {
			t.Fatal("cancelled publication wrote a destination", err)
		}
	}
}
