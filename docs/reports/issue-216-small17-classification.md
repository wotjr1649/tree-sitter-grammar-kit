# #216 Small17 정의 연결과 Windows 분류 관측

Windows/amd64에서 JSON1·YAML1·XML2·HTML2·CSS11의 고정 입력을 실제 native/reference로 수집했다. 독립 graph 검토와 새 외부 caller approval 뒤 공개 API parity 및 같은 guarded snapshot의 정의 projection을 통과했다. 정의 17개의 결과는 조건부 `STRUCTURAL_ONLY`이며, 정적 inventory의 `STRUCTURAL_CANDIDATE`·`compiled_metadata:UNVERIFIED`·`NOT_ASSESSED`와 `semantic_evaluated:false`는 유지한다. 전체 #216/#195와 원래 사례별·semantic 평가 완료를 뜻하지 않는다.

## 입력과 분모

[닫힌 Small17 계약](../specs/instrumented-query-reference.md#small17-단일-frame-정의-분류)의 다섯 profile을 사용했다. UTF-8 source 하나, edit 없음, `native-query`·single frame·`api=false`다. Native 요청의 API는 false이고 응답은 null이므로 tree API 관측으로 해석하지 않는다.

| Profile | Source | 정의 / patterns / capture-name 항목 | 전체 CST nodes | ABI symbols/fields | 정적 잠재 연결 |
|---|---|---:|---:|---:|---:|
| `json1-tiny` | `null` | 1 / 3 / 4 | 2 | 23/2 | 71 |
| `yaml1-empty` | 빈 source | 1 / 4 / 4 | 1 | 301/2 | 71 |
| `xml2-tiny` | `<r/>` | 2 / 7 / 9 | 6 | 142/2 | 196 |
| `html2-empty` | 빈 source | 2 / 8 / 7 | 1 | 48/0 | 100 |
| `css11-empty` | 빈 source | 11 / 22 / 22 | 1 | 204/0 | 3,366 |
| 합계 | 5 frames | 17 / 44 / 46 | — | — | 3,804 |

모든 query의 predicate 목록은 명시적인 빈 배열이다. ABI language/runtime/min-compatible은 모두 15/15/13이다. CSS symbols 204는 generated symbol 199와 alias 5를 합한 runtime API 관측이다. JSON의 `document`·named `null`, XML의 `document`·`element`·`EmptyElemTag`·이름·구두점과 모든 node의 parent·field·flags·byte/point span을 anchor와 대조했다. YAML·HTML·CSS는 각각 `stream`·`document`·`stylesheet` root다. XML fact query의 capture 순서는 바깥 element 뒤 element.name이다.

[JSON](../../src/testdata/native/queries/json.json), [YAML](../../src/testdata/native/queries/yaml.json), [XML](../../src/testdata/native/queries/xml.json), [HTML](../../src/testdata/native/queries/html.json), [CSS](../../src/testdata/native/queries/css.json)의 전체 원문 bytes·SHA와 ID·순서·metadata를 결속했다. XML의 별도 `facts.xml_structure`는 [fact query pack](../../src/contracts/fact-query-pack.json)의 원본에도 연결했다. 고정 grammar closure 29개는 JSON4·YAML8·XML6·HTML6·CSS5이며 role/path/bytes/SHA, nested XML 경로, YAML schema C 파일의 header 역할을 보존했다.

[슬롯 원장](../../src/contracts/query-slot-inventory-r1.json)과 [qualification](../../src/contracts/qualification-c1.json)의 원래 좌표는 다음과 같다. 이 사례나 잠재 연결을 이번 실행 횟수로 세지 않는다.

| Route | Cases | Edits | Case steps | Tree roles | Potential query-tree links |
|---|---:|---:|---:|---:|---:|
| JSON | 23 | 24 | 47 | 71 | 71 |
| YAML | 27 | 22 | 49 | 71 | 71 |
| XML | 30 | 34 | 64 | 98 | 196 |
| HTML | 34 | 8 | 42 | 50 | 100 |
| CSS | 246 | 30 | 276 | 306 | 3,366 |

## Source와 reference 교정

수집·조립·guarded reader의 source는 `59deb3c8498f03c9060b8218835f9cb29aacdd85`, tree는 `e51e99bd616a1f2c59928411678a5ad01efbf710`이다. 이후 보고서·PR·merge revision을 당시 실행 source로 소급하지 않는다. Source manifest는 tracked 파일 716개와 별도 controller 5개의 핀을 기록하며 SHA-256은 `fa5f1f01f3a3023ca85abd006d21ec37fb35815c8f5fe7457c2dff587d313a00`이다. Native/kit test binary도 이 source로 각각 빌드하고 선택 SDK·module 입력 12,604개를 전후 대조했다.

Rust reference의 정상 match는 `next_capture` 순회 중 같은 match의 capture 배열이 prefix를 유지하며 늘어날 수 있다. 종전 collector가 이를 일괄 변경으로 거부하던 경로를 고쳤다. 이미 관측한 prefix·pattern·keep 상태의 축소·재작성은 계속 거부하며, raw capture cap은 상태 변경 전에 적용한다. Predicate evaluator의 의미는 유지하고 unchanged 경로도 raw 관측 예산에서 제외하지 않는다.

새 reference의 source는 `9eba39be83174829ef81899f79b13aaccb77aa69`이고 binary SHA-256은 `019ef75611390ae078e6c12b3dc006dcb2372e9946f88cf1bd34472371f918f7`이다. 새 Windows build9·Rust unit4·control53·assertion36·pipeline14/skip0과 exact XML/plain kept/rejected 회귀를 통과했다. 추가 회귀를 canonical 9/4/53/36 분모에 더하지 않았다. 기존 실패 C11 수집을 성공으로 바꾸거나 재사용하지 않고, 새 C12 source/controller/preregistration/binary로 수집했다.

## 수집·조립·승인

C12 수집은 library compiler5·native5·서로 다른 reference17의 원본 27회다. Root와 compiler/native는 HARD 4GiB, reference는 각 HARD 1GiB이며 원본 종료 0·truncation 없음·검증된 Job cleanup 잔여0을 확인했다. Root 원 session `69647`은 257,798ms·peak 571,052,032 bytes/HARD4GiB로 종료했다. 원 case 파일 651개와 source/SDK/controller/tool pin을 전후 대조했다.

같은 source의 고정 kit binary로 assemble5를 순차 실행했다. 조립 원 session `30179`의 Root Job은 58,199ms·peak 296,075,264 bytes/HARD4GiB·exit0·cleanup0이며, Go5도 모두 정상 종료했다. 새 graph 파일 수는 profile 순서대로 291/295/296/296/322, 합계1,500개다. 각 graph의 invocation 수는 64/64/65/65/74, owned member 수는 286/290/291/291/317, node 수는 67/67/68/68/77이다.

각 graph는 fresh source9eba의 build9/control53 및 static donor의 178 owned members와 10 tools를 보존한다. 이를 새 reference 실행 310회로 세지 않는다. Current cohort만 해당 profile의 native1+referenceQ에 연결한다. 새 donor registration SHA는 `05008567cd8085fa81ecafcaeedd222befe054238ac104b13e9d3dc9b634af57`이다. 독립 graph 검토 후 Root가 graph 밖에서 profile별 approval5와 별도 승인 결정·manifest를 발행했다. 과거 C#/CJR/CPP의 current 승인 값을 복사하지 않았다.

Build/control의 공통 run 필드가 없는 원본에는 실제 fresh session ID를 approved-local cohort 식별자로 선택하고 그 결정을 명시했다. 기존 node의 run 값은 바꾸지 않았다. Provenance는 실제 source·supervisor·선택 controller/context·새 graph·새 review 결정 및 해당 cohort의 정확한 invocation ID 집합을 결속한다. 이는 caller의 로컬 신뢰 결정이며 서명이나 외부 저자 인증이 아니다.

## Guarded 검증과 정의 projection

실행 원 session `17008`에서 verify5를 모두 통과한 뒤 definitions5를 수행했다. Root Job의 실제 wall은 137,557ms, peak 287,416,320 bytes/HARD4GiB, exit0·Job cleanup 확인·잔여0이다. Go10도 모두 exit0/HARD4GiB/cleanup0이며 실제 wall은 695~2,555ms였다. Root900초/transport930초, inner840초, Go60초·test45초·transport75초 한도를 지켰다.

각 verify는 required=verified=Q·complete PASS, full CST·query metadata·request/frame·compiler/library·stderr owner 결속을 확인하고 별도 공개 eligibility API와 core의 상태·counts·digest parity를 대조했다. Definitions는 이 검증을 다시 수행한 live guarded session 안에서 projection을 만들었다. 저장된 classification PASS만 신뢰해 정의를 생성한 것이 아니다. 새 native/reference 관측도 만들지 않았다.

기존 assembly 파일 1,500개를 바꾸지 않고 caller classification 결과5개와 case/assembly 밖 별도 물리적 출력 directory의 projection5개만 추가했다. 원 source716·case651·donor183과 승인5를 보존했다. 외부 read lease 관측3,123개는 모두 사후 pin이 일치하고 해제됐으며 Root final command의 별도 self lease도 해제했다. 입력 manifest는 고유 경로3,112개다. 중복 lease7개와 manifest 밖 추가 고유 경로4개를 포함한 관측 수와 고유 파일 수를 구분한다.

| Profile | Definition result SHA-256 |
|---|---|
| JSON1 | `fcedbab4c6e2c69de54f3e0780de6ca346da1d5a7391a09da4c56a52c852a01a` |
| YAML1 | `fff92111106dc8dc6e573119476ea95671c13cf7ddbfdc845b1bd5772431aee9` |
| XML2 | `bba79704af686ffe99bcef95f5f2801128e1b812e2d5159b77ac7750c5a87c20` |
| HTML2 | `ca2670c91364e5773bdecfa0de627173af6cf6e968b4d81ad8d4781100a18354` |
| CSS11 | `6efb132fd5d00bc48f243e389de191820329715fcf38b8268305a89a33c57465` |

두 실제 결과 사후 검토는 B0/M0/신규 Minor0이었다. Correctness는 원본 요청·receipt와 메타데이터·해시를 읽기 전용으로 대조했다. Security는 결과·승인·source·guard·receipt·입력 보존을 대조하고 고정 qualification/query 원본으로 좌표 수와 digest를 다시 계산했다. 검토자가 Go나 parity를 새로 실행한 것은 아니다. 마지막 tool exit/self lease marker는 Root가 직접 회수한 원본이며, 별도 리뷰어는 자기 tool scope에서 같은 session을 재회수하지 못했다.

| 보존 근거 | SHA-256 |
|---|---|
| C12 수집 채택 | `a3bf3036b88501c378520579d67c7cf21afb18853308a321d1df1a9172bdebd0` |
| 새 조립 채택 | `a05d63fa1c8cfbf159e56d1b333fa766c356e74a7d50ef40db33456559566ad1` |
| 새 graph 승인 결정 | `7cb0ff031a560e8e11bbbeb4174d8414305e0691bb6c7d369f3d17325b8d9447` |
| 승인 manifest | `7cadcfd2c7fb1f6d2b58ed749e79c1c9c65f14ec596fe3ce2a5230c95d17d092` |
| 실제 guarded 결과 채택 | `30de74f9d439be94da3629f7cf2224ad1ad1706b7e774f9c3ea7ea738de496b7` |
| 실제 결과 correctness 검토 | `c6764be1efb6d22618ec753a7dbfdea45139e7ba4715e60a4ca97dbe06006639` |
| 실제 결과 security 검토 | `c84c83139a944ff5db9eb97fbe15d4e36a6774b8ebc21d4c71b55db719f86448` |
| 현재 source 정적 회귀·vet 채택 | `4835c260716fffec659ccdbb21597893a2f1eb9959909548361156ac0f5ee1ea` |

## 검사와 보존 한계

현재59용 native/kit compiled test binary의 정적 회귀는 최상위23 PASS, Windows case-insensitive filesystem에서 만들 수 없는 case-only-link 하위 반례1 SKIP이다. 일반 디렉터리 link는 실제로 만들고 symlink/irregular·target identity와 거부를 확인했다. Manual capture entry의 기본 SKIP을 실제 수집 성공으로 세지 않았다. `CGO_ENABLED=0 GOEXPERIMENT=jsonv2`의 `go vet -p=1 ./src/...`도 통과했고 source 및 선택 SDK/module12604개의 전후 pin이 일치했다. 검사의 세 Go Job은 HARD4GiB와 검증된 cleanup0으로 종료했다. `-p=1`은 vet의 package 범위를 줄이지 않는 빌드 병렬도 제한이다.

최초 assembly 시도는 PowerShell의 같은 함수 안에서 byte 임시 변수와 경로 변수의 대소문자 이름이 겹쳐 Go dispatch 전에 실패했다. 원본 Root exit1·cleanup0, 입력1,577개 pin/해제와 실패 파일을 보존했고, 별도 이름·새 출력 namespace의 교정본으로 미실행 assemble5만 수행했다. 검토 중 발견한 supervisor900초 상한·실제 dispatch 직전 예약·review raw와 decision 결속·Windows 경로 구분자 문제는 실제 guarded 호출 전에 새 버전에서 고쳤다. 실패나 미실행 초안을 성공으로 바꾸지 않았다.

Transport의 `children_cleanup_verified:false`는 그대로 두며 실제 자식 정리 판단은 원본 Job receipt의 verified/residual0에 한정한다. Source environment의 특정 음성 반례 미실행, legacy/external 진단 표기의 범위, 과거 manifest 설명 fixed15 대 실제16, 승인 metadata 생성기의 일반 타입 검사 및 중복 JSON 일반 거부 미구현은 기존 수용 범위로 남는다. 고정된 이번 보고서의 실제 정수 B0/M0와 대상 pin은 따로 검사했다. 마지막 실행 결정은 동일 read lease에서 두 리뷰의 역할·source·대상5개와 정수 B0/M0를 확인한 canonical Root 발행 경로에 한정한다.

기존 toolchain·Windows·선택 경로가 변하지 않는 단일 Root의 cooperative host를 전제로 한다. 모든 host/SDK 파일, vendor/publisher 인증, 실행 진위 서명, 적대적 owner의 경로 교환 방어를 주장하지 않는다. 미회수 6,263-byte 옛 transient 초안도 복구했다고 주장하지 않는다.

원본 graph·approval·결과·receipt는 미추적 로컬 보존 증거다. 이 추적 보고서나 hosted CI만으로 원본을 직접 대조하거나 당시 관측을 독립 재구성·재승인할 수 없다. 기존 [CPP4까지의 역사적35/130](issue-216-cpp4-classification.md)에 이번17개를 중복 없이 더한 조건부 분류 누계는52/130이다. 서로 다른 source/reader/profile 실행을 합친 역사적 수치이며 한 source에서52개를 실행한 것은 아니다. 나머지78개와 전체 ordinary 사례·semantic 평가, Linux/macOS 정의 분류 및 #216/#195 완료는 주장하지 않는다. PR 필수 CI·공식 artifact와 실제 main 검증은 별도의 통합 gate다.
