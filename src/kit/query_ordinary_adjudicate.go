package kit

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

const (
	ordinaryExpectedTreeSchema    = "tsgk-query-ordinary-expected-tree/r1"
	ordinaryExpectedCaptureSchema = "tsgk-query-ordinary-expected-captures/r1"
	ordinaryExpectedFactSchema    = "tsgk-query-ordinary-expected-facts/r1"
	ordinaryStructuralOnly        = "STRUCTURAL_ONLY"
	ordinaryTupleVerified         = "VERIFIED"
	ordinaryTupleFailed           = "FAILED"
	ordinaryTupleBlocked          = "BLOCKED"
)

type ordinaryTupleIdentity struct {
	Case     string      `json:"case_id"`
	Query    string      `json:"query_id"`
	Step     uint32      `json:"step"`
	TreeRole string      `json:"tree_role"`
	Source   OrdinaryPin `json:"source_pin"`
}

type ordinaryExpectedTree struct {
	Schema   string      `json:"schema"`
	Case     string      `json:"case_id"`
	Step     uint32      `json:"step"`
	TreeRole string      `json:"tree_role"`
	Source   OrdinaryPin `json:"source_pin"`
	Tree     []TreeNode  `json:"tree"`
}

type ordinaryExpectedCaptures struct {
	Schema   string      `json:"schema"`
	Case     string      `json:"case_id"`
	Query    string      `json:"query_id"`
	Step     uint32      `json:"step"`
	TreeRole string      `json:"tree_role"`
	Source   OrdinaryPin `json:"source_pin"`
	Matches  uint64      `json:"matches"`
	Ordered  []Capture   `json:"ordered_captures"`
}

type ordinaryFactMappingPin struct {
	Schema   string `json:"schema"`
	Revision string `json:"revision"`
	SHA256   string `json:"sha256"`
}

type ordinaryFactQueryPin struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
}

type ordinaryExpectedDeclarations struct {
	Items      []DeclarationItem `json:"items"`
	Assessment string            `json:"assessment"`
}

type ordinaryExpectedFact struct {
	Schema       string                        `json:"schema"`
	Case         string                        `json:"case_id"`
	Query        string                        `json:"query_id"`
	Step         uint32                        `json:"step"`
	TreeRole     string                        `json:"tree_role"`
	Source       OrdinaryPin                   `json:"source_pin"`
	Pack         FactPackRef                   `json:"pack"`
	Mapping      ordinaryFactMappingPin        `json:"mapping"`
	FactQuery    ordinaryFactQueryPin          `json:"fact_query"`
	Status       string                        `json:"status"`
	Code         string                        `json:"code"`
	Declarations *ordinaryExpectedDeclarations `json:"declarations,omitempty"`
	DynamicSQL   *DynamicSQLFacts              `json:"dynamic_sql,omitempty"`
	XML          *map[string]int               `json:"xml_structure,omitempty"`
}

type ordinaryFactInputs struct {
	Pack           FactPack
	PackData       []byte
	PackRef        FactPackRef
	PackPin        OrdinaryPin
	Mapping        []byte
	MappingPin     ordinaryFactMappingPin
	ProducerByCase map[string][]byte
	Ready          bool
}

type ordinaryProducerFactRecord struct {
	Schema             string                     `json:"schema"`
	Case               string                     `json:"case"`
	Pack               FactPackRef                `json:"pack"`
	Mapping            ordinaryFactMappingPin     `json:"mapping"`
	DynamicSQLRevision string                     `json:"dynamic_sql_revision"`
	XMLRevision        string                     `json:"xml_revision"`
	FactSuffixSHA256   string                     `json:"fact_suffix_sha256"`
	FactQueries        []ordinaryFactQueryPin     `json:"fact_queries"`
	Steps              []ordinaryProducerFactStep `json:"steps"`
}

type ordinaryProducerFactStep struct {
	Step         int                         `json:"step"`
	TreeRole     string                      `json:"tree_role"`
	SourceBytes  uint64                      `json:"source_bytes"`
	SourceSHA256 string                      `json:"source_sha256"`
	Status       string                      `json:"status"`
	Code         string                      `json:"code,omitempty"`
	Queries      []ordinaryProducerFactQuery `json:"queries"`
}

type ordinaryProducerFactQuery struct {
	ID           string                     `json:"id"`
	SHA256       string                     `json:"sha256"`
	Kind         string                     `json:"kind"`
	Status       string                     `json:"status"`
	Code         string                     `json:"code,omitempty"`
	Difference   string                     `json:"first_difference,omitempty"`
	Declarations *ordinaryProducerDeclFacts `json:"declarations,omitempty"`
	DynamicSQL   *DynamicSQLFacts           `json:"dynamic_sql,omitempty"`
	XML          map[string]int             `json:"xml_structure,omitempty"`
}

type ordinaryProducerDeclFacts struct {
	Items      []DeclarationItem `json:"items"`
	Assessment string            `json:"assessment"`
	Reproduces bool              `json:"reproduces_s05"`
}

type ordinaryTupleFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type ordinaryTupleJudgment struct {
	State    string                 `json:"state"`
	Code     string                 `json:"code"`
	Findings []ordinaryTupleFinding `json:"findings"`
}

type ordinaryJudgmentBuilder struct {
	failed      bool
	blocked     bool
	failureCode string
	blockedCode string
	result      ordinaryTupleJudgment
}

func (j *ordinaryJudgmentBuilder) fail(code, detail string) {
	j.failed = true
	if j.failureCode == "" {
		j.failureCode = code
	}
	j.result.Findings = append(j.result.Findings, ordinaryTupleFinding{Severity: "error", Code: code, Message: detail})
}

func (j *ordinaryJudgmentBuilder) block(code, detail string) {
	j.blocked = true
	if j.blockedCode == "" {
		j.blockedCode = code
	}
	j.result.Findings = append(j.result.Findings, ordinaryTupleFinding{Severity: "warning", Code: code, Message: detail})
}

func (j *ordinaryJudgmentBuilder) value() ordinaryTupleJudgment {
	switch {
	case j.failed:
		j.result.State = ordinaryTupleFailed
		j.result.Code = j.failureCode
	case j.blocked:
		j.result.State = ordinaryTupleBlocked
		j.result.Code = j.blockedCode
	default:
		j.result.State = ordinaryTupleVerified
	}
	if j.result.Findings == nil {
		j.result.Findings = []ordinaryTupleFinding{}
	}
	return j.result
}

