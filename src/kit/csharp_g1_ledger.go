package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// G1 obligation ledger for the C# support scope declared by grammar-kit (#233, G1-1).
//
// The ledger spells out, from the repository's own declarations alone, every obligation of the
// declared scope: 15 rows x required kind -> 1,531 production/fact alternatives -> registered case
// and step -> the node types the case already declares. It holds no expected value: every entry is
// NO_EXPECTED until a later stage seals an independent expectation. It never reads native output.

const (
	CSharpG1LedgerSchema = "tsgk-csharp-g1-ledger/r1"
	CSharpG1LedgerPath   = "src/contracts/csharp-g1-ledger-r1.json"
	// shortcut: the asset is about 1 MiB today; raise only with a new ledger revision.
	csharpG1MaxBytes = 4 << 20

	// Obligation states. Only NO_EXPECTED is derivable now: SEALED and EXECUTED need a sealed
	// expected asset (G1-2 and later) and are rejected until the ledger revision admits them.
	G1NoExpected = "NO_EXPECTED"
	G1Sealed     = "SEALED"
	G1Executed   = "EXECUTED"

	// Presence of an alternative in the registered case sources, by static pattern only.
	G1PresenceMatch      = "LITERAL_MATCH"       // every required literal occurs, in order, in a linked P step source
	G1PresenceAbsent     = "LITERAL_ABSENT"      // a pattern exists but no linked P step source holds it
	G1PresenceUnverified = "UNVERIFIED_PRESENCE" // no static pattern can be derived from the declaration
	g1StrengthKeyword    = "KEYWORD"             // a matched literal is a word, so it names the construct
	g1StrengthPunct      = "PUNCTUATION"         // only punctuation matched: it proves little
	g1StrengthFragment   = "FRAGMENT"            // a lexical fragment matched as a bare substring: it proves little
	g1SourceLimit        = 1 << 24               // upper bound of one case source in bytes
	g1Route              = "csharp"
	g1MappingSchema      = "tsgk-fact-mapping/r1"
	g1AlternativesSchema = "tsgk-feature-alternatives/r1"
	g1FixtureSchema      = "tsgk-native-route-cases/r1"
	g1QueryFixtureSchema = "tsgk-native-query-cases/r1"
)

var csharpG1Fixtures = []string{"routes", "gaps", "n461", "queries"}

func csharpG1FixturePath(dir string) string { return "src/testdata/native/" + dir + "/csharp.json" }

// CSharpG1Sources are the pinned repository declarations the ledger is derived from.
type CSharpG1Sources struct {
	Inventory    []byte
	Alternatives []byte
	FactMapping  []byte
	Fixtures     map[string][]byte // by directory: routes, gaps, n461, queries
}

type CSharpG1Inputs struct {
	Inventory    queryregistrycore.Pin   `json:"qualification"`
	Alternatives queryregistrycore.Pin   `json:"feature_alternatives"`
	FactMapping  queryregistrycore.Pin   `json:"fact_mapping"`
	Fixtures     []queryregistrycore.Pin `json:"fixtures"`
}

// G1Link binds an obligation to one registered case: Steps are the steps of its source versions
// that can supply the obligation (the steps the coverage rule counts), Matched the subset of them
// whose source holds the alternative's static pattern (alternatives only).
type G1Link struct {
	Case    string `json:"case"`
	Steps   []int  `json:"steps"`
	Matched []int  `json:"matched"`
}

type G1Obligation struct {
	Kind   string   `json:"kind"`
	Status string   `json:"status"`
	Links  []G1Link `json:"links"`
}

type G1Row struct {
	Row          string         `json:"row"`
	Alternatives int            `json:"alternatives"`
	Obligations  []G1Obligation `json:"obligations"`
}

type G1Step struct {
	Step   int      `json:"step"`
	Syntax string   `json:"syntax"`
	Types  []string `json:"types"` // node types the registered expectation names (contains and anchors)
}

type G1Case struct {
	ID      string   `json:"id"`
	Fixture string   `json:"fixture"`
	SHA256  string   `json:"sha256"`
	Steps   []G1Step `json:"steps"`
}

type G1Evidence struct {
	Case    string   `json:"case"`
	Step    int      `json:"step"`
	Offsets []uint32 `json:"offsets"`
}

