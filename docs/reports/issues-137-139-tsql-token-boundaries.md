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

이름 범위 확대 후 `ADD EVENT .lock_deadlock`의 새로운 수용 회귀를 engine102로 확인했다. Extended Events의 ADD/DROP EVENT/TARGET와 ACTION package 이름은 첫 qualifier 생략을 허용하지 않는 shared helper로 분리하며 sibling5개를 추가해 총83개 등록 정상→음성→복구를 고정한다. `.t WITH (NOLOCK,HOLDLOCK)` 두 입력은 leading-dot 구문 수정 뒤에도 engine1047인 hint conflict이며 새로운 순수 구문 결함으로 세지 않는다.

NOT와 AND/OR의 predicate alternatives에는 CONTAINS/FREETEXT도 포함한다. 기존 full-text 양성 둘의 회귀를 확인해 동일 predicate family에 복구하고 NOT full-text 대조군을 추가했다. COUNT window 호출, VECTOR_SEARCH의 TABLE alias와 OPENROWSET의 ORDER 인자는 각 문맥의 전용 대안으로 보존한다. 기존 양성 fixture·anchor를 완화하지 않는다.

독립 리뷰의 추가 구문 경계는 [#140](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/140)으로 추적한다. COUNT의 일반 인자 개수 오류174는 의미 영역으로 유지하되 DISTINCT는 단일 인자를 요구한다. CONTAINS/FREETEXT는 column 또는 column list·wildcard·PROPERTY와 검색식의 두 인자, 선택 LANGUAGE만 허용한다. ORDER는 OPENROWSET의 BULK 입력 전용이며 OPENXML/PREDICT와 일반 provider 호출로 전파하지 않는다. 정상·음성·복원 18개를 추가했다. 첫 target의 scalar literal·불완전 PROPERTY·숫자 column list는 실제102로 확인해 전용 target 규칙으로 제한한다.

VECTOR_SEARCH는 version별 syntax union을 보존한다. 설치 빌드의 PARSEONLY는 scalar 호출을 ACCEPT하나 문서상 TABLE/COLUMN/SIMILAR_TO/METRIC named form을156으로 거부한다. 따라서 이 결과로 named form의 필수 인자나 고정 순서를 축소하지 않는다. 인자 조합·타입·binding은 의미/버전 영역이며 실제 기능 실행 성공을 주장하지 않는다.

[CONTAINS syntax](https://learn.microsoft.com/en-us/sql/t-sql/queries/contains-transact-sql)와 [OPENROWSET BULK](https://learn.microsoft.com/en-us/sql/t-sql/functions/openrowset-bulk-transact-sql)의 문맥을 기준으로 삼으며, ORDER 예약은 일반 함수 이름에만 적용해 query hint·BULK option 양성을 보존한다.

full-text 검색식은 문자열 또는 @변수이며 LANGUAGE는 문자열·integer/hex·@변수를 허용한다. numeric 검색식·LANGUAGE 산술식·숫자 PROPERTY 이름은 실제102다. 엔진이 수용하는 qualified wildcard는 보존한다. catalog 이름·LCID domain 및 runtime 값은 검증하지 않는다.

수정 후 원래 토큰 변형에 남는23개 의미·문맥 관측은 [#141](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/141)에서 추적한다. 구문 수정의 완료와 의미 분석의 미완료를 구별한다.
