# 변경 기록

## Unreleased

- csharp C2 patch를 등록했다(#76, `csharp-P2-01`~`11`, `csharp-P2-PP`). 채택 chain 뒤에 grammar.js와 scanner.c 단계가 온다.
  - 고친 결함: 문장 수준 `static` local function과 identifier `async`(lambda modifier prec), `allows ref struct` 제약, `var` pattern의 모양, U+0085·U+2028·U+2029 줄 끝과 Zs 공백, `$""`, `unsafe` finalizer, 메시지 없는 `#warning`·`#error`가 다음 줄을 삼키던 문제, label 여러 개인 switch section, namespace 본문의 `extern alias`, catch·finally 없는 try 수용, async 밖 identifier `await`.
  - 조건부 section(표준 6.5.5)을 symbol 없이 정할 수 있는 만큼만 고른다. 조건이 정확히 literal `false` 또는 `!true`이면 거짓이고, 그 밖의 조건(`true`, `DEBUG` 같은 identifier, `(false)`, `false || X` 등)은 모두 참으로 본다. `#if`·`#elif`·`#else` 묶음에서 literal false가 아닌 조건을 가진 첫 section(앞 section이 모두 literal false일 때의 `#else` 포함)만 code로 parse하고, 나머지 section은 각각 opaque token 하나인 새 node type `preproc_skipped_section`이 된다. skipped section 안의 중첩 `#if … #endif`는 section을 일찍 끝내지 않는다. 이전에는 `#if` section을 늘 code로 읽고 모든 `#elif`·`#else` section을 opaque로 두었다.
  - 이 규칙의 결과: skipped section 안의 잘못된 code(예: 받아들여진 `#if` 뒤 `#else` 안, `#if false` 본문)는 ERROR 없이 받아들여진다. skipped section의 선언은 tree에 나타나지 않는다. 정의되지 않은 symbol 조건(`#if DEBUG`)의 section도 code로 parse된다. literal false 조건의 `boolean_literal`은 익명 `false` 자식이 없는 leaf다. 새 사례 `probe-csharp-B01-P-1`·`P-2`·`P-3`·`PE-1`이 `#if false … #else class Y { }`, skipped `#else` 안의 중첩 `#if`와 code가 아닌 text, `#elif` chain, 조건 전환 edit을 고정한다. upstream corpus와 저장소 `.cs` 186개 입력의 ERROR 수는 1로 그대로다.
  - 사례 수정(사양 근거): 표준(csharpstandard draft-v8 patterns.md 11.2.1)은 "If the input can be syntactically recognised as both a constant_pattern and a positional_pattern then the constant_pattern shall be chosen"이라고 정하고 subpattern 수를 제한하지 않는다. `(0, 0)`(괄호 식)과 `Point(0, 0)`(invocation expression)이 모두 이 경우라서 grammar는 이 구문 규칙대로 `constant_pattern`으로 읽는다. Roslyn은 두 형식을 positional pattern으로 읽으므로 이 점에서 grammar와 Roslyn이 다르다. 그래서 `probe-csharp-V08-P-1`의 arm을 property subpattern이 붙어 positional pattern으로만 읽히는 `Point(0, 0) { }`로 바꾸고 `probe-csharp-V08-Q-1` capture를 맞췄다. `alt-csharp-B05-08`에서 finalizer(§15.13)에 붙어 있던 `constructor_declaration` anchor 2개를 뺐다.
  - Windows amd64 실행(baseline main 대비): route 105/121 → 125/125, oracle 113/133 → 134/137(새 사례 4건 포함). oracle의 나머지 3건은 API claim만의 차이다.
  - registry 시험은 csharp가 C2 기록을 가져도 adoption 전용 규칙을 계속 검사하도록, csharp를 adoption 기록으로 되돌린 상태에서 그 mutation을 적용한다. reproduction 시험의 "parser.c 고정만 있는 C2 채택 route" mutation은 고정을 직접 줄인다. 검사 규칙은 그대로다.
- javascript·jsx·typescript·tsx C2 patch를 등록했다(#76).
  - javascript·jsx(grammar.js, scanner.c): 이름 없는 `export default` 선언([+Default])의 모양, `-x ** 2`와 괄호 없는 `??`·`||`/`&&` 혼용 거부, tagged template의 NotEscapeSequence, import attribute가 붙은 re-export, `new a?.b()` 거부, if·loop 절의 선언 거부(Annex B의 if 절 plain FunctionDeclaration만 허용), non-generator code의 `yield` label·identifier. jsx는 `<Comp {props} />`도 거부한다. scanner가 generator 본문을 추적해 [Yield] 문맥을 구분한다.
  - typescript·tsx(`common/define-grammar.js`, npm `tree-sitter-javascript/grammar.js`): variance annotation `in`·`out`, `export type *`, generic tagged template, `static`·`override` 뒤 `accessor`, import attribute re-export, `using`·`await using` 선언(새 node type `using_declaration`, for-of head 포함), static index signature, 괄호 decorator(`@( Expression )`만 임의 식을 받고 그 밖의 decorator는 identifier·member·call 형태 그대로라 `@a + b class C {}`는 ERROR), `bigint` predefined type. tsx는 `.tsx`의 맨 `<T>(value: T) => …`를 거부한다.
  - strict code에서만 거부되는 `if (x) function f() {}`(`javascript-P2-08`)는 strict scanner를 쓰지 않고 javascript-S01(SEM)로 옮겼다(2026-10-05 사용자 결정). `probe-javascript-L01-R-2`를 지웠고, javascript-L01 N/R은 `probe-javascript-L01-R-1`이 덮는다.
  - 사례 수정: `alt-javascript-V15-07`과 `alt-jsx-B04-js-V15-07`의 contains에서 `class`를 뺐다(`export default class {}`는 ClassDeclaration[+Default]다).
  - generator 안에서도 arrow function의 concise body에서는 `yield`를 `yield_expression`으로 읽지 않는다. ConciseBody는 [~Yield]다(ECMA-262 15.3 `ExpressionBody[In, Await] : AssignmentExpression[?In, ~Yield, ?Await]`). scanner가 concise body에 non-generator frame을 열고, 식이 끝나는 곳(같은 줄의 `)`·`]`·`}`·`,`·`;`·`:`, 입력 끝, 자동 semicolon이 들어갈 줄바꿈)에서 닫는다. `function* g(){ const f = () => yield 1; }`는 이제 ERROR이고, `() => yield`(identifier)는 그대로 받는다. class field initializer는 generator의 문맥을 그대로 따른다(15.7 `FieldDefinition[Yield, Await] : ClassElementName[?Yield, ?Await] Initializer[+In, ?Yield, ?Await]opt`, 15.7.1 early error는 `arguments`와 SuperCall만 금지). 그래서 generator 안의 `class C { x = yield 1 }`은 `yield_expression`이다. 고정 node 24.21.0(V8)은 이를 거부하지만 사양 원문을 따른다. generator 안의 `(a = yield) => 1`(ArrowParameters early error)은 아직 받아들인다. 새 사례: `probe-javascript-V15-N-1`(arrow, ERROR), `probe-javascript-V22-P-1`(field initializer의 `yield_expression`), `probe-javascript-V15-PNE-1`(`function*` ↔ `function` 전환), jsx는 같은 내용의 `probe-jsx-B04-js-*`.
  - Windows amd64 실행: route는 javascript 53/53, jsx 53/53, typescript 72/72, tsx 68/68 PASS. oracle은 javascript 57/57, jsx 56/56, typescript 73/79, tsx 66/70 PASS이며 나머지는 API claim만의 차이다.
  - 받아들인 corpus ERROR 증가: javascript는 generator 밖 최상위의 `yield` 식(이제 [~Yield] 문맥에서 `yield`를 식으로 읽지 않는다), tsx는 `<A>(...) => 2`(JSX 여는 tag로 읽음).
- c·cpp C2 patch를 등록했다(#76, `c-P2-01`~`25`, `cpp-P2-01`~`45`). cpp는 npm `tree-sitter-c/grammar.js`에도 c 수정을 같은 방식으로 적용한다.
  - c: 초기화 목록 안 `#embed`, `_BitInt`, `_Bool`·`_Decimal*` keyword, `_Complex`·`_Imaginary`, `__has_include`·`__has_embed`·`__has_c_attribute` 피연산자, digraph, 이름 없는 bit-field, skipped group의 C가 아닌 text, `static_assert`, 짝 없는 `#elifdef`, `_Thread_local`, `_Atomic(T)`, `auto` 추론, 빈 attribute, C23 attribute 위치, storage class가 있는 compound literal, `constexpr`·`_Noreturn` 분류, null directive, 단독 `_Pragma`, balanced token attribute 인자, enum 기반 형식, block 끝 label, `typeof`·`typeof_unqual`, `_Generic` `default`, attribute가 붙은 case label. 새 node type: `atomic_type_specifier`, `bit_int_specifier`, `pragma_operator`, `preproc_skipped`, `static_assert_declaration`, `typeof_specifier`.
  - cpp: 빈 attribute와 pack expansion, deduction guide(file·namespace 범위의 전용 규칙으로 받아 call의 type template 인자를 되살림. class 범위의 guide `struct S { template<class T> struct I { I(T); }; I(int) -> I<long>; };`([temp.deduct.guide]/3)도 ERROR 없이 같은 `declaration` 모양으로 parse된다), `if consteval`, trigraph·digraph, operator·literal operator template-id, 한정 pseudo-destructor, `.*`·`->*`, `wchar_t`, `~decltype`, `typeid(type)`, class template 명시적 instantiation, init-capture `{}`, designated `{ .x{5} }`, `co_yield {}`·`co_return {}`, C++23 delimited·named escape, `__has_include`, skipped group text, `explicit (S)(…)`, attribute 위치, `new int[n][3]` 선언, `mutable`, `sizeof(int ())`, type-requirement, conversion-function-id member access, `typename T::type()`, new-expression 형식, `using typename`, parameter pack 선언자, 이름 없는 bit-field, decltype base, pure-specifier, 기본값 있는 constrained type parameter, `auto(x)`·`auto{x}`, null directive, `_Pragma`, `throw(Ts...)`, nested-requirement, `noexcept` 연산자, N3337 attribute 위치, C++20 이전 `import<int> f();`·`Y<operator<=> y;` 읽기. 새 node type: `nested_requirement`, `noexcept_expression`, `pragma_operator`, `preproc_skipped`.
  - Windows amd64 실행: route는 c 50/50, cpp 69/69 PASS. oracle은 c 51/54, cpp 73/73 PASS이며 c의 나머지 3건은 API claim만의 차이다.
  - 사례 수정(anchor occurrence와 모순된 contains): `alt-c-V23-01`, `alt-cpp-B01-13`, `alt-cpp-L01-01`의 anchor occurrence, cpp 사례 3개의 모순된 `contains`(`field_expression`, `field_declaration` 2개)와 `Ts *...` anchor occurrence.
- qualification inventory(`qualification-c1.json`)를 다시 생성했다(위 route의 patched file hash와 사례 수정).

- 26 route 어느 것이든 저장소 안 C2 patch를 실을 수 있게 했다(#76, `C2-PATCH-r1`: 모든 route를 FULL PASS까지 patch할 수 있다는 사용자 결정). 첫 patch는 yaml `yaml-P2-01`이다.
  - patch subject는 `src/dev/c2-patches/<route>.json`(`tsgk-c2-patch/r1`)이다. `language-sources.json`의 route에 `c2_patch` 기록(`decision`, `subjects`, chain 전체 뒤의 `patched_files` hash)을 둔다. `native-routes.json`의 chain은 채택 chain(채택 route만) 뒤에 C2 subject의 `/files/<i>` 단계가 순서대로 온다. native `patched_files`는 C2 기록의 것이다. 채택 6 route의 adoption 기록은 P05 identity 그대로다. 채택 route의 C2 기록은 adoption의 patched file을 모두 담고, C2 단계가 대상으로 삼지 않는 file은 adoption hash 그대로다. C2 patch가 있는 route는 채택 route라도 `reproduction-routes.json`이 출력 6개를 모두 고정한다.
  - foundation 시험이 다음을 거부한다: 기록 없는 C2 chain, chain 없는 기록, 다른 route의 subject, C2 단계의 pointer·target 불일치, native와 기록의 patched file hash 불일치, chain target이 아닌 patched file과 patched file 없는 target, upstream 그대로 쓰는 patched file, 출력 고정 누락·불일치(채택 route 포함), adoption subject가 아닌 채택 chain 단계, 채택 route의 C2 기록에서 빠진 adoption patched file, C2 단계가 대상으로 삼지 않는데 adoption hash와 다른 file, adoption patched file도 C2 대상도 아닌 C2 기록 file, adoption hash가 없는 file을 대상으로 하는 채택 chain 단계, C2 기록이 없는 채택 route에서 adoption 기록과 다른 native patched file, C2 기록의 decision·subject 경로·patched file 형식 오류. 각 규칙에 음성 대조 시험이 있다.
  - `prepare-routes.ps1`은 채택 단계까지의 결과를 `language-sources.json`의 adoption hash와 대조한 뒤 C2 단계를 적용하고, 결과를 native 등록부의 patched file hash와 대조한다. `node_modules/<package>/…` patch 대상의 원문은 등록 npm tarball에서 가져온다(cpp의 tree-sitter-c, typescript·tsx의 tree-sitter-javascript patch용). 등록되지 않은 package면 실패한다.
  - yaml scanner가 줄 첫 칸(column 0)의 BOM(U+FEFF)을 두 경우에만 받는다(YAML 1.2.2 [202] `l-document-prefix`, [211] `l-yaml-stream`). directive가 올 수 있는 document prefix(stream 시작, `...` 뒤)에서는 BOM을 건너뛰고 그 뒤를 평소처럼 읽는다. 그 밖에서는 BOM 바로 뒤, 사이에 아무것도 없이 column 0의 `---`가 올 때만 받고(열린 block collection은 먼저 닫는다), 그렇지 않으면 오류다. BOM은 들여쓰기로 세지 않는다. 따옴표 scalar 문맥과 오류 복구 호출(`ERR_REC`)에서는 BOM을 건너뛰지 않는다. BOM 줄이 있는 무효 입력은 계속 오류지만 일부는 오류 tree 모양이 바뀌었다(예: corpus `Invalid use of BOM`). scanner.c는 생성기 입력이 아니므로 출력 6개는 바뀌지 않았다. `alt-yaml-V12-01`이 PASS가 됐고, 사례 4개를 등록했다: `alt-yaml-V12-04`(prefix 밖, block mapping 뒤 `\uFEFF---`, NO_ERROR)와 `probe-yaml-V12-NR-1~3`(prefix 밖 `\uFEFF   ---`, `\uFEFF\n  ---`, `\uFEFFkey: v`, ERROR). 알려진 한계: [211]은 prefix 밖에서도 BOM 뒤에 주석·빈 줄·`...`·입력 끝을 허용하지만, 이 scanner는 이 경우를 모두 오류로 둔다(patch 전에도 오류였다).
  - qualification inventory(`qualification-c1.json`)를 다시 생성했다(yaml `src/scanner.c` hash와 새 사례 4개).

- 버전 충돌 legacy 대안을 구문 합집합에서 뺐다(#91, #76 사용자 결정). 같은 byte가 채택 범위의 두 version에서 각각 유효한 다른 구문이라 grammar 하나로 함께 검증할 수 없는 대안이다: C++17 이전 `module`/`import` 줄(`cpp-L01.a13`), C++23 이전 `arr[1, 2]`(`cpp-L01.a18`), PHP 7.0에서 제거된 ASP·script tag(`php-L01.a006`·`a007`), 8.0 이전 `#[` 주석(`php-L01.a009`). 사양 인용은 [disposition 문서](docs/validation/language-feature-disposition.md#읽는-법과-완료-경계)에 있다.
  - 이 대안만 담던 사례 `alt-cpp-L01-06`·`09`, `alt-php-L01-05`를 뺐다. `alt-php-L01-04`는 여전히 유효한 `<?` short tag(`php-L01.a008`)만 남겼다.
  - 이후 version에서 ill-formed가 될 뿐인 형식(`import<int> f();`, `Y<operator<=> y;`, N3337 attribute 위치)은 충돌이 아니라 REQ로 남는다.

- 비채택 20 route도 고정 생성기 재생성 경로에 올렸다(#89, `C2-REGENERATE-r1`: #76에서 사용자가 승인한 26 route patch 경로의 첫 단계). grammar는 바꾸지 않았다.
  - 20 route의 native 입력 `parser.c`·`tree_sitter/*.h`는 이제 upstream에 들어 있는 이전 CLI 생성물이 아니다. 고정 commit의 patch 없는 grammar를 tree-sitter 0.27.0·Node 24.21.0·ABI 15로 재생성한 출력이다. `native-routes.json`의 `regeneration`은 patch chain과 patched file이 비어 있다. `language-sources.json`의 `regeneration` 기록과 `reproduction-routes.json`의 `PREPARE_GENERATED` 기준이 출력 6개를 모두 고정한다. Windows에서 20 route 모두 두 작업 공간 출력이 byte 단위로 같았고 S04 재생성 출력과도 같았다. php는 upstream 생성물과 같고, 나머지 19 route는 `array.h` 등이 다르다.
  - foundation 시험은 두 종류의 route만 받는다. 하나는 기존 규칙 그대로인 채택 route이고, 다른 하나는 adoption·patch 없이 출력이 고정된 재생성 전용 route다. 다음은 거부한다: 둘 다 아니거나 둘 다인 route, upstream parser, chain 없는 patched file, 빠지거나 다른 출력 고정, 재생성 기록이 있는 upstream 기준. 이 거부 사례마다 음성 대조 시험이 있다.
  - qualification inventory(`qualification-c1.json`)의 grammar 파일 hash를 등록부에서 다시 생성했다.
  - Windows amd64에서 등록 사례를 baseline(main 6d151e0)과 비교했다. 같은 사례와 같은 compiler를 썼고, 19 route는 새 parser로 build했다. 판정·code·claim·기대값 실패가 바뀐 사례는 없다(s05 2394, s06 2532). s05 step tree digest 3007개도 모두 같다. Linux·macOS의 native 결과는 이 변경 뒤 CI가 근거다.

- native CI의 요구 축과 kit 축을 분리했다(#76). 오늘의 grammar에서 정직하게 실패하는 등록 사례가 kit 결함처럼 job과 mechanism gate를 실패시켰다.
  - 오류 tree 위의 incremental route를 BLOCKED로 둔다. route 계측이 있고 바뀐 것이 있는데 incremental·fresh 어느 parse도 node를 재사용하지 않은 edit step에서, 이전 tree와 새 incremental tree가 모두 full tree이고 그중 하나가 `has_error`인 경우다. summary·record 형식 tree는 replay가 `has_error`를 다시 계산하지 못하므로 FAIL로 남는다. tree-sitter는 오류 tree에서도 node를 재사용할 수 있지만, grammar의 오류 tree에서는 재사용할 node가 남지 않을 수 있으므로 이 무재사용을 kit 결함이 아니라 관측 불가로 취급한다. code는 `INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_<n>`이고, 다른 실패 code가 없을 때만 사례 code가 된다. 깨끗한 tree, 계측 없음, 변경 없음, fresh 재사용은 그대로 FAIL이다. S05 판정, r2 replay reducer와 qualify의 `incremental-route` gate가 같은 규칙으로 다시 계산한다.
  - replay reducer `native-result-r2`·`oracle-set-r2`·`private-corpus-r2`를 더했다(#87). 각 r1과 같고 이 route 규칙만 더한다. r1은 그대로 남아 규칙 이전의 근거를 전과 같은 결과로 replay한다. reducer는 registration의 `reducer` id로 고른다. `tsgk qualify`는 새 CI 근거를 r2 규칙으로 판정한다.
  - qualify는 route BLOCKED를 `KIT_CLAIM_FAILED`로 세지 않는다. kit 축은 PASS로 남고 E 의무가 BLOCKED가 되므로 칸은 `INCOMPLETE`다. FULL PASS에는 여전히 E PASS가 필요하다. 기록과 다시 계산한 route claim이 다르면 gate 실패다.
  - `run-routes.ps1`은 oracle `query_expectations` FAIL·BLOCKED와 route BLOCKED로 job을 실패시키지 않는다. 이 결과는 요구 축 결과(Q는 `tsgk qualify`가 판정)로 보고, 실패 내용과 함께 `summary.json`의 `requirement_results`에 기록한다. 그 밖의 job 실패 조건은 그대로다. set 검증 실패, error finding, build 실패·거부, 완료되지 않은 사례, incremental equality FAIL·BLOCKED, route FAIL, query equality·fact reproduction·dynamic SQL FAIL·BLOCKED가 이에 해당한다.
  - 결과·record schema는 claim 값(`BLOCKED` 포함)과 code를 열거하지 않으므로 revision을 올리지 않았다.
  - 실제 route 결함이 오류 tree BLOCKED 뒤에 숨지 않도록, foundation 시험이 등록 route마다 route·gap 사례 파일(n461 제외)에 모든 step이 `NO_ERROR`인 edit 사례가 하나 이상 있는지 확인한다(#87). 지금 26 route 모두 충족한다.
  - 알려진 한계: SVC 관측 전용 기록은 parse하지 않으므로 route claim이 없고 route gate를 거치지 않는다(#87). 이런 사례는 route 근거가 되지 않는다.
- native `incremental_equality` claim이 앞 step의 FAIL 뒤에 비교 없는 step이 오면 BLOCKED로 덮이던 결함을 고쳤다(#76). 이제 replay처럼 step 중 가장 나쁜 값(FAIL > BLOCKED > PASS)이다.

- 단계 기대값에 anchor를 더했다(#76). `contains`는 같은 type이 다른 줄에 있어도 통과하므로, 대상 구조를 잘못 parse한 사례가 통과할 수 있었다. TypeScript generic tagged template이 `binary_expression`으로 parse됐는데도 다른 곳의 `call_expression` 때문에 통과한 것이 그 예다.
  - 기대값의 선택 필드 `anchors`(`{type, start_byte, end_byte}`)는 그 step의 full tree에 정확히 그 type·byte 범위의 named node가 있어야 통과한다. 없으면 FAIL, full tree가 아니면 BLOCKED다. S05 판정, replay, qualify가 같은 규칙으로 다시 계산한다. anchor가 없는 기대값의 판정은 그대로다.
  - profile이 `tsgk-incremental/r2`·`tsgk-oracle/r2`, inventory가 `tsgk-qualification-inventory/r3`이 됐다. 형식이 틀리거나 step source 밖인 anchor는 profile에서 `EXPECT_ANCHOR_INVALID`, inventory에서 `CASE_INVALID`다. r1 profile은 이전 근거를 replay할 수 있게 anchor 없이 계속 읽는다.
  - 원본 사례 파일은 anchor를 `{type, text, occurrence}`로 적는다. `run-routes.ps1`과 inventory 생성기가 같은 방식으로 byte 범위로 바꾸고, foundation test가 둘을 독립 구현과 대조한다. 아직 anchor를 단 등록 사례는 없다.
- `DeriveDeclarations`가 한 match 안에 같은 level의 node가 여럿일 때 마지막 node만 남기던 결함을 고쳤다. PostgreSQL `CREATE TABLE … PARTITION OF …`의 선언 이름이 이 결함 때문에 잘못 나왔다(#77).
- 개발 helper `run-routes.ps1`이 capture 기대값을 잘못 직렬화하던 결함을 고쳤다(#77).
  - capture가 하나이면 scalar가 되어 `JSON_TYPE`으로 거부됐다.
  - capture가 0개이면 `null`, 즉 "검사하지 않음"이 됐다.
- `tsgk qualify` 결과가 `tsgk-qualification-result/r2`가 됐다. r1에 platform별 지원 claim `platform_claims`(platform id → `SUPPORTED`|`BLOCKED`)를 더한다. platform은 근거 무결성(완결성 PASS, finding 없음), 그 platform의 모든 칸 kit 축·요구 축 PASS, 실행된 추가 역할 행 PASS일 때만 `SUPPORTED`다. platform 간 비교는 넣지 않는다. 전역 `support_claim`·`mechanism_gate`·assessment·exit는 그대로다.
- qualification inventory 사례에 `expect_assessment`·`expect_code`를 더했다. SVC 관측 전용이고 요구 행을 덮지 않는 사례만 등록할 수 있으며(그 밖은 `CASE_INVALID`), 기록된 판정과 code가 등록값과 같을 때만 등록 검사가 PASS이고 다르면 FAIL이다. `n461-svc`의 의도된 BLOCKED 8사례(`SVC_INLINE_UNRESOLVED` 4, `SVC_INLINE_UNSUPPORTED` 1, `SVC_INLINE_NOT_PARSED` 1, `SVC_DIRECTIVE_DIAGNOSTICS` 2)를 등록했다. 그 역할 행의 PASS는 다음 CI qualification 근거로 확인한다.
- coverage 규칙이 `tsgk-coverage-rule/r2`, inventory가 `tsgk-qualification-inventory/r2`, 결과가 `tsgk-qualification-result/r3`이 됐다(#76).
  - 행의 P 의무는 production 대안 등록부 `src/contracts/feature-alternatives.json`에서 그 행이 `COMPLETE`이고, 모든 대안을 그 대안을 적은(`alternatives`) P 사례가 덮을 때만 덮인다. 그 밖은 `NOT_COVERED`다. 결과의 P 의무에 `alternatives`·`alternatives_uncovered`를 더한다.
  - 등록부는 178개 REQ 행 전부를 `PENDING`, 대안 없음으로 시작한다. 따라서 지금은 모든 P 의무가 `NOT_COVERED`이고 그 칸은 PASS가 되지 않는다. 정책이 처음부터 요구한 경계를 반영한 것이다.
  - route별 `error_nodes`(`native-routes.json`, html은 `erroneous_end_tag`)를 등록했다. 이 node를 `contains`에 적은 `NO_ERROR` step은 N, 다른 구조 node가 함께 있으면 R을 만든다. P와 W는 만들지 않는다. 이런 route에서 P·W step은 기록된 full tree에 error node가 없어야 한다(`has_error`는 이 node를 보지 않는다). 있으면 FAIL이고, full tree가 아니면 BLOCKED다(#81).
  - W 생산자를 등록했다. `sample`(repository·commit·path·SPDX license·sha256·bytes)이 있는 요구 사례가 대상이다. license는 MIT·Apache-2.0·BSD-2/3-Clause·PostgreSQL이다. 크기는 65536 bytes 이하이고 step 0 tree는 10000 node 이하다. step 0이 `NO_ERROR`를 기대하는 사례가 W를 덮는다. sample마다 `src/testdata/native/samples/NOTICE.md`에 고지 항목이 있어야 한다.

## v0.1.0 — 2026-10-04

첫 release의 범위는 [제품 범위](docs/specs/scope.md)가 정하고, 근거와 한계는 [플랫폼 지원 계약](docs/specs/platform-support.md)에 있다. tag `v0.1.0`(`3625072`)과 GitHub Release로 소스만 발행했다. binary와 package는 없다.

### VERIFIED(Windows amd64, Linux amd64, macOS arm64)

- offline core와 그 공개 API(`src/kit`): `inspect`, `identity`, `verify`(디렉터리와 ZIP, 압축을 풀지 않음), `schema check`/`schema diff`. CGO-free이고 CLI와 API 결과 bytes가 같다.

### experimental(호환성 약속 없음)

- `corpus`(비공개 corpus inventory)
- `reproduce`(두 작업 공간 생성 비교)
- `incremental`·`oracle record`(native parse/edit/query 기록; 승인 capability와 host compiler 필요)
- `replay`·`evidence verify`
- `qualify`(26 route × 3 platform 집계)

### 상태

- 26 route 지원 claim은 **BLOCKED**다. 필수 feature 사례가 부족하고(#70) 채택 route에 grammar gap이 남아 있다. 최종 qualification 근거는 main `03ea1f0`의 CI run 37184449649이다.

### 알려진 한계

- tree-sitter runtime의 field 조회와 cursor가 다른 API claim FAIL 23 사례(6 route)
- T-SQL 과잉 수용(r6에서 3종, r5부터 2종)
- `--out`이 `subst`·bind mount 별칭을 검출하지 못함
- 동적 SQL 탐지는 첫 문장이 아닌 `;sp_executesql`을 known miss로 둠
- BrightScript·cooklang 역할 NOT_RUN, `n461-svc` 역할 INCOMPLETE(SVC negative 기대값 8개, #70)
- `n461-large` 대형 실사용 profile은 Windows에서만 실행(Linux·macOS NOT_APPLICABLE)
- 시험 시간 여유(#65)

### Session 이력

- Session 00: Go CGO-free foundation, 제품·검증 계약, 세 OS CI와 순차 개발 campaign을 준비한다.
- Session 01: `tsgk inspect`/`identity`/`corpus`와 공개 offline API `src/kit`를 구현한다. 다음을 포함한다.
  - manifest r2(판별 encoding 결속), E0 report
  - known-paths discovery와 정적 closure 관측
  - no-follow guard, 유한 한도, kit wall과 caller 취소의 구분
  - no-clobber `--out`
  - 비공개 corpus inventory(N461 역할, PRESENCE_ONLY, `.csproj` 선언 관측)
- Session 02~07: strict verify, schema, reproduce, incremental, oracle record, replay·evidence verify (각 Session 보고서 참고).
- Session 08: 다음을 더한다. 지원 claim은 BLOCKED다.
  - `tsgk qualify`와 `kit.Qualify`
  - qualification inventory(`src/contracts/qualification-c1.json`)
  - CI의 host 실행 identity·기록 set 보관과 qualification job
  - module proxy 소비자 시험
  - T-SQL patch r6
- release 후보 준비(#72):
  - `src/contracts/campaign-01.json`에 Session 통합 상태(`INTEGRATED`, PR, merge commit, post-merge run)를 기록하고 guard로 검사한다.
  - 플랫폼 지원 상태표를 현재 근거로 갱신한다.
  - Linux `race diagnostic` job을 추가한다(비필수, CGO 진단 lane).