type G1Presence struct {
	Status   string      `json:"status"`
	Strength string      `json:"strength"`
	Literals []string    `json:"literals"`
	Reason   string      `json:"reason"`
	Evidence *G1Evidence `json:"evidence"`
}

type G1Alternative struct {
	ID       string     `json:"id"`
	Row      string     `json:"row"`
	Kind     string     `json:"kind"`
	Status   string     `json:"status"`
	Presence G1Presence `json:"presence"`
	Links    []G1Link   `json:"links"`
}

// G1Declaration is one declaration node of the fact mapping (declaration-facts-r1) with the
// alternatives whose production head is that node and the cases whose expectation names it.
type G1Declaration struct {
	Fact         string   `json:"fact"`
	Node         string   `json:"node"`
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	Alternatives []string `json:"alternatives"`
	Cases        int      `json:"cases"`
}

// G1Outside is a declaration-like construct the fact mapping does not list. It is an obligation
// when the declared alternatives contain its productions (IN_DECLARED_SCOPE), else it is only
// listed (NOT_IN_DECLARED_SCOPE). The construct table is hand-declared in csharpG1Constructs.
type G1Outside struct {
	Construct    string   `json:"construct"`
	Scope        string   `json:"scope"`
	Status       string   `json:"status"`
	Nodes        []G1Node `json:"nodes"`
	Heads        string   `json:"heads"`
	Alternatives []string `json:"alternatives"`
}

type G1Node struct {
	Type  string `json:"type"`
	Cases int    `json:"cases"`
}

type G1WorkloadRef struct {
	Set   string `json:"set"`
	Cases int    `json:"cases"`
	Steps int    `json:"steps"`
}

type G1Role struct {
	ID        string          `json:"id"`
	Status    string          `json:"status"`
	Workloads []G1WorkloadRef `json:"workloads"`
}

type G1Counts struct {
	Rows            int `json:"rows"`
	Obligations     int `json:"obligations"`
	Alternatives    int `json:"alternatives"`
	Production      int `json:"production"`
	Fact            int `json:"fact"`
	Cases           int `json:"cases"`
	AlternativeLink int `json:"alternative_links"`
	LiteralMatch    int `json:"literal_match"`
	LiteralAbsent   int `json:"literal_absent"`
	Unverified      int `json:"unverified_presence"`
	NoCase          int `json:"alternatives_without_case"`
}

// CSharpG1Ledger is the decoded tsgk-csharp-g1-ledger/r1 asset.
type CSharpG1Ledger struct {
	Schema         string          `json:"schema"`
	Inputs         CSharpG1Inputs  `json:"inputs"`
	Basis          string          `json:"basis"`
	Counts         G1Counts        `json:"counts"`
	Rows           []G1Row         `json:"rows"`
	Cases          []G1Case        `json:"cases"`
	Alternatives   []G1Alternative `json:"alternatives"`
	Declarations   []G1Declaration `json:"declarations"`
	OutsideMapping []G1Outside     `json:"outside_mapping"`
	ExtraRoles     []G1Role        `json:"extra_roles"`
}

const csharpG1Basis = "Obligations are enumerated from the repository declarations alone: the C# route requirements and cases of the qualification inventory, the production/fact alternatives of feature-alternatives.json, the declaration mapping of fact-mapping.json and the registered case sources of the native fixtures. No native or reference output is read and no expected value is derived: every state is NO_EXPECTED. An alternative's presence is a static check of the literals its production requires (top-level quoted terminals that are not optional) against the source versions of its linked P steps; it is textual (comments and strings are not excluded), so LITERAL_MATCH with PUNCTUATION strength proves little, and an alternative with no such literal is UNVERIFIED_PRESENCE. Out-of-mapping declaration constructs come from a hand-declared construct table matched against the alternatives' production heads."

func g1Fault(code string) error { return fail(KindInvalidInput, code, CSharpG1LedgerPath, nil) }

// g1Construct is one hand-declared out-of-mapping construct: the tree-sitter node types it uses and
// the pattern of production heads (the text before ':' of a declared alternative) that spell it.
type g1Construct struct {
	name  string
	nodes []string
	heads *regexp.Regexp
}

