// Command query-scope-merge folds the stored scope judgments of both hosts of one CI run (#362 L2).
//
// Each host hands over a closed directory: its run-identity.json, the two files
// query-scope-{predicate,svc}-<host>.json that the native fan-in test stores, and
// query-scope-svc-inline-<host>.json from the real-C#-grammar SVC test (#362 L3). The last is the same
// scope and rows as the svc file with that host's inline rows judged further; it must agree with the
// svc file everywhere else and then stands for the host's svc result. The tool binds those
// directories to the run it executes in (artifact layer: run id, attempt, commit and head SHA, the
// stored format stays tsgk-query-scope-host/r1), decodes and merges the host files, folds the merged
// scopes with FoldQueryCompletion and writes one report. It is evidence only: it opens no gate.
package main

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/scopeobligations"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

const (
	reportSchema = "tsgk-query-scope-merge/r1"
	identityFile = "run-identity.json"
	producerJob  = "native-routes"

	statusMerged  = "MERGED"  // both hosts bound, merged and folded, no FAILED row
	statusFail    = "FAIL"    // merged and folded, a host judged an obligation FAILED
	statusBlocked = "BLOCKED" // a registered host's evidence is absent, or its inline run judged less than the asset derives
	statusRefused = "REFUSED" // evidence present but not bound to this run or not intact

	maxIdentityBytes = 64 << 10
	maxHostBytes     = 512 << 10
)

// hostPairs are the registered hosts: the fan-in host name and the Go GOOS/GOARCH of its producer.
var hostPairs = map[string][2]string{"windows-amd64": {"windows", "amd64"}, "linux-amd64": {"linux", "amd64"}}

var scopes = []string{kit.QueryScopePredicate, kit.QueryScopeSvc}

// svcInlineKey names the stored svc result of the inline run next to the scopes; it is not a scope.
const svcInlineKey = "svc-inline"

func fileOf(key, host string) string {
	if key == svcInlineKey {
		return kit.QueryScopeSvcInlineFile(host)
	}
	return kit.QueryScopeHostFile(key, host)
}

// refinesOffline reports whether the inline-run svc result of a host only refines the host's offline svc
// result: every row is the same except this host's inline rows (SVC_INLINE_TREE, SVC_INLINE_QUERY), and
// those may only move obligations out of BLOCKED into judged ones. An offline VERIFIED or FAILED is never
// taken back, UNPROCESSED never changes, and the inline expected asset is pinned after the offline pins.
// Any other difference means the two runs disagree about the same judgments.
func refinesOffline(off, in kit.QueryScopeHostResult) bool {
	a, b := off.Result, in.Result
	n := len(a.Assets)
	if off.Host != in.Host || a.Scope != b.Scope || a.RegistrySHA256 != b.RegistrySHA256 || a.DenominatorSHA256 != b.DenominatorSHA256 ||
		len(a.Counters) != len(b.Counters) || len(b.Assets) != n+1 || !slices.Equal(b.Assets[:n], a.Assets) {
		return false
	}
	for i, c := range b.Counters {
		x := a.Counters[i]
		if c.Kind != x.Kind || c.Platform != x.Platform || c.Required != x.Required {
			return false
		}
		if inlineRow := c.Platform == in.Host && (c.Kind == kit.SvcKindInlineTree || c.Kind == kit.SvcKindQuery); !inlineRow {
			if c != x {
				return false
			}
		} else if c.Verified < x.Verified || c.Failed < x.Failed || c.Unprocessed != x.Unprocessed {
			return false
		}
	}
	return true
}

type fileDigest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type hostInput struct {
	Host  string       `json:"host"`
	Files []fileDigest `json:"files"`
	// InlineUnjudged counts the inline obligations the committed inline expected asset derives that the host's
	// inline run left BLOCKED (not run): a host that judged less than it could.
	InlineUnjudged int `json:"inline_unjudged"`
}

