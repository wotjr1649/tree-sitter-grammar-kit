package kit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
)

// Discovery, manifest and closure values.
const (
	DiscoveryPolicy = "known-paths-r1"
	ManifestSchema  = "tsgk-manifest/r2"
	FileSetSchema   = "tsgk-files/r2"
	ModePolicy      = "portable-default"

	StateFound       = "FOUND"
	StateNotFound    = "NOT_FOUND"
	StateUnreadable  = "UNREADABLE"
	StateUnsupported = "UNSUPPORTED"

	ClosureObserved       = "OBSERVED"
	ClosureUnresolved     = "UNRESOLVED"
	ClosureNotApplicable  = "NOT_APPLICABLE"
	ClosureCallerSelected = "CALLER_SELECTED"
)

var roles = map[string]bool{"grammar": true, "generated": true, "scanner": true, "query": true, "corpus": true, "metadata": true}

// InspectRequest selects a root and grammar for zero-config inventory.
type InspectRequest struct {
	Root      string
	Selection Selection
	Limits    Limits
}

// IdentityRequest selects files whose bytes are bound into manifest r2.
type IdentityRequest struct {
	Root             string
	Selection        Selection
	Limits           Limits
	Encoding         EncodingPolicy
	LargeFileProfile string // "" or a registered identity-scoped exception such as "pg-large-source-r1"
}

// GrammarCandidate is a grammar directory named by metadata; it is an observation.
type GrammarCandidate struct {
	Path   string `json:"path"`
	Name   string `json:"name,omitempty"`
	Source string `json:"source"`
}

// InventoryEntry is one known or discovered path. Size is nil when not observed.
type InventoryEntry struct {
	Path  string  `json:"path"`
	Role  string  `json:"role"`
	State string  `json:"state"`
	Basis string  `json:"basis"`
	Size  *uint64 `json:"size"`
}

// InventoryResult is the inspect result; Report fields are the E0 envelope.
type InventoryResult struct {
	Report
	Policy       Policy             `json:"policy"`
	Grammar      string             `json:"grammar"`
	Grammars     []GrammarCandidate `json:"grammars"`
	Entries      []InventoryEntry   `json:"entries"`
	ClosureState string             `json:"closure_state"`
}

// IdentityResult binds the selected file set. SetSHA256 is empty unless every selected
// file was read completely.
type IdentityResult struct {
	Report
	Policy       Policy   `json:"policy"`
	Grammar      string   `json:"grammar"`
	ClosureState string   `json:"closure_state"`
	Manifest     Manifest `json:"manifest"`
	SetSHA256    string   `json:"set_sha256"`
}

// Manifest is the versioned file identity list (tsgk-manifest/r2).
type Manifest struct {
	Schema         string         `json:"schema"`
	Algorithm      string         `json:"algorithm"`
	ModePolicy     string         `json:"mode_policy"`
	EncodingPolicy string         `json:"encoding_policy"`
	Files          []FileIdentity `json:"files"`
}

// FileIdentity is one file record; Encoding is bound into the set identity.
type FileIdentity struct {
	Path           string          `json:"path"`
	Role           string          `json:"role"`
	Mode           string          `json:"mode"`
	ModeProvenance string          `json:"mode_provenance"`
	Size           uint64          `json:"size"`
	SHA256         string          `json:"sha256"`
	Encoding       EncodingOutcome `json:"encoding"`
}

type largeFileProfile struct {
	limit uint64
	ids   map[string]uint64 // sha256 -> exact size
}

// Identity-scoped exceptions registered in docs/specs/cli-and-profile.md.
var largeFileProfiles = map[string]largeFileProfile{
	"pg-large-source-r1": {limit: 104857600, ids: map[string]uint64{
		"a9090d5082ae5c23892d05aa59e61476f9bd39ad634228f2046024debdf815b5": 97664793,
		"cc47959aac26b9e749883dac2fb2852dcb7efd895d51e4b19d38d4b1a3528f7d": 97664835,
	}},
}

type entry struct {
	InventoryEntry
	info fs.FileInfo
}

type inventory struct {
	grammar  string
	entries  map[string]*entry
	grammars []GrammarCandidate
	findings []Finding
	closure  string
	jsSeen   bool
	budget   uint64
}

func (inv *inventory) finding(code, severity, p, msg string) {
	inv.findings = append(inv.findings, Finding{Code: code, Severity: severity, Path: p, Message: msg})
}

func (inv *inventory) sorted() []InventoryEntry {
	out := make([]InventoryEntry, 0, len(inv.entries))
	for _, e := range inv.entries {
		out = append(out, e.InventoryEntry)
	}
	slices.SortFunc(out, func(a, b InventoryEntry) int { return strings.Compare(a.Path, b.Path) })
	return out
}

