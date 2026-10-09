# Issue #151: EXEC OUTPUT 반환 인자의 local variable 경계

공개 corpus의 `EXEC dbo.p 1 OUTPUT;`은 SQL Server2025 LocalDB17.0.1000.7 compatibility110·170에서 오류179였지만 정상 CST와 빈 문맥 진단을 반환했다. 두 level 관측을 하나의 원인으로 추적했다. [EXECUTE 공식 규칙](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/execute-transact-sql?view=sql-server-ver17)은 constant OUTPUT 값을 허용하지 않고 반환 변수를 요구한다.

`CheckTSQLContext`는 `exec_argument`와 pass-through `execute_statement`의 OUT/OUTPUT 직전 값을 검사한다. Extra comment를 제외한 single-part field/identifier의 @ prefix를 네 encoding에서 판정하고 @@global은 제외한다. 변수 이름 전체를 복사하지 않고 prefix만 최대4bytes 읽는다. literal·DEFAULT·bare/qualified 이름·@@global은 `TSQL_EXEC_OUTPUT_VARIABLE`로 진단하며 위치는 원래 값의 byte/point 범위다. formal parameter OUTPUT, OUTPUT 없는 값, 동적 SQL string은 이 규칙에 포함하지 않는다. 변수 존재·scope·type·procedure catalog는 추정하지 않는다.

32개 공개 대조를 양 level에서 PARSEONLY로 검사하고 owned DB 정리를 확인했다. 16개 OUTPUT 값이179이고 qualified field1개는102 구문 오류였다. 정상 명시적 local variable, named 인자, comment·Unicode·최대 길이 변수, pass-through 및 formal OUTPUT을 보존했다. 선언 없는 implicit variable의137은 catalog/scope 관측이며 OUTPUT 값 자체의 위반으로 분류하지 않는다. 원격 명령은 실행하지 않았다.

기존694개와 새32개 합계726개의 actual native CST snapshot을 같은 parser identity에 묶었다. 독립 구문·문맥 기대, 입력 불변성·원래 범위와 ERROR CST 거부를 검사한다. hint 및 EXEC 정상/음성6개 대조는 UTF-8·CP949·UTF-16LE/BE에서 검사하고 기존 잘못된 encoding·parent·range·ERROR/missing 반례를 유지한다. grammar/source/generated pins는 변경하지 않는다.

전체 공개 corpus·mutation·대용량 문맥 API와 독립 리뷰·현재 head CI·main post-merge 결과는 실제 관측 후 기록한다. 이전 API 결과를 새 구현에 승계하지 않는다. 실제 SQL Server2012는 NOT_RUN이며 빈 진단은 전체 compile/execute 성공이나 모든 T-SQL 무결함의 증명이 아니다.
