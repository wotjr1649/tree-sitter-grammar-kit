package kit

// Rule tables of the G1-3b classification of csharp-B03 (expressions, patterns) and csharp-B04 (statements). They are authored
// from the production text of feature-alternatives.json and the Roslyn node kind names of the pinned observation. They name no
// tree-sitter node type and read no expected value: the node types of a kind come from the kind map only after the classification
// (G1-3b PR2), and the classification looks at the kind map's class, map and reason only.

// g1clsRule is the rule of one alternative: either a blocked disposition authored here, or the Roslyn node kinds that exhibit the
// construct, optionally with the kind of the direct parent node that places the construct in its slot.
type g1clsRule struct {
	kinds  []string
	parent []string
	block  string
	why    string
}

func g1clsNode(kinds ...string) g1clsRule { return g1clsRule{kinds: kinds} }

func g1clsNodeIn(parent string, kinds ...string) g1clsRule {
	return g1clsRule{kinds: kinds, parent: []string{parent}}
}

func g1clsBlock(code, why string) g1clsRule { return g1clsRule{block: code, why: why} }

var (
	g1clsCategory = g1clsBlock(G1ClassBlockCategory, "a ladder or category production: the alternative names the next level or a union, which has no node of its own")
	g1clsSlot     = g1clsBlock(G1ClassBlockPosition, "a role in its parent: the node is decided by the slot it occupies and the observation records no slot")
	g1clsList     = g1clsBlock(G1ClassBlockNoNode, "a separated list: Roslyn has a list of nodes, not a node, and the grammar has no node for it")
	g1clsBinding  = g1clsBlock(G1ClassBlockSemantic, "the form is told from its neighbours only by name binding")
)

// g1clsLexemes are nonterminals whose production is one fixed lexeme (6.4.6, 12.x). An alternative that names one has the lexeme
// as its anchor, as an alternative with a quoted literal has that literal.
var g1clsLexemes = map[string]string{
	"logical_negation_operator": "!",
	"right_shift":               ">>",
	"right_shift_assignment":    ">>=",
	"discard_token":             "_",
}

var g1clsCompound = []string{
	"AddAssignmentExpression", "SubtractAssignmentExpression", "MultiplyAssignmentExpression", "DivideAssignmentExpression",
	"ModuloAssignmentExpression", "AndAssignmentExpression", "OrAssignmentExpression", "ExclusiveOrAssignmentExpression",
	"LeftShiftAssignmentExpression", "RightShiftAssignmentExpression",
}

