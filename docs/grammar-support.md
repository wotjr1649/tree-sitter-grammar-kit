# Grammar 지원과 검증 범위 안내

[한국어](grammar-support.md) · [English](grammar-support.en.md)

이 문서는 grammar-kit의 목적과 설계 이유, 등록된 grammar의 버전 범위와 테스트 의미를 설명하는 사용자 안내다. 새 지원 계약을 만들지 않는다. 제품 책임은 [scope](specs/scope.md), 구조는 [architecture](design/architecture.md), 지원 판정은 [platform](specs/platform-support.md), 구문 요구는 [feature scope](validation/language-feature-scope.md)와 [disposition](validation/language-feature-disposition.md)이 소유한다.

## 목적과 설계 이유

`tree-sitter-grammar-kit`은 Tree-sitter grammar를 도입·업데이트할 때 파일·생성물·파싱 구조·query·runtime API와 그 검증 근거를 검사·비교하는 Go CLI(`tsgk`)와 작은 공개 offline Go API(`src/kit`)다. 첫 사용자는 `go-treesitter`의 grammar 도입·업데이트 담당자다. 소비자의 Go 변환·scanner 이식·runtime 통합·최종 채택은 별도 책임이며 일반 application 파싱마다 kit 실행을 요구하지 않는다.

컴파일 성공이나 `ERROR` 없는 tree만으로는 업데이트의 안전성을 판단할 수 없다. 선언의 field·원본 범위가 달라지면 소비자 분석이 깨질 수 있고, incremental parsing만 fresh parsing과 다른 결과를 낼 수도 있다. 잘못된 구문을 정상으로 받는 과잉 수용도 별도 negative case가 필요하다.

CLI와 API는 같은 offline 코어를 사용한다. 코어는 `CGO_ENABLED=0`으로 검사·비교하며, generator와 native C driver는 별도 process에서 runner의 시간·출력·메모리·cleanup 검사를 받는다. 파일 검사에 native toolchain을 요구하지 않으면서 실행형 검사의 실패 경계를 분리하기 위한 설계다. runner는 임의 악성 native 코드의 완전한 sandbox가 아니다.

| 검증 단계 | 기능 | 확인하는 것 |
|---|---|---|
| 입력과 파일 집합 | `inspect`, `identity`, `corpus`, `verify` | 무엇을 검사했고 신뢰된 기준과 어떤 bytes·파일이 다른가 |
| 정적 구조 | `schema check`, `schema diff` | node·field·참조 계약의 변경과 검토 위험 |
| 생성물 재현 | `reproduce` | 고정 generator로 독립 작업 공간의 결과가 일치하는가 |
| native 동작 | `incremental`, `oracle record` | tree·edit/fresh·capture·사실·API 관측이 등록 기대와 일치하는가 |
| 근거와 종합 판정 | `replay`, `evidence verify`, `qualify` | 근거가 완전하고 identity가 맞으며 요구를 충족하는가 |

`verify`의 PASS는 선택 집합이 caller-trusted expected와 같다는 뜻이다. fingerprint는 출처 인증이 아니며 `schema diff`의 차이는 자동 결함·SemVer 판정이 아니다. Ordered CST는 부모·순서·field·flag·원본 byte/point 범위를, capture는 순서와 중복을 보존한다. 비교 전에 정렬·중복 제거·범위 보정으로 차이를 숨기지 않는다.

source·도구·입력·정책·comparator·실행 identity를 연결하고 `execution_status`, `evidence_mode`, `assessment`를 분리한다. 실행 완료가 요구 PASS를 뜻하지 않고 replay도 새 native 실행이 아니다. Qualification의 `mechanism`은 kit 검사 장치, `requirement`는 grammar의 구문 요구를 판정한다. Grammar 실패를 정확히 검출한 kit의 성공으로 grammar 지원 실패를 덮지 않는다. 상세 규칙은 [identity/evidence](specs/identity-and-evidence.md)와 [tree/protocol](specs/tree-and-adapter-protocol.md)에 있다.

## 지원 상태와 버전의 의미

