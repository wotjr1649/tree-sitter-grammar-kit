# T-SQL 과잉 수용 원인별 수정 (#131)

## 조사 범위

[이전 corpus 보고서](issue-128-tsql-corpus-gaps.md)의 `NEGATIVE_SYNTAX` 18행을 공개 upstream `meloncholera/tree-sitter-mssql@8620fbcfca9438e1ff7104835bcb8305aba3bdfa`에서 hash로 식별했다. SQL Server 2025 LocalDB `17.0.1000.7`의 compatibility110·170에서 원래 18개와 독립적인 최소 음성·정상 대조를 PARSEONLY로 검사했다. 공개153개와 MERGE 경계18개, review 대조34개를 두 level에서 검사해 합계410개 관측을 확보했다. 두 level에서 원래18개 모두 REJECT이며 task 소유 database 제거를 확인했다. catalog·변수·출력 인자 오류를 순수 구문 오류로 바꾸지 않는다. 실제2012 engine 관측은 아니다.

## 원인과 처분

| 원인 | 공개 source / corpus entry index | 관측 수 | 수정 또는 잔여 처분 |
|---|---|---:|---|
| 예약어 문맥 | `test/corpus/select.txt:5`, `:10`, `test/corpus/service-broker-and-partitioning.txt:1` | 3 | bare KEY·DEFAULT·SCHEMA를 identifier로 수용하던 경로를 제한하고 `[KEY]`·`[DEFAULT]`·`[SCHEMA]`는 유지한다. contract message의 bare DEFAULT 특별 허용도 제거한다. |
| CTE 괄호 | `test/corpus/cte.txt:2` | 1 | standalone CTE 또는 WITH 부분을 괄호로 감싸는 경로를 제거한다. SELECT 괄호·정상 CTE·subquery는 유지한다. |
| 문장 경계 | `test/corpus/merge.txt:0`, `test/corpus/triggers.txt:6` | 2 | standalone MERGE는 후행 세미콜론을 요구한다. ENABLE/DISABLE TRIGGER는 문장 목록 첫 위치 또는 선행 terminator 뒤에서 시작한다. |
| EXEC 인자 | `test/corpus/execute.txt:4` | 1 | procedure 인자의 `+3`과 dynamic EXEC의 `AS LOGIN = @variable`을 제한한다. `-3`·DEFAULT·문자열 principal과 기존 동적 SQL 사실 계약은 유지한다. |
| Graph 경로 | `test/fixtures/graph_match.sql`, `test/corpus/select.txt:49` | 2 | edge alternation과 괄호 없는 SHORTEST_PATH 반복을 제거한다. 괄호 안 path의 `+`·bounded 반복·역방향은 유지한다. |
| FROM hint | `test/corpus/select.txt:28`, `:35` | 2 | bare hint는 단일 hint로 제한하고 일반 함수 relation에는 table hint를 붙이지 않는다. WITH 복수 hint·함수 alias/column list·rowset schema는 유지한다. |
| DDL keyword / 인자 종류 | `test/corpus/alter_table.txt:6`, `test/corpus/security.txt:7`, `test/corpus/service-broker-and-partitioning.txt:12` | 3 | ALTER COLUMN의 NOT_FOR_REPLICATION 오탈자와 DROP SERVER ROLE IF EXISTS를 제한한다. ASSEMBLY bare file_name은 공식 예제와 충돌하므로 아래 별도 처분으로 보존한다. 문자열 file_name과 숫자 literal은 구분한다. |
| DDL body / option | `test/corpus/misc_ddl.txt:14`, `test/corpus/security.txt:16`, `test/corpus/service-broker-and-partitioning.txt:11`, `:16` | 4 | ALTER EVENT SESSION의 이종 ADD/DROP 조합, QUEUE 옵션의 생략된 괄호, 빈 ALTER ROUTE를 제한한다. SECURITY POLICY의 NOT FOR REPLICATION은 아래 문서/엔진 불일치로 별도 처분한다. |

`select.txt:10`의 기존 “생략 object segment” 설명은 수정한다. `d..t`, `s.d..t`, `s..dbo.t`, `s...t`는110·170 모두 수용됐다. 원래 입력의 `srv..schema.t`에서 bare SCHEMA가156을 유발하며 `[schema]`로 바꾼 대조는 수용됐다. 생략 segment 자체를 금지하지 않는다.

MERGE는 hidden external guard로 바로 다음 세미콜론을 요구하며 실제 token은 기존 문장 목록이 소비한다. 다음 CTE/trigger의 선행 terminator 문맥과 visible `merge` 본문 범위를 유지한다. OUTPUT을 쓰는 composable MERGE row source에는 standalone terminator를 요구하지 않는다. ENABLE/DISABLE의 제한은 **다음 trigger 문장 앞**에 적용한다. trigger 뒤 SELECT·END CONVERSATION, IF/ELSE의 body, label 뒤, block 종료, GO는 별도 정상 대조를 유지한다. trigger는 기존 CTE head/tail 경로를 재사용한다. MERGE guard는 기존 scanner enum 뒤에 토큰 하나를 추가하며 QUOTED_IDENTIFIER의1-byte serialized state는 그대로다.