// Inspect reports a bounded, zero-config inventory of the selected grammar without
// running any of its code. An unknown layout is an observation, not a failure.
func Inspect(ctx context.Context, req InspectRequest) (InventoryResult, error) {
	res := InventoryResult{Report: newReport("inspect"), Grammars: []GrammarCandidate{}, Entries: []InventoryEntry{}}
	res.Policy = grammarPolicy(req.Limits, "", "")
	inv, r, err := prepare(ctx, req.Root, req.Selection, req.Limits)
	if r != nil {
		defer r.cancel()
	}
	if err != nil {
		failReport(&res.Report, err)
		return res, err
	}
	inv.root.Close()
	res.Grammar, res.Grammars, res.Entries, res.ClosureState = inv.grammar, inv.grammars, inv.sorted(), inv.closure
	res.Findings = append(res.Findings, inv.findings...)
	res.Identities = append(res.Identities, res.Policy.ref())
	res.Coverage = coverage(inv.inventory)
	if err := sealOutput(res, req.Limits.OutputBytes); err != nil {
		out := InventoryResult{Report: newReport("inspect"), Policy: res.Policy, Grammars: []GrammarCandidate{}, Entries: []InventoryEntry{}}
		failReport(&out.Report, err)
		return out, err
	}
	return res, nil
}

// Identity reads every selected file completely and binds manifest r2.
func Identity(ctx context.Context, req IdentityRequest) (IdentityResult, error) {
	res := IdentityResult{Report: newReport("identity"), Manifest: Manifest{Schema: ManifestSchema, Algorithm: "sha256", ModePolicy: ModePolicy, Files: []FileIdentity{}}}
	encPolicy := "detect-r1"
	if req.Encoding.Profile == "cp949" {
		encPolicy += ";profile=cp949"
	}
	res.Manifest.EncodingPolicy = encPolicy
	res.Policy = grammarPolicy(req.Limits, req.LargeFileProfile, encPolicy)
	failWith := func(e *Error) (IdentityResult, error) {
		out := IdentityResult{Report: newReport("identity"), Policy: res.Policy, Manifest: Manifest{Schema: ManifestSchema, Algorithm: "sha256", ModePolicy: ModePolicy, EncodingPolicy: encPolicy, Files: []FileIdentity{}}}
		failReport(&out.Report, e)
		return out, e
	}
	profile, ok := largeFileProfiles[req.LargeFileProfile]
	if req.LargeFileProfile != "" && !ok {
		return failWith(fail(KindInvalidInput, "LARGE_FILE_PROFILE_UNKNOWN", "", nil))
	}
	declared, e := declarations(req.Encoding)
	if e != nil {
		return failWith(e)
	}
	inv, r, e := prepare(ctx, req.Root, req.Selection, req.Limits)
	if r != nil {
		defer r.cancel()
	}
	if e != nil {
		return failWith(e)
	}
	defer inv.root.Close()
	if req.Selection.Files == nil && inv.closure == ClosureNotApplicable {
		// Metadata alone is not a grammar identity; an unknown layout stays an inspect observation.
		return failWith(fail(KindInvalidInput, "NO_GRAMMAR_SELECTED", inv.grammar, nil))
	}
	var selected []*entry
	folded := map[string]string{}
	for _, view := range inv.sorted() { // byte order keeps the reported path deterministic
		item := inv.entries[view.Path]
		switch item.State {
		case StateFound:
			if prev, dup := folded[strings.ToLower(item.Path)]; dup {
				return failWith(fail(KindInvalidInput, "PATH_CASE_COLLISION", prev, nil))
			}
			folded[strings.ToLower(item.Path)] = item.Path
			selected = append(selected, item)
		case StateUnsupported:
			return failWith(fail(KindInvalidInput, "LINK_OR_SPECIAL_REJECTED", item.Path, nil))
		case StateNotFound:
			if req.Selection.Files != nil { // an explicit member may not silently disappear
				return failWith(fail(KindInvalidInput, "SELECTED_FILE_NOT_FOUND", item.Path, nil))
			}
		}
	}
	if len(selected) == 0 {
		return failWith(fail(KindInvalidInput, "EMPTY_SELECTION", "", nil))
	}
	for p := range declared {
		if item := inv.entries[p]; item == nil || item.State != StateFound {
			return failWith(fail(KindInvalidInput, "DECLARATION_UNMATCHED", p, nil))
		}
	}
	g := newGuard(inv.root)
	hardMax := req.Limits.FileBytes
	if ok {
		hardMax = max(hardMax, profile.limit)
	}
	for _, item := range selected {
		st, e := g.readFile(r, item.Path, item.info, hardMax, req.Limits.TotalBytes, 0)
		if e != nil {
			return failWith(e)
		}
		if st.size > req.Limits.FileBytes {
			if size, hit := profile.ids[st.sha256]; !ok || !hit || size != st.size {
				return failWith(fail(KindResourceLimit, "FILE_BYTES_LIMIT", item.Path, nil))
			}
			inv.finding("LARGE_FILE_EXCEPTION", "info", item.Path, "등록된 identity 한정 대용량 예외("+req.LargeFileProfile+")로 읽었다")
		}
		outcome := st.encoding.result(req.Encoding.Profile == "cp949", declared[item.Path])
		if outcome.Assessment != "PASS" {
			inv.finding("ENCODING_"+outcome.Assessment, "warning", item.Path, "encoding 판정: "+outcome.Code)
		}
		res.Manifest.Files = append(res.Manifest.Files, FileIdentity{Path: item.Path, Role: item.Role, Mode: "100644", ModeProvenance: "POLICY_DEFAULT", Size: st.size, SHA256: st.sha256, Encoding: outcome})
	}
	res.Grammar, res.ClosureState = inv.grammar, inv.closure
	res.SetSHA256 = setSHA256(res.Manifest)
	res.Findings = append(res.Findings, inv.findings...)
	res.Identities = append(res.Identities, IdentityRef{Role: "source-set", Schema: FileSetSchema, SHA256: res.SetSHA256}, res.Policy.ref())
	res.Coverage = coverage(inv.inventory)
	if e := sealOutput(res, req.Limits.OutputBytes); e != nil {
		return failWith(e)
	}
	return res, nil
}

