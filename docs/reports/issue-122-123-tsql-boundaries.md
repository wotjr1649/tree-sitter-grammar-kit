# CTE 종료와 구분 함수 호출 경계 (#122·#123)

[#122](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/122)는 실제 선행 SQL 문장 뒤 CTE의 세미콜론 누락을 거부한다. [#123](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/123)는 schema 없는 구분 식별자 함수 호출을 거부한다. #121에서 남긴 중복 포함4개 원문 관측을 대상으로 시작했으며, normal/error 대조를 batch·block·중첩·IF/WHILE·TRY·procedure·DEFAULT·SELECT 문맥으로 확대했다.

## 엔진 대조와 해석

2026-10-07 기존 SQL Server2025 Express LocalDB17.0.1000.7의 task 소유 임시 DB에서 compatibility110·170을 각각 확인했다. 새 DB와 파일의 충돌을 먼저 검사하고 각 data/log 파일의 최대 크기를64/32MiB로 제한했다. 기존 DB는 변경하지 않았고 probe 종료 시 생성 DB를 제거했다. 연결과 명령은5초, 전체 process는180초 이하다. DDL은 PARSEONLY, 실제 실행은 검토한 고정 SELECT-only 입력으로 한정한다. integrated-auth의 local 연결을 쓰고 credentials·사용자 identity·raw SQL error message를 저장하지 않는다. 오류 number/state/severity/line/batch와 bounded SELECT 결과만 기록한다.

기본53개 입력의 PARSEONLY와 SELECT-only12개 실행은 설정마다65회, 합계130회다. 두 설정의 status·error·completed batch·rows가65/65 같았다. 확장16개 PARSEONLY 입력도 설정마다 같은 결과였다. `STRING_AGG` SELECT는 두 설정에서 모두 `a,b`를 반환했다. Microsoft의 [compatibility 계약](https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-database-transact-sql-compatibility-level?view=sql-server-ver17)과 [STRING_AGG 계약](https://learn.microsoft.com/en-us/sql/t-sql/functions/string-agg-transact-sql?view=sql-server-ver17)에 따라110은2012 engine 또는2012 기능 제한의 대체 증거로 사용하지 않는다. 사용자가2025의110·170 검증을 우선하고 실제2012 engine은 필요 시 추가하도록 확정했다. 추가 설치는 수행하지 않았다.

[CTE 계약](https://learn.microsoft.com/en-us/sql/t-sql/queries/with-common-table-expression-transact-sql?view=sql-server-ver17)의 preceding statement는 BEGIN 자체나 label을 뜻하지 않는다. 첫 CTE, label 뒤 CTE와 직접 IF/WHILE body는 수용한다. 실제 앞선 SELECT 또는 block/IF/ELSE 문장 뒤 `;` 없는 CTE는 오류319다. GO 뒤는 새 batch이고 table hint/OPTION의 WITH는 계속 수용한다.

`DEFAULT ([foo]())`·`[PERCENT]()`·`[ABS](-1)`·`[GETDATE]()`는 오류128, SELECT의 unqualified bracket call은102다. schema-qualified 구분 호출과 정상 built-in은 수용한다. 이 문법 경계를 invocation의 공통 reference에 적용하고 EXEC/implicit procedure target의 기존 object_reference는 유지한다. [함수 계약](https://learn.microsoft.com/en-us/sql/t-sql/statements/create-function-transact-sql?view=sql-server-ver17)의 schema-qualified scalar UDF 규칙과 맞춘다. bare 미정의 함수195와 미정의 변수137은 이름 해석이며 문법 오류로 바꾸지 않는다. PARSEONLY 수용은 객체 존재·타입·binding·실행 가능성의 증명이 아니다.

## 구현과 검증

문장 목록은 첫 CTE와 세미콜론 뒤 CTE를 허용하고 이어지는 일반 문장에는 CTE prefix를 허용하지 않는다. batch의 분리는 GO로 고정해 parser가 새 batch를 만들어 종료 요구를 회피할 수 없게 한다. visible batch/statement/object_reference 이름을 유지한다. bare table hint는 동적으로 우선해 qualified invocation과의 fork에서도 기존 `(NOLOCK)` 구조를 보존한다. END를 block 종료와 END CONVERSATION 시작으로 읽는 fork는 실제 lookahead로 결정한다. 기존 source를 고정 원본에서 literal C2 chain으로 만들며 generated parser를 직접 편집하지 않는다.

원래95개 원문과12개 보정 fixture 단계를 합친107개를 native와 두 compatibility level에서 다시 확인했다. 엔진214회 관측은 설정 간107/107 같았고 native107개는 완료·PASS, API 차이0이다. 원래 네 불일치(#122의1개·#123의중복 포함3개)는4→0으로 해소됐다.

기본 native53개는 완료·PASS이고 API 차이0이다. baseline의 엔진 불일치21개 중 대상14개(CTE8·구분 호출6)를 해결하고 정상 첫 CTE를 유지했다. 잔여7개 중195/137 다섯 개는 이름 해석 경계, DEFAULT naked/qualified 열 참조의128 두 개는 추가 문법 격차다. 확장 TRY/CATCH의 leading semicolon과 procedure body 소유권도 별도 검증 대상이다. 이를53개 전체 엔진 적합 PASS라고 표시하지 않는다.

새 fixture21개는 normal/error/restore14개와 label·IF·WHILE·block-leading-semicolon 정상4개, batch/module 첫 CTE와 bare hint/function 구분 정상3개다. SELECT 오류 fixture의 target batch는 그대로 두고 뒤 독립 batch를 추가해 재사용을 관측한다. 기존 fixture의 기대값은 변경하지 않는다. 정상 build인 이전 parser mutant를 새21개 fixture에 적용해 음성14개 모두 오류 단계1의 syntax expectation에서 실패하고 정상 대조7개는 PASS하는 것을 확인했다. 모든21개는 완료됐고 API·incremental route/equality·query equality·fact reproduction은 PASS다. root-cause 검출이 runtime 또는 route 실패에 의존하지 않는다. 등록 후 재생성, 전체 등록 회귀, mutant 결과와 main 통합 근거는 해당 Issue/PR의 정확한 후보 SHA에 결속한다.

최종 등록 회귀는 incremental686개와 oracle692개 모두 완료·PASS이며 oracle set valid, API 차이0, helper 실패0이다. baseline incremental665개·oracle671개와 대조해 기존 claim·문법 판정 차이0, 정상 tree 차이0을 확인했다. 비교한 incremental/fresh817개 tree projection 중 오류 recovery16개만 구조가 달라졌다. 기존 오류 판정·기대값·query/fact·route/equality claim은 유지된다. 재생성은 두 workspace의 generator/determinism PASS와 등록 reference6개 MATCH를 확인했고 원본 source는 변경되지 않았다. 생성 parser는67,700,075 bytes로 기존100MiB file/128MiB build cap 안에 있다.

전체 비공개 SQL corpus, large-input 전체 API 관측과 추가 격차 처분은 [#126](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/126)의 범위다. 비공개 입력·파일 이름·경로·hash는 ignored local evidence에만 보존하고 공개 결과는 count와 판정만 사용한다. 이 보고서의 합성 입력 대조가 그 확대 범위의 완료를 뜻하지 않는다.
