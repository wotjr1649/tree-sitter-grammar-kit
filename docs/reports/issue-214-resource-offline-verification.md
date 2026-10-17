# #214 resource control의 offline 검증 범위와 개선 제안

이 문서는 #214/PR #225에서 확인한 과거 r1의 한계와 #226의 bounded resource capture/replay 구현 범위를 기록한다. 새 구현은 과거 결과를 승격하지 않는다. 형식의 owning 계약은 [tree/protocol](../specs/tree-and-adapter-protocol.md), CI 집계·artifact 계약은 [validation](../validation/validation.md#ci-and-evidence)이다.

## 기존 r1에서도 offline으로 검사한 것

[gate](../../src/dev/ci-gate/large.go)는 run/attempt/SHA, 고정 fixture·source·registry와 executable identity, 정상 build와 fault define, 정상 page의 연속 coverage·field inventory·검사 수, fault 9개·resource control 1개를 검사한다. resource는 정상 executable과 같은 SHA, 정확한 빈 audit receipt 20개 key, FAILED/exit 3, truncation 없음, wall·memory·출력 예산과 cleanup 기록을 요구한다. 원본 세 host의 qualification도 raw record로 재집계한다. 따라서 “offline으로 검증 가능한 것이 없다”는 뜻은 아니다.

하지만 resource의 `Error=RESOURCE_LIMIT:ALLOCATION_LIMIT`가 native 원본 응답과 맞는지는 과거 r1 artifact만으로 다시 확인할 수 없다. 과거 [live caller](../../src/dev/large-api/main.go)는 stdout을 frame으로 분리하고 typed 응답을 검사한 뒤 error 요약만 남겼다. 새 r2에서는 별도 capture를 보존한다. [runner.Result](../../src/internal/runner/runner.go)의 Stdout/Stderr는 `json:"-"`다. r1 audit receipt는 비어 있고 response는 별도 저장되지 않는다.

실제 main `df19b907466bdc8da0e5f42dd2b6c5ef3aff68fa`, Foundation run `38844446782` attempt 1의 artifact를 확인하면 resource process는 stdout_bytes 280, FAILED, exit 3을 기록하지만 원본 280 bytes는 없다. 길이·요약·hash만으로 그 내용을 복원할 수 없다. native를 새로 실행해 얻은 frame도 과거 stdout이 아니다.

## #226에서 추가한 offline 검사

새 outer `tsgk-large-api-validation/r2`와 controls-only `tsgk-large-api-controls/r2`는 resource 전용 원본 framed stdout을 보존한다. [공용 helper](../../src/internal/largeaudit/resource.go)를 live caller와 offline gate가 함께 사용한다. 빈 `largeaudit.Receipt`의 의미를 바꾸거나 모든 성공 page의 stdout을 중복 저장할 필요는 없다.

