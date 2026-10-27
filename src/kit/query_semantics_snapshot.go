package kit

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Local names acquire meaning only after graph ownership and physical identity agree.
func (b *eligibilityBudget) semanticSnapshot(parent, child *os.Root, snapshot *evidenceSnapshot, reg semanticRegistration, add func(string, string, string)) (OracleManifest, map[string][]byte, semanticSetState, *Error) {
	var manifest OracleManifest
	state := semanticSetState{producerFailure: add}
	files := map[string][]byte{}
	if snapshot == nil || !snapshot.graphBound {
		return manifest, files, state, fail(KindInvalidInput, "SEMANTICS_GRAPH_UNBOUND", "evidence.json", nil)
	}
	if e := b.semanticSnapshotIdentity(parent, child, snapshot); e != nil {
		return manifest, files, state, e
	}
	plan := map[string]semanticPlannedMember{}
	owners := map[string]string{}
	for _, node := range snapshot.nodes {
		for _, member := range node.Files {
			owners[member.Path] = node.ID
		}
	}
	for _, member := range reg.PlannedMembers {
		if e := b.charge(20*uint64(len(member.Path))+1, 256+16*uint64(len(member.Path)), "semantic-snapshot"); e != nil {
			return manifest, files, state, e
		}
		plan[member.Path] = member
		path := "oracle/" + member.Path
		pin, ok := snapshot.verifiedMembers[path]
		data, cached := snapshot.files[path]
		if !ok || !cached || pin.Role != member.Role || owners[path] == "" || snapshot.invalidNodes[owners[path]] {
			add("SEMANTICS_MEMBER_UNBOUND", path, "등록한 역할과 owner의 동일 pass 검증 bytes가 없다")
			continue
		}
		files[member.Path] = data
	}
	data, ok := files["manifest.json"]
	if !ok || plan["manifest.json"].Owner != "O" || plan["manifest.json"].Role != "manifest" {
		return manifest, files, state, fail(KindInvalidInput, "SEMANTICS_MANIFEST_PLAN", "oracle/manifest.json", nil)
	}
	v, e := b.semanticAST("oracle/manifest.json", data)
	if e != nil {
		return manifest, files, state, e
	}
	significant, sizeErr := b.significantJSONBytes(data)
	if sizeErr != nil {
		return manifest, files, state, sizeErr
	}
	if e := b.charge(8*significant, 16*uint64(len(data)), "semantic-manifest-typed"); e != nil {
		return manifest, files, state, e
	}
	if e := decodeOracleManifestValue(data, &manifest, v, typed{doc: "oracle/manifest.json", check: b.r.check}, semanticContextManifest(reg.Context)); e != nil {
		return manifest, files, state, e
	}
	if len(manifest.Cases) != 1 || manifest.Cases[0] != reg.Case || manifest.Records != 1 {
		add("SEMANTICS_MANIFEST_WORKLOAD", "oracle/manifest.json", "manifest 사례와 record 수가 사전 등록과 다르다")
	}
	listed := map[string]bool{"manifest.json": true}
	var record *jv
	var recordPin OracleMember
	for _, member := range manifest.Members {
		if e := b.charge(20*uint64(len(member.Path))+1, 512+16*uint64(len(member.Path)), "semantic-manifest-members"); e != nil {
			return manifest, files, state, e
		}
		planned, ok := plan[member.Path]
		path := "oracle/" + member.Path
		pin := snapshot.verifiedMembers[path]
		if !ok || planned.Role != member.Role || listed[member.Path] || pin != (ReplayMember{Path: path, Role: member.Role, Bytes: member.Bytes, SHA256: member.SHA256}) {
			add("SEMANTICS_MEMBER_PLAN", path, "manifest 전체 member가 등록한 경로·역할·bytes와 일치해야 한다")
		}
		listed[member.Path] = true
		valid := false
		switch member.Role {
		case "record", "raw":
			valid = member.Case == reg.Case && member.Path == fmt.Sprintf("%ss/00000-%s.json", member.Role, reg.Case)
			if member.Role == "raw" {
				valid = member.Case == reg.Case && member.Path == "raw/00000-"+reg.Case+".json"
			}
		case "source":
			valid = member.Case == reg.Case && member.Path == "source/00000-"+reg.Case+".bin" && member.Bytes == reg.Input.Bytes && member.SHA256 == reg.Input.SHA256
		case "projection":
			valid = member.Case == reg.Case && member.Path == "projection/00000-"+reg.Case+".json"
		case "capture", "stdin-frame", "process", "stdout-frame", "stderr":
			valid = member.Case == reg.Case && strings.HasPrefix(member.Path, "capture/")
		case "driver", "build-input", "grammar-parser", "grammar-scanner", "grammar-header", "build", "build-manifest":
			valid = member.Case == "" && strings.HasPrefix(member.Path, "capture/") && member.InputBytes == member.Bytes && member.InputSHA256 == member.SHA256 && member.ExecutionStatus == StatusCompleted
		}
		if manifest.Schema == OraclePagedSemanticsManifestSchema && (member.Role == "grammar-paged-descriptor" || member.Role == "grammar-paged-part") {
			valid = member.Case == "" && strings.HasPrefix(member.Path, "capture/") && member.InputBytes == member.Bytes && member.InputSHA256 == member.SHA256 && member.ExecutionStatus == StatusCompleted
		}
		if !valid || !portable(member.Path, false) {
			add("MEMBER_INVALID", path, "producer의 역할별 경로와 입력 결속이 다르다")
		}
		if member.Case != "" && (member.InputBytes != reg.Input.Bytes || member.InputSHA256 != reg.Input.SHA256) {
			add("CASE_MEMBER_MISMATCH", path, "member 입력이 사전 등록과 다르다")
		}
		if member.Role == "record" {
			recordPin = member
			if data, ok := files[member.Path]; ok {
				record, e = b.semanticAST(path, data)
				if e != nil {
					return manifest, files, state, e
				}
				if _, e := checkRecordValueError(typed{doc: path, check: b.r.check}, record, member, manifest.Schema); e != nil {
					return manifest, files, state, e
				}
			}
		}
	}
	for path := range plan {
		if !listed[path] {
			add("SEMANTICS_PLAN_UNUSED", "oracle/"+path, "사전 등록한 member가 manifest에 없다")
		}
	}
	for _, member := range manifest.Members {
		if member.Case != "" && member.ExecutionStatus != recordPin.ExecutionStatus {
			add("CASE_MEMBER_MISMATCH", member.Path, "member와 record 실행 상태가 다르다")
		}
	}
	if e := b.checkSemanticSet(manifest, files, record, &state, add); e != nil {
		return manifest, files, state, e
	}
	if e := b.walkSemanticSet(child.FS(), listed, add); e != nil {
		return manifest, files, state, e
	}
	if e := b.semanticSnapshotIdentity(parent, child, snapshot); e != nil {
		return manifest, files, state, e
	}
	return manifest, files, state, nil
}

