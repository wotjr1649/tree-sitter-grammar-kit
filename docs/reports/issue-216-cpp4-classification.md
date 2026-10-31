# #216 CPP4 정의 연결과 Windows 분류 관측

Windows/amd64에서 C++의 고정 빈 입력을 실제 native/reference로 수집하고, 독립 graph 검토와 외부 caller approval 뒤 같은 guarded snapshot의 정의 projection 및 별도 공개 API parity를 통과했다. 결과는 CPP4 정의 4개의 조건부 `STRUCTURAL_ONLY`이며 `semantic_evaluated:false`다. 정적 원장의 `STRUCTURAL_CANDIDATE`, `compiled_metadata:UNVERIFIED`, `semantic_required:null`·`NOT_ASSESSED` 및 전체 #216/#195의 OPEN 상태는 유지한다.

## 원문과 관측 범위

[계측 reference 계약](../specs/instrumented-query-reference.md)의 닫힌 `cpp4-empty` profile을 사용했다. [CPP query 원문](../../src/testdata/native/queries/cpp.json), [CPP4 anchor](../../src/testdata/query-semantics/cpp4-classification-anchors.json), [슬롯 원장](../../src/contracts/query-slot-inventory-r1.json), [qualification](../../src/contracts/qualification-c1.json)의 ID·순서·정확한 UTF-8 원문/hash를 연결했다.

| Query | Patterns | 순서 있는 capture names |
|---|---:|---|
| `q.structure` | 4 | `namespace`, `struct`, `concept`, `class` |
| `q.probe-cpp-V14` | 3 | `init`, `vartemplate`, `num` |
| `q.probe-cpp-V17` | 2 | `ns`, `binding` |
| `q.probe-cpp-V23` | 2 | `self`, `index` |

네 predicate 목록은 명시적인 빈 배열이다. 실제 입력은 빈 UTF-8 source 하나, edit 없음, `native-query`·single frame·`api=false`였다. Native와 네 reference에서 named `translation_unit` root 하나, step0 `NO_ERROR`, 빈 match/capture를 독립 기대값과 대조했다. ABI language/runtime/minimum은 `15/15/13`, symbols/fields는 `621/53`이다.

Ordinary `s06-cpp`의 77 cases, 13 edits, 90 case steps, 103 tree roles, 412 potential query-tree links는 정의의 정적 연결 범위다. 이번 입력이 원래 77개 사례를 실행하거나 의미 평가한 것은 아니다. 실제 좌표 digest는 `97412ed156c044b7b58cd5b0bf237cb0e10e4848289b117f3fa208ef1692ba5a`다. 합성 preflight의 좌표 대체값을 실제 digest로 채택하지 않았다.

## 실행 identity와 parser paging

Capture·조립·guarded 정의 검증의 관측 source와 reader revision은 `b85987720e69eb3930552d4680db5b033759c192`, tree는 `31f18a5c7150da87c56860fe274a9a1e83df7dc0`이다. 이후 보고서·PR·merge revision을 당시 실행 source로 소급하지 않는다. Source manifest는 708개 tracked 파일과 capture controller 4개의 pin을 기록하며 SHA-256은 `ea19c953b5d68da6012c609d8579fe5757f82fb749c24d641794421d4ed85226`이다.

Run ID는 `cpp4-empty-20261031-b859877-1`, preregistration은 3,023 bytes/SHA-256 `fdd82426c4d7f1141635be1e5916e1c6d42fd350d955df9fa3edfa4c96f0ba31`이다. CPP query file은 4,734 bytes/SHA-256 `fc90268c86c9133e2801b2fb7a20bda479137aacc0445911fa5046f11fd65766`, anchor는 3,537 bytes/SHA-256 `e807345cb9a528facbaddaae3b914027acc4c99c8148f65f365e83f4b58ee5b4`다.

Parser는 49,598,509 bytes/SHA-256 `3856d51ead4b6ecf5d4e1ec3fd39c52c9ba0cb535e69ea1785822280c45b75fc`다. Owner `D`의 descriptor와 세 part를 사용하며 크기는 16,777,216 / 16,777,216 / 16,044,077 bytes, offset은 0 / 16,777,216 / 33,554,432다. Descriptor의 743 bytes와 logical parser의 49 MB를 구별해 owner·역할·순서·길이·개별 및 재결합 hash를 결속했다. Scanner 6,772 bytes와 header 3개를 포함한 logical grammar 5개도 사전등록과 일치한다.

별도 context `full-input-utf8-paged-parser/r1`로 CPP4만 받아들이며 기존 CJR의 CPP 거부와 unpaged parser의 16 MiB 한도를 유지한다. 공개 core API·revision·의미 분모를 바꾸지 않았다.

## 실제 수집·조립과 caller 승인

