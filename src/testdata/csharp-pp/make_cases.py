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
    # malformed
    ("malformed-expression", "#if A &&\nclass X {}\n#endif\n", ["A"]),
    ("unterminated", "#if A\nclass X {}\n", ["A"]),
    ("else-after-else", "#if A\nclass X {}\n#else\nclass Y {}\n#else\nclass Z {}\n#endif\n", []),
    ("endif-without-if", "class C {}\n#endif\n", []),
]

out = {"schema": "tsgk-csharp-pp-cases/r1", "cases": [
    {"id": i, "source": base64.b64encode(s.encode("utf-8")).decode(), "symbols": sym} for i, s, sym in CASES]}
here = os.path.dirname(os.path.abspath(__file__))
with open(os.path.join(here, "cases.json"), "w", encoding="utf-8", newline="\n") as f:
    json.dump(out, f, indent=1)
    f.write("\n")
