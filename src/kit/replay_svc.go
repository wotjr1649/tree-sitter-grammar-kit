package kit

import "reflect"

// R2 recomputes the directive from a source pinned by the profile, including its edits.
func (x *replayEnv) checkSvcComposite(c *rcCase, want *IncrementalCase) {
	g := x.gate("case-binding")
	if x.svcFormat != SvcFormat && x.svcFormat != SvcLegacyFormat {
		for _, s := range c.Steps {
			g.check(s.Composite == nil, c.ID, "SVC_COMPOSITE_UNEXPECTED", "SVC format에 등록되지 않은 composite다")
		}
		return
	}
	ref := false
	if x.svcContext != nil {
		for _, r := range x.svcContext.CodeBehind {
			if r.Case == c.ID {
				ref = true
			}
		}
	}
	if x.svcFormat == SvcFormat && (want == nil || want.SvcSource == nil) {
		g.fail(c.ID, "SVC_SOURCE_MISSING", "r2 재관측용 원본 등록이 없다")
		return
	}
	if ref {
		for _, s := range c.Steps {
			g.check(s.Composite == nil, c.ID, "SVC_REFERENCE_INVALID", "참조 C# case에 composite가 있다")
		}
		return
	}
	if c.ExecutionStatus != StatusCompleted {
		return
	}
	var versions [][]byte
	if x.svcFormat == SvcFormat {
		if want == nil || want.SvcSource == nil {
			g.fail(c.ID, "SVC_SOURCE_MISSING", "r2 재관측용 원본 등록이 없다")
			return
		}
		var err error
		versions, _, err = ApplyEdits(want.Encoding, want.SvcSource, want.Edits, uint64(len(want.SvcSource))+1<<20)
		if err != nil {
			g.fail(c.ID, "SVC_SOURCE_INVALID", "등록된 원본의 edit가 유효하지 않다")
			return
		}
	}
	for i, s := range c.Steps {
		if !g.check(s.Composite != nil, c.ID, "SVC_COMPOSITE_MISSING", "SVC step에 composite가 없다") {
			continue
		}
		if x.svcFormat == SvcFormat {
			roles := map[string]string{"producer": "tsgk-native-build/r1", "source": "tsgk-source-bytes/r1", "policy": "tsgk-native-policy/r1"}
			valid := len(s.Composite.Identities) == len(roles)
			for _, id := range s.Composite.Identities {
				valid = valid && roles[id.Role] == id.Schema && hex64.MatchString(id.SHA256)
				if id.Role == "source" {
					valid = valid && id.SHA256 == s.SourceSHA256
				}
				delete(roles, id.Role)
			}
			g.check(valid && len(roles) == 0, c.ID, "SVC_IDENTITY_INVALID", "composite의 producer/source/policy identity가 없거나 다르다")
		}
		if versions == nil {
			continue
		}
		if i >= len(versions) {
			g.fail(c.ID, "SVC_SOURCE_INVALID", "등록 source step이 없다")
			continue
		}
		o := ObserveServiceHostWithContext(want.Encoding, versions[i], x.svcContext)
		g.check(s.SourceBytes == uint64(len(versions[i])) && s.SourceSHA256 == digestHex(versions[i]), c.ID, "SVC_SOURCE_MISMATCH", "step 원본 identity가 등록 edit 결과와 다르다")
		g.check(s.Restarted == (i > 0 && s.Incremental != nil && c.Steps[i-1].Incremental == nil), c.ID, "SVC_RESTART_INVALID", "restart가 gap 뒤 첫 tree와 다르다")
		cb := o.CodeBehind
		if cb != nil && cb.Case != "" {
			cb.Resolution = "NOT_PARSED"
			for _, r := range c.References {
				if r.Case == cb.Case && r.ExecutionStatus == StatusCompleted {
					cb.Resolution = "PARSED"
				}
			}
		}
		a := s.Composite
		g.check(a.Schema == SvcCompositeSchema && a.Format == SvcFormat && a.Input.SHA256 == digestHex(versions[i]) && a.Input.Bytes == uint64(len(versions[i])) && a.Input.Encoding == want.Encoding &&
			reflect.DeepEqual(a.Directive, o.Directive) && reflect.DeepEqual(a.AdditionalDirectives, o.AdditionalDirectives) && reflect.DeepEqual(a.Language, o.Language) && reflect.DeepEqual(a.CodeBehind, cb) && a.Coverage == o.Coverage,
			c.ID, "SVC_OBSERVATION_MISMATCH", "composite가 등록된 원본 재관측과 다르다")
		g.check((s.Incremental != nil) == (o.IncludedRanges != nil), c.ID, "SVC_INLINE_MISMATCH", "inline tree 유무가 실제 included range와 다르다")
		if o.IncludedRanges != nil {
			g.check(a.Inline != nil && s.Incremental != nil && (s.Incremental.Form != "full" || s.Incremental.Tree != nil) && reflect.DeepEqual(a.Inline.Tree, s.Incremental.Tree) && reflect.DeepEqual(a.Inline.IncludedRanges, o.IncludedRanges), c.ID, "SVC_INLINE_MISMATCH", "inline tree 또는 included range가 실제 step과 다르다")
		} else {
			g.check(a.Inline == nil, c.ID, "SVC_INLINE_MISMATCH", "관측 전용 step에 inline이 있다")
		}
	}
	gap, trees := false, false
	for _, s := range c.Steps {
		if s.Incremental == nil {
			gap = true
		} else {
			trees = true
		}
	}
	segments := [][2]int{}
	if gap && trees {
		for i := 0; i < len(c.Steps); {
			if c.Steps[i].Incremental == nil {
				i++
				continue
			}
			end := i + 1
			for end < len(c.Steps) && c.Steps[end].Incremental != nil {
				end++
			}
			segments = append(segments, [2]int{i, end})
			i = end
		}
	}
	g.check(len(segments) == len(c.Segments), c.ID, "SVC_SEGMENT_MISMATCH", "native segment 수가 tree 구간과 다르다")
	for i, seg := range c.Segments {
		if i >= len(segments) {
			break
		}
		g.check(seg.StartStep == segments[i][0] && seg.EndStep == segments[i][1] && seg.Process != nil && seg.Process.Status == StatusCompleted && seg.Process.ExitCode == 0 && seg.Process.Cleanup.Verified, c.ID, "SVC_SEGMENT_MISMATCH", "segment 경계나 실행·회수 증거가 다르다")
	}
}

