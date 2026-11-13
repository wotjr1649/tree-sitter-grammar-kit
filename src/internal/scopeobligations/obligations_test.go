package scopeobligations

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func expectedFor(t *testing.T) (kit.SvcExpected, []kit.SvcJudgment, map[string]string) {
	t.Helper()
	read := func(p string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	inventory := read("src/contracts/qualification-c1.json")
	e, err := kit.ParseSvcExpected(read(kit.SvcExpectedPath), read("src/contracts/query-semantic-registry-r1.json"), inventory, read("src/testdata/native/n461/svc.json"))
	if err != nil {
		t.Fatal(err)
	}
	offline, err := kit.VerifySvcExpected(e, inventory)
	if err != nil {
		t.Fatal(err)
	}
	x, err := kit.ParseSvcInlineExpected(read(kit.SvcInlinePath), read(kit.SvcExpectedPath), read("src/contracts/query-semantic-registry-r1.json"), inventory,
		read("src/testdata/native/n461/svc.json"), read("src/contracts/fact-query-pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	return e, offline, x.InlineCodes()
}

// The per-obligation rules the merge relies on, one obligation at a time: the inline list may change an
// offline BLOCKED inline obligation and nothing else, and only one the asset derives (#362 F1).
func TestCheck(t *testing.T) {
	e, offline, codes := expectedFor(t)
	obligations := e.Obligations()
	derivable := func(kind string, want bool) int {
		return slices.IndexFunc(obligations, func(o kit.SvcObligation) bool { return o.Kind == kind && (codes[o.ID] == "") == want })
	}
	tree, blockedTree, query, blockedQuery := derivable(kit.SvcKindInlineTree, true), derivable(kit.SvcKindInlineTree, false), derivable(kit.SvcKindQuery, true), derivable(kit.SvcKindQuery, false)
	if tree < 0 || blockedTree < 0 || query < 0 || blockedQuery < 0 {
		t.Fatal("the asset must hold derivable and blocked inline obligations of both kinds")
	}
	var plain []int // obligations the inline run may not change
	for i, o := range obligations {
		if o.Kind != kit.SvcKindInlineTree && o.Kind != kit.SvcKindQuery {
			plain = append(plain, i)
		}
	}
	history, failed, blocked := plain[0], plain[1], plain[2]
	j := func(i int, state, code string) Judgment { return Judgment{obligations[i].ID, state, code} }
	// the offline list as the offline run judges it, plus one synthetic offline FAILED and one BLOCKED non-inline obligation
	base := New("windows-amd64", offline, offline)
	base.Offline[failed] = j(failed, "FAILED", "SVC_X")
	base.Inline[failed] = base.Offline[failed]
	base.Offline[blocked] = j(blocked, "BLOCKED", "SVC_B")
	base.Inline[blocked] = base.Offline[blocked]
	if code := base.Check(e, codes); code != "" {
		t.Fatalf("base refused: %s", code)
	}
	for name, c := range map[string]struct {
		edit func(d *Document)
		want string
	}{
		"inline obligation judged":                      {func(d *Document) { d.Inline[tree] = j(tree, "VERIFIED", "") }, ""},
		"inline obligation failed":                      {func(d *Document) { d.Inline[tree] = j(tree, "FAILED", "SVC_INLINE_NATIVE_TREE") }, ""},
		"derivable query judged":                        {func(d *Document) { d.Inline[query] = j(query, "VERIFIED", "") }, ""},
		"derivable obligation reported as not run":      {func(d *Document) { d.Inline[tree] = j(tree, "BLOCKED", kit.SvcInlineNativeNotRunCode) }, ""},
		"derivable obligation blocked for another code": {func(d *Document) { d.Inline[tree] = j(tree, "BLOCKED", "SVC_ELSE") }, CodeInline},
		"blocked tree kept with its registered code": {func(d *Document) {
			d.Inline[blockedTree] = j(blockedTree, "BLOCKED", codes[obligations[blockedTree].ID])
		}, ""},
		"blocked query kept with its registered code": {func(d *Document) {
			d.Inline[blockedQuery] = j(blockedQuery, "BLOCKED", codes[obligations[blockedQuery].ID])
		}, ""},
		"blocked tree judged VERIFIED":   {func(d *Document) { d.Inline[blockedTree] = j(blockedTree, "VERIFIED", "") }, CodeInline},
		"blocked tree judged FAILED":     {func(d *Document) { d.Inline[blockedTree] = j(blockedTree, "FAILED", "SVC_INLINE_NATIVE_TREE") }, CodeInline},
		"blocked query judged VERIFIED":  {func(d *Document) { d.Inline[blockedQuery] = j(blockedQuery, "VERIFIED", "") }, CodeInline},
		"blocked tree with another code": {func(d *Document) { d.Inline[blockedTree] = j(blockedTree, "BLOCKED", "SVC_ELSE") }, CodeInline},
		"offline tree VERIFIED (no grammar offline)": {func(d *Document) {
			d.Offline[tree], d.Inline[tree] = j(tree, "VERIFIED", ""), j(tree, "VERIFIED", "")
		}, CodeInline},
		"offline query BLOCKED for another code": {func(d *Document) {
			d.Offline[query], d.Inline[query] = j(query, "BLOCKED", "SVC_ELSE"), j(query, "BLOCKED", "SVC_ELSE")
		}, CodeInline},
		"offline VERIFIED taken back":                  {func(d *Document) { d.Inline[history] = j(history, "BLOCKED", "SVC_X") }, CodeInline},
		"offline FAILED judged VERIFIED":               {func(d *Document) { d.Inline[failed] = j(failed, "VERIFIED", "") }, CodeInline},
		"offline FAILED code changed":                  {func(d *Document) { d.Inline[failed] = j(failed, "FAILED", "SVC_OTHER") }, CodeInline},
		"offline BLOCKED non-inline obligation judged": {func(d *Document) { d.Inline[blocked] = j(blocked, "VERIFIED", "") }, CodeInline},
		"offline BLOCKED non-inline code changed":      {func(d *Document) { d.Inline[blocked] = j(blocked, "BLOCKED", "SVC_OTHER") }, CodeInline},
		"inline obligation dropped":                    {func(d *Document) { d.Inline = d.Inline[:len(d.Inline)-1] }, CodeShape},
		"offline obligation dropped":                   {func(d *Document) { d.Offline = d.Offline[:len(d.Offline)-1] }, CodeShape},
		"obligation of another ID":                     {func(d *Document) { d.Inline[tree].ID = "SVC_OTHER/0" }, CodeShape},
		"unknown offline state":                        {func(d *Document) { d.Offline[tree].State = "PASSED" }, CodeShape},
		"unknown inline state":                         {func(d *Document) { d.Inline[tree].State = "PASSED" }, CodeShape},
	} {
		d := base
		d.Offline, d.Inline = slices.Clone(base.Offline), slices.Clone(base.Inline)
		c.edit(&d)
		if got := d.Check(e, codes); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	// an inline obligation the asset does not name at all
	if got := base.Check(e, map[string]string{}); got != CodeShape {
		t.Errorf("asset without the inline obligations: %q", got)
	}
}

// Unjudged counts the derivable inline obligations a run left BLOCKED.
func TestUnjudged(t *testing.T) {
	e, offline, codes := expectedFor(t)
	d := New("windows-amd64", offline, offline)
	derived := 0
	for _, c := range codes {
		if c == "" {
			derived++
		}
	}
	if derived == 0 || d.Unjudged(codes) != derived {
		t.Fatalf("untouched inline list: %d unjudged, %d derivable", d.Unjudged(codes), derived)
	}
	for i, o := range e.Obligations() {
		if code, ok := codes[o.ID]; ok && code == "" {
			d.Inline[i] = Judgment{o.ID, "VERIFIED", ""}
		}
	}
	if d.Unjudged(codes) != 0 {
		t.Fatalf("every derivable obligation judged: %d unjudged", d.Unjudged(codes))
	}
}

func TestEncodeDecodeRoundTripsAndIsStrict(t *testing.T) {
	_, offline, _ := expectedFor(t)
	d := New("linux-amd64", offline, offline)
	data, err := Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(data)
	if err != nil || back.Host != d.Host || !slices.Equal(back.Offline, d.Offline) || !slices.Equal(back.Inline, d.Inline) {
		t.Fatalf("round trip: %v", err)
	}
	for name, bad := range map[string]string{"unknown member": `{"schema":"` + Schema + `","extra":1}`, "other schema": `{"schema":"x"}`, "trailing": string(data) + `{}`, "empty": ""} {
		if _, err := Decode([]byte(bad)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
