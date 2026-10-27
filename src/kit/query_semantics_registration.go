package kit

import (
	"bytes"
	"reflect"
	"slices"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
)

const querySemanticsRegistrationSchema = "tsgk-query-semantics-registration/r1"

type semanticOracleCase struct {
	IncrementalCase `json:",inline"`
	QueryExpect     []QueryExpectation     `json:"query_expect"`
	DynamicSQL      *DynamicSQLExpectation `json:"dynamic_sql_expect"`
}

// The preregistration binds meaning; the profile's later registration digest cannot point back to itself.
type semanticOracleMeaning struct {
	ID           string               `json:"id"`
	Route        string               `json:"route"`
	Operation    string               `json:"operation"`
	Symbol       string               `json:"symbol"`
	Encoding     string               `json:"encoding"`
	Output       string               `json:"output"`
	Compiler     ToolIdentity         `json:"compiler"`
	Grammar      []NativeInput        `json:"grammar"`
	Declarations *Declarations        `json:"declarations"`
	Queries      []OracleQuery        `json:"queries"`
	FactPack     *FactPackRef         `json:"fact_pack"`
	API          bool                 `json:"api"`
	Cases        []semanticOracleCase `json:"cases"`
}

type semanticPlannedMember struct {
	Owner string `json:"owner"`
	Path  string `json:"path"`
	Role  string `json:"role"`
}

type semanticRegistration struct {
	QueryWitnessRegistration `json:",inline"`
	Context                  string                  `json:"context"`
	Profile                  semanticOracleMeaning   `json:"profile"`
	NativePolicy             IdentityRef             `json:"native_policy"`
	Limits                   semanticRequestLimits   `json:"limits"`
	Classes                  []string                `json:"classes"`
	PlannedMembers           []semanticPlannedMember `json:"planned_members"`
}

func semanticLimitsFor(op NativeOperation) semanticRequestLimits {
	return semanticRequestLimits{InputBytes: op.InputBytes, Nodes: op.Nodes, FullNodes: op.FullNodes, Depth: op.Depth, OutputBytes: op.OutputBytes,
		ParseMillis: op.ParseMillis, MemoryBytes: op.MemoryBytes, Errors: op.Errors, PartialNodes: op.PartialNodes, Matches: op.Matches, Captures: op.Captures, QueryMillis: op.QueryMillis}
}

func semanticMeaning(profile OracleProfile) semanticOracleMeaning {
	n := profile.Native
	meaning := semanticOracleMeaning{ID: n.ID, Route: n.Route, Operation: n.Operation, Symbol: n.Symbol, Encoding: n.Encoding, Output: n.Output,
		Compiler: n.Compiler, Grammar: n.Grammar, Declarations: n.Declarations, Queries: profile.Queries, FactPack: profile.FactPack, API: profile.API,
		Cases: make([]semanticOracleCase, len(profile.OracleCases))}
	for i, c := range profile.OracleCases {
		meaning.Cases[i] = semanticOracleCase{IncrementalCase: c.IncrementalCase, QueryExpect: c.QueryExpect, DynamicSQL: c.DynamicSQL}
	}
	return canonicalSemanticMeaning(meaning)
}