type rcSvcReference struct {
	Case            string      `json:"case"`
	Input           NativeInput `json:"input"`
	ExecutionStatus string      `json:"execution_status"`
	Assessment      string      `json:"assessment"`
	HasError        bool        `json:"has_error"`
}

type svcCaseEvidence struct {
	Input              NativeInput
	Status, Assessment string
	HasError           bool
}
type svcReferenceLink struct {
	Owner     string
	Reference rcSvcReference
}

func (x *replayEnv) startSvcReferences(context *SvcContext) {
	x.svcContext = context
	x.svcEvidence = map[string]svcCaseEvidence{}
	x.svcReferences = nil
}

func (x *replayEnv) observeSvcReferences(c *rcCase) {
	if x.svcEvidence == nil {
		x.svcEvidence = map[string]svcCaseEvidence{}
	}
	evidence := svcCaseEvidence{Input: c.Input, Status: c.ExecutionStatus, Assessment: c.Assessment}
	for _, s := range c.Steps {
		if s.Incremental != nil && s.Incremental.HasError {
			evidence.HasError = true
		}
	}
	x.svcEvidence[c.ID] = evidence
	expected := map[string]bool{}
	for _, s := range c.Steps {
		if s.Composite != nil && s.Composite.CodeBehind != nil && s.Composite.CodeBehind.Case != "" {
			expected[s.Composite.CodeBehind.Case] = true
		}
	}
	seen := map[string]bool{}
	g := x.gate("case-binding")
	if len(c.References) > 16 {
		g.fail(c.ID, "SVC_REFERENCE_INVALID", "SVC reference 수가 상한을 넘는다")
		return
	}
	for _, r := range c.References {
		registered := false
		if x.svcContext != nil {
			for _, decl := range x.svcContext.CodeBehind {
				if decl.Case == r.Case {
					registered = true
					break
				}
			}
		}
		g.check(registered && expected[r.Case] && !seen[r.Case] && r.Case != c.ID, c.ID, "SVC_REFERENCE_INVALID", "등록되지 않거나 중복된 SVC reference다")
		seen[r.Case] = true
		x.svcReferences = append(x.svcReferences, svcReferenceLink{Owner: c.ID, Reference: r})
	}
	for id := range expected {
		g.check(seen[id], c.ID, "SVC_REFERENCE_MISSING", "선언된 SVC reference 결과가 없다")
	}
}

func (x *replayEnv) checkSvcReferences() {
	g := x.gate("case-binding")
	for _, link := range x.svcReferences {
		r := link.Reference
		e, found := x.svcEvidence[r.Case]
		assessment := e.Assessment
		if e.Status != StatusCompleted {
			assessment = AssessBlocked
		}
		g.check(found && r.Input == e.Input && r.ExecutionStatus == e.Status && r.Assessment == assessment && r.HasError == e.HasError, link.Owner, "SVC_REFERENCE_MISMATCH", "SVC reference가 별도 case record와 다르다")
	}
}
