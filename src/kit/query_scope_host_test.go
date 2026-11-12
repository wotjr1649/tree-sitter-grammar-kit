package kit

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const (
	hostWindows = "windows-amd64"
	hostLinux   = "linux-amd64"
)

func otherHostOf(host string) string {
	if host == hostWindows {
		return hostLinux
	}
	return hostWindows
}

// hostPredicate and hostSvc are the scope results one host produces when all its own obligations
// are judged VERIFIED and the other host stays BLOCKED, the shape the native fan-in test stores.
func (f *faninFixture) hostPredicate(t *testing.T, host string) QueryScopeHostResult {
	t.Helper()
	judgments, _ := predicateJudgments(t, f.req.Registry, host)
	r, err := PredicateScopeResult(f.req.Registry, f.predAsset, judgments, map[string]bool{otherHostOf(host): true})
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewQueryScopeHostResult(host, r)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (f *faninFixture) hostSvc(t *testing.T, host string, mutate func([]SvcJudgment)) QueryScopeHostResult {
	t.Helper()
	judgments, err := VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(judgments)
	}
	r, err := SvcScopeResult(f.svc, f.svcAsset, f.req.Registry, host, judgments, map[string]bool{otherHostOf(host): true})
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewQueryScopeHostResult(host, r)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func resealHost(h QueryScopeHostResult) QueryScopeHostResult {
	h.Result = seal(cloneScope(h.Result))
	h.HostSHA256 = h.digest()
	return h
}

func TestQueryScopeHostMergeFoldsBothHosts(t *testing.T) {
	t.Parallel()
	f := faninFixtureFor(t)
	var scopes []QueryScopeResult
	for _, hosts := range [][]QueryScopeHostResult{
		// the order of the stored files does not matter
		{f.hostPredicate(t, hostLinux), f.hostPredicate(t, hostWindows)},
		{f.hostSvc(t, hostWindows, nil), f.hostSvc(t, hostLinux, nil)},
	} {
		// through the stored form, as two CI hosts would hand them over
		var stored []QueryScopeHostResult
		for _, h := range hosts {
			data, err := EncodeQueryScopeHostResult(h)
			if err != nil {
				t.Fatal(err)
			}
			back, err := DecodeQueryScopeHostResult(data)
			if err != nil || !reflect.DeepEqual(back, h) {
				t.Fatalf("round trip: %v", err)
			}
			again, err := EncodeQueryScopeHostResult(back)
			if err != nil || !bytes.Equal(again, data) {
				t.Fatalf("encoding is not stable: %v", err)
			}
			stored = append(stored, back)
		}
		merged, err := MergeQueryScopeHostResults(stored)
		if err != nil {
			t.Fatal(err)
		}
		scopes = append(scopes, merged)
	}
	res, err := FoldQueryCompletion(f.request(scopes...))
	if err != nil {
		t.Fatal(err)
	}
	// both hosts judged their own rows: predicate 2 x 67, SVC 2 x 263 offline, the rest BLOCKED
	for scope, want := range map[string]QueryCompletionCounts{
		QueryScopePredicate: {Required: 134, Verified: 134},
		QueryScopeSvc:       {Required: 1018, Verified: 526, Blocked: 492},
	} {
		if got := totalsOf(res, scope); got != want {
			t.Errorf("%s %+v, want %+v", scope, got, want)
		}
	}
	for _, c := range res.Counters {
		if (c.Scope == QueryScopePredicate || c.Scope == QueryScopeSvc && c.Kind != SvcKindInlineTree && c.Kind != SvcKindQuery) && c.Verified != c.Required {
			t.Errorf("%+v", c)
		}
	}
	if res.Total.Failed != 0 || res.Complete || res.Completion != QueryCompletionBlocked || res.Code != "QUERY_SCOPE_RESULT_ABSENT" {
		t.Fatalf("verdict %s %s %+v", res.Completion, res.Code, res.Total)
	}
	for _, h := range res.Hosts {
		if h.Verified != 330 {
			t.Errorf("host %s verified %d, want 330 (67 + 263)", h.Platform, h.Verified)
		}
	}
}

func TestQueryScopeHostMergeKeepsAHostFailure(t *testing.T) {
	t.Parallel()
	f := faninFixtureFor(t)
	failing := f.hostSvc(t, hostLinux, func(j []SvcJudgment) {
		i := slices.IndexFunc(j, func(x SvcJudgment) bool { return x.State == "VERIFIED" })
		j[i] = SvcJudgment{ID: j[i].ID, State: "FAILED", Code: "SVC_LINUX_ONLY"}
	})
	merged, err := MergeQueryScopeHostResults([]QueryScopeHostResult{f.hostSvc(t, hostWindows, nil), failing})
	if err != nil || merged.FirstFailure != "SVC_LINUX_ONLY" {
		t.Fatalf("merge: %+v %v", merged, err)
	}
	for _, c := range merged.Counters {
		if (c.Failed != 0) != (c.Platform == hostLinux && c.Kind == SvcKindHistory) {
			t.Errorf("failure on the wrong row: %+v", c)
		}
	}
	res, err := FoldQueryCompletion(f.request(merged))
	if err != nil || res.Completion != QueryCompletionFail || res.Code != "SVC_LINUX_ONLY" {
		t.Fatalf("%s %s %v", res.Completion, res.Code, err)
	}
}

// A host's judgments never count for the other host: a result that claims the other host's rows,
// or a result of one host stored under the other host's name, is refused.
func TestQueryScopeHostRefusesJudgmentsOfAnotherHost(t *testing.T) {
	t.Parallel()
	f := faninFixtureFor(t)
	code := func(err error) string {
		t.Helper()
		if err == nil {
			t.Fatal("accepted")
		}
		return faninCode(t, err)
	}
	// predicate judgments of both hosts folded as one host's result
	both, _ := predicateJudgments(t, f.req.Registry, hostWindows)
	linux, _ := predicateJudgments(t, f.req.Registry, hostLinux)
	mixed, err := PredicateScopeResult(f.req.Registry, f.predAsset, append(both, linux...), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{hostWindows, hostLinux} {
		_, err := NewQueryScopeHostResult(host, mixed)
		if got := code(err); got != "QUERY_SCOPE_HOST_FOREIGN" {
			t.Errorf("mixed judgments stored as %s: %s", host, got)
		}
	}
	// the Linux result copied onto the Windows name, and the reverse
	for _, h := range []QueryScopeHostResult{f.hostPredicate(t, hostLinux), f.hostSvc(t, hostLinux, nil)} {
		_, err := NewQueryScopeHostResult(hostWindows, h.Result)
		if got := code(err); got != "QUERY_SCOPE_HOST_FOREIGN" {
			t.Errorf("%s result under the windows name: %s", h.Result.Scope, got)
		}
		relabeled := h
		relabeled.Host = hostWindows
		_, err = MergeQueryScopeHostResults([]QueryScopeHostResult{relabeled, f.hostSvc(t, hostWindows, nil)})
		if got := code(err); got != "QUERY_SCOPE_HOST_FOREIGN" {
			t.Errorf("relabeled %s result in a merge: %s", h.Result.Scope, got)
		}
	}
	// a host that is not registered
	if _, err := NewQueryScopeHostResult("darwin-arm64", f.hostSvc(t, hostWindows, nil).Result); code(err) != "QUERY_SCOPE_HOST_UNKNOWN" {
		t.Error("unregistered host accepted")
	}
}

func TestQueryScopeHostRejectsTamperedFiles(t *testing.T) {
	t.Parallel()
	f := faninFixtureFor(t)
	base := f.hostSvc(t, hostWindows, nil)
	edit := func(fn func(h *QueryScopeHostResult)) QueryScopeHostResult {
		h := base
		h.Result = cloneScope(h.Result)
		fn(&h)
		return h
	}
	row := func(h *QueryScopeHostResult, kind, host string) *QueryScopeCounter {
		i := slices.IndexFunc(h.Result.Counters, func(c QueryScopeCounter) bool { return c.Kind == kind && c.Platform == host })
		return &h.Result.Counters[i]
	}
	for _, tc := range []struct {
		name, code string
		h          QueryScopeHostResult
	}{
		{"schema", "QUERY_SCOPE_HOST_SCHEMA", edit(func(h *QueryScopeHostResult) { h.Schema += "x" })},
		{"host renamed", "QUERY_SCOPE_HOST_FOREIGN", edit(func(h *QueryScopeHostResult) { h.Host = hostLinux })},
		{"count without the result digest", "QUERY_SCOPE_RESULT_DIGEST", edit(func(h *QueryScopeHostResult) { row(h, SvcKindHistory, hostWindows).Verified-- })},
		{"result resealed, host digest stale", "QUERY_SCOPE_HOST_DIGEST", func() QueryScopeHostResult {
			h := edit(func(h *QueryScopeHostResult) {
				row(h, SvcKindHistory, hostWindows).Verified--
				row(h, SvcKindHistory, hostWindows).Blocked++
			})
			h.Result = seal(h.Result)
			return h
		}()},
		{"sum broken", "QUERY_SCOPE_SUM", resealHost(edit(func(h *QueryScopeHostResult) { row(h, SvcKindHistory, hostWindows).Verified-- }))},
		{"failure code without a failure", "QUERY_SCOPE_FAILURE_CODE", resealHost(edit(func(h *QueryScopeHostResult) { h.Result.FirstFailure = "X" }))},
		{"foreign row verified", "QUERY_SCOPE_HOST_FOREIGN", resealHost(edit(func(h *QueryScopeHostResult) {
			c := row(h, SvcKindHistory, hostLinux)
			c.Verified, c.Blocked = 1, c.Blocked-1
		}))},
		{"null counters", "QUERY_SCOPE_DENOMINATOR", edit(func(h *QueryScopeHostResult) { h.Result.Counters = nil })},
	} {
		if _, err := EncodeQueryScopeHostResult(tc.h); err == nil || faninCode(t, err) != tc.code {
			t.Errorf("%s: encoded or wrong code: %v, want %s", tc.name, err, tc.code)
		}
		if _, err := MergeQueryScopeHostResults([]QueryScopeHostResult{tc.h}); err == nil || faninCode(t, err) != tc.code {
			t.Errorf("%s: merged or wrong code: %v, want %s", tc.name, err, tc.code)
		}
	}

	data, err := EncodeQueryScopeHostResult(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, code string
		data       []byte
	}{
		{"unknown member", "QUERY_SCOPE_JSON", bytes.Replace(data, []byte(`"host":`), []byte(`"extra":1,"host":`), 1)},
		{"truncated", "QUERY_SCOPE_JSON", data[:len(data)-1]},
		{"host text changed", "QUERY_SCOPE_HOST_FOREIGN", bytes.Replace(data, []byte(`"host":"windows-amd64"`), []byte(`"host":"linux-amd64"`), 1)},
		{"counter text changed", "QUERY_SCOPE_RESULT_DIGEST", bytes.Replace(data, []byte(`"verified":110`), []byte(`"verified":109`), 1)},
		{"digest text changed", "QUERY_SCOPE_HOST_DIGEST", bytes.Replace(data, []byte(base.HostSHA256), []byte(strings.Repeat("0", 64)), 1)},
		{"oversized", "QUERY_SCOPE_SIZE", bytes.Repeat([]byte(" "), queryScopeResultMaxBytes+2048)},
	} {
		if bytes.Equal(tc.data, data) {
			t.Fatalf("%s: the mutation changed nothing", tc.name)
		}
		if _, err := DecodeQueryScopeHostResult(tc.data); err == nil || faninCode(t, err) != tc.code {
			t.Errorf("%s: decoded or wrong code: %v, want %s", tc.name, err, tc.code)
		}
	}
}

func TestQueryScopeHostMergeRefusesIncompleteOrInconsistentSets(t *testing.T) {
	t.Parallel()
	f := faninFixtureFor(t)
	win, lin := f.hostSvc(t, hostWindows, nil), f.hostSvc(t, hostLinux, nil)
	otherAsset := func(h QueryScopeHostResult) QueryScopeHostResult {
		h.Result = cloneScope(h.Result)
		h.Result.Assets[0].Bytes++
		return resealHost(h)
	}
	otherRows := func(h QueryScopeHostResult) QueryScopeHostResult {
		h.Result = cloneScope(h.Result)
		h.Result.Counters = h.Result.Counters[1:]
		return resealHost(h)
	}
	blockedLarge := f.largeScope(t, func(j []LargeJudgment) []LargeJudgment {
		for i := range j {
			j[i] = LargeJudgment{ID: j[i].ID, State: "BLOCKED", Code: "LARGE_NOT_RUN_HERE"}
		}
		return j
	})
	largeLinux, err := NewQueryScopeHostResult(hostLinux, blockedLarge)
	if err != nil {
		t.Fatal(err)
	}
	largeWindows, err := NewQueryScopeHostResult(hostWindows, f.largeScope(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, code string
		hosts      []QueryScopeHostResult
	}{
		{"no result", "QUERY_SCOPE_HOST_ABSENT", nil},
		{"one host missing", "QUERY_SCOPE_HOST_ABSENT", []QueryScopeHostResult{win}},
		{"the same host twice", "QUERY_SCOPE_HOST_DUPLICATE", []QueryScopeHostResult{win, win}},
		{"two scopes", "QUERY_SCOPE_HOST_MISMATCH", []QueryScopeHostResult{win, f.hostPredicate(t, hostLinux)}},
		{"other evidence pin", "QUERY_SCOPE_HOST_MISMATCH", []QueryScopeHostResult{win, otherAsset(lin)}},
		{"other rows", "QUERY_SCOPE_HOST_MISMATCH", []QueryScopeHostResult{win, otherRows(lin)}},
		{"a host without rows", "QUERY_SCOPE_HOST_UNUSED", []QueryScopeHostResult{largeWindows, largeLinux}},
	} {
		if _, err := MergeQueryScopeHostResults(tc.hosts); err == nil || faninCode(t, err) != tc.code {
			t.Errorf("%s: merged or wrong code: %v, want %s", tc.name, err, tc.code)
		}
	}
	// a merged result is still pinned by the caller: another pin is refused by the fan-in
	merged, err := MergeQueryScopeHostResults([]QueryScopeHostResult{win, lin})
	if err != nil {
		t.Fatal(err)
	}
	req := f.request(merged)
	req.ResultPins[QueryScopeSvc] = win.Result.ResultSHA256
	if _, err := FoldQueryCompletion(req); err == nil || faninCode(t, err) != "QUERY_SCOPE_RESULT_PIN" {
		t.Errorf("a host result digest is not the merged result's pin: %v", err)
	}
	if QueryScopeHostFile(QueryScopeSvc, hostLinux) != "query-scope-svc-linux-amd64.json" {
		t.Error("file name")
	}
}