## 문서와 실제 엔진의 불일치

아래는 #131 완료 당시의 보류 기록이다. SECURITY POLICY의 후속 조사·구현 처분은 [#132 erratum 보고서](issue-132-security-policy-erratum.md)가 대체한다. ASSEMBLY 보류는 #134에서 독립적으로 추적한다.

`ALTER SECURITY POLICY p NOT FOR REPLICATION;`은110·170 모두102이며 관련 조합도102/319다. 그러나 [Microsoft ALTER SECURITY POLICY 문서](https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-security-policy-transact-sql?view=sql-server-ver17)는 해당 suffix를 명시한다. 현재 지원 계약은2012~2025 source 합집합이다. 이 관측만으로 문서상 문법을 삭제하거나 “수정 완료”로 표시하지 않는다. engine-version 또는 문서 오류를 확인하는 [후속 Issue #132](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/132)로 추적한다.

ASSEMBLY의 `ADD FILE ... AS PointSource`는110·170에서102지만 [Microsoft CLR 예제](https://learn.microsoft.com/en-us/sql/relational-databases/clr-integration/assemblies/altering-an-assembly?view=sql-server-ver17)가 bare 형태를 명시한다. ADD/DROP file_name의 bare·문자열 형태를 합집합 계약에 유지하고 [후속 Issue #134](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/134)로 engine·문서 revision 차이를 추적한다. 따라서 원래18개 중 순수 구문16개를 수정하고 문서/엔진 불일치2개를 보존한다.

자동 리뷰의 trigger 지적은 `PRINT N'x'`·SET·RETURN 뒤 ENABLE이 세미콜론 없이102/156이고 세미콜론 대조는ACCEPT인 실측으로 반박한다. 확인된 추가 구문 경계는 빈 DML VALUES, 복합 대입 DEFAULT, OPENJSON의 세미콜론 인자다. VALUES는1개 이상을 요구하고 DEFAULT는 column의 단순 대입에 허용한다. bare @variable 직접 DEFAULT는156이며 `@variable = column = DEFAULT`는두 level 모두ACCEPT이므로 별도 capture 형태로 유지한다. 구분 column `[@v]`와 qualified column의 DEFAULT는 유지한다. OPENJSON은 comma-only 호출로 OPENROWSET의 연결 문자열 separator와 구분한다.

## 검증 계약

등록 회귀는 정상 → 음성 → 복구의 구문 기대, 독립적인 앞 문장의 재사용, incremental/fresh ordered CST, 전체 작은 API와 fact 재현을 검사한다. 추가 EOF edit는 trigger 뒤 정상 SELECT 삽입, 무구분 다음 DISABLE로의 변경, 세미콜론 복구, 원래 source 복원을 검사한다. 작은 단독 trigger의 EOF 진단에서는 equality/API는 통과했지만 노드 재사용을 관측하지 못해 incremental-route gate가 FAIL이었다. 이를 PASS로 승격하지 않고 안정된 앞 문장이 있는 등록 회귀로 대조한다.

C2 literal patch·source input·generated output의 hash/size를 함께 등록한다. 기존 adoption과 다른25개 route는 변경하지 않는다. canonical support claim은 현재 후보의 필수 CI·독립 리뷰·actual main 검증 이후에만 갱신한다. 상한·expectation comparator·API 허용 차이는 완화하지 않는다.

자동 review finding을 반영한 최종 후보는 새 회귀62개를 등록했다. 실제 등록2회 생성의 deterministic/reference/js reproduction, CGO_ENABLED=0 core test/vet/build, S05 801개·S06 808개, 공개 대조205개+upstream513개 Oracle/API/fact718개가 모두PASS이며 failures/API/requirements0이다. 독립 정적 리뷰의 BLOCKER/MATERIAL0이고 feature tag8개 보정을 inventory에 반영했다. 기존 main의 upstream513개와 source hash가 모두 일치하며, 원래 순수구문16개와 같은 trigger 경계의 기존REJECT1개를 추가ERROR로 검출하고 정상WINDOW2개를 복구했다. 최종 NO_ERROR469/ERROR44, engine ACCEPT의 새ERROR0이다. 이 수치는 모든 engine REJECT가 구문 결함이라는 주장은 아니다.

초기 head `866a459`의11개 PR CI job과 qualification/large API/실제 진단 artifact는 PASS였지만 변경된 후보의 성공으로 승계하지 않는다. 최종 PR 및 actual main의 동일 검증은 해당 commit의 CI와 판정artifact로 별도 확인한다. private6,150개 재검사·private 전체-node API·SQLCMD client 실행·실제2012 engine은 이번 후보 범위에서 미실행이다.