func (b *eligibilityBudget) semanticSnapshotIdentity(parent, child *os.Root, snapshot *evidenceSnapshot) *Error {
	if snapshot == nil {
		return fail(KindInvalidInput, "SEMANTICS_SNAPSHOT_MISSING", "oracle", nil)
	}
	pg, cg := newGuard(parent), newGuard(child)
	expected := snapshot.directories["oracle"]
	outer, e := pg.lstat("oracle")
	if e != nil {
		return e
	}
	inner, err := child.Stat(".")
	if err != nil || expected == nil || outer == nil || kind(outer) != "dir" || !os.SameFile(expected, outer) || !os.SameFile(outer, inner) {
		return fail(KindIO, "SOURCE_CHANGED", "oracle", err)
	}
	check := func(path string, before fs.FileInfo, directory bool) *Error {
		if e := b.charge(20*uint64(len(path))+1, 0, "semantic-physical-identity"); e != nil {
			return e
		}
		if !strings.HasPrefix(path, "oracle/") {
			return nil
		}
		info, e := pg.lstat(path)
		if e != nil {
			return e
		}
		local, e := cg.lstat(strings.TrimPrefix(path, "oracle/"))
		if e != nil {
			return e
		}
		if info == nil || local == nil || before == nil || !os.SameFile(before, info) || !os.SameFile(info, local) || !directory && (kind(info) != "file" || info.Size() != before.Size() || !info.ModTime().Equal(before.ModTime())) || directory && kind(info) != "dir" {
			return fail(KindIO, "SOURCE_CHANGED", path, nil)
		}
		return nil
	}
	for path, info := range snapshot.directories {
		if e := check(path, info, true); e != nil {
			return e
		}
	}
	for path, info := range snapshot.observed {
		if e := check(path, info, false); e != nil {
			return e
		}
	}
	return nil
}
