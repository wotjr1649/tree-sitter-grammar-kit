# T-SQL 문맥 검사와 문맥별 구문 경계 (#141·#145·#146·#148)

#141의 23개 관측은 여섯 원인 계열이다. CTE 뒤 table source 없는 SELECT, scalar를 predicate로 사용한 조건, table hint 이름·충돌, COUNT/COUNT_BIG arity, scalar function의 빈 RETURN을 구문 수용과 분리해 검사한다. `CheckTSQLContext`는 원본 source와 complete/error-free CST를 입력받는 공개 offline Go API다. 원문·이름을 담지 않는 code, node index, 원본 range를 preorder로 반환하며 입력을 변경하거나 보관하지 않는다. 외부 process, network, DB, catalog를 사용하지 않는다. 세부 계약은 [공개 API](../specs/public-go-api.md)의 같은 함수가 소유한다.

## 구문 수정

#145는 공백으로 구분한 deprecated table hint를 보존하고, `WITH (NOLOCK NOWAIT)`를 rowset schema로 해석하지 않게 한다. schema는 OPENJSON/OPENROWSET/OPENXML/PREDICT의 shared rowset source에서만 FROM/APPLY로 접근한다. 일반 TVF의 schema 형태는 거부한다. 이름을 알 수 없는 plain hint는 CST의 table_hint_item으로 보존하고 문맥 검사에서 진단한다.

#146은 INDEX의 이름·정수 인자, FORCESEEK의 index/column 인자, SPATIAL_WINDOW_MAX_CELLS의 숫자 값 형태를 분리한다. parameter 없는 다른 hint에 인자·등호 값을 붙이지 않는다. scanner의 두 stateless token은 hexadecimal·fraction·exponent를 숫자와 새 hint로 쪼개지 않게 하면서 `1ROWLOCK`처럼 엔진이 수용한 인접 token을 유지한다. INDEX/FORCESEEK의 Integer token은 10자리 이내, 0..2147483647이고 quoted index 이름은 identifier로 보존한다. spatial 옵션의 긴 leading-zero numeric token은 구문 수용을 보존하며, 문맥 검사에서 31bit 범위와 FORCESEEK의 index 0을 검사한다. 기존 QUOTED_IDENTIFIER serialized state와 recovery guard는 유지한다.

WITH, deprecated bare hint, OPTION (TABLE HINT ...)는 같은 hint 인자 문법을 공유한다. 문맥 검사는 table_hint와 TABLE HINT query_hint의 direct table_hint_item을 모두 검사한다. 일반 query hint에는 이 규칙을 적용하지 않는다.

## 문서·엔진 충돌의 판정

