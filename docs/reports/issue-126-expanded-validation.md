# 전체 T-SQL corpus와 대용량 API 검증 (#126)

[#126](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/126)는 [#122·#123 경계 수정](issue-122-123-tsql-boundaries.md)을 전체 등록 T-SQL 회귀, 현재 비공개 SQL corpus, SQL Server 2025 compatibility level110·170, 등록 large-input의 `tsgk-api/r1` 관측 범위에서 검증한다. 실제 SQL Server2012 engine은 필요 시 추가하는 범위로 확정됐으며 설치하거나 실행하지 않았다. [Microsoft compatibility 계약](https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-database-transact-sql-compatibility-level?view=sql-server-ver17)에 따라110은 실제2012 engine 또는2012 기능 제한의 대체 증거가 아니다.

## 전체 SQL corpus

공개 등록 T-SQL은 incremental691개와 oracle697개 모두 COMPLETED/PASS, API 차이0, helper 실패0이다. 원래107개 엔진 입력도 설정마다107회, 합계214회 관측에서 설정 간 차이0을 확인했다. 원래 중복 포함 네 불일치는4→0이다. 등록 회귀와 engine/PARSEONLY의 의미는 구분한다.

공개 upstream `meloncholera/tree-sitter-mssql@8620fbcfca9438e1ff7104835bcb8305aba3bdfa`의31개 corpus 파일에서351개 입력을 분리하고 `.sql`162개를 모두 포함해513개를 추가 대조했다.193개 원본 파일은 해당 commit의 공개 Git tree blob identity와 전부 MATCH했고 corpus header 경계도351개 모두 확인했다. native513개는 COMPLETED/PASS, API 차이0이며 NO_ERROR478개·ERROR35개다. upstream expected CST의 동일성을 주장하지 않는다. 현재 C2 문법의 tree/API 관측과 engine parse status를 비교한다.

upstream engine 관측은 설정별513개·728개 batch, 합계1,026개·1,456개 batch다.110에서는 ACCEPT349·REJECT158·SQLCMD CLIENT_SKIPPED6이고170에서는 ACCEPT353·REJECT154·CLIENT_SKIPPED6이다. native와 대조한 engine ACCEPT/native ERROR는 두 설정 모두7개다. engine REJECT/native NO_ERROR는110의130개·170의126개이며 binding·batch/module 문맥·version 요구·의도된 음성을 포함하므로 모두 grammar false accept로 단정하지 않는다. engine REJECT/native ERROR는28개, SQLCMD6개는 명시적으로 미검사 처분이다. 잔여7개 수용 격차와 REJECT 문맥 분리는 [#128](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/128)에 연결한다.

upstream의 설정 간 전체 status/error 차이는5개다. OPENJSON fixture는 두 설정 모두 REJECT지만110에서102 진단이 추가되고, REGEXP_LIKE·확장 TRIM·WINDOW 두 사례는110 REJECT/170 ACCEPT다. 이는 [OPENJSON의130 이상 요구](https://learn.microsoft.com/en-us/sql/t-sql/functions/openjson-transact-sql?view=sql-server-ver17), [REGEXP_LIKE의170 이상 요구](https://learn.microsoft.com/en-us/sql/t-sql/functions/regexp-like-transact-sql?view=sql-server-ver17), [확장 TRIM의160 요구](https://learn.microsoft.com/en-us/sql/t-sql/functions/trim-transact-sql?view=sql-server-ver17), [WINDOW의160 이상 요구](https://learn.microsoft.com/en-us/sql/t-sql/queries/select-window-transact-sql?view=sql-server-ver17)와 일치한다. OPENJSON의 중복 변수134는 두 설정에서 남는다. 문법은2012~2025 합집합 계약이므로 이 기능들을 모든 level에서 거부하도록 바꾸지 않는다. 이5개를 업무 corpus의 설정 간 차이0 또는 원래 네 결함과 혼동하지 않는다.

비공개 corpus의 현재 `.sql` 전체6,150개를 다시 열거하고 size/SHA256을 재확인했다. source와 파일 이름·경로·hash는 ignored local evidence에만 보존한다. 원래 root 전체 inventory는 SQL 외의 읽기 불가 파일에서 중단됐다. 해당 파일의 권한을 바꾸지 않고 SQL 선택 집합을 독립적으로 재검증했다. encoding은 UTF-8 2,777개·UTF-16LE 2,788개·CP949 585개 모두 PASS다. native는6,150/6,150개,13개 batch 모두 COMPLETED/PASS, fatal/not-run/resource/encoding block0이다. CST의 NO_ERROR는6,088개, ERROR는62개다. 이 PASS는 native 처리·관측의 완료이며 SQL source 전체의 engine 적합성 PASS를 뜻하지 않는다. 최종 SQLCMD 보완 parser로6,150개를 다시 검사해 기존 source identity·완료/오류 판정 차이0을 확인했다. tree digest는 정상1개·오류1개에서 달랐다. 두 입력을 별도 full tree로 대조했으며 node count·node type 순서는 같고 semicolon/후속 statement의 소유자가 batch에서 procedure_body로 옮겨진 차이다. engine status는 두 설정 모두 기존 REJECT를 유지했다. 이 module-body ownership 경계는 #128에서 공개 합성 재현으로 추가 확인하며 전체 corpus CST 차이0이라고 주장하지 않는다.

기존 LocalDB17.0.1000.7에 task 소유 임시 DB 두 개를 만들어 compatibility110·170을 read-back하고 SQL을 in-place로 읽었다. data/log max64/32MiB, 연결·명령5초, 전체 probe1,800초로 제한했다. 모든 입력은 [PARSEONLY](https://learn.microsoft.com/en-us/sql/t-sql/statements/set-parseonly-transact-sql?view=sql-server-ver17)로 검사하며 arbitrary SQL 실행·DDL 실행·결과 row 수집은 하지 않는다. PARSEONLY를 바꾸는 입력과 SQLCMD directive는 guard로 처분한다. GO는 quote/bracket/nested comment 밖의 단독 batch delimiter만 인식하며 repeat는 한 번만 parse한다. SQL 오류 뒤에도 나머지 batch를 전부 시도하고 각 batch 전에 task DB context를 복구한다. 다른 DB·사용자 설정은 변경하지 않고 task DB 제거를 확인했다. 오류 message/raw SQL을 기록하지 않는다.

설정마다6,150개 파일,19,522개 batch를 검사했다. 합계12,300개 파일 관측·39,044개 batch 시도다. 모든 batch를 시도한 최종 결과는 설정 간6,150/6,150 동일하며 context 변경·PARSEONLY guard skip0이다. SQLCMD 입력25개는 CLIENT_SKIPPED로 보존한다. 엔진 status는 설정마다 ACCEPT4,232개·REJECT1,893개·CLIENT_SKIPPED25개다.

| 설정별 비교 | 파일 수 | 처분 |
|---|---:|---|
| engine ACCEPT / native NO_ERROR | 4,216 | parse 수용 일치; binding·실행 보증 아님 |
| engine binding-only / native NO_ERROR | 1,852 | 오류911·137·195 등 catalog/name 경계; 문법 실패로 바꾸지 않음 |
| engine binding-only / native ERROR | 6 | binding 오류와 native recovery가 혼재; 원문은 local evidence에서 보존하고 공개 합성 재현으로 분리할 후속 대상 |
| engine REJECT / native NO_ERROR | 12 | 아래 오류 signature로 분리; 전부 순수 문법 오류라고 부르지 않음 |
| engine REJECT / native ERROR | 23 | 오류 수용 판정 일치; 각 진단의 의미가 동일하다는 주장 아님 |
| engine ACCEPT / native ERROR | 16 | 문맥별 native 수용 격차; 후속 공개 합성 재현 대상 |
| CLIENT_SKIPPED | 25 | SQLCMD 전처리·client semantics 범위; 미검사 상태를 PASS로 바꾸지 않음 |

native가 수용한 engine REJECT12개는 오류 signature `{111}`8개, `{111,178}`1개, `{111,911}`1개, `{102,137,156}`1개, `{174}`1개다. 오류111이 있는10개는 module/batch-first 제약 대조 대상이고178은 RETURN 문맥,174는 함수 argument 수의 semantic/context 경계다. 102/156과 변수137의 혼합1개는 문법과 binding을 분리할 대상이다. 이12개 전체를 “grammar false accept12개”로 단정하지 않는다. 엔진에 없는 DB/객체를 생성하거나 semantic 오류를 grammar ERROR로 바꿔 수용률을 맞추지 않는다.

합성 입력에서는 DEFAULT naked/qualified column reference의128 두 개와 TRY/CATCH leading semicolon의 engine ACCEPT/native ERROR가 별도 재현됐다. procedure body ownership의 semicolon도 추가 분리 대상이다. 이 격차와16개 수용 격차·6개 혼합 recovery·SQLCMD25개의 처분은 [후속 #128](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/128)에 남기며 이번 검증에서 숨기거나 skip 기대값으로 바꾸지 않는다.

`run-corpus.ps1 -Routes tsql -InventoryPath <inventory>`는 기존 처리 경로를 재사용한다. caller-supplied inventory 자체가 현재 전체 집합 또는 freshness를 증명하지 않는다. 독립적인 열거·source identity 재확인이 선행돼야 하고 각 selected source의 hash는 native command가 다시 검사한다. summary는 inventory provenance와 선택 route를 기록한다.

## 대용량 API 관측

검사 범위와 receipt 계약은 [tree/adapter protocol](../specs/tree-and-adapter-protocol.md)의 대용량 API audit 절이 소유한다. 기존 summary의 `api:null` 또는 API claim을 수정하지 않고 별도 Windows audit로 등록3개 C# large-input의 모든 preorder node, 직접 child field, language field ID의 by-id/by-name lookup과 등록 point·0·EOF를 검사한다. node metadata accessor의 독립 semantic correctness나 모든 byte/range의 exact descendant 선택을 주장하지 않는다.

기존 input32MiB·output16MiB·request50,331,648 bytes·node25m·depth100k·parse60초·process90초·memory8GiB HARD 한도를 유지한다. 구간별 최대1m node를 검사하고 처음0부터 마지막 node까지 연속 coverage, normal driver baseline node count, 모든 child field의 합node count-1을 요구한다. 구간 실패·누락·중복·resource limit은 전체 PASS가 아니다. 단일 process의 전체 관측이90초를 넘었던 초기 실행은 RESOURCE_LIMIT으로 보존하고 PASS로 승격하지 않았다.

native C 구현의 로컬 full audit는 등록3개 입력에서33,110,006개 node·35개 연속 구간·2,185,260,882개 비교를 완료했다. difference와 positional divergence는 모두0, cleanup 검증·HARD memory는 모든 구간에서 PASS다. 실행한 C/Go source hash를 당시 후보와 대조했다. 이후 T-SQL pin 갱신으로 전체 route registry hash는 달라졌지만 선택한 C# route·runtime·large fixture inventory와 C/Go 구현은 같음을 대조했다. 첫 Windows CI의 full receipt도 같은 node·range·비교 수와 difference/divergence0을 확인했다. Linux race는 Go package 안의 standalone C 파일을 cgo source로 읽어 실패했다. C 파일에 `//go:build ignore`를 추가해 Go package source에서 제외했고 실제 C 본문은 바꾸지 않았다. CGO0 필수 시험에서도 CGO1 build context의 CFiles/CgoFiles가 비어 있음을 검사하며 별도 native compile/control과 CGO1 race를 검증한다. 최종 exact-head CI는 이 source 경계를 포함해 full audit를 다시 결속한다. small control은 full large 완료를 뜻하지 않도록 별도 schema를 사용한다. 작은 정상·오류4개 full Go oracle,8개 거부 mutant, 실제 MISSING semicolon의 ZERO_SKIP 허용 control, typed ALLOCATION_LIMIT control과 strict receipt/inventory 변조 검사를 유지한다.

| 등록 입력 | 전체 node | 연속 구간 | 비교 수 | 최대 process wall | 최대 memory bytes |
|---|---:|---:|---:|---:|---:|
| cs-large-22m | 11,463,151 | 12 | 756,568,109 | 28.406초 | 4,537,864,192 |
| cs-large-8m-errors | 4,168,686 | 5 | 275,133,350 | 27.471초 | 1,623,953,408 |
| cs-large-32mib-errors | 17,478,169 | 18 | 1,153,559,423 | 41.650초 | 7,207,354,368 |

Windows CI의 별도 audit step은 command exit를 gate로 삼고 run/attempt별 receipt를 보관한다. 세 OS의 기존 qualification과 Windows large audit는 각각의 범위로 판정한다. 이 보고서만으로 최종 PR CI나 main 통합 완료를 주장하지 않으며 정확한 commit/run/attempt와 finding 처분은 Issue/PR에 기록한다.
