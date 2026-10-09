package kit

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// TSQLDiagnostic identifies a proven violation of one of CheckTSQLContext's
// bounded structural rules. It contains no source text or object names.
type TSQLDiagnostic struct {
	Code  string `json:"code"`
	Node  int    `json:"node"`
	Range Span   `json:"range"`
}

// CheckTSQLContext checks a complete, error-free T-SQL CST against its original
// source snapshot. It checks SELECT CTEs without a table source, scalar predicates,
// table hint names/conflicts, COUNT/COUNT_BIG arity and empty scalar-function RETURN.
// It also checks DML target hints, column permissions, local window frames and
// fixed CREATE/ALTER trigger name, event and execution-context restrictions.
// EXEC OUT/OUTPUT values must be local variables, including pass-through calls.
// An empty result means these rules found no violation, not that SQL Server will
// compile or execute the source. Catalog, type, collation and runtime checks are
// outside this API. The caller binds the CST to the source and grammar identity.
// Input is not modified or retained; returned diagnostics are in preorder.
func CheckTSQLContext(nodes []TreeNode, source []byte, encoding string) ([]TSQLDiagnostic, error) {
	if !SourceEncodingValid(encoding, source) {
		return nil, fail(KindInvalidInput, "SOURCE_ENCODING_INVALID", "", nil)
	}
	if err := ValidateTree(nodes, uint64(len(source))); err != nil {
		return nil, err
	}
	children := make([][]int, len(nodes))
	for i, n := range nodes {
		if n.IsError || n.IsMissing || n.HasError {
			return nil, fail(KindInvalidInput, "TSQL_TREE_HAS_ERROR", "", nil)
		}
		if (encoding == EncodingUTF16LE || encoding == EncodingUTF16BE) && (n.StartByte%2 != 0 || n.EndByte%2 != 0) {
			return nil, fail(KindInvalidInput, "TREE_ENCODING_BOUNDARY", "", nil)
		}
		if n.Parent >= 0 {
			children[n.Parent] = append(children[n.Parent], i)
		}
	}
	x := tsqlContext{nodes: nodes, children: children, source: srcText{source, encoding}}
	boolean := make([]bool, len(nodes))
	tableSource := make([]bool, len(nodes))
	simpleCase := make([]bool, len(nodes))
	for i := len(nodes) - 1; i >= 0; i-- {
		boolean[i] = x.boolean(i, boolean)
		tableSource[i] = nodes[i].Type == "relation" || nodes[i].Type == "from"
		for _, c := range children[i] {
			if nodes[i].Type == "case" && x.field(c) == "input" {
				simpleCase[i] = true
			}
			if nodes[c].Type != "cte" {
				tableSource[i] = tableSource[i] || tableSource[c]
			}
		}
	}
	out := []TSQLDiagnostic{}
	add := func(code string, i int) {
		n := nodes[i]
		out = append(out, TSQLDiagnostic{code, i, Span{n.StartByte, n.EndByte, n.StartPoint, n.EndPoint}})
	}
	scalarFunction := make([]bool, len(nodes))
	for i, n := range nodes {
		if n.Parent >= 0 {
			scalarFunction[i] = scalarFunction[n.Parent]
		}
		switch n.Type {
		case "exec_argument", "execute_statement":
			previous := -1
			for _, c := range children[i] {
				if nodes[c].Extra {
					continue
				}
				if (nodes[c].Type == "keyword_out" || nodes[c].Type == "keyword_output") && previous >= 0 && !x.localVariable(previous) {
					add("TSQL_EXEC_OUTPUT_VARIABLE", previous)
				}
				previous = c
			}
		case "create_function", "alter_function":
			scalarFunction[i] = true
			for _, c := range children[i] {
				if nodes[c].Type == "keyword_table" || nodes[c].Type == "table_var_declaration" {
					scalarFunction[i] = false
				}
			}
		case "create_procedure", "alter_procedure", "create_trigger", "alter_trigger":
			scalarFunction[i] = false
			if n.Type == "create_trigger" || n.Type == "alter_trigger" {
				x.trigger(i, add)
			}
		case "statement":
			cte, selectBody := false, false
			for _, c := range children[i] {
				cte = cte || nodes[c].Type == "cte"
				selectBody = selectBody || nodes[c].Type == "query_specification" || nodes[c].Type == "set_operation"
			}
			if cte && selectBody && !tableSource[i] {
				add("TSQL_CTE_SELECT_WITHOUT_TABLE_SOURCE", i)
			}
		case "table_hint", "query_hint":
			x.hints(i, add)
		case "grant_statement":
			x.columnPermissions(i, add)
		case "window_specification":
			frame, order, base := -1, false, false
			for _, c := range children[i] {
				if nodes[c].Type == "window_frame" {
					frame = c
				}
				order = order || nodes[c].Type == "order_by"
				base = base || x.field(c) == "base_window"
			}
			if frame >= 0 && !order && !base {
				add("TSQL_WINDOW_FRAME_ORDER", frame)
			}
		case "window_frame":
			x.windowFrame(i, add)
		case "invocation":
			name := x.functionName(i)
			if name == "COUNT" || name == "COUNT_BIG" {
				parameters, wildcard := 0, false
				for _, c := range children[i] {
					if x.field(c) == "parameter" {
						parameters++
					}
					wildcard = wildcard || nodes[c].Type == "*"
				}
				if parameters != 1 && !(parameters == 0 && wildcard) {
					add("TSQL_COUNT_ARITY", i)
				}
			}
			if name == "IIF" {
				for _, c := range children[i] {
					if x.field(c) == "parameter" {
						if !boolean[c] {
							add("TSQL_SCALAR_PREDICATE", c)
						}
						break
					}
				}
			}
		case "return_statement":
			if scalarFunction[i] {
				value := false
				for _, c := range children[i] {
					value = value || nodes[c].Named && !nodes[c].Extra && nodes[c].Type != "keyword_return"
				}
				if !value {
					add("TSQL_SCALAR_FUNCTION_RETURN_VALUE", i)
				}
			}
		}
		for _, c := range children[i] {
			check := x.field(c) == "predicate"
			if x.field(c) == "condition" {
				check = n.Type == "if_statement" || n.Type == "while_statement"
				if n.Type == "when_clause" && n.Parent >= 0 {
					check = !simpleCase[n.Parent]
				}
			}
			if n.Type == "having" && nodes[c].Named && !nodes[c].Extra && nodes[c].Type != "keyword_having" {
				check = true
			}
			if check && !boolean[c] {
				add("TSQL_SCALAR_PREDICATE", c)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b TSQLDiagnostic) int { return cmp.Compare(a.Node, b.Node) })
	return out, nil
}

type tsqlContext struct {
	nodes    []TreeNode
	children [][]int
	source   srcText
}

func (x tsqlContext) field(i int) string {
	if x.nodes[i].Field == nil {
		return ""
	}
	return *x.nodes[i].Field
}

func (x tsqlContext) token(i int) string {
	n := x.nodes[i]
	// Recognized rule tokens are short. Do not copy arbitrarily large source spans.
	if n.EndByte-n.StartByte > 512 {
		return ""
	}
	return strings.ToUpper(x.source.span(n.StartByte, n.EndByte))
}

func (x tsqlContext) functionName(i int) string {
	for _, c := range x.children[i] {
		if x.nodes[c].Type != "object_reference" {
			continue
		}
		name := ""
		for _, p := range x.children[c] {
			if x.nodes[p].Extra {
				continue
			}
			if x.field(p) != "name" || name != "" {
				return "" // qualified UDFs are not built-ins
			}
			name = x.token(p)
		}
		return name
	}
	return ""
}

func (x tsqlContext) localVariable(i int) bool {
	if x.nodes[i].Type != "field" {
		return false
	}
	identifier := -1
	for _, c := range x.children[i] {
		if x.nodes[c].Extra {
			continue
		}
		if x.nodes[c].Type != "identifier" || identifier >= 0 {
			return false
		}
		identifier = c
	}
	if identifier < 0 {
		return false
	}
	n := x.nodes[identifier]
	// Four bytes cover the @/@@ prefix in every supported encoding, without
	// copying arbitrarily long variable names or applying catalog/name limits.
	prefix := x.source.span(n.StartByte, n.StartByte+min(4, n.EndByte-n.StartByte))
	return strings.HasPrefix(prefix, "@") && !strings.HasPrefix(prefix, "@@")
}

func (x tsqlContext) boolean(i int, values []bool) bool {
	switch x.nodes[i].Type {
	case "exists", "graph_match_predicate", "between_expression":
		return true
	case "invocation":
		name := x.functionName(i)
		return name == "UPDATE" || name == "CONTAINS" || name == "FREETEXT" || name == "REGEXP_LIKE"
	case "term", "parenthesized_expression":
		for _, c := range x.children[i] {
			if x.nodes[c].Named && !x.nodes[c].Extra {
				return values[c]
			}
		}
	case "binary_expression":
		left, right, op := -1, -1, ""
		for _, c := range x.children[i] {
			switch x.field(c) {
			case "left":
				left = c
			case "right":
				right = c
			case "operator":
				op = x.nodes[c].Type
			}
		}
		if op == "keyword_and" || op == "keyword_or" {
			return left >= 0 && right >= 0 && values[left] && values[right]
		}
		switch op {
		case "=", "<", "<=", ">", ">=", "<>", "!=", "!<", "!>",
			"keyword_is", "is_not", "keyword_like", "not_like", "keyword_in", "not_in", "distinct_from", "not_distinct_from":
			return true
		}
	case "unary_expression":
		operand, not := -1, false
		for _, c := range x.children[i] {
			if x.field(c) == "operand" {
				operand = c
			}
			not = not || x.field(c) == "operator" && x.nodes[c].Type == "keyword_not"
		}
		return not && operand >= 0 && values[operand]
	}
	return false
}

func (x tsqlContext) hints(i int, add func(string, int)) {
	groups := [4]string{}
	conflict := false
	parameterizedSeek, indexHint := false, false
	for _, c := range x.children[i] {
		if x.nodes[c].Type != "table_hint_item" {
			continue
		}
		name := ""
		for _, p := range x.children[c] {
			if !x.nodes[p].Extra {
				name = x.token(p)
				break
			}
		}
		if !strings.Contains("|INDEX|NOEXPAND|FORCESEEK|FORCESCAN|FORCE_ANN_ONLY|HOLDLOCK|NOLOCK|NOWAIT|PAGLOCK|READCOMMITTED|READCOMMITTEDLOCK|READPAST|READUNCOMMITTED|REPEATABLEREAD|ROWLOCK|SERIALIZABLE|SNAPSHOT|SPATIAL_WINDOW_MAX_CELLS|TABLOCK|TABLOCKX|UPDLOCK|XLOCK|KEEPIDENTITY|KEEPDEFAULTS|IGNORE_CONSTRAINTS|IGNORE_TRIGGERS|", "|"+name+"|") {
			add("TSQL_TABLE_HINT_NAME", c)
			continue
		}
		if name == "READUNCOMMITTED" {
			name = "NOLOCK"
		}
		if name == "SERIALIZABLE" {
			name = "HOLDLOCK"
		}
		if name == "NOLOCK" && x.nodes[i].Parent >= 0 {
			switch x.nodes[x.nodes[i].Parent].Type {
			case "delete", "update", "insert", "merge":
				add("TSQL_DML_TARGET_HINT", c)
			}
		}
		indexHint = indexHint || name == "INDEX"
		for _, p := range x.children[c] {
			parameterizedSeek = parameterizedSeek || name == "FORCESEEK" && x.nodes[p].Type == "("
			if x.nodes[p].Type == "literal" {
				value, err := strconv.ParseUint(x.token(p), 10, 31)
				if err != nil || name == "FORCESEEK" && value == 0 {
					add("TSQL_TABLE_HINT_ARGUMENT", c)
				}
			}
		}
		membership := [4]bool{
			name == "PAGLOCK" || name == "NOLOCK" || name == "ROWLOCK" || name == "TABLOCK" || name == "TABLOCKX",
			name == "HOLDLOCK" || name == "NOLOCK" || name == "READCOMMITTED" || name == "READCOMMITTEDLOCK" || name == "REPEATABLEREAD" || name == "SNAPSHOT",
			name == "NOLOCK" || name == "UPDLOCK" || name == "XLOCK",
			name == "FORCESEEK" || name == "FORCESCAN",
		}
		for g, present := range membership {
			if present {
				conflict = conflict || groups[g] != "" && groups[g] != name
				groups[g] = name
			}
		}
	}
	if conflict || parameterizedSeek && indexHint {
		add("TSQL_TABLE_HINT_CONFLICT", i)
	}
}

func (x tsqlContext) columnPermissions(i int, add func(string, int)) {
	global := false
	for _, c := range x.children[i] {
		global = global || x.nodes[c].Type == "("
	}
	for _, c := range x.children[i] {
		if x.nodes[c].Type != "permission" {
			continue
		}
		columns, words := global, []string{}
		for _, p := range x.children[c] {
			if x.nodes[p].Type == "list" {
				columns = true
			} else if !x.nodes[p].Extra {
				words = append(words, x.token(p))
			}
		}
		if columns {
			switch strings.Join(words, " ") {
			case "SELECT", "UPDATE", "REFERENCES", "UNMASK":
			default:
				add("TSQL_COLUMN_PERMISSION", c)
			}
		}
	}
}

func (x tsqlContext) windowFrame(i int, add func(string, int)) {
	rangeFrame, invalid := false, false
	definitions := []int{}
	for _, c := range x.children[i] {
		rangeFrame = rangeFrame || x.nodes[c].Type == "keyword_range"
		if x.nodes[c].Type == "frame_definition" {
			definitions = append(definitions, c)
		}
	}
	for k, d := range definitions {
		unbounded, preceding, following := false, false, false
		for _, c := range x.children[d] {
			unbounded = unbounded || x.nodes[c].Type == "keyword_unbounded"
			preceding = preceding || x.nodes[c].Type == "keyword_preceding"
			following = following || x.nodes[c].Type == "keyword_following"
			invalid = invalid || rangeFrame && (x.field(c) == "start" || x.field(c) == "end")
		}
		invalid = invalid || len(definitions) == 1 && following ||
			unbounded && (k == 0 && following || k == 1 && preceding)
	}
	if invalid {
		add("TSQL_WINDOW_FRAME_BOUND", i)
	}
}

func (x tsqlContext) trigger(i int, add func(string, int)) {
	ddl, database, name := false, false, -1
	for _, c := range x.children[i] {
		ddl = ddl || x.nodes[c].Type == "keyword_database" || x.nodes[c].Type == "keyword_server"
		database = database || x.nodes[c].Type == "keyword_database"
		if name < 0 && x.nodes[c].Type == "object_reference" {
			name = c
		}
	}
	if ddl && name >= 0 {
		for _, c := range x.children[name] {
			if !x.nodes[c].Extra && x.field(c) != "name" {
				add("TSQL_DDL_TRIGGER_NAME", name)
				break
			}
		}
	}
	for _, c := range x.children[i] {
		if x.nodes[c].Type == "trigger_event" {
			event := x.token(c)
			dml := event == "INSERT" || event == "UPDATE" || event == "DELETE"
			if ddl == dml || database && (event == "DDL_SERVER_LEVEL_EVENTS" || event == "CREATE_LOGIN") {
				add("TSQL_TRIGGER_EVENT_SCOPE", c)
			}
		}
		if ddl && x.nodes[c].Type == "trigger_options" {
			for _, option := range x.children[c] {
				for _, clause := range x.children[option] {
					if x.nodes[clause].Type == "execute_as_clause" {
						for _, p := range x.children[clause] {
							if x.nodes[p].Type == "keyword_owner" {
								add("TSQL_DDL_TRIGGER_OWNER", clause)
							}
						}
					}
				}
			}
		}
	}
}