type report struct {
	Schema     string                     `json:"schema"`
	Status     string                     `json:"status"`
	Code       string                     `json:"code"`
	Run        kit.RunIdentity            `json:"run"`
	Inputs     []hostInput                `json:"inputs"`
	Scopes     map[string]string          `json:"scope_result_sha256,omitempty"`
	Completion *kit.QueryCompletionResult `json:"completion,omitempty"`
}

// refusal is a verdict that stops before the fold.
type refusal struct{ status, code string }

func (r refusal) Error() string { return r.status + " " + r.code }

func refuse(code string) error { return refusal{statusRefused, code} }

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("unreadable or over the size limit")
	}
	return data, nil
}

// readIdentity strictly decodes a run identity: exact member names (no case folding), no unknown or
// repeated member, no trailing data.
func readIdentity(data []byte) (kit.RunIdentity, error) {
	var id kit.RunIdentity
	if jsonv2.Unmarshal(data, &id, jsonv2.RejectUnknownMembers(true)) != nil || id.Schema != kit.RunIdentitySchema {
		return kit.RunIdentity{}, errors.New("run identity form")
	}
	return id, nil
}

// expectedComplete reports whether the identity of the run this tool executes in names everything a host
// identity is compared with. A blank field would let a blank host field "match" it.
func expectedComplete(id kit.RunIdentity) bool {
	for _, v := range []string{id.Repository, id.Workflow, id.RunID, id.RunAttempt, id.Event, id.SHA, id.HeadSHA, id.Checkout, id.Job, id.GOOS, id.GOARCH} {
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	return id.Schema == kit.RunIdentitySchema && id.EvidenceMode == "NEW_RUN"
}

// bound reports whether a host's identity belongs to the run this tool executes in: the same
// repository, workflow, run, attempt, event, commit and head SHA, a NEW_RUN of the producer job on
// the host the directory is named for.
func bound(got, want kit.RunIdentity, host string) bool {
	pair := hostPairs[host]
	return got.Repository == want.Repository && got.Workflow == want.Workflow && got.RunID == want.RunID && got.RunAttempt == want.RunAttempt &&
		got.Event == want.Event && got.SHA == want.SHA && got.HeadSHA == want.HeadSHA && got.Checkout == want.Checkout &&
		got.EvidenceMode == "NEW_RUN" && got.Job == producerJob && got.GOOS == pair[0] && got.GOARCH == pair[1]
}

func wantedFiles(host string) []string {
	names := []string{identityFile}
	for _, key := range append(slices.Clone(scopes), svcInlineKey) {
		names = append(names, fileOf(key, host))
	}
	names = append(names, scopeobligations.FileName(host))
	slices.Sort(names)
	return names
}

// loadHost reads one host directory: exactly its identity and its two scope files, no other
// entry. The decoded results are returned per scope, and a file under another host's name is refused.
func loadHost(dir, host string, expected kit.RunIdentity, c contracts) (hostInput, map[string]kit.QueryScopeHostResult, error) {
	in := hostInput{Host: host}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return in, nil, refusal{statusBlocked, "QUERY_SCOPE_HOST_ABSENT"}
	} else if err != nil { // present but unreadable (permission, not a directory): not the same as absent
		return in, nil, refuse("QUERY_SCOPE_HOST_FILES")
	}
	var names []string
	for _, e := range entries {
		if !e.Type().IsRegular() {
			return in, nil, refuse("QUERY_SCOPE_HOST_FILES")
		}
		names = append(names, e.Name())
	}
	if !slices.Equal(names, wantedFiles(host)) { // ReadDir is sorted by name
		return in, nil, refuse("QUERY_SCOPE_HOST_FILES")
	}
	read := map[string][]byte{}
	for _, name := range names {
		limit := int64(maxHostBytes)
		if name == identityFile {
			limit = maxIdentityBytes
		}
		data, err := readBounded(filepath.Join(dir, name), limit)
		if err != nil {
			return in, nil, refuse("QUERY_SCOPE_HOST_FILES")
		}
		read[name] = data
		in.Files = append(in.Files, fileDigest{name, queryregistrycore.Digest(data)})
	}
	id, err := readIdentity(read[identityFile])
	if err != nil {
		return in, nil, refuse("QUERY_SCOPE_RUN_IDENTITY")
	}
	if !bound(id, expected, host) {
		return in, nil, refuse("QUERY_SCOPE_RUN_MISMATCH")
	}
	results := map[string]kit.QueryScopeHostResult{}
	for _, key := range append(slices.Clone(scopes), svcInlineKey) {
		h, err := kit.DecodeQueryScopeHostResult(read[fileOf(key, host)])
		if err != nil {
			return in, nil, refuse(codeOf(err))
		}
		scope := key
		if key == svcInlineKey {
			scope = kit.QueryScopeSvc
		}
		if h.Host != host || h.Result.Scope != scope {
			return in, nil, refuse("QUERY_SCOPE_HOST_MISMATCH")
		}
		results[key] = h
	}
	// the inline run's result is the same scope and rows: it stands for the host's svc result
	if !refinesOffline(results[kit.QueryScopeSvc], results[svcInlineKey]) {
		return in, nil, refuse("QUERY_SCOPE_SVC_INLINE_MISMATCH")
	}
	// the per-obligation companion: both stored results recomputed from it, the inline run changing only
	// what the offline run left BLOCKED
	code, unjudged := c.checkObligations(read[scopeobligations.FileName(host)], host, results)
	if code != "" {
		return in, nil, refuse(code)
	}
	in.InlineUnjudged = unjudged
	results[kit.QueryScopeSvc] = results[svcInlineKey]
	return in, results, nil
}

