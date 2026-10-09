# T-SQL 잔여 corpus 문맥 경계 (#149)

공개 corpus의 엔진/parser 재집계에서 DELETE target, CLEAR PROCEDURE_CACHE handle, column permission, window frame, DDL trigger 이름의 다섯 계열을 최소 입력으로 다시 대조했다. 원래 혼합 trigger의1094는 event group이나 EXECUTE AS CALLER가 아니라 DDL trigger 이름의 schema qualifier에서 발생했다. CALLER·SELF 정상과 DDL OWNER1083·DML OWNER 정상을 별도 확인했다.

## 구문과 고정 문맥

쓰기 대상은 WITH hint를 요구한다. 일반 invocation을 그대로 두면 bare hint가 함수 호출로 재해석되므로 DELETE/UPDATE의 TVF 호출 인자도 literal·local variable·DEFAULT 및 signed literal로 한정한다. 숫자/변수 인자의 갱신 가능한 inline TVF는 owned fixture NOEXEC에서 실제 수용했다. FROM relation의 deprecated bare hint와 일반 expression 호출에는 이 인자 제한을 적용하지 않는다. OPENROWSET provider와 OPENQUERY는 provider별 인자 형태를 보존한다. OPENROWSET BULK를 쓰기 대상으로 일반화하지 않는다.

CLEAR PROCEDURE_CACHE는 handle 없는 형태와 binary literal을 보존하며 variable·다른 literal을 거부한다. 다음 label/SELECT를 handle로 삼키지 않는다. 모든 옵션을 varbinary expression으로 넓히지 않는다.

고정 column permission·window frame·trigger 문맥은 기존 `CheckTSQLContext`에 추가한다. 직접 대상 hint와 읽기 relation, permission별 column list와 전체 column list, window의 직접 ORDER BY와 nested subquery ORDER BY, DML과 DDL trigger를 구분한다. code·원본 node/range만 출력하는 계약과 encoding·tree guard는 유지한다. 각 진단의 정확한 소유 범위는 [공개 API](../specs/public-go-api.md)가 규정한다.

Microsoft [DELETE](https://learn.microsoft.com/en-us/sql/t-sql/statements/delete-transact-sql), [GRANT object permission](https://learn.microsoft.com/en-us/sql/t-sql/statements/grant-object-permissions-transact-sql), [OVER](https://learn.microsoft.com/en-us/sql/t-sql/queries/select-over-clause-transact-sql), [CREATE TRIGGER](https://learn.microsoft.com/en-us/sql/t-sql/statements/create-trigger-transact-sql), [EXECUTE AS](https://learn.microsoft.com/en-us/sql/t-sql/statements/execute-as-clause-transact-sql)의 syntax와 SQL Server2025 LocalDB17.0.1000.7의110/170 관측을 대조한다. PARSEONLY가 수용한 RANGE offset·short FOLLOWING은 NOEXEC의4194/4193을 구분하며, 갱신 불가능한 TVF의4406은 catalog/결과 형태와 관련된 의미 오류다. 설치·permission 변경·외부 provider 실행·server trigger 생성은 하지 않는다. provider 합성 입력은 PARSEONLY로만 검사한다.

## 검증 범위

합성 대조147개와 기존502개 snapshot을 새 parser/source identity로 확인한다. normal/negative sibling, 반복·중첩·named window 대조를 포함하며 기대값은 실제 engine 오류 번호와 canonical 문맥 규칙에 연결한다. 649개 실제 native 기록은 구문·문맥 기대 및 API 차이0을 확인했다. C2 literal patch·source/생성 pins는 같은 후보로 갱신했고 공식 deterministic/reference 재생성 및 CGO0 core·vet·build·module 검증을 통과했다. 기존 음성36개를18개 pair로 묶고 새34개 구문 음성을17개 pair,113개 정상 구문을1개 그룹으로 등록해 S05 993개/S06 1000개·MaxEdits4 상한을 유지했다. 각 pair는 첫 음성만 ERROR→복구한 뒤 두 번째 음성만 ERROR→복구하며 편집 밖 정상 batch에서 실제 재사용을 관측한다. 독립649개 snapshot을 유지하며 전체 등록·qualification의 최종 결과는 별도로 확인한다. core·독립 정적 리뷰·전체 corpus·token mutation·large API·세 OS CI·post-merge는 각각 실제 관측 후 완료를 판정한다.

빈 diagnostics와 parser NO_ERROR를 전체 SQL Server 실행 성공으로 합치지 않는다. catalog/name binding, type·collation, event catalog, window name resolution, 함수별 frame/업데이트 가능 여부, engine version, 권한·계획·runtime은 이 제한된 offline API 밖이다. 실제 SQL Server2012는 NOT_RUN이다. 모든 가능한 T-SQL의 무결함을 보증하지 않으며 비공개 원문·경로·세부 결과는 이 공개 보고서에 포함하지 않는다.