// The node types are the names the registered case expectations already use (counted in the ledger).
var csharpG1Constructs = []g1Construct{
	{"local function", []string{"local_function_statement"}, regexp.MustCompile(`^(local_function_declaration|local_function_header|local_function_body|ref_local_function_body|local_function_modifier|ref_local_function_modifier)$`)},
	{"local variable declaration", []string{"local_declaration_statement", "variable_declaration"}, regexp.MustCompile(`^(local_variable_declaration|local_constant_declaration|local_using_declaration|non_ref_local_variable_declaration|(implicitly|explicitly)_typed_(ref_)?local_variable_declaration|(implicitly|explicitly)_typed_local_variable_declarators?|(ref_)?local_variable_declarators?)$`)},
	{"accessor", []string{"accessor_declaration", "accessor_list"}, regexp.MustCompile(`accessor`)},
	{"parameter", []string{"parameter", "parameter_list", "bracketed_parameter_list"}, regexp.MustCompile(`^(parameter_list|fixed_parameters?|parameter_collection|parameter_modifier|parameter_mode_modifier|delimited_parameter_list)$`)},
	{"type parameter", []string{"type_parameter", "type_parameter_list", "type_parameter_constraints_clause"}, regexp.MustCompile(`^(type_parameter|type_parameter_list|type_parameter_constraints_clause|type_parameter_constraints|restrictive_type_parameter_constraints|decorated_type_parameter|variant_type_parameter|variant_type_parameter_list)$`)},
	{"using alias", []string{"using_directive"}, regexp.MustCompile(`^(global_)?using_alias_directive$`)},
	{"global using", []string{"using_directive"}, regexp.MustCompile(`^global_using`)},
	{"extern alias", []string{"extern_alias_directive"}, regexp.MustCompile(`^extern_alias_directive$`)},
	{"global attribute", []string{"global_attribute"}, regexp.MustCompile(`^global_attribute`)},
	{"top-level statement", []string{"global_statement"}, regexp.MustCompile(`^compilation_unit_body$`)},
	{"record or primary-constructor positional parameter", []string{"parameter_list"}, regexp.MustCompile(`^(record_class_declaration|record_struct_declaration|non_record_(class|struct)_with_positional_members)$`)},
}

type g1AltSource struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Production string `json:"production"`
}

type g1AltFile struct {
	Schema string `json:"schema"`
	Routes map[string]map[string]struct {
		Alternatives []g1AltSource `json:"alternatives"`
	} `json:"routes"`
}

type g1MappingFile struct {
	Schema   string `json:"schema"`
	Revision string `json:"revision"`
	Routes   []struct {
		Route string `json:"route"`
		Facts []struct {
			Fact string `json:"fact"`
			Node string `json:"node"`
			Name string `json:"name"`
		} `json:"facts"`
	} `json:"routes"`
}

type g1FixtureFile struct {
	Schema string `json:"schema"`
	Route  string `json:"route"`
	Cases  []struct {
		ID         string `json:"id"`
		Source     string `json:"source_utf8"`
		SourceCase string `json:"source_case"`
	} `json:"cases"`
}

// g1Head returns the production head (without a "fragment " prefix) and the right side without
// the "[alt i/n]" suffix.
var g1AltSuffix = regexp.MustCompile(`\s+\[alt \d+/\d+\]$`)

func g1SplitProduction(p string) (head, rhs string, ok bool) {
	head, rhs, ok = strings.Cut(p, ": ")
	if !ok {
		return "", "", false
	}
	return strings.TrimPrefix(head, "fragment "), g1AltSuffix.ReplaceAllString(rhs, ""), true
}