// contracts are the committed contracts of the run's checkout: the fold request, the parsed SVC
// denominator and the inline expected asset's per-obligation codes (see Document.Check) the per-obligation
// checks run against.
type contracts struct {
	req    kit.QueryCompletionRequest
	e      kit.SvcExpected
	inline map[string]string
}

func loadContracts(root string) (contracts, error) {
	req, err := foldRequest(root)
	if err != nil {
		return contracts{}, err
	}
	e, err := kit.ParseSvcExpected(req.SvcExpected, req.Registry, req.Qualification, req.SvcFixture)
	if err != nil {
		return contracts{}, err
	}
	// the asset the inline pin names, parsed against the pinned sources: which obligations are derivable
	pack, err := os.ReadFile(filepath.Join(root, "src", "contracts", "fact-query-pack.json"))
	if err != nil {
		return contracts{}, err
	}
	asset, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(kit.SvcInlinePath)))
	if err != nil {
		return contracts{}, err
	}
	x, err := kit.ParseSvcInlineExpected(asset, req.SvcExpected, req.Registry, req.Qualification, req.SvcFixture, pack)
	return contracts{req, e, x.InlineCodes()}, err
}

// checkObligations validates a host's companion file (scopeobligations): it is the host's, lists exactly
// the SVC obligations, refines per obligation (Document.Check), and recomputes to the two stored svc
// results of the host, the offline one and the inline one. It returns the refusal code, "" when sound, and
// the number of derivable inline obligations the host's inline run left unjudged.
func (c contracts) checkObligations(data []byte, host string, results map[string]kit.QueryScopeHostResult) (string, int) {
	d, err := scopeobligations.Decode(data)
	if err != nil || d.Host != host {
		return scopeobligations.CodeShape, 0
	}
	if code := d.Check(c.e, c.inline); code != "" {
		return code, 0
	}
	blocked := map[string]bool{}
	for h := range hostPairs {
		blocked[h] = h != host
	}
	recompute := func(js []kit.SvcJudgment, extra ...queryregistrycore.Pin) string {
		r, err := kit.SvcScopeResult(c.e, c.req.SvcExpected, c.req.Registry, host, js, blocked)
		if err != nil {
			return ""
		}
		return kit.NewQueryScopeResult(r.Scope, r.RegistrySHA256, append(slices.Clone(r.Assets), extra...), r.Counters, r.FirstFailure).ResultSHA256
	}
	if recompute(d.OfflineJudgments()) != results[kit.QueryScopeSvc].Result.ResultSHA256 ||
		recompute(d.InlineJudgments(), c.req.AssetPins[kit.QueryScopeSvc][1:]...) != results[svcInlineKey].Result.ResultSHA256 {
		return scopeobligations.CodeShape, 0
	}
	return "", d.Unjudged(c.inline)
}

