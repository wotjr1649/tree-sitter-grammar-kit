import { comma_list, paren_list } from '../helpers.js';

// T-SQL CREATE [OR ALTER] FUNCTION. Three SQL Server function kinds share one
// rule and are told apart by what follows RETURNS:
//
//   scalar:                  RETURNS <type>            AS BEGIN ... RETURN x END
//   inline table-valued:     RETURNS TABLE             [AS] RETURN ( select )
//   multi-statement TVF:     RETURNS @t TABLE ( ... )  AS BEGIN ... RETURN END
export default {
  create_function: ($) => seq($.keyword_create, optional($._or_alter), $._function_definition),

  alter_function: ($) => seq($.keyword_alter, $._function_definition),

  _function_definition: ($) =>
    seq(
      $.keyword_function,
      field('name', $.object_reference),
      $.function_arguments,
      $.keyword_returns,
      choice($._type, $.keyword_table, $.table_var_declaration),
      optional($.function_options),
      $.function_body,
    ),

  // @name [AS] type [= default] [OUT | OUTPUT] [READONLY]
  // The name is required (T-SQL has no unnamed parameters).
  function_argument: ($) =>
    seq(
      $.identifier,
      optional($.keyword_as),
      $._type,
      optional(seq('=', $._expression)),
      optional(choice($.keyword_out, $.keyword_output)),
      optional($.keyword_readonly),
    ),

  // Functions require the parenthesized list (and allow it empty).
  // Procedures also accept the bare comma-separated spelling — see
  // `procedure_arguments` in create-procedure.js, which is the only other
  // consumer of `function_argument`.
  function_arguments: ($) => paren_list($.function_argument, false),

  function_options: ($) => seq($.keyword_with, comma_list($.function_option, true)),

  function_option: ($) =>
    choice(
      $.keyword_encryption,
      $.keyword_schemabinding,
      seq($.keyword_returns, $.keyword_null, $.keyword_on, $.keyword_null, $.keyword_input),
      seq($.keyword_called, $.keyword_on, $.keyword_null, $.keyword_input),
      $.execute_as_clause,
      // INLINE = ON and the like
      seq($.identifier, '=', choice($.keyword_on, $.keyword_off)),
    ),

  function_body: ($) =>
    choice(
      // Scalar and multi-statement table-valued: a BEGIN ... END block whose
      // RETURN is an ordinary return_statement among the body items.
      seq(optional($.keyword_as), $.keyword_begin, repeat($._body_item), $.keyword_end),
      // Inline table-valued: RETURN followed directly by the query, with or
      // without parentheses.
      seq(optional($.keyword_as), $.keyword_return, $._dml_read),
    ),
};