var g1Word = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// g1RequiredLiterals extracts the quoted terminals a production needs in every use: top-level
// (outside parentheses), not followed by ? or *. A top-level '|', a character range or class makes
// the form unsupported (reason), never guessed. No required literal is the reason NO_REQUIRED_LITERAL.
func g1RequiredLiterals(rhs string) (lits []string, reason string) {
	depth := 0
	for i := 0; i < len(rhs); {
		switch c := rhs[i]; {
		case c == '\'':
			var lit strings.Builder
			j := i + 1
			for ; j < len(rhs) && rhs[j] != '\''; j++ {
				if rhs[j] == '\\' && j+1 < len(rhs) && (rhs[j+1] == '\'' || rhs[j+1] == '\\') {
					j++
				} else if r, ok := g1UnicodeEscape(rhs[j:]); ok {
					lit.WriteRune(r)
					j += 5
					continue
				}
				lit.WriteByte(rhs[j])
			}
			if j >= len(rhs) || lit.Len() == 0 {
				return nil, "UNTERMINATED_LITERAL"
			}
			i = j + 1
			k := i
			for k < len(rhs) && rhs[k] == ' ' {
				k++
			}
			if depth == 0 && (k >= len(rhs) || (rhs[k] != '?' && rhs[k] != '*')) {
				lits = append(lits, lit.String())
			}
		case c == '(':
			depth++
			i++
		case c == ')':
			depth--
			i++
		case c == '|' && depth == 0:
			return nil, "TOP_LEVEL_ALTERNATION"
		case c == '~' || c == '[' || (c == '.' && i+1 < len(rhs) && rhs[i+1] == '.'):
			return nil, "CHARACTER_CLASS_OR_RANGE"
		default:
			i++
		}
	}
	if len(lits) == 0 {
		return nil, "NO_REQUIRED_LITERAL"
	}
	return lits, ""
}

// g1UnicodeEscape decodes a leading \uXXXX: the production names the code point, not the escape text.
func g1UnicodeEscape(s string) (rune, bool) {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return 0, false
	}
	var r rune
	for _, c := range []byte(s[2:6]) {
		switch {
		case c >= '0' && c <= '9':
			r = r<<4 | rune(c-'0')
		case c >= 'A' && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		case c >= 'a' && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		default:
			return 0, false
		}
	}
	return r, true
}

