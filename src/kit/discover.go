package kit

import (
	"bytes"
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"strings"
)

var (
	jsRequire = regexp.MustCompile(`\brequire\s*\(\s*['"]([^'"\n]+)['"]\s*\)`)
	jsImport  = regexp.MustCompile(`\bimport\s*(?:\(\s*|[^'";]*?\bfrom\s*)['"]([^'"\n]+)['"]`)
	jsToken   = regexp.MustCompile(`\b(?:require|import)\b`)
	cInclude  = regexp.MustCompile(`(?m)^[ \t]*#[ \t]*include[ \t]*"([^"\n]+)"`)
)

var sourceExt = map[string]bool{".c": true, ".cc": true, ".cpp": true, ".h": true, ".hpp": true, ".inc": true}

// discover fills inv from known paths (known-paths-r1) or from an explicit file list.
// It never follows links, evaluates JS, runs processes or searches parent directories.
func discover(r *run, g *guard, inv *inventory, sel Selection, limits Limits) *Error {
	gi, e := g.lstat(sel.Grammar)
	if e != nil {
		return e
	}
	if gi == nil {
		return fail(KindInvalidInput, "GRAMMAR_NOT_FOUND", sel.Grammar, nil)
	}
	switch kind(gi) {
	case "dir":
	case "file":
		return fail(KindInvalidInput, "GRAMMAR_NOT_DIRECTORY", sel.Grammar, nil)
	default:
		return fail(KindInvalidInput, "LINK_OR_SPECIAL_REJECTED", sel.Grammar, nil)
	}
	d := &discovery{r: r, g: g, inv: inv, limits: limits}
	if sel.Files != nil {
		inv.closure = ClosureCallerSelected
		for _, f := range sel.Files {
			if !portable(f.Path, false) || !roles[f.Role] {
				return fail(KindInvalidInput, "SELECTION_INVALID", f.Path, nil)
			}
			if inv.entries[f.Path] != nil {
				return fail(KindInvalidInput, "SELECTION_DUPLICATE", f.Path, nil)
			}
			if e := d.add(f.Path, f.Role, "SELECTION", true); e != nil {
				return e
			}
		}
		return d.countSelected()
	}
	prefix := ""
	if sel.Grammar != "." {
		prefix = sel.Grammar + "/"
	}
	for _, m := range []string{"tree-sitter.json", "package.json"} {
		if e := d.add(m, "metadata", "KNOWN_PATH", m == "tree-sitter.json"); e != nil {
			return e
		}
		if prefix != "" {
			if e := d.add(prefix+m, "metadata", "KNOWN_PATH", false); e != nil {
				return e
			}
		}
	}
	for _, k := range []struct{ name, role string }{{"grammar.js", "grammar"}, {"src/grammar.json", "grammar"}, {"src/parser.c", "generated"}, {"src/node-types.json", "generated"}} {
		if e := d.add(prefix+k.name, k.role, "KNOWN_PATH", true); e != nil {
			return e
		}
	}
	if e := d.listSources(prefix + "src"); e != nil {
		return e
	}
	if e := d.walk(prefix+"queries", "query", ".scm", true); e != nil {
		return e
	}
	if e := d.walk(prefix+"test/corpus", "corpus", "", true); e != nil {
		return e
	}
	if e := d.walk(prefix+"corpus", "corpus", "", false); e != nil {
		return e
	}
	for _, m := range []string{"tree-sitter.json", "package.json", prefix + "tree-sitter.json", prefix + "package.json"} {
		if e := d.metadata(m); e != nil {
			return e
		}
	}
	if e := d.closure(); e != nil {
		return e
	}
	return d.countSelected()
}

type discovery struct {
	r          *run
	g          *guard
	inv        *inventory
	limits     Limits
	unresolved bool
	parsedMeta map[string]bool
}

// add observes one path. Missing paths are recorded only when report is true.
func (d *discovery) add(p, role, basis string, report bool) *Error {
	if d.inv.entries[p] != nil {
		return nil
	}
	if depthOf(p) > d.limits.Depth {
		return fail(KindResourceLimit, "DEPTH_LIMIT", p, nil)
	}
	info, e := d.g.lstat(p)
	if e != nil {
		if e.Code != "LINK_OR_SPECIAL_REJECTED" {
			return e
		}
		d.inv.entries[p] = &entry{InventoryEntry: InventoryEntry{Path: p, Role: role, State: StateUnsupported, Basis: basis}}
		d.inv.finding("LINK_NOT_FOLLOWED", "error", e.Path, "link·reparse point·special 경로는 따라가거나 읽지 않는다")
		return nil
	}
	item := &entry{InventoryEntry: InventoryEntry{Path: p, Role: role, Basis: basis}, info: info}
	switch {
	case info == nil:
		if !report {
			return nil
		}
		item.State = StateNotFound
	case kind(info) == "file":
		item.State = StateFound
		size := uint64(info.Size())
		item.Size = &size
	default:
		item.State = StateUnsupported
		d.inv.finding("LINK_NOT_FOLLOWED", "error", p, "선택 위치가 일반 파일이 아니다(link·reparse point·special·directory)")
	}
	d.inv.entries[p] = item
	return nil
}

