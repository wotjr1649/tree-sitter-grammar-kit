# Issue #150: TABLE 예약어와 XML schema collection 구문 경계

공개 합성 corpus의 잔여 두 불일치를 최소 입력으로 분리했다. `SELECT * FROM TABLE` 및 unquoted qualified TABLE·function name은 현재 엔진의156 오류인데 일반 identifier로 파싱됐다. `DROP XML SCHEMA COLLECTION IF EXISTS dbo.x`에도 잘못된 optional IF EXISTS가 있었다. XML collection 명령은 공통으로1/2part 이름만 허용한다.

## 구현과 정상 대조

기존 reserved 집합4개에 TABLE을 추가해 identifier와 함수 이름의 우회를 닫는다. explicit TABLE keyword, bracket/double-quoted identifier, @TABLE·#TABLE은 유지한다. XML collection 전용1/2part reference를 CREATE·ALTER·DROP에 alias하여 기존 object_reference CST 형태를 보존한다. 빈 schema와3part 이름은 거부하고 DROP의 IF EXISTS를 제거한다. 다른 DROP SCHEMA/TABLE의 IF EXISTS는 유지한다.

[예약어 규칙](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/reserved-keywords-transact-sql)과 [DROP XML SCHEMA COLLECTION](https://learn.microsoft.com/en-us/sql/t-sql/statements/drop-xml-schema-collection-transact-sql)·[CREATE XML SCHEMA COLLECTION](https://learn.microsoft.com/en-us/sql/t-sql/statements/create-xml-schema-collection-transact-sql)·[ALTER XML SCHEMA COLLECTION](https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-xml-schema-collection-transact-sql)을 현재 SQL Server2025 LocalDB17.0.1000.7 compatibility110/170의 실제 PARSEONLY 관측과 대조했다.44개 case(42개 source hash)에는 quoted 정상·함수·변수·temporary table·기존 DROP SCHEMA IF EXISTS·인접 statement 대조를 포함한다. 엔진 검증용 DB 정리를 확인했다.

`ALTER TABLE ... SET (LOCK_ESCALATION = TABLE)`은 TABLE 예약어의 정상 사용 위치다. 해당 옵션만 전용 토큰 우선순위로 AUTO·TABLE·DISABLE 값을 허용하며 기존 세 문장의 CST 앵커를 유지한다. bracket 옵션 이름·다른 옵션의 TABLE 값·SELECT의 TABLE 값은 음성 대조로 거부한다.

## 검증과 상한

기존649개와 새44개 및 기존 LOCK_ESCALATION 앵커 대조1개 합계694개의 actual native CST/API가 구문·문맥 기대를 통과했고 차이는0이다. 기존 정상 입력10개는 원문을 그대로 보존하여 GO로 분리한1개 그룹으로 묶었다. 새16개 고유 음성은8개 pair에 각각 ERROR→복구로 등록하고23개 고유 정상은 별도1개 그룹에 넣었다. 모든 pair에는 편집 밖 정상 batch를 두며 MaxEdits4, S05 993/S06 1000 상한을 유지한다. 각 원문의694개 독립 snapshot과 등록된 전체 native 검사를 함께 사용한다.

C2 patch와 source/generated pins를 같은 작업 단위로 갱신한다. 공식 deterministic/reference, core·독립 리뷰, 전체 corpus·token mutation·large API·3OS CI·qualification·post-merge 완료는 각각 실제 관측 후 기록한다. 이전 후보의 통과를 새 parser의 통과로 승계하지 않는다. 비공개 원문·경로는 공개하지 않으며 모든 T-SQL 무결함을 보증하지 않는다. 실제 SQL Server2012는 NOT_RUN이고 catalog/type/runtime은 이 구문 수정 밖이다.
