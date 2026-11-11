package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func inventorySelectionBudget(t *testing.T) *eligibilityBudget {
	t.Helper()
	r, e := startRun(ctxT(t), 120*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(r.cancel)
	b := &eligibilityBudget{r: r}
	r.eligibility = b
	return b
}

func TestQualificationInventoryRouteDifferential(t *testing.T) {
	data, err := os.ReadFile("../contracts/qualification-c1.json")
	if err != nil {
		t.Fatal(err)
	}
	full, err := ParseQualificationInventory(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Routes) != 26 {
		t.Fatalf("routes=%d", len(full.Routes))
	}
	header := full
	header.Routes, header.ExtraRoles = nil, nil
	for i, route := range full.Routes {
		t.Run(route.Route, func(t *testing.T) {
			b := inventorySelectionBudget(t)
			got, e := parseQualificationInventoryRoute(data, full.SHA256, route.Route, b)
			if e != nil {
				t.Fatal(e)
			}
			if got.RouteIndex != i || !reflect.DeepEqual(got.Header, header) || !reflect.DeepEqual(got.Route, route) {
				t.Fatal("selection differs from full inventory")
			}
			t.Logf("logical=%d work=%d", b.logical, b.work)
		})
	}
}

func inventoryErrorClass(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("invalid inventory accepted")
	}
	var e *Error
	if !errors.As(err, &e) || e == nil {
		t.Fatalf("untyped error: %v", err)
	}
	return e.Kind + "/" + e.Code
}

func TestQualificationInventoryRouteAcceptedPlatformParity(t *testing.T) {
	data, err := os.ReadFile("../contracts/qualification-c1.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"platform-pair", "role-id"} {
		t.Run(mutation, func(t *testing.T) {
			var inv QualificationInventory
			if e := json.Unmarshal(data, &inv); e != nil {
				t.Fatal(e)
			}
			if mutation == "platform-pair" {
				inv.Platforms[0].GOOS = "other"
			} else {
				for i := range inv.ExtraRoles {
					if inv.ExtraRoles[i].ID == "reuse-accepted" {
						inv.ExtraRoles[i].ID = "renamed"
					}
				}
			}
			changed, e := json.Marshal(inv)
			if e != nil {
				t.Fatal(e)
			}
			_, full := ParseQualificationInventory(changed)
			_, selected := parseQualificationInventoryRoute(changed, digestHex(changed), "json", inventorySelectionBudget(t))
			if inventoryErrorClass(t, full) != inventoryErrorClass(t, selected) {
				t.Fatalf("full=%v selected=%v", full, selected)
			}
		})
	}
}

func TestQualificationInventoryRouteGlobalParity(t *testing.T) {
	for name, mutate := range map[string]func(*QualificationInventory){
		"cells":                func(inv *QualificationInventory) { inv.Cells++ },
		"platform-id":          func(inv *QualificationInventory) { inv.Platforms[1].ID = inv.Platforms[0].ID },
		"unselected-set":       func(inv *QualificationInventory) { inv.Routes[1].Workload.Set = inv.Routes[0].Workload.Set },
		"unselected-route":     func(inv *QualificationInventory) { inv.Routes[1] = inv.Routes[0] },
		"route-workload":       func(inv *QualificationInventory) { inv.Routes[1].Workload.Route = inv.Routes[0].Route },
		"role-route-collision": func(inv *QualificationInventory) { inv.ExtraRoles[0].ID = inv.Routes[1].Route },
		"role-id":              func(inv *QualificationInventory) { inv.ExtraRoles[1].ID = inv.ExtraRoles[0].ID },
		"role-platform":        func(inv *QualificationInventory) { inv.ExtraRoles[0].Platforms = []string{"unknown"} },
		"role-status":          func(inv *QualificationInventory) { inv.ExtraRoles[0].Status = "UNKNOWN" },
		"role-set": func(inv *QualificationInventory) {
			inv.ExtraRoles[0].Status = RoleExecuted
			inv.ExtraRoles[0].Workloads = []QualWorkload{inv.Routes[1].Workload}
			inv.ExtraRoles[0].Workloads[0].Set = inv.Routes[0].Workload.Set
			inv.ExtraRoles[0].Platforms = nil
			inv.ExtraRoles[0].NotApplicable = nil
			for _, p := range inv.Platforms {
				inv.ExtraRoles[0].Platforms = append(inv.ExtraRoles[0].Platforms, p.ID)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newQfx(t)
			f.inv.Platforms = append([]QualPlatform(nil), f.inv.Platforms...)
			mutate(&f.inv)
			data := f.invBytes(t)
			_, full := ParseQualificationInventory(data)
			_, selected := parseQualificationInventoryRoute(data, digestHex(data), f.inv.Routes[0].Route, inventorySelectionBudget(t))
			if selected == nil {
				t.Fatal("selection accepted invalid global metadata")
			}
			if inventoryErrorClass(t, full) != inventoryErrorClass(t, selected) {
				t.Fatalf("full=%v selected=%v", full, selected)
			}
		})
	}
}