func (d *discovery) countSelected() *Error {
	n := uint64(0)
	for _, e := range d.inv.entries {
		if e.State == StateFound {
			n++
		}
	}
	if n > d.limits.Files {
		return fail(KindResourceLimit, "FILE_COUNT_LIMIT", "", nil)
	}
	return nil
}

// listSources classifies the grammar src directory without recursing beyond tree_sitter/.
func (d *discovery) listSources(dir string) *Error {
	ok, e := d.probeDir(dir, "scanner", false)
	if !ok || e != nil {
		return e
	}
	entries, e := d.g.readDir(d.r, dir, &d.inv.budget, d.limits.Files)
	if e != nil {
		return e
	}
	for _, ent := range entries {
		name := ent.Name()
		p := dir + "/" + name
		switch {
		case name == "tree_sitter":
			if e := d.walk(p, "generated", ".h", false); e != nil {
				return e
			}
		case name == "grammar.json" || name == "parser.c" || name == "node-types.json":
		case sourceExt[path.Ext(name)]:
			if e := d.addListed(p, "scanner"); e != nil {
				return e
			}
		}
	}
	return nil
}

// probeDir reports whether a known directory is a plain directory to search. A missing
// directory is NOT_FOUND when reported; a link, reparse point or special entry (or one
// below a link) becomes an UNSUPPORTED entry and is not searched.
func (d *discovery) probeDir(p, role string, report bool) (bool, *Error) {
	info, e := d.g.lstat(p)
	if e != nil {
		if e.Code == "LINK_OR_SPECIAL_REJECTED" {
			return false, d.add(p, role, "KNOWN_PATH", true)
		}
		return false, e
	}
	switch {
	case info == nil:
		if report {
			d.inv.entries[p] = &entry{InventoryEntry: InventoryEntry{Path: p, Role: role, State: StateNotFound, Basis: "KNOWN_PATH"}}
		}
		return false, nil
	case kind(info) == "dir":
		return true, nil
	case kind(info) == "file":
		return false, nil
	}
	return false, d.add(p, role, "KNOWN_PATH", true)
}

func (d *discovery) addListed(p, role string) *Error {
	if !portable(p, false) {
		d.inv.entries[p] = &entry{InventoryEntry: InventoryEntry{Path: p, Role: role, State: StateUnsupported, Basis: "LISTING"}}
		d.inv.finding("NONPORTABLE_PATH", "error", p, "portable path 규칙을 벗어난 이름은 선택하지 않는다")
		return nil
	}
	return d.add(p, role, "LISTING", true)
}

// walk lists a known directory iteratively; ext "" selects every regular file.
func (d *discovery) walk(dir, role, ext string, report bool) *Error {
	ok, e := d.probeDir(dir, role, report)
	if !ok || e != nil {
		return e
	}
	stack := []string{dir}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if depthOf(cur)+1 > d.limits.Depth {
			return fail(KindResourceLimit, "DEPTH_LIMIT", cur, nil)
		}
		entries, e := d.g.readDir(d.r, cur, &d.inv.budget, d.limits.Files)
		if e != nil {
			return e
		}
		for i := len(entries) - 1; i >= 0; i-- {
			ent := entries[i]
			p := cur + "/" + ent.Name()
			if !portable(p, false) {
				if e := d.addListed(p, role); e != nil {
					return e
				}
				continue
			}
			info, e := d.g.lstat(p)
			if e != nil {
				return e
			}
			if info == nil {
				continue // removed while listing; not an observed file
			}
			switch kind(info) {
			case "dir":
				stack = append(stack, p)
			case "file":
				if ext == "" || path.Ext(p) == ext {
					if e := d.add(p, role, "LISTING", true); e != nil {
						return e
					}
				}
			default:
				if e := d.add(p, role, "LISTING", true); e != nil {
					return e
				}
			}
		}
	}
	return nil
}

