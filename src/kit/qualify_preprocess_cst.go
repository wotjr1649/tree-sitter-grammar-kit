package kit

import (
	"fmt"
	"reflect"
	"slices"
)

// QualPreprocessCSTCase is one source-bound comparison, separate from syntax/reuse checks.
type QualPreprocessCSTCase struct {
	Set    string              `json:"set"`
	Case   string              `json:"case"`
	Result string              `json:"result"`
	Code   string              `json:"code"`
	Steps  []PreprocessCSTStep `json:"steps"`
}

// QualPreprocessCSTCounts counts cases; ComparedSteps counts only the compared prefix.
type QualPreprocessCSTCounts struct {
	Pass          int `json:"pass"`
	Mismatch      int `json:"mismatch"`
	Unverified    int `json:"unverified"`
	Fail          int `json:"fail"`
	NotApplicable int `json:"not_applicable"`
}

// QualPreprocessCSTAxis never counts a safe mismatch stop as resolution PASS.
type QualPreprocessCSTAxis struct {
	Result        string                  `json:"result"`
	Counts        QualPreprocessCSTCounts `json:"counts"`
	ComparedSteps int                     `json:"compared_steps"`
	Cases         []QualPreprocessCSTCase `json:"cases"`
}

func qualCSTAxis(cases []QualPreprocessCSTCase) *QualPreprocessCSTAxis {
	a := &QualPreprocessCSTAxis{Result: AssessBlocked, Cases: cases}
	for _, c := range cases {
		switch c.Result {
		case "PASS":
			a.Counts.Pass++
		case "MISMATCH":
			a.Counts.Mismatch++
		case "UNVERIFIED":
			a.Counts.Unverified++
		case "NOT_APPLICABLE":
			a.Counts.NotApplicable++
		default:
			a.Counts.Fail++
		}
		a.ComparedSteps += len(c.Steps)
	}
	if a.Counts.Fail > 0 {
		a.Result = AssessFail
	} else if a.Counts.Pass > 0 && a.Counts.Mismatch == 0 && a.Counts.Unverified == 0 {
		a.Result = AssessPass
	}
	return a
}

func positionDifference(a, b []uint32) []uint32 {
	out := []uint32{}
	j := 0
	for _, v := range a {
		for j < len(b) && b[j] < v {
			j++
		}
		if j == len(b) || b[j] != v {
			out = append(out, v)
		}
	}
	return out
}

func validRegisteredCST(c QualCase) bool {
	pe := c.PreprocessExpect
	if len(pe) == 0 {
		return false
	}
	if pe[len(pe)-1].Status != PreprocessOK {
		return len(c.PreprocessCSTExpect) == 0
	}
	steps := c.PreprocessCSTExpect
	if len(steps) == 0 || len(steps) > len(c.Edits)+1 {
		return false
	}
	length := int64(c.Input.Bytes)
	for i, s := range steps {
		if i > 0 {
			length += int64(c.Edits[i-1].NewEndByte) - int64(c.Edits[i-1].OldEndByte)
		}
		if length < 0 || uint64(length) > NativeOperations()["native-parse-edit"].InputBytes || s.Step != i || s.SourceBytes != uint64(length) || !hex64.MatchString(s.SourceSHA256) || i == 0 && s.SourceSHA256 != c.Input.SHA256 {
			return false
		}
		for _, positions := range [][]uint32{s.HostHashes, s.CSTHashes, s.HostOnly, s.CSTOnly} {
			if positions == nil {
				return false
			}
			for j, p := range positions {
				if uint64(p) >= s.SourceBytes || j > 0 && positions[j-1] >= p {
					return false
				}
			}
		}
		host := []uint32{}
		for _, d := range pe[i].Directives {
			if d.Hash < 0 {
				return false
			}
			host = append(host, uint32(d.Hash))
		}
		if !slices.Equal(host, s.HostHashes) || !slices.Equal(positionDifference(s.HostHashes, s.CSTHashes), s.HostOnly) || !slices.Equal(positionDifference(s.CSTHashes, s.HostHashes), s.CSTOnly) {
			return false
		}
		mismatch := len(s.HostOnly)+len(s.CSTOnly) > 0
		if mismatch {
			if i != len(steps)-1 || s.Result != "MISMATCH" || s.Code != fmt.Sprintf("%s%d", PreprocessCSTPrefix, i) {
				return false
			}
		} else if s.Result != "MATCH" || s.Code != "" {
			return false
		}
	}
	return steps[len(steps)-1].Result == "MISMATCH" || len(steps) == len(c.Edits)+1
}

func registeredCSTCase(w QualWorkload, c QualCase, result PreprocessCSTResult) QualPreprocessCSTCase {
	out := QualPreprocessCSTCase{Set: w.Set, Case: c.ID, Result: result.Result, Code: result.Code, Steps: result.Steps}
	if result.Result == "UNVERIFIED" || result.Result == "FAIL" {
		return out
	}
	wantResult, wantCode := "NOT_APPLICABLE", ""
	if len(c.PreprocessCSTExpect) == 0 {
		wantCode = c.ExpectCode
	}
	if len(c.PreprocessCSTExpect) > 0 {
		last := c.PreprocessCSTExpect[len(c.PreprocessCSTExpect)-1]
		wantResult = "PASS"
		if last.Result == "MISMATCH" {
			wantResult, wantCode = "MISMATCH", last.Code
		}
	}
	want := c.PreprocessCSTExpect
	if want == nil {
		want = []PreprocessCSTStep{}
	}
	if result.Result != wantResult || result.Code != wantCode || !reflect.DeepEqual(result.Steps, want) {
		out.Result, out.Code = "FAIL", "PREPROCESS_CST_EXPECTATION_MISMATCH"
	}
	return out
}