// g1clsRules is keyed, in the order the lookup tries them, by "head#i" (the i-th alternative of the production), "head>target"
// (a pass-through alternative of head to the nonterminal target), "head" (every alternative of the production) and ">target"
// (every pass-through to target whose head has no rule). An alternative without a rule is an error of the classification, never
// a default.
var g1clsRules = map[string]g1clsRule{
	// ---- patterns ----
	"pattern":                      g1clsCategory,
	">declaration_pattern":         g1clsNode("DeclarationPattern"),
	">constant_pattern":            g1clsNode("ConstantPattern"),
	">var_pattern":                 g1clsNode("VarPattern"),
	"declaration_pattern":          g1clsNode("DeclarationPattern"),
	"constant_pattern":             g1clsNode("ConstantPattern"),
	">discard_designation":         g1clsNode("DiscardDesignation"),
	">single_variable_designation": g1clsNode("SingleVariableDesignation"),
	"single_variable_designation":  g1clsNode("SingleVariableDesignation"),
	"designation":                  g1clsCategory,

	// ---- arguments, names, tuples ----
	"argument_list":  g1clsNode("ArgumentList"),
	"argument":       g1clsNode("Argument"),
	"argument_value": g1clsSlot,
	"simple_name":    g1clsSlot,
	">simple_name":   g1clsSlot,
	"tuple_element":  g1clsBlock(G1ClassBlockPosition, "an element of a tuple expression is an Argument node; TupleElement is the element of a tuple type"),

	// ---- primary expressions ----
	">literal":                              g1clsCategory,
	">interpolated_string_expression":       g1clsNode("InterpolatedStringExpression"),
	">parenthesized_expression":             g1clsNode("ParenthesizedExpression"),
	">tuple_literal":                        g1clsNode("TupleExpression"),
	">member_access":                        g1clsNode("SimpleMemberAccessExpression"),
	">null_conditional_member_access":       g1clsNode("ConditionalAccessExpression"),
	">invocation_expression":                g1clsNode("InvocationExpression"),
	">element_access":                       g1clsNode("ElementAccessExpression"),
	">null_conditional_element_access":      g1clsNode("ConditionalAccessExpression"),
	">this_access":                          g1clsNode("ThisExpression"),
	">base_access":                          g1clsNode("BaseExpression"),
	">post_increment_expression":            g1clsNode("PostIncrementExpression"),
	">post_decrement_expression":            g1clsNode("PostDecrementExpression"),
	">array_creation_expression":            g1clsNode("ArrayCreationExpression", "ImplicitArrayCreationExpression"),
	">object_creation_expression":           g1clsNode("ObjectCreationExpression"),
	">delegate_creation_expression":         g1clsBlock(G1ClassBlockSemantic, "new D(x) parses as an object creation: a delegate type is told from a class type only by binding"),
	">anonymous_object_creation_expression": g1clsNode("AnonymousObjectCreationExpression"),
	">typeof_expression":                    g1clsNode("TypeOfExpression"),
	">sizeof_expression":                    g1clsNode("SizeOfExpression"),
	">checked_expression":                   g1clsNode("CheckedExpression"),
	">unchecked_expression":                 g1clsNode("UncheckedExpression"),
	">default_value_expression":             g1clsNode("DefaultExpression", "DefaultLiteralExpression"),
	">nameof_expression":                    g1clsBlock(G1ClassBlockSemantic, "nameof is an identifier that names an operator only by binding"),
	">anonymous_method_expression":          g1clsNode("AnonymousMethodExpression"),
	">stackalloc_expression":                g1clsNode("StackAllocArrayCreationExpression"),
	">explicitly_typed_default":             g1clsNode("DefaultExpression"),
	">default_literal":                      g1clsNode("DefaultLiteralExpression"),
	"named_entity":                          g1clsBinding,
	"named_entity_target":                   g1clsBinding,
	"unbound_type_name":                     g1clsBlock(G1ClassBlockNoNode, "an unbound type name is a name with omitted type arguments: Roslyn has no node for the form"),

	// ---- creation and initializers ----
	">object_initializer":             g1clsNode("ObjectInitializerExpression"),
	">collection_initializer":         g1clsNode("CollectionInitializerExpression"),
	"member_initializer_list":         g1clsList,
	"element_initializer_list":        g1clsList,
	"expression_list":                 g1clsList,
	"member_declarator_list":          g1clsList,
	"initializer_target":              g1clsSlot,
	"initializer_value":               g1clsSlot,
	"member_declarator":               g1clsBlock(G1ClassBlockNoNode, "AnonymousObjectMemberDeclarator is a hidden node of the grammar"),
	"member_declarator>member_access": g1clsNodeIn("AnonymousObjectMemberDeclarator", "SimpleMemberAccessExpression"),
	"member_declarator>null_conditional_projection_initializer": g1clsNodeIn("AnonymousObjectMemberDeclarator", "ConditionalAccessExpression"),
	"member_declarator>base_access":                             g1clsNodeIn("AnonymousObjectMemberDeclarator", "BaseExpression"),

	// ---- unary and binary ladder ----
	">primary_expression":         g1clsCategory,
	">pre_increment_expression":   g1clsNode("PreIncrementExpression"),
	">pre_decrement_expression":   g1clsNode("PreDecrementExpression"),
	">cast_expression":            g1clsNode("CastExpression"),
	">await_expression":           g1clsNode("AwaitExpression"),
	">switch_expression":          g1clsNode("SwitchExpression"),
	">multiplicative_expression":  g1clsCategory,
	">additive_expression":        g1clsCategory,
	">shift_expression":           g1clsCategory,
	">relational_expression":      g1clsCategory,
	">equality_expression":        g1clsCategory,
	">and_expression":             g1clsCategory,
	">exclusive_or_expression":    g1clsCategory,
	">inclusive_or_expression":    g1clsCategory,
	">conditional_and_expression": g1clsCategory,
	">conditional_or_expression":  g1clsCategory,
	">null_coalescing_expression": g1clsCategory,
	">conditional_expression":     g1clsCategory,
	">non_assignment_expression":  g1clsCategory,
	">assignment":                 g1clsCategory,
	">expression":                 g1clsCategory,
	">type":                       g1clsCategory,
	">throw_expression":           g1clsNode("ThrowExpression"),
	">lambda_expression":          g1clsNode("SimpleLambdaExpression", "ParenthesizedLambdaExpression"),
	">query_expression":           g1clsNode("QueryExpression"),
	"declaration_expression":      g1clsNode("DeclarationExpression"),
	"constant_expression":         g1clsBlock(G1ClassBlockSemantic, "a constant expression is an expression whose value binding must find constant"),
	"boolean_expression":          g1clsBlock(G1ClassBlockSemantic, "a boolean expression is an expression of a type that only binding gives"),

	// ---- lambdas ----
	"anonymous_function_signature":               g1clsBlock(G1ClassBlockNoNode, "Roslyn has no node for the signature: a parameter list or a parameter"),
	"implicit_anonymous_function_signature":      g1clsBlock(G1ClassBlockNoNode, "Roslyn has no node for the signature: a parameter list or a parameter"),
	"implicit_anonymous_function_parameter":      g1clsSlot,
	"implicit_anonymous_function_parameter_list": g1clsList,
	"anonymous_function_body":                    g1clsSlot,

	// ---- query expressions ----
	"query_expression":                   g1clsNode("QueryExpression"),
	"query_body":                         g1clsNode("QueryBody"),
	"query_body_clause>from_clause":      g1clsNodeIn("QueryBody", "FromClause"),
	"query_body_clause>let_clause":       g1clsNodeIn("QueryBody", "LetClause"),
	"query_body_clause>where_clause":     g1clsNodeIn("QueryBody", "WhereClause"),
	"query_body_clause>join_clause":      g1clsNodeIn("QueryBody", "JoinClause"),
	"query_body_clause>join_into_clause": g1clsNodeIn("JoinClause", "JoinIntoClause"), // Roslyn holds the into part as a child of the join clause
	"query_body_clause>orderby_clause":   g1clsNodeIn("QueryBody", "OrderByClause"),
	"orderings":                          g1clsList,
	"ordering":                           g1clsNode("AscendingOrdering", "DescendingOrdering"),
	">select_clause":                     g1clsNode("SelectClause"),
	">group_clause":                      g1clsNode("GroupClause"),

	// ---- assignments ----
	">simple_assignment":                                             g1clsNode("SimpleAssignmentExpression"),
	">compound_assignment":                                           g1clsNode(g1clsCompound...),
	"compound_assignment":                                            g1clsNode(g1clsCompound...),
	">deconstructing_assignment":                                     g1clsBlock(G1ClassBlockNoNode, "a deconstruction is a simple assignment whose left side is a tuple or a declaration: Roslyn has no kind for it"),
	">abridged_deconstructor":                                        g1clsBlock(G1ClassBlockNoNode, "var (a, b) is a declaration expression with a parenthesized designation, the node of declaration_expression"),
	"deconstructor_element>deconstructor":                            g1clsCategory,
	"deconstructor_element>declaration_expression":                   g1clsNodeIn("Argument", "DeclarationExpression"),
	"deconstructor_element>variable_reference":                       g1clsBinding,
	"abridged_deconstructor_element>identifier":                      g1clsNodeIn("ParenthesizedVariableDesignation", "SingleVariableDesignation"),
	"abridged_deconstructor_element>abridged_deconstructor_elements": g1clsNodeIn("ParenthesizedVariableDesignation", "ParenthesizedVariableDesignation"),

	// ---- statements ----
	">labeled_statement":          g1clsNode("LabeledStatement"),
	">declaration_statement":      g1clsNode("LocalDeclarationStatement", "LocalFunctionStatement"),
	">embedded_statement":         g1clsCategory,
	"embedded_statement>block":    g1clsNode("Block"),
	">empty_statement":            g1clsNode("EmptyStatement"),
	">expression_statement":       g1clsNode("ExpressionStatement"),
	">selection_statement":        g1clsCategory,
	">iteration_statement":        g1clsCategory,
	">jump_statement":             g1clsCategory,
	">try_statement":              g1clsNode("TryStatement"),
	">checked_statement":          g1clsNode("CheckedStatement"),
	">unchecked_statement":        g1clsNode("UncheckedStatement"),
	">lock_statement":             g1clsNode("LockStatement"),
	">using_statement":            g1clsNode("UsingStatement"),
	">yield_statement":            g1clsNode("YieldReturnStatement", "YieldBreakStatement"),
	">unsafe_statement":           g1clsNode("UnsafeStatement"),
	">fixed_statement":            g1clsNode("FixedStatement"),
	"statement_list":              g1clsList,
	">local_function_declaration": g1clsNode("LocalFunctionStatement"),

	// ---- local declarations and local functions ----
	"local_variable_declaration":                      g1clsCategory,
	"non_ref_local_variable_declaration":              g1clsCategory,
	"explicitly_typed_local_variable_declaration":     g1clsNode("VariableDeclaration"),
	"explicitly_typed_local_variable_declarators":     g1clsList,
	"explicitly_typed_local_variable_declarator":      g1clsNode("VariableDeclarator"),
	"local_variable_initializer>array_initializer":    g1clsNode("ArrayInitializerExpression"),
	"explicitly_typed_ref_local_variable_declaration": g1clsBlock(G1ClassBlockNoNode, "a ref local is a VariableDeclaration whose type is a RefType: the node is the one of the other forms"),
	"ref_local_variable_declarators":                  g1clsList,
	"constant_declarators":                            g1clsList,
	"local_function_declaration#1":                    g1clsNode("LocalFunctionStatement"),
	"local_function_declaration#2":                    g1clsBlock(G1ClassBlockNoNode, "the ref-returning form is a LocalFunctionStatement whose return type is a RefType: the node is the one of the first form"),
	"local_function_modifier":                         g1clsCategory,
	"ref_local_function_modifier":                     g1clsCategory,
	"local_function_body":                             g1clsSlot,
	"ref_local_function_body":                         g1clsSlot,

	// ---- expression statements, selection, loops, jumps, try, using, fixed ----
	"statement_expression>null_conditional_invocation_expression": g1clsBlock(G1ClassBlockNoNode, "a null-conditional invocation is a ConditionalAccessExpression whose right side is an invocation: Roslyn has no kind for it"),
	"statement_expression>invocation_expression":                  g1clsNodeIn("ExpressionStatement", "InvocationExpression"),
	"statement_expression>object_creation_expression":             g1clsNodeIn("ExpressionStatement", "ObjectCreationExpression"),
	"statement_expression>post_increment_expression":              g1clsNodeIn("ExpressionStatement", "PostIncrementExpression"),
	"statement_expression>post_decrement_expression":              g1clsNodeIn("ExpressionStatement", "PostDecrementExpression"),
	"statement_expression>pre_increment_expression":               g1clsNodeIn("ExpressionStatement", "PreIncrementExpression"),
	"statement_expression>pre_decrement_expression":               g1clsNodeIn("ExpressionStatement", "PreDecrementExpression"),
	"statement_expression>await_expression":                       g1clsNodeIn("ExpressionStatement", "AwaitExpression"),
	">if_statement":                                               g1clsNode("IfStatement"),
	">switch_statement":                                           g1clsNode("SwitchStatement"),
	"switch_section":                                              g1clsNode("SwitchSection"),
	">while_statement":                                            g1clsNode("WhileStatement"),
	">do_statement":                                               g1clsNode("DoStatement"),
	">for_statement":                                              g1clsNode("ForStatement"),
	">foreach_statement":                                          g1clsNode("ForEachStatement", "ForEachVariableStatement"),
	"for_initializer>local_variable_declaration":                  g1clsNodeIn("ForStatement", "VariableDeclaration"),
	">statement_expression_list":                                  g1clsList,
	"statement_expression_list":                                   g1clsList,
	"for_condition":                                               g1clsSlot,
	">break_statement":                                            g1clsNode("BreakStatement"),
	">continue_statement":                                         g1clsNode("ContinueStatement"),
	">goto_statement":                                             g1clsNode("GotoStatement", "GotoCaseStatement", "GotoDefaultStatement"),
	">return_statement":                                           g1clsNode("ReturnStatement"),
	">throw_statement":                                            g1clsNode("ThrowStatement"),
	"catch_clauses":                                               g1clsList,
	"resource_acquisition>non_ref_local_variable_declaration":     g1clsNodeIn("UsingStatement", "VariableDeclaration"),
	"fixed_pointer_declarators":                                   g1clsList,
}
