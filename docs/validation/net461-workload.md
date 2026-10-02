# NET461 업무 workload 준비 계약

`NET461-WINFORMS-DX202-WCF`는 Campaign `TSGK-C1-20260929-R1`의 추가 업무 profile이다. 사용자 채택값은 .NET Framework 4.6.1/net461, C# 7.3 source baseline, WinForms, DevExpress 20.2, Framework WCF와 `.svc` 필수다. 기존 일반 C# 중간 버전·.NET 10/C# 14 profile, 26 route·256 feature 행, S08의 78개 요약 칸은 유지한다. 업무 case·producer·effect·결과는 별도로 연결한다. 제품 구현과 전체 업무 corpus 실행은 NOT_RUN이다.

## 파일 역할 등록부

자동 생성 여부는 제외 사유가 아니다. 역할별 원본 bytes/size/hash/encoding, project 포함 관계와 expected record를 결속한다. private 원본·endpoint·credential·vendor binary·license 자료는 공개 commit/hosted artifact에 포함하지 않는다. 자체 비식별화 fixture는 `OWNED_FIXTURE`, 실제 업무 원본은 `PRIVATE_BUSINESS_SOURCE`다.

| Role ID | 파일 역할 | 요구 구문·구조 / feasibility 경로 |
|---|---|---|
| N461-FORM | Form/UserControl `.cs`, `.Designer.cs` | partial class, base/qualified/global 이름, 필드, InitializeComponent, 중첩 cast, 객체·배열·컬렉션 초기화, 속성 대입, 이벤트 연결, Dispose / C# source |
| N461-RESOURCE | Resources.Designer.cs, `.resx`, 등록 문화권 리소스 | generated partial/member·ResourceManager 접근; XML 요소·속성·문자값·선언된 참조 / C# 및 inert XML |
| N461-DX202 | 실제 DevExpress 20.2 사용·초기화 `.cs` | qualified component type, generic, new/init/cast/property/event 구조 / source만 검사 |
| N461-WCF-CS | 계약·구현 `.cs`, `.svc.cs`, 생성 Reference.cs | ServiceContract/OperationContract attribute, interface/implementation, generic ClientBase/channel/proxy / C# source |
| N461-SVC | `.svc` | `SVC-SERVICEHOST-r1` directive + 등록 inline C# + 원본 included range |
| N461-XML | App.config/Web.config, 필요한 기존 WSDL/XSD | XML source, namespace/qualified 이름, 값·import/include/설정 참조 선언 / inert XML |

실제 20.2.x·사용 component·compiler·LangVersion은 caller가 읽기를 허용한 metadata에서 확인하고 미확인 값은 UNKNOWN이다. project 설정을 변경하지 않는다. 역할의 실제 파일이 제공 corpus에 없으면 NOT_PROVIDED이며 요구는 유지한다. 소스 구문/구조, net461 build, Framework runtime 실행, designer/WCF 통합은 별도 축이다. IIS/WCF endpoint·폼·component·designer 실행, SDK 설치·restore·MSBuild target·svcutil·runtime downgrade는 승인되지 않았다.

요구의 기존 feature 연결은 C# lexical `csharp-B01`, type `csharp-B02`, expression `csharp-B03`, statement `csharp-B04`, member/attribute `csharp-B05`, 채택 C#7.3 경계 `csharp-V73`이다. XML 파일은 `xml-B01`/`xml-B02`와 등록된 version 경계 `xml-V11`을 사용한다. WSDL/XSD는 XML source이며 별도 XSD language route를 추가하지 않는다. `.svc` composite case는 아래 별도 format 등록부에서 C#/XML의 적용 segment와 coverage를 연결하며 기존256행을 늘리거나 줄이지 않는다.

## `SVC-SERVICEHOST-r1` 형식과 위치