// metadata records grammar candidates named by tree-sitter.json or legacy package.json.
func (d *discovery) metadata(p string) *Error {
	item := d.inv.entries[p]
	if item == nil || item.State != StateFound {
		return nil
	}
	if d.parsedMeta == nil {
		d.parsedMeta = map[string]bool{}
	}
	if d.parsedMeta[p] {
		return nil
	}
	d.parsedMeta[p] = true
	st, e := d.g.readFile(d.r, p, item.info, d.limits.FileBytes, d.limits.TotalBytes, d.limits.FileBytes)
	if e != nil {
		return e
	}
	base := path.Dir(p)
	type cand struct {
		Name       string          `json:"name"`
		Path       json.RawMessage `json:"path"`
		Highlights json.RawMessage `json:"highlights"`
		Injections json.RawMessage `json:"injections"`
		Locals     json.RawMessage `json:"locals"`
		Tags       json.RawMessage `json:"tags"`
	}
	var doc struct {
		Grammars   []cand          `json:"grammars"`
		TreeSitter json.RawMessage `json:"tree-sitter"`
	}
	if err := json.Unmarshal(bytes.TrimPrefix(st.content, []byte("\xEF\xBB\xBF")), &doc); err != nil {
		d.inv.finding("METADATA_UNPARSED", "warning", p, "metadata JSON을 해석하지 못해 관측에서 제외했다")
		return nil
	}
	source := path.Base(p)
	list := doc.Grammars
	if source == "package.json" && len(doc.TreeSitter) > 0 {
		var legacy []cand
		if json.Unmarshal(doc.TreeSitter, &legacy) != nil {
			d.inv.finding("METADATA_UNPARSED", "warning", p, "package.json의 tree-sitter 항목을 해석하지 못했다")
		}
		list = append(list, legacy...)
	}
	for _, c := range list {
		rel := "."
		if len(c.Path) > 0 && json.Unmarshal(c.Path, &rel) != nil {
			d.inv.finding("METADATA_PATH_INVALID", "warning", p, "grammar path가 문자열이 아니다")
			continue
		}
		joined := path.Clean(path.Join(base, rel))
		if path.IsAbs(rel) || strings.ContainsAny(rel, `\:`) || !portable(joined, true) {
			d.inv.finding("METADATA_PATH_INVALID", "warning", p, "metadata grammar path가 root 밖이거나 portable하지 않다")
			continue
		}
		d.inv.grammars = append(d.inv.grammars, GrammarCandidate{Path: joined, Name: c.Name, Source: source})
		if joined != d.inv.grammar {
			continue
		}
		// Query files declared for the selected grammar are part of its observed inputs.
		for _, raw := range []json.RawMessage{c.Highlights, c.Injections, c.Locals, c.Tags} {
			var many []string
			var one string
			if len(raw) == 0 {
				continue
			}
			if json.Unmarshal(raw, &one) == nil {
				many = []string{one}
			} else if json.Unmarshal(raw, &many) != nil {
				d.inv.finding("METADATA_PATH_INVALID", "warning", p, "query 경로가 문자열이나 문자열 배열이 아니다")
				continue
			}
			for _, q := range many {
				qp := path.Clean(path.Join(base, q))
				if path.IsAbs(q) || strings.ContainsAny(q, `\:`) || !portable(qp, false) {
					d.inv.finding("METADATA_PATH_INVALID", "warning", p, "metadata query 경로가 root 밖이거나 portable하지 않다")
					continue
				}
				if e := d.add(qp, "query", "METADATA", true); e != nil {
					return e
				}
			}
		}
	}
	return nil
}

