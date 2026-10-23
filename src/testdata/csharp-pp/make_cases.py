"""Writes cases.json, the #218 preprocessor evaluator cases the Roslyn oracle observes.

Run: python -I make_cases.py (in this directory). Sources are UTF-8, stored base64.
"""
import base64
import json
import os

ELIF = "#if A\nclass X {}\n#elif B\nclass Y {}\n#elif C\nclass Z {}\n#else\nclass W {}\n#endif\n"
OPS = "#if A && !B || (C == D)\nclass X {}\n#else\nclass Y {}\n#endif\n"
CASES = [
    # #198 holdout sources
    ("issue198-inactive-broken", "#if NEVER\nclass C { int M() { return 1 + ; } }\n#else\nclass C {}\n#endif\n", []),
    ("issue198-undef-inactive", "#undef ENABLED\n#if ENABLED\ninvalid + ;\n#else\nclass C {}\n#endif\n", ["ENABLED"]),
    ("issue198-nested-directives", "#if A\n#if B\ninvalid + ;\n#elif C\nclass C {}\n#else\nclass D {}\n#endif\n#endif\n", ["A", "C"]),
    # evaluation
    ("ops-a", OPS, ["A"]),
    ("ops-ab", OPS, ["A", "B"]),
    ("ops-cd", OPS, ["C", "D"]),
    ("ops-c", OPS, ["C"]),
    ("ops-none", OPS, []),
    ("equality-false", "#if A == false\nclass X {}\n#endif\n", []),
    ("not-equal", "#if A != B\nclass X {}\n#endif\n", ["B"]),
    ("elif-b", ELIF, ["B"]),
    ("elif-ab", ELIF, ["A", "B"]),
    ("elif-none", ELIF, []),
    ("literal-true", "#if true\nclass X {}\n#else\nclass Y {}\n#endif\n", []),
    ("literal-paren-false", "#if (false)\nclass X {}\n#else\nclass Y {}\n#endif\n", []),
    ("literal-not-true", "#if !(true)\nclass X {}\n#else\nclass Y {}\n#endif\n", []),
    ("unicode-escape", "#if \\u0041\nclass X {}\n#endif\n", ["A"]),
    # define and undef
    ("define-undef-order", "#define A\n#undef A\n#if A\nclass X {}\n#else\nclass Y {}\n#endif\n", []),
    ("define-after-token", "class Z {}\n#define A\n#if A\nclass X {}\n#endif\n", []),
    ("define-inactive", "#if NO\n#define A\n#endif\n#if A\nclass X {}\n#endif\n", []),
    ("nested-in-inactive", "#if NO\n#if YES\nclass X {}\n#else\nclass Y {}\n#endif\n#endif\n", ["YES"]),
    # directive recognition
    ("verbatim-string", "class C { string s = @\"\n#if A\n\"; }\n#if B\nclass X {}\n#endif\n", []),
    ("raw-string", "class C { string s = \"\"\"\n#if A\n\"\"\"; }\n", []),
    ("interpolated-verbatim", "class C { string s = $@\"{1}\n#if A\n\"; }\n", []),
    ("interpolated-raw", "class C { string s = $$\"\"\"\n{{1}}\n#if A\n\"\"\"; }\n", []),
    ("block-comment", "/*\n#if A\n*/\nclass C {}\n", []),
    ("line-comment", "// #if A\nclass C {}\n", []),
    ("char-literal", "class C { char c = '#'; }\n", []),
    ("directive-after-code", "class C {} #if A\nclass X {}\n#endif\n", ["A"]),
    ("trailing-line-comment", "#if A // note\nclass X {}\n#endif\n", ["A"]),
    ("trailing-block-comment", "#if A /* c */\nclass X {}\n#endif\n", ["A"]),
    ("region-slash-star", "#region x/*\n#if A\nclass X {}\n#endif\n#endregion\n", ["A"]),
    ("nullable-kept", "#nullable enable\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("indented-directive", "  \t#if A\nclass X {}\n  #endif\n", []),
    # line terminators, BOM, EOF, non-ASCII
    ("crlf", OPS.replace("\n", "\r\n"), ["A"]),
    ("bom", "\ufeff" + OPS, ["C", "D"]),
    ("eof-no-newline", "#if A\nclass X {}\n#endif", ["A"]),
    ("lone-cr", "#if A\rclass X {}\r#endif\r", []),
    ("line-separator", "#if A\u2028class X {}\u2028#endif\u2028", []),
    ("non-ascii", "class C { string s = \"\ud55c\uae00\"; }\n#if A\nclass X {}\n#else\nclass Y {}\n#endif\n", []),
    # interpolation holes, format clauses and raw strings
    ("interp-format", "class C { string s = $\"{123:#,##0}\"; }\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("interp-format-slashes", "class C { string s = $\"{1:/*x//}\"; }\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("interp-nested-string", "class C { string s = $\"{(\"}\" + \"{\")}\"; }\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("interp-ternary", "class C { string s = $\"{(true ? 1 : 2):D2}\"; }\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("interp-global-alias", "class C { string s = $\"{global::System.Math.PI}\"; }\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("interp-verbatim-hole-multiline", "class C { string s = $@\"{1 +\n2}\n#if A\n\"; }\n", []),
    ("raw-four-quotes", "class C { string s = \"\"\"\"\n#if A\n\"\"\" still\n\"\"\"\"; }\n", []),
    ("raw-interp-two-dollars", "class C { string s = $$\"\"\"\n{{1:#}}{x}\n#if A\n\"\"\"; }\n", []),
    # symbols, define and undef
    ("escape-upper", "#if \\U00000041\nclass X {}\n#endif\n", ["A"]),
    ("cf-symbol", "#define A\\u200C\n#if A\nclass X {}\n#else\nclass Y {}\n#endif\n", []),
    ("define-undef-define", "#define A\n#undef A\n#define A\n#if A\nclass X {}\n#endif\n", []),
    ("inactive-define-before-token", "#if NO\n#define A\n#endif\nclass Z {}\n", []),
    ("inactive-malformed-symbol", "#if NO\n#define 1A\n#endif\nclass X {}\n", []),
    ("inactive-unknown-directive", "#if NO\n#foo\n#endif\nclass X {}\n", []),
    # Unicode whitespace and pragma kinds
    ("nbsp-line-before-define", "\u00a0\n#define A\n#if A\nclass X {}\n#endif\n", []),
    ("nbsp-comment-before-define", "\u00a0// c\n#define A\n#if A\nclass X {}\n#endif\n", []),
    ("nbsp-indented-directive", "\u00a0#if A\nclass X {}\n#endif\n", ["A"]),
    ("pragma-warning", "#pragma warning disable CS0168\nclass X {}\n", []),
    ("pragma-warning-nbsp", "#pragma warning\u00a0disable CS0168\nclass X {}\n", []),
    ("pragma-checksum", "#pragma checksum \"f.cs\" \"{406EA660-64CF-4C82-B6F0-42D48172A799}\" \"ab\"\nclass X {}\n", []),
    ("pragma-bare", "#pragma\nclass X {}\n", []),
    # more line terminators and positions
    ("nel", "#if A\u0085class X {}\u0085#endif\u0085", []),
    ("paragraph-separator", "#if A\u2029class X {}\u2029#endif\u2029", []),
    ("bom-non-ascii", "\ufeff// \ud55c\n#if A\nclass X {}\n#endif\n", ["A"]),
    # PR #270 review: position, region nesting and bool constants
    ("inactive-define-after-token", "class C {}\n#if false\n#define A\n#endif\n", []),
    ("region-crosses-endif-inactive", "#if false\n#region R\n#endif\nclass C {}\n", []),
    ("unclosed-region", "#region R\nclass C {}\n", []),
    ("endregion-inside-group", "#region R\n#if false\n#endregion\n#endif\n", []),
    ("region-crosses-else", "#if A\n#region R\n#else\n#endregion\n#endif\nclass C {}\n", ["A"]),
    ("region-crosses-elif", "#if A\n#region R\n#elif B\n#endregion\n#endif\nclass C {}\n", ["A"]),
    ("region-in-group","#if A\n#region R\n#endregion\n#endif\nclass C {}\n", ["A"]),
    ("escaped-true", "#if \\u0074rue\nclass X {}\n#else\nclass Y {}\n#endif\n", []),
    ("upper-true", "#if TRUE\nclass X {}\n#else\nclass Y {}\n#endif\n", []),
    ("mixed-false", "#if FaLsE\nclass X {}\n#else\nclass Y {}\n#endif\n", []),
    ("supplementary-escape-symbol", "#define \\U00010400\n#if \\U00010400\nclass X {}\n#endif\n", []),
    ("supplementary-symbol", "#define \U00010400\n#if \U00010400\nclass X {}\n#endif\n", []),
    # PR #270 review: C# 14 file-based directives and more whitespace
    ("shebang", "#!/usr/bin/env dotnet\n#if A\nclass X {}\n#endif\n", ["A"], ["FileBasedProgram"]),
    ("file-based-sdk", "#:sdk Microsoft.NET.Sdk\n#if A\nclass X {}\n#endif\n", ["A"], ["FileBasedProgram"]),
    ("file-based-in-inactive", "#if NO\n#:package Foo@1.0\n#endif\nclass X {}\n", [], ["FileBasedProgram"]),
    ("file-based-after-token", "class C {}\n#:sdk Microsoft.NET.Sdk\n", [], ["FileBasedProgram"]),
    ("file-based-in-active-if", "#if true\n#:sdk Microsoft.NET.Sdk\n#endif\nclass C {}\n", [], ["FileBasedProgram"]),
    ("file-based-after-endif", "#if NO\n#endif\n#:sdk Microsoft.NET.Sdk\nclass C {}\n", [], ["FileBasedProgram"]),
    ("long-s-symbol", "#define fal\\u017Fe\n#if fal\\u017Fe\nclass Selected {}\n#else\nclass Other {}\n#endif\n", []),
    ("caller-cf-symbol", "#if A\nclass Selected {}\n#else\nclass Other {}\n#endif\n", ["A‌"]),
    ("caller-plain-matches-cf-source", "#if A\\u200C\nclass Selected {}\n#else\nclass Other {}\n#endif\n", ["A"]),
    ("shebang-after-bom", "﻿#!/usr/bin/env dotnet\nclass C {}\n", [], ["FileBasedProgram"]),
    ("shebang-not-first", "class C {}\n#!/usr/bin/env dotnet\n", [], ["FileBasedProgram"]),
    ("feff-before-directive", "\n﻿#if A\nclass C {}\n#endif\n", ["A"]),
    ("feff-in-directive", "#if﻿A\nclass C {}\n#endif\n", ["A"]),
    ("feff-line-before-define", "﻿\n﻿\n#define A\n#if A\nclass C {}\n#endif\n", []),
    ("ctrl-z-space", "#if\u001aA\nclass C {}\n#endif\n", ["A"]),
    # 218-A Roslyn differential (src/dev/csharp-pp-diff): one minimal case per defect found
    ("raw-single-line-unterminated", "x = \"\"\"; \n#define A\n#if A\nclass X {}\n#endif\n", []),
    ("interp-raw-single-line-unterminated", "x = $\"\"\"{1}; \n#define A\n", []),
    ("dollars-two-quotes-raw", "class C { string s = $$\"\"\n{{1:#}}{x}\n#if A\n\"\"\"; }\n", []),
    ("dollars-at-raw", "class C { string s = $$@\"\"\"\"\n{{1:#}}{x}\nf A\n\"\"\"; }\n", []),
    ("at-dollar-at-raw", "x = @$@\"a\n#if A\nb\";\n", ["A"]),
    ("bad-string-prefix","class C { string s = $@$\"{\"{1}\n#if A\n\"; }\n", ["A"]),
    ("hole-hash-no-directive", "x = $\"{1 +\n#if A\nclass X {}\n#endif\n2}\";\n", ["A"]),
    ("hole-paren-then-brace", "x = $\"{(a}\";\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("hole-mismatch-runaway", "x = $\"{(a @\"b}\"; }\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("verbatim-runaway-two-quotes", "x = $@\"{1 +\n#x\n\"\"\nclass X {}\n#undef B\n", []),
    ("verbatim-error-content-quotes", "x = $@\"{1 +\n#a\n}\"\"; \nclass C {} #if A\n#endif\n", ["A"]),
    ("raw-runaway-quote", "x = $$\"\"\"{{1 +\n#x\n\"\nclass X {}\n#undef B\n", []),
    ("regular-lone-close-brace-runaway", "x = $\"}{1\" \n#if A\nclass X {}\n#endif\n", ["A"]),
    ("lone-close-brace-runaway","class C { string s = $@\"}{1\" \n2}\n#if A\n\"; }\n", []),
    ("nested-string-open-no-runaway", "class C { string s = $\"{(\"}\" + \"{#else\n\")}\"; }\n# A\nclass X {}\n#endif\n", []),
    ("format-clause-quote", "class C { string s = $\"{123:#,#\"\"\"#0}\"; }\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("format-after-global-colon", "x = $\"{global::A //}\";\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("feff-after-hash", "#﻿if A\nclass X {}\n#endif\n", ["A"]),
    ("feff-before-condition", "#if ﻿A\nclass X {}\n#else\nclass Y {}\n#endif\n", ["A"]),
    ("reference-directive", "#if NO\n#r x\n#endif\n#r \"a\"\nclass C {}\n", []),
    ("directive-name-backslash", "#r\\u0egion} R\nclass C {}\n", []),
    ("directive-name-then-backslash", "#r\\gion} R\nclass C {}\n", []),
    ("dollars-raw-quote-no-runaway", "class C { string s = $$\"\"\n{{\"1:#}}{x}\n#i==f A\n\"\"\"; }\n", []),
    ("raw-content-close-braces", "x = $\"\"\"\"}{1\" \n#if A\n#region\nclass X {}\n#endif\n", []),
    ("multi-line-raw-runaway-quote-content", "x = $$\"\"\"\n{{1 +\n#x\n\"\nclass X {}\n#undef B\n", []),
    ("single-line-raw-runaway-then-comment", "x = $$\"\"\"{{1 +\n#x\n\"/*\nclass X {}\n#undef B\n", []),
    ("razor-comment-lines", "x = @* c\n#if A\n*@\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("razor-comment-unterminated", "x = @* c */\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("razor-comment-not-a-token", "@* c *@\n#define B\n", []),
    ("raw-too-many-open-braces", "x = $\"\"\"{{1 +\n\"/*\nclass X {}\n#undef B\n", []),
    ("raw-open-braces-within-limit", "x = $$\"\"\"{{{1 +\n\"/*\nclass X {}\n#undef B\n", []),
    ("at-run-verbatim","x = @@\"a\n#if A\nb \";\n", ["A"]),
    ("one-quote-raw-hole-quote-closes", "class C { string s = $$\"\n{{\"1:#&&}}{x}\n#i==f A\n\"\"\"; }\n", ["A"]),
    ("few-quotes-single-line-raw-runaway", "x = $$\"\")\"{{1 +\n\"/*\nclass X {}\n#undef B\n", []),
    ("dollars-at-run-raw","class C { string s = $$@@\"\n\"\"\"\"\n{{1:#}}{x}\nf A\n\"\"; }\n", []),
    ("shebang-not-equals", "#!==r/bin/env dotnet\n#if A\nclass X {}\n#endif\n", ["A"], ["FileBasedProgram"]),
    ("elif-sees-skipped-define", "#if NO\n#define B\n#elif B\nclass Y {}\n#endif\n", []),
    ("elif-skips-earlier-branch", "#if NO\n#define B\n#elif NO2\n#elif B\nclass Y {}\n#endif\n", []),
    ("elif-sees-skipped-region-define", "#if NO\n#region r\n#define B\n#endregion\n#elif B\nclass Y {}\n#endif\n", []),
    ("elif-skips-nested-group-define", "#if NO\n#if YES\n#define B\n#endif\n#elif B\nclass Y {}\n#endif\n", ["YES"]),
    ("elif-sees-skipped-undef", "#define B\n#if NO\n#undef B\n#elif B\nclass Y {}\n#else\nclass Z {}\n#endif\n", []),
    ("pragma-raw-takes-lines", "#pragma warning disable \"\"\"\n#if A\n\"\"\"\n#if A\nclass X {}\n#endif\n", ["A"]),
    ("inactive-pragma-raw-takes-endif", "#if NO\n#pragma warning disable \"\"\"\n#endif\n\"\"\"\nclass C {}\n#endif\n", []),
    ("nullable-raw-to-eof", "#nullable enable \"\"\"\n#if A\n", ["A"]),
    ("pragma-single-line-raw", "#pragma warning disable \"\"\"x\"\"\"\n#if A\nclass X {}\n#endif\n", ["A"]),
    # malformed
    ("malformed-expression", "#if A &&\nclass X {}\n#endif\n", ["A"]),
    ("unterminated", "#if A\nclass X {}\n", ["A"]),
    ("else-after-else", "#if A\nclass X {}\n#else\nclass Y {}\n#else\nclass Z {}\n#endif\n", []),
    ("endif-without-if", "class C {}\n#endif\n", []),
]

def case(i, s, sym, features=None):
    c = {"id": i, "source": base64.b64encode(s.encode("utf-8")).decode(), "symbols": sym}
    if features:
        c["features"] = features
    return c


out = {"schema": "tsgk-csharp-pp-cases/r1", "cases": [case(*c) for c in CASES]}
here = os.path.dirname(os.path.abspath(__file__))
with open(os.path.join(here, "cases.json"), "w", encoding="utf-8", newline="\n") as f:
    json.dump(out, f, indent=1)
    f.write("\n")