일반 XML/C# PASS는 `.svc` 전체 지원 근거가 아니다. [공식 ServiceHost 정의](https://learn.microsoft.com/en-us/dotnet/framework/configure-apps/file-schema/wcf-directive/servicehost)의 `Service`, `Factory`, `Debug`, `Language`, `CodeBehind`를 이름·값·quote·구분자·원문 span으로 보존한다. Factory/Language 생략도 등록한다. [공식 배치 설명](https://learn.microsoft.com/en-us/dotnet/framework/wcf/feature-details/deploying-an-internet-information-services-hosted-wcf-service)은 directive 뒤 inline source와 별도 구현 파일을 구분한다.

* 전체 원본 identity와 directive 시작/끝, 이름, attribute 이름/값/quote, `<%`·`@`·`%>` 경계를 반열림 byte/point span으로 기록한다. 여러 줄·공백·CRLF·LF·BOM을 보존한다. unknown/duplicate attribute·중복 directive의 진단과 미검증 의미를 보존한다.
* CodeBehind는 선언된 참조다. 자동 파일 접근이나 project 밖 탐색을 유발하지 않는다. resolved 여부와 별도 입력 identity는 caller의 승인·등록 파일 집합으로 정한다. directive-only·CodeBehind·inline coverage를 구분한다.
* inline이 있고 Language가 등록된 `C#`/`c#`일 때만 기존 C# producer를 재사용한다. inline Language 생략은 UNRESOLVED_LANGUAGE, VB/JS/기타는 UNSUPPORTED_LANGUAGE이며 C# PASS가 아니다. inline 없는 Language 생략 directive 관측과 구분한다.
* encoding 판별과 입력은 아래 [실사용 source 입력 정책](#실사용-source-입력-정책)을 따른다. UTF-8·UTF-16 LE/BE·선언된 cp949 모두 변환 없이 원본 bytes를 판별 encoding으로 입력하고 inline 구간은 included range로 지정한다. 불명 encoding·손상 sequence는 BLOCKED다. charset 추정·newline 정규화·directive 삭제 후 전체 원본 PASS·임의 wrapper 삽입은 금지한다.
* composite result는 전체 원본 identity, format revision, directive 관측, inline 언어와 included range, 판별 encoding identity, C# producer/source/tool, 원본 좌표의 node/capture span과 coverage를 결속한다. point는 [tree 계약](../specs/tree-and-adapter-protocol.md)의 0-based row와 byte column이다. included range와 원본 좌표 검증 없이는 전체 `.svc` 지원을 주장하지 않는다.
* 원본을 직접 입력하므로 producer byte/point는 원본 좌표다. row는 판별 encoding에서 해석한 LF 경계로 세며 column은 원본의 행 시작부터 실제 byte 수다. UTF-16 code unit 내부의 단독 `0x0A` byte를 줄바꿈으로 오인하지 않는다. BOM·CRLF·included range 경계를 원본 byte로 기록한다.
* 원본 byte 좌표 edit마다 directive/inline 경계와 included range를 재계산한다. damaged incremental/fresh, restored incremental/fresh, original syntax/구조를 각각 대조한다. segment/language 변경을 기록한다. 동일한 잘못된 tree의 복구는 지원 PASS가 아니다.

XML source는 데이터다. 객체 역직렬화, vendor assembly 로딩, 외부 entity/schema 취득, ResXFileRef·config/WSDL/XSD 참조 자동 접근을 수행하지 않는다. 기존 C#/XML 경로와 제한된 format 처리를 재사용하며 범용 ASP.NET/Razor compiler나 새 parser runtime을 만들지 않는다.

## 별도 case·기대값·producer 등록부

아래 ID는 향후 exact input/expectation manifest의 owner다. 현재 case 종류를 채택한 준비 등록이며 원문 hash·producer executable·실행 결과는 NOT_RUN이다. A의 39입력/7 producer edit와 B의 46행/3edit에 추가하지 않는다. P=positive, N=negative, R=recovery, E=byte edit.

| Case ID | 종류 | 사실/기대값 | Producer / owner |
|---|---|---|---|
| N461-CS-PARTIAL | P,N,R,E | Form/UserControl partial·base·필드·generated 멤버 | C# / S03,S05,S06 |
| N461-CS-INIT | P,N,R,E | InitializeComponent·nested cast·new/array/collection init·property·event·Dispose | C# / S03,S05,S06 |
| N461-CS-RESOURCE | P,N,R,E | generated resource 접근·qualified/global 이름 | C# / S03,S05,S06 |
| N461-CS-DX202 | P,N,R,E | 등록 component 생성/초기화·generic; binary 불필요 | C# / S03,S05,S06 |
| N461-CS-WCF | P,N,R,E | attribute·contract/implementation·generic proxy/Reference.cs | C# / S03,S05,S06 |
| N461-XML-RESOURCE | P,N,R,E | resx/culture·namespace·요소/속성/값·손상/복구 | XML / S03,S05,S06 |
| N461-XML-REFERENCE | P,N,R,E | config/WSDL/XSD declared reference; DTD/entity/schema/ResXFileRef 자동 접근 거부 | XML / S01,S02,S03,S05 |
| N461-SVC-DIRECTIVE | P,N | directive-only, Service 이름/값/원문 범위 | svc / S03,S05,S06 |
| N461-SVC-CODEBEHIND | P,N | CodeBehind 참조·inline 유무의 독립 coverage | svc+C# / S01,S02,S03,S05 |
| N461-SVC-INLINE | P,N,R,E | C# included range·전체 원본 byte/point | svc+C# / S03,S05,S06 |
| N461-SVC-MULTILINE | P,N | 여러 줄 attribute·optional Factory/Debug/Language | svc / S03,S05 |
| N461-SVC-QUOTE | N,R,E | quote 손상/복구, 오류 위치·후속 source 보존 | svc / S05 |
| N461-SVC-TERMINATOR | N,R,E | `%>` 손상/복구·segment 경계 진단 | svc / S05 |
| N461-SVC-BOUNDARY | P,N,R,E | directive/코드 경계 edit·included range 재계산·fresh 대조 | svc+C# / S03,S05 |
| N461-SVC-LANGUAGE | P,N | directive-only 생략, inline 생략 UNRESOLVED, 비C# UNSUPPORTED | svc / S03,S05 |
| N461-SVC-ENCODING | P,N,R,E | BOM/CRLF/LF/Unicode·UTF-16 직접 입력과 included range; 불명/손상 encoding 거부 | svc included range / S01,S02,S03,S05 |

## 실사용 source 입력 정책

`REAL-WORLD-SOURCE-r1`은 업무 실사용 C#·T-SQL·`.svc` source를 grammar에 넣는 정책이다. 2026-10-02 PREPARE에서 사용자가 채택했고 분리 context 검증 조건을 반영했다. 대용량 값의 profile identity는 `real-world-source-r2`(r1 대체)이며 2026-10-03 보완 값을 포함한다. 구현과 측정은 아래 담당 Session이 하며 이 문서는 지원 증거가 아니다.

**Encoding 판별.** 아래 순서에서 처음 결정되는 단계가 결과다. 추정은 하지 않으며 판별 encoding과 그 출처(BOM·검증·선언)를 결과 identity에 넣는다. 파일별 선언(`utf-8` 또는 `cp949`)이 있으면 4~5단계 대신 선언한 encoding으로만 검증하고, 실패하면 BLOCKED다.

1. UTF-32 BOM은 BLOCKED다.
2. UTF-8·UTF-16LE·UTF-16BE BOM이 있으면 그 encoding으로 내용을 검증한다. 실패하면 다른 encoding으로 넘어가지 않고 BLOCKED다.
3. BOM 없이 NUL byte가 있으면 BLOCKED다.
4. 파일 전체가 엄격한 UTF-8이면 UTF-8이다. 순수 ASCII도 UTF-8이다. 단, profile이 `cp949`를 선언했고 비ASCII bytes가 cp949로도 유효하면 AMBIGUOUS(BLOCKED)다. profile이 cp949를 선언하지 않으면 cp949는 후보가 아니다.
5. UTF-8이 아니면 profile이 `cp949`를 선언했을 때만 cp949로 검증한다.
6. 그 밖에는 BLOCKED다.

**UTF-16.** 원본 bytes를 tree-sitter UTF-16LE/BE 입력으로 그대로 넣는다. tree-sitter는 잘못된 sequence를 거부하지 않으므로 Go 사전 검증이 짝수 길이, 짝이 맞는 surrogate, U+0000 부재를 확인한다. edit 경계는 짝수 byte offset이다.

**CP949.** 엄격 사전 검증을 통과한 파일만 C 드라이버의 decode callback으로 파싱한다. decode callback은 오류로 중단할 수 없으므로 검증은 사전 단계에서 끝낸다. 매핑 표는 [WHATWG Encoding Standard](https://encoding.spec.whatwg.org/#euc-kr)의 `index-euc-kr`(identifier `1d97134cbf187263585bc8f593ca4196654ed4c7a673f5672eaad4f5d9fdc4ba`, 파일 sha256 `89af20dd867c84cefb710b1790229786cfef2bf11916361a210d81b90381e267`)이며 표는 CC BY 4.0, 소스에 넣은 부분은 BSD-3-Clause다. 고지를 보존한다. 이 표에 없는 Windows 전용 확장 code point는 BLOCKED다. S05가 source-prepare 예산 안에서 pin한 identity로 들여온다.

**동적 SQL.** 바깥 문장만 구조 해석한다. 문자열 안 SQL은 구조 해석하지 않고([tsql-S01](language-feature-disposition.md)) 위치 사실만 기록한다.

* 구문 종류는 닫힌 목록이다: `EXEC(...)`, `EXEC(...) AT` linked server, 이름을 정규화한 `sp_executesql`, C# command 생성자·initializer·`CommandText` 대입. `EXEC @module_var`는 동적 SQL이 아니다.
* 인자 종류도 닫힌 목록이다: `literal_unicode`, `literal_char`, `variable`, `bare_word`, `concatenation`, `other`.
* 사실은 구문 종류, 인자 종류, 원본 byte/point 범위, 변수 이름이다. 인자는 위치로 추출하며 `AS USER`와 pass-through 매개변수는 제외한다.
* `AT DATA_SOURCE`, `WITH RESULT SETS`, `EXEC` 없는 batch 첫 호출은 알려진 누락이다.
* C# 위치는 버전을 붙인 API 이름 목록으로 찾는 heuristic이며 결과에 heuristic으로 표시한다. `CommandType.StoredProcedure`는 알려진 false positive다.
* 이스케이프 없는 완성 리터럴을 단일 included range로 2차 파싱하는 것은 범위 밖의 후속 선택지다.

**대용량.** 제품 기본 `file_bytes` 16777216은 유지한다. 실사용 source는 별도 policy identity의 profile을 쓴다.

* 연산 값: 입력 33554432 bytes, encoded 요청 50331648 bytes, 응답 출력 16777216 bytes, ERROR/MISSING 목록 상한 1000건, wall 초과 후 종료 유예 5초다. 기본 `native-parse-edit`·`native-query`에도 tree depth 상한 100000을 둔다.
* 파싱 후 `descendant_count`가 50000 이하이고 출력이 16777216 bytes 이하면 전체 tree를 낸다. 아니면 summary를 낸다.
* summary는 canonical tree digest, 상한 있는 ERROR/MISSING 목록, 선언 구조 자동 검사(type·member·procedure 선언의 이름과 범위), 등록 지점 부분 tree를 포함한다. S05는 선언 구조를 S03 schema의 선언 node 종류 순회로 검사하며 query를 쓰지 않는다. S06 사실 query 세트는 같은 사실을 재현해야 한다. 오류 개수만으로는 구조 PASS가 아니다.
* 시간은 파싱당 60초(progress callback, 협조적 취소, S05 driver), 단일 parse 요청의 process wall 90초, edit 요청 300초(최대 4 edit, S04 runner)다. memory는 4 GiB다. 정책 wall·deadline 초과는 `RESOURCE_LIMIT`이고 caller의 명시적 취소만 `CANCELLED`다.
* memory 상한은 Linux cgroup과 Windows Job Object에서는 hard cap이다. macOS는 sampling 후 종료로 강제하며 이 profile은 macOS에서 non-strict로 결과에 기록한다. 할당 실패(runtime allocator hook으로 감지)·OOM·sampling 종료는 모두 `RESOURCE_LIMIT`다. macOS 결과는 hard cap 근거가 아니며, strict memory cap을 요구하는 operation은 [trust 계약](../specs/trust-and-execution.md)대로 macOS에서 BLOCKED다.
* traversal은 반복 cursor로 한다. 상한은 tree depth 100000, summary node 25000000(잠정), encoded request와 output bytes다. 연산의 `max_depth`는 요청/JSON 구조 중첩이며 tree depth가 아니다. 깊은 중첩은 depth 10000 이상 정상 1건과 상한 초과 1건(`RESOURCE_LIMIT`)으로 세 OS에서 확인한다.
* 대형 입력 query는 `native-query-large`(입력 33554432 bytes, node 25000000, wall 90초, 출력 16777216 bytes, capture 1000000)를 쓴다.
* hosted 측정이 값에 못 미치면 S05-A18 결과로 기록하고 S05 종료를 막지 않으며, 값을 올리는 것은 사용자 결정이다.
* 세 OS 측정은 합성 대형 fixture(크기는 fixture identity가 고정)로 한다. 비공개 source는 로컬에서만 측정한다. 자동 생성 파일도 전체 파싱하되 집계를 분리한다.

| 담당 | 책임 |
|---|---|
| S01 / #3 | 판별 순서·BOM·NUL·UTF-8 검증·UTF-16 사전 검증·선언 처리와 negative, 입력 size 상한, profile·파일별 encoding 선언 필드 고정 |
| S03 / #5 | 동적 SQL node/field mapping, summary 출력 schema, tree envelope의 판별 encoding 필드, 원본 byte/point |
| S04 / #6 | wall·memory 강제 방식의 resource policy 필드 |
| S05 / #7 | `index-euc-kr` 취득, 표가 필요한 cp949 검증·AMBIGUOUS 판정 완성과 decode 실행, UTF-16/cp949/동적 SQL/대용량/깊은 중첩 fixture와 세 OS 측정, 선언 node 종류 순회 검사, full tree gate·상한 값의 profile revision |
| S06 / #8 | 동적 SQL 위치 사실 추출, 사실 query 세트(S05 선언 사실 재현) |
| S07 / #9 | 비공개 corpus workload 등록, 로컬 실행 identity와 evidence 결속 |
| S08 / #10 | `real-world-source-r2`와 판별 규칙으로 비공개 corpus 전체 로컬 qualification, 세 OS는 OWNED_FIXTURE 비교 |

[profile r0](../specs/cli-and-profile.md)는 encoding 선언, wall·memory, full tree gate를 표현하지 못하고, [tree envelope r0](../specs/tree-and-adapter-protocol.md)의 `input.encoding`은 판별 encoding과 출처를 표현하지 못한다. 위 표의 담당 Session이 각 r0 확장 revision을 고정하기 전에는 그 필드에 의존하는 실행을 하지 않는다. S01은 표가 필요한 cp949 검증·AMBIGUOUS 판정과 cp949 decode를 하지 않으며, 그 경로는 S05가 표를 pin한 뒤 완성한다.

## 비공개 로컬 corpus

`NET461-PHASE2-LOCAL-r1`은 2026-10-03 사용자가 등록한 업무 실사용 corpus다. corpus root는 caller가 로컬 입력으로 주며, 경로는 추적하지 않는 로컬 기록에만 둔다. 위 정책과 역할 등록부로 S01부터 다루며, 등록은 지원 증거가 아니다.

* 로컬 전용이다. hosted CI로 보내지 않는다. 2026-10-03 사용자 결정으로 실행 모델과 분리 리뷰어는 진단을 위해 소스 내용을 열람할 수 있으며, 그 내용이 모델 제공자에게 전송되는 것을 감수한다. 경로·이름·hash·내용이 담긴 기록은 추적하지 않는 로컬 artifacts에만 둔다. 공개 commit·PR·Issue에는 개수와 판정만 남긴다.
* ERROR 진단은 실행 모델이 로컬 내용으로 한다. 처분할 수 없는 파일은 `UNDISPOSITIONED`로 두고 사용자에게 묻는다.
* 자격 증명 성격 파일(`.pfx`, `.p12`, `.snk`, `.key`, `.pem`, `.jks`, `.keystore`, `.pvk`)과 vendor binary(`.dll`, `.exe`, `.pdb`)는 존재와 크기만 기록하고 내용을 읽거나 복사하지 않는다(`PRESENCE_ONLY`). 자격 증명 파일은 모델과 리뷰어도 읽지 않으며, Claude Code에서는 로컬 deny 규칙으로 구조적으로 막는다.
* `.config` 등의 연결 문자열·비밀번호 같은 비밀 값은 인용·출력·전송하지 않으며, 결과에서는 tree/capture 범위로만 다룬다.
* `bin`·`obj`·`.vs`·`packages`·`TestResults` 빌드 산출물과 `.git`·`.svn`·`.hg` VCS 디렉터리는 제외한다. MSBuild는 실행하지 않는다. `.csproj`에 적힌 포함 목록만 관측한다. `\`는 `/`로 정규화하고, 조건·import·wildcard·`$(...)` 속성은 `UNRESOLVED`, corpus root 밖이거나 없는 항목은 읽지 않고 `NOT_FOUND`다. 파일마다 N461 역할은 하나다.
* S01은 이를 위해 소유 계약인 [identity/evidence](../specs/identity-and-evidence.md)와 [공개 API](../specs/public-go-api.md)를 개정할 수 있다. manifest revision에서 판별 encoding을 identity에 결속하고, sha256 없는 `PRESENCE_ONLY` 레코드와 corpus 진입점을 정의한다.
* 연산은 `private-corpus-local`이다. 파일 26000, 합계 3489660928 bytes, 단일 파일 33554432 bytes, 레코드 26000이다. 제외 디렉터리는 읽지 않고 잘라내며 수에 넣지 않는다. `PRESENCE_ONLY` 파일은 파일 수와 레코드에는 넣고 bytes에는 넣지 않는다. 레코드는 파일 항목 하나가 1개이며 포함 관계·중복 묶음은 항목의 필드다. 디렉터리 깊이 32, inventory 보고 67108864 bytes, Go 프로세스 memory 2147483648 bytes다. native 단계는 batch 요청으로 프로세스 하나가 최대 500 파일 또는 268435456 bytes를 처리하며 1 invocation으로 센다(파일당 60초, batch 프로세스 wall 3600초). in-process inventory는 invocation으로 세지 않는다. pg-large-source-r1 예외는 이 연산에 적용하지 않는다. 보존은 파일당 레코드(상태·has_error·digest·상한 있는 ERROR 범위·encoding)만이며 full tree는 보존하지 않는다. 로컬 저장은 실행 1회당 2147483648 bytes다. 실행 wall은 S01 1800초, S05 3600초, S07 1800초, S08 7200초다. S07 비공개 replay는 파일 26000, 레코드 합계 2147483648 bytes다. corpus profile은 `cp949`를 선언하고, 파일별 선언은 결과 관측 전 등록한 로컬 manifest로만 준다. 한도를 넘으면 `RESOURCE_LIMIT`이며 자동으로 올리지 않는다.
* corpus route 표: `.cs`(`.Designer.cs`·`.svc.cs`·Reference.cs 포함)는 csharp, `.sql`은 tsql, `.svc`는 `SVC-SERVICEHOST-r1` composite, `.config`·`.resx`·`.xsd`·`.wsdl`·`.xml`·`.settings`·`.datasource`는 xml이다. route는 N461 역할과 무관하게 확장자로 정하며, 표에 없는 확장자만 route 없음(unrouted)이다. `UNCLASSIFIED`는 역할 축에만 쓰는 값이고 route를 없애지 않는다. S01이 역할을 붙이고 S07이 이 표를 workload로 등록한다. route를 줄이는 변경은 분리 리뷰와 사용자 결정이 필요하며 결과를 관측하기 전에만 적용한다.
* S08 판정은 route가 있는 파일마다 `execution_status` COMPLETED(`has_error` 구분)·CANCELLED·RESOURCE_LIMIT·FAILED, 또는 실행하지 않은 `assessment=BLOCKED`(인코딩·정책)로 집계한다. unrouted와 `PRESENCE_ONLY` 파일은 따로 센다. 통과는 kit 결함 0, 모든 파일의 상태 집계, ERROR 파일(COMPLETED이면서 `has_error`) 전부 처분(grammar gap·원본 손상·미지원, 또는 사용자에게 넘긴 `UNDISPOSITIONED`), CANCELLED·RESOURCE_LIMIT·FAILED 파일의 한도·환경·kit 결함 분류다. 분류하지 못한 FAILED는 kit 결함으로 센다. 비율 임계값은 없다. 26 route·78칸과 별도 row이며, 세 host 비교는 OWNED_FIXTURE만 쓴다.

## Session 책임과 준비 전제

| Session / Issue / Milestone | 추가 책임 |
|---|---|
| S01 / #3 / MS2 | role·입력 identity/size/encoding·원본 포함 관계·허용 root·included range 입력 제한 |
| S02 / #4 / MS3 | 안전한 참조·entity/schema/ResXFileRef 비실행·private source/effect 경계 |
| S03 / #5 / MS4 | C#/XML 구조 mapping·svc composite-result·원본 byte/point 계약 |
| S05 / #7 / MS6 | 등록 format syntax/구조/negative/recovery/edit; 기존 bounded runner 재사용 |
| S06 / #8 / MS7 | C#/XML query/capture·svc directive/segment 관측 |
| S07 / #9 / MS8 | 원본/판별 encoding·cp949 매핑 표/expectation/producer/실행 identity·private 근거 |
| S08 / #10 / MS9 | Windows amd64/Linux amd64/macOS arm64 **source parser** qualification 별도 결과(세 host는 OWNED_FIXTURE, 비공개 corpus는 로컬 Windows) |

PREPARE 필수 전제는 scope/roles/format/위치·encoding/안전 경계/case 종류/owner와 feasibility 경로의 정합성이다. C# 등 producer의 현재 필수 source 격차는 P05에 남긴다. 비공개 로컬 corpus는 등록됐지만 실행은 NOT_RUN이고, 미제공 업무 파일·svc 제품 기능·S08 78셀도 NOT_RUN/NOT_PROVIDED이며 미래 업무 검증 전체를 pre-S01 gate로 추가하지 않는다. 세 host source parser 검증은 WinForms/WCF 애플리케이션의 세 플랫폼 실행이 아니다.

새 native 실행은 정확한 owned/비공개 input·expectation·producer·owner·effect·유한 한도를 별도 manifest에 연결하고 기존 allowlist 포함 여부를 확인한다. private 원본 hosted 전송 권한은 없다. 이번 scope 채택은 source/build/runtime/designer 지원 증거가 아니다.
