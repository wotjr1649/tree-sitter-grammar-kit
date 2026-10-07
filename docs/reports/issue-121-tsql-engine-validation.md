# SQL Server 2025 엔진 대조와 T-SQL 경계 보정 (#121)

[#121](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/121)은 #109 합성 재현과 #113·#114 경계를 실제 엔진으로 대조한다. **BEGIN 직후 첫 CTE는 유효하다.** #113의 거부 guard와 기대값을 수정하고 DEFAULT 예약어 시험의 정상 대조군을 schema-qualified 호출로 보정한다. #114 dynamic SQL 판정은 이 범위에서 엔진과 일치했다.

## 엔진과 실행 경계

2026-10-07 관측은 SQL Server 2025 Express LocalDB **17.0.1000.7**, Express Edition (64-bit), engine edition4, `MSSQLLocalDB`다. 사용자 요청으로 중지 인스턴스를 시작했다. 로컬 `master`, compatibility **170**, collation `SQL_Latin1_General_CP1_CI_AS`, `QUOTED_IDENTIFIER ON`, `ANSI_NULLS ON`에서 확인했다. 기존 .NET Framework4.8 compiler와 `System.Data.SqlClient`를 사용했다. 제품의 offline Go API에 DB 의존성을 추가하지 않는다.

각 사례는 pooling 없는 별도 integrated-auth 연결에서 실행한다. 연결·명령 상한은 각각 5초, 초기 probe 전체는 180초, 후속·보정 probe는 각각 120초다. 임의 SQL·주소·credentials를 받지 않는 고정 공개 합성 입력만 사용했다. 오류 number/state/severity/line/batch만 기록했고 계정·server name·raw error message는 기록하지 않았다.

DDL·BACKUP·RESTORE·key 입력은 [PARSEONLY](https://learn.microsoft.com/en-us/sql/t-sql/statements/set-parseonly-transact-sql?view=sql-server-ver17)로 확인했다. INSERT 대조군 두 개는 추가로 [NOEXEC](https://learn.microsoft.com/en-us/sql/t-sql/statements/set-noexec-transact-sql?view=sql-server-ver17) compile-only를 실행했다. 실제 실행은 소유 SELECT-only CTE와 dynamic SQL로 한정했다. 객체·backup·key를 생성하거나 변경하지 않았다. NOEXEC의 deferred name resolution과 PARSEONLY의 비컴파일 경계상 객체 존재·binding/type·실행 가능성을 증명하지 않는다.

GO는 검토한 합성 입력의 명시된 경계에서 client가 batch를 분할했다. 일반 GO lexer나 임의 script runner를 구현한 결과가 아니다. 뒤 batch가 실패해도 앞 batch의 SELECT 결과는 남을 수 있으므로 batch index와 completed batches를 함께 기록했다.

## 실제 결과

| probe | 입력·방식 | 관측 |
|---|---|---|
| 초기 | #109 23개 + #113 20 fixture ×3단계 =83 PARSEONLY | ACCEPT55 / REJECT28 |
| 초기 dynamic | 위 입력 중 SELECT-only 9개 실제 실행 | ACCEPT3 / REJECT6 |
| 후속 | 첫 CTE·앞선 실제 문장·qualified call 12개 PARSEONLY | ACCEPT8 / REJECT4 |
| 후속 실행 | SELECT-only 6개 + INSERT NOEXEC2개 | 실행 ACCEPT5 / REJECT1, compile ACCEPT2 |
| 보정 | 수정한 4 fixture의 원문·편집 후 12단계 PARSEONLY | 기대값12/12 일치: ACCEPT11 / REJECT1 |

합계 **124회**의 구문·실행·compile 검사다. 보정 전 원문 사례는95개이며 중복 정상·복원 단계와 방식별 실행은 별도 검사로 센다. 등록 T-SQL 전체의 엔진 적합성 검사가 아니다.

| 경계·대표 입력 | 엔진 관측 | parser·계약 처분 |
|---|---|---|
| `PERCENT;`, bare alias, `DEFAULT (dbo.PERCENT())` | 오류156 | 예약어 제한 유지 |
| Unicode 3.2 이후 regular identifier와 supplementary 문자 | 오류102 | 고정 Unicode 3.2 BMP 범위 유지; 구분 식별자는 허용 |
| BACKUP/RESTORE MASTER/SERVICE MASTER KEY `BY CERTIFICATE c` | 오류102 | PASSWORD 제한 유지; key 작업 미실행 |
| `BEGIN END;`, `BEGIN; END;`, 주석만 있는 block | 오류102 | 본문 `repeat1` 유지 |
| `BEGIN WITH c AS (SELECT 1 AS a) SELECT a FROM c; END;` | PARSEONLY 수용, 실제 SELECT `[[1]]` | 첫 CTE guard 제거 |
| 중첩 block의 첫 CTE | PARSEONLY 수용, 실제 SELECT `[[1]]` | 일반 statement 규칙으로 수용 |
| 첫 CTE INSERT 대조군 | PARSEONLY·NOEXEC 수용 | 정상 fixture로 보정; INSERT 미실행 |
| 앞선 SELECT 뒤 `;` 없는 CTE | 오류319, PARSEONLY·실제 실행 모두 거부 | 기존 과잉 수용 잔여 [#122](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/122) |
| `DEFAULT ([PERCENT]())` | PARSEONLY 오류128 | 잘못된 정상 대조군 보정; 호출 문맥 잔여 [#123](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/123) |
| `DEFAULT (dbo.[PERCENT]())` | PARSEONLY 수용 | 예약어 시험의 정상 대조군 |

[CTE 문서](https://learn.microsoft.com/en-us/sql/t-sql/queries/with-common-table-expression-transact-sql?view=sql-server-ver17)의 preceding statement 종료 요구를 BEGIN 자체에 확장한 #113 해석이 잘못됐다. 실제 첫 CTE 수용과 앞선 SELECT 뒤 오류319를 각각 확인했다. #109 보고서는 당시 관측을 보존하고 현재 해석 링크를 추가한다.

LF 끝을 포함한 대표 원문 SHA-256은 다음과 같다.

| 입력 | SHA-256 |
|---|---|
| `BEGIN WITH c AS (SELECT 1 AS a) SELECT a FROM c; END;` | `60033028e1ea3e4556fb677b6bfc5e3c29a8b23114e45820af1ba15c36974a3c` |

## dynamic SQL과 회귀 검증

batch 첫 `sp_executesql N'SELECT 1'`는 실제 `[[1]]`, explicit EXEC와 GO 뒤 호출은 `[[1],[2]]`를 반환했다. leading semicolon, non-first single/double semicolon, non-first comment/bracket 호출은 오류102였다. GO 다음 leading semicolon도 두 번째 batch에서 오류102이며 첫 batch `[[1]]`는 이미 반환됐다. native dynamic fact는 유효 호출3개에서 item 각1개, 잘못된6개에서 item0·known_miss0이었다. #114 fallback 오분류 수정과 일치한다.

C2 subject에서 첫 CTE 전용 helper·규칙·분기 세 operation을 제거하고 block 본문 필수 조건을 유지한다. #113 fixture20개와 편집·incremental anchor를 보존한다. DEFAULT는 qualified 정상→reserved 오류→qualified 정상, 첫 CTE 세 fixture는 정상→정상→정상으로 바뀐다. 현재14개 음성과6개 정상 편집 흐름이다. 정확한12 보정 단계는 엔진으로 먼저 확인했다.

고정 upstream/adoption source에서 subject chain을 적용하고 입력 hash를 갱신한다. tree-sitter0.27.0·Node24.21.0으로 두 workspace에서 만든 출력6개가 EQUAL이고 source snapshot은 변하지 않았다. 최초 bootstrap은 기준 부재 `REFERENCE_ABSENT`/BLOCKED여서 generator·determinism만 PASS였다. 그 출력을 등록한 뒤 정상 준비 절차로 다시 생성해 **reference MATCH6 / deterministic PASS**, source·runtime closure를 확인했다. 기존 reference checkout이나 생성 파일을 직접 수정하지 않았다.

Windows 로컬 등록 회귀는 **incremental665 / oracle671 모두 PASS**, set valid, API 차이0, helper 실패0이다. 의도한 네 fixture 밖의667개는 이전 main 후보와 tree·query·fact·선언·요구 결과가 같다. 같은95 원문을 보정 parser로 실행한 결과 불일치는11→4로 감소했다. 해결한7개 관측은 첫 CTE 거부였고, 잔여4개는 중복을 포함한 #122의1개와 #123의3개다. 추가로 정확한12 보정 fixture 단계를 native로 확인해 엔진과12/12 일치했다.

기존 첫 CTE guard를 갖는 정상 build를 mutant로 실행했다. 같은 세 fixture의 단계0·2는 각각 PASS이고 단계1의 `NO_ERROR` 단언만 `root has_error`로 FAIL이었다. 세 사례 모두 실행 COMPLETED, incremental equality와 route PASS여서 hash/build 거부나 비교기 완화로 검출한 결과가 아니다.

최종 local·hosted 회귀, 독립 리뷰와 actual main 통합은 #121 및 PR의 해당 후보 SHA·run에 결속해 기록한다. SQL Server2012 실제 엔진, compatibility110, 전체 SQL corpus, semantic/runtime 전반의 적합성은 미검증이다. [compatibility 문서](https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-database-transact-sql-compatibility-level?view=sql-server-ver17)와 별개로 이번 관측은170 하나다. #122·#123의 잔여를 이 결과로 해결된 것으로 처리하지 않는다.