// closure scans grammar JS and scanner C sources for static references without
// evaluation. OBSERVED means every observed reference resolved inside the root; it is
// not a completeness proof. Unknown layouts are NOT_APPLICABLE observations.
func (d *discovery) closure() *Error {
	inv := d.inv
	queue := []string{}
	for p, e := range inv.entries {
		if e.State == StateFound && (e.Role == "grammar" && strings.HasSuffix(p, ".js") || e.Role == "scanner") {
			queue = append(queue, p)
		}
	}
	slices.Sort(queue)
	scanned := map[string]bool{}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if scanned[p] {
			continue
		}
		scanned[p] = true
		item := inv.entries[p]
		st, e := d.g.readFile(d.r, p, item.info, d.limits.FileBytes, d.limits.TotalBytes, d.limits.FileBytes)
		if e != nil {
			return e
		}
		js := strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs") || strings.HasSuffix(p, ".cjs")
		var refs []string
		if js {
			inv.jsSeen = true
			matched := 0
			for _, re := range []*regexp.Regexp{jsRequire, jsImport} {
				for _, m := range re.FindAllSubmatch(st.content, -1) {
					refs = append(refs, string(m[1]))
					matched++
				}
			}
			if len(jsToken.FindAll(st.content, -1)) > matched {
				d.unresolve("UNPARSED_REFERENCE_TOKEN", p, "literal이 아닌 require/import 또는 해석하지 못한 참조 token이 있다")
			}
		} else {
			for _, m := range cInclude.FindAllSubmatch(st.content, -1) {
				refs = append(refs, string(m[1]))
			}
		}
		for _, ref := range refs {
			next, e := d.resolve(p, ref, js)
			if e != nil {
				return e
			}
			if next != "" {
				queue = append(queue, next)
			}
		}
	}
	hasGrammar := false
	for _, k := range []string{"grammar.js", "src/grammar.json"} {
		p := k
		if inv.grammar != "." {
			p = inv.grammar + "/" + k
		}
		if e := inv.entries[p]; e != nil && e.State == StateFound {
			hasGrammar = true
		}
	}
	switch {
	case !hasGrammar:
		inv.closure = ClosureNotApplicable
		inv.finding("UNKNOWN_LAYOUT", "warning", inv.grammar, "선택한 위치에서 grammar.js나 src/grammar.json을 찾지 못했다; qualification이 아닌 관측이다")
		reported := map[string]bool{}
		for _, c := range inv.grammars {
			if c.Path != inv.grammar && !reported[c.Path] {
				reported[c.Path] = true
				inv.finding("GRAMMAR_CANDIDATE", "info", c.Path, "metadata가 이 grammar 위치를 가리킨다; 명시 선택이 필요하다")
			}
		}
	case d.unresolved:
		inv.closure = ClosureUnresolved
	default:
		inv.closure = ClosureObserved
	}
	if meta := inv.entries["tree-sitter.json"]; hasGrammar && meta != nil && meta.State == StateNotFound {
		inv.finding("METADATA_ABSENT", "info", "tree-sitter.json", "tree-sitter.json이 없다; legacy layout으로 관측하며 invalid로 판정하지 않는다")
	}
	return nil
}

func (d *discovery) unresolve(code, p, msg string) {
	d.unresolved = true
	d.inv.finding(code, "warning", p, msg)
}

// resolve maps one literal reference to a file inside the root, or records why not.
func (d *discovery) resolve(from, ref string, js bool) (string, *Error) {
	if js && !strings.HasPrefix(ref, "./") && !strings.HasPrefix(ref, "../") {
		d.unresolve("EXTERNAL_MODULE_REFERENCE", from, "root 밖 module 참조 "+ref+"는 실행 없이 확인하지 않는다")
		return "", nil
	}
	base := path.Join(path.Dir(from), ref)
	if base == ".." || strings.HasPrefix(base, "../") || strings.HasPrefix(ref, "/") {
		d.unresolve("REFERENCE_OUTSIDE_ROOT", from, "참조 "+ref+"가 root 밖을 가리킨다")
		return "", nil
	}
	candidates := []string{base}
	if js {
		candidates = append(candidates, base+".js", base+".json", base+"/index.js")
	} else if src := path.Join(d.inv.grammar, "src"); path.Dir(from) != src {
		// tree-sitter builds compile with -I <grammar>/src; quoted includes also resolve there.
		candidates = append(candidates, path.Join(src, ref))
	}
	for _, c := range candidates {
		if !portable(c, false) {
			continue
		}
		if e := d.inv.entries[c]; e != nil && e.State == StateFound {
			return "", nil
		}
		info, e := d.g.lstat(c)
		if e != nil && e.Code != "LINK_OR_SPECIAL_REJECTED" {
			return "", e
		}
		if info == nil && e == nil {
			continue
		}
		role, basis := "scanner", "INCLUDE"
		if js {
			role, basis = "grammar", "REQUIRE"
		} else if strings.HasPrefix(c, "tree_sitter/") || strings.Contains(c, "/tree_sitter/") {
			role = "generated"
		}
		if info != nil && kind(info) == "dir" {
			continue
		}
		if e := d.add(c, role, basis, true); e != nil {
			return "", e
		}
		if d.inv.entries[c].State != StateFound {
			d.unresolved = true
			return "", nil
		}
		if strings.HasSuffix(c, ".json") {
			return "", nil
		}
		return c, nil
	}
	code := "REFERENCE_NOT_FOUND"
	if !js {
		code = "INCLUDE_NOT_FOUND"
	}
	d.unresolve(code, from, "참조 "+ref+"를 root 안에서 찾지 못했다")
	return "", nil
}
