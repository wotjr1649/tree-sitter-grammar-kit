// Development tool for #233 G1-2: records what the Roslyn of the local .NET SDK observes of registered
// C# sources: acceptance, diagnostics, declarations, syntax nodes, directives and the acceptance under
// each LanguageVersion. It is never a runtime dependency; its output is an asset the Go offline
// verifier reads. It reads no native output.
//
// usage: dotnet Observe.dll <input.json> <out.json> [<ledger.json>]
// With a ledger file the asset pins it (path, bytes, SHA-256); the Go verifier requires that pin.
// The input is {"cases":[{"id","source"(base64),"symbols"[],"features"[]}]}, the shape of
// src/testdata/csharp-pp/cases.json. Every offset is a UTF-8 byte offset into the source.
// The directive fields (features, directives, disabled, errors, diagnostics) are the
// tsgk-csharp-pp-roslyn/r2 observation of src/dev/csharp-pp-oracle, so that tool's 157 observations
// can be reproduced from this one.
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Text.RegularExpressions;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using Microsoft.CodeAnalysis.Text;

public static class Observe
{
    const int InputLimit = 32 << 20;  // bytes of the input document
    const int OutputLimit = 64 << 20; // bytes of the output document
    const int NodeLimit = 1 << 20;    // syntax nodes of one source
    const int MinVersion = 703;       // LanguageVersion.CSharp7_3
    const string LedgerPath = "src/contracts/csharp-g1-ledger-r1.json";