CPP4 전용 native·kit test binary를 고정 source로 각각 한 번 빌드하고 source/SDK 입력을 전후 확인했다. 이후 수집은 library compiler 1회, native 관측 1회, 개별 reference 4회였다. Native 안의 GCC version/runtime/parser/scanner/driver/shim/link 일곱 단계도 원 receipt로 보존했다. 모든 실제 작업은 exit 0, truncation 없음, HARD memory 한도 및 검증된 Job cleanup 잔여 0을 기록했다.

| 작업 | 실제 wall ms | Peak job bytes | HARD memory 한도 |
|---|---:|---:|---:|
| Library compiler | 4,202 | 346,185,728 | 4 GiB |
| Native 관측 | 11,902 | 508,502,016 | 4 GiB |
| Reference 0 / 1 / 2 / 3 | 242 / 99 / 93 / 95 | 43,876,352 / 43,843,584 / 43,929,600 / 43,958,272 | 각 1 GiB |
| Offline 조립 Go | 519 | 114,167,808 | 4 GiB |
| Guarded 정의 Go | 1,055 | 442,306,560 | 4 GiB |

수집 부모 Job은 42,721 ms·peak 550,928,384 bytes/HARD 4 GiB였다. 수집 원 session `30629`, 조립 원 session `48562`는 각각 exit 0으로 닫혔다. 조립은 저장된 binary로 한 번 실행해 304개 파일·15개 디렉터리의 새 graph를 만들었다. 원 case 136개와 합쳐 retained case는 440개 파일·28개 디렉터리다.

Graph는 70 nodes, 299 owned members, 67 invocations, 10 tools다. 기존 Windows donor의 build/control 62 invocations(9+53), 10 tools, 178 owned members를 원 source·runner·controller·context·session과 함께 보존했다. 과거 current invocation 세 개를 CPP 관측으로 복사하지 않았다. Graph 밖 Root가 실제 종료·raw receipt/streams·source/parser/library/native/reference·ownership 결속을 독립 검토하고 CPP4 전용 승인을 새로 발급했다.

| 근거 | SHA-256 |
|---|---|
| Evidence graph | `2d457d0d32a00bade08b0999e1c372e24911c322366f2ccc17a5b2aee3caad9f` |
| Registration | `1719ea0f045c6319af3c21683328dd2225c14a94199a19a7ccf07ebf3737a39d` |
| Caller approval | `a2a087a39a1960108ca487e9fe6834ac578d0701a8ed008bf2f076f629a7deaa` |
| Root 승인 결정 | `1cbe781bbfd4824a31cbe617e7be9248a580bd8a472dd1cc1e2d0264605a4421` |
| Definition result | `2aa378427964de95c320e5d852e9b58cbd4fee5df618765a9bbe1c8995d920f9` |
| Root 실행 채택 | `d5049fe42c26a5cbdb3ae917061c6e6a529d7ba85aa5ce9343056df5e3498d70` |
| 실제 결과 독립 검토 | `e3b4493f0d1bcf0cb833ffbc428b10ab180a4c004f448ff778d471b6be01587b` |
| 실제 결과 보안 검토 원문 | `5d0b664bdc11578b38700447a1a7fc3d28091e160fdeb5c34f841c21ade9013e` |

## Guarded 검증과 원본 종료

`TestQueryClassificationCPP4Local`의 `definitions` stage를 기존 고정 kit binary로 정확히 한 번 호출했다. 그 호출 안에서 retained core snapshot의 eligibility·독립 CST·request/frame·compiler/library·native/reference metadata 결속을 검사하고 별도 공개 eligibility API의 상태·counts·digest parity를 확인한 뒤 같은 retained snapshot으로 projection을 생성했다. 새 Go build·compiler·native·reference 실행과 별도 사전 verify 호출은 없었다.

실제 Go Job은 1,055 ms·peak 442,306,560 bytes/HARD 4 GiB, exit 0, cleanup 확인·잔류 0이었다. Child wall 180초·Go test 120초·inner 전체 240초 한도를 지켰다. Root 원 session `68066`도 exit 0으로 닫혔고 전체 terminal 관측은 17,221 ms였다. Root의 300초는 OS hard watchdog이 아닌 외부 owner의 관측·종료 책임이다. 이 실행은 정상 종료해 강제 정지가 필요하지 않았다.

입력 read lease 1,180개, 내부 원본 산출물 9개, 외부 정의 결과 1개의 총 1,190개를 사후 hash/physical 대조하고 해제했다. Root 자기 image lease도 해제했다. Retained case 440개와 graph 304개를 바꾸지 않고 외부 새 directory에 10,475 bytes의 결과 하나만 생성했다. Root return의 `RETURNED_PENDING_ORIGINAL_TOOL_CHECK` 상태는 원문 그대로 두고, 동일 session의 별도 original-tool exit 0과 함께 채택했다.