func TestQualificationInventoryRouteTokenParity(t *testing.T) {
	f := newQfx(t)
	base := f.invBytes(t)
	needle, _ := json.Marshal(f.inv.Routes[1].Route)
	field := append([]byte(`"route":`), needle...)
	unselected, _ := json.Marshal(f.inv.Routes[1])
	caseDuplicate := bytes.Replace(unselected, []byte(`"role":"requirement"`), []byte(`"role":"requirement","role":"requirement"`), 1)
	for name, data := range map[string][]byte{
		"unselected-case-duplicate":    bytes.Replace(base, unselected, caseDuplicate, 1),
		"unselected-duplicate":         bytes.Replace(base, field, append(append(append([]byte{}, field...), ','), field...), 1),
		"unselected-escaped-duplicate": bytes.Replace(base, field, append(append(append([]byte{}, field...), []byte(`,"\u0072oute":`)...), needle...), 1),
		"unselected-syntax":            bytes.Replace(base, field, []byte(`"route":@`), 1),
		"unselected-surrogate":         bytes.Replace(base, field, []byte(`"route":"\ud800"`), 1),
		"unselected-utf8":              bytes.Replace(base, field, []byte{'"', 'r', 'o', 'u', 't', 'e', '"', ':', '"', 0xff, '"'}, 1),
		"trailing":                     append(append([]byte{}, base...), []byte(" {}")...),
		"truncated":                    base[:len(base)-1],
		"depth":                        bytes.Replace(base, field, []byte(`"route":`+strings.Repeat("[", 33)+"0"+strings.Repeat("]", 33)), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if bytes.Equal(data, base) {
				t.Fatal("mutation did not change input")
			}
			_, full := ParseQualificationInventory(data)
			_, selected := parseQualificationInventoryRoute(data, digestHex(data), f.inv.Routes[0].Route, inventorySelectionBudget(t))
			if selected == nil {
				t.Fatal("selection accepted invalid unselected tokens")
			}
			if inventoryErrorClass(t, full) != inventoryErrorClass(t, selected) {
				t.Fatalf("full=%v selected=%v", full, selected)
			}
		})
	}
	if _, e := parseQualificationInventoryRoute(base, strings.Repeat("0", 64), f.inv.Routes[0].Route, inventorySelectionBudget(t)); e == nil || e.Code != "FILE_MEMBER_MISMATCH" {
		t.Fatalf("forged full-document hash: %v", e)
	}
}

func TestQualificationInventoryRouteLocalParity(t *testing.T) {
	for name, mutate := range map[string]func(*QualRoute){
		"source-pin":   func(r *QualRoute) { r.Workload.Cases[2].Input.SHA256 = strings.Repeat("0", 64) },
		"query-id":     func(r *QualRoute) { r.Workload.Cases[2].QueryExpect[0].Query = "unknown" },
		"case-id":      func(r *QualRoute) { r.Workload.Cases[1].ID = r.Workload.Cases[0].ID },
		"coverage":     func(r *QualRoute) { r.Workload.Cases[0].Covers = map[string][]string{"fxb-B09": {"P"}} },
		"requirements": func(r *QualRoute) { r.Requirements = nil },
	} {
		t.Run(name, func(t *testing.T) {
			f := newQfx(t)
			mutate(&f.inv.Routes[1])
			data := f.invBytes(t)
			_, full := ParseQualificationInventory(data)
			_, selected := parseQualificationInventoryRoute(data, digestHex(data), f.inv.Routes[1].Route, inventorySelectionBudget(t))
			if inventoryErrorClass(t, full) != inventoryErrorClass(t, selected) {
				t.Fatalf("full=%v selected=%v", full, selected)
			}
			if _, e := parseQualificationInventoryRoute(data, digestHex(data), f.inv.Routes[0].Route, inventorySelectionBudget(t)); e != nil {
				t.Fatalf("unselected local semantics evaluated: %v", e)
			}
		})
	}
	f := newQfx(t)
	base := f.invBytes(t)
	data := bytes.Replace(base, []byte(`"source":"alpha"`), []byte(`"preprocess_cst_expect":null,"source":"alpha"`), 1)
	_, full := ParseQualificationInventory(data)
	_, selected := parseQualificationInventoryRoute(data, digestHex(data), f.inv.Routes[0].Route, inventorySelectionBudget(t))
	if inventoryErrorClass(t, full) != inventoryErrorClass(t, selected) {
		t.Fatalf("special shape full=%v selected=%v", full, selected)
	}
}

