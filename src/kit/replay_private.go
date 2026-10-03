package kit

import (
	"bufio"
	"bytes"
	jsonv2 "encoding/json/v2"
	"fmt"
	"maps"
	"regexp"
	"strings"
)

// The NET461-PHASE2-LOCAL-r1 workload registration: the corpus route table groups (svc is
// its own group parsed by the csharp route), the per-file projection and the local-run
// identity. Paths, names and content stay in the caller's local records; this reducer
// reports ordinals and counts only.
var privateGroups = []string{"csharp", "tsql", "xml", "svc"}

type pcInventory struct {
	Schema  string `json:"schema"`
	Command string `json:"command"`
	Summary struct {
		EncodingCodes map[string]int `json:"encoding_codes"`
	} `json:"summary"`
	Records []struct {
		Path     string `json:"path"`
		Route    string `json:"route"`
		State    string `json:"state"`
		Size     uint64 `json:"size"`
		SHA256   string `json:"sha256"`
		Encoding *struct {
			Assessment string `json:"assessment"`
			Encoding   string `json:"encoding"`
			Code       string `json:"code"`
		} `json:"encoding"`
	} `json:"records"`
}

// pcEntry is one per-file projection line, recomputed and compared field by field.
type pcEntry struct {
	ID              string  `json:"id"`
	Path            string  `json:"path"`
	Route           string  `json:"route"`
	Size            uint64  `json:"size"`
	ExecutionStatus string  `json:"execution_status"`
	Assessment      string  `json:"assessment"`
	Code            string  `json:"code"`
	HasError        *bool   `json:"has_error"`
	DescendantCount *uint64 `json:"descendant_count"`
	Digest          *string `json:"digest"`
	Form            *string `json:"form"`
}

type pcSummary struct {
	Schema          string `json:"schema"`
	Corpus          string `json:"corpus"`
	Operation       string `json:"operation"`
	CandidateCommit string `json:"candidate_commit"`
	CleanTree       bool   `json:"clean_tree"`
	DirtyEntries    int    `json:"dirty_entries"`
	Host            struct {
		OS   string `json:"os"`
		Arch string `json:"arch"`
	} `json:"host"`
	Tools struct {
		CompilerSHA256 string `json:"compiler_sha256"`
		RuntimeCommit  string `json:"runtime_commit"`
	} `json:"tools"`
	Inventory struct {
		Records           int            `json:"records"`
		RoutedNonPresence int            `json:"routed_non_presence"`
		PresenceOnly      int            `json:"presence_only"`
		Unrouted          int            `json:"unrouted"`
		EncodingCodes     map[string]int `json:"encoding_codes"`
	} `json:"inventory"`
	ByRouteStatus map[string]int `json:"by_route_status"`
	HasError      map[string]int `json:"has_error"`
	Codes         map[string]int `json:"codes"`
	Forms         map[string]int `json:"forms"`
	NotRun        int            `json:"not_run"`
	Runs          []struct {
		Group           string `json:"group"`
		ExecutionStatus string `json:"execution_status"`
		Assessment      string `json:"assessment"`
		Cases           int    `json:"cases"`
	} `json:"runs"`
}

var commitHex = regexp.MustCompile(`^[0-9a-f]{40}$`)

// portableHelper is the projection helper's portability test (the S05 corpus helper), kept
// apart from the kit's own path policy so the projection is recomputed as it was made.
func portableHelper(p string) bool {
	if strings.ContainsAny(p, `\:`) {
		return false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.TrimRight(seg, " .") != seg {
			return false
		}
		base, _, _ := strings.Cut(strings.ToLower(seg), ".")
		if base == "con" || base == "prn" || base == "aux" || base == "nul" ||
			(len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '0' && base[3] <= '9') {
			return false
		}
	}
	return true
}

