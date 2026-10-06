# T-SQL 과잉 수용 경계 수정 (#113)

[#109 보고서](issue-109-tsql-reclassification.md)는 당시 parser의 관측 기록이다. [#113](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/113)은 그 보고서의 다섯 과잉 수용 종류를 아래 범위에서 제한한다. SQL Server 엔진을 실행하거나 모든 T-SQL 문법의 적합성을 주장하는 작업은 아니다.

| 경계 | 거부하는 입력 | 보존하는 대조군 |
|---|---|---|
| 예약어 | `PERCENT;`, `AS PERCENT`, `DEFAULT (PERCENT())` | `[PERCENT]`, 옵션 단위 `SAMPLE 50 PERCENT` |
| regular identifier 문자 | Unicode 3.2 이후 문자·숫자와 supplementary 문자 | Unicode 3.2 BMP 문자·숫자, Hangul, 구분된 새 Unicode 식별자 |
| key PASSWORD 옵션 | BACKUP/RESTORE MASTER/SERVICE MASTER KEY의 `BY CERTIFICATE c` | 같은 위치의 `PASSWORD = '...'`, 다른 보안 문맥의 CERTIFICATE |
| block 본문 | `BEGIN END`, 주석만 있는 본문 | 비어 있지 않은 본문과 중첩 block |
| block의 첫 CTE | `BEGIN WITH ...`, 그 위치의 SELECT·INSERT와 중첩 block | `BEGIN; WITH ...` |

예약어는 [Microsoft 문서](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/reserved-keywords-transact-sql?view=sql-server-ver17)의 PERCENT 재현을 대상으로 한다. 기존 generic identifier를 쓰는 모든 예약어 문맥을 일괄 재작성하지 않는다. 예약 terminal을 global 및 column constraint 규칙에 등록하고, 유효한 옵션 단위에서만 기존 identifier node로 alias한다.

[regular identifier 규칙](https://learn.microsoft.com/en-us/sql/relational-databases/databases/database-identifiers?view=sql-server-ver17)에 따라 공유 문자 클래스를 Unicode 3.2 BMP Letter/Nd 범위로 고정했다. 범위는 Python 표준 라이브러리 `unicodedata.ucd_3_2_0`에서 도출했고 BMP 전체 65,536개 code point에 대해 start/continue를 대조했다. Unicode property escape의 최신 문자표를 사용하는 대신 고정된 범위를 grammar 입력에 기록한다. Python은 개발 시 도출에만 사용하며 제품 의존성으로 추가하지 않는다. 구분 식별자는 별도 token이므로 이 제한을 적용하지 않는다.

[BACKUP MASTER KEY](https://learn.microsoft.com/en-us/sql/t-sql/statements/backup-master-key-transact-sql?view=sql-server-ver17)와 [RESTORE MASTER KEY](https://learn.microsoft.com/en-us/sql/t-sql/statements/restore-master-key-transact-sql?view=sql-server-ver17)의 PASSWORD 위치에는 공용 encryption mechanism 대신 제한된 공유 규칙을 쓴다. 기존 `option`·`identifier`·`literal`·`encryption_mechanism` node와 field를 alias로 보존한다. CERTIFICATE를 허용하는 다른 보안 문장은 기존 규칙을 사용한다.

[BEGIN...END 규칙](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/begin-end-transact-sql?view=sql-server-ver17)에 따라 본문을 필수로 만들고, BEGIN 직후 세미콜론이 없는 첫 문장은 CTE 없이 시작하도록 한다. 일반 statement와 첫 statement의 JavaScript choice 구성을 공유하며, 첫 statement는 기존 `statement`로 alias한다. procedure·TRY/CATCH 등 다른 본문 문법이나 모든 CTE separator 위치를 새로 정의하지 않는다.

patch의 소유자는 `src/dev/c2-patches/tsql.json`이며, [source 등록부](../../src/contracts/language-sources.json), [native 등록부](../../src/contracts/native-routes.json), [재생성 등록부](../../src/contracts/reproduction-routes.json)가 입력과 출력 identity를 함께 고정한다. 고정 source와 기존 adoption 단계는 보존한다. 생성 파일은 직접 편집하지 않고 등록된 tree-sitter 0.27.0·Node 24.21.0으로 두 작업 공간에서 생성한다.

등록 fixture `src/testdata/native/gaps/tsql.json`에 정상→음성→정상 17개와 정상→정상→정상 3개를 추가했다. 수정 영역 밖의 SQL anchor를 보존해 기존 incremental reuse 단언을 실제로 검증한다. 기대값과 기존 645 incremental/651 oracle 사례는 확대하거나 제거하지 않는다. 수정 전 parser에서 17개 음성 기대가 실패하고 정상 대조군은 통과해야 한다. 수정 후에는 같은 fixture의 구문 기대, 증분/전체 tree, query, fact 재현을 대조한다. 상세 실행 결과와 main 통합 증거는 PR 및 Issue의 해당 commit/run 기록으로 확인한다.
