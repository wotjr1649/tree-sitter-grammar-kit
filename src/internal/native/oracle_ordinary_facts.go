package native

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const ordinaryStepFactsSchema = "tsgk-ordinary-step-facts/r1"

// OrdinaryStepFacts is producer-local derivation evidence. Its pack pin binds the mapping
// revisions, while FactQueries preserves their exact ordered suffix and query hashes.
type OrdinaryStepFacts struct {
	Schema             string                  `json:"schema"`
	Case               string                  `json:"case"`
	Pack               kit.FactPackRef         `json:"pack"`
	Mapping            OrdinaryFactMappingPin  `json:"mapping"`
	DynamicSQLRevision string                  `json:"dynamic_sql_revision"`
	XMLRevision        string                  `json:"xml_revision"`
	FactSuffixSHA256   string                  `json:"fact_suffix_sha256"`
	FactQueries        []OrdinaryFactQueryPin  `json:"fact_queries"`
	Steps              []OrdinaryTreeRoleFacts `json:"steps"`
}

type OrdinaryFactMappingPin struct {
	Schema   string `json:"schema"`
	Revision string `json:"revision"`
	SHA256   string `json:"sha256"`
}

type OrdinaryFactQueryPin struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
}

type OrdinaryTreeRoleFacts struct {
	Step         int                 `json:"step"`
	TreeRole     string              `json:"tree_role"`
	SourceBytes  uint64              `json:"source_bytes"`
	SourceSHA256 string              `json:"source_sha256"`
	Status       string              `json:"status"`
	Code         string              `json:"code,omitempty"`
	Queries      []OrdinaryFactQuery `json:"queries"`
}

type OrdinaryFactQuery struct {
	ID           string               `json:"id"`
	SHA256       string               `json:"sha256"`
	Kind         string               `json:"kind"`
	Status       string               `json:"status"`
	Code         string               `json:"code,omitempty"`
	Difference   string               `json:"first_difference,omitempty"`
	Declarations *DeclFacts           `json:"declarations,omitempty"`
	DynamicSQL   *kit.DynamicSQLFacts `json:"dynamic_sql,omitempty"`
	XML          map[string]int       `json:"xml_structure,omitempty"`
}