func TestQualificationInventoryRouteBoundaries(t *testing.T) {
	f := newQfx(t)
	base := f.invBytes(t)
	for _, schema := range []string{QualificationInventorySchema, qualificationInventoryR7, qualificationInventoryR6, qualificationInventoryR5} {
		f.inv.Schema = schema
		data := f.invBytes(t)
		full, err := ParseQualificationInventory(data)
		if err != nil {
			t.Fatal(err)
		}
		got, e := parseQualificationInventoryRoute(data, digestHex(data), f.inv.Routes[1].Route, inventorySelectionBudget(t))
		if e != nil || !reflect.DeepEqual(got.Route, full.Routes[1]) {
			t.Fatalf("schema=%s err=%v", schema, e)
		}
	}
	// Member order and escaped identifiers must not influence route selection.
	data := bytes.Replace(base, []byte(`"routes":`), []byte(`"\u0072outes":`), 1)
	if _, e := parseQualificationInventoryRoute(data, digestHex(data), f.inv.Routes[1].Route, inventorySelectionBudget(t)); e != nil {
		t.Fatal(e)
	}
	for _, stop := range []string{"logical", "work", "cancelled"} {
		t.Run(stop, func(t *testing.T) {
			b := inventorySelectionBudget(t)
			switch stop {
			case "logical":
				b.logical = eligibilityLogicalLimit
			case "work":
				b.work = eligibilityWorkLimit
			case "cancelled":
				ctx, cancel := context.WithCancel(b.r.caller)
				cancel()
				b.r.caller = ctx
			}
			_, e := parseQualificationInventoryRoute(base, digestHex(base), f.inv.Routes[0].Route, b)
			if e == nil || (e.Kind != KindResourceLimit && e.Kind != KindCancelled) {
				t.Fatalf("stop not propagated: %v", e)
			}
		})
	}
	if _, e := parseQualificationInventoryRoute(base, digestHex(base), "absent", inventorySelectionBudget(t)); e == nil || e.Code != "ROUTE_INVALID" {
		t.Fatalf("missing route: %v", e)
	}
}

func TestQualificationInventoryRouteValidationProductLimit(t *testing.T) {
	f := newQfx(t)
	c := &f.inv.Routes[0].Workload.Cases[0]
	c.Expect = make([]StepExpectation, 4000)
	for i := range c.Expect {
		c.Expect[i] = StepExpectation{Step: 0, Syntax: "ERROR", Contains: []string{"word"}}
	}
	c.Expect[len(c.Expect)-1].Syntax = "NO_ERROR"
	claims := make([]string, 4000)
	for i := range claims {
		claims[i] = "P"
	}
	c.Covers["fxa-B01"] = claims
	data := f.invBytes(t)
	_, e := parseQualificationInventoryRoute(data, digestHex(data), "fxa", inventorySelectionBudget(t))
	if e == nil || e.Kind != KindResourceLimit || e.Path != "inventory-validation-product" {
		t.Fatalf("coverage product not bounded before validation: %v", e)
	}
}

func TestQualificationInventoryRouteJSONLimits(t *testing.T) {
	oversized := make([]byte, MaxDocumentBytes+1)
	if _, e := parseQualificationInventoryRoute(oversized, "", "json", inventorySelectionBudget(t)); e == nil || e.Code != "DOCUMENT_BYTES_LIMIT" {
		t.Fatalf("document cap: %v", e)
	}
	data := []byte("[" + strings.Repeat("0,", 1999999) + "0]")
	if _, _, e := inventoryStrictIndex(data, inventorySelectionBudget(t)); e == nil || e.Code != "JSON_VALUE_LIMIT" {
		t.Fatalf("value cap: %v", e)
	}
}

