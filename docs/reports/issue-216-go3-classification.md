# #216 Go3 정의 연결과 Windows 분류 관측

Go3의 OWNED query 세 정의는 ordinary `go/s06-go` 등록의 정의와 ID·정확한 UTF-8 원문·SHA-256이 같다. 이 연결은 기존 정의에 당시의 metadata 관측을 대응시키는 범위다. 정적 원장 r1의 `STRUCTURAL_CANDIDATE`, `compiled_metadata:UNVERIFIED`, `semantic_required:null`·`NOT_ASSESSED`는 유지한다. 전체 #216/#195는 OPEN이다.

## 정의와 관측 범위

[Go 원문](../../src/testdata/native/queries/go.json), [정적 원장](../../src/contracts/query-slot-inventory-r1.json), [독립 Go3 anchor](../../src/testdata/query-semantics/go3-classification-anchors.json)를 연결한다. 세 파일의 SHA-256은 각각 `9ba4b6c5ca375214844811aeb06a6fedda92163189cf2b8d1bd42801dd94a192`, `f24b4f0b4aad3fe165afd7950d2166fdc7e1d6a59d809faf670d5907c4f12789`, `096785af5cbd09ece1f649ab7ea41648f11b45c751386636238537787bec0f19`다.

| Query | 원문 SHA-256 | Pattern 수 | 첫 등장 capture 이름 | 명시적 predicate |
|---|---|---:|---|---|
| `q.structure` | `ce0ace9af2164936693efb63dcc12574cf64eda21fe78a4d9ad1ce7950df15eb` | 4 | `type, method, function, call` | 빈 배열 |
| `q.probe-v18` | `d0bde1d011f6faff3d6ffeff5990639afe92e5f05492b79e60c93466a52a9416` | 2 | `tparam, alias` | 빈 배열 |
| `q.probe-v27` | `30695fe32e69f8e31a1d1051b64251a747d2380e151e9fe1af598b6163d6c1d9` | 3 | `method, tparam, key` | 빈 배열 |

