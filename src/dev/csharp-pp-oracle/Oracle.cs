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
using Microsoft.CodeAnalysis.Text;

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
            var root = (CompilationUnitSyntax)tree.GetRoot();
            int Byte(int charPos) => utf8.GetByteCount(text.AsSpan(0, charPos));
            // a diagnostic may point between the halves of a surrogate pair: report the pair's start
            int DiagByte(int charPos) => Byte(charPos > 0 && char.IsHighSurrogate(text[charPos - 1]) ? charPos - 1 : charPos);
            var directives = new List<object>();
            var disabled = new List<object>();
            // where Roslyn attaches each error: a directive's kind, EndOfFile, or Other
            var context = new Dictionary<(string, TextSpan), string>();
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
                string kind = d.Kind().ToString();
                foreach (var x in d.GetDiagnostics()) context.TryAdd((x.Id, x.Location.SourceSpan), kind[..^"DirectiveTrivia".Length]);
                // the physical lines of the directive by Roslyn's own line breaks, last line break included
                // (a directive's token may run over several lines, e.g. a raw string in #pragma)
                var lines = tree.GetText().Lines;
                var line = lines.GetLineFromPosition(d.HashToken.Span.Start);
                var last = lines.GetLineFromPosition(Math.Max(d.FullSpan.End - 1, d.FullSpan.Start));
                directives.Add(new
                {
                    kind,
                    start = Byte(d.HashToken.Span.Start),
                    active = d.IsActive,
                    taken,
                    line_start = Byte(line.Start),
                    line_end = Byte(last.EndIncludingLineBreak),
                });
            }
            // after the directives: the end-of-file token also carries the errors of directives in its leading trivia
            foreach (var x in root.EndOfFileToken.GetDiagnostics()) context.TryAdd((x.Id, x.Location.SourceSpan), "EndOfFile");
            Diagnostic[] errors;
            try
            {
                errors = tree.GetDiagnostics().Where(x => x.Severity == DiagnosticSeverity.Error).ToArray();
            }
            catch (ArgumentOutOfRangeException e) // Roslyn 5.9 fails to enumerate some diagnostics (observed): no observation
            {
                results.Add(new { id, oracle_error = "GetDiagnostics " + e.GetType().Name });
                continue;
            }
            var errorIds = errors.Select(x => x.Id).Distinct().OrderBy(x => x, StringComparer.Ordinal).ToArray();
            var located = errors.Select(x => new
            {
                id = x.Id,
                start = DiagByte(x.Location.SourceSpan.Start),
                end = DiagByte(x.Location.SourceSpan.End),
                context = context.GetValueOrDefault((x.Id, x.Location.SourceSpan), "Other"),
            }).ToArray();
            results.Add(new { id, features = options.Features.Keys.OrderBy(k => k, StringComparer.Ordinal).ToArray(), directives, disabled, errors = errorIds, diagnostics = located });
        }
        var output = new
        {
            schema = "tsgk-csharp-pp-roslyn/r2",
            cases_sha256 = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(input)).ToLowerInvariant(),
            roslyn = typeof(CSharpSyntaxTree).Assembly.GetName().Version!.ToString(),
            language_version = "Latest",
            cases = results,
        };
        File.WriteAllText(args[1], JsonSerializer.Serialize(output, new JsonSerializerOptions { WriteIndented = true, NewLine = "\n" }) + "\n", utf8);
        return 0;
    }
}
