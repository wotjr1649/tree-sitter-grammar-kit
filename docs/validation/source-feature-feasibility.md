# Campaign 01 source와 feature feasibility

기준일 `2026-09-29`, PREPARE [#20](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/20). 모든 후보는 [등록된 26개 immutable identity](../../src/contracts/language-sources.json) 그대로다. feature ID는 [disposition](language-feature-disposition.md)를 가리킨다. 이 표는 P05 조사 결과이며 grammar 채택 변경이나 native 실행 허가가 아니다.

## 관측 등급

- `UPSTREAM_DECLARED`: 고정 upstream 문서가 선언한 제한. local 재현 없이도 알려진 위험이지만 실패 receipt라고 부르지 않는다.
- `STATIC_SOURCE_OBSERVATION`: 고정 source/layout에서 실제 읽은 경로·rule·누락. 실행 성공/실패로 승격하지 않는다.
- `UNVERIFIED_SUPPORT`: 공식 required feature와 후보 간 검증이 남음. README의 지원 문구나 파일 존재로 PASS가 되지 않는다.
- `REPRODUCED_FAILURE`: 승인된 동일 source/input/tool/한도 실행 receipt가 실제 실패를 관측한 경우만 사용한다. run36646415814에서 T-SQL, run36651074932에서 C#의 필수 구문 실패가 baseline/재생성 producer별로 재현됐다. 다른 route의 미실행을 실패로 바꾸지 않는다.

과거 PREPARE-04까지 upstream G/B/X는0회였다. run36592527846에서 등록된24 repo·runtime83개·npm6개를 합친474파일과 CLI의 exact bytes를 확인했고 owned 격리4회 뒤 tmpfs 회수가 실패했다. run36606981293은 같은474파일·canary12bytes·owned isolation8개·container9개 cleanup 뒤 기존 image의 `GLIBC_2.39` loader 실패로 중단됐다. 두 기록은 grammar 실패가 아니다. 명시 승인한 Trixie의 run36644794802는 toolchain/격리/자체 G/B/X를 통과한 뒤 JSON reader에서 멈췄다. 수정한 run36646415814는 T-SQL G1/B2/X56을 실행했으며 C# generation exit0 뒤 큰 결과 회수가 timeout됐다. 나머지 selected route의 G/B/X는 그 run에서 NOT_RUN이다. byte 검증, 실행 closure, 실제 source 지원을 구분하며 npm install script와 과거 owned fixture7회를 지원 근거로 대신하지 않는다.

## 26 route 위험 매핑

이 표의 등급은 고정 source에서 얻은 근거의 종류를 유지한다. 승인 후 실제 실행의 REPRODUCED_FAILURE는 아래 native 관측 절과 immutable run receipt에 별도로 기록하며, source의 선언·정적 관찰을 실행 결과로 덮어쓰지 않는다.

`근거`의 등록부는 고정 repository commit의 tree/file metadata를 뜻한다. 이 표에서 source가 직접 연결된 경우에만 그 부분의 정적 내용을 관측했다. 최신 feature의 미확인은 grammar에 그 기능이 없다는 단정이 아니다.

| Route | 등급 | Required feature ID | 관측·남은 위험 | 근거·다음 증거 |
|---|---|---|---|---|
| csharp | UPSTREAM_DECLARED | csharp-B01,csharp-V14c | 유효한 async/var/await identifier 일부와 file-based directives 미지원 선언 | 아래 CS-1/CS-2; P05 해결·별도 승인 필요 |
| go | UNVERIFIED_SUPPORT | go-V18,go-V27 | 고정 parser 존재; 1.27 generic method/selector key 구조·legacy 누적 지원 미검증 | 등록부; 해당 feature positive/negative·mapping |
| python | UNVERIFIED_SUPPORT | python-B01,python-V312,python-V314 | scanner 의존 indent/f-string/t-string과 recovery/edit 경계 | 등록부; scanner closure·version별 case |
| javascript | UNVERIFIED_SUPPORT | javascript-B01,javascript-V22,javascript-B02 | shared scanner/regexp·ASI·module 문법 누적 범위 미검증 | 등록부; script/module·regex boundary |
| jsx | UNVERIFIED_SUPPORT | jsx-B01,jsx-B02,jsx-B03 | javascript와 같은 pin이어도 JSX 태그/문자열 mode 전환 별도 검증 필요 | 등록부; JSX 전용 source/case identity |
| typescript | STATIC_SOURCE_OBSERVATION | typescript-V50,typescript-V59 | common grammar가 JS npm dependency를 상속; 두 producer의 import defer 오류와 legacy namespace import 수용 관측 | TS-1/TS-2와 아래 run36657324824; required defer remedy 필요 |
| tsx | STATIC_SOURCE_OBSERVATION | tsx-B01,tsx-B02,tsx-V29 | 두 producer의 import defer 오류; 등록 generic/JSX/fragment/relational5개 수용. 전체 TS7/5.9 지원 주장은 아님 | TS-1/TS-2와 아래 native 절; TS producer와 별도 관측 |
| java | UNVERIFIED_SUPPORT | java-V21,java-V25 | parser 존재는 25 compact source/module import/flexible constructor 지원 증거 아님 | 등록부; 해당 syntax와 Java8 legacy case |
| kotlin | UNVERIFIED_SUPPORT | kotlin-V22,kotlin-V24 | scanner와 2.4 context/field/@all 지원 미검증 | 등록부; stable와 experimental case 분리 |
| c | UNVERIFIED_SUPPORT | c-B02,c-V23,c-V23pp,c-L01 | C23와 legacy declarator/preprocessor 범위 미검증 | 등록부; macro source·compiler 의미 구별 |
| cpp | UNVERIFIED_SUPPORT | cpp-V20,cpp-V23,cpp-L01 | scanner/grammar dependency 및 최신/제거된 구문 합집합 미검증 | 등록부; generator dependency closure 필요 |
| rust | UNVERIFIED_SUPPORT | rust-B02,rust-V24,rust-L01 | scanner/token-tree·edition 문맥 미검증 | 등록부; edition/capture/let-chain cases |
| swift | STATIC_SOURCE_OBSERVATION | swift-V62,swift-V64 | 고정 tree에는 parser.c 없음; generation 경로와 최신 source syntax 미검증 | 등록부; generation 효과·closure를 별도 승인 후 검증 |
| dart | UNVERIFIED_SUPPORT | dart-V30,dart-V310 | scanner/Tree-sitter submodule과 3.13 constructor 구문 미검증 | 등록부; submodule 실행 필요성·closure 분류 |
| php | STATIC_SOURCE_OBSERVATION | php-B01,php-V84,php-L01 | php grammar는 common factory를 사용; php_only와 mode/산출물 다름 | PHP-1; selector 고정·mixed boundary/8.5 지원 미검증 |
| ruby | UNVERIFIED_SUPPORT | ruby-B01,ruby-V31,ruby-V40 | scanner state/heredoc 및 4.0 newline continuation 미검증 | 등록부; raw bytes·edit cases |
| r | UNVERIFIED_SUPPORT | r-B01,r-V40,r-V42 | scanner·pipe/lambda/placeholder grammar 미검증 | 등록부; precedence/continuation cases |
| bash | UNVERIFIED_SUPPORT | bash-B01,bash-V53 | scanner state와 5.3 command substitution 구문 미검증 | 등록부; source parse만 허용, shell 실행 제외 |
| powershell | UNVERIFIED_SUPPORT | powershell-B01,powershell-L01,powershell-V7 | scanner·command mode와 5.1 legacy/7.6 syntax 합집합 미검증 | 등록부; pwsh5 실행 없이 source cases |
| html | UNVERIFIED_SUPPORT | html-B01,html-B02 | external scanner의 raw-text/foreign/fragment recovery 범위 미검증 | 등록부; DOM 동일성 대신 source CST |
| css | UNVERIFIED_SUPPORT | css-M01,css-M11,css-M27,css-M28,css-M30 | 단일 parse smoke로 Snapshot30개 module 계열 지원 입증 불가 | 등록부; module syntax별 case·scanner closure |
| json | UNVERIFIED_SUPPORT | json-B01,json-B02 | scannerless parser 존재; strict token/negative/edit 실제 실행 미검증 | 등록부; JSONC 허용을 strict PASS로 대체 금지 |
| yaml | UNVERIFIED_SUPPORT | yaml-B01,yaml-B03,yaml-V12 | schema별 generated variants·scanner와 test-suite submodule 존재 | 등록부; 선택 root grammar의 closure, variant 자동 변경 금지 |
| xml | UNVERIFIED_SUPPORT | xml-B01,xml-B02,xml-V11 | xml/common/dtd source 관계와 scanner/1.1 문자 경계 미검증 | 등록부; DTD 별도 route 자동 채택 금지 |
| tsql | STATIC_SOURCE_OBSERVATION | tsql-B01,tsql-B02,tsql-B03,tsql-B04,tsql-B05,tsql-V16,tsql-V22 | 고정 source의 baseline/재생성 각각28개 중23개에서 ERROR/MISSING. bare/escaped identifier, DDL/DML/CTE/procedure/transaction와 등록 modern 절의 실패 재현; lowercase @@version은 syntax 수용과 분류 차이를 별도 관측 | SQL-1/SQL-2의 source 등급 유지; 별도 native 절은 REPRODUCED_FAILURE이며 remedy 결정 필요 |
| postgresql-sql | STATIC_SOURCE_OBSERVATION | postgresql-sql-B01,postgresql-sql-B02,postgresql-sql-B03,postgresql-sql-B04,postgresql-sql-V18,postgresql-sql-L01 | 생성 기반 PG19와 별개로 baseline parser는 LFS pointer. generation exit137 뒤 legacy/18 모두 NOT_RUN | PG-1은 미지원 선언 아님; 실제 LFS object·generation capability 확인 필요 |

## 읽은 고정 source와 판정 한계

| 근거 ID | 고정 primary source | 관측 | 판정 |
|---|---|---|---|
| CS-1 | [README Status](https://github.com/tree-sitter/tree-sitter-c-sharp/blob/9150f7d56bb47f1a809fa23623f1ba1413e93fa9/README.md#status) | contextual identifier의 유효 문맥 일부에 예외 명시 | 등록3개 좁은 문맥은 수용; 넓은 예외와 그 실패 문맥은 미해소 |
| CS-2 | 같은 README | #:property/package/sdk/project 미인식 명시 | csharp-V14c의 required gap; 요구 제외 불가 |
| TS-1 | [tsx/grammar.js](https://github.com/tree-sitter/tree-sitter-typescript/blob/75b3874edb2dc714fb1fd77a32013d0f8699989f/tsx/grammar.js), [common/define-grammar.js](https://github.com/tree-sitter/tree-sitter-typescript/blob/75b3874edb2dc714fb1fd77a32013d0f8699989f/common/define-grammar.js) | common factory의 tsx 분기/JSX conflicts/typed JSX·type assertion 배제; import_statement는 type/typeof 후 기존 clause만 명시 | import defer가 이 source rule에 없음. generated parser 일치/실패는 아직 미검증 |
| TS-2 | [package.json](https://github.com/tree-sitter/tree-sitter-typescript/blob/75b3874edb2dc714fb1fd77a32013d0f8699989f/package.json) | tree-sitter-javascript ^0.23.1 dependency, node-gyp-build install script | range는 immutable 실행 bytes가 아님; JS route pin과 자동 동일시 금지; npm install 미실행 |
| PHP-1 | [php/grammar.js](https://github.com/tree-sitter/tree-sitter-php/blob/92b5271b60bec77fb65b5e5bc41561e8dac81299/php/grammar.js) | common/define-grammar의 php selector 사용 | php_only의 결과로 php route를 대체하지 않음 |
| SQL-1 | [README Errata](https://github.com/Crary-Systems/tree-sitter-tsql/blob/443d2bc774f1d779af7dcabcc99160fb24da96e6/README.md#errata) | uppercase가 아닌 configuration function이 LOCAL_ID로 분류될 수 있음 | node 분류 위험이며 syntax rejection 관측 아님 |
| SQL-2 | [grammar.js](https://github.com/Crary-Systems/tree-sitter-tsql/blob/443d2bc774f1d779af7dcabcc99160fb24da96e6/grammar.js) | sql_clauses→dml_clause/another_statement, 각각 SELECT/EXEC만; CTE/DDL/다른 DML 경로 TODO. ID/대괄호 identifier regex도 제한적 | 문법 source상 필수 지원 격차. source/generated artifact correspondence와 runtime 실패는 아직 미검증 |
| PG-1 | [README Features/Design](https://github.com/gmr/tree-sitter-postgres/blob/59d0d8cd7506d68de1229fb4bbce838c83b60c8a/README.md) | REL_19_STABLE b368bdd2301에서 생성했다고 선언, 별도 PL/pgSQL grammar 및 generation scripts | PG18/legacy 지원은 별도 확인. regeneration/script 실행 미승인 |

T-SQL root grammar가 읽는 `grammar/precedences.js`, `grammar/builtins.js`, `grammar/functions/*`, `grammar/data_types.js`는 scanner/header 목록만으로 닫히지 않는 generation input이다. TS/TSX의 npm JavaScript grammar, CPP의 generation dependency, Dart/YAML의 submodule도 해당 연산에 실제 필요한지 분류하고 bytes를 pin해야 한다. 이미 생성된 grammar.json/parser.c를 읽는 연산과 grammar.js를 실행하는 재생성 연산의 closure는 같지 않다. 26행의 단계별 입력·검증 기한과 실행 순서는 [P05 source closure](p05-source-closure.md)를 따른다.

## 필요한 고위험 case의 내용

아래는 등록된 검증 설계와 현재 관측의 대응이다. 실제 tree는 아래 native 절과 원 receipt로 구분하며 가공한 golden을 사용하지 않는다. 각 입력은 source bytes로만 취급하며 C#/shell/SQL application을 실행하지 않는다. 등록된 N/R/E의 손상·복구도 원 bytes/기대에 결속한다.

| Case ID | Feature | 입력 골격·검증할 사실 | 상태 |
|---|---|---|---|
| P05-CS-ID | csharp-B01 | class/field/local 이름에 async/await/var를 놓은 공식 valid context 여러 개; identifier와 modifier 분리 | OBSERVED_BOUNDED_POSITIVES; 두 producer의 등록3case 수용, 넓은 README 예외는 유지 |
| P05-CS-DIRECTIVE | csharp-V14c | #:property TargetFramework=net10.0 뒤 일반 .cs; 나머지 세 directive도 각각 등록 | REPRODUCED_FAILURE; 네 #: directive 모두 두 producer에서 오류, shebang/raw 대조와 구분 |
| P05-CS-DIRECTIVE-INCLUDE | csharp-V14c | SDK10.0.300의 `#:include helpers.cs` 뒤 일반 .cs; 이름·인수·행 경계와 후속 statement 보존 | REPRODUCED_FAILURE; 두 producer 모두 실패, 기존 네 directive의 upstream 선언과 별도 구체 관측 |
| P05-TS-DEFER | typescript-V59 | `import defer * as m from "m";`와 typed declaration; 일반 namespace import legacy 대조 | REPRODUCED_FAILURE; 두 producer의 defer 오류와 typed 후속 구조, legacy 수용 구분 |
| P05-TSX-AMBIGUITY | tsx-B02,tsx-V29 | `const id = <T,>(x: T) => x;`와 generic JSX/self-closing/fragment/relational expression | OBSERVED_BOUNDED_POSITIVES;5개 대조의 parameter/tag/fragment/operator 구조 확인 |
| P05-TSX-DEFER | tsx-B01 | `import defer * as m from "m";` + typed declaration/JSX body | REPRODUCED_FAILURE; exact TSX 두 producer에서 defer 오류, 뒤 JSX 보존·edit 복구 확인 |
| P05-TSQL-CASE | tsql-B01 | `SELECT @@VERSION;`와 lowercase spelling | OBSERVED_CLASSIFICATION_DIVERGENCE; 둘의 syntax 수용과 configuration_functions/LOCAL_ID_ 분류 차이를 별도 보존 |
| P05-TSQL-STATEMENTS | tsql-B02,tsql-B03,tsql-B04,tsql-V16,tsql-V22 | 한 글자 identifier의 CREATE TABLE/INSERT, CTE/SELECT, procedure/transaction; FOR JSON/OPENJSON, FOR SYSTEM_TIME/SYSTEM_VERSIONING, DROP IF EXISTS와 CREATE OR ALTER의 별도 source case, AS NODE/EDGE/MATCH, WINDOW/IS DISTINCT FROM, JSON/VECTOR type, LEDGER table option의 각 source case | REPRODUCED_FAILURE;21case 모두 두 producer에서 오류, unrelated SELECT로 대체 금지 |
| P05-TSQL-GO | tsql-B05 | batch 사이 독립 행 GO 및 quoted/string 내부 GO 대조 | OBSERVED_BOUNDED_POSITIVES; 등록 separator/count와 문자열 대조의 구조 확인 |
| P05-PG-LEGACY | postgresql-sql-B01,postgresql-sql-L01 | 9.6 WITH OIDS/quoted identifier/dollar string/ON CONFLICT와 18 counterpart | PLANNED_NOT_RUN; 유효 legacy source가 보존되는지 |
| P05-PG-18 | postgresql-sql-V18 | VIRTUAL generated column, WITHOUT OVERLAPS/PERIOD key, RETURNING OLD/NEW | PLANNED_NOT_RUN; clause/body/identifier 구조 |
| P05-SWIFT-GENERATION | swift-B01,swift-V62,swift-V64 | 고정 grammar generation과 Swift5 generic function/inline array/module selector source의 구분된 입력 | PLANNED_NOT_RUN; generation과 syntax 결과를 분리하며 새 syntax 범위 채택이 아님 |

## 남은 승인과 readiness

PR #22/prepare-02 당시 native 승인은 **owned fixtures만**, source/tool artifact 다운로드 한도는 0 bytes였고 upstream generation/build/parse/edit는 0회였다. 이 이전 기록과 소비는 보존한다.

후속 PREPARE-03에서 사용자는 명시한 source/tool acquisition과 격리 upstream probe를 별도로 승인했다. [고정 입력](../../src/dev/prepare-p05/inputs.json)과 [수동 workflow](../../.github/workflows/prepare-p05.yml)는 그 승인에 연결된 실행 대상이다. 상한은 HTTP32회·download1 GiB·generation6회·build11회·parse/edit128회·preflight8회·diagnostic16회, 수동 native job1회/80분, artifact256 MiB/7일이다. PREPARE 시간은 과거 소비를 포함한 누적28,800초이며 CI120 job-minutes·유료KRW0이다. 승인은 실행 성공이나 잔여량을 뜻하지 않는다. 실제 commit/review/CI/main, 선행 input·격리 검증, 누적 ledger와 실행·cleanup receipt를 확인한 뒤 각 연산을 시작한다.

현재 P05가 남는 이유는 (1) C#의 넓은 declared identifier gap과 재현한 다섯 directive 실패, (2) T-SQL의 재현한 statement/identifier 및 분류 격차, (3) TS/TSX import defer의 실제 실패, (4) PG artifact/generation 차단과 Swift native 미실행 및 각26route의 단계별 남은 입력이다. exact candidate와 요구 유지 조건 아래 이 gap을 해결됐다고 할 수 없다. 지속 승인으로 유한 동일 범위 배치를 갱신하며 옛 누적 시간/job quota를 새 권한 질문으로 반복하지 않는다.

## TS/TSX의 실제 native와 PG 단계 차단

[run36657324824/attempt1](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/36657324824)은 같은 source75b387/npmJS0.23.1/CLI0.27.0/runtime659cda7/Trixie에서 TS/TSX G2/B4/X16을 실행했다.8개 원문 각각의 baseline/regenerated raw SHA가 같았다. 두 route의 `import defer`는 각각 ERROR7..12/exit2이며 typed declaration와 TSX JSX body는 후속 구조로 보존됐다. legacy namespace import와5개 TSX ambiguity 대조는 두 producer에서 오류 없이 등록한 구조를 관측했다.

TS defer의 `as` 손상과 TSX defer의 JSX `{` 삭제는 각각 등록 negative 오류를 남겼다. 네 producer edit의5stage/2comparison과 damaged/restored incremental-fresh 동일성, 복구 뒤 원 ordered facts 일치를 확인했다. 복구 후에도 원래 defer 오류가 남으므로 지원 PASS가 아니다. source bytes·comparator·기대는 변경하지 않았다.

PG G1은92.265초 후 exit137/출력0bytes였고 원인은 아직 확정하지 않았다. B1은133bytes의 Git LFS pointer를 C로 컴파일하다 exit1이었다. 실제 object는97,664,793bytes/SHA-256 `a9090d5082ae5c23892d05aa59e61476f9bd39ad634228f2046024debdf815b5`이며 새 취득 효과·단일 파일 한도의 추가 승인 없이는 받을 수 없다.8개 PG 원문/두 producer의 syntax·negative/recovery/edit는 모두 NOT_RUN이다. Swift는 고정 grammar의 `"import"` 토큰을 module import로 오인한 helper 검사에서 NOT_RUN이다. 이 둘을 source syntax failure로 기록하지 않는다. container43개의 종료·PID0·제거를 확인했고 전체 run은 FAILED다.

## C#의 실제 baseline/재생성 관측

[run36651074932/attempt1](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/36651074932)은 같은 승인 Trixie/CLI0.27.0/runtime659cda7 및 고정 C#9150f7d에서 G1/B2/X20을 수행했다. 10개 원문 각각의 baseline/regenerated stdout SHA와 ordered facts가 동일하며, generation/build 및 20개 command의 종료·회수·cleanup을 확인했다. 전체 run은 이후 JS closure helper에서 FAILED이고 whole_feature_support/product_qualification은 false다.

| 등록 case | 실제 syntax·구조·negative/recovery 사실 | 판정의 범위 |
|---|---|---|
| P05-CS-ID-ASYNC | class name identifier/name6..11, field name18..23, declaration_list/body12..26; `{` 삭제 후 ERROR13..22/24..25와 후속 bytes, damaged incremental/fresh 동일; 복구 후 원27bytes의 point/order/구조 및 fresh/incremental 동일 | 등록 type-name 문맥과 두 producer edit만 확인; 넓은 identifier 선언 gap 유지 |
| P05-CS-ID-AWAIT | field identifier/name14..19 및 local name36..41, method/body30..48; 오류·누락 없음 | 등록 field/local 문맥만 확인 |
| P05-CS-ID-VAR | 첫 class name6..9 및 둘째 class local name39..42, 같은 범위의 var token child; 오류·누락 없음 | 등록 type/local 문맥만 확인 |
| P05-CS-DIRECTIVE-SHEBANG | shebang_directive0..21, 다음 행 global_statement22..50와 invocation22..49 | shebang 대조 성공이며 #: directive 대체 근거가 아님 |
| P05-CS-DIRECTIVE-PROPERTY/PACKAGE/SDK/PROJECT/INCLUDE | 각각 두 producer에서 exit2/ERROR; directive 이름이 type 등으로 오분류되고 행/후속 구조가 합쳐짐. package에는 후속 정상 global_statement32..60이 남지만 directive 자체는 실패 | csharp-V14c의 다섯 required directive 실패. include는 별도 채택 사실의 실제 재현 |
| P05-CS-DIRECTIVE-RAW | raw_string_literal21..49, content24..46에 include text 보존, class/body/후속 byte·point/order와 오류 없음 | raw 내부 text는 directive로 분류하지 않음; include 지원 주장이 아님 |

3개 identifier의 좁은 positive는 upstream의 모든 유효 contextual 문맥을 검증하지 않는다. 원 실패를 보존하고 실제 grammar remedy/후보 결정은 별도 승인이 필요하다. TS/TSX/PG/Swift는 이 run에서 NOT_RUN이며 dependency 분류 오류를 language 실패로 표시하지 않는다.

## Trixie의 첫 upstream native 관측

[run36646415814/attempt1](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/36646415814)은 고정 `Crary-Systems/tree-sitter-tsql@443d2bc774f1d779af7dcabcc99160fb24da96e6`을 CLI0.27.0/ABI15로 재생성하고 원 parser와 별도로 빌드했다. 두 producer에서 등록28개 입력씩 총56회를 실행했다. 원문 bytes/구문 기대를 고치지 않았다.

| 등록 case | 각 producer의 관측 | required 사실과 판정 |
|---|---|---|
| P05-TSQL-ID-BARE/ESCAPED | ERROR/MISSING | 한 글자 bare·escaped bracket identifier 미충족 |
| P05-TSQL-STATEMENTS의21개 입력 | ERROR/MISSING | tsql-B02/B03/B04/V16/V22의 등록 statement/절 미충족; 다른 SELECT로 대체 불가 |
| P05-TSQL-ID-BRACKET | syntax 수용 | column `[a]`의 full_column_name/id_ 범위7..10, table `[t]`의 full_table_name/table 범위16..19 확인 |
| P05-TSQL-CASE-UPPER/LOWER | 둘 다 syntax 수용 | `@@VERSION`은 configuration_functions/version_, `@@version`은 primitive_expression/LOCAL_ID_; 같은7..16 범위지만 분류 차이가 실제 재현됨. 이 분류 관측은 syntax 실패와 구분 |
| P05-TSQL-GO-BATCH/LITERAL | syntax 수용 | GO 범위10..14·count13..14·두 SELECT0..8/15..23, literal/alias 내부 GO에 go_statement 없음 확인 |
| P05-TSQL-ID-BARE edit | 손상/복구 incremental=fresh | original/restored에도 ERROR가 남아 required positive/recovery PASS가 아님 |

G1/B2/X56과23×2 syntax 실패를 확인했어도 T-SQL 전체 범위의 전수 qualification은 아니다. C# generation은5.216초/exit0이나 회수는10.052초에32,305,152bytes에서 timeout돼 build/execution으로 진행하지 않았다. cleanup77개가 확인됐고 원 partial tar와 명령·output·limits를 보존했다. 이 회수 helper 실패는 C# grammar 실패가 아니다. 나머지 C#/TS/TSX/PG/Swift input과 edit는 해당 run에서 미실행으로 기록한다.

위 수치는 최초 B 승인 당시 envelope다. 이후 수동 job2회 및3회째와 T-SQL raw 취득의 별도 승인을 각각 소비했다. 2026-09-30 현재 사용자는 동일 PREPARE 개발·검증·CI·기존 고정 입력의 추가 job과 `quiescent-tar-r1`을 지속 승인했다. 과거 총시간/3회 quota만으로 재승인을 요구하지 않고 유한 배치별 한도를 기록하며 모든 이전 소비·실패를 이월한다. 다섯 번째 job까지 source HTTP 누적150회·image pull3회·owned isolation13회, upstream G/B/X0이다. 최초 세 job14+14+66초와 후속66+65초를 보존한다. 새 image artifact는 고정 입력 승인에 자동 포함되지 않으며 별도 exact effect 승인이 필요하다. 현재 loader 장애와 image 보완 조건은 [source closure](p05-source-closure.md)를 따른다.

담당은 PREPARE/#20, 기한은 S01 readiness 전이다. 추가 source acquisition/native 효과가 필요해지면 source set·destination·isolation·횟수·입출력/시간/memory/저장 한도를 구체화해 먼저 승인받는다. 환경변수 제거는 hostile native network/filesystem isolation의 증거가 아니다. 다른 grammar 채택·다른 repo 수정·기대값 완화·알려진 gap을 안고 구현을 시작하는 예외는 각각 별도 결정이며 이번 문서가 부여하지 않는다. 이 조건을 해결하기 전 verdict는 `ADOPTED_PENDING_INPUTS`와 `BLOCKED_EXTERNAL` 사유를 유지한다.