func TestQualificationInventoryRouteSpecialParity(t *testing.T) {
	f := newQfx(t)
	base := f.invBytes(t)
	for _, ri := range []int{0, 1} {
		for _, field := range []string{`"preprocess_cst_expect":[{"unknown":1}],`, `"reuse_accepted":{"unknown":1},`} {
			raw, _ := json.Marshal(f.inv.Routes[ri])
			changed := bytes.Replace(raw, []byte(`"workload":{`), []byte(`"workload":{`+field), 1)
			data := bytes.Replace(base, raw, changed, 1)
			_, full := ParseQualificationInventory(data)
			_, selected := parseQualificationInventoryRoute(data, digestHex(data), "fxa", inventorySelectionBudget(t))
			if inventoryErrorClass(t, full) != inventoryErrorClass(t, selected) {
				t.Fatalf("route=%d field=%s full=%v selected=%v", ri, field, full, selected)
			}
		}
	}
	data, err := os.ReadFile("../contracts/qualification-c1.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{qualificationInventoryR7, qualificationInventoryR6, qualificationInventoryR5} {
		changed := bytes.Replace(data, []byte(QualificationInventorySchema), []byte(schema), 1)
		_, full := ParseQualificationInventory(changed)
		_, selected := parseQualificationInventoryRoute(changed, digestHex(changed), "go", inventorySelectionBudget(t))
		if inventoryErrorClass(t, full) != inventoryErrorClass(t, selected) {
			t.Fatalf("schema=%s full=%v selected=%v", schema, full, selected)
		}
	}
}

func TestQualificationInventoryRouteAlternativeProductLimit(t *testing.T) {
	f := newQfx(t)
	r := &f.inv.Routes[0]
	r.Requirements[0].Alternatives = nil
	c := &r.Workload.Cases[0]
	c.Alternatives = nil
	claims := make([]string, 6000)
	for i := range claims {
		claims[i] = "E"
		alt := fmt.Sprintf("fxa-B01.a%05d", i)
		r.Requirements[0].Alternatives = append(r.Requirements[0].Alternatives, alt)
		c.Alternatives = append(c.Alternatives, alt)
	}
	c.Covers["fxa-B01"] = append(claims, "P")
	c.Edits = []Edit{{StartByte: 4, OldEndByte: 5, NewEndByte: 5, Old: []byte("a"), New: []byte("z")}}
	data := f.invBytes(t)
	_, e := parseQualificationInventoryRoute(data, digestHex(data), "fxa", inventorySelectionBudget(t))
	if e == nil || e.Kind != KindResourceLimit || e.Path != "inventory-validation-product" {
		t.Fatalf("alternative product not bounded: %v", e)
	}
}

func TestQualificationInventoryRoutePlatformProductLimit(t *testing.T) {
	data, err := os.ReadFile("../contracts/qualification-c1.json")
	if err != nil {
		t.Fatal(err)
	}
	var inv QualificationInventory
	if e := json.Unmarshal(data, &inv); e != nil {
		t.Fatal(e)
	}
	var role QualRole
	for _, r := range inv.ExtraRoles {
		if r.ID == "reuse-accepted" {
			role = r
		}
	}
	role.Platforms, role.Deferred = nil, map[string]string{}
	inv.Platforms = nil
	for i := range 5000 {
		p := QualPlatform{ID: fmt.Sprintf("platform-%05d", i), GOOS: "other", GOARCH: "amd64"}
		if i < 1000 {
			p.GOOS = "windows"
			role.Platforms = append(role.Platforms, p.ID)
		} else {
			role.Deferred[p.ID] = "outside accepted host scope"
		}
		inv.Platforms = append(inv.Platforms, p)
	}
	inv.ExtraRoles = []QualRole{role}
	inv.Cells = len(inv.Platforms) * len(inv.Routes)
	changed, e := json.Marshal(inv)
	if e != nil {
		t.Fatal(e)
	}
	_, ke := parseQualificationInventoryRoute(changed, digestHex(changed), "json", inventorySelectionBudget(t))
	if ke == nil || ke.Kind != KindResourceLimit || ke.Path != "inventory-validation-product" {
		t.Fatalf("accepted platform product not bounded: %v", ke)
	}
}