func codeOf(err error) string {
	var e *kit.Error
	var f *queryregistrycore.Fault
	switch {
	case errors.As(err, &e):
		return e.Code
	case errors.As(err, &f):
		return f.Code
	}
	return "QUERY_SCOPE_HOST_INVALID"
}

// foldRequest reads the committed contracts the fan-in pins, as the checkout of this run holds them.
func foldRequest(root string) (kit.QueryCompletionRequest, error) {
	read := func(path string) []byte {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil
		}
		return data
	}
	registry := read(queryregistrycore.Path)
	predicate, svc, large, svcInline := read(kit.PredicateExpectedPath), read(kit.SvcExpectedPath), read(kit.LargeExpectedPath), read(kit.SvcInlinePath)
	req := kit.QueryCompletionRequest{
		Registry: registry, RegistrySHA256: queryregistrycore.Digest(registry),
		Qualification: read(queryregistrycore.QualificationPin.Path), Slots: read(queryregistrycore.SlotPin.Path),
		Basis:       map[string][]byte{"src/dev/query-reference/check.py": read("src/dev/query-reference/check.py"), "src/internal/native/query_policy_test.go": read("src/internal/native/query_policy_test.go")},
		SvcExpected: svc, SvcFixture: read("src/testdata/native/n461/svc.json"), LargeExpected: large, LargeFixtures: read("src/contracts/native-large-fixtures.json"),
		AssetPins: map[string][]queryregistrycore.Pin{
			kit.QueryScopePredicate: {queryregistrycore.FilePin(kit.PredicateExpectedPath, predicate)},
			kit.QueryScopeSvc:       {queryregistrycore.FilePin(kit.SvcExpectedPath, svc), queryregistrycore.FilePin(kit.SvcInlinePath, svcInline)},
			kit.QueryScopeLarge:     {queryregistrycore.FilePin(kit.LargeExpectedPath, large)},
		},
		ResultPins: map[string]string{},
	}
	for _, b := range [][]byte{registry, predicate, svc, svcInline, large, req.Qualification, req.Slots, req.SvcFixture, req.LargeFixtures} {
		if len(b) == 0 {
			return req, errors.New("committed contract missing")
		}
	}
	for path, b := range req.Basis {
		if len(b) == 0 {
			return req, errors.New("fan-in basis missing: " + path)
		}
	}
	return req, nil
}

