# T-SQL 한계 재분류 (#109)

이 보고서는 #109 당시의 관측·분류를 보존한다. `BEGIN WITH`를 과잉 수용으로 분류한 해석은 이후 [#121 SQL Server 엔진 대조](issue-121-tsql-engine-validation.md)에서 잘못됐음을 확인했다. 현재 경계는 [#113 보정 보고서](issue-113-tsql-boundaries.md)를 따른다.


2026-10-07(KST), 기준 main `fc0850074c7f0f1229dde7b54349b9278dc00aef`. [#109](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/109)의 분석·문서 범위다. S08의 과잉 수용 다섯 종류는 모두 남아 있다. 비첫 `;sp_executesql`이라는 포괄적 known-miss 설명은 입력 모양별로 나눈다. 단일 `;` 사례는 ERROR·사실 0·known miss 0이며 문법상 비첫 EXEC 생략 호출의 거부는 의도된 동작이다. `;;` 사례의 batch-first known-miss 오분류는 남아 있다.

## 관측 범위와 identity

Windows/amd64에서 Go1.27.1·MSYS2 GCC16.2.0으로 현재 C2-PATCH-r1 T-SQL parser와 #105의 검토된 runtime closure를 사용했다. source/tool/runtime identity 검사와 기존 runner의 hard memory·timeout·출력 제한을 유지했다. 합성 입력 23개만 파싱했으며 SQL Server 엔진은 실행하지 않았다. 이 관측을 임의 입력 또는 다른 OS에서의 추가 재현으로 확대하지 않는다.

- upstream grammar: `meloncholera/tree-sitter-mssql@8620fbcfca9438e1ff7104835bcb8305aba3bdfa`, 현재 채택/C2 chain은 [native registry](../../src/contracts/native-routes.json)와 `src/dev/c2-patches/tsql.json`가 소유한다.
- parser.c: 58379787 bytes, SHA256 `68151433386e2ee6c467956fde8945e104accd1955a9931538b941800882d5de`. grammar 파일 5개와 runtime 파일 83개를 현재 등록 hash/bytes로 확인했다.
- 기반 runtime `659cda7c7f86ebe31cc825dc5da59e9add172dc7` + [field patch 정책](../provenance/upstream-sources.md#native-runtime의-field-조회-patch-105). 적용 후 node.c SHA256 `0c2531b763ae83ae87e76d3af6ec69137628eb0d4231faedf3cd6ab23bfc8de0`.
- fact pack `fact-queries-r1`, SHA256 `864cb13d56a52043230246a366fe4588876cbde73b7ab5ae9424a31ce4efd852`. 기존 `facts.declarations`·`facts.dynamic_sql` query bytes를 사용했다.
- observation profile `issue109-observation`, SHA256 `f0c3f9e1346862ad64d2dceccadbbf8e89965182943eba5febf7793dd9a0fe84`; compiler SHA256 `75e87953e9d04e8a5d81225397292e9f8ae599a0ed85f70ed175767577aaa9f6`; build identity `3e31f963eccde97b1f0187c25f402883549193071c0bb98ccbbb3b501cb49153`.

`oracle record`는 23개 모두 COMPLETED, NEW_RUN, manifest assessment PASS, findings 0, record set `valid=true`로 끝났다. 이는 기록 장치의 성공이며 SQL Server 유효성 판정은 아니다. 별도 관측 profile의 언어 기대값은 비워 두었고 등록 fixture·기대값·comparator·query·mapping·지원 규칙은 수정하지 않았다. 원본 입력·flat ordered CST·query captures·facts·profile·compiler/runtime build closure는 primary의 `artifacts/issue109-20261007/`에 보존한다.

## S08 과잉 수용 다섯 종류

아래 CST는 실제 flat cursor 기록에서 named node를 순서대로 투영한 것이다. unnamed token은 생략하고 field 이름·named keyword·leaf text를 남겼다. full-tree digest는 기존 `tsgk-tree-digest/r1` 값이며 이 투영 문자열의 hash가 아니다. 입력은 모두 UTF-8/LF이고 마지막 LF를 포함한다.

### 예약어 단독 호출 — 남음

```sql
PERCENT;
```

`has_error=false`, tree digest `48a9a3c491b8d40f67b5396664fc0d6de0a5fdaf3167d20adcea1652eb0da87d`.

```text
(program (batch (execute_statement (procedure:object_reference (name:identifier "PERCENT")))))
```

[예약어 문서](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/reserved-keywords-transact-sql?view=sql-server-ver17)는 PERCENT를 예약어로 분류하며 식별자에는 구분자가 필요하다. `[PERCENT];` 대조군도 무오류 수용한다. 예약어를 일반 procedure identifier로 읽는 과잉 수용은 남아 있다.

### Unicode 3.2 밖 문자 — 남음

```sql
SELECT 1 AS Ა;
```

`has_error=false`, tree digest `745efba9fd11c1099c0a6696b7764aa8650559f20cddfd2f64e78fb199e905f7`.

```text
(program (batch (statement (query_specification (select (keyword_select "SELECT") (select_expression (term (value:literal "1") (keyword_as "AS") (alias:identifier "Ა"))))))))
```

[식별자 문서](https://learn.microsoft.com/en-us/sql/relational-databases/databases/database-identifiers?view=sql-server-ver17)는 regular identifier의 문자를 Unicode 3.2로 정의한다. U+1C90은 Python `unicodedata.ucd_3_2_0.category`에서 Cn, 현재 category에서 Lu다. Unicode 3.2 문자 U+10D0와 ASCII a는 정상 대조군으로 수용된다. 문자 범위 과잉 수용은 남아 있다.

### master key의 ENCRYPTION/DECRYPTION — 남음

```sql
BACKUP MASTER KEY TO FILE = 'fixture.bak' ENCRYPTION BY CERTIFICATE c;
```

`has_error=false`, tree digest `02ab1f259b7b145f432420fc4d7ee9d94c935ed2adce448546fe3b838561fd02`.

```text
(program (batch (statement (backup_statement (keyword_backup "BACKUP") (keyword_master "MASTER") (keyword_key "KEY") (keyword_to "TO") (option (name:identifier "FILE") (value:literal "'fixture.bak'")) (encryption_mechanism (keyword_encryption "ENCRYPTION") (keyword_by "BY") (keyword_certificate "CERTIFICATE") (identifier "c"))))))
```

[BACKUP MASTER KEY](https://learn.microsoft.com/en-us/sql/t-sql/statements/backup-master-key-transact-sql?view=sql-server-ver17)와 [RESTORE MASTER KEY](https://learn.microsoft.com/en-us/sql/t-sql/statements/restore-master-key-transact-sql?view=sql-server-ver17)는 이 문맥의 BY 뒤에 PASSWORD를 요구한다. BACKUP의 CERTIFICATE와 아래 RESTORE의 CERTIFICATE 변형 모두 무오류 수용한다. PASSWORD 대조군은 유지된다. 문맥별 과잉 수용은 남아 있다. CERTIFICATE가 모든 SQL 문맥에서 잘못된다는 주장은 하지 않는다.

### 빈 BEGIN END — 남음

```sql
BEGIN END;
```

`has_error=false`, tree digest `7afff0401c605261d4627014ad965c4d19e1abac41916fed12f3afc534337623`.

```text
(program (batch (block (keyword_begin "BEGIN") (keyword_end "END"))))
```

[BEGIN...END 문서](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/begin-end-transact-sql?view=sql-server-ver17)는 한 문장 이상을 요구한다. `BEGIN END;`는 수용하고 `BEGIN; END;`는 ERROR로 거부한다. 비어 있지 않은 block은 수용한다. 세미콜론 없는 빈 block 과잉 수용은 남아 있다.

### BEGIN 바로 뒤 CTE의 세미콜론 — 남음

```sql
BEGIN WITH c AS (SELECT 1 AS a) SELECT a FROM c; END;
```

`has_error=false`, tree digest `04e0172883b558652a7e669578f2129f0a870af9f30fc71eddd582ff146a60e4`.

```text
(program (batch (block (keyword_begin "BEGIN") (statement (keyword_with "WITH") (cte (identifier "c") (keyword_as "AS") (statement (query_specification (select (keyword_select "SELECT") (select_expression (term (value:literal "1") (keyword_as "AS") (alias:identifier "a"))))))) (query_specification (select (keyword_select "SELECT") (select_expression (term (value:field (name:identifier "a"))))) (from (keyword_from "FROM") (relation (object_reference (name:identifier "c")))))) (keyword_end "END"))))
```

같은 [BEGIN...END 문서](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/begin-end-transact-sql?view=sql-server-ver17)는 CTE를 시작하는 WITH 앞에 세미콜론을 요구한다. `BEGIN WITH`와 정상 대조군 `BEGIN; WITH` 모두 수용한다. 세미콜론 누락 과잉 수용은 남아 있다.

ENCRYPTION/DECRYPTION 분류의 추가 관측:

```sql
RESTORE MASTER KEY FROM FILE = 'fixture.bak' DECRYPTION BY CERTIFICATE c ENCRYPTION BY PASSWORD = 'fixture-only';
```

`has_error=false`, tree digest `f45c084c200a7ca8ac1d376a007dbcb508b69080685907dc9563a49acea5d02e`.

```text
(program (batch (statement (restore_statement (keyword_restore "RESTORE") (keyword_master "MASTER") (keyword_key "KEY") (keyword_from "FROM") (option (name:identifier "FILE") (value:literal "'fixture.bak'")) (encryption_mechanism (keyword_decryption "DECRYPTION") (keyword_by "BY") (option (name:identifier "CERTIFICATE") (value:identifier "c"))) (encryption_mechanism (keyword_encryption "ENCRYPTION") (keyword_by "BY") (option (name:identifier "PASSWORD") (value:literal "'fixture-only'")))))))
```

다섯 종류의 문맥별 grammar 수정은 [#113](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/113)이 추적한다. [S08 보고서](session-08-qualification.md)의 당시 관측은 이 새 기준 결과로 소급 덮어쓰지 않는다.

## 동적 SQL known miss 재분류

[EXECUTE](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/execute-transact-sql?view=sql-server-ver17)·[GO 문서](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/sql-server-utilities-statements-go?view=sql-server-ver17)에 따르면 batch 첫 호출은 EXEC를 생략할 수 있지만 같은 batch의 비첫 procedure 호출에는 EXEC가 필요하다. 아래 비첫 단일·이중 세미콜론 입력에는 GO가 없다. ERROR·동적 사실 0은 유효한 호출을 놓쳤다는 근거가 아니다.

```sql
sp_executesql N'SELECT 1';
```

```text
(program (batch (execute_statement (procedure:object_reference (name:identifier "sp_executesql")) (exec_argument (value:literal "N'SELECT 1'")))))
```

tree digest `41b3fdff80c3e8396b67253812f0e6e1085556bd32593c87315779c00704204b`. 해소됨: 실제 첫 호출은 execute_statement로 파싱되고 SP_EXECUTESQL 사실 1개·known miss 0개다.

```sql
SELECT 1;sp_executesql N'SELECT 2';
```

```text
(program (ERROR (statement (query_specification (select (keyword_select "SELECT") (select_expression (term (value:literal "1")))))) (identifier "sp_executesql")) (batch (execute_statement (procedure:object_reference (name:identifier "N")) (exec_argument (value:literal "'SELECT 2'")))))
```

tree digest `4602bc94879acebd095a57c3a98487dde63c76c8ee9f952ee31a59d9d7f2406e`. 의도된 동작: 비첫 EXEC 생략 호출은 ERROR다. 이 단일 세미콜론 모양의 batch-first known-miss 오분류는 재현되지 않았다(사실 0·known miss 0).

```sql
SELECT 1;;sp_executesql N'SELECT 2';
```

```text
(program (batch (statement (query_specification (select (keyword_select "SELECT") (select_expression (term (value:literal "1"))))))) (ERROR (identifier "sp_executesql")))
```

tree digest `5675e4a9b358b97eeae4a0d8e716edfb106c2ba7df089c9ef7e2cc1b76fb5fa2`. 남음: 비첫 호출의 ERROR를 batch-first known miss로 오분류한다(사실 0·known miss 1, byte 범위 [10,36)). ERROR 자체의 거부는 의도된 동작이다.

`src/kit/facts.go`의 fallback은 query `(ERROR . (identifier) @err.first) @err`와 ERROR text 시작의 `sp_executesql` 정규식으로 분류하고 실제 batch 위치를 검사하지 않는다. 단일 `;` 입력은 ERROR가 SELECT를 포함하고 문자열 일부가 별도 batch로 회복되는 반면, `;;` 입력은 ERROR가 byte 10의 sp_executesql로 시작한다. 서로 다른 recovery CST 때문에 label 결과가 달라진다는 해석은 실제 tree·captures와 현재 코드를 대조한 추론이다.

명시적 EXEC와 GO 뒤 실제 첫 호출은 모두 무오류·사실 1·known miss 0으로 관측했다. leading `;`, GO 뒤 leading `;`, 주석·구분자 변형은 아래와 같다. leading `;` 두 사례는 ERROR·사실 0·known miss 0이라는 관측만 기록하며 SQL Server에서 그 정확한 모양을 실행한 근거는 없다. 비첫 `;;`의 kit 오분류 수정은 [#114](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/114)가 추적한다. C2의 등록 dynamic SQL fixture가 facts 16·known miss 0이라는 사실을 이 fallback의 모든 입력 정확성 주장으로 확대하지 않는다.

## 전체 입력과 대조군

입력 표의 `\n`은 LF 하나다. ERROR는 `has_error=true`, NO_ERROR는 false다. 사실 수는 SP_EXECUTESQL 동적 SQL items이며 언어 유효성 판정과 별개다.

| case | 입력(끝 LF 포함) | syntax | 사실 | known miss |
|---|---|---|---:|---:|
| reserved-bare | `PERCENT;\n` | NO_ERROR | 0 | 0 |
| reserved-delimited | `[PERCENT];\n` | NO_ERROR | 0 | 0 |
| unicode-new | `SELECT 1 AS Ა;\n` | NO_ERROR | 0 | 0 |
| unicode-old | `SELECT 1 AS ა;\n` | NO_ERROR | 0 | 0 |
| identifier-ascii | `SELECT 1 AS a;\n` | NO_ERROR | 0 | 0 |
| backup-certificate | `BACKUP MASTER KEY TO FILE = 'fixture.bak' ENCRYPTION BY CERTIFICATE c;\n` | NO_ERROR | 0 | 0 |
| backup-password | `BACKUP MASTER KEY TO FILE = 'fixture.bak' ENCRYPTION BY PASSWORD = 'fixture-only';\n` | NO_ERROR | 0 | 0 |
| restore-certificate | `RESTORE MASTER KEY FROM FILE = 'fixture.bak' DECRYPTION BY CERTIFICATE c ENCRYPTION BY PASSWORD = 'fixture-only';\n` | NO_ERROR | 0 | 0 |
| restore-password | `RESTORE MASTER KEY FROM FILE = 'fixture.bak' DECRYPTION BY PASSWORD = 'fixture-only' ENCRYPTION BY PASSWORD = 'fixture-only';\n` | NO_ERROR | 0 | 0 |
| empty-block | `BEGIN END;\n` | NO_ERROR | 0 | 0 |
| empty-block-semicolon | `BEGIN; END;\n` | ERROR | 0 | 0 |
| nonempty-block | `BEGIN SELECT 1; END;\n` | NO_ERROR | 0 | 0 |
| cte-no-semicolon | `BEGIN WITH c AS (SELECT 1 AS a) SELECT a FROM c; END;\n` | NO_ERROR | 0 | 0 |
| cte-semicolon | `BEGIN; WITH c AS (SELECT 1 AS a) SELECT a FROM c; END;\n` | NO_ERROR | 0 | 0 |
| dynamic-first | `sp_executesql N'SELECT 1';\n` | NO_ERROR | 1 | 0 |
| dynamic-leading-semicolon | `;sp_executesql N'SELECT 1';\n` | ERROR | 0 | 0 |
| dynamic-nonfirst | `SELECT 1;sp_executesql N'SELECT 2';\n` | ERROR | 0 | 0 |
| dynamic-nonfirst-double | `SELECT 1;;sp_executesql N'SELECT 2';\n` | ERROR | 0 | 1 |
| dynamic-explicit-exec | `SELECT 1; EXEC sp_executesql N'SELECT 2';\n` | NO_ERROR | 1 | 0 |
| dynamic-after-go | `SELECT 1;\nGO\nsp_executesql N'SELECT 2';\n` | NO_ERROR | 1 | 0 |
| dynamic-go-semicolon | `SELECT 1;\nGO\n;sp_executesql N'SELECT 2';\n` | ERROR | 0 | 0 |
| dynamic-comment | `SELECT 1; /* owned */ sp_executesql N'SELECT 2';\n` | ERROR | 0 | 0 |
| dynamic-bracket | `SELECT 1; [sys].[sp_executesql] N'SELECT 2';\n` | ERROR | 0 | 0 |


## 재현과 검증

현재 main을 clean 전용 worktree에서 build하고 [S05/S06 절차](../validation/validation.md)로 T-SQL route를 새 destination에 준비한다(`prepare-routes.ps1 -Destination <prepared> -Platform windows/amd64 -Routes tsql`; 등록 source/tool/runtime 검사 유지). 기존 `run-routes.ps1 -Prepared <prepared> -Destination <baseline> -Compiler C:/msys64/ucrt64/bin/gcc.exe -Platform windows/amd64 -Routes tsql -Oracle`로 만든 `profiles/s06-tsql.json`에서 grammar·compiler·declarations·fact_pack·api 값을 보존한다. observation profile은 id를 `issue109-observation`, query를 기존 facts 두 개, cases를 위 UTF-8/LF 입력과 bytes/SHA256으로 바꾼 별도 사본이다. 각 case는 edits/points/expect/query_expect 빈 배열, dynamic_sql_expect null이다. 이는 기존 시험 기대값을 변경하는 절차가 아니다. profile과 fact pack은 두 입력 root 밖에 둔다.

```powershell
# Fresh caller-owned work/output; <work> exists, <out> does not.
# Build the Go CLI with CGO_ENABLED=0.
tsgk oracle record --root <owned-inputs> --grammar-root <prepared>/routes/tsql --profile <observation-profile> --fact-pack src/contracts/fact-query-pack.json --runtime <prepared>/runtime --tool cc=C:/msys64/ucrt64/bin/gcc.exe --work <work> --out <out> --allow BUILD_NATIVE --allow EXEC_NATIVE --run-wall 1800
```

검증: source/tool/runtime/fact-pack pin 대조, 23개 completed/complete 기록 및 입력 hash·tree parent 순서 확인, 관측 원본 보존. 재분류의 정상·부정 대조는 위 표에 있다. docs의 link/format/foundation과 독립 context 리뷰, 최종 PR 및 실제 main의 필수 CI·qualification은 각 PR/Issue의 실제 실행 근거로 기록한다. 이 보고서는 미래 CI 결과를 주장하지 않는다.