// adjudicateOrdinaryTuple checks one structural query against separately pinned metadata,
// full CST, capture and optional fact expectations. actual must come from nativewire.Check.
func adjudicateOrdinaryTuple(b *eligibilityBudget, id ordinaryTupleIdentity, actual nativewire.Tree, source, metadataData, treeData, captureData, factData []byte, facts *ordinaryFactInputs) (ordinaryTupleJudgment, *Error) {
	j := &ordinaryJudgmentBuilder{}
	if b == nil {
		return j.value(), fail(KindInvalidInput, "ORDINARY_ADJUDICATOR_BUDGET", "ordinary", nil)
	}
	if !ordinaryIdentityValid(id) || uint64(len(source)) != id.Source.Bytes || digestHex(source) != id.Source.SHA256 {
		j.fail("ORDINARY_TUPLE_SOURCE_BINDING", "선택된 tuple의 source pin과 byte가 다르다")
		return j.value(), nil
	}
	if e := b.charge(uint64(len(source))+1, uint64(len(source)), "ordinary-tuple-input"); e != nil {
		return j.value(), e
	}
	actualTreeReady := actual.Status == StatusCompleted && actual.Form == "full" && actual.Nodes != nil
	if !actualTreeReady {
		j.block("ORDINARY_ACTUAL_TREE_INCOMPLETE", "native full tree가 완료되지 않았다")
	}
	// The actual tree's own checks do not depend on the query: with a share they run once per tree.
	var actualEntry *ordinaryActualEntry
	if actualTreeReady {
		var e *Error
		if actualEntry, e = b.ordinaryActualEntry(actual); e != nil {
			return j.value(), e
		}
		check, e := b.ordinaryActualTreeCheck(actualEntry, actual.Nodes, id.Source, source)
		if e != nil {
			return j.value(), e
		}
		if check.invalid != "" {
			j.fail("ORDINARY_ACTUAL_TREE_INVALID", check.invalid)
		}
		if check.points {
			j.fail("ORDINARY_ACTUAL_TREE_POINTS", "native tree point가 선택된 source와 다르다")
		}
	}

	// Confirm producer metadata before comparing the full tree. A metadata failure must
	// remain visible even when the tree itself only differs in fields.
	var metadata QueryWitnessMetadata
	metadataReady := false
	if len(metadataData) == 0 {
		j.block("ORDINARY_METADATA_UNAVAILABLE", "독립 query metadata가 제공되지 않았다")
	} else if shared, _, e := ordinaryExpectedDecode[QueryWitnessMetadata](b, "ordinary-expected-metadata", metadataData); e != nil {
		if e.Kind != KindInvalidInput {
			return j.value(), e
		}
		j.fail(e.Code, "독립 query metadata가 strict decode를 통과하지 못했다")
	} else if metadata = *shared; !ordinaryStructuralMetadataValid(metadata) {
		j.fail("ORDINARY_EXPECTED_METADATA_INVALID", "metadata는 predicates가 명시적으로 빈 structural-only 목록이어야 한다")
	} else {
		metadataReady = true
	}
	query, queryOK := ordinaryActualQuery(actual, id.Query)
	if !queryOK {
		j.fail("ORDINARY_ACTUAL_QUERY_MISSING", "checked native tree에 등록 query가 없다")
	} else {
		if query.Predicates == nil || len(query.Predicates) != 0 {
			j.fail("ORDINARY_STRUCTURAL_PREDICATES", "actual query metadata에 예상치 못한 predicate가 있다")
		}
		if metadataReady && !reflect.DeepEqual(metadata, QueryWitnessMetadata{Patterns: query.Patterns, CaptureNames: query.CaptureNames, Predicates: query.Predicates}) {
			j.fail("ORDINARY_METADATA_MISMATCH", "native compiled metadata가 독립 metadata와 다르다")
		}
	}

	var expectedTree *ordinaryExpectedTree
	if len(treeData) == 0 {
		j.block("ORDINARY_EXPECTED_TREE_UNAVAILABLE", "독립 full CST가 제공되지 않았다")
	} else {
		v, entry, e := ordinaryExpectedDecode[ordinaryExpectedTree](b, "ordinary-expected-tree", treeData)
		if e != nil {
			if e.Kind != KindInvalidInput {
				return j.value(), e
			}
			j.fail(e.Code, "독립 expected tree가 strict decode를 통과하지 못했다")
		} else if !ordinaryTreeIdentity(v.Schema, v.Case, v.Step, v.TreeRole, v.Source, id) {
			j.fail("ORDINARY_EXPECTED_TREE_BINDING", "expected tree의 case/step/role/source 결속이 다르다")
		} else {
			check, e := b.ordinaryExpectedTreeCheck(entry, v, id.Source, source)
			if e != nil {
				return j.value(), e
			}
			if check.code != "" {
				j.fail(check.code, check.detail)
			} else {
				expectedTree = v
			}
		}
	}

	var captures []Capture
	var captureReady bool
	if queryOK {
		if query.Status != StatusCompleted || query.Partial || query.Error != nil {
			j.block("ORDINARY_ACTUAL_QUERY_INCOMPLETE", "structural query가 완료되지 않았다")
		}
		if query.Captures == nil {
			j.block("ORDINARY_ACTUAL_CAPTURES_UNAVAILABLE", "actual structural capture stream이 없다")
		} else {
			rows := uint64(len(query.Captures))
			logical, e := eligibilityProduct("ordinary-actual-captures", rows, 512)
			if e != nil {
				return j.value(), e
			}
			work, e := eligibilityProduct("ordinary-actual-captures", rows, 16)
			if e != nil {
				return j.value(), e
			}
			if e := b.charge(work+uint64(len(source))+1, logical, "ordinary-actual-captures"); e != nil {
				return j.value(), e
			}
			var ok bool
			if !actualTreeReady {
				j.block("ORDINARY_ACTUAL_CAPTURES_UNAVAILABLE", "full tree가 없어 node link를 검증할 수 없다")
			} else {
				captures, ok = ordinaryCaptureStream(query, actual.Nodes, source)
			}
			if !ok {
				j.fail("ORDINARY_ACTUAL_CAPTURE_INVALID", "actual structural capture의 node, pattern, 순서 또는 point가 유효하지 않다")
			} else {
				captureReady = true
			}
		}
	}

	var expectedCaptures *ordinaryExpectedCaptures
	if len(captureData) == 0 {
		j.block("ORDINARY_EXPECTED_CAPTURES_UNAVAILABLE", "독립 ordered capture stream이 제공되지 않았다")
	} else {
		v, _, e := ordinaryExpectedDecode[ordinaryExpectedCaptures](b, "ordinary-expected-captures", captureData)
		if e != nil {
			if e.Kind != KindInvalidInput {
				return j.value(), e
			}
			j.fail(e.Code, "독립 expected captures가 strict decode를 통과하지 못했다")
		} else if !ordinaryCaptureIdentity(v.Schema, v.Case, v.Query, v.Step, v.TreeRole, v.Source, id) {
			j.fail("ORDINARY_EXPECTED_CAPTURE_BINDING", "expected captures의 case/query/step/role/source 결속이 다르다")
		} else {
			expectedCaptures = v
		}
	}
	if expectedCaptures != nil && captureReady {
		rows := uint64(len(expectedCaptures.Ordered))
		logical, e := eligibilityProduct("ordinary-expected-captures", rows, 128)
		if e != nil {
			return j.value(), e
		}
		work, e := eligibilityProduct("ordinary-expected-captures", rows, 16)
		if e != nil {
			return j.value(), e
		}
		if e := b.charge(work+uint64(len(source))+1, logical, "ordinary-expected-captures"); e != nil {
			return j.value(), e
		}
		if metadataReady && expectedTree != nil && !ordinaryCapturesValid(expectedCaptures.Ordered, metadata, expectedTree.Tree, source) {
			j.fail("ORDINARY_EXPECTED_CAPTURE_INVALID", "expected capture가 tree/metadata/source에 연결되지 않는다")
		} else if metadataReady && expectedTree == nil && !ordinaryCapturesMetadataValid(expectedCaptures.Ordered, metadata, source) {
			j.fail("ORDINARY_EXPECTED_CAPTURE_INVALID", "expected capture metadata 또는 source range가 유효하지 않다")
		}
		if actualMatches := query.Matches; actualMatches != expectedCaptures.Matches {
			j.fail("ORDINARY_CAPTURE_MATCHES_MISMATCH", fmt.Sprintf("matches %d != %d", actualMatches, expectedCaptures.Matches))
		}
		if difference := CompareCaptures(captures, expectedCaptures.Ordered); difference != nil {
			j.fail("ORDINARY_CAPTURE_MISMATCH", fmt.Sprintf("capture[%d].%s %s != %s", difference.Index, difference.Field, difference.Left, difference.Right))
		}
	}

	if expectedTree != nil && actualTreeReady {
		compared, e := b.ordinaryActualTreeCompare(actualEntry, actual.Nodes, expectedTree)
		if e != nil {
			return j.value(), e
		}
		if compared.changed {
			if compared.fieldOnly {
				j.block("ORDINARY_EXPECTED_TREE_FIELD_DIFFERENCE", "full CST field 차이로 ordinary tuple을 판정할 수 없다")
			} else {
				j.fail("ORDINARY_EXPECTED_TREE_MISMATCH", compared.detail)
			}
		}
	}

	if !actualTreeReady {
		// API interpretation depends on the complete node table.
	} else if actual.API == nil {
		j.fail("ORDINARY_API_UNAVAILABLE", "API=true 요청의 full tree에 raw API table이 없다")
	} else {
		check, e := b.ordinaryActualAPICheck(actualEntry, actual)
		if e != nil {
			return j.value(), e
		}
		if check.code != "" {
			j.fail(check.code, check.detail)
		}
	}

	needsFact := facts != nil && ordinaryFactQueryForTuple(facts.Pack, facts.PackRef.Route, id.Query)
	if needsFact || len(factData) != 0 {
		if facts == nil {
			j.fail("ORDINARY_FACT_INPUTS_UNAVAILABLE", "pinned fact pack 또는 mapping 입력이 없다")
		} else if len(factData) == 0 {
			j.block("ORDINARY_EXPECTED_FACT_UNAVAILABLE", "독립 expected fact가 제공되지 않았다")
		} else if !captureReady || !queryOK {
			j.block("ORDINARY_FACT_ACTUAL_UNAVAILABLE", "fact 재계산에 필요한 structural capture가 없다")
		} else if e := adjudicateOrdinaryFact(b, id, captures, source, query, factData, *facts, j); e != nil {
			return j.value(), e
		}
	}

	return j.value(), nil
}