// Profile arrays are logical values; raw capture nulls and unavailable witness anchors remain untouched.
func canonicalSemanticMeaning(meaning semanticOracleMeaning) semanticOracleMeaning {
	if len(meaning.Queries) == 0 {
		meaning.Queries = []OracleQuery{}
	}
	if meaning.Declarations != nil {
		copy := *meaning.Declarations
		if len(copy.Items) == 0 {
			copy.Items = []NativeDeclaration{}
		}
		meaning.Declarations = &copy
	}
	meaning.Cases = slices.Clone(meaning.Cases)
	for i := range meaning.Cases {
		c := &meaning.Cases[i]
		c.Edits = slices.Clone(c.Edits)
		if len(c.Edits) == 0 {
			c.Edits = []Edit{}
		}
		for i := range c.Edits {
			if len(c.Edits[i].Old) == 0 {
				c.Edits[i].Old = []byte{}
			}
			if len(c.Edits[i].New) == 0 {
				c.Edits[i].New = []byte{}
			}
		}
		if len(c.Points) == 0 {
			c.Points = []NativePoint{}
		}
		c.Expect = slices.Clone(c.Expect)
		if len(c.Expect) == 0 {
			c.Expect = []StepExpectation{}
		}
		for i := range c.Expect {
			if len(c.Expect[i].Contains) == 0 {
				c.Expect[i].Contains = []string{}
			}
			if len(c.Expect[i].Anchors) == 0 {
				c.Expect[i].Anchors = nil
			}
		}
		c.QueryExpect = slices.Clone(c.QueryExpect)
		if len(c.QueryExpect) == 0 {
			c.QueryExpect = []QueryExpectation{}
		}
		for i := range c.QueryExpect {
			if c.QueryExpect[i].Captures != nil && len(*c.QueryExpect[i].Captures) == 0 {
				empty := []CaptureExpectation{}
				c.QueryExpect[i].Captures = &empty
			}
		}
	}
	return meaning
}

func semanticEditsEqual(a, b []Edit) bool {
	return slices.EqualFunc(a, b, func(a, b Edit) bool {
		return a.StartByte == b.StartByte && a.OldEndByte == b.OldEndByte && a.NewEndByte == b.NewEndByte && bytes.Equal(a.Old, b.Old) && bytes.Equal(a.New, b.New)
	})
}

func (b *eligibilityBudget) oracleProfile(data []byte) (OracleProfile, *Error) {
	if e := b.reserveJSON("oracle", data, 3); e != nil {
		return OracleProfile{}, e
	}
	values := &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, "oracle") }}
	v, e := decodeBounded("oracle", data, MaxDocumentBytes, values)
	if e != nil {
		return OracleProfile{}, e
	}
	if e := b.charge(8*values.n+uint64(len(data)), 256*values.n+4*uint64(len(data)), "oracle-typed"); e != nil {
		return OracleProfile{}, e
	}
	return parseOracleValue(data, v, b.r.check)
}

