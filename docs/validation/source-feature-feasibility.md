# Campaign 01 source와 feature feasibility

기준일 `2026-09-29`, PREPARE [#20](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/20). 모든 후보는 [등록된 26개 immutable identity](../../src/contracts/language-sources.json) 그대로다. feature ID는 [disposition](language-feature-disposition.md)를 가리킨다. 이 표는 P05 조사 결과이며 grammar 채택 변경이나 native 실행 허가가 아니다.

## 관측 등급

- `UPSTREAM_DECLARED`: 고정 upstream 문서가 선언한 제한. local 재현 없이도 알려진 위험이지만 실패 receipt라고 부르지 않는다.
- `STATIC_SOURCE_OBSERVATION`: 고정 source/layout에서 실제 읽은 경로·rule·누락. 실행 성공/실패로 승격하지 않는다.
- `UNVERIFIED_SUPPORT`: 공식 required feature와 후보 간 검증이 남음. README의 지원 문구나 파일 존재로 PASS가 되지 않는다.
- `REPRODUCED_FAILURE`: 승인된 동일 source/input/tool/한도 실행 receipt가 실제 실패를 관측한 경우만 사용한다. **현재 26 upstream route에 이 등급의 기록은 0개**다.

모든 행의 native 상태는 `NOT_RUN`, 전체 source closure는 `CONTENT_REVIEW_PENDING`이다. metadata/file-list 관측, 일부 source 정적 열람, executable closure의 완전한 검토는 서로 다르다. 후보의 설치 script는 실행하지 않았다. 과거 owned scannerless/stateful 7회 probe는 공통 CLI/GCC/runtime 경로의 근거이며 이 26행의 지원 성공을 대신하지 않는다.

## 26 route 위험 매핑

`근거`의 등록부는 고정 repository commit의 tree/file metadata를 뜻한다. 이 표에서 source가 직접 연결된 경우에만 그 부분의 정적 내용을 관측했다. 최신 feature의 미확인은 grammar에 그 기능이 없다는 단정이 아니다.

| Route | 등급 | Required feature ID | 관측·남은 위험 | 근거·다음 증거 |
|---|---|---|---|---|
| csharp | UPSTREAM_DECLARED | csharp-B01,csharp-V14c | 유효한 async/var/await identifier 일부와 file-based directives 미지원 선언 | 아래 CS-1/CS-2; P05 해결·별도 승인 필요 |
| go | UNVERIFIED_SUPPORT | go-V18,go-V27 | 고정 parser 존재; 1.27 generic method/selector key 구조·legacy 누적 지원 미검증 | 등록부; 해당 feature positive/negative·mapping |
| python | UNVERIFIED_SUPPORT | python-B01,python-V312,python-V314 | scanner 의존 indent/f-string/t-string과 recovery/edit 경계 | 등록부; scanner closure·version별 case |
| javascript | UNVERIFIED_SUPPORT | javascript-B01,javascript-V22,javascript-B02 | shared scanner/regexp·ASI·module 문법 누적 범위 미검증 | 등록부; script/module·regex boundary |
| jsx | UNVERIFIED_SUPPORT | jsx-B01,jsx-B02,jsx-B03 | javascript와 같은 pin이어도 JSX 태그/문자열 mode 전환 별도 검증 필요 | 등록부; JSX 전용 source/case identity |
| typescript | STATIC_SOURCE_OBSERVATION | typescript-V50,typescript-V59 | common grammar가 JS npm dependency를 상속; import rule에 defer 선택이 보이지 않음 | TS-1/TS-2; closure 및 generated artifact 대응 미확인 |
| tsx | STATIC_SOURCE_OBSERVATION | tsx-B01,tsx-B02,tsx-V29 | dialect 분기·JSX/type-parameter conflicts는 있음; TS7/5.9 누적 및 ambiguity native 미검증 | TS-1/TS-2; exact tsx producer로 확인 |
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
| tsql | STATIC_SOURCE_OBSERVATION | tsql-B01,tsql-B02,tsql-B03,tsql-B04,tsql-B05 | SELECT/EXEC 중심 production, DDL/다른 DML/CTE TODO, identifier regex 제한; README의 config-function 대소문자 errata도 있음 | SQL-1/SQL-2; 알려진 필수 source 격차, native NOT_RUN |
| postgresql-sql | UNVERIFIED_SUPPORT | postgresql-sql-B01,postgresql-sql-B02,postgresql-sql-B03,postgresql-sql-B04,postgresql-sql-V18,postgresql-sql-L01 | 후보의 생성 기반은 PG19 post-beta3; 채택 9.6~18 query/DML/DDL/utility·legacy 누적 지원 미검증 | PG-1은 생성 기반 선언이며 미지원 선언 아님; 19 superset 수용 자체는 실패 아님 |

## 읽은 고정 source와 판정 한계

| 근거 ID | 고정 primary source | 관측 | 판정 |
|---|---|---|---|
| CS-1 | [README Status](https://github.com/tree-sitter/tree-sitter-c-sharp/blob/9150f7d56bb47f1a809fa23623f1ba1413e93fa9/README.md#status) | contextual identifier의 유효 문맥 일부에 예외 명시 | 어느 예제가 실패하는지는 미재현 |
| CS-2 | 같은 README | #:property/package/sdk/project 미인식 명시 | csharp-V14c의 required gap; 요구 제외 불가 |
| TS-1 | [tsx/grammar.js](https://github.com/tree-sitter/tree-sitter-typescript/blob/75b3874edb2dc714fb1fd77a32013d0f8699989f/tsx/grammar.js), [common/define-grammar.js](https://github.com/tree-sitter/tree-sitter-typescript/blob/75b3874edb2dc714fb1fd77a32013d0f8699989f/common/define-grammar.js) | common factory의 tsx 분기/JSX conflicts/typed JSX·type assertion 배제; import_statement는 type/typeof 후 기존 clause만 명시 | import defer가 이 source rule에 없음. generated parser 일치/실패는 아직 미검증 |
| TS-2 | [package.json](https://github.com/tree-sitter/tree-sitter-typescript/blob/75b3874edb2dc714fb1fd77a32013d0f8699989f/package.json) | tree-sitter-javascript ^0.23.1 dependency, node-gyp-build install script | range는 immutable 실행 bytes가 아님; JS route pin과 자동 동일시 금지; npm install 미실행 |
| PHP-1 | [php/grammar.js](https://github.com/tree-sitter/tree-sitter-php/blob/92b5271b60bec77fb65b5e5bc41561e8dac81299/php/grammar.js) | common/define-grammar의 php selector 사용 | php_only의 결과로 php route를 대체하지 않음 |
| SQL-1 | [README Errata](https://github.com/Crary-Systems/tree-sitter-tsql/blob/443d2bc774f1d779af7dcabcc99160fb24da96e6/README.md#errata) | uppercase가 아닌 configuration function이 LOCAL_ID로 분류될 수 있음 | node 분류 위험이며 syntax rejection 관측 아님 |
| SQL-2 | [grammar.js](https://github.com/Crary-Systems/tree-sitter-tsql/blob/443d2bc774f1d779af7dcabcc99160fb24da96e6/grammar.js) | sql_clauses→dml_clause/another_statement, 각각 SELECT/EXEC만; CTE/DDL/다른 DML 경로 TODO. ID/대괄호 identifier regex도 제한적 | 문법 source상 필수 지원 격차. source/generated artifact correspondence와 runtime 실패는 아직 미검증 |
| PG-1 | [README Features/Design](https://github.com/gmr/tree-sitter-postgres/blob/59d0d8cd7506d68de1229fb4bbce838c83b60c8a/README.md) | REL_19_STABLE b368bdd2301에서 생성했다고 선언, 별도 PL/pgSQL grammar 및 generation scripts | PG18/legacy 지원은 별도 확인. regeneration/script 실행 미승인 |

T-SQL root grammar가 읽는 `grammar/precedences.js`, `grammar/builtins.js`, `grammar/functions/*`, `grammar/data_types.js`는 scanner/header 목록만으로 닫히지 않는 generation input이다. TS/TSX의 npm JavaScript grammar, CPP의 generation dependency, Dart/YAML의 submodule도 해당 연산에 실제 필요한지 분류하고 bytes를 pin해야 한다. 이미 생성된 grammar.json/parser.c를 읽는 연산과 grammar.js를 실행하는 재생성 연산의 closure는 같지 않다.

## 필요한 고위험 case의 내용

아래는 **미실행 검증 설계**다. 예상 tree 또는 가공한 PASS/FAIL 출력이 아니다. 각 입력은 source bytes로만 취급하며 C#/shell/SQL application을 실행하지 않는다. N/R/E는 valid counterpart를 확정한 뒤 delimiter 손상·수정 순서를 추가한다.

| Case ID | Feature | 입력 골격·검증할 사실 | 상태 |
|---|---|---|---|
| P05-CS-ID | csharp-B01 | class/field/local 이름에 async/await/var를 놓은 공식 valid context 여러 개; identifier와 modifier 분리 | PLANNED_NOT_RUN; README의 일부 예외를 특정 한 예제 실패로 단정하지 않음 |
| P05-CS-DIRECTIVE | csharp-V14c | #:property TargetFramework=net10.0 뒤 일반 .cs; 나머지 세 directive도 각각 등록 | PLANNED_NOT_RUN; directive 행과 후속 statement 보존 |
| P05-TSX-AMBIGUITY | tsx-B02,tsx-V29 | `const id = <T,>(x: T) => x;`와 generic JSX/self-closing/fragment/relational expression | PLANNED_NOT_RUN; generic parameter/tag/query field 구분 |
| P05-TSX-DEFER | tsx-B01 | `import defer * as m from "m";` + typed declaration/JSX body | PLANNED_NOT_RUN; TS-1의 common factory 관찰을 TSX producer로 별도 확인하며 .ts 증거로 대체하지 않음 |
| P05-TSQL-CASE | tsql-B01 | `SELECT @@VERSION;`와 lowercase spelling | PLANNED_NOT_RUN; 둘의 syntax 수용과 node 분류를 각각 비교 |
| P05-TSQL-STATEMENTS | tsql-B02,tsql-B03,tsql-B04 | 한 글자 identifier의 CREATE TABLE/INSERT, CTE/SELECT, procedure/transaction, modern DDL | PLANNED_NOT_RUN; unrelated ERROR-free SELECT로 대체 금지 |
| P05-TSQL-GO | tsql-B05 | batch 사이 독립 행 GO 및 quoted/string 내부 GO 대조 | PLANNED_NOT_RUN; client separator 경계 |
| P05-PG-LEGACY | postgresql-sql-B01,postgresql-sql-L01 | 9.6 WITH OIDS/quoted identifier/dollar string/ON CONFLICT와 18 counterpart | PLANNED_NOT_RUN; 유효 legacy source가 보존되는지 |
| P05-PG-18 | postgresql-sql-V18 | VIRTUAL generated column, WITHOUT OVERLAPS/PERIOD key, RETURNING OLD/NEW | PLANNED_NOT_RUN; clause/body/identifier 구조 |

## 남은 승인과 readiness

기존 native 승인은 **owned fixtures만** 대상으로 한다. 이 표의 upstream probe에 전용할 수 없다. 실제 잔여 횟수·시간·CI·보관량은 immutable 승인 기록에 연결된 누적 ledger와 새 checkpoint에서 대조한다. 새 source/tool artifact 다운로드 한도는 0 bytes다. 공개 primary 문서·source의 읽기 전용 조사는 이 실행/다운로드 한도를 확대하지 않는다. 이번 후속 준비에서는 upstream generation/build/parse/edit를 0회 수행했다.

현재 P05가 남는 이유는 (1) C#의 문서상 필수 gap, (2) T-SQL의 정적 필수 source gap, (3) 26개 executable source closure 및 선택 high-risk native 경로의 미확인이다. TS/TSX import-defer source 위험도 남는다. exact candidate 유지와 요구 유지 조건 아래 이 gap을 무조건 해결됐다고 할 수 없다.

담당은 PREPARE/#20, 기한은 S01 readiness 전이다. 추가 source acquisition/native 효과가 필요해지면 source set·destination·isolation·횟수·입출력/시간/memory/저장 한도를 구체화해 먼저 승인받는다. 환경변수 제거는 hostile native network/filesystem isolation의 증거가 아니다. 다른 grammar 채택·다른 repo 수정·기대값 완화·알려진 gap을 안고 구현을 시작하는 예외는 각각 별도 결정이며 이번 문서가 부여하지 않는다. 이 조건을 해결하기 전 verdict는 `ADOPTED_PENDING_INPUTS`와 `BLOCKED_EXTERNAL` 사유를 유지한다.