// setSHA256 hashes the domain-separated r2 preimage (docs/specs/identity-and-evidence.md).
func setSHA256(m Manifest) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n%s\n", FileSetSchema, m.ModePolicy, m.EncodingPolicy)
	for _, f := range m.Files {
		e := f.Encoding
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s\n", f.Path, f.Role, f.Mode, f.ModeProvenance, f.Size, f.SHA256, e.Assessment, e.Encoding, e.Source, e.Code)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func declarations(p EncodingPolicy) (map[string]string, *Error) {
	if p.Profile != "" && p.Profile != "cp949" {
		return nil, fail(KindInvalidInput, "ENCODING_PROFILE_INVALID", "", nil)
	}
	out := map[string]string{}
	for _, d := range p.Files {
		if !portable(d.Path, false) || (d.Encoding != "utf-8" && d.Encoding != "cp949") {
			return nil, fail(KindInvalidInput, "ENCODING_DECLARATION_INVALID", d.Path, nil)
		}
		if _, dup := out[d.Path]; dup {
			return nil, fail(KindInvalidInput, "ENCODING_DECLARATION_DUPLICATE", d.Path, nil)
		}
		out[d.Path] = d.Encoding
	}
	return out, nil
}

func grammarPolicy(l Limits, large, enc string) Policy {
	return Policy{Operation: "offline-inspect", Discovery: DiscoveryPolicy, Files: l.Files, FileBytes: l.FileBytes, TotalBytes: l.TotalBytes,
		Depth: l.Depth, OutputBytes: l.OutputBytes, WallMillis: l.Wall.Milliseconds(), LargeFileProfile: large, EncodingPolicy: enc,
		Exclusions: []string{"outside-known-paths", "links-not-followed", "no-parent-discovery", "no-ignore-files"}}
}

func coverage(inv *inventory) Coverage {
	c := Coverage{Requested: []string{"corpus", "generated", "grammar", "metadata", "query", "scanner"}, Observed: []string{}, Unsupported: []string{}}
	seen := map[string]bool{}
	for _, e := range inv.entries {
		if e.State == StateFound && !seen[e.Role] {
			seen[e.Role] = true
			c.Observed = append(c.Observed, e.Role)
		}
	}
	slices.Sort(c.Observed)
	if inv.jsSeen {
		c.Unsupported = append(c.Unsupported, "js-closure-proof")
	}
	return c
}

type inventoryWithRoot struct {
	*inventory
	root *os.Root
}

// prepare validates the request at the shared core boundary and discovers the selection.
func prepare(ctx context.Context, root string, sel Selection, limits Limits) (*inventoryWithRoot, *run, *Error) {
	if !limits.valid() {
		return nil, nil, fail(KindInvalidInput, "LIMITS_INVALID", "", nil)
	}
	if !portable(sel.Grammar, true) {
		return nil, nil, fail(KindInvalidInput, "GRAMMAR_INVALID", "", nil)
	}
	if sel.Files != nil && len(sel.Files) == 0 {
		return nil, nil, fail(KindInvalidInput, "EMPTY_SELECTION", "", nil)
	}
	r, e := startRun(ctx, limits.Wall)
	if e != nil {
		return nil, nil, e
	}
	osRoot, _, e := openRoot(root)
	if e != nil {
		return nil, r, e
	}
	inv := &inventoryWithRoot{inventory: &inventory{grammar: sel.Grammar, entries: map[string]*entry{}, grammars: []GrammarCandidate{}}, root: osRoot}
	g := newGuard(osRoot)
	if e := discover(r, g, inv.inventory, sel, limits); e != nil {
		osRoot.Close()
		return nil, r, e
	}
	return inv, r, nil
}
