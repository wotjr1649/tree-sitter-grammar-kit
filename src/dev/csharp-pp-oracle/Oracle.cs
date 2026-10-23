// Development oracle for #218: records how Roslyn treats the conditional-compilation
// directives of registered sources under registered symbols. It is never a runtime
// dependency; its output is a fixture the Go evaluator tests read offline.
//
// usage: dotnet Oracle.dll <cases.json> <out.json>
// Every offset is a UTF-8 byte offset into the source.
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text;
using System.Text.Json;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

public static class Oracle
{
    public static int Main(string[] args)
    {
        if (args.Length != 2) throw new Exception("usage: Oracle <cases.json> <out.json>");
        byte[] input = File.ReadAllBytes(args[0]);
        if (input.Length > 1 << 20) throw new Exception("input bound");
        var utf8 = new UTF8Encoding(false, true);
        using var doc = JsonDocument.Parse(input);
        var results = new List<object>();
        var ids = new HashSet<string>();
        foreach (var item in doc.RootElement.GetProperty("cases").EnumerateArray())
        {
            string id = item.GetProperty("id").GetString()!;
            if (!ids.Add(id)) throw new Exception("duplicate id " + id);
            byte[] bytes = Convert.FromBase64String(item.GetProperty("source").GetString()!);
            string text = utf8.GetString(bytes);
            var symbols = item.GetProperty("symbols").EnumerateArray().Select(s => s.GetString()!).ToArray();
            var options = new CSharpParseOptions(LanguageVersion.Latest, DocumentationMode.Parse, SourceCodeKind.Regular, symbols);
            // a case may name parser features, e.g. FileBasedProgram for #! and #: directives (C# 14)
            if (item.TryGetProperty("features", out var features))
                options = options.WithFeatures(features.EnumerateArray().Select(f => new KeyValuePair<string, string>(f.GetString()!, "true")));
            var tree = CSharpSyntaxTree.ParseText(text, options);
            var root = tree.GetRoot();
            int Byte(int charPos) => utf8.GetByteCount(text.AsSpan(0, charPos));
            var directives = new List<object>();
            var disabled = new List<object>();
            foreach (var trivia in root.DescendantTrivia(descendIntoTrivia: true))
            {
                if (trivia.IsKind(SyntaxKind.DisabledTextTrivia))
                {
                    disabled.Add(new { start = Byte(trivia.Span.Start), end = Byte(trivia.Span.End) });
                    continue;
                }
                if (trivia.GetStructure() is not DirectiveTriviaSyntax d) continue;
                bool? taken = d switch
                {
                    BranchingDirectiveTriviaSyntax b => b.BranchTaken,
                    _ => null,
                };
                directives.Add(new
                {
                    kind = d.Kind().ToString(),
                    start = Byte(d.HashToken.Span.Start),
                    active = d.IsActive,
                    taken,
                });
            }
            var diagnostics = tree.GetDiagnostics().Where(x => x.Severity == DiagnosticSeverity.Error).Select(x => x.Id).Distinct().OrderBy(x => x, StringComparer.Ordinal).ToArray();
            results.Add(new { id, features = options.Features.Keys.OrderBy(k => k, StringComparer.Ordinal).ToArray(), directives, disabled, errors = diagnostics });
        }
        var output = new
        {
            schema = "tsgk-csharp-pp-roslyn/r1",
            cases_sha256 = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(input)).ToLowerInvariant(),
            roslyn = typeof(CSharpSyntaxTree).Assembly.GetName().Version!.ToString(),
            language_version = "Latest",
            cases = results,
        };
        File.WriteAllText(args[1], JsonSerializer.Serialize(output, new JsonSerializerOptions { WriteIndented = true, NewLine = "\n" }) + "\n", utf8);
        return 0;
    }
}
