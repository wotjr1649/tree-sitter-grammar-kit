# #216 C·Java·Rust13 정의 연결과 Windows 분류 관측

Windows/amd64에서 C4·Java5·Rust4의 고정 빈 입력을 실제 native/reference로 수집하고, 외부 caller approval 뒤 같은 guarded snapshot의 core·공개 API 검증과 정의 projection을 통과했다. 결과는 13개 정의의 조건부 `STRUCTURAL_ONLY`이며 `semantic_evaluated:false`다. 정적 원장 r1/r10의 `STRUCTURAL_CANDIDATE`, `compiled_metadata:UNVERIFIED`, `semantic_required:null`·`NOT_ASSESSED`와 전체 #216/#195의 OPEN 상태는 유지한다.

## 원문 연결과 관측 범위

[계측 reference 계약](../specs/instrumented-query-reference.md)의 닫힌 `c4-empty`, `java5-empty`, `rust4-empty` profile을 사용했다. 각 anchor의 query ID·순서·정확한 UTF-8 원문/hash는 해당 [C](../../src/testdata/native/queries/c.json)·[Java](../../src/testdata/native/queries/java.json)·[Rust](../../src/testdata/native/queries/rust.json) 원문, [정적 원장](../../src/contracts/query-slot-inventory-r1.json), [qualification](../../src/contracts/qualification-c1.json)과 연결된다.

| Profile | 정의 | 원래 cases | Edits | Case steps | Tree roles | Potential links | 빈 입력 root | ABI language/runtime/min; symbols/fields |
|---|---:|---:|---:|---:|---:|---:|---|---|
| C4 | 4 | 58 | 18 | 76 | 94 | 376 | `translation_unit` | 15/15/13; 403/40 |
| Java5 | 5 | 40 | 32 | 72 | 104 | 520 | `program` | 15/15/13; 323/40 |
| Rust4 | 4 | 33 | 14 | 47 | 61 | 244 | `source_file` | 15/15/13; 360/31 |
| 합계 | 13 | 131 | 64 | 195 | 259 | 1,140 | — | — |

