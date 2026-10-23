"""#218 Roslyn differential: writes seeded batches of C# preprocessor cases (local check, not CI).

usage: python -I gen.py OUT_DIR SEED... [--count N]

Each seed gives N cases (default 500): half generated from a small directive grammar, half
mutated from src/testdata/csharp-pp/cases.json, so a seed reproduces only with the same
cases.json (record its sha256 with a run). Batches are cases.json-shaped files
OUT_DIR/batch-<seed>-<k>.json, each under 900 KiB so the oracle's 1 MiB input bound holds.
Then, per batch, with a 120 s bound:
  timeout 120 dotnet <oracle>/Oracle.dll OUT_DIR/batch-S-K.json OUT_DIR/observed-S-K.json
and compare:
  TSGK_PP_DIFF_DIR=OUT_DIR go test ./src/kit -run TestCSharpPreprocessDifferential -count=1 -v
(a case Roslyn fails to observe passes only when named in TSGK_PP_DIFF_UNOBSERVED=id,id)
Generated sources stay far below the evaluator's byte, directive, depth and symbol bounds.
"""
import base64
import json
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
FIXED = os.path.join(HERE, "..", "..", "testdata", "csharp-pp", "cases.json")
BATCH_BYTES = 900 * 1024
LIMIT_BOUNDARY = {"define-feff-tail-127"}

SYMBOLS = ["A", "B", "C", "D", "\\u0041", "\\U00000042", "A\\u200C", "A\u200c", "fal\\u017Fe", "_x1", "\u00e9t\u00e9"]
CONSTANTS = ["true", "false", "TRUE", "False", "tRuE", "\\u0074rue"]
CALLER = ["A", "B", "C", "D", "A\u200c", "_x1", "\u00e9t\u00e9"]
CODE = [
    "class X {}",
    "int x = 1;",
    'string s = "#if A";',
    'string s = @"',
    '#if A',
    '";',
    'string s = """',
    '"""; ',
    'string s = $"{1:#,##0}";',
    'string s = $"{(true ? 1 : 2):D2}";',
    'string s = $@"{1 +',
    '2}";',
    'string s = $$"""{{1}}{x}""";',
    "char c = '#';",
    "string s = \"\U0001F600\u00e9\";",
    "// \U00010400 #if A",
    "// #if A",
    "/*",
    "#endif",
    "*/",
    "/* #if A */ class Y {}",
    "class Z { }}",
    "x = 1 + ;",
]
LEAD = ["", "", "", " ", "\t", "  ", "\u00a0", "\ufeff", "\u001a", "\u3000", "\u2003"]
TRAIL = ["", "", "", " ", " // c", " /* c */"]
OTHER = ["#region r", "#endregion", "#pragma warning disable CS0168", "#nullable enable", "#error e", "#warning w", "#line 5", "#line abc", "#foo", "#if", "#define 1A"]
TERMS = ["\n", "\n", "\n", "\r\n", "\r", "\u0085", "\u2028", "\u2029"]
SNIPPETS = ["#", "\n", "\r", '"', '@"', '$"{', "}", "{", "/*", "*/", "//", "#if A\n", "#else\n", "#endif\n", "#define B\n",
            "\\u0041", "\u200c", "\ufeff", "!", "&&", "||", "(", ")", "==", " ", "#region\n", "#endregion\n", '"""']


def expr(rng, depth=0):
    k = rng.randrange(9 if depth < 3 else 2)
    if k == 0:
        return rng.choice(SYMBOLS)
    if k == 1:
        return rng.choice(CONSTANTS + SYMBOLS)
    if k == 2:
        return "!" + expr(rng, depth + 1)
    if k == 3:
        return "(" + expr(rng, depth + 1) + ")"
    op = rng.choice([" && ", " || ", " == ", " != ", "&&", "||"])
    return expr(rng, depth + 1) + op + expr(rng, depth + 1)