func g1IdentChar(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

// g1Subsequence finds lits in order in text (a word literal only at identifier boundaries) and
// returns the start offsets, or nil.
func g1Subsequence(text []byte, lits []string, bounded bool) []uint32 {
	var out []uint32
	pos := 0
	for _, lit := range lits {
		word := bounded && g1Word.MatchString(lit)
		found := -1
		for from := pos; from <= len(text)-len(lit); {
			i := bytes.Index(text[from:], []byte(lit))
			if i < 0 {
				break
			}
			i += from
			end := i + len(lit)
			if !word || (i == 0 || !g1IdentChar(text[i-1])) && (end == len(text) || !g1IdentChar(text[end])) {
				found = i
				break
			}
			from = i + 1
		}
		if found < 0 {
			return nil
		}
		out = append(out, uint32(found))
		pos = found + len(lit)
	}
	return out
}

func g1Types(e StepExpectation) []string {
	set := map[string]bool{}
	for _, n := range e.Contains {
		set[n] = true
	}
	for _, a := range e.Anchors {
		set[a.Type] = true
	}
	out := []string{}
	for n := range set {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

type g1CaseData struct {
	c        QualCase
	fixture  string
	versions [][]byte
}

// DeriveCSharpG1Ledger derives the ledger from the pinned declarations alone.
func DeriveCSharpG1Ledger(src CSharpG1Sources) (CSharpG1Ledger, error) {
	var out CSharpG1Ledger
	inv, err := ParseQualificationInventory(src.Inventory)
	if err != nil {
		return out, err
	}
	var route *QualRoute
	for i := range inv.Routes {
		if inv.Routes[i].Route == g1Route {
			route = &inv.Routes[i]
		}
	}
	if route == nil {
		return out, g1Fault("G1_LEDGER_ROUTE")
	}
	var alts g1AltFile
	if err := json.Unmarshal(src.Alternatives, &alts); err != nil || alts.Schema != g1AlternativesSchema || alts.Routes[g1Route] == nil {
		return out, g1Fault("G1_LEDGER_ALTERNATIVES")
	}
	var mapping g1MappingFile
	if err := json.Unmarshal(src.FactMapping, &mapping); err != nil || mapping.Schema != g1MappingSchema {
		return out, g1Fault("G1_LEDGER_MAPPING")
	}
	decl := route.Workload.Declarations
	if decl == nil || decl.Mapping != mapping.Revision {
		return out, g1Fault("G1_LEDGER_MAPPING")
	}
	// the declaration nodes are the workload's declaration items; each must be a mapping fact
	mapped := decl.Items
	if len(mapped) == 0 {
		return out, g1Fault("G1_LEDGER_MAPPING")
	}
	for _, m := range mapped {
		ok := false
		for _, r := range mapping.Routes {
			for _, f := range r.Facts {
				ok = ok || r.Route == g1Route && f.Fact == m.Fact && f.Node == m.Node && f.Name == m.Name
			}
		}
		if !ok {
			return out, g1Fault("G1_LEDGER_MAPPING")
		}
	}
	out = CSharpG1Ledger{Schema: CSharpG1LedgerSchema, Basis: csharpG1Basis}
	out.Inputs.Inventory = queryregistrycore.FilePin(queryregistrycore.QualificationPin.Path, src.Inventory)
	out.Inputs.Alternatives = queryregistrycore.FilePin("src/contracts/feature-alternatives.json", src.Alternatives)
	out.Inputs.FactMapping = queryregistrycore.FilePin("src/contracts/fact-mapping.json", src.FactMapping)

	// fixture sources by case ID; a query case reuses the source of its source_case
	sources := map[string]string{}
	fixtureOf := map[string]string{}
	queryCases := map[string]string{}
	for _, dir := range csharpG1Fixtures {
		data, ok := src.Fixtures[dir]
		if !ok {
			return out, g1Fault("G1_LEDGER_FIXTURE")
		}
		out.Inputs.Fixtures = append(out.Inputs.Fixtures, queryregistrycore.FilePin(csharpG1FixturePath(dir), data))
		var f g1FixtureFile
		want := g1FixtureSchema
		if dir == "queries" {
			want = g1QueryFixtureSchema
		}
		if err := json.Unmarshal(data, &f); err != nil || f.Schema != want || f.Route != g1Route {
			return out, g1Fault("G1_LEDGER_FIXTURE")
		}
		for _, c := range f.Cases {
			if _, dup := fixtureOf[c.ID]; dup {
				return out, g1Fault("G1_LEDGER_FIXTURE")
			}
			fixtureOf[c.ID] = dir
			if dir == "queries" {
				queryCases[c.ID] = c.SourceCase
			} else {
				sources[c.ID] = c.Source
			}
		}
	}
	for id, from := range queryCases {
		s, ok := sources[from]
		if !ok {
			return out, g1Fault("G1_LEDGER_FIXTURE")
		}
		sources[id] = s
	}

	// cases: every requirement case of the route workload, with its source versions
	data := map[string]*g1CaseData{}
	for _, c := range route.Workload.Cases {
		if c.Role != "requirement" {
			if len(c.Covers) != 0 || len(c.Alternatives) != 0 {
				return out, g1Fault("G1_LEDGER_CASE")
			}
			continue
		}
		s, ok := sources[c.ID]
		if !ok || digestHex([]byte(s)) != c.Input.SHA256 || uint64(len(s)) != c.Input.Bytes {
			return out, g1Fault("G1_LEDGER_SOURCE")
		}
		versions, _, err := ApplyEdits(EncodingUTF8, []byte(s), c.Edits, g1SourceLimit)
		if err != nil {
			return out, err
		}
		for _, e := range c.Expect {
			if e.Step < 0 || e.Step >= len(versions) {
				return out, g1Fault("G1_LEDGER_CASE")
			}
		}
		data[c.ID] = &g1CaseData{c: c, fixture: csharpG1FixturePath(fixtureOf[c.ID]), versions: versions}
		g := G1Case{ID: c.ID, Fixture: data[c.ID].fixture, SHA256: c.Input.SHA256, Steps: []G1Step{}}
		for _, e := range c.Expect {
			g.Steps = append(g.Steps, G1Step{Step: e.Step, Syntax: e.Syntax, Types: g1Types(e)})
		}
		out.Cases = append(out.Cases, g)
	}
	out.Counts.Cases = len(out.Cases)

	// steps of a case that can supply a kind (the coverage rule's steps)
	kindSteps := func(d *g1CaseData, kind string) []int {
		steps := []int{}
		switch kind {
		case "P", "N", "R":
			for _, e := range d.c.Expect {
				if stepCovers(e, kind, route.ErrorNodes) {
					steps = append(steps, e.Step)
				}
			}
		case "W":
			if d.c.Sample != nil {
				for _, e := range d.c.Expect {
					if stepCovers(e, kind, route.ErrorNodes) {
						steps = append(steps, e.Step)
					}
				}
			}
		case "E":
			for i := range d.c.Edits {
				steps = append(steps, i+1)
			}
		case "Q":
			for _, q := range d.c.QueryExpect {
				if q.Captures != nil && !slices.Contains(steps, q.Step) {
					steps = append(steps, q.Step)
				}
			}
			slices.Sort(steps)
		}
		return steps
	}

	// rows x kinds, and the alternatives of each row in the inventory's order
	altSource := map[string]g1AltSource{}
	altLinks := map[string][]G1Link{}
	for _, req := range route.Requirements {
		row := G1Row{Row: req.Row, Alternatives: len(req.Alternatives)}
		for _, kind := range req.Kinds {
			ob := G1Obligation{Kind: kind, Status: G1NoExpected, Links: []G1Link{}}
			for _, c := range route.Workload.Cases {
				d := data[c.ID]
				if d == nil || !slices.Contains(c.Covers[req.Row], kind) {
					continue
				}
				ob.Links = append(ob.Links, G1Link{Case: c.ID, Steps: kindSteps(d, kind), Matched: []int{}})
			}
			row.Obligations = append(row.Obligations, ob)
			out.Counts.Obligations++
		}
		out.Rows = append(out.Rows, row)
		out.Counts.Rows++

		file, ok := alts.Routes[g1Route][req.Row]
		if !ok || len(file.Alternatives) != len(req.Alternatives) {
			return out, g1Fault("G1_LEDGER_ALTERNATIVES")
		}
		for i, a := range file.Alternatives {
			if a.ID != req.Alternatives[i] || a.Kind != "production" && a.Kind != "fact" {
				return out, g1Fault("G1_LEDGER_ALTERNATIVES")
			}
			altSource[a.ID] = a
		}
	}
	for _, c := range route.Workload.Cases {
		d := data[c.ID]
		if d == nil {
			continue
		}
		for _, a := range c.Alternatives {
			altLinks[a] = append(altLinks[a], G1Link{Case: c.ID, Steps: kindSteps(d, "P"), Matched: []int{}})
		}
	}

	for _, req := range route.Requirements {
		for _, id := range req.Alternatives {
			a := altSource[id]
			alt := G1Alternative{ID: id, Row: req.Row, Kind: a.Kind, Status: G1NoExpected, Links: altLinks[id]}
			if alt.Links == nil {
				alt.Links = []G1Link{}
			}
			alt.Presence = G1Presence{Status: G1PresenceUnverified, Literals: []string{}}
			switch {
			case a.Kind == "fact":
				alt.Presence.Reason = "FACT_DESCRIPTION"
			default:
				_, rhs, ok := g1SplitProduction(a.Production)
				if !ok {
					alt.Presence.Reason = "PRODUCTION_FORM"
					break
				}
				lits, reason := g1RequiredLiterals(rhs)
				if reason != "" {
					alt.Presence.Reason = reason
					break
				}
				alt.Presence.Literals = lits
				alt.Presence.Status = G1PresenceAbsent
				fragment := strings.HasPrefix(a.Production, "fragment ")
				switch {
				case fragment:
					alt.Presence.Strength = g1StrengthFragment
				case slices.ContainsFunc(lits, g1Word.MatchString):
					alt.Presence.Strength = g1StrengthKeyword
				default:
					alt.Presence.Strength = g1StrengthPunct
				}
				for li := range alt.Links {
					link := &alt.Links[li]
					d := data[link.Case]
					for _, s := range link.Steps {
						offs := g1Subsequence(d.versions[s], lits, !fragment)
						if offs == nil {
							continue
						}
						link.Matched = append(link.Matched, s)
						if alt.Presence.Evidence == nil {
							alt.Presence.Evidence = &G1Evidence{Case: link.Case, Step: s, Offsets: offs}
							alt.Presence.Status = G1PresenceMatch
						}
					}
				}
			}
			out.Alternatives = append(out.Alternatives, alt)
			out.Counts.Alternatives++
			out.Counts.AlternativeLink += len(alt.Links)
			if a.Kind == "fact" {
				out.Counts.Fact++
			} else {
				out.Counts.Production++
			}
			if len(alt.Links) == 0 {
				out.Counts.NoCase++
			}
			switch alt.Presence.Status {
			case G1PresenceMatch:
				out.Counts.LiteralMatch++
			case G1PresenceAbsent:
				out.Counts.LiteralAbsent++
			default:
				out.Counts.Unverified++
			}
		}
	}

	// node type -> number of cases whose registered expectation names it
	typeCases := map[string]int{}
	for _, g := range out.Cases {
		seen := map[string]bool{}
		for _, s := range g.Steps {
			for _, n := range s.Types {
				seen[n] = true
			}
		}
		for n := range seen {
			typeCases[n]++
		}
	}
	heads := map[string][]string{}
	for _, alt := range out.Alternatives {
		if altSource[alt.ID].Kind != "production" {
			continue
		}
		if head, _, ok := g1SplitProduction(altSource[alt.ID].Production); ok {
			heads[alt.ID] = []string{head}
		}
	}
	for _, m := range mapped {
		d := G1Declaration{Fact: m.Fact, Node: m.Node, Name: m.Name, Status: G1NoExpected, Alternatives: []string{}, Cases: typeCases[m.Node]}
		for _, alt := range out.Alternatives {
			if h := heads[alt.ID]; len(h) == 1 && h[0] == m.Node {
				d.Alternatives = append(d.Alternatives, alt.ID)
			}
		}
		out.Declarations = append(out.Declarations, d)
	}
	for _, c := range csharpG1Constructs {
		o := G1Outside{Construct: c.name, Scope: "NOT_IN_DECLARED_SCOPE", Status: "", Heads: c.heads.String(), Alternatives: []string{}}
		for _, alt := range out.Alternatives {
			if h := heads[alt.ID]; len(h) == 1 && c.heads.MatchString(h[0]) {
				o.Alternatives = append(o.Alternatives, alt.ID)
			}
		}
		if len(o.Alternatives) > 0 {
			o.Scope, o.Status = "IN_DECLARED_SCOPE", G1NoExpected
		}
		for _, n := range c.nodes {
			o.Nodes = append(o.Nodes, G1Node{Type: n, Cases: typeCases[n]})
		}
		out.OutsideMapping = append(out.OutsideMapping, o)
	}

	// extra roles that run the C# route
	for _, role := range inv.ExtraRoles {
		r := G1Role{ID: role.ID, Status: G1NoExpected, Workloads: []G1WorkloadRef{}}
		for _, w := range role.Workloads {
			if w.Route != g1Route {
				continue
			}
			ref := G1WorkloadRef{Set: w.Set, Cases: len(w.Cases)}
			for _, c := range w.Cases {
				ref.Steps += len(c.Expect)
			}
			r.Workloads = append(r.Workloads, ref)
		}
		if len(r.Workloads) > 0 {
			out.ExtraRoles = append(out.ExtraRoles, r)
		}
	}
	return out, nil
}

// MarshalCSharpG1Ledger writes the canonical asset: one line per row, case, alternative and the
// other list entries, so a change shows as a small diff.
func MarshalCSharpG1Ledger(l CSharpG1Ledger) ([]byte, error) {
	var b bytes.Buffer
	var werr error
	put := func(v any) {
		d, err := json.Marshal(v)
		if err != nil {
			werr = err
		}
		b.Write(d)
	}
	list := func(name string, n int, at func(int) any, last bool) {
		fmt.Fprintf(&b, " %q: [\n", name)
		for i := 0; i < n; i++ {
			b.WriteString("  ")
			put(at(i))
			if i < n-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(" ]")
		if !last {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	scalar := func(name string, v any) {
		fmt.Fprintf(&b, " %q: ", name)
		put(v)
		b.WriteString(",\n")
	}
	b.WriteString("{\n")
	scalar("schema", l.Schema)
	scalar("inputs", l.Inputs)
	scalar("basis", l.Basis)
	scalar("counts", l.Counts)
	list("rows", len(l.Rows), func(i int) any { return l.Rows[i] }, false)
	list("cases", len(l.Cases), func(i int) any { return l.Cases[i] }, false)
	list("alternatives", len(l.Alternatives), func(i int) any { return l.Alternatives[i] }, false)
	list("declarations", len(l.Declarations), func(i int) any { return l.Declarations[i] }, false)
	list("outside_mapping", len(l.OutsideMapping), func(i int) any { return l.OutsideMapping[i] }, false)
	list("extra_roles", len(l.ExtraRoles), func(i int) any { return l.ExtraRoles[i] }, true)
	b.WriteString("}\n")
	return b.Bytes(), werr
}

func g1Same(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// ParseCSharpG1Ledger decodes the asset and requires it to equal the derivation from the pinned
// sources: the pins, the rows, the exact alternative list (no missing, repeated, extra or reordered
// entry), every entry, the declaration and role sections, the counts, and the canonical bytes.
func ParseCSharpG1Ledger(data []byte, src CSharpG1Sources) (CSharpG1Ledger, error) {
	want, err := DeriveCSharpG1Ledger(src)
	if err != nil {
		return CSharpG1Ledger{}, err
	}
	return checkCSharpG1Ledger(data, want)
}

func checkCSharpG1Ledger(data []byte, want CSharpG1Ledger) (CSharpG1Ledger, error) {
	var x CSharpG1Ledger
	if len(data) > csharpG1MaxBytes {
		return x, fail(KindResourceLimit, "G1_LEDGER_SIZE", CSharpG1LedgerPath, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_LEDGER_JSON", CSharpG1LedgerPath, err)
	}
	if x.Schema != CSharpG1LedgerSchema || x.Basis == "" {
		return x, g1Fault("G1_LEDGER_SCHEMA")
	}
	if !g1Same(x.Inputs, want.Inputs) {
		return x, g1Fault("G1_LEDGER_PIN")
	}
	for _, s := range g1States(x) {
		if s != G1NoExpected && s != "" {
			return x, g1Fault("G1_LEDGER_STATUS")
		}
	}
	if !g1Same(x.Rows, want.Rows) {
		return x, g1Fault("G1_LEDGER_ROWS")
	}
	if code := g1MembershipFault(x.Alternatives, want.Alternatives); code != "" {
		return x, g1Fault(code)
	}
	for _, s := range []struct {
		code string
		a, b any
	}{
		{"G1_LEDGER_ALTERNATIVE", x.Alternatives, want.Alternatives},
		{"G1_LEDGER_CASES", x.Cases, want.Cases},
		{"G1_LEDGER_DECLARATIONS", x.Declarations, want.Declarations},
		{"G1_LEDGER_OUTSIDE_MAPPING", x.OutsideMapping, want.OutsideMapping},
		{"G1_LEDGER_ROLES", x.ExtraRoles, want.ExtraRoles},
		{"G1_LEDGER_COUNTS", x.Counts, want.Counts},
	} {
		if !g1Same(s.a, s.b) {
			return x, g1Fault(s.code)
		}
	}
	canonical, err := MarshalCSharpG1Ledger(want)
	if err != nil || !bytes.Equal(canonical, data) {
		return x, g1Fault("G1_LEDGER_BYTES")
	}
	return x, nil
}

func g1States(x CSharpG1Ledger) []string {
	var out []string
	for _, r := range x.Rows {
		for _, o := range r.Obligations {
			out = append(out, o.Status)
		}
	}
	for _, a := range x.Alternatives {
		out = append(out, a.Status)
	}
	for _, d := range x.Declarations {
		out = append(out, d.Status)
	}
	for _, o := range x.OutsideMapping {
		out = append(out, o.Status)
	}
	for _, r := range x.ExtraRoles {
		out = append(out, r.Status)
	}
	return out
}

// g1MembershipFault names how the alternative ID list differs from the derived one.
func g1MembershipFault(got, want []G1Alternative) string {
	count := map[string]int{}
	for _, a := range got {
		count[a.ID]++
	}
	for _, n := range count {
		if n > 1 {
			return "G1_LEDGER_ALTERNATIVE_DUPLICATE"
		}
	}
	wantSet := map[string]bool{}
	for _, a := range want {
		wantSet[a.ID] = true
		if count[a.ID] == 0 {
			return "G1_LEDGER_ALTERNATIVE_MISSING"
		}
	}
	for _, a := range got {
		if !wantSet[a.ID] {
			return "G1_LEDGER_ALTERNATIVE_EXTRA"
		}
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			return "G1_LEDGER_ALTERNATIVE_ORDER"
		}
	}
	return ""
}
