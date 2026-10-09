# T-SQL 토큰 변형 경계 보완 (#137·#138·#139·#140)

공개 합성 입력 30개 주변에서 토큰 삭제·삽입·치환으로 만든 2,860개 고유 입력을 SQL Server 2025 Express LocalDB `17.0.1000.7`의 compatibility110·170과 대조했다. 두 level에서 공통 engine REJECT/parser NO_ERROR 133개 중 112개가 9종 순수 구문 경계다. 나머지 21개는 CTE 사용, boolean 문맥, hint domain/충돌, function arity/RETURN 문맥으로 별도 처분한다. 112는 독립 결함 수가 아니다.

| Issue | 원인 | 수정 소유 |
|---|---|---|
| #137 | bare NOT/WITH 예약어, scalar NOT, DDL option 등호/값, DEFAULT 인자, SECURITY POLICY kind, wildcard alias/일반 함수 인자, 숫자 dynamic EXEC, OPENJSON 인자 구분 | 예약 집합·expression/predicate·호출·projection·문맥별 option |
| #138 | 첫 이름 segment 생략을 shared helper가 거부 | object/function reference와 qualified field, terminal name 필수 |
| #139 | formal 이름을 일반 identifier로 수용 | function_argument의 @parameter, 기존 identifier alias 보존 |

첫 segment 생략의 24개 PARSEONLY ACCEPT는 모두 정상 구문 회귀로 등록하며, 독립 `SELECT a FROM .t;`는 준비된 dbo.t에서 NOEXEC ACCEPT도 확인한 기준이다. 그 24개 전체의 binding/compile 성공을 주장하지 않는다. `CREATE TABLE ... int(1)` 및 INSERT의 빈/DEFAULT/numeric column 목록 네 사례는 PARSEONLY ACCEPT라도 NOEXEC가 거부하므로 parser ERROR를 유지한다.

호출의 projection term wrapper는 유지하지만 alias/wildcard를 그 안에 허용하지 않는다. COUNT/COUNT_BIG wildcard와 schema-qualified UDF의 DEFAULT는 별도 대안이다. SECURITY POLICY는 일반 UDF DEFAULT 대안에 접근하지 않는다. QUEUE/ROUTE/SECURITY POLICY는 public option/with_clause/with_options node를 유지하면서 각 값 종류와 등호를 검사한다. 나머지 DDL의 범용 option 규칙을 축소하지 않는다.