원래 사례 분모와 potential links는 정의의 정적 연결 범위다. 이번 실제 입력은 빈 UTF-8 source 하나씩이며 편집 없음·API=false·single frame이다. Source SHA-256은 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`다. 세 native와 13개 reference 관측에서 독립 기대 CST의 named root 하나, step0 `NO_ERROR`, 빈 match/capture를 대조했다. 관측된 query metadata는 합계37 patterns·36 capture-name 항목이며 모든 predicate 목록은 명시적 빈 배열이다. 원래131개 사례를 실행하거나 의미 평가한 것으로 세지 않는다.

## 고정 실행 identity와 외부 승인

Capture·조립·정의 검증의 source와 reader revision은 `4eff1259da85c0e2fb8b88d7abea5d49441e7ddf`, tree는 `7ebbb9800c5fa031c089cd5815633959ed2daef1`이다. 이후 보고서 commit이나 merge revision을 당시 실행 source로 소급하지 않는다. 고정 source manifest는704개 파일이며 SHA-256은 `bac1635c923fec5f45937b8f036e2d5bf900b5ed7a5083c8458cb4fd8ab1aa57`이다. Runtime manifest는 `1b962a519e6aece2140eb1c56589cb5ca88b501612921a0ce8dd4acd1a63d541`이다.

| Profile | Preregistration SHA-256 | Run ID |
|---|---|---|
| C4 | `bc2a343b2f8cfdc1bb5ebc97a093a3f495d78e078d022d9553e5e7772c8f720c` | `cjr13-c4-empty-20261030-4eff1259-3` |
| Java5 | `a3af04832fdf026ee6f5320b8ffe2389c3f929717c3e0047e6f780743cd4660c` | `cjr13-java5-empty-20261030-4eff1259-3` |
| Rust4 | `381f42b9f6274ccb1f510229bb294fe805d09a9eb22ff879224d07d5fe463e36` | `cjr13-rust4-empty-20261030-4eff1259-3` |

Capture는 순차 compiler3·native3·reference13 호출이었다. Reference build/control은 기존 Windows donor의 build9·unit4·control53·assertion36 closure를 보존해 재사용했다. 그 실행 source `cecec482eb4219e449f1150b38b1f10d6c7bf0b5`와 이번 current workload source를 구별한다. 과거 build/control을 다시 실행한 것으로 세지 않는다. Reference executable·library와 source/vendor snapshot, compiler 출력·grammar source의 역할·경로·bytes/hash, 원 request/process Spec/raw frame과 full-input context를 새 graph의 독립 검토에 연결했다.

Producer 밖의 Root가 독립 graph/security/provenance 검토와 실제 종료 증거를 확인하고 graph 밖에 별도 caller approval을 만들었다. 이전 C# approval을 새 CJR graph의 승인으로 재사용하지 않았다. 새 decision SHA-256은 `59e3aa9dba3eeebba8cc39e55942be4963bb7501658c7c08f544b873c12acf87`이다.

| Profile | Evidence graph SHA-256 | 외부 approval SHA-256 | Definition result SHA-256 |
|---|---|---|---|
| C4 | `dd0b1e25a94305bfe43f808e3ab7a9de99b58705149dc2f790628bc14202dda1` | `087b9b3e9cfc159fd917959c8d099643d151c5b58ef1a694db9d4b4d340a8b86` | `f80ff11a0d5225089a04dc04ec0e5959e0bdd01a93b71834bdd9f6a3ad5dd6f6` |
| Java5 | `73c3146a7b5836b5c744d8baf092e50813bef9fabe78de506af58ef510197f9f` | `53f43c2b0f81987f6eec468b5f9daf43aef0588f4127520cb3f707c1a855f300` | `89871205e485a12fad12380e0be13434e6463df168502019791623da2890f53d` |
| Rust4 | `38adc1e2a56e709898dfeda169f2270d01fc810821e7a88d0e799ba08e81e9ba` | `86924efc09f17b6351c751c182f9d4f0ed405c4cf972b915fb5dcb7a6acec7ee` | `0fb86b248ad0bafd21f1f356393a71acae2838bf023fc372f6c0ceccda4c1c64` |

## Guarded 검증과 실행 경계

`TestQueryClassificationCJRLocal`의 `definitions` stage를 기존 고정 kit test binary로 각 profile에 한 번씩 호출했다. 각 호출은 같은 retained core snapshot에서 eligibility·독립 CST·compiler/library·native/reference metadata·request/frame 결속을 검사한 뒤 공개 eligibility API와 counts·완료 상태·관련 digest parity를 확인하고 projection을 쓴다. 출력은 retained case/caller/graph/approval 밖의 새 directory에만 생성했다. 이 단계에서 새 Go build·compiler·native·reference 호출과 승인 발급은 없었다.

정의 검증의 부모 Python Job은 HARD4GiB에서11.426초·peak258,105,344bytes였고 세 Go Job의 실행 시간은546/440/483ms였다. 모두 정상 exit0·검증된 JOB_OBJECT cleanup·잔류0이었다. 부모750초·각 Go180초·Go test120초의 한도를 지켰다. Root 도구의 원본 세션도 정상 종료했고, 독립 동일 host/boot QPC로 확인한 전체 호출 상한은20.096초로780초 관측 한도 안이었다. 입력 lease2,408개와 image lease6개는 정상 반환 뒤 모두 해제됐다.

원본 capture와 조립의1,493개 입력 파일·99,655,728bytes, source704개 및 이전 실패 증거의 pin을 검증 전후 보존했다. 최종 정의 기록은13,104bytes/SHA-256 `5818fa75cd0b7a8dacaf5e761d6c53ed6197c13743cf1e866d7123878588c180`이다. RootReturn과 원본 도구 seal·receipt·stdout/stderr·lease release를 별도로 대조했다. 이 근거는 caller가 신뢰하는 저장 bytes와 cooperative local host라는 조건 아래의 무결성 결속이다. Hash나 receipt를 서명·실행 진위 인증으로 간주하지 않는다.

첫 definitions controller는 root-return pin의 서로 다른 key set을 전체 비교해 Go/Context를 시작하기 전에 실패했다. 그 원본 exit1·FAILED_PRESTART·cleanup/lease 기록과 claim을 보존했다. 두 pin의 각 형태와 공통 bytes/hash 및 고정 물리 경로를 대조하도록 새 controller를 만들고, 실제 입력만 사용하는 사전검사와 독립 검토 후 새 namespace에서 한 번 실행했다. 원본 controller를 다시 호출하거나 실패를 성공으로 덮어쓰지 않았다.

## 검사와 남은 의무

고정 source의 focused `go test ./src/internal/native ./src/kit -run ^TestQueryClassification -count=1 -p=1 -parallel=1 -v -timeout=180s`는 최상위31 PASS·6 수동 helper SKIP였다. CJR의 profile·anchor·grammar·stderr·compiler closure·원장 집계·정의 binding 반례를 포함한다. 이 합성/정적 회귀와 수동 helper SKIP는 실제 native/reference 관측을 대체하지 않는다. 이번 실제 세 정의 검증 PASS는 위의 별도 수집·조립·외부 승인에서 얻었다.

PR의 필수 자동 CI·공식 artifact 및 실제 main 검증은 별도 통합 gate다. 이 보고서는 당시 Windows 로컬 관측을 기록하며 다른 source·host의 완료를 선언하지 않는다. Raw graph·approval·controller·receipt는 미추적 보존 대상이므로 CI와 이 보고서만으로 당시 관측을 독립 재현하거나 재승인할 수 없다. 상세 검사·통합 기준은 [validation](../validation/validation.md)을 따른다.

새 Linux reference나 신규 macOS 의미 검증을 수행하지 않았다. C++의 큰 parser는 이 full-input 경로에서 제외되며 별도 paging과 사전등록·검토가 필요하다. 전체26 routes/3,201 ordinary cases/130 definitions/1,852 edits/5,053 case steps/6,905 tree roles/53,104 potential coordinates 및 SVC69·large4, inventory/fan-in과 후속 소비 검증은 남아 있다. Potential coordinates를 semantic 필수 분모나 `EVALUATED/PASS`로 바꾸지 않는다.
