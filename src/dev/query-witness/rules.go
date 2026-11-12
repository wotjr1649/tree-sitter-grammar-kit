package main

import (
	"fmt"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistry"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type ruleRoute struct {
	Route          string            `json:"route"`
	GrammarRef     string            `json:"grammar_ref"`
	Grammar        []kit.NativeInput `json:"grammar"`
	LanguageSource origin            `json:"language_source"`
}

type rule struct {
	Route       string `json:"route"`
	Query       string `json:"query"`
	SHA256      string `json:"query_sha256"`
	Source      origin `json:"query_source"`
	Declaration string `json:"declaration"`
	GrammarRef  string `json:"grammar_basis"`
	Rule        string `json:"expectation_rule"`
	Automation  string `json:"automation"`
	Scope       string `json:"scope"`
	Exceptions  struct {
		State       string   `json:"state"`
		ManualCount *int     `json:"manual_count"`
		Items       []string `json:"items"`
	} `json:"manual_exceptions"`
	Review *queryregistry.Pin `json:"independent_review"`
}

type ruleLedger struct {
	Schema        string            `json:"schema"`
	Registry      queryregistry.Pin `json:"registry"`
	Qualification queryregistry.Pin `json:"qualification_inventory"`
	State         string            `json:"state"`
	Routes        []ruleRoute       `json:"routes"`
	Rules         []rule            `json:"rules"`
}

func buildRules(in *inputs) (ruleLedger, error) {
	out := ruleLedger{Schema: "tsgk-query-expectation-rules/r1", Registry: queryregistry.FilePin(queryregistry.Path, in.registry), Qualification: queryregistry.QualificationPin, State: "SKELETON_UNREVIEWED"}
	const languagePath = "src/contracts/language-sources.json"
	var languages struct {
		Routes []struct {
			Route string `json:"route_id"`
		} `json:"routes"`
	}
	if err := in.decode(languagePath, &languages); err != nil {
		return out, err
	}
	const factPath = "src/contracts/fact-query-pack.json"
	var facts struct {
		Routes []struct {
			Route   string `json:"route"`
			Queries []struct {
				ID     string `json:"id"`
				Source string `json:"source"`
			} `json:"queries"`
		} `json:"routes"`
	}
	if err := in.decode(factPath, &facts); err != nil {
		return out, err
	}
	for ri, r := range in.inv.Routes {
		var language *origin
		for i, l := range languages.Routes {
			if l.Route == r.Route {
				o := origin{queryregistry.FilePin(languagePath, in.files[languagePath]), fmt.Sprintf("#/routes/%d", i), r.Route}
				language = &o
				break
			}
		}
		if language == nil {
			return out, fmt.Errorf("language provenance missing: %s", r.Route)
		}
		out.Routes = append(out.Routes, ruleRoute{r.Route, fmt.Sprintf("qualification_inventory#/routes/%d/workload/grammar", ri), r.Workload.Grammar, *language})
		path := "src/testdata/native/queries/" + r.Route + ".json"
		var qf queryFile
		if err := in.decode(path, &qf); err != nil {
			return out, err
		}
		available := map[string]rule{}
		add := func(id, source, path, selector string) error {
			if _, ok := available[id]; ok {
				return fmt.Errorf("duplicate query %s/%s", r.Route, id)
			}
			available[id] = rule{Route: r.Route, Query: id, SHA256: queryregistry.Digest([]byte(source)), Source: origin{queryregistry.FilePin(path, in.files[path]), selector, id}, Declaration: source, GrammarRef: fmt.Sprintf("#/routes/%d", ri), Rule: "고정 query 선언의 node·field·capture 선택을 따른다. 문법별 CST/API·metadata·순서·fact mapping 규칙은 후속 자료 검토에서 작성한다.", Automation: "UNASSESSED", Scope: "PENDING_FULL_CST_API_METADATA_ORDERED_CAPTURES_FACTS"}
			return nil
		}
		for i, q := range qf.Queries {
			if err := add(q.ID, q.Source, path, fmt.Sprintf("#/queries/%d/source", i)); err != nil {
				return out, err
			}
		}
		for fi, f := range facts.Routes {
			if f.Route == r.Route {
				for qi, q := range f.Queries {
					if err := add(q.ID, q.Source, factPath, fmt.Sprintf("#/routes/%d/queries/%d/source", fi, qi)); err != nil {
						return out, err
					}
				}
			}
		}
		if len(available) != len(r.Workload.Queries) {
			return out, fmt.Errorf("query definition membership mismatch: %s", r.Route)
		}
		for _, q := range r.Workload.Queries {
			row, ok := available[q.ID]
			if !ok || row.SHA256 != q.SHA256 {
				return out, fmt.Errorf("query pin mismatch: %s/%s", r.Route, q.ID)
			}
			row.Exceptions.State = "UNASSESSED"
			row.Exceptions.Items = []string{}
			if r.Route == "json" && q.ID == "q.structure" {
				row.Rule = "JSON_OBJECT_MEMBERS/r1: RFC 8259 §4의 object member를 byte 순서로 방문한다. array 값의 key string 전체(따옴표 포함)는 pattern0/array_key, object 값은 pattern1/object_key, string 값은 pattern2/key 다음 value로 선택한다. match_index는 pattern을 가리지 않고 start_byte 순서로 처음 관측한 match에 query 전체 순번을 매기고, capture_index는 고정 query의 capture 이름 선언 순서(array_key=0, object_key=1, key=2, value=3)다. 중첩 member도 방문하고 결과는 start_byte 순서다. Go encoding/json Token·InputOffset으로 원문의 span을 계산하며 중복 key·escape·Unicode 원문을 보존한다. Tree-sitter pair의 key/value와 string/array/object mapping은 고정 query 선언 및 route grammar pin에 연결한다."
				row.Automation = "IMPLEMENTED_VALID_JSON_SUBSET"
				row.Scope = "ORDERED_CAPTURE_SPANS_ONLY; max 65536 bytes/depth64; comments·multiple roots·malformed/recovery·full CST/API·compiled metadata·fact는 미지원이며 후속 예외 처분 필요"
				row.Exceptions.State = "PARTIAL_MAPPING"
				row.Exceptions.Items = []string{"RFC JSON 밖의 comment·multiple-root·malformed/recovery source version은 별도 규칙이 필요하다.", "전체 JSON version의 자동화/예외 분류 및 독립 검토는 아직 수행하지 않았다."}
			}
			out.Rules = append(out.Rules, row)
		}
	}
	return out, nil
}
