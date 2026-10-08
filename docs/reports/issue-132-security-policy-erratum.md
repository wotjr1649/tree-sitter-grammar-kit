# SECURITY POLICY 문서 erratum과 ALTER 경계 (#132)

## 원인과 처분

Microsoft Learn의 [ALTER SECURITY POLICY Syntax](https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-security-policy-transact-sql?view=sql-server-ver17)는 `NOT FOR REPLICATION`을 suffix로 표시한다. 그러나 Microsoft 공식 SqlScriptDOM 고정 revision `eaf3a6e8cf49350c7600bf70691de6314594ffba`의 [TSql130 ALTER production](https://github.com/microsoft/SqlScriptDOM/blob/eaf3a6e8cf49350c7600bf70691de6314594ffba/SqlScriptDom/Parser/TSql/TSql130.g#L21853), [TSql170 ALTER production](https://github.com/microsoft/SqlScriptDOM/blob/eaf3a6e8cf49350c7600bf70691de6314594ffba/SqlScriptDom/Parser/TSql/TSql170.g#L23901)과 [공식 test fixture](https://github.com/microsoft/SqlScriptDOM/blob/eaf3a6e8cf49350c7600bf70691de6314594ffba/Test/SqlDom/TestScripts/CreateAlterSecurityPolicyStatementTests130.sql#L94)는 predicate 목록 변경, STATE 변경, `ADD NOT FOR REPLICATION`, `DROP NOT FOR REPLICATION`을 독립 ALTER 대안으로 정의한다. CREATE의 suffix와 구분한다.

SQL Server 2025 Express LocalDB `17.0.1000.7`의 PARSEONLY에서 compatibility110·170 모두 ADD/DROP 복제 옵션 변경을 수용한다. bare suffix·STATE 뒤 suffix는102, predicate 뒤 STATE는319, predicate 뒤 suffix는102다. CREATE suffix는 수용된다. 두 level의 결과와 공식 parser가 일치하므로 문서 erratum으로 처분한다. 과거 유효했던 SQL의 합집합을 줄이는 근거로 상위 엔진의 거부만을 사용하지 않는다. ScriptDom source는 정적 근거이며 binary parser 실행 결과가 아니다.

현재 문서 표기는 공개 MicrosoftDocs revision `320dd592bc3f4c3b8a6e2d9ea33bedc0723d0f45` (2017-03-16)부터 존재한다. 오래된 표기의 지속은 과거 engine의 수용 증거가 아니다. 기능의 SQL Server 적용 하한은2016이며 실제2012 engine은 이 항목의 기능 비교 대상이 아니다. compatibility110은2025 engine의 관측으로 기록한다.

## 구현과 검증 계약

ALTER의 복제 옵션 ADD/DROP 대안을 지원하고 bare suffix와 predicate/STATE 결합을 제한한다. CREATE suffix와 공유 `_not_for_replication` token은 유지한다. 복수 작업은 별도 ALTER 문장으로 표현한다. 기존 #131의 문서 불일치 control은 원문을 보존한 음성 사례로 정정한다. 이 처분은 [#131 당시의 보류](issue-131-tsql-overacceptance.md)를 대체한다.

등록 회귀12개는 CREATE suffix·ALTER ADD/DROP 정상, bare suffix·predicate/STATE/replication 결합 음성, 정상→음성→복구, 복수 predicate, 분리된 ALTER, 뒤따르는 SELECT/CTE를 검사한다. 독립 anchor 문장을 포함해 incremental/fresh ordered CST·전체 작은 API·fact 재현과 기존 검사의 유지 여부를 함께 확인한다. 기대값이나 comparator를 완화하지 않는다. C2 literal patch, 생성기 입력, 생성물 identity와 qualification inventory를 같은 작업 단위에서 갱신한다.

local 등록 재생성·core·S05/S06·공개 전체 corpus와 engine110·170 대조, 독립 리뷰, 필수 세OS PR CI·large-input 전체 API·actual main 검증의 결과는 해당 후보의 receipt와 PR에 기록한다. 검증 전에는 완료 또는 지원 성공으로 승격하지 않는다. 실제 policy 생성/변경·권한/binding/runtime 성공·실제2012 engine·private corpus는 이번 범위에서 미실행이다. PARSEONLY 수용은 실행 성공을 의미하지 않는다. [#134 ASSEMBLY](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/134)는 별도 작업이다.