// ordinaryStepFacts derives each registered fact query from the captures owned by that
// step and tree role. It never substitutes an incremental stream for a fresh one.
func ordinaryStepFacts(cr *CaseResult, prof kit.OracleProfile, pack *kit.FactPack, x Context, src, mapping []byte) (OrdinaryStepFacts, error) {
	if cr == nil {
		return OrdinaryStepFacts{}, fmt.Errorf("ordinary fact case missing")
	}
	mappingPin, factQueries, suffixSHA256, err := validateOrdinaryFactPins(prof, pack, mapping)
	if err != nil {
		return OrdinaryStepFacts{}, err
	}
	routeIndex := slices.IndexFunc(pack.Routes, func(r kit.FactPackRoute) bool { return r.Route == prof.FactPack.Route })
	route := pack.Routes[routeIndex]
	caseIndex := slices.IndexFunc(prof.OracleCases, func(c kit.OracleCase) bool { return cr != nil && c.ID == cr.ID })
	if caseIndex < 0 {
		return OrdinaryStepFacts{}, fmt.Errorf("ordinary fact case missing")
	}
	oc := prof.OracleCases[caseIndex]
	versions, _, err := kit.ApplyEdits(oc.Encoding, src, oc.Edits, x.Op.InputBytes)
	if err != nil {
		return OrdinaryStepFacts{}, fmt.Errorf("ordinary fact source history invalid: %w", err)
	}

	out := OrdinaryStepFacts{
		Schema: ordinaryStepFactsSchema, Case: cr.ID,
		Pack: *prof.FactPack, Mapping: mappingPin,
		DynamicSQLRevision: pack.DynamicSQL, XMLRevision: pack.XML,
		FactSuffixSHA256: suffixSHA256, FactQueries: factQueries,
		Steps: make([]OrdinaryTreeRoleFacts, 0, len(versions)*2),
	}
	for step, version := range versions {
		var recorded *StepResult
		if step < len(cr.Steps) && cr.Steps[step].Step == step {
			recorded = &cr.Steps[step]
		}
		for _, role := range []string{"incremental", "fresh"} {
			if role == "fresh" && step == 0 {
				continue
			}
			treeRole := OrdinaryTreeRoleFacts{Step: step, TreeRole: role, SourceBytes: uint64(len(version)), SourceSHA256: sha(version),
				Status: kit.StatusCompleted, Queries: make([]OrdinaryFactQuery, len(route.Queries))}
			var tree *TreeOut
			treeAvailable := recorded != nil && recorded.SourceBytes == uint64(len(version)) && recorded.SourceSHA256 == treeRole.SourceSHA256
			if !treeAvailable {
				treeRole.Status, treeRole.Code = kit.StatusNotRun, "FACT_SOURCE_VERSION_MISMATCH"
			} else if role == "incremental" {
				tree = recorded.Incremental
			} else {
				tree = recorded.Fresh
			}
			if treeAvailable && (tree == nil || tree.Status != kit.StatusCompleted) {
				treeRole.Status, treeRole.Code = kit.StatusNotRun, "FACT_TREE_UNAVAILABLE"
			}
			roleStatus, roleCode := treeRole.Status, treeRole.Code
			for qIndex, q := range route.Queries {
				pin := out.FactQueries[qIndex]
				result := OrdinaryFactQuery{ID: q.ID, SHA256: pin.SHA256, Kind: q.Facts, Status: kit.StatusNotRun, Code: treeRole.Code}
				if treeAvailable && tree != nil && tree.Status == kit.StatusCompleted {
					queryIndex := slices.IndexFunc(tree.Queries, func(got QueryOut) bool { return got.ID == q.ID })
					switch {
					case queryIndex < 0:
						result.Code = "FACT_QUERY_UNAVAILABLE"
					case tree.Queries[queryIndex].SHA256 != pin.SHA256:
						result.Status, result.Code = kit.StatusFailed, "FACT_QUERY_SOURCE_MISMATCH"
					case tree.Queries[queryIndex].Status != kit.StatusCompleted || tree.Queries[queryIndex].Evaluation != EvalStructural || tree.Queries[queryIndex].Partial:
						result.Code = "FACT_QUERY_NOT_COMPLETE_STRUCTURAL"
					case tree.Queries[queryIndex].Captures == nil:
						result.Code = "FACT_CAPTURE_UNAVAILABLE"
					default:
						result, err = deriveOrdinaryFactQuery(q, pin, tree.Queries[queryIndex].Captures, tree, prof.FactPack.Route, oc.Encoding, version, route.Declarations)
						if err != nil {
							result.Status, result.Code = kit.StatusFailed, "FACT_DERIVATION_FAILED"
							result.Difference = err.Error()
						}
					}
				}
				treeRole.Queries[qIndex] = result
				if result.Status == kit.StatusFailed {
					roleStatus, roleCode = kit.StatusFailed, result.Code
				} else if result.Status != kit.StatusCompleted && roleStatus == kit.StatusCompleted {
					roleStatus, roleCode = result.Status, result.Code
				}
			}
			treeRole.Status, treeRole.Code = roleStatus, roleCode
			out.Steps = append(out.Steps, treeRole)
		}
	}
	return out, nil
}