1. producer가 실제 runner stdout 전체를 bounded `[]byte`로 보존한다. JSON의 base64 표현과 실제 length/SHA-256을 쓰고, 잘라 저장한 값은 유효 처리하지 않는다. request의 실제 framed stdin SHA/bytes, 재구성 가능한 request policy와 source SHA를 함께 결속한다.
2. 현재 [C driver](../../src/drivers/native-c/driver.c)의 `fatal_limit`는 static buffer 1,024 bytes에 응답을 만들고 `0 < n < sizeof buf`일 때만 쓴다. 따라서 이 경로의 stdout 상한은 header 4 + payload 1,023 = **1,027 bytes**다. base64는 최대 1,372문자다. 예비로 64 KiB를 허용할 필요가 없다. writer가 바뀌면 이 근거와 상한·schema·대조를 같이 갱신해야 한다. capture object는 JSON 2,048 bytes 상한을 검사하고 base64 raw JSON 문자열은 quotes 포함 1,374 bytes 이하로 제한한 뒤 decoded frame 1,027 bytes 상한도 적용한다. 전체 artifact는 기존 16 MiB 입력 상한을 유지한다. JSON object 밖 whitespace는 외부 decoder의 입력 한도에 속한다.
3. verifier가 고정 small-0 source와 기존 request를 재구성하고 `Limits.MemoryBytes=1`을 적용한다. request frame의 SHA/bytes, 실제 output의 SHA/bytes와 process.stdout_bytes, run/attempt/SHA·executable identity를 대조한다.
4. 기존 [SplitFrame·DecodeResponse·Check](../../src/internal/native/protocol.go)를 그대로 사용해 framing, protocol/ID, producer capability, source byte 수, complete, RESOURCE_LIMIT/ALLOCATION_LIMIT, steps_completed 0, 빈 steps, exit 3을 재검사한다. SplitFrame은 잘림과 뒤의 bytes/다중 frame을 거부하고 DecodeResponse는 unknown/duplicate member·잘못된 UTF-8·trailing JSON을 거부한다. frame이 없는 형식이나 r1 fallback으로 현재 후보를 통과시키지 않는다.
5. frame 누락·잘림·다중 frame·잘못된 length/SHA·다른 ID/source 길이/protocol/status/code/exit·성공 응답으로 교환·steps 누락/위조·요청 budget 변경·overflow/상한 초과를 음성 대조로 확인한다. 새로운 live resource control에서 실제 frame을 받은 뒤 저장→offline 재검증을 연결한다. 기존 fault 9개, resource 1개와 native/OS 예산을 유지한다.

allocation failure는 steps가 없어서 응답에 source SHA가 없다. Check는 source byte 수와 ID 등을 확인하지만, 응답만으로 원문 SHA나 실제 stdin 전달을 증명하지 못한다. 따라서 외부 observation에 request/source identity를 결속하고 “보존된 request와 response의 일관성 재검증”으로 표현해야 한다. hash는 producer의 정직성을 보증하는 서명이 아니다.

## 원본 frame이 있어도 남는 경계

| 항목 | offline 검사 가능 범위 | 추가로 필요한 근거 |
|---|---|---|
| native allocation budget | 보존된 frame이 재구성 request에 대한 typed failure와 일치하는지 | 해당 후보 live control의 실제 capture |
| OS hard memory 8 GiB | 저장된 limit·peak·enforcement·종료 기록의 일관성 | OS backend가 한도를 실제 집행하는 live adversarial test |
| 자식 process 정리 | 저장된 scope·residual·verified 기록과 종료 상태의 일관성 | 실제 descendant/escaped descendant·pipe holder 회수 시험 |
| 실제 process·입출력의 진실성 | run/SHA·입출력·executable·공식 artifact digest의 결속 | trusted producer와 CI 실행 경계; 완전히 악의적인 producer에 대한 독립 증명은 아님 |

이 control의 request memory는 1 byte이고 supervised process hard memory는 8 GiB다. **ALLOCATION_LIMIT 응답은 OS 8 GiB 제한의 작동 증명이 아니다.** 기존 runner의 실제 OS별 hard memory·process tree 시험은 계속 필요하다. signed attestation을 추가해도 서명한 producer 자체의 동작을 자동으로 참으로 만들지는 않는다.

## 검증과 남은 경계

#226은 margin과 분리한 변경 단위로 owning schema·live caller·offline gate·음성 대조를 함께 갱신한다. `TestResourceCaptureReplay`·`TestResourceResponseAndBounds`는 누락/null·변조·같은 base64 길이에서 decoded 1,028 bytes인 입력까지 거부하며, `TestNativeAuditSharedParser`는 고정 runtime과 owned grammar의 실제 resource frame을 저장→decode→offline 검사한다. 전체 등록 C# large control의 실제 결과는 해당 후보의 CI artifact로 별도 확인해야 한다. 기존 r1은 당시의 제한된 역사 근거로 남기고, 새 후보에서는 새 revision을 요구한다. 과거 누락 bytes를 추정하거나 현재 live 실행으로 덮어쓰지 않는다. OS 집행의 live 근거와 frame의 offline 재검증은 별도로 보고한다.