Microsoft [CREATE ROUTE](https://learn.microsoft.com/en-us/sql/t-sql/statements/create-route-transact-sql), [ALTER QUEUE](https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-queue-transact-sql), [CREATE SECURITY POLICY](https://learn.microsoft.com/en-us/sql/t-sql/statements/create-security-policy-transact-sql)의 syntax와 실제 엔진 대조를 함께 사용한다. SECURITY POLICY kind의 선택 표기는 실제 엔진의102와 충돌하며 이번 고정 관측에서는 kind를 요구한다. 실제 SQL Server2012 실행, SQL 원문 실행 결과, dynamic literal 내부의 재귀 파싱은 이 근거가 아니다.

등록 회귀는 정상→음성→복구를 포함한다. source C2 literal chain, patched source 및 생성물 pins, qualification inventory, source export 필수 파일 목록과 canonical disposition을 함께 갱신한다. comparator·API 허용 목록·상한·skip 정책은 유지한다. 공개 합성 입력과 비공개 corpus의 세부 자료는 분리하며 원문·파일 이름·경로는 공개 보고서에 넣지 않는다.

이름 범위 확대 후 `ADD EVENT .lock_deadlock`의 새로운 수용 회귀를 engine102로 확인했다. Extended Events의 ADD/DROP EVENT/TARGET와 ACTION package 이름은 첫 qualifier 생략을 허용하지 않는 shared helper로 분리하며 sibling5개를 추가해 총99개 등록 정상→음성→복구를 고정한다. `.t WITH (NOLOCK,HOLDLOCK)` 두 입력은 leading-dot 구문 수정 뒤에도 engine1047인 hint conflict이며 새로운 순수 구문 결함으로 세지 않는다.

NOT와 AND/OR의 predicate alternatives에는 CONTAINS/FREETEXT도 포함한다. 기존 full-text 양성 둘의 회귀를 확인해 동일 predicate family에 복구하고 NOT full-text 대조군을 추가했다. COUNT window 호출, VECTOR_SEARCH의 TABLE alias와 OPENROWSET의 ORDER 인자는 각 문맥의 전용 대안으로 보존한다. 기존 양성 fixture·anchor를 완화하지 않는다.

독립 리뷰의 추가 구문 경계는 [#140](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/140)으로 추적한다. COUNT의 일반 인자 개수 오류174는 의미 영역으로 유지하되 DISTINCT는 단일 인자를 요구한다. CONTAINS/FREETEXT는 column 또는 column list·wildcard·PROPERTY와 검색식의 두 인자, 선택 LANGUAGE만 허용한다. ORDER는 OPENROWSET의 BULK 입력 전용이며 OPENXML/PREDICT와 일반 provider 호출로 전파하지 않는다. 정상·음성·복원 18개를 추가했다. 첫 target의 scalar literal·불완전 PROPERTY·숫자 column list는 실제102로 확인해 전용 target 규칙으로 제한한다.

VECTOR_SEARCH는 version별 syntax union을 보존한다. 설치 빌드의 PARSEONLY는 scalar 호출을 ACCEPT하나 문서상 TABLE/COLUMN/SIMILAR_TO/METRIC named form을156으로 거부한다. 따라서 이 결과로 named form의 필수 인자나 고정 순서를 축소하지 않는다. 인자 조합·타입·binding은 의미/버전 영역이며 실제 기능 실행 성공을 주장하지 않는다.

[CONTAINS syntax](https://learn.microsoft.com/en-us/sql/t-sql/queries/contains-transact-sql)와 [OPENROWSET BULK](https://learn.microsoft.com/en-us/sql/t-sql/functions/openrowset-bulk-transact-sql)의 문맥을 기준으로 삼으며, ORDER 예약은 일반 함수 이름에만 적용해 query hint·BULK option 양성을 보존한다.

full-text 검색식은 문자열 또는 @변수이며 LANGUAGE는 문자열·integer/hex·@변수를 허용한다. numeric 검색식·LANGUAGE 산술식·숫자 PROPERTY 이름은 실제102다. 엔진이 수용하는 qualified wildcard는 보존한다. catalog 이름·LCID domain 및 runtime 값은 검증하지 않는다.

수정 후 원래 토큰 변형에 남는23개 의미·문맥 관측은 [#141](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/141)에서 추적한다. 구문 수정의 완료와 의미 분석의 미완료를 구별한다.

일반 AND/OR는 기존 expression operand를 유지하며, NOT guard의 논리 결합만 predicate operand로 제한한다. BETWEEN/NOT BETWEEN은 shared predicate rule을 named/aliased public node로 재사용한다. 공개 AND/OR·NOT BETWEEN·함수 range 정상→음성→복원6개로 괄호 연결의 회귀를 고정한다.

NOT predicate의 reduction precedence를 고정하고 `NOT EXISTS ... AND/OR ...` 두 정상→음성→복구를 추가했다. 괄호 없는 NOT는 EXISTS까지만 결합하며 외부 AND/OR는 별도 binary node라는 CST 범위도 공개 대조군에서 확인한다.

이전 후보의 공개 대조군91쌍(182개)은 parser와 전체 node API가 모두 기대와 일치하며, 실제 LocalDB110·170의364개 PARSEONLY 판정과도 일치했다. 그 후보의2,860개 변형에서는112개 순수 구문 음성과24개 leading-dot 양성이 모두 복구됐고 전체 API가 PASS였다. 남은23개는 #141의 문맥·의미 범위다. 네 개 PARSEONLY ACCEPT/parser ERROR 변형은 대상 table을 준비한 NOEXEC가 거부하는 type length/INSERT column 문맥이고, level110의 추가 둘은 OPENJSON compatibility 문맥이다. 따라서 raw engine/parser 불일치 수를 구문 결함 수로 승격하지 않는다.

PR 리뷰에서 Boolean을 반환하는 트리거 `UPDATE(column)`의 NOT 경로 누락을 추가로 재현했다. [UPDATE() 계약](https://learn.microsoft.com/en-us/sql/t-sql/functions/update-trigger-functions-transact-sql)은 단일 column 이름과 Boolean 반환을 규정한다. generic invocation으로 우회하지 않도록 UPDATE를 전용 대안으로 처리하고, NOT·괄호·중첩 NOT·AND/OR의 정상→음성→복원6개를 추가했다. 숫자·복수·빈 인자와 qualified column·DEFAULT·DISTINCT는 engine102/156 음성이다. schema-qualified delimited UDF 이름은 보존한다.

기존 새 경계 fixture99개의 full-source 교체를 동일 SQL 단계에 도달하는 최소 token 편집으로 좁혔다. 원래 SQL 본문의 각 단계·기대값·coverage를 대조했다. 재사용이 관측되지 않는28개 fixture에는 변경되지 않는 `SELECT @@ROWCOUNT AS rows_affected;`와 GO로 끝나는 독립 앞쪽 batch를 추가했다. GO가 target batch 경계를 유지하며 원래 고립된 본문의 음성·양성은 별도 대조군으로 보존한다. prefix 뒤에 중복되는 token은 고유 find 문맥으로 편집한다. occurrence 속성은 변환기가 지원하지 않으므로 사용하지 않는다. 오류 tree와 전체 교체 때문에 재사용 경로가 관측되지 않던 E 의무를 실제 증분 편집으로 검증하며, route 판정·qualification 계약·comparator·skip은 완화하지 않는다.

UPDATE 인자에 @변수가 column 이름으로 들어가는 우회와 schema-qualified 호출의 bare 예약 이름 우회도 actual engine156으로 확인했다. column 이름 대안에서 @변수를 제외하고 qualified UDF에도 같은 function-name 예약 경계를 적용하되 `dbo.[UPDATE](a)`는 보존한다. 두 정상→음성→복원을 추가했다.


UPDATE(column)는 IF/WHILE·WHERE/HAVING·JOIN ON·searched CASE·IIF의 첫 condition에서 허용한다. SELECT scalar·중첩 scalar 호출·IIF의 value 인자에서는 허용하지 않는다. 전용 condition 대안과 단일 column predicate를 사용하며 일반 함수 이름 예약 집합은 bare UPDATE를 차단한다. qualified `dbo.[UPDATE](a)`와 `dbo."UPDATE"(a)`는 보존하고 unqualified delimited 함수 이름은 기존 #123 음성 계약을 유지한다. 기존 #123의 정상→음성→복원18개와 새 경계·기존 recovery 대조220개를 합친238개는 최종 문법 생성물에서 기대/API PASS다. 220개는 같은 입력 hash의 compatibility110·170 엔진440개 판정과 일치한다. 새 등록 회귀는106개다.

query 없는 API profile의 manifest `queries:null` 자기 검증 실패는 [#143](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/143)으로 추적한다. producer가 빈 identity 배열을 만들도록 고치며 strict array 검증은 유지한다. `TestOracleRecordSetWithoutQueries`와 공개 합성 T-SQL 재현 입력은 수정 전 JSON_NULL, 수정 후 완결 set·API PASS를 확인했다.

BOM/padding 앞 byte 조회에서 정상 root 반환을 오류로 판정하는 비교기 결함은 [#144](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/144)이다. 공개 UTF-16LE BOM `SELECT 1;`과 byte0으로 재현했다. root 밖에서는 root만 반환해야 하며 root 안의 범위·named 검사와 mutant 거부는 유지한다. runtime patch와 운영 한도를 변경하지 않는다.