이 안내의 분석 기준은 main `679d7dd`(2026-10-12)다. 채택 언어 범위의 기준일은 `2026-09-29`이며 아래 상한을 현재 외부 세계의 최신 release라고 표현하지 않는다. [플랫폼 계약](specs/platform-support.md)과 [#188 보고서](reports/issue-188-corpus-cst-api.md)에 기록된 qualification은 Windows amd64·Linux amd64·macOS arm64의 26 route × 3 = 78칸과 2,478개 의무(플랫폼당 826개)가 PASS다. 이 문서 작성 자체는 native 재실행이 아니다.

Offline `inspect`, `identity`, `verify`, `schema`와 해당 공개 API는 VERIFIED다. `corpus`, `reproduce`, `incremental`, `oracle record`, `replay`, `evidence verify`, `qualify`는 experimental이며 호환성을 약속하지 않는다. 26 route의 SUPPORTED는 등록 후보·사례에 대한 판정이지 임의 입력의 완전 적합성 인증이 아니다.

언어 버전은 검사할 구문 범위이고 grammar revision은 parser source의 identity다. 테스트는 [qualification inventory](../src/contracts/qualification-c1.json)의 사례와 각 버전의 구문 요구를 연결한다. 모든 compiler·SDK·interpreter 버전을 설치·실행한 시험이나 모든 minor/patch release별 시험이 아니다. 기본 구문과 변경 계열을 함께 검사하며, 표의 묶음 사이 숫자가 없다고 그 버전 전체를 미지원으로 해석하지 않는다.

## 필수 26개 route

아래 표는 [feature scope](validation/language-feature-scope.md), [disposition](validation/language-feature-disposition.md), [source registry](../src/contracts/language-sources.json)의 요약이다. 테스트 열은 등록된 구문 계열이며 개별 실행 도구의 버전 목록이 아니다. 모든 route의 지원 판정은 앞 절의 등록 사례·세 플랫폼 범위에 한정한다.

| 문법 / route | 채택 구문 범위 | 버전별 테스트 등록 계열(기본 구문 포함) | 기반 grammar commit(12자리) |
|---|---|---|---|
| C# / `csharp` | 7.3 / net461 source → 14 / net10.0 source | 7.3·8·9·10·11·12·13·14, 14 file-based directive | `9150f7d56bb4` |
| Go / `go` | 1.17 → 1.27 | 1.18~1.27 변경 계열, 1.27 항목 | `2346a3ab1bb3` |
| Python / `python` | 3.8 → 3.14 | 3.8·3.9·3.10·3.11·3.12·3.13·3.14 | `26855eabccb1` |
| JavaScript / `javascript` | ES5.1 → ECMAScript 2026 | ES2015, 2016~2019, 2020~2021, 2022~2025 계열, script/Annex B | `58404d8cf191` |
| JSX / `jsx` | classic JSX → 고정 JSX 사양 | element·attribute·expression 등 채택 구문; 숫자 버전 없이 사양 revision 고정 | `58404d8cf191` |
| TypeScript / `typescript` | 2.8 → 7.0 | 2.8~2.9, 3.0~3.9, 4.0~4.4, 4.5~4.9, 5.0~5.4, 5.5~7.0 | `75b3874edb2d` |
| TSX / `tsx` | TS 2.8 + JSX → TS 7.0 + JSX | 기본 TSX, 2.9~7.0, type parameter/JSX 경계 | `75b3874edb2d` |
| Java / `java` | Java SE 8 → 27 | 9~11, 14~17, 21~22, 25~27 변경 계열 | `e10607b45ff7` |
| Kotlin / `kotlin` | 1.3 → 2.4 language line | 1.4~1.9, 2.0~2.3, 2.4 정식 구문 계열 | `3dea6dfa9c01` |
| C / `c` | C99 → C23 | C11/C17, C23·preprocessor, 채택 legacy 구문 | `b780e47fc780` |
| C++ / `cpp` | C++11 → C++23 | C++14·17·20·23, 채택 legacy 구문 | `c00922280863` |
| Rust / `rust` | Edition 2015 → 2024 / rustc 1.98.1까지의 stable syntax | 2018 이후 변경 계열, 2024/1.98.1까지, edition 경계 | `77a3747266f4` |
| Swift / `swift` | 5.0 → 6.4 | 5.0~5.4, 5.5~5.8, 5.9~6.1, 6.1·6.2, 6.3~6.4 | `35245fbfee2f` |
| Dart / `dart` | 2.12 → 3.13 | 2.13~2.19, 3.0~3.3, 3.6~3.9, 3.10~3.13, legacy | `be07cf7118d3` |
| PHP / `php` | 5.6 → 8.5 | 5.6~7.1, 7.2~7.4, 8.0~8.1, 8.2~8.3, 8.4~8.5, legacy | `92b5271b60be` |
| Ruby / `ruby` | 2.7 → 4.0 | 2.7~3.0, 3.1~3.4, 4.0 | `ad907a69da0c` |
| R / `r` | 3.6 → 4.6 | 4.0~4.1, 4.2~4.6 | `58a22794466c` |
| Bash / `bash` | 3.2 → 5.3 | 4.x~5.2, 5.3 | `a06c2e4415e9` |
| PowerShell / `powershell` | Windows PowerShell 5.1 syntax → PowerShell 7.6 syntax | 5.1 legacy, 6.0~7.6 | `e7bd348c49fd` |
| HTML / `html` | HTML5 REC 2014 → Living Standard 2026-09-28 | document·fragment·raw-text 등 채택 구문; 사양 revision 고정 | `73a3947324f6` |
| CSS / `css` | CSS2.1 → Syntax Level3 + Snapshot2026 채택 module | 기본 구문과 30개 module 행(selectors·cascade·color·fonts·flexbox·grid 등) | `dda5cfc5722c` |
| JSON / `json` | RFC8259 / ECMA-404 2판 | strict JSON·negative·구조 | `254c42a64764` |
| YAML / `yaml` | 1.1 → 1.2.2 | directive·flow·block, 1.1/1.2 구문 경계 | `a1c4812a73ec` |
| XML / `xml` | 1.0 5판 + 1.1 2판 | namespace·DOCTYPE, 1.0/1.1 경계 | `5000ae8f22d1` |
| T-SQL / `tsql` | SQL Server2012/compat110 → SQL Server2025/compat170 구문 | 기본 구문, 2016~2019, 2022~2025, batch·문맥 경계 | `8620fbcfca94` |
| PostgreSQL SQL / `postgresql-sql` | 9.6 → 18 | 10~12, 13~15, 16~17, 18, legacy | `59d0d8cd7506` |

Commit 열은 기반 upstream revision이다. 실제 후보는 해당 route의 patch chain·재생성물·runtime patch까지 포함하므로 commit만으로 같은 parser를 재현했다고 주장하지 않는다. JSX는 JavaScript grammar를 사용하고 TypeScript/TSX는 같은 repository의 `typescript`/`tsx` 디렉터리를 사용한다. 실제 closure는 [native routes](../src/contracts/native-routes.json)와 [runtime manifest](../src/drivers/native-c/runtime-manifest.json)가 고정한다. 등록 native 입력은 Tree-sitter CLI 0.27.0/ABI15로 재생성한다.

여러 버전의 구문 합집합은 버전 선택형 compiler와 다르다. C# 행의 net461/net10.0은 source profile이며 .NET runtime 실행을 보증하지 않는다. T-SQL이 채택한 2012~2025 구문을 받는다고 모든 구문이 compatibility110에서 유효하다는 뜻은 아니다. 같은 bytes의 해석이 달라지는 C++ `module`/`import`·comma subscript와 PHP legacy tag·`#[` 항목은 [version conflict 처분](validation/language-feature-disposition.md)에 따라 일부 legacy 해석을 제외한다.

T-SQL에는 별도의 SQL Server2025 Express LocalDB17.0.1000.7 엔진 대조가 있다. Compatibility110·170으로 실행한 검사는 실제 SQL Server2012 engine 실행이나 전체 애플리케이션 실행 호환성의 대체 근거가 아니다. [엔진 대조 보고서](reports/issue-126-expanded-validation.md)가 실제 실행 범위와 미실행을 소유한다.

## 추가 형식과 참조의 상태

| 대상 | 상태 | 범위·이유 |
|---|---|---|
| WCF `.svc` | 등록 사례 세 플랫폼 PASS | `SVC-SERVICEHOST-r2` directive·inline C#·복구·편집·등록 참조; C#을 쓰는 복합 형식이며 새 언어 route가 아님 |
| NET461 대용량 C# workload | Windows 실행·검증 | Windows 업무 profile; Linux·macOS NOT_APPLICABLE |
| `.csproj`·`.resx`·`.config`·`.wsdl`·`.xsd` 등 | XML source 구문 관측 | MSBuild·resource·XSD/WSDL의 고유 의미와 실행을 보증하지 않음 |
| BrightScript grammar v0.1.4 | maintained reference, NOT_RUN | campaign에 native route/workload 미등록 |
| BrightScript grammar v0.1.2 | historical replay 대상, NOT_RUN | verification bundle 미준비; NEW_RUN 근거로 사용 불가 |
| Cooklang | static metadata audit, NOT_RUN | S00 정적 metadata 관측만 있고 native 미실행 |
| 자체 scannerless/stateful fixture | 별도 native 시험 | kit 장치와 scanner 검증용; 사용자 언어 지원 목록이 아님 |

BrightScript의 v0.1.x는 언어 version이 아니라 grammar 프로젝트 release다. 추가 역할의 상태는 [qualification inventory](../src/contracts/qualification-c1.json), 업무 형식은 [NET461 workload](validation/net461-workload.md), 참조 revision은 [provenance](provenance/upstream-sources.md)가 소유한다. XML로 읽는다는 이유만으로 XSD/XSLT 전용 언어 지원을 주장하지 않는다.

## 다른 문법과 확장을 지원 범위에 넣지 않은 이유

현재 campaign은 26 route로 범위를 고정했다. Lua·Elixir·Scala·Haskell·Zig·OCaml·Julia·Objective-C·MySQL·SQLite·Oracle SQL 등은 필수 route가 아니다. 이들 각각의 기술적 배제 사유는 등록되어 있지 않으며, 미등록을 구현 불가능으로 해석하지 않는다. 추가 지원에는 source closure·사양/버전 범위·기대값·positive/negative/recovery/edit/query 사례와 플랫폼별 근거가 필요하다.

아래는 [disposition](validation/language-feature-disposition.md)의 대표적인 제외다. 전체 항목은 각 route의 EXT 행과 version conflict 처분을 따른다. 후보 grammar가 미지원이라는 사실 자체는 채택한 필수 구문을 제외할 이유가 아니다.

| 제외 대상 | 이유 |
|---|---|
| Razor·별도 `.csx` host·Vue/Svelte template·ERB·Blade | 일반 source와 다른 복합 형식·host·template 언어 |
| JSONC/JSON5·Sass/Less·Helm/Jinja YAML | strict JSON·채택 CSS module·YAML과 다른 확장 문법 |
| zsh/fish/ksh 고유 구문 | Bash와 다른 shell 문법 |
| PL/pgSQL·psql backslash command | PostgreSQL SQL과 별도 언어·client 문법 |
| Azure/Fabric/Synapse 전용 SQL | 채택 SQL Server T-SQL 범위 밖 |
| Python2·notebook magic·pre-2.12 Dart | 채택 version/mode 밖 |
| GNU/MS C/C++ 확장·C++26 draft | 채택 표준과 정식 version 범위 밖 |
| Java preview·Rust nightly·Kotlin experimental 등 | 채택 stable syntax 밖 |
| XQuery·DTD-only route·XSD/XSLT 고유 의미 | XML source 구문과 별도 언어·검증 책임 |

SQLCMD `:r`/`:setvar`는 후속 수정에서 주변 T-SQL의 batch·CTE 경계를 검사하는 사례를 추가했다. Include 파일 실행·변수 치환은 범위 밖이며 SQLCMD 전체 실행 지원을 뜻하지 않는다. 준비 시점 EXT 목록과 후속 구현을 구분하여 [#122·#123 보고서](reports/issue-122-123-tsql-boundaries.md)의 경계를 따른다.

타입·이름 해결, macro/proc-macro 실행, DB 객체 존재·SQL 실행, DOM/rendering, 외부 XML entity 취득, MSBuild/NuGet/WCF activation은 구문 해석을 넘어서는 별도 책임이다. 실제 corpus의 CST/API PASS와 ERROR 없는 원문, 애플리케이션 build/runtime 성공은 각각 다르다. [#188 보고서](reports/issue-188-corpus-cst-api.md)는 오류 tree의 구조/API 검증과 원문·방언 경계를 구분한다.

Offline `inspect`·`identity`·`verify`·`schema`는 26개 route만을 대상으로 제한하지 않는다. 다른 grammar도 지원되는 파일·schema 형식이면 검사할 수 있다. 그 검사는 해당 언어의 구문 지원 qualification을 대신하지 않는다.

## 이 안내를 갱신할 때

지원 범위·version·source·상태가 바뀌면 해당 canonical 계약과 등록부를 먼저 확인하고 두 언어 문서를 함께 갱신한다. 과거 source registry의 준비 상태나 release 당시 한계를 현재 native 성공/실패로 자동 승계하지 않는다. 지원 판정은 후보·run·platform 근거에 결속하고 로컬 문서 검사와 native/cross-platform 재실행을 구분한다. 검증·리뷰·통합 절차는 [validation](validation/validation.md)이 소유한다.