관측 입력은 빈 UTF-8 source이며 SHA-256은 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`다. 편집은 없고 query당 initial incremental tuple 하나다. Native와 reference의 전체 단일 `source_file` root 및 anchor의 ABI15/15/13/222/35·CST 필드를 대조했다. Match와 captures는 비어 있고 predicate 평가를 수행하지 않았다. 세 정의의 결과는 `STRUCTURAL_ONLY`, `semantic_evaluated:false`다.

Ordinary Go의 57 cases·37 edits·131 tree roles·393 potential slots는 이 빈 입력 pilot의 분모가 아니다. `go3`는 그 57개 ordinary case에 포함되지 않는다. 같은 정의라는 이유로 ordinary 사례의 실행·편집 결과나 semantic tuple을 채우지 않는다.

## 당시 실행 context

Windows/amd64의 run `go3-local-442d4e5337e84cd3addf34aeaecc06b2`에서 library1/native1/reference3를 실제 수집했다. Capture·조립·최종 검증 source는 `4f7c67a78ffa8bf52092773d0a8585127cb00684`, tree는 `e585e429737ae1c95c45aa7edb8e8068862d48d1`이다. 이후 merge revision을 원래 실행 source로 소급하지 않는다. 사전 plan digest는 `b56c690c666da5f1bc6cd086a621207047d2113e0b4d2826fefc359a0b53b574`, runtime manifest pin은 `1b962a519e6aece2140eb1c56589cb5ca88b501612921a0ce8dd4acd1a63d541`이다.

당시 preregistration의 grammar closure는 ordinary Go 등록과 경로·역할·크기·hash가 같았다.

| Grammar member | Bytes | SHA-256 |
|---|---:|---|
| `src/parser.c` | 1695280 | `ec699b64a1133af5747071fc5a7eb5d7024ab7403d3f6b767479f589dbaf5436` |
| `src/scanner.c` | 4041 | `808af6eb05206fd5ebd26c40fc7990f08e3cb34c9667c1693520e9809a949159` |
| `src/tree_sitter/alloc.h` | 985 | `b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea` |
| `src/tree_sitter/array.h` | 13453 | `20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2` |
| `src/tree_sitter/parser.h` | 7624 | `180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2` |

| 실행 identity | SHA-256 |
|---|---|
| C compiler | `75e87953e9d04e8a5d81225397292e9f8ae599a0ed85f70ed175767577aaa9f6` |
| 새 Go library, 262842 bytes | `3201f1e829da5b66512d3d5b2544d8615d34486acdb7b9efca673efa16e4b6d6` |
| 새 native driver | `8ba3d3437f5e3ea4d910a8519343485a844e83f833884f3614ee325692b9b394` |
| 재사용 reference executable | `3a3200a42189f4bec8892119d5a9e9921d1262acd74f8b757b9ca794adbeae88` |
| Reference build identity | `d8b0f2aa04edfda2efaa7f7147110b753cf63eca621f6363d3e799d6a4bd906f` |
| Reference source snapshot | `d67cb1ac3ad581abf4ed19d219ae4ad4194c3dcebc44ffce099a81b21ed72396` |
| Reference vendor snapshot | `8fec9dcc2dc84590867a7add1b33ad36be98c0a1b58634f228a5ce3a2632171c` |

Reference build/control은 기존 Windows donor closure를 보존해 재사용했다. 그 build 실행 source `cecec482eb4219e449f1150b38b1f10d6c7bf0b5`와 새 Go3 workload source를 구별하며, 과거 control을 현재 Go workload 실행으로 세지 않는다. 기존 C# 및 JSON provenance 검토의 원문·고정 source도 새 graph의 독립 검토에 연결했다. 새 library/native/reference request·process Spec·raw frame·사전 plan 및 전체 11-role identity는 별도 current cohort와 graph에서 확인했다.

## 외부 승인과 저장 결과

Graph는 69 nodes·292 physical members이며 registration에는 66 invocations와 역할별 tool 등록10개가 있다. Tool 등록 수는 고유 실행 파일 수나 이번 run의 새 도구 실행 수가 아니다. 기존 CLI 무결성 결과는 PASS/findings0이다. Producer 밖의 parent가 독립 correctness와 security/provenance 판정·실제 완료 기록·역사 원문을 확인해 별도 조건부 caller approval을 발급한 뒤 guarded core와 공개 eligibility API를 검증했다.

| 결속 대상 | SHA-256 |
|---|---|
| Evidence graph | `3727a7f7906f2e7e48feecccb02108d87b6aff2d594396ee8a918feaa621e567` |
| Registration | `8b65f3bd22a41be5cccc8fdf6006584d9300b45827574aa8354968c2a54406d3` |
| Policy | `66858f2b3925e52c22f940d11af0d68e5b9db150d75e781541f40eade9047749` |
| Witness | `2dc13e46997b259689cc8ef67cdb16cd97638f5b3b4d743b3a3f7799213e42e9` |
| Caller review record | `cc6d2ba554fb7d3caa52052bfda9d89146b95c895d2c948d5cb384d613a214d2` |
| Caller approval | `8bbff6fc5c6b95ebb5bf447f5fd7ec4aaed11921c1f8141f20ea76bcfc618ef7` |
| Classification result | `ec49684608fbb2d92cfea7d1fc78d4c0cf70673b76b25574a0946dc0e17f1e9f` |
| Local acceptance | `158e055a9970f4fd83ba9c39d62ae148b13d56180bd9275d0e223270f34ed153` |

저장 closure의 build9/unit4/control53/assertion36/tuple3은 모두 required와 verified가 같고 failed·blocked·unprocessed는0이었다. 두 API의 counts·digest parity와 독립 full CST를 확인했다. 이 숫자는 현재 run에서 역사적 build/control을 다시 실행했다는 뜻이 아니다.

실제 최종 호출 전에 외부 host가 절대 도구·controller·manifest·승인 hash를 확인하고 새 `pwsh -NoProfile -File`로 호출했다. 외부 Windows Job은 Python 초기화부터 모든 자손을 HARD4GiB/420초로 보호했고14.653초·peak173142016bytes였다. 내부 kit Job은 HARD4GiB/360초·2.424초·peak105385984bytes이며 API당120초였다. 둘 다 COMPLETED/exit0·cleanup verified·잔류0이었다. Caller 소유의 안정된 경로·도구 설치와 기록된 provenance 신뢰 조건을 전제로 한다. Hash와 receipt는 무결성 결속이며 실행 진위·서명 인증은 아니다.

기존 실패 controller·부분 효과·원본도 보존했다. 제품 graph PASS 뒤 collector가 nodes67을 기대한 실패, 정상 nested Job 뒤 slash 문자열 비교 실패, 승인 발급 전 보조 text assertion 실패를 실제 workload 결과와 구별했다. 이를 이유로 성공한 수집·검증을 반복하지 않았다. 비차단 N5는 controls 설명 두 곳의 이전 K 이름이며 실제 L source/request/pin과 구별한 처분을 보존했다.

## CI와 남은 의무

`TestQuerySlotGoDefinitionLinks`는 추적 파일의 정의 연결만 검사한다. Route·ID 변경, 누락·중복, 한쪽 원문+hash 교체 및 hash 모순을 거부한다. 원본 anchor의 full CST/ABI/metadata 검사는 기존 수동 helper와 해당 guard의 범위다. Raw graph·승인·controller·완료 기록은 정책상 미추적이므로 CI와 이 보고서만으로 당시 관측을 독립 재현하거나 재승인할 수 없다.

수동 helper·anchor subset은 [PR301](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/301)의 후보 attempt2에서 필수11 SUCCESS·예정된 진단2 SKIP을 확인한 뒤 정상 병합했다. 실제 merge `c61601710d48e24c303b6c7a28fb738442298ab8`의 [Foundation40686181078](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/40686181078), attempt1/push는14 SUCCESS이며 공식 ZIP·동일 cohort·merge 부모/tree 결속을 확인했다. 이 통합 검증을 새 query runtime 관측으로 세지 않는다. 원래 Windows package timeout과 승인된 전체1회 재실행 이력은 보존하며 수동 PR attempt2는 #280 strict 자연 표본에서 제외한다. Margin은n1/INSUFFICIENT_SAMPLE이다.

Query 원문이 같아도 grammar/runtime/library/reference·도구·source snapshot이 바뀌거나 확인되지 않으면 이 보고서는 당시 Windows 관측으로만 남는다. 새 환경의 현재 검증에는 해당 context의 실제 근거와 graph 밖의 별도 caller 승인이 필요하다. 기존 Linux archive는 current executable/library/input/workload/승인을 채우지 못해 현재 closure로 승격하지 않는다. 새 Linux reference와 신규 macOS 의미 검증을 수행하지 않았다.

전체26 routes/3201 ordinary cases/130 definitions, SVC69, large4 및 inventory/fan-in·후속 소비 의무는 남아 있다. 정본은 [계측 reference 계약](../specs/instrumented-query-reference.md)과 [validation](../validation/validation.md)이며 잠재 슬롯을 semantic 필수 분모나 EVALUATED/PASS로 바꾸지 않는다.