func ordinaryIdentityValid(id ordinaryTupleIdentity) bool {
	return id.Case != "" && id.Query != "" && (id.TreeRole == "incremental" || id.TreeRole == "fresh") && (id.Step != 0 || id.TreeRole != "fresh") && ordinaryPin(id.Source)
}

func ordinaryTreeIdentity(schema, caseID string, step uint32, role string, source OrdinaryPin, id ordinaryTupleIdentity) bool {
	return schema == ordinaryExpectedTreeSchema && caseID == id.Case && step == id.Step && role == id.TreeRole && source == id.Source
}

func ordinaryCaptureIdentity(schema, caseID, query string, step uint32, role string, source OrdinaryPin, id ordinaryTupleIdentity) bool {
	return schema == ordinaryExpectedCaptureSchema && caseID == id.Case && query == id.Query && step == id.Step && role == id.TreeRole && source == id.Source
}

func ordinaryStructuralMetadataValid(m QueryWitnessMetadata) bool {
	if m.Patterns == 0 || m.CaptureNames == nil || m.Predicates == nil || len(m.Predicates) != 0 {
		return false
	}
	seen := make(map[string]bool, len(m.CaptureNames))
	for _, name := range m.CaptureNames {
		if name == "" || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

func ordinaryActualQuery(tree nativewire.Tree, id string) (q querywire.Query, ok bool) {
	for _, value := range tree.Queries {
		if value.ID == id {
			return value, true
		}
	}
	return querywire.Query{}, false
}

func ordinaryCaptureStream(q querywire.Query, nodes []TreeNode, source []byte) ([]Capture, bool) {
	if q.CaptureNames == nil || q.Predicates == nil || q.Types == nil || !ordinaryCaptureMetadataValid(q) {
		return nil, false
	}
	out := make([]Capture, 0, len(q.Captures))
	for _, row := range q.Captures {
		if len(row) != 12 || row[0] < 0 || row[0] > int64(^uint32(0)) || row[1] < 0 || row[1] >= int64(q.Patterns) || row[2] < 0 || row[2] >= int64(len(q.CaptureNames)) || row[3] < 0 || row[3] >= int64(len(nodes)) || row[4] < 0 || row[4] >= int64(len(q.Types)) || row[5] < 0 || row[5] > 31 {
			return nil, false
		}
		for _, v := range row[6:] {
			if v < 0 || v > int64(^uint32(0)) {
				return nil, false
			}
		}
		out = append(out, Capture{Match: uint32(row[0]), Pattern: uint32(row[1]), Capture: uint32(row[2]), Name: q.CaptureNames[row[2]], Node: row[3], Type: q.Types[row[4]],
			Named: row[5]&1 != 0, Extra: row[5]&2 != 0, IsError: row[5]&4 != 0, HasError: row[5]&8 != 0, IsMissing: row[5]&16 != 0,
			StartByte: uint32(row[6]), EndByte: uint32(row[7]), StartPoint: Point{Row: uint32(row[8]), Column: uint32(row[9])}, EndPoint: Point{Row: uint32(row[10]), Column: uint32(row[11])}})
	}
	for _, capture := range out {
		if !witnessCaptureNode(capture, nodes) {
			return nil, false
		}
	}
	return out, ordinaryCapturesMetadataValid(out, QueryWitnessMetadata{Patterns: q.Patterns, CaptureNames: q.CaptureNames, Predicates: q.Predicates}, source) && querywire.MatchAccounting(q.Matches, len(out), func(index int) (int64, int64) { return int64(out[index].Match), int64(out[index].Pattern) })
}

func ordinaryCaptureMetadataValid(q querywire.Query) bool {
	if q.Patterns == 0 || q.CaptureNames == nil || q.Predicates == nil || q.Types == nil || len(q.Predicates) != 0 {
		return false
	}
	for i, name := range q.CaptureNames {
		if name == "" || slices.Contains(q.CaptureNames[:i], name) {
			return false
		}
	}
	for i, name := range q.Types {
		if name == "" || slices.Contains(q.Types[:i], name) {
			return false
		}
	}
	return true
}

func ordinaryCapturesMetadataValid(captures []Capture, metadata QueryWitnessMetadata, source []byte) bool {
	if metadata.CaptureNames == nil || metadata.Predicates == nil || metadata.Patterns == 0 {
		return false
	}
	type occurrence struct {
		match, pattern, capture uint32
		node                    int64
	}
	seen := make(map[occurrence]bool, len(captures))
	for _, c := range captures {
		if c.Pattern >= metadata.Patterns || c.Capture >= uint32(len(metadata.CaptureNames)) || c.Name != metadata.CaptureNames[c.Capture] || c.StartByte > c.EndByte || uint64(c.EndByte) > uint64(len(source)) {
			return false
		}
		key := occurrence{c.Match, c.Pattern, c.Capture, c.Node}
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return ordinaryCapturePointsValid(captures, source)
}

func ordinaryCapturesValid(captures []Capture, metadata QueryWitnessMetadata, nodes []TreeNode, source []byte) bool {
	if !ordinaryCapturesMetadataValid(captures, metadata, source) {
		return false
	}
	for _, c := range captures {
		if !witnessCaptureNode(c, nodes) {
			return false
		}
	}
	return true
}

func ordinaryCapturePointsValid(captures []Capture, source []byte) bool {
	nodes := make([]TreeNode, 0, len(captures))
	for _, c := range captures {
		nodes = append(nodes, TreeNode{StartByte: c.StartByte, EndByte: c.EndByte, StartPoint: c.StartPoint, EndPoint: c.EndPoint})
	}
	return witnessPointsChecked(nodes, EncodingUTF8, source, nil) == ""
}

func ordinaryTreePointsValid(nodes []TreeNode, source []byte) bool {
	return witnessPointsChecked(nodes, EncodingUTF8, source, nil) == ""
}

func ordinaryTreeFieldOnlyDifference(actual, expected []TreeNode) (fieldOnly, changed bool) {
	if len(actual) != len(expected) {
		return false, true
	}
	for i := range actual {
		a, e := actual[i], expected[i]
		if a.Parent != e.Parent || a.Type != e.Type || a.Named != e.Named || a.Extra != e.Extra || a.IsError != e.IsError || a.HasError != e.HasError || a.IsMissing != e.IsMissing || a.StartByte != e.StartByte || a.EndByte != e.EndByte || a.StartPoint != e.StartPoint || a.EndPoint != e.EndPoint {
			return false, true
		}
		if (a.Field == nil) != (e.Field == nil) || a.Field != nil && *a.Field != *e.Field {
			changed = true
		}
	}
	return changed, changed
}

func ordinaryAPIShapeValid(api *nativewire.WireAPI, nodes int) bool {
	if api.Revision != "tsgk-api/r1" || len(api.Nodes) != nodes || api.Names == nil || api.Points == nil || api.ChildFields == nil || api.FieldLookups == nil {
		return false
	}
	for _, row := range api.Nodes {
		if len(row) != 13 {
			return false
		}
	}
	for _, row := range api.ChildFields {
		if len(row) != 3 || row[0] < 0 || row[0] >= int64(nodes) || row[1] < 0 || row[2] < 0 || row[2] >= int64(len(api.Names)) {
			return false
		}
	}
	for _, row := range api.FieldLookups {
		if len(row) != 3 || row[0] < 0 || row[0] >= int64(nodes) || row[1] < 0 || row[1] >= int64(len(api.Names)) || row[2] < 0 || row[2] >= int64(nodes) {
			return false
		}
	}
	for _, row := range api.Points {
		if len(row) != 4 || row[0] < 0 {
			return false
		}
		for _, node := range row[1:] {
			if node < -1 || node >= int64(nodes) {
				return false
			}
		}
	}
	return true
}

func adjudicateOrdinaryFact(b *eligibilityBudget, id ordinaryTupleIdentity, captures []Capture, source []byte, query querywire.Query, expectedData []byte, inputs ordinaryFactInputs, judgment *ordinaryJudgmentBuilder) *Error {
	var expected ordinaryExpectedFact
	if _, e := b.decodeOrdinaryExpected("ordinary-expected-facts", expectedData, &expected); e != nil {
		if e.Kind != KindInvalidInput {
			return e
		}
		judgment.fail(e.Code, "독립 expected fact가 strict decode를 통과하지 못했다")
		return nil
	}
	if expected.Schema != ordinaryExpectedFactSchema || expected.Case != id.Case || expected.Query != id.Query || expected.Step != id.Step || expected.TreeRole != id.TreeRole || expected.Source != id.Source {
		judgment.fail("ORDINARY_EXPECTED_FACT_BINDING", "expected fact의 tuple/source 결속이 다르다")
		return nil
	}
	if !inputs.Ready || !ordinaryPin(inputs.PackPin) || inputs.Pack.SHA256 != inputs.PackPin.SHA256 || inputs.PackPin.Bytes != uint64(len(inputs.PackData)) || inputs.PackRef.Revision != inputs.Pack.Revision || inputs.PackRef.SHA256 != inputs.PackPin.SHA256 || inputs.PackRef.Route == "" {
		judgment.fail("ORDINARY_FACT_INPUT_BINDING", "fact pack 또는 mapping pin이 유효하지 않다")
		return nil
	}
	if len(inputs.Mapping) == 0 || inputs.MappingPin.Schema != "tsgk-fact-mapping/r1" || inputs.MappingPin.SHA256 != digestHex(inputs.Mapping) || inputs.MappingPin.Revision != inputs.Pack.Mapping {
		judgment.fail("ORDINARY_FACT_MAPPING_BINDING", "fact mapping 원문 또는 revision pin이 다르다")
		return nil
	}
	if expected.Pack != inputs.PackRef || expected.Mapping != inputs.MappingPin {
		judgment.fail("ORDINARY_EXPECTED_FACT_PIN", "expected fact의 pack/mapping pin이 선택된 입력과 다르다")
		return nil
	}
	route, ok := ordinaryFactRoute(inputs.Pack, inputs.PackRef.Route)
	if !ok {
		judgment.fail("ORDINARY_FACT_ROUTE", "pinned fact pack에 선택 route가 없다")
		return nil
	}
	queryIndex := slices.IndexFunc(route.Queries, func(q FactQuery) bool { return q.ID == id.Query })
	if queryIndex < 0 {
		judgment.fail("ORDINARY_FACT_QUERY", "선택 query가 pinned fact suffix에 없다")
		return nil
	}
	factQuery := route.Queries[queryIndex]
	queryPin := ordinaryFactQueryPin{ID: factQuery.ID, SHA256: digestHex([]byte(factQuery.Source)), Kind: factQuery.Facts}
	if expected.FactQuery != queryPin || query.Status != StatusCompleted || query.Partial || query.Captures == nil {
		judgment.fail("ORDINARY_FACT_QUERY_BINDING", "actual query 또는 expected fact query pin이 다르다")
		return nil
	}
	if e := b.charge(uint64(len(captures))+1, 256*uint64(len(captures)), "ordinary-fact-adjudication"); e != nil {
		return e
	}
	actualFact := ordinaryExpectedFact{Status: StatusCompleted, Code: ""}
	switch factQuery.Facts {
	case FactDeclarations:
		items := DeriveDeclarations(route.Declarations, captures)
		actualFact.Declarations = &ordinaryExpectedDeclarations{Items: items, Assessment: ordinaryDeclarationAssessment(items)}
	case FactDynamicSQL:
		if e := b.factReservation(FactDynamicSQL, expected.Pack.Route, len(captures), func(i int) Capture { return captures[i] }, source); e != nil {
			return e
		}
		facts, err := DeriveDynamicSQLChecked(expected.Pack.Route, EncodingUTF8, captures, source)
		if err != nil {
			actualFact.Status, actualFact.Code = StatusFailed, "FACT_DERIVATION_FAILED"
		} else {
			actualFact.DynamicSQL = &facts
		}
	case FactXMLStructure:
		xml := make(map[string]int)
		for _, capture := range captures {
			xml[capture.Name]++
		}
		actualFact.XML = &xml
	default:
		judgment.fail("ORDINARY_FACT_KIND", "unknown pinned fact query kind")
		return nil
	}
	if (factQuery.Facts == FactDeclarations && (expected.Declarations == nil || expected.DynamicSQL != nil || expected.XML != nil)) ||
		(factQuery.Facts == FactDynamicSQL && (expected.DynamicSQL == nil || expected.Declarations != nil || expected.XML != nil)) ||
		(factQuery.Facts == FactXMLStructure && (expected.XML == nil || expected.Declarations != nil || expected.DynamicSQL != nil)) {
		judgment.fail("ORDINARY_EXPECTED_FACT_SHAPE", "fact kind와 expected payload가 다르다")
		return nil
	}
	if expected.Status != StatusCompleted && expected.Status != StatusFailed {
		judgment.fail("ORDINARY_EXPECTED_FACT_STATUS", "expected fact status가 완료 또는 실패가 아니다")
		return nil
	}
	if expected.Status == StatusFailed {
		code := expected.Code
		if code == "" {
			code = "ORDINARY_EXPECTED_FACT_FAILED"
		}
		judgment.fail(code, "독립 expected facts가 이 tuple을 실패로 판정한다")
	}
	if expected.Status != actualFact.Status || expected.Code != actualFact.Code || !reflect.DeepEqual(expected.Declarations, actualFact.Declarations) || !reflect.DeepEqual(expected.DynamicSQL, actualFact.DynamicSQL) || !reflect.DeepEqual(expected.XML, actualFact.XML) {
		judgment.fail("ORDINARY_FACT_MISMATCH", "source와 structural captures에서 재계산한 fact가 독립 expected fact와 다르다")
	}
	producer, ok := inputs.ProducerByCase[id.Case]
	if !ok {
		judgment.fail("ORDINARY_PRODUCER_FACT_MISSING", "r5 case record에 producer step facts가 없다")
	} else if e := verifyOrdinaryProducerFact(b, producer, id, expected, inputs); e != nil {
		if e.Kind != KindInvalidInput {
			return e
		}
		judgment.fail(e.Code, "r5 producer step facts가 independently derived fact와 다르다")
	}
	return nil
}

func ordinaryFactQueryForTuple(pack FactPack, route, query string) bool {
	r, ok := ordinaryFactRoute(pack, route)
	return ok && slices.ContainsFunc(r.Queries, func(q FactQuery) bool { return q.ID == query })
}

func verifyOrdinaryProducerFact(b *eligibilityBudget, data []byte, id ordinaryTupleIdentity, expected ordinaryExpectedFact, inputs ordinaryFactInputs) *Error {
	var record ordinaryProducerFactRecord
	if e := b.decodeWitnessTyped("ordinary-producer-step-facts", data, &record, true); e != nil {
		return e
	}
	if record.Schema != "tsgk-ordinary-step-facts/r1" || record.Case != id.Case || record.Pack != inputs.PackRef || record.Mapping != inputs.MappingPin ||
		record.DynamicSQLRevision != inputs.Pack.DynamicSQL || record.XMLRevision != inputs.Pack.XML || !slices.Equal(record.FactQueries, ordinaryPackFactQueryPins(inputs.Pack, inputs.PackRef.Route)) ||
		record.FactSuffixSHA256 != ordinaryFactSuffixDigest(record.FactQueries) {
		return fail(KindInvalidInput, "ORDINARY_PRODUCER_FACT_BINDING", "record/step_facts", nil)
	}
	var step *ordinaryProducerFactStep
	for i := range record.Steps {
		candidate := &record.Steps[i]
		if candidate.Step == int(id.Step) && candidate.TreeRole == id.TreeRole {
			if step != nil {
				return fail(KindInvalidInput, "ORDINARY_PRODUCER_FACT_DUPLICATE", "record/step_facts", nil)
			}
			step = candidate
		}
	}
	if step == nil || step.SourceBytes != id.Source.Bytes || step.SourceSHA256 != id.Source.SHA256 {
		return fail(KindInvalidInput, "ORDINARY_PRODUCER_FACT_SOURCE", "record/step_facts", nil)
	}
	index := slices.IndexFunc(step.Queries, func(q ordinaryProducerFactQuery) bool { return q.ID == id.Query })
	if index < 0 {
		return fail(KindInvalidInput, "ORDINARY_PRODUCER_FACT_QUERY_MISSING", "record/step_facts", nil)
	}
	got := step.Queries[index]
	if got.SHA256 != expected.FactQuery.SHA256 || got.Kind != expected.FactQuery.Kind || got.Status != expected.Status || got.Code != expected.Code || !ordinaryProducerFactValueEqual(got, expected) {
		return fail(KindInvalidInput, "ORDINARY_PRODUCER_FACT_MISMATCH", "record/step_facts", nil)
	}
	return b.r.check()
}

func ordinaryProducerFactValueEqual(got ordinaryProducerFactQuery, want ordinaryExpectedFact) bool {
	switch want.FactQuery.Kind {
	case FactDeclarations:
		return got.Declarations != nil && got.Declarations.Assessment == want.Declarations.Assessment && reflect.DeepEqual(got.Declarations.Items, want.Declarations.Items)
	case FactDynamicSQL:
		return reflect.DeepEqual(got.DynamicSQL, want.DynamicSQL)
	case FactXMLStructure:
		return want.XML != nil && reflect.DeepEqual(got.XML, *want.XML)
	default:
		return false
	}
}

func ordinaryPackFactQueryPins(pack FactPack, route string) []ordinaryFactQueryPin {
	r, ok := ordinaryFactRoute(pack, route)
	if !ok {
		return nil
	}
	pins := make([]ordinaryFactQueryPin, len(r.Queries))
	for i, q := range r.Queries {
		pins[i] = ordinaryFactQueryPin{ID: q.ID, SHA256: digestHex([]byte(q.Source)), Kind: q.Facts}
	}
	return pins
}

func ordinaryFactSuffixDigest(pins []ordinaryFactQueryPin) string {
	h := sha256.New()
	_, _ = h.Write([]byte("tsgk-ordinary-fact-suffix/r1\x00"))
	var size [8]byte
	for _, pin := range pins {
		for _, value := range []string{pin.ID, pin.SHA256, pin.Kind} {
			binary.BigEndian.PutUint64(size[:], uint64(len(value)))
			_, _ = h.Write(size[:])
			_, _ = h.Write([]byte(value))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (b *eligibilityBudget) ordinaryFactContext(s *eligibilitySession, manifest OrdinaryOracleManifest, profile OracleProfile, producerByCase map[string][]byte) (*ordinaryFactInputs, *Error) {
	if profile.FactPack == nil {
		return nil, nil
	}
	var packMember, mappingMember *OrdinaryMember
	for i := range manifest.Members {
		member := &manifest.Members[i]
		if member.Owner != "O" || member.CaseOrBatch != "run" {
			continue
		}
		switch member.Role {
		case "fact-pack":
			if packMember != nil {
				return nil, fail(KindInvalidInput, "ORDINARY_FACT_PACK_DUPLICATE", "manifest", nil)
			}
			packMember = member
		case "fact-mapping":
			if mappingMember != nil {
				return nil, fail(KindInvalidInput, "ORDINARY_FACT_MAPPING_DUPLICATE", "manifest", nil)
			}
			mappingMember = member
		}
	}
	if packMember == nil || mappingMember == nil || packMember.Path != packMember.Pin.Path || mappingMember.Path != mappingMember.Pin.Path || !ordinaryPin(packMember.Pin) || !ordinaryPin(mappingMember.Pin) || s == nil || s.snapshot == nil {
		return nil, fail(KindInvalidInput, "ORDINARY_FACT_MEMBERS_MISSING", "manifest", nil)
	}
	packData, packOK := s.snapshot.files["oracle/"+packMember.Path]
	mappingData, mappingOK := s.snapshot.files["oracle/"+mappingMember.Path]
	if packOK && mappingOK {
		if e := b.charge(uint64(len(packData)+len(mappingData))+1, 0, "ordinary-fact-member-pins"); e != nil {
			return nil, e
		}
	}
	if !packOK || !mappingOK || uint64(len(packData)) != packMember.Pin.Bytes || digestHex(packData) != packMember.Pin.SHA256 || uint64(len(mappingData)) != mappingMember.Pin.Bytes || digestHex(mappingData) != mappingMember.Pin.SHA256 {
		return nil, fail(KindInvalidInput, "ORDINARY_FACT_MEMBER_PIN", "manifest", nil)
	}
	if e := b.reserveJSON("ordinary-fact-pack", packData, 4); e != nil {
		return nil, e
	}
	pack, err := ParseFactPack(packData)
	if err != nil {
		if e, ok := err.(*Error); ok {
			return nil, e
		}
		return nil, fail(KindInvalidInput, "ORDINARY_FACT_PACK", packMember.Path, err)
	}
	var mapping struct {
		Schema   string `json:"schema"`
		Revision string `json:"revision"`
	}
	if e := b.decodeWitnessTyped("ordinary-fact-mapping", mappingData, &mapping, false); e != nil {
		return nil, e
	}
	ref := *profile.FactPack
	if ref.SHA256 != packMember.Pin.SHA256 || ref.Revision != pack.Revision || ref.Route == "" || mapping.Schema != "tsgk-fact-mapping/r1" || mapping.Revision != pack.Mapping {
		return nil, fail(KindInvalidInput, "ORDINARY_FACT_PROFILE_PIN", "oracle/fact_pack", nil)
	}
	route, ok := ordinaryFactRoute(pack, ref.Route)
	if !ok || len(profile.Queries) < len(route.Queries) {
		return nil, fail(KindInvalidInput, "ORDINARY_FACT_ROUTE", "oracle/fact_pack", nil)
	}
	start := len(profile.Queries) - len(route.Queries)
	for i, query := range route.Queries {
		if profile.Queries[start+i].ID != query.ID || profile.Queries[start+i].Source != query.Source {
			return nil, fail(KindInvalidInput, "ORDINARY_FACT_SUFFIX", "oracle/queries", nil)
		}
	}
	if e := b.charge(uint64(len(route.Queries))+uint64(len(profile.Queries))+1, 0, "ordinary-fact-context"); e != nil {
		return nil, e
	}
	return &ordinaryFactInputs{Pack: pack, PackData: packData, PackRef: ref, PackPin: packMember.Pin, Mapping: mappingData,
		MappingPin: ordinaryFactMappingPin{Schema: mapping.Schema, Revision: mapping.Revision, SHA256: mappingMember.Pin.SHA256}, ProducerByCase: producerByCase, Ready: true}, nil
}

func ordinaryFactRoute(pack FactPack, route string) (FactPackRoute, bool) {
	index := slices.IndexFunc(pack.Routes, func(r FactPackRoute) bool { return r.Route == route })
	if index < 0 {
		return FactPackRoute{}, false
	}
	return pack.Routes[index], true
}

func ordinaryDeclarationAssessment(items []DeclarationItem) string {
	if len(items) == 0 {
		return "NOT_APPLICABLE"
	}
	for _, item := range items {
		if item.Status != AssessPass {
			return AssessFail
		}
	}
	return AssessPass
}