Microsoft [table hint 문서](https://learn.microsoft.com/en-us/sql/t-sql/queries/hints-transact-sql-table?view=sql-server-ver17)와 [query hint 문서](https://learn.microsoft.com/en-us/sql/t-sql/queries/hints-transact-sql-query?view=sql-server-ver17)는 INDEX의 등호 뒤 값을 괄호로 표시한다. 실제 SQL Server 2025 LocalDB `17.0.1000.7`, compatibility110·170은 WITH 및 TABLE HINT의 `INDEX = (ix)`/`INDEX = (0)`을102로 거부하고 `INDEX = ix`/`INDEX = 0`을 수용한다. Microsoft/SqlScriptDOM commit `eaf3a6e8cf49350c7600bf70691de6314594ffba`의 TSql90/110/130/170 `indexTableHint`도 등호 뒤 identifierOrInteger를 요구한다. 따라서 [production 등록부](../../src/contracts/feature-alternatives.json)의 해당 세 문자열과 정상 fixture/anchor를 실제 형태로 정정한다. 기존 alternative ID와 NO_ERROR 기대는 유지하고 괄호형 음성 대조군을 추가한다. 원문 문서의 표기를 엔진 실행 성공 근거로 취급하지 않는다.

문서는 READCOMMITTEDLOCK을 granularity hint에도 넣지만 이번 엔진은 PAGLOCK/ROWLOCK/TABLOCK/TABLOCKX와 병용을 수용했다. 문맥 검사는 이 네 정상 조합을 거부하지 않는다. isolation 충돌과 NOLOCK/UPDLOCK/XLOCK 및 FORCESEEK/FORCESCAN 충돌은 실제 관측대로 검사한다. synonym/동일 hint 반복은 보존한다. 19개 parameterless hint의 171개 서로 다른 조합을 양쪽 순서로 엔진에서 대조했으며 방향 차이는 없었다. 이는 모든 query plan/configuration의 충돌 표가 아니다.

## 최종 리뷰의 경계 (#148)

QUOTED_IDENTIFIER OFF에서는 fulltext 검색식·PROPERTY 이름·LANGUAGE의 double-quoted literal을 보존하고 ON에서는 같은 입력을 거부한다. CREATE SECURITY POLICY의 SCHEMABINDING을 ALTER의 STATE 규칙으로 전파하지 않는다. CREATE QUEUE activation은 procedure/readers/execute 세 항목을 요구하며 선택 STATUS와 모든 순서를 보존한다. ALTER의 DROP은 단독 대안이고 일반 activation 목록과 섞지 않는다. 네 activation 항목은 중복 없이 구성한다. CREATE ROUTE는 ADDRESS 한 개를 요구하고 ALTER의 부분 변경과 option 순서를 보존하며 중복 ADDRESS를 거부한다.

OPENROWSET의 semicolon은 두 번째 연결 인자의 datasource/user/password 묶음에만 속한다. 일반 provider 호출에는 schema를 붙이지 않고 BULK의 column schema는 유지한다. OPENXML은 comma 인자와 XPath column/table schema, PREDICT는 comma 인자와 필수 결과 column schema, OPENJSON은 comma 인자와 column schema를 각각 소유한다. OPENXML의 AS JSON/NULL 및 PREDICT의 JSON path/AS JSON은 전파하지 않는다. FROM/APPLY는 같은 provider 규칙에 접근한다.

`REGEXP_LIKE`는 unqualified boolean built-in으로 검사하며 NOT·AND/OR·괄호의 predicate 경로도 보존한다. scalar REGEXP_COUNT와 qualified UDF는 이 판정을 공유하지 않는다. 이는 버전별 built-in 존재·type·arity·실행 성공 검사가 아니다.

Extended Events의 EVENT/TARGET/ACTION은 Microsoft/SqlScriptDOM의 `nonEmptyThreePartObjectName`과 공식 syntax가 요구하는 정확한2/3segment를 쓴다. CREATE는 ADD EVENT가 필요하고 ALTER의 부분 변경은 보존한다. 현재2025의 PARSEONLY/NOEXEC는 bare event/action도 수용하므로 그 결과를 실제 등록·실행 성공으로 보지 않는다. 이 네 bare-name control은 문서/formal 문법과 엔진 관측의 충돌로 명시하며 일반 leading-dot 이름 수정의 반례와 구분한다. server-scoped event session 생성은 실행하지 않았다.

이 경계는 [CREATE/ALTER QUEUE](https://learn.microsoft.com/en-us/sql/t-sql/statements/create-queue-transact-sql), [CREATE EVENT SESSION](https://learn.microsoft.com/en-us/sql/t-sql/statements/create-event-session-transact-sql), [PREDICT](https://learn.microsoft.com/en-us/sql/t-sql/queries/predict-transact-sql), [OPENXML](https://learn.microsoft.com/en-us/sql/t-sql/functions/openxml-transact-sql), [REGEXP_LIKE](https://learn.microsoft.com/en-us/sql/t-sql/functions/regexp-like-transact-sql)의 공개 syntax와 합성 입력의110·170 관측으로 고정한다. 설치 엔진의 PARSEONLY/NOEXEC 대조는 원문 실행 성공을 보증하지 않는다.

## 검증과 한계

공개 native snapshot 502개는 parser identity와 source hash를 결속하고 정상·구문 오류·각 진단·정상 sibling을 검사한다. UTF-8, UTF-16LE/BE, CP949, ERROR/missing/has-error tree, range/parent 오류와 UTF-16 홀수 경계도 검사한다. snapshot과 등록 parser SHA가 다르면 시험이 실패한다. 추가 회귀의50개 원래 시나리오는 공백 hint·rowset schema·정수/숫자 token·인자 음성을 다루고 정상→음성→복구 편집을 포함한다. 추가 #148의167개 원래 시나리오는 CREATE/ALTER option, fulltext QUOTED_IDENTIFIER, provider separator/schema, REGEXP_LIKE를 다루며 구문 음성은 정상→음성→복구를 포함한다. 기존50개 SELECT 시나리오는 B01/B02로 정정하고 기존292개 객체의 서식을 보존했다. 기존 route-value 음성 한 개에는 편집 밖의 정상 batch를 추가해 ERROR tree에서 실제 재사용을 관측하게 했다. 전체217개 새 시나리오는61개 등록 그룹에 담아 S05 993개/S06 1000개와 기존 MaxCases1000·MaxEdits4 상한을 지킨다. 정상 입력 묶음은 모두 NO_ERROR여야 하며 기존 contains마다 원문 위치의 anchor를 유지한다. 음성 묶음은 두 사례를 각각 단독 ERROR→복구로 검사하므로 한 오류가 다른 음성을 숨기지 않는다. 독립502개 native snapshot도 그대로 유지한다. 실제 오류 tree에서 reuse0인 여덟 사례는 편집 밖의 정상 batch를 추가해 별도8개 대조군에서 equality·expectation·route 모두 PASS를 확인했다. 비교기·오류 기대·HasError 처리와 상한은 바꾸지 않는다. #149에서 기본502개와 새147개를 합친649개 실제 native 기록을 확인했고 API 차이와 구문·문맥 기대 차이가 모두0이다. #149 등록 시 기존 task 음성36개를18개 독립 pair로 묶어 새147개 시나리오를 담으면서 같은993/1000 및 MaxEdits4 상한을 유지했다. 이 후속 regrouping은 앞서 기록한292개 보존 이후의 별도 변경이며 각 음성은 단독 ERROR→복구로 검증한다. 전체 등록 S05/S06, 공개 corpus, 기존 token mutation, core checks, 세 OS CI와 qualification은 최종 후보의 완료 근거를 각각 확인해야 한다.

빈 diagnostics는 이 함수가 소유한 제한된 규칙에서 위반을 찾지 않았다는 뜻이다. parser NO_ERROR와 의미 진단을 동일한 claim으로 합치지 않는다. catalog/name binding, type resolution, collation, permission, configuration, engine version, plan/runtime 성공과 모든 가능한 T-SQL의 무결함은 보장하지 않는다. 구문 합집합에서는 OPENJSON 같은 높은 버전 기능이 compatibility110 엔진에서 거부될 수 있다. 실제 SQL Server2012 엔진은 NOT_RUN이다. 비공개 원문·경로·세부 관측은 이 공개 보고서에 포함하지 않는다. main 통합은 최종 candidate의 review·CI·post-merge 검증 후 별도로 판정한다.