def generated(rng):
    lines, stack = [], []  # stack: has #else
    if rng.random() < 0.1:
        lines.append(rng.choice(["#define A", "#undef B", "#define C"]))
    for _ in range(rng.randrange(1, 24)):
        r = rng.random()
        if r < 0.3:
            lines.append(rng.choice(CODE))
        elif r < 0.45 and len(stack) < 5:
            lines.append(rng.choice(LEAD) + "#if " + expr(rng) + rng.choice(TRAIL))
            stack.append(False)
        elif r < 0.55 and stack and not stack[-1]:
            lines.append(rng.choice(LEAD) + "#elif " + expr(rng) + rng.choice(TRAIL))
        elif r < 0.62 and stack and not stack[-1]:
            lines.append(rng.choice(LEAD) + "#else" + rng.choice(TRAIL))
            stack[-1] = True
        elif r < 0.75 and stack:
            lines.append(rng.choice(LEAD) + "#endif" + rng.choice(TRAIL))
            stack.pop()
        elif r < 0.82:
            lines.append(rng.choice(LEAD) + rng.choice(["#define ", "#undef "]) + rng.choice(SYMBOLS) + rng.choice(TRAIL))
        else:
            lines.append(rng.choice(LEAD) + rng.choice(OTHER))
    if rng.random() < 0.9:
        lines += ["#endif"] * len(stack)
    if rng.random() < 0.05:
        lines.insert(rng.randrange(len(lines) + 1), "class C {} #if A")
    term = rng.choice(TERMS)
    mixed = rng.random() < 0.15
    src = "".join(l + (rng.choice(TERMS) if mixed else term) for l in lines)
    if rng.random() < 0.15:
        src = src.rstrip("\r\n\u0085\u2028\u2029")
    r = rng.random()
    if r < 0.05:
        src = "#!/usr/bin/env dotnet\n" + src
    elif r < 0.1:
        src = "#:sdk Microsoft.NET.Sdk\n" + src
    if rng.random() < 0.08:
        src = "\ufeff" + src
    return src


def mutated(rng, fixed):
    base = rng.choice(fixed)
    src = base64.b64decode(base["source"]).decode("utf-8")
    for _ in range(rng.randrange(1, 4)):
        i = rng.randrange(len(src) + 1)
        k = rng.randrange(3)
        if k == 0 and src:
            src = src[:i] + src[i + rng.randrange(1, 4):]
        elif k == 1:
            src = src[:i] + rng.choice(SNIPPETS) + src[i:]
        else:
            src = src.replace("\n", rng.choice(TERMS))
    return src


def main():
    args = sys.argv[1:]
    count = 500
    if "--count" in args:
        j = args.index("--count")
        count = int(args[j + 1])
        del args[j:j + 2]
    out, seeds = args[0], [int(s) for s in args[1:]]
    if not seeds or count < 2:
        sys.exit(__doc__)
    with open(FIXED, encoding="utf-8") as f:
        # a case at a symbol-length bound mutates into intended LIMIT results, which the bound
        # tests in csharp_pp_test.go own; the case itself stays in the fixed comparison
        fixed = [c for c in json.load(f)["cases"] if c["id"] not in LIMIT_BOUNDARY]
    os.makedirs(out, exist_ok=True)
    for seed in seeds:
        rng = random.Random(seed)
        batch, k = [], 0

        def flush():
            nonlocal batch, k
            if batch:
                path = os.path.join(out, f"batch-{seed}-{k}.json")
                with open(path, "w", encoding="utf-8", newline="\n") as f:
                    json.dump({"schema": "tsgk-csharp-pp-cases/r1", "cases": batch}, f, indent=1)
                    f.write("\n")
                if os.path.getsize(path) > BATCH_BYTES:
                    sys.exit(f"{path}: over {BATCH_BYTES} bytes")
                batch, k = [], k + 1

        size = 0
        for n in range(count):
            src = generated(rng) if n % 2 == 0 else mutated(rng, fixed)
            # the evaluator reads C# 14 file-based directives (#!, #:) as the registered grammar does
            case = {"id": f"s{seed}-{n}", "source": base64.b64encode(src.encode("utf-8")).decode(),
                    "symbols": rng.sample(CALLER, rng.randrange(0, 4)), "features": ["FileBasedProgram"]}
            text = json.dumps(case, indent=1)
            n_bytes = len(text.encode("utf-8")) + 2 * text.count("\n") + 8  # one level deeper in the batch
            if size + n_bytes > BATCH_BYTES:
                flush()
                size = 0
            batch.append(case)
            size += n_bytes
        flush()


if __name__ == "__main__":
    main()