    sealed class Name { public string text { get; set; } = ""; public int start { get; set; } public int end { get; set; } }
    sealed class Decl { public string kind { get; set; } = ""; public int start { get; set; } public int end { get; set; } public List<Name> names { get; set; } = new(); }
    sealed class Diag { public string id { get; set; } = ""; public int start { get; set; } public int end { get; set; } }
    sealed class VersionObs
    {
        public string version { get; set; } = "";
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public bool? accepted { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public List<Diag>? diagnostics { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public string? oracle_error { get; set; }
    }
    sealed class SourceObs
    {
        public string id { get; set; } = "";
        public string sha256 { get; set; } = "";
        public int bytes { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public string? oracle_error { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public bool? accepted { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public string[]? features { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public List<object>? directives { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public List<object>? disabled { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public string[]? errors { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public object[]? diagnostics { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public List<Decl>? declarations { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public List<int[]>? nodes { get; set; }
        [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public List<VersionObs>? versions { get; set; }
    }

    static string Hex(byte[] data) => Convert.ToHexString(SHA256.HashData(data)).ToLowerInvariant();

    // The syntax nodes that declare something; the names are the tokens (or the namespace name node) that name it.
    static bool IsDeclaration(SyntaxNode n) =>
        n is MemberDeclarationSyntax or BaseTypeDeclarationSyntax or DelegateDeclarationSyntax or BaseNamespaceDeclarationSyntax
        or LocalFunctionStatementSyntax or LocalDeclarationStatementSyntax or AccessorDeclarationSyntax or ParameterSyntax
        or TypeParameterSyntax or UsingDirectiveSyntax or ExternAliasDirectiveSyntax
        || n is AttributeListSyntax { Parent: CompilationUnitSyntax };

    static IEnumerable<TextSpan> T(SyntaxToken t) => t.IsKind(SyntaxKind.None) || t.IsMissing ? Array.Empty<TextSpan>() : new[] { t.Span };

    static IEnumerable<TextSpan> Names(SyntaxNode n) => n switch
    {
        BaseTypeDeclarationSyntax x => T(x.Identifier),
        DelegateDeclarationSyntax x => T(x.Identifier),
        BaseNamespaceDeclarationSyntax x => x.Name.IsMissing ? Array.Empty<TextSpan>() : new[] { x.Name.Span },
        MethodDeclarationSyntax x => T(x.Identifier),
        ConstructorDeclarationSyntax x => T(x.Identifier),
        DestructorDeclarationSyntax x => T(x.Identifier),
        PropertyDeclarationSyntax x => T(x.Identifier),
        EventDeclarationSyntax x => T(x.Identifier),
        EnumMemberDeclarationSyntax x => T(x.Identifier),
        BaseFieldDeclarationSyntax x => x.Declaration.Variables.SelectMany(v => T(v.Identifier)),
        IndexerDeclarationSyntax x => T(x.ThisKeyword),
        OperatorDeclarationSyntax x => T(x.OperatorToken),
        ConversionOperatorDeclarationSyntax x => T(x.ImplicitOrExplicitKeyword),
        LocalFunctionStatementSyntax x => T(x.Identifier),
        LocalDeclarationStatementSyntax x => x.Declaration.Variables.SelectMany(v => T(v.Identifier)),
        AccessorDeclarationSyntax x => T(x.Keyword),
        ParameterSyntax x => x.Identifier.IsKind(SyntaxKind.None) ? Array.Empty<TextSpan>() : T(x.Identifier),
        TypeParameterSyntax x => T(x.Identifier),
        UsingDirectiveSyntax x => x.Alias is null ? Array.Empty<TextSpan>() : T(x.Alias.Name.Identifier),
        ExternAliasDirectiveSyntax x => T(x.Identifier),
        _ => Array.Empty<TextSpan>(),
    };

    // a Roslyn failure to enumerate diagnostics (observed in 5.9) is no observation
    static Diagnostic[]? Errors(SyntaxTree tree, out string? failure)
    {
        failure = null;
        try { return tree.GetDiagnostics().Where(x => x.Severity == DiagnosticSeverity.Error).ToArray(); }
        catch (ArgumentOutOfRangeException e) { failure = "GetDiagnostics " + e.GetType().Name; return null; }
    }

    public static int Main(string[] args)
    {
        if (args.Length != 2 && args.Length != 3) throw new Exception("usage: Observe <input.json> <out.json> [<ledger.json>]");
        byte[] input = File.ReadAllBytes(args[0]);
        if (input.Length > InputLimit) throw new Exception("input bound");
        var utf8 = new UTF8Encoding(false, true);
        var producer = typeof(Observe).Assembly;
        string meta(string key) => producer.GetCustomAttributes<AssemblyMetadataAttribute>().Single(x => x.Key == key).Value!;
        string sdk = meta("TSGK.SDK");
        string roslynDir = Path.GetFullPath(meta("TSGK.RoslynDir"));
        if (string.IsNullOrWhiteSpace(sdk)) throw new Exception("missing build SDK identity");

        // LanguageVersion: the whole enum, the versions observed (C# 7.3 and later by name) and what Latest means
        var enumNames = Enum.GetValues<LanguageVersion>().Select(v => (name: v.ToString(), value: (int)v)).Distinct().OrderBy(x => x.value).ThenBy(x => x.name, StringComparer.Ordinal).ToArray();
        var observed = enumNames.Where(x => Regex.IsMatch(x.name, @"^CSharp\d+(_\d+)?$") && x.value >= MinVersion).ToArray();
        int effective = (int)LanguageVersion.Latest.MapSpecifiedToEffectiveVersion();
        string effectiveName = observed.First(x => x.value == effective).name;
        var versions = observed.Select(x => (x.name, version: (LanguageVersion)x.value)).ToArray();

        using var doc = JsonDocument.Parse(input);
        var sources = new List<SourceObs>();
        var ids = new HashSet<string>();
        var kinds = new SortedSet<string>(StringComparer.Ordinal);
        var raw = new List<(SourceObs obs, List<(string kind, int start, int end)> nodes)>();
        foreach (var item in doc.RootElement.GetProperty("cases").EnumerateArray())
        {
            string id = item.GetProperty("id").GetString()!;
            if (!ids.Add(id)) throw new Exception("duplicate id " + id);
            byte[] bytes = Convert.FromBase64String(item.GetProperty("source").GetString()!);
            string text = utf8.GetString(bytes);
            var symbols = item.GetProperty("symbols").EnumerateArray().Select(s => s.GetString()!).ToArray();
            var features = item.TryGetProperty("features", out var f) ? f.EnumerateArray().Select(x => new KeyValuePair<string, string>(x.GetString()!, "true")).ToArray() : Array.Empty<KeyValuePair<string, string>>();
            CSharpParseOptions Options(LanguageVersion v) => new CSharpParseOptions(v, DocumentationMode.Parse, SourceCodeKind.Regular, symbols).WithFeatures(features);
            var obs = new SourceObs { id = id, sha256 = Hex(bytes), bytes = bytes.Length };
            sources.Add(obs);

            // char index -> UTF-8 byte offset, built once (a surrogate pair is counted at its high half)
            var prefix = new int[text.Length + 1];
            for (int i = 0; i < text.Length; i++)
                prefix[i + 1] = prefix[i] + (text[i] < 0x80 ? 1 : text[i] < 0x800 ? 2 : char.IsHighSurrogate(text[i]) ? 4 : char.IsLowSurrogate(text[i]) ? 0 : 3);
            if (prefix[text.Length] != bytes.Length) throw new Exception("offset table " + id);
            int Byte(int charPos) => prefix[charPos];
            // a diagnostic may point between the halves of a surrogate pair: report the pair's start
            int DiagByte(int charPos) => Byte(charPos > 0 && char.IsHighSurrogate(text[charPos - 1]) ? charPos - 1 : charPos);

            var tree = CSharpSyntaxTree.ParseText(text, Options(LanguageVersion.Latest));
            var root = (CompilationUnitSyntax)tree.GetRoot();
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
                bool? taken = d switch { BranchingDirectiveTriviaSyntax b => b.BranchTaken, _ => null };
                string kind = d.Kind().ToString();
                foreach (var x in d.GetDiagnostics()) context.TryAdd((x.Id, x.Location.SourceSpan), kind[..^"DirectiveTrivia".Length]);
                // the physical lines of the directive by Roslyn's own line breaks, last line break included
                var lines = tree.GetText().Lines;
                var line = lines.GetLineFromPosition(d.HashToken.Span.Start);
                var last = lines.GetLineFromPosition(Math.Max(d.FullSpan.End - 1, d.FullSpan.Start));
                directives.Add(new { kind, start = Byte(d.HashToken.Span.Start), active = d.IsActive, taken, line_start = Byte(line.Start), line_end = Byte(last.EndIncludingLineBreak) });
            }
            // the end-of-file token also carries the errors of directives in its leading trivia
            foreach (var x in root.EndOfFileToken.GetDiagnostics()) context.TryAdd((x.Id, x.Location.SourceSpan), "EndOfFile");
            var errors = Errors(tree, out var failure);
            if (errors is null) { obs.oracle_error = failure; continue; }
            obs.features = Options(LanguageVersion.Latest).Features.Keys.OrderBy(k => k, StringComparer.Ordinal).ToArray();
            obs.directives = directives;
            obs.disabled = disabled;
            obs.errors = errors.Select(x => x.Id).Distinct().OrderBy(x => x, StringComparer.Ordinal).ToArray();
            obs.diagnostics = errors.Select(x => (object)new { id = x.Id, start = DiagByte(x.Location.SourceSpan.Start), end = DiagByte(x.Location.SourceSpan.End), context = context.GetValueOrDefault((x.Id, x.Location.SourceSpan), "Other") }).ToArray();
            obs.accepted = errors.Length == 0;

            // declarations and syntax nodes (not tokens or trivia), in preorder
            obs.declarations = new List<Decl>();
            var nodes = new List<(string, int, int)>();
            foreach (var n in root.DescendantNodesAndSelf())
            {
                if (nodes.Count >= NodeLimit) throw new Exception("node bound " + id);
                string kind = n.Kind().ToString();
                int start = Byte(n.Span.Start), end = Byte(n.Span.End);
                nodes.Add((kind, start, end));
                kinds.Add(kind);
                if (!IsDeclaration(n)) continue;
                var decl = new Decl { kind = kind, start = start, end = end };
                foreach (var s in Names(n)) decl.names.Add(new Name { text = text.Substring(s.Start, s.Length), start = Byte(s.Start), end = Byte(s.End) });
                obs.declarations.Add(decl);
            }
            raw.Add((obs, nodes));

            // acceptance under each observed LanguageVersion
            obs.versions = new List<VersionObs>();
            foreach (var (name, version) in versions)
            {
                var vt = CSharpSyntaxTree.ParseText(text, Options(version));
                var ve = Errors(vt, out var vfail);
                obs.versions.Add(ve is null
                    ? new VersionObs { version = name, oracle_error = vfail }
                    : new VersionObs { version = name, accepted = ve.Length == 0, diagnostics = ve.Select(x => new Diag { id = x.Id, start = DiagByte(x.Location.SourceSpan.Start), end = DiagByte(x.Location.SourceSpan.End) }).ToList() });
            }
        }

        var kindTable = kinds.ToList();
        var kindIndex = kindTable.Select((k, i) => (k, i)).ToDictionary(x => x.k, x => x.i, StringComparer.Ordinal);
        foreach (var (obs, nodes) in raw) obs.nodes = nodes.Select(x => new[] { kindIndex[x.kind], x.start, x.end }).ToList();

        // the loaded Roslyn assemblies: the file of the SDK and the copy this tool loaded must be the same bytes
        var assemblies = AppDomain.CurrentDomain.GetAssemblies().Where(a => a.GetName().Name!.StartsWith("Microsoft.CodeAnalysis", StringComparison.Ordinal)).OrderBy(a => a.GetName().Name, StringComparer.Ordinal).Select(a =>
        {
            string name = a.GetName().Name!;
            byte[] sdkBytes = File.ReadAllBytes(Path.Combine(roslynDir, name + ".dll"));
            if (!sdkBytes.AsSpan().SequenceEqual(File.ReadAllBytes(a.Location))) throw new Exception("loaded assembly differs from the SDK file " + name);
            string rel = Path.GetFullPath(Path.Combine(roslynDir, name + ".dll")).Replace('\\', '/');
            int at = rel.IndexOf("/sdk/" + sdk + "/", StringComparison.Ordinal);
            if (at < 0) throw new Exception("assembly outside the SDK " + name);
            return new { name, path = rel[(at + 1)..], bytes = sdkBytes.Length, sha256 = Hex(sdkBytes), assembly_version = a.GetName().Version!.ToString(), product_version = System.Diagnostics.FileVersionInfo.GetVersionInfo(a.Location).ProductVersion ?? "" };
        }).ToArray();
        var provenance = new
        {
            sdk,
            runtime = new { framework = RuntimeInformation.FrameworkDescription, version = Environment.Version.ToString() },
            tool_sha256 = Hex(File.ReadAllBytes(producer.Location)),
            assemblies,
            language_versions = new
            {
                enumeration = enumNames.Select(x => new { x.name, x.value }).ToArray(),
                observed = observed.Select(x => x.name).ToArray(),
                latest = new { effective = effectiveName, value = effective },
            },
            parse = new { documentation_mode = "Parse", source_code_kind = "Regular" },
        };

        var write = new JsonSerializerOptions();
        var sb = new StringBuilder();
        sb.Append("{\n");
        sb.Append(" \"schema\": \"tsgk-csharp-g1-roslyn-observed/r1\",\n");
        if (args.Length == 3)
        {
            byte[] ledger = File.ReadAllBytes(args[2]);
            sb.Append(" \"ledger\": ").Append(JsonSerializer.Serialize(new { path = LedgerPath, bytes = ledger.Length, sha256 = Hex(ledger) }, write)).Append(",\n");
        }
        sb.Append(" \"input\": ").Append(JsonSerializer.Serialize(new { bytes = input.Length, sha256 = Hex(input) }, write)).Append(",\n");
        sb.Append(" \"provenance\": ").Append(JsonSerializer.Serialize(provenance, write)).Append(",\n");
        sb.Append(" \"kinds\": ").Append(JsonSerializer.Serialize(kindTable, write)).Append(",\n");
        sb.Append(" \"sources\": [\n");
        for (int i = 0; i < sources.Count; i++)
            sb.Append("  ").Append(JsonSerializer.Serialize(sources[i], write)).Append(i < sources.Count - 1 ? ",\n" : "\n");
        sb.Append(" ]\n}\n");
        byte[] output = utf8.GetBytes(sb.ToString());
        if (output.Length > OutputLimit) throw new Exception("output bound");
        File.WriteAllBytes(args[1], output);
        return 0;
    }
}
