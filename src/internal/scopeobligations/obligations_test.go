package scopeobligations

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func expectedFor(t *testing.T) (kit.SvcExpected, []kit.SvcJudgment) {
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
	return e, offline
}

// The per-obligation rules the merge relies on, one obligation at a time: the inline list may change an
// offline BLOCKED inline obligation and nothing else.
func TestCheck(t *testing.T) {
	e, offline := expectedFor(t)
	obligations := e.Obligations()
	tree := slices.IndexFunc(obligations, func(o kit.SvcObligation) bool { return o.Kind == kit.SvcKindInlineTree })
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
	if code := base.Check(e); code != "" {
		t.Fatalf("base refused: %s", code)
	}
	for name, c := range map[string]struct {
		edit func(d *Document)
		want string
	}{
		"inline obligation judged":                     {func(d *Document) { d.Inline[tree] = j(tree, "VERIFIED", "") }, ""},
		"inline obligation failed":                     {func(d *Document) { d.Inline[tree] = j(tree, "FAILED", "SVC_INLINE_NATIVE_TREE") }, ""},
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
		if got := d.Check(e); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func TestEncodeDecodeRoundTripsAndIsStrict(t *testing.T) {
	_, offline := expectedFor(t)
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
