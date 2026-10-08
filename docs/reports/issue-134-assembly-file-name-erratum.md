# ASSEMBLY file_name 문서 erratum과 문자열 경계 (#134)

## 원인과 처분

Microsoft Learn의 [Altering an Assembly](https://learn.microsoft.com/en-us/sql/relational-databases/clr-integration/assemblies/altering-an-assembly?view=sql-server-ver17) 예제는 `AS PointSource`를 사용한다. Microsoft 공식 SqlScriptDOM 고정 revision `eaf3a6e8cf49350c7600bf70691de6314594ffba`의 [TSql90](https://github.com/microsoft/SqlScriptDOM/blob/eaf3a6e8cf49350c7600bf70691de6314594ffba/SqlScriptDom/Parser/TSql/TSql90.g#L5363)·[TSql110](https://github.com/microsoft/SqlScriptDOM/blob/eaf3a6e8cf49350c7600bf70691de6314594ffba/SqlScriptDom/Parser/TSql/TSql110.g#L9049)·[TSql170](https://github.com/microsoft/SqlScriptDOM/blob/eaf3a6e8cf49350c7600bf70691de6314594ffba/SqlScriptDom/Parser/TSql/TSql170.g#L11321) ALTER ASSEMBLY production은 ADD FILE의 AS file_name과 DROP FILE 목록 항목을 StringLiteral로 정의한다. [공식 ALTER ASSEMBLY test fixture](https://github.com/microsoft/SqlScriptDOM/blob/eaf3a6e8cf49350c7600bf70691de6314594ffba/Test/SqlDom/TestScripts/AlterAssemblyStatementTests.sql)도 문자열 DROP FILE을 사용한다. assembly 객체 이름과 file_name은 별도 구문이다.

SQL Server 2025 Express LocalDB `17.0.1000.7` PARSEONLY의 compatibility110·170에서 문자열 file_name은 수용되고 bare·대괄호 identifier·숫자 file_name은102로 거부된다. 두 level의 18개 대조 결과가 동일하고 공식 parser의90/110/170 정의와 일치하므로 문서 erratum으로 처분한다. ScriptDom source는 정적 근거이며 binary parser 실행 결과가 아니다. 오래된 parser 정의와2025 engine의 compatibility110을 실제2012 engine 실행으로 표시하지 않는다.

문제 예제는 공개 MicrosoftDocs revision `320dd592bc3f4c3b8a6e2d9ea33bedc0723d0f45` (2017-03-16)부터 존재한다. 오래된 문서의 지속은 과거 engine에서 bare file_name이 유효했다는 증거가 아니다. historical valid source 합집합은 유지한다.

## 구현과 검증 계약

두 file_name 위치에서 identifier를 제거하고 기존 문자열 token을 사용한다. 작은따옴표·N prefix·doubled quote를 유지하며 `QUOTED_IDENTIFIER OFF`의 큰따옴표 문자열도 기존 scanner token으로 수용한다. ON의 큰따옴표 identifier는 거부한다. 공유 literal/identifier rule과 scanner는 이 결함 때문에 변경하지 않는다. assembly 객체 identifier, source literal, DROP FILE ALL, 여러 파일 목록, DROP 뒤 ADD, 생략 가능한 AS와 뒤따르는 SELECT는 보존한다.

기존 #131 bare file_name control 두 개는 원문을 보존해 음성으로 정정한다. 등록 회귀13개는 bare·대괄호·숫자·QUOTED_IDENTIFIER ON·ALL 혼합 목록의 정상→음성→복구9개와 Unicode/escaped quote·여러 파일·AS 생략·후속SELECT 양성4개다. 각 사례의 독립 anchor와 incremental/fresh ordered CST·전체 작은 API·fact 검사를 유지한다. C2 literal patch, source/generated pins, qualification inventory, canonical disposition과 source export 필수 파일 목록을 같은 작업 단위에서 갱신한다.

## 현재 로컬 관측

등록된 TSQL route의 prepare는 두 workspace 생성·재생성·reference·JavaScript 재현과 warnings 0을 통과했고, S05 route는 826 cases, S06 Oracle은 833 cases에서 각각 `PASS`로 완료됐다. 후보 workspace의 두 재현은 generator 실행과 결정성은 통과했지만 기존 reference가 없는 bounded 재현이므로 `REFERENCE_ABSENT`에 의해 reference match와 JavaScript 재현은 주장하지 않는다. 캐시를 끈 `go test ./src/... -count=1 -v -timeout 300s`, `go vet ./...`, `go build ./...`, `go mod verify`, 전체 `gofmt -l src`와 `git diff --cached --check`도 `CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly` 환경에서 통과했다.

기존 공개 748건의 input hash를 유지한 채 신규 18건을 더한 766건의 Oracle record는 766/766 `COMPLETED`·`PASS`이며 API·fact·cleanup 모두 766/766이다. 발행 record set 검증은 `valid=true`, 766 records, findings 0으로 통과했다. 기존 748건과 비교한 adaptive comparator는 source hash 일치, tree change 13건, 의도한 기존 overacceptance 오류 전이 11건, source-mapping 18건의 오류 수 17→18(`upstream-sql-000469`)을 확인했다. compatibility 110·170 각각에서 새 parser error를 `ACCEPT`한 사례는 0건이다.

공개 corpus의 LocalDB `17.0.1000.7` PARSEONLY 대조는 110에서 `ACCEPT 490 / REJECT 270 / CLIENT_SKIPPED 6`, 170에서 `ACCEPT 495 / REJECT 265 / CLIENT_SKIPPED 6`이었다. `CLIENT_SKIPPED`는 성공으로 세지 않았고, 두 level 모두 task preflight 18개 flag가 pinned expectation과 일치했으며 owned database cleanup을 확인했다. 이는 실제 SQL Server 2012 실행, assembly upload/runtime 또는 CLR 동작의 증거가 아니다.

local 등록 재생성·core·S05/S06·공개 전체 corpus와 engine110·170 대조, 독립 리뷰, 필수 세OS PR CI·large-input 전체 API·actual main 검증은 해당 receipt와 PR에 기록한다. 검증 전에는 완료 또는 지원 성공으로 승격하지 않는다. 실제2012 engine·private corpus·assembly 업로드/변경·CLR·binding/runtime/권한 성공은 이번 범위에서 미실행이다. PARSEONLY 수용은 실행 성공을 의미하지 않는다.