결과의 guarded tuple은 required 4 / verified 4 / failed 0 / blocked 0 / unprocessed 0이다. 네 row는 조건부 `STRUCTURAL_ONLY`이고 정적 inventory의 `STRUCTURAL_CANDIDATE`·`UNVERIFIED`·`NOT_ASSESSED`는 유지한다. 기존 Go 3개 정의(positive-batch/native-query-batch)·C#15·CJR13과 이번 CPP4를 중복 없이 합친 35/130개는 서로 다른 profile 실행과 source/reader revision에서 얻은 조건부 분류의 역사적 누계다. 이 Go 실행은 기존 빈 입력 Go3 보고서의 source `4f7c67a7`과 별개이며, source/reader는 각각 `2b1e6814`/`671ab226`, C#는 `18eb5736`/`18eb5736`, CJR은 `4eff1259`/`4eff1259`, 이번 CPP4만 `b8598772`/`b8598772`다. 공통 고정 inventory를 쓰지만 하나의 입력 실행이나 하나의 source revision에서 35개를 검증한 결과는 아니다. 나머지 95개 및 전체 ordinary 사례·semantic 평가 완료는 주장하지 않는다.

실제 결과의 독립 검토는 B0/M0/Minor0, 별도 보안 검토는 B0/M0/Minor1로 완료했다. 보안 검토는 실행 전 graph/approval 검토와 별개로 실제 결과·원본 종료·request/receipt·좌표·보존 파일 맵을 대조했다. Minor는 독립 검토 보고서의 `public_api_parity_run:false`가 읽기 전용 검토에서 재실행하지 않았다는 뜻임을 분명히 하라는 표기 지적이다. 원 실행은 공개 API parity를 검사한 뒤 projection하는 경로를 SKIP 없이 PASS했으며, 이 보고서의 parity 설명은 그 원 실행에 한정한다. 검토자가 parity를 새로 실행한 것으로 해석하지 않는다.

Graph·승인·결과 hash의 원본은 미추적 로컬 보존 증거다. 추적 보고서와 hosted CI만으로 해당 원본을 직접 대조하거나 당시 관측을 독립 재구성·재승인할 수 없다.

## 검증과 보존 한계

구현 source의 focused 검사는 39 PASS·8 manual SKIP, vet와 foundation 검사는 통과했다. SKIP을 실제 수집 성공으로 세지 않았다. 별도 실제 수집·조립·guarded 실행 근거는 위와 같다. 실행 전 검사에서는 실제 입력과 Root admission prefix의 1,180개 lease 개방·사후 대조·해제를 확인했다. 합성 wire와 실패 I/O 시험은 잘못된 digest·logical parser 크기·provenance `attempt:0` 누락, 읽기 오류와 EOF 구별, 시작 미확인의 `null`, 이중 기록 실패 시 첫 원인과 raw prefix/tail 보존 상태를 검사했다. 이 fixture는 실제 Go/Job 실행이나 실제 snapshot·의미 평가의 대체 근거가 아니다.

미실행 controller 초안에서 환경·경로·plan/결과 계약 및 실패 기록 문제가 발견돼 원본을 보존하고 새 버전에서 교정했다. 실제 단일 호출 전 두 검토의 미해결 finding은 B0/M0/Minor0이었다. 같은 경로로 수정된 Root 초안도 최초·중간 버전을 별도 보존했고 실행에 사용하지 않았다. 사용하지 않은 bootstrap 초안의 최초 file-length 관측에는 raw bytes/hash가 없어 내용 변경이나 partial write로 단정하지 않았다. 기존 빈 출력 디렉터리도 생성 원인을 추정하거나 지우지 않고 별도 새 경로를 사용했다.

기존 GCC와 하위 compiler·linker·sysroot·CRT·runtime, Python·Git·PowerShell·Windows 및 선택 입력은 변경하지 않는 cooperative host라는 전제다. 선별 anchor·hash·receipt를 전체 toolchain closure, vendor/publisher 인증, 실행 진위의 서명 또는 적대적 동시 변경 방지로 해석하지 않는다. 이전 정적 SDK 원장 생성기의 원 stdout/exit와 미실행 66,979-byte transient 초안의 정확한 bytes 미회수 한계도 남아 있다. 실제 준비의 live SDK/source 전후 검증과 원본 종료는 별도 근거다.

이 보고서는 CPP4 로컬 관측 부분의 기록이다. 전체 #216/#195, Linux/macOS 정의 분류, 전체 query qualification, semantic·per-case 완료 및 release 완료를 뜻하지 않는다. PR 필수 CI·공식 artifact와 실제 main 검증은 별도 통합 gate로 기록한다.
