# Campaign 01 언어·feature 범위

필수 집합은 아래 26개 route와 Windows amd64/Linux amd64/macOS arm64다. 정확한 집합·세션 매핑은 [기계 정의](../../src/contracts/campaign-01.json), 후보 source 관측은 [source 등록부](../../src/contracts/language-sources.json), 실행 gate는 [workload](workload-matrix.md)가 소유한다. 이 등록은 제품·grammar 지원 성공 기록이 아니다.

## 버전과 기준일

기준일은 2026-09-29다. 아래 하한·상한·모드는 공식 자료 대조 후 현재 PREPARE에서 사용자가 **표의 26개 version/mode 범위 채택**으로 승인했다. feature 전수 disposition과 grammar 지원 성공은 별도 gate다. 이후 release를 이 날짜의 stable로 소급하지 않는다. SDK 설치 버전은 언어 하한·상한을 결정하지 않는다.

| Route | 채택 legacy 하한 → stable 상한 | 명시 모드·확장과 공식 근거 |
|---|---|---|
| `csharp` | C# 7.3 / net461 source → C# 14 / net10.0 | 일반 `.cs`, explicit LangVersion, ASP.NET Core 10 `.cs` 사용 사례; [언어 변경](https://learn.microsoft.com/en-us/dotnet/csharp/whats-new/csharp-14) |
| `go` | 1.17 → 1.27 | 일반 `.go`, Go syntax와 compiler/runtime 구분; [1.27](https://go.dev/doc/go1.27) |
| `python` | 3.8 → 3.14 | module/script, async; [3.14](https://docs.python.org/3.14/whatsnew/3.14.html) |
| `javascript` | ES5.1 → ECMAScript 2026 | script/module, Annex B 여부를 case별 표시; [ECMA-262 17판](https://ecma-international.org/publications-and-standards/standards/ecma-262/) |
| `jsx` | classic JSX → 고정 JSX 문법 | JS/JSX를 별도 행으로 검증; [JSX specification](https://github.com/react/jsx/tree/d614ce76e6ea996ea6dfa122f2a7be71ed96e6eb) |
| `typescript` | 2.8 → 7.0 | `.ts`, declaration, JSX 제외; [7.0](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/) |
| `tsx` | TS2.8 + JSX → TS7.0 + 고정 JSX | `.tsx`, type parameter/JSX ambiguity; 위 TS/JSX 근거 |
| `java` | Java SE 8 → 27 | 정식 source, preview 제외; [버전별 언어 변경](https://docs.oracle.com/en/java/javase/27/language/java-language-changes-summary.html) |
| `kotlin` | 1.3 → 2.4 language line | `.kt`/`.kts`, 안정 기능, platform SDK 제외; [release](https://kotlinlang.org/docs/releases.html) |
| `c` | C99 → C23 | translation unit/preprocessor, GNU/MS 확장 별도; [ISO/IEC 9899:2024](https://committee.iso.org/standard/82075.html) |
| `cpp` | C++11 → C++23 | translation unit/preprocessor, compiler 확장 별도; [ISO/IEC 14882:2024](https://committee.iso.org/standard/83626.html) |
| `rust` | Edition 2015 → Edition 2024 / rustc1.98.1 stable syntax | edition별 keyword/macro 경계, nightly 제외; [release](https://blog.rust-lang.org/2026/09/03/Rust-1.98.1/) |
| `swift` | 5.0 → 6.4 | 정식 source와 conditional compilation; [6.4](https://www.swift.org/blog/swift-6.4-released/) |
| `dart` | 2.12 → 3.13 | null-safe source, legacy/new constructor 구문; [language evolution](https://dart.dev/resources/language/evolution) |
| `php` | 5.6 → 8.5 | PHP와 mixed HTML mode 구분; [8.5 migration](https://www.php.net/manual/en/migration85.php) |
| `ruby` | 2.7 → 4.0 | 일반 source, ERB 제외; [4.0](https://www.ruby-lang.org/en/news/2025/12/25/ruby-4-0-0-released/) |
| `r` | 3.6 → 4.6 | R source, Rscript 별도 route 아님; [R](https://www.r-project.org/) |
| `bash` | 3.2 → 5.3 | Bash mode, POSIX mode 별도 표시, shell 실행 제외; [5.3 manual](https://www.gnu.org/s/bash/manual/html_node/index.html) |
| `powershell` | Windows PowerShell 5.1 syntax → PowerShell 7.6 syntax | source 구문만, 실제 프로젝트 실행 host는 pwsh7; [lifecycle](https://learn.microsoft.com/en-us/powershell/scripting/install/powershell-support-lifecycle?view=powershell-7.6) |
| `html` | HTML5 REC 2014 → Living Standard 2026-09-28 | document/명시 fragment/raw-text 경계, DOM/rendering 제외; [고정 spec](https://github.com/whatwg/html/tree/b346db728f365e89bca1fafc4789acf6296571d4) |
| `css` | CSS2.1 → Syntax Level3 + Snapshot2026 | stylesheet/declaration, Snapshot §2.1/2.2의 syntax; [2026-06-22 snapshot](https://www.w3.org/TR/2026/NOTE-css-2026-20260622/) |
| `json` | RFC8259 / ECMA-404 2판 | strict JSON, JSONC/JSON5 제외; [RFC8259](https://www.rfc-editor.org/rfc/rfc8259) |
| `yaml` | YAML1.1 → YAML1.2 revision1.2.2 | version directive/flow/block, value resolution과 분리; [1.2.2](https://yaml.org/spec/1.2.2/) |
| `xml` | XML1.0 5판 → XML1.0 5판 + XML1.1 2판 | 두 version 구분, namespace/doctype 구문, 외부 entity 해결 제외; [1.0](https://www.w3.org/TR/xml/), [1.1](https://www.w3.org/TR/xml11/) |
| `tsql` | SQL Server2012/compat110 → SQL Server2025/compat170 | T-SQL script/quoted identifier; GO는 client batch separator로 표시; [2025](https://learn.microsoft.com/en-us/sql/sql-server/what-s-new-in-sql-server-2025?view=sql-server-ver17) |
| `postgresql-sql` | PostgreSQL9.6 → 18 | SQL dialect, PL/pgSQL/extension language 제외; [정식 release 목록](https://www.postgresql.org/docs/release/) |

PostgreSQL19는 기준일에 beta4로 확인되어 정식 상한에 넣지 않았다. C# net461과 C#7.3 조합은 legacy source 요구이며 해당 framework에서 현대 기능의 build/runtime 지원을 주장하지 않는다. ASP.NET Core 서버·DI/HTTP·NuGet·Razor `.razor/.cshtml`은 포함하지 않는다. CSS snapshot은 단일 언어 version이 아니므로 선택 module의 revision과 syntax disposition을 개별 등록해야 한다.

## Feature inventory의 완료 의미

route마다 legacy 기본 syntax와 상한까지 추가·변경·제거된 syntax, dialect/mode boundary를 열거한다. 공식 사양 장과 version별 변경 목록의 각 항목을 `REQUIRED_SYNTAX`, `SEMANTIC_ONLY`, `LIBRARY_RUNTIME_ONLY`, `EXCLUDED_EXTENSION`, `UNRESOLVED`로 처분한다. 제외에는 근거·이유를 붙이며 upstream 미지원은 syntax 제외 사유가 아니다. 하한/상한 snippet 두 개나 grammar README의 지원 문구는 이 inventory를 대신하지 못한다.

각 feature는 stable ID, route/version/mode, spec revision/section, 구문 사실, candidate node/field mapping과 구조 assertion의 목표, required case kind, 기대값 검토, 보조 parser 필요 여부, 담당 Session/기한을 가진다. scope와 필수 case 종류는 S01 전에 채택한다. fixture bytes/완전한 expected tree·실행 receipt는 담당 단계까지 완성하며 지금 생성하지 않는다. 공식 구문 사실과 parser가 출력한 node 이름/전체 AST의 동일성을 혼동하지 않는다.

검사군은 inventory/identity(S01), strict artifact selection(S02), static schema(S03), reproduction(S04), feature/recovery/edit(S05), query/API(S06), evidence/replay(S07), 현재 후보의 전체 실행·실사용·cross-OS(S08)다. N/A는 실행 전 근거로 등록하며 필수 실패 뒤 분류를 바꾸지 않는다. 모든 feature에 임의로 같은 fixture 개수를 요구하지 않는다.

실제 feature 등록과 전수 처분의 완료 여부는 [준비 보고서](../reports/campaign-01-2026-09-29-preparation.md)에 기록한다. 숫자 상한 대조만 끝난 상태는 feature scope 완료가 아니다. 미확인 항목·미승인 범위는 P04를 막고 master는 시작하지 않는다.

## Source feasibility와 네 역할

등록된 후보 commit·grammar subdir·scanner/shared source·generated artifact 존재와 실제 source closure 검토를 구분한다. repository SHA나 file list만으로 실행된 dependency bytes가 완전하다고 주장하지 않는다. 실행 전에 실제 source manifest·license 관측·generator/runtime compatibility를 채운다. SQL 두 route는 각각 dialect 근거가 필요하며 generic `sql` 하나를 이름만 바꿔 대체하지 않는다.

네 역할은 `mainstream`(26 route 전수), `maintained`(선택한 immutable BrightScript), `historical`(등록한 결함을 검출하는 bounded audit), `owned`(scannerless/stateful와 malformed/mutant controls)다. [Provenance](../provenance/upstream-sources.md)의 과거 release·실패 근거를 보존한다. maintained/historical/owned 결과는 필수 mainstream 행의 대체가 아니다.

kit detector가 정확히 실패를 보고했어도 grammar의 required 지원 격차는 남는다. 해결되지 않은 필수 외부 격차는 무조건적인 implementation readiness를 막는다. 다른 repo 수정·새 grammar 채택·목표 축소는 별도 구체 결정이 필요하며 이 등록으로 자동 승인하지 않는다.