func replayPrivateCorpus(x *replayEnv) *Error {
	x.redact = true
	im, e := x.oneMember("inventory")
	if e != nil {
		return e
	}
	data, e := x.member(im.Path)
	if e != nil {
		return e
	}
	var inv pcInventory
	if err := jsonv2.Unmarshal(data, &inv); err != nil {
		return fail(KindInvalidInput, "RECORD_INVALID", im.Path, nil)
	}
	data = nil
	if inv.Schema != ReportSchema || inv.Command != "corpus" {
		return fail(KindUnsupported, "SCHEMA_UNSUPPORTED", im.Path, nil)
	}
	if uint64(len(inv.Records)) > x.lim.Records {
		return fail(KindResourceLimit, "RECORD_COUNT_LIMIT", im.Path, nil)
	}
	// 1. the workload: every routed non-presence inventory record, in inventory order
	entries := []*pcEntry{}
	byID := map[string]*pcEntry{}
	groups := map[string][]IncrementalCase{}
	presence, unrouted := 0, 0
	for i, rec := range inv.Records {
		if rec.State == "PRESENCE_ONLY" {
			presence++
		}
		if rec.Route == "" {
			unrouted++
		}
		if rec.Route == "" || rec.State == "PRESENCE_ONLY" {
			continue
		}
		en := &pcEntry{ID: fmt.Sprintf("f%06d", i+1), Path: rec.Path, Route: rec.Route, Size: rec.Size, ExecutionStatus: StatusNotRun, Assessment: AssessNotAssessed, Code: "NOT_RUN"}
		switch {
		case rec.State != StatusCompleted:
			en.Assessment, en.Code = AssessBlocked, "INVENTORY_"+rec.State
		case rec.Encoding == nil || rec.Encoding.Assessment != AssessPass:
			en.Assessment, en.Code = AssessBlocked, "ENCODING_ABSENT"
			if rec.Encoding != nil {
				en.Code = rec.Encoding.Code
			}
		case !portableHelper(rec.Path):
			en.Assessment, en.Code = AssessBlocked, "PATH_NOT_PORTABLE"
		default:
			groups[rec.Route] = append(groups[rec.Route], IncrementalCase{ID: en.ID, Encoding: rec.Encoding.Encoding,
				Input: NativeInput{Path: rec.Path, Role: "case", SHA256: rec.SHA256, Bytes: rec.Size}})
		}
		entries = append(entries, en)
		byID[en.ID] = en
	}
	// 2. each route group: its profile must register exactly the group, then its result is
	// replayed case by case against that profile
	bind := x.gate("corpus-binding")
	var outs []caseOutcome
	for _, g := range privateGroups {
		pms, rms := x.membersByRole("workload-profile:"+g), x.membersByRole("result:"+g)
		if len(groups[g]) == 0 && len(pms) == 0 && len(rms) == 0 {
			continue
		}
		if len(pms) != 1 || len(rms) != 1 {
			return fail(KindInvalidInput, "MEMBER_ROLE_COUNT", g, nil)
		}
		pdata, e := x.member(pms[0].Path)
		if e != nil {
			return e
		}
		prof, e := parseIncremental(pdata)
		if e != nil {
			return fail(KindInvalidInput, "MEMBER_INVALID_"+e.Code, pms[0].Path, nil)
		}
		prof.SHA256 = digestHex(pdata)
		want := groups[g]
		same := len(prof.Cases) == len(want)
		for i := 0; same && i < len(want); i++ {
			pc := prof.Cases[i]
			same = pc.ID == want[i].ID && pc.Input == want[i].Input && pc.Encoding == want[i].Encoding && len(pc.Edits) == 0 && len(pc.Expect) == 0
		}
		bind.check(same && prof.Operation == "private-corpus-local" && prof.Output == "record", g, "CORPUS_BINDING_MISMATCH",
			"route profile이 inventory의 route 파일 집합·순서·identity와 다르다")
		_, o, e := x.replayResultFile(rms[0].Path, prof, g+":", func(c *rcCase, o caseOutcome, i int) {
			en := byID[c.ID]
			if en == nil {
				return
			}
			en.ExecutionStatus, en.Assessment, en.Code = c.ExecutionStatus, c.Assessment, c.Code
			if len(c.Steps) > 0 && c.Steps[0].Incremental != nil {
				t := c.Steps[0].Incremental
				h, n, d, f := t.HasError, t.DescendantCount, t.Digest, t.Form
				en.HasError, en.DescendantCount, en.Digest, en.Form = &h, &n, &d, &f
			}
		})
		if e != nil {
			return e
		}
		outs = append(outs, o...)
	}
	x.recomp = aggregateCases(outs)
	// 3. the per-file projection: every line consumed once and equal to the recomputed entry
	pm, e := x.oneMember("projection")
	if e != nil {
		return e
	}
	pdata, e := x.member(pm.Path)
	if e != nil {
		return e
	}
	ids := make([]string, len(entries))
	for i, en := range entries {
		ids[i] = en.ID
	}
	cons := newConsumer(x, ids)
	pg := x.gate("record-projection")
	sc := bufio.NewScanner(bytes.NewReader(pdata))
	sc.Buffer(make([]byte, 0, 1<<16), int(x.lim.RecordBytes))
	for i := 0; sc.Scan(); i++ {
		line := bytes.TrimPrefix(sc.Bytes(), []byte("\xef\xbb\xbf"))
		var got pcEntry
		if err := jsonv2.Unmarshal(line, &got); err != nil {
			return fail(KindInvalidInput, "RECORD_INVALID", pm.Path, nil)
		}
		if !cons.take(got.ID, i) {
			continue
		}
		pg.check(equalEntry(&got, byID[got.ID]), x.name(got.ID, i), "PROJECTION_MISMATCH", "파일 projection이 다시 계산한 값과 다르다")
	}
	if sc.Err() != nil {
		return fail(KindResourceLimit, "RECORD_BYTES_LIMIT", pm.Path, nil)
	}
	cons.close()
	// 4. the summary counts and the local-run identity
	sm, e := x.oneMember("summary")
	if e != nil {
		return e
	}
	sdata, e := x.member(sm.Path)
	if e != nil {
		return e
	}
	var s pcSummary
	if err := jsonv2.Unmarshal(bytes.TrimPrefix(sdata, []byte("\xef\xbb\xbf")), &s); err != nil {
		return fail(KindInvalidInput, "RECORD_INVALID", sm.Path, nil)
	}
	if s.Schema != "tsgk-s05-private-corpus-run/r1" || s.Corpus != "NET461-PHASE2-LOCAL-r1" {
		return fail(KindUnsupported, "SCHEMA_UNSUPPORTED", sm.Path, nil)
	}
	count := func(key func(*pcEntry) string) map[string]int {
		out := map[string]int{}
		for _, en := range entries {
			out[key(en)]++
		}
		return out
	}
	opt := func(cond bool, s string) string {
		if cond {
			return s
		}
		return "null"
	}
	notRun := 0
	for _, en := range entries {
		if en.ExecutionStatus == StatusNotRun && en.Assessment != AssessBlocked {
			notRun++
		}
	}
	sg := x.gate("summary")
	checks := []struct {
		ok   bool
		what string
	}{
		{s.Inventory.Records == len(inv.Records) && s.Inventory.RoutedNonPresence == len(entries) && s.Inventory.PresenceOnly == presence && s.Inventory.Unrouted == unrouted &&
			maps.Equal(s.Inventory.EncodingCodes, inv.Summary.EncodingCodes), "inventory"},
		{maps.Equal(s.ByRouteStatus, count(func(e *pcEntry) string { return e.Route + ":" + e.ExecutionStatus + ":" + e.Assessment })), "by_route_status"},
		{maps.Equal(s.HasError, count(func(e *pcEntry) string {
			if e.ExecutionStatus != StatusCompleted {
				return "null"
			}
			if e.HasError == nil {
				return e.Route + ":null"
			}
			return e.Route + ":" + map[bool]string{true: "True", false: "False"}[*e.HasError]
		})), "has_error"},
		{maps.Equal(s.Codes, count(func(e *pcEntry) string { return opt(e.Code != "", e.Route+":"+e.Code) })), "codes"},
		{maps.Equal(s.Forms, count(func(e *pcEntry) string { return opt(e.Form != nil && *e.Form != "", e.Route+":"+deref(e.Form)) })), "forms"},
		{s.NotRun == notRun, "not_run"},
	}
	for _, c := range checks {
		sg.check(c.ok, c.what, "SUMMARY_MISMATCH", "summary의 "+c.what+" 개수가 다시 센 값과 다르다")
	}
	runs := map[string]Verdict{}
	for _, r := range s.Runs {
		runs[r.Group] = Verdict{r.ExecutionStatus, r.Assessment}
	}
	var recs []caseOutcome
	for _, v := range runs {
		recs = append(recs, caseOutcome{status: v.ExecutionStatus, assess: v.Assessment})
	}
	x.recorded = aggregateCases(recs)
	lg := x.gate("local-run-identity")
	lg.check(commitHex.MatchString(s.CandidateCommit), "candidate_commit", "LOCAL_RUN_IDENTITY_INVALID", "후보 commit이 40자리 hex가 아니다")
	lg.check(s.CleanTree && s.DirtyEntries == 0, "clean_tree", "LOCAL_RUN_DIRTY", "실행 당시 작업 트리가 clean이 아니었다")
	lg.check(s.Host.OS != "" && s.Host.Arch != "", "host", "LOCAL_RUN_IDENTITY_INVALID", "host 정보가 없다")
	for _, g := range privateGroups {
		if c, ok := x.actual[g+":compiler"]; ok {
			lg.check(c == s.Tools.CompilerSHA256 && x.actual[g+":runtime"] == s.Tools.RuntimeCommit, g, "LOCAL_RUN_TOOL_MISMATCH", "route 결과의 도구 identity가 실행 기록과 다르다")
		}
	}
	x.actual["commit"], x.actual["clean_tree"] = s.CandidateCommit, fmt.Sprint(s.CleanTree)
	x.actual["host"], x.actual["compiler"], x.actual["runtime"] = s.Host.OS+"/"+s.Host.Arch, s.Tools.CompilerSHA256, s.Tools.RuntimeCommit
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func equalEntry(got, want *pcEntry) bool {
	if want == nil {
		return false
	}
	eqb := func(a, b *bool) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	equ := func(a, b *uint64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	eqs := func(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	return got.Path == want.Path && got.Route == want.Route && got.Size == want.Size && got.ExecutionStatus == want.ExecutionStatus &&
		got.Assessment == want.Assessment && got.Code == want.Code && eqb(got.HasError, want.HasError) && equ(got.DescendantCount, want.DescendantCount) &&
		eqs(got.Digest, want.Digest) && eqs(got.Form, want.Form)
}