// merge binds the host directories to the run, merges both hosts of each scope and folds the result.
func merge(root string, expected kit.RunIdentity, dirs map[string]string) report {
	rep := report{Schema: reportSchema, Run: expected, Inputs: []hostInput{}}
	fail := func(err error) report {
		var r refusal
		if errors.As(err, &r) {
			rep.Status, rep.Code = r.status, r.code
		} else {
			rep.Status, rep.Code = statusRefused, "QUERY_SCOPE_MERGE_FAILED"
		}
		return rep
	}
	if !expectedComplete(expected) {
		return fail(refuse("QUERY_SCOPE_RUN_EXPECTED"))
	}
	c, err := loadContracts(root)
	if err != nil {
		return fail(refuse("QUERY_SCOPE_CONTRACT_MISSING"))
	}
	byScope := map[string][]kit.QueryScopeHostResult{}
	var absent error
	for _, host := range []string{"windows-amd64", "linux-amd64"} {
		dir, ok := dirs[host]
		if !ok {
			absent = refusal{statusBlocked, "QUERY_SCOPE_HOST_ABSENT"}
			continue
		}
		in, results, err := loadHost(dir, host, expected, c)
		rep.Inputs = append(rep.Inputs, in)
		var r refusal
		if errors.As(err, &r) && r.status == statusBlocked {
			absent = err
			continue
		}
		if err != nil {
			return fail(err)
		}
		for _, scope := range scopes {
			byScope[scope] = append(byScope[scope], results[scope])
		}
	}
	if absent != nil { // a present host is not folded alone: the other host stays unjudged
		return fail(absent)
	}
	req := c.req
	rep.Scopes = map[string]string{}
	for _, scope := range scopes {
		merged, err := kit.MergeQueryScopeHostResults(byScope[scope])
		if err != nil {
			return fail(refuse(codeOf(err)))
		}
		req.Scopes = append(req.Scopes, merged)
		req.ResultPins[scope] = merged.ResultSHA256 // the pin is the merged digest itself: only the checks above give it meaning
		rep.Scopes[scope] = merged.ResultSHA256
	}
	res, err := kit.FoldQueryCompletion(req)
	if err != nil {
		return fail(refuse(codeOf(err)))
	}
	rep.Completion = &res
	rep.Status, rep.Code = statusMerged, res.Code
	switch {
	case res.Total.Failed != 0:
		rep.Status = statusFail
	case slices.ContainsFunc(rep.Inputs, func(in hostInput) bool { return in.InlineUnjudged != 0 }):
		// folded and sound, but not complete: a host's inline run judged less than the asset derives
		rep.Status, rep.Code = statusBlocked, "QUERY_SCOPE_SVC_INLINE_UNDERFILLED"
	}
	return rep
}

type hostFlags map[string]string

func (h hostFlags) String() string { return fmt.Sprint(map[string]string(h)) }

func (h hostFlags) Set(v string) error {
	host, dir, ok := strings.Cut(v, "=")
	if _, known := hostPairs[host]; !ok || !known || dir == "" {
		return errors.New("want <windows-amd64|linux-amd64>=<directory>")
	}
	if _, dup := h[host]; dup {
		return errors.New("duplicate host " + host)
	}
	h[host] = dir
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root of the run's checkout")
	expectedPath := flag.String("expected", "", "this run's run-identity.json")
	out := flag.String("out", "", "report file to create")
	hosts := hostFlags{}
	flag.Var(hosts, "host", "<host>=<artifact directory>; once per host")
	flag.Parse()
	if *expectedPath == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "-expected and -out are required")
		os.Exit(2)
	}
	data, err := readBounded(*expectedPath, maxIdentityBytes)
	var expected kit.RunIdentity
	if err == nil {
		expected, err = readIdentity(data)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "expected run identity unreadable")
		os.Exit(2)
	}
	rep := merge(*root, expected, hosts)
	encoded, err := json.MarshalIndent(rep, "", "  ")
	if err == nil {
		var f *os.File
		if f, err = os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644); err == nil {
			_, err = f.Write(append(encoded, '\n'))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "report not written:", err)
		os.Exit(2)
	}
	fmt.Printf("query-scope-merge %s %s\n", rep.Status, rep.Code)
	if rep.Completion != nil {
		t := rep.Completion.Total
		fmt.Printf("TOTAL required=%d verified=%d failed=%d blocked=%d unprocessed=%d\n", t.Required, t.Verified, t.Failed, t.Blocked, t.Unprocessed)
		for _, h := range rep.Completion.Hosts {
			fmt.Printf("HOST %s verified=%d failed=%d blocked=%d\n", h.Platform, h.Verified, h.Failed, h.Blocked)
		}
	}
	if rep.Status != statusMerged {
		os.Exit(1)
	}
}