// validateOrdinaryFactPins is the no-process preflight for pack, mapping and ordered
// query-suffix inputs. The same values are written into the per-case producer record.
func validateOrdinaryFactPins(prof kit.OracleProfile, pack *kit.FactPack, mapping []byte) (OrdinaryFactMappingPin, []OrdinaryFactQueryPin, string, error) {
	if prof.FactPack == nil || pack == nil || pack.SHA256 == "" || prof.FactPack.Revision != pack.Revision ||
		prof.FactPack.SHA256 != pack.SHA256 || prof.FactPack.Route == "" || len(mapping) == 0 {
		return OrdinaryFactMappingPin{}, nil, "", fmt.Errorf("ordinary fact pack pin mismatch")
	}
	var mappingIdentity struct {
		Schema   string `json:"schema"`
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(mapping, &mappingIdentity); err != nil || mappingIdentity.Schema != "tsgk-fact-mapping/r1" || mappingIdentity.Revision != pack.Mapping {
		return OrdinaryFactMappingPin{}, nil, "", fmt.Errorf("ordinary fact mapping pin mismatch")
	}
	routeIndex := slices.IndexFunc(pack.Routes, func(r kit.FactPackRoute) bool { return r.Route == prof.FactPack.Route })
	if routeIndex < 0 || len(pack.Routes[routeIndex].Queries) == 0 {
		return OrdinaryFactMappingPin{}, nil, "", fmt.Errorf("ordinary fact route missing")
	}
	route := pack.Routes[routeIndex]
	if err := validateOrdinaryFactSuffix(prof, route); err != nil {
		return OrdinaryFactMappingPin{}, nil, "", err
	}
	pins := make([]OrdinaryFactQueryPin, len(route.Queries))
	start := len(prof.Queries) - len(route.Queries)
	for i, q := range route.Queries {
		pins[i] = OrdinaryFactQueryPin{ID: q.ID, SHA256: sha([]byte(prof.Queries[start+i].Source)), Kind: q.Facts}
	}
	mappingPin := OrdinaryFactMappingPin{Schema: mappingIdentity.Schema, Revision: mappingIdentity.Revision, SHA256: sha(mapping)}
	return mappingPin, pins, ordinaryFactSuffixDigest(pins), nil
}

func validateOrdinaryFactSuffix(prof kit.OracleProfile, route kit.FactPackRoute) error {
	if len(prof.Queries) < len(route.Queries) {
		return fmt.Errorf("ordinary fact suffix missing")
	}
	start := len(prof.Queries) - len(route.Queries)
	for i, want := range route.Queries {
		got := prof.Queries[start+i]
		if got.ID != want.ID || got.Source != want.Source {
			return fmt.Errorf("ordinary fact suffix order or source mismatch at %d", i)
		}
	}
	return nil
}

func ordinaryFactSuffixDigest(queries []OrdinaryFactQueryPin) string {
	h := sha256.New()
	_, _ = h.Write([]byte("tsgk-ordinary-fact-suffix/r1\x00"))
	var size [8]byte
	for _, q := range queries {
		for _, value := range []string{q.ID, q.SHA256, q.Kind} {
			binary.BigEndian.PutUint64(size[:], uint64(len(value)))
			_, _ = h.Write(size[:])
			_, _ = h.Write([]byte(value))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func deriveOrdinaryFactQuery(q kit.FactQuery, pin OrdinaryFactQueryPin, captures []kit.Capture, tree *TreeOut, route, enc string, source []byte, declarations []kit.NativeDeclaration) (OrdinaryFactQuery, error) {
	result := OrdinaryFactQuery{ID: pin.ID, SHA256: pin.SHA256, Kind: pin.Kind, Status: kit.StatusCompleted}
	switch q.Facts {
	case kit.FactDeclarations:
		items := kit.DeriveDeclarations(declarations, captures)
		derived := &DeclFacts{Items: items, Assessment: declAssessment(items)}
		var prior []DeclItem
		if tree.Summary != nil {
			prior = tree.Summary.Declarations.Items
		} else if tree.Declarations != nil {
			prior = *tree.Declarations
		} else {
			return result, fmt.Errorf("declaration summary unavailable")
		}
		derived.Reproduces = sameOrdinaryDeclarations(items, prior)
		result.Declarations = derived
		if !derived.Reproduces {
			result.Status, result.Code = kit.StatusFailed, "FACT_REPRODUCTION_MISMATCH"
			result.Difference = fmt.Sprintf("declaration items %d != %d", len(items), len(prior))
		}
	case kit.FactDynamicSQL:
		facts, err := kit.DeriveDynamicSQLChecked(route, enc, captures, source)
		if err != nil {
			return result, err
		}
		result.DynamicSQL = &facts
	case kit.FactXMLStructure:
		result.XML = map[string]int{}
		for _, capture := range captures {
			result.XML[capture.Name]++
		}
	default:
		return result, fmt.Errorf("unknown fact kind %q", q.Facts)
	}
	return result, nil
}

func sameOrdinaryDeclarations(items []kit.DeclarationItem, prior []DeclItem) bool {
	if len(items) != len(prior) {
		return false
	}
	for i, a := range items {
		b := prior[i]
		if a.Fact != b.Fact || a.NodeType != b.NodeType || a.StartByte != b.StartByte || a.EndByte != b.EndByte || a.Status != b.Status ||
			(a.Name == nil) != (b.Name == nil) || (a.Name != nil && (a.Name.StartByte != b.Name.StartByte || a.Name.EndByte != b.Name.EndByte)) {
			return false
		}
	}
	return true
}
