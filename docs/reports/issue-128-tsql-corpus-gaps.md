# T-SQL corpus 문맥 격차 (#128)

[#128](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/128)은 [#126 확대 검증](issue-126-expanded-validation.md)의 잔여 문법·문맥 처분을 소유한다. 기준 main은 `d553f9176968bfa5d460386d1afb240a7f75004f`다. 이 보고서의 공개 합성 관측과 최종 등록 회귀·전체 corpus·CI 완료를 구분한다. 비공개 원문·파일 이름·경로·hash는 공개하지 않는다.

## 구현과 합성 대조

SQL Server 2025 LocalDB `17.0.1000.7`, compatibility110·170에서 독립적으로 작성한 공개 합성 입력97개를 PARSEONLY로 대조했다. type·CASE·niladic function·변수·함수·숫자와 column reference, TRY/CATCH의 비어 있는 본문과 앞 terminator, column/table constraint, XML method, label/keyword 문맥을 구분한다. 초기69개는 NOEXEC 컴파일도 대조했다. 각 실행의 task 소유 database 제거를 확인했다. private corpus는 PARSEONLY로만 검사하며 임의 원문 실행·row·오류 message 수집을 하지 않는다.

| 경계 | 관측과 구현 |
|---|---|
| column/ALTER/CREATE DEFAULT | naked/qualified column reference 및 nested invocation·CAST·CASE·IIF의 column reference는128. literal·산술·함수·CAST/CONVERT·CASE·niladic function은 수용하며 constant 경로를 별도로 parse한다 |
| TRY/CATCH | 앞 세미콜론과 실제 문장은 수용. 빈 TRY와 세미콜론만 있는 TRY/CATCH는102. 빈 CATCH는 수용 |
| procedure body | 앞·뒤 세미콜론과 후속 문장을 body에 소유시키고 GO 이후 문장은 다음 batch에 둔다 |
| column constraints | 명시 FOREIGN KEY 목록과 PRIMARY KEY/UNIQUE ASC/DESC를 수용한다 |
| CREATE TABLE 마지막 쉼표 | 마지막 column 및 table constraint 뒤 쉼표를 수용. table variable의 엄격한 공유 목록을 유지 |
| typed expression | CONVERT/TRY_CONVERT의 delimited/sized type argument와 CAST 뒤 XML method를 수용 |
| RESTORE MOVE | 선언 변수의 logical/os file 인자를 parse. 실제 파일 복원은 실행하지 않음 |
| 문맥 keyword/label | WINDOW alias·MATCH column·ALTER PARTITION SCHEME 뒤 label을 구분 |
| FROM 없는 grouping | GROUP BY/HAVING을 parse. constant grouping은 PARSEONLY 수용 뒤 NOEXEC164이므로 의미 판정과 구분 |

공개97개 native 실행은 모두 COMPLETED/PASS이고 runtime API 차이는0이다. 새48개 normal/error/restore·semantic control과 procedure 소유권 query를 등록했다. 기존52개 gap fixture의 기대값은 넓히지 않았다. C2 subject를 append하고 관련 source/generated pin·qualification inventory를 함께 갱신했다. 실제 C2 chain을 적용한 두 workspace의 생성·source/generated hash·reference 대조는 PASS다. 최종 등록 검사는 incremental739개·Oracle746개 모두 COMPLETED/PASS, API 차이0·helper 실패0이다. Procedure ownership query의3개 step capture도 모두 PASS다. `FROM t WINDOW ...`의 named WINDOW 및 inherited WINDOW, 기존 OPENROWSET·OPENXML·PREDICT의 relation anchor를 함께 보존했다. JOIN·CROSS APPLY·OUTER APPLY의 독립 합성 입력도 API PASS이며 `window` alias의 소유는 JOIN → `relation`, CROSS APPLY → `apply_join`, OUTER APPLY → `apply_join`이다. 기존 r6 negative edit의 `PRIMARY KEY (Id ASC)`는 엔진이 수용하는 정상 SQL이므로 `ASCX`로 교정했다. 기존 `ERROR` 기대값과 comparator는 유지한다. CI 결과는 정확한 PR/main 실행 근거로 별도 기록한다.

## 기준 corpus의 잔여 처분

비공개6,150개 기준의 engine ACCEPT/native ERROR16개는 column FOREIGN KEY 목록4개, column PRIMARY KEY ASC/DESC9개, table constraint 뒤 쉼표1개, CAST 결과 XML method1개, RESTORE MOVE 변수1개다. 원래 파일별 evidence는 로컬에서 source identity와 결속한다.

binding-only/native ERROR6개 중911을 가진2개는 XML method 및 column constraint 문맥과 함께 관측됐다.195를 가진4개는 colon client parameter가 각각9·9·24·12개 native ERROR 지점이었다. 합성 `SELECT foo(1) WHERE a = :p;`는195, 알려진 함수 대조 `SELECT ABS(1) WHERE a = :p;`는102다. 앞선 함수 binding 오류가 뒤쪽 잘못된 client parameter 구문을 가릴 수 있으므로195-only를 정상 SQL로 보지 않는다.

engine REJECT/native NO_ERROR12개 중111을 가진10개는 module/batch placement이며178·911의 동반 의미 오류를 별도로 보존한다. 혼합102/137/156 1개는 같은 실제 batch의 두 번째 module header에서156이 시작하고 뒤쪽102·137이 동반된다. 그 header 위치와 source hash를 로컬에서 확인했다. 독립 합성 두 `ALTER PROCEDURE` header의 동일 batch 대조도 양 level156/native NO_ERROR·API PASS다.174 1개는 함수 arity다. 이 집합을 모두 grammar ERROR로 만들어 비율을 맞추지 않는다. SQLCMD25개는 client semantics의 미검사 상태로 유지한다.

공개 upstream은 `meloncholera/tree-sitter-mssql@8620fbcfca9438e1ff7104835bcb8305aba3bdfa`의 corpus351개·SQL162개, 총513개다. 기준 ACCEPT/native ERROR7개 중 CONVERT type argument2개·GROUP BY without FROM1개·WINDOW alias1개·MATCH column1개·partition label1개가 구현 대조 대상이다. 뒤쪽 UNION INTO1개는 PARSEONLY 수용 뒤 독립적인 literal 대조의 NOEXEC196으로 확인한 기존 구조 거부다. 최종 native513개는 COMPLETED/PASS, API 차이0, NO_ERROR486·ERROR27이다. 전체513개 upstream의 기준 native ERROR35개 중8개가 해소돼27개가 됐고 새 syntax regression은0이다. 이8개는 위 engine ACCEPT 격차에서 해소한6개와 REJECT의 binding 문맥2개를 합한 집합이며, ACCEPT/native ERROR7개와 denominator가 다르다. 두 level 모두 ACCEPT/native ERROR는 뒤쪽 UNION INTO1개다.110 ACCEPT/native NO_ERROR348·REJECT/native NO_ERROR132·REJECT/native ERROR26,170은352·128·26이다. SQLCMD6개는 engine 미검사다. REJECT/native NO_ERROR의132·128개는 아래 source별 표에 binding/module/version/음성 alternative로 개별 처분한다. 기준130·126개에 추가된2개는 type/constraint gap 해소 후에도 남는911 및137 binding 문맥이다. OPENJSON130+·REGEXP_LIKE170·확장TRIM160·WINDOW160의5개 설정 차이를 일반 parser 결함으로 세지 않는다.

위8개 native ERROR → NO_ERROR 전이의 공개 source와 engine status는 다음과 같다.

- `test/fixtures/table_bracketed_types_and_index.sql`: engine 110 REJECT(911), 170 REJECT(911); native ERROR → NO_ERROR.
- `test/fixtures/table_with_defaults_and_check.sql`: engine 110 ACCEPT, 170 ACCEPT; native ERROR → NO_ERROR.
- `test/corpus/alter_table.txt:2`: engine 110 ACCEPT, 170 ACCEPT; native ERROR → NO_ERROR.
- `test/corpus/errors.txt:14`: engine 110 ACCEPT, 170 ACCEPT; native ERROR → NO_ERROR.
- `test/corpus/errors.txt:28`: engine 110 ACCEPT, 170 ACCEPT; native ERROR → NO_ERROR.
- `test/corpus/errors.txt:29`: engine 110 ACCEPT, 170 ACCEPT; native ERROR → NO_ERROR.
- `test/corpus/errors.txt:30`: engine 110 ACCEPT, 170 ACCEPT; native ERROR → NO_ERROR.
- `test/corpus/expressions.txt:17`: engine 110 REJECT(137), 170 REJECT(137); native ERROR → NO_ERROR.

## 검증의 범위

2012/compat110부터2025/compat170까지의 source 합집합을 유지한다. 실제2012 engine은 실행하지 않았으며 상위 엔진의110 관측으로 대체했다는 주장을 하지 않는다. 별도 설치는 포함하지 않는다. native 상한·core CGO0·별도 native process 경계 및 large-input API의 기존 전체 receipt 계약을 유지한다. 최종 CI는 PR synthetic SHA와 실제 merge/main SHA를 별도로 확인해야 한다.

근거 계약: [CREATE TABLE DEFAULT](https://learn.microsoft.com/en-us/sql/t-sql/statements/create-table-transact-sql?view=sql-server-ver17), [TRY/CATCH](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/try-catch-transact-sql?view=sql-server-ver17), [오류164·196 및 binding](https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/database-engine-events-and-errors-0-to-999?view=sql-server-ver17), [compatibility](https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-database-transact-sql-compatibility-level?view=sql-server-ver17).

## 전체 비공개 SQL 최종 관측

현재 SQL 전체 집합6,150개와 모든 size/SHA256을 독립 열거로 재확인했고, 최종 생성 parser로6,150개·13개 batch를 다시 처리했다. COMPLETED/PASS6,150, fatal/not-run/requeue0, native NO_ERROR6,107·ERROR43이다. 기준6,088/62에서19개 ERROR가 해소됐다.16개 engine ACCEPT gap 전부,911 혼합2개,111 module 문맥1개가 이에 해당한다.19개가 동일 원인이라는 주장은 하지 않는다. 원래 root 전체 inventory CLI는 UNREADABLE로 중단됐다. 기존 caller-supplied inventory를 그대로 신뢰하지 않고 현재 SQL 선택 집합과 전 파일 hash의 동일성을 독립 확인한 후 사용했으며 native command도 각 hash를 재검사했다.

엔진 원래 관측은 같은17.0.1000.7/110·170, 같은6,150개 source hash에 결속된 설정별19,522개 batch의 PARSEONLY 결과다. 입력이 바뀌지 않았음을 이번에 확인해 최종 native 결과와 대조했으며 전체 private engine probe를 새로 실행했다는 주장은 하지 않는다. 두 level의 최종 비교는 같다.

| 설정별 engine/native 비교 | 파일 수 | 처분 |
|---|---:|---|
| ACCEPT / NO_ERROR | 4,232 | parse 수용 일치; 실행·binding 보증 아님 |
| binding-only / NO_ERROR | 1,854 | catalog/name/type 문맥 |
| binding-only / ERROR | 4 | colon client parameter;195가 뒤쪽 syntax를 가리는 대조 유지 |
| REJECT / NO_ERROR | 13 | 원래12개와 grammar gap이 해소된111 module 문맥1개; semantic/context 처분 |
| REJECT / ERROR | 22 | native 오류 판정 일치; 진단 동일성 주장은 하지 않음 |
| SQLCMD CLIENT_SKIPPED | 25 | native NO_ERROR8·ERROR17; client 미검사 |

이 native PASS는 처리 완료이며 모든 SQL의 엔진 적합성을 뜻하지 않는다. 비공개 전체6,150개의 모든 node API audit를 수행했다고 주장하지 않는다. API 범위는 등록/public/upstream Oracle와 별도 large-input 전체 audit다. 전체 corpus 관측은 최종 등록 source/generated hash와 실제 C2 chain의 reference 대조에 결속한다.

## 공개 upstream source별 처분

아래132행은 최종110 `REJECT/native NO_ERROR` 전체이고170의128개도 포함한다. 기준130·126개와 추가 binding 문맥2개를 구분한다. source는 위 pinned commit의 공개 경로와 corpus entry index로 식별한다. engine 입력 hash를 공개 source mapping과 전부 대조했다. `BINDING_RECOVERY`는137/195 뒤 syntax 진단이 함께 있다는 뜻이며 pure binding-only 성공으로 승격하지 않는다. `NEGATIVE_SYNTAX`·`NEGATIVE_BINDING`은 engine이 거부한 공개 alternative를 현재 parser가 수용한다는 명시된 한계다. upstream corpus의 parse 기대와 SQL Server 엔진 validator의 적합성을 구분하며 이 행을 정상 SQL 또는 수정 완료로 표시하지 않는다. VERSION은 설정 또는2012~2025 합집합의 역사적/플랫폼 경계이며 actual2012 실행 근거는 아니다. CLIENT는 SQLCMD 전처리다.

| 공개 source | 오류110 / 170 | 처분 | 근거 문맥 |
|---|---|---|---|
| `test/fixtures/alter_index_rebuild_reorganize.sql` | 11905 / 11905 | EDITION | resumable/online index 또는 full-text의 LocalDB/Express 제한 |
| `test/fixtures/at_time_zone.sql` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/cursor_temp_table.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/dbcc_freeproccache.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/deprecated_syntax_snippets.sql` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/dynamic_sql_try_catch.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/external_table.sql` | 102 / 102 | VERSION | HADOOP external data source의 제거된 alternative |
| `test/fixtures/for_xml.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/function_scalar_and_multi_statement_tvf.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/graph_match.sql` | 102 / 102 | NEGATIVE_SYNTAX | SHORTEST_PATH 반복 패턴 및 edge type alternation |
| `test/fixtures/inline_tvf.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/json_data_type.sql` | 134 / 134 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/json_object_array.sql` | 134 / 134 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/json_path_exists.sql` | 134 / 134 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/json_value_query.sql` | 134 / 134 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/merge_upsert.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/offset_fetch.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/opendatasource_changetable.sql` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/openjson.sql` | 102,134 / 134 | VERSION_BINDING | 110의 OPENJSON WITH 및 중복 변수134 |
| `test/fixtures/pivot.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/procedure_stub_then_alter.sql` | 111,178 / 111,178 | MODULE | module batch-first 또는 RETURN context; 동반 오류 별도 보존 |
| `test/fixtures/regexp_functions.sql` | 195 / ACCEPT | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/server_ddl_trigger_body.sql` | 1094 / 1094 | CONTEXT | server/database trigger 이름의 schema prefix 제한 |
| `test/fixtures/server_ddl_trigger_guarded_enable.sql` | 102,156 / 102,156 | MODULE | IF 다음 CREATE TRIGGER 및 module 뒤 ENABLE |
| `test/fixtures/sp_rename.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/string_agg_within_group.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/transaction_throw.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/trigger_instead_of_insert_enable.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/trim_functions.sql` | 102 / ACCEPT | VERSION | 160 미만의 extended TRIM |
| `test/fixtures/update_target_table_variable.sql` | 1087 / 1087 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/vector_type.sql` | 134,137 / 134,137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/vector_type_float16.sql` | 137,195 / 137,195 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/xml_dml.sql` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/fixtures/xml_methods.sql` | 134,137 / 134,137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/alter_table.txt:6` | 102 / 102 | NEGATIVE_SYNTAX | DROP NOT_FOR_REPLICATION의 underscored token |
| `test/corpus/comment.txt:4` | 111 / 111 | MODULE | module batch-first 또는 RETURN context; 동반 오류 별도 보존 |
| `test/corpus/control_flow.txt:0` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/control_flow.txt:1` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/control_flow.txt:2` | 137,156 / 137,156 | BINDING_RECOVERY | 미선언 IF 변수137 뒤 ELSE recovery156 |
| `test/corpus/control_flow.txt:3` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/control_flow.txt:4` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/control_flow.txt:5` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/control_flow.txt:6` | 137,156 / 137,156 | NEGATIVE_BINDING | 미선언 변수와 delimiter 없는 예약어 TABLE |
| `test/corpus/control_flow.txt:7` | 137,319 / 137,319 | BINDING_RECOVERY | 미선언 변수와 WITH recovery; pure binding-only로 단정하지 않음 |
| `test/corpus/control_flow.txt:8` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/control_flow.txt:9` | 137,178 / 137,178 | MODULE | module batch-first 또는 RETURN context; 동반 오류 별도 보존 |
| `test/corpus/control_flow.txt:10` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/control_flow.txt:12` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/create_function.txt:2` | 111,156 / 111,156 | MODULE | GO 없는 두 함수 정의 |
| `test/corpus/create_function.txt:5` | 111,156,319 / 111,156,319 | MODULE | GO 없는 두 함수 정의 및 WITH recovery |
| `test/corpus/create_index.txt:2` | 1712 / 1712 | EDITION | resumable/online index 또는 full-text의 LocalDB/Express 제한 |
| `test/corpus/create_index.txt:5` | 1712 / 1712 | EDITION | resumable/online index 또는 full-text의 LocalDB/Express 제한 |
| `test/corpus/create_index.txt:6` | 11905 / 11905 | EDITION | resumable/online index 또는 full-text의 LocalDB/Express 제한 |
| `test/corpus/create_table.txt:12` | 195 / 195 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/create_view.txt:0` | 156 / 156 | MODULE | GO 없는 두 view 정의 |
| `test/corpus/create_view.txt:3` | 156 / 156 | MODULE | ALTER VIEW 본문 뒤 DROP VIEW |
| `test/corpus/cte.txt:2` | 102,156 / 102,156 | NEGATIVE_SYNTAX | CTE를 괄호로 단독 wrapping |
| `test/corpus/cursor.txt:0` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/cursor.txt:2` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/delete.txt:1` | 1087 / 1087 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/delete.txt:3` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/delete.txt:4` | 102 / 102 | CONTEXT | DELETE target에서 허용하지 않는 NOLOCK hint |
| `test/corpus/execute.txt:0` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/execute.txt:1` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/execute.txt:2` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/execute.txt:3` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/execute.txt:4` | 102 / 102 | NEGATIVE_SYNTAX | EXEC argument/AS LOGIN의 literal alternative |
| `test/corpus/expressions.txt:0` | 4145 / 4145 | CONTEXT | 조건 식의 boolean/type context |
| `test/corpus/expressions.txt:7` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/expressions.txt:9` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/expressions.txt:16` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/expressions.txt:19` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/expressions.txt:21` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/expressions.txt:22` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/insert.txt:3` | 1087 / 1087 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/insert.txt:5` | 1087 / 1087 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/invocations.txt:1` | 195 / 195 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/invocations.txt:4` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/invocations.txt:6` | 1087 / 1087 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/json.txt:0` | 102,137 / 102,137 | BINDING_RECOVERY | 미선언 JSON key/value와 constructor alternative102 |
| `test/corpus/json.txt:1` | 137,319 / 137,319 | BINDING_RECOVERY | 미선언 변수와 WITH recovery; pure binding-only로 단정하지 않음 |
| `test/corpus/json.txt:2` | 137,319 / 137,319 | BINDING_RECOVERY | 미선언 변수와 WITH recovery; pure binding-only로 단정하지 않음 |
| `test/corpus/literals.txt:6` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/merge.txt:0` | 10713 / 10713 | NEGATIVE_SYNTAX | MERGE의 필수 마지막 semicolon 누락 |
| `test/corpus/merge.txt:3` | 1087 / 1087 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/merge.txt:4` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/misc_ddl.txt:3` | 102 / 102 | CONTEXT | CLEAR PROCEDURE_CACHE handle 위치의 변수 |
| `test/corpus/misc_ddl.txt:5` | 137,319 / 137,319 | BINDING_RECOVERY | 미선언 변수와 WITH recovery; pure binding-only로 단정하지 않음 |
| `test/corpus/misc_ddl.txt:6` | 911 / 911 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/misc_ddl.txt:9` | 137,319 / 137,319 | BINDING_RECOVERY | 미선언 변수와 WITH recovery; pure binding-only로 단정하지 않음 |
| `test/corpus/misc_ddl.txt:10` | 102 / 102 | VERSION | HADOOP external data source의 제거된 alternative |
| `test/corpus/misc_ddl.txt:14` | 343 / 343 | NEGATIVE_SYNTAX | 동일 ALTER EVENT SESSION의 ADD/DROP 혼합 금지343 |
| `test/corpus/misc_ddl.txt:15` | 137,156 / 137,156 | NEGATIVE_BINDING | 미선언 XML source 및 XML SCHEMA DROP IF EXISTS |
| `test/corpus/program.txt:2` | 111,137,178 / 111,137,178 | MODULE | module batch-first 또는 RETURN context; 동반 오류 별도 보존 |
| `test/corpus/security.txt:0` | 156 / 156 | MODULE | GO 없는 여러 CREATE SCHEMA |
| `test/corpus/security.txt:7` | 156 / 156 | NEGATIVE_SYNTAX | DROP SERVER ROLE IF EXISTS alternative |
| `test/corpus/security.txt:8` | 1020 / 1020 | CONTEXT | entity permission에 column-list 조합1020 |
| `test/corpus/security.txt:10` | 102,137,319 / 102,137,319 | MODULE_BINDING | AS CALLER/SELF 문맥 및 AS LOGIN 변수 |
| `test/corpus/security.txt:15` | 33161 / 33161 | VERSION | password 없는 master key의 engine/platform 제한 |
| `test/corpus/security.txt:16` | 102,319 / 102,319 | NEGATIVE_SYNTAX | SECURITY POLICY option의 위치/조합 |
| `test/corpus/select.txt:4` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/select.txt:5` | 156 / 156 | NEGATIVE_SYNTAX | delimiter 없는 예약어 column 이름 |
| `test/corpus/select.txt:10` | 156 / 156 | NEGATIVE_SYNTAX | 4-part object의 비지원 omitted segment |
| `test/corpus/select.txt:12` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/select.txt:25` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/select.txt:28` | 1018 / 1018 | NEGATIVE_SYNTAX | legacy table hint와 hint/column-list alternative |
| `test/corpus/select.txt:29` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/select.txt:30` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/select.txt:35` | 102,1018 / 102,1018 | NEGATIVE_SYNTAX | rowset function 뒤 table hint alternative |
| `test/corpus/select.txt:49` | 102 / 102 | NEGATIVE_SYNTAX | SHORTEST_PATH 반복 패턴 및 edge type alternation |
| `test/corpus/service-broker-and-partitioning.txt:1` | 156 / 156 | NEGATIVE_SYNTAX | message type의 delimiter 없는 DEFAULT |
| `test/corpus/service-broker-and-partitioning.txt:4` | 7609 / 7609 | EDITION | resumable/online index 또는 full-text의 LocalDB/Express 제한 |
| `test/corpus/service-broker-and-partitioning.txt:6` | 137,319 / 137,319 | BINDING_RECOVERY | 미선언 변수와 WITH recovery; pure binding-only로 단정하지 않음 |
| `test/corpus/service-broker-and-partitioning.txt:7` | 137,1087 / 137,1087 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/service-broker-and-partitioning.txt:8` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/service-broker-and-partitioning.txt:11` | 102 / 102 | NEGATIVE_SYNTAX | QUEUE REORGANIZE WITH의 괄호 누락 |
| `test/corpus/service-broker-and-partitioning.txt:12` | 102 / 102 | NEGATIVE_SYNTAX | ASSEMBLY ADD FILE AS의 identifier/string alternative |
| `test/corpus/service-broker-and-partitioning.txt:16` | 102 / 102 | NEGATIVE_SYNTAX | WITH 없는 빈 ALTER ROUTE |
| `test/corpus/service-broker-and-partitioning.txt:19` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/sqlcmd.txt:3` | 102 / 102 | CLIENT | SQLCMD variable substitution은 engine에 그대로 전달 |
| `test/corpus/subquery.txt:1` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/transaction.txt:3` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/triggers.txt:5` | 137,156 / 137,156 | MODULE_BINDING | GO 없는 trigger 두 정의와 미선언 변수 |
| `test/corpus/triggers.txt:6` | 102 / 102 | NEGATIVE_SYNTAX | 여러 ENABLE/DISABLE TRIGGER의 terminator 경계 |
| `test/corpus/triggers.txt:8` | 195 / 195 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/update.txt:3` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/update.txt:6` | 137 / 137 | BINDING | catalog/database/function/variable binding 및 동일 batch 변수 선언 |
| `test/corpus/window_functions.txt:1` | 10756 / 10756 | CONTEXT | window frame에 ORDER BY 누락10756 |
| `test/corpus/window_functions.txt:3` | 102 / ACCEPT | VERSION | 160 미만 WINDOW clause |
| `test/corpus/window_functions.txt:4` | 102 / ACCEPT | VERSION | 160 미만 WINDOW clause |
| `test/fixtures/table_bracketed_types_and_index.sql` | 911 / 911 | BINDING | column constraint gap 해소 뒤에도 남는 database/catalog 문맥 |
| `test/corpus/expressions.txt:17` | 137 / 137 | BINDING | typed CONVERT gap 해소 뒤에도 남는 undeclared variable 문맥 |

`EVENT SESSION` 단독 CREATE의 NOEXEC는 두 level에서 수용하지만 upstream의 동일 ALTER statement ADD/DROP 혼합은343이고 comma 추가 대조도156/343이다. [ALTER EVENT SESSION 계약](https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-event-session-transact-sql?view=sql-server-ver17)은 동일 statement의 ADD/DROP 혼합을 허용하지 않는다. 이를 LocalDB의 전체 EVENT SESSION 미지원으로 분류하지 않는다. HADOOP external data source 제거 경계는 [SQL Server의 discontinued 기능](https://learn.microsoft.com/en-us/sql/database-engine/discontinued-database-engine-functionality-in-sql-server?view=sql-server-ver17)에 따른다.