func (b *eligibilityBudget) semanticsRegistration(data []byte, profile OracleProfile) (semanticRegistration, *Error) {
	var reg semanticRegistration
	if e := b.decode("semantics-registration", data, &reg, true); e != nil {
		return reg, e
	}
	bad := func(code string) (semanticRegistration, *Error) {
		return reg, fail(KindInvalidInput, code, "semantics-registration", nil)
	}
	significant, sizeErr := b.significantJSONBytes(data)
	if sizeErr != nil {
		return reg, sizeErr
	}
	if e := b.charge(4*significant+1, 16*uint64(len(data))+256*uint64(len(reg.Required)+len(reg.Queries)), "semantics-metadata"); e != nil {
		return reg, e
	}
	for _, q := range reg.Queries {
		if q.Metadata == nil {
			continue
		}
		if e := b.charge(uint64(len(q.Metadata.CaptureNames)), 256*uint64(len(q.Metadata.CaptureNames)), "semantics-metadata-names"); e != nil {
			return reg, e
		}
		for _, p := range q.Metadata.Predicates {
			if e := b.charge(16*uint64(len(p.Steps))+1, 0, "semantics-metadata-steps"); e != nil {
				return reg, e
			}
		}
	}
	if _, e := validateQueryWitnessRegistrationChecked(reg.QueryWitnessRegistration, querySemanticsRegistrationSchema, b.r.check); e != nil {
		return reg, e
	}
	n := profile.Native
	if profile.Native.Schema != OracleSemanticsSchema || profile.QuerySemanticsRegistrationSHA256 != digestHex(data) {
		return bad("SEMANTICS_REGISTRATION_BINDING")
	}
	if reg.Platform != "windows/amd64" && reg.Platform != "linux/amd64" || reg.Context != "full-input-utf8/r2" || n.Operation != "native-query" || n.Output != OutputTree || n.Encoding != EncodingUTF8 || n.Format != "" || n.SvcContext != nil || n.Preprocess != nil ||
		len(profile.OracleCases) != 1 || profile.FactPack != nil || reg.Profile.FactPack != nil {
		return reg, fail(KindUnsupported, "SEMANTICS_CONTEXT_UNSUPPORTED", "semantics-registration", nil)
	}
	if e := b.charge(significant, 4*uint64(len(data))+4096, "semantics-profile-meaning"); e != nil {
		return reg, e
	}
	if !reflect.DeepEqual(canonicalSemanticMeaning(reg.Profile), semanticMeaning(profile)) || len(profile.OracleCases[0].SvcSource) != 0 {
		return bad("SEMANTICS_PROFILE_MEANING")
	}
	c := profile.OracleCases[0]
	if reg.Route != n.Route || reg.Case != c.ID || reg.Input != c.Input || reg.Encoding != c.Encoding || c.Encoding != EncodingUTF8 ||
		!semanticEditsEqual(reg.Edits, c.Edits) || len(reg.Queries) != len(profile.Queries) {
		return bad("SEMANTICS_WORKLOAD_BINDING")
	}
	op := NativeOperations()[n.Operation]
	if reg.Limits != semanticLimitsFor(op) || reg.NativePolicy != NativePolicy(op, n.Output, nil) {
		return bad("SEMANTICS_POLICY_BINDING")
	}
	if len(reg.Classes) != len(reg.Required) || len(reg.Required) != len(reg.Queries)*(1+2*len(c.Edits)) {
		return bad("SEMANTICS_REQUIRED_COVERAGE")
	}
	for i, query := range reg.Queries {
		if e := b.charge(uint64(len(query.Source))+1, 0, "semantics-query"); e != nil {
			return reg, e
		}
		if query.ID != profile.Queries[i].ID || query.Source != profile.Queries[i].Source || query.Metadata == nil || len(query.Metadata.Predicates) == 0 {
			return bad("SEMANTICS_QUERY_REGISTRATION")
		}
		var stopped *Error
		code, _ := querypolicy.SupportChecked(func() error {
			stopped = b.r.check()
			if stopped != nil {
				return stopped
			}
			return nil
		}, query.Metadata.Patterns, query.Metadata.Predicates, EncodingUTF8)
		if stopped != nil {
			return reg, stopped
		}
		if code != "" {
			return bad("SEMANTICS_QUERY_REGISTRATION")
		}
		for step := 0; step <= len(c.Edits); step++ {
			roles := []string{"incremental"}
			if step > 0 {
				roles = append(roles, "fresh")
			}
			for _, role := range roles {
				if e := b.charge(uint64(len(reg.Required))+1, 0, "semantics-required-coverage"); e != nil {
					return reg, e
				}
				index := slices.IndexFunc(reg.Required, func(t QueryWitnessTuple) bool {
					return t.Query == query.ID && t.Step == uint32(step) && t.TreeRole == role
				})
				if index < 0 || reg.Classes[index] != "semantic-required" || reg.Required[index].Structural == nil || reg.Required[index].Expected == nil {
					return bad("SEMANTICS_REQUIRED_COVERAGE")
				}
			}
		}
	}
	if e := b.charge(uint64(len(reg.PlannedMembers))*2+1, uint64(len(reg.PlannedMembers))*256, "semantics-planned-members"); e != nil {
		return reg, e
	}
	seen := map[string]bool{}
	for _, member := range reg.PlannedMembers {
		if e := b.charge(20*uint64(len(member.Path))+1, 16*uint64(len(member.Path)), "semantics-planned-path"); e != nil {
			return reg, e
		}

		owner := "D"
		if slices.Contains([]string{"manifest", "record", "source", "projection"}, member.Role) {
			owner = "O"
		}
		if member.Owner != owner {
			return bad("SEMANTICS_MEMBER_OWNER")
		}
		folded := strings.ToLower(member.Path)
		if !portable(member.Path, false) || !validID(member.Role) || member.Owner != "D" && member.Owner != "O" || seen[folded] {
			return bad("SEMANTICS_MEMBER_PLAN")
		}
		seen[folded] = true
	}
	if len(reg.PlannedMembers) == 0 {
		return bad("SEMANTICS_MEMBER_PLAN")
	}
	return reg, nil
}
