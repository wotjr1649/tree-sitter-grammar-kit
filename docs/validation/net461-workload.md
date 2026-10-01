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
| N461-SVC | `.svc` | `SVC-SERVICEHOST-r1` directive + 등록 inline C# + 원본 위치 mapping |
| N461-XML | App.config/Web.config, 필요한 기존 WSDL/XSD | XML source, namespace/qualified 이름, 값·import/include/설정 참조 선언 / inert XML |

실제 20.2.x·사용 component·compiler·LangVersion은 caller가 읽기를 허용한 metadata에서 확인하고 미확인 값은 UNKNOWN이다. project 설정을 변경하지 않는다. 역할의 실제 파일이 제공 corpus에 없으면 NOT_PROVIDED이며 요구는 유지한다. 소스 구문/구조, net461 build, Framework runtime 실행, designer/WCF 통합은 별도 축이다. IIS/WCF endpoint·폼·component·designer 실행, SDK 설치·restore·MSBuild target·svcutil·runtime downgrade는 승인되지 않았다.

## `SVC-SERVICEHOST-r1` 형식과 위치

일반 XML/C# PASS는 `.svc` 전체 지원 근거가 아니다. [공식 ServiceHost 정의](https://learn.microsoft.com/en-us/dotnet/framework/configure-apps/file-schema/wcf-directive/servicehost)의 `Service`, `Factory`, `Debug`, `Language`, `CodeBehind`를 이름·값·quote·구분자·원문 span으로 보존한다. Factory/Language 생략도 등록한다. [공식 배치 설명](https://learn.microsoft.com/en-us/dotnet/framework/wcf/feature-details/deploying-an-internet-information-services-hosted-wcf-service)은 directive 뒤 inline source와 별도 구현 파일을 구분한다.

* 전체 원본 identity와 directive 시작/끝, 이름, attribute 이름/값/quote, `<%`·`@`·`%>` 경계를 반열림 byte/point span으로 기록한다. 여러 줄·공백·CRLF·LF·BOM을 보존한다. unknown/duplicate attribute·중복 directive의 진단과 미검증 의미를 보존한다.
* CodeBehind는 선언된 참조다. 자동 파일 접근이나 project 밖 탐색을 유발하지 않는다. resolved 여부와 별도 입력 identity는 caller의 승인·등록 파일 집합으로 정한다. directive-only·CodeBehind·inline coverage를 구분한다.
* inline이 있고 Language가 등록된 `C#`/`c#`일 때만 기존 C# producer를 재사용한다. inline Language 생략은 UNRESOLVED_LANGUAGE, VB/JS/기타는 UNSUPPORTED_LANGUAGE이며 C# PASS가 아니다. inline 없는 Language 생략 directive 관측과 구분한다.
* UTF-8(유효 bytes, BOM 유무)은 원본 slice와 offset mapping을 기록한다. UTF-16 LE/BE는 BOM과 caller의 encoding 등록을 확인한 뒤에만 lossless UTF-8 변환을 허용하며 원본↔변환 byte/point 경계 mapping을 결속한다. 불명 encoding·손상 sequence·mapping 불가능 경계는 BLOCKED다. charset 추정·newline 정규화·directive 삭제 후 전체 원본 PASS·임의 wrapper 삽입은 금지한다.
* composite result는 전체 원본 identity, format revision, directive 관측, inline 언어/slice, 변환/mapping identity, C# producer/source/tool, 원본에 대응한 node/capture span과 coverage를 결속한다. producer local span도 보존한다. point는 [tree 계약](../specs/tree-and-adapter-protocol.md)의 0-based row와 byte column이다. mapping 없이는 전체 `.svc` 지원을 주장하지 않는다.
* 원본 byte 좌표 edit마다 directive/inline 경계와 mapping을 재계산한다. damaged incremental/fresh, restored incremental/fresh, original syntax/구조를 각각 대조한다. segment/language 변경을 기록한다. 동일한 잘못된 tree의 복구는 지원 PASS가 아니다.

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
| N461-SVC-INLINE | P,N,R,E | C# slice·전체 원본 byte/point mapping | svc+C# / S03,S05,S06 |
| N461-SVC-MULTILINE | P,N | 여러 줄 attribute·optional Factory/Debug/Language | svc / S03,S05 |
| N461-SVC-QUOTE | N,R,E | quote 손상/복구, 오류 위치·후속 source 보존 | svc / S05 |
| N461-SVC-TERMINATOR | N,R,E | `%>` 손상/복구·segment 경계 진단 | svc / S05 |
| N461-SVC-BOUNDARY | P,N,R,E | directive/코드 경계 edit·mapping 재계산·fresh 대조 | svc+C# / S03,S05 |
| N461-SVC-LANGUAGE | P,N | directive-only 생략, inline 생략 UNRESOLVED, 비C# UNSUPPORTED | svc / S03,S05 |
| N461-SVC-ENCODING | P,N,R,E | BOM/CRLF/LF/Unicode·등록 UTF-16 mapping; 불명/손상 encoding 거부 | svc mapping / S01,S02,S03,S05 |

## Session 책임과 준비 전제

| Session / Issue / Milestone | 추가 책임 |
|---|---|
| S01 / #3 / MS2 | role·입력 identity/size/encoding·원본 포함 관계·허용 root·mapping 입력 제한 |
| S02 / #4 / MS3 | 안전한 참조·entity/schema/ResXFileRef 비실행·private source/effect 경계 |
| S03 / #5 / MS4 | C#/XML 구조 mapping·svc composite-result·원본 byte/point 계약 |
| S05 / #7 / MS6 | 등록 format syntax/구조/negative/recovery/edit; 기존 bounded runner 재사용 |
| S06 / #8 / MS7 | C#/XML query/capture·svc directive/segment 관측 |
| S07 / #9 / MS8 | 원본/변환/mapping/expectation/producer/실행 identity·private 근거 |
| S08 / #10 / MS9 | Windows amd64/Linux amd64/macOS arm64 **source parser** qualification 별도 결과 |

PREPARE 필수 전제는 scope/roles/format/위치·encoding/안전 경계/case 종류/owner와 feasibility 경로의 정합성이다. C# 등 producer의 현재 필수 source 격차는 P05에 남긴다. 미제공 업무 파일·전체 corpus·svc 제품 기능·S08 78셀은 NOT_RUN/NOT_PROVIDED이며 미래 업무 검증 전체를 pre-S01 gate로 추가하지 않는다. 세 host source parser 검증은 WinForms/WCF 애플리케이션의 세 플랫폼 실행이 아니다.

새 native 실행은 정확한 owned/비공개 input·expectation·producer·owner·effect·유한 한도를 별도 manifest에 연결하고 기존 allowlist 포함 여부를 확인한다. private 원본 hosted 전송 권한은 없다. 이번 scope 채택은 source/build/runtime/designer 지원 증거가 아니다.
