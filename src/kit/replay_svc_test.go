package kit

import (
	"encoding/json"
	"testing"
)

func TestSvcReplayCompositeSyntax(t *testing.T) {
	for _, source := range []string{`<%@ ServiceHost Service="S" ; %>`, `<%@ ServiceHost Language="C#" Service="S" Bogus="x" %>` + "\nclass S {}"} {
		o := ObserveServiceHost(EncodingUTF8, []byte(source))
		step := rcStep{}
		step.Composite = &rcComposite{Directive: o.Directive, AdditionalDirectives: o.AdditionalDirectives, Coverage: o.Coverage}
		if o.IncludedRanges != nil {
			step.Incremental = &rcTree{HasError: false}
		}
		for _, syntax := range []string{"ERROR", "NO_ERROR"} {
			got, known := svcExpectResult(StepExpectation{Syntax: syntax}, step)
			want := claimPass
			if syntax == "NO_ERROR" {
				want = claimFail
			}
			if !known || got != want {
				t.Fatalf("%s %s: %s/%v", source, syntax, got, known)
			}
		}
	}
}

func TestSvcReplayRejectsCompositeTampering(t *testing.T) {
	source := []byte(`<%@ ServiceHost Service="S" CodeBehind="A.cs" %>`)
	ctx := &SvcContext{DefaultLanguage: "C#", LanguageSource: "owned", CodeBehind: []SvcReference{{Name: "A.cs", Case: "ref"}, {Name: "B.cs", Case: "other"}}}
	for _, bad := range []string{"", "removed", "language", "spelling", "resolution", "diagnostic", "input", "tree", "missing-source", "identity-missing", "identity-duplicate", "identity-source", "identity-schema", "noncompleted-missing-source"} {
		o := ObserveServiceHostWithContext(EncodingUTF8, source, ctx)
		o.CodeBehind.Resolution = "PARSED"
		data, _ := json.Marshal(o)
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		m["schema"], m["format"] = SvcCompositeSchema, SvcFormat
		m["identities"] = []IdentityRef{{"producer", "tsgk-native-build/r1", fxProducer}, {"source", "tsgk-source-bytes/r1", digestHex(source)}, {"policy", "tsgk-native-policy/r1", fxPolicy}}
		m["input"] = rcInput{Bytes: uint64(len(source)), SHA256: digestHex(source), Encoding: EncodingUTF8}
		data, _ = json.Marshal(m)
		var comp rcComposite
		if e := json.Unmarshal(data, &comp); e != nil {
			t.Fatal(e)
		}
		c := &rcCase{ID: "owner", ExecutionStatus: StatusCompleted, Steps: []rcStep{{Composite: &comp, SourceBytes: uint64(len(source)), SourceSHA256: digestHex(source)}}, References: []rcSvcReference{{Case: "ref", ExecutionStatus: StatusCompleted}}}
		want := &IncrementalCase{ID: c.ID, Encoding: EncodingUTF8, SvcSource: source}
		switch bad {
		case "removed":
			c.Steps[0].Composite = nil
		case "language":
			comp.Language.Status = "UNSUPPORTED_LANGUAGE"
		case "spelling":
			comp.CodeBehind.Case = "other"
			c.References[0].Case = "other"
		case "resolution":
			comp.CodeBehind.Resolution = "NOT_RESOLVED"
		case "diagnostic":
			comp.Directive.Diagnostics = []string{"ATTRIBUTE_UNKNOWN"}
		case "input":
			comp.Input.SHA256 = "wrong"
		case "tree":
			c.Steps[0].Incremental = &rcTree{Status: StatusCompleted}
		case "identity-missing":
			comp.Identities = comp.Identities[:1]
		case "identity-duplicate":
			comp.Identities[1] = comp.Identities[0]
		case "identity-source":
			comp.Identities[1].SHA256 = fxPolicy
		case "identity-schema":
			comp.Identities[0].Schema = "wrong"
		case "noncompleted-missing-source":
			c.ExecutionStatus = StatusNotRun
			want.SvcSource = nil
		case "missing-source":
			want.SvcSource = nil
		}
		x := &replayEnv{svcFormat: SvcFormat, svcContext: ctx}
		x.checkSvcComposite(c, want)
		if (x.gate("case-binding").Failed > 0) != (bad != "") {
			t.Fatalf("%s: %+v", bad, x.gate("case-binding"))
		}
	}
}

func TestSvcReplayRejectsUnregisteredComposite(t *testing.T) {
	for _, status := range []string{StatusCompleted, StatusNotRun} {
		x := &replayEnv{}
		c := &rcCase{ID: "forged", ExecutionStatus: status, Steps: []rcStep{{Composite: &rcComposite{}}}}
		x.checkSvcComposite(c, nil)
		if x.gate("case-binding").Code != "SVC_COMPOSITE_UNEXPECTED" {
			t.Fatalf("%s: %+v", status, x.gate("case-binding"))
		}
	}
}

func TestQualifyRejectsUnregisteredComposite(t *testing.T) {
	f := newQfx(t)
	mut := qfxMut{record: func(set string, records []map[string]any) {
		if set != "s06-fxa" {
			return
		}
		step := records[0]["steps"].([]any)[0].(map[string]any)
		step["composite"] = map[string]any{"directive": SvcDirective{Diagnostics: []string{"ATTRIBUTE_UNKNOWN"}}, "coverage": SvcCoverage{Inline: "ABSENT"}}
		step["incremental"], step["fresh"] = nil, nil
	}}
	r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": mut}))
	c := cellOf(r, "fxa", "windows-amd64")
	if c.Mechanism == AssessPass {
		t.Fatalf("forged composite passed: %+v", c)
	}
}

func TestQualifyRejectsStepOrdinal(t *testing.T) {
	f := newQfx(t)
	mut := qfxMut{record: func(set string, records []map[string]any) {
		if set == "s06-fxa" {
			records[0]["steps"].([]any)[0].(map[string]any)["step"] = 9
		}
	}}
	r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": mut}))
	c := cellOf(r, "fxa", "windows-amd64")
	found := false
	for _, g := range c.Set.Gates {
		if g.ID == "case-binding" && g.Code == "STEP_INDEX_MISMATCH" {
			found = true
		}
	}
	if c.Mechanism == AssessPass || !found {
		t.Fatalf("ordinal mismatch accepted: %+v", c)
	}
}
